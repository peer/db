package search_test

import (
	"math"
	"sort"
	"testing"

	esSearch "github.com/elastic/go-elasticsearch/v9/typedapi/core/search"
	"github.com/elastic/go-elasticsearch/v9/typedapi/esdsl"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/identifier"

	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
	"gitlab.com/peerdb/peerdb/search"
)

func TestResultsGetIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	doc1ID := identifier.From("doc1")
	doc2ID := identifier.From("doc2")
	doc3ID := identifier.From("doc3")

	indexScoreDoc(t, ctx, esClient, index, doc1ID, "hello world", nil)
	indexScoreDoc(t, ctx, esClient, index, doc2ID, "goodbye world", nil)
	indexScoreDoc(t, ctx, esClient, index, doc3ID, "hello there", nil)
	refreshIndex(t, ctx, esClient, index)

	// Empty query returns all documents.
	session := createSession(t, ctx, search.SessionData{})

	results, metadata, errE := search.ResultsGet(ctx, getSearchService, &session.SessionData, nil, 0)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Len(t, results, 3)
	assert.Equal(t, int64(3), metadata["total"])

	// Verify all expected IDs are present (order is unspecified for empty query).
	gotIDs := make([]string, 0, len(results))
	for _, r := range results {
		gotIDs = append(gotIDs, r.ID)
	}
	sort.Strings(gotIDs)
	expectedIDs := []string{doc1ID.String(), doc2ID.String(), doc3ID.String()}
	sort.Strings(expectedIDs)
	assert.Equal(t, expectedIDs, gotIDs)

	// Query "hello" returns 2 documents.
	helloSession := createSession(t, ctx, search.SessionData{ //nolint:exhaustruct
		Query: "hello",
	})

	results, metadata, errE = search.ResultsGet(ctx, getSearchService, &helloSession.SessionData, nil, 0)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Len(t, results, 2)
	assert.Equal(t, int64(2), metadata["total"])

	// Verify all expected IDs are present (order may vary by relevance).
	gotIDs = make([]string, 0, len(results))
	for _, r := range results {
		gotIDs = append(gotIDs, r.ID)
	}
	sort.Strings(gotIDs)
	expectedIDs = []string{doc1ID.String(), doc3ID.String()}
	sort.Strings(expectedIDs)
	assert.Equal(t, expectedIDs, gotIDs)

	// Query "goodbye" returns 1 document.
	goodbyeSession := createSession(t, ctx, search.SessionData{ //nolint:exhaustruct
		Query: "goodbye",
	})

	results, metadata, errE = search.ResultsGet(ctx, getSearchService, &goodbyeSession.SessionData, nil, 0)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []search.Result{{Count: nil, Col: 0, Group: nil, ID: doc2ID.String()}}, results)
	assert.Equal(t, int64(1), metadata["total"])

	// Query "nonexistent" returns 0 documents.
	noResultsSession := createSession(t, ctx, search.SessionData{ //nolint:exhaustruct
		Query: "nonexistent",
	})

	results, metadata, errE = search.ResultsGet(ctx, getSearchService, &noResultsSession.SessionData, nil, 0)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []search.Result{}, results)
	assert.Equal(t, int64(0), metadata["total"])
}

func TestResultsGetWithIDsIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	doc1ID := identifier.From("doc1")
	doc2ID := identifier.From("doc2")
	doc3ID := identifier.From("doc3")

	for _, doc := range []struct {
		ID   identifier.Identifier
		Text string
	}{
		{doc1ID, "hello world"},
		{doc2ID, "goodbye world"},
		{doc3ID, "hello there"},
	} {
		indexScoreDoc(t, ctx, esClient, index, doc.ID, doc.Text, nil)
	}
	refreshIndex(t, ctx, esClient, index)

	// IDs scope alone returns exactly the listed documents.
	session := createSession(t, ctx, search.SessionData{ //nolint:exhaustruct
		IDs: []identifier.Identifier{doc1ID, doc3ID},
	})

	results, metadata, errE := search.ResultsGet(ctx, getSearchService, &session.SessionData, nil, 0)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Len(t, results, 2)
	assert.Equal(t, int64(2), metadata["total"])

	gotIDs := make([]string, 0, len(results))
	for _, r := range results {
		gotIDs = append(gotIDs, r.ID)
	}
	sort.Strings(gotIDs)
	expectedIDs := []string{doc1ID.String(), doc3ID.String()}
	sort.Strings(expectedIDs)
	assert.Equal(t, expectedIDs, gotIDs)

	// IDs scope combines with a full-text query: only listed documents matching the query.
	querySession := createSession(t, ctx, search.SessionData{ //nolint:exhaustruct
		Query: "world",
		IDs:   []identifier.Identifier{doc1ID, doc3ID},
	})

	results, metadata, errE = search.ResultsGet(ctx, getSearchService, &querySession.SessionData, nil, 0)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []search.Result{{Count: nil, Col: 0, Group: nil, ID: doc1ID.String()}}, results)
	assert.Equal(t, int64(1), metadata["total"])
}

func TestResultsGetWithRefFilterIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	refProp := identifier.From("refProp")
	refTarget := identifier.From("refTarget")
	doc1ID := identifier.From("doc1")
	doc2ID := identifier.From("doc2")

	indexDocument(t, ctx, esClient, index, claimsDoc("doc1", internalSearch.ClaimTypes{ //nolint:exhaustruct
		Rel: internalSearch.RelClaims{refRecord(refProp, refTarget, nil)},
	}))
	indexDocument(t, ctx, esClient, index, claimsDoc("doc2", internalSearch.ClaimTypes{
		Rel: nil, Amount: nil, Time: nil, Identifier: nil, String: nil, HTML: nil, Link: nil,
	}))
	refreshIndex(t, ctx, esClient, index)

	// Filter by reference value.
	session := createSession(t, ctx, search.SessionData{ //nolint:exhaustruct
		Filters: []search.Filter{{ //nolint:exhaustruct
			Prop: []identifier.Identifier{refProp},
			Ref:  &search.RefFilter{To: []search.ToValue{{ID: refTarget}}, Direct: nil},
		}},
	})

	results, _, errE := search.ResultsGet(ctx, getSearchService, &session.SessionData, nil, 0)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []search.Result{{Count: nil, Col: 0, Group: nil, ID: doc1ID.String()}}, results)

	// Filter by the missing special: documents that state nothing facetable for the property.
	missingSession := createSession(t, ctx, search.SessionData{ //nolint:exhaustruct
		Filters: []search.Filter{{ //nolint:exhaustruct
			Prop:     []identifier.Identifier{refProp},
			Specials: &search.SpecialsFilter{Missing: true, None: false, Unknown: false, HasProperty: false},
		}},
	})

	results, _, errE = search.ResultsGet(ctx, getSearchService, &missingSession.SessionData, nil, 0)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []search.Result{{Count: nil, Col: 0, Group: nil, ID: doc2ID.String()}}, results)
}

func TestResultsGetWithAmountFilterIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	amountProp := identifier.From("amountProp")
	unitID := identifier.From("unitID")
	doc1ID := identifier.From("doc1")
	doc2ID := identifier.From("doc2")
	doc3ID := identifier.From("doc3")

	five := 5.0
	fifteen := 15.0

	// Point amounts at 5 and 15 (indexed as their precision windows), and a document
	// without the property.
	indexAmountDoc(t, ctx, esClient, index, "doc1", amountProp, unitID, &five)
	indexAmountDoc(t, ctx, esClient, index, "doc2", amountProp, unitID, &fifteen)
	indexDocument(t, ctx, esClient, index, claimsDoc("doc3", internalSearch.ClaimTypes{
		Rel: nil, Amount: nil, Time: nil, Identifier: nil, String: nil, HTML: nil, Link: nil,
	}))
	refreshIndex(t, ctx, esClient, index)

	// Filter: amount in [10, 100] - matches doc2 (15) but not doc1 (5).
	gte := 10.0
	lteBig := 100.0
	session := createSession(t, ctx, search.SessionData{ //nolint:exhaustruct
		Filters: []search.Filter{{ //nolint:exhaustruct
			Prop:   []identifier.Identifier{amountProp},
			Amount: &search.AmountFilter{Unit: &unitID, Gte: &gte, Lte: &lteBig, Exists: false},
		}},
	})

	results, _, errE := search.ResultsGet(ctx, getSearchService, &session.SessionData, nil, 0)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []search.Result{{Count: nil, Col: 0, Group: nil, ID: doc2ID.String()}}, results)

	// Filter: amount in [0, 10] - matches doc1 (5) but not doc2 (15).
	gteSmall := 0.0
	lte := 10.0
	session2 := createSession(t, ctx, search.SessionData{ //nolint:exhaustruct
		Filters: []search.Filter{{ //nolint:exhaustruct
			Prop:   []identifier.Identifier{amountProp},
			Amount: &search.AmountFilter{Unit: &unitID, Gte: &gteSmall, Lte: &lte, Exists: false},
		}},
	})

	results, _, errE = search.ResultsGet(ctx, getSearchService, &session2.SessionData, nil, 0)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []search.Result{{Count: nil, Col: 0, Group: nil, ID: doc1ID.String()}}, results)

	// The missing special selects the document without any claim for the property.
	session3 := createSession(t, ctx, search.SessionData{ //nolint:exhaustruct
		Filters: []search.Filter{{ //nolint:exhaustruct
			Prop:     []identifier.Identifier{amountProp},
			Specials: &search.SpecialsFilter{Missing: true, None: false, Unknown: false, HasProperty: false},
		}},
	})

	results, _, errE = search.ResultsGet(ctx, getSearchService, &session3.SessionData, nil, 0)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []search.Result{{Count: nil, Col: 0, Group: nil, ID: doc3ID.String()}}, results)
}

func TestResultsGetWithTimeFilterIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	timeProp := identifier.From("timeProp")
	doc2ID := identifier.From("doc2")
	doc3ID := identifier.From("doc3")

	t1000 := float64(1000)
	t2000 := float64(2000)

	// Point times at 1000 and 2000 (indexed as their precision windows), and a document
	// without the property.
	indexTimePointDoc(t, ctx, esClient, index, "doc1", timeProp, &t1000)
	indexTimePointDoc(t, ctx, esClient, index, "doc2", timeProp, &t2000)
	indexDocument(t, ctx, esClient, index, claimsDoc("doc3", internalSearch.ClaimTypes{
		Rel: nil, Amount: nil, Time: nil, Identifier: nil, String: nil, HTML: nil, Link: nil,
	}))
	refreshIndex(t, ctx, esClient, index)

	// Filter: time in [1500, 10000] - matches doc2 (2000) but not doc1 (1000).
	gte := float64(1500)
	lteBig := float64(10000)
	session := createSession(t, ctx, search.SessionData{ //nolint:exhaustruct
		Filters: []search.Filter{{ //nolint:exhaustruct
			Prop: []identifier.Identifier{timeProp},
			Time: &search.TimeFilter{Gte: &gte, Lte: &lteBig, Exists: false},
		}},
	})

	results, _, errE := search.ResultsGet(ctx, getSearchService, &session.SessionData, nil, 0)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []search.Result{{Count: nil, Col: 0, Group: nil, ID: doc2ID.String()}}, results)

	// The missing special selects the document without any claim for the property.
	session2 := createSession(t, ctx, search.SessionData{ //nolint:exhaustruct
		Filters: []search.Filter{{ //nolint:exhaustruct
			Prop:     []identifier.Identifier{timeProp},
			Specials: &search.SpecialsFilter{Missing: true, None: false, Unknown: false, HasProperty: false},
		}},
	})

	results, _, errE = search.ResultsGet(ctx, getSearchService, &session2.SessionData, nil, 0)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []search.Result{{Count: nil, Col: 0, Group: nil, ID: doc3ID.String()}}, results)
}

func TestResultsGetWithMultipleFiltersIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	refProp1 := identifier.From("refProp1")
	refTarget1 := identifier.From("refTarget1")
	refProp2 := identifier.From("refProp2")
	refTarget2 := identifier.From("refTarget2")
	doc3ID := identifier.From("doc3")

	indexDocument(t, ctx, esClient, index, claimsDoc("doc1", internalSearch.ClaimTypes{ //nolint:exhaustruct
		Rel: internalSearch.RelClaims{refRecord(refProp1, refTarget1, nil)},
	}))
	indexDocument(t, ctx, esClient, index, claimsDoc("doc2", internalSearch.ClaimTypes{ //nolint:exhaustruct
		Rel: internalSearch.RelClaims{refRecord(refProp2, refTarget2, nil)},
	}))
	indexDocument(t, ctx, esClient, index, claimsDoc("doc3", internalSearch.ClaimTypes{ //nolint:exhaustruct
		Rel: internalSearch.RelClaims{
			refRecord(refProp1, refTarget1, nil),
			refRecord(refProp2, refTarget2, nil),
		},
	}))
	refreshIndex(t, ctx, esClient, index)

	// Multiple filters in the slice act as AND: both references must match.
	andSession := createSession(t, ctx, search.SessionData{ //nolint:exhaustruct
		Filters: []search.Filter{
			{ //nolint:exhaustruct
				Prop: []identifier.Identifier{refProp1},
				Ref:  &search.RefFilter{To: []search.ToValue{{ID: refTarget1}}, Direct: nil},
			},
			{ //nolint:exhaustruct
				Prop: []identifier.Identifier{refProp2},
				Ref:  &search.RefFilter{To: []search.ToValue{{ID: refTarget2}}, Direct: nil},
			},
		},
	})

	results, _, errE := search.ResultsGet(ctx, getSearchService, &andSession.SessionData, nil, 0)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []search.Result{{Count: nil, Col: 0, Group: nil, ID: doc3ID.String()}}, results)
}

func TestResultsGetTotalGteIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, _, index := initES(t)

	docID := identifier.From("doc1")
	indexDocument(t, ctx, esClient, index, claimsDoc("doc1", internalSearch.ClaimTypes{
		Rel: nil, Amount: nil, Time: nil, Identifier: nil, String: nil, HTML: nil, Link: nil,
	}))
	refreshIndex(t, ctx, esClient, index)

	getSearchServiceTracked := func() *esSearch.Search {
		return esClient.Search().Index(index).TrackTotalHits(esdsl.NewTrackHits().Bool(true))
	}

	session := createSession(t, ctx, search.SessionData{})

	results, metadata, errE := search.ResultsGet(ctx, getSearchServiceTracked, &session.SessionData, nil, 0)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []search.Result{{Count: nil, Col: 0, Group: nil, ID: docID.String()}}, results)
	assert.Equal(t, int64(1), metadata["total"])
}

func TestResultsGetTotalGteRelationIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, _, index := initES(t)

	// Index multiple documents with deterministic IDs.
	for i := range 5 {
		indexDocument(t, ctx, esClient, index, claimsDoc("gteDoc"+string(rune('0'+i)), internalSearch.ClaimTypes{
			Rel: nil, Amount: nil, Time: nil, Identifier: nil, String: nil, HTML: nil, Link: nil,
		}))
	}
	refreshIndex(t, ctx, esClient, index)

	// Set TrackTotalHits to 1 so ES returns "gte" relation when there are more than 1 hit.
	getSearchServiceLimited := func() *esSearch.Search {
		return esClient.Search().Index(index).TrackTotalHits(esdsl.NewTrackHits().Int(1))
	}

	session := createSession(t, ctx, search.SessionData{})

	results, metadata, errE := search.ResultsGet(ctx, getSearchServiceLimited, &session.SessionData, nil, 0)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Len(t, results, 5)
	// With TrackTotalHits(1), ES returns relation "gte" and value 1, so total should be "1+".
	assert.Equal(t, "1+", metadata["total"])
}

func TestResultsGetScoreBoost(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	lowID := identifier.From("low")
	highID := identifier.From("high")
	zeroID := identifier.From("zero")

	// Three documents matching the query text identically. They differ only in
	// counts.score, so any ranking difference is due to the boost alone.
	low := 10
	high := 1000
	zero := 0
	indexScoreDoc(t, ctx, esClient, index, lowID, "hello world", &low)
	indexScoreDoc(t, ctx, esClient, index, highID, "hello world", &high)
	indexScoreDoc(t, ctx, esClient, index, zeroID, "hello world", &zero)
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{ //nolint:exhaustruct
		Query: "hello world",
	})

	// A positive factor must rank the higher-counts.score document first, while the
	// counts.score-0 document is still returned (missing/zero is not dropped). The 2 is
	// the log2p modifier offset.
	factor := (math.Pow(2, search.TestingScoreBoostMax) - 2) / float64(high)
	results, _, errE := search.ResultsGet(ctx, getSearchService, &session.SessionData, nil, factor)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 3)
	assert.Equal(t, highID.String(), results[0].ID, "highest counts.score should rank first")
	assert.Equal(t, lowID.String(), results[1].ID, "lower counts.score should rank second")

	gotIDs := make([]string, 0, len(results))
	for _, r := range results {
		gotIDs = append(gotIDs, r.ID)
	}
	assert.Contains(t, gotIDs, zeroID.String(), "counts.score 0 document should still be returned")
}

