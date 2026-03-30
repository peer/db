package search_test

import (
	"sort"
	"testing"

	essearch "github.com/elastic/go-elasticsearch/v9/typedapi/core/search"
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
			HTML:      nil,
			Amount:    nil,
			Time:      nil,
			Link:      nil,
			Reference: nil,
			Has:       nil,
			None:      nil,
			Unknown:   nil,
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
			HTML:      nil,
			Amount:    nil,
			Time:      nil,
			Link:      nil,
			Reference: nil,
			Has:       nil,
			None:      nil,
			Unknown:   nil,
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
			HTML:      nil,
			Amount:    nil,
			Time:      nil,
			Link:      nil,
			Reference: nil,
			Has:       nil,
			None:      nil,
			Unknown:   nil,
		},
	})
	refreshIndex(t, ctx, esClient, index)

	// Empty query returns all documents.
	session := &search.Session{
		ID:      nil,
		Version: 0,
		View:    "",
		Query:   "",
		Filters: nil,
	}
	createSession(t, ctx, session)

	results, metadata, errE := search.ResultsGet(ctx, getSearchService, session)
	require.NoError(t, errE)
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
	helloSession := &search.Session{
		ID:      nil,
		Version: 0,
		View:    "",
		Query:   "hello",
		Filters: nil,
	}
	createSession(t, ctx, helloSession)

	results, metadata, errE = search.ResultsGet(ctx, getSearchService, helloSession)
	require.NoError(t, errE)
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
	goodbyeSession := &search.Session{
		ID:      nil,
		Version: 0,
		View:    "",
		Query:   "goodbye",
		Filters: nil,
	}
	createSession(t, ctx, goodbyeSession)

	results, metadata, errE = search.ResultsGet(ctx, getSearchService, goodbyeSession)
	require.NoError(t, errE)
	assert.Equal(t, []search.Result{{ID: doc2ID.String()}}, results)
	assert.Equal(t, int64(1), metadata["total"])

	// Query "nonexistent" returns 0 documents.
	noResultsSession := &search.Session{
		ID:      nil,
		Version: 0,
		View:    "",
		Query:   "nonexistent",
		Filters: nil,
	}
	createSession(t, ctx, noResultsSession)

	results, metadata, errE = search.ResultsGet(ctx, getSearchService, noResultsSession)
	require.NoError(t, errE)
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
				Reference:     nil,
			}},
			Has:     nil,
			None:    nil,
			Unknown: nil,
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
			Reference:  nil,
			Has:        nil,
			None:       nil,
			Unknown:    nil,
		},
	})
	refreshIndex(t, ctx, esClient, index)

	// Filter by relation value.
	session := &search.Session{
		ID:      nil,
		Version: 0,
		View:    "",
		Query:   "",
		Filters: &search.Filters{
			And: nil,
			Or:  nil,
			Not: nil,
			Ref: &search.RefFilter{
				Prop:  refProp,
				Value: &refTarget,
				None:  false,
			},
			Amount: nil,
			Time:   nil,
		},
	}
	createSession(t, ctx, session)

	results, _, errE := search.ResultsGet(ctx, getSearchService, session)
	require.NoError(t, errE)
	assert.Equal(t, []search.Result{{ID: doc1ID.String()}}, results)

	// Filter by None relation.
	noneSession := &search.Session{
		ID:      nil,
		Version: 0,
		View:    "",
		Query:   "",
		Filters: &search.Filters{
			And: nil,
			Or:  nil,
			Not: nil,
			Ref: &search.RefFilter{
				Prop:  refProp,
				Value: nil,
				None:  true,
			},
			Amount: nil,
			Time:   nil,
		},
	}
	createSession(t, ctx, noneSession)

	results, _, errE = search.ResultsGet(ctx, getSearchService, noneSession)
	require.NoError(t, errE)
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
			Time:      nil,
			Link:      nil,
			Reference: nil,
			Has:       nil,
			None:      nil,
			Unknown:   nil,
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
			Time:      nil,
			Link:      nil,
			Reference: nil,
			Has:       nil,
			None:      nil,
			Unknown:   nil,
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
			Reference:  nil,
			Has:        nil,
			None:       nil,
			Unknown:    nil,
		},
	})
	refreshIndex(t, ctx, esClient, index)

	// Filter: amount in [10, 100] — matches doc2 (15) but not doc1 (5).
	gte := 10.0
	lteBig := 100.0
	session := &search.Session{
		ID:      nil,
		Version: 0,
		View:    "",
		Query:   "",
		Filters: &search.Filters{
			And: nil,
			Or:  nil,
			Not: nil,
			Ref: nil,
			Amount: &search.AmountFilter{
				Prop: amountProp,
				Unit: &unitID,
				Gte:  &gte,
				Lte:  &lteBig,
				None: false,
			},
			Time: nil,
		},
	}
	createSession(t, ctx, session)

	results, _, errE := search.ResultsGet(ctx, getSearchService, session)
	require.NoError(t, errE)
	assert.Equal(t, []search.Result{{ID: doc2ID.String()}}, results)

	// Filter: amount in [0, 10] — matches doc1 (5) but not doc2 (15).
	gteSmall := 0.0
	lte := 10.0
	session2 := &search.Session{
		ID:      nil,
		Version: 0,
		View:    "",
		Query:   "",
		Filters: &search.Filters{
			And: nil,
			Or:  nil,
			Not: nil,
			Ref: nil,
			Amount: &search.AmountFilter{
				Prop: amountProp,
				Unit: &unitID,
				Gte:  &gteSmall,
				Lte:  &lte,
				None: false,
			},
			Time: nil,
		},
	}
	createSession(t, ctx, session2)

	results, _, errE = search.ResultsGet(ctx, getSearchService, session2)
	require.NoError(t, errE)
	assert.Equal(t, []search.Result{{ID: doc1ID.String()}}, results)

	// Filter: amount none.
	session3 := &search.Session{
		ID:      nil,
		Version: 0,
		View:    "",
		Query:   "",
		Filters: &search.Filters{
			And: nil,
			Or:  nil,
			Not: nil,
			Ref: nil,
			Amount: &search.AmountFilter{
				Prop: amountProp,
				Unit: nil,
				Gte:  nil,
				Lte:  nil,
				None: true,
			},
			Time: nil,
		},
	}
	createSession(t, ctx, session3)

	results, _, errE = search.ResultsGet(ctx, getSearchService, session3)
	require.NoError(t, errE)
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
			Link:      nil,
			Reference: nil,
			Has:       nil,
			None:      nil,
			Unknown:   nil,
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
			Link:      nil,
			Reference: nil,
			Has:       nil,
			None:      nil,
			Unknown:   nil,
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
			Reference:  nil,
			Has:        nil,
			None:       nil,
			Unknown:    nil,
		},
	})
	refreshIndex(t, ctx, esClient, index)

	// Filter: time in [1500, 10000] — matches doc2 (2000) but not doc1 (1000).
	gte := float64(1500)
	lteBig := float64(10000)
	session := &search.Session{
		ID:      nil,
		Version: 0,
		View:    "",
		Query:   "",
		Filters: &search.Filters{
			And:    nil,
			Or:     nil,
			Not:    nil,
			Ref:    nil,
			Amount: nil,
			Time: &search.TimeFilter{
				Prop: timeProp,
				Gte:  &gte,
				Lte:  &lteBig,
				None: false,
			},
		},
	}
	createSession(t, ctx, session)

	results, _, errE := search.ResultsGet(ctx, getSearchService, session)
	require.NoError(t, errE)
	assert.Equal(t, []search.Result{{ID: doc2ID.String()}}, results)

	// Filter: time none.
	session2 := &search.Session{
		ID:      nil,
		Version: 0,
		View:    "",
		Query:   "",
		Filters: &search.Filters{
			And:    nil,
			Or:     nil,
			Not:    nil,
			Ref:    nil,
			Amount: nil,
			Time: &search.TimeFilter{
				Prop: timeProp,
				Gte:  nil,
				Lte:  nil,
				None: true,
			},
		},
	}
	createSession(t, ctx, session2)

	results, _, errE = search.ResultsGet(ctx, getSearchService, session2)
	require.NoError(t, errE)
	assert.Equal(t, []search.Result{{ID: doc3ID.String()}}, results)
}

