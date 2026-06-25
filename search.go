package peerdb

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	internalSite "gitlab.com/peerdb/peerdb/internal/site"

	esSearch "github.com/elastic/go-elasticsearch/v9/typedapi/core/search"
	"github.com/elastic/go-elasticsearch/v9/typedapi/esdsl"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"
	"gitlab.com/tozd/waf"

	"gitlab.com/peerdb/peerdb/document"
	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
	internalShortcut "gitlab.com/peerdb/peerdb/internal/shortcut"
	internalStore "gitlab.com/peerdb/peerdb/internal/store"
	"gitlab.com/peerdb/peerdb/search"
	"gitlab.com/peerdb/peerdb/store"
)

// resolveReadIndex resolves the ElasticSearch index the request should read, routed by the caller's
// visibility level. When the caller may not read any index (no visibility level on a site that defines
// visibility levels) it writes a 403 Forbidden response and returns handled=true, so the calling handler
// must return without searching.
func (s *Service) resolveReadIndex(w http.ResponseWriter, req *http.Request) (string, bool) {
	ctx := req.Context()
	site := waf.MustGetSite[*internalSite.Site](ctx)
	index, errE := site.ReadIndex(ctx)
	if errors.Is(errE, store.ErrAccessDenied) {
		s.ForbiddenWithError(w, req, errE)
		return "", true
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return "", true
	}
	return index, false
}

// enabledSearchLanguages returns the indexed language set for the site serving the request,
// used to scope the text-search query to the languages the index actually has.
func enabledSearchLanguages(ctx context.Context) []string {
	site := waf.MustGetSite[*internalSite.Site](ctx)
	return internalSearch.EnabledLanguages(site.LanguagePriority)
}

// searchAccessFilter returns the site's optional per-caller search restriction
// (from base.B.SearchQueryHook) for the request in ctx, or a nil query if the
// site sets no hook. It is added as a filter clause to every search query so
// results and facets only include documents the caller may access.
func searchAccessFilter(ctx context.Context) (types.QueryVariant, errors.E) { //nolint:ireturn
	site := waf.MustGetSite[*internalSite.Site](ctx)
	if site.Base.SearchQueryHook == nil {
		// No hook means no restriction.
		return nil, nil //nolint:nilnil
	}
	return site.Base.SearchQueryHook(ctx)
}

// sessionQuery builds the session's ElasticSearch query with the site's access filter applied.
func sessionQuery(ctx context.Context, session *search.Session) (types.QueryVariant, errors.E) { //nolint:ireturn
	accessFilter, errE := searchAccessFilter(ctx)
	if errE != nil {
		return nil, errE
	}
	return session.ToQuery(enabledSearchLanguages(ctx), accessFilter), nil
}

// sessionQueryExcluding builds the session's ElasticSearch query excluding the
// given filter, with the site's access filter applied.
func sessionQueryExcluding(ctx context.Context, session *search.Session, excludeFilterID identifier.Identifier) (types.QueryVariant, errors.E) { //nolint:ireturn
	accessFilter, errE := searchAccessFilter(ctx)
	if errE != nil {
		return nil, errE
	}
	return session.ToQueryExcluding(excludeFilterID, enabledSearchLanguages(ctx), accessFilter), nil
}

// documentFullPaths resolves a value's full hierarchy paths (the indexed toFullPath form) for the caller's
// visibility level.
func (s *Service) documentFullPaths(ctx context.Context, id identifier.Identifier) ([]string, errors.E) {
	return waf.MustGetSite[*internalSite.Site](ctx).Base.DocumentFullPaths(ctx, id)
}

func (s *Service) getSearchService(req *http.Request, index string) *esSearch.Search {
	ctx := req.Context()

	site := waf.MustGetSite[*internalSite.Site](ctx)

	// We set TrackTotalHits to true to always get exact number of results. For now we didn't notice any performance
	// issues at data scale PeerDB is currently being used with, but in the future we might want to make this configurable.
	return site.ESClient.Search().Index(index).
		Source_(esdsl.NewSourceConfig().Bool(false)).
		Preference(getHost(req.RemoteAddr)).
		Header("X-Opaque-ID", waf.MustRequestID(ctx).String()).
		TrackTotalHits(esdsl.NewTrackHits().Bool(true)).
		AllowPartialSearchResults(false)
}

func (s *Service) getSearchServiceClosure(req *http.Request, index string) func() *esSearch.Search {
	return func() *esSearch.Search {
		return s.getSearchService(req, index)
	}
}

// scoreFactorTTL is how long a cached counts.score boost factor is reused before it
// is recomputed from the corpus.
const scoreFactorTTL = time.Hour

type scoreFactorEntry struct {
	// mu guards this entry's factor and computed, so that recomputing one index's
	// factor only serializes requests for that same index and not for other sites.
	mu       sync.Mutex
	factor   float64
	computed time.Time
}

