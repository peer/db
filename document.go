package peerdb

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"

	internalSite "gitlab.com/peerdb/peerdb/internal/site"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"
	"gitlab.com/tozd/waf"

	"gitlab.com/peerdb/peerdb/auth"
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
		if errors.Is(errE, search.ErrNotFound) || errors.Is(errE, auth.ErrAccessDenied) {
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

	site := waf.MustGetSite[*internalSite.Site](req.Context())

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
	} else if errors.Is(errE, auth.ErrAccessDenied) {
		s.ForbiddenWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.HomeGet(w, req, nil)
}

// DocumentDeleteGet is a GET/HEAD HTTP request handler which returns HTML frontend for the
// document deletion confirmation page given its ID as a parameter. It requires the delete
// permission on the document and that the document exists, so the page is not shown when the
// action is not possible.
func (s *Service) DocumentDeleteGet(w http.ResponseWriter, req *http.Request, params waf.Params) {
	ctx := req.Context()
	metrics := waf.MustGetMetrics(ctx)

	id, errE := identifier.MaybeString(params["id"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"id" is not a valid identifier`))
		return
	}

	site := waf.MustGetSite[*internalSite.Site](req.Context())

	m := metrics.Duration(internalStore.MetricDatabase).Start()
	doc, _, _, _, errE := site.Base.GetDocumentLatestDoc(ctx, id) //nolint:dogsled
	m.Stop()

	if errors.Is(errE, store.ErrValueNotFound) {
		// This includes ErrValueDeleted, too.
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, auth.ErrAccessDenied) {
		s.ForbiddenWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	errE = s.HasDocumentPermission(ctx, auth.ActionDelete, doc)
	if errE != nil {
		s.ForbiddenWithError(w, req, errE)
		return
	}

	s.HomeGet(w, req, nil)
}

// documentGetData is a shared helper that validates the document ID and version parameters,
// retrieves the document from the store, and returns the raw JSON data and metadata.
func (s *Service) documentGetData(
	w http.ResponseWriter, req *http.Request, params waf.Params,
) (json.RawMessage, *store.DocumentMetadata, store.Version, bool) { //nolint:unparam
	ctx := req.Context()
	metrics := waf.MustGetMetrics(ctx)

	id, errE := identifier.MaybeString(params["id"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"id" is not a valid identifier`))
		return nil, nil, store.Version{}, true
	}

	// We do not check the "s" parameter because the expectation is that
	// it is not provided with JSON request (because it is not used).

	var reqVersion *store.Version
	if req.Form.Has("version") {
		v, errE := store.VersionFromString(req.Form.Get("version"))
		if errE != nil {
			s.BadRequestWithError(w, req, errE)
			return nil, nil, store.Version{}, true
		}
		reqVersion = &v
	}

	site := waf.MustGetSite[*internalSite.Site](req.Context())

	var dataJSON json.RawMessage
	var metadata *store.DocumentMetadata
	var version store.Version

	m := metrics.Duration(internalStore.MetricDatabase).Start()
	if reqVersion != nil {
		dataJSON, metadata, version, _, errE = site.Base.GetDocument(ctx, id, *reqVersion)
	} else {
		dataJSON, metadata, version, _, errE = site.Base.GetDocumentLatest(ctx, id)
	}
	m.Stop()

	if errors.Is(errE, store.ErrValueNotFound) {
		// This includes ErrValueDeleted, too.
		s.NotFoundWithError(w, req, errE)
		return nil, nil, store.Version{}, true
	} else if errors.Is(errE, auth.ErrAccessDenied) {
		s.ForbiddenWithError(w, req, errE)
		return nil, nil, store.Version{}, true
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return nil, nil, store.Version{}, true
	}

	return dataJSON, metadata, version, false
}

// DocumentGetGetAPI is a GET/HEAD HTTP request handler which returns a document given its ID as a parameter.
// It supports compression based on accepted content encoding and range requests.
func (s *Service) DocumentGetGetAPI(w http.ResponseWriter, req *http.Request, params waf.Params) {
	dataJSON, _, version, handled := s.documentGetData(w, req, params)
	if handled {
		return
	}

	w.Header().Set("Version", version.String())

	// TODO: Requesting with version should be cached long, while without version it should be no-cache.
	w.Header().Set("Cache-Control", "no-cache")

	s.WriteJSON(w, req, dataJSON, nil)
}

