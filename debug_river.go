package peerdb

import (
	"net/http"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/waf"

	internalSite "gitlab.com/peerdb/peerdb/internal/site"
)

// DebugRiver handles requests under /debug/river by forwarding them to the
// site's River UI handler.
func (s *Service) DebugRiver(w http.ResponseWriter, req *http.Request, _ waf.Params) {
	if !s.Development {
		s.NotFoundWithError(w, req, errors.New("not in development mode"))
		return
	}

	site := waf.MustGetSite[*internalSite.Site](req.Context())
	if site.DebugRiverHandler == nil {
		s.InternalServerErrorWithError(w, req, errors.New("no River UI handler for site"))
		return
	}
	site.DebugRiverHandler.ServeHTTP(w, req)
}
