package search

import (
	"slices"
	"strings"

	"github.com/elastic/go-elasticsearch/v9/typedapi/esdsl"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types/enums/sortorder"
	"gitlab.com/tozd/go/errors"

	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
)

// groupTopK is the maximum number of documents returned per leaf group. A leaf group with more members
// is truncated to its first groupTopK by the within-group sort.
const groupTopK = 100

// groupAggName is the top-level aggregation key under which the grouping aggregation is added.
const groupAggName = "group"

// hierarchyPathSeparator is the null byte separating display labels in toDisplayPath (mirrors
// internal/search.hierarchyPathSeparator); it sorts before all printable characters so display paths
// sort hierarchically.
const hierarchyPathSeparator = "\x00"

// buildGroupAggregation builds the nested grouping aggregation for the given leading group columns. Each
// column contributes nested(claims.ref) -> filter(prop & isLeaf) -> terms(toPath) -> { display path,
// reverse_nested -> next column or top_hits }. isLeaf restricts buckets to each document's most-specific
// values, so a document is grouped under its leaf value(s) only; documents with several values appear in
// several groups. withinSort orders the documents inside each leaf group.
func buildGroupAggregation(groupCols []SortKey, withinSort []types.SortCombinationsVariant, lang string) types.AggregationsVariant { //nolint:ireturn
	return groupLevelAggregation(groupCols, 0, withinSort, lang)
}

func groupLevelAggregation(cols []SortKey, idx int, withinSort []types.SortCombinationsVariant, lang string) types.AggregationsVariant { //nolint:ireturn
	prop := cols[idx].Prop[0]

	// The reverse_nested "back" sub-aggregation holds the next group level, or the leaf documents.
	back := esdsl.NewAggregations().ReverseNested(esdsl.NewReverseNestedAggregation())
	if idx+1 < len(cols) {
		back = back.AddAggregation("g", groupLevelAggregation(cols, idx+1, withinSort, lang))
	} else {
		topHits := esdsl.NewTopHitsAggregation().Size(groupTopK).Source_(esdsl.NewSourceConfig().Bool(false))
		if len(withinSort) > 0 {
			topHits = topHits.Sort(withinSort...)
		}
		back = back.AddAggregation("hits", esdsl.NewAggregations().TopHits(topHits))
	}

	buckets := esdsl.NewAggregations().
		Terms(esdsl.NewTermsAggregation().Field("claims.ref.toPath").Size(MaxResultsCount)).
		// The size-1 desc bucket is the longest toDisplayPath for this value, i.e. the full leaf display
		// path; its segments label the value and all of its synthesized ancestors for ordering.
		AddAggregation("dp", esdsl.NewAggregations().Terms(
			esdsl.NewTermsAggregation().Field("claims.ref.toDisplayPath."+lang).Size(1).
				Order(esdsl.NewAggregateOrder().Map(map[string]sortorder.SortOrder{"_key": sortorder.Desc})))).
		AddAggregation("back", back)

	filterQuery := esdsl.NewBoolQuery().Must(
		esdsl.NewTermQuery("claims.ref.prop", esdsl.NewFieldValue().String(prop)),
		esdsl.NewTermQuery("claims.ref.isLeaf", esdsl.NewFieldValue().Bool(true)),
	)

	return esdsl.NewAggregations().
		Nested(esdsl.NewNestedAggregation().Path("claims.ref")).
		AddAggregation("f", esdsl.NewAggregations().Filter(filterQuery).AddAggregation("b", buckets))
}

// bucketEntry is one leaf-value bucket of a group level: ids is the value's hierarchy id path (root to
// leaf), labels the matching display labels, count the number of documents at this value, and direct the
// already-folded content attached at this value (the next column's groups, or the documents).
type bucketEntry struct {
	ids    []string
	labels []string
	count  int64
	direct []Result
}

// leadingGroupKeys returns the leading contiguous run of group=true sort keys, which define the group
// levels. validateSort guarantees group keys cannot follow a non-group key.
func leadingGroupKeys(sort []SortKey) []SortKey {
	n := 0
	for n < len(sort) && sort[n].Group {
		n++
	}
	return sort[:n]
}

// foldGroups parses the grouping aggregation from the response and folds it into a nested Result tree.
func foldGroups(aggs map[string]types.Aggregate, groupCols []SortKey) ([]Result, errors.E) {
	entries, errE := parseGroupLevel(aggs, groupAggName, groupCols, 0)
	if errE != nil {
		return nil, errE
	}
	return foldLevel(entries, groupCols[0].Descending), nil
}