// scoreFactor returns the counts.score ranking boost factor for the site serving the
// request, computed via search.ScoreFactor and cached per index for scoreFactorTTL.
//
// Each visibility level has its own filtered index and therefore its own corpus, so we
// cache the factor per per-level index rather than per site.
func (s *Service) scoreFactor(ctx context.Context, req *http.Request, index string) (float64, errors.E) {
	// We hold the cache lock only long enough to get or create the per-index entry,
	// so computing one index's factor does not block requests for other indexes.
	s.scoreFactorMu.Lock()
	entry, ok := s.scoreFactorCache[index]
	if !ok {
		entry = &scoreFactorEntry{}
		s.scoreFactorCache[index] = entry
	}
	s.scoreFactorMu.Unlock()

	// We lock only this index's entry while reading or recomputing its factor.
	entry.mu.Lock()
	defer entry.mu.Unlock()

	if !entry.computed.IsZero() && time.Since(entry.computed) < scoreFactorTTL {
		return entry.factor, nil
	}

	factor, errE := search.ScoreFactor(ctx, s.getSearchServiceClosure(req, index))
	if errE != nil {
		return 0, errE
	}

	entry.factor = factor
	entry.computed = time.Now()

	return factor, nil
}

// collectParentToFromSession returns the To values of any active top-level
// ref filter on parentProp in the given filter set. These values supply
// cross-filter parentTo restrictions for sub-claim filters and aggregations
// keyed on the same parentProp.
func collectParentToFromSession(filters []search.Filter, parentProp identifier.Identifier) []identifier.Identifier {
	var out []identifier.Identifier
	for _, f := range filters {
		if f.Ref != nil && len(f.Prop) == 1 && f.Prop[0] == parentProp {
			for _, to := range f.Ref.To {
				out = append(out, to.ID)
			}
		}
	}
	return out
}

// SearchFilterGetAPI handles GET requests for individual active (those in the session) filter search endpoint.
//
// It dispatches to the appropriate filter handler based on the filter type.
func (s *Service) SearchFilterGetAPI(w http.ResponseWriter, req *http.Request, params waf.Params) {
	ctx := req.Context()

	id, errE := identifier.MaybeString(params["id"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"id" is not a valid identifier`))
		return
	}

	filterID, errE := identifier.MaybeString(params["filter"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"filter" is not a valid identifier`))
		return
	}

	// Look up the session and find the filter to determine its type.
	searchSession, errE := search.GetSession(ctx, id)
	if errors.Is(errE, search.ErrNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, store.ErrAccessDenied) {
		s.ForbiddenWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	f, errE := searchSession.GetFilterByID(filterID)
	if errors.Is(errE, search.ErrNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	query, errE := sessionQueryExcluding(ctx, searchSession, filterID)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	// valueQuery narrows a reference or has facet to the values whose display label matches the typed text.
	valueQuery := req.URL.Query().Get("q")
	enabledLanguages := enabledSearchLanguages(ctx)

	index, handled := s.resolveReadIndex(w, req)
	if handled {
		return
	}

	var data any
	var metadata map[string]any

	searchService := s.getSearchServiceClosure(req, index)

	excludes, errE := searchSession.PrefilterExcludeFullPaths(ctx, s.documentFullPaths)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	switch {
	case f.Ref != nil:
		if len(f.Prop) == 2 { //nolint:mnd
			data, metadata, errE = f.Ref.GetSubRef(
				ctx, searchService, query, f.Prop[0], f.Prop[1],
				collectParentToFromSession(searchSession.Filters, f.Prop[0]),
				excludes.SubRef(f.Prop[0], f.Prop[1]),
				valueQuery, enabledLanguages,
			)
		} else {
			data, metadata, errE = f.Ref.Get(ctx, searchService, query, f.Prop[0], excludes.Ref(f.Prop[0]), valueQuery, enabledLanguages)
		}
	case f.Amount != nil:
		if len(f.Prop) == 2 { //nolint:mnd
			data, metadata, errE = f.Amount.GetSubAmount(
				ctx, searchService, query, f.Prop[0], f.Prop[1],
				collectParentToFromSession(searchSession.Filters, f.Prop[0]),
			)
		} else {
			data, metadata, errE = f.Amount.Get(ctx, searchService, query, f.Prop[0])
		}
	case f.Time != nil:
		if len(f.Prop) == 2 { //nolint:mnd
			data, metadata, errE = f.Time.GetSubTime(
				ctx, searchService, query, f.Prop[0], f.Prop[1],
				collectParentToFromSession(searchSession.Filters, f.Prop[0]),
			)
		} else {
			data, metadata, errE = f.Time.Get(ctx, searchService, query, f.Prop[0])
		}
	case f.Has != nil:
		if len(f.Prop) == 1 {
			data, metadata, errE = f.Has.GetSubHas(
				ctx, searchService, query, f.Prop[0],
				collectParentToFromSession(searchSession.Filters, f.Prop[0]),
				valueQuery, enabledLanguages,
			)
		} else {
			data, metadata, errE = f.Has.Get(ctx, searchService, query, valueQuery, enabledLanguages)
		}
	default:
		panic(errors.New("invalid filter"))
	}

	if errE != nil {
		errors.Details(errE)["session"] = id.String()
		errors.Details(errE)["filter"] = filterID.String()
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, data, metadata)
}

// SearchRefFilterGetAPI handles GET requests for reference filter data by property.
//
// Used for inactive filters (not yet in the session).
func (s *Service) SearchRefFilterGetAPI(w http.ResponseWriter, req *http.Request, params waf.Params) {
	ctx := req.Context()

	id, errE := identifier.MaybeString(params["id"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"id" is not a valid identifier`))
		return
	}

	prop, errE := identifier.MaybeString(params["prop"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"prop" is not a valid identifier`))
		return
	}

	searchSession, errE := search.GetSession(ctx, id)
	if errors.Is(errE, search.ErrNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, store.ErrAccessDenied) {
		s.ForbiddenWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	query, errE := sessionQuery(ctx, searchSession)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}
	index, handled := s.resolveReadIndex(w, req)
	if handled {
		return
	}
	excludes, errE := searchSession.PrefilterExcludeFullPaths(ctx, s.documentFullPaths)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}
	f := search.RefFilter{}
	data, metadata, errE := f.Get(
		ctx, s.getSearchServiceClosure(req, index), query, prop, excludes.Ref(prop),
		req.URL.Query().Get("q"), enabledSearchLanguages(ctx),
	)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, data, metadata)
}

