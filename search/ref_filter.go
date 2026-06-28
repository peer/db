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

// MissingValueID is the synthetic API ID for the "missing" bucket: documents that have no claim at all for
// a property (the property is absent, which is distinct from an explicit none or unknown claim). It labels
// the missing entry in reference filter results and the missing group in grouped search results; the
// frontend special-cases this ID and renders it with the common.values.missing label.
const MissingValueID = "__MISSING__"

// DirectRefFilterPrefix prefixes the synthetic "direct" value id in reference filter results;
// the suffix is the parent value id. It is appended as a child of a value that has narrower values
// and represents documents that are exactly that value, with none of its narrower values (its
// most-specific/leaf instances).
const DirectRefFilterPrefix = "__DIRECT__:"

// maxHierarchyPathsPerValue caps the distinct toPath strings a terms aggregation returns per reference filter
// value. A value in a single hierarchy has one path; this bound only matters for diamond or multi-hierarchy
// values (a value reachable through more than one path).
const maxHierarchyPathsPerValue = 100

// HierarchyPathsResolver resolves a value's indexed hierarchy path strings ("<hierProp>:<root>/.../<self>"),
// the same form fullToPathChain parses. The handler injects it (backed by the cached Converter via
// Service.documentFullPaths) so an active reference filter can resolve a selected value's ancestors at query
// time, without an Elasticsearch aggregation, and thus know the augment ids up front.
type HierarchyPathsResolver = func(ctx context.Context, id identifier.Identifier) ([]string, errors.E)

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
	chain := fullToPathChain(raw)
	if len(chain) <= 1 {
		return nil
	}
	return chain[:len(chain)-1]
}

// fullToPathChain turns one indexed hierarchy path string into its full chain, ordered from root to the
// value itself (the trailing segment, kept). The input format is "<hierarchy_property_id>:<root_id>/.../<this_id>";
// only the hierarchy-property prefix is dropped. Unlike parseToPath (which drops the trailing own-id segment)
// this keeps it, so the chain can be split into a value and each of its ancestors. Returns nil when the input
// has no ":" separator.
func fullToPathChain(raw string) []string {
	_, chain, ok := strings.Cut(raw, ":")
	if !ok {
		return nil
	}
	return strings.Split(chain, "/")
}

// collectPaths extracts the distinct hierarchy ancestor chains for a single filter value from its "paths"
// terms sub-aggregation on a toPath field. Each bucket key is one raw path string; it adapts those to the
// ancestorChains primitive.
func collectPaths(buckets []types.StringTermsBucket) [][]string {
	raw := make([]string, 0, len(buckets))
	for _, b := range buckets {
		if key, ok := b.Key.(string); ok {
			raw = append(raw, key)
		}
	}
	return ancestorChains(raw)
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
// <field>.toPath effectively returns that value's path set, capped at maxHierarchyPathsPerValue.
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
					Terms(esdsl.NewTermsAggregation().Field(field+".toPath").Size(maxHierarchyPathsPerValue)))).
			AddAggregation("total", esdsl.NewAggregations().
				Cardinality(esdsl.NewCardinalityAggregation().Field(field+".to").PrecisionThreshold(maxPrecisionThreshold))))
}

// toTermsQuery matches reference records on the given nested field ("claims.ref" or "claims.subRef") whose
// to value is one of ids.
func toTermsQuery(field string, ids []identifier.Identifier) types.QueryVariant { //nolint:ireturn
	values := make([]types.FieldValueVariant, len(ids))
	for i, id := range ids {
		values[i] = esdsl.NewFieldValue().String(id.String())
	}
	return esdsl.NewTermsQuery().AddTermsQuery(field+".to", esdsl.NewTermsQueryField().FieldValues(values...))
}

// selectedRefIDs returns the explicitly selected reference value ids (the union of To and Direct, deduplicated
// and order-preserving) both as identifiers (for the aggregation filter) and as strings (for the merge step).
func selectedRefIDs(f *RefFilter) ([]identifier.Identifier, []string) {
	seen := make(map[identifier.Identifier]bool, len(f.To)+len(f.Direct))
	idents := make([]identifier.Identifier, 0, len(f.To)+len(f.Direct))
	ids := make([]string, 0, len(f.To)+len(f.Direct))
	for _, values := range [][]ToValue{f.To, f.Direct} {
		for _, v := range values {
			if seen[v.ID] {
				continue
			}
			seen[v.ID] = true
			idents = append(idents, v.ID)
			ids = append(ids, v.ID.String())
		}
	}
	return idents, ids
}

