package search_test

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/identifier"

	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
	"gitlab.com/peerdb/peerdb/search"
)

// TestRefFilterGetSpecialSearchIntegration verifies that a value query matching a special value label
// (missing, none, unknown, has property) surfaces exactly that special row in a reference facet, even
// though no real value name matches the typed text and the facet's own property name does not match
// either (so the whole-facet includeSpecials gate does not fire).
func TestRefFilterGetSpecialSearchIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	refProp := identifier.From("refProp")
	target1 := identifier.From("target1")
	subProp := identifier.From("subProp")
	subTarget := identifier.From("subTarget")

	indexDocument(t, ctx, esClient, index, relDoc("refClaimTypeDoc", internalSearch.RelClaims{refRecord(refProp, target1, nil)}))
	indexDocument(t, ctx, esClient, index, relDoc("hasClaimTypeDoc", internalSearch.RelClaims{
		simpleRelRecord(internalSearch.ClaimTypeHas, refProp, relSub(refRecord(subProp, subTarget, nil))),
	}))
	indexDocument(t, ctx, esClient, index, relDoc("unknownClaimTypeDoc", internalSearch.RelClaims{
		simpleRelRecord(internalSearch.ClaimTypeUnknown, refProp, nil),
	}))
	indexDocument(t, ctx, esClient, index, relDoc("noneClaimTypeDoc", internalSearch.RelClaims{
		simpleRelRecord(internalSearch.ClaimTypeNone, refProp, nil),
	}))
	indexDocument(t, ctx, esClient, index, claimsDoc("missingDoc", internalSearch.ClaimTypes{}))
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})
	f := search.RefFilter{To: nil, Direct: nil}

	// Each special label narrows the facet to just its own row (the prefix wildcard is what the frontend
	// appends). The real value target1 has no name, so the text never matches it.
	for _, tt := range []struct {
		query string
		id    string
	}{
		{"none*", search.NoneValueID},
		{"unknown*", search.UnknownValueID},
		{"missing*", search.MissingValueID},
		{"has property*", search.HasPropertyValueID},
	} {
		results, metadata, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), refProp, nil, tt.query, nil, nil)
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.Equal(t, []search.RefFilterResult{
			{ID: tt.id, Count: 1, ChildCount: 0, ChildCountAtLeast: false, Paths: nil},
		}, results, "query %q", tt.query)
		assert.Equal(t, "1", metadata["total"], "query %q", tt.query)
	}
}

// TestRefFilterGetDirectSearchIntegration verifies that a value query matching the "direct" label
// surfaces every value's direct entry across the whole facet (with the value it hangs under), not only
// the text-matched values, so the per-value direct rows are reachable by searching "direct".
func TestRefFilterGetDirectSearchIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	refProp := identifier.From("refProp")
	hierProp := identifier.From("hierProp")
	artist := identifier.From("artist")
	painter := identifier.From("painter")
	sculptor := identifier.From("sculptor")

	// Hierarchy: artist > {painter, sculptor}. Only artist has narrower values and a most-specific
	// (direct) document count, so only artist carries a synthetic direct entry.
	artistPath := hierProp.String() + ":" + artist.String()
	painterPath := hierProp.String() + ":" + artist.String() + "/" + painter.String()
	sculptorPath := hierProp.String() + ":" + artist.String() + "/" + sculptor.String()

	painterClaims := internalSearch.RelClaims{
		hierRelRecord(refProp, painter, []string{painterPath}, true),
		hierRelRecord(refProp, artist, []string{artistPath}, false),
	}
	sculptorClaims := internalSearch.RelClaims{
		hierRelRecord(refProp, sculptor, []string{sculptorPath}, true),
		hierRelRecord(refProp, artist, []string{artistPath}, false),
	}
	artistClaims := internalSearch.RelClaims{
		hierRelRecord(refProp, artist, []string{artistPath}, true),
	}

	indexDocument(t, ctx, esClient, index, relDoc("painterDoc1", painterClaims))
	indexDocument(t, ctx, esClient, index, relDoc("painterDoc2", painterClaims))
	indexDocument(t, ctx, esClient, index, relDoc("sculptorDoc1", sculptorClaims))
	indexDocument(t, ctx, esClient, index, relDoc("sculptorDoc2", sculptorClaims))
	indexDocument(t, ctx, esClient, index, relDoc("sculptorDoc3", sculptorClaims))
	indexDocument(t, ctx, esClient, index, relDoc("artistDoc1", artistClaims))
	indexDocument(t, ctx, esClient, index, relDoc("artistDoc2", artistClaims))
	indexDocument(t, ctx, esClient, index, relDoc("artistDoc3", artistClaims))
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})
	f := search.RefFilter{To: nil, Direct: nil}

	// Searching "direct" surfaces the artist direct entry (3 artist-only documents) together with its
	// parent value artist (8 documents), and nothing else: the leaves carry no direct entry and no value
	// name matches the text.
	results, metadata, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), refProp, nil, "direct*", nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []search.RefFilterResult{
		{ID: artist.String(), Count: 8, ChildCount: 0, ChildCountAtLeast: false, Paths: nil},
		{ID: search.DirectRefFilterPrefix + artist.String(), Count: 3, ChildCount: 0, ChildCountAtLeast: false, Paths: [][]string{{artist.String()}}},
	}, results)
	assert.Equal(t, "2", metadata["total"])
}

