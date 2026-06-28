package search_test

import (
	"strconv"
	"testing"

	"github.com/elastic/go-elasticsearch/v9/typedapi/esdsl"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/identifier"

	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
	"gitlab.com/peerdb/peerdb/search"
)

// hasResultsByID indexes has filter results by their property id for assertions.
func hasResultsByID(results []search.HasFilterResult) map[string]search.HasFilterResult {
	out := make(map[string]search.HasFilterResult, len(results))
	for _, r := range results {
		out[r.ID] = r
	}
	return out
}

// mergeValueSearchLikeUIHas reproduces the frontend overlay for the flat has facet: from the primary (q="")
// results it keeps each entry whose id is in the search response (a direct match). The has facet has no
// hierarchy, direct entries, or missing bucket, so a matched id maps to exactly its own primary entry.
func mergeValueSearchLikeUIHas(primary, matched []search.HasFilterResult) []search.HasFilterResult {
	matchedIDs := make(map[string]bool, len(matched))
	for _, r := range matched {
		matchedIDs[r.ID] = true
	}
	out := make([]search.HasFilterResult, 0, len(primary))
	for _, p := range primary {
		if matchedIDs[p.ID] {
			out = append(out, p)
		}
	}
	return out
}

func TestHasFilterGetIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	color := identifier.From("color")
	shape := identifier.From("shape")

	indexDocument(t, ctx, esClient, index, internalSearch.Document{ //nolint:exhaustruct
		ID: identifier.From("hasDoc1"),
		Claims: internalSearch.ClaimTypes{ //nolint:exhaustruct
			Has: internalSearch.HasClaims{{Prop: color, PropDisplay: map[string]string{"en": "Color"}}}, //nolint:exhaustruct
		},
	})
	indexDocument(t, ctx, esClient, index, internalSearch.Document{ //nolint:exhaustruct
		ID: identifier.From("hasDoc2"),
		Claims: internalSearch.ClaimTypes{ //nolint:exhaustruct
			Has: internalSearch.HasClaims{{Prop: shape, PropDisplay: map[string]string{"en": "Shape"}}}, //nolint:exhaustruct
		},
	})
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	enabledLanguages := internalSearch.EnabledLanguages(nil)
	f := search.HasFilter{}

	// Without a value query both has-properties are listed; this primary carries the real counts that a value
	// search overlays its matching ids onto (reproduced by mergeValueSearchLikeUIHas).
	primary, metadata, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), "", enabledLanguages)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.ElementsMatch(t, []search.HasFilterResult{
		{ID: color.String(), Count: 1},
		{ID: shape.String(), Count: 1},
	}, primary)
	assert.Equal(t, "2", metadata["total"])

	// The value query (a prefix wildcard, as the frontend appends) narrows the facet to the matching property.
	matched, _, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), "col*", enabledLanguages)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []search.HasFilterResult{
		{ID: color.String(), Count: 1},
	}, mergeValueSearchLikeUIHas(primary, matched))
}

// TestHasFilterGetSelectedPropShownIntegration verifies that an active has filter's selected property is
// always listed, at count 0, even when no matching document has it, so it stays individually deselectable.
func TestHasFilterGetSelectedPropShownIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	color := identifier.From("color")
	shape := identifier.From("shape")

	indexDocument(t, ctx, esClient, index, internalSearch.Document{ //nolint:exhaustruct
		ID: identifier.From("hasDoc1"),
		Claims: internalSearch.ClaimTypes{ //nolint:exhaustruct
			Has: internalSearch.HasClaims{{Prop: color, PropDisplay: map[string]string{"en": "Color"}}}, //nolint:exhaustruct
		},
	})
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})
	enabledLanguages := internalSearch.EnabledLanguages(nil)

	// shape is selected but no matching document has it, so the bucket aggregation drops it; it must still be
	// returned at count 0 alongside the matching color property.
	f := search.HasFilter{Props: []search.HasValue{{ID: shape}}}
	results, _, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), "", enabledLanguages)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.ElementsMatch(t, []search.HasFilterResult{
		{ID: color.String(), Count: 1},
		{ID: shape.String(), Count: 0},
	}, results)
}

