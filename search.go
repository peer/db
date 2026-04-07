package peerdb

import (
	"io"
	"net/http"
	"slices"

	esSearch "github.com/elastic/go-elasticsearch/v9/typedapi/core/search"
	"github.com/elastic/go-elasticsearch/v9/typedapi/esdsl"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"
	"gitlab.com/tozd/waf"

	internalStore "gitlab.com/peerdb/peerdb/internal/store"
	"gitlab.com/peerdb/peerdb/search"
)

func (s *Service) getSearchService(req *http.Request) (*esSearch.Search, int64, int64) {
	ctx := req.Context()

	site := waf.MustGetSite[*Site](ctx)

	// We set TrackTotalHits to true to always get exact number of results. For now we didn't notice any performance
	// issues at data scale PeerDB is currently being used with, but in the future we might want to make this configurable.
	return site.ESClient.Search().Index(site.Index).
		Source_(esdsl.NewSourceConfig().Bool(false)).
		Preference(getHost(req.RemoteAddr)).
		Header("X-Opaque-ID", waf.MustRequestID(ctx).String()).
		TrackTotalHits(esdsl.NewTrackHits().Bool(true)).
		AllowPartialSearchResults(false), site.propertiesTotal, site.unitsTotal
}

func (s *Service) getSearchServiceClosure(req *http.Request) func() (*esSearch.Search, int64, int64) {
	return func() (*esSearch.Search, int64, int64) {
		return s.getSearchService(req)
	}
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

	query := searchSession.ToQueryExcluding(filterID)

	var data any
	var metadata map[string]any

	searchService := s.getSearchServiceClosure(req)
	switch {
	case f.Ref != nil:
		if len(f.Prop) == 2 { //nolint:mnd
			// Sub-ref filter: find parentTo restrictions from active parent ref filter.
			var parentToRestrictions []identifier.Identifier
			for _, other := range searchSession.Filters {
				if other.Ref != nil && len(other.Prop) == 1 && other.Prop[0] == f.Prop[0] {
					for _, to := range other.Ref.To {
						parentToRestrictions = append(parentToRestrictions, to.ID)
					}
				}
			}
			data, metadata, errE = f.Ref.GetSubRef(ctx, searchService, query, f.Prop[0], f.Prop[1], parentToRestrictions)
		} else {
			data, metadata, errE = f.Ref.Get(ctx, searchService, query, f.Prop[0])
		}
	case f.Amount != nil:
		data, metadata, errE = f.Amount.Get(ctx, searchService, query, f.Prop[0])
	case f.Time != nil:
		data, metadata, errE = f.Time.Get(ctx, searchService, query, f.Prop[0])
	case f.Has != nil:
		data, metadata, errE = f.Has.Get(ctx, searchService, query)
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
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	query := searchSession.ToQuery()
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
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	query := searchSession.ToQuery()
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
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	query := searchSession.ToQuery()
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
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	// Find parentTo restrictions from active parent ref filter for cross-filtering.
	var parentToRestrictions []identifier.Identifier
	for _, f := range searchSession.Filters {
		if f.Ref != nil && len(f.Prop) == 1 && f.Prop[0] == parentProp {
			for _, to := range f.Ref.To {
				parentToRestrictions = append(parentToRestrictions, to.ID)
			}
		}
	}

	query := searchSession.ToQuery()
	f := search.RefFilter{}
	data, metadata, errE := f.GetSubRef(ctx, s.getSearchServiceClosure(req), query, parentProp, prop, parentToRestrictions)
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
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	query := searchSession.ToQuery()
	f := search.HasFilter{}
	data, metadata, errE := f.Get(ctx, s.getSearchServiceClosure(req), query)
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
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	data, metadata, errE := search.FiltersGet(ctx, s.getSearchServiceClosure(req), searchSession)
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
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	data, metadata, errE := search.ResultsGet(ctx, s.getSearchServiceClosure(req), &searchSession.SessionData)
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

	data, metadata, errE := search.ResultsGet(ctx, s.getSearchServiceClosure(req), &searchData)
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
		},
		ID:      id,
		Base:    base,
		Version: 0,
	}

	m := metrics.Duration(internalStore.MetricSearchSession).Start()
	errE = search.CreateSession(ctx, searchSession)
	m.Stop()
	if errE != nil {
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
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, updateSessionResponse{
		Version: newSession.Version,
	}, nil)
}

// SearchShortcutGet is a GET/HEAD HTTP request handler which creates a new search session
// from query parameters and redirects to the search page. Query parameters are interpreted
// as ref filters where key is the property ID and value is the value ID.
// Values for the same property are grouped into a single filter.
func (s *Service) SearchShortcutGet(w http.ResponseWriter, req *http.Request, _ waf.Params) {
	ctx := req.Context()
	metrics := waf.MustGetMetrics(ctx)
	site := waf.MustGetSite[*Site](ctx)

	// Group values by property.
	filterMap := map[identifier.Identifier][]search.ToValue{}
	for prop, values := range req.URL.Query() {
		propID, errE := identifier.MaybeString(prop)
		if errE != nil {
			s.BadRequestWithError(w, req, errors.WithMessage(errE, "query parameter key is not a valid identifier"))
			return
		}
		for _, value := range values {
			valueID, errE := identifier.MaybeString(value)
			if errE != nil {
				s.BadRequestWithError(w, req, errors.WithMessage(errE, "query parameter value is not a valid identifier"))
				return
			}
			filterMap[propID] = append(filterMap[propID], search.ToValue{ID: valueID})
		}
	}

	// TODO: Support configuring base and not just use the domain.
	base := []string{site.Domain, "SEARCH", identifier.New().String()}
	id := identifier.From(base...)

	searchData := search.SessionData{
		View:    search.ViewFeed,
		Query:   "",
		Filters: nil,
	}

	for propID, toValues := range filterMap {
		filterBase := append(slices.Clone(base), "FILTER", identifier.New().String())
		filterID := identifier.From(filterBase...)
		searchData.Filters = append(searchData.Filters, search.Filter{
			ID:   &filterID,
			Base: filterBase,
			Prop: []identifier.Identifier{propID},
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

	m := metrics.Duration(internalStore.MetricSearchSession).Start()
	errE := search.CreateSession(ctx, searchSession)
	m.Stop()
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	path, errE := s.Reverse("SearchGet", waf.Params{"id": id.String()}, nil)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.TemporaryRedirectGetMethod(w, req, path)
}
