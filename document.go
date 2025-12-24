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
	"gitlab.com/peerdb/peerdb/internal/types"
	"gitlab.com/peerdb/peerdb/search"
	"gitlab.com/peerdb/peerdb/store"
)

// TODO: Support slug per document.

// DocumentGet is a GET/HEAD HTTP request handler which returns HTML frontend for a
// document given its ID as a parameter.
func (s *Service) DocumentGet(w http.ResponseWriter, req *http.Request, params waf.Params) {
	ctx := req.Context()
	metrics := waf.MustGetMetrics(ctx)

	id, errE := identifier.MaybeString(params["id"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"id" is not a valid identifier`))
		return
	}

	// We validate the "s" parameter.
	if req.Form.Has("s") {
		m := metrics.Duration(internal.MetricSearchSession).Start()
		_, errE = search.GetSessionFromID(ctx, req.Form.Get("s"))
		m.Stop()
		if errors.Is(errE, search.ErrNotFound) {
			// Session not found, so we redirect to the URL without "s".
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
		} else if errE != nil {
			s.InternalServerErrorWithError(w, req, errE)
			return
		}
	}

	var reqVersion *store.Version
	if req.Form.Has("version") {
		v, errE := store.VersionFromString(req.Form.Get("version"))
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

	id, errE := identifier.MaybeString(params["id"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"id" is not a valid identifier`))
		return
	}

	// We do not check the "s" parameter because the expectation is that
	// it is not provided with JSON request (because it is not used).

	var reqVersion *store.Version
	if req.Form.Has("version") {
		v, errE := store.VersionFromString(req.Form.Get("version"))
		if errE != nil {
			s.BadRequestWithError(w, req, errE)
			return
		}
		reqVersion = &v
	}

	site := waf.MustGetSite[*Site](req.Context())

	var dataJSON json.RawMessage
	var version store.Version

	m := metrics.Duration(internal.MetricDatabase).Start()
	if reqVersion != nil {
		version = *reqVersion
		dataJSON, _, errE = site.store.Get(ctx, id, *reqVersion)
	} else {
		dataJSON, _, version, errE = site.store.GetLatest(ctx, id)
	}
	m.Stop()

	if errors.Is(errE, store.ErrValueNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	w.Header().Set("Version", version.String())

	// TODO: Requesting with version should be cached long, while without version it should be no-cache.
	w.Header().Set("Cache-Control", "max-age=604800")

	s.WriteJSON(w, req, dataJSON, nil)
}

type documentCreateResponse struct {
	ID identifier.Identifier `json:"id"`
}

// DocumentCreatePost handles POST requests to create a new document.
func (s *Service) DocumentCreatePost(w http.ResponseWriter, req *http.Request, _ waf.Params) {
	defer req.Body.Close()              //nolint:errcheck
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
			Score: 1.0,
		},
	}
	dataJSON, errE := x.MarshalWithoutEscapeHTML(doc)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	_, errE = site.store.Insert(ctx, id, dataJSON, &types.DocumentMetadata{
		At: types.Time(time.Now().UTC()),
	}, &types.NoMetadata{})
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, documentCreateResponse{ID: id}, nil)
}

type documentBeginEditResponse struct {
	Session identifier.Identifier `json:"session"`
	Version store.Version         `json:"version"`
}

// DocumentBeginEditPost handles POST requests to begin an edit session for a document.
func (s *Service) DocumentBeginEditPost(w http.ResponseWriter, req *http.Request, params waf.Params) {
	defer req.Body.Close()              //nolint:errcheck
	defer io.Copy(io.Discard, req.Body) //nolint:errcheck

	ctx := req.Context()

	id, errE := identifier.MaybeString(params["id"])
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

	metadata := &types.DocumentBeginMetadata{
		At:      types.Time(time.Now().UTC()),
		ID:      id,
		Version: version,
	}

	session, errE := site.coordinator.Begin(ctx, metadata)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, documentBeginEditResponse{Session: session, Version: version}, nil)
}

