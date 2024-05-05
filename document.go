package peerdb

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"
	"gitlab.com/tozd/waf"

	"gitlab.com/peerdb/peerdb/coordinator"
	"gitlab.com/peerdb/peerdb/document"
	internal "gitlab.com/peerdb/peerdb/internal/store"
	"gitlab.com/peerdb/peerdb/search"
	"gitlab.com/peerdb/peerdb/store"
)

// TODO: Support slug per document.

// DocumentGet is a GET/HEAD HTTP request handler which returns HTML frontend for a
// document given its ID as a parameter.
func (s *Service) DocumentGet(w http.ResponseWriter, req *http.Request, params waf.Params) {
	ctx := req.Context()
	metrics := waf.MustGetMetrics(ctx)

	id, errE := identifier.FromString(params["id"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"id" is not a valid identifier`))
		return
	}

	// We validate "s" and "q" parameters.
	if req.Form.Has("s") || req.Form.Has("q") {
		var q *string
		if req.Form.Has("q") {
			qq := req.Form.Get("q")
			q = &qq
		}

		m := metrics.Duration(internal.MetricSearchState).Start()
		sh := search.GetState(req.Form.Get("s"), q)
		m.Stop()
		if sh == nil {
			// Something was not OK, so we redirect to the URL without both "s" and "q".
			path, err := s.Reverse("DocumentGet", waf.Params{"id": id.String()}, url.Values{"tab": req.Form["tab"]})
			if err != nil {
				s.InternalServerErrorWithError(w, req, err)
				return
			}
			// TODO: Should we already do the query, to warm up store cache?
			//       Maybe we should cache response ourselves so that we do not hit store twice?
			w.Header().Set("Location", path)
			w.WriteHeader(http.StatusSeeOther)
			return
		} else if req.Form.Has("q") {
			// We redirect to the URL without "q".
			path, err := s.Reverse("DocumentGet", waf.Params{"id": id.String()}, url.Values{"s": {sh.ID.String()}, "tab": req.Form["tab"]})
			if err != nil {
				s.InternalServerErrorWithError(w, req, err)
				return
			}
			// TODO: Should we already do the query, to warm up store cache?
			//       Maybe we should cache response ourselves so that we do not hit store twice?
			w.Header().Set("Location", path)
			w.WriteHeader(http.StatusSeeOther)
			return
		}
	}

	var reqVersion *store.Version
	if req.Form.Has("version") {
		v, errE := store.VersionFromString(req.Form.Get("version")) //nolint:govet
		if errE != nil {
			s.BadRequestWithError(w, req, errE)
			return
		}
		reqVersion = &v
	}

	// TODO: If "s" is provided, should we validate that id is really part of search? Currently we do on the frontend.

	site := waf.MustGetSite[*Site](req.Context())

	m := metrics.Duration(internal.MetricDatabase).Start()
	// TODO: Add API to store to just check if the value exists.
	// TODO: To support "omni" instances, allow getting across multiple schemas.
	if reqVersion != nil {
		_, _, errE = site.store.Get(ctx, id, *reqVersion)
	} else {
		_, _, _, errE = site.store.GetLatest(ctx, id)
	}
	m.Stop()

	if errors.Is(errE, store.ErrValueNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.Home(w, req, nil)
}

// DocumentGetGet is a GET/HEAD HTTP request handler which returns a document given its ID as a parameter.
// It supports compression based on accepted content encoding and range requests.
func (s *Service) DocumentGetGet(w http.ResponseWriter, req *http.Request, params waf.Params) {
	ctx := req.Context()
	metrics := waf.MustGetMetrics(ctx)

	id, errE := identifier.FromString(params["id"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"id" is not a valid identifier`))
		return
	}

	// We do not check "s" and "q" parameters because the expectation is that
	// they are not provided with JSON request (because they are not used).

	var reqVersion *store.Version
	if req.Form.Has("version") {
		v, errE := store.VersionFromString(req.Form.Get("version")) //nolint:govet
		if errE != nil {
			s.BadRequestWithError(w, req, errE)
			return
		}
		reqVersion = &v
	}

	site := waf.MustGetSite[*Site](req.Context())

	var dataJSON json.RawMessage
	var metadataJSON json.RawMessage
	var version store.Version

	m := metrics.Duration(internal.MetricDatabase).Start()
	// TODO: To support "omni" instances, allow getting across multiple schemas.
	if reqVersion != nil {
		version = *reqVersion
		dataJSON, metadataJSON, errE = site.store.Get(ctx, id, *reqVersion)
	} else {
		dataJSON, metadataJSON, version, errE = site.store.GetLatest(ctx, id)
	}
	m.Stop()

	if errors.Is(errE, store.ErrValueNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	var metadata map[string]interface{}
	errE = x.Unmarshal(metadataJSON, &metadata)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	w.Header().Set("Version", version.String())

	// TODO: Requesting with version should be cached long, while without version it should be no-cache.
	w.Header().Set("Cache-Control", "max-age=604800")

	s.WriteJSON(w, req, dataJSON, metadata)
}

