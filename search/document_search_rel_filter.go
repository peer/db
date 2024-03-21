package search

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	servertiming "github.com/mitchellh/go-server-timing"
	"github.com/olivere/elastic/v7"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"
)

type filteredTermAggregations struct {
	Filter termAggregations `json:"filter"`
}

type searchRelFilterResult struct {
	ID    string `json:"_id"`
	Count int64  `json:"_count"`
}

//nolint:dupl
func DocumentSearchRelFilterGet(
	ctx context.Context, getSearchService func() (*elastic.SearchService, int64), id, prop identifier.Identifier,
) (interface{}, map[string]interface{}, errors.E) {
	timing := servertiming.FromContext(ctx)

	m := timing.NewMetric("s").Start()
	ss, ok := searches.Load(id)
	m.Stop()
	if !ok {
		// Something was not OK, so we return not found.
		return nil, nil, errors.WithStack(ErrNotFound)
	}
	sh := ss.(*State) //nolint:errcheck

	query := sh.Query()

	searchService, _ := getSearchService()
	aggregation := elastic.NewNestedAggregation().Path("claims.rel").SubAggregation(
		"filter",
		elastic.NewFilterAggregation().Filter(
			elastic.NewTermQuery("claims.rel.prop._id", prop),
		).SubAggregation(
			"props",
			elastic.NewTermsAggregation().Field("claims.rel.to._id").Size(MaxResultsCount).OrderByAggregation("docs", false).SubAggregation(
				"docs",
				elastic.NewReverseNestedAggregation(),
			),
		).SubAggregation(
			"total",
			// Cardinality aggregation returns the count of all buckets. 40000 is the maximum precision threshold,
			// so we use it to get the most accurate approximation.
			elastic.NewCardinalityAggregation().Field("claims.rel.to._id").PrecisionThreshold(40000), //nolint:gomnd
		),
	)
	searchService = searchService.Size(0).Query(query).Aggregation("rel", aggregation)

	m = timing.NewMetric("es").Start()
	res, err := searchService.Do(ctx)
	m.Stop()
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	timing.NewMetric("esi").Duration = time.Duration(res.TookInMillis) * time.Millisecond

	m = timing.NewMetric("d").Start()
	var rel filteredTermAggregations
	err = json.Unmarshal(res.Aggregations["rel"], &rel)
	m.Stop()
	if err != nil {
		return nil, nil, errors.WithStack(err)
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
