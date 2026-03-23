package peerdb

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"
	"gitlab.com/tozd/waf"

	"gitlab.com/peerdb/peerdb/base"
	"gitlab.com/peerdb/peerdb/coordinator"
	"gitlab.com/peerdb/peerdb/document"
	internalStore "gitlab.com/peerdb/peerdb/internal/store"
	"gitlab.com/peerdb/peerdb/search"
	"gitlab.com/peerdb/peerdb/store"
)

// TODO: Support slug per document.

// DocumentGetGet is a GET/HEAD HTTP request handler which returns HTML frontend for a
// document given its ID as a parameter.
func (s *Service) DocumentGetGet(w http.ResponseWriter, req *http.Request, params waf.Params) {
	ctx := req.Context()
	metrics := waf.MustGetMetrics(ctx)

	id, errE := identifier.MaybeString(params["id"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"id" is not a valid identifier`))
		return
	}

	// We validate the "s" parameter.
	if req.Form.Has("s") {
		m := metrics.Duration(internalStore.MetricSearchSession).Start()
		_, errE = search.GetSessionFromID(ctx, req.Form.Get("s"))
		m.Stop()
		if errors.Is(errE, search.ErrNotFound) {
			// Session not found, so we redirect to the URL without "s".
			path, errE := s.Reverse("DocumentGet", waf.Params{"id": id.String()}, url.Values{"tab": req.Form["tab"]})
			if errE != nil {
				s.InternalServerErrorWithError(w, req, errE)
				return
			}
			// TODO: Should we already do the query, to warm up store cache?
			//       Maybe we should cache response ourselves so that we do not hit store twice?
			s.TemporaryRedirectGetMethod(w, req, path)
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

	m := metrics.Duration(internalStore.MetricDatabase).Start()
	// TODO: Add API to store to just check if the value exists.
	if reqVersion != nil {
		_, _, _, _, errE = site.Base.GetDocument(ctx, id, *reqVersion)
	} else {
		_, _, _, _, errE = site.Base.GetDocumentLatest(ctx, id)
	}
	m.Stop()

	if errors.Is(errE, store.ErrValueNotFound) {
		// This includes ErrValueDeleted, too.
		s.NotFoundWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.HomeGet(w, req, nil)
}

// DocumentGetGetAPI is a GET/HEAD HTTP request handler which returns a document given its ID as a parameter.
// It supports compression based on accepted content encoding and range requests.
func (s *Service) DocumentGetGetAPI(w http.ResponseWriter, req *http.Request, params waf.Params) {
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

	m := metrics.Duration(internalStore.MetricDatabase).Start()
	if reqVersion != nil {
		dataJSON, _, version, _, errE = site.Base.GetDocument(ctx, id, *reqVersion)
	} else {
		dataJSON, _, version, _, errE = site.Base.GetDocumentLatest(ctx, id)
	}
	m.Stop()

	if errors.Is(errE, store.ErrValueNotFound) {
		// This includes ErrValueDeleted, too.
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

// DocumentCreatePostAPI handles POST requests to create a new document.
func (s *Service) DocumentCreatePostAPI(w http.ResponseWriter, req *http.Request, _ waf.Params) {
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

	// TODO: Support configuring base and not just use the domain.
	base := []string{site.Domain, identifier.New().String()}
	id := identifier.From(base...)
	doc := &document.D{
		CoreDocument: document.CoreDocument{
			ID:   id,
			Base: base,
		},
	}

	errE = site.Base.InsertDocument(ctx, doc)
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

// DocumentBeginEditPostAPI handles POST requests to begin an edit session for a document.
func (s *Service) DocumentBeginEditPostAPI(w http.ResponseWriter, req *http.Request, params waf.Params) {
	defer req.Body.Close()              //nolint:errcheck
	defer io.Copy(io.Discard, req.Body) //nolint:errcheck

	ctx := req.Context()

	id, errE := identifier.MaybeString(params["id"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"id" is not a valid identifier`))
		return
	}

	var ea emptyRequest
	errE = x.DecodeJSONWithoutUnknownFields(req.Body, &ea)
	if errE != nil {
		s.BadRequestWithError(w, req, errE)
		return
	}

	site := waf.MustGetSite[*Site](ctx)

	session, version, errE := site.Base.BeginEditDocumentLatest(ctx, id)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, documentBeginEditResponse{Session: session, Version: version}, nil)
}

