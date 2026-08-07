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

// namedHasRecord builds a has rel record for prop carrying an English display label and optional
// naming strings, so property-label matching can find it. sub is the record's Sub container.
func namedHasRecord(prop identifier.Identifier, display string, naming []string, sub *internalSearch.ClaimTypes) internalSearch.RelClaim {
	rec := simpleRelRecord(internalSearch.ClaimTypeHas, prop, sub)
	rec.PropDisplay = map[string]string{"en": display}
	if naming != nil {
		rec.PropNaming = map[string][]string{"en": naming}
	}
	return rec
}

func TestHasFilterGetIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	color := identifier.From("color")
	shape := identifier.From("shape")

	indexDocument(t, ctx, esClient, index, relDoc("hasDoc1", internalSearch.RelClaims{namedHasRecord(color, "Color", nil, nil)}))
	indexDocument(t, ctx, esClient, index, relDoc("hasDoc2", internalSearch.RelClaims{namedHasRecord(shape, "Shape", nil, nil)}))
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	enabledLanguages := internalSearch.EnabledLanguages(nil)
	f := search.HasFilter{Props: nil}

	// Without a value query both has-properties are listed.
	results, metadata, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), "", nil, searchLangs(enabledLanguages))
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.ElementsMatch(t, []search.HasFilterResult{
		{ID: color.String(), Count: 1},
		{ID: shape.String(), Count: 1},
	}, results)
	assert.Equal(t, "2", metadata["total"])

	// The value query (a prefix wildcard, as the frontend appends) narrows the facet to the matching property.
	results, metadata, errE = f.Get(ctx, getSearchService, session.ToQuery(nil), "col*", nil, searchLangs(enabledLanguages))
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []search.HasFilterResult{
		{ID: color.String(), Count: 1},
	}, results)
	assert.Equal(t, "1", metadata["total"])
}