// documentHistoryItem is one entry in a document's changeset history: the changeset that
// produced this version, the version string used to link to the document at that version,
// the timestamp of the change, and the users who contributed to it.
type documentHistoryItem struct {
	Changeset identifier.Identifier `json:"changeset"`
	Version   store.Version         `json:"version"`
	At        store.Time            `json:"at"`
	Authors   []store.User          `json:"authors,omitempty"`
}

// DocumentHistoryGetAPI handles GET requests to list a document's changeset history. Entries are
// returned newest first, one store page at a time (with optional "after" keyset pagination), each
// carrying the timestamp, the contributing users, and the version for linking to that revision.
// Only versions the caller can open are listed: all of them when the caller holds the historic read
// action on the document, and otherwise the versions readable at their own state, so a page can be
// shorter than the store page.
func (s *Service) DocumentHistoryGetAPI(w http.ResponseWriter, req *http.Request, params waf.Params) {
	ctx := req.Context()
	metrics := waf.MustGetMetrics(ctx)

	id, errE := identifier.MaybeString(params["id"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"id" is not a valid identifier`))
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

	site := waf.MustGetSite[*internalSite.Site](ctx)

	// We confirm the document exists and is accessible to the caller (same access semantics as
	// viewing it, including the read action on its latest version) before exposing its history.
	m := metrics.Duration(internalStore.MetricDatabase).Start()
	doc, _, _, _, errE := site.Base.GetDocumentLatestDoc(ctx, id) //nolint:dogsled
	m.Stop()
	if errors.Is(errE, store.ErrValueNotFound) {
		// This includes ErrValueDeleted, too.
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, auth.ErrAccessDenied) {
		s.ForbiddenWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	// With the historic read action on the document every version is openable and listed; without it
	// only the versions readable at their own state.
	historic := checkDocumentPermission(ctx, site, auth.ActionReadHistoric, doc)

	changesets, errE := site.Base.Documents().Changes(ctx, id, after)
	if errors.Is(errE, store.ErrValueNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, auth.ErrAccessDenied) {
		s.ForbiddenWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	history := make([]documentHistoryItem, 0, len(changesets))
	for _, changesetID := range changesets {
		changeset, errE := site.Base.DocumentChangeset(ctx, changesetID)
		if errE != nil {
			s.InternalServerErrorWithError(w, req, errE)
			return
		}
		// Revision 0 asks for the latest revision in the changeset. The version's document is fetched raw
		// (so we cannot use GetDocumentFromChangeset): it serves only the permission check and the history
		// item's metadata, and the caller was already allowed to read the document at its latest version.
		versionJSON, metadata, version, _, errE := changeset.Get(ctx, id, 0)
		if errors.Is(errE, store.ErrValueDeleted) {
			// A deleted version still has valid metadata and version, but it carries no claims
			// through which era readers could be allowed, so only callers holding the historic read
			// action see it.
			// TODO: Expose deleted documents and their versions once we have an undelete action.
			if !historic {
				continue
			}
		} else if errE != nil {
			s.InternalServerErrorWithError(w, req, errE)
			return
		} else if !historic {
			versionDoc := new(document.D)
			errE = x.UnmarshalWithoutUnknownFields(versionJSON, versionDoc)
			if errE != nil {
				s.InternalServerErrorWithError(w, req, errE)
				return
			}
			if !checkDocumentPermission(ctx, site, auth.ActionRead, versionDoc) {
				continue
			}
		}
		history = append(history, documentHistoryItem{
			Changeset: changesetID,
			Version:   version,
			At:        metadata.At,
			Authors:   metadata.Users,
		})
	}

	s.WriteJSON(w, req, history, nil)
}

// documentListItem is one entry in the readable-documents listing: a document's ID.
type documentListItem struct {
	ID identifier.Identifier `json:"id"`
}

// listReadableDocuments returns the documents in the site that the caller (identified by ctx) may read,
// starting after the given document ID (nil for the first page). It walks the store's committed document IDs
// and keeps only those a read by ID would return, skipping deleted documents (ErrValueNotFound, which includes
// ErrValueDeleted) and ones the read-path permission hooks deny (ErrAccessDenied), so it applies the same
// access semantics as viewing a document. A fully unreadable store page does not end the listing: it keeps
// scanning so the result is empty only when no readable document remains after the cursor. The returned slice
// is never nil, and its last item's ID is the cursor to pass as "after" for the next page.
func listReadableDocuments(ctx context.Context, site *internalSite.Site, after *identifier.Identifier) ([]documentListItem, errors.E) {
	documents := []documentListItem{}
	for {
		ids, errE := site.Base.Documents().List(ctx, after)
		if errE != nil {
			return nil, errE
		}
		for _, id := range ids {
			// TODO: Add API to store to just check if the value exists.
			_, _, _, _, errE := site.Base.GetDocumentLatest(ctx, id)
			if errors.Is(errE, store.ErrValueNotFound) || errors.Is(errE, auth.ErrAccessDenied) {
				continue
			} else if errE != nil {
				return nil, errE
			}
			documents = append(documents, documentListItem{ID: id})
		}
		// Stop once this scan produced something to return, or once the store has no more pages (a page shorter
		// than a full one is the last). Otherwise keep scanning from the last id, so a fully unreadable page does
		// not end the listing early: the caller pages by the last returned id and stops only on an empty result.
		if len(documents) > 0 || len(ids) < store.MaxPageLength {
			return documents, nil
		}
		after = &ids[len(ids)-1]
	}
}

// DocumentListGetAPI is a GET/HEAD HTTP request handler which lists the documents the caller may read, as a
// JSON array of document IDs. It uses keyset pagination via the "after" query parameter: to fetch the next
// page, request again with "after" set to the ID of the last item in the returned array; an empty array means
// there are no more. Each entry is a document a read by ID would return, applying the same access semantics as
// viewing a document (see listReadableDocuments).
//
// Enumerating documents requires the bulk read action on documents, on top of the per-document read action
// the listing itself applies: a caller who may read documents one by one does not thereby get to enumerate
// them. Because the check is done up front, a caller without it does not make the listing scan the store.
func (s *Service) DocumentListGetAPI(w http.ResponseWriter, req *http.Request, _ waf.Params) {
	ctx := req.Context()
	metrics := waf.MustGetMetrics(ctx)

	errE := s.HasDocumentPermission(ctx, auth.ActionReadBulk, nil)
	if errE != nil {
		s.ForbiddenWithError(w, req, errE)
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

	site := waf.MustGetSite[*internalSite.Site](ctx)

	m := metrics.Duration(internalStore.MetricDatabase).Start()
	documents, errE := listReadableDocuments(ctx, site, after)
	m.Stop()
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, documents, nil)
}

// DocumentSessionResponse is one edit session DocumentSessionsGetAPI returns.
type DocumentSessionResponse struct {
	Session identifier.Identifier `json:"session"`
	// Doc is the document as the session has it, so a document being created is described by what the
	// session has put into it and not by what is committed of it, which is nothing until it is saved.
	Doc *document.D `json:"doc"`
	// Create is true for a session creating a new document and false for one editing a document which exists.
	Create bool `json:"create"`
	// At is when the session began and By who began it, LastChangeAt when a change was last appended to it
	// and LastChangeBy who appended it. Either user is absent when done unauthenticated.
	At           store.Time  `json:"at"`
	By           *store.User `json:"by,omitempty"`
	LastChangeAt store.Time  `json:"lastChangeAt"`
	LastChangeBy *store.User `json:"lastChangeBy,omitempty"`
}

// DocumentSessionsGet is a GET/HEAD HTTP request handler which returns HTML frontend for the page
// listing the caller's edit sessions.
func (s *Service) DocumentSessionsGet(w http.ResponseWriter, req *http.Request, _ waf.Params) {
	s.HomeGet(w, req, nil)
}

// DocumentSessionsGetAPI is a GET HTTP request API handler which returns the edit sessions the caller
// is taking part in and can still open, newest first: the sessions they began or appended a change to,
// which have not ended yet (see base.ListEditSessions), each with the document as the session has it.
//
// A session the caller may no longer access is left out, so opening any of the listed ones works, and
// so is a session which ends while the list is being made: it is no longer one to continue either.
func (s *Service) DocumentSessionsGetAPI(w http.ResponseWriter, req *http.Request, _ waf.Params) {
	ctx := req.Context()
	site := waf.MustGetSite[*internalSite.Site](ctx)

	sessions, errE := site.Base.ListEditSessions(ctx)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	response := make([]DocumentSessionResponse, 0, len(sessions))
	for _, session := range sessions {
		beginMetadata, doc, errE := s.sessionDocumentWithPermission(ctx, session.Session)
		if errors.Is(errE, auth.ErrAccessDenied) || errors.Is(errE, coordinator.ErrSessionNotFound) {
			continue
		} else if errE != nil {
			s.InternalServerErrorWithError(w, req, errE)
			return
		} else if doc == nil {
			// The session completed while the list was being made, so it is not one to continue either.
			continue
		}

		// The state is built raw, so it is filtered for the caller the same as a document they read
		// (see base.SessionDocumentHooks), which is also what enforces the read action on it.
		doc, errE = site.Base.FilterSessionDocument(ctx, doc, beginMetadata)
		if errors.Is(errE, auth.ErrAccessDenied) {
			continue
		} else if errE != nil {
			s.InternalServerErrorWithError(w, req, errE)
			return
		} else if doc == nil {
			continue
		}

		response = append(response, DocumentSessionResponse{
			Session:      session.Session,
			Doc:          doc,
			Create:       session.Create,
			At:           session.At,
			By:           session.By,
			LastChangeAt: session.LastChangeAt,
			LastChangeBy: session.LastChangeBy,
		})
	}

	s.WriteJSON(w, req, response, nil)
}

type documentCreateResponse struct {
	ID      identifier.Identifier `json:"id"`
	Base    []string              `json:"base"`
	Session identifier.Identifier `json:"session"`
	// LastChange is the sequence number of the last change seeded into the session by the server;
	// client changes continue at LastChange plus one.
	LastChange int64 `json:"lastChange"`
}

// DocumentCreateGet is a GET/HEAD HTTP request handler which returns HTML frontend for the document
// creation page.
func (s *Service) DocumentCreateGet(w http.ResponseWriter, req *http.Request, _ waf.Params) {
	errE := s.HasDocumentPermission(req.Context(), auth.ActionCreate, nil)
	if errE != nil {
		s.ForbiddenWithError(w, req, errE)
		return
	}

	s.HomeGet(w, req, nil)
}

// DocumentCreatePostAPI handles POST requests to start creating a new document.
//
// It does not insert anything into the store. Instead, it pre-allocates a document
// ID and base, opens a coordinator "create" session, and returns id + base + session.
// The actual document is materialized in the store only when the client ends the
// session with EndEditDocument (Save). At that point an empty document is inserted
// and the session's accumulated changes are applied as the second changeset, so the
// patch history records the transition from empty to populated. Discarding the
// session leaves the store untouched.
func (s *Service) DocumentCreatePostAPI(w http.ResponseWriter, req *http.Request, _ waf.Params) {
	defer req.Body.Close()              //nolint:errcheck
	defer io.Copy(io.Discard, req.Body) //nolint:errcheck

	ctx := req.Context()

	errE := s.HasDocumentPermission(ctx, auth.ActionCreate, nil)
	if errE != nil {
		s.ForbiddenWithError(w, req, errE)
		return
	}

	site := waf.MustGetSite[*internalSite.Site](ctx)

	// TODO: Support configuring base and not just use the domain.
	docBase := []string{site.Domain, "DOCUMENT", identifier.New().String()}

	session, errE := site.Base.BeginCreateDocument(ctx, docBase)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}
	systemCtx := base.WithSystemSession(ctx)

	// The session's first changes come from the seed hook.
	var seedChanges document.Changes
	if site.Base.CreateDocumentSeed != nil {
		seedChanges, errE = site.Base.CreateDocumentSeed(ctx, req, session, docBase)
		if errE != nil {
			// Attempt to discard the session to cleanup.
			_ = site.Base.EndEditDocument(systemCtx, session, true)
			// TODO: Support mapping errors to 500 Internal Server Error.
			//       Currently RequestedClaimsChanges returns only errors which should b mapped to 400 Bad Request, but non-default implementations
			//       might return other errors (like transient errors) which should te mapped to 500 Internal Server Error or even something else.
			s.BadRequestWithError(w, req, errE)
			return
		}
	}

	// The session's initial document state: the seed changes are validated and applied to it
	// before anything is appended to the session, so a misbehaving seed hook fails early.
	changesetBase := append(slices.Clone(docBase), "SESSION", session.String())
	doc := &document.D{CoreDocument: document.CoreDocument{ID: identifier.From(docBase...), Base: docBase}}
	errE = seedChanges.ValidateAndApply(doc, changesetBase, 0)
	if errE != nil {
		// Attempt to discard the session to cleanup.
		_ = site.Base.EndEditDocument(systemCtx, session, true)
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	lastChange, errE := site.Base.AppendDocumentChanges(systemCtx, session, seedChanges, 0)
	if errE != nil {
		// Attempt to discard the session to cleanup.
		_ = site.Base.EndEditDocument(systemCtx, session, true)
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	// The document with the requested and seeded claims (bare when there are none) has to satisfy
	// the caller's create grants (only role grants can give the create action): claim-scoped create
	// grants cover only documents carrying a matching claim, so callers holding only such grants
	// have to request matching initial claims. The check runs after all initial changes, so it
	// evaluates the document state the session actually starts from; on denial the session, with
	// the changes appended to it, is discarded.
	if !checkRoleDocumentPermission(ctx, site, auth.ActionCreate, doc) {
		_ = site.Base.EndEditDocument(systemCtx, session, true)
		errE := errors.WithStack(auth.ErrAccessDenied)
		errors.Details(errE)["action"] = auth.ActionCreate.String()
		s.ForbiddenWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, documentCreateResponse{
		ID:         identifier.From(docBase...),
		Base:       docBase,
		Session:    session,
		LastChange: lastChange,
	}, nil)
}

// createOptionsResponse is the JSON body of DocumentCreateOptionsGetAPI. Classes are the classes to offer
// in the create-document view, already ordered for tree rendering. It is an object (rather than a bare
// array) so it can carry more create-time options in the future without breaking clients.
type createOptionsResponse struct {
	Classes []search.ClassCreateOption `json:"classes"`
}

// DocumentCreateOptionsGetAPI is a GET HTTP request API handler which returns the classes offered in the
// create-document view as a flat, render-ordered list the frontend builds into a tree (see
// search.CreateOptions): every class a document can be created for, plus the structural ancestors needed
// to place them, with classes that have more documents ordered first. It uses the same permission gate as
// the create page.
func (s *Service) DocumentCreateOptionsGetAPI(w http.ResponseWriter, req *http.Request, _ waf.Params) {
	ctx := req.Context()

	errE := s.HasDocumentPermission(ctx, auth.ActionCreate, nil)
	if errE != nil {
		s.ForbiddenWithError(w, req, errE)
		return
	}

	index, handled := s.resolveReadIndex(w, req)
	if handled {
		return
	}

	accessFilter, errE := searchAccessFilter(ctx)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	// An optional "limit" restricts the offered classes to that class and its descendants (with its
	// ancestors shown only as labels).
	limit := ""
	if l := req.URL.Query().Get("limit"); l != "" {
		limitID, errE := identifier.MaybeString(l)
		if errE != nil {
			s.BadRequestWithError(w, req, errors.WithMessage(errE, `"limit" is not a valid identifier`))
			return
		}
		limit = limitID.String()
	}

	site := waf.MustGetSite[*internalSite.Site](ctx)

	// The rest of the query string asks for the same initial claims the creation itself would be asked
	// for, so that a caller whose create grants are scoped on them is offered the classes they can
	// create with them (see DocumentCreatePostAPI, which seeds exactly these claims).
	requestedQuery := req.URL.Query()
	requestedQuery.Del("limit")
	requested, errE := base.RequestedClaims(requestedQuery, site.ScopeProperties)
	if errE != nil {
		s.BadRequestWithError(w, req, errE)
		return
	}

	// loadDocument reads a class document so search.CreateOptions can decide createability. CreateOptions
	// skips a class that is not found or not accessible (an ErrValueNotFound or ErrAccessDenied error).
	loadDocument := func(ctx context.Context, id identifier.Identifier) (*document.D, errors.E) {
		doc, _, _, _, errE := site.Base.GetDocumentLatestDoc(ctx, id)
		return doc, errE
	}

	// A class the caller's create grants do not cover is not offered, so the view offers what creating
	// a document would accept instead of what the create gate lets the caller reach.
	canCreate := func(ctx context.Context, id identifier.Identifier) bool {
		return checkCreateClassPermission(ctx, site, id, requested)
	}

	classes, errE := search.CreateOptions(ctx, s.getSearchServiceClosure(req, index), accessFilter, loadDocument, canCreate, s.documentHierarchyPaths, limit)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, createOptionsResponse{Classes: classes}, nil)
}

type documentBeginEditResponse struct {
	Session identifier.Identifier `json:"session"`
	Version store.Version         `json:"version"`
}

// DocumentBeginEditPostAPI handles POST requests to begin an edit session for a document. It requires the
// update action on the document, so document-level permission claims count besides role grants.
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

	site := waf.MustGetSite[*internalSite.Site](ctx)

	// The update action is checked against the document itself, so both role grants and the
	// document's own permission claims count, and the session then begins at exactly the version
	// the permission check saw.
	doc, _, version, _, errE := site.Base.GetDocumentLatestDoc(ctx, id)
	if errors.Is(errE, store.ErrValueNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, auth.ErrAccessDenied) {
		s.ForbiddenWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}
	errE = s.HasDocumentPermission(ctx, auth.ActionUpdate, doc)
	if errE != nil {
		s.ForbiddenWithError(w, req, errE)
		return
	}

	session, errE := site.Base.BeginEditDocument(ctx, version, doc)
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

	errE = s.HasSessionPermission(ctx, session)
	if errors.Is(errE, coordinator.ErrSessionNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, auth.ErrAccessDenied) {
		s.ForbiddenWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
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

	site := waf.MustGetSite[*internalSite.Site](ctx)

	_, errE = site.Base.AppendDocumentChange(ctx, session, buffer, change)
	if errors.Is(errE, base.ErrInvalidChange) {
		s.BadRequestWithError(w, req, errE)
		return
	} else if errors.Is(errE, coordinator.ErrSessionNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, coordinator.ErrAlreadyEnded) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, coordinator.ErrConflict) {
		waf.Error(w, req, http.StatusConflict)
		return
	} else if errors.Is(errE, auth.ErrAccessDenied) {
		s.ForbiddenWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, []byte(`{"success":true}`), nil)
}

// DocumentLastChangeGetAPI handles GET requests to get the sequence number of the latest
// change in an edit session, 0 when there are none. Changes are numbered sequentially without
// gaps starting at 1, so the session's changes are exactly 1 through the returned number.
func (s *Service) DocumentLastChangeGetAPI(w http.ResponseWriter, req *http.Request, params waf.Params) {
	ctx := req.Context()

	session, errE := identifier.MaybeString(params["session"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"session" is not a valid identifier`))
		return
	}

	errE = s.HasSessionPermission(ctx, session)
	if errors.Is(errE, coordinator.ErrSessionNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, auth.ErrAccessDenied) {
		s.ForbiddenWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	site := waf.MustGetSite[*internalSite.Site](ctx)

	lastChange, errE := site.Base.LastDocumentChange(ctx, session)
	if errors.Is(errE, coordinator.ErrSessionNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, coordinator.ErrAlreadyCompleted) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, auth.ErrAccessDenied) {
		s.ForbiddenWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, lastOperationResponse{LastOperation: lastChange}, nil)
}

