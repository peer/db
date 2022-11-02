package search

import (
	"net/http"
	"strings"
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

func (s *Service) HomeGetAPIGet(w http.ResponseWriter, req *http.Request, _ Params) {
	s.staticFile(w, req, "/index.json", false)
}

// HomeGet is a GET/HEAD HTTP request handler which returns HTML frontend for the home page.
func (s *Service) HomeGet(w http.ResponseWriter, req *http.Request, _ Params) {
	// During development Vite creates WebSocket connection. We do not send early hints then.
	if strings.ToLower(req.Header.Get("Connection")) != "upgrade" {
		w.Header().Add("Link", "</api/>; rel=preload; as=fetch; crossorigin=anonymous")
		w.WriteHeader(http.StatusEarlyHints)
	}

	if s.Development != "" {
		s.Proxy(w, req, nil)
	} else {
		s.staticFile(w, req, "/index.html", false)
	}
}