// DocumentSaveChangePostAPI handles POST requests to save a change within an edit session.
func (s *Service) DocumentSaveChangePostAPI(w http.ResponseWriter, req *http.Request, params waf.Params) {
	defer req.Body.Close()              //nolint:errcheck
	defer io.Copy(io.Discard, req.Body) //nolint:errcheck

	ctx := req.Context()

	session, errE := identifier.MaybeString(params["session"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"session" is not a valid identifier`))
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

	site := waf.MustGetSite[*Site](ctx)

	_, errE = site.Base.AppendDocumentChange(ctx, session, buffer, change)
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

// DocumentListChangesGetAPI handles GET requests to list all changes in an edit session.
func (s *Service) DocumentListChangesGetAPI(w http.ResponseWriter, req *http.Request, params waf.Params) {
	ctx := req.Context()

	session, errE := identifier.MaybeString(params["session"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"session" is not a valid identifier`))
		return
	}

	site := waf.MustGetSite[*Site](ctx)

	// TODO: Support more than 5000 changes.
	changes, errE := site.Base.ListDocumentChanges(ctx, session)
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

// DocumentGetChangeGetAPI handles GET requests to retrieve a specific change from an edit session.
func (s *Service) DocumentGetChangeGetAPI(w http.ResponseWriter, req *http.Request, params waf.Params) {
	ctx := req.Context()

	session, errE := identifier.MaybeString(params["session"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"session" is not a valid identifier`))
		return
	}

	chunk, err := strconv.ParseInt(params["change"], 10, 64)
	if err != nil {
		s.BadRequestWithError(w, req, errors.WithStack(err))
		return
	}

	site := waf.MustGetSite[*Site](ctx)

	dataJSON, errE := site.Base.GetDocumentChange(ctx, session, chunk)
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

// DocumentEndEditPostAPI handles POST requests to finalize an edit session and commit changes.
func (s *Service) DocumentEndEditPostAPI(w http.ResponseWriter, req *http.Request, params waf.Params) {
	s.documentEndEdit(w, req, params, false)
}

// DocumentDiscardEditPostAPI handles POST requests to discard an edit session without committing changes.
func (s *Service) DocumentDiscardEditPostAPI(w http.ResponseWriter, req *http.Request, params waf.Params) {
	s.documentEndEdit(w, req, params, true)
}

func (s *Service) documentEndEdit(w http.ResponseWriter, req *http.Request, params waf.Params, discard bool) {
	defer req.Body.Close()              //nolint:errcheck
	defer io.Copy(io.Discard, req.Body) //nolint:errcheck

	ctx := req.Context()

	session, errE := identifier.MaybeString(params["session"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"session" is not a valid identifier`))
		return
	}

	var ea emptyRequest
	errE = x.DecodeJSONWithoutUnknownFields(req.Body, &ea)
	if errE != nil {
		s.BadRequestWithError(w, req, errE)
		return
	}

	site := waf.MustGetSite[*Site](ctx)

	errE = site.Base.EndEditDocument(ctx, session, discard)
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

	s.WriteJSON(w, req, []byte(`{"success":true}`), nil)
}

