package search_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/identifier"

	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
	"gitlab.com/peerdb/peerdb/search"
)

// subClaims builds a sub container with the given ref sub records and time sub records.
func subClaims(refs internalSearch.RelClaims, times internalSearch.TimeClaims) *internalSearch.ClaimTypes {
	return &internalSearch.ClaimTypes{ //nolint:exhaustruct
		Rel:  refs,
		Time: times,
	}
}

// periodRecord builds a time sub record for prop spanning [1000, 2000).
func periodRecord(prop identifier.Identifier) internalSearch.TimeClaim {
	from := float64(1000)
	to := float64(2000)
	return timeRecord(prop, &from, &to, nil)
}

// periodFilter builds a time filter selecting a range inside periodRecord's span.
func periodFilter() *search.TimeFilter {
	gte := float64(1200)
	lte := float64(1800)
	return &search.TimeFilter{Gte: &gte, Lte: &lte, Exists: false}
}

// TestCorrelationSameParentIntegration verifies that sub filters sharing a parent property match on
// the same parent claim: a document carrying the artist under one location claim and the period
// under another does not match, while a document carrying both under one location claim does.
func TestCorrelationSameParentIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	locationProp := identifier.From("locationProp")
	artistProp := identifier.From("artistProp")
	timeProp := identifier.From("periodProp")
	artist := identifier.From("artistA")
	venue1 := identifier.From("venue1")
	venue2 := identifier.From("venue2")

	// The artist is recorded under venue1 and the period under venue2.
	indexDocument(t, ctx, esClient, index, relDoc("splitDoc", internalSearch.RelClaims{
		refRecord(locationProp, venue1, subClaims(internalSearch.RelClaims{refRecord(artistProp, artist, nil)}, nil)),
		refRecord(locationProp, venue2, subClaims(nil, internalSearch.TimeClaims{periodRecord(timeProp)})),
	}))
	// The artist and the period are both recorded under venue1.
	indexDocument(t, ctx, esClient, index, relDoc("sameDoc", internalSearch.RelClaims{
		refRecord(locationProp, venue1, subClaims(
			internalSearch.RelClaims{refRecord(artistProp, artist, nil)},
			internalSearch.TimeClaims{periodRecord(timeProp)},
		)),
	}))
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{ //nolint:exhaustruct
		Filters: []search.Filter{
			{ //nolint:exhaustruct
				Prop: []identifier.Identifier{locationProp, artistProp},
				Ref:  &search.RefFilter{To: []search.ToValue{{ID: artist}}, Direct: nil},
			},
			{ //nolint:exhaustruct
				Prop: []identifier.Identifier{locationProp, timeProp},
				Time: periodFilter(),
			},
		},
	})

	results, _, errE := search.ResultsGet(ctx, getSearchService, &session.SessionData, nil, 0)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []string{identifier.From("sameDoc").String()}, resultIDs(results))
}

// TestCorrelationDuplicateVenueIntegration verifies that correlation distinguishes duplicate parent
// claims with the same target: the same venue listed twice, once with the artist and once with the
// period, does not satisfy filters requiring both on the same claim, even with the venue itself
// selected at the top level.
func TestCorrelationDuplicateVenueIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	locationProp := identifier.From("locationProp")
	artistProp := identifier.From("artistProp")
	timeProp := identifier.From("periodProp")
	artist := identifier.From("artistA")
	venue := identifier.From("venue")

	// The same venue twice: one entry with the artist, the other with the period.
	indexDocument(t, ctx, esClient, index, relDoc("twoEntriesDoc", internalSearch.RelClaims{
		refRecord(locationProp, venue, subClaims(internalSearch.RelClaims{refRecord(artistProp, artist, nil)}, nil)),
		refRecord(locationProp, venue, subClaims(nil, internalSearch.TimeClaims{periodRecord(timeProp)})),
	}))
	// One venue entry carrying both.
	indexDocument(t, ctx, esClient, index, relDoc("oneEntryDoc", internalSearch.RelClaims{
		refRecord(locationProp, venue, subClaims(
			internalSearch.RelClaims{refRecord(artistProp, artist, nil)},
			internalSearch.TimeClaims{periodRecord(timeProp)},
		)),
	}))
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{ //nolint:exhaustruct
		Filters: []search.Filter{
			{ //nolint:exhaustruct
				Prop: []identifier.Identifier{locationProp},
				Ref:  &search.RefFilter{To: []search.ToValue{{ID: venue}}, Direct: nil},
			},
			{ //nolint:exhaustruct
				Prop: []identifier.Identifier{locationProp, artistProp},
				Ref:  &search.RefFilter{To: []search.ToValue{{ID: artist}}, Direct: nil},
			},
			{ //nolint:exhaustruct
				Prop: []identifier.Identifier{locationProp, timeProp},
				Time: periodFilter(),
			},
		},
	})

	results, _, errE := search.ResultsGet(ctx, getSearchService, &session.SessionData, nil, 0)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []string{identifier.From("oneEntryDoc").String()}, resultIDs(results))
}

