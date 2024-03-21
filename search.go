package peerdb

import (
	"context"
	"fmt"
	"net/http"
	"time"

	servertiming "github.com/mitchellh/go-server-timing"
	"github.com/olivere/elastic/v7"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"
	"gitlab.com/tozd/waf"

	"gitlab.com/peerdb/peerdb/search"
)

// TODO: Limit properties only to those really used in filters ("rel", "amount", "amountRange")?

func (s *Service) populateProperties(ctx context.Context) errors.E {
	boolQuery := elastic.NewBoolQuery().Must(
		elastic.NewTermQuery("claims.rel.prop.id", "2fjzZyP7rv8E4aHnBc6KAa"),
		elastic.NewTermQuery("claims.rel.to.id", "HohteEmv2o7gPRnJ5wukVe"),
	)
	query := elastic.NewNestedQuery("claims.rel", boolQuery)

	for _, site := range s.Sites {
		total, err := s.ESClient.Count(site.Index).Query(query).Do(ctx)
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

	// The fact that TrackTotalHits is set to true is important because the count is used as the
	// number of documents of the filter on the _index field.
	return s.ESClient.Search(site.Index).FetchSource(false).Preference(getHost(req.RemoteAddr)).
		Header("X-Opaque-ID", waf.MustRequestID(ctx).String()).TrackTotalHits(true).AllowPartialSearchResults(false), site.propertiesTotal
}

func (s *Service) getSearchServiceClosure(req *http.Request) func() (*elastic.SearchService, int64) {
	return func() (*elastic.SearchService, int64) {
		return s.getSearchService(req)
	}
}

func (s *Service) DocumentSearchAmountFilterGet(w http.ResponseWriter, req *http.Request, params waf.Params) {
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

	data, metadata, errE := search.DocumentSearchAmountFilterGet(req.Context(), s.getSearchServiceClosure(req), id, prop, params["unit"])
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

func (s *Service) DocumentSearchFiltersGet(w http.ResponseWriter, req *http.Request, params waf.Params) {
	id, errE := identifier.FromString(params["s"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"s" is not a valid identifier`))
		return
	}

	data, metadata, errE := search.DocumentSearchFiltersGet(req.Context(), s.getSearchServiceClosure(req), id)
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

func (s *Service) DocumentSearchIndexFilterGet(w http.ResponseWriter, req *http.Request, params waf.Params) {
	id, errE := identifier.FromString(params["s"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"s" is not a valid identifier`))
		return
	}

	data, metadata, errE := search.DocumentSearchIndexFilterGet(req.Context(), s.getSearchServiceClosure(req), id)
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
func (s *Service) DocumentSearchRelFilterGet(w http.ResponseWriter, req *http.Request, params waf.Params) {
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

	data, metadata, errE := search.DocumentSearchRelFilterGet(req.Context(), s.getSearchServiceClosure(req), id, prop)
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

func (s *Service) DocumentSearchSizeFilterGet(w http.ResponseWriter, req *http.Request, params waf.Params) {
	id, errE := identifier.FromString(params["s"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"s" is not a valid identifier`))
		return
	}

	data, metadata, errE := search.DocumentSearchSizeFilterGet(req.Context(), s.getSearchServiceClosure(req), id)
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
func (s *Service) DocumentSearchStringFilterGet(w http.ResponseWriter, req *http.Request, params waf.Params) {
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

	data, metadata, errE := search.DocumentSearchStringFilterGet(req.Context(), s.getSearchServiceClosure(req), id, prop)
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
func (s *Service) DocumentSearchTimeFilterGet(w http.ResponseWriter, req *http.Request, params waf.Params) {
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

	data, metadata, errE := search.DocumentSearchTimeFilterGet(req.Context(), s.getSearchServiceClosure(req), id, prop)
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

// DocumentSearch is a GET/HEAD HTTP request handler which returns HTML frontend for searching documents.
// If search state is invalid, it redirects to a valid one.
func (s *Service) DocumentSearch(w http.ResponseWriter, req *http.Request, _ waf.Params) {
	ctx := req.Context()
	timing := servertiming.FromContext(ctx)

	var q *string
	if req.Form.Has("q") {
		qq := req.Form.Get("q")
		q = &qq
	}

	var filters *string
	if req.Form.Has("filters") {
		f := req.Form.Get("filters")
		filters = &f
	}

	m := timing.NewMetric("s").Start()
	sh, ok := search.GetOrCreateState(req.Form.Get("s"), q, filters)
	m.Stop()
	if !ok {
		// Something was not OK, so we redirect to the correct URL.
		path, err := s.Reverse("DocumentSearch", nil, sh.Values())
		if err != nil {
			s.InternalServerErrorWithError(w, req, err)
			return
		}
		// TODO: Should we already do the query, to warm up ES cache?
		//       Maybe we should cache response ourselves so that we do not hit ES twice?
		w.Header().Set("Location", path)
		w.WriteHeader(http.StatusSeeOther)
		return
	} else if !req.Form.Has("q") {
		// "q" is missing, so we redirect to the correct URL.
		path, err := s.Reverse("DocumentSearch", nil, sh.ValuesWithAt(req.Form.Get("at")))
		if err != nil {
			s.InternalServerErrorWithError(w, req, err)
			return
		}
		// TODO: Should we already do the query, to warm up ES cache?
		//       Maybe we should cache response ourselves so that we do not hit ES twice?
		w.Header().Set("Location", path)
		w.WriteHeader(http.StatusSeeOther)
		return
	}

	s.Home(w, req, nil)
}

type searchResult struct {
	ID string `json:"id"`
}

// DocumentSearchGet is a GET/HEAD HTTP request handler and it searches ElasticSearch index using provided
// search state and returns to the client a JSON with an array of IDs of found documents. If search state is
// invalid, it returns correct query parameters as JSON. It supports compression based on accepted content
// encoding and range requests. It returns search metadata (e.g., total results) as PeerDB HTTP response headers.
func (s *Service) DocumentSearchGet(w http.ResponseWriter, req *http.Request, _ waf.Params) {
	ctx := req.Context()
	timing := servertiming.FromContext(ctx)

	var q *string
	if req.Form.Has("q") {
		qq := req.Form.Get("q")
		q = &qq
	}

	var filters *string
	if req.Form.Has("filters") {
		f := req.Form.Get("filters")
		filters = &f
	}

	m := timing.NewMetric("s").Start()
	sh, ok := search.GetOrCreateState(req.Form.Get("s"), q, filters)
	m.Stop()
	if !ok {
		// Something was not OK, so we return new query parameters.
		// TODO: Should we already do the query, to warm up ES cache?
		//       Maybe we should cache response ourselves so that we do not hit ES twice?
		s.WriteJSON(w, req, sh, nil)
		return
	}

	searchService, _ := s.getSearchService(req)
	searchService = searchService.From(0).Size(search.MaxResultsCount).Query(sh.Query())

	m = timing.NewMetric("es").Start()
	res, err := searchService.Do(ctx)
	m.Stop()
	if err != nil {
		s.InternalServerErrorWithError(w, req, errors.WithStack(err))
		return
	}
	timing.NewMetric("esi").Duration = time.Duration(res.TookInMillis) * time.Millisecond

	results := make([]searchResult, len(res.Hits.Hits))
	for i, hit := range res.Hits.Hits {
		results[i] = searchResult{ID: hit.Id}
	}

	// Total is a string or a number.
	var total interface{}
	if res.Hits.TotalHits.Relation == "gte" {
		total = fmt.Sprintf("+%d", res.Hits.TotalHits.Value)
	} else {
		total = res.Hits.TotalHits.Value
	}

	// TODO: Move this to a separate API endpoint.
	filtersJSON, err := x.MarshalWithoutEscapeHTML(sh.Filters)
	if err != nil {
		s.InternalServerErrorWithError(w, req, errors.WithStack(err))
		return
	}

	s.WriteJSON(w, req, results, map[string]interface{}{
		"total":   total,
		"query":   sh.Text,
		"filters": string(filtersJSON),
	})
}

// DocumentSearchPost is a POST HTTP request handler which stores the search state and returns
// query parameters for the GET endpoint as JSON or redirects to the GET endpoint based on search ID.
func (s *Service) DocumentSearchPost(w http.ResponseWriter, req *http.Request, _ waf.Params) {
	ctx := req.Context()
	timing := servertiming.FromContext(ctx)

	var q *string
	if req.Form.Has("q") {
		qq := req.Form.Get("q")
		q = &qq
	}

	var filters *string
	if req.Form.Has("filters") {
		f := req.Form.Get("filters")
		filters = &f
	}

	m := timing.NewMetric("s").Start()
	sh := search.CreateState(req.Form.Get("s"), q, filters)
	m.Stop()

	// TODO: Should we already do the query, to warm up ES cache?
	//       Maybe we should cache response ourselves so that we do not hit ES twice?
	s.WriteJSON(w, req, sh, nil)
}
