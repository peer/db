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
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"
	"gitlab.com/tozd/waf"

	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
	internalStore "gitlab.com/peerdb/peerdb/internal/store"
)

// MissingValueID is the synthetic API ID for the "missing" bucket: documents in the facet's universe that
// state nothing facetable for the property path. It labels the missing entry in reference filter results
// and the missing group in grouped search results; the frontend special-cases this ID and renders it with
// the common.values.missing label.
const MissingValueID = "__MISSING__"

// NoneValueID is the synthetic API ID for the "none" special value: documents with an explicit none
// statement for the property path.
const NoneValueID = "__NONE__"

// UnknownValueID is the synthetic API ID for the "unknown" special value: documents stating that a value
// for the property path exists but is unknown.
const UnknownValueID = "__UNKNOWN__"

// HasPropertyValueID is the synthetic API ID for the "has property" special value: documents with a has
// statement for the property path. It appears in a property's own facet when the property's has claims
// migrated out of the pooled has facet.
const HasPropertyValueID = "__HAS__"

// DirectRefFilterPrefix prefixes the synthetic "direct" value id in reference filter results;
// the suffix is the parent value id. It is appended as a child of a value that has narrower values
// and represents documents that are exactly that value, with none of its narrower values (its
// most-specific/leaf instances).
const DirectRefFilterPrefix = "__DIRECT__:"

// maxHierarchyPathsPerValue caps the distinct toPath strings a terms aggregation returns per reference filter
// value. A value in a single hierarchy has one path; this bound only matters for diamond or multi-hierarchy
// values (a value reachable through more than one path).
const maxHierarchyPathsPerValue = 100

// HierarchyPathsResolver resolves a value's full hierarchy path strings ("<hierProp>:<root>/.../<self>"),
// the same form hierarchyPathChain parses. The handler injects it (backed by the cached Converter via
// Service.documentHierarchyPaths) so an active reference filter can resolve a selected value's ancestors at
// query time, without an Elasticsearch aggregation, and thus know the augment ids up front.
type HierarchyPathsResolver = func(ctx context.Context, id identifier.Identifier) ([]string, errors.E)

// RefFilterResult represents occurrences count for a single reference in a reference filter.
// Paths lists hierarchy chains from root to immediate parent for this value, one entry
// per parent path the value participates in (multiple entries for diamond hierarchies
// or when the value sits in more than one value-hierarchy property). The frontend uses
// these to render filter values as a tree.
//
// ChildCount is the value's number of distinct child values across the whole hierarchy (robust to
// multiple inheritance, since it counts child values rather than documents), computed only for the primary
// (unfiltered-by-value-name) list. It is 0 for a leaf value. The frontend compares it to how many children
// were actually returned to mark values whose children were truncated by the MaxResultsCount cap.
//
// ChildCountAtLeast reports whether ChildCount is a lower bound: a sub facet's collected child key set is
// known to be incomplete, so more child values exist beyond ChildCount of them. It serializes childCount
// as the string "<n>+" in place of the number ("at least n", equality allowed). Only sub facets set it
// (see the childCounts aggregation in GetSubRef); top-level childCount comes from a cardinality
// aggregation and stays a plain number.
type RefFilterResult struct {
	ID                string     `json:"id"`
	Count             int64      `json:"count"`
	ChildCount        int64      `json:"childCount"`
	ChildCountAtLeast bool       `json:"-"`
	Paths             [][]string `json:"paths,omitempty"`
}

// MarshalJSON serializes childCount as a number when exact and as the string "<n>+" when it is a
// lower bound, the same convention the search results total uses.
func (r RefFilterResult) MarshalJSON() ([]byte, error) {
	type plain RefFilterResult
	if !r.ChildCountAtLeast {
		return x.MarshalWithoutEscapeHTML(plain(r))
	}
	return x.MarshalWithoutEscapeHTML(struct {
		plain
		ChildCount string `json:"childCount"`
	}{plain(r), strconv.FormatInt(r.ChildCount, 10) + "+"})
}

// parseToPath turns one indexed hierarchy path string into its ancestor chain.
// The input format is "<hierarchy_property_id>:<root_id>/<parent_id>/.../<this_id>".
// The hierarchy-property prefix is dropped (the consumer does not care which hierarchy
// the path belongs to), and the trailing segment is dropped (it is the value's own id).
// The returned slice is ordered from root to immediate parent. Returns nil when the
// input has no ":" separator or when the chain contains a single segment (the value
// itself has no ancestors in that hierarchy).
func parseToPath(raw string) []string {
	chain := hierarchyPathChain(raw)
	if len(chain) <= 1 {
		return nil
	}
	return chain[:len(chain)-1]
}

// hierarchyPathChain turns one indexed hierarchy path string into its full chain, ordered from root to the
// value itself (the trailing segment, kept). The input format is "<hierarchy_property_id>:<root_id>/.../<this_id>";
// only the hierarchy-property prefix is dropped. Unlike parseToPath (which drops the trailing own-id segment)
// this keeps it, so the chain can be split into a value and each of its ancestors. Returns nil when the input
// has no ":" separator.
func hierarchyPathChain(raw string) []string {
	_, chain, ok := strings.Cut(raw, ":")
	if !ok {
		return nil
	}
	return strings.Split(chain, "/")
}

// mergedRefValue accumulates one facet value across the parent collection paths a sub facet spans:
// summed document and direct counts, and the union of the value's hierarchy paths.
type mergedRefValue struct {
	ID     string
	Count  int64
	Direct int64
	Paths  []string
	Seen   map[string]bool
}

// refValueMerge accumulates value buckets from one or more aggregation contexts by value id, in
// first-seen order.
type refValueMerge struct {
	Order  []string
	Values map[string]*mergedRefValue
}

func newRefValueMerge() *refValueMerge {
	return &refValueMerge{Order: nil, Values: map[string]*mergedRefValue{}}
}

func (m *refValueMerge) Value(id string) *mergedRefValue {
	v, ok := m.Values[id]
	if !ok {
		v = &mergedRefValue{ID: id, Count: 0, Direct: 0, Paths: nil, Seen: map[string]bool{}}
		m.Values[id] = v
		m.Order = append(m.Order, id)
	}
	return v
}