// SearchAmountFilterGetAPI handles GET requests for amount filter data by property.
//
// Used for inactive filters (not yet in the session).
func (s *Service) SearchAmountFilterGetAPI(w http.ResponseWriter, req *http.Request, params waf.Params) {
	ctx := req.Context()

	id, errE := identifier.MaybeString(params["id"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"id" is not a valid identifier`))
		return
	}

	prop, errE := identifier.MaybeString(params["prop"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"prop" is not a valid identifier`))
		return
	}

	var unit *identifier.Identifier
	if unitStr, ok := params["unit"]; ok {
		u, errE := identifier.MaybeString(unitStr)
		if errE != nil {
			s.BadRequestWithError(w, req, errors.WithMessage(errE, `"unit" is not a valid identifier`))
			return
		}
		unit = &u
	}

	searchSession, errE := search.GetSession(ctx, id)
	if errors.Is(errE, search.ErrNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, store.ErrAccessDenied) {
		s.ForbiddenWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	query, errE := sessionQuery(ctx, searchSession)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}
	index, handled := s.resolveReadIndex(w, req)
	if handled {
		return
	}
	f := search.AmountFilter{Unit: unit} //nolint:exhaustruct
	data, metadata, errE := f.Get(ctx, s.getSearchServiceClosure(req, index), query, prop)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, data, metadata)
}

// SearchTimeFilterGetAPI handles GET requests for time filter data by property.
//
// Used for inactive filters (not yet in the session).
func (s *Service) SearchTimeFilterGetAPI(w http.ResponseWriter, req *http.Request, params waf.Params) {
	ctx := req.Context()

	id, errE := identifier.MaybeString(params["id"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"id" is not a valid identifier`))
		return
	}

	prop, errE := identifier.MaybeString(params["prop"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"prop" is not a valid identifier`))
		return
	}

	searchSession, errE := search.GetSession(ctx, id)
	if errors.Is(errE, search.ErrNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, store.ErrAccessDenied) {
		s.ForbiddenWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	query, errE := sessionQuery(ctx, searchSession)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}
	index, handled := s.resolveReadIndex(w, req)
	if handled {
		return
	}
	f := search.TimeFilter{}
	data, metadata, errE := f.Get(ctx, s.getSearchServiceClosure(req, index), query, prop)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, data, metadata)
}

// SearchSubRefFilterGetAPI handles GET requests for sub-reference filter data by parentProp and prop.
//
// Used for inactive filters (not yet in the session).
func (s *Service) SearchSubRefFilterGetAPI(w http.ResponseWriter, req *http.Request, params waf.Params) {
	ctx := req.Context()

	id, errE := identifier.MaybeString(params["id"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"id" is not a valid identifier`))
		return
	}

	parentProp, errE := identifier.MaybeString(params["parentProp"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"parentProp" is not a valid identifier`))
		return
	}

	prop, errE := identifier.MaybeString(params["prop"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"prop" is not a valid identifier`))
		return
	}

	searchSession, errE := search.GetSession(ctx, id)
	if errors.Is(errE, search.ErrNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, store.ErrAccessDenied) {
		s.ForbiddenWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	parentToRestrictions := collectParentToFromSession(searchSession.Filters, parentProp)

	query, errE := sessionQuery(ctx, searchSession)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}
	index, handled := s.resolveReadIndex(w, req)
	if handled {
		return
	}
	excludes, errE := searchSession.PrefilterExcludeFullPaths(ctx, s.documentFullPaths)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}
	f := search.RefFilter{}
	data, metadata, errE := f.GetSubRef(
		ctx, s.getSearchServiceClosure(req, index), query, parentProp, prop, parentToRestrictions,
		excludes.SubRef(parentProp, prop),
		req.URL.Query().Get("q"), enabledSearchLanguages(ctx),
	)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, data, metadata)
}

// SearchSubAmountFilterGetAPI handles GET requests for sub-amount filter data by parentProp and prop.
//
// Used for inactive filters (not yet in the session).
func (s *Service) SearchSubAmountFilterGetAPI(w http.ResponseWriter, req *http.Request, params waf.Params) {
	ctx := req.Context()

	id, errE := identifier.MaybeString(params["id"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"id" is not a valid identifier`))
		return
	}

	parentProp, errE := identifier.MaybeString(params["parentProp"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"parentProp" is not a valid identifier`))
		return
	}

	prop, errE := identifier.MaybeString(params["prop"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"prop" is not a valid identifier`))
		return
	}

	var unit *identifier.Identifier
	if unitStr, ok := params["unit"]; ok {
		u, errE := identifier.MaybeString(unitStr)
		if errE != nil {
			s.BadRequestWithError(w, req, errors.WithMessage(errE, `"unit" is not a valid identifier`))
			return
		}
		unit = &u
	}

	searchSession, errE := search.GetSession(ctx, id)
	if errors.Is(errE, search.ErrNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	parentToRestrictions := collectParentToFromSession(searchSession.Filters, parentProp)

	query, errE := sessionQuery(ctx, searchSession)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}
	index, handled := s.resolveReadIndex(w, req)
	if handled {
		return
	}
	f := search.AmountFilter{Unit: unit} //nolint:exhaustruct
	data, metadata, errE := f.GetSubAmount(ctx, s.getSearchServiceClosure(req, index), query, parentProp, prop, parentToRestrictions)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, data, metadata)
}

