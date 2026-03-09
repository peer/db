package peerdb

import (
	"net/http"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/waf"

	"gitlab.com/peerdb/peerdb/internal/mapping"
)

// DebugMappingGetAPI handles GET requests to serve generated ElasticSearch mapping for debugging.
func (s *Service) DebugMappingGetAPI(w http.ResponseWriter, req *http.Request, _ waf.Params) {
	if !s.Development {
		s.NotFoundWithError(w, req, errors.New("not in development mode"))
		return
	}

	indexConfiguration, errE := mapping.Generate()
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, indexConfiguration, nil)
}