type documentCreateResponse struct {
	ID identifier.Identifier `json:"id"`
}

type documentMetadata struct {
	At time.Time `json:"at"`
}

type documentBeginMetadata struct {
	At      time.Time             `json:"at"`
	ID      identifier.Identifier `json:"id"`
	Version store.Version         `json:"version"`
}

type documentEndMetadata struct {
	At        time.Time              `json:"at"`
	Discarded bool                   `json:"discarded,omitempty"`
	Changeset *identifier.Identifier `json:"changeset,omitempty"`
}

func (s *Service) DocumentCreatePost(w http.ResponseWriter, req *http.Request, _ waf.Params) {
	defer req.Body.Close()
	defer io.Copy(io.Discard, req.Body) //nolint:errcheck

	ctx := req.Context()

	var ea emptyRequest
	errE := x.DecodeJSONWithoutUnknownFields(req.Body, &ea)
	if errE != nil {
		s.BadRequestWithError(w, req, errE)
		return
	}

	site := waf.MustGetSite[*Site](ctx)

	id := identifier.New()
	doc := document.D{
		CoreDocument: document.CoreDocument{
			ID:    id,
			Score: 1.0, // TODO How to make it configurable?
		},
	}
	data, errE := x.MarshalWithoutEscapeHTML(doc)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	metadata, errE := x.MarshalWithoutEscapeHTML(documentMetadata{
		At: time.Now().UTC(),
	})
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	_, errE = site.store.Insert(ctx, id, data, metadata)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, documentCreateResponse{ID: id}, nil)
}

// TODO: Include also the version of the document at the point of edit.
type documentBeginEditResponse struct {
	Session identifier.Identifier `json:"session"`
	Version store.Version         `json:"version"`
}

func (s *Service) DocumentBeginEditPost(w http.ResponseWriter, req *http.Request, params waf.Params) {
	defer req.Body.Close()
	defer io.Copy(io.Discard, req.Body) //nolint:errcheck

	ctx := req.Context()

	id, errE := identifier.FromString(params["id"])
	if errE != nil {
		s.BadRequestWithError(w, req, errE)
		return
	}

	var ea emptyRequest
	errE = x.DecodeJSONWithoutUnknownFields(req.Body, &ea)
	if errE != nil {
		s.BadRequestWithError(w, req, errE)
		return
	}

	site := waf.MustGetSite[*Site](ctx)

	_, _, version, errE := site.store.GetLatest(ctx, id)
	if errors.Is(errE, store.ErrValueNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	metadata, errE := x.MarshalWithoutEscapeHTML(documentBeginMetadata{
		At:      time.Now().UTC(),
		ID:      id,
		Version: version,
	})
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	session, errE := site.coordinator.Begin(ctx, metadata)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, documentBeginEditResponse{Session: session, Version: version}, nil)
}

