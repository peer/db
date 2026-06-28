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

	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
	internalStore "gitlab.com/peerdb/peerdb/internal/store"
)

// HasFilterResult represents occurrences count for a single property in a has filter.
type HasFilterResult struct {
	ID    string `json:"id"`
	Count int64  `json:"count"`
}

// mergeSelectedHasProps appends, at count 0, any selected has-property not already present in the value list,
// so an active has filter always shows its selection (otherwise dropped when it matches no document under the
// rest of the search) and each property stays individually deselectable. The has facet is flat, so no
// hierarchy is involved.
func mergeSelectedHasProps(results []HasFilterResult, props []HasValue) []HasFilterResult {
	present := make(map[string]bool, len(results))
	for _, r := range results {
		present[r.ID] = true
	}
	for _, p := range props {
		id := p.ID.String()
		if present[id] {
			continue
		}
		results = append(results, HasFilterResult{ID: id, Count: 0})
		present[id] = true
	}
	return results
}

// hasPropsTermsQuery matches has (or sub-has) records whose prop field is one of the selected property ids.
// field is the nested path ("claims.has" or "claims.subHas").
func hasPropsTermsQuery(field string, props []HasValue) types.QueryVariant { //nolint:ireturn
	values := make([]types.FieldValueVariant, len(props))
	for i, p := range props {
		values[i] = esdsl.NewFieldValue().String(p.ID.String())
	}
	return esdsl.NewTermsQuery().AddTermsQuery(field+".prop", esdsl.NewTermsQueryField().FieldValues(values...))
}

// buildHasValueSearchResults assembles a has (or sub-has) value-search response: the matched property ids
// (matchingValueIDs, the union of the in-scope value aggregation's bucket keys under name and the augment ids the
// selectedMatch aggregation matched) wrapped as id-only HasFilterResult entries. The has facet is flat and has no
// missing bucket, so there are no ancestors, counts, or missing entry; the frontend applies these ids as a visual
// overlay on top of its unfiltered primary. The metadata total is the number of returned ids. hasSelectedMatch
// reports whether the selectedMatch aggregation was added.
func buildHasValueSearchResults(aggs map[string]types.Aggregate, name string, hasSelectedMatch bool) ([]HasFilterResult, map[string]any, errors.E) {
	ids, errE := matchingValueIDs(aggs, name, hasSelectedMatch)
	if errE != nil {
		return nil, nil, errE
	}
	results := make([]HasFilterResult, 0, len(ids))
	for _, id := range ids {
		results = append(results, HasFilterResult{ID: id, Count: 0})
	}
	total := strconv.FormatInt(int64(len(results)), 10)
	return results, map[string]any{
		"total": total,
	}, nil
}

// hasPropAggregation builds the property-count aggregation shared by the has and sub-has filters. field is the
// nested path ("claims.has" or "claims.subHas") and filterQuery scopes the records counted. It produces one
// "props" bucket per property with its document count ("docs", reverse_nested to the document level) and a
// cardinality "total" of distinct properties. The has facet is flat, so there is no hierarchy sub-aggregation.
func hasPropAggregation(field string, filterQuery types.QueryVariant) types.AggregationsVariant { //nolint:ireturn
	return esdsl.NewAggregations().
		Nested(esdsl.NewNestedAggregation().Path(field)).
		AddAggregation("filter", esdsl.NewAggregations().
			Filter(filterQuery).
			AddAggregation("props", esdsl.NewAggregations().
				Terms(esdsl.NewTermsAggregation().Field(field+".prop").Size(MaxResultsCount).
					Order(esdsl.NewAggregateOrder().Map(map[string]sortorder.SortOrder{"docs": sortorder.Desc}))).
				AddAggregation("docs", esdsl.NewAggregations().
					ReverseNested(esdsl.NewReverseNestedAggregation()))).
			AddAggregation("total", esdsl.NewAggregations().
				Cardinality(esdsl.NewCardinalityAggregation().Field(field+".prop").PrecisionThreshold(maxPrecisionThreshold))))
}