// TestAllMissingGroupIntegration verifies a group whose members are all missing: it matches
// documents with some parent claim lacking both sub-properties, not documents whose parent claims
// each lack only one of them.
func TestAllMissingGroupIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	locationProp := identifier.From("locationProp")
	artistProp := identifier.From("artistProp")
	timeProp := identifier.From("periodProp")
	artist := identifier.From("artistA")
	venue1 := identifier.From("venue1")
	venue2 := identifier.From("venue2")

	// venue2 carries neither the artist nor the period.
	indexDocument(t, ctx, esClient, index, relDoc("bareEntryDoc", internalSearch.RelClaims{
		refRecord(locationProp, venue1, subClaims(internalSearch.RelClaims{refRecord(artistProp, artist, nil)}, nil)),
		refRecord(locationProp, venue2, nil),
	}))
	// Each venue lacks one sub-property but carries the other, so no claim lacks both.
	indexDocument(t, ctx, esClient, index, relDoc("complementaryDoc", internalSearch.RelClaims{
		refRecord(locationProp, venue1, subClaims(internalSearch.RelClaims{refRecord(artistProp, artist, nil)}, nil)),
		refRecord(locationProp, venue2, subClaims(nil, internalSearch.TimeClaims{periodRecord(timeProp)})),
	}))
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{ //nolint:exhaustruct
		Filters: []search.Filter{
			{ //nolint:exhaustruct
				Prop:     []identifier.Identifier{locationProp, artistProp},
				Specials: &search.SpecialsFilter{Missing: true, None: false, Unknown: false, HasProperty: false},
			},
			{ //nolint:exhaustruct
				Prop:     []identifier.Identifier{locationProp, timeProp},
				Specials: &search.SpecialsFilter{Missing: true, None: false, Unknown: false, HasProperty: false},
			},
		},
	})

	results, _, errE := search.ResultsGet(ctx, getSearchService, &session.SessionData, nil, 0)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []string{identifier.From("bareEntryDoc").String()}, resultIDs(results))
}