func TestScoreFactor(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	// Empty corpus: no meaningful p99, so no boost.
	factor, errE := search.ScoreFactor(ctx, getSearchService)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Zero(t, factor)

	// A corpus whose counts.score is uniformly 50: the t-digest p99 is exactly 50, so
	// the factor is (2^scoreBoostMax - 2)/50. The 2 is the log2p modifier offset.
	value := 50
	for range 20 {
		indexScoreDoc(t, ctx, esClient, index, identifier.New(), "doc", &value)
	}
	refreshIndex(t, ctx, esClient, index)

	factor, errE = search.ScoreFactor(ctx, getSearchService)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.InDelta(t, (math.Pow(2, search.TestingScoreBoostMax)-2)/float64(value), factor, 0.001)
}

// TestResultsGetExtraFiltersIntegration verifies that an extraFilters argument to
// ResultsGet (the mechanism behind base.B.SearchQueryHook) actually restricts
// results: it wraps the query so only matching documents are returned.
func TestResultsGetExtraFiltersIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	instanceOf := identifier.From("INSTANCE_OF")
	classA := identifier.From("classA")
	classB := identifier.From("classB")

	docA := identifier.From("docA")
	docB := identifier.From("docB")
	docA2 := identifier.From("docA2")

	indexInstanceOf := func(id string, class identifier.Identifier) {
		indexDocument(t, ctx, esClient, index, claimsDoc(id, internalSearch.ClaimTypes{ //nolint:exhaustruct
			Rel: internalSearch.RelClaims{refRecord(instanceOf, class, nil)},
		}))
	}
	indexInstanceOf("docA", classA)
	indexInstanceOf("docB", classB)
	indexInstanceOf("docA2", classA)
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	// Without an access filter, all three documents are returned.
	results, _, errE := search.ResultsGet(ctx, getSearchService, &session.SessionData, nil, 0)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Len(t, results, 3)

	// With an access filter restricting INSTANCE_OF to classA, only the two
	// classA documents are returned: the filter wraps the user's query.
	accessFilter := esdsl.NewNestedQuery(esdsl.NewBoolQuery().Must(
		esdsl.NewTermQuery("claims.rel.prop", esdsl.NewFieldValue().String(instanceOf.String())),
		esdsl.NewTermQuery("claims.rel.to", esdsl.NewFieldValue().String(classA.String())),
	)).Path("claims.rel")

	results, _, errE = search.ResultsGet(ctx, getSearchService, &session.SessionData, nil, 0, accessFilter)
	require.NoError(t, errE, "% -+#.1v", errE)
	gotIDs := make([]string, 0, len(results))
	for _, r := range results {
		gotIDs = append(gotIDs, r.ID)
	}
	sort.Strings(gotIDs)
	expectedIDs := []string{docA.String(), docA2.String()}
	sort.Strings(expectedIDs)
	assert.Equal(t, expectedIDs, gotIDs)
	assert.NotContains(t, gotIDs, docB.String())
}

func TestResultsGetSortOrderIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	t100 := float64(100)
	t200 := float64(200)

	// indexSortDoc indexes a document with the given earliest time and English display-sort label.
	indexSortDoc := func(id string, tm *float64, displaySort string) {
		var ds map[string]string
		if displaySort != "" {
			ds = map[string]string{"en": displaySort}
		}
		indexDocument(t, ctx, esClient, index, internalSearch.Document{
			DisplaySort: ds,
			ID:          identifier.From(id),
			Display:     nil,
			Text:        nil,
			Time:        tm,
			LastUpdated: nil,
			Counts:      internalSearch.Counts{References: nil, Claims: nil, Score: nil},
			Claims: internalSearch.ClaimTypes{
				Rel: nil, Amount: nil, Time: nil, Identifier: nil, String: nil, HTML: nil, Link: nil,
			},
			ReadableByRoles: nil,
			ReadableByUsers: nil,
		})
	}

	// No query, filters, or prefilters, so every document scores 0 and ordering is decided by the time
	// key (newer first) and then the display-label key (a before z). The document without a time sorts
	// last regardless of its label.
	indexSortDoc("docNewerB", &t200, "b")
	indexSortDoc("docNewerA", &t200, "a")
	indexSortDoc("docOlder", &t100, "b")
	indexSortDoc("docNoTime", nil, "a")
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	results, _, errE := search.ResultsGet(ctx, getSearchService, &session.SessionData, nil, 0)
	require.NoError(t, errE, "% -+#.1v", errE)

	gotIDs := make([]string, 0, len(results))
	for _, r := range results {
		gotIDs = append(gotIDs, r.ID)
	}
	assert.Equal(t, []string{
		identifier.From("docNewerA").String(),
		identifier.From("docNewerB").String(),
		identifier.From("docOlder").String(),
		identifier.From("docNoTime").String(),
	}, gotIDs)
}
