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

// Get retrieves reference filter data for search results.
func (f *RefFilter) Get(
	ctx context.Context, getSearchService func() *esSearch.Search,
	query types.QueryVariant, prop identifier.Identifier,
) ([]RefFilterResult, map[string]any, errors.E) {
	metrics, _ := waf.GetMetrics(ctx)

	searchService := getSearchService()

	// Aggregation for documents that have the property: terms on claims.ref.to.
	// The "paths" sub-aggregation extracts the indexed hierarchy paths for each
	// value so the frontend can render filter results as a tree. Within a single
	// claims.ref.to bucket all nested ref records share the same toPath array
	// (it is computed from the target value, not the source doc), so a terms
	// aggregation on claims.ref.toPath effectively returns that value's path set.
	// Size 100 caps the distinct path strings per filter value, which only matters
	// for diamond or multi-hierarchy values.
	refAggregation := esdsl.NewAggregations().
		Nested(esdsl.NewNestedAggregation().Path("claims.ref")).
		AddAggregation("filter", esdsl.NewAggregations().
			Filter(esdsl.NewTermQuery("claims.ref.prop", esdsl.NewFieldValue().String(prop.String()))).
			AddAggregation("props", esdsl.NewAggregations().
				Terms(esdsl.NewTermsAggregation().Field("claims.ref.to").Size(MaxResultsCount).
					Order(esdsl.NewAggregateOrder().Map(map[string]sortorder.SortOrder{"docs": sortorder.Desc}))).
				AddAggregation("docs", esdsl.NewAggregations().
					ReverseNested(esdsl.NewReverseNestedAggregation())).
				AddAggregation("paths", esdsl.NewAggregations().
					Terms(esdsl.NewTermsAggregation().Field("claims.ref.toPath").Size(100)))). //nolint:mnd
			AddAggregation("total", esdsl.NewAggregations().
				Cardinality(esdsl.NewCardinalityAggregation().Field("claims.ref.to").PrecisionThreshold(maxPrecisionThreshold))))

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

	// Include the missing bucket if there are documents without this property.
	if missingCount > 0 {
		results = append(results, RefFilterResult{ID: MissingRefFilterID, Count: missingCount, Paths: nil})
	}

	// Order for hierarchical tree rendering on the frontend.
	// This also puts missing in the right position.
	slices.SortStableFunc(results, compareRefFilterResults)

	refTotalValue := distinctValuesTotal(len(refBuckets), refTotal.Value)
	// Include missing in the total if present.
	if missingCount > 0 {
		refTotalValue++
	}
	total := strconv.FormatInt(refTotalValue, 10)

	return results, map[string]any{
		"total": total,
	}, nil
}

// DescendantValues returns value together with every value that has it as an ancestor in
// the indexed hierarchy of the given reference property (for a class property, value plus
// its subclasses). Membership is read from the same indexed hierarchy paths the reference
// filter facet renders: a value is a descendant when value appears as an ancestor in one
// of its paths. A leaf value, or a value whose property has no hierarchy, expands to just
// itself.
//
// Selecting a value in a reference filter conceptually covers its whole subtree (ancestor
// values are indexed, so the value already matches every descendant document). This is
// used to expand a parent selection into the explicit set of values the filters UI would
// have selected through its cascade. The requested value is always first in the result.
func DescendantValues(
	ctx context.Context, getSearchService func() *esSearch.Search,
	prop, value identifier.Identifier,
) ([]identifier.Identifier, errors.E) {
	f := RefFilter{To: []ToValue{{ID: value}}, Missing: false}
	results, _, errE := f.Get(ctx, getSearchService, f.ToQuery(prop), prop)
	if errE != nil {
		return nil, errE
	}
	out := []identifier.Identifier{value}
	valueStr := value.String()
	for _, r := range results {
		if r.ID == MissingRefFilterID {
			continue
		}
		// Each path is r's own ancestor chain (root to its immediate parent; r itself is not
		// included). r is a descendant of value exactly when value appears as one of those ancestors.
		isDescendant := false
		for _, path := range r.Paths {
			if slices.Contains(path, valueStr) {
				isDescendant = true
				break
			}
		}
		if !isDescendant {
			continue
		}
		id, errE := identifier.MaybeString(r.ID)
		if errE != nil {
			return nil, errE
		}
		out = append(out, id)
	}
	return out, nil
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
	if f.Missing && len(f.To) == 0 {
		return missingQuery
	}

	// Build value queries (OR across all To values).
	shoulds := make([]types.QueryVariant, 0, len(f.To)+1)
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
func (f *RefFilter) GetSubRef(
	ctx context.Context, getSearchService func() *esSearch.Search,
	query types.QueryVariant, parentProp, prop identifier.Identifier,
	parentToRestrictions []identifier.Identifier,
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

	// Aggregation for documents that have matching subRef: terms on claims.subRef.to.
	// The "paths" sub-aggregation extracts the indexed hierarchy paths for each
	// value so the frontend can render filter results as a tree. See RefFilter.Get
	// for the rationale and parsing rules.
	subRefAggregation := esdsl.NewAggregations().
		Nested(esdsl.NewNestedAggregation().Path("claims.subRef")).
		AddAggregation("filter", esdsl.NewAggregations().
			Filter(esdsl.NewBoolQuery().Must(filterMusts...)).
			AddAggregation("props", esdsl.NewAggregations().
				Terms(esdsl.NewTermsAggregation().Field("claims.subRef.to").Size(MaxResultsCount).
					Order(esdsl.NewAggregateOrder().Map(map[string]sortorder.SortOrder{"docs": sortorder.Desc}))).
				AddAggregation("docs", esdsl.NewAggregations().
					ReverseNested(esdsl.NewReverseNestedAggregation())).
				AddAggregation("paths", esdsl.NewAggregations().
					Terms(esdsl.NewTermsAggregation().Field("claims.subRef.toPath").Size(100)))). //nolint:mnd
			AddAggregation("total", esdsl.NewAggregations().
				Cardinality(esdsl.NewCardinalityAggregation().Field("claims.subRef.to").PrecisionThreshold(maxPrecisionThreshold))))

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

	// Include the missing bucket if there are documents without this sub-reference.
	if missingCount > 0 {
		results = append(results, RefFilterResult{ID: MissingRefFilterID, Count: missingCount, Paths: nil})
	}

	// Order for hierarchical tree rendering on the frontend.
	// This also puts missing in the right position.
	slices.SortStableFunc(results, compareRefFilterResults)

	subRefTotalValue := distinctValuesTotal(len(subRefBuckets), subRefTotal.Value)
	if missingCount > 0 {
		subRefTotalValue++
	}
	total := strconv.FormatInt(subRefTotalValue, 10)

	return results, map[string]any{
		"total": total,
	}, nil
}
