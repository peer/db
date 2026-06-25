package search_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"gitlab.com/peerdb/peerdb/search"
)

func cnt(n int64) *int64 {
	return &n
}

func TestFoldLevelHierarchy(t *testing.T) {
	t.Parallel()

	// Two leaf values in a continent > country > city hierarchy. doc1 is multi-placed (Paris and Berlin).
	entries := []search.TestingBucketEntry{
		{IDs: []string{"eu", "fr", "paris"}, Labels: []string{"Europe", "France", "Paris"}, Count: 2, Direct: []search.Result{{ID: "doc1"}, {ID: "doc2"}}}, //nolint:exhaustruct
		{IDs: []string{"eu", "de", "berlin"}, Labels: []string{"Europe", "Germany", "Berlin"}, Count: 1, Direct: []search.Result{{ID: "doc1"}}},            //nolint:exhaustruct
	}

	got := search.TestingFoldLevel(entries, false)

	want := []search.Result{
		{ID: "eu", Group: []search.Result{ //nolint:exhaustruct
			{ID: "fr", Group: []search.Result{ //nolint:exhaustruct
				{ID: "paris", Count: cnt(2), Group: []search.Result{{ID: "doc1"}, {ID: "doc2"}}}, //nolint:exhaustruct
			}},
			{ID: "de", Group: []search.Result{ //nolint:exhaustruct
				{ID: "berlin", Count: cnt(1), Group: []search.Result{{ID: "doc1"}}}, //nolint:exhaustruct
			}},
		}},
	}
	assert.Equal(t, want, got)
}

func TestFoldLevelNodeIsBothValueAndAncestor(t *testing.T) {
	t.Parallel()

	// France is a stated leaf value (has direct docs) and also an ancestor of Paris. Its sub-group (Paris)
	// comes before its own direct documents.
	entries := []search.TestingBucketEntry{
		{IDs: []string{"eu", "fr"}, Labels: []string{"Europe", "France"}, Count: 5, Direct: []search.Result{{ID: "docFR"}}},                  //nolint:exhaustruct
		{IDs: []string{"eu", "fr", "paris"}, Labels: []string{"Europe", "France", "Paris"}, Count: 2, Direct: []search.Result{{ID: "docP"}}}, //nolint:exhaustruct
	}

	got := search.TestingFoldLevel(entries, false)

	want := []search.Result{
		{ID: "eu", Group: []search.Result{ //nolint:exhaustruct
			{ID: "fr", Count: cnt(5), Group: []search.Result{
				{ID: "paris", Count: cnt(2), Group: []search.Result{{ID: "docP"}}}, //nolint:exhaustruct
				{Count: nil, Group: nil, ID: "docFR"},
			}},
		}},
	}
	assert.Equal(t, want, got)
}

func TestFoldLevelDescendingOrder(t *testing.T) {
	t.Parallel()

	entries := []search.TestingBucketEntry{
		{IDs: []string{"a"}, Labels: []string{"Apple"}, Count: 1, Direct: []search.Result{{ID: "d1"}}},  //nolint:exhaustruct
		{IDs: []string{"b"}, Labels: []string{"Banana"}, Count: 1, Direct: []search.Result{{ID: "d2"}}}, //nolint:exhaustruct
	}

	asc := search.TestingFoldLevel(entries, false)
	assert.Equal(t, []string{"a", "b"}, []string{asc[0].ID, asc[1].ID})

	desc := search.TestingFoldLevel(entries, true)
	assert.Equal(t, []string{"b", "a"}, []string{desc[0].ID, desc[1].ID})
}

func TestFoldLevelFlat(t *testing.T) {
	t.Parallel()

	// Depth-1 values (no hierarchy): each is its own top-level heading.
	entries := []search.TestingBucketEntry{
		{IDs: []string{"paris"}, Labels: []string{"Paris"}, Count: 3, Direct: []search.Result{{ID: "x"}}},   //nolint:exhaustruct
		{IDs: []string{"berlin"}, Labels: []string{"Berlin"}, Count: 1, Direct: []search.Result{{ID: "y"}}}, //nolint:exhaustruct
	}

	got := search.TestingFoldLevel(entries, false)

	want := []search.Result{
		{ID: "berlin", Count: cnt(1), Group: []search.Result{{ID: "y"}}}, //nolint:exhaustruct
		{ID: "paris", Count: cnt(3), Group: []search.Result{{ID: "x"}}},  //nolint:exhaustruct
	}
	assert.Equal(t, want, got)
}

