package search

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
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

// MissingRefFilterID is a special ID used for the "missing" bucket in reference filter results.
// It represents documents that do not have a value for the filtered property.
const MissingRefFilterID = "__MISSING__"

// DirectRefFilterPrefix prefixes the synthetic "direct" value id in reference filter results;
// the suffix is the parent value id. It is appended as a child of a value that has narrower values
// and represents documents that are exactly that value, with none of its narrower values (its
// most-specific/leaf instances).
const DirectRefFilterPrefix = "__DIRECT__:"

// RefFilterResult represents occurrences count for a single reference in a reference filter.
// Paths lists hierarchy chains from root to immediate parent for this value, one entry
// per parent path the value participates in (multiple entries for diamond hierarchies
// or when the value sits in more than one value-hierarchy property). The frontend uses
// these to render filter values as a tree.
type RefFilterResult struct {
	ID    string     `json:"id"`
	Count int64      `json:"count"`
	Paths [][]string `json:"paths,omitempty"`
}

// parseToPath turns one indexed hierarchy path string into its ancestor chain.
// The input format is "<hierarchy_property_id>:<root_id>/<parent_id>/.../<this_id>".
// The hierarchy-property prefix is dropped (the consumer does not care which hierarchy
// the path belongs to), and the trailing segment is dropped (it is the value's own id).
// The returned slice is ordered from root to immediate parent. Returns nil when the
// input has no ":" separator or when the chain contains a single segment (the value
// itself has no ancestors in that hierarchy).
func parseToPath(raw string) []string {
	_, chain, ok := strings.Cut(raw, ":")
	if !ok {
		return nil
	}
	parts := strings.Split(chain, "/")
	if len(parts) <= 1 {
		return nil
	}
	ancestors := make([]string, len(parts)-1)
	copy(ancestors, parts[:len(parts)-1])
	return ancestors
}

