package peerdb

import (
	"net/http"

	internalSite "gitlab.com/peerdb/peerdb/internal/site"
	"gitlab.com/peerdb/peerdb/store"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"
	"gitlab.com/tozd/waf"

	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
)

// DebugMappingGetAPI handles GET requests to serve generated ElasticSearch mapping for debugging.
func (s *Service) DebugMappingGetAPI(w http.ResponseWriter, req *http.Request, _ waf.Params) {
	if !s.Development {
		s.NotFoundWithError(w, req, errors.New("not in development mode"))
		return
	}

	site := waf.MustGetSite[*internalSite.Site](req.Context())
	indexConfiguration, errE := internalSearch.Mapping(site.LanguagePriority)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, indexConfiguration, nil)
}

// DebugIndexedGetAPI handles GET requests to return a document as it would be converted and indexed to ElasticSearch.
//
// The document is first fetched normally, through the read-path document hooks, so the endpoint enforces the
// same access as reading the document. The indexed form is then produced the way indexing produces it: from
// the raw stored document (the read-path hooks are not run during indexing), passed through the indexing
// hooks inside IndexedDocument and the conversion.
func (s *Service) DebugIndexedGetAPI(w http.ResponseWriter, req *http.Request, params waf.Params) {
	if !s.Development {
		s.NotFoundWithError(w, req, errors.New("not in development mode"))
		return
	}

	ctx := req.Context()

	_, _, version, handled := s.documentGetData(w, req, params)
	if handled {
		return
	}

	// documentGetData validated the parameter, so this cannot fail.
	id, errE := identifier.MaybeString(params["id"])
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	site := waf.MustGetSite[*internalSite.Site](ctx)

	dataJSON, metadata, _, _, errE := site.Base.Documents().Get(ctx, id, version)
	if errors.Is(errE, store.ErrValueNotFound) {
		// This includes ErrValueDeleted, too.
		s.NotFoundWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	searchDoc, errE := site.Base.IndexedDocument(ctx, dataJSON, metadata)
	if errors.Is(errE, store.ErrAccessDenied) {
		s.ForbiddenWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	w.Header().Set("Version", version.String())

	s.WriteJSON(w, req, searchDoc, nil)
}
