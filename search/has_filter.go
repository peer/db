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

// mergeMatchedHasProps appends, during a value search, at count 0, each selected has-property whose label
// matched the typed text (the matched set comes from the selectedMatch global aggregations) and that is not
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

// hasPropsMerge accumulates has-property buckets by property id in first-seen order, summing
// document counts across aggregation contexts. A document contributing the same property through
// two parent collections is the residual overcount described in the package comment (the same
// parent property stated in two value types with the same has sub-claim under each).
type hasPropsMerge struct {
	Order  []string
	Counts map[string]int64
}

func newHasPropsMerge() *hasPropsMerge {
	return &hasPropsMerge{Order: nil, Counts: map[string]int64{}}
}

// AddBuckets folds one props terms aggregation's buckets (each with a "docs" reverse_nested count)
// into the merge.
func (m *hasPropsMerge) AddBuckets(buckets []types.StringTermsBucket, name string) errors.E {
	for _, bucket := range buckets {
		key, ok := bucket.Key.(string)
		if !ok {
			errE := errors.New("unexpected key type for " + name + " bucket")
			errors.Details(errE)["type"] = fmt.Sprintf("%T", bucket.Key)
			return errE
		}
		bucketDocs, errE := internalSearch.AggAs[types.ReverseNestedAggregate](bucket.Aggregations, "docs")
		if errE != nil {
			return errE
		}
		if _, ok := m.Counts[key]; !ok {
			m.Order = append(m.Order, key)
		}
		m.Counts[key] += bucketDocs.DocCount
	}
	return nil
}

// Results converts the merged buckets into HasFilterResult entries, dropping properties in the
// excluded set (those that migrated out of the pooled facet).
func (m *hasPropsMerge) Results(excluded map[string]bool) []HasFilterResult {
	out := make([]HasFilterResult, 0, len(m.Order))
	for _, id := range m.Order {
		if excluded[id] {
			continue
		}
		out = append(out, HasFilterResult{ID: id, Count: m.Counts[id]})
	}
	return out
}

// hasPropsTermsAggregation builds a props terms aggregation over rel records at path, each bucket
// carrying a "docs" reverse_nested document count.
func hasPropsTermsAggregation(path string) *types.Aggregations {
	return esdsl.NewAggregations().
		Terms(esdsl.NewTermsAggregation().Field(path+".prop").Size(MaxResultsCount).
			Order(esdsl.NewAggregateOrder().Map(map[string]sortorder.SortOrder{"docs": sortorder.Desc}))).
		AddAggregation("docs", esdsl.NewAggregations().
			ReverseNested(esdsl.NewReverseNestedAggregation())).
		AggregationsCaster()
}

// collectTermKeys extracts the string keys of a terms aggregation into out.
func collectTermKeys(agg *types.StringTermsAggregate, out map[string]bool) {
	buckets, ok := agg.Buckets.([]types.StringTermsBucket)
	if !ok {
		return
	}
	for _, bucket := range buckets {
		if key, ok := bucket.Key.(string); ok {
			out[key] = true
		}
	}
}

// notHasClaimTypeQuery matches rel records at path with any claimType other than has.
func notHasClaimTypeQuery(path string) types.QueryVariant { //nolint:ireturn
	return esdsl.NewBoolQuery().MustNot(claimTypeTerm(path, internalSearch.ClaimTypeHas))
}