// TestHasFilterGetPooledOnlyIntegration verifies that the pooled has facet lists ONLY properties whose
// facetable claims in scope are all has claims: a property with a ref or none rel record, an amount claim,
// or a time claim migrates to its own facet (where its has claims surface as the has-property special) and
// is subtracted here. A has claim with sub-claims still pools when the property is otherwise has-only.
func TestHasFilterGetPooledOnlyIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	color := identifier.From("color")
	shape := identifier.From("shape")
	size := identifier.From("size")
	date := identifier.From("date")
	mood := identifier.From("mood")
	shapeTarget := identifier.From("shapeTarget")
	subProp := identifier.From("subProp")
	subTarget := identifier.From("subTarget")

	amountFrom := 9.5
	amountTo := 10.5
	timeFrom := float64(1000)
	timeTo := float64(1001)

	// color is has-only; its has claim carries sub-claims, which does not affect pooling.
	indexDocument(t, ctx, esClient, index, relDoc("colorDoc", internalSearch.RelClaims{
		namedHasRecord(color, "Color", nil, relSub(refRecord(subProp, subTarget, nil))),
	}))
	// shape also has a ref record in scope: migrated out.
	indexDocument(t, ctx, esClient, index, relDoc("shapeHasDoc", internalSearch.RelClaims{namedHasRecord(shape, "Shape", nil, nil)}))
	indexDocument(t, ctx, esClient, index, relDoc("shapeRefDoc", internalSearch.RelClaims{refRecord(shape, shapeTarget, nil)}))
	// size also has an amount claim in scope: migrated out.
	indexDocument(t, ctx, esClient, index, relDoc("sizeHasDoc", internalSearch.RelClaims{namedHasRecord(size, "Size", nil, nil)}))
	indexDocument(t, ctx, esClient, index, claimsDoc("sizeAmountDoc", internalSearch.ClaimTypes{ //nolint:exhaustruct
		Amount: internalSearch.AmountClaims{amountRecord(size, nil, &amountFrom, &amountTo, nil)},
	}))
	// date also has a time claim in scope: migrated out.
	indexDocument(t, ctx, esClient, index, relDoc("dateHasDoc", internalSearch.RelClaims{namedHasRecord(date, "Date", nil, nil)}))
	indexDocument(t, ctx, esClient, index, claimsDoc("dateTimeDoc", internalSearch.ClaimTypes{ //nolint:exhaustruct
		Time: internalSearch.TimeClaims{timeRecord(date, &timeFrom, &timeTo, nil)},
	}))
	// mood also has a none record in scope: migrated out.
	indexDocument(t, ctx, esClient, index, relDoc("moodHasDoc", internalSearch.RelClaims{namedHasRecord(mood, "Mood", nil, nil)}))
	indexDocument(t, ctx, esClient, index, relDoc("moodNoneDoc", internalSearch.RelClaims{
		simpleRelRecord(internalSearch.ClaimTypeNone, mood, nil),
	}))
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})
	enabledLanguages := internalSearch.EnabledLanguages(nil)
	f := search.HasFilter{Props: nil}

	results, metadata, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), "", nil, searchLangs(enabledLanguages))
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

	indexDocument(t, ctx, esClient, index, relDoc("hasDoc1", internalSearch.RelClaims{namedHasRecord(color, "Color", nil, nil)}))
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})
	enabledLanguages := internalSearch.EnabledLanguages(nil)

	// shape is selected but no matching document has it, so the bucket aggregation drops it; it must still be
	// returned at count 0 alongside the matching color property.
	f := search.HasFilter{Props: []search.HasValue{{ID: shape}}}
	results, _, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), "", nil, searchLangs(enabledLanguages))
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

	indexDocument(t, ctx, esClient, index, relDoc("hasDoc1", internalSearch.RelClaims{namedHasRecord(color, "Color", nil, nil)}))
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})
	enabledLanguages := internalSearch.EnabledLanguages(nil)
	f := search.HasFilter{Props: []search.HasValue{{ID: shape}}}

	// Searching "col*" matches color but not the selected shape, so only color is returned (shape is not
	// force-shown during a search).
	results, _, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), "col*", nil, searchLangs(enabledLanguages))
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
	indexDocument(t, ctx, esClient, index, relDoc("hasDoc1", internalSearch.RelClaims{namedHasRecord(color, "Color", nil, nil)}))
	indexDocument(t, ctx, esClient, index, relDoc("hasDoc2", internalSearch.RelClaims{namedHasRecord(shape, "Shape", []string{"form"}, nil)}))
	refreshIndex(t, ctx, esClient, index)

	enabledLanguages := internalSearch.EnabledLanguages(nil)
	// The rest of the search matches only doc1, so the selected shape is not in scope.
	restOfSearch := esdsl.NewNestedQuery(
		esdsl.NewTermQuery("claims.rel.prop", esdsl.NewFieldValue().String(color.String())),
	).Path("claims.rel")
	f := search.HasFilter{Props: []search.HasValue{{ID: shape}}}

	// Searching shape's display label surfaces it at count 0, even though it has no document in scope.
	results, _, errE := f.Get(ctx, getSearchService, restOfSearch, "shape*", nil, searchLangs(enabledLanguages))
	require.NoError(t, errE, "% -+#.1v", errE)
	byID := hasResultsByID(results)
	require.Contains(t, byID, shape.String())
	assert.Equal(t, int64(0), byID[shape.String()].Count)
	assert.NotContains(t, byID, color.String())

	// Searching shape by its naming string ("form") surfaces it too, since the augment uses the same prop-label
	// matcher real properties use.
	results, _, errE = f.Get(ctx, getSearchService, restOfSearch, "form*", nil, searchLangs(enabledLanguages))
	require.NoError(t, errE, "% -+#.1v", errE)
	byID = hasResultsByID(results)
	require.Contains(t, byID, shape.String())

	// Searching "color" matches the real in-scope color property and hides the selected shape.
	results, _, errE = f.Get(ctx, getSearchService, restOfSearch, "color*", nil, searchLangs(enabledLanguages))
	require.NoError(t, errE, "% -+#.1v", errE)
	byID = hasResultsByID(results)
	require.Contains(t, byID, color.String())
	assert.Equal(t, int64(1), byID[color.String()].Count)
	assert.NotContains(t, byID, shape.String())

	// Outside a value search shape is force-shown at count 0 alongside the in-scope color.
	results, _, errE = f.Get(ctx, getSearchService, restOfSearch, "", nil, searchLangs(enabledLanguages))
	require.NoError(t, errE, "% -+#.1v", errE)
	byID = hasResultsByID(results)
	require.Contains(t, byID, shape.String())
	assert.Equal(t, int64(0), byID[shape.String()].Count)
	require.Contains(t, byID, color.String())
	assert.Equal(t, int64(1), byID[color.String()].Count)
}