// DocumentSaveChangePost handles POST requests to save a change within an edit session.
func (s *Service) DocumentSaveChangePost(w http.ResponseWriter, req *http.Request, params waf.Params) {
	defer req.Body.Close()              //nolint:errcheck
	defer io.Copy(io.Discard, req.Body) //nolint:errcheck

	ctx := req.Context()

	session, errE := identifier.MaybeString(params["session"])
	if errE != nil {
		s.BadRequestWithError(w, req, errE)
		return
	}

	if !req.Form.Has("change") {
		s.BadRequestWithError(w, req, errors.New(`"change" query parameter is missing`))
		return
	}

	change, err := strconv.ParseInt(req.Form.Get("change"), 10, 64)
	if err != nil {
		s.BadRequestWithError(w, req, errors.WithStack(err))
		return
	}

	if change <= 0 {
		s.BadRequestWithError(w, req, errors.New(`non-positive "change" query parameter`))
		return
	}

	if req.ContentLength < 0 || req.ContentLength > maxPayloadSize {
		s.BadRequestWithError(w, req, errors.New("invalid content length"))
		return
	}

	buffer := make([]byte, req.ContentLength)
	_, err = io.ReadFull(req.Body, buffer)
	if err != nil {
		s.BadRequestWithError(w, req, errors.WithStack(err))
		return
	}

	// TODO: Validate the change.
	_, errE = document.ChangeUnmarshalJSON(buffer)
	if errE != nil {
		s.BadRequestWithError(w, req, errE)
		return
	}

	site := waf.MustGetSite[*Site](ctx)

	metadata := &types.DocumentChangeMetadata{
		At: types.Time(time.Now().UTC()),
	}

	_, errE = site.coordinator.Append(ctx, session, buffer, metadata, &change)
	if errors.Is(errE, coordinator.ErrSessionNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, coordinator.ErrAlreadyEnded) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, coordinator.ErrConflict) {
		waf.Error(w, req, http.StatusConflict)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, []byte(`{"success":true}`), nil)
}

// DocumentListChangesGet handles GET requests to list all changes in an edit session.
func (s *Service) DocumentListChangesGet(w http.ResponseWriter, req *http.Request, params waf.Params) {
	ctx := req.Context()

	session, errE := identifier.MaybeString(params["session"])
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

// DocumentGetChangeGet handles GET requests to retrieve a specific change from an edit session.
func (s *Service) DocumentGetChangeGet(w http.ResponseWriter, req *http.Request, params waf.Params) {
	ctx := req.Context()

	session, errE := identifier.MaybeString(params["session"])
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

// DocumentEndEditPost handles POST requests to finalize an edit session and commit changes.
func (s *Service) DocumentEndEditPost(w http.ResponseWriter, req *http.Request, params waf.Params) {
	s.documentEndEdit(w, req, params, false)
}

// DocumentDiscardEditPost handles POST requests to discard an edit session without committing changes.
func (s *Service) DocumentDiscardEditPost(w http.ResponseWriter, req *http.Request, params waf.Params) {
	s.documentEndEdit(w, req, params, true)
}

func (s *Service) documentEndEdit(w http.ResponseWriter, req *http.Request, params waf.Params, discard bool) {
	defer req.Body.Close()              //nolint:errcheck
	defer io.Copy(io.Discard, req.Body) //nolint:errcheck

	ctx := req.Context()

	session, errE := identifier.MaybeString(params["session"])
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

	metadata := &types.DocumentEndMetadata{
		At:        types.Time(time.Now().UTC()),
		Discarded: discard,
		Changeset: nil,
		Time:      0,
	}

	metadata, errE = site.coordinator.End(ctx, session, metadata)
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

	s.WriteJSON(w, req, documentEndEditResponse{
		Changeset: *metadata.Changeset,
	}, nil)
}

// DocumentEdit is a GET/HEAD HTTP request handler which returns HTML frontend for editing documents.
func (s *Service) DocumentEdit(w http.ResponseWriter, req *http.Request, params waf.Params) {
	ctx := req.Context()

	id, errE := identifier.MaybeString(params["id"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"id" is not a valid identifier`))
		return
	}

	session, errE := identifier.MaybeString(params["session"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"session" is not a valid identifier`))
		return
	}

	site := waf.MustGetSite[*Site](req.Context())

	beginMetadata, endMetadata, errE := site.coordinator.Get(ctx, session)
	if errors.Is(errE, coordinator.ErrSessionNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	if endMetadata != nil {
		s.NotFoundWithError(w, req, errors.WithStack(coordinator.ErrAlreadyEnded))
		return
	}

	if beginMetadata.ID != id {
		// TODO: Should we redirect to the correct ID?
		s.NotFoundWithError(w, req, errors.New(`"session" does not match "id"`))
		return
	}

	s.Home(w, req, nil)
}

// DocumentEditGet handles GET requests to retrieve metadata about a document edit session.
func (s *Service) DocumentEditGet(w http.ResponseWriter, req *http.Request, params waf.Params) {
	ctx := req.Context()

	id, errE := identifier.MaybeString(params["id"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"id" is not a valid identifier`))
		return
	}

	session, errE := identifier.MaybeString(params["session"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"session" is not a valid identifier`))
		return
	}

	site := waf.MustGetSite[*Site](req.Context())

	beginMetadata, endMetadata, errE := site.coordinator.Get(ctx, session)
	if errors.Is(errE, coordinator.ErrSessionNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	if endMetadata != nil {
		s.NotFoundWithError(w, req, errors.WithStack(coordinator.ErrAlreadyEnded))
		return
	}

	if beginMetadata.ID != id {
		// TODO: Should we redirect to the correct ID?
		s.NotFoundWithError(w, req, errors.New(`"session" does not match "id"`))
		return
	}

	s.WriteJSON(w, req, beginMetadata, nil)
}
