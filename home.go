package search

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
)

// homeHTML is a GET/HEAD HTTP request handler which returns HTML frontend for the home page.
func (s *Service) homeHTML(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	if s.Development != "" {
		s.proxy(w, req)
	} else {
		// TODO
		http.Error(w, "501 not implemented", http.StatusNotImplemented)
	}
}
