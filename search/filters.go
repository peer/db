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
	"gitlab.com/tozd/waf"

	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
	internalStore "gitlab.com/peerdb/peerdb/internal/store"
)

// FilterResult describes an available filter as an union of possible fields for each supported filter type.
type FilterResult struct {
	Props    []string `json:"props,omitempty"`
	Type     string   `json:"type"`
	Unit     string   `json:"unit,omitempty"`
	FilterID string   `json:"filterId,omitempty"`
	Count    int64    `json:"count"`
}

// matchedDocCount reads a discovery bucket's per-bucket "matched" filter and its "matchedDocs" reverse-nested
// count, the number of the bucket's records that match the value query. It is used only to decide whether a
// facet is kept, never as the facet's reported count.
func matchedDocCount(aggs map[string]types.Aggregate) (int64, errors.E) {
	matched, errE := internalSearch.AggAs[types.FilterAggregate](aggs, "matched")
	if errE != nil {
		return 0, errE
	}
	matchedDocs, errE := internalSearch.AggAs[types.ReverseNestedAggregate](matched.Aggregations, "matchedDocs")
	if errE != nil {
		return 0, errE
	}
	return matchedDocs.DocCount, nil
}

// parseStringTermsBuckets converts string terms buckets with reverse-nested doc counts into FilterResult slices.
// Count is always the bucket's full document count. When valueQueryActive is true, a bucket whose records do not
// match the value query (matchedDocs is zero) is skipped, so the value query decides only which facets appear.
func parseStringTermsBuckets(buckets []types.StringTermsBucket, filterType string, valueQueryActive bool) ([]FilterResult, errors.E) {
	results := make([]FilterResult, 0, len(buckets))
	for _, bucket := range buckets {
		bucketDocs, errE := internalSearch.AggAs[types.ReverseNestedAggregate](bucket.Aggregations, "docs")
		if errE != nil {
			return nil, errE
		}
		if valueQueryActive {
			matchedCount, errE := matchedDocCount(bucket.Aggregations)
			if errE != nil {
				return nil, errE
			}
			if matchedCount == 0 {
				continue
			}
		}
		key, ok := bucket.Key.(string)
		if !ok {
			errE := errors.New("unexpected key type for bucket")
			errors.Details(errE)["type"] = fmt.Sprintf("%T", bucket.Key)
			return nil, errE
		}
		results = append(results, FilterResult{
			Props:    []string{key},
			Type:     filterType,
			Unit:     "",
			FilterID: "",
			Count:    bucketDocs.DocCount,
		})
	}
	return results, nil
}

// parseSubClaimBuckets converts multi-terms buckets keyed by (parentProp, prop)
// and optionally a third unit term into FilterResult entries with 2-element Props.
// The filterType becomes FilterResult.Type. aggName is used in error messages.
// When hasUnit is true, the third bucket key is the unit (with "__missing__" mapped
// to an empty Unit). Count is always the bucket's full document count. When valueQueryActive
// is true, a bucket whose records do not match the value query (matchedDocs is zero) is skipped,
// so the value query decides only which facets appear.
func parseSubClaimBuckets(buckets []types.MultiTermsBucket, filterType, aggName string, hasUnit, valueQueryActive bool) ([]FilterResult, errors.E) {
	results := make([]FilterResult, 0, len(buckets))
	for _, bucket := range buckets {
		bucketDocs, errE := internalSearch.AggAs[types.ReverseNestedAggregate](bucket.Aggregations, "docs")
		if errE != nil {
			return nil, errE
		}
		if valueQueryActive {
			matchedCount, errE := matchedDocCount(bucket.Aggregations)
			if errE != nil {
				return nil, errE
			}
			if matchedCount == 0 {
				continue
			}
		}
		if len(bucket.Key) < 2 { //nolint:mnd
			errE := errors.New("unexpected key length for bucket")
			errors.Details(errE)["agg"] = aggName
			return nil, errE
		}
		parentPropKey, ok := bucket.Key[0].(string)
		if !ok {
			errE := errors.New("unexpected key type for bucket parentProp")
			errors.Details(errE)["agg"] = aggName
			errors.Details(errE)["type"] = fmt.Sprintf("%T", bucket.Key[0])
			return nil, errE
		}
		propKey, ok := bucket.Key[1].(string)
		if !ok {
			errE := errors.New("unexpected key type for bucket prop")
			errors.Details(errE)["agg"] = aggName
			errors.Details(errE)["type"] = fmt.Sprintf("%T", bucket.Key[1])
			return nil, errE
		}
		var unit string
		if hasUnit {
			if len(bucket.Key) < 3 { //nolint:mnd
				errE := errors.New("unexpected key length for bucket with unit")
				errors.Details(errE)["agg"] = aggName
				return nil, errE
			}
			unitKey, ok := bucket.Key[2].(string)
			if !ok {
				errE := errors.New("unexpected key type for bucket unit")
				errors.Details(errE)["agg"] = aggName
				errors.Details(errE)["type"] = fmt.Sprintf("%T", bucket.Key[2])
				return nil, errE
			}
			if unitKey != "__missing__" {
				unit = unitKey
			}
		}
		results = append(results, FilterResult{
			Props:    []string{parentPropKey, propKey},
			Type:     filterType,
			Unit:     unit,
			FilterID: "",
			Count:    bucketDocs.DocCount,
		})
	}
	return results, nil
}