func TestLimitGroups(t *testing.T) {
	t.Parallel()

	// Three flat groups whose Count reports the true group size: g1 and g2 hold two documents each, g3 one.
	tree := func() []search.Result {
		return []search.Result{
			{ID: "g1", Count: cnt(2), Group: []search.Result{{ID: "d1"}, {ID: "d2"}}}, //nolint:exhaustruct
			{ID: "g2", Count: cnt(2), Group: []search.Result{{ID: "d3"}, {ID: "d4"}}}, //nolint:exhaustruct
			{ID: "g3", Count: cnt(1), Group: []search.Result{{ID: "d5"}}},             //nolint:exhaustruct
		}
	}

	// A limit at or above the total leaf count keeps the whole tree unchanged.
	got, n := search.TestingLimitGroups(tree(), 10)
	assert.Equal(t, 5, n)
	assert.Equal(t, tree(), got)

	// A limit of three keeps g1 whole (two docs) plus the first doc of g2, dropping g2's second doc and all of
	// g3. Group headings do not consume the budget, and the kept g2 heading keeps its true Count of two.
	got, n = search.TestingLimitGroups(tree(), 3)
	assert.Equal(t, 3, n)
	assert.Equal(t, []search.Result{
		{ID: "g1", Count: cnt(2), Group: []search.Result{{ID: "d1"}, {ID: "d2"}}}, //nolint:exhaustruct
		{ID: "g2", Count: cnt(2), Group: []search.Result{{ID: "d3"}}},             //nolint:exhaustruct
	}, got)

	// A limit that exactly fills g1 drops the later groups entirely: no empty headings are emitted.
	got, n = search.TestingLimitGroups(tree(), 2)
	assert.Equal(t, 2, n)
	assert.Equal(t, []search.Result{
		{ID: "g1", Count: cnt(2), Group: []search.Result{{ID: "d1"}, {ID: "d2"}}}, //nolint:exhaustruct
	}, got)
}

func TestLimitGroupsNested(t *testing.T) {
	t.Parallel()

	// Two-level grouping: outer o1 nests inner i1 (two docs) and i2 (one doc); outer o2 nests i3 (two docs).
	tree := func() []search.Result {
		return []search.Result{
			{ID: "o1", Count: cnt(3), Group: []search.Result{
				{ID: "i1", Count: cnt(2), Group: []search.Result{{ID: "d1"}, {ID: "d2"}}}, //nolint:exhaustruct
				{ID: "i2", Count: cnt(1), Group: []search.Result{{ID: "d3"}}},             //nolint:exhaustruct
			}},
			{ID: "o2", Count: cnt(2), Group: []search.Result{
				{ID: "i3", Count: cnt(2), Group: []search.Result{{ID: "d4"}, {ID: "d5"}}}, //nolint:exhaustruct
			}},
		}
	}

	// A limit of three keeps all of o1 (i1 with two docs, i2 with one) and drops o2 entirely.
	got, n := search.TestingLimitGroups(tree(), 3)
	assert.Equal(t, 3, n)
	assert.Equal(t, []search.Result{
		{ID: "o1", Count: cnt(3), Group: []search.Result{
			{ID: "i1", Count: cnt(2), Group: []search.Result{{ID: "d1"}, {ID: "d2"}}}, //nolint:exhaustruct
			{ID: "i2", Count: cnt(1), Group: []search.Result{{ID: "d3"}}},             //nolint:exhaustruct
		}},
	}, got)

	// A limit of two fills i1 and drops the now-empty i2 along with the whole o2 branch.
	got, n = search.TestingLimitGroups(tree(), 2)
	assert.Equal(t, 2, n)
	assert.Equal(t, []search.Result{
		{ID: "o1", Count: cnt(3), Group: []search.Result{
			{ID: "i1", Count: cnt(2), Group: []search.Result{{ID: "d1"}, {ID: "d2"}}}, //nolint:exhaustruct
		}},
	}, got)
}