// TestMissingQuantifierShiftIntegration verifies that a missing selection quantifies over the
// claims its group binds: with a sibling period filter the document matches through the venue that
// has the period and no artist (same-claim form), while the missing selection standalone does not
// match the document because another venue does carry an artist (all-claims form).
func TestMissingQuantifierShiftIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	locationProp := identifier.From("locationProp")
	artistProp := identifier.From("artistProp")
	timeProp := identifier.From("periodProp")
	artist := identifier.From("artistA")
	venue1 := identifier.From("venue1")
	venue2 := identifier.From("venue2")

	// venue1 has the period and the artist; venue2 has the period and no artist.
	indexDocument(t, ctx, esClient, index, relDoc("shiftDoc", internalSearch.RelClaims{
		refRecord(locationProp, venue1, subClaims(
			internalSearch.RelClaims{refRecord(artistProp, artist, nil)},
			internalSearch.TimeClaims{periodRecord(timeProp)},
		)),
		refRecord(locationProp, venue2, subClaims(nil, internalSearch.TimeClaims{periodRecord(timeProp)})),
	}))
	refreshIndex(t, ctx, esClient, index)

	artistMissing := search.Filter{ //nolint:exhaustruct
		Prop:     []identifier.Identifier{locationProp, artistProp},
		Specials: &search.SpecialsFilter{Missing: true, None: false, Unknown: false, HasProperty: false},
	}

	// With the sibling period filter the group requires one venue with the period and no artist.
	groupedSession := createSession(t, ctx, search.SessionData{ //nolint:exhaustruct
		Filters: []search.Filter{
			{ //nolint:exhaustruct
				Prop: []identifier.Identifier{locationProp, timeProp},
				Time: periodFilter(),
			},
			artistMissing,
		},
	})

	results, _, errE := search.ResultsGet(ctx, getSearchService, &groupedSession.SessionData, nil, 0)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []string{identifier.From("shiftDoc").String()}, resultIDs(results))

	// Standalone the missing selection requires that no venue carries an artist, and venue1 does.
	standaloneSession := createSession(t, ctx, search.SessionData{ //nolint:exhaustruct
		Filters: []search.Filter{artistMissing},
	})

	results, _, errE = search.ResultsGet(ctx, getSearchService, &standaloneSession.SessionData, nil, 0)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Empty(t, results)
}

// TestParentMissingEmptiesSubFacetsIntegration verifies that selecting a parent property's missing
// special removes that property's sub facets from discovery (no document in scope has a parent
// claim to host them), while an active sub filter stays listed with count 0.
func TestParentMissingEmptiesSubFacetsIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	locationProp := identifier.From("locationProp")
	artistProp := identifier.From("artistProp")
	otherProp := identifier.From("otherProp")
	artist := identifier.From("artistA")
	venue := identifier.From("venue")
	other := identifier.From("other")

	indexDocument(t, ctx, esClient, index, relDoc("locatedDoc", internalSearch.RelClaims{
		refRecord(locationProp, venue, subClaims(internalSearch.RelClaims{refRecord(artistProp, artist, nil)}, nil)),
	}))
	indexDocument(t, ctx, esClient, index, relDoc("unlocatedDoc", internalSearch.RelClaims{
		refRecord(otherProp, other, nil),
	}))
	refreshIndex(t, ctx, esClient, index)

	locationMissing := search.Filter{ //nolint:exhaustruct
		Prop:     []identifier.Identifier{locationProp},
		Specials: &search.SpecialsFilter{Missing: true, None: false, Unknown: false, HasProperty: false},
	}

	// With only the parent missing selected, the located document leaves the scope and no
	// (location, artist) sub facet is discovered.
	session := createSession(t, ctx, search.SessionData{ //nolint:exhaustruct
		Filters: []search.Filter{locationMissing},
	})

	results, _, errE := search.FiltersGet(ctx, getSearchService, session, nil, "")
	require.NoError(t, errE, "% -+#.1v", errE)
	subFacetProps := []string{locationProp.String(), artistProp.String()}
	for _, r := range results {
		assert.NotEqual(t, subFacetProps, r.Props)
	}

	// An active sub filter under the missing parent stays listed, with count 0.
	activeSession := createSession(t, ctx, search.SessionData{ //nolint:exhaustruct
		Filters: []search.Filter{
			locationMissing,
			{ //nolint:exhaustruct
				Prop: []identifier.Identifier{locationProp, artistProp},
				Ref:  &search.RefFilter{To: []search.ToValue{{ID: artist}}, Direct: nil},
			},
		},
	})

	results, _, errE = search.FiltersGet(ctx, getSearchService, activeSession, nil, "")
	require.NoError(t, errE, "% -+#.1v", errE)
	activeID := activeSession.Filters[1].ID.String()
	found := false
	for _, r := range results {
		if r.FilterID == activeID {
			found = true
			assert.Equal(t, subFacetProps, r.Props)
			assert.Equal(t, int64(0), r.Count)
		}
	}
	assert.True(t, found, "active sub filter entry not found in filter results")
}