// AddBucket folds one value terms bucket (with its "docs" reverse_nested count, "direct" leaf count
// and "paths" terms) into the merge. Document counts sum across contexts; a document contributing
// the same value through two parent collections is the residual overcount described in the package
// comment (the same property stated in two value types with the same sub-value).
func (m *refValueMerge) AddBucket(bucket types.StringTermsBucket, name string) errors.E {
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
	direct, errE := internalSearch.AggAs[types.FilterAggregate](bucket.Aggregations, "direct")
	if errE != nil {
		return errE
	}
	directDocs, errE := internalSearch.AggAs[types.ReverseNestedAggregate](direct.Aggregations, "docs")
	if errE != nil {
		return errE
	}
	bucketPaths, errE := internalSearch.AggAs[types.StringTermsAggregate](bucket.Aggregations, "paths")
	if errE != nil {
		return errE
	}
	pathBuckets, ok := bucketPaths.Buckets.([]types.StringTermsBucket)
	if !ok {
		errE := errors.New("unexpected bucket type for " + name + " paths")
		errors.Details(errE)["type"] = fmt.Sprintf("%T", bucketPaths.Buckets)
		return errE
	}
	v := m.Value(key)
	v.Count += bucketDocs.DocCount
	v.Direct += directDocs.DocCount
	for _, b := range pathBuckets {
		if p, ok := b.Key.(string); ok && !v.Seen[p] {
			v.Seen[p] = true
			v.Paths = append(v.Paths, p)
		}
	}
	return nil
}

// Results converts the merged values into RefFilterResult entries, in first-seen order.
func (m *refValueMerge) Results() []RefFilterResult {
	out := make([]RefFilterResult, 0, len(m.Order))
	for _, id := range m.Order {
		v := m.Values[id]
		out = append(out, RefFilterResult{
			ID:    v.ID,
			Count: v.Count,
			// ChildCount is stamped later (only in the primary, no value search path).
			ChildCount: 0, ChildCountAtLeast: false,
			Paths: ancestorChains(v.Paths),
		})
	}
	return out
}