// selectedMatchAggregation label-matches a fixed augment id set (an active filter's selected values plus, for
// references, their ancestors, all resolved up front) against the value-search query, so augmented values,
// which have zero documents in the current search scope, can still be narrowed by the SAME Elasticsearch
// matcher real values use. It is a global aggregation (escaping the search query) scoped by filterQuery (the
// prop/parentProp match, a terms query restricting to the augment ids, and the value-search labelMatchQuery),
// bucketed on field+"."+termField ("to" for references, "prop" for has). Only the matched augment ids come
// back. filterQuery deliberately omits the prefilter exclusion and (for sub-references) the parentTo
// restriction, so a checked value is never hidden.
func selectedMatchAggregation(field, termField string, filterQuery types.QueryVariant) types.AggregationsVariant { //nolint:ireturn
	return esdsl.NewAggregations().
		Global(esdsl.NewGlobalAggregation()).
		AddAggregation("nested", esdsl.NewAggregations().
			Nested(esdsl.NewNestedAggregation().Path(field)).
			AddAggregation("filter", esdsl.NewAggregations().
				Filter(filterQuery).
				AddAggregation("match", esdsl.NewAggregations().
					Terms(esdsl.NewTermsAggregation().Field(field+"."+termField).Size(MaxResultsCount)))))
}

// parseSelectedMatchIDs unwraps the selectedMatch aggregation (global -> nested -> filter -> match) into the
// set of augment ids whose label matched the value-search query.
func parseSelectedMatchIDs(globalAgg *types.GlobalAggregate) (map[string]bool, errors.E) {
	nested, errE := internalSearch.AggAs[types.NestedAggregate](globalAgg.Aggregations, "nested")
	if errE != nil {
		return nil, errE
	}
	filter, errE := internalSearch.AggAs[types.FilterAggregate](nested.Aggregations, "filter")
	if errE != nil {
		return nil, errE
	}
	matchTerms, errE := internalSearch.AggAs[types.StringTermsAggregate](filter.Aggregations, "match")
	if errE != nil {
		return nil, errE
	}
	buckets, ok := matchTerms.Buckets.([]types.StringTermsBucket)
	if !ok {
		errE := errors.New("unexpected bucket type for selected match")
		errors.Details(errE)["type"] = fmt.Sprintf("%T", matchTerms.Buckets)
		return nil, errE
	}
	matched := make(map[string]bool, len(buckets))
	for _, bucket := range buckets {
		if key, ok := bucket.Key.(string); ok {
			matched[key] = true
		}
	}
	return matched, nil
}

// selectedPathAccumulator collects, per value id, the deduplicated set of ancestor chains (root to that id's
// immediate parent) discovered while walking hierarchy path chains, so a value and every ancestor in a chain
// is recorded. Its finalize step turns each id's set into a sorted slice of paths.
type selectedPathAccumulator struct {
	acc map[string]map[string][]string
}

func newSelectedPathAccumulator() *selectedPathAccumulator {
	return &selectedPathAccumulator{acc: map[string]map[string][]string{}}
}

// ensure records an id with no paths yet, so a value with no indexed hierarchy still appears (rendered flat).
func (a *selectedPathAccumulator) ensure(id string) {
	if _, ok := a.acc[id]; !ok {
		a.acc[id] = map[string][]string{}
	}
}

// addChain records, for a single root-to-self chain, the value AND every ancestor in it: for a chain
// [a,b,c,d] (self d) it records d with path [a,b,c], c with [a,b], b with [a], and a as a root (no path).
func (a *selectedPathAccumulator) addChain(chain []string) {
	for i, id := range chain {
		a.ensure(id)
		if i == 0 {
			// Root of this chain, no ancestors.
			continue
		}
		prefix := make([]string, i)
		copy(prefix, chain[:i])
		a.acc[id][strings.Join(prefix, "/")] = prefix
	}
}

// finalize turns the accumulated per-id chain sets into the augment map of id to its deduplicated, sorted
// hierarchy paths (root to immediate parent); an id with no ancestors maps to nil paths.
func (a *selectedPathAccumulator) finalize() map[string][][]string {
	out := make(map[string][][]string, len(a.acc))
	for id, set := range a.acc {
		if len(set) == 0 {
			out[id] = nil
			continue
		}
		paths := make([][]string, 0, len(set))
		for _, p := range set {
			paths = append(paths, p)
		}
		slices.SortFunc(paths, slices.Compare)
		out[id] = paths
	}
	return out
}