func (s *Service) DocumentListChangesGet(w http.ResponseWriter, req *http.Request, params waf.Params) {
	ctx := req.Context()

	session, errE := identifier.FromString(params["session"])
	if errE != nil {
		s.BadRequestWithError(w, req, errE)
		return
	}

	site := waf.MustGetSite[*Site](ctx)

	// TODO: Support more than 5000 changes.
	changes, errE := site.coordinator.List(ctx, session, nil)
	if errors.Is(errE, coordinator.ErrSessionNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, coordinator.ErrAlreadyEnded) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, changes, nil)
}

func (s *Service) DocumentGetChangeGet(w http.ResponseWriter, req *http.Request, params waf.Params) {
	ctx := req.Context()

	session, errE := identifier.FromString(params["session"])
	if errE != nil {
		s.BadRequestWithError(w, req, errE)
		return
	}

	chunk, err := strconv.ParseInt(params["change"], 10, 64)
	if err != nil {
		s.BadRequestWithError(w, req, errors.WithStack(err))
		return
	}

	site := waf.MustGetSite[*Site](ctx)

	dataJSON, _, errE := site.coordinator.GetData(ctx, session, chunk)
	if errors.Is(errE, coordinator.ErrSessionNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, coordinator.ErrAlreadyEnded) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, coordinator.ErrOperationNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, dataJSON, nil)
}

type documentEndEditResponse struct {
	Changeset identifier.Identifier `json:"changeset"`
}

func (s *Service) DocumentEndEditPost(w http.ResponseWriter, req *http.Request, params waf.Params) {
	s.documentEndEdit(w, req, params, false)
}

func (s *Service) DocumentDiscardEditPost(w http.ResponseWriter, req *http.Request, params waf.Params) {
	s.documentEndEdit(w, req, params, true)
}

func (s *Service) documentEndEdit(w http.ResponseWriter, req *http.Request, params waf.Params, discard bool) {
	defer req.Body.Close()
	defer io.Copy(io.Discard, req.Body) //nolint:errcheck

	ctx := req.Context()

	session, errE := identifier.FromString(params["session"])
	if errE != nil {
		s.BadRequestWithError(w, req, errE)
		return
	}

	var ea emptyRequest
	errE = x.DecodeJSONWithoutUnknownFields(req.Body, &ea)
	if errE != nil {
		s.BadRequestWithError(w, req, errE)
		return
	}

	site := waf.MustGetSite[*Site](ctx)

	metadataJSON, errE := x.MarshalWithoutEscapeHTML(documentEndMetadata{
		At:        time.Now().UTC(),
		Discarded: discard,
		Changeset: nil,
	})
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	metadataJSON, errE = site.coordinator.End(ctx, session, metadataJSON)
	if errors.Is(errE, coordinator.ErrSessionNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, coordinator.ErrAlreadyEnded) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	if discard {
		s.WriteJSON(w, req, []byte(`{"success":true}`), nil)
		return
	}

	var metadata documentEndMetadata
	errE = x.UnmarshalWithoutUnknownFields(metadataJSON, &metadata)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, documentEndEditResponse{
		Changeset: *metadata.Changeset,
	}, nil)
}

func (s *Service) DocumentEdit(w http.ResponseWriter, req *http.Request, params waf.Params) {
	ctx := req.Context()

	id, errE := identifier.FromString(params["id"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"id" is not a valid identifier`))
		return
	}

	session, errE := identifier.FromString(params["session"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"session" is not a valid identifier`))
		return
	}

	site := waf.MustGetSite[*Site](req.Context())

	beginMetadataJSON, endMetadataJSON, errE := site.coordinator.Get(ctx, session)
	if errors.Is(errE, coordinator.ErrSessionNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	if endMetadataJSON != nil {
		s.NotFoundWithError(w, req, errors.WithStack(coordinator.ErrAlreadyEnded))
		return
	}

	var beginMetadata documentBeginMetadata
	errE = x.UnmarshalWithoutUnknownFields(beginMetadataJSON, &beginMetadata)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	if beginMetadata.ID != id {
		// TODO: Should we redirect to the correct ID?
		s.NotFoundWithError(w, req, errors.New(`"session" does not match "id"`))
		return
	}

	s.Home(w, req, nil)
}
