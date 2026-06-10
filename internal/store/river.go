package store

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode"

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

// jobTimeout is the default River timeout for a job. It is short because most jobs are quick.
// Jobs that run longer by design override it with their own value via Worker.Timeout.
const jobTimeout = 1 * time.Minute

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
	if updateErr != nil && !errors.Is(updateErr, context.Canceled) && !errors.Is(updateErr, context.DeadlineExceeded) {
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

// River bundles a per-schema river client with its workers registry and the per-client job dispatchers.
// One River exists per site (per schema): the client polls only that schema's river tables, so workers
// registered on it only ever receive that site's jobs.
//
// All worker registration goes through RiverAddWorker or RiverDispatcher and must happen before Start:
// river does not support registering workers on a started client, so registration after Start is a hard
// failure.
type River struct {
	Client *river.Client[pgx.Tx]
	Schema string

	mu          sync.Mutex
	workers     *river.Workers
	started     bool
	dispatchers map[string]any
}

// Start starts the river client. After Start, registering workers (RiverAddWorker or RiverDispatcher)
// returns an error.
func (r *River) Start(ctx context.Context) errors.E {
	r.mu.Lock()
	r.started = true
	r.mu.Unlock()
	return errors.WithStack(r.Client.Start(ctx))
}

// errAfterStart returns the error for a registration attempted after Start. The caller must hold r.mu.
func (r *River) errAfterStart() errors.E {
	errE := errors.New("river workers cannot be registered after the river client was started")
	errors.Details(errE)["schema"] = r.Schema
	return errE
}

// validateJobArgs validates that the job args route jobs of their kind into the kind's queue: they must
// implement InsertOpts and set the queue to the kind's queue name. Because every kind runs in its own queue
// which only clients with the kind's worker fetch from, job args which do not route into that queue would
// produce jobs nobody ever works on. It returns the kind. The caller must hold r.mu.
func (r *River) validateJobArgs(args river.JobArgs) (string, errors.E) {
	kind := args.Kind()
	withOpts, ok := args.(river.JobArgsWithInsertOpts)
	if !ok {
		errE := errors.New("job args do not implement InsertOpts to set the job kind's queue")
		details := errors.Details(errE)
		details["schema"] = r.Schema
		details["kind"] = kind
		return "", errE
	}
	if opts := withOpts.InsertOpts(); opts.Queue != RiverQueueName(kind) {
		errE := errors.New("job args InsertOpts queue does not match the job kind's queue")
		details := errors.Details(errE)
		details["schema"] = r.Schema
		details["kind"] = kind
		details["expected"] = RiverQueueName(kind)
		details["got"] = opts.Queue
		return "", errE
	}
	return kind, nil
}

// RiverAddWorker registers a river worker with the client's workers and adds the job kind's queue (named
// after the kind, with the given queue configuration) to the client, so the client fetches jobs of this
// kind. Because every kind runs in its own queue and a queue is only added together with its worker, a
// client can never fetch a job of a kind it has no worker for. For the same reason the kind's job args must
// route jobs of this kind into that queue, which is validated here. It must be called before Start.
// Registration after the client was started is a hard failure because river does not support it.
func RiverAddWorker[T river.JobArgs](r *River, worker river.Worker[T], queueConfig river.QueueConfig) errors.E {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.started {
		return r.errAfterStart()
	}

	var args T
	kind, errE := r.validateJobArgs(args)
	if errE != nil {
		return errE
	}

	errE = r.addQueue(kind, queueConfig)
	if errE != nil {
		return errE
	}
	return errors.WithStack(river.AddWorkerSafely(r.workers, worker))
}

// RiverQueueName derives the river queue name for a job kind. Every job kind runs in its own queue. River
// allows only lower-case letters and numbers separated by underscores or hyphens in queue names, so the
// CamelCase kind is converted to snake_case, keeping an uppercase run together as one word.
func RiverQueueName(kind string) string {
	var b strings.Builder
	runes := []rune(kind)
	for i, r := range runes {
		if unicode.IsUpper(r) {
			if i > 0 && (!unicode.IsUpper(runes[i-1]) || (i+1 < len(runes) && unicode.IsLower(runes[i+1]))) {
				b.WriteRune('_')
			}
			b.WriteRune(unicode.ToLower(r))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// addQueue adds the job kind's queue to the client. The caller must hold r.mu.
func (r *River) addQueue(kind string, queueConfig river.QueueConfig) errors.E {
	err := r.Client.Queues().Add(RiverQueueName(kind), queueConfig)
	if err != nil {
		errE := errors.WithStack(err)
		details := errors.Details(errE)
		details["schema"] = r.Schema
		details["kind"] = kind
		return errE
	}
	return nil
}

// RiverDispatcher returns the per-client dispatcher for the job kind of T, creating it with create on the
// first call. A package which processes jobs of some kind calls this when registering a job source: the
// first call constructs the kind's worker through create and registers it together with the kind's queue
// (with the queue configuration create returns), and later calls return the same value, so multiple job
// sources of the same kind on one client (e.g. several stores with different prefixes in the same schema)
// share one worker which dispatches among them. Job args are validated like in RiverAddWorker. Like
// RiverAddWorker it must be called before Start, also because callers extend the returned dispatcher's
// state (e.g. its prefix table), which must not change once jobs can run.
func RiverDispatcher[T river.JobArgs, W river.Worker[T]](r *River, create func() (W, river.QueueConfig, errors.E)) (W, errors.E) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.started {
		return *new(W), r.errAfterStart()
	}

	var args T
	kind, errE := r.validateJobArgs(args)
	if errE != nil {
		return *new(W), errE
	}

	if d, ok := r.dispatchers[kind]; ok {
		w, ok := d.(W)
		if !ok {
			errE := errors.New("existing river dispatcher has unexpected type")
			details := errors.Details(errE)
			details["schema"] = r.Schema
			details["kind"] = kind
			details["got"] = fmt.Sprintf("%T", d)
			details["expected"] = fmt.Sprintf("%T", *new(W))
			return *new(W), errE
		}
		return w, nil
	}

	w, queueConfig, errE := create()
	if errE != nil {
		return *new(W), errE
	}
	errE = r.addQueue(kind, queueConfig)
	if errE != nil {
		return *new(W), errE
	}
	err := river.AddWorkerSafely(r.workers, w)
	if err != nil {
		return *new(W), errors.WithStack(err)
	}
	if r.dispatchers == nil {
		r.dispatchers = map[string]any{}
	}
	r.dispatchers[kind] = w
	return w, nil
}

// NewRiver creates a new River client and workers and initializes the database for it.
//
// withContext, when non-nil, establishes a per-job context logger so that a
// job's debug logs are buffered and only emitted when the job fails or panics.
func NewRiver(
	ctx context.Context, logger zerolog.Logger, withContext z.WithContextFunc, dbpool *pgxpool.Pool, schema string,
) (*River, errors.E) {
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
		// Every job kind runs in its own queue (named after the kind), added by RiverAddWorker or
		// RiverDispatcher when the kind's worker is registered, so a client only ever fetches jobs of
		// kinds it has workers for. The default queue is configured here only because river decides at
		// construction whether a client executes jobs at all (and refuses Queues().Add otherwise); no
		// jobs are inserted into it, so it idles.
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {
				MaxWorkers:        runtime.GOMAXPROCS(0),
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
		return nil, errors.WithStack(err)
	}

	migrator, err := rivermigrate.New(driver, &rivermigrate.Config{
		Line:   "main",
		Logger: l,
		Schema: schema,
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}

	_, err = migrator.Migrate(ctx, rivermigrate.DirectionUp, nil)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &River{
		Client:      riverClient,
		Schema:      schema,
		mu:          sync.Mutex{},
		workers:     workers,
		started:     false,
		dispatchers: map[string]any{},
	}, nil
}
