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

type filteredTermAggregations struct {
	Filter termAggregations `json:"filter"`
}

type searchRelFilterResult struct {
	ID    string `json:"id"`
	Count int64  `json:"count"`
}

//nolint:dupl
func RelFilterGet(
	ctx context.Context, getSearchService func() (*elastic.SearchService, int64), id, prop identifier.Identifier,
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
	aggregation := elastic.NewNestedAggregation().Path("claims.rel").SubAggregation(
		"filter",
		elastic.NewFilterAggregation().Filter(
			elastic.NewTermQuery("claims.rel.prop.id", prop),
		).SubAggregation(
			"props",
			elastic.NewTermsAggregation().Field("claims.rel.to.id").Size(MaxResultsCount).OrderByAggregation("docs", false).SubAggregation(
				"docs",
				elastic.NewReverseNestedAggregation(),
			),
		).SubAggregation(
			"total",
			// Cardinality aggregation returns the count of all buckets. 40000 is the maximum precision threshold,
			// so we use it to get the most accurate approximation. For now we didn't notice any performance issues
			// at data scale PeerDB is currently being used with, but in the future we might want to make this configurable.
			elastic.NewCardinalityAggregation().Field("claims.rel.to.id").PrecisionThreshold(40000), //nolint:mnd
		),
	)
	searchService = searchService.Size(0).Query(query).Aggregation("rel", aggregation)

	m = metrics.Duration(internal.MetricElasticSearch).Start()
	res, err := searchService.Do(ctx)
	m.Stop()
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	metrics.Duration(internal.MetricElasticSearchInternal).Duration = time.Duration(res.TookInMillis) * time.Millisecond

	m = metrics.Duration(internal.MetricJSONUnmarshal).Start()
	var rel filteredTermAggregations
	errE := x.Unmarshal(res.Aggregations["rel"], &rel)
	m.Stop()
	if errE != nil {
		return nil, nil, errE
	}

	results := make([]searchRelFilterResult, len(rel.Filter.Props.Buckets))
	for i, bucket := range rel.Filter.Props.Buckets {
		results[i] = searchRelFilterResult{ID: bucket.Key, Count: bucket.Docs.Count}
	}

	// Cardinality count is approximate, so we make sure the total is sane.
	// See: https://www.elastic.co/guide/en/elasticsearch/reference/current/search-aggregations-metrics-cardinality-aggregation.html#_counts_are_approximate
	if int64(len(rel.Filter.Props.Buckets)) > rel.Filter.Total.Value {
		rel.Filter.Total.Value = int64(len(rel.Filter.Props.Buckets))
	}
	total := strconv.FormatInt(rel.Filter.Total.Value, 10)

	return results, map[string]interface{}{
		"total": total,
	}, nil
}
