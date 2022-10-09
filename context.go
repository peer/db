package search

import (
	"net/http"
)

type siteContext struct {
	Site Site `json:"site"`
}

func (s *Service) getSiteContext(site Site) siteContext {
	return siteContext{
		Site: site,
	}
}

func (s *Service) ContextGetGetJSON(w http.ResponseWriter, req *http.Request, _ Params) {
	s.staticFile(w, req, "/context.json", false)
}
