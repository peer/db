package peerdb

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/olivere/elastic/v7"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"
	"gitlab.com/tozd/waf"

	internal "gitlab.com/peerdb/peerdb/internal/store"
	"gitlab.com/peerdb/peerdb/search"
)

// TODO: Limit properties only to those really used in filters ("rel", "amount", "amountRange")?

func (s *Service) populatePropertiesTotal(ctx context.Context) errors.E {
	boolQuery := elastic.NewBoolQuery().Must(
		elastic.NewTermQuery("claims.rel.prop.id", "CAfaL1ZZs6L4uyFdrJZ2wN"), // TYPE.
		elastic.NewTermQuery("claims.rel.to.id", "HohteEmv2o7gPRnJ5wukVe"),   // PROPERTY.
	)
	query := elastic.NewNestedQuery("claims.rel", boolQuery)

	for _, site := range s.Sites {
		total, err := s.esClient.Count(site.Index).Query(query).Do(ctx)
		if err != nil {
			return errors.Errorf(`site "%s": %w`, site.Index, err)
		}
		site.propertiesTotal = total
	}

	return nil
}

func (s *Service) getSearchService(req *http.Request) (*elastic.SearchService, int64) {
	ctx := req.Context()

	site := waf.MustGetSite[*Site](ctx)

	// We set TrackTotalHits to true to always get exact number of results. For now we didn't notice any performance
	// issues at data scale PeerDB is currently being used with, but in the future we might want to make this configurable.
	return s.esClient.Search(site.Index).FetchSource(false).Preference(getHost(req.RemoteAddr)).
		Header("X-Opaque-ID", waf.MustRequestID(ctx).String()).TrackTotalHits(true).AllowPartialSearchResults(false), site.propertiesTotal
}

func (s *Service) getSearchServiceClosure(req *http.Request) func() (*elastic.SearchService, int64) {
	return func() (*elastic.SearchService, int64) {
		return s.getSearchService(req)
	}
}

func (s *Service) SearchAmountFilterGet(w http.ResponseWriter, req *http.Request, params waf.Params) {
	id, errE := identifier.FromString(params["s"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"s" is not a valid identifier`))
		return
	}

	prop, errE := identifier.FromString(params["prop"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"prop" is not a valid identifier`))
		return
	}

	data, metadata, errE := search.AmountFilterGet(req.Context(), s.getSearchServiceClosure(req), id, prop, params["unit"])
	if errors.Is(errE, search.ErrNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, search.ErrInvalidArgument) {
		s.BadRequestWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, data, metadata)
}

