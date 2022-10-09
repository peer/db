package search

import (
	"net/http"
)

type siteContext struct {
	Site           Site   `json:"site"`
	Version        string `json:"version,omitempty"`
	BuildTimestamp string `json:"buildTimestamp,omitempty"`
	Revision       string `json:"revision,omitempty"`
}

func (s *Service) getSiteContext(site Site) siteContext {
	return siteContext{
		Site:           site,
		Version:        s.Version,
		BuildTimestamp: s.BuildTimestamp,
		Revision:       s.Revision,
	}
}

func (s *Service) ContextGetGetJSON(w http.ResponseWriter, req *http.Request, _ Params) {
	s.staticFile(w, req, "/context.json", false)
}
