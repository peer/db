package search

import (
	"context"
	"fmt"
	"strconv"
	"time"

	esSearch "github.com/elastic/go-elasticsearch/v9/typedapi/core/search"
	"github.com/elastic/go-elasticsearch/v9/typedapi/esdsl"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types/enums/sortorder"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"
	"gitlab.com/tozd/waf"

	internalStore "gitlab.com/peerdb/peerdb/internal/store"
)

// HasFilterResult represents occurrences count for a single property in a has filter.
type HasFilterResult struct {
	ID    string `json:"id"`
	Count int64  `json:"count"`
}

// GetSubHas retrieves sub-has filter data for search results. It aggregates
// claims.subHas.prop values nested under a parent claim with the given
// parentProp, optionally restricted to listed parentTo values for
// cross-filtering with a sibling parent ref filter.
func (f *HasFilter) GetSubHas(
	ctx context.Context, getSearchService func() (*esSearch.Search, int64, int64),
	query types.QueryVariant, parentProp identifier.Identifier,
	parentToRestrictions []identifier.Identifier,
) ([]HasFilterResult, map[string]any, errors.E) {
	metrics, _ := waf.GetMetrics(ctx)

	searchService, propertiesTotal, _ := getSearchService()

	filterMusts := []types.QueryVariant{
		esdsl.NewTermQuery("claims.subHas.parentProp", esdsl.NewFieldValue().String(parentProp.String())),
	}
	if len(parentToRestrictions) > 0 {
		shoulds := make([]types.QueryVariant, 0, len(parentToRestrictions))
		for _, pto := range parentToRestrictions {
			shoulds = append(shoulds, esdsl.NewTermQuery("claims.subHas.parentTo", esdsl.NewFieldValue().String(pto.String())))
		}
		filterMusts = append(filterMusts, esdsl.NewBoolQuery().Should(shoulds...).MinimumShouldMatch(esdsl.NewMinimumShouldMatch().Int(1)))
	}

	subHasAggregation := esdsl.NewAggregations().
		Nested(esdsl.NewNestedAggregation().Path("claims.subHas")).
		AddAggregation("filter", esdsl.NewAggregations().
			Filter(esdsl.NewBoolQuery().Must(filterMusts...)).
			AddAggregation("props", esdsl.NewAggregations().
				Terms(esdsl.NewTermsAggregation().Field("claims.subHas.prop").Size(MaxResultsCount).
					Order(esdsl.NewAggregateOrder().Map(map[string]sortorder.SortOrder{"docs": sortorder.Desc}))).
				AddAggregation("docs", esdsl.NewAggregations().
					ReverseNested(esdsl.NewReverseNestedAggregation()))).
			AddAggregation("total", esdsl.NewAggregations().
				Cardinality(esdsl.NewCardinalityAggregation().Field("claims.subHas.prop").PrecisionThreshold(int(2*propertiesTotal))))) //nolint:mnd

	searchService = searchService.Size(0).Query(query).
		AddAggregation("subHas", subHasAggregation)

	m := metrics.Duration(internalStore.MetricElasticSearch).Start()
	res, err := searchService.Do(ctx)
	m.Stop()
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	metrics.Duration(internalStore.MetricElasticSearchInternal).Duration = time.Duration(res.Took) * time.Millisecond

	subHasNested, errE := aggAs[types.NestedAggregate](res.Aggregations, "subHas")
	if errE != nil {
		return nil, nil, errE
	}
	subHasFilter, errE := aggAs[types.FilterAggregate](subHasNested.Aggregations, "filter")
	if errE != nil {
		return nil, nil, errE
	}
	subHasTerms, errE := aggAs[types.StringTermsAggregate](subHasFilter.Aggregations, "props")
	if errE != nil {
		return nil, nil, errE
	}
	subHasBuckets, ok := subHasTerms.Buckets.([]types.StringTermsBucket)
	if !ok {
		errE := errors.New("unexpected bucket type for subHas")
		errors.Details(errE)["type"] = fmt.Sprintf("%T", subHasTerms.Buckets)
		return nil, nil, errE
	}
	subHasTotal, errE := aggAs[types.CardinalityAggregate](subHasFilter.Aggregations, "total")
	if errE != nil {
		return nil, nil, errE
	}

	results := make([]HasFilterResult, 0, len(subHasBuckets))
	for _, bucket := range subHasBuckets {
		bucketDocs, errE := aggAs[types.ReverseNestedAggregate](bucket.Aggregations, "docs")
		if errE != nil {
			return nil, nil, errE
		}
		key, ok := bucket.Key.(string)
		if !ok {
			errE := errors.New("unexpected key type for subHas bucket")
			errors.Details(errE)["type"] = fmt.Sprintf("%T", bucket.Key)
			return nil, nil, errE
		}
		results = append(results, HasFilterResult{ID: key, Count: bucketDocs.DocCount})
	}

	subHasTotalValue := max(int64(len(subHasBuckets)), subHasTotal.Value)
	total := strconv.FormatInt(subHasTotalValue, 10)

	return results, map[string]any{
		"total": total,
	}, nil
}

