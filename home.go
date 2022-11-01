package search

import (
	"net/http"
)

// HomeGetGetHTML is a GET/HEAD HTTP request handler which returns HTML frontend for the home page.
func (s *Service) HomeGetGetHTML(w http.ResponseWriter, req *http.Request, _ Params) {
	w.Header().Add("Link", "</context.json>; rel=preload; as=fetch; crossorigin=anonymous")
	w.WriteHeader(http.StatusEarlyHints)

	if s.Development != "" {
		s.Proxy(w, req, nil)
	} else {
		s.staticFile(w, req, "/index.html", false)
	}
}