// collectPaths extracts all distinct hierarchy paths for a single filter bucket
// from a "paths" terms sub-aggregation on a toPath field. Each input bucket key
// is one raw path string; this function parses each and drops empty results.
func collectPaths(buckets []types.StringTermsBucket) [][]string {
	if len(buckets) == 0 {
		return nil
	}
	out := make([][]string, 0, len(buckets))
	for _, b := range buckets {
		key, ok := b.Key.(string)
		if !ok {
			continue
		}
		ancestors := parseToPath(key)
		if len(ancestors) == 0 {
			continue
		}
		out = append(out, ancestors)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// bucketsToRefFilterResults turns the top-level terms-aggregation buckets of a
// ref or sub-ref filter aggregation into RefFilterResult entries. Each bucket
// is expected to expose a "docs" reverse_nested sub-aggregation for the count
// and a "paths" terms sub-aggregation on the corresponding toPath field. The
// kind label is woven into error messages so an unexpected aggregation shape is
// attributable to either ref or sub-ref handling.
func bucketsToRefFilterResults(buckets []types.StringTermsBucket, kind string) ([]RefFilterResult, errors.E) {
	results := make([]RefFilterResult, 0, len(buckets))
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
		bucketPaths, errE := internalSearch.AggAs[types.StringTermsAggregate](bucket.Aggregations, "paths")
		if errE != nil {
			return nil, errE
		}
		pathBuckets, ok := bucketPaths.Buckets.([]types.StringTermsBucket)
		if !ok {
			errE := errors.New("unexpected bucket type for " + kind + " paths")
			errors.Details(errE)["type"] = fmt.Sprintf("%T", bucketPaths.Buckets)
			return nil, errE
		}
		results = append(results, RefFilterResult{
			ID:    key,
			Count: bucketDocs.DocCount,
			Paths: collectPaths(pathBuckets),
		})
	}
	return results, nil
}

// bucketDirectCount reads a value bucket's "direct" sub-aggregation count: the number of
// documents for which this value is most-specific (it references the value but none of the
// value's narrower values). The sub-aggregation is a filter on claims.ref.isLeaf wrapping a
// reverse_nested "docs" aggregation, so its document count is at the document level.
func bucketDirectCount(bucket types.StringTermsBucket) (int64, errors.E) {
	direct, errE := internalSearch.AggAs[types.FilterAggregate](bucket.Aggregations, "direct")
	if errE != nil {
		return 0, errE
	}
	docs, errE := internalSearch.AggAs[types.ReverseNestedAggregate](direct.Aggregations, "docs")
	if errE != nil {
		return 0, errE
	}
	return docs.DocCount, nil
}

// directPaths builds the hierarchy paths for a value's synthetic "direct" entry so the tree
// builder nests it immediately under the value: each of the value's own paths (root to its immediate
// parent) is extended with the value itself, and a root value (no paths) gets a single path
// containing just the value.
func directPaths(value RefFilterResult) [][]string {
	if len(value.Paths) == 0 {
		return [][]string{{value.ID}}
	}
	out := make([][]string, 0, len(value.Paths))
	for _, path := range value.Paths {
		extended := make([]string, 0, len(path)+1)
		extended = append(extended, path...)
		extended = append(extended, value.ID)
		out = append(out, extended)
	}
	return out
}

// directResults builds the synthetic "direct" child entries for a reference or sub-reference
// filter. buckets and values are parallel (same order, one entry per facet value). A "direct"
// entry is emitted for a value that has narrower values present in this facet (it appears as an
// ancestor in another value's hierarchy paths) and whose most-specific document count is greater
// than zero. The entry is nested under the value (via directPaths) and carries the
// DirectRefFilterPrefix-prefixed value id.
func directResults(buckets []types.StringTermsBucket, values []RefFilterResult) ([]RefFilterResult, errors.E) {
	hasNarrower := make(map[string]bool, len(values))
	for _, value := range values {
		for _, path := range value.Paths {
			for _, ancestor := range path {
				hasNarrower[ancestor] = true
			}
		}
	}
	out := make([]RefFilterResult, 0)
	for i := range buckets {
		value := values[i]
		if !hasNarrower[value.ID] {
			continue
		}
		count, errE := bucketDirectCount(buckets[i])
		if errE != nil {
			return nil, errE
		}
		if count <= 0 {
			continue
		}
		out = append(out, RefFilterResult{
			ID:    DirectRefFilterPrefix + value.ID,
			Count: count,
			Paths: directPaths(value),
		})
	}
	return out, nil
}

// refFilterDepth returns a value's depth in its class hierarchy: the length of
// its longest ancestor chain (root to immediate parent), or 0 for a root value
// or one without indexed paths. The longest chain is what makes a count-tie
// ordering by depth a valid topological order even under multiple inheritance:
// for any ancestor A of a value V, A's longest chain is strictly shorter than
// V's longest chain, so A always sorts before V.
func refFilterDepth(r RefFilterResult) int {
	depth := 0
	for _, path := range r.Paths {
		if len(path) > depth {
			depth = len(path)
		}
	}
	return depth
}

// compareRefFilterResults orders reference filter results for the frontend tree:
// by count descending, then by hierarchy depth ascending. Ancestor counts are
// always greater than or equal to descendant counts (a reference is indexed for
// the target and every ancestor), so the only way a descendant could precede an
// ancestor is a count tie, which the depth tiebreak resolves by placing the
// shallower (ancestor) value first.
func compareRefFilterResults(a, b RefFilterResult) int {
	if c := cmp.Compare(b.Count, a.Count); c != 0 {
		return c
	}
	return cmp.Compare(refFilterDepth(a), refFilterDepth(b))
}

// valueAggregation builds the value-count aggregation shared by the reference and sub-reference filters.
// field is the nested path ("claims.ref" or "claims.subRef") and filterQuery scopes the records counted
// (the property match plus any prefilter toFullPath exclusion). It produces one "to" bucket per value,
// each carrying the document count ("docs"), the most-specific/leaf document count ("direct"), and the
// value's hierarchy paths ("paths"), plus a cardinality "total" of distinct values.
//
// The "paths" sub-aggregation extracts the indexed hierarchy paths for each value so the frontend can
// render filter results as a tree. Within a single <field>.to bucket all nested records share the same
// toPath array (it is computed from the target value, not the source doc), so a terms aggregation on
// <field>.toPath effectively returns that value's path set. Size 100 caps the distinct path strings per
// filter value, which only matters for diamond or multi-hierarchy values.
func valueAggregation(field string, filterQuery types.QueryVariant) types.AggregationsVariant { //nolint:ireturn
	return esdsl.NewAggregations().
		Nested(esdsl.NewNestedAggregation().Path(field)).
		AddAggregation("filter", esdsl.NewAggregations().
			Filter(filterQuery).
			AddAggregation("props", esdsl.NewAggregations().
				Terms(esdsl.NewTermsAggregation().Field(field+".to").Size(MaxResultsCount).
					Order(esdsl.NewAggregateOrder().Map(map[string]sortorder.SortOrder{"docs": sortorder.Desc}))).
				AddAggregation("docs", esdsl.NewAggregations().
					ReverseNested(esdsl.NewReverseNestedAggregation())).
				// "direct" counts the documents for which this value is most-specific (a leaf):
				// they reference the value but none of its narrower values.
				AddAggregation("direct", esdsl.NewAggregations().
					Filter(esdsl.NewTermQuery(field+".isLeaf", esdsl.NewFieldValue().Bool(true))).
					AddAggregation("docs", esdsl.NewAggregations().
						ReverseNested(esdsl.NewReverseNestedAggregation()))).
				AddAggregation("paths", esdsl.NewAggregations().
					Terms(esdsl.NewTermsAggregation().Field(field+".toPath").Size(100)))). //nolint:mnd
			AddAggregation("total", esdsl.NewAggregations().
				Cardinality(esdsl.NewCardinalityAggregation().Field(field+".to").PrecisionThreshold(maxPrecisionThreshold))))
}

// Get retrieves reference filter data for search results.
//
// excludeFullPaths, when non-empty, are claims.subRef.toFullPath values to control values dropped from
// the value aggregation: records derived from a prefilter value so the facet does not re-count the
// prefilter's own value hierarchy.
func (f *RefFilter) Get(
	ctx context.Context, getSearchService func() *esSearch.Search,
	query types.QueryVariant, prop identifier.Identifier, excludeFullPaths []string,
) ([]RefFilterResult, map[string]any, errors.E) {
	metrics, _ := waf.GetMetrics(ctx)

	searchService := getSearchService()

	// The value aggregation is scoped to records for this property. When a prefilter on this property is
	// active, also drop the records derived from its value so ancestor buckets are not re-counted.
	refFilterQuery := esdsl.NewBoolQuery().Must(esdsl.NewTermQuery("claims.ref.prop", esdsl.NewFieldValue().String(prop.String())))
	if len(excludeFullPaths) > 0 {
		refFilterQuery = refFilterQuery.MustNot(toFullPathTermsQuery("claims.ref", excludeFullPaths))
	}

	refAggregation := valueAggregation("claims.ref", refFilterQuery)

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
		return nil, nil, WithESError(err)
	}
	metrics.Duration(internalStore.MetricElasticSearchInternal).Duration = time.Duration(res.Took) * time.Millisecond

	refNested, errE := internalSearch.AggAs[types.NestedAggregate](res.Aggregations, "ref")
	if errE != nil {
		return nil, nil, errE
	}
	refFilter, errE := internalSearch.AggAs[types.FilterAggregate](refNested.Aggregations, "filter")
	if errE != nil {
		return nil, nil, errE
	}
	refTerms, errE := internalSearch.AggAs[types.StringTermsAggregate](refFilter.Aggregations, "props")
	if errE != nil {
		return nil, nil, errE
	}
	refBuckets, ok := refTerms.Buckets.([]types.StringTermsBucket)
	if !ok {
		errE := errors.New("unexpected bucket type for ref")
		errors.Details(errE)["type"] = fmt.Sprintf("%T", refTerms.Buckets)
		return nil, nil, errE
	}
	refTotal, errE := internalSearch.AggAs[types.CardinalityAggregate](refFilter.Aggregations, "total")
	if errE != nil {
		return nil, nil, errE
	}

	// Parse the missing count.
	missingFilter, errE := internalSearch.AggAs[types.FilterAggregate](res.Aggregations, "missing")
	if errE != nil {
		return nil, nil, errE
	}
	missingCount := missingFilter.DocCount

	results, errE := bucketsToRefFilterResults(refBuckets, "ref")
	if errE != nil {
		return nil, nil, errE
	}

	// Append a synthetic "direct" entry under each value that has narrower values present and
	// has documents for which it is most-specific, so the value reads as an exact aggregate of its
	// narrower values plus this entry.
	direct, errE := directResults(refBuckets, results)
	if errE != nil {
		return nil, nil, errE
	}
	results = append(results, direct...)

	// Include the missing bucket if there are documents without this property.
	if missingCount > 0 {
		results = append(results, RefFilterResult{ID: MissingRefFilterID, Count: missingCount, Paths: nil})
	}

	// Order for hierarchical tree rendering on the frontend.
	// This also puts missing and the direct entries in the right positions.
	slices.SortStableFunc(results, compareRefFilterResults)

	refTotalValue := distinctValuesTotal(len(refBuckets), refTotal.Value) + int64(len(direct))
	// Include missing in the total if present.
	if missingCount > 0 {
		refTotalValue++
	}
	total := strconv.FormatInt(refTotalValue, 10)

	return results, map[string]any{
		"total": total,
	}, nil
}