// TestHasFilterGetSelectedPropNotForcedDuringSearchIntegration verifies that during a filter-pane value search
// a selected has-property is not force-shown unless it matches the typed text (it is only force-shown outside a
// search, so it stays deselectable then).
func TestHasFilterGetSelectedPropNotForcedDuringSearchIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	color := identifier.From("color")
	shape := identifier.From("shape")

	indexDocument(t, ctx, esClient, index, internalSearch.Document{ //nolint:exhaustruct
		ID: identifier.From("hasDoc1"),
		Claims: internalSearch.ClaimTypes{ //nolint:exhaustruct
			Has: internalSearch.HasClaims{{Prop: color, PropDisplay: map[string]string{"en": "Color"}}}, //nolint:exhaustruct
		},
	})
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})
	enabledLanguages := internalSearch.EnabledLanguages(nil)
	f := search.HasFilter{Props: []search.HasValue{{ID: shape}}}

	// The primary (q="") force-shows the selected shape at count 0 alongside color.
	primary, _, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), "", enabledLanguages)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Searching "col*" matches color but not the selected shape, so only color is shown (shape is not
	// force-shown during a search).
	matched, _, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), "col*", enabledLanguages)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []search.HasFilterResult{
		{ID: color.String(), Count: 1},
	}, mergeValueSearchLikeUIHas(primary, matched))
}

// TestHasFilterGetSelectedAugmentValueSearchIntegration verifies that an active has filter's selected property,
// which has zero documents in the current search scope, is still searchable in the filter pane by the SAME
// property-label matcher real properties use (display label or any naming string). A non-matching term hides it
// (while a real in-scope property still shows), and outside a search it is force-shown at count 0.
func TestHasFilterGetSelectedAugmentValueSearchIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	color := identifier.From("color")
	shape := identifier.From("shape")

	// doc1 has color; doc2 has shape (with a display label and a naming string). The search scope below matches
	// only doc1, so the selected shape has zero documents in scope yet exists globally.
	indexDocument(t, ctx, esClient, index, internalSearch.Document{ //nolint:exhaustruct
		ID: identifier.From("hasDoc1"),
		Claims: internalSearch.ClaimTypes{ //nolint:exhaustruct
			Has: internalSearch.HasClaims{{Prop: color, PropDisplay: map[string]string{"en": "Color"}}}, //nolint:exhaustruct
		},
	})
	indexDocument(t, ctx, esClient, index, internalSearch.Document{ //nolint:exhaustruct
		ID: identifier.From("hasDoc2"),
		Claims: internalSearch.ClaimTypes{ //nolint:exhaustruct
			Has: internalSearch.HasClaims{{ //nolint:exhaustruct
				Prop: shape, PropDisplay: map[string]string{"en": "Shape"}, PropNaming: map[string][]string{"en": {"form"}},
			}},
		},
	})
	refreshIndex(t, ctx, esClient, index)

	enabledLanguages := internalSearch.EnabledLanguages(nil)
	// The rest of the search matches only doc1, so the selected shape is not in scope.
	restOfSearch := esdsl.NewNestedQuery(
		esdsl.NewTermQuery("claims.has.prop", esdsl.NewFieldValue().String(color.String())),
	).Path("claims.has")
	f := search.HasFilter{Props: []search.HasValue{{ID: shape}}}

	// The primary (q="") force-shows the selected shape at count 0 alongside the in-scope color; a value search
	// returns only the matching ids, which the frontend overlays on the primary (reproduced by
	// mergeValueSearchLikeUIHas).
	primary, _, errE := f.Get(ctx, getSearchService, restOfSearch, "", enabledLanguages)
	require.NoError(t, errE, "% -+#.1v", errE)
	byID := hasResultsByID(primary)
	require.Contains(t, byID, shape.String())
	assert.Equal(t, int64(0), byID[shape.String()].Count)
	require.Contains(t, byID, color.String())
	assert.Equal(t, int64(1), byID[color.String()].Count)

	// Searching shape's display label surfaces it at count 0, even though it has no document in scope.
	matched, _, errE := f.Get(ctx, getSearchService, restOfSearch, "shape*", enabledLanguages)
	require.NoError(t, errE, "% -+#.1v", errE)
	byID = hasResultsByID(mergeValueSearchLikeUIHas(primary, matched))
	require.Contains(t, byID, shape.String())
	assert.Equal(t, int64(0), byID[shape.String()].Count)
	assert.NotContains(t, byID, color.String())

	// Searching shape by its naming string ("form") surfaces it too, since the augment uses the same prop-label
	// matcher real properties use.
	matched, _, errE = f.Get(ctx, getSearchService, restOfSearch, "form*", enabledLanguages)
	require.NoError(t, errE, "% -+#.1v", errE)
	byID = hasResultsByID(mergeValueSearchLikeUIHas(primary, matched))
	require.Contains(t, byID, shape.String())

	// Searching "color" matches the real in-scope color property and hides the selected shape.
	matched, _, errE = f.Get(ctx, getSearchService, restOfSearch, "color*", enabledLanguages)
	require.NoError(t, errE, "% -+#.1v", errE)
	byID = hasResultsByID(mergeValueSearchLikeUIHas(primary, matched))
	require.Contains(t, byID, color.String())
	assert.Equal(t, int64(1), byID[color.String()].Count)
	assert.NotContains(t, byID, shape.String())
}