func TestResultsGetWithBoolFiltersIntegration(t *testing.T) {
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
				Reference:     nil,
			}},
			Has:     nil,
			None:    nil,
			Unknown: nil,
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
				Reference:     nil,
			}},
			Has:     nil,
			None:    nil,
			Unknown: nil,
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
					Reference:     nil,
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
					Reference:     nil,
				},
			},
			Has:     nil,
			None:    nil,
			Unknown: nil,
		},
	})
	refreshIndex(t, ctx, esClient, index)

	// AND: both relations.
	andSession := &search.Session{
		ID:      nil,
		Version: 0,
		View:    "",
		Query:   "",
		Filters: &search.Filters{
			And: []search.Filters{
				{
					And: nil,
					Or:  nil,
					Not: nil,
					Ref: &search.RefFilter{
						Prop:  refProp1,
						Value: &refTarget1,
						None:  false,
					},
					Amount: nil,
					Time:   nil,
				},
				{
					And: nil,
					Or:  nil,
					Not: nil,
					Ref: &search.RefFilter{
						Prop:  refProp2,
						Value: &refTarget2,
						None:  false,
					},
					Amount: nil,
					Time:   nil,
				},
			},
			Or:     nil,
			Not:    nil,
			Ref:    nil,
			Amount: nil,
			Time:   nil,
		},
	}
	createSession(t, ctx, andSession)

	results, _, errE := search.ResultsGet(ctx, getSearchService, andSession)
	require.NoError(t, errE)
	assert.Equal(t, []search.Result{{ID: doc3ID.String()}}, results)

	// OR: either relation.
	orSession := &search.Session{
		ID:      nil,
		Version: 0,
		View:    "",
		Query:   "",
		Filters: &search.Filters{
			And: nil,
			Or: []search.Filters{
				{
					And: nil,
					Or:  nil,
					Not: nil,
					Ref: &search.RefFilter{
						Prop:  refProp1,
						Value: &refTarget1,
						None:  false,
					},
					Amount: nil,
					Time:   nil,
				},
				{
					And: nil,
					Or:  nil,
					Not: nil,
					Ref: &search.RefFilter{
						Prop:  refProp2,
						Value: &refTarget2,
						None:  false,
					},
					Amount: nil,
					Time:   nil,
				},
			},
			Not:    nil,
			Ref:    nil,
			Amount: nil,
			Time:   nil,
		},
	}
	createSession(t, ctx, orSession)

	results, _, errE = search.ResultsGet(ctx, getSearchService, orSession)
	require.NoError(t, errE)
	assert.Len(t, results, 3)

	// Verify all expected IDs are present (order is unspecified).
	gotIDs := make([]string, 0, len(results))
	for _, r := range results {
		gotIDs = append(gotIDs, r.ID)
	}
	sort.Strings(gotIDs)
	expectedIDs := []string{doc1ID.String(), doc2ID.String(), doc3ID.String()}
	sort.Strings(expectedIDs)
	assert.Equal(t, expectedIDs, gotIDs)

	// NOT: not rel1.
	notSession := &search.Session{
		ID:      nil,
		Version: 0,
		View:    "",
		Query:   "",
		Filters: &search.Filters{
			And: nil,
			Or:  nil,
			Not: &search.Filters{
				And: nil,
				Or:  nil,
				Not: nil,
				Ref: &search.RefFilter{
					Prop:  refProp1,
					Value: &refTarget1,
					None:  false,
				},
				Amount: nil,
				Time:   nil,
			},
			Ref:    nil,
			Amount: nil,
			Time:   nil,
		},
	}
	createSession(t, ctx, notSession)

	results, _, errE = search.ResultsGet(ctx, getSearchService, notSession)
	require.NoError(t, errE)
	assert.Equal(t, []search.Result{{ID: doc2ID.String()}}, results)
}

func TestResultsGetTotalGteIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, _, index := initES(t)

	docID := identifier.From("doc1")
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID: docID,
		Claims: internalSearch.ClaimTypes{
			Identifier: nil,
			String:     nil,
			HTML:       nil,
			Amount:     nil,
			Time:       nil,
			Link:       nil,
			Reference:  nil,
			Has:        nil,
			None:       nil,
			Unknown:    nil,
		},
	})
	refreshIndex(t, ctx, esClient, index)

	getSearchServiceTracked := func() (*essearch.Search, int64, int64) {
		return esClient.Search().Index(index).TrackTotalHits(esdsl.NewTrackHits().Bool(true)), 100, 10
	}

	session := &search.Session{
		ID:      nil,
		Version: 0,
		View:    "",
		Query:   "",
		Filters: nil,
	}
	createSession(t, ctx, session)

	results, metadata, errE := search.ResultsGet(ctx, getSearchServiceTracked, session)
	require.NoError(t, errE)
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
				Identifier: nil,
				String:     nil,
				HTML:       nil,
				Amount:     nil,
				Time:       nil,
				Link:       nil,
				Reference:  nil,
				Has:        nil,
				None:       nil,
				Unknown:    nil,
			},
		})
	}
	refreshIndex(t, ctx, esClient, index)

	// Set TrackTotalHits to 1 so ES returns "gte" relation when there are more than 1 hit.
	getSearchServiceLimited := func() (*essearch.Search, int64, int64) {
		return esClient.Search().Index(index).TrackTotalHits(esdsl.NewTrackHits().Int(1)), 100, 10
	}

	session := &search.Session{
		ID:      nil,
		Version: 0,
		View:    "",
		Query:   "",
		Filters: nil,
	}
	createSession(t, ctx, session)

	results, metadata, errE := search.ResultsGet(ctx, getSearchServiceLimited, session)
	require.NoError(t, errE)
	assert.Len(t, results, 5)
	// With TrackTotalHits(1), ES returns relation "gte" and value 1, so total should be "1+".
	assert.Equal(t, "1+", metadata["total"])
}
