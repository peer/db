package search_test

import (
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

	// Without a value query both has-properties are listed.
	results, metadata, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), "", enabledLanguages)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.ElementsMatch(t, []search.HasFilterResult{
		{ID: color.String(), Count: 1},
		{ID: shape.String(), Count: 1},
	}, results)
	assert.Equal(t, "2", metadata["total"])

	// The value query (a prefix wildcard, as the frontend appends) narrows the facet to the matching property.
	results, metadata, errE = f.Get(ctx, getSearchService, session.ToQuery(nil), "col*", enabledLanguages)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []search.HasFilterResult{
		{ID: color.String(), Count: 1},
	}, results)
	assert.Equal(t, "1", metadata["total"])
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

	// Searching "col*" matches color but not the selected shape, so only color is returned (shape is not
	// force-shown during a search).
	results, _, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), "col*", enabledLanguages)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []search.HasFilterResult{
		{ID: color.String(), Count: 1},
	}, results)
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

	// Searching shape's display label surfaces it at count 0, even though it has no document in scope.
	results, _, errE := f.Get(ctx, getSearchService, restOfSearch, "shape*", enabledLanguages)
	require.NoError(t, errE, "% -+#.1v", errE)
	byID := hasResultsByID(results)
	require.Contains(t, byID, shape.String())
	assert.Equal(t, int64(0), byID[shape.String()].Count)
	assert.NotContains(t, byID, color.String())

	// Searching shape by its naming string ("form") surfaces it too, since the augment uses the same prop-label
	// matcher real properties use.
	results, _, errE = f.Get(ctx, getSearchService, restOfSearch, "form*", enabledLanguages)
	require.NoError(t, errE, "% -+#.1v", errE)
	byID = hasResultsByID(results)
	require.Contains(t, byID, shape.String())

	// Searching "color" matches the real in-scope color property and hides the selected shape.
	results, _, errE = f.Get(ctx, getSearchService, restOfSearch, "color*", enabledLanguages)
	require.NoError(t, errE, "% -+#.1v", errE)
	byID = hasResultsByID(results)
	require.Contains(t, byID, color.String())
	assert.Equal(t, int64(1), byID[color.String()].Count)
	assert.NotContains(t, byID, shape.String())

	// Outside a value search shape is force-shown at count 0 alongside the in-scope color.
	results, _, errE = f.Get(ctx, getSearchService, restOfSearch, "", enabledLanguages)
	require.NoError(t, errE, "% -+#.1v", errE)
	byID = hasResultsByID(results)
	require.Contains(t, byID, shape.String())
	assert.Equal(t, int64(0), byID[shape.String()].Count)
	require.Contains(t, byID, color.String())
	assert.Equal(t, int64(1), byID[color.String()].Count)
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

	// Searching shape's display label surfaces it at count 0, even though it has no document in scope.
	results, _, errE := f.GetSubHas(ctx, getSearchService, restOfSearch, parentProp, nil, "shape*", enabledLanguages)
	require.NoError(t, errE, "% -+#.1v", errE)
	byID := hasResultsByID(results)
	require.Contains(t, byID, shape.String())
	assert.Equal(t, int64(0), byID[shape.String()].Count)
	assert.NotContains(t, byID, color.String())

	// Searching shape by its naming string ("form") surfaces it too.
	results, _, errE = f.GetSubHas(ctx, getSearchService, restOfSearch, parentProp, nil, "form*", enabledLanguages)
	require.NoError(t, errE, "% -+#.1v", errE)
	byID = hasResultsByID(results)
	require.Contains(t, byID, shape.String())

	// Searching "color" matches the real in-scope color sub-property and hides the selected shape.
	results, _, errE = f.GetSubHas(ctx, getSearchService, restOfSearch, parentProp, nil, "color*", enabledLanguages)
	require.NoError(t, errE, "% -+#.1v", errE)
	byID = hasResultsByID(results)
	require.Contains(t, byID, color.String())
	assert.Equal(t, int64(1), byID[color.String()].Count)
	assert.NotContains(t, byID, shape.String())

	// Outside a value search shape is force-shown at count 0 alongside the in-scope color.
	results, _, errE = f.GetSubHas(ctx, getSearchService, restOfSearch, parentProp, nil, "", enabledLanguages)
	require.NoError(t, errE, "% -+#.1v", errE)
	byID = hasResultsByID(results)
	require.Contains(t, byID, shape.String())
	assert.Equal(t, int64(0), byID[shape.String()].Count)
	require.Contains(t, byID, color.String())
}