// parseGroupLevel reads one group level's leaf-value buckets from the aggregation under key, recursively
// folding deeper levels into each bucket's direct content. The display path needs no language here: the
// per-language toDisplayPath field was already selected when the aggregation was built.
func parseGroupLevel(aggs map[string]types.Aggregate, key string, cols []SortKey, idx int) ([]bucketEntry, errors.E) {
	nested, errE := internalSearch.AggAs[types.NestedAggregate](aggs, key)
	if errE != nil {
		return nil, errE
	}
	filtered, errE := internalSearch.AggAs[types.FilterAggregate](nested.Aggregations, "f")
	if errE != nil {
		return nil, errE
	}
	terms, errE := internalSearch.AggAs[types.StringTermsAggregate](filtered.Aggregations, "b")
	if errE != nil {
		return nil, errE
	}
	buckets, ok := terms.Buckets.([]types.StringTermsBucket)
	if !ok {
		errE := errors.New("unexpected bucket type for group level")
		errors.Details(errE)["sort"] = idx
		return nil, errE
	}

	entries := make([]bucketEntry, 0, len(buckets))
	for i := range buckets {
		bucket := buckets[i]
		toPath, ok := bucket.Key.(string)
		if !ok {
			continue
		}
		ids := pathSegments(toPath)
		if len(ids) == 0 {
			continue
		}

		back, errE := internalSearch.AggAs[types.ReverseNestedAggregate](bucket.Aggregations, "back")
		if errE != nil {
			return nil, errE
		}

		var direct []Result
		if idx+1 < len(cols) {
			childEntries, errE := parseGroupLevel(back.Aggregations, "g", cols, idx+1)
			if errE != nil {
				return nil, errE
			}
			direct = foldLevel(childEntries, cols[idx+1].Descending)
		} else {
			direct, errE = foldHits(back.Aggregations)
			if errE != nil {
				return nil, errE
			}
		}

		entries = append(entries, bucketEntry{
			ids:    ids,
			labels: displaySegments(bucket.Aggregations, "dp"),
			count:  back.DocCount,
			direct: direct,
		})
	}
	return entries, nil
}

// foldHits reads the leaf-group documents from the top_hits aggregation as plain (leaf) results.
func foldHits(aggs map[string]types.Aggregate) ([]Result, errors.E) {
	hits, errE := internalSearch.AggAs[types.TopHitsAggregate](aggs, "hits")
	if errE != nil {
		return nil, errE
	}
	out := make([]Result, 0, len(hits.Hits.Hits))
	for _, hit := range hits.Hits.Hits {
		if hit.Id_ != nil {
			out = append(out, Result{ID: *hit.Id_}) //nolint:exhaustruct
		}
	}
	return out, nil
}

// groupNode is a node in the per-column hierarchy trie used to fold leaf-value buckets into nested groups.
type groupNode struct {
	id    string
	label string
	// count is set when this node is itself a stated leaf value (a bucket); nil for synthesized ancestors.
	count *int64
	// direct is the content attached directly at this value (deeper-column groups, or documents).
	direct   []Result
	children map[string]*groupNode
	order    []string
}

func newGroupNode(id string) *groupNode {
	return &groupNode{id: id, label: "", count: nil, direct: nil, children: map[string]*groupNode{}, order: nil}
}

// foldLevel builds the hierarchy trie from one group level's leaf-value buckets and returns the ordered
// nested results. desc reverses the per-level display-label ordering of group headings.
func foldLevel(entries []bucketEntry, desc bool) []Result {
	root := newGroupNode("")
	for _, e := range entries {
		node := root
		for i, id := range e.ids {
			child, ok := node.children[id]
			if !ok {
				child = newGroupNode(id)
				node.children[id] = child
				node.order = append(node.order, id)
			}
			if child.label == "" && i < len(e.labels) {
				child.label = e.labels[i]
			}
			node = child
		}
		count := e.count
		node.count = &count
		node.direct = append(node.direct, e.direct...)
	}
	return root.results(desc)
}

// results returns this node's children as ordered group headings followed by this node's own direct
// content. Children are ordered by display label (ascending, or descending when desc).
func (n *groupNode) results(desc bool) []Result {
	order := slices.Clone(n.order)
	slices.SortStableFunc(order, func(a, b string) int {
		c := strings.Compare(n.children[a].label, n.children[b].label)
		if desc {
			return -c
		}
		return c
	})
	out := make([]Result, 0, len(order)+len(n.direct))
	for _, id := range order {
		child := n.children[id]
		out = append(out, Result{ID: child.id, Count: child.count, Group: child.results(desc)})
	}
	return append(out, n.direct...)
}

// pathSegments splits a toPath ("<propID>:<rootID>/.../<leafID>") into the hierarchy id chain from root
// to leaf.
func pathSegments(toPath string) []string {
	_, chain, ok := strings.Cut(toPath, ":")
	if !ok {
		return nil
	}
	return strings.Split(chain, "/")
}

// displaySegments reads the leaf value's full display path from the "dp" sub-aggregation and splits it
// into per-level labels.
func displaySegments(aggs map[string]types.Aggregate, key string) []string {
	dp, errE := internalSearch.AggAs[types.StringTermsAggregate](aggs, key)
	if errE != nil {
		return nil
	}
	buckets, ok := dp.Buckets.([]types.StringTermsBucket)
	if !ok || len(buckets) == 0 {
		return nil
	}
	label, ok := buckets[0].Key.(string)
	if !ok {
		return nil
	}
	return strings.Split(label, hierarchyPathSeparator)
}
