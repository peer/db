package search_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
	"gitlab.com/peerdb/peerdb/search"
)

// pathParents returns the immediate-parent value ids for hierarchy paths, matching what the converter stamps
// as ToParent: for each path "<hierProp>:<root>/.../<self>" the id segment before the last; a self or root
// path (a single id) contributes none. The result preserves first-seen order and is nil when none has a parent.
func pathParents(toPath []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, raw := range toPath {
		_, chain, ok := strings.Cut(raw, ":")
		if !ok {
			continue
		}
		parts := strings.Split(chain, "/")
		if len(parts) < 2 {
			continue
		}
		parent := parts[len(parts)-2]
		if seen[parent] {
			continue
		}
		seen[parent] = true
		out = append(out, parent)
	}
	return out
}

// prefilterSession returns a session carrying only the given prefilters. PrefilterExcludeFullPaths reads
// nothing else off the session.
func prefilterSession(prefilters []search.Filter) *search.Session {
	return &search.Session{ //nolint:exhaustruct
		SessionData: search.SessionData{ //nolint:exhaustruct
			Prefilters: prefilters,
		},
	}
}

// newFilterID returns a pointer to a fresh filter ID.
func newFilterID() *identifier.Identifier {
	id := identifier.New()
	return &id
}

func TestPrefilterExcludeFullPathsRef(t *testing.T) {
	t.Parallel()

	prop := identifier.New()
	v1 := identifier.New()
	v2 := identifier.New()
	resolve := func(_ context.Context, id identifier.Identifier) ([]string, errors.E) {
		switch id {
		case v1:
			return []string{"h:r/v1"}, nil
		case v2:
			return []string{"h:r/v2"}, nil
		default:
			return nil, nil
		}
	}
	session := prefilterSession([]search.Filter{{ //nolint:exhaustruct
		ID:   newFilterID(),
		Prop: []identifier.Identifier{prop},
		Ref:  &search.RefFilter{To: []search.ToValue{{ID: v1}, {ID: v2}}, Direct: nil, Missing: false},
	}})

	excludes, errE := session.PrefilterExcludeFullPaths(t.Context(), resolve)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.ElementsMatch(t, []string{"h:r/v1", "h:r/v2"}, excludes.Ref(prop))
	// A property with no prefilter, and any sub-reference facet, yield nothing.
	assert.Nil(t, excludes.Ref(identifier.New()))
	assert.Nil(t, excludes.SubRef(prop, prop))
}

func TestPrefilterExcludeFullPathsSubRef(t *testing.T) {
	t.Parallel()

	parentProp := identifier.New()
	prop := identifier.New()
	value := identifier.New()
	resolve := func(_ context.Context, id identifier.Identifier) ([]string, errors.E) {
		if id == value {
			return []string{"h:s/value"}, nil
		}
		return nil, nil
	}
	session := prefilterSession([]search.Filter{{ //nolint:exhaustruct
		ID:   newFilterID(),
		Prop: []identifier.Identifier{parentProp, prop},
		Ref:  &search.RefFilter{To: []search.ToValue{{ID: value}}, Direct: nil, Missing: false},
	}})

	excludes, errE := session.PrefilterExcludeFullPaths(t.Context(), resolve)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []string{"h:s/value"}, excludes.SubRef(parentProp, prop))
	// The same property as a top-level reference facet, and the reversed key, are unaffected.
	assert.Nil(t, excludes.Ref(prop))
	assert.Nil(t, excludes.SubRef(prop, parentProp))
}

func TestPrefilterExcludeFullPathsToAndDirectDeduplicated(t *testing.T) {
	t.Parallel()

	prop := identifier.New()
	value := identifier.New()
	calls := 0
	resolve := func(_ context.Context, id identifier.Identifier) ([]string, errors.E) {
		calls++
		if id == value {
			return []string{"h:d/a", "h:d/b"}, nil
		}
		return nil, nil
	}
	// The same value appears as both a To and a Direct value, and resolves to two paths.
	session := prefilterSession([]search.Filter{{ //nolint:exhaustruct
		ID:   newFilterID(),
		Prop: []identifier.Identifier{prop},
		Ref:  &search.RefFilter{To: []search.ToValue{{ID: value}}, Direct: []search.ToValue{{ID: value}}, Missing: false},
	}})

	excludes, errE := session.PrefilterExcludeFullPaths(t.Context(), resolve)
	require.NoError(t, errE, "% -+#.1v", errE)
	// The value is resolved once per occurrence (To and Direct), but each path is kept only once.
	assert.Equal(t, 2, calls)
	assert.Equal(t, []string{"h:d/a", "h:d/b"}, excludes.Ref(prop))
}