// TestRefFilterGetSubRefSpecialSearchIntegration verifies that matching a special value label surfaces
// that special's row in a sub facet (parentProp > subProp) too, through the same per-facet gate as the
// top-level facet.
func TestRefFilterGetSubRefSpecialSearchIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	parentProp := identifier.From("parentProp")
	parentTarget := identifier.From("parentToValue")
	subProp := identifier.From("subProp")
	valueA := identifier.From("valueA")

	// A sub ref value, a sub none statement, and a parent claim missing the sub property.
	indexDocument(t, ctx, esClient, index, relDoc("subValueDoc", internalSearch.RelClaims{
		refRecord(parentProp, parentTarget, relSub(refRecord(subProp, valueA, nil))),
	}))
	indexDocument(t, ctx, esClient, index, relDoc("subNoneDoc", internalSearch.RelClaims{
		refRecord(parentProp, parentTarget, relSub(simpleRelRecord(internalSearch.ClaimTypeNone, subProp, nil))),
	}))
	indexDocument(t, ctx, esClient, index, relDoc("subMissingDoc", internalSearch.RelClaims{
		refRecord(parentProp, parentTarget, nil),
	}))
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})
	f := search.RefFilter{To: nil, Direct: nil}
	parentCtx := session.ParentContextFor(parentProp, subProp)

	for _, tt := range []struct {
		query string
		id    string
	}{
		{"none*", search.NoneValueID},
		{"missing*", search.MissingValueID},
	} {
		results, _, errE := f.GetSubRef(ctx, getSearchService, session.ToQuery(nil), subProp, parentCtx, nil, tt.query, nil, nil)
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.Equal(t, []search.RefFilterResult{
			{ID: tt.id, Count: 1, ChildCount: 0, ChildCountAtLeast: false, Paths: nil},
		}, results, "query %q", tt.query)
	}
}

