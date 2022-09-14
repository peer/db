package search

import (
	"context"
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

// TODO: Limit properties only to those really used in filters ("rel", "amount", "amountRange")?

func (s *Service) populateProperties(ctx context.Context) errors.E {
	boolQuery := elastic.NewBoolQuery().Must(
		elastic.NewTermQuery("active.rel.prop._id", "2fjzZyP7rv8E4aHnBc6KAa"),
		elastic.NewTermQuery("active.rel.to._id", "HohteEmv2o7gPRnJ5wukVe"),
	)
	query := elastic.NewNestedQuery("active.rel", boolQuery)

	total, err := s.ESClient.Count(s.Index).Query(query).Do(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	s.propertiesTotal = total

	return nil
}

type propsAggregations struct {
	Props struct {
		Buckets []struct {
			Key  string `json:"key"`
			Docs struct {
				Count int64 `json:"doc_count"`
			} `json:"docs"`
		} `json:"buckets"`
	} `json:"props"`
	Total struct {
		Value int64 `json:"value"`
	} `json:"total"`
}

func (s *Service) DocumentSearchFiltersGetJSON(w http.ResponseWriter, req *http.Request, params Params) {
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
		"props",
		elastic.NewTermsAggregation().Field("active.rel.prop._id").Size(maxResultsCount).OrderByAggregation("docs", false).SubAggregation(
			"docs",
			elastic.NewReverseNestedAggregation(),
		),
	).SubAggregation(
		"total",
		// Cardinality aggregation returns the count of all buckets. It can be at most s.propertiesTotalInt,
		// so we set precision threshold to twice as much to try to always get precise counts.
		elastic.NewCardinalityAggregation().Field("active.rel.prop._id").PrecisionThreshold(2*s.propertiesTotal), //nolint:gomnd
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
	var props propsAggregations
	err = json.Unmarshal(res.Aggregations["rel"], &props)
	m.Stop()
	if err != nil {
		s.internalServerErrorWithError(w, req, errors.WithStack(err))
		return
	}

	results := make([]searchResult, len(props.Props.Buckets))
	for i, bucket := range props.Props.Buckets {
		results[i] = searchResult{ID: bucket.Key, Count: bucket.Docs.Count}
	}

	// Cardinality count is approximate, so we make sure the total is sane.
	// See: https://www.elastic.co/guide/en/elasticsearch/reference/current/search-aggregations-metrics-cardinality-aggregation.html#_counts_are_approximate
	if int64(len(props.Props.Buckets)) > props.Total.Value {
		props.Total.Value = int64(len(props.Props.Buckets))
	}
	total := strconv.FormatInt(props.Total.Value, 10) //nolint:gomnd

	metadata := http.Header{
		"Total": {total},
	}

	s.writeJSON(w, req, contentEncoding, results, metadata)
}