// hasBucketsToResults turns the property buckets of a has (or sub-has) aggregation into HasFilterResult entries,
// reading each bucket's "docs" reverse_nested count. kind labels error messages (has or subHas).
func hasBucketsToResults(buckets []types.StringTermsBucket, kind string) ([]HasFilterResult, errors.E) {
	results := make([]HasFilterResult, 0, len(buckets))
	for _, bucket := range buckets {
		bucketDocs, errE := internalSearch.AggAs[types.ReverseNestedAggregate](bucket.Aggregations, "docs")
		if errE != nil {
			return nil, errE
		}
		key, ok := bucket.Key.(string)
		if !ok {
			errE := errors.New("unexpected key type for " + kind + " bucket")
			errors.Details(errE)["type"] = fmt.Sprintf("%T", bucket.Key)
			return nil, errE
		}
		results = append(results, HasFilterResult{ID: key, Count: bucketDocs.DocCount})
	}
	return results, nil
}

// getMatchingSubHasPropIDs runs the sub-has filter's value-search path: it returns only the property ids whose
// sub-property or parent-property name matched valueQuery, as id-only results (see buildHasValueSearchResults).
// The match set is the union of the in-scope value aggregation's bucket keys and the selected property ids the
// selectedMatch aggregation matched. It is split out of GetSubHas so each path stays small.
func (f *HasFilter) getMatchingSubHasPropIDs(
	ctx context.Context, getSearchService func() *esSearch.Search,
	query types.QueryVariant, parentProp identifier.Identifier,
	parentToRestrictions []identifier.Identifier,
	valueQuery string, enabledLanguages []string,
) ([]HasFilterResult, map[string]any, errors.E) {
	metrics, _ := waf.GetMetrics(ctx)

	searchService := getSearchService()

	parentPropTerm := esdsl.NewTermQuery("claims.subHas.parentProp", esdsl.NewFieldValue().String(parentProp.String()))
	filterMusts := []types.QueryVariant{parentPropTerm}
	if len(parentToRestrictions) > 0 {
		shoulds := make([]types.QueryVariant, 0, len(parentToRestrictions))
		for _, pto := range parentToRestrictions {
			shoulds = append(shoulds, esdsl.NewTermQuery("claims.subHas.parentTo", esdsl.NewFieldValue().String(pto.String())))
		}
		filterMusts = append(filterMusts, esdsl.NewBoolQuery().Should(shoulds...).MinimumShouldMatch(esdsl.NewMinimumShouldMatch().Int(1)))
	}
	propLabelMatch := propLabelMatchQuery(
		[]string{"claims.subHas.propNaming", "claims.subHas.parentPropNaming"},
		[]string{"claims.subHas.propDisplay", "claims.subHas.parentPropDisplay"}, valueQuery, enabledLanguages)
	filterMusts = append(filterMusts, propLabelMatch)

	searchService = searchService.Size(0).Query(query).
		AddAggregation("subHas", hasPropAggregation("claims.subHas", esdsl.NewBoolQuery().Must(filterMusts...)))

	// The selectedMatch label-matches the selected sub-has properties globally so an active filter's selection
	// (which has zero documents in the search scope) stays searchable by its own label. It is scoped to the parent
	// property and the selected prop ids, mirroring the facet's own filter.
	if len(f.Props) > 0 {
		selectedMatchFilter := esdsl.NewBoolQuery().Must(parentPropTerm, propLabelMatch, hasPropsTermsQuery("claims.subHas", f.Props))
		searchService = searchService.AddAggregation("selectedMatch", selectedMatchAggregation("claims.subHas", "prop", selectedMatchFilter))
	}

	m := metrics.Duration(internalStore.MetricElasticSearch).Start()
	res, err := searchService.Do(ctx)
	m.Stop()
	if err != nil {
		return nil, nil, WithESError(err)
	}
	metrics.Duration(internalStore.MetricElasticSearchInternal).Duration = time.Duration(res.Took) * time.Millisecond

	return buildHasValueSearchResults(res.Aggregations, "subHas", len(f.Props) > 0)
}

