package peerdb

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	esSearch "github.com/elastic/go-elasticsearch/v9/typedapi/core/search"
	"github.com/elastic/go-elasticsearch/v9/typedapi/esdsl"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"
	"gitlab.com/tozd/waf"

	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
	internalStore "gitlab.com/peerdb/peerdb/internal/store"
	"gitlab.com/peerdb/peerdb/search"
	"gitlab.com/peerdb/peerdb/store"
)

// enabledSearchLanguages returns the indexed language set for the site serving the request,
// used to scope the text-search query to the languages the index actually has.
func enabledSearchLanguages(ctx context.Context) []string {
	site := waf.MustGetSite[*Site](ctx)
	return internalSearch.EnabledLanguages(site.LanguagePriority)
}

// searchAccessFilter returns the site's optional per-caller search restriction
// (from base.B.SearchQueryHook) for the request in ctx, or a nil query if the
// site sets no hook. It is added as a filter clause to every search query so
// results and facets only include documents the caller may access.
func searchAccessFilter(ctx context.Context) (types.QueryVariant, errors.E) { //nolint:ireturn
	site := waf.MustGetSite[*Site](ctx)
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

func (s *Service) getSearchService(req *http.Request) *esSearch.Search {
	ctx := req.Context()

	site := waf.MustGetSite[*Site](ctx)

	// We set TrackTotalHits to true to always get exact number of results. For now we didn't notice any performance
	// issues at data scale PeerDB is currently being used with, but in the future we might want to make this configurable.
	return site.ESClient.Search().Index(site.Index).
		Source_(esdsl.NewSourceConfig().Bool(false)).
		Preference(getHost(req.RemoteAddr)).
		Header("X-Opaque-ID", waf.MustRequestID(ctx).String()).
		TrackTotalHits(esdsl.NewTrackHits().Bool(true)).
		AllowPartialSearchResults(false)
}

func (s *Service) getSearchServiceClosure(req *http.Request) func() *esSearch.Search {
	return func() *esSearch.Search {
		return s.getSearchService(req)
	}
}

// scoreFactorTTL is how long a cached scoreCount boost factor is reused before it
// is recomputed from the corpus.
const scoreFactorTTL = time.Hour

type scoreFactorEntry struct {
	factor   float64
	computed time.Time
}

// scoreFactor returns the scoreCount ranking boost factor for the site serving the
// request, computed via search.ScoreFactor and cached per index for scoreFactorTTL.
func (s *Service) scoreFactor(ctx context.Context, req *http.Request) (float64, errors.E) {
	site := waf.MustGetSite[*Site](ctx)

	s.scoreFactorMu.Lock()
	defer s.scoreFactorMu.Unlock()

	entry, ok := s.scoreFactorCache[site.Index]
	if ok && time.Since(entry.computed) < scoreFactorTTL {
		return entry.factor, nil
	}

	factor, errE := search.ScoreFactor(ctx, s.getSearchServiceClosure(req))
	if errE != nil {
		return 0, errE
	}

	s.scoreFactorCache[site.Index] = scoreFactorEntry{factor: factor, computed: time.Now()}

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

	var data any
	var metadata map[string]any

	searchService := s.getSearchServiceClosure(req)
	switch {
	case f.Ref != nil:
		if len(f.Prop) == 2 { //nolint:mnd
			data, metadata, errE = f.Ref.GetSubRef(ctx, searchService, query, f.Prop[0], f.Prop[1],
				collectParentToFromSession(searchSession.Filters, f.Prop[0]))
		} else {
			data, metadata, errE = f.Ref.Get(ctx, searchService, query, f.Prop[0])
		}
	case f.Amount != nil:
		if len(f.Prop) == 2 { //nolint:mnd
			data, metadata, errE = f.Amount.GetSubAmount(ctx, searchService, query, f.Prop[0], f.Prop[1],
				collectParentToFromSession(searchSession.Filters, f.Prop[0]))
		} else {
			data, metadata, errE = f.Amount.Get(ctx, searchService, query, f.Prop[0])
		}
	case f.Time != nil:
		if len(f.Prop) == 2 { //nolint:mnd
			data, metadata, errE = f.Time.GetSubTime(ctx, searchService, query, f.Prop[0], f.Prop[1],
				collectParentToFromSession(searchSession.Filters, f.Prop[0]))
		} else {
			data, metadata, errE = f.Time.Get(ctx, searchService, query, f.Prop[0])
		}
	case f.Has != nil:
		if len(f.Prop) == 1 {
			data, metadata, errE = f.Has.GetSubHas(ctx, searchService, query, f.Prop[0],
				collectParentToFromSession(searchSession.Filters, f.Prop[0]))
		} else {
			data, metadata, errE = f.Has.Get(ctx, searchService, query)
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
//
//nolint:dupl
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
	f := search.RefFilter{}
	data, metadata, errE := f.Get(ctx, s.getSearchServiceClosure(req), query, prop)
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
	f := search.AmountFilter{Unit: unit} //nolint:exhaustruct
	data, metadata, errE := f.Get(ctx, s.getSearchServiceClosure(req), query, prop)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, data, metadata)
}

// SearchTimeFilterGetAPI handles GET requests for time filter data by property.
//
// Used for inactive filters (not yet in the session).
//
//nolint:dupl
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
	f := search.TimeFilter{}
	data, metadata, errE := f.Get(ctx, s.getSearchServiceClosure(req), query, prop)
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
	f := search.RefFilter{}
	data, metadata, errE := f.GetSubRef(ctx, s.getSearchServiceClosure(req), query, parentProp, prop, parentToRestrictions)
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
	f := search.AmountFilter{Unit: unit} //nolint:exhaustruct
	data, metadata, errE := f.GetSubAmount(ctx, s.getSearchServiceClosure(req), query, parentProp, prop, parentToRestrictions)
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
	f := search.TimeFilter{}
	data, metadata, errE := f.GetSubTime(ctx, s.getSearchServiceClosure(req), query, parentProp, prop, parentToRestrictions)
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
	f := search.HasFilter{}
	data, metadata, errE := f.Get(ctx, s.getSearchServiceClosure(req), query)
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
	f := search.HasFilter{}
	data, metadata, errE := f.GetSubHas(ctx, s.getSearchServiceClosure(req), query, parentProp, parentToRestrictions)
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

	data, metadata, errE := search.FiltersGet(ctx, s.getSearchServiceClosure(req), searchSession, enabledSearchLanguages(ctx), accessFilter)
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

	factor, errE := s.scoreFactor(ctx, req)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	accessFilter, errE := searchAccessFilter(ctx)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	data, metadata, errE := search.ResultsGet(ctx, s.getSearchServiceClosure(req), &searchSession.SessionData, enabledSearchLanguages(ctx), factor, accessFilter)
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

	errE = searchData.Validate(true)
	if errE != nil {
		errE = errors.WrapWith(errE, search.ErrValidationFailed)
		s.BadRequestWithError(w, req, errE)
		return
	}

	factor, errE := s.scoreFactor(ctx, req)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	accessFilter, errE := searchAccessFilter(ctx)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	data, metadata, errE := search.ResultsGet(ctx, s.getSearchServiceClosure(req), &searchData, enabledSearchLanguages(ctx), factor, accessFilter)
	if errors.Is(errE, search.ErrValidationFailed) {
		s.BadRequestWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, data, metadata)
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

	factor, errE := s.scoreFactor(ctx, req)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	accessFilter, errE := searchAccessFilter(ctx)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	data, metadata, errE := search.ResultsGet(ctx, s.getSearchServiceClosure(req), &searchSession.SessionData, enabledSearchLanguages(ctx), factor, accessFilter)
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

// SearchCreatePostAPI is a POST HTTP API request handler which creates a new empty search session.
func (s *Service) SearchCreatePostAPI(w http.ResponseWriter, req *http.Request, _ waf.Params) {
	defer req.Body.Close()              //nolint:errcheck
	defer io.Copy(io.Discard, req.Body) //nolint:errcheck

	ctx := req.Context()
	metrics := waf.MustGetMetrics(ctx)
	site := waf.MustGetSite[*Site](ctx)

	var ea emptyRequest
	errE := x.DecodeJSONWithoutUnknownFields(req.Body, &ea)
	if errE != nil {
		s.BadRequestWithError(w, req, errE)
		return
	}

	// TODO: Support configuring base and not just use the domain.
	base := []string{site.Domain, "SEARCH", identifier.New().String()}
	id := identifier.From(base...)

	searchSession := &search.Session{
		SessionData: search.SessionData{
			View:    search.ViewFeed,
			Query:   "",
			Filters: nil,
			Reverse: nil,
		},
		ID:      id,
		Base:    base,
		Version: 0,
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
// When nested is true, parent and prop together identify a sub-ref filter; otherwise
// prop is a top-level prop.
type shortcutPropKey struct {
	parent identifier.Identifier
	prop   identifier.Identifier
	nested bool
}

// parseSearchShortcutQuery parses query parameters using the search shortcut grammar
// described on SearchShortcutGet into a search.Session.
func parseSearchShortcutQuery(ctx context.Context, query url.Values) (*search.Session, errors.E) {
	site := waf.MustGetSite[*Site](ctx)

	// Group values by property.
	filterMap := map[shortcutPropKey][]search.ToValue{}
	var reverse *identifier.Identifier
	for prop, values := range query {
		if prop == "reverse" {
			if len(values) != 1 {
				return nil, errors.New(`"reverse" query parameter must be set exactly once`)
			}
			reverseID, errE := identifier.MaybeString(values[0])
			if errE != nil {
				return nil, errors.WithMessage(errE, `"reverse" query parameter value is not a valid identifier`)
			}
			reverse = &reverseID
			continue
		}
		var key shortcutPropKey
		if parentStr, propStr, ok := strings.Cut(prop, ":"); ok {
			parentID, errE := identifier.MaybeString(parentStr)
			if errE != nil {
				return nil, errors.WithMessage(errE, "query parameter key parent prop is not a valid identifier")
			}
			propID, errE := identifier.MaybeString(propStr)
			if errE != nil {
				return nil, errors.WithMessage(errE, "query parameter key nested prop is not a valid identifier")
			}
			key = shortcutPropKey{parent: parentID, prop: propID, nested: true}
		} else {
			propID, errE := identifier.MaybeString(prop)
			if errE != nil {
				return nil, errors.WithMessage(errE, "query parameter key is not a valid identifier")
			}
			key = shortcutPropKey{parent: identifier.Identifier{}, prop: propID, nested: false}
		}
		for _, value := range values {
			valueID, errE := identifier.MaybeString(value)
			if errE != nil {
				return nil, errors.WithMessage(errE, "query parameter value is not a valid identifier")
			}
			filterMap[key] = append(filterMap[key], search.ToValue{ID: valueID})
		}
	}

	// TODO: Support configuring base and not just use the domain.
	base := []string{site.Domain, "SEARCH", identifier.New().String()}
	id := identifier.From(base...)

	searchData := search.SessionData{
		View:    search.ViewFeed,
		Query:   "",
		Filters: nil,
		Reverse: reverse,
	}

	for key, toValues := range filterMap {
		filterBase := append(slices.Clone(base), "FILTER", identifier.New().String())
		filterID := identifier.From(filterBase...)
		var props []identifier.Identifier
		if key.nested {
			props = []identifier.Identifier{key.parent, key.prop}
		} else {
			props = []identifier.Identifier{key.prop}
		}
		searchData.Filters = append(searchData.Filters, search.Filter{
			ID:   &filterID,
			Base: filterBase,
			Prop: props,
			Ref: &search.RefFilter{
				To:      toValues,
				Missing: false,
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

	errE := searchSession.Validate()
	if errE != nil {
		return nil, errors.WrapWith(errE, search.ErrValidationFailed)
	}

	return searchSession, nil
}

// SearchShortcutGet is a GET/HEAD HTTP request handler which creates a new search session
// from query parameters and redirects to the search page. Query parameters are interpreted
// as ref filters where key is the property ID and value is the value ID.
// Values for the same property are grouped into a single filter.
//
// A key of the form "parentProp:prop" creates a nested (sub-ref) filter, matching
// reference sub-claims under parentProp whose property is prop.
//
// The "reverse" query parameter is special: its value is a document ID that scopes
// the session to documents which reference that ID via any property.
func (s *Service) SearchShortcutGet(w http.ResponseWriter, req *http.Request, _ waf.Params) {
	ctx := req.Context()
	metrics := waf.MustGetMetrics(ctx)

	searchSession, errE := parseSearchShortcutQuery(ctx, req.URL.Query())
	if errE != nil {
		s.BadRequestWithError(w, req, errE)
		return
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

	path, errE := s.Reverse("SearchGet", waf.Params{"id": searchSession.ID.String()}, nil)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.TemporaryRedirectGetMethod(w, req, path)
}