// DocumentEditGet is a GET/HEAD HTTP request handler which returns HTML frontend for editing documents.
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

	documentID, _, completeMetadata, errE := site.Base.GetEditDocumentSession(ctx, session)
	if errors.Is(errE, coordinator.ErrSessionNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	if documentID != id {
		// TODO: Should we redirect to the correct ID?
		s.NotFoundWithError(w, req, errors.New(`"session" does not match "id"`))
		return
	}

	if completeMetadata != nil {
		path, errE := s.Reverse("DocumentGet", waf.Params{"id": id.String()}, nil)
		if errE != nil {
			s.InternalServerErrorWithError(w, req, errE)
			return
		}
		s.TemporaryRedirectGetMethod(w, req, path)
		return
	}

	s.HomeGet(w, req, nil)
}

// DocumentEditGetAPI handles GET requests to retrieve metadata about a document edit session.
func (s *Service) DocumentEditGetAPI(w http.ResponseWriter, req *http.Request, params waf.Params) {
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

	documentID, sessionEnded, completeMetadata, errE := site.Base.GetEditDocumentSession(ctx, session)
	if errors.Is(errE, coordinator.ErrSessionNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	if documentID != id {
		// TODO: Should we redirect to the correct ID?
		s.NotFoundWithError(w, req, errors.New(`"session" does not match "id"`))
		return
	}

	if sessionEnded {
		s.WriteJSON(w, req, `{"active":false}`, nil)
	} else if completeMetadata != nil {
		s.WriteJSON(w, req, struct {
			*base.DocumentCompleteMetadata

			Active bool `json:"active"`
		}{
			DocumentCompleteMetadata: completeMetadata,
			Active:                   false,
		}, nil)
	} else {
		s.WriteJSON(w, req, `{"active":true}`, nil)
	}
}

// changesetChangesGetAPI is a shared helper for listing changes in a changeset.
func (s *Service) changesetChangesGetAPI(
	w http.ResponseWriter, req *http.Request, params waf.Params,
	getChanges func(ctx context.Context, changesetID identifier.Identifier, after *identifier.Identifier) ([]store.Change, errors.E),
) {
	ctx := req.Context()

	changesetID, errE := identifier.MaybeString(params["changeset"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"changeset" is not a valid identifier`))
		return
	}

	var after *identifier.Identifier
	if req.Form.Has("after") {
		a, errE := identifier.MaybeString(req.Form.Get("after"))
		if errE != nil {
			s.BadRequestWithError(w, req, errors.WithMessage(errE, `"after" is not a valid identifier`))
			return
		}
		after = &a
	}

	changes, errE := getChanges(ctx, changesetID, after)
	if errors.Is(errE, store.ErrChangesetNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, store.ErrValueNotFound) {
		// This happens when "after" is not found.
		s.NotFoundWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, changes, nil)
}

// DocumentChangesGetAPI handles GET requests to list changes in a document changeset.
func (s *Service) DocumentChangesGetAPI(w http.ResponseWriter, req *http.Request, params waf.Params) {
	s.changesetChangesGetAPI(w, req, params, func(ctx context.Context, changesetID identifier.Identifier, after *identifier.Identifier) ([]store.Change, errors.E) {
		return waf.MustGetSite[*Site](ctx).Base.GetDocumentChanges(ctx, changesetID, after)
	})
}

// DocumentChangesGetGetAPI handles GET requests to retrieve a document from a changeset.
func (s *Service) DocumentChangesGetGetAPI(w http.ResponseWriter, req *http.Request, params waf.Params) {
	ctx := req.Context()

	changesetID, errE := identifier.MaybeString(params["changeset"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"changeset" is not a valid identifier`))
		return
	}

	id, errE := identifier.MaybeString(params["id"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"id" is not a valid identifier`))
		return
	}

	site := waf.MustGetSite[*Site](ctx)

	// Revision 0 means latest revision.
	dataJSON, _, version, _, errE := site.Base.GetDocumentFromChangeset(ctx, changesetID, id, 0)
	if errors.Is(errE, store.ErrValueNotFound) {
		// This includes ErrValueDeleted, too.
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, store.ErrChangesetNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	w.Header().Set("Version", version.String())

	s.WriteJSON(w, req, dataJSON, nil)
}