func TestPrefilterExcludeFullPathsSkipsNonRefAndPathless(t *testing.T) {
	t.Parallel()

	hasProp := identifier.New()
	rootProp := identifier.New()
	rootValue := identifier.New()
	resolve := func(_ context.Context, _ identifier.Identifier) ([]string, errors.E) {
		// Every value is a hierarchy root here, so it resolves to no path.
		return nil, nil
	}
	session := prefilterSession([]search.Filter{
		{ //nolint:exhaustruct
			// A non-reference prefilter creates no hierarchy redundancy and is ignored.
			ID:   newFilterID(),
			Prop: []identifier.Identifier{hasProp},
			Has:  &search.HasFilter{Props: nil},
		},
		{ //nolint:exhaustruct
			// A reference prefilter whose value has no path contributes nothing.
			ID:   newFilterID(),
			Prop: []identifier.Identifier{rootProp},
			Ref:  &search.RefFilter{To: []search.ToValue{{ID: rootValue}}, Direct: nil, Missing: false},
		},
	})

	excludes, errE := session.PrefilterExcludeFullPaths(t.Context(), resolve)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Nil(t, excludes.Ref(hasProp))
	assert.Nil(t, excludes.Ref(rootProp))
}

func TestPrefilterExcludeFullPathsResolverError(t *testing.T) {
	t.Parallel()

	prop := identifier.New()
	value := identifier.New()
	filterID := newFilterID()
	sentinel := errors.Base("resolve failed")
	resolve := func(_ context.Context, id identifier.Identifier) ([]string, errors.E) {
		if id == value {
			return nil, errors.WithStack(sentinel)
		}
		return nil, nil
	}
	session := prefilterSession([]search.Filter{{ //nolint:exhaustruct
		ID:   filterID,
		Prop: []identifier.Identifier{prop},
		Ref:  &search.RefFilter{To: []search.ToValue{{ID: value}}, Direct: nil, Missing: false},
	}})

	_, errE := session.PrefilterExcludeFullPaths(t.Context(), resolve)
	require.Error(t, errE)
	assert.ErrorIs(t, errE, sentinel)
	// The error carries the value that failed to resolve and the prefilter it came from.
	assert.Equal(t, value.String(), errors.Details(errE)["id"])
	assert.Equal(t, filterID.String(), errors.Details(errE)["filter"])
}

// hierRefClaim builds one expanded reference record: the record for value to with its own toPath,
// stamped with the stated leaf's toFullPath, as convertReference produces at index time.
func hierRefClaim(prop, to identifier.Identifier, toPath, fullPath []string) internalSearch.ReferenceClaim {
	return internalSearch.ReferenceClaim{
		Prop: prop, PropDisplay: nil, PropNaming: nil, PropSortKey: nil,
		To: to, ToDisplay: nil, ToNaming: nil, ToSortKey: nil,
		ToPath: toPath, ToFullPath: fullPath, ToParent: pathParents(toPath), ToDisplayPath: nil, ToPathSortKey: nil,
		IsLeaf: false,
	}
}

// refDoc builds an indexable document carrying only the given reference claims.
func refDoc(id string, claims internalSearch.ReferenceClaims) internalSearch.Document {
	return internalSearch.Document{
		DisplaySort: nil,
		ID:          identifier.From(id),
		Display:     nil,
		Text:        nil,
		Time:        nil,
		LastUpdated: nil,
		Counts:      internalSearch.Counts{References: nil, Claims: nil, Score: nil},
		Claims: internalSearch.ClaimTypes{
			Identifier: nil,
			String:     nil,
			HTML:       nil,
			Amount:     nil,
			Time:       nil,
			Link:       nil,
			Reference:  claims,
			Has:        nil,
			None:       nil,
			Unknown:    nil,
			SubRef:     nil,
			SubAmount:  nil,
			SubTime:    nil,
			SubHas:     nil,
		},
	}
}

// refFilterProps returns the set of property IDs that appear as top-level reference facets.
func refFilterProps(results []search.FilterResult) map[string]bool {
	out := map[string]bool{}
	for _, r := range results {
		if r.Type == "ref" && len(r.Props) > 0 {
			out[r.Props[0]] = true
		}
	}
	return out
}