// ToSubRefQuery converts the RefFilter to an ElasticSearch query on claims.subRef
// for a sub-reference filter with parentProp and prop.
//
// parentToRestrictions, when non-empty, restricts the sub-claim match to entries
// whose claims.subRef.parentTo is one of the listed values. This enables cross-
// filter joins: when a session has both a parent-level ref filter (e.g.
// HAS_LOCATION = L1) and a sub-ref filter (e.g. HAS_LOCATION > HAS_ARTIST = A),
// the sub-claim is required to live under one of the same parent values, so the
// result is "A under L1" rather than the looser "A anywhere AND L1 anywhere".
func (f *RefFilter) ToSubRefQuery(parentProp, prop identifier.Identifier, parentToRestrictions []identifier.Identifier) types.QueryVariant { //nolint:ireturn
	// withParentTo appends the parentTo restriction clause (if any) to a slice
	// of must-clauses building a single nested sub-claim match. The clause is
	// "claims.subRef.parentTo is one of the restriction values", joined with the
	// existing parentProp/prop (and optional to) constraints inside the same
	// nested query so the join happens within a single sub-claim record.
	withParentTo := func(must []types.QueryVariant) []types.QueryVariant {
		if len(parentToRestrictions) == 0 {
			return must
		}
		shoulds := make([]types.QueryVariant, 0, len(parentToRestrictions))
		for _, pto := range parentToRestrictions {
			shoulds = append(shoulds, esdsl.NewTermQuery("claims.subRef.parentTo", esdsl.NewFieldValue().String(pto.String())))
		}
		return append(must, esdsl.NewBoolQuery().Should(shoulds...).MinimumShouldMatch(esdsl.NewMinimumShouldMatch().Int(1)))
	}

	missingMust := withParentTo([]types.QueryVariant{
		esdsl.NewTermQuery("claims.subRef.parentProp", esdsl.NewFieldValue().String(parentProp.String())),
		esdsl.NewTermQuery("claims.subRef.prop", esdsl.NewFieldValue().String(prop.String())),
	})
	missingQuery := esdsl.NewBoolQuery().MustNot(
		esdsl.NewNestedQuery(
			esdsl.NewBoolQuery().Must(missingMust...),
		).Path("claims.subRef"),
	)

	// Missing only.
	if f.Missing && len(f.To) == 0 && len(f.Direct) == 0 {
		return missingQuery
	}

	// Build value queries (OR across all To and Direct values).
	shoulds := make([]types.QueryVariant, 0, len(f.To)+len(f.Direct)+1)
	for _, to := range f.To {
		valueMust := withParentTo([]types.QueryVariant{
			esdsl.NewTermQuery("claims.subRef.parentProp", esdsl.NewFieldValue().String(parentProp.String())),
			esdsl.NewTermQuery("claims.subRef.prop", esdsl.NewFieldValue().String(prop.String())),
			esdsl.NewTermQuery("claims.subRef.to", esdsl.NewFieldValue().String(to.ID.String())),
		})
		shoulds = append(shoulds, esdsl.NewNestedQuery(
			esdsl.NewBoolQuery().Must(valueMust...),
		).Path("claims.subRef"))
	}

	// A "direct" value additionally requires isLeaf=true, so it matches only documents for which
	// the value is most-specific (none of its narrower values present).
	for _, to := range f.Direct {
		valueMust := withParentTo([]types.QueryVariant{
			esdsl.NewTermQuery("claims.subRef.parentProp", esdsl.NewFieldValue().String(parentProp.String())),
			esdsl.NewTermQuery("claims.subRef.prop", esdsl.NewFieldValue().String(prop.String())),
			esdsl.NewTermQuery("claims.subRef.to", esdsl.NewFieldValue().String(to.ID.String())),
			esdsl.NewTermQuery("claims.subRef.isLeaf", esdsl.NewFieldValue().Bool(true)),
		})
		shoulds = append(shoulds, esdsl.NewNestedQuery(
			esdsl.NewBoolQuery().Must(valueMust...),
		).Path("claims.subRef"))
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
// It aggregates claims.subRef.to values for a given (parentProp, prop) combination.
// parentToRestrictions optionally restricts results to specific parentTo values (for cross-filtering).
//
// excludeFullPaths, when non-empty, are claims.subRef.toFullPath values to control values dropped from
// the value aggregation: records derived from a prefilter value so the facet does not re-count the
// prefilter's own value hierarchy.
func (f *RefFilter) GetSubRef(
	ctx context.Context, getSearchService func() *esSearch.Search,
	query types.QueryVariant, parentProp, prop identifier.Identifier,
	parentToRestrictions []identifier.Identifier, excludeFullPaths []string,
) ([]RefFilterResult, map[string]any, errors.E) {
	metrics, _ := waf.GetMetrics(ctx)

	searchService := getSearchService()

	// Build the filter for parentProp + prop (+ optional parentTo restriction).
	filterMusts := []types.QueryVariant{
		esdsl.NewTermQuery("claims.subRef.parentProp", esdsl.NewFieldValue().String(parentProp.String())),
		esdsl.NewTermQuery("claims.subRef.prop", esdsl.NewFieldValue().String(prop.String())),
	}
	if len(parentToRestrictions) > 0 {
		parentToShoulds := make([]types.QueryVariant, 0, len(parentToRestrictions))
		for _, pto := range parentToRestrictions {
			parentToShoulds = append(parentToShoulds, esdsl.NewTermQuery("claims.subRef.parentTo", esdsl.NewFieldValue().String(pto.String())))
		}
		filterMusts = append(filterMusts, esdsl.NewBoolQuery().Should(parentToShoulds...).MinimumShouldMatch(esdsl.NewMinimumShouldMatch().Int(1)))
	}
	// When a prefilter on this (parentProp, prop) is active, drop the records derived from its value so
	// ancestor buckets are not re-counted.
	subRefFilterQuery := esdsl.NewBoolQuery().Must(filterMusts...)
	if len(excludeFullPaths) > 0 {
		subRefFilterQuery = subRefFilterQuery.MustNot(toFullPathTermsQuery("claims.subRef", excludeFullPaths))
	}

	subRefAggregation := valueAggregation("claims.subRef", subRefFilterQuery)

	// Aggregation for documents missing this sub-reference.
	missingAggregation := esdsl.NewAggregations().
		Filter(esdsl.NewBoolQuery().MustNot(
			esdsl.NewNestedQuery(
				esdsl.NewBoolQuery().Must(
					esdsl.NewTermQuery("claims.subRef.parentProp", esdsl.NewFieldValue().String(parentProp.String())),
					esdsl.NewTermQuery("claims.subRef.prop", esdsl.NewFieldValue().String(prop.String())),
				),
			).Path("claims.subRef"),
		))

	searchService = searchService.Size(0).Query(query).
		AddAggregation("subRef", subRefAggregation).
		AddAggregation("missing", missingAggregation)

	m := metrics.Duration(internalStore.MetricElasticSearch).Start()
	res, err := searchService.Do(ctx)
	m.Stop()
	if err != nil {
		return nil, nil, WithESError(err)
	}
	metrics.Duration(internalStore.MetricElasticSearchInternal).Duration = time.Duration(res.Took) * time.Millisecond

	subRefNested, errE := internalSearch.AggAs[types.NestedAggregate](res.Aggregations, "subRef")
	if errE != nil {
		return nil, nil, errE
	}
	subRefFilter, errE := internalSearch.AggAs[types.FilterAggregate](subRefNested.Aggregations, "filter")
	if errE != nil {
		return nil, nil, errE
	}
	subRefTerms, errE := internalSearch.AggAs[types.StringTermsAggregate](subRefFilter.Aggregations, "props")
	if errE != nil {
		return nil, nil, errE
	}
	subRefBuckets, ok := subRefTerms.Buckets.([]types.StringTermsBucket)
	if !ok {
		errE := errors.New("unexpected bucket type for subRef")
		errors.Details(errE)["type"] = fmt.Sprintf("%T", subRefTerms.Buckets)
		return nil, nil, errE
	}
	subRefTotal, errE := internalSearch.AggAs[types.CardinalityAggregate](subRefFilter.Aggregations, "total")
	if errE != nil {
		return nil, nil, errE
	}

	// Parse the missing count.
	missingFilter, errE := internalSearch.AggAs[types.FilterAggregate](res.Aggregations, "missing")
	if errE != nil {
		return nil, nil, errE
	}
	missingCount := missingFilter.DocCount

	results, errE := bucketsToRefFilterResults(subRefBuckets, "subRef")
	if errE != nil {
		return nil, nil, errE
	}

	// Append a synthetic "direct" entry under each value that has narrower values present and
	// has documents for which it is most-specific, so the value reads as an exact aggregate of its
	// narrower values plus this entry.
	direct, errE := directResults(subRefBuckets, results)
	if errE != nil {
		return nil, nil, errE
	}
	results = append(results, direct...)

	// Include the missing bucket if there are documents without this sub-reference.
	if missingCount > 0 {
		results = append(results, RefFilterResult{ID: MissingRefFilterID, Count: missingCount, Paths: nil})
	}

	// Order for hierarchical tree rendering on the frontend.
	// This also puts missing and the direct entries in the right positions.
	slices.SortStableFunc(results, compareRefFilterResults)

	subRefTotalValue := distinctValuesTotal(len(subRefBuckets), subRefTotal.Value) + int64(len(direct))
	if missingCount > 0 {
		subRefTotalValue++
	}
	total := strconv.FormatInt(subRefTotalValue, 10)

	return results, map[string]any{
		"total": total,
	}, nil
}