// GetSubHas retrieves sub-has filter data for search results. It aggregates
// claims.subHas.prop values nested under a parent claim with the given
// parentProp, optionally restricted to listed parentTo values for
// cross-filtering with a sibling parent ref filter.
func (f *HasFilter) GetSubHas(
	ctx context.Context, getSearchService func() *esSearch.Search,
	query types.QueryVariant, parentProp identifier.Identifier,
	parentToRestrictions []identifier.Identifier,
	valueQuery string, enabledLanguages []string,
) ([]HasFilterResult, map[string]any, errors.E) {
	// During a filter-pane value search the response carries only the matching property ids (an overlay the
	// frontend applies on top of its unfiltered primary), so it takes a dedicated path.
	if valueQuery != "" {
		return f.getMatchingSubHasPropIDs(ctx, getSearchService, query, parentProp, parentToRestrictions, valueQuery, enabledLanguages)
	}

	metrics, _ := waf.GetMetrics(ctx)

	searchService := getSearchService()

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

	searchService = searchService.Size(0).Query(query).
		AddAggregation("subHas", hasPropAggregation("claims.subHas", esdsl.NewBoolQuery().Must(filterMusts...)))

	m := metrics.Duration(internalStore.MetricElasticSearch).Start()
	res, err := searchService.Do(ctx)
	m.Stop()
	if err != nil {
		return nil, nil, WithESError(err)
	}
	metrics.Duration(internalStore.MetricElasticSearchInternal).Duration = time.Duration(res.Took) * time.Millisecond

	subHasNested, errE := internalSearch.AggAs[types.NestedAggregate](res.Aggregations, "subHas")
	if errE != nil {
		return nil, nil, errE
	}
	subHasFilter, errE := internalSearch.AggAs[types.FilterAggregate](subHasNested.Aggregations, "filter")
	if errE != nil {
		return nil, nil, errE
	}
	subHasTerms, errE := internalSearch.AggAs[types.StringTermsAggregate](subHasFilter.Aggregations, "props")
	if errE != nil {
		return nil, nil, errE
	}
	subHasBuckets, ok := subHasTerms.Buckets.([]types.StringTermsBucket)
	if !ok {
		errE := errors.New("unexpected bucket type for subHas")
		errors.Details(errE)["type"] = fmt.Sprintf("%T", subHasTerms.Buckets)
		return nil, nil, errE
	}
	subHasTotal, errE := internalSearch.AggAs[types.CardinalityAggregate](subHasFilter.Aggregations, "total")
	if errE != nil {
		return nil, nil, errE
	}

	results, errE := hasBucketsToResults(subHasBuckets, "subHas")
	if errE != nil {
		return nil, nil, errE
	}
	// Force-show the selected properties (at count 0 when unmatched) so the selection is always visible and
	// deselectable.
	results = mergeSelectedHasProps(results, f.Props)

	subHasTotalValue := distinctValuesTotal(len(subHasBuckets), subHasTotal.Value)
	total := strconv.FormatInt(subHasTotalValue, 10)

	return results, map[string]any{
		"total": total,
	}, nil
}