// resolveSelectedAugment resolves the augment value set for an active reference (or sub-reference) filter:
// each explicitly selected value plus every ancestor of it, mapped to its deduplicated hierarchy paths (root
// to immediate parent). For each selected value it calls the injected resolver for that value's indexed
// hierarchy path strings ("<hierProp>:<root>/.../<self>", the same form fullToPathChain parses) and
// accumulates them, so a selected value with no indexed hierarchy is still present (rendered flat). The map
// keys are exactly the ids that must be present in the value list for the selection (and its ancestor tree)
// to render. It returns nil when there is no resolver or no selection.
func resolveSelectedAugment(ctx context.Context, resolver HierarchyPathsResolver, selectedIDs []identifier.Identifier) (map[string][][]string, errors.E) {
	if resolver == nil || len(selectedIDs) == 0 {
		return nil, nil //nolint:nilnil
	}
	acc := newSelectedPathAccumulator()
	for _, sel := range selectedIDs {
		acc.ensure(sel.String())
		paths, errE := resolver(ctx, sel)
		if errE != nil {
			return nil, errE
		}
		for _, raw := range paths {
			chain := fullToPathChain(raw)
			if len(chain) == 0 {
				continue
			}
			acc.addChain(chain)
		}
	}
	return acc.finalize(), nil
}

// augmentIdentifiers converts an augment map's keys (value and ancestor id strings) to identifiers, skipping
// any that fail to parse (none are expected to, since they originate as valid identifier strings). They scope
// the selectedMatch aggregation's terms query.
func augmentIdentifiers(augment map[string][][]string) []identifier.Identifier {
	out := make([]identifier.Identifier, 0, len(augment))
	for id := range augment {
		ident, errE := identifier.MaybeString(id)
		if errE != nil {
			continue
		}
		out = append(out, ident)
	}
	return out
}

// addRefSelectedMatchAggregation adds, during a value search, the global selectedMatch aggregation that
// label-matches the augment id set for a reference (field "claims.ref") or sub-reference (field
// "claims.subRef") filter, so the active filter's selected values and their ancestors stay searchable even
// with zero documents in the search scope. propMusts identify the facet (the prop, and for sub-references the
// parentProp, term queries); valueLabelMatch is the value-search matcher. It is a no-op when the augment has
// no ids.
func addRefSelectedMatchAggregation(
	searchService *esSearch.Search, field string, propMusts []types.QueryVariant, valueLabelMatch types.QueryVariant, augment map[string][][]string,
) *esSearch.Search {
	augmentIdents := augmentIdentifiers(augment)
	if len(augmentIdents) == 0 {
		return searchService
	}
	musts := append(slices.Clone(propMusts), toTermsQuery(field, augmentIdents), valueLabelMatch)
	return searchService.AddAggregation("selectedMatch", selectedMatchAggregation(field, "to", esdsl.NewBoolQuery().Must(musts...)))
}

// unionPaths returns the distinct union of two hierarchy-path sets, keeping existing entries first.
func unionPaths(existing, extra [][]string) [][]string {
	if len(extra) == 0 {
		return existing
	}
	seen := make(map[string]bool, len(existing)+len(extra))
	out := make([][]string, 0, len(existing)+len(extra))
	for _, paths := range [][][]string{existing, extra} {
		for _, p := range paths {
			key := strings.Join(p, "/")
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, p)
		}
	}
	return out
}

