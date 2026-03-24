package search

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/elastic/go-elasticsearch/v9/typedapi/core/search"
	"github.com/elastic/go-elasticsearch/v9/typedapi/esdsl"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types/enums/sortorder"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"
	"gitlab.com/tozd/waf"

	internalStore "gitlab.com/peerdb/peerdb/internal/store"
)

// RelFilterResult represents occurrences count for a single relation in a relation filter.
type RelFilterResult struct {
	ID    string `json:"id"`
	Count int64  `json:"count"`
}

// RelFilterGet retrieves relation filter data for search results.
func RelFilterGet(
	ctx context.Context, getSearchService func() (*search.Search, int64, int64), id, prop identifier.Identifier,
) ([]RelFilterResult, map[string]interface{}, errors.E) {
	metrics, _ := waf.GetMetrics(ctx)

	m := metrics.Duration(internalStore.MetricSearchSession).Start()
	searchSession, errE := GetSession(ctx, id)
	m.Stop()
	if errE != nil {
		return nil, nil, errE
	}

	query := searchSession.ToQuery()

	searchService, _, _ := getSearchService()
	aggregation := esdsl.NewAggregations().
		Nested(esdsl.NewNestedAggregation().Path("claims.rel")).
		AddAggregation("filter", esdsl.NewAggregations().
			Filter(esdsl.NewTermQuery("claims.rel.prop", esdsl.NewFieldValue().String(prop.String()))).
			AddAggregation("props", esdsl.NewAggregations().
				Terms(esdsl.NewTermsAggregation().Field("claims.rel.to").Size(MaxResultsCount).
					Order(esdsl.NewAggregateOrder().Map(map[string]sortorder.SortOrder{"docs": sortorder.Desc}))).
				AddAggregation("docs", esdsl.NewAggregations().
					ReverseNested(esdsl.NewReverseNestedAggregation()))).
			AddAggregation("total", esdsl.NewAggregations().
				// Cardinality aggregation returns the count of all buckets. 40000 is the maximum precision threshold,
				// so we use it to get the most accurate approximation. For now we didn't notice any performance issues
				// at data scale PeerDB is currently being used with, but in the future we might want to make this configurable.
				Cardinality(esdsl.NewCardinalityAggregation().Field("claims.rel.to").PrecisionThreshold(40000)))) //nolint:mnd
	searchService = searchService.Size(0).Query(query).AddAggregation("rel", aggregation)

	m = metrics.Duration(internalStore.MetricElasticSearch).Start()
	res, err := searchService.Do(ctx)
	m.Stop()
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	metrics.Duration(internalStore.MetricElasticSearchInternal).Duration = time.Duration(res.Took) * time.Millisecond

	relNested, errE := aggAs[types.NestedAggregate](res.Aggregations, "rel")
	if errE != nil {
		return nil, nil, errE
	}
	relFilter, errE := aggAs[types.FilterAggregate](relNested.Aggregations, "filter")
	if errE != nil {
		return nil, nil, errE
	}
	relTerms, errE := aggAs[types.StringTermsAggregate](relFilter.Aggregations, "props")
	if errE != nil {
		return nil, nil, errE
	}
	relBuckets, ok := relTerms.Buckets.([]types.StringTermsBucket)
	if !ok {
		errE := errors.New("unexpected bucket type for rel")
		errors.Details(errE)["type"] = fmt.Sprintf("%T", relTerms.Buckets)
		return nil, nil, errE
	}
	relTotal, errE := aggAs[types.CardinalityAggregate](relFilter.Aggregations, "total")
	if errE != nil {
		return nil, nil, errE
	}

	results := make([]RelFilterResult, 0, len(relBuckets))
	for _, bucket := range relBuckets {
		bucketDocs, errE := aggAs[types.ReverseNestedAggregate](bucket.Aggregations, "docs")
		if errE != nil {
			return nil, nil, errE
		}
		key, ok := bucket.Key.(string)
		if !ok {
			errE := errors.New("unexpected key type for rel bucket")
			errors.Details(errE)["type"] = fmt.Sprintf("%T", bucket.Key)
			return nil, nil, errE
		}
		results = append(results, RelFilterResult{ID: key, Count: bucketDocs.DocCount})
	}

	// Cardinality count is approximate, so we make sure the total is sane.
	// See: https://www.elastic.co/guide/en/elasticsearch/reference/current/search-aggregations-metrics-cardinality-aggregation.html#_counts_are_approximate
	relTotalValue := relTotal.Value
	if int64(len(relBuckets)) > relTotalValue {
		relTotalValue = int64(len(relBuckets))
	}
	total := strconv.FormatInt(relTotalValue, 10)

	return results, map[string]interface{}{
		"total": total,
	}, nil
}
