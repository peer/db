package search

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	servertiming "github.com/mitchellh/go-server-timing"
	"github.com/olivere/elastic/v7"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"
	"gitlab.com/tozd/waf"
)

type indexAggregations struct {
	Buckets []struct {
		Key   string `json:"key"`
		Count int64  `json:"doc_count"`
	} `json:"buckets"`
}

func (s *Service) DocumentSearchIndexFilterGet(w http.ResponseWriter, req *http.Request, params waf.Params) {
	ctx := req.Context()
	timing := servertiming.FromContext(ctx)

	id, errE := identifier.FromString(params["s"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"s" parameter is not a valid identifier`))
		return
	}

	m := timing.NewMetric("s").Start()
	ss, ok := searches.Load(id)
	m.Stop()
	if !ok {
		// Something was not OK, so we return not found.
		s.NotFound(w, req)
		return
	}
	sh := ss.(*searchState) //nolint:errcheck

	query := s.getSearchQuery(sh)
	searchService, _, errE := s.getSearchService(req)
	if errE != nil {
		s.NotFoundWithError(w, req, errE)
		return
	}
	termsAggregation := elastic.NewTermsAggregation().Field("_index").Size(maxResultsCount)
	// Cardinality aggregation returns the count of all buckets. 40000 is the maximum precision threshold,
	// so we use it to get the most accurate approximation.
	indexAggregation := elastic.NewCardinalityAggregation().Field("_index").PrecisionThreshold(40000) //nolint:gomnd
	searchService = searchService.Size(0).Query(query).Aggregation("terms", termsAggregation).Aggregation("index", indexAggregation)

	m = timing.NewMetric("es").Start()
	res, err := searchService.Do(ctx)
	m.Stop()
	if err != nil {
		s.InternalServerErrorWithError(w, req, errors.WithStack(err))
		return
	}
	timing.NewMetric("esi").Duration = time.Duration(res.TookInMillis) * time.Millisecond

	m = timing.NewMetric("d").Start()
	var terms indexAggregations
	err = json.Unmarshal(res.Aggregations["terms"], &terms)
	if err != nil {
		m.Stop()
		s.InternalServerErrorWithError(w, req, errors.WithStack(err))
		return
	}
	var index intValueAggregation
	err = json.Unmarshal(res.Aggregations["index"], &index)
	if err != nil {
		m.Stop()
		s.InternalServerErrorWithError(w, req, errors.WithStack(err))
		return
	}
	m.Stop()

	results := make([]searchStringFilterResult, len(terms.Buckets))
	for i, bucket := range terms.Buckets {
		results[i] = searchStringFilterResult{Str: bucket.Key, Count: bucket.Count}
	}

	// Cardinality count is approximate, so we make sure the total is sane.
	// See: https://www.elastic.co/guide/en/elasticsearch/reference/current/search-aggregations-metrics-cardinality-aggregation.html#_counts_are_approximate
	if int64(len(terms.Buckets)) > index.Value {
		index.Value = int64(len(terms.Buckets))
	}
	total := strconv.FormatInt(index.Value, 10)

	s.WriteJSON(w, req, results, map[string]interface{}{
		"total": total,
	})
}
