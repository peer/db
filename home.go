package search

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
)

// HomeGetGetHTML is a GET/HEAD HTTP request handler which returns HTML frontend for the home page.
func (s *Service) HomeGetGetHTML(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	if s.Development != "" {
		s.Proxy(w, req)
	} else {
		s.staticFile(w, req, "/index.html", false)
	}
}
