package peerdb

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/rs/zerolog"
	slogzerolog "github.com/samber/slog-zerolog/v2"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/waf"
	"riverqueue.com/riverui"
)

// debugRiverPrefix is the URL path at which the River UI is mounted.
const debugRiverPrefix = "/debug/river"

// initDebugRiverHandler creates the site's River UI handler, mounted at
// debugRiverPrefix, and starts its background services. The services stop
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
		Prefix:                   debugRiverPrefix,
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

	s.debugRiverHandler = handler

	return nil
}

// DebugRiver handles requests under /debug/river by forwarding them to the
// site's River UI handler.
func (s *Service) DebugRiver(w http.ResponseWriter, req *http.Request, _ waf.Params) {
	if !s.Development {
		s.NotFoundWithError(w, req, errors.New("not in development mode"))
		return
	}

	site := waf.MustGetSite[*Site](req.Context())
	if site.debugRiverHandler == nil {
		s.InternalServerErrorWithError(w, req, errors.New("no River UI handler for site"))
		return
	}
	site.debugRiverHandler.ServeHTTP(w, req)
}
