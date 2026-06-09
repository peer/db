package site

import (
	"context"
	"log/slog"

	"github.com/rs/zerolog"
	slogzerolog "github.com/samber/slog-zerolog/v2"
	"gitlab.com/tozd/go/errors"
	"riverqueue.com/riverui"
)

// DebugRiverPrefix is the URL path at which the River UI is mounted.
const DebugRiverPrefix = "/debug/river"

// initDebugRiverHandler creates the site's River UI handler, mounted at
// DebugRiverPrefix, and starts its background services. The services stop
// when ctx is cancelled.
func (s *Site) initDebugRiverHandler(ctx context.Context, logger zerolog.Logger) errors.E {
	l := slog.New(slogzerolog.Option{
		Level:           slogzerolog.ZeroLogLeveler{Logger: &logger},
		Logger:          &logger,
		NoTimestamp:     true,
		Converter:       nil,
		AttrFromContext: nil,
		AddSource:       false,
		ReplaceAttr:     nil,
	}.NewZerologHandler())

	handler, err := riverui.NewHandler(&riverui.HandlerOpts{
		Endpoints:                riverui.NewEndpoints(s.RiverClient, nil),
		Logger:                   l,
		Prefix:                   DebugRiverPrefix,
		DevMode:                  false,
		JobListHideArgsByDefault: false,
		LiveFS:                   false,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	err = handler.Start(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	s.DebugRiverHandler = handler

	return nil
}
