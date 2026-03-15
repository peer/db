package store

import (
	"context"
	"log/slog"
	"runtime"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	"github.com/riverqueue/river/rivertype"
	"github.com/rs/zerolog"
	slogzerolog "github.com/samber/slog-zerolog/v2"
	"gitlab.com/tozd/go/errors"
)

const jobTimeout = 15 * time.Minute

type riverErrorHandler struct {
	Logger zerolog.Logger
}

func (r riverErrorHandler) HandleError(_ context.Context, job *rivertype.JobRow, err error) *river.ErrorHandlerResult {
	e := r.Logger.Error().Err(err).
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
	return nil
}

func (r riverErrorHandler) HandlePanic(_ context.Context, job *rivertype.JobRow, panicVal any, trace string) *river.ErrorHandlerResult {
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
	return nil
}

// NewRiver creates a new River client and workers and initializes the database for it.
func NewRiver(
	ctx context.Context, logger zerolog.Logger, dbpool *pgxpool.Pool, schema string,
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

	workers := river.NewWorkers()
	riverClient, err := river.NewClient(riverpgxv5.New(dbpool), &river.Config{ //nolint:exhaustruct
		Workers: workers,
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {
				MaxWorkers:        runtime.GOMAXPROCS(0),
				FetchCooldown:     0,
				FetchPollInterval: 0,
			},
		},
		ErrorHandler: riverErrorHandler{
			Logger: logger,
		},
		JobTimeout: jobTimeout,
		Logger:     l,
		Schema:     schema,
	})
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}

	migrator, err := rivermigrate.New(riverpgxv5.New(dbpool), &rivermigrate.Config{
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