func (s *Service) SearchFiltersGet(w http.ResponseWriter, req *http.Request, params waf.Params) {
	id, errE := identifier.FromString(params["s"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"s" is not a valid identifier`))
		return
	}

	data, metadata, errE := search.FiltersGet(req.Context(), s.getSearchServiceClosure(req), id)
	if errors.Is(errE, search.ErrNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, search.ErrInvalidArgument) {
		s.BadRequestWithError(w, req, errE)
		return
	} else if errors.Is(errE, search.ErrNotReady) {
		s.WithError(req.Context(), errE)
		waf.Error(w, req, http.StatusConflict)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, data, metadata)
}

//nolint:dupl
func (s *Service) SearchRelFilterGet(w http.ResponseWriter, req *http.Request, params waf.Params) {
	id, errE := identifier.FromString(params["s"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"s" is not a valid identifier`))
		return
	}

	prop, errE := identifier.FromString(params["prop"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"prop" is not a valid identifier`))
		return
	}

	data, metadata, errE := search.RelFilterGet(req.Context(), s.getSearchServiceClosure(req), id, prop)
	if errors.Is(errE, search.ErrNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, search.ErrInvalidArgument) {
		s.BadRequestWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, data, metadata)
}

//nolint:dupl
func (s *Service) SearchStringFilterGet(w http.ResponseWriter, req *http.Request, params waf.Params) {
	id, errE := identifier.FromString(params["s"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"s" is not a valid identifier`))
		return
	}

	prop, errE := identifier.FromString(params["prop"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"prop" is not a valid identifier`))
		return
	}

	data, metadata, errE := search.StringFilterGet(req.Context(), s.getSearchServiceClosure(req), id, prop)
	if errors.Is(errE, search.ErrNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, search.ErrInvalidArgument) {
		s.BadRequestWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, data, metadata)
}

//nolint:dupl
func (s *Service) SearchTimeFilterGet(w http.ResponseWriter, req *http.Request, params waf.Params) {
	id, errE := identifier.FromString(params["s"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"s" is not a valid identifier`))
		return
	}

	prop, errE := identifier.FromString(params["prop"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"prop" is not a valid identifier`))
		return
	}

	data, metadata, errE := search.TimeFilterGet(req.Context(), s.getSearchServiceClosure(req), id, prop)
	if errors.Is(errE, search.ErrNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, search.ErrInvalidArgument) {
		s.BadRequestWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, data, metadata)
}

// SearchResults is a GET/HEAD HTTP request handler which returns HTML frontend for searching documents.
// If search state is invalid, it redirects to a valid one.
func (s *Service) SearchResults(w http.ResponseWriter, req *http.Request, params waf.Params) {
	ctx := req.Context()
	metrics := waf.MustGetMetrics(ctx)

	var searchQuery *string
	if req.Form.Has("q") {
		q := req.Form.Get("q")
		searchQuery = &q
	}

	var filters *string
	if req.Form.Has("filters") {
		f := req.Form.Get("filters")
		filters = &f
	}

	m := metrics.Duration(internal.MetricSearchState).Start()
	sh, ok := search.GetOrCreateState(ctx, params["s"], searchQuery, filters)
	m.Stop()
	if !ok {
		// Something was not OK, so we redirect to the correct URL.
		path, err := s.Reverse("SearchResults", waf.Params{"s": sh.ID.String()}, sh.Values())
		if err != nil {
			s.InternalServerErrorWithError(w, req, err)
			return
		}
		w.Header().Set("Location", path)
		w.WriteHeader(http.StatusSeeOther)
		return
	}

	s.Home(w, req, nil)
}

type searchResult struct {
	ID string `json:"id"`
}

// SearchResultsGet is a GET/HEAD HTTP request handler and it searches ElasticSearch index using provided
// search state and returns to the client a JSON with an array of IDs of found documents.
// It returns search metadata (e.g., total results) as PeerDB HTTP response headers.
func (s *Service) SearchResultsGet(w http.ResponseWriter, req *http.Request, params waf.Params) {
	ctx := req.Context()
	metrics := waf.MustGetMetrics(ctx)

	// TODO: Move most of this logic to search package (similar to SearchFiltersGet).

	m := metrics.Duration(internal.MetricSearchState).Start()
	sh := search.GetState(params["s"])
	m.Stop()
	if sh == nil {
		s.NotFound(w, req)
		return
	}

	searchService, _ := s.getSearchService(req)
	searchService = searchService.From(0).Size(search.MaxResultsCount).Query(sh.Query())

	m = metrics.Duration(internal.MetricElasticSearch).Start()
	res, err := searchService.Do(ctx)
	m.Stop()
	if err != nil {
		s.InternalServerErrorWithError(w, req, errors.WithStack(err))
		return
	}
	metrics.Duration(internal.MetricElasticSearchInternal).Duration = time.Duration(res.TookInMillis) * time.Millisecond

	results := make([]searchResult, len(res.Hits.Hits))
	for i, hit := range res.Hits.Hits {
		results[i] = searchResult{ID: hit.Id}
	}

	// Total is a string or a number.
	var total interface{}
	if res.Hits.TotalHits.Relation == "gte" {
		total = fmt.Sprintf("%d+", res.Hits.TotalHits.Value)
	} else {
		total = res.Hits.TotalHits.Value
	}

	s.WriteJSON(w, req, results, map[string]interface{}{
		"total": total,
	})
}

// SearchGetGet is a GET/HEAD HTTP request handler and returns the search state.
func (s *Service) SearchGetGet(w http.ResponseWriter, req *http.Request, params waf.Params) {
	ctx := req.Context()
	metrics := waf.MustGetMetrics(ctx)

	m := metrics.Duration(internal.MetricSearchState).Start()
	sh := search.GetState(params["s"])
	m.Stop()
	if sh == nil {
		s.NotFound(w, req)
		return
	}

	s.WriteJSON(w, req, sh, nil)
}

type searchCreateResponse struct {
	ID          identifier.Identifier `json:"s"`
	SearchQuery string                `json:"q,omitempty"`
}

// SearchCreatePost is a POST HTTP request handler which stores the search state
// and returns the search state ID in the response.
func (s *Service) SearchCreatePost(w http.ResponseWriter, req *http.Request, _ waf.Params) {
	ctx := req.Context()
	metrics := waf.MustGetMetrics(ctx)

	currentSearchState := req.Form.Get("s")

	searchQuery := req.Form.Get("q")
	filtersJSON := req.Form.Get("filters")

	m := metrics.Duration(internal.MetricSearchState).Start()
	sh := search.CreateState(ctx, currentSearchState, searchQuery, filtersJSON)
	m.Stop()

	s.WriteJSON(w, req, searchCreateResponse{ID: sh.ID, SearchQuery: sh.SearchQuery}, nil)
}
