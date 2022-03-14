package search

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
)

// HomeGetHTML is a GET/HEAD HTTP request handler which returns HTML frontend for the home page.
func (s *Service) HomeGetHTML(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	if s.Development != "" {
		s.Proxy(w, req)
	} else {
		s.staticFile(w, req, "/index.html", false)
	}
}