// Get retrieves has filter data for search results.
func (f *HasFilter) Get(
	ctx context.Context, getSearchService func() (*esSearch.Search, int64, int64),
	query types.QueryVariant,
) ([]HasFilterResult, map[string]any, errors.E) {
	metrics, _ := waf.GetMetrics(ctx)

	searchService, propertiesTotal, _ := getSearchService()

	// Aggregation for has claims: terms on claims.has.prop.
	// Only simple has claims (without sub-claims) are indexed in claims.has, so no
	// additional filtering is needed. Has claims with sub-claims are stored in claims.subRef.
	hasAggregation := esdsl.NewAggregations().
		Nested(esdsl.NewNestedAggregation().Path("claims.has")).
		AddAggregation("props", esdsl.NewAggregations().
			Terms(esdsl.NewTermsAggregation().Field("claims.has.prop").Size(MaxResultsCount).
				Order(esdsl.NewAggregateOrder().Map(map[string]sortorder.SortOrder{"docs": sortorder.Desc}))).
			AddAggregation("docs", esdsl.NewAggregations().
				ReverseNested(esdsl.NewReverseNestedAggregation()))).
		AddAggregation("total", esdsl.NewAggregations().
			// Cardinality aggregation returns the count of all buckets. It can be at most propertiesTotal,
			// so we set precision threshold to twice as much to try to always get precise counts.
			Cardinality(esdsl.NewCardinalityAggregation().Field("claims.has.prop").PrecisionThreshold(int(2*propertiesTotal)))) //nolint:mnd

	searchService = searchService.Size(0).Query(query).
		AddAggregation("has", hasAggregation)

	m := metrics.Duration(internalStore.MetricElasticSearch).Start()
	res, err := searchService.Do(ctx)
	m.Stop()
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	metrics.Duration(internalStore.MetricElasticSearchInternal).Duration = time.Duration(res.Took) * time.Millisecond

	hasNested, errE := aggAs[types.NestedAggregate](res.Aggregations, "has")
	if errE != nil {
		return nil, nil, errE
	}
	hasTerms, errE := aggAs[types.StringTermsAggregate](hasNested.Aggregations, "props")
	if errE != nil {
		return nil, nil, errE
	}
	hasBuckets, ok := hasTerms.Buckets.([]types.StringTermsBucket)
	if !ok {
		errE := errors.New("unexpected bucket type for has")
		errors.Details(errE)["type"] = fmt.Sprintf("%T", hasTerms.Buckets)
		return nil, nil, errE
	}
	hasTotal, errE := aggAs[types.CardinalityAggregate](hasNested.Aggregations, "total")
	if errE != nil {
		return nil, nil, errE
	}

	results := make([]HasFilterResult, 0, len(hasBuckets))
	for _, bucket := range hasBuckets {
		bucketDocs, errE := aggAs[types.ReverseNestedAggregate](bucket.Aggregations, "docs")
		if errE != nil {
			return nil, nil, errE
		}
		key, ok := bucket.Key.(string)
		if !ok {
			errE := errors.New("unexpected key type for has bucket")
			errors.Details(errE)["type"] = fmt.Sprintf("%T", bucket.Key)
			return nil, nil, errE
		}
		results = append(results, HasFilterResult{ID: key, Count: bucketDocs.DocCount})
	}

	// Cardinality count is approximate, so we make sure the total is sane.
	// See: https://www.elastic.co/guide/en/elasticsearch/reference/current/search-aggregations-metrics-cardinality-aggregation.html#_counts_are_approximate
	hasTotalValue := max(int64(len(hasBuckets)), hasTotal.Value)
	total := strconv.FormatInt(hasTotalValue, 10)

	return results, map[string]any{
		"total": total,
	}, nil
}
