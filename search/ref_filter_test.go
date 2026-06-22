package search_test

import (
	"os"
	"testing"

	"github.com/elastic/go-elasticsearch/v9/typedapi/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/identifier"

	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
	"gitlab.com/peerdb/peerdb/search"
)

func TestRefFilterGetIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	refProp := identifier.From("refProp")
	target1 := identifier.From("target1")
	target2 := identifier.From("target2")

	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		DisplaySort: nil,
		ID:          identifier.From("refDoc1"),
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
			Reference: internalSearch.ReferenceClaims{{
				Prop: refProp, PropDisplay: nil, PropNaming: nil, PropSortKey: nil,
				To: target1, ToDisplay: nil, ToNaming: nil, ToSortKey: nil, ToPath: nil, ToFullPath: nil, ToDisplayPath: nil, ToPathSortKey: nil,
				IsLeaf: false,
			}},
			Has: nil, None: nil, Unknown: nil,
			SubRef:    nil,
			SubAmount: nil,
			SubTime:   nil,
			SubHas:    nil,
		},
	})
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		DisplaySort: nil,
		ID:          identifier.From("refDoc2"),
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
			Reference: internalSearch.ReferenceClaims{{
				Prop: refProp, PropDisplay: nil, PropNaming: nil, PropSortKey: nil,
				To: target1, ToDisplay: nil, ToNaming: nil, ToSortKey: nil, ToPath: nil, ToFullPath: nil, ToDisplayPath: nil, ToPathSortKey: nil,
				IsLeaf: false,
			}},
			Has:       nil,
			None:      nil,
			Unknown:   nil,
			SubRef:    nil,
			SubAmount: nil,
			SubTime:   nil,
			SubHas:    nil,
		},
	})
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		DisplaySort: nil,
		ID:          identifier.From("refDoc3"),
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
			Reference: internalSearch.ReferenceClaims{{
				Prop: refProp, PropDisplay: nil, PropNaming: nil, PropSortKey: nil,
				To: target2, ToDisplay: nil, ToNaming: nil, ToSortKey: nil, ToPath: nil, ToFullPath: nil, ToDisplayPath: nil, ToPathSortKey: nil,
				IsLeaf: false,
			}},
			Has:       nil,
			None:      nil,
			Unknown:   nil,
			SubRef:    nil,
			SubAmount: nil,
			SubTime:   nil,
			SubHas:    nil,
		},
	})
	refreshIndex(t, ctx, esClient, index)

	// Create a session with a ref filter so we can look up the filter by ID.
	session := createSession(t, ctx, search.SessionData{
		Sort:     nil,
		Language: "",
		View:     "",
		Query:    "",
		Filters: []search.Filter{{ //nolint:exhaustruct
			Prop: []identifier.Identifier{refProp},
			Ref: &search.RefFilter{
				Direct:  nil,
				To:      []search.ToValue{{ID: target1}},
				Missing: false,
			},
		}},
		Prefilters:    nil,
		Reverse:       nil,
		ReverseExpand: false,
	})

	results, metadata, errE := session.Filters[0].Ref.Get(
		ctx, getSearchService, session.ToQueryExcluding(*session.Filters[0].ID, nil), session.Filters[0].Prop[0], nil, "", nil,
	)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Results are sorted by count descending: target1 (count 2) first, target2 (count 1) second.
	assert.Equal(t, []search.RefFilterResult{
		{ID: target1.String(), Count: 2, Paths: nil},
		{ID: target2.String(), Count: 1, Paths: nil},
	}, results)
	assert.Equal(t, "2", metadata["total"])
}

func TestRefFilterGetInactiveIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	refProp := identifier.From("refProp")
	target1 := identifier.From("target1")
	target2 := identifier.From("target2")

	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		DisplaySort: nil,
		ID:          identifier.From("refDoc1"),
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
			Reference: internalSearch.ReferenceClaims{{
				Prop: refProp, PropDisplay: nil, PropNaming: nil, PropSortKey: nil,
				To: target1, ToDisplay: nil, ToNaming: nil, ToSortKey: nil, ToPath: nil, ToFullPath: nil, ToDisplayPath: nil, ToPathSortKey: nil,
				IsLeaf: false,
			}},
			Has:       nil,
			None:      nil,
			Unknown:   nil,
			SubRef:    nil,
			SubAmount: nil,
			SubTime:   nil,
			SubHas:    nil,
		},
	})
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		DisplaySort: nil,
		ID:          identifier.From("refDoc2"),
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
			Reference: internalSearch.ReferenceClaims{{
				Prop: refProp, PropDisplay: nil, PropNaming: nil, PropSortKey: nil,
				To: target2, ToDisplay: nil, ToNaming: nil, ToSortKey: nil, ToPath: nil, ToFullPath: nil, ToDisplayPath: nil, ToPathSortKey: nil,
				IsLeaf: false,
			}},
			Has:       nil,
			None:      nil,
			Unknown:   nil,
			SubRef:    nil,
			SubAmount: nil,
			SubTime:   nil,
			SubHas:    nil,
		},
	})
	refreshIndex(t, ctx, esClient, index)

	// Create a session without any filters (inactive filter scenario).
	session := createSession(t, ctx, search.SessionData{})

	// Query for ref filter values using the session's full query and prop from outside the session.
	f := search.RefFilter{}
	results, metadata, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), refProp, nil, "", nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Results order is non-deterministic when counts are equal.
	assert.ElementsMatch(t, []search.RefFilterResult{
		{ID: target1.String(), Count: 1, Paths: nil},
		{ID: target2.String(), Count: 1, Paths: nil},
	}, results)
	assert.Equal(t, "2", metadata["total"])
}

func TestRefFilterGetMissingIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	refProp := identifier.From("refProp")
	target1 := identifier.From("target1")

	// Doc with the ref prop.
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		DisplaySort: nil,
		ID:          identifier.From("refDoc1"),
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
			Reference: internalSearch.ReferenceClaims{{
				Prop: refProp, PropDisplay: nil, PropNaming: nil, PropSortKey: nil,
				To: target1, ToDisplay: nil, ToNaming: nil, ToSortKey: nil, ToPath: nil, ToFullPath: nil, ToDisplayPath: nil, ToPathSortKey: nil,
				IsLeaf: false,
			}},
			Has:       nil,
			None:      nil,
			Unknown:   nil,
			SubRef:    nil,
			SubAmount: nil,
			SubTime:   nil,
			SubHas:    nil,
		},
	})
	// Doc without the ref prop.
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		DisplaySort: nil,
		ID:          identifier.From("refDoc2"),
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
			Reference:  nil,
			Has:        nil,
			None:       nil,
			Unknown:    nil,
			SubRef:     nil,
			SubAmount:  nil,
			SubTime:    nil,
			SubHas:     nil,
		},
	})
	// Another doc without the ref prop.
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		DisplaySort: nil,
		ID:          identifier.From("refDoc3"),
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
			Reference:  nil,
			Has:        nil,
			None:       nil,
			Unknown:    nil,
			SubRef:     nil,
			SubAmount:  nil,
			SubTime:    nil,
			SubHas:     nil,
		},
	})
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	f := search.RefFilter{}
	results, metadata, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), refProp, nil, "", nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Results should include target1 (count 1) and __MISSING__ (count 2), sorted by count descending.
	assert.Equal(t, []search.RefFilterResult{
		{ID: search.MissingValueID, Count: 2, Paths: nil},
		{ID: target1.String(), Count: 1, Paths: nil},
	}, results)
	// Total includes the missing bucket.
	assert.Equal(t, "2", metadata["total"])
}

func TestRefFilterGetNoMissingIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	refProp := identifier.From("refProp")
	target1 := identifier.From("target1")

	// All docs have the ref prop.
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		DisplaySort: nil,
		ID:          identifier.From("refDoc1"),
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
			Reference: internalSearch.ReferenceClaims{{
				Prop: refProp, PropDisplay: nil, PropNaming: nil, PropSortKey: nil,
				To: target1, ToDisplay: nil, ToNaming: nil, ToSortKey: nil, ToPath: nil, ToFullPath: nil, ToDisplayPath: nil, ToPathSortKey: nil,
				IsLeaf: false,
			}},
			Has:       nil,
			None:      nil,
			Unknown:   nil,
			SubRef:    nil,
			SubAmount: nil,
			SubTime:   nil,
			SubHas:    nil,
		},
	})
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	f := search.RefFilter{}
	results, metadata, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), refProp, nil, "", nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	// No missing bucket since all documents have the prop.
	assert.Equal(t, []search.RefFilterResult{
		{ID: target1.String(), Count: 1, Paths: nil},
	}, results)
	assert.Equal(t, "1", metadata["total"])
}

func TestRefFilterGetNotFoundIntegration(t *testing.T) {
	t.Parallel()

	if os.Getenv("ELASTIC") == "" {
		t.Skip("ELASTIC is not available")
	}

	ctx := t.Context()
	nonExistentID := identifier.From("nonExistent")

	_, errE := search.GetSession(ctx, nonExistentID)
	require.Error(t, errE)
	assert.EqualError(t, errE, "not found")
}

func TestRefFilterGetHierarchyIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	refProp := identifier.From("refProp")
	hierProp := identifier.From("hierProp")
	animal := identifier.From("animal")
	mammal := identifier.From("mammal")
	dog := identifier.From("dog")

	// Hierarchy paths follow the indexed format "<hierProp>:<root>/.../<this>".
	animalPath := hierProp.String() + ":" + animal.String()
	mammalPath := hierProp.String() + ":" + animal.String() + "/" + mammal.String()
	dogPath := hierProp.String() + ":" + animal.String() + "/" + mammal.String() + "/" + dog.String()

	// One source doc with three reference claims, one per target in the chain, as
	// produced at index time by ancestor expansion in convertReference.
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		DisplaySort: nil,
		ID:          identifier.From("dogDoc"),
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
			Reference: internalSearch.ReferenceClaims{
				{
					Prop: refProp, PropDisplay: nil, PropNaming: nil, PropSortKey: nil,
					To: dog, ToDisplay: nil, ToNaming: nil, ToSortKey: nil, ToPath: []string{dogPath}, ToFullPath: []string{dogPath}, ToDisplayPath: nil, ToPathSortKey: nil,
					IsLeaf: false,
				},
				{
					Prop: refProp, PropDisplay: nil, PropNaming: nil, PropSortKey: nil,
					To: mammal, ToDisplay: nil, ToNaming: nil, ToSortKey: nil, ToPath: []string{mammalPath}, ToFullPath: []string{dogPath}, ToDisplayPath: nil, ToPathSortKey: nil,
					IsLeaf: false,
				},
				{
					Prop: refProp, PropDisplay: nil, PropNaming: nil, PropSortKey: nil,
					To: animal, ToDisplay: nil, ToNaming: nil, ToSortKey: nil, ToPath: []string{animalPath}, ToFullPath: []string{dogPath}, ToDisplayPath: nil, ToPathSortKey: nil,
					IsLeaf: false,
				},
			},
			Has:       nil,
			None:      nil,
			Unknown:   nil,
			SubRef:    nil,
			SubAmount: nil,
			SubTime:   nil,
			SubHas:    nil,
		},
	})
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	f := search.RefFilter{}
	results, metadata, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), refProp, nil, "", nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	// One source doc per bucket; on equal counts results are ordered by hierarchy
	// depth ascending, so ancestors precede their descendants.
	assert.Equal(t, []search.RefFilterResult{
		{ID: animal.String(), Count: 1, Paths: nil},
		{ID: mammal.String(), Count: 1, Paths: [][]string{{animal.String()}}},
		{ID: dog.String(), Count: 1, Paths: [][]string{{animal.String(), mammal.String()}}},
	}, results)
	assert.Equal(t, "3", metadata["total"])
}

func TestRefFilterDirectIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	refProp := identifier.From("refProp")
	hierProp := identifier.From("hierProp")
	artist := identifier.From("artist")
	painter := identifier.From("painter")
	sculptor := identifier.From("sculptor")

	// Hierarchy: artist > {painter, sculptor}. Paths follow the indexed format "<hierProp>:<root>/.../<this>".
	artistPath := hierProp.String() + ":" + artist.String()
	painterPath := hierProp.String() + ":" + artist.String() + "/" + painter.String()
	sculptorPath := hierProp.String() + ":" + artist.String() + "/" + sculptor.String()

	painterDoc1 := identifier.From("painterDoc1")
	painterDoc2 := identifier.From("painterDoc2")
	sculptorDoc1 := identifier.From("sculptorDoc1")
	sculptorDoc2 := identifier.From("sculptorDoc2")
	sculptorDoc3 := identifier.From("sculptorDoc3")
	sculptorDoc4 := identifier.From("sculptorDoc4")
	artistDoc1 := identifier.From("artistDoc1")
	artistDoc2 := identifier.From("artistDoc2")
	artistDoc3 := identifier.From("artistDoc3")

	// A painter document is most-specific painter (isLeaf), and also an artist via ancestor
	// expansion (not most-specific, so isLeaf is false on the artist claim).
	painterClaims := internalSearch.ReferenceClaims{
		{
			Prop: refProp, PropDisplay: nil, PropNaming: nil, PropSortKey: nil,
			To: painter, ToDisplay: nil, ToNaming: nil, ToSortKey: nil, ToPath: []string{painterPath}, ToFullPath: []string{painterPath}, ToDisplayPath: nil, ToPathSortKey: nil,
			IsLeaf: true,
		},
		{
			Prop: refProp, PropDisplay: nil, PropNaming: nil, PropSortKey: nil,
			To: artist, ToDisplay: nil, ToNaming: nil, ToSortKey: nil, ToPath: []string{artistPath}, ToFullPath: []string{painterPath}, ToDisplayPath: nil, ToPathSortKey: nil,
			IsLeaf: false,
		},
	}
	// A sculptor document is most-specific sculptor (isLeaf), and also an artist via ancestor
	// expansion. There are more sculptors than artist-only documents, so the sculptor value
	// outcounts the artist "direct" entry, while painter undercounts it.
	sculptorClaims := internalSearch.ReferenceClaims{
		{
			Prop: refProp, PropDisplay: nil, PropNaming: nil, PropSortKey: nil,
			To: sculptor, ToDisplay: nil, ToNaming: nil, ToSortKey: nil,
			ToPath: []string{sculptorPath}, ToFullPath: []string{sculptorPath}, ToDisplayPath: nil, ToPathSortKey: nil,
			IsLeaf: true,
		},
		{
			Prop: refProp, PropDisplay: nil, PropNaming: nil, PropSortKey: nil,
			To: artist, ToDisplay: nil, ToNaming: nil, ToSortKey: nil, ToPath: []string{artistPath}, ToFullPath: []string{sculptorPath}, ToDisplayPath: nil, ToPathSortKey: nil,
			IsLeaf: false,
		},
	}
	// An artist-only document is most-specific artist (isLeaf), with no narrower painter or sculptor.
	artistClaims := internalSearch.ReferenceClaims{
		{
			Prop: refProp, PropDisplay: nil, PropNaming: nil, PropSortKey: nil,
			To: artist, ToDisplay: nil, ToNaming: nil, ToSortKey: nil, ToPath: []string{artistPath}, ToFullPath: []string{artistPath}, ToDisplayPath: nil, ToPathSortKey: nil,
			IsLeaf: true,
		},
	}

	indexRefDoc := func(id identifier.Identifier, claims internalSearch.ReferenceClaims) {
		indexDocument(t, ctx, esClient, index, internalSearch.Document{
			DisplaySort: nil,
			ID:          id,
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
		})
	}

	indexRefDoc(painterDoc1, painterClaims)
	indexRefDoc(painterDoc2, painterClaims)
	indexRefDoc(sculptorDoc1, sculptorClaims)
	indexRefDoc(sculptorDoc2, sculptorClaims)
	indexRefDoc(sculptorDoc3, sculptorClaims)
	indexRefDoc(sculptorDoc4, sculptorClaims)
	indexRefDoc(artistDoc1, artistClaims)
	indexRefDoc(artistDoc2, artistClaims)
	indexRefDoc(artistDoc3, artistClaims)
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	f := search.RefFilter{}
	results, metadata, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), refProp, nil, "", nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	// artist aggregates all nine documents; its children (the sculptor value, the artist "direct"
	// entry, and the painter value) are nested under artist and sorted by count exactly like any
	// other value, so the "direct" entry (3) interleaves between sculptor (4) and painter (2).
	assert.Equal(t, []search.RefFilterResult{
		{ID: artist.String(), Count: 9, Paths: nil},
		{ID: sculptor.String(), Count: 4, Paths: [][]string{{artist.String()}}},
		{ID: search.DirectRefFilterPrefix + artist.String(), Count: 3, Paths: [][]string{{artist.String()}}},
		{ID: painter.String(), Count: 2, Paths: [][]string{{artist.String()}}},
	}, results)
	// Three distinct values (artist, painter, sculptor) plus the one "direct" entry.
	assert.Equal(t, "4", metadata["total"])

	// hitIDs runs a search with query and returns the matched document IDs.
	hitIDs := func(query types.QueryVariant) []string {
		res, err := getSearchService().Size(100).Query(query).Do(ctx)
		require.NoError(t, err)
		ids := make([]string, 0, len(res.Hits.Hits))
		for _, h := range res.Hits.Hits {
			if h.Id_ != nil {
				ids = append(ids, *h.Id_)
			}
		}
		return ids
	}

	// The "direct" filter selects exactly the artist-only documents (most-specific artist),
	// none of the painters.
	directFilter := search.RefFilter{To: nil, Direct: []search.ToValue{{ID: artist}}, Missing: false}
	assert.ElementsMatch(t, []string{artistDoc1.String(), artistDoc2.String(), artistDoc3.String()}, hitIDs(directFilter.ToQuery(refProp)))

	// The plain value filter selects every artist, painters and sculptors included.
	toFilter := search.RefFilter{To: []search.ToValue{{ID: artist}}, Direct: nil, Missing: false}
	assert.ElementsMatch(t,
		[]string{
			painterDoc1.String(), painterDoc2.String(),
			sculptorDoc1.String(), sculptorDoc2.String(), sculptorDoc3.String(), sculptorDoc4.String(),
			artistDoc1.String(), artistDoc2.String(), artistDoc3.String(),
		},
		hitIDs(toFilter.ToQuery(refProp)),
	)
}

func TestRefFilterGetDiamondIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	refProp := identifier.From("refProp")
	hierProp := identifier.From("hierProp")
	root := identifier.From("root")
	parentA := identifier.From("parentA")
	parentB := identifier.From("parentB")
	leaf := identifier.From("leaf")

	// Leaf has two parents (parentA and parentB), both descend from root.
	leafPathA := hierProp.String() + ":" + root.String() + "/" + parentA.String() + "/" + leaf.String()
	leafPathB := hierProp.String() + ":" + root.String() + "/" + parentB.String() + "/" + leaf.String()

	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		DisplaySort: nil,
		ID:          identifier.From("leafDoc"),
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
			Reference: internalSearch.ReferenceClaims{
				{
					Prop: refProp, PropDisplay: nil, PropNaming: nil, PropSortKey: nil,
					To: leaf, ToDisplay: nil, ToNaming: nil, ToSortKey: nil,
					ToPath: []string{leafPathA, leafPathB}, ToFullPath: []string{leafPathA, leafPathB}, ToDisplayPath: nil, ToPathSortKey: nil,
					IsLeaf: false,
				},
			},
			Has:       nil,
			None:      nil,
			Unknown:   nil,
			SubRef:    nil,
			SubAmount: nil,
			SubTime:   nil,
			SubHas:    nil,
		},
	})
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	f := search.RefFilter{}
	results, _, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), refProp, nil, "", nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	require.Len(t, results, 1)
	assert.Equal(t, leaf.String(), results[0].ID)
	assert.Equal(t, int64(1), results[0].Count)
	assert.ElementsMatch(t, [][]string{
		{root.String(), parentA.String()},
		{root.String(), parentB.String()},
	}, results[0].Paths)
}

func TestRefFilterGetMultipleInheritanceIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	refProp := identifier.From("refProp")
	hierProp := identifier.From("hierProp")
	root := identifier.From("root")
	mid1 := identifier.From("mid1")
	mid2 := identifier.From("mid2")
	deepParent := identifier.From("deepParent")
	shallowParent := identifier.From("shallowParent")
	leaf := identifier.From("leaf")

	// leaf has two parents at different depths: deepParent (root/mid1/mid2/deepParent,
	// depth 3) and shallowParent (root/shallowParent, depth 1). Its longest ancestor
	// chain is depth 4 (via deepParent) and its shortest is depth 2 (via shallowParent).
	// The shortest chain is strictly shallower than deepParent itself (depth 3), so
	// ordering the count tie by the shortest chain would place leaf ahead of its own
	// ancestor deepParent. Ordering by the longest chain keeps every ancestor in front.
	rootPath := hierProp.String() + ":" + root.String()
	mid1Path := rootPath + "/" + mid1.String()
	mid2Path := mid1Path + "/" + mid2.String()
	deepParentPath := mid2Path + "/" + deepParent.String()
	shallowParentPath := rootPath + "/" + shallowParent.String()
	leafViaDeep := deepParentPath + "/" + leaf.String()
	leafViaShallow := shallowParentPath + "/" + leaf.String()

	// One source doc, instance of leaf, expanded to a reference claim per ancestor as
	// convertReference does at index time. Every bucket therefore has the same single-
	// document count, so ordering is decided entirely by hierarchy depth.
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		DisplaySort: nil,
		ID:          identifier.From("leafDoc"),
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
			Reference: internalSearch.ReferenceClaims{
				{
					Prop: refProp, PropDisplay: nil, PropNaming: nil, PropSortKey: nil,
					To: leaf, ToDisplay: nil, ToNaming: nil, ToSortKey: nil, ToPath: []string{leafViaDeep, leafViaShallow},
					ToFullPath: []string{leafViaDeep, leafViaShallow}, ToDisplayPath: nil, ToPathSortKey: nil,
					IsLeaf: false,
				},
				{
					Prop: refProp, PropDisplay: nil, PropNaming: nil, PropSortKey: nil,
					To: deepParent, ToDisplay: nil, ToNaming: nil, ToSortKey: nil, ToPath: []string{deepParentPath},
					ToFullPath: []string{leafViaDeep, leafViaShallow}, ToDisplayPath: nil, ToPathSortKey: nil,
					IsLeaf: false,
				},
				{
					Prop: refProp, PropDisplay: nil, PropNaming: nil, PropSortKey: nil,
					To: shallowParent, ToDisplay: nil, ToNaming: nil, ToSortKey: nil, ToPath: []string{shallowParentPath},
					ToFullPath: []string{leafViaDeep, leafViaShallow}, ToDisplayPath: nil, ToPathSortKey: nil,
					IsLeaf: false,
				},
				{
					Prop: refProp, PropDisplay: nil, PropNaming: nil, PropSortKey: nil,
					To: mid2, ToDisplay: nil, ToNaming: nil, ToSortKey: nil,
					ToPath: []string{mid2Path}, ToFullPath: []string{leafViaDeep, leafViaShallow}, ToDisplayPath: nil, ToPathSortKey: nil,
					IsLeaf: false,
				},
				{
					Prop: refProp, PropDisplay: nil, PropNaming: nil, PropSortKey: nil,
					To: mid1, ToDisplay: nil, ToNaming: nil, ToSortKey: nil,
					ToPath: []string{mid1Path}, ToFullPath: []string{leafViaDeep, leafViaShallow}, ToDisplayPath: nil, ToPathSortKey: nil,
					IsLeaf: false,
				},
				{
					Prop: refProp, PropDisplay: nil, PropNaming: nil, PropSortKey: nil,
					To: root, ToDisplay: nil, ToNaming: nil, ToSortKey: nil,
					ToPath: []string{rootPath}, ToFullPath: []string{leafViaDeep, leafViaShallow}, ToDisplayPath: nil, ToPathSortKey: nil,
					IsLeaf: false,
				},
			},
			Has:       nil,
			None:      nil,
			Unknown:   nil,
			SubRef:    nil,
			SubAmount: nil,
			SubTime:   nil,
			SubHas:    nil,
		},
	})
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	f := search.RefFilter{}
	results, metadata, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), refProp, nil, "", nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 6)
	assert.Equal(t, "6", metadata["total"])

	// All counts are equal, so the ordering must be a valid topological sort: every
	// ancestor precedes its descendants, regardless of which parent is the shorter one.
	pos := map[string]int{}
	for i, r := range results {
		assert.Equal(t, int64(1), r.Count, "unexpected count for %s", r.ID)
		pos[r.ID] = i
	}
	assert.Less(t, pos[root.String()], pos[mid1.String()])
	assert.Less(t, pos[root.String()], pos[shallowParent.String()])
	assert.Less(t, pos[mid1.String()], pos[mid2.String()])
	assert.Less(t, pos[mid2.String()], pos[deepParent.String()])
	assert.Less(t, pos[deepParent.String()], pos[leaf.String()])
	assert.Less(t, pos[shallowParent.String()], pos[leaf.String()])

	// leaf carries both parent chains.
	var leafResult search.RefFilterResult
	for _, r := range results {
		if r.ID == leaf.String() {
			leafResult = r
		}
	}
	assert.ElementsMatch(t, [][]string{
		{root.String(), mid1.String(), mid2.String(), deepParent.String()},
		{root.String(), shallowParent.String()},
	}, leafResult.Paths)
}

func TestRefFilterGetSubRefHierarchyIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	parentProp := identifier.From("parentProp")
	parentTo := identifier.From("parentToValue").String()
	subProp := identifier.From("subProp")
	hierProp := identifier.From("hierProp")
	animal := identifier.From("animal")
	mammal := identifier.From("mammal")
	dog := identifier.From("dog")

	animalPath := hierProp.String() + ":" + animal.String()
	mammalPath := hierProp.String() + ":" + animal.String() + "/" + mammal.String()
	dogPath := hierProp.String() + ":" + animal.String() + "/" + mammal.String() + "/" + dog.String()

	// Three sub-reference claims on the same doc, one per target in the chain.
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		DisplaySort: nil,
		ID:          identifier.From("subDog"),
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
			Reference:  nil,
			Has:        nil,
			None:       nil,
			Unknown:    nil,
			SubRef: internalSearch.SubRefClaims{
				{
					ParentProp: parentProp, ParentPropDisplay: nil, ParentPropNaming: nil, ParentTo: parentTo,
					ReferenceClaim: internalSearch.ReferenceClaim{
						Prop: subProp, PropDisplay: nil, PropNaming: nil, PropSortKey: nil,
						To: dog, ToDisplay: nil, ToNaming: nil, ToSortKey: nil,
						ToPath: []string{dogPath}, ToFullPath: []string{dogPath}, ToDisplayPath: nil, ToPathSortKey: nil,
						IsLeaf: false,
					},
				},
				{
					ParentProp: parentProp, ParentPropDisplay: nil, ParentPropNaming: nil, ParentTo: parentTo,
					ReferenceClaim: internalSearch.ReferenceClaim{
						Prop: subProp, PropDisplay: nil, PropNaming: nil, PropSortKey: nil,
						To: mammal, ToDisplay: nil, ToNaming: nil, ToSortKey: nil,
						ToPath: []string{mammalPath}, ToFullPath: []string{dogPath}, ToDisplayPath: nil, ToPathSortKey: nil,
						IsLeaf: false,
					},
				},
				{
					ParentProp: parentProp, ParentPropDisplay: nil, ParentPropNaming: nil, ParentTo: parentTo,
					ReferenceClaim: internalSearch.ReferenceClaim{
						Prop: subProp, PropDisplay: nil, PropNaming: nil, PropSortKey: nil,
						To: animal, ToDisplay: nil, ToNaming: nil, ToSortKey: nil,
						ToPath: []string{animalPath}, ToFullPath: []string{dogPath}, ToDisplayPath: nil, ToPathSortKey: nil,
						IsLeaf: false,
					},
				},
			},
			SubAmount: nil,
			SubTime:   nil,
			SubHas:    nil,
		},
	})
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	f := search.RefFilter{}
	results, metadata, errE := f.GetSubRef(ctx, getSearchService, session.ToQuery(nil), parentProp, subProp, nil, nil, "", nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	// On equal counts results are ordered by hierarchy depth ascending, so
	// ancestors precede their descendants.
	assert.Equal(t, []search.RefFilterResult{
		{ID: animal.String(), Count: 1, Paths: nil},
		{ID: mammal.String(), Count: 1, Paths: [][]string{{animal.String()}}},
		{ID: dog.String(), Count: 1, Paths: [][]string{{animal.String(), mammal.String()}}},
	}, results)
	assert.Equal(t, "3", metadata["total"])
}

func TestRefFilterGetValueQueryIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	refProp := identifier.From("refProp")
	otherProp := identifier.From("otherProp")
	germany := identifier.From("germany")
	france := identifier.From("france")
	germanium := identifier.From("germanium")

	// Two documents referencing values with distinct display labels under refProp. Germany also carries an
	// alternative naming string so the facet search can be exercised against the naming fields, not just the
	// display label.
	indexDocument(t, ctx, esClient, index, internalSearch.Document{ //nolint:exhaustruct
		ID: identifier.From("refDoc1"),
		Claims: internalSearch.ClaimTypes{ //nolint:exhaustruct
			Reference: internalSearch.ReferenceClaims{{ //nolint:exhaustruct
				Prop: refProp, To: germany, ToDisplay: map[string]string{"en": "Germany"}, ToNaming: map[string][]string{"en": {"Deutschland"}},
			}},
		},
	})
	indexDocument(t, ctx, esClient, index, internalSearch.Document{ //nolint:exhaustruct
		ID: identifier.From("refDoc2"),
		Claims: internalSearch.ClaimTypes{ //nolint:exhaustruct
			Reference: internalSearch.ReferenceClaims{{ //nolint:exhaustruct
				Prop: refProp, To: france, ToDisplay: map[string]string{"en": "France"},
			}},
		},
	})
	// A document referencing a value under a different property whose label also matches "germ*". The value
	// query on refProp must not leak this value, which guards against the per-property scope being dropped.
	indexDocument(t, ctx, esClient, index, internalSearch.Document{ //nolint:exhaustruct
		ID: identifier.From("refDoc3"),
		Claims: internalSearch.ClaimTypes{ //nolint:exhaustruct
			Reference: internalSearch.ReferenceClaims{{ //nolint:exhaustruct
				Prop: otherProp, To: germanium, ToDisplay: map[string]string{"en": "Germanium"},
			}},
		},
	})
	// A document without refProp contributes a missing bucket that the value query must drop.
	indexDocument(t, ctx, esClient, index, internalSearch.Document{ //nolint:exhaustruct
		ID:     identifier.From("refDoc4"),
		Claims: internalSearch.ClaimTypes{},
	})
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	enabledLanguages := internalSearch.EnabledLanguages(nil)
	f := search.RefFilter{}

	// The value query (a prefix wildcard, as the frontend appends) narrows the facet to the matching value
	// under this property only. Germanium matches "germ*" too but belongs to otherProp, so it must not leak.
	// The missing bucket is dropped because it has no display label to match.
	results, metadata, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), refProp, nil, "germ*", enabledLanguages)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []search.RefFilterResult{
		{ID: germany.String(), Count: 1, Paths: nil},
	}, results)
	assert.Equal(t, "1", metadata["total"])

	// Matching is over all naming strings, not just the display label: Germany's alternative name
	// "Deutschland" is found even though its display label is "Germany".
	results, _, errE = f.Get(ctx, getSearchService, session.ToQuery(nil), refProp, nil, "deutsch*", enabledLanguages)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []search.RefFilterResult{
		{ID: germany.String(), Count: 1, Paths: nil},
	}, results)

	// A bare "*" matches everything, including this property's own name, so the whole facet is shown (all
	// values plus the missing bucket), still scoped to this property.
	results, metadata, errE = f.Get(ctx, getSearchService, session.ToQuery(nil), refProp, nil, "*", enabledLanguages)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.ElementsMatch(t, []search.RefFilterResult{
		{ID: germany.String(), Count: 1, Paths: nil},
		{ID: france.String(), Count: 1, Paths: nil},
		{ID: search.MissingValueID, Count: 2, Paths: nil},
	}, results)
	assert.Equal(t, "3", metadata["total"])

	// An empty value query restores all values, including the missing bucket.
	results, metadata, errE = f.Get(ctx, getSearchService, session.ToQuery(nil), refProp, nil, "", enabledLanguages)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.ElementsMatch(t, []search.RefFilterResult{
		{ID: germany.String(), Count: 1, Paths: nil},
		{ID: france.String(), Count: 1, Paths: nil},
		{ID: search.MissingValueID, Count: 2, Paths: nil},
	}, results)
	assert.Equal(t, "3", metadata["total"])
}

func TestRefFilterGetSubRefParentNameQueryIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	parentProp := identifier.From("hasLocation")
	parentTo := identifier.From("venue").String()
	subProp := identifier.From("hasUser")
	alice := identifier.From("alice")

	// A sub-reference facet "has location > has user" with value "Alice". The parent property's label is
	// denormalized so the facet can be matched by it.
	indexDocument(t, ctx, esClient, index, internalSearch.Document{ //nolint:exhaustruct
		ID: identifier.From("subDoc1"),
		Claims: internalSearch.ClaimTypes{ //nolint:exhaustruct
			SubRef: internalSearch.SubRefClaims{{ //nolint:exhaustruct
				ParentProp:        parentProp,
				ParentPropDisplay: map[string]string{"en": "has location"},
				ParentTo:          parentTo,
				ReferenceClaim: internalSearch.ReferenceClaim{ //nolint:exhaustruct
					Prop: subProp, PropDisplay: map[string]string{"en": "has user"},
					To: alice, ToDisplay: map[string]string{"en": "Alice"},
				},
			}},
		},
	})
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})
	enabledLanguages := internalSearch.EnabledLanguages(nil)
	f := search.RefFilter{}

	expected := []search.RefFilterResult{{ID: alice.String(), Count: 1, Paths: nil}}

	// Matched by the parent property's name ("has location").
	results, _, errE := f.GetSubRef(ctx, getSearchService, session.ToQuery(nil), parentProp, subProp, nil, nil, "has location*", enabledLanguages)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, expected, results)

	// Matched by the sub-property's name ("has user").
	results, _, errE = f.GetSubRef(ctx, getSearchService, session.ToQuery(nil), parentProp, subProp, nil, nil, "has user*", enabledLanguages)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, expected, results)

	// Matched by the value's name ("Alice").
	results, _, errE = f.GetSubRef(ctx, getSearchService, session.ToQuery(nil), parentProp, subProp, nil, nil, "alic*", enabledLanguages)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, expected, results)

	// A query that matches neither the parent, sub-property, nor value names returns nothing.
	results, _, errE = f.GetSubRef(ctx, getSearchService, session.ToQuery(nil), parentProp, subProp, nil, nil, "zzz*", enabledLanguages)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Empty(t, results)
}