// TestHasFilterGetSubHasSelectedAugmentValueSearchIntegration verifies the same augment searchability for
// sub-has filters: an active sub-has selection, which has zero documents in the current search scope, is
// searchable by its display label or naming string, a non-matching term hides it, and outside a search it is
// force-shown at count 0.
func TestHasFilterGetSubHasSelectedAugmentValueSearchIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	parentProp := identifier.From("parentProp")
	parentTo := identifier.From("parentToValue").String()
	color := identifier.From("color")
	shape := identifier.From("shape")

	subHasClaim := func(prop identifier.Identifier, display string, naming []string) internalSearch.SubHasClaim {
		var propNaming map[string][]string
		if naming != nil {
			propNaming = map[string][]string{"en": naming}
		}
		return internalSearch.SubHasClaim{ //nolint:exhaustruct
			ParentProp: parentProp, ParentTo: parentTo,
			HasClaim: internalSearch.HasClaim{ //nolint:exhaustruct
				Prop: prop, PropDisplay: map[string]string{"en": display}, PropNaming: propNaming,
			},
		}
	}

	// subDoc1 has the color sub-property; subDoc2 has the shape sub-property (with a naming string). The search
	// scope below matches only subDoc1, so the selected shape has zero documents in scope yet exists globally.
	indexDocument(t, ctx, esClient, index, internalSearch.Document{ //nolint:exhaustruct
		ID: identifier.From("subHasDoc1"),
		Claims: internalSearch.ClaimTypes{ //nolint:exhaustruct
			SubHas: internalSearch.SubHasClaims{subHasClaim(color, "Color", nil)},
		},
	})
	indexDocument(t, ctx, esClient, index, internalSearch.Document{ //nolint:exhaustruct
		ID: identifier.From("subHasDoc2"),
		Claims: internalSearch.ClaimTypes{ //nolint:exhaustruct
			SubHas: internalSearch.SubHasClaims{subHasClaim(shape, "Shape", []string{"form"})},
		},
	})
	refreshIndex(t, ctx, esClient, index)

	enabledLanguages := internalSearch.EnabledLanguages(nil)
	restOfSearch := esdsl.NewNestedQuery(
		esdsl.NewTermQuery("claims.subHas.prop", esdsl.NewFieldValue().String(color.String())),
	).Path("claims.subHas")
	f := search.HasFilter{Props: []search.HasValue{{ID: shape}}}

	// The primary (q="") force-shows the selected shape at count 0 alongside the in-scope color; a value search
	// returns only the matching ids, which the frontend overlays on the primary (reproduced by
	// mergeValueSearchLikeUIHas).
	primary, _, errE := f.GetSubHas(ctx, getSearchService, restOfSearch, parentProp, nil, "", enabledLanguages)
	require.NoError(t, errE, "% -+#.1v", errE)
	byID := hasResultsByID(primary)
	require.Contains(t, byID, shape.String())
	assert.Equal(t, int64(0), byID[shape.String()].Count)
	require.Contains(t, byID, color.String())

	// Searching shape's display label surfaces it at count 0, even though it has no document in scope.
	matched, _, errE := f.GetSubHas(ctx, getSearchService, restOfSearch, parentProp, nil, "shape*", enabledLanguages)
	require.NoError(t, errE, "% -+#.1v", errE)
	byID = hasResultsByID(mergeValueSearchLikeUIHas(primary, matched))
	require.Contains(t, byID, shape.String())
	assert.Equal(t, int64(0), byID[shape.String()].Count)
	assert.NotContains(t, byID, color.String())

	// Searching shape by its naming string ("form") surfaces it too.
	matched, _, errE = f.GetSubHas(ctx, getSearchService, restOfSearch, parentProp, nil, "form*", enabledLanguages)
	require.NoError(t, errE, "% -+#.1v", errE)
	byID = hasResultsByID(mergeValueSearchLikeUIHas(primary, matched))
	require.Contains(t, byID, shape.String())

	// Searching "color" matches the real in-scope color sub-property and hides the selected shape.
	matched, _, errE = f.GetSubHas(ctx, getSearchService, restOfSearch, parentProp, nil, "color*", enabledLanguages)
	require.NoError(t, errE, "% -+#.1v", errE)
	byID = hasResultsByID(mergeValueSearchLikeUIHas(primary, matched))
	require.Contains(t, byID, color.String())
	assert.Equal(t, int64(1), byID[color.String()].Count)
	assert.NotContains(t, byID, shape.String())
}