// TestRefFilterGetPrefilterExcludeIntegration verifies that excludeFullPaths drops a prefilter value's
// records from the value aggregation, so the prefilter value's own bucket disappears and its ancestor
// buckets deflate to only the documents that still reach them through a sibling.
func TestRefFilterGetPrefilterExcludeIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	refProp := identifier.From("refProp")
	hierProp := identifier.From("hierProp")
	animal := identifier.From("animal")
	mammal := identifier.From("mammal")
	dog := identifier.From("dog")
	cat := identifier.From("cat")

	// Hierarchy: animal > mammal > {dog, cat}. Paths follow the indexed "<hierProp>:<root>/.../<this>" form.
	animalPath := hierProp.String() + ":" + animal.String()
	mammalPath := animalPath + "/" + mammal.String()
	dogPath := mammalPath + "/" + dog.String()
	catPath := mammalPath + "/" + cat.String()

	// One document references dog (expanded to dog, mammal, animal, all stamped with dog's toFullPath), and
	// another references cat (expanded likewise with cat's toFullPath).
	indexDocument(t, ctx, esClient, index, refDoc("dogDoc", internalSearch.ReferenceClaims{
		hierRefClaim(refProp, dog, []string{dogPath}, []string{dogPath}),
		hierRefClaim(refProp, mammal, []string{mammalPath}, []string{dogPath}),
		hierRefClaim(refProp, animal, []string{animalPath}, []string{dogPath}),
	}))
	indexDocument(t, ctx, esClient, index, refDoc("catDoc", internalSearch.ReferenceClaims{
		hierRefClaim(refProp, cat, []string{catPath}, []string{catPath}),
		hierRefClaim(refProp, mammal, []string{mammalPath}, []string{catPath}),
		hierRefClaim(refProp, animal, []string{animalPath}, []string{catPath}),
	}))
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})
	f := search.RefFilter{}

	// Without an exclude both ancestors are double-counted: each is reached through dog and through cat.
	results, _, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), refProp, nil, "", nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	counts := map[string]int64{}
	for _, r := range results {
		counts[r.ID] = r.Count
	}
	assert.Equal(t, int64(1), counts[dog.String()])
	assert.Equal(t, int64(1), counts[cat.String()])
	assert.Equal(t, int64(2), counts[mammal.String()])
	assert.Equal(t, int64(2), counts[animal.String()])

	// Excluding dog's toFullPath drops dogDoc's records: dog disappears and the ancestors deflate to the
	// single document (catDoc) that still reaches them. The child counts respect the same exclusion, so
	// mammal now has one visible child (cat) and animal one (mammal); cat is a leaf.
	excluded, metadata, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), refProp, []string{dogPath}, "", nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []search.RefFilterResult{
		{ID: animal.String(), Count: 1, ChildCount: 1, Paths: nil},
		{ID: mammal.String(), Count: 1, ChildCount: 1, Paths: [][]string{{animal.String()}}},
		{ID: cat.String(), Count: 1, ChildCount: 0, Paths: [][]string{{animal.String(), mammal.String()}}},
	}, excluded)
	assert.Equal(t, "3", metadata["total"])
}

// TestFiltersGetPrefilterExcludeIntegration verifies that a discovery facet whose only values come from a
// prefilter value is skipped, while a facet on another property is unaffected.
func TestFiltersGetPrefilterExcludeIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	refProp := identifier.From("refProp")
	otherProp := identifier.From("otherProp")
	hierProp := identifier.From("hierProp")
	animal := identifier.From("animal")
	mammal := identifier.From("mammal")
	dog := identifier.From("dog")
	other := identifier.From("other")

	animalPath := hierProp.String() + ":" + animal.String()
	mammalPath := animalPath + "/" + mammal.String()
	dogPath := mammalPath + "/" + dog.String()

	// One document: under refProp it references dog (every record stamped with dog's toFullPath), and under
	// otherProp it references an unrelated value. So refProp's only values all derive from dog.
	indexDocument(t, ctx, esClient, index, refDoc("discDoc", internalSearch.ReferenceClaims{
		hierRefClaim(refProp, dog, []string{dogPath}, []string{dogPath}),
		hierRefClaim(refProp, mammal, []string{mammalPath}, []string{dogPath}),
		hierRefClaim(refProp, animal, []string{animalPath}, []string{dogPath}),
		hierRefClaim(otherProp, other, nil, nil),
	}))
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	// Without excludes, both reference properties are discoverable.
	results, _, errE := search.FiltersGet(ctx, getSearchService, session, nil, "", search.PrefilterExcludes{})
	require.NoError(t, errE, "% -+#.1v", errE)
	props := refFilterProps(results)
	assert.True(t, props[refProp.String()])
	assert.True(t, props[otherProp.String()])

	// Build the excludes a prefilter on refProp = dog would produce, then re-run discovery.
	prefilter := prefilterSession([]search.Filter{{ //nolint:exhaustruct
		ID:   newFilterID(),
		Prop: []identifier.Identifier{refProp},
		Ref:  &search.RefFilter{To: []search.ToValue{{ID: dog}}, Direct: nil, Missing: false},
	}})
	resolve := func(_ context.Context, id identifier.Identifier) ([]string, errors.E) {
		if id == dog {
			return []string{dogPath}, nil
		}
		return nil, nil
	}
	excludes, errE := prefilter.PrefilterExcludeFullPaths(ctx, resolve)
	require.NoError(t, errE, "% -+#.1v", errE)

	excludedResults, _, errE := search.FiltersGet(ctx, getSearchService, session, nil, "", excludes)
	require.NoError(t, errE, "% -+#.1v", errE)
	excludedProps := refFilterProps(excludedResults)
	// refProp's only values were dog's hierarchy, so its facet is skipped; otherProp is unaffected.
	assert.False(t, excludedProps[refProp.String()])
	assert.True(t, excludedProps[otherProp.String()])
}