// TestHasFilterGetSubHasPooledOnlyIntegration verifies the pooled sub-has facet one level down: it lists the
// has-properties nested under qualifying parent claims whose only facetable sub-claims are has records; a
// sub-property with a non-has sub rel record or a sub amount claim migrates out, and sub-claims under a
// different parent property are out of scope entirely.
func TestHasFilterGetSubHasPooledOnlyIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	parentProp := identifier.From("parentProp")
	parentTarget := identifier.From("parentToValue")
	otherParentProp := identifier.From("otherParentProp")
	color := identifier.From("color")
	mixed := identifier.From("mixed")
	size := identifier.From("size")
	otherColor := identifier.From("otherColor")
	mixedTarget := identifier.From("mixedTarget")

	amountFrom := 9.5
	amountTo := 10.5

	// color is has-only under parentProp: pooled.
	indexDocument(t, ctx, esClient, index, relDoc("subHasDoc1", internalSearch.RelClaims{
		refRecord(parentProp, parentTarget, relSub(namedHasRecord(color, "Color", nil, nil))),
	}))
	// mixed also has a ref sub record under a qualifying parent claim: migrated out.
	indexDocument(t, ctx, esClient, index, relDoc("subHasDoc2", internalSearch.RelClaims{
		refRecord(parentProp, parentTarget, relSub(
			namedHasRecord(mixed, "Mixed", nil, nil),
			refRecord(mixed, mixedTarget, nil),
		)),
	}))
	// size also has a sub amount claim under a qualifying parent claim: migrated out.
	indexDocument(t, ctx, esClient, index, relDoc("subHasDoc3", internalSearch.RelClaims{
		refRecord(parentProp, parentTarget, &internalSearch.ClaimTypes{ //nolint:exhaustruct
			Rel:    internalSearch.RelClaims{namedHasRecord(size, "Size", nil, nil)},
			Amount: internalSearch.AmountClaims{amountRecord(size, nil, &amountFrom, &amountTo, nil)},
		}),
	}))
	// otherColor is a has sub record under a DIFFERENT parent property: out of this facet's scope.
	indexDocument(t, ctx, esClient, index, relDoc("subHasDoc4", internalSearch.RelClaims{
		refRecord(otherParentProp, parentTarget, relSub(namedHasRecord(otherColor, "Other color", nil, nil))),
	}))
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})
	enabledLanguages := internalSearch.EnabledLanguages(nil)
	f := search.HasFilter{Props: nil}
	parentCtx := session.ParentContextFor(parentProp, identifier.Identifier{})

	results, metadata, errE := f.GetSubHas(ctx, getSearchService, session.ToQuery(nil), parentCtx, "", nil, searchLangs(enabledLanguages))
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []search.HasFilterResult{
		{ID: color.String(), Count: 1},
	}, results)
	assert.Equal(t, "1", metadata["total"])
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
	parentTarget := identifier.From("parentToValue")
	color := identifier.From("color")
	shape := identifier.From("shape")

	// subDoc1 has the color sub-property; subDoc2 has the shape sub-property (with a naming string). The search
	// scope below matches only subDoc1, so the selected shape has zero documents in scope yet exists globally.
	indexDocument(t, ctx, esClient, index, relDoc("subHasDoc1", internalSearch.RelClaims{
		refRecord(parentProp, parentTarget, relSub(namedHasRecord(color, "Color", nil, nil))),
	}))
	indexDocument(t, ctx, esClient, index, relDoc("subHasDoc2", internalSearch.RelClaims{
		refRecord(parentProp, parentTarget, relSub(namedHasRecord(shape, "Shape", []string{"form"}, nil))),
	}))
	refreshIndex(t, ctx, esClient, index)

	enabledLanguages := internalSearch.EnabledLanguages(nil)
	restOfSearch := esdsl.NewNestedQuery(
		esdsl.NewNestedQuery(
			esdsl.NewTermQuery("claims.rel.sub.rel.prop", esdsl.NewFieldValue().String(color.String())),
		).Path("claims.rel.sub.rel"),
	).Path("claims.rel")
	f := search.HasFilter{Props: []search.HasValue{{ID: shape}}}
	var sessionData search.SessionData
	parentCtx := sessionData.ParentContextFor(parentProp, identifier.Identifier{})

	// Searching shape's display label surfaces it at count 0, even though it has no document in scope.
	results, _, errE := f.GetSubHas(ctx, getSearchService, restOfSearch, parentCtx, "shape*", nil, searchLangs(enabledLanguages))
	require.NoError(t, errE, "% -+#.1v", errE)
	byID := hasResultsByID(results)
	require.Contains(t, byID, shape.String())
	assert.Equal(t, int64(0), byID[shape.String()].Count)
	assert.NotContains(t, byID, color.String())

	// Searching shape by its naming string ("form") surfaces it too.
	results, _, errE = f.GetSubHas(ctx, getSearchService, restOfSearch, parentCtx, "form*", nil, searchLangs(enabledLanguages))
	require.NoError(t, errE, "% -+#.1v", errE)
	byID = hasResultsByID(results)
	require.Contains(t, byID, shape.String())

	// Searching "color" matches the real in-scope color sub-property and hides the selected shape.
	results, _, errE = f.GetSubHas(ctx, getSearchService, restOfSearch, parentCtx, "color*", nil, searchLangs(enabledLanguages))
	require.NoError(t, errE, "% -+#.1v", errE)
	byID = hasResultsByID(results)
	require.Contains(t, byID, color.String())
	assert.Equal(t, int64(1), byID[color.String()].Count)
	assert.NotContains(t, byID, shape.String())

	// Outside a value search shape is force-shown at count 0 alongside the in-scope color.
	results, _, errE = f.GetSubHas(ctx, getSearchService, restOfSearch, parentCtx, "", nil, searchLangs(enabledLanguages))
	require.NoError(t, errE, "% -+#.1v", errE)
	byID = hasResultsByID(results)
	require.Contains(t, byID, shape.String())
	assert.Equal(t, int64(0), byID[shape.String()].Count)
	require.Contains(t, byID, color.String())
}