// TestHasFilterGetMatchingPropIDsIntegration asserts the raw has filter value-search contract directly (not via
// the frontend overlay): a value search returns only the directly matching property ids, as id-only results
// (count 0), with metadata total equal to the number of returned ids and never a MissingValueID (the flat has
// facet has no missing bucket). A selected-but-0-doc property is returned by its display label or naming string;
// a real in-scope property is returned (and hides the selection) when its label matches; a non-matching term
// returns an empty set, while the in-scope property still shows under q="".
func TestHasFilterGetMatchingPropIDsIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	color := identifier.From("color")
	shape := identifier.From("shape")

	// doc1 has color; doc2 has shape (with a naming string). The search scope below matches only doc1, so the
	// selected shape has zero documents in scope yet exists globally.
	indexDocument(t, ctx, esClient, index, internalSearch.Document{ //nolint:exhaustruct
		ID: identifier.From("hasDoc1"),
		Claims: internalSearch.ClaimTypes{ //nolint:exhaustruct
			Has: internalSearch.HasClaims{{Prop: color, PropDisplay: map[string]string{"en": "Color"}}}, //nolint:exhaustruct
		},
	})
	indexDocument(t, ctx, esClient, index, internalSearch.Document{ //nolint:exhaustruct
		ID: identifier.From("hasDoc2"),
		Claims: internalSearch.ClaimTypes{ //nolint:exhaustruct
			Has: internalSearch.HasClaims{{ //nolint:exhaustruct
				Prop: shape, PropDisplay: map[string]string{"en": "Shape"}, PropNaming: map[string][]string{"en": {"form"}},
			}},
		},
	})
	refreshIndex(t, ctx, esClient, index)

	enabledLanguages := internalSearch.EnabledLanguages(nil)
	restOfSearch := esdsl.NewNestedQuery(
		esdsl.NewTermQuery("claims.has.prop", esdsl.NewFieldValue().String(color.String())),
	).Path("claims.has")
	f := search.HasFilter{Props: []search.HasValue{{ID: shape}}}

	// Searching the selected-but-0-doc property by its display label returns just its id, as an id-only result.
	matched, metadata, errE := f.Get(ctx, getSearchService, restOfSearch, "shape*", enabledLanguages)
	require.NoError(t, errE, "% -+#.1v", errE)
	for _, r := range matched {
		assert.Zero(t, r.Count)
	}
	ids := hasResultsByID(matched)
	assert.Contains(t, ids, shape.String())
	assert.NotContains(t, ids, color.String())
	assert.NotContains(t, ids, search.MissingValueID)
	assert.Equal(t, strconv.Itoa(len(matched)), metadata["total"])

	// Searching it by a naming string ("form") returns its id too.
	matched, _, errE = f.Get(ctx, getSearchService, restOfSearch, "form*", enabledLanguages)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Contains(t, hasResultsByID(matched), shape.String())

	// Searching the real in-scope property ("color") returns it and hides the selected shape.
	matched, _, errE = f.Get(ctx, getSearchService, restOfSearch, "color*", enabledLanguages)
	require.NoError(t, errE, "% -+#.1v", errE)
	ids = hasResultsByID(matched)
	assert.Contains(t, ids, color.String())
	assert.NotContains(t, ids, shape.String())

	// A non-matching term returns an empty set, while the in-scope color still shows under q="".
	matched, _, errE = f.Get(ctx, getSearchService, restOfSearch, "zzz*", enabledLanguages)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Empty(t, matched)
	primary, _, errE := f.Get(ctx, getSearchService, restOfSearch, "", enabledLanguages)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Contains(t, hasResultsByID(primary), color.String())
}