// mergeSelectedEntries makes the value list always contain the active filter's current selection so each
// selected value can be individually deselected. It adds, at count 0, any selected value (and the ancestors
// surfaced for it) not already present, a flat entry for a selected value that vanished from the index, the
// direct-child entry for each selected "direct" value, and the missing entry when missing is selected. Values
// already present (with a real count) keep their count and only gain newly surfaced hierarchy paths. selected
// maps value/ancestor ids to their paths (from synthesizeSelectedEntries); selectedIDs are the explicitly
// selected to/direct value ids (for the flat fallback).
func mergeSelectedEntries(results []RefFilterResult, selected map[string][][]string, selectedIDs []string, direct []ToValue, missing bool) []RefFilterResult {
	byID := make(map[string]int, len(results))
	for i, r := range results {
		if r.ID == MissingValueID || strings.HasPrefix(r.ID, DirectRefFilterPrefix) {
			continue
		}
		byID[r.ID] = i
	}

	// Surfaced selected values and their ancestors: union paths into an existing entry, or append at count 0.
	for id, paths := range selected {
		if i, ok := byID[id]; ok {
			results[i].Paths = unionPaths(results[i].Paths, paths)
			continue
		}
		results = append(results, RefFilterResult{ID: id, Count: 0, Paths: paths})
		byID[id] = len(results) - 1
	}

	// A selected value with no indexed hierarchy anywhere produces no bucket; add it flat so it stays deselectable.
	for _, id := range selectedIDs {
		if _, ok := byID[id]; ok {
			continue
		}
		results = append(results, RefFilterResult{ID: id, Count: 0, Paths: nil})
		byID[id] = len(results) - 1
	}

	present := make(map[string]bool, len(results))
	for _, r := range results {
		present[r.ID] = true
	}

	// Direct child entry for each selected direct value, nested under its (now guaranteed present) value.
	for _, d := range direct {
		directID := DirectRefFilterPrefix + d.ID.String()
		if present[directID] {
			continue
		}
		value := RefFilterResult{ID: d.ID.String(), Count: 0, Paths: nil}
		if i, ok := byID[d.ID.String()]; ok {
			value = results[i]
		}
		results = append(results, RefFilterResult{ID: directID, Count: 0, Paths: directPaths(value)})
		present[directID] = true
	}

	// Missing entry when missing is selected and not already present (its real-count entry is added earlier).
	if missing && !present[MissingValueID] {
		results = append(results, RefFilterResult{ID: MissingValueID, Count: 0, Paths: nil})
	}

	return results
}

