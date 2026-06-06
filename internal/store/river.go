package store

import (
	"context"
	"log/slog"
	"runtime"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	"github.com/riverqueue/river/rivertype"
	"github.com/rs/zerolog"
	slogzerolog "github.com/samber/slog-zerolog/v2"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	z "gitlab.com/tozd/go/zerolog"
)

const jobTimeout = 15 * time.Minute

// MetadataKeyError is the job metadata key under which a structured JSON
// representation of the job's error is stored when HandleError fires.
const MetadataKeyError = "error"

// MetadataKeyPanic is the job metadata key under which a structured JSON
// representation of a panic value is stored when HandlePanic fires and the
// panic value is an error.
const MetadataKeyPanic = "panic"

type riverErrorHandler struct {
	Logger zerolog.Logger
	Driver *riverpgxv5.Driver
	Schema string
}

// storeErrorMetadata merges a JSON representation of err into the job's
// metadata under the given key. It logs but does not propagate marshal or
// update failures.
func (r riverErrorHandler) storeErrorMetadata(ctx context.Context, job *rivertype.JobRow, key string, err any) {
	var val any
	switch v := err.(type) {
	case error:
		val = errors.Formatter{Error: v}
	default:
		val = v
	}

	metadataJSON, errE := x.MarshalWithoutEscapeHTML(map[string]any{key: val})
	if errE != nil {
		r.Logger.Error().Err(errE).Int64("id", job.ID).Msgf("failed to marshal %s metadata", key)
		return
	}

	_, updateErr := r.Driver.GetExecutor().JobUpdate(ctx, &riverdriver.JobUpdateParams{
		ID:              job.ID,
		MetadataDoMerge: true,
		Metadata:        metadataJSON,
		Schema:          r.Schema,
	})
	if updateErr != nil {
		r.Logger.Error().Err(errors.WithStack(updateErr)).Int64("id", job.ID).Msgf("failed to store %s metadata", key)
	}
}

func (r riverErrorHandler) HandleError(ctx context.Context, job *rivertype.JobRow, err error) *river.ErrorHandlerResult {
	// A job that still has attempts left will be retried, so we log it as a warning and reserve the
	// error level for the final attempt, matching how River logs the same outcome.
	var e *zerolog.Event
	if job.Attempt >= job.MaxAttempts {
		e = r.Logger.Error()
	} else {
		e = r.Logger.Warn()
	}
	e = e.Err(err).
		Int64("id", job.ID).
		Int("attempt", job.Attempt).
		Time("createdAt", job.CreatedAt).
		Time("scheduledAt", job.ScheduledAt).
		Str("kind", job.Kind).
		Str("queue", job.Queue).
		Int("priority", job.Priority).
		Int("maxAttempts", job.MaxAttempts).
		RawJSON("args", job.EncodedArgs)
	if job.AttemptedAt != nil {
		e = e.Time("attemptedAt", *job.AttemptedAt)
	}
	if job.FinalizedAt != nil {
		e = e.Time("finalizedAt", *job.FinalizedAt)
	}
	if len(job.Tags) > 0 {
		e = e.Strs("tags", job.Tags)
	}
	e.Msg("job error")

	r.storeErrorMetadata(ctx, job, MetadataKeyError, err)
	return nil
}

func (r riverErrorHandler) HandlePanic(ctx context.Context, job *rivertype.JobRow, panicVal any, trace string) *river.ErrorHandlerResult {
	e := r.Logger.Error().
		Int64("id", job.ID).
		Int("attempt", job.Attempt).
		Time("createdAt", job.CreatedAt).
		Time("scheduledAt", job.ScheduledAt).
		Str("kind", job.Kind).
		Str("queue", job.Queue).
		Int("priority", job.Priority).
		Int("maxAttempts", job.MaxAttempts).
		RawJSON("args", job.EncodedArgs).
		Str("trace", trace)
	if job.AttemptedAt != nil {
		e = e.Time("attemptedAt", *job.AttemptedAt)
	}
	if job.FinalizedAt != nil {
		e = e.Time("finalizedAt", *job.FinalizedAt)
	}
	if len(job.Tags) > 0 {
		e = e.Strs("tags", job.Tags)
	}
	switch v := panicVal.(type) {
	case string:
		e = e.Str("panic", v)
	case error:
		e = e.Err(v)
	default:
		e = e.Interface("panic", v)
	}
	e.Msg("job panic")

	r.storeErrorMetadata(ctx, job, MetadataKeyPanic, panicVal)
	return nil
}