// TestHasFilterGetHiddenIntegration verifies that hidden facet properties are left out of the pooled
// has facet's value list, at the top level and one level down, while a hidden property the filter
// itself selects stays listed with its real count so the selection remains visible and deselectable.
func TestHasFilterGetHiddenIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	hiddenProp := identifier.From("hiddenHasProp")
	visibleProp := identifier.From("visibleHasProp")
	parentProp := identifier.From("hasParentProp")
	parentTarget := identifier.From("hasParentTarget")

	// Both properties as top-level has claims, and both as has sub-claims under one parent property.
	indexDocument(t, ctx, esClient, index, relDoc("hiddenHasDoc1", internalSearch.RelClaims{
		namedHasRecord(hiddenProp, "Hidden", nil, nil),
		refRecord(parentProp, parentTarget, relSub(namedHasRecord(hiddenProp, "Hidden", nil, nil))),
	}))
	indexDocument(t, ctx, esClient, index, relDoc("hiddenHasDoc2", internalSearch.RelClaims{
		namedHasRecord(visibleProp, "Visible", nil, nil),
		refRecord(parentProp, parentTarget, relSub(namedHasRecord(visibleProp, "Visible", nil, nil))),
	}))
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})
	enabledLanguages := internalSearch.EnabledLanguages(nil)
	hidden := map[string]bool{hiddenProp.String(): true}

	// Top level: the hidden property is not listed and not counted in the total.
	unselected := search.HasFilter{Props: nil}
	results, metadata, errE := unselected.Get(ctx, getSearchService, session.ToQuery(nil), "", hidden, searchLangs(enabledLanguages))
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []search.HasFilterResult{{ID: visibleProp.String(), Count: 1}}, results)
	assert.Equal(t, "1", metadata["total"])

	// Selecting the hidden property keeps it listed, with its real count rather than the zero count a
	// force-shown selection gets.
	selected := search.HasFilter{Props: []search.HasValue{{ID: hiddenProp}}}
	results, metadata, errE = selected.Get(ctx, getSearchService, session.ToQuery(nil), "", hidden, searchLangs(enabledLanguages))
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.ElementsMatch(t, []search.HasFilterResult{
		{ID: hiddenProp.String(), Count: 1},
		{ID: visibleProp.String(), Count: 1},
	}, results)
	assert.Equal(t, "2", metadata["total"])

	// One level down the same holds for the sub-has properties under the parent property.
	parentCtx := session.ParentContextFor(parentProp, identifier.Identifier{})
	results, metadata, errE = unselected.GetSubHas(ctx, getSearchService, session.ToQuery(nil), parentCtx, "", hidden, searchLangs(enabledLanguages))
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []search.HasFilterResult{{ID: visibleProp.String(), Count: 1}}, results)
	assert.Equal(t, "1", metadata["total"])

	results, metadata, errE = selected.GetSubHas(ctx, getSearchService, session.ToQuery(nil), parentCtx, "", hidden, searchLangs(enabledLanguages))
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.ElementsMatch(t, []search.HasFilterResult{
		{ID: hiddenProp.String(), Count: 1},
		{ID: visibleProp.String(), Count: 1},
	}, results)
	assert.Equal(t, "2", metadata["total"])
}