// valueAggregationBuckets unwraps a value aggregation built with valueAggregation under name (nested -> filter
// -> props) into its top-level value buckets. It is the shared shape of the reference, sub-reference, has, and
// sub-has value aggregations.
func valueAggregationBuckets(aggs map[string]types.Aggregate, name string) ([]types.StringTermsBucket, errors.E) {
	nested, errE := internalSearch.AggAs[types.NestedAggregate](aggs, name)
	if errE != nil {
		return nil, errE
	}
	filter, errE := internalSearch.AggAs[types.FilterAggregate](nested.Aggregations, "filter")
	if errE != nil {
		return nil, errE
	}
	terms, errE := internalSearch.AggAs[types.StringTermsAggregate](filter.Aggregations, "props")
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

// matchingValueIDs returns the deduplicated value (or property) ids that matched a filter-pane value search: the
// in-scope value aggregation's bucket keys (under name) unioned with the augment ids the selectedMatch global
// aggregation matched. Bucket keys come first, in the aggregation's docs-descending order, then the matched
// augment ids in sorted order, so the result is deterministic. hasSelectedMatch reports whether the selectedMatch
// aggregation was added (the active filter has an augment id set), so its parse is skipped when it is absent.
func matchingValueIDs(aggs map[string]types.Aggregate, name string, hasSelectedMatch bool) ([]string, errors.E) {
	buckets, errE := valueAggregationBuckets(aggs, name)
	if errE != nil {
		return nil, errE
	}
	seen := make(map[string]bool, len(buckets))
	out := make([]string, 0, len(buckets))
	for _, bucket := range buckets {
		key, ok := bucket.Key.(string)
		if !ok {
			errE := errors.New("unexpected key type for " + name + " bucket")
			errors.Details(errE)["type"] = fmt.Sprintf("%T", bucket.Key)
			return nil, errE
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, key)
	}
	if !hasSelectedMatch {
		return out, nil
	}
	selectedMatch, errE := internalSearch.AggAs[types.GlobalAggregate](aggs, "selectedMatch")
	if errE != nil {
		return nil, errE
	}
	matched, errE := parseSelectedMatchIDs(selectedMatch)
	if errE != nil {
		return nil, errE
	}
	matchedIDs := make([]string, 0, len(matched))
	for id := range matched {
		matchedIDs = append(matchedIDs, id)
	}
	slices.Sort(matchedIDs)
	for _, id := range matchedIDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out, nil
}

// buildRefValueSearchResults assembles the reference (or sub-reference) value-search response: the matched value
// ids (matchingValueIDs) wrapped as id-only RefFilterResult entries (no count, paths, direct, or ancestor
// expansion), plus a trailing {ID: MissingValueID} entry when the typed text matched this property's own name
// (the propMatch aggregation has documents), which tells the frontend to also show the missing bucket carried by
// its unfiltered primary. The metadata total is the number of returned ids. name is the value aggregation name
// ("ref" or "subRef"); hasSelectedMatch reports whether the selectedMatch aggregation was added.
func buildRefValueSearchResults(aggs map[string]types.Aggregate, name string, hasSelectedMatch bool) ([]RefFilterResult, map[string]any, errors.E) {
	ids, errE := matchingValueIDs(aggs, name, hasSelectedMatch)
	if errE != nil {
		return nil, nil, errE
	}
	propMatch, errE := internalSearch.AggAs[types.FilterAggregate](aggs, "propMatch")
	if errE != nil {
		return nil, nil, errE
	}
	if propMatch.DocCount > 0 {
		ids = append(ids, MissingValueID)
	}
	results := make([]RefFilterResult, 0, len(ids))
	for _, id := range ids {
		results = append(results, RefFilterResult{ID: id, Count: 0, Paths: nil})
	}
	total := strconv.FormatInt(int64(len(results)), 10)
	return results, map[string]any{
		"total": total,
	}, nil
}

// getMatchingRefValueIDs runs the reference filter's value-search path: it returns only the value ids whose value
// name or this property's own name matched valueQuery, as id-only results (see buildRefValueSearchResults). The
// match set is the union of the in-scope value aggregation's bucket keys and the augment ids (the active filter's
// selected values and their ancestors) the selectedMatch aggregation matched. It is split out of Get so the
// value-search and the no-search paths each stay small.
func (f *RefFilter) getMatchingRefValueIDs(
	ctx context.Context, getSearchService func() *esSearch.Search,
	query types.QueryVariant, prop identifier.Identifier, excludeFullPaths []string,
	valueQuery string, enabledLanguages []string, resolver HierarchyPathsResolver,
) ([]RefFilterResult, map[string]any, errors.E) {
	metrics, _ := waf.GetMetrics(ctx)

	searchService := getSearchService()

	// Resolve the augment (the active filter's selected values plus their ancestors) so the selectedMatch
	// aggregation can label-match the whole augment id set in Elasticsearch; those values have zero documents in
	// the search scope and so never appear in the value aggregation.
	selectedIdents, _ := selectedRefIDs(f)
	augment, errE := resolveSelectedAugment(ctx, resolver, selectedIdents)
	if errE != nil {
		return nil, nil, errE
	}

	propTerm := esdsl.NewTermQuery("claims.ref.prop", esdsl.NewFieldValue().String(prop.String()))
	// The value match scopes the facet to records whose value name or this property's own name matches the typed
	// text; the property name is the same on every record, so a property-name match passes the whole facet.
	valueLabelMatch := labelMatchQuery(
		[]string{"claims.ref.toNaming"}, []string{"claims.ref.toDisplay"},
		[]string{"claims.ref.propNaming"}, []string{"claims.ref.propDisplay"},
		valueQuery, enabledLanguages,
	)
	refFilterQuery := esdsl.NewBoolQuery().Must(propTerm, valueLabelMatch)
	// When a prefilter on this property is active, drop the records derived from its value so the facet does not
	// re-count the prefilter's own value hierarchy.
	if len(excludeFullPaths) > 0 {
		refFilterQuery = refFilterQuery.MustNot(toFullPathTermsQuery("claims.ref", excludeFullPaths))
	}

	searchService = searchService.Size(0).Query(query).
		AddAggregation("ref", valueAggregation("claims.ref", refFilterQuery))
	searchService = addRefSelectedMatchAggregation(searchService, "claims.ref", []types.QueryVariant{propTerm}, valueLabelMatch, augment)
	// propMatch counts documents that have a record for this property whose property name matches the query; when
	// it is non-zero the missing bucket should show (the facet was reached by name, so the whole facet, missing
	// included, is shown).
	searchService = searchService.AddAggregation("propMatch", esdsl.NewAggregations().Filter(
		esdsl.NewNestedQuery(esdsl.NewBoolQuery().Must(
			propTerm,
			propLabelMatchQuery([]string{"claims.ref.propNaming"}, []string{"claims.ref.propDisplay"}, valueQuery, enabledLanguages),
		)).Path("claims.ref"),
	))

	m := metrics.Duration(internalStore.MetricElasticSearch).Start()
	res, err := searchService.Do(ctx)
	m.Stop()
	if err != nil {
		return nil, nil, WithESError(err)
	}
	metrics.Duration(internalStore.MetricElasticSearchInternal).Duration = time.Duration(res.Took) * time.Millisecond

	return buildRefValueSearchResults(res.Aggregations, "ref", len(augment) > 0)
}

// Get retrieves reference filter data for search results.
//
// excludeFullPaths, when non-empty, are claims.subRef.toFullPath values to control values dropped from
// the value aggregation: records derived from a prefilter value so the facet does not re-count the
// prefilter's own value hierarchy.
func (f *RefFilter) Get(
	ctx context.Context, getSearchService func() *esSearch.Search,
	query types.QueryVariant, prop identifier.Identifier, excludeFullPaths []string,
	valueQuery string, enabledLanguages []string, resolver HierarchyPathsResolver,
) ([]RefFilterResult, map[string]any, errors.E) {
	// During a filter-pane value search the response carries only the matching value ids (an overlay the frontend
	// applies on top of its unfiltered primary), so it takes a dedicated path.
	if valueQuery != "" {
		return f.getMatchingRefValueIDs(ctx, getSearchService, query, prop, excludeFullPaths, valueQuery, enabledLanguages, resolver)
	}

	metrics, _ := waf.GetMetrics(ctx)

	searchService := getSearchService()

	// Resolve the augment (the active filter's selected values plus their ancestors, with hierarchy paths) up
	// front via the resolver, so it can be force-shown at count 0, keeping the selection visible and individually
	// deselectable even when it matches no document under the rest of the search.
	selectedIdents, selectedIDs := selectedRefIDs(f)
	augment, errE := resolveSelectedAugment(ctx, resolver, selectedIdents)
	if errE != nil {
		return nil, nil, errE
	}

	// The value aggregation is scoped to records for this property.
	refFilterQuery := esdsl.NewBoolQuery().Must(esdsl.NewTermQuery("claims.ref.prop", esdsl.NewFieldValue().String(prop.String())))
	// When a prefilter on this property is active, also drop the records derived from its value so ancestor
	// buckets are not re-counted.
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
		results = append(results, RefFilterResult{ID: MissingValueID, Count: missingCount, Paths: nil})
	}

	// Force-show the active filter's augment (selected values and their ancestors) at count 0 so the selection is
	// always visible and individually deselectable.
	results = mergeSelectedEntries(results, augment, selectedIDs, f.Direct, f.Missing)

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
// HAS_LOCATION = L1) and a sub-ref filter (e.g. HAS_LOCATION > HAS_USER = A),
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

// getMatchingSubRefValueIDs runs the sub-reference filter's value-search path: it returns only the value ids
// whose value name, this sub-property's own name, or the parent property's name matched valueQuery, as id-only
// results (see buildRefValueSearchResults). The match set is the union of the in-scope value aggregation's bucket
// keys and the augment ids the selectedMatch aggregation matched. It is split out of GetSubRef so the
// value-search and the no-search paths each stay small.
func (f *RefFilter) getMatchingSubRefValueIDs(
	ctx context.Context, getSearchService func() *esSearch.Search,
	query types.QueryVariant, parentProp, prop identifier.Identifier,
	parentToRestrictions []identifier.Identifier, excludeFullPaths []string,
	valueQuery string, enabledLanguages []string, resolver HierarchyPathsResolver,
) ([]RefFilterResult, map[string]any, errors.E) {
	metrics, _ := waf.GetMetrics(ctx)

	searchService := getSearchService()

	selectedIdents, _ := selectedRefIDs(f)
	augment, errE := resolveSelectedAugment(ctx, resolver, selectedIdents)
	if errE != nil {
		return nil, nil, errE
	}

	parentPropTerm := esdsl.NewTermQuery("claims.subRef.parentProp", esdsl.NewFieldValue().String(parentProp.String()))
	propTerm := esdsl.NewTermQuery("claims.subRef.prop", esdsl.NewFieldValue().String(prop.String()))
	filterMusts := []types.QueryVariant{parentPropTerm, propTerm}
	if len(parentToRestrictions) > 0 {
		parentToShoulds := make([]types.QueryVariant, 0, len(parentToRestrictions))
		for _, pto := range parentToRestrictions {
			parentToShoulds = append(parentToShoulds, esdsl.NewTermQuery("claims.subRef.parentTo", esdsl.NewFieldValue().String(pto.String())))
		}
		filterMusts = append(filterMusts, esdsl.NewBoolQuery().Should(parentToShoulds...).MinimumShouldMatch(esdsl.NewMinimumShouldMatch().Int(1)))
	}
	// The value match scopes the facet to records whose value name, this sub-property's own name, or the parent
	// property's name matches the typed text. The parent property name is denormalized onto sub-reference records
	// as parentPropNaming/parentPropDisplay, so a sub-facet ("parentProp > prop") is matchable by it too.
	valueLabelMatch := labelMatchQuery(
		[]string{"claims.subRef.toNaming"}, []string{"claims.subRef.toDisplay"},
		[]string{"claims.subRef.propNaming", "claims.subRef.parentPropNaming"},
		[]string{"claims.subRef.propDisplay", "claims.subRef.parentPropDisplay"},
		valueQuery, enabledLanguages,
	)
	filterMusts = append(filterMusts, valueLabelMatch)
	subRefFilterQuery := esdsl.NewBoolQuery().Must(filterMusts...)
	if len(excludeFullPaths) > 0 {
		subRefFilterQuery = subRefFilterQuery.MustNot(toFullPathTermsQuery("claims.subRef", excludeFullPaths))
	}

	searchService = searchService.Size(0).Query(query).
		AddAggregation("subRef", valueAggregation("claims.subRef", subRefFilterQuery))
	// The selectedMatch is scoped to parentProp + prop and the augment ids, deliberately without the parentTo
	// restriction so a checked value is never hidden.
	searchService = addRefSelectedMatchAggregation(searchService, "claims.subRef", []types.QueryVariant{parentPropTerm, propTerm}, valueLabelMatch, augment)
	// propMatch counts documents that have a record for this (parentProp, prop) whose sub-property or parent
	// property name matches the query; when it is non-zero the missing bucket should show.
	searchService = searchService.AddAggregation("propMatch", esdsl.NewAggregations().Filter(
		esdsl.NewNestedQuery(esdsl.NewBoolQuery().Must(
			parentPropTerm, propTerm,
			propLabelMatchQuery(
				[]string{"claims.subRef.propNaming", "claims.subRef.parentPropNaming"},
				[]string{"claims.subRef.propDisplay", "claims.subRef.parentPropDisplay"}, valueQuery, enabledLanguages),
		)).Path("claims.subRef"),
	))

	m := metrics.Duration(internalStore.MetricElasticSearch).Start()
	res, err := searchService.Do(ctx)
	m.Stop()
	if err != nil {
		return nil, nil, WithESError(err)
	}
	metrics.Duration(internalStore.MetricElasticSearchInternal).Duration = time.Duration(res.Took) * time.Millisecond

	return buildRefValueSearchResults(res.Aggregations, "subRef", len(augment) > 0)
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
	valueQuery string, enabledLanguages []string, resolver HierarchyPathsResolver,
) ([]RefFilterResult, map[string]any, errors.E) {
	// During a filter-pane value search the response carries only the matching value ids (an overlay the frontend
	// applies on top of its unfiltered primary), so it takes a dedicated path.
	if valueQuery != "" {
		return f.getMatchingSubRefValueIDs(
			ctx, getSearchService, query, parentProp, prop, parentToRestrictions, excludeFullPaths, valueQuery, enabledLanguages, resolver,
		)
	}

	metrics, _ := waf.GetMetrics(ctx)

	searchService := getSearchService()

	// Resolve the augment (the active filter's selected values plus their ancestors, with hierarchy paths) up
	// front via the resolver, so it can be force-shown at count 0, keeping the selection visible and individually
	// deselectable even when it matches no document under the rest of the search.
	selectedIdents, selectedIDs := selectedRefIDs(f)
	augment, errE := resolveSelectedAugment(ctx, resolver, selectedIdents)
	if errE != nil {
		return nil, nil, errE
	}

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
	subRefFilterQuery := esdsl.NewBoolQuery().Must(filterMusts...)
	// When a prefilter on this (parentProp, prop) is active, drop the records derived from its value so
	// ancestor buckets are not re-counted.
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
		results = append(results, RefFilterResult{ID: MissingValueID, Count: missingCount, Paths: nil})
	}

	// Force-show the active filter's augment (selected values and their ancestors) at count 0 so the selection is
	// always visible and individually deselectable.
	results = mergeSelectedEntries(results, augment, selectedIDs, f.Direct, f.Missing)

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
