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

// mergeMatchedHasProps appends, during a value search, at count 0, each selected has-property whose label
// matched the typed text (the matched set comes from the selectedMatch global aggregation) and that is not
// already present. It is the value-search counterpart of mergeSelectedHasProps, which force-shows the whole
// selection outside a search; here only the matched selected properties are surfaced, so a selected property
// (which has zero documents in the search scope) stays searchable by its own label.
func mergeMatchedHasProps(results []HasFilterResult, props []HasValue, matched map[string]bool) []HasFilterResult {
	present := make(map[string]bool, len(results))
	for _, r := range results {
		present[r.ID] = true
	}
	for _, p := range props {
		id := p.ID.String()
		if !matched[id] || present[id] {
			continue
		}
		results = append(results, HasFilterResult{ID: id, Count: 0})
		present[id] = true
	}
	return results
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
	// valueQuery restricts the facet to has-properties whose display label matches the user-typed text, so
	// the filter pane can be narrowed without changing the search. It never alters which documents match.
	var propLabelMatch types.QueryVariant
	if valueQuery != "" {
		propLabelMatch = propLabelMatchQuery(
			[]string{"claims.subHas.propNaming", "claims.subHas.parentPropNaming"},
			[]string{"claims.subHas.propDisplay", "claims.subHas.parentPropDisplay"}, valueQuery, enabledLanguages)
		filterMusts = append(filterMusts, propLabelMatch)
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
				Cardinality(esdsl.NewCardinalityAggregation().Field("claims.subHas.prop").PrecisionThreshold(maxPrecisionThreshold))))

	searchService = searchService.Size(0).Query(query).
		AddAggregation("subHas", subHasAggregation)

	// During a value search, label-match the selected sub-has properties globally so an active filter's selection
	// (which has zero documents in the search scope) can still be narrowed by its own label. It is scoped to the
	// parent property and the selected prop ids, mirroring the facet's own filter.
	if valueQuery != "" && len(f.Props) > 0 {
		selectedMatchFilter := esdsl.NewBoolQuery().Must(
			esdsl.NewTermQuery("claims.subHas.parentProp", esdsl.NewFieldValue().String(parentProp.String())),
			propLabelMatch,
			hasPropsTermsQuery("claims.subHas", f.Props),
		)
		searchService = searchService.AddAggregation("selectedMatch", selectedMatchAggregation("claims.subHas", "prop", selectedMatchFilter))
	}

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

	results := make([]HasFilterResult, 0, len(subHasBuckets))
	for _, bucket := range subHasBuckets {
		bucketDocs, errE := internalSearch.AggAs[types.ReverseNestedAggregate](bucket.Aggregations, "docs")
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
	// Outside a value search, force-show the selected properties (at count 0 when unmatched) so the selection is
	// always visible and deselectable. During a value search a selected property is shown only when its own label
	// matches the typed text (from the selectedMatch aggregation), so it stays searchable but is not force-shown.
	if valueQuery == "" {
		results = mergeSelectedHasProps(results, f.Props)
	} else if len(f.Props) > 0 {
		selectedMatch, errE := internalSearch.AggAs[types.GlobalAggregate](res.Aggregations, "selectedMatch")
		if errE != nil {
			return nil, nil, errE
		}
		matched, errE := parseSelectedMatchIDs(selectedMatch)
		if errE != nil {
			return nil, nil, errE
		}
		results = mergeMatchedHasProps(results, f.Props, matched)
	}

	subHasTotalValue := distinctValuesTotal(len(subHasBuckets), subHasTotal.Value)
	total := strconv.FormatInt(subHasTotalValue, 10)

	return results, map[string]any{
		"total": total,
	}, nil
}

// Get retrieves has filter data for search results.
func (f *HasFilter) Get(
	ctx context.Context, getSearchService func() *esSearch.Search,
	query types.QueryVariant, valueQuery string, enabledLanguages []string,
) ([]HasFilterResult, map[string]any, errors.E) {
	metrics, _ := waf.GetMetrics(ctx)

	searchService := getSearchService()

	// valueQuery restricts the facet to has-properties whose display label matches the user-typed text, so
	// the filter pane can be narrowed without changing the search. It never alters which documents match.
	var hasFilterQuery types.QueryVariant = esdsl.NewMatchAllQuery()
	var propLabelMatch types.QueryVariant
	if valueQuery != "" {
		propLabelMatch = propLabelMatchQuery([]string{"claims.has.propNaming"}, []string{"claims.has.propDisplay"}, valueQuery, enabledLanguages)
		hasFilterQuery = propLabelMatch
	}

	// Aggregation for has claims: terms on claims.has.prop.
	// Only simple has claims (without sub-claims) are indexed in claims.has, so no
	// additional filtering is needed. Has claims with sub-claims are stored in claims.subRef.
	hasAggregation := esdsl.NewAggregations().
		Nested(esdsl.NewNestedAggregation().Path("claims.has")).
		AddAggregation("filter", esdsl.NewAggregations().
			Filter(hasFilterQuery).
			AddAggregation("props", esdsl.NewAggregations().
				Terms(esdsl.NewTermsAggregation().Field("claims.has.prop").Size(MaxResultsCount).
					Order(esdsl.NewAggregateOrder().Map(map[string]sortorder.SortOrder{"docs": sortorder.Desc}))).
				AddAggregation("docs", esdsl.NewAggregations().
					ReverseNested(esdsl.NewReverseNestedAggregation()))).
			AddAggregation("total", esdsl.NewAggregations().
				Cardinality(esdsl.NewCardinalityAggregation().Field("claims.has.prop").PrecisionThreshold(maxPrecisionThreshold))))

	searchService = searchService.Size(0).Query(query).
		AddAggregation("has", hasAggregation)

	// During a value search, label-match the selected has-properties globally so an active filter's selection
	// (which has zero documents in the search scope) can still be narrowed by its own label, using the SAME
	// matcher real properties use. The has facet is flat, so there are no ancestors to surface.
	if valueQuery != "" && len(f.Props) > 0 {
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

	results := make([]HasFilterResult, 0, len(hasBuckets))
	for _, bucket := range hasBuckets {
		bucketDocs, errE := internalSearch.AggAs[types.ReverseNestedAggregate](bucket.Aggregations, "docs")
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
	// Outside a value search, force-show the selected properties (at count 0 when unmatched) so the selection is
	// always visible and deselectable. During a value search a selected property is shown only when its own label
	// matches the typed text (from the selectedMatch aggregation), so it stays searchable but is not force-shown.
	if valueQuery == "" {
		results = mergeSelectedHasProps(results, f.Props)
	} else if len(f.Props) > 0 {
		selectedMatch, errE := internalSearch.AggAs[types.GlobalAggregate](res.Aggregations, "selectedMatch")
		if errE != nil {
			return nil, nil, errE
		}
		matched, errE := parseSelectedMatchIDs(selectedMatch)
		if errE != nil {
			return nil, nil, errE
		}
		results = mergeMatchedHasProps(results, f.Props, matched)
	}

	hasTotalValue := distinctValuesTotal(len(hasBuckets), hasTotal.Value)
	total := strconv.FormatInt(hasTotalValue, 10)

	return results, map[string]any{
		"total": total,
	}, nil
}
