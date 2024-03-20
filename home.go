package peerdb

import (
	"net/http"

	"gitlab.com/tozd/waf"
)

// Home is a GET/HEAD HTTP request handler which returns HTML frontend for the home page.
func (s *Service) Home(w http.ResponseWriter, req *http.Request, _ waf.Params) {
	// During development Vite creates WebSocket connection. We always proxy it.
	if s.ProxyStaticTo != "" && hasConnectionUpgrade(req) {
		s.Proxy(w, req)
		return
	}

	// We have it hard-coded here because we have it hard-coded on the frontend as well.
	w.Header().Add("Link", "</context.json>; rel=preload; as=fetch; crossorigin=anonymous")
	w.WriteHeader(http.StatusEarlyHints)

	if s.ProxyStaticTo != "" {
		s.Proxy(w, req)
	} else {
		s.ServeStaticFile(w, req, "/index.html")
	}
}