// getMatchingHasPropIDs runs the has filter's value-search path: it returns only the property ids whose property
// name matched valueQuery, as id-only results (see buildHasValueSearchResults). The match set is the union of the
// in-scope value aggregation's bucket keys and the selected property ids the selectedMatch aggregation matched.
func (f *HasFilter) getMatchingHasPropIDs(
	ctx context.Context, getSearchService func() *esSearch.Search,
	query types.QueryVariant, valueQuery string, enabledLanguages []string,
) ([]HasFilterResult, map[string]any, errors.E) {
	metrics, _ := waf.GetMetrics(ctx)

	searchService := getSearchService()

	propLabelMatch := propLabelMatchQuery([]string{"claims.has.propNaming"}, []string{"claims.has.propDisplay"}, valueQuery, enabledLanguages)

	searchService = searchService.Size(0).Query(query).
		AddAggregation("has", hasPropAggregation("claims.has", propLabelMatch))

	// The selectedMatch label-matches the selected has-properties globally so an active filter's selection (which
	// has zero documents in the search scope) stays searchable by its own label, using the SAME matcher real
	// properties use. The has facet is flat, so there are no ancestors to surface.
	if len(f.Props) > 0 {
		selectedMatchFilter := esdsl.NewBoolQuery().Must(propLabelMatch, hasPropsTermsQuery("claims.has", f.Props))
		searchService = searchService.AddAggregation("selectedMatch", selectedMatchAggregation("claims.has", "prop", selectedMatchFilter))
	}

	m := metrics.Duration(internalStore.MetricElasticSearch).Start()
	res, err := searchService.Do(ctx)
	m.Stop()
	if err != nil {
		return nil, nil, WithESError(err)
	}
	metrics.Duration(internalStore.MetricElasticSearchInternal).Duration = time.Duration(res.Took) * time.Millisecond

	return buildHasValueSearchResults(res.Aggregations, "has", len(f.Props) > 0)
}

// Get retrieves has filter data for search results.
func (f *HasFilter) Get(
	ctx context.Context, getSearchService func() *esSearch.Search,
	query types.QueryVariant, valueQuery string, enabledLanguages []string,
) ([]HasFilterResult, map[string]any, errors.E) {
	// During a filter-pane value search the response carries only the matching property ids (an overlay the
	// frontend applies on top of its unfiltered primary), so it takes a dedicated path.
	if valueQuery != "" {
		return f.getMatchingHasPropIDs(ctx, getSearchService, query, valueQuery, enabledLanguages)
	}

	metrics, _ := waf.GetMetrics(ctx)

	searchService := getSearchService()

	// Aggregation for has claims: terms on claims.has.prop.
	// Only simple has claims (without sub-claims) are indexed in claims.has, so no
	// additional filtering is needed. Has claims with sub-claims are stored in claims.subRef.
	searchService = searchService.Size(0).Query(query).
		AddAggregation("has", hasPropAggregation("claims.has", esdsl.NewMatchAllQuery()))

	m := metrics.Duration(internalStore.MetricElasticSearch).Start()
	res, err := searchService.Do(ctx)
	m.Stop()
	if err != nil {
		return nil, nil, WithESError(err)
	}
	metrics.Duration(internalStore.MetricElasticSearchInternal).Duration = time.Duration(res.Took) * time.Millisecond

	hasNested, errE := internalSearch.AggAs[types.NestedAggregate](res.Aggregations, "has")
	if errE != nil {
		return nil, nil, errE
	}
	hasFilter, errE := internalSearch.AggAs[types.FilterAggregate](hasNested.Aggregations, "filter")
	if errE != nil {
		return nil, nil, errE
	}
	hasTerms, errE := internalSearch.AggAs[types.StringTermsAggregate](hasFilter.Aggregations, "props")
	if errE != nil {
		return nil, nil, errE
	}
	hasBuckets, ok := hasTerms.Buckets.([]types.StringTermsBucket)
	if !ok {
		errE := errors.New("unexpected bucket type for has")
		errors.Details(errE)["type"] = fmt.Sprintf("%T", hasTerms.Buckets)
		return nil, nil, errE
	}
	hasTotal, errE := internalSearch.AggAs[types.CardinalityAggregate](hasFilter.Aggregations, "total")
	if errE != nil {
		return nil, nil, errE
	}

	results, errE := hasBucketsToResults(hasBuckets, "has")
	if errE != nil {
		return nil, nil, errE
	}
	// Force-show the selected properties (at count 0 when unmatched) so the selection is always visible and
	// deselectable.
	results = mergeSelectedHasProps(results, f.Props)

	hasTotalValue := distinctValuesTotal(len(hasBuckets), hasTotal.Value)
	total := strconv.FormatInt(hasTotalValue, 10)

	return results, map[string]any{
		"total": total,
	}, nil
}