// Get retrieves pooled has facet data: the has-properties whose only facetable claims in scope are
// has claims. Properties with any other facetable claim (a rel record of another claimType, an amount,
// or a time claim) migrated to their own facet, where their has claims surface as the "has
// property" special value, so they are subtracted here. The subtraction sets are capped at
// MaxResultsCount terms each; a migrated property past every cap would stay listed, matching the
// cap philosophy of the value lists.
//
// hiddenFacetProperties are left out of the listing, matching the facet's count in FiltersGet, except
// the ones this filter selects: a selected property stays listed with its count so the selection stays
// visible and deselectable.
func (f *HasFilter) Get(
	ctx context.Context, getSearchService func() *esSearch.Search,
	query types.QueryVariant, valueQuery string, hiddenFacetProperties map[string]bool, languages *Languages,
) ([]HasFilterResult, map[string]any, errors.E) {
	metrics, _ := waf.GetMetrics(ctx)

	enabledLanguages := languages.enabled()

	searchService := getSearchService()

	// valueQuery restricts the facet to has-properties whose display label matches the user-typed text, so
	// the filter pane can be narrowed without changing the search. It never alters which documents match.
	// The exception is when the query matched the "has property" special label itself: the whole pooled has
	// facet is then what the user is searching for, so the property list is shown in full (like no value
	// search), which discovery mirrors by keeping the facet on that special.
	spec := matchedSpecials(valueQuery, languages.special())
	narrowByText := valueQuery != "" && !spec.HasProperty

	hasFilterMusts := []types.QueryVariant{claimTypeTerm(relPath, internalSearch.ClaimTypeHas)}
	var propLabelMatch types.QueryVariant
	if narrowByText {
		propLabelMatch = propLabelMatchQuery([]string{relPath + ".propNaming"}, []string{relPath + ".propDisplay"}, valueQuery, enabledLanguages)
		hasFilterMusts = append(hasFilterMusts, propLabelMatch)
	}
	hidden := hiddenFacetClauses(hiddenExceptSelected(hiddenFacetProperties, f.Props), relPath+".prop")

	hasAggregation := esdsl.NewAggregations().
		Nested(esdsl.NewNestedAggregation().Path(relPath)).
		AddAggregation("filter", esdsl.NewAggregations().
			Filter(esdsl.NewBoolQuery().Must(hasFilterMusts...).MustNot(hidden...)).
			AddAggregation("props", hasPropsTermsAggregation(relPath)).
			AddAggregation("total", esdsl.NewAggregations().
				Cardinality(esdsl.NewCardinalityAggregation().Field(relPath+".prop").PrecisionThreshold(maxPrecisionThreshold))))

	// The migrated-out properties: those with a non-has rel record, an amount claim, or a time claim
	// in scope.
	otherClaimTypesAggregation := esdsl.NewAggregations().
		Nested(esdsl.NewNestedAggregation().Path(relPath)).
		AddAggregation("filter", esdsl.NewAggregations().
			Filter(notHasClaimTypeQuery(relPath)).
			AddAggregation("props", esdsl.NewAggregations().
				Terms(esdsl.NewTermsAggregation().Field(relPath+".prop").Size(MaxResultsCount))))
	amountPropsAggregation := esdsl.NewAggregations().
		Nested(esdsl.NewNestedAggregation().Path(amountPath)).
		AddAggregation("props", esdsl.NewAggregations().
			Terms(esdsl.NewTermsAggregation().Field(amountPath+".prop").Size(MaxResultsCount)))
	timePropsAggregation := esdsl.NewAggregations().
		Nested(esdsl.NewNestedAggregation().Path(timePath)).
		AddAggregation("props", esdsl.NewAggregations().
			Terms(esdsl.NewTermsAggregation().Field(timePath+".prop").Size(MaxResultsCount)))

	searchService = searchService.Size(0).Query(query).
		AddAggregation("has", hasAggregation).
		AddAggregation("otherClaimTypes", otherClaimTypesAggregation).
		AddAggregation("amountProps", amountPropsAggregation).
		AddAggregation("timeProps", timePropsAggregation)

	// During a value search, label-match the selected has-properties globally so an active filter's selection
	// (which has zero documents in the search scope) can still be narrowed by its own label, using the SAME
	// matcher real properties use. The has facet is flat, so there are no ancestors to surface.
	if narrowByText && len(f.Props) > 0 {
		searchService = searchService.AddAggregation("selectedMatch", esdsl.NewAggregations().
			Global(esdsl.NewGlobalAggregation()).
			AddAggregation("nested", esdsl.NewAggregations().
				Nested(esdsl.NewNestedAggregation().Path(relPath)).
				AddAggregation("filter", esdsl.NewAggregations().
					Filter(esdsl.NewBoolQuery().Must(
						claimTypeTerm(relPath, internalSearch.ClaimTypeHas),
						propLabelMatch,
						hasPropsTerms(relPath, f.Props),
					)).
					AddAggregation("match", esdsl.NewAggregations().
						Terms(esdsl.NewTermsAggregation().Field(relPath+".prop").Size(MaxResultsCount))))))
	}

	m := metrics.Duration(internalStore.MetricElasticSearch).Start()
	res, err := searchService.Do(ctx)
	m.Stop()
	if err != nil {
		return nil, nil, WithESError(ctx, err)
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

	excluded := map[string]bool{}
	errE = parseExcludedProps(res.Aggregations, "otherClaimTypes", true, excluded)
	if errE != nil {
		return nil, nil, errE
	}
	errE = parseExcludedProps(res.Aggregations, "amountProps", false, excluded)
	if errE != nil {
		return nil, nil, errE
	}
	errE = parseExcludedProps(res.Aggregations, "timeProps", false, excluded)
	if errE != nil {
		return nil, nil, errE
	}

	merge := newHasPropsMerge()
	errE = merge.AddBuckets(hasBuckets, "has")
	if errE != nil {
		return nil, nil, errE
	}
	results := merge.Results(excluded)

	// Outside a value search, force-show the selected properties (at count 0 when unmatched) so the selection is
	// always visible and deselectable. During a value search a selected property is shown only when its own label
	// matches the typed text (from the selectedMatch aggregation), so it stays searchable but is not force-shown.
	if !narrowByText {
		results = mergeSelectedHasProps(results, f.Props)
	} else if len(f.Props) > 0 {
		selectedMatch, errE := internalSearch.AggAs[types.GlobalAggregate](res.Aggregations, "selectedMatch")
		if errE != nil {
			return nil, nil, errE
		}
		matched := map[string]bool{}
		errE = parseSelectedMatchIDs(selectedMatch, 1, matched)
		if errE != nil {
			return nil, nil, errE
		}
		results = mergeMatchedHasProps(results, f.Props, matched)
	}

	// The total is exact while the has terms are unsaturated (the pooled set is then fully known);
	// past the cap the cardinality (which cannot see the subtraction) is the estimate.
	var hasTotalValue int64
	if len(hasBuckets) < MaxResultsCount {
		hasTotalValue = int64(len(results))
	} else {
		hasTotalValue = max(int64(len(results)), hasTotal.Value)
	}
	total := strconv.FormatInt(hasTotalValue, 10)

	return results, map[string]any{
		"total": total,
	}, nil
}

// parseExcludedProps folds one migrated-props aggregation into the excluded set. Filtered
// aggregations (the non-has rel claim types) carry an intermediate "filter" level.
func parseExcludedProps(aggs map[string]types.Aggregate, name string, filtered bool, out map[string]bool) errors.E {
	nested, errE := internalSearch.AggAs[types.NestedAggregate](aggs, name)
	if errE != nil {
		return errE
	}
	level := nested.Aggregations
	if filtered {
		filter, errE := internalSearch.AggAs[types.FilterAggregate](level, "filter")
		if errE != nil {
			return errE
		}
		level = filter.Aggregations
	}
	terms, errE := internalSearch.AggAs[types.StringTermsAggregate](level, "props")
	if errE != nil {
		return errE
	}
	collectTermKeys(terms, out)
	return nil
}

// GetSubHas retrieves pooled sub-has facet data: the has-properties nested under qualifying parent
// claims whose only facetable sub-claims are has records. It runs once per parent collection the
// context allows and merges in Go, mirroring Get's pooling subtraction one level down. parentCtx
// scopes every aggregation to qualifying parent claims.
//
// hiddenFacetProperties are left out of the listing the same way Get does it, the ones this filter
// selects excepted.
func (f *HasFilter) GetSubHas(
	ctx context.Context, getSearchService func() *esSearch.Search,
	query types.QueryVariant, parentCtx *ParentContext,
	valueQuery string, hiddenFacetProperties map[string]bool, languages *Languages,
) ([]HasFilterResult, map[string]any, errors.E) {
	metrics, _ := waf.GetMetrics(ctx)

	enabledLanguages := languages.enabled()

	searchService := getSearchService()

	// The value query narrows the listed sub-has-properties by their label, except when it matched the
	// "has property" special label itself: the whole pooled sub-has facet is then shown in full, the same
	// facet discovery keeps on that special.
	spec := matchedSpecials(valueQuery, languages.special())
	narrowByText := valueQuery != "" && !spec.HasProperty

	collections := parentCtx.Collections()
	var propLabelMatch types.QueryVariant
	for _, parent := range collections {
		pf, ok := parentCtx.CollectionFilter(parent)
		if !ok {
			continue
		}
		subRel := subPath(parent, "rel")
		subMusts := []types.QueryVariant{claimTypeTerm(subRel, internalSearch.ClaimTypeHas)}
		if narrowByText {
			propLabelMatch = propLabelMatchQuery([]string{subRel + ".propNaming"}, []string{subRel + ".propDisplay"}, valueQuery, enabledLanguages)
			subMusts = append(subMusts, propLabelMatch)
		}
		hidden := hiddenFacetClauses(hiddenExceptSelected(hiddenFacetProperties, f.Props), subRel+".prop")
		searchService = searchService.AddAggregation("has:"+parent, esdsl.NewAggregations().
			Nested(esdsl.NewNestedAggregation().Path(parentPath(parent))).
			AddAggregation("parentFilter", esdsl.NewAggregations().
				Filter(pf).
				AddAggregation("sub", esdsl.NewAggregations().
					Nested(esdsl.NewNestedAggregation().Path(subRel)).
					AddAggregation("filter", esdsl.NewAggregations().
						Filter(esdsl.NewBoolQuery().Must(subMusts...).MustNot(hidden...)).
						AddAggregation("props", hasPropsTermsAggregation(subRel))))))
		searchService = searchService.AddAggregation("otherClaimTypes:"+parent, esdsl.NewAggregations().
			Nested(esdsl.NewNestedAggregation().Path(parentPath(parent))).
			AddAggregation("parentFilter", esdsl.NewAggregations().
				Filter(pf).
				AddAggregation("sub", esdsl.NewAggregations().
					Nested(esdsl.NewNestedAggregation().Path(subRel)).
					AddAggregation("filter", esdsl.NewAggregations().
						Filter(notHasClaimTypeQuery(subRel)).
						AddAggregation("props", esdsl.NewAggregations().
							Terms(esdsl.NewTermsAggregation().Field(subRel+".prop").Size(MaxResultsCount)))))))
		for _, sub := range []string{"amount", "time"} {
			path := subPath(parent, sub)
			searchService = searchService.AddAggregation(sub+"Props:"+parent, esdsl.NewAggregations().
				Nested(esdsl.NewNestedAggregation().Path(parentPath(parent))).
				AddAggregation("parentFilter", esdsl.NewAggregations().
					Filter(pf).
					AddAggregation("sub", esdsl.NewAggregations().
						Nested(esdsl.NewNestedAggregation().Path(path)).
						AddAggregation("props", esdsl.NewAggregations().
							Terms(esdsl.NewTermsAggregation().Field(path+".prop").Size(MaxResultsCount))))))
		}
		if narrowByText && len(f.Props) > 0 {
			// selectedMatch is scoped to the parent property and the selected prop ids, deliberately without
			// the rest of the parent context, so a checked property is never hidden.
			searchService = searchService.AddAggregation("selectedMatch:"+parent, esdsl.NewAggregations().
				Global(esdsl.NewGlobalAggregation()).
				AddAggregation("nested", esdsl.NewAggregations().
					Nested(esdsl.NewNestedAggregation().Path(parentPath(parent))).
					AddAggregation("nested", esdsl.NewAggregations().
						Nested(esdsl.NewNestedAggregation().Path(subRel)).
						AddAggregation("filter", esdsl.NewAggregations().
							Filter(esdsl.NewBoolQuery().Must(
								claimTypeTerm(subRel, internalSearch.ClaimTypeHas),
								propLabelMatch,
								hasPropsTerms(subRel, f.Props),
							)).
							AddAggregation("match", esdsl.NewAggregations().
								Terms(esdsl.NewTermsAggregation().Field(subRel+".prop").Size(MaxResultsCount)))))))
		}
	}

	searchService = searchService.Size(0).Query(query)

	m := metrics.Duration(internalStore.MetricElasticSearch).Start()
	res, err := searchService.Do(ctx)
	m.Stop()
	if err != nil {
		return nil, nil, WithESError(ctx, err)
	}
	metrics.Duration(internalStore.MetricElasticSearchInternal).Duration = time.Duration(res.Took) * time.Millisecond

	merge := newHasPropsMerge()
	excluded := map[string]bool{}
	for _, parent := range collections {
		buckets, errE := parseSubHasPropBuckets(res.Aggregations, "has:"+parent, true)
		if errE != nil {
			return nil, nil, errE
		}
		errE = merge.AddBuckets(buckets, "has:"+parent)
		if errE != nil {
			return nil, nil, errE
		}
		for _, name := range []string{"otherClaimTypes:" + parent, "amountProps:" + parent, "timeProps:" + parent} {
			filtered := name == "otherClaimTypes:"+parent
			keyBuckets, errE := parseSubHasPropBuckets(res.Aggregations, name, filtered)
			if errE != nil {
				return nil, nil, errE
			}
			for _, bucket := range keyBuckets {
				if key, ok := bucket.Key.(string); ok {
					excluded[key] = true
				}
			}
		}
	}
	results := merge.Results(excluded)

	if !narrowByText {
		results = mergeSelectedHasProps(results, f.Props)
	} else if len(f.Props) > 0 {
		matched := map[string]bool{}
		for _, parent := range collections {
			selectedMatch, errE := internalSearch.AggAs[types.GlobalAggregate](res.Aggregations, "selectedMatch:"+parent)
			if errE != nil {
				return nil, nil, errE
			}
			errE = parseSelectedMatchIDs(selectedMatch, 2, matched) //nolint:mnd
			if errE != nil {
				return nil, nil, errE
			}
		}
		results = mergeMatchedHasProps(results, f.Props, matched)
	}

	// The total is exact while no per-collection terms aggregation saturated; past the cap it is a
	// lower bound, matching the value lists' cap philosophy.
	total := strconv.FormatInt(int64(len(results)), 10)

	return results, map[string]any{
		"total": total,
	}, nil
}

// parseSubHasPropBuckets extracts the props terms buckets from one parent collection's sub-has
// aggregation (nested parent -> pf -> nested sub [-> filter] -> props).
func parseSubHasPropBuckets(aggs map[string]types.Aggregate, name string, filtered bool) ([]types.StringTermsBucket, errors.E) {
	parentNested, errE := internalSearch.AggAs[types.NestedAggregate](aggs, name)
	if errE != nil {
		return nil, errE
	}
	pf, errE := internalSearch.AggAs[types.FilterAggregate](parentNested.Aggregations, "parentFilter")
	if errE != nil {
		return nil, errE
	}
	subNested, errE := internalSearch.AggAs[types.NestedAggregate](pf.Aggregations, "sub")
	if errE != nil {
		return nil, errE
	}
	level := subNested.Aggregations
	if filtered {
		filter, errE := internalSearch.AggAs[types.FilterAggregate](level, "filter")
		if errE != nil {
			return nil, errE
		}
		level = filter.Aggregations
	}
	terms, errE := internalSearch.AggAs[types.StringTermsAggregate](level, "props")
	if errE != nil {
		return nil, errE
	}
	buckets, ok := terms.Buckets.([]types.StringTermsBucket)
	if !ok {
		errE := errors.New("unexpected bucket type for " + name)
		errors.Details(errE)["type"] = fmt.Sprintf("%T", terms.Buckets)
		return nil, errE
	}
	return buckets, nil
}