// parseMultiTermsBuckets converts multi-terms buckets into FilterResult slices.
// Count is always the bucket's full document count. When valueQueryActive is true, a bucket whose records do
// not match the value query (matchedDocs is zero) is skipped, so the value query decides only which facets appear.
func parseMultiTermsBuckets(buckets []types.MultiTermsBucket, valueQueryActive bool) ([]FilterResult, errors.E) {
	results := make([]FilterResult, 0, len(buckets))
	for _, bucket := range buckets {
		bucketDocs, errE := internalSearch.AggAs[types.ReverseNestedAggregate](bucket.Aggregations, "docs")
		if errE != nil {
			return nil, errE
		}
		if valueQueryActive {
			matchedCount, errE := matchedDocCount(bucket.Aggregations)
			if errE != nil {
				return nil, errE
			}
			if matchedCount == 0 {
				continue
			}
		}
		if len(bucket.Key) < 2 { //nolint:mnd
			return nil, errors.New("unexpected key length for amount bucket")
		}
		propKey, ok := bucket.Key[0].(string)
		if !ok {
			errE := errors.New("unexpected key type for amount bucket prop")
			errors.Details(errE)["type"] = fmt.Sprintf("%T", bucket.Key[0])
			return nil, errE
		}
		unitKey, ok := bucket.Key[1].(string)
		if !ok {
			errE := errors.New("unexpected key type for amount bucket unit")
			errors.Details(errE)["type"] = fmt.Sprintf("%T", bucket.Key[1])
			return nil, errE
		}
		unit := unitKey
		if unit == "__missing__" {
			unit = ""
		}
		results = append(results, FilterResult{
			Props:    []string{propKey},
			Type:     "amount",
			Unit:     unit,
			FilterID: "",
			Count:    bucketDocs.DocCount,
		})
	}
	return results, nil
}

// matchedDocsAggregation builds a discovery facet's per-bucket "matched" filter holding a "matchedDocs"
// reverse-nested count. It runs the value-query match over the bucket's records; the resulting count decides
// only whether the facet is kept, never the facet's reported count (which is the bucket's full "docs" count).
func matchedDocsAggregation(match types.QueryVariant) *types.Aggregations {
	return esdsl.NewAggregations().
		Filter(match).
		AddAggregation("matchedDocs", esdsl.NewAggregations().
			ReverseNested(esdsl.NewReverseNestedAggregation())).
		AggregationsCaster()
}