// TestFiltersGetSpecialSearchDiscoveryIntegration verifies that the discovery pass keeps the facets a
// matched special label applies to: none/unknown/has property keep only the properties with that rel
// claim type (exact), while missing is broad (every facet with missing documents). The
// value-query-independent total stays stable across all queries.
func TestFiltersGetSpecialSearchDiscoveryIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	propNone := identifier.From("propNone")
	propPlain := identifier.From("propPlain")
	targetN := identifier.From("targetN")
	targetP := identifier.From("targetP")

	// propNone is a reference facet that also carries a none statement; propPlain is a plain reference
	// facet. A document stating neither leaves both properties missing.
	indexDocument(t, ctx, esClient, index, relDoc("noneRefDoc", internalSearch.RelClaims{refRecord(propNone, targetN, nil)}))
	indexDocument(t, ctx, esClient, index, relDoc("noneDoc", internalSearch.RelClaims{
		simpleRelRecord(internalSearch.ClaimTypeNone, propNone, nil),
	}))
	indexDocument(t, ctx, esClient, index, relDoc("plainRefDoc", internalSearch.RelClaims{refRecord(propPlain, targetP, nil)}))
	indexDocument(t, ctx, esClient, index, claimsDoc("neitherDoc", internalSearch.ClaimTypes{}))
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	has := func(results []search.FilterResult, prop identifier.Identifier) bool {
		for _, r := range results {
			if r.Type == "ref" && len(r.Props) > 0 && r.Props[0] == prop.String() {
				return true
			}
		}
		return false
	}

	// Baseline: both facets are offered, total is two.
	results, metadata, errE := search.FiltersGet(ctx, getSearchService, session, nil, "", nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.True(t, has(results, propNone))
	assert.True(t, has(results, propPlain))
	assert.Equal(t, "2", metadata["total"])

	// Searching "none" keeps only the property that has a none statement; the total stays two.
	results, metadata, errE = search.FiltersGet(ctx, getSearchService, session, nil, "none*", nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.True(t, has(results, propNone))
	assert.False(t, has(results, propPlain))
	assert.Equal(t, "2", metadata["total"])

	// Searching "unknown" keeps neither (no unknown statements, and the text matches no name or value).
	results, _, errE = search.FiltersGet(ctx, getSearchService, session, nil, "unknown*", nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.False(t, has(results, propNone))
	assert.False(t, has(results, propPlain))

	// Searching "missing" is broad: both facets have missing documents, so both are kept.
	results, metadata, errE = search.FiltersGet(ctx, getSearchService, session, nil, "missing*", nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.True(t, has(results, propNone))
	assert.True(t, has(results, propPlain))
	assert.Equal(t, "2", metadata["total"])

	// Searching "direct" keeps neither: both properties are flat (no hierarchy), so neither can carry a
	// direct entry. The direct gate is selective (hierarchy only), not every ref facet.
	results, _, errE = search.FiltersGet(ctx, getSearchService, session, nil, "direct*", nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.False(t, has(results, propNone))
	assert.False(t, has(results, propPlain))
}

// TestFiltersGetSpecialBeyondCapValueQueryIntegration verifies the beyond-cap special discovery pass:
// a property carrying a matched claim-type special (none, or direct via a non-leaf ref record) but
// ranking beyond the unfiltered discovery's Size cap by document count is still surfaced by searching
// that special's label, mirroring how a text match reaches beyond the cap. The value-query-independent
// total stays the saturated lower bound.
func TestFiltersGetSpecialBeyondCapValueQueryIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	// MaxResultsCount filler ref properties, each present in three documents, so they all rank ahead of
	// the target properties (document counts 1 and 2) and fill the unfiltered discovery cap.
	fillers := make(internalSearch.RelClaims, 0, search.MaxResultsCount)
	for i := range search.MaxResultsCount {
		fillers = append(fillers, refRecord(identifier.From("filler", strconv.Itoa(i)), identifier.From("fillerTarget", strconv.Itoa(i)), nil))
	}
	indexDocument(t, ctx, esClient, index, relDoc("fillerDoc1", fillers))
	indexDocument(t, ctx, esClient, index, relDoc("fillerDoc2", fillers))
	indexDocument(t, ctx, esClient, index, relDoc("fillerDoc3", fillers))

	// A none-only property, one document, beyond the cap and with no distinctive name to match by text.
	noneBeyondProp := identifier.From("noneBeyondProp")
	indexDocument(t, ctx, esClient, index, relDoc("noneBeyondDoc", internalSearch.RelClaims{
		simpleRelRecord(internalSearch.ClaimTypeNone, noneBeyondProp, nil),
	}))

	// A hierarchical property (artist > painter) with a real artist direct entry, two documents, beyond
	// the cap. The non-leaf artist record is what the direct special pass filters on.
	directBeyondProp := identifier.From("directBeyondProp")
	artist := identifier.From("artist")
	painter := identifier.From("painter")
	hierProp := identifier.From("hierProp")
	artistPath := hierProp.String() + ":" + artist.String()
	painterPath := hierProp.String() + ":" + artist.String() + "/" + painter.String()
	indexDocument(t, ctx, esClient, index, relDoc("directBeyondDoc1", internalSearch.RelClaims{
		hierRelRecord(directBeyondProp, painter, []string{painterPath}, true),
		hierRelRecord(directBeyondProp, artist, []string{artistPath}, false),
	}))
	indexDocument(t, ctx, esClient, index, relDoc("directBeyondDoc2", internalSearch.RelClaims{
		hierRelRecord(directBeyondProp, artist, []string{artistPath}, true),
	}))
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	has := func(results []search.FilterResult, prop identifier.Identifier) bool {
		for _, r := range results {
			if r.Type == "ref" && len(r.Props) == 1 && r.Props[0] == prop.String() {
				return true
			}
		}
		return false
	}

	// Searching "none" surfaces the beyond-cap none-only facet (and not the direct one).
	results, metadata, errE := search.FiltersGet(ctx, getSearchService, session, nil, "none*", nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.True(t, has(results, noneBeyondProp), "beyond-cap none facet surfaced by the none label")
	assert.False(t, has(results, directBeyondProp))
	assert.Equal(t, strconv.Itoa(search.MaxResultsCount)+"+", metadata["total"])

	// Searching "direct" surfaces the beyond-cap hierarchical facet (and not the none one).
	results, _, errE = search.FiltersGet(ctx, getSearchService, session, nil, "direct*", nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.True(t, has(results, directBeyondProp), "beyond-cap direct facet surfaced by the direct label")
	assert.False(t, has(results, noneBeyondProp))
}

// TestFiltersGetSubSpecialBeyondCapValueQueryIntegration verifies the sub-level beyond-cap special
// pass: a sub-property carrying a matched claim-type special (none) but ranking beyond the per-parent
// sub cap by document count is surfaced by searching that special's label, just as a sub-property name
// text match reaches beyond the sub cap.
func TestFiltersGetSubSpecialBeyondCapValueQueryIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	locationProp := identifier.From("locationProp")
	venue := identifier.From("venue")

	// MaxResultsCount filler sub-properties under one parent claim fill the sub cap ahead of the target.
	fillerSubs := make(internalSearch.RelClaims, 0, search.MaxResultsCount)
	for i := range search.MaxResultsCount {
		fillerSubs = append(fillerSubs, refRecord(identifier.From("subFiller", strconv.Itoa(i)), identifier.From("subFillerTarget", strconv.Itoa(i)), nil))
	}
	fillerParent := refRecord(locationProp, venue, relSub(fillerSubs...))
	indexDocument(t, ctx, esClient, index, relDoc("subFillerDoc1", internalSearch.RelClaims{fillerParent}))
	indexDocument(t, ctx, esClient, index, relDoc("subFillerDoc2", internalSearch.RelClaims{fillerParent}))

	// The target sub-property carries a none sub-claim, beyond the sub cap, with no distinctive name.
	noneSubProp := identifier.From("noneSubProp")
	indexDocument(t, ctx, esClient, index, relDoc("subNoneDoc", internalSearch.RelClaims{
		refRecord(locationProp, venue, relSub(simpleRelRecord(internalSearch.ClaimTypeNone, noneSubProp, nil))),
	}))
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	results, _, errE := search.FiltersGet(ctx, getSearchService, session, nil, "none*", nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	found := false
	for _, r := range results {
		if r.Type == "ref" && len(r.Props) == 2 && r.Props[0] == locationProp.String() && r.Props[1] == noneSubProp.String() {
			found = true
		}
	}
	assert.True(t, found, "the beyond-sub-cap none sub facet was surfaced by the none label")
}

// TestFiltersGetSubSpecialBeyondParentCapValueQueryIntegration verifies the beyond-parent-cap special
// pass: a parent property ranking beyond the parent-property cap, whose only match is a special
// sub-claim (a none sub statement), still surfaces its sub facets, just as a matching sub-property name
// surfaces them beyond the parent cap.
func TestFiltersGetSubSpecialBeyondParentCapValueQueryIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	// MaxResultsCount filler parent properties, each with a ref sub-claim, fill the parent-property cap.
	fillerParents := make(internalSearch.RelClaims, 0, search.MaxResultsCount)
	for i := range search.MaxResultsCount {
		sub := refRecord(identifier.From("pFillerSub", strconv.Itoa(i)), identifier.From("pFillerSubTarget", strconv.Itoa(i)), nil)
		fillerParents = append(fillerParents, refRecord(identifier.From("pFiller", strconv.Itoa(i)), identifier.From("pFillerTarget", strconv.Itoa(i)), relSub(sub)))
	}
	indexDocument(t, ctx, esClient, index, relDoc("pFillerDoc1", fillerParents))
	indexDocument(t, ctx, esClient, index, relDoc("pFillerDoc2", fillerParents))

	// The target parent (one document, beyond the parent cap) carries a none sub-claim, no distinctive name.
	targetParentProp := identifier.From("targetParentProp")
	noneSubProp := identifier.From("noneSubProp")
	indexDocument(t, ctx, esClient, index, relDoc("targetParentDoc", internalSearch.RelClaims{
		refRecord(targetParentProp, identifier.From("targetParentTarget"), relSub(simpleRelRecord(internalSearch.ClaimTypeNone, noneSubProp, nil))),
	}))
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	results, _, errE := search.FiltersGet(ctx, getSearchService, session, nil, "none*", nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	found := false
	for _, r := range results {
		if r.Type == "ref" && len(r.Props) == 2 && r.Props[0] == targetParentProp.String() && r.Props[1] == noneSubProp.String() {
			found = true
		}
	}
	assert.True(t, found, "the none sub facet under the beyond-parent-cap property was surfaced by the none label")
}

// TestHasFilterGetHasPropertySearchIntegration verifies that a value query matching the "has property"
// label shows the whole pooled has facet (every has-property), the same facet discovery keeps on that
// special, rather than narrowing the list by the typed text as a plain property-name search would.
func TestHasFilterGetHasPropertySearchIntegration(t *testing.T) {
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

	// A plain property-name search narrows to the matching property.
	results, _, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), "col*", nil, searchLangs(enabledLanguages))
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []search.HasFilterResult{{ID: color.String(), Count: 1}}, results)

	// The "has property" label shows the whole facet, both properties.
	results, metadata, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), "has property*", nil, searchLangs(enabledLanguages))
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.ElementsMatch(t, []search.HasFilterResult{
		{ID: color.String(), Count: 1},
		{ID: shape.String(), Count: 1},
	}, results)
	assert.Equal(t, "2", metadata["total"])
}