// SearchSubTimeFilterGetAPI handles GET requests for sub-time filter data by parentProp and prop.
//
// Used for inactive filters (not yet in the session).
func (s *Service) SearchSubTimeFilterGetAPI(w http.ResponseWriter, req *http.Request, params waf.Params) {
	ctx := req.Context()

	id, errE := identifier.MaybeString(params["id"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"id" is not a valid identifier`))
		return
	}

	parentProp, errE := identifier.MaybeString(params["parentProp"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"parentProp" is not a valid identifier`))
		return
	}

	prop, errE := identifier.MaybeString(params["prop"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"prop" is not a valid identifier`))
		return
	}

	searchSession, errE := search.GetSession(ctx, id)
	if errors.Is(errE, search.ErrNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	parentToRestrictions := collectParentToFromSession(searchSession.Filters, parentProp)

	query, errE := sessionQuery(ctx, searchSession)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}
	index, handled := s.resolveReadIndex(w, req)
	if handled {
		return
	}
	f := search.TimeFilter{}
	data, metadata, errE := f.GetSubTime(ctx, s.getSearchServiceClosure(req, index), query, parentProp, prop, parentToRestrictions)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, data, metadata)
}

// SearchHasFilterGetAPI handles GET requests for has filter data.
//
// Used for inactive filters (not yet in the session).
func (s *Service) SearchHasFilterGetAPI(w http.ResponseWriter, req *http.Request, params waf.Params) {
	ctx := req.Context()

	id, errE := identifier.MaybeString(params["id"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"id" is not a valid identifier`))
		return
	}

	searchSession, errE := search.GetSession(ctx, id)
	if errors.Is(errE, search.ErrNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, store.ErrAccessDenied) {
		s.ForbiddenWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	query, errE := sessionQuery(ctx, searchSession)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}
	index, handled := s.resolveReadIndex(w, req)
	if handled {
		return
	}
	f := search.HasFilter{}
	data, metadata, errE := f.Get(ctx, s.getSearchServiceClosure(req, index), query, req.URL.Query().Get("q"), enabledSearchLanguages(ctx))
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, data, metadata)
}

// SearchSubHasFilterGetAPI handles GET requests for sub-has filter data by parentProp.
//
// Used for inactive filters (not yet in the session).
func (s *Service) SearchSubHasFilterGetAPI(w http.ResponseWriter, req *http.Request, params waf.Params) {
	ctx := req.Context()

	id, errE := identifier.MaybeString(params["id"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"id" is not a valid identifier`))
		return
	}

	parentProp, errE := identifier.MaybeString(params["parentProp"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"parentProp" is not a valid identifier`))
		return
	}

	searchSession, errE := search.GetSession(ctx, id)
	if errors.Is(errE, search.ErrNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	parentToRestrictions := collectParentToFromSession(searchSession.Filters, parentProp)

	query, errE := sessionQuery(ctx, searchSession)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}
	index, handled := s.resolveReadIndex(w, req)
	if handled {
		return
	}
	f := search.HasFilter{}
	data, metadata, errE := f.GetSubHas(
		ctx, s.getSearchServiceClosure(req, index), query, parentProp, parentToRestrictions,
		req.URL.Query().Get("q"), enabledSearchLanguages(ctx),
	)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, data, metadata)
}

