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

type histogramStringResult struct {
	Str   string `json:"str"`
	Count int64  `json:"count"`
}

func (s *Service) DocumentSearchStringFilterGetGetJSON(w http.ResponseWriter, req *http.Request, params Params) {
	contentEncoding := gddo.NegotiateContentEncoding(req, allCompressions)
	if contentEncoding == "" {
		s.NotAcceptable(w, req, nil)
		return
	}

	ctx := req.Context()
	timing := servertiming.FromContext(ctx)

	id := params["s"]
	if !identifier.Valid(id) {
		s.badRequestWithError(w, req, errors.New(`"s" parameter is not a valid identifier`))
		return
	}

	prop := params["prop"]
	if !identifier.Valid(prop) {
		s.badRequestWithError(w, req, errors.New(`"prop" parameter is not a valid identifier`))
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
	aggregation := elastic.NewNestedAggregation().Path("active.string").SubAggregation(
		"filter",
		elastic.NewFilterAggregation().Filter(
			elastic.NewTermQuery("active.string.prop._id", prop),
		).SubAggregation(
			"props",
			elastic.NewTermsAggregation().Field("active.string.string").Size(maxResultsCount).OrderByAggregation("docs", false).SubAggregation(
				"docs",
				elastic.NewReverseNestedAggregation(),
			),
		).SubAggregation(
			"total",
			// Cardinality aggregation returns the count of all buckets. 40000 is the maximum precision threshold,
			// so we use it to get the most accurate approximation.
			elastic.NewCardinalityAggregation().Field("active.string.string").PrecisionThreshold(40000), //nolint:gomnd
		),
	)
	searchService := s.getSearchService(req).Size(0).Query(query).Aggregation("string", aggregation)

	m = timing.NewMetric("es").Start()
	res, err := searchService.Do(ctx)
	m.Stop()
	if err != nil {
		s.internalServerErrorWithError(w, req, errors.WithStack(err))
		return
	}
	timing.NewMetric("esi").Duration = time.Duration(res.TookInMillis) * time.Millisecond

	m = timing.NewMetric("d").Start()
	var str filteredTermAggregations
	err = json.Unmarshal(res.Aggregations["string"], &str)
	m.Stop()
	if err != nil {
		s.internalServerErrorWithError(w, req, errors.WithStack(err))
		return
	}

	results := make([]histogramStringResult, len(str.Filter.Props.Buckets))
	for i, bucket := range str.Filter.Props.Buckets {
		results[i] = histogramStringResult{Str: bucket.Key, Count: bucket.Docs.Count}
	}

	// Cardinality count is approximate, so we make sure the total is sane.
	// See: https://www.elastic.co/guide/en/elasticsearch/reference/current/search-aggregations-metrics-cardinality-aggregation.html#_counts_are_approximate
	if int64(len(str.Filter.Props.Buckets)) > str.Filter.Total.Value {
		str.Filter.Total.Value = int64(len(str.Filter.Props.Buckets))
	}
	total := strconv.FormatInt(str.Filter.Total.Value, 10) //nolint:gomnd

	metadata := http.Header{
		"Total": {total},
	}

	s.writeJSON(w, req, contentEncoding, results, metadata)
}