// DocumentGetChangeGetAPI handles GET requests to retrieve a specific change from an edit session.
func (s *Service) DocumentGetChangeGetAPI(w http.ResponseWriter, req *http.Request, params waf.Params) {
	ctx := req.Context()

	session, errE := identifier.MaybeString(params["session"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"session" is not a valid identifier`))
		return
	}

	errE = s.HasSessionPermission(ctx, session)
	if errors.Is(errE, coordinator.ErrSessionNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, auth.ErrAccessDenied) {
		s.ForbiddenWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	chunk, err := strconv.ParseInt(params["change"], 10, 64)
	if err != nil {
		s.BadRequestWithError(w, req, errors.WithStack(err))
		return
	}

	site := waf.MustGetSite[*internalSite.Site](ctx)

	dataJSON, errE := site.Base.GetDocumentChange(ctx, session, chunk)
	if errors.Is(errE, coordinator.ErrSessionNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, coordinator.ErrAlreadyCompleted) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, coordinator.ErrOperationNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, auth.ErrAccessDenied) {
		s.ForbiddenWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	// TODO: Decide what taking part in an edit session may reveal of the document.
	//       The change is returned as it was appended, so every participant reads every change the
	//       others made to the session, including changes to parts of the document a site hides from
	//       them on the read path. No document hook can filter this: a change is not a document (see
	//       base.DocumentPostHooks for a read and base.SessionDocumentHooks for the session's state,
	//       both of which stay filtered). So a session between participants who may see different
	//       parts of the same document shows each of them all of it. Either changes are filtered per
	//       participant as well, which requires the site to say what a change reveals, or taking part
	//       in a session requires being able to see the whole document, which makes it a permission
	//       rule instead of a filtering one.
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

	_, doc, errE := s.sessionDocumentWithPermission(ctx, session)
	if errors.Is(errE, coordinator.ErrSessionNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, auth.ErrAccessDenied) {
		s.ForbiddenWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	var ea emptyRequest
	errE = x.DecodeJSONWithoutUnknownFields(req.Body, &ea)
	if errE != nil {
		s.BadRequestWithError(w, req, errE)
		return
	}

	site := waf.MustGetSite[*internalSite.Site](ctx)

	if site.Base.EndEditPermissionCheck != nil {
		// The completion (commit or discard) is gated by the same configured check which runs again
		// when the session completes: there with the user and roles recorded at the session's end
		// as the authoritative one (it sees the session's final change list), here with the
		// request's user and roles so the request fails fast. The check gets the session's
		// resulting document, so scoped role grants are enforced against what the session actually
		// produces (and an edit cannot move the document out of the granted scope).
		if doc == nil {
			// The session has completed, so it has no state of its own left to check, nor anything to end.
			s.NotFoundWithError(w, req, errors.WithStack(coordinator.ErrAlreadyCompleted))
			return
		}
		errE = site.Base.EndEditPermissionCheck(store.UserFromContext(ctx), auth.Roles(ctx), doc)
		if errE != nil {
			s.ForbiddenWithError(w, req, errE)
			return
		}
	}

	errE = site.Base.EndEditDocument(ctx, session, discard)
	if errors.Is(errE, coordinator.ErrSessionNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, coordinator.ErrAlreadyEnded) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, auth.ErrAccessDenied) {
		s.ForbiddenWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, []byte(`{"success":true}`), nil)
}

// DocumentDeletePostAPI handles POST requests to delete a document.
func (s *Service) DocumentDeletePostAPI(w http.ResponseWriter, req *http.Request, params waf.Params) {
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

	site := waf.MustGetSite[*internalSite.Site](ctx)

	// The delete action is checked against the document itself, so document-level permission claims
	// count besides role grants.
	doc, _, _, _, errE := site.Base.GetDocumentLatestDoc(ctx, id) //nolint:dogsled
	if errors.Is(errE, store.ErrValueNotFound) {
		// This includes ErrValueDeleted, too.
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, auth.ErrAccessDenied) {
		s.ForbiddenWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}
	errE = s.HasDocumentPermission(ctx, auth.ActionDelete, doc)
	if errE != nil {
		s.ForbiddenWithError(w, req, errE)
		return
	}

	errE = site.Base.DeleteDocument(ctx, id)
	if errors.Is(errE, store.ErrValueNotFound) {
		// This includes ErrValueDeleted, too.
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, store.ErrConflict) {
		waf.Error(w, req, http.StatusConflict)
		return
	} else if errors.Is(errE, auth.ErrAccessDenied) {
		s.ForbiddenWithError(w, req, errE)
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

	errE = s.HasSessionPermission(ctx, session)
	if errors.Is(errE, coordinator.ErrSessionNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, auth.ErrAccessDenied) {
		s.ForbiddenWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	site := waf.MustGetSite[*internalSite.Site](req.Context())

	beginMetadata, _, completeMetadata, errE := site.Base.GetEditDocumentSession(ctx, session)
	if errors.Is(errE, coordinator.ErrSessionNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, auth.ErrAccessDenied) {
		s.ForbiddenWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	if beginMetadata.DocumentID != id {
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

	errE = s.HasSessionPermission(ctx, session)
	if errors.Is(errE, coordinator.ErrSessionNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, auth.ErrAccessDenied) {
		s.ForbiddenWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	site := waf.MustGetSite[*internalSite.Site](req.Context())

	beginMetadata, sessionEnded, completeMetadata, errE := site.Base.GetEditDocumentSession(ctx, session)
	if errors.Is(errE, coordinator.ErrSessionNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, auth.ErrAccessDenied) {
		s.ForbiddenWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	if beginMetadata.DocumentID != id {
		// TODO: Should we redirect to the correct ID?
		s.NotFoundWithError(w, req, errors.New(`"session" does not match "id"`))
		return
	}

	if completeMetadata != nil {
		s.WriteJSON(w, req, struct {
			*base.DocumentCompleteMetadata

			Active bool `json:"active"`
		}{
			DocumentCompleteMetadata: completeMetadata,
			Active:                   false,
		}, nil)
	} else if sessionEnded {
		s.WriteJSON(w, req, `{"active":false}`, nil)
	} else {
		// Active session: include base and (for edit sessions) version, so the
		// client can rebuild claim IDs from base and decide whether to fetch the
		// parent document. Absent version signals a create session.
		s.WriteJSON(w, req, struct {
			Active  bool           `json:"active"`
			Base    []string       `json:"base"`
			Version *store.Version `json:"version,omitempty"`
		}{
			Active:  true,
			Base:    beginMetadata.Base,
			Version: beginMetadata.Version,
		}, nil)
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
	} else if errors.Is(errE, auth.ErrAccessDenied) {
		s.ForbiddenWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, changes, nil)
}

// DocumentChangesGetAPI handles GET requests to list changes in a document changeset. Only changes
// of documents whose changed version the caller may read are listed, so a page can be shorter than
// the store page.
//
//nolint:dupl
func (s *Service) DocumentChangesGetAPI(w http.ResponseWriter, req *http.Request, params waf.Params) {
	s.changesetChangesGetAPI(w, req, params, func(ctx context.Context, changesetID identifier.Identifier, after *identifier.Identifier) ([]store.Change, errors.E) {
		site := waf.MustGetSite[*internalSite.Site](ctx)
		cs, errE := site.Base.DocumentChangeset(ctx, changesetID)
		if errE != nil {
			return nil, errE
		}
		changes, errE := cs.Changes(ctx, after)
		if errE != nil {
			return nil, errE
		}
		visible := make([]store.Change, 0, len(changes))
		for _, change := range changes {
			ok, errE := checkVersionedReadPermission(ctx, site, change.ID, change.Version)
			if errE != nil {
				return nil, errE
			}
			if ok {
				visible = append(visible, change)
			}
		}
		return visible, nil
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

	site := waf.MustGetSite[*internalSite.Site](ctx)

	// Revision 0 means latest revision.
	dataJSON, _, version, _, errE := site.Base.GetDocumentFromChangeset(ctx, changesetID, id, 0)
	if errors.Is(errE, store.ErrValueNotFound) {
		// This includes ErrValueDeleted, too.
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, store.ErrChangesetNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, auth.ErrAccessDenied) {
		s.ForbiddenWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	w.Header().Set("Version", version.String())

	s.WriteJSON(w, req, dataJSON, nil)
}