// DirectEntries builds the synthetic "direct" child entries: one under each value that has narrower
// values present in the facet (it appears as an ancestor in another value's hierarchy paths) and
// whose most-specific document count is greater than zero.
func (m *refValueMerge) DirectEntries(values []RefFilterResult) []RefFilterResult {
	hasNarrower := map[string]bool{}
	for _, value := range values {
		for _, path := range value.Paths {
			for _, ancestor := range path {
				hasNarrower[ancestor] = true
			}
		}
	}
	out := make([]RefFilterResult, 0)
	for _, value := range values {
		if !hasNarrower[value.ID] {
			continue
		}
		merged := m.Values[value.ID]
		if merged == nil || merged.Direct <= 0 {
			continue
		}
		out = append(out, RefFilterResult{
			ID:         DirectRefFilterPrefix + value.ID,
			Count:      merged.Direct,
			ChildCount: 0, ChildCountAtLeast: false,
			Paths: directPaths(value),
		})
	}
	return out
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

// refValueTermsAggregation builds the value terms sub-aggregation shared by the reference facets:
// one bucket per "to" value carrying the document count ("docs", a reverse_nested to the root), the
// most-specific/leaf document count ("direct"), and the value's hierarchy paths ("paths"). subRel is
// the rel sub path the buckets aggregate (a top-level facet passes claims.rel itself).
func refValueTermsAggregation(subRel string) *types.Aggregations {
	return esdsl.NewAggregations().
		Terms(esdsl.NewTermsAggregation().Field(subRel+".to").Size(MaxResultsCount).
			Order(esdsl.NewAggregateOrder().Map(map[string]sortorder.SortOrder{"docs": sortorder.Desc}))).
		AddAggregation("docs", esdsl.NewAggregations().
			ReverseNested(esdsl.NewReverseNestedAggregation())).
		// "direct" counts the documents for which this value is most-specific (a leaf):
		// they reference the value but none of its narrower values.
		AddAggregation("direct", esdsl.NewAggregations().
			Filter(esdsl.NewTermQuery(subRel+".isLeaf", esdsl.NewFieldValue().Bool(true))).
			AddAggregation("docs", esdsl.NewAggregations().
				ReverseNested(esdsl.NewReverseNestedAggregation()))).
		AddAggregation("paths", esdsl.NewAggregations().
			Terms(esdsl.NewTermsAggregation().Field(subRel+".toPath").Size(maxHierarchyPathsPerValue))).
		AggregationsCaster()
}

// specialAggs adds the property path's special-value and identity aggregations: per-special
// document counts, the missing count, the universe size, and the other-value-types count. They are
// root-level filter aggregations, exact by construction.
func specialAggs(searchService *esSearch.Search, specials refSpecialQueries) *esSearch.Search {
	searchService = searchService.
		AddAggregation("specialNone", esdsl.NewAggregations().Filter(specials.None)).
		AddAggregation("specialUnknown", esdsl.NewAggregations().Filter(specials.Unknown)).
		AddAggregation("specialHas", esdsl.NewAggregations().Filter(specials.HasProperty)).
		AddAggregation(missingKey, esdsl.NewAggregations().Filter(specials.Missing)).
		AddAggregation(universeKey, esdsl.NewAggregations().Filter(specials.Universe))
	if specials.OtherTypes != nil {
		searchService = searchService.AddAggregation(otherTypesKey, esdsl.NewAggregations().Filter(specials.OtherTypes))
	}
	return searchService
}

// refSpecialQueries carries the compiled special-value and identity queries of one reference facet.
type refSpecialQueries struct {
	None        types.QueryVariant
	Unknown     types.QueryVariant
	HasProperty types.QueryVariant
	Missing     types.QueryVariant
	Universe    types.QueryVariant
	OtherTypes  types.QueryVariant
}

// specialCounts holds the parsed special-value and identity counts.
type specialCounts struct {
	None        int64
	Unknown     int64
	HasProperty int64
	Missing     int64
	Universe    int64
	OtherTypes  *int64
}

// parseSpecialCounts reads the special-value and identity aggregations.
func parseSpecialCounts(aggs map[string]types.Aggregate, withOtherTypes bool) (specialCounts, errors.E) {
	out := specialCounts{None: 0, Unknown: 0, HasProperty: 0, Missing: 0, Universe: 0, OtherTypes: nil}
	for _, entry := range []struct {
		name string
		dst  *int64
	}{
		{"specialNone", &out.None},
		{"specialUnknown", &out.Unknown},
		{"specialHas", &out.HasProperty},
		{missingKey, &out.Missing},
		{universeKey, &out.Universe},
	} {
		agg, errE := internalSearch.AggAs[types.FilterAggregate](aggs, entry.name)
		if errE != nil {
			return out, errE
		}
		*entry.dst = agg.DocCount
	}
	if withOtherTypes {
		agg, errE := internalSearch.AggAs[types.FilterAggregate](aggs, otherTypesKey)
		if errE != nil {
			return out, errE
		}
		out.OtherTypes = &agg.DocCount
	}
	return out, nil
}

// specialEntries appends the special-value entries (has property, unknown, none, missing) whose
// count is positive and which pass the value-search gate (includeSpecials: outside a value search
// always, during one only when the facet was reached by a property name). Selected specials are
// merged later so they stay deselectable even at zero.
func specialEntries(results []RefFilterResult, counts specialCounts, includeSpecials bool) ([]RefFilterResult, int) {
	if !includeSpecials {
		return results, 0
	}
	added := 0
	for _, entry := range []struct {
		id    string
		count int64
	}{
		{HasPropertyValueID, counts.HasProperty},
		{UnknownValueID, counts.Unknown},
		{NoneValueID, counts.None},
		{MissingValueID, counts.Missing},
	} {
		if entry.count > 0 {
			results = append(results, RefFilterResult{ID: entry.id, Count: entry.count, ChildCount: 0, ChildCountAtLeast: false, Paths: nil})
			added++
		}
	}
	return results, added
}

// specialsMetadata folds the identity counts into the facet metadata: the universe size (the
// facet's total document scope: everything in scope for a top-level facet, documents with a
// qualifying parent claim for a sub facet) and, when computed, the other-value-types count (the
// documents reachable only through the property's facets of other value types).
func specialsMetadata(metadata map[string]any, counts specialCounts) map[string]any {
	metadata[universeKey] = strconv.FormatInt(counts.Universe, 10)
	if counts.OtherTypes != nil {
		metadata[otherTypesKey] = strconv.FormatInt(*counts.OtherTypes, 10)
	}
	return metadata
}

// toTermsQuery matches rel records on the given path whose to value is one of ids.
func toTermsQuery(path string, ids []identifier.Identifier) types.QueryVariant { //nolint:ireturn
	values := make([]types.FieldValueVariant, len(ids))
	for i, id := range ids {
		values[i] = esdsl.NewFieldValue().String(id.String())
	}
	return esdsl.NewTermsQuery().AddTermsQuery(path+".to", esdsl.NewTermsQueryField().FieldValues(values...))
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

// selectedPathAccumulator collects, per value id, the deduplicated set of ancestor chains (root to that id's
// immediate parent) discovered while walking hierarchy path chains, so a value and every ancestor in a chain
// is recorded. Its finalize step turns each id's set into a sorted slice of paths.
type selectedPathAccumulator struct {
	Acc map[string]map[string][]string
}

func newSelectedPathAccumulator() *selectedPathAccumulator {
	return &selectedPathAccumulator{Acc: map[string]map[string][]string{}}
}

// Ensure records an id with no paths yet, so a value with no indexed hierarchy still appears (rendered flat).
func (a *selectedPathAccumulator) Ensure(id string) {
	if _, ok := a.Acc[id]; !ok {
		a.Acc[id] = map[string][]string{}
	}
}

// AddChain records, for a single root-to-self chain, the value AND every ancestor in it: for a chain
// [a,b,c,d] (self d) it records d with path [a,b,c], c with [a,b], b with [a], and a as a root (no path).
func (a *selectedPathAccumulator) AddChain(chain []string) {
	for i, id := range chain {
		a.Ensure(id)
		if i == 0 {
			// Root of this chain, no ancestors.
			continue
		}
		prefix := make([]string, i)
		copy(prefix, chain[:i])
		a.Acc[id][strings.Join(prefix, "/")] = prefix
	}
}

// Finalize turns the accumulated per-id chain sets into the augment map of id to its deduplicated, sorted
// hierarchy paths (root to immediate parent); an id with no ancestors maps to nil paths.
func (a *selectedPathAccumulator) Finalize() map[string][][]string {
	out := make(map[string][][]string, len(a.Acc))
	for id, set := range a.Acc {
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

// resolveSelectedAugment resolves the augment value set for an active reference filter: each explicitly
// selected value plus every ancestor of it, mapped to its deduplicated hierarchy paths (root to immediate
// parent). For each selected value it calls the injected resolver for that value's hierarchy path strings
// ("<hierProp>:<root>/.../<self>", the same form hierarchyPathChain parses) and accumulates them, so a selected
// value with no indexed hierarchy is still present (rendered flat). The map keys are exactly the ids that
// must be present in the value list for the selection (and its ancestor tree) to render. It returns nil when
// there is no resolver or no selection.
func resolveSelectedAugment(ctx context.Context, resolver HierarchyPathsResolver, selectedIDs []identifier.Identifier) (map[string][][]string, errors.E) {
	if resolver == nil || len(selectedIDs) == 0 {
		return nil, nil //nolint:nilnil
	}
	acc := newSelectedPathAccumulator()
	for _, sel := range selectedIDs {
		acc.Ensure(sel.String())
		paths, errE := resolver(ctx, sel)
		if errE != nil {
			return nil, errE
		}
		for _, raw := range paths {
			chain := hierarchyPathChain(raw)
			if len(chain) == 0 {
				continue
			}
			acc.AddChain(chain)
		}
	}
	return acc.Finalize(), nil
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

// isSpecialResultID reports whether id is one of the synthetic special or direct entry ids.
func isSpecialResultID(id string) bool {
	switch id {
	case MissingValueID, NoneValueID, UnknownValueID, HasPropertyValueID:
		return true
	}
	return strings.HasPrefix(id, DirectRefFilterPrefix)
}

// mergeSelectedEntries makes the value list always contain the active selection so each selected value can
// be individually deselected. It adds, at count 0, any selected value (and the ancestors surfaced for it)
// not already present, a flat entry for a selected value that vanished from the index, the direct-child
// entry for each selected "direct" value, and each selected special entry. Values already present (with a
// real count) keep their count and only gain newly surfaced hierarchy paths. selected maps value/ancestor
// ids to their paths; selectedIDs are the explicitly selected to/direct value ids.
func mergeSelectedEntries(results []RefFilterResult, selected map[string][][]string, selectedIDs []string, direct []ToValue, specials *SpecialsFilter) []RefFilterResult {
	byID := make(map[string]int, len(results))
	for i, r := range results {
		if isSpecialResultID(r.ID) {
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
		results = append(results, RefFilterResult{ID: id, Count: 0, ChildCount: 0, ChildCountAtLeast: false, Paths: paths})
		byID[id] = len(results) - 1
	}

	// A selected value with no indexed hierarchy anywhere produces no bucket; add it flat so it stays deselectable.
	for _, id := range selectedIDs {
		if _, ok := byID[id]; ok {
			continue
		}
		results = append(results, RefFilterResult{ID: id, Count: 0, ChildCount: 0, ChildCountAtLeast: false, Paths: nil})
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
		value := RefFilterResult{ID: d.ID.String(), Count: 0, ChildCount: 0, ChildCountAtLeast: false, Paths: nil}
		if i, ok := byID[d.ID.String()]; ok {
			value = results[i]
		}
		results = append(results, RefFilterResult{ID: directID, Count: 0, ChildCount: 0, ChildCountAtLeast: false, Paths: directPaths(value)})
		present[directID] = true
	}

	// Selected special entries stay deselectable even at zero count.
	if specials != nil {
		for _, entry := range []struct {
			id       string
			selected bool
		}{
			{HasPropertyValueID, specials.HasProperty},
			{UnknownValueID, specials.Unknown},
			{NoneValueID, specials.None},
			{MissingValueID, specials.Missing},
		} {
			if entry.selected && !present[entry.id] {
				results = append(results, RefFilterResult{ID: entry.id, Count: 0, ChildCount: 0, ChildCountAtLeast: false, Paths: nil})
				present[entry.id] = true
			}
		}
	}

	return results
}

// addMatchedAncestors adds, during a value search, the ancestor values of the values already in results (the
// matched values), taking their real counts and paths from allValues, so the matched values render under their
// tree context. A value search only changes what is shown, never the counts. It returns the updated results and
// how many ancestor entries were added (for the total). Direct, missing, and special entries carry no ancestor
// paths.
func addMatchedAncestors(results []RefFilterResult, allValues map[string]RefFilterResult) ([]RefFilterResult, int) {
	present := make(map[string]bool, len(results))
	for _, r := range results {
		present[r.ID] = true
	}
	ancestors := map[string]bool{}
	for _, r := range results {
		if isSpecialResultID(r.ID) {
			continue
		}
		for _, path := range r.Paths {
			for _, anc := range path {
				ancestors[anc] = true
			}
		}
	}
	added := 0
	for id := range ancestors {
		if present[id] {
			continue
		}
		value, ok := allValues[id]
		if !ok {
			continue
		}
		results = append(results, RefFilterResult{ID: id, Count: value.Count, ChildCount: value.ChildCount, ChildCountAtLeast: value.ChildCountAtLeast, Paths: value.Paths})
		present[id] = true
		added++
	}
	return results, added
}

// mergeSearchAugment adds, during a value search, the augment values whose label matched the typed text. The
// augment ids that matched come from the selectedMatch aggregations; each matched id is shown together with
// its ancestors (for tree context, from the resolver-built augment), but a matched ancestor does NOT pull in
// its descendants. It returns the updated results and how many entries were appended (for the total).
func mergeSearchAugment(
	matched map[string]bool, results []RefFilterResult, f *RefFilter, augment map[string][][]string, selectedIDs []string,
) ([]RefFilterResult, int) {
	// shown is the matched augment ids plus, for each, its ancestors (so the tree context renders); a matched
	// ancestor brings only itself and its own ancestors, never its descendants.
	shown := make(map[string]bool, len(matched))
	for id := range matched {
		shown[id] = true
		for _, path := range augment[id] {
			for _, anc := range path {
				shown[anc] = true
			}
		}
	}

	filteredAugment := make(map[string][][]string, len(shown))
	for id := range shown {
		if paths, ok := augment[id]; ok {
			filteredAugment[id] = paths
		}
	}
	matchedSelectedIDs := make([]string, 0, len(selectedIDs))
	for _, id := range selectedIDs {
		if shown[id] {
			matchedSelectedIDs = append(matchedSelectedIDs, id)
		}
	}
	matchedDirect := make([]ToValue, 0, len(f.Direct))
	for _, d := range f.Direct {
		if shown[d.ID.String()] {
			matchedDirect = append(matchedDirect, d)
		}
	}

	// The special entries are governed by the includeSpecials gating, not by the augment, so the merge runs
	// with no specials here.
	before := len(results)
	results = mergeSelectedEntries(results, filteredAugment, matchedSelectedIDs, matchedDirect, nil)
	return results, len(results) - before
}

// parseSelectedMatchIDs unwraps a selectedMatch aggregation (global -> nested chain -> filter -> match) into
// the set of augment ids whose label matched the value-search query, adding them to matched.
func parseSelectedMatchIDs(globalAgg *types.GlobalAggregate, nestedDepth int, matched map[string]bool) errors.E {
	aggs := globalAgg.Aggregations
	for range nestedDepth {
		nested, errE := internalSearch.AggAs[types.NestedAggregate](aggs, "nested")
		if errE != nil {
			return errE
		}
		aggs = nested.Aggregations
	}
	filter, errE := internalSearch.AggAs[types.FilterAggregate](aggs, "filter")
	if errE != nil {
		return errE
	}
	matchTerms, errE := internalSearch.AggAs[types.StringTermsAggregate](filter.Aggregations, "match")
	if errE != nil {
		return errE
	}
	buckets, ok := matchTerms.Buckets.([]types.StringTermsBucket)
	if !ok {
		errE := errors.New("unexpected bucket type for selected match")
		errors.Details(errE)["type"] = fmt.Sprintf("%T", matchTerms.Buckets)
		return errE
	}
	for _, bucket := range buckets {
		if key, ok := bucket.Key.(string); ok {
			matched[key] = true
		}
	}
	return nil
}

// Get retrieves reference filter data for a top-level property facet: the value tree (with direct
// entries), the special-value entries (has property, unknown, none, missing), and the identity
// metadata (universe and other-value-types counts). specials is the path's active specials
// selection (nil when none), merged so selected specials stay deselectable.
func (f *RefFilter) Get( //nolint:maintidx
	ctx context.Context, getSearchService func() *esSearch.Search,
	query types.QueryVariant, prop identifier.Identifier, specials *SpecialsFilter,
	valueQuery string, enabledLanguages []string, resolver HierarchyPathsResolver,
) ([]RefFilterResult, map[string]any, errors.E) {
	metrics, _ := waf.GetMetrics(ctx)

	searchService := getSearchService()

	// Resolve the augment (the active filter's selected values plus their ancestors, with hierarchy paths) up
	// front via the resolver, so during a value search the selectedMatch aggregation can label-match the whole
	// augment id set in Elasticsearch (those values have zero documents in the search scope and so never appear
	// in the value aggregation).
	selectedIdents, selectedIDs := selectedRefIDs(f)
	augment, errE := resolveSelectedAugment(ctx, resolver, selectedIdents)
	if errE != nil {
		return nil, nil, errE
	}

	// The value aggregation is scoped to ref records for this property. valueQuery additionally restricts the
	// facet to records whose value name or this property's own name matches the user-typed text, so the
	// pane can be narrowed without changing the search; it never alters which documents match.
	// Because the property name is the same on every record, when it matches the query the whole facet
	// passes (all values are shown), which is what a user searching for the facet by name wants.
	refFilterMusts := []types.QueryVariant{propTerm(relPath, prop)}
	var valueLabelMatch types.QueryVariant
	if valueQuery != "" {
		valueLabelMatch = labelMatchQuery(
			[]string{relPath + ".toNaming"}, []string{relPath + ".toDisplay"},
			[]string{relPath + ".propNaming"}, []string{relPath + ".propDisplay"},
			valueQuery, enabledLanguages,
		)
		refFilterMusts = append(refFilterMusts, valueLabelMatch)
	}
	refFilterQuery := esdsl.NewBoolQuery().Must(refFilterMusts...)

	refAggregation := esdsl.NewAggregations().
		Nested(esdsl.NewNestedAggregation().Path(relPath)).
		AddAggregation("filter", esdsl.NewAggregations().
			Filter(refFilterQuery).
			AddAggregation("props", refValueTermsAggregation(relPath)).
			AddAggregation("total", esdsl.NewAggregations().
				Cardinality(esdsl.NewCardinalityAggregation().Field(relPath+".to").PrecisionThreshold(maxPrecisionThreshold))))

	searchService = searchService.Size(0).Query(query).
		AddAggregation("ref", refAggregation)
	searchService = specialAggs(searchService, refSpecialQueries{
		None:        TopSpecialQuery(prop, internalSearch.ClaimTypeNone),
		Unknown:     TopSpecialQuery(prop, internalSearch.ClaimTypeUnknown),
		HasProperty: TopSpecialQuery(prop, internalSearch.ClaimTypeHas),
		Missing:     TopMissingQuery(prop),
		Universe:    esdsl.NewMatchAllQuery(),
		OtherTypes:  TopOtherTypesQuery(prop, "rel"),
	})

	// In the primary (no value search) list, count each value's distinct children so the frontend can mark
	// values whose children were truncated by the MaxResultsCount cap. It is scoped exactly like the value
	// aggregation and is a property of the full hierarchy.
	if valueQuery == "" {
		searchService = searchService.AddAggregation("childCounts", esdsl.NewAggregations().
			Nested(esdsl.NewNestedAggregation().Path(relPath)).
			AddAggregation("filter", esdsl.NewAggregations().
				Filter(refFilterQuery).
				AddAggregation("parents", esdsl.NewAggregations().
					Terms(esdsl.NewTermsAggregation().Field(relPath+".toParent").Size(MaxResultsCount)).
					AddAggregation("children", esdsl.NewAggregations().
						Cardinality(esdsl.NewCardinalityAggregation().Field(relPath+".to").PrecisionThreshold(maxPrecisionThreshold))))))
	}

	// During a value search the value aggregation above is narrowed to matching values, which drops their
	// ancestors. allRef recomputes every value's count and paths without the value-query narrowing, so the
	// matched values' ancestors can be shown for tree context with their unchanged (no-search) counts.
	// selectedMatch additionally label-matches the augment id set globally, so the active filter's selected
	// values and their ancestors (which have zero documents in the search scope) can still be narrowed by the
	// typed text using the SAME matcher real values use. Outside a value search the augment is force-shown
	// wholesale (in mergeSelectedEntries) and neither aggregation is needed.
	if valueQuery != "" {
		baseFilterQuery := esdsl.NewBoolQuery().Must(propTerm(relPath, prop))
		searchService = searchService.AddAggregation("allRef", esdsl.NewAggregations().
			Nested(esdsl.NewNestedAggregation().Path(relPath)).
			AddAggregation("filter", esdsl.NewAggregations().
				Filter(baseFilterQuery).
				AddAggregation("props", refValueTermsAggregation(relPath))))
		augmentIdents := augmentIdentifiers(augment)
		if len(augmentIdents) > 0 {
			searchService = searchService.AddAggregation("selectedMatch", esdsl.NewAggregations().
				Global(esdsl.NewGlobalAggregation()).
				AddAggregation("nested", esdsl.NewAggregations().
					Nested(esdsl.NewNestedAggregation().Path(relPath)).
					AddAggregation("filter", esdsl.NewAggregations().
						Filter(esdsl.NewBoolQuery().Must(propTerm(relPath, prop), toTermsQuery(relPath, augmentIdents), valueLabelMatch)).
						AddAggregation("match", esdsl.NewAggregations().
							Terms(esdsl.NewTermsAggregation().Field(relPath+".to").Size(MaxResultsCount))))))
		}
	}

	// When a value query is active, the special entries are only kept if the query matches this property's own
	// name (the user is searching for the facet by name and wants the whole facet, specials included). propMatch
	// counts documents that have a record for this property whose property name matches the query.
	if valueQuery != "" {
		searchService = searchService.AddAggregation("propMatch", esdsl.NewAggregations().Filter(
			esdsl.NewNestedQuery(esdsl.NewBoolQuery().Must(
				propTerm(relPath, prop),
				propLabelMatchQuery([]string{relPath + ".propNaming"}, []string{relPath + ".propDisplay"}, valueQuery, enabledLanguages),
			)).Path(relPath),
		))
	}

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

	counts, errE := parseSpecialCounts(res.Aggregations, true)
	if errE != nil {
		return nil, nil, errE
	}

	// The special entries are shown when there is no value query, or when the value query matches this
	// property's own name (the facet was reached by name, so the whole facet, specials included, is shown).
	includeSpecials := valueQuery == ""
	if valueQuery != "" {
		propMatch, errE := internalSearch.AggAs[types.FilterAggregate](res.Aggregations, "propMatch")
		if errE != nil {
			return nil, nil, errE
		}
		includeSpecials = propMatch.DocCount > 0
	}

	merge := newRefValueMerge()
	for _, bucket := range refBuckets {
		errE = merge.AddBucket(bucket, "ref")
		if errE != nil {
			return nil, nil, errE
		}
	}
	results := merge.Results()

	// Stamp each primary value's exact distinct-child count (computed only in the no value search path).
	if valueQuery == "" {
		childCounts, errE := parseChildCardinality(res.Aggregations, "childCounts")
		if errE != nil {
			return nil, nil, errE
		}
		for i := range results {
			if n, ok := childCounts[results[i].ID]; ok {
				results[i].ChildCount = n
			}
		}
	}

	// Append a synthetic "direct" entry under each value that has narrower values present and
	// has documents for which it is most-specific, so the value reads as an exact aggregate of its
	// narrower values plus this entry.
	direct := merge.DirectEntries(results)
	results = append(results, direct...)

	var specialsAdded int
	results, specialsAdded = specialEntries(results, counts, includeSpecials)

	addedAncestors := 0
	if valueQuery == "" {
		results = mergeSelectedEntries(results, augment, selectedIDs, f.Direct, specials)
	} else {
		allValues, errE := parseAllRefValues(res.Aggregations, "allRef")
		if errE != nil {
			return nil, nil, errE
		}
		results, addedAncestors = addMatchedAncestors(results, allValues)
		if len(augment) > 0 {
			selectedMatch, errE := internalSearch.AggAs[types.GlobalAggregate](res.Aggregations, "selectedMatch")
			if errE != nil {
				return nil, nil, errE
			}
			matched := map[string]bool{}
			errE = parseSelectedMatchIDs(selectedMatch, 1, matched)
			if errE != nil {
				return nil, nil, errE
			}
			var augmentAdded int
			results, augmentAdded = mergeSearchAugment(matched, results, f, augment, selectedIDs)
			addedAncestors += augmentAdded
		}
	}

	// Order for hierarchical tree rendering on the frontend.
	// This also puts the special and direct entries in the right positions.
	slices.SortStableFunc(results, compareRefFilterResults)

	refTotalValue := distinctValuesTotal(len(refBuckets), refTotal.Value) + int64(len(direct)) + int64(addedAncestors) + int64(specialsAdded)
	total := strconv.FormatInt(refTotalValue, 10)

	return results, specialsMetadata(map[string]any{
		"total": total,
	}, counts), nil
}

// parseChildCardinality unwraps a childCounts aggregation (nested -> filter -> terms "parents" ->
// cardinality "children") into a map from parent value id to its number of distinct child values.
func parseChildCardinality(aggs map[string]types.Aggregate, name string) (map[string]int64, errors.E) {
	nested, errE := internalSearch.AggAs[types.NestedAggregate](aggs, name)
	if errE != nil {
		return nil, errE
	}
	filter, errE := internalSearch.AggAs[types.FilterAggregate](nested.Aggregations, "filter")
	if errE != nil {
		return nil, errE
	}
	terms, errE := internalSearch.AggAs[types.StringTermsAggregate](filter.Aggregations, "parents")
	if errE != nil {
		return nil, errE
	}
	buckets, ok := terms.Buckets.([]types.StringTermsBucket)
	if !ok {
		errE := errors.New("unexpected bucket type for child counts")
		errors.Details(errE)["type"] = fmt.Sprintf("%T", terms.Buckets)
		return nil, errE
	}
	out := make(map[string]int64, len(buckets))
	for _, bucket := range buckets {
		key, ok := bucket.Key.(string)
		if !ok {
			errE := errors.New("unexpected key type for child counts bucket")
			errors.Details(errE)["type"] = fmt.Sprintf("%T", bucket.Key)
			return nil, errE
		}
		children, errE := internalSearch.AggAs[types.CardinalityAggregate](bucket.Aggregations, "children")
		if errE != nil {
			return nil, errE
		}
		out[key] = children.Value
	}
	return out, nil
}

// parseAllRefValues parses an unfiltered top-level value aggregation (nested -> filter -> props) into a map
// of value id to its result (real document count and hierarchy paths), used during a filter-pane value
// search to recover the ancestors of matched values with their unchanged (no-search) counts.
func parseAllRefValues(aggs map[string]types.Aggregate, name string) (map[string]RefFilterResult, errors.E) {
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
	merge := newRefValueMerge()
	for _, bucket := range buckets {
		errE = merge.AddBucket(bucket, name)
		if errE != nil {
			return nil, errE
		}
	}
	out := map[string]RefFilterResult{}
	for _, r := range merge.Results() {
		out[r.ID] = r
	}
	return out, nil
}

// GetSubRef retrieves reference filter data for a sub facet (parentProp > prop): the value tree
// (with direct entries), the special-value entries, and the identity metadata. The value
// aggregations run once per parent collection the context allows and merge in Go: document and
// direct counts sum (a document contributing the same value through two parent collections is the
// package comment's cross-value-type residual overcount), hierarchy paths union, and
// child counts union by key. parentCtx scopes every aggregation to qualifying parent claims (the
// same-set top-level constraints and sibling correlated conditions), so counts match what selecting
// a value would return.
func (f *RefFilter) GetSubRef( //nolint:maintidx
	ctx context.Context, getSearchService func() *esSearch.Search,
	query types.QueryVariant, prop identifier.Identifier,
	parentCtx *ParentContext, specials *SpecialsFilter,
	valueQuery string, enabledLanguages []string, resolver HierarchyPathsResolver,
) ([]RefFilterResult, map[string]any, errors.E) {
	metrics, _ := waf.GetMetrics(ctx)

	searchService := getSearchService()

	selectedIdents, selectedIDs := selectedRefIDs(f)
	augment, errE := resolveSelectedAugment(ctx, resolver, selectedIdents)
	if errE != nil {
		return nil, nil, errE
	}

	// valueQuery restricts the facet to records whose value name or this sub-property's own name matches the
	// user-typed text. The parent property's name is matched at parent level (parentPropMatch below); when
	// either property name matches, the whole facet passes and the primary list falls back to the unnarrowed
	// values.
	var valueLabelMatch types.QueryVariant
	collections := parentCtx.Collections()
	for _, parent := range collections {
		subRel := subPath(parent, "rel")
		pf, ok := parentCtx.CollectionFilter(parent)
		if !ok {
			continue
		}
		subMusts := []types.QueryVariant{propTerm(subRel, prop)}
		if valueQuery != "" {
			valueLabelMatch = labelMatchQuery(
				[]string{subRel + ".toNaming"}, []string{subRel + ".toDisplay"},
				[]string{subRel + ".propNaming"}, []string{subRel + ".propDisplay"},
				valueQuery, enabledLanguages,
			)
			subMusts = append(subMusts, valueLabelMatch)
		}
		agg := esdsl.NewAggregations().
			Nested(esdsl.NewNestedAggregation().Path(parentPath(parent))).
			AddAggregation("parentFilter", esdsl.NewAggregations().
				Filter(pf).
				AddAggregation("sub", esdsl.NewAggregations().
					Nested(esdsl.NewNestedAggregation().Path(subRel)).
					AddAggregation("filter", esdsl.NewAggregations().
						Filter(esdsl.NewBoolQuery().Must(subMusts...)).
						AddAggregation("props", refValueTermsAggregation(subRel)).
						AddAggregation("total", esdsl.NewAggregations().
							Cardinality(esdsl.NewCardinalityAggregation().Field(subRel+".to").PrecisionThreshold(maxPrecisionThreshold))))))
		searchService = searchService.AddAggregation("ref:"+parent, agg)

		// In the primary (no value search) list, collect each value's distinct child value keys, so the
		// counts can be unioned across parent collections in Go: summed cardinalities would overcount a
		// child value present under more than one collection. The children terms size shares
		// MaxResultsCount with the value-list cap deliberately: a saturated key set still reports at
		// least as many children as the value list can ever load, so the completeness comparisons
		// (the full-check gating and the values-not-shown marker) stay correct past the cap.
		//
		// The children terms' sum_other_doc_count decides whether the collected key set is complete
		// (ChildCountAtLeast). Terms buckets are per-term: a document contributes only to the buckets
		// of the terms it carries, and sum_other_doc_count is the total document count of the buckets
		// not returned. Zero therefore proves completeness even on a sharded index: every shard
		// returned every bucket it had and the merge truncated nothing, so the keys are all the child
		// values and childCount is exact, including exactly at the cap. A positive value proves only
		// that childCount is a lower bound, never how much more: within one shard an omitted bucket is
		// an unseen term, but a shard can also route documents of a globally returned term into
		// sum_other_doc_count (when that term missed the shard's own top list), and an omitted key of
		// one collection can already be in the union through another collection, so not even one more
		// distinct child value is provable.
		if valueQuery == "" {
			searchService = searchService.AddAggregation("childCounts:"+parent, esdsl.NewAggregations().
				Nested(esdsl.NewNestedAggregation().Path(parentPath(parent))).
				AddAggregation("parentFilter", esdsl.NewAggregations().
					Filter(pf).
					AddAggregation("sub", esdsl.NewAggregations().
						Nested(esdsl.NewNestedAggregation().Path(subRel)).
						AddAggregation("filter", esdsl.NewAggregations().
							Filter(esdsl.NewBoolQuery().Must(propTerm(subRel, prop))).
							AddAggregation("parents", esdsl.NewAggregations().
								Terms(esdsl.NewTermsAggregation().Field(subRel+".toParent").Size(MaxResultsCount)).
								AddAggregation("children", esdsl.NewAggregations().
									Terms(esdsl.NewTermsAggregation().Field(subRel+".to").Size(MaxResultsCount))))))))
		}

		if valueQuery != "" {
			searchService = searchService.AddAggregation("allRef:"+parent, esdsl.NewAggregations().
				Nested(esdsl.NewNestedAggregation().Path(parentPath(parent))).
				AddAggregation("parentFilter", esdsl.NewAggregations().
					Filter(pf).
					AddAggregation("sub", esdsl.NewAggregations().
						Nested(esdsl.NewNestedAggregation().Path(subRel)).
						AddAggregation("filter", esdsl.NewAggregations().
							Filter(esdsl.NewBoolQuery().Must(propTerm(subRel, prop))).
							AddAggregation("props", refValueTermsAggregation(subRel))))))
			augmentIdents := augmentIdentifiers(augment)
			if len(augmentIdents) > 0 {
				// selectedMatch is scoped to the sub property and the augment ids, deliberately without the
				// parent context, so a checked value is never hidden.
				searchService = searchService.AddAggregation("selectedMatch:"+parent, esdsl.NewAggregations().
					Global(esdsl.NewGlobalAggregation()).
					AddAggregation("nested", esdsl.NewAggregations().
						Nested(esdsl.NewNestedAggregation().Path(parentPath(parent))).
						AddAggregation("nested", esdsl.NewAggregations().
							Nested(esdsl.NewNestedAggregation().Path(subRel)).
							AddAggregation("filter", esdsl.NewAggregations().
								Filter(esdsl.NewBoolQuery().Must(propTerm(subRel, prop), toTermsQuery(subRel, augmentIdents), valueLabelMatch)).
								AddAggregation("match", esdsl.NewAggregations().
									Terms(esdsl.NewTermsAggregation().Field(subRel+".to").Size(MaxResultsCount)))))))
			}
		}
	}

	searchService = searchService.Size(0).Query(query)
	searchService = specialAggs(searchService, refSpecialQueries{
		None:        parentCtx.SpecialQuery(prop, internalSearch.ClaimTypeNone),
		Unknown:     parentCtx.SpecialQuery(prop, internalSearch.ClaimTypeUnknown),
		HasProperty: parentCtx.SpecialQuery(prop, internalSearch.ClaimTypeHas),
		Missing:     parentCtx.MissingQuery(prop),
		Universe:    parentCtx.ExistsQuery(),
		OtherTypes:  parentCtx.OtherTypesQuery(prop, "rel"),
	})

	// When a value query is active, the special entries are only kept if the query matches this
	// sub-property's own name or its parent property's name (the facet was reached by name and the whole
	// facet is shown). The parent property's labels live on the parent records, so they are matched at
	// parent level.
	if valueQuery != "" {
		var propMatchArms []types.QueryVariant
		for _, parent := range collections {
			pf, ok := parentCtx.CollectionFilter(parent)
			if !ok {
				continue
			}
			subRel := subPath(parent, "rel")
			propMatchArms = append(propMatchArms,
				esdsl.NewNestedQuery(esdsl.NewBoolQuery().Must(pf,
					esdsl.NewNestedQuery(esdsl.NewBoolQuery().Must(
						propTerm(subRel, prop),
						propLabelMatchQuery([]string{subRel + ".propNaming"}, []string{subRel + ".propDisplay"}, valueQuery, enabledLanguages),
					)).Path(subRel),
				)).Path(parentPath(parent)),
				esdsl.NewNestedQuery(esdsl.NewBoolQuery().Must(pf,
					propLabelMatchQuery([]string{parentPath(parent) + ".propNaming"}, []string{parentPath(parent) + ".propDisplay"}, valueQuery, enabledLanguages),
				)).Path(parentPath(parent)),
			)
		}
		searchService = searchService.AddAggregation("propMatch", esdsl.NewAggregations().Filter(oneOrShould(propMatchArms)))
	}

	m := metrics.Duration(internalStore.MetricElasticSearch).Start()
	res, err := searchService.Do(ctx)
	m.Stop()
	if err != nil {
		return nil, nil, WithESError(err)
	}
	metrics.Duration(internalStore.MetricElasticSearchInternal).Duration = time.Duration(res.Took) * time.Millisecond

	merge := newRefValueMerge()
	var bucketCount int
	var cardinalitySum int64
	for _, parent := range collections {
		buckets, total, errE := parseSubRefValueBuckets(res.Aggregations, "ref:"+parent)
		if errE != nil {
			return nil, nil, errE
		}
		for _, bucket := range buckets {
			errE = merge.AddBucket(bucket, "ref:"+parent)
			if errE != nil {
				return nil, nil, errE
			}
		}
		bucketCount = max(bucketCount, len(buckets))
		cardinalitySum += total
	}

	counts, errE := parseSpecialCounts(res.Aggregations, true)
	if errE != nil {
		return nil, nil, errE
	}

	includeSpecials := valueQuery == ""
	wholeFacet := false
	if valueQuery != "" {
		propMatch, errE := internalSearch.AggAs[types.FilterAggregate](res.Aggregations, "propMatch")
		if errE != nil {
			return nil, nil, errE
		}
		includeSpecials = propMatch.DocCount > 0
		// A property-name match (the sub property's or the parent property's) shows the whole facet: the
		// unnarrowed values replace the narrowed primary list.
		wholeFacet = propMatch.DocCount > 0
	}

	if wholeFacet {
		allMerge := newRefValueMerge()
		for _, parent := range collections {
			buckets, _, errE := parseSubRefValueBuckets(res.Aggregations, "allRef:"+parent)
			if errE != nil {
				return nil, nil, errE
			}
			for _, bucket := range buckets {
				errE = allMerge.AddBucket(bucket, "allRef:"+parent)
				if errE != nil {
					return nil, nil, errE
				}
			}
		}
		if len(allMerge.Order) > 0 {
			merge = allMerge
		}
	}
	results := merge.Results()

	// Stamp each primary value's distinct-child count by unioning the child value key sets across the
	// parent collections (computed only in the no value search path). A value whose children terms
	// were truncated in any collection gets ChildCountAtLeast: its union is a lower bound.
	if valueQuery == "" {
		childKeys := map[string]map[string]bool{}
		childIncomplete := map[string]bool{}
		for _, parent := range collections {
			errE := unionChildKeys(res.Aggregations, "childCounts:"+parent, childKeys, childIncomplete)
			if errE != nil {
				return nil, nil, errE
			}
		}
		for i := range results {
			if keys, ok := childKeys[results[i].ID]; ok {
				results[i].ChildCount = int64(len(keys))
				results[i].ChildCountAtLeast = childIncomplete[results[i].ID]
			}
		}
	}

	direct := merge.DirectEntries(results)
	results = append(results, direct...)

	var specialsAdded int
	results, specialsAdded = specialEntries(results, counts, includeSpecials)

	addedAncestors := 0
	if valueQuery == "" {
		results = mergeSelectedEntries(results, augment, selectedIDs, f.Direct, specials)
	} else {
		allValues := map[string]RefFilterResult{}
		allMerge := newRefValueMerge()
		for _, parent := range collections {
			buckets, _, errE := parseSubRefValueBuckets(res.Aggregations, "allRef:"+parent)
			if errE != nil {
				return nil, nil, errE
			}
			for _, bucket := range buckets {
				errE = allMerge.AddBucket(bucket, "allRef:"+parent)
				if errE != nil {
					return nil, nil, errE
				}
			}
		}
		for _, r := range allMerge.Results() {
			allValues[r.ID] = r
		}
		results, addedAncestors = addMatchedAncestors(results, allValues)
		if len(augment) > 0 {
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
			var augmentAdded int
			results, augmentAdded = mergeSearchAugment(matched, results, f, augment, selectedIDs)
			addedAncestors += augmentAdded
		}
	}

	// Order for hierarchical tree rendering on the frontend.
	// This also puts the special and direct entries in the right positions.
	slices.SortStableFunc(results, compareRefFilterResults)

	// The distinct-value total is exact while no per-collection terms aggregation saturated (the merged
	// key set is then complete); past the cap the summed cardinalities are the estimate.
	var subRefTotalValue int64
	if bucketCount < MaxResultsCount {
		subRefTotalValue = int64(len(merge.Order))
	} else {
		subRefTotalValue = max(int64(len(merge.Order)), cardinalitySum)
	}
	subRefTotalValue += int64(len(direct)) + int64(addedAncestors) + int64(specialsAdded)
	total := strconv.FormatInt(subRefTotalValue, 10)

	return results, specialsMetadata(map[string]any{
		"total": total,
	}, counts), nil
}

// parseSubRefValueBuckets extracts the value terms buckets and the distinct-value cardinality from
// one parent collection's sub facet aggregation (nested parent -> pf filter -> nested sub -> filter
// -> props/total). name is also woven into error messages.
func parseSubRefValueBuckets(aggs map[string]types.Aggregate, name string) ([]types.StringTermsBucket, int64, errors.E) {
	parentNested, errE := internalSearch.AggAs[types.NestedAggregate](aggs, name)
	if errE != nil {
		return nil, 0, errE
	}
	pf, errE := internalSearch.AggAs[types.FilterAggregate](parentNested.Aggregations, "parentFilter")
	if errE != nil {
		return nil, 0, errE
	}
	subNested, errE := internalSearch.AggAs[types.NestedAggregate](pf.Aggregations, "sub")
	if errE != nil {
		return nil, 0, errE
	}
	filter, errE := internalSearch.AggAs[types.FilterAggregate](subNested.Aggregations, "filter")
	if errE != nil {
		return nil, 0, errE
	}
	terms, errE := internalSearch.AggAs[types.StringTermsAggregate](filter.Aggregations, "props")
	if errE != nil {
		return nil, 0, errE
	}
	buckets, ok := terms.Buckets.([]types.StringTermsBucket)
	if !ok {
		errE := errors.New("unexpected bucket type for " + name)
		errors.Details(errE)["type"] = fmt.Sprintf("%T", terms.Buckets)
		return nil, 0, errE
	}
	var cardinality int64
	totalAgg, totalErrE := internalSearch.AggAs[types.CardinalityAggregate](filter.Aggregations, "total")
	if totalErrE == nil {
		cardinality = totalAgg.Value
	}
	return buckets, cardinality, nil
}

// unionChildKeys folds one parent collection's childCounts aggregation (nested parent -> pf ->
// nested sub -> filter -> parents terms -> children terms) into the per-parent-value child key
// sets, and records in incomplete the parent values whose children terms were truncated (a
// positive sum_other_doc_count), so their unioned key count is only a lower bound.
func unionChildKeys(aggs map[string]types.Aggregate, name string, out map[string]map[string]bool, incomplete map[string]bool) errors.E {
	parentNested, errE := internalSearch.AggAs[types.NestedAggregate](aggs, name)
	if errE != nil {
		return errE
	}
	pf, errE := internalSearch.AggAs[types.FilterAggregate](parentNested.Aggregations, "parentFilter")
	if errE != nil {
		return errE
	}
	subNested, errE := internalSearch.AggAs[types.NestedAggregate](pf.Aggregations, "sub")
	if errE != nil {
		return errE
	}
	filter, errE := internalSearch.AggAs[types.FilterAggregate](subNested.Aggregations, "filter")
	if errE != nil {
		return errE
	}
	terms, errE := internalSearch.AggAs[types.StringTermsAggregate](filter.Aggregations, "parents")
	if errE != nil {
		return errE
	}
	buckets, ok := terms.Buckets.([]types.StringTermsBucket)
	if !ok {
		errE := errors.New("unexpected bucket type for child counts")
		errors.Details(errE)["type"] = fmt.Sprintf("%T", terms.Buckets)
		return errE
	}
	for _, bucket := range buckets {
		key, ok := bucket.Key.(string)
		if !ok {
			continue
		}
		children, errE := internalSearch.AggAs[types.StringTermsAggregate](bucket.Aggregations, "children")
		if errE != nil {
			return errE
		}
		if children.SumOtherDocCount != nil && *children.SumOtherDocCount > 0 {
			incomplete[key] = true
		}
		childBuckets, ok := children.Buckets.([]types.StringTermsBucket)
		if !ok {
			continue
		}
		set := out[key]
		if set == nil {
			set = map[string]bool{}
			out[key] = set
		}
		for _, cb := range childBuckets {
			if ck, ok := cb.Key.(string); ok {
				set[ck] = true
			}
		}
	}
	return nil
}
