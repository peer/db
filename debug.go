package peerdb

import (
	"net/http"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/waf"

	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
)

// DebugMappingGetAPI handles GET requests to serve generated ElasticSearch mapping for debugging.
func (s *Service) DebugMappingGetAPI(w http.ResponseWriter, req *http.Request, _ waf.Params) {
	if !s.Development {
		s.NotFoundWithError(w, req, errors.New("not in development mode"))
		return
	}

	site := waf.MustGetSite[*Site](req.Context())
	indexConfiguration, errE := internalSearch.Mapping(site.LanguagePriority)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, indexConfiguration, nil)
}

// DebugIndexedGetAPI handles GET requests to return a document as it would be converted and indexed to ElasticSearch.
func (s *Service) DebugIndexedGetAPI(w http.ResponseWriter, req *http.Request, params waf.Params) {
	if !s.Development {
		s.NotFoundWithError(w, req, errors.New("not in development mode"))
		return
	}

	dataJSON, metadata, version, handled := s.documentGetData(w, req, params)
	if handled {
		return
	}

	site := waf.MustGetSite[*Site](req.Context())

	searchDoc, errE := site.Base.IndexedDocument(req.Context(), dataJSON, metadata)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	w.Header().Set("Version", version.String())

	s.WriteJSON(w, req, searchDoc, nil)
}
