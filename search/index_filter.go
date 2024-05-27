package search

import (
	"context"
	"strconv"
	"time"

	"github.com/olivere/elastic/v7"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"
	"gitlab.com/tozd/waf"

	internal "gitlab.com/peerdb/peerdb/internal/store"
)

//nolint:tagliatelle
type indexAggregations struct {
	Buckets []struct {
		Key   string `json:"key"`
		Count int64  `json:"doc_count"`
	} `json:"buckets"`
}

func IndexFilterGet(
	ctx context.Context, getSearchService func() (*elastic.SearchService, int64), id identifier.Identifier,
) (interface{}, map[string]interface{}, errors.E) {
	metrics := waf.MustGetMetrics(ctx)

	m := metrics.Duration(internal.MetricSearchState).Start()
	ss, ok := searches.Load(id)
	m.Stop()
	if !ok {
		// Something was not OK, so we return not found.
		return nil, nil, errors.WithStack(ErrNotFound)
	}
	sh := ss.(*State) //nolint:errcheck,forcetypeassert

	query := sh.Query()

	searchService, _ := getSearchService()
	termsAggregation := elastic.NewTermsAggregation().Field("_index").Size(MaxResultsCount)
	// Cardinality aggregation returns the count of all buckets. 40000 is the maximum precision threshold,
	// so we use it to get the most accurate approximation.
	indexAggregation := elastic.NewCardinalityAggregation().Field("_index").PrecisionThreshold(40000) //nolint:gomnd
	searchService = searchService.Size(0).Query(query).Aggregation("terms", termsAggregation).Aggregation("index", indexAggregation)

	m = metrics.Duration(internal.MetricElasticSearch).Start()
	res, err := searchService.Do(ctx)
	m.Stop()
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	metrics.Duration(internal.MetricElasticSearchInternal).Duration = time.Duration(res.TookInMillis) * time.Millisecond

	m = metrics.Duration(internal.MetricJSONUnmarshal).Start()
	var terms indexAggregations
	errE := x.UnmarshalWithoutUnknownFields(res.Aggregations["terms"], &terms)
	if errE != nil {
		m.Stop()
		return nil, nil, errE
	}
	var index intValueAggregation
	errE = x.UnmarshalWithoutUnknownFields(res.Aggregations["index"], &index)
	if errE != nil {
		m.Stop()
		return nil, nil, errE
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

	return results, map[string]interface{}{
		"total": total,
	}, nil
}
