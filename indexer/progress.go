package indexer

import (
	"context"
	"time"

	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/x"
)

// Progress returns a function which logs progress.
func Progress(logger zerolog.Logger, description string, log func(event *zerolog.Event)) func(ctx context.Context, p x.Progress) {
	if description == "" {
		description = "progress"
	}
	return func(_ context.Context, p x.Progress) {
		e := logger.Info().
			Int64("count", p.Count).
			Int64("total", p.Size).
			// We format it ourselves. See: https://github.com/rs/zerolog/issues/709
			Str("eta", p.Remaining().Truncate(time.Second).String()).
			Float64("%", p.Percent())
		if log != nil {
			// Log additional fields.
			log(e)
		}
		e.Msg(description)
	}
}
