package search

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	gddo "github.com/golang/gddo/httputil"
	servertiming "github.com/mitchellh/go-server-timing"
	"github.com/olivere/elastic/v7"
	"gitlab.com/tozd/go/errors"

	"gitlab.com/peerdb/search/identifier"
)

type filteredPropsAggregations struct {
	Filter propsAggregations `json:"filter"`
}

func (s *Service) DocumentSearchFilterGetGetJSON(w http.ResponseWriter, req *http.Request, params Params) {
	contentEncoding := gddo.NegotiateContentEncoding(req, allCompressions)
	if contentEncoding == "" {
		s.NotAcceptable(w, req, nil)
		return
	}

	ctx := req.Context()
	timing := servertiming.FromContext(ctx)

	id := params["s"]
	if !identifier.Valid(id) {
		s.BadRequest(w, req, nil)
		return
	}

	prop := params["prop"]
	if !identifier.Valid(prop) {
		s.BadRequest(w, req, nil)
		return
	}

	m := timing.NewMetric("s").Start()
	ss, ok := searches.Load(id)
	m.Stop()
	if !ok {
		// Something was not OK, so we return not found.
		s.NotFound(w, req, nil)
		return
	}
	sh := ss.(*search) //nolint:errcheck

	query := s.getSearchQuery(sh)
	aggregation := elastic.NewNestedAggregation().Path("active.rel").SubAggregation(
		"filter",
		elastic.NewFilterAggregation().Filter(elastic.NewTermQuery("active.rel.prop._id", prop)).SubAggregation(
			"props",
			elastic.NewTermsAggregation().Field("active.rel.to._id").Size(maxResultsCount).OrderByAggregation("docs", false).SubAggregation(
				"docs",
				elastic.NewReverseNestedAggregation(),
			),
		).SubAggregation(
			"total",
			// Cardinality aggregation returns the count of all buckets. It can be at most s.propertiesTotalInt,
			// so we set precision threshold to twice as much to try to always get precise counts.
			elastic.NewCardinalityAggregation().Field("active.rel.to._id").PrecisionThreshold(2*int64(s.propertiesTotalInt)),
		),
	)
	searchService := s.getSearchService(req).Size(0).Query(query).Aggregation("rel", aggregation)

	m = timing.NewMetric("es").Start()
	res, err := searchService.Do(ctx)
	m.Stop()
	if err != nil {
		s.internalServerErrorWithError(w, req, errors.WithStack(err))
		return
	}
	timing.NewMetric("esi").Duration = time.Duration(res.TookInMillis) * time.Millisecond

	m = timing.NewMetric("d").Start()
	var props filteredPropsAggregations
	err = json.Unmarshal(res.Aggregations["rel"], &props)
	m.Stop()
	if err != nil {
		s.internalServerErrorWithError(w, req, errors.WithStack(err))
		return
	}

	results := make([]searchResult, len(props.Filter.Props.Buckets))
	for i, bucket := range props.Filter.Props.Buckets {
		results[i] = searchResult{ID: bucket.Key, Count: bucket.Docs.Count}
	}

	// Cardinality count is approximate, so we make sure the total is sane.
	// See: https://www.elastic.co/guide/en/elasticsearch/reference/current/search-aggregations-metrics-cardinality-aggregation.html#_counts_are_approximate
	if int64(len(props.Filter.Props.Buckets)) > props.Filter.Total.Value {
		props.Filter.Total.Value = int64(len(props.Filter.Props.Buckets))
	}
	total := strconv.FormatInt(props.Filter.Total.Value, 10) //nolint:gomnd

	metadata := http.Header{
		"Total": {total},
	}

	s.writeJSON(w, req, contentEncoding, results, metadata)
}