// TestHasFilterGetSubHasMatchingPropIDsIntegration asserts the raw sub-has filter value-search contract, the
// sub-has counterpart of TestHasFilterGetMatchingPropIDsIntegration.
func TestHasFilterGetSubHasMatchingPropIDsIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	parentProp := identifier.From("parentProp")
	parentTo := identifier.From("parentToValue").String()
	color := identifier.From("color")
	shape := identifier.From("shape")

	subHasClaim := func(prop identifier.Identifier, display string, naming []string) internalSearch.SubHasClaim {
		var propNaming map[string][]string
		if naming != nil {
			propNaming = map[string][]string{"en": naming}
		}
		return internalSearch.SubHasClaim{ //nolint:exhaustruct
			ParentProp: parentProp, ParentTo: parentTo,
			HasClaim: internalSearch.HasClaim{ //nolint:exhaustruct
				Prop: prop, PropDisplay: map[string]string{"en": display}, PropNaming: propNaming,
			},
		}
	}

	// subHasDoc1 has the color sub-property; subHasDoc2 has the shape sub-property (with a naming string). The
	// search scope below matches only subHasDoc1, so the selected shape has zero documents in scope yet exists
	// globally.
	indexDocument(t, ctx, esClient, index, internalSearch.Document{ //nolint:exhaustruct
		ID: identifier.From("subHasDoc1"),
		Claims: internalSearch.ClaimTypes{ //nolint:exhaustruct
			SubHas: internalSearch.SubHasClaims{subHasClaim(color, "Color", nil)},
		},
	})
	indexDocument(t, ctx, esClient, index, internalSearch.Document{ //nolint:exhaustruct
		ID: identifier.From("subHasDoc2"),
		Claims: internalSearch.ClaimTypes{ //nolint:exhaustruct
			SubHas: internalSearch.SubHasClaims{subHasClaim(shape, "Shape", []string{"form"})},
		},
	})
	refreshIndex(t, ctx, esClient, index)

	enabledLanguages := internalSearch.EnabledLanguages(nil)
	restOfSearch := esdsl.NewNestedQuery(
		esdsl.NewTermQuery("claims.subHas.prop", esdsl.NewFieldValue().String(color.String())),
	).Path("claims.subHas")
	f := search.HasFilter{Props: []search.HasValue{{ID: shape}}}

	// Searching the selected-but-0-doc property by its display label returns just its id, as an id-only result.
	matched, metadata, errE := f.GetSubHas(ctx, getSearchService, restOfSearch, parentProp, nil, "shape*", enabledLanguages)
	require.NoError(t, errE, "% -+#.1v", errE)
	for _, r := range matched {
		assert.Zero(t, r.Count)
	}
	ids := hasResultsByID(matched)
	assert.Contains(t, ids, shape.String())
	assert.NotContains(t, ids, color.String())
	assert.NotContains(t, ids, search.MissingValueID)
	assert.Equal(t, strconv.Itoa(len(matched)), metadata["total"])

	// Searching it by a naming string ("form") returns its id too.
	matched, _, errE = f.GetSubHas(ctx, getSearchService, restOfSearch, parentProp, nil, "form*", enabledLanguages)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Contains(t, hasResultsByID(matched), shape.String())

	// Searching the real in-scope sub-property ("color") returns it and hides the selected shape.
	matched, _, errE = f.GetSubHas(ctx, getSearchService, restOfSearch, parentProp, nil, "color*", enabledLanguages)
	require.NoError(t, errE, "% -+#.1v", errE)
	ids = hasResultsByID(matched)
	assert.Contains(t, ids, color.String())
	assert.NotContains(t, ids, shape.String())

	// A non-matching term returns an empty set, while the in-scope color still shows under q="".
	matched, _, errE = f.GetSubHas(ctx, getSearchService, restOfSearch, parentProp, nil, "zzz*", enabledLanguages)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Empty(t, matched)
	primary, _, errE := f.GetSubHas(ctx, getSearchService, restOfSearch, parentProp, nil, "", enabledLanguages)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Contains(t, hasResultsByID(primary), color.String())
}
