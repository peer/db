package peerdb

import (
	"net/http"

	"gitlab.com/tozd/waf"
)

// License serves the LICENSE file to the client.
func (s *Service) License(w http.ResponseWriter, req *http.Request, _ waf.Params) {
	if s.ProxyStaticTo != "" {
		// This really serves the LICENSE file from the root directory and not /public/LICENSE.txt,
		// but that is fine, they are the same (they are symlinked).
		s.Proxy(w, req)
	} else {
		s.ServeStaticFile(w, req, "/LICENSE.txt")
	}
}

// Notice serves the NOTICE file to the client.
func (s *Service) Notice(w http.ResponseWriter, req *http.Request, _ waf.Params) {
	if s.ProxyStaticTo != "" {
		// rollup-plugin-license does not make the file available during development,
		// so we just return empty response.
		w.WriteHeader(http.StatusOK)
	} else {
		s.ServeStaticFile(w, req, "/NOTICE.txt")
	}
}
