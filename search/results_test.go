package search_test

import (
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

	stringProp := identifier.From("stringProp")

	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID: doc1ID,
		Claims: internalSearch.ClaimTypes{
			Identifier: nil,
			String: internalSearch.StringClaims{{
				Prop:        stringProp,
				PropDisplay: nil,
				PropNaming:  nil,
				String:      map[string]string{"en": "hello world"},
			}},
			HTML:         nil,
			Amount:       nil,
			Time:         nil,
			Link:         nil,
			Reference:    nil,
			Has:          nil,
			None:         nil,
			Unknown:      nil,
			SubReference: nil,
		},
	})
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID: doc2ID,
		Claims: internalSearch.ClaimTypes{
			Identifier: nil,
			String: internalSearch.StringClaims{{
				Prop:        stringProp,
				PropDisplay: nil,
				PropNaming:  nil,
				String:      map[string]string{"en": "goodbye world"},
			}},
			HTML:         nil,
			Amount:       nil,
			Time:         nil,
			Link:         nil,
			Reference:    nil,
			Has:          nil,
			None:         nil,
			Unknown:      nil,
			SubReference: nil,
		},
	})
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID: doc3ID,
		Claims: internalSearch.ClaimTypes{
			Identifier: nil,
			String: internalSearch.StringClaims{{
				Prop:        stringProp,
				PropDisplay: nil,
				PropNaming:  nil,
				String:      map[string]string{"en": "hello there"},
			}},
			HTML:         nil,
			Amount:       nil,
			Time:         nil,
			Link:         nil,
			Reference:    nil,
			Has:          nil,
			None:         nil,
			Unknown:      nil,
			SubReference: nil,
		},
	})
	refreshIndex(t, ctx, esClient, index)

	// Empty query returns all documents.
	session := createSession(t, ctx, search.SessionData{
		View:    "",
		Query:   "",
		Filters: nil,
	})

	results, metadata, errE := search.ResultsGet(ctx, getSearchService, &session.SessionData)
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
	helloSession := createSession(t, ctx, search.SessionData{
		View:    "",
		Query:   "hello",
		Filters: nil,
	})

	results, metadata, errE = search.ResultsGet(ctx, getSearchService, &helloSession.SessionData)
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
	goodbyeSession := createSession(t, ctx, search.SessionData{
		View:    "",
		Query:   "goodbye",
		Filters: nil,
	})

	results, metadata, errE = search.ResultsGet(ctx, getSearchService, &goodbyeSession.SessionData)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []search.Result{{ID: doc2ID.String()}}, results)
	assert.Equal(t, int64(1), metadata["total"])

	// Query "nonexistent" returns 0 documents.
	noResultsSession := createSession(t, ctx, search.SessionData{
		View:    "",
		Query:   "nonexistent",
		Filters: nil,
	})

	results, metadata, errE = search.ResultsGet(ctx, getSearchService, &noResultsSession.SessionData)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []search.Result{}, results)
	assert.Equal(t, int64(0), metadata["total"])
}

func TestResultsGetWithRefFilterIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	refProp := identifier.From("refProp")
	refTarget := identifier.From("refTarget")
	doc1ID := identifier.From("doc1")
	doc2ID := identifier.From("doc2")

	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID: doc1ID,
		Claims: internalSearch.ClaimTypes{
			Identifier: nil,
			String:     nil,
			HTML:       nil,
			Amount:     nil,
			Time:       nil,
			Link:       nil,
			Reference: internalSearch.ReferenceClaims{{
				Prop:          refProp,
				PropDisplay:   nil,
				PropNaming:    nil,
				To:            refTarget,
				ToDisplay:     nil,
				ToNaming:      nil,
				ToPath:        nil,
				ToDisplayPath: nil,
			}},
			Has:          nil,
			None:         nil,
			Unknown:      nil,
			SubReference: nil,
		},
	})
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID: doc2ID,
		Claims: internalSearch.ClaimTypes{
			Identifier:   nil,
			String:       nil,
			HTML:         nil,
			Amount:       nil,
			Time:         nil,
			Link:         nil,
			Reference:    nil,
			Has:          nil,
			None:         nil,
			Unknown:      nil,
			SubReference: nil,
		},
	})
	refreshIndex(t, ctx, esClient, index)

	// Filter by reference value.
	session := createSession(t, ctx, search.SessionData{
		View:  "",
		Query: "",
		Filters: []search.Filter{{ //nolint:exhaustruct
			Prop: []identifier.Identifier{refProp},
			Ref: &search.RefFilter{
				To:      []search.ToValue{{ID: refTarget}},
				Missing: false,
			},
		}},
	})

	results, _, errE := search.ResultsGet(ctx, getSearchService, &session.SessionData)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []search.Result{{ID: doc1ID.String()}}, results)

	// Filter by None reference.
	noneSession := createSession(t, ctx, search.SessionData{
		View:  "",
		Query: "",
		Filters: []search.Filter{{ //nolint:exhaustruct
			Prop: []identifier.Identifier{refProp},
			Ref: &search.RefFilter{
				To:      nil,
				Missing: true,
			},
		}},
	})

	results, _, errE = search.ResultsGet(ctx, getSearchService, &noneSession.SessionData)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []search.Result{{ID: doc2ID.String()}}, results)
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

	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID: doc1ID,
		Claims: internalSearch.ClaimTypes{
			Identifier: nil,
			String:     nil,
			HTML:       nil,
			Amount: internalSearch.AmountClaims{{
				Prop:        amountProp,
				PropDisplay: nil,
				PropNaming:  nil,
				Unit:        &unitID,
				Range: internalSearch.RangeFloat{
					GreaterThan:        nil,
					GreaterThanOrEqual: &five,
					LessThan:           nil,
					LessThanOrEqual:    &five,
				},
				From:        &five,
				FromDisplay: "",
				To:          &five,
				ToDisplay:   "",
			}},
			Time:         nil,
			Link:         nil,
			Reference:    nil,
			Has:          nil,
			None:         nil,
			Unknown:      nil,
			SubReference: nil,
		},
	})
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID: doc2ID,
		Claims: internalSearch.ClaimTypes{
			Identifier: nil,
			String:     nil,
			HTML:       nil,
			Amount: internalSearch.AmountClaims{{
				Prop:        amountProp,
				PropDisplay: nil,
				PropNaming:  nil,
				Unit:        &unitID,
				Range: internalSearch.RangeFloat{
					GreaterThan:        nil,
					GreaterThanOrEqual: &fifteen,
					LessThan:           nil,
					LessThanOrEqual:    &fifteen,
				},
				From:        &fifteen,
				FromDisplay: "",
				To:          &fifteen,
				ToDisplay:   "",
			}},
			Time:         nil,
			Link:         nil,
			Reference:    nil,
			Has:          nil,
			None:         nil,
			Unknown:      nil,
			SubReference: nil,
		},
	})
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID: doc3ID,
		Claims: internalSearch.ClaimTypes{
			Identifier:   nil,
			String:       nil,
			HTML:         nil,
			Amount:       nil,
			Time:         nil,
			Link:         nil,
			Reference:    nil,
			Has:          nil,
			None:         nil,
			Unknown:      nil,
			SubReference: nil,
		},
	})
	refreshIndex(t, ctx, esClient, index)

	// Filter: amount in [10, 100] - matches doc2 (15) but not doc1 (5).
	gte := 10.0
	lteBig := 100.0
	session := createSession(t, ctx, search.SessionData{
		View:  "",
		Query: "",
		Filters: []search.Filter{{ //nolint:exhaustruct
			Prop: []identifier.Identifier{amountProp},
			Amount: &search.AmountFilter{
				Unit:    &unitID,
				Gte:     &gte,
				Lte:     &lteBig,
				Missing: false,
			},
		}},
	})

	results, _, errE := search.ResultsGet(ctx, getSearchService, &session.SessionData)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []search.Result{{ID: doc2ID.String()}}, results)

	// Filter: amount in [0, 10] - matches doc1 (5) but not doc2 (15).
	gteSmall := 0.0
	lte := 10.0
	session2 := createSession(t, ctx, search.SessionData{
		View:  "",
		Query: "",
		Filters: []search.Filter{{ //nolint:exhaustruct
			Prop: []identifier.Identifier{amountProp},
			Amount: &search.AmountFilter{
				Unit:    &unitID,
				Gte:     &gteSmall,
				Lte:     &lte,
				Missing: false,
			},
		}},
	})

	results, _, errE = search.ResultsGet(ctx, getSearchService, &session2.SessionData)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []search.Result{{ID: doc1ID.String()}}, results)

	// Filter: amount none.
	session3 := createSession(t, ctx, search.SessionData{
		View:  "",
		Query: "",
		Filters: []search.Filter{{ //nolint:exhaustruct
			Prop: []identifier.Identifier{amountProp},
			Amount: &search.AmountFilter{
				Unit:    nil,
				Gte:     nil,
				Lte:     nil,
				Missing: true,
			},
		}},
	})

	results, _, errE = search.ResultsGet(ctx, getSearchService, &session3.SessionData)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []search.Result{{ID: doc3ID.String()}}, results)
}

func TestResultsGetWithTimeFilterIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	timeProp := identifier.From("timeProp")
	doc1ID := identifier.From("doc1")
	doc2ID := identifier.From("doc2")
	doc3ID := identifier.From("doc3")

	t1000 := float64(1000)
	t2000 := float64(2000)

	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID: doc1ID,
		Claims: internalSearch.ClaimTypes{
			Identifier: nil,
			String:     nil,
			HTML:       nil,
			Amount:     nil,
			Time: internalSearch.TimeClaims{{
				Prop:        timeProp,
				PropDisplay: nil,
				PropNaming:  nil,
				Range: internalSearch.RangeFloat{
					GreaterThan:        nil,
					GreaterThanOrEqual: &t1000,
					LessThan:           nil,
					LessThanOrEqual:    &t1000,
				},
				From:        &t1000,
				FromDisplay: "",
				To:          &t1000,
				ToDisplay:   "",
			}},
			Link:         nil,
			Reference:    nil,
			Has:          nil,
			None:         nil,
			Unknown:      nil,
			SubReference: nil,
		},
	})
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID: doc2ID,
		Claims: internalSearch.ClaimTypes{
			Identifier: nil,
			String:     nil,
			HTML:       nil,
			Amount:     nil,
			Time: internalSearch.TimeClaims{{
				Prop:        timeProp,
				PropDisplay: nil,
				PropNaming:  nil,
				Range: internalSearch.RangeFloat{
					GreaterThan:        nil,
					GreaterThanOrEqual: &t2000,
					LessThan:           nil,
					LessThanOrEqual:    &t2000,
				},
				From:        &t2000,
				FromDisplay: "",
				To:          &t2000,
				ToDisplay:   "",
			}},
			Link:         nil,
			Reference:    nil,
			Has:          nil,
			None:         nil,
			Unknown:      nil,
			SubReference: nil,
		},
	})
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID: doc3ID,
		Claims: internalSearch.ClaimTypes{
			Identifier:   nil,
			String:       nil,
			HTML:         nil,
			Amount:       nil,
			Time:         nil,
			Link:         nil,
			Reference:    nil,
			Has:          nil,
			None:         nil,
			Unknown:      nil,
			SubReference: nil,
		},
	})
	refreshIndex(t, ctx, esClient, index)

	// Filter: time in [1500, 10000] - matches doc2 (2000) but not doc1 (1000).
	gte := float64(1500)
	lteBig := float64(10000)
	session := createSession(t, ctx, search.SessionData{
		View:  "",
		Query: "",
		Filters: []search.Filter{{ //nolint:exhaustruct
			Prop: []identifier.Identifier{timeProp},
			Time: &search.TimeFilter{
				Gte:     &gte,
				Lte:     &lteBig,
				Missing: false,
			},
		}},
	})

	results, _, errE := search.ResultsGet(ctx, getSearchService, &session.SessionData)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []search.Result{{ID: doc2ID.String()}}, results)

	// Filter: time none.
	session2 := createSession(t, ctx, search.SessionData{
		View:  "",
		Query: "",
		Filters: []search.Filter{{ //nolint:exhaustruct
			Prop: []identifier.Identifier{timeProp},
			Time: &search.TimeFilter{
				Gte:     nil,
				Lte:     nil,
				Missing: true,
			},
		}},
	})

	results, _, errE = search.ResultsGet(ctx, getSearchService, &session2.SessionData)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []search.Result{{ID: doc3ID.String()}}, results)
}

func TestResultsGetWithMultipleFiltersIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	refProp1 := identifier.From("refProp1")
	refTarget1 := identifier.From("refTarget1")
	refProp2 := identifier.From("refProp2")
	refTarget2 := identifier.From("refTarget2")
	doc1ID := identifier.From("doc1")
	doc2ID := identifier.From("doc2")
	doc3ID := identifier.From("doc3")

	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID: doc1ID,
		Claims: internalSearch.ClaimTypes{
			Identifier: nil,
			String:     nil,
			HTML:       nil,
			Amount:     nil,
			Time:       nil,
			Link:       nil,
			Reference: internalSearch.ReferenceClaims{{
				Prop:          refProp1,
				PropDisplay:   nil,
				PropNaming:    nil,
				To:            refTarget1,
				ToDisplay:     nil,
				ToNaming:      nil,
				ToPath:        nil,
				ToDisplayPath: nil,
			}},
			Has:          nil,
			None:         nil,
			Unknown:      nil,
			SubReference: nil,
		},
	})
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID: doc2ID,
		Claims: internalSearch.ClaimTypes{
			Identifier: nil,
			String:     nil,
			HTML:       nil,
			Amount:     nil,
			Time:       nil,
			Link:       nil,
			Reference: internalSearch.ReferenceClaims{{
				Prop:          refProp2,
				PropDisplay:   nil,
				PropNaming:    nil,
				To:            refTarget2,
				ToDisplay:     nil,
				ToNaming:      nil,
				ToPath:        nil,
				ToDisplayPath: nil,
			}},
			Has:          nil,
			None:         nil,
			Unknown:      nil,
			SubReference: nil,
		},
	})
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID: doc3ID,
		Claims: internalSearch.ClaimTypes{
			Identifier: nil,
			String:     nil,
			HTML:       nil,
			Amount:     nil,
			Time:       nil,
			Link:       nil,
			Reference: internalSearch.ReferenceClaims{
				{
					Prop:          refProp1,
					PropDisplay:   nil,
					PropNaming:    nil,
					To:            refTarget1,
					ToDisplay:     nil,
					ToNaming:      nil,
					ToPath:        nil,
					ToDisplayPath: nil,
				},
				{
					Prop:          refProp2,
					PropDisplay:   nil,
					PropNaming:    nil,
					To:            refTarget2,
					ToDisplay:     nil,
					ToNaming:      nil,
					ToPath:        nil,
					ToDisplayPath: nil,
				},
			},
			Has:          nil,
			None:         nil,
			Unknown:      nil,
			SubReference: nil,
		},
	})
	refreshIndex(t, ctx, esClient, index)

	// Multiple filters in the slice act as AND: both references must match.
	andSession := createSession(t, ctx, search.SessionData{
		View:  "",
		Query: "",
		Filters: []search.Filter{
			{ //nolint:exhaustruct
				Prop: []identifier.Identifier{refProp1},
				Ref: &search.RefFilter{
					To:      []search.ToValue{{ID: refTarget1}},
					Missing: false,
				},
			},
			{ //nolint:exhaustruct
				Prop: []identifier.Identifier{refProp2},
				Ref: &search.RefFilter{
					To:      []search.ToValue{{ID: refTarget2}},
					Missing: false,
				},
			},
		},
	})

	results, _, errE := search.ResultsGet(ctx, getSearchService, &andSession.SessionData)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []search.Result{{ID: doc3ID.String()}}, results)
}

func TestResultsGetTotalGteIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, _, index := initES(t)

	docID := identifier.From("doc1")
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID: docID,
		Claims: internalSearch.ClaimTypes{
			Identifier:   nil,
			String:       nil,
			HTML:         nil,
			Amount:       nil,
			Time:         nil,
			Link:         nil,
			Reference:    nil,
			Has:          nil,
			None:         nil,
			Unknown:      nil,
			SubReference: nil,
		},
	})
	refreshIndex(t, ctx, esClient, index)

	getSearchServiceTracked := func() (*esSearch.Search, int64, int64) {
		return esClient.Search().Index(index).TrackTotalHits(esdsl.NewTrackHits().Bool(true)), 100, 10
	}

	session := createSession(t, ctx, search.SessionData{
		View:    "",
		Query:   "",
		Filters: nil,
	})

	results, metadata, errE := search.ResultsGet(ctx, getSearchServiceTracked, &session.SessionData)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []search.Result{{ID: docID.String()}}, results)
	assert.Equal(t, int64(1), metadata["total"])
}

func TestResultsGetTotalGteRelationIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, _, index := initES(t)

	// Index multiple documents with deterministic IDs.
	for i := range 5 {
		indexDocument(t, ctx, esClient, index, internalSearch.Document{
			ID: identifier.From("gteDoc", string(rune('0'+i))),
			Claims: internalSearch.ClaimTypes{
				Identifier:   nil,
				String:       nil,
				HTML:         nil,
				Amount:       nil,
				Time:         nil,
				Link:         nil,
				Reference:    nil,
				Has:          nil,
				None:         nil,
				Unknown:      nil,
				SubReference: nil,
			},
		})
	}
	refreshIndex(t, ctx, esClient, index)

	// Set TrackTotalHits to 1 so ES returns "gte" relation when there are more than 1 hit.
	getSearchServiceLimited := func() (*esSearch.Search, int64, int64) {
		return esClient.Search().Index(index).TrackTotalHits(esdsl.NewTrackHits().Int(1)), 100, 10
	}

	session := createSession(t, ctx, search.SessionData{
		View:    "",
		Query:   "",
		Filters: nil,
	})

	results, metadata, errE := search.ResultsGet(ctx, getSearchServiceLimited, &session.SessionData)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Len(t, results, 5)
	// With TrackTotalHits(1), ES returns relation "gte" and value 1, so total should be "1+".
	assert.Equal(t, "1+", metadata["total"])
}
