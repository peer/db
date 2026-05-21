package search

import (
	"cmp"
	"context"
	"fmt"
	"slices"
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

// MissingRefFilterID is a special ID used for the "missing" bucket in reference filter results.
// It represents documents that do not have a value for the filtered property.
const MissingRefFilterID = "__MISSING__"

// RefFilterResult represents occurrences count for a single reference in a reference filter.
type RefFilterResult struct {
	ID    string `json:"id"`
	Count int64  `json:"count"`
}

// Get retrieves reference filter data for search results.
func (f *RefFilter) Get(
	ctx context.Context, getSearchService func() (*esSearch.Search, int64, int64),
	query types.QueryVariant, prop identifier.Identifier,
) ([]RefFilterResult, map[string]any, errors.E) {
	metrics, _ := waf.GetMetrics(ctx)

	searchService, _, _ := getSearchService()

	// Aggregation for documents that have the property: terms on claims.ref.to.
	refAggregation := esdsl.NewAggregations().
		Nested(esdsl.NewNestedAggregation().Path("claims.ref")).
		AddAggregation("filter", esdsl.NewAggregations().
			Filter(esdsl.NewTermQuery("claims.ref.prop", esdsl.NewFieldValue().String(prop.String()))).
			AddAggregation("props", esdsl.NewAggregations().
				Terms(esdsl.NewTermsAggregation().Field("claims.ref.to").Size(MaxResultsCount).
					Order(esdsl.NewAggregateOrder().Map(map[string]sortorder.SortOrder{"docs": sortorder.Desc}))).
				AddAggregation("docs", esdsl.NewAggregations().
					ReverseNested(esdsl.NewReverseNestedAggregation()))).
			AddAggregation("total", esdsl.NewAggregations().
				// Cardinality aggregation returns the count of all buckets. 40000 is the maximum precision threshold,
				// so we use it to get the most accurate approximation. For now we didn't notice any performance issues
				// at data scale PeerDB is currently being used with, but in the future we might want to make this configurable.
				Cardinality(esdsl.NewCardinalityAggregation().Field("claims.ref.to").PrecisionThreshold(40000)))) //nolint:mnd

	// Aggregation for documents missing the property: count documents where the prop does not exist.
	missingAggregation := esdsl.NewAggregations().
		Filter(esdsl.NewBoolQuery().MustNot(
			esdsl.NewNestedQuery(
				esdsl.NewTermQuery("claims.ref.prop", esdsl.NewFieldValue().String(prop.String())),
			).Path("claims.ref"),
		))

	searchService = searchService.Size(0).Query(query).
		AddAggregation("ref", refAggregation).
		AddAggregation("missing", missingAggregation)

	m := metrics.Duration(internalStore.MetricElasticSearch).Start()
	res, err := searchService.Do(ctx)
	m.Stop()
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	metrics.Duration(internalStore.MetricElasticSearchInternal).Duration = time.Duration(res.Took) * time.Millisecond

	refNested, errE := aggAs[types.NestedAggregate](res.Aggregations, "ref")
	if errE != nil {
		return nil, nil, errE
	}
	refFilter, errE := aggAs[types.FilterAggregate](refNested.Aggregations, "filter")
	if errE != nil {
		return nil, nil, errE
	}
	refTerms, errE := aggAs[types.StringTermsAggregate](refFilter.Aggregations, "props")
	if errE != nil {
		return nil, nil, errE
	}
	refBuckets, ok := refTerms.Buckets.([]types.StringTermsBucket)
	if !ok {
		errE := errors.New("unexpected bucket type for ref")
		errors.Details(errE)["type"] = fmt.Sprintf("%T", refTerms.Buckets)
		return nil, nil, errE
	}
	refTotal, errE := aggAs[types.CardinalityAggregate](refFilter.Aggregations, "total")
	if errE != nil {
		return nil, nil, errE
	}

	// Parse the missing count.
	missingFilter, errE := aggAs[types.FilterAggregate](res.Aggregations, "missing")
	if errE != nil {
		return nil, nil, errE
	}
	missingCount := missingFilter.DocCount

	results := make([]RefFilterResult, 0, len(refBuckets)+1)
	for _, bucket := range refBuckets {
		bucketDocs, errE := aggAs[types.ReverseNestedAggregate](bucket.Aggregations, "docs")
		if errE != nil {
			return nil, nil, errE
		}
		key, ok := bucket.Key.(string)
		if !ok {
			errE := errors.New("unexpected key type for ref bucket")
			errors.Details(errE)["type"] = fmt.Sprintf("%T", bucket.Key)
			return nil, nil, errE
		}
		results = append(results, RefFilterResult{ID: key, Count: bucketDocs.DocCount})
	}

	// Include the missing bucket if there are documents without this property.
	if missingCount > 0 {
		results = append(results, RefFilterResult{ID: MissingRefFilterID, Count: missingCount})
		// Re-sort by count descending so that missing is in the right position.
		slices.SortStableFunc(results, func(a, b RefFilterResult) int {
			return cmp.Compare(b.Count, a.Count)
		})
	}

	// Cardinality count is approximate, so we make sure the total is sane.
	// See: https://www.elastic.co/guide/en/elasticsearch/reference/current/search-aggregations-metrics-cardinality-aggregation.html#_counts_are_approximate
	refTotalValue := max(int64(len(refBuckets)), refTotal.Value)
	// Include missing in the total if present.
	if missingCount > 0 {
		refTotalValue++
	}
	total := strconv.FormatInt(refTotalValue, 10)

	return results, map[string]any{
		"total": total,
	}, nil
}

// ToSubRefQuery converts the RefFilter to an ElasticSearch query on claims.sub
// for a sub-reference filter with parentProp and prop.
//
// parentToRestrictions, when non-empty, restricts the sub-claim match to entries
// whose claims.sub.parentTo is one of the listed values. This enables cross-
// filter joins: when a session has both a parent-level ref filter (e.g.
// HAS_LOCATION = L1) and a sub-ref filter (e.g. HAS_LOCATION > HAS_ARTIST = A),
// the sub-claim is required to live under one of the same parent values, so the
// result is "A under L1" rather than the looser "A anywhere AND L1 anywhere".
func (f *RefFilter) ToSubRefQuery(parentProp, prop identifier.Identifier, parentToRestrictions []identifier.Identifier) types.QueryVariant { //nolint:ireturn
	// withParentTo appends the parentTo restriction clause (if any) to a slice
	// of must-clauses building a single nested sub-claim match. The clause is
	// "claims.sub.parentTo is one of the restriction values", joined with the
	// existing parentProp/prop (and optional to) constraints inside the same
	// nested query so the join happens within a single sub-claim record.
	withParentTo := func(must []types.QueryVariant) []types.QueryVariant {
		if len(parentToRestrictions) == 0 {
			return must
		}
		shoulds := make([]types.QueryVariant, 0, len(parentToRestrictions))
		for _, pto := range parentToRestrictions {
			shoulds = append(shoulds, esdsl.NewTermQuery("claims.sub.parentTo", esdsl.NewFieldValue().String(pto.String())))
		}
		return append(must, esdsl.NewBoolQuery().Should(shoulds...).MinimumShouldMatch(esdsl.NewMinimumShouldMatch().Int(1)))
	}

	missingMust := withParentTo([]types.QueryVariant{
		esdsl.NewTermQuery("claims.sub.parentProp", esdsl.NewFieldValue().String(parentProp.String())),
		esdsl.NewTermQuery("claims.sub.prop", esdsl.NewFieldValue().String(prop.String())),
	})
	missingQuery := esdsl.NewBoolQuery().MustNot(
		esdsl.NewNestedQuery(
			esdsl.NewBoolQuery().Must(missingMust...),
		).Path("claims.sub"),
	)

	// Missing only.
	if f.Missing && len(f.To) == 0 {
		return missingQuery
	}

	// Build value queries (OR across all To values).
	shoulds := make([]types.QueryVariant, 0, len(f.To)+1)
	for _, to := range f.To {
		valueMust := withParentTo([]types.QueryVariant{
			esdsl.NewTermQuery("claims.sub.parentProp", esdsl.NewFieldValue().String(parentProp.String())),
			esdsl.NewTermQuery("claims.sub.prop", esdsl.NewFieldValue().String(prop.String())),
			esdsl.NewTermQuery("claims.sub.to", esdsl.NewFieldValue().String(to.ID.String())),
		})
		shoulds = append(shoulds, esdsl.NewNestedQuery(
			esdsl.NewBoolQuery().Must(valueMust...),
		).Path("claims.sub"))
	}

	// Values + missing: OR them together.
	if f.Missing {
		shoulds = append(shoulds, missingQuery)
	}

	if len(shoulds) == 1 {
		return shoulds[0]
	}
	return esdsl.NewBoolQuery().Should(shoulds...).MinimumShouldMatch(esdsl.NewMinimumShouldMatch().Int(1))
}

// GetSubRef retrieves sub-reference filter data for search results.
// It aggregates claims.sub.to values for a given (parentProp, prop) combination.
// parentToRestrictions optionally restricts results to specific parentTo values (for cross-filtering).
func (f *RefFilter) GetSubRef(
	ctx context.Context, getSearchService func() (*esSearch.Search, int64, int64),
	query types.QueryVariant, parentProp, prop identifier.Identifier,
	parentToRestrictions []identifier.Identifier,
) ([]RefFilterResult, map[string]any, errors.E) {
	metrics, _ := waf.GetMetrics(ctx)

	searchService, _, _ := getSearchService()

	// Build the filter for parentProp + prop (+ optional parentTo restriction).
	filterMusts := []types.QueryVariant{
		esdsl.NewTermQuery("claims.sub.parentProp", esdsl.NewFieldValue().String(parentProp.String())),
		esdsl.NewTermQuery("claims.sub.prop", esdsl.NewFieldValue().String(prop.String())),
	}
	if len(parentToRestrictions) > 0 {
		parentToShoulds := make([]types.QueryVariant, 0, len(parentToRestrictions))
		for _, pto := range parentToRestrictions {
			parentToShoulds = append(parentToShoulds, esdsl.NewTermQuery("claims.sub.parentTo", esdsl.NewFieldValue().String(pto.String())))
		}
		filterMusts = append(filterMusts, esdsl.NewBoolQuery().Should(parentToShoulds...).MinimumShouldMatch(esdsl.NewMinimumShouldMatch().Int(1)))
	}

	// Aggregation for documents that have matching subRef: terms on claims.sub.to.
	subRefAggregation := esdsl.NewAggregations().
		Nested(esdsl.NewNestedAggregation().Path("claims.sub")).
		AddAggregation("filter", esdsl.NewAggregations().
			Filter(esdsl.NewBoolQuery().Must(filterMusts...)).
			AddAggregation("props", esdsl.NewAggregations().
				Terms(esdsl.NewTermsAggregation().Field("claims.sub.to").Size(MaxResultsCount).
					Order(esdsl.NewAggregateOrder().Map(map[string]sortorder.SortOrder{"docs": sortorder.Desc}))).
				AddAggregation("docs", esdsl.NewAggregations().
					ReverseNested(esdsl.NewReverseNestedAggregation()))).
			AddAggregation("total", esdsl.NewAggregations().
				Cardinality(esdsl.NewCardinalityAggregation().Field("claims.sub.to").PrecisionThreshold(40000)))) //nolint:mnd

	// Aggregation for documents missing this sub-reference.
	missingAggregation := esdsl.NewAggregations().
		Filter(esdsl.NewBoolQuery().MustNot(
			esdsl.NewNestedQuery(
				esdsl.NewBoolQuery().Must(
					esdsl.NewTermQuery("claims.sub.parentProp", esdsl.NewFieldValue().String(parentProp.String())),
					esdsl.NewTermQuery("claims.sub.prop", esdsl.NewFieldValue().String(prop.String())),
				),
			).Path("claims.sub"),
		))

	searchService = searchService.Size(0).Query(query).
		AddAggregation("subRef", subRefAggregation).
		AddAggregation("missing", missingAggregation)

	m := metrics.Duration(internalStore.MetricElasticSearch).Start()
	res, err := searchService.Do(ctx)
	m.Stop()
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	metrics.Duration(internalStore.MetricElasticSearchInternal).Duration = time.Duration(res.Took) * time.Millisecond

	subRefNested, errE := aggAs[types.NestedAggregate](res.Aggregations, "subRef")
	if errE != nil {
		return nil, nil, errE
	}
	subRefFilter, errE := aggAs[types.FilterAggregate](subRefNested.Aggregations, "filter")
	if errE != nil {
		return nil, nil, errE
	}
	subRefTerms, errE := aggAs[types.StringTermsAggregate](subRefFilter.Aggregations, "props")
	if errE != nil {
		return nil, nil, errE
	}
	subRefBuckets, ok := subRefTerms.Buckets.([]types.StringTermsBucket)
	if !ok {
		errE := errors.New("unexpected bucket type for subRef")
		errors.Details(errE)["type"] = fmt.Sprintf("%T", subRefTerms.Buckets)
		return nil, nil, errE
	}
	subRefTotal, errE := aggAs[types.CardinalityAggregate](subRefFilter.Aggregations, "total")
	if errE != nil {
		return nil, nil, errE
	}

	// Parse the missing count.
	missingFilter, errE := aggAs[types.FilterAggregate](res.Aggregations, "missing")
	if errE != nil {
		return nil, nil, errE
	}
	missingCount := missingFilter.DocCount

	results := make([]RefFilterResult, 0, len(subRefBuckets)+1)
	for _, bucket := range subRefBuckets {
		bucketDocs, errE := aggAs[types.ReverseNestedAggregate](bucket.Aggregations, "docs")
		if errE != nil {
			return nil, nil, errE
		}
		key, ok := bucket.Key.(string)
		if !ok {
			errE := errors.New("unexpected key type for subRef bucket")
			errors.Details(errE)["type"] = fmt.Sprintf("%T", bucket.Key)
			return nil, nil, errE
		}
		results = append(results, RefFilterResult{ID: key, Count: bucketDocs.DocCount})
	}

	// Include the missing bucket if there are documents without this sub-reference.
	if missingCount > 0 {
		results = append(results, RefFilterResult{ID: MissingRefFilterID, Count: missingCount})
		slices.SortStableFunc(results, func(a, b RefFilterResult) int {
			return cmp.Compare(b.Count, a.Count)
		})
	}

	subRefTotalValue := max(int64(len(subRefBuckets)), subRefTotal.Value)
	if missingCount > 0 {
		subRefTotalValue++
	}
	total := strconv.FormatInt(subRefTotalValue, 10)

	return results, map[string]any{
		"total": total,
	}, nil
}
