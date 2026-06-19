package search

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func cnt(n int64) *int64 {
	return &n
}

func TestFoldLevelHierarchy(t *testing.T) {
	t.Parallel()

	// Two leaf values in a continent > country > city hierarchy. doc1 is multi-placed (Paris and Berlin).
	entries := []bucketEntry{
		{ids: []string{"eu", "fr", "paris"}, labels: []string{"Europe", "France", "Paris"}, count: 2, direct: []Result{{ID: "doc1"}, {ID: "doc2"}}}, //nolint:exhaustruct
		{ids: []string{"eu", "de", "berlin"}, labels: []string{"Europe", "Germany", "Berlin"}, count: 1, direct: []Result{{ID: "doc1"}}},            //nolint:exhaustruct
	}

	got := foldLevel(entries, false)

	want := []Result{
		{ID: "eu", Group: []Result{ //nolint:exhaustruct
			{ID: "fr", Group: []Result{ //nolint:exhaustruct
				{ID: "paris", Count: cnt(2), Group: []Result{{ID: "doc1"}, {ID: "doc2"}}}, //nolint:exhaustruct
			}},
			{ID: "de", Group: []Result{ //nolint:exhaustruct
				{ID: "berlin", Count: cnt(1), Group: []Result{{ID: "doc1"}}}, //nolint:exhaustruct
			}},
		}},
	}
	assert.Equal(t, want, got)
}

func TestFoldLevelNodeIsBothValueAndAncestor(t *testing.T) {
	t.Parallel()

	// France is a stated leaf value (has direct docs) and also an ancestor of Paris. Its sub-group (Paris)
	// comes before its own direct documents.
	entries := []bucketEntry{
		{ids: []string{"eu", "fr"}, labels: []string{"Europe", "France"}, count: 5, direct: []Result{{ID: "docFR"}}},                  //nolint:exhaustruct
		{ids: []string{"eu", "fr", "paris"}, labels: []string{"Europe", "France", "Paris"}, count: 2, direct: []Result{{ID: "docP"}}}, //nolint:exhaustruct
	}

	got := foldLevel(entries, false)

	want := []Result{
		{ID: "eu", Group: []Result{ //nolint:exhaustruct
			{ID: "fr", Count: cnt(5), Group: []Result{
				{ID: "paris", Count: cnt(2), Group: []Result{{ID: "docP"}}}, //nolint:exhaustruct
				{Count: nil, Group: nil, ID: "docFR"},
			}},
		}},
	}
	assert.Equal(t, want, got)
}

func TestFoldLevelDescendingOrder(t *testing.T) {
	t.Parallel()

	entries := []bucketEntry{
		{ids: []string{"a"}, labels: []string{"Apple"}, count: 1, direct: []Result{{ID: "d1"}}},  //nolint:exhaustruct
		{ids: []string{"b"}, labels: []string{"Banana"}, count: 1, direct: []Result{{ID: "d2"}}}, //nolint:exhaustruct
	}

	asc := foldLevel(entries, false)
	assert.Equal(t, []string{"a", "b"}, []string{asc[0].ID, asc[1].ID})

	desc := foldLevel(entries, true)
	assert.Equal(t, []string{"b", "a"}, []string{desc[0].ID, desc[1].ID})
}

func TestFoldLevelFlat(t *testing.T) {
	t.Parallel()

	// Depth-1 values (no hierarchy): each is its own top-level heading.
	entries := []bucketEntry{
		{ids: []string{"paris"}, labels: []string{"Paris"}, count: 3, direct: []Result{{ID: "x"}}},   //nolint:exhaustruct
		{ids: []string{"berlin"}, labels: []string{"Berlin"}, count: 1, direct: []Result{{ID: "y"}}}, //nolint:exhaustruct
	}

	got := foldLevel(entries, false)

	want := []Result{
		{ID: "berlin", Count: cnt(1), Group: []Result{{ID: "y"}}}, //nolint:exhaustruct
		{ID: "paris", Count: cnt(3), Group: []Result{{ID: "x"}}},  //nolint:exhaustruct
	}
	assert.Equal(t, want, got)
}