// FiltersGet retrieves all available filters for the current search.
// excludes drops, from the ref and subRef discovery aggregations, records a prefilter makes redundant
// (those derived from a prefilter value), so a facet whose only values come from the prefilter's own
// value hierarchy is left empty and therefore skipped.
func FiltersGet( //nolint:maintidx
	ctx context.Context, getSearchService func() *esSearch.Search, searchSession *Session, enabledLanguages []string,
	valueQuery string, excludes PrefilterExcludes, extraFilters ...types.QueryVariant,
) ([]FilterResult, map[string]any, errors.E) {
	metrics, _ := waf.GetMetrics(ctx)

	// The access filter goes on the top-level query only; ES scopes every
	// aggregation (including the per-active-filter ToQueryExcluding sub-aggregations
	// below) to the documents it matches, so facet counts never include
	// documents the caller cannot access.
	query := searchSession.ToQuery(enabledLanguages, extraFilters...)

	searchService := getSearchService()

	// When a value query is active, each discovery aggregation decides which facets to return, but never their
	// counts: a facet is kept when at least one of its records matches the query, either through one of the
	// facet's values or through the facet's own property name. Reference facets match on value and property
	// names; amount, time and has facets only have a property name to match (their values are numbers, dates,
	// or the listed properties themselves). This mirrors the matching the per-facet value endpoints use, so a
	// facet that appears here always has at least one value to show.
	//
	// The value query decides only which facets (gated on the per-bucket "matched" sub-aggregation) are
	// returned. Each facet's reported count (the per-bucket unscoped "docs") and the per-type "total" counts
	// stay outside the value query, so neither the individual facet counts nor the total number of available
	// filters (shown to the user next to the search box) change as the box is typed in. This matches the
	// active-filter counts, which are likewise value-query-independent.
	matchAll := types.QueryVariant(esdsl.NewMatchAllQuery())
	refMatch, amountMatch, timeMatch, hasMatch := matchAll, matchAll, matchAll, matchAll
	subRefMatch, subAmountMatch, subTimeMatch, subHasMatch := matchAll, matchAll, matchAll, matchAll
	if valueQuery != "" {
		// Reference facets match on value names (regular search query) and the property name (phrase-prefix).
		refMatch = labelMatchQuery(
			[]string{"claims.ref.toNaming"}, []string{"claims.ref.toDisplay"},
			[]string{"claims.ref.propNaming"}, []string{"claims.ref.propDisplay"}, valueQuery, enabledLanguages)
		// Sub-facets ("parentProp > prop") also match on the parent property's name, denormalized onto the
		// sub-claim records as parentPropNaming/parentPropDisplay.
		subRefMatch = labelMatchQuery(
			[]string{"claims.subRef.toNaming"}, []string{"claims.subRef.toDisplay"},
			[]string{"claims.subRef.propNaming", "claims.subRef.parentPropNaming"},
			[]string{"claims.subRef.propDisplay", "claims.subRef.parentPropDisplay"}, valueQuery, enabledLanguages)
		// Amount and time facets have no value documents, but their value bounds carry a formatted display
		// label (from/toDisplay), so they match on the property name or on a value-bound display.
		amountMatch = amountTimeMatchQuery(
			[]string{"claims.amount.propNaming"}, []string{"claims.amount.propDisplay"},
			[]string{"claims.amount.fromDisplay", "claims.amount.toDisplay"}, valueQuery, enabledLanguages)
		timeMatch = amountTimeMatchQuery(
			[]string{"claims.time.propNaming"}, []string{"claims.time.propDisplay"},
			[]string{"claims.time.fromDisplay", "claims.time.toDisplay"}, valueQuery, enabledLanguages)
		// The has facet has no value documents to search, so it matches on property names only.
		hasMatch = propLabelMatchQuery([]string{"claims.has.propNaming"}, []string{"claims.has.propDisplay"}, valueQuery, enabledLanguages)
		// Sub-amount and sub-time facets match like amount/time (property name or value-bound display), plus the
		// parent property's name.
		subAmountMatch = amountTimeMatchQuery(
			[]string{"claims.subAmount.propNaming", "claims.subAmount.parentPropNaming"},
			[]string{"claims.subAmount.propDisplay", "claims.subAmount.parentPropDisplay"},
			[]string{"claims.subAmount.fromDisplay", "claims.subAmount.toDisplay"}, valueQuery, enabledLanguages)
		subTimeMatch = amountTimeMatchQuery(
			[]string{"claims.subTime.propNaming", "claims.subTime.parentPropNaming"},
			[]string{"claims.subTime.propDisplay", "claims.subTime.parentPropDisplay"},
			[]string{"claims.subTime.fromDisplay", "claims.subTime.toDisplay"}, valueQuery, enabledLanguages)
		subHasMatch = propLabelMatchQuery(
			[]string{"claims.subHas.propNaming", "claims.subHas.parentPropNaming"},
			[]string{"claims.subHas.propDisplay", "claims.subHas.parentPropDisplay"}, valueQuery, enabledLanguages)
	}
	// The "scoped" filter drops records a prefilter makes redundant before the prop terms and total are
	// computed, so a property whose only ref records come from a prefilter value disappears from discovery.
	// Under each prop, "docs" is the facet's full document count (also the term order target), while the
	// per-prop "matched" filter holds, in "matchedDocs", the count of records reaching the value query, used
	// only to decide whether the facet is returned. "total" stays outside the value query so the
	// available-filters total is unaffected by it.
	refAggregation := esdsl.NewAggregations().
		Nested(esdsl.NewNestedAggregation().Path("claims.ref")).
		AddAggregation("scoped", esdsl.NewAggregations().
			Filter(excludes.refDiscoveryFilter()).
			AddAggregation("props", esdsl.NewAggregations().
				Terms(esdsl.NewTermsAggregation().Field("claims.ref.prop").Size(MaxResultsCount).
					Order(esdsl.NewAggregateOrder().Map(map[string]sortorder.SortOrder{"docs": sortorder.Desc}))).
				AddAggregation("docs", esdsl.NewAggregations().
					ReverseNested(esdsl.NewReverseNestedAggregation())).
				AddAggregation("matched", matchedDocsAggregation(refMatch))).
			AddAggregation("total", esdsl.NewAggregations().
				Cardinality(esdsl.NewCardinalityAggregation().Field("claims.ref.prop").PrecisionThreshold(maxPrecisionThreshold))))
	amountAggregation := esdsl.NewAggregations().
		Nested(esdsl.NewNestedAggregation().Path("claims.amount")).
		AddAggregation("props", esdsl.NewAggregations().
			MultiTerms(esdsl.NewMultiTermsAggregation().Terms(
				esdsl.NewMultiTermLookup().Field("claims.amount.prop"),
				// Units are document IDs, so valid units can never be string "__missing__".
				esdsl.NewMultiTermLookup().Field("claims.amount.unit").Missing(esdsl.NewMissing().String("__missing__")),
			).Size(MaxResultsCount).Order(esdsl.NewAggregateOrder().Map(map[string]sortorder.SortOrder{"docs": sortorder.Desc}))).
			AddAggregation("docs", esdsl.NewAggregations().
				ReverseNested(esdsl.NewReverseNestedAggregation())).
			AddAggregation("matched", matchedDocsAggregation(amountMatch))).
		AddAggregation("total", esdsl.NewAggregations().
			// TODO: Use a runtime field.
			//       See: https://www.elastic.co/guide/en/elasticsearch/reference/7.17/search-aggregations-metrics-cardinality-aggregation.html#_script_4
			Cardinality(esdsl.NewCardinalityAggregation().Script(
				// We use "|" as separator because this is used by ElasticSearch in "key_as_string" as well.
				// When unit is missing, "__missing__" is used as placeholder.
				esdsl.NewScript().Source(esdsl.NewScriptSource().String(
					`return doc['claims.amount.prop'].value + '|' + (doc['claims.amount.unit'].size() > 0 ? doc['claims.amount.unit'].value : '__missing__')`,
				)),
			).PrecisionThreshold(maxPrecisionThreshold)))
	timeAggregation := esdsl.NewAggregations().
		Nested(esdsl.NewNestedAggregation().Path("claims.time")).
		AddAggregation("props", esdsl.NewAggregations().
			Terms(esdsl.NewTermsAggregation().Field("claims.time.prop").Size(MaxResultsCount).
				Order(esdsl.NewAggregateOrder().Map(map[string]sortorder.SortOrder{"docs": sortorder.Desc}))).
			AddAggregation("docs", esdsl.NewAggregations().
				ReverseNested(esdsl.NewReverseNestedAggregation())).
			AddAggregation("matched", matchedDocsAggregation(timeMatch))).
		AddAggregation("total", esdsl.NewAggregations().
			Cardinality(esdsl.NewCardinalityAggregation().Field("claims.time.prop").PrecisionThreshold(maxPrecisionThreshold)))
	// Has aggregation counts documents that have at least one has claim.
	// Only simple has claims (without sub-claims) are indexed in claims.has, so no
	// additional filtering is needed. Unlike other filter types, has produces a single
	// filter rather than one per property.
	// The outer filter counts documents with any has claim, which is both the has facet's reported count and
	// its contribution to the available-filters total, regardless of the value query. The inner "matched"
	// filter counts those whose has-property matches the value query and only decides whether the has facet is
	// returned, never its count.
	hasAggregation := esdsl.NewAggregations().
		Filter(esdsl.NewNestedQuery(
			esdsl.NewMatchAllQuery(),
		).Path("claims.has")).
		AddAggregation("matched", esdsl.NewAggregations().
			Filter(esdsl.NewNestedQuery(
				hasMatch,
			).Path("claims.has")))
	// SubRef aggregation discovers available (parentProp, prop) combinations across all sub-references.
	subRefAggregation := esdsl.NewAggregations().
		Nested(esdsl.NewNestedAggregation().Path("claims.subRef")).
		AddAggregation("scoped", esdsl.NewAggregations().
			Filter(excludes.subRefDiscoveryFilter()).
			AddAggregation("props", esdsl.NewAggregations().
				MultiTerms(esdsl.NewMultiTermsAggregation().Terms(
					esdsl.NewMultiTermLookup().Field("claims.subRef.parentProp"),
					esdsl.NewMultiTermLookup().Field("claims.subRef.prop"),
				).Size(MaxResultsCount).Order(esdsl.NewAggregateOrder().Map(map[string]sortorder.SortOrder{"docs": sortorder.Desc}))).
				AddAggregation("docs", esdsl.NewAggregations().
					ReverseNested(esdsl.NewReverseNestedAggregation())).
				AddAggregation("matched", matchedDocsAggregation(subRefMatch))).
			AddAggregation("total", esdsl.NewAggregations().
				Cardinality(esdsl.NewCardinalityAggregation().Script(
					esdsl.NewScript().Source(esdsl.NewScriptSource().String(
						`return doc['claims.subRef.parentProp'].value + '|' + doc['claims.subRef.prop'].value`,
					)),
				).PrecisionThreshold(maxPrecisionThreshold))))
	// SubAmount aggregation discovers available (parentProp, prop, unit) combinations
	// across all sub-amounts. Units are document IDs, so "__missing__" is safe as the
	// missing-unit placeholder.
	subAmountAggregation := esdsl.NewAggregations().
		Nested(esdsl.NewNestedAggregation().Path("claims.subAmount")).
		AddAggregation("props", esdsl.NewAggregations().
			MultiTerms(esdsl.NewMultiTermsAggregation().Terms(
				esdsl.NewMultiTermLookup().Field("claims.subAmount.parentProp"),
				esdsl.NewMultiTermLookup().Field("claims.subAmount.prop"),
				esdsl.NewMultiTermLookup().Field("claims.subAmount.unit").Missing(esdsl.NewMissing().String("__missing__")),
			).Size(MaxResultsCount).Order(esdsl.NewAggregateOrder().Map(map[string]sortorder.SortOrder{"docs": sortorder.Desc}))).
			AddAggregation("docs", esdsl.NewAggregations().
				ReverseNested(esdsl.NewReverseNestedAggregation())).
			AddAggregation("matched", matchedDocsAggregation(subAmountMatch))).
		AddAggregation("total", esdsl.NewAggregations().
			// TODO: Use a runtime field.
			//       See: https://www.elastic.co/guide/en/elasticsearch/reference/7.17/search-aggregations-metrics-cardinality-aggregation.html#_script_4
			Cardinality(esdsl.NewCardinalityAggregation().Script(
				esdsl.NewScript().Source(esdsl.NewScriptSource().String(
					`return doc['claims.subAmount.parentProp'].value + '|' + doc['claims.subAmount.prop'].value + '|' + `+
						`(doc['claims.subAmount.unit'].size() > 0 ? doc['claims.subAmount.unit'].value : '__missing__')`,
				)),
			).PrecisionThreshold(maxPrecisionThreshold)))
	// SubTime aggregation discovers available (parentProp, prop) combinations across all sub-times.
	subTimeAggregation := esdsl.NewAggregations().
		Nested(esdsl.NewNestedAggregation().Path("claims.subTime")).
		AddAggregation("props", esdsl.NewAggregations().
			MultiTerms(esdsl.NewMultiTermsAggregation().Terms(
				esdsl.NewMultiTermLookup().Field("claims.subTime.parentProp"),
				esdsl.NewMultiTermLookup().Field("claims.subTime.prop"),
			).Size(MaxResultsCount).Order(esdsl.NewAggregateOrder().Map(map[string]sortorder.SortOrder{"docs": sortorder.Desc}))).
			AddAggregation("docs", esdsl.NewAggregations().
				ReverseNested(esdsl.NewReverseNestedAggregation())).
			AddAggregation("matched", matchedDocsAggregation(subTimeMatch))).
		AddAggregation("total", esdsl.NewAggregations().
			Cardinality(esdsl.NewCardinalityAggregation().Script(
				esdsl.NewScript().Source(esdsl.NewScriptSource().String(
					`return doc['claims.subTime.parentProp'].value + '|' + doc['claims.subTime.prop'].value`,
				)),
			).PrecisionThreshold(maxPrecisionThreshold)))
	// SubHas aggregation discovers parent properties under which sub-has filters
	// can be applied. The user later selects which has-properties to match via
	// HasFilter.Props, so discovery only enumerates parentProp.
	subHasAggregation := esdsl.NewAggregations().
		Nested(esdsl.NewNestedAggregation().Path("claims.subHas")).
		AddAggregation("props", esdsl.NewAggregations().
			Terms(esdsl.NewTermsAggregation().Field("claims.subHas.parentProp").Size(MaxResultsCount).
				Order(esdsl.NewAggregateOrder().Map(map[string]sortorder.SortOrder{"docs": sortorder.Desc}))).
			AddAggregation("docs", esdsl.NewAggregations().
				ReverseNested(esdsl.NewReverseNestedAggregation())).
			AddAggregation("matched", matchedDocsAggregation(subHasMatch))).
		AddAggregation("total", esdsl.NewAggregations().
			Cardinality(esdsl.NewCardinalityAggregation().Field("claims.subHas.parentProp").PrecisionThreshold(maxPrecisionThreshold)))
	searchService = searchService.Size(0).Query(query).
		AddAggregation("ref", refAggregation).
		AddAggregation("amount", amountAggregation).
		AddAggregation("time", timeAggregation).
		AddAggregation("has", hasAggregation).
		AddAggregation("subRef", subRefAggregation).
		AddAggregation("subAmount", subAmountAggregation).
		AddAggregation("subTime", subTimeAggregation).
		AddAggregation("subHas", subHasAggregation)

	// For each active filter, add an aggregation that computes the property count
	// excluding that filter's own restriction. This ensures active filters always
	// appear in results with correct counts.
	for i, f := range searchSession.Filters {
		if f.ID == nil {
			// This should not be possible.
			continue
		}
		if f.Has != nil && len(f.Prop) == 0 {
			// Top-level has filter: no specific prop, count documents with any simple has claim.
			// Only simple has claims (without sub-claims) are indexed in claims.has.
			activeAgg := esdsl.NewAggregations().
				Filter(searchSession.ToQueryExcluding(*f.ID, enabledLanguages)).
				AddAggregation("count", esdsl.NewAggregations().
					Filter(esdsl.NewNestedQuery(
						esdsl.NewMatchAllQuery(),
					).Path("claims.has")))
			searchService = searchService.AddAggregation(fmt.Sprintf("active_%d", i), activeAgg)
			continue
		}
		if f.Has != nil && len(f.Prop) == 1 {
			// Sub-has filter: aggregate on claims.subHas with parentProp filter.
			activeAgg := esdsl.NewAggregations().
				Filter(searchSession.ToQueryExcluding(*f.ID, enabledLanguages)).
				AddAggregation("count", esdsl.NewAggregations().
					Filter(esdsl.NewNestedQuery(
						esdsl.NewTermQuery("claims.subHas.parentProp", esdsl.NewFieldValue().String(f.Prop[0].String())),
					).Path("claims.subHas")))
			searchService = searchService.AddAggregation(fmt.Sprintf("active_%d", i), activeAgg)
			continue
		}
		if len(f.Prop) == 2 { //nolint:mnd
			// Sub-claim filter (ref, amount, time): aggregate on the matching claims.sub*
			// nested path with parentProp + prop filter.
			var subPath string
			switch {
			case f.Ref != nil:
				subPath = "claims.subRef"
			case f.Amount != nil:
				subPath = "claims.subAmount"
			case f.Time != nil:
				subPath = "claims.subTime"
			default:
				// This should not be possible.
				continue
			}
			activeAgg := esdsl.NewAggregations().
				Filter(searchSession.ToQueryExcluding(*f.ID, enabledLanguages)).
				AddAggregation("count", esdsl.NewAggregations().
					Filter(esdsl.NewNestedQuery(
						esdsl.NewBoolQuery().Must(
							esdsl.NewTermQuery(subPath+".parentProp", esdsl.NewFieldValue().String(f.Prop[0].String())),
							esdsl.NewTermQuery(subPath+".prop", esdsl.NewFieldValue().String(f.Prop[1].String())),
						),
					).Path(subPath)))
			searchService = searchService.AddAggregation(fmt.Sprintf("active_%d", i), activeAgg)
			continue
		}
		prop := f.Prop[0]
		var nestedPath string
		switch {
		case f.Ref != nil:
			nestedPath = "claims.ref"
		case f.Amount != nil:
			nestedPath = "claims.amount"
		case f.Time != nil:
			nestedPath = "claims.time"
		default:
			// This should not be possible.
			continue
		}
		activeAgg := esdsl.NewAggregations().
			Filter(searchSession.ToQueryExcluding(*f.ID, enabledLanguages)).
			AddAggregation("nested", esdsl.NewAggregations().
				Nested(esdsl.NewNestedAggregation().Path(nestedPath)).
				AddAggregation("filter", esdsl.NewAggregations().
					Filter(esdsl.NewTermQuery(nestedPath+".prop", esdsl.NewFieldValue().String(prop.String()))).
					AddAggregation("docs", esdsl.NewAggregations().
						ReverseNested(esdsl.NewReverseNestedAggregation()))))
		searchService = searchService.AddAggregation(fmt.Sprintf("active_%d", i), activeAgg)
	}

	m := metrics.Duration(internalStore.MetricElasticSearch).Start()
	res, err := searchService.Do(ctx)
	m.Stop()
	if err != nil {
		return nil, nil, WithESError(err)
	}
	metrics.Duration(internalStore.MetricElasticSearchInternal).Duration = time.Duration(res.Took) * time.Millisecond

	// Parse ref aggregation.
	refNested, errE := internalSearch.AggAs[types.NestedAggregate](res.Aggregations, "ref")
	if errE != nil {
		return nil, nil, errE
	}
	refScoped, errE := internalSearch.AggAs[types.FilterAggregate](refNested.Aggregations, "scoped")
	if errE != nil {
		return nil, nil, errE
	}
	refTerms, errE := internalSearch.AggAs[types.StringTermsAggregate](refScoped.Aggregations, "props")
	if errE != nil {
		return nil, nil, errE
	}
	refBuckets, ok := refTerms.Buckets.([]types.StringTermsBucket)
	if !ok {
		errE := errors.New("unexpected bucket type for ref")
		errors.Details(errE)["type"] = fmt.Sprintf("%T", refTerms.Buckets)
		return nil, nil, errE
	}
	refTotal, errE := internalSearch.AggAs[types.CardinalityAggregate](refScoped.Aggregations, "total")
	if errE != nil {
		return nil, nil, errE
	}

	// Parse amount aggregation.
	amountNested, errE := internalSearch.AggAs[types.NestedAggregate](res.Aggregations, "amount")
	if errE != nil {
		return nil, nil, errE
	}
	amountTerms, errE := internalSearch.AggAs[types.MultiTermsAggregate](amountNested.Aggregations, "props")
	if errE != nil {
		return nil, nil, errE
	}
	amountBuckets, ok := amountTerms.Buckets.([]types.MultiTermsBucket)
	if !ok {
		errE := errors.New("unexpected bucket type for amount")
		errors.Details(errE)["type"] = fmt.Sprintf("%T", amountTerms.Buckets)
		return nil, nil, errE
	}
	amountTotal, errE := internalSearch.AggAs[types.CardinalityAggregate](amountNested.Aggregations, "total")
	if errE != nil {
		return nil, nil, errE
	}

	// Parse time aggregation.
	timeNested, errE := internalSearch.AggAs[types.NestedAggregate](res.Aggregations, "time")
	if errE != nil {
		return nil, nil, errE
	}
	timeTerms, errE := internalSearch.AggAs[types.StringTermsAggregate](timeNested.Aggregations, "props")
	if errE != nil {
		return nil, nil, errE
	}
	timeBuckets, ok := timeTerms.Buckets.([]types.StringTermsBucket)
	if !ok {
		errE := errors.New("unexpected bucket type for time")
		errors.Details(errE)["type"] = fmt.Sprintf("%T", timeTerms.Buckets)
		return nil, nil, errE
	}
	timeTotal, errE := internalSearch.AggAs[types.CardinalityAggregate](timeNested.Aggregations, "total")
	if errE != nil {
		return nil, nil, errE
	}

	// Parse has aggregation. hasDocCount is the documents with any has claim (the has facet's reported count
	// and its contribution to the available-filters total); hasMatchedCount is those whose has-property
	// matches the value query (only decides whether the facet is returned).
	hasFilterAgg, errE := internalSearch.AggAs[types.FilterAggregate](res.Aggregations, "has")
	if errE != nil {
		return nil, nil, errE
	}
	hasDocCount := hasFilterAgg.DocCount
	hasMatchedAgg, errE := internalSearch.AggAs[types.FilterAggregate](hasFilterAgg.Aggregations, "matched")
	if errE != nil {
		return nil, nil, errE
	}
	hasMatchedCount := hasMatchedAgg.DocCount

	// Parse subRef aggregation.
	subRefNested, errE := internalSearch.AggAs[types.NestedAggregate](res.Aggregations, "subRef")
	if errE != nil {
		return nil, nil, errE
	}
	subRefScoped, errE := internalSearch.AggAs[types.FilterAggregate](subRefNested.Aggregations, "scoped")
	if errE != nil {
		return nil, nil, errE
	}
	subRefTerms, errE := internalSearch.AggAs[types.MultiTermsAggregate](subRefScoped.Aggregations, "props")
	if errE != nil {
		return nil, nil, errE
	}
	subRefBuckets, ok := subRefTerms.Buckets.([]types.MultiTermsBucket)
	if !ok {
		errE := errors.New("unexpected bucket type for subRef")
		errors.Details(errE)["type"] = fmt.Sprintf("%T", subRefTerms.Buckets)
		return nil, nil, errE
	}
	subRefTotal, errE := internalSearch.AggAs[types.CardinalityAggregate](subRefScoped.Aggregations, "total")
	if errE != nil {
		return nil, nil, errE
	}

	// Parse subAmount aggregation.
	subAmountNested, errE := internalSearch.AggAs[types.NestedAggregate](res.Aggregations, "subAmount")
	if errE != nil {
		return nil, nil, errE
	}
	subAmountTerms, errE := internalSearch.AggAs[types.MultiTermsAggregate](subAmountNested.Aggregations, "props")
	if errE != nil {
		return nil, nil, errE
	}
	subAmountBuckets, ok := subAmountTerms.Buckets.([]types.MultiTermsBucket)
	if !ok {
		errE := errors.New("unexpected bucket type for subAmount")
		errors.Details(errE)["type"] = fmt.Sprintf("%T", subAmountTerms.Buckets)
		return nil, nil, errE
	}
	subAmountTotal, errE := internalSearch.AggAs[types.CardinalityAggregate](subAmountNested.Aggregations, "total")
	if errE != nil {
		return nil, nil, errE
	}

	// Parse subTime aggregation.
	subTimeNested, errE := internalSearch.AggAs[types.NestedAggregate](res.Aggregations, "subTime")
	if errE != nil {
		return nil, nil, errE
	}
	subTimeTerms, errE := internalSearch.AggAs[types.MultiTermsAggregate](subTimeNested.Aggregations, "props")
	if errE != nil {
		return nil, nil, errE
	}
	subTimeBuckets, ok := subTimeTerms.Buckets.([]types.MultiTermsBucket)
	if !ok {
		errE := errors.New("unexpected bucket type for subTime")
		errors.Details(errE)["type"] = fmt.Sprintf("%T", subTimeTerms.Buckets)
		return nil, nil, errE
	}
	subTimeTotal, errE := internalSearch.AggAs[types.CardinalityAggregate](subTimeNested.Aggregations, "total")
	if errE != nil {
		return nil, nil, errE
	}

	// Parse subHas aggregation.
	subHasNested, errE := internalSearch.AggAs[types.NestedAggregate](res.Aggregations, "subHas")
	if errE != nil {
		return nil, nil, errE
	}
	subHasTerms, errE := internalSearch.AggAs[types.StringTermsAggregate](subHasNested.Aggregations, "props")
	if errE != nil {
		return nil, nil, errE
	}
	subHasBuckets, ok := subHasTerms.Buckets.([]types.StringTermsBucket)
	if !ok {
		errE := errors.New("unexpected bucket type for subHas")
		errors.Details(errE)["type"] = fmt.Sprintf("%T", subHasTerms.Buckets)
		return nil, nil, errE
	}
	subHasTotal, errE := internalSearch.AggAs[types.CardinalityAggregate](subHasNested.Aggregations, "total")
	if errE != nil {
		return nil, nil, errE
	}

	valueQueryActive := valueQuery != ""

	refResults, errE := parseStringTermsBuckets(refBuckets, "ref", valueQueryActive)
	if errE != nil {
		return nil, nil, errE
	}
	amountResults, errE := parseMultiTermsBuckets(amountBuckets, valueQueryActive)
	if errE != nil {
		return nil, nil, errE
	}
	timeResults, errE := parseStringTermsBuckets(timeBuckets, "time", valueQueryActive)
	if errE != nil {
		return nil, nil, errE
	}

	// Parse subRef multi-terms buckets into FilterResult entries with 2-element Props.
	subRefResults, errE := parseSubClaimBuckets(subRefBuckets, "ref", "subRef", false, valueQueryActive)
	if errE != nil {
		return nil, nil, errE
	}
	// Parse subAmount multi-terms buckets (parentProp, prop, unit).
	subAmountResults, errE := parseSubClaimBuckets(subAmountBuckets, "amount", "subAmount", true, valueQueryActive)
	if errE != nil {
		return nil, nil, errE
	}
	// Parse subTime multi-terms buckets (parentProp, prop).
	subTimeResults, errE := parseSubClaimBuckets(subTimeBuckets, "time", "subTime", false, valueQueryActive)
	if errE != nil {
		return nil, nil, errE
	}
	// Parse subHas single-terms buckets (parentProp only). The user later selects
	// which has-properties to match via HasFilter.Props.
	subHasResults, errE := parseStringTermsBuckets(subHasBuckets, "has", valueQueryActive)
	if errE != nil {
		return nil, nil, errE
	}

	results := make([]FilterResult, 0, len(refResults)+len(amountResults)+len(timeResults)+
		len(subRefResults)+len(subAmountResults)+len(subTimeResults)+len(subHasResults)+1)
	results = append(results, refResults...)
	results = append(results, amountResults...)
	results = append(results, timeResults...)
	results = append(results, subRefResults...)
	results = append(results, subAmountResults...)
	results = append(results, subTimeResults...)
	results = append(results, subHasResults...)

	// Add the has filter result when a has-property matches the value query (any has claim when there is no
	// query). Its count is the full document count (documents with any has claim), unchanged as the box is
	// typed in, matching how the other facets keep their full count under a query.
	if hasMatchedCount > 0 {
		results = append(results, FilterResult{
			Props:    nil,
			Type:     "has",
			Unit:     "",
			FilterID: "",
			Count:    hasDocCount,
		})
	}

	// Parse per-active-filter aggregation results and append them with FilterID set.
	// Main results (without FilterID) remain as inactive filter options.
	for i, f := range searchSession.Filters {
		if f.ID == nil {
			// This should not be possible.
			continue
		}

		props := make([]string, 0, len(f.Prop))
		for _, p := range f.Prop {
			props = append(props, p.String())
		}
		var result FilterResult
		switch {
		case f.Has != nil:
			result = FilterResult{Props: props, Type: "has", Unit: "", FilterID: f.ID.String(), Count: 0}
			if len(props) == 0 {
				result.Props = nil
			}
		case f.Ref != nil:
			result = FilterResult{Props: props, Type: "ref", Unit: "", FilterID: f.ID.String(), Count: 0}
		case f.Amount != nil:
			result = FilterResult{Props: props, Type: "amount", Unit: "", FilterID: f.ID.String(), Count: 0}
			if f.Amount.Unit != nil {
				result.Unit = f.Amount.Unit.String()
			}
		case f.Time != nil:
			result = FilterResult{Props: props, Type: "time", Unit: "", FilterID: f.ID.String(), Count: 0}
		default:
			// This should not be possible.
			continue
		}

		aggName := fmt.Sprintf("active_%d", i)
		activeFilter, errE := internalSearch.AggAs[types.FilterAggregate](res.Aggregations, aggName)
		if errE != nil {
			return nil, nil, errE
		}

		// Sub-claim filters and the top-level has filter use a "count" sub-aggregation.
		// Top-level ref/amount/time use the nested.filter.docs structure.
		useCount := f.Has != nil || len(f.Prop) == 2 //nolint:mnd
		if useCount {
			countFilter, errE := internalSearch.AggAs[types.FilterAggregate](activeFilter.Aggregations, "count")
			if errE != nil {
				return nil, nil, errE
			}
			result.Count = countFilter.DocCount
		} else {
			activeNested, errE := internalSearch.AggAs[types.NestedAggregate](activeFilter.Aggregations, "nested")
			if errE != nil {
				return nil, nil, errE
			}
			propFilter, errE := internalSearch.AggAs[types.FilterAggregate](activeNested.Aggregations, "filter")
			if errE != nil {
				return nil, nil, errE
			}
			activeDocs, errE := internalSearch.AggAs[types.ReverseNestedAggregate](propFilter.Aggregations, "docs")
			if errE != nil {
				return nil, nil, errE
			}
			result.Count = activeDocs.DocCount
		}

		results = append(results, result)
	}

	// Sort: active filters first, then inactive, each group by count descending.
	slices.SortStableFunc(results, func(a FilterResult, b FilterResult) int {
		aActive := a.FilterID != ""
		bActive := b.FilterID != ""
		if aActive != bActive {
			if aActive {
				return -1
			}
			return 1
		}
		return cmp.Compare(b.Count, a.Count)
	})
	if len(results) > MaxResultsCount {
		results = results[:MaxResultsCount]
	}

	// The available-filters total is the distinct facet count per type, taken from the cardinality
	// aggregations which are computed outside the value-query filter, so the total reflects the search and is
	// not narrowed as the filter-pane search box is typed in. The cardinality is exact for the facet counts
	// in play (well under the precision threshold). The per-type bucket lists are the facets actually returned
	// (the value query decides which are kept, never their counts), and are not used for the total.
	refTotalValue := refTotal.Value
	amountTotalValue := amountTotal.Value
	timeTotalValue := timeTotal.Value
	// The top-level has facet is a single "has property" filter for all simple has claims, so it contributes
	// at most 1 to the total, counted whenever any document has such a claim, regardless of the value query.
	// Nested has claims are not counted here: they are the subHas facets, counted below as one filter per
	// parent property (subHasTotalValue).
	var hasTotalValue int64
	if hasDocCount > 0 {
		hasTotalValue = 1
	}
	subRefTotalValue := subRefTotal.Value
	subAmountTotalValue := subAmountTotal.Value
	subTimeTotalValue := subTimeTotal.Value
	subHasTotalValue := subHasTotal.Value
	total := strconv.FormatInt(
		refTotalValue+amountTotalValue+timeTotalValue+hasTotalValue+
			subRefTotalValue+subAmountTotalValue+subTimeTotalValue+subHasTotalValue,
		10,
	)

	return results, map[string]any{
		"total": total,
	}, nil
}