// SearchGetGet is a GET/HEAD HTTP request handler which returns HTML frontend for searching documents.
func (s *Service) SearchGetGet(w http.ResponseWriter, req *http.Request, params waf.Params) {
	ctx := req.Context()
	metrics := waf.MustGetMetrics(ctx)

	m := metrics.Duration(internalStore.MetricSearchSession).Start()
	_, errE := search.GetSessionFromID(ctx, params["id"])
	m.Stop()
	if errors.Is(errE, search.ErrNotFound) {
		// TODO: We should show some nice 404 error page here.
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, store.ErrAccessDenied) {
		// TODO: We should show some nice 403 error page here.
		s.ForbiddenWithError(w, req, errE)
		return
	} else if errE != nil {
		// TODO: We should show some nice 500 error page here.
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.HomeGet(w, req, nil)
}

// SearchGetGetAPI is a GET/HEAD HTTP request API handler which returns a search session.
func (s *Service) SearchGetGetAPI(w http.ResponseWriter, req *http.Request, params waf.Params) {
	ctx := req.Context()
	metrics := waf.MustGetMetrics(ctx)

	m := metrics.Duration(internalStore.MetricSearchSession).Start()
	searchSession, errE := search.GetSessionFromID(ctx, params["id"])
	m.Stop()
	if errors.Is(errE, search.ErrNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, store.ErrAccessDenied) {
		s.ForbiddenWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, searchSession, nil)
}

// SearchFiltersGetAPI is a GET/HEAD HTTP request API handler which returns filters available for the search session.
func (s *Service) SearchFiltersGetAPI(w http.ResponseWriter, req *http.Request, params waf.Params) {
	ctx := req.Context()
	metrics := waf.MustGetMetrics(ctx)

	m := metrics.Duration(internalStore.MetricSearchSession).Start()
	searchSession, errE := search.GetSessionFromID(ctx, params["id"])
	m.Stop()
	if errors.Is(errE, search.ErrNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, store.ErrAccessDenied) {
		s.ForbiddenWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	accessFilter, errE := searchAccessFilter(ctx)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	index, handled := s.resolveReadIndex(w, req)
	if handled {
		return
	}

	excludes, errE := searchSession.PrefilterExcludeFullPaths(ctx, s.documentFullPaths)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	data, metadata, errE := search.FiltersGet(
		ctx, s.getSearchServiceClosure(req, index), searchSession, enabledSearchLanguages(ctx),
		req.URL.Query().Get("q"), excludes, accessFilter,
	)
	if errors.Is(errE, search.ErrValidationFailed) {
		s.BadRequestWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, data, metadata)
}

// SearchResultsGetAPI is a GET/HEAD HTTP request API handler and it searches ElasticSearch index for the provided
// search session and returns to the client a JSON with an array of IDs of found documents.
// It returns search metadata (e.g., total results) as waf HTTP response header.
func (s *Service) SearchResultsGetAPI(w http.ResponseWriter, req *http.Request, params waf.Params) {
	ctx := req.Context()
	metrics := waf.MustGetMetrics(ctx)

	m := metrics.Duration(internalStore.MetricSearchSession).Start()
	searchSession, errE := search.GetSessionFromID(ctx, params["id"])
	m.Stop()
	if errors.Is(errE, search.ErrNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, store.ErrAccessDenied) {
		s.ForbiddenWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	index, handled := s.resolveReadIndex(w, req)
	if handled {
		return
	}

	factor, errE := s.scoreFactor(ctx, req, index)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	accessFilter, errE := searchAccessFilter(ctx)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	data, metadata, errE := search.ResultsGet(ctx, s.getSearchServiceClosure(req, index), &searchSession.SessionData, enabledSearchLanguages(ctx), factor, accessFilter)
	if errors.Is(errE, search.ErrValidationFailed) {
		s.BadRequestWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, data, metadata)
}

// SearchJustResultsPostAPI is a POST HTTP request API handler and it searches ElasticSearch index without
// creating a search session and returns to the client a JSON with an array of IDs of found documents.
// It returns search metadata (e.g., total results) as waf HTTP response header.
func (s *Service) SearchJustResultsPostAPI(w http.ResponseWriter, req *http.Request, _ waf.Params) {
	defer req.Body.Close()              //nolint:errcheck
	defer io.Copy(io.Discard, req.Body) //nolint:errcheck

	ctx := req.Context()

	var searchData search.SessionData
	errE := x.DecodeJSONWithoutUnknownFields(req.Body, &searchData)
	if errE != nil {
		s.BadRequestWithError(w, req, errE)
		return
	}

	errE = searchData.Validate(ctx, true)
	if errE != nil {
		errE = errors.WrapWith(errE, search.ErrValidationFailed)
		s.BadRequestWithError(w, req, errE)
		return
	}

	index, handled := s.resolveReadIndex(w, req)
	if handled {
		return
	}

	factor, errE := s.scoreFactor(ctx, req, index)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	accessFilter, errE := searchAccessFilter(ctx)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	data, metadata, errE := search.ResultsGet(ctx, s.getSearchServiceClosure(req, index), &searchData, enabledSearchLanguages(ctx), factor, accessFilter)
	if errors.Is(errE, search.ErrValidationFailed) {
		s.BadRequestWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, data, metadata)
}

// maxDuplicates is the maximum number of potential duplicates DocumentFindDuplicatesPostAPI returns.
const maxDuplicates = 5

// documentFindDuplicatesRequest is the JSON body of DocumentFindDuplicatesPostAPI. Document is the
// in-progress document being created (or edited), whose claims are compared structurally against the
// index. The document's own ID is excluded from the results, so it is never its own duplicate.
type documentFindDuplicatesRequest struct {
	Document json.RawMessage `json:"doc"`
}

// DocumentFindDuplicatesPostAPI is a POST HTTP request API handler which searches the ElasticSearch
// index for documents that potentially duplicate the one in the request, comparing them by structure:
// each of the document's claims (identifier, string, link, reference including INSTANCE_OF, amount,
// time, has) contributes a weighted match against documents that share that field, and candidates are
// ranked by the sum of matched weights (see search.DuplicatesGet). It returns to the client a JSON
// array of up to maxDuplicates result IDs above the match threshold, excluding the document itself. A
// document with no matchable claims yields an empty list.
func (s *Service) DocumentFindDuplicatesPostAPI(w http.ResponseWriter, req *http.Request, _ waf.Params) {
	defer req.Body.Close()              //nolint:errcheck
	defer io.Copy(io.Discard, req.Body) //nolint:errcheck

	ctx := req.Context()

	var request documentFindDuplicatesRequest
	errE := x.DecodeJSONWithoutUnknownFields(req.Body, &request)
	if errE != nil {
		s.BadRequestWithError(w, req, errE)
		return
	}

	// The client sends its in-progress, possibly partial, document. We unmarshal it leniently
	// (unknown fields are ignored) because it is used only to build a best-effort structural query,
	// not to mutate anything.
	doc := new(document.D)
	errE = x.Unmarshal(request.Document, doc)
	if errE != nil {
		s.BadRequestWithError(w, req, errE)
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

	duplicates, errE := search.DuplicatesGet(ctx, s.getSearchServiceClosure(req, index), doc, doc.ID, enabledSearchLanguages(ctx), maxDuplicates, accessFilter)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, duplicates, nil)
}

// SearchJustResultsGetAPI is a GET/HEAD HTTP request API handler which searches the
// ElasticSearch index without creating a search session. It accepts the same query
// parameter grammar as SearchShortcutGet and returns to the client a JSON with an
// array of IDs of found documents. It returns search metadata (e.g., total results)
// as waf HTTP response header.
func (s *Service) SearchJustResultsGetAPI(w http.ResponseWriter, req *http.Request, _ waf.Params) {
	ctx := req.Context()

	searchSession, errE := parseSearchShortcutQuery(ctx, req.URL.Query())
	if errE != nil {
		s.BadRequestWithError(w, req, errE)
		return
	}

	index, handled := s.resolveReadIndex(w, req)
	if handled {
		return
	}

	factor, errE := s.scoreFactor(ctx, req, index)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	accessFilter, errE := searchAccessFilter(ctx)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	data, metadata, errE := search.ResultsGet(ctx, s.getSearchServiceClosure(req, index), &searchSession.SessionData, enabledSearchLanguages(ctx), factor, accessFilter)
	if errors.Is(errE, search.ErrValidationFailed) {
		s.BadRequestWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, data, metadata)
}

type createSessionResponse struct {
	ID      identifier.Identifier `json:"id"`
	Base    []string              `json:"base"`
	Version int                   `json:"version"`
}

// createSessionRequest is the JSON body of SearchCreatePostAPI. Its optional query
// field sets the initial full-text query of the new search session.
type createSessionRequest struct {
	Query    string `json:"query,omitempty"`
	Language string `json:"language,omitempty"`
}

// SearchCreatePostAPI is a POST HTTP API request handler which creates a new search session.
func (s *Service) SearchCreatePostAPI(w http.ResponseWriter, req *http.Request, _ waf.Params) {
	defer req.Body.Close()              //nolint:errcheck
	defer io.Copy(io.Discard, req.Body) //nolint:errcheck

	ctx := req.Context()
	metrics := waf.MustGetMetrics(ctx)
	site := waf.MustGetSite[*internalSite.Site](ctx)

	var request createSessionRequest
	errE := x.DecodeJSONWithoutUnknownFields(req.Body, &request)
	if errE != nil {
		s.BadRequestWithError(w, req, errE)
		return
	}

	// TODO: Support configuring base and not just use the domain.
	base := []string{site.Domain, "SEARCH", identifier.New().String()}
	id := identifier.From(base...)

	sessionData := search.SessionData{
		View:          search.ViewFeed,
		Query:         request.Query,
		Language:      request.Language,
		Filters:       nil,
		Prefilters:    nil,
		Reverse:       nil,
		ReverseExpand: false,
		Sort:          nil,
	}

	searchSession := &search.Session{
		SessionData: sessionData,
		ID:          id,
		Base:        base,
		Version:     0,
	}

	m := metrics.Duration(internalStore.MetricSearchSession).Start()
	errE = search.CreateSession(ctx, searchSession)
	m.Stop()
	if errors.Is(errE, store.ErrAccessDenied) {
		s.ForbiddenWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, createSessionResponse{
		ID:      id,
		Base:    base,
		Version: 0,
	}, nil)
}

type updateSessionResponse struct {
	Version int `json:"version"`
}

// SearchUpdatePostAPI is a POST HTTP API request handler which updates the search session.
func (s *Service) SearchUpdatePostAPI(w http.ResponseWriter, req *http.Request, params waf.Params) {
	defer req.Body.Close()              //nolint:errcheck
	defer io.Copy(io.Discard, req.Body) //nolint:errcheck

	ctx := req.Context()
	metrics := waf.MustGetMetrics(ctx)

	var searchData search.SessionData
	errE := x.DecodeJSONWithoutUnknownFields(req.Body, &searchData)
	if errE != nil {
		s.BadRequestWithError(w, req, errE)
		return
	}

	existingSession, errE := search.GetSessionFromID(ctx, params["id"])
	if errors.Is(errE, search.ErrNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, store.ErrAccessDenied) {
		s.ForbiddenWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	newSession := &search.Session{
		SessionData: searchData,
		ID:          existingSession.ID,
		Base:        existingSession.Base,
		// TODO: This is not race safe, needs improvement once we have storage that supports transactions.
		Version: existingSession.Version + 1,
	}

	m := metrics.Duration(internalStore.MetricSearchSession).Start()
	errE = search.UpdateSession(ctx, newSession)
	m.Stop()
	if errors.Is(errE, search.ErrNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, search.ErrValidationFailed) {
		s.BadRequestWithError(w, req, errE)
		return
	} else if errors.Is(errE, store.ErrAccessDenied) {
		s.ForbiddenWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, updateSessionResponse{
		Version: newSession.Version,
	}, nil)
}

// shortcutPropKey identifies a filter group parsed from search shortcut query parameters.
// When Nested is true, Parent and Prop together identify a sub-ref filter; otherwise
// Prop is a top-level prop.
type shortcutPropKey struct {
	Parent identifier.Identifier
	Prop   identifier.Identifier
	Nested bool
}

// shortcutDirectQueryPrefix prefixes a query parameter value to select its identifier as a "direct"
// (most-specific) match instead of a plain target. It mirrors the "direct:<identifier>" form of the
// shortcut string grammar.
const shortcutDirectQueryPrefix = internalShortcut.DirectValue + internalShortcut.PathSeparator

// shortcutQueryGroup accumulates the selections parsed for one filter group from search shortcut query
// parameters: plain target values (To), "direct" most-specific target values, and whether the "missing"
// bucket was selected.
type shortcutQueryGroup struct {
	To      []search.ToValue
	Direct  []search.ToValue
	Missing bool
}

// add parses one query parameter value into the group: the literal "missing" selects the missing bucket,
// a "direct:<identifier>" value adds a most-specific target, and any other value is a plain target. The
// identifier (for the plain and direct forms) must be valid.
func (g *shortcutQueryGroup) add(value string) errors.E {
	if value == internalShortcut.MissingValue {
		g.Missing = true
		return nil
	}
	if idStr, ok := strings.CutPrefix(value, shortcutDirectQueryPrefix); ok {
		valueID, errE := identifier.MaybeString(idStr)
		if errE != nil {
			return errors.WithMessage(errE, "query parameter direct value is not a valid identifier")
		}
		g.Direct = append(g.Direct, search.ToValue{ID: valueID})
		return nil
	}
	valueID, errE := identifier.MaybeString(value)
	if errE != nil {
		return errors.WithMessage(errE, "query parameter value is not a valid identifier")
	}
	g.To = append(g.To, search.ToValue{ID: valueID})
	return nil
}

// parseShortcutPropKey parses a query parameter key into a shortcutPropKey: a single property, or a
// "parent:prop" nested sub-reference. Both sides must be valid identifiers.
func parseShortcutPropKey(prop string) (shortcutPropKey, errors.E) {
	if parentStr, propStr, ok := strings.Cut(prop, ":"); ok {
		parentID, errE := identifier.MaybeString(parentStr)
		if errE != nil {
			return shortcutPropKey{}, errors.WithMessage(errE, "query parameter key parent prop is not a valid identifier")
		}
		propID, errE := identifier.MaybeString(propStr)
		if errE != nil {
			return shortcutPropKey{}, errors.WithMessage(errE, "query parameter key nested prop is not a valid identifier")
		}
		return shortcutPropKey{Parent: parentID, Prop: propID, Nested: true}, nil
	}
	propID, errE := identifier.MaybeString(prop)
	if errE != nil {
		return shortcutPropKey{}, errors.WithMessage(errE, "query parameter key is not a valid identifier")
	}
	return shortcutPropKey{Parent: identifier.Identifier{}, Prop: propID, Nested: false}, nil
}

// parseShortcutQueryGroups parses search shortcut query parameters into per-property filter groups plus
// the optional reverse target, the optional language, and the optional full-text query. Each value is a
// plain identifier (a target), the literal "missing" (the missing bucket), or "direct:<identifier>" (a
// most-specific target). It is the pure core of parseSearchShortcutQuery and carries no site or session concerns.
func parseShortcutQueryGroups(query url.Values) (map[shortcutPropKey]*shortcutQueryGroup, *identifier.Identifier, string, string, errors.E) {
	groups := map[shortcutPropKey]*shortcutQueryGroup{}
	var reverse *identifier.Identifier
	var language string
	var fullTextQuery string
	for prop, values := range query {
		if prop == internalShortcut.ReverseKey {
			if len(values) != 1 {
				return nil, nil, "", "", errors.New(`"reverse" query parameter must be set exactly once`)
			}
			reverseID, errE := identifier.MaybeString(values[0])
			if errE != nil {
				return nil, nil, "", "", errors.WithMessage(errE, `"reverse" query parameter value is not a valid identifier`)
			}
			reverse = &reverseID
			continue
		}
		if prop == "language" {
			if len(values) != 1 {
				return nil, nil, "", "", errors.New(`"language" query parameter must be set exactly once`)
			}
			language = values[0]
			continue
		}
		if prop == "q" {
			if len(values) != 1 {
				return nil, nil, "", "", errors.New(`"q" query parameter must be set exactly once`)
			}
			fullTextQuery = values[0]
			continue
		}
		key, errE := parseShortcutPropKey(prop)
		if errE != nil {
			return nil, nil, "", "", errE
		}
		group := groups[key]
		if group == nil {
			group = &shortcutQueryGroup{To: nil, Direct: nil, Missing: false}
			groups[key] = group
		}
		for _, value := range values {
			errE := group.add(value)
			if errE != nil {
				return nil, nil, "", "", errE
			}
		}
	}
	return groups, reverse, language, fullTextQuery, nil
}

// parseSearchShortcutQuery parses query parameters using the search shortcut grammar
// described on SearchShortcutGet into a search.Session whose Prefilters carry the parsed values.
func parseSearchShortcutQuery(ctx context.Context, query url.Values) (*search.Session, errors.E) {
	site := waf.MustGetSite[*internalSite.Site](ctx)

	groups, reverse, language, fullTextQuery, errE := parseShortcutQueryGroups(query)
	if errE != nil {
		return nil, errE
	}

	// TODO: Support configuring base and not just use the domain.
	base := []string{site.Domain, "SEARCH", identifier.New().String()}
	id := identifier.From(base...)

	searchData := search.SessionData{
		View:          search.ViewFeed,
		Query:         fullTextQuery,
		Language:      language,
		Filters:       nil,
		Prefilters:    nil,
		Reverse:       reverse,
		ReverseExpand: false,
		Sort:          nil,
	}

	for key, group := range groups {
		filterBase := append(slices.Clone(base), "FILTER", identifier.New().String())
		filterID := identifier.From(filterBase...)
		var props []identifier.Identifier
		if key.Nested {
			props = []identifier.Identifier{key.Parent, key.Prop}
		} else {
			props = []identifier.Identifier{key.Prop}
		}
		// Search shortcuts populate Prefilters (not Filters): the shortcut defines the scope the
		// user is looking at, so it constrains results without contributing to ranking, and the
		// original values are kept (not expanded to descendants) so the UI can show what the
		// prefilter is on.
		searchData.Prefilters = append(searchData.Prefilters, search.Filter{
			ID:   &filterID,
			Base: filterBase,
			Prop: props,
			Ref: &search.RefFilter{
				To:      group.To,
				Direct:  group.Direct,
				Missing: group.Missing,
			},
			Amount: nil,
			Time:   nil,
			Has:    nil,
		})
	}

	searchSession := &search.Session{
		SessionData: searchData,
		ID:          id,
		Base:        base,
		Version:     0,
	}

	errE = searchSession.Validate(ctx)
	if errE != nil {
		return nil, errors.WrapWith(errE, search.ErrValidationFailed)
	}

	return searchSession, nil
}

// createShortcutSession parses the search shortcut query grammar into prefilters and creates the
// session. On any failure it writes the appropriate error response and returns nil, so callers
// must return when it returns nil.
func (s *Service) createShortcutSession(w http.ResponseWriter, req *http.Request, query url.Values) *search.Session {
	ctx := req.Context()
	metrics := waf.MustGetMetrics(ctx)

	searchSession, errE := parseSearchShortcutQuery(ctx, query)
	if errE != nil {
		s.BadRequestWithError(w, req, errE)
		return nil
	}

	m := metrics.Duration(internalStore.MetricSearchSession).Start()
	errE = search.CreateSession(ctx, searchSession)
	m.Stop()
	if errors.Is(errE, store.ErrAccessDenied) {
		s.ForbiddenWithError(w, req, errE)
		return nil
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return nil
	}

	return searchSession
}

// SearchShortcutGet is a GET/HEAD HTTP request handler which creates a new search session
// from query parameters and redirects to the search page. Query parameters are interpreted
// as ref prefilters where key is the property ID and value is the value ID (prefilters scope
// the results without contributing to ranking). Values for the same property are grouped into
// a single prefilter.
//
// A key of the form "parentProp:prop" creates a nested (sub-ref) prefilter, matching
// reference sub-claims under parentProp whose property is prop.
//
// A value is normally a target value ID, but two forms are special: the literal "missing"
// selects the property's missing bucket (documents that have no claim for it), and a value of
// the form "direct:<valueID>" selects the target as a most-specific (leaf) match. Both can be
// mixed with plain target values for the same property.
//
// The "reverse" query parameter is special: its value is a document ID that scopes
// the session to documents which reference that ID via any property.
//
// The "q" query parameter is special: its value sets the session's full-text query, so a shortcut
// can combine prefilters with a free-text search (for example the query already typed in the navbar).
func (s *Service) SearchShortcutGet(w http.ResponseWriter, req *http.Request, _ waf.Params) {
	searchSession := s.createShortcutSession(w, req, req.URL.Query())
	if searchSession == nil {
		return
	}

	path, errE := s.Reverse("SearchGet", waf.Params{"id": searchSession.ID.String()}, nil)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.TemporaryRedirectGetMethod(w, req, path)
}

// SearchShortcutRequest is the JSON body of SearchShortcutPostAPI. Its only field carries
// the shortcut as a URL query string, the same form the GET handler reads from the
// request URL.
type SearchShortcutRequest struct {
	Query string `json:"query"`
}

// SearchShortcutPostAPI is the API counterpart of SearchShortcutGet: it creates the search
// session from the search shortcut and returns the created session as JSON instead of
// redirecting. The frontend uses it for client-side shortcut navigation, which never reaches
// the redirecting GET handler.
func (s *Service) SearchShortcutPostAPI(w http.ResponseWriter, req *http.Request, _ waf.Params) {
	defer req.Body.Close()              //nolint:errcheck
	defer io.Copy(io.Discard, req.Body) //nolint:errcheck

	var request SearchShortcutRequest
	errE := x.DecodeJSONWithoutUnknownFields(req.Body, &request)
	if errE != nil {
		s.BadRequestWithError(w, req, errE)
		return
	}

	query, err := url.ParseQuery(request.Query)
	if err != nil {
		s.BadRequestWithError(w, req, errors.WithStack(err))
		return
	}

	searchSession := s.createShortcutSession(w, req, query)
	if searchSession == nil {
		return
	}

	s.WriteJSON(w, req, createSessionResponse{
		ID:      searchSession.ID,
		Base:    searchSession.Base,
		Version: searchSession.Version,
	}, nil)
}