// jobLoggingMiddleware wraps each job's work in a per-job context logger so that the job's debug logs
// are buffered and only flushed when the job fails or panics, the same way requests are logged. It is
// a no-op when WithContext is nil.
type jobLoggingMiddleware struct {
	river.MiddlewareDefaults

	WithContext z.WithContextFunc
}

// Work implements rivertype.WorkerMiddleware.
func (m *jobLoggingMiddleware) Work(ctx context.Context, _ *rivertype.JobRow, doInner func(context.Context) error) error {
	if m.WithContext == nil {
		return doInner(ctx)
	}

	ctx, closeCtx, trigger := m.WithContext(ctx)
	// closeCtx is deferred first so it runs last (after any flush below), discarding the buffer when
	// the job did not fail.
	defer closeCtx()

	// A worker panic unwinds through this middleware (River recovers it above us), so flush the
	// buffered debug while it is still unwinding, before closeCtx discards it.
	panicking := true
	defer func() {
		if panicking {
			// We have to call trigger ourselves because HandlePanic logging to error level
			// is called outside of current's ctx context logger and does not trigger it.
			trigger()
		}
	}()

	err := doInner(ctx)
	panicking = false

	if jobFailed(err) {
		// We have to call trigger ourselves because HandleError logging to error level
		// is called outside of current's ctx context logger and does not trigger it.
		trigger()
	}
	return err
}

// jobFailed reports whether err is a genuine job failure whose buffered debug should be flushed. It
// excludes River's control sentinels: a snooze is normal retry-later flow, and a deliberate cancel
// with no wrapped error (river.JobCancel(nil)) is not a failure. A cancel that wraps an error
// (river.JobCancel(err)) is treated as a failure so its debug is kept.
func jobFailed(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, &rivertype.JobSnoozeError{}) {
		return false
	}
	if cancel, ok := errors.AsType[*rivertype.JobCancelError](err); ok {
		e := cancel.Unwrap()
		if e == nil {
			return false
		} else if errors.Is(e, context.Canceled) {
			// We do mark job as failed on context.DeadlineExceeded, but not on context.Canceled.
			// The former might mean that the job took too long too run and was killed and it might
			// be useful to have more information what inside it took too long.
			return false
		}
		return true
	}
	return true
}

// NewRiver creates a new River client and workers and initializes the database for it.
//
// withContext, when non-nil, establishes a per-job context logger so that a
// job's debug logs are buffered and only emitted when the job fails or panics.
func NewRiver(
	ctx context.Context, logger zerolog.Logger, withContext z.WithContextFunc, dbpool *pgxpool.Pool, schema string,
) (*river.Client[pgx.Tx], *river.Workers, errors.E) {
	l := slog.New(slogzerolog.Option{
		Level:           slogzerolog.ZeroLogLeveler{Logger: &logger},
		Logger:          &logger,
		NoTimestamp:     true,
		Converter:       nil,
		AttrFromContext: nil,
		AddSource:       false,
		ReplaceAttr:     nil,
	}.NewZerologHandler())

	driver := riverpgxv5.New(dbpool)

	workers := river.NewWorkers()
	riverClient, err := river.NewClient(driver, &river.Config{ //nolint:exhaustruct
		Workers: workers,
		Middleware: []rivertype.Middleware{
			&jobLoggingMiddleware{MiddlewareDefaults: river.MiddlewareDefaults{}, WithContext: withContext},
		},
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {
				MaxWorkers:        runtime.GOMAXPROCS(0),
				FetchCooldown:     0,
				FetchPollInterval: 0,
			},
			// We use a single worker for the bridge queue so that its jobs are run sequentially.
			"bridge": {
				MaxWorkers:        1,
				FetchCooldown:     0,
				FetchPollInterval: 0,
			},
		},
		ErrorHandler: riverErrorHandler{
			Logger: logger,
			Driver: driver,
			Schema: schema,
		},
		JobTimeout: jobTimeout,
		Logger:     l,
		Schema:     schema,
	})
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}

	migrator, err := rivermigrate.New(driver, &rivermigrate.Config{
		Line:   "main",
		Logger: l,
		Schema: schema,
	})
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}

	_, err = migrator.Migrate(ctx, rivermigrate.DirectionUp, nil)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}

	return riverClient, workers, nil
}
