package peerdb

import (
	"net/http"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/waf"

	"gitlab.com/peerdb/peerdb/document"
	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
)

// DebugMappingGetAPI handles GET requests to serve generated ElasticSearch mapping for debugging.
func (s *Service) DebugMappingGetAPI(w http.ResponseWriter, req *http.Request, _ waf.Params) {
	if !s.Development {
		s.NotFoundWithError(w, req, errors.New("not in development mode"))
		return
	}

	indexConfiguration, errE := internalSearch.Mapping()
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

	var doc document.D
	errE := x.UnmarshalWithoutUnknownFields(dataJSON, &doc)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	site := waf.MustGetSite[*Site](req.Context())

	searchDoc, errE := site.Base.Bridge().Converter().FromDocument(req.Context(), &doc, metadata.InverseRelations)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	w.Header().Set("Version", version.String())

	s.WriteJSON(w, req, searchDoc, nil)
}
