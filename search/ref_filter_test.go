package search_test

import (
	"os"
	"testing"

	"github.com/elastic/go-elasticsearch/v9/typedapi/esdsl"
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
				To: target1, ToDisplay: nil, ToNaming: nil, ToSortKey: nil, ToPath: nil, ToFullPath: nil, ToParent: nil, ToDisplayPath: nil, ToPathSortKey: nil,
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
				To: target1, ToDisplay: nil, ToNaming: nil, ToSortKey: nil, ToPath: nil, ToFullPath: nil, ToParent: nil, ToDisplayPath: nil, ToPathSortKey: nil,
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
				To: target2, ToDisplay: nil, ToNaming: nil, ToSortKey: nil, ToPath: nil, ToFullPath: nil, ToParent: nil, ToDisplayPath: nil, ToPathSortKey: nil,
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
		ctx, getSearchService, session.ToQueryExcluding(*session.Filters[0].ID, nil), session.Filters[0].Prop[0], nil, "", nil, nil,
	)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Results are sorted by count descending: target1 (count 2) first, target2 (count 1) second.
	assert.Equal(t, []search.RefFilterResult{
		{ID: target1.String(), Count: 2, ChildCount: 0, Paths: nil},
		{ID: target2.String(), Count: 1, ChildCount: 0, Paths: nil},
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
				To: target1, ToDisplay: nil, ToNaming: nil, ToSortKey: nil, ToPath: nil, ToFullPath: nil, ToParent: nil, ToDisplayPath: nil, ToPathSortKey: nil,
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
				To: target2, ToDisplay: nil, ToNaming: nil, ToSortKey: nil, ToPath: nil, ToFullPath: nil, ToParent: nil, ToDisplayPath: nil, ToPathSortKey: nil,
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
	results, metadata, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), refProp, nil, "", nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Results order is non-deterministic when counts are equal.
	assert.ElementsMatch(t, []search.RefFilterResult{
		{ID: target1.String(), Count: 1, ChildCount: 0, Paths: nil},
		{ID: target2.String(), Count: 1, ChildCount: 0, Paths: nil},
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
				To: target1, ToDisplay: nil, ToNaming: nil, ToSortKey: nil, ToPath: nil, ToFullPath: nil, ToParent: nil, ToDisplayPath: nil, ToPathSortKey: nil,
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
	results, metadata, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), refProp, nil, "", nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Results should include target1 (count 1) and __MISSING__ (count 2), sorted by count descending.
	assert.Equal(t, []search.RefFilterResult{
		{ID: search.MissingValueID, Count: 2, ChildCount: 0, Paths: nil},
		{ID: target1.String(), Count: 1, ChildCount: 0, Paths: nil},
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
				To: target1, ToDisplay: nil, ToNaming: nil, ToSortKey: nil, ToPath: nil, ToFullPath: nil, ToParent: nil, ToDisplayPath: nil, ToPathSortKey: nil,
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
	results, metadata, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), refProp, nil, "", nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	// No missing bucket since all documents have the prop.
	assert.Equal(t, []search.RefFilterResult{
		{ID: target1.String(), Count: 1, ChildCount: 0, Paths: nil},
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
					To: dog, ToDisplay: nil, ToNaming: nil, ToSortKey: nil,
					ToPath: []string{dogPath}, ToFullPath: []string{dogPath}, ToParent: []string{mammal.String()}, ToDisplayPath: nil, ToPathSortKey: nil,
					IsLeaf: false,
				},
				{
					Prop: refProp, PropDisplay: nil, PropNaming: nil, PropSortKey: nil,
					To: mammal, ToDisplay: nil, ToNaming: nil, ToSortKey: nil,
					ToPath: []string{mammalPath}, ToFullPath: []string{dogPath}, ToParent: []string{animal.String()}, ToDisplayPath: nil, ToPathSortKey: nil,
					IsLeaf: false,
				},
				{
					Prop: refProp, PropDisplay: nil, PropNaming: nil, PropSortKey: nil,
					To: animal, ToDisplay: nil, ToNaming: nil, ToSortKey: nil,
					ToPath: []string{animalPath}, ToFullPath: []string{dogPath}, ToParent: nil, ToDisplayPath: nil, ToPathSortKey: nil,
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
	results, metadata, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), refProp, nil, "", nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	// One source doc per bucket; on equal counts results are ordered by hierarchy
	// depth ascending, so ancestors precede their descendants. Each value's ChildCount is its number of
	// distinct child values: animal has one child (mammal), mammal has one child (dog), dog is a leaf.
	assert.Equal(t, []search.RefFilterResult{
		{ID: animal.String(), Count: 1, ChildCount: 1, Paths: nil},
		{ID: mammal.String(), Count: 1, ChildCount: 1, Paths: [][]string{{animal.String()}}},
		{ID: dog.String(), Count: 1, ChildCount: 0, Paths: [][]string{{animal.String(), mammal.String()}}},
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
			To: painter, ToDisplay: nil, ToNaming: nil, ToSortKey: nil,
			ToPath: []string{painterPath}, ToFullPath: []string{painterPath}, ToParent: []string{artist.String()}, ToDisplayPath: nil, ToPathSortKey: nil,
			IsLeaf: true,
		},
		{
			Prop: refProp, PropDisplay: nil, PropNaming: nil, PropSortKey: nil,
			To: artist, ToDisplay: nil, ToNaming: nil, ToSortKey: nil,
			ToPath: []string{artistPath}, ToFullPath: []string{painterPath}, ToParent: nil, ToDisplayPath: nil, ToPathSortKey: nil,
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
			ToPath: []string{sculptorPath}, ToFullPath: []string{sculptorPath}, ToParent: []string{artist.String()}, ToDisplayPath: nil, ToPathSortKey: nil,
			IsLeaf: true,
		},
		{
			Prop: refProp, PropDisplay: nil, PropNaming: nil, PropSortKey: nil,
			To: artist, ToDisplay: nil, ToNaming: nil, ToSortKey: nil,
			ToPath: []string{artistPath}, ToFullPath: []string{sculptorPath}, ToParent: nil, ToDisplayPath: nil, ToPathSortKey: nil,
			IsLeaf: false,
		},
	}
	// An artist-only document is most-specific artist (isLeaf), with no narrower painter or sculptor.
	artistClaims := internalSearch.ReferenceClaims{
		{
			Prop: refProp, PropDisplay: nil, PropNaming: nil, PropSortKey: nil,
			To: artist, ToDisplay: nil, ToNaming: nil, ToSortKey: nil,
			ToPath: []string{artistPath}, ToFullPath: []string{artistPath}, ToParent: nil, ToDisplayPath: nil, ToPathSortKey: nil,
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
	results, metadata, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), refProp, nil, "", nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	// artist aggregates all nine documents; its children (the sculptor value, the artist "direct"
	// entry, and the painter value) are nested under artist and sorted by count exactly like any
	// other value, so the "direct" entry (3) interleaves between sculptor (4) and painter (2). artist has two
	// distinct child values (painter and sculptor), so its ChildCount is 2; the leaves and the synthetic
	// "direct" entry have none.
	assert.Equal(t, []search.RefFilterResult{
		{ID: artist.String(), Count: 9, ChildCount: 2, Paths: nil},
		{ID: sculptor.String(), Count: 4, ChildCount: 0, Paths: [][]string{{artist.String()}}},
		{ID: search.DirectRefFilterPrefix + artist.String(), Count: 3, ChildCount: 0, Paths: [][]string{{artist.String()}}},
		{ID: painter.String(), Count: 2, ChildCount: 0, Paths: [][]string{{artist.String()}}},
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
					ToPath: []string{leafPathA, leafPathB}, ToFullPath: []string{leafPathA, leafPathB},
					ToParent: []string{parentA.String(), parentB.String()}, ToDisplayPath: nil, ToPathSortKey: nil,
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
	results, _, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), refProp, nil, "", nil, nil)
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
					ToFullPath: []string{leafViaDeep, leafViaShallow}, ToParent: []string{deepParent.String(), shallowParent.String()}, ToDisplayPath: nil, ToPathSortKey: nil,
					IsLeaf: false,
				},
				{
					Prop: refProp, PropDisplay: nil, PropNaming: nil, PropSortKey: nil,
					To: deepParent, ToDisplay: nil, ToNaming: nil, ToSortKey: nil, ToPath: []string{deepParentPath},
					ToFullPath: []string{leafViaDeep, leafViaShallow}, ToParent: []string{mid2.String()}, ToDisplayPath: nil, ToPathSortKey: nil,
					IsLeaf: false,
				},
				{
					Prop: refProp, PropDisplay: nil, PropNaming: nil, PropSortKey: nil,
					To: shallowParent, ToDisplay: nil, ToNaming: nil, ToSortKey: nil, ToPath: []string{shallowParentPath},
					ToFullPath: []string{leafViaDeep, leafViaShallow}, ToParent: []string{root.String()}, ToDisplayPath: nil, ToPathSortKey: nil,
					IsLeaf: false,
				},
				{
					Prop: refProp, PropDisplay: nil, PropNaming: nil, PropSortKey: nil,
					To: mid2, ToDisplay: nil, ToNaming: nil, ToSortKey: nil,
					ToPath: []string{mid2Path}, ToFullPath: []string{leafViaDeep, leafViaShallow}, ToParent: []string{mid1.String()}, ToDisplayPath: nil, ToPathSortKey: nil,
					IsLeaf: false,
				},
				{
					Prop: refProp, PropDisplay: nil, PropNaming: nil, PropSortKey: nil,
					To: mid1, ToDisplay: nil, ToNaming: nil, ToSortKey: nil,
					ToPath: []string{mid1Path}, ToFullPath: []string{leafViaDeep, leafViaShallow}, ToParent: []string{root.String()}, ToDisplayPath: nil, ToPathSortKey: nil,
					IsLeaf: false,
				},
				{
					Prop: refProp, PropDisplay: nil, PropNaming: nil, PropSortKey: nil,
					To: root, ToDisplay: nil, ToNaming: nil, ToSortKey: nil,
					ToPath: []string{rootPath}, ToFullPath: []string{leafViaDeep, leafViaShallow}, ToParent: nil, ToDisplayPath: nil, ToPathSortKey: nil,
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
	results, metadata, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), refProp, nil, "", nil, nil)
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
						ToPath: []string{dogPath}, ToFullPath: []string{dogPath}, ToParent: []string{mammal.String()}, ToDisplayPath: nil, ToPathSortKey: nil,
						IsLeaf: false,
					},
				},
				{
					ParentProp: parentProp, ParentPropDisplay: nil, ParentPropNaming: nil, ParentTo: parentTo,
					ReferenceClaim: internalSearch.ReferenceClaim{
						Prop: subProp, PropDisplay: nil, PropNaming: nil, PropSortKey: nil,
						To: mammal, ToDisplay: nil, ToNaming: nil, ToSortKey: nil,
						ToPath: []string{mammalPath}, ToFullPath: []string{dogPath}, ToParent: []string{animal.String()}, ToDisplayPath: nil, ToPathSortKey: nil,
						IsLeaf: false,
					},
				},
				{
					ParentProp: parentProp, ParentPropDisplay: nil, ParentPropNaming: nil, ParentTo: parentTo,
					ReferenceClaim: internalSearch.ReferenceClaim{
						Prop: subProp, PropDisplay: nil, PropNaming: nil, PropSortKey: nil,
						To: animal, ToDisplay: nil, ToNaming: nil, ToSortKey: nil,
						ToPath: []string{animalPath}, ToFullPath: []string{dogPath}, ToParent: nil, ToDisplayPath: nil, ToPathSortKey: nil,
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
	results, metadata, errE := f.GetSubRef(ctx, getSearchService, session.ToQuery(nil), parentProp, subProp, nil, nil, "", nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	// On equal counts results are ordered by hierarchy depth ascending, so ancestors precede their
	// descendants. Each value's ChildCount is its number of distinct child values: animal has one child
	// (mammal), mammal has one child (dog), dog is a leaf.
	assert.Equal(t, []search.RefFilterResult{
		{ID: animal.String(), Count: 1, ChildCount: 1, Paths: nil},
		{ID: mammal.String(), Count: 1, ChildCount: 1, Paths: [][]string{{animal.String()}}},
		{ID: dog.String(), Count: 1, ChildCount: 0, Paths: [][]string{{animal.String(), mammal.String()}}},
	}, results)
	assert.Equal(t, "3", metadata["total"])
}

// TestRefFilterGetChildCountMultipleInheritanceIntegration verifies that ChildCount is the exact number of
// distinct child VALUES a value has, robust to multiple inheritance: dog is a child of both mammal and pet
// (two hierarchy paths, so toParent = [mammal, pet]); cat is a child of mammal only. Because the count is over
// distinct child values (not documents), mammal counts two children (dog, cat) and pet counts one (dog), even
// though dog is shared between them. The single-inheritance case is covered too: mammal is a plain parent with
// two distinct children.
func TestRefFilterGetChildCountMultipleInheritanceIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	refProp := identifier.From("refProp")
	hierProp := identifier.From("hierProp")
	mammal := identifier.From("mammal")
	pet := identifier.From("pet")
	dog := identifier.From("dog")
	cat := identifier.From("cat")

	// mammal and pet are roots; dog descends from both (a diamond), cat descends from mammal only. Paths follow
	// the indexed "<hierProp>:<root>/.../<this>" form.
	mammalPath := hierProp.String() + ":" + mammal.String()
	petPath := hierProp.String() + ":" + pet.String()
	dogViaMammal := mammalPath + "/" + dog.String()
	dogViaPet := petPath + "/" + dog.String()
	catViaMammal := mammalPath + "/" + cat.String()

	// dogDoc references dog (expanded to dog plus its two parents mammal and pet, as convertReference does).
	indexDocument(t, ctx, esClient, index, refDoc("dogDoc", internalSearch.ReferenceClaims{
		hierRefClaim(refProp, dog, []string{dogViaMammal, dogViaPet}, []string{dogViaMammal, dogViaPet}),
		hierRefClaim(refProp, mammal, []string{mammalPath}, []string{dogViaMammal, dogViaPet}),
		hierRefClaim(refProp, pet, []string{petPath}, []string{dogViaMammal, dogViaPet}),
	}))
	// catDoc references cat (expanded to cat plus its single parent mammal).
	indexDocument(t, ctx, esClient, index, refDoc("catDoc", internalSearch.ReferenceClaims{
		hierRefClaim(refProp, cat, []string{catViaMammal}, []string{catViaMammal}),
		hierRefClaim(refProp, mammal, []string{mammalPath}, []string{catViaMammal}),
	}))
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	f := search.RefFilter{}
	results, _, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), refProp, nil, "", nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	byID := refResultsByID(results)
	// mammal has two distinct children (dog and cat): a plain single-inheritance parent with two children, and
	// dog is also the shared child of the diamond.
	require.Contains(t, byID, mammal.String())
	assert.Equal(t, int64(2), byID[mammal.String()].ChildCount)
	// pet has one distinct child (dog), counted exactly once even though dog is also mammal's child.
	require.Contains(t, byID, pet.String())
	assert.Equal(t, int64(1), byID[pet.String()].ChildCount)
	// The leaves have no children.
	require.Contains(t, byID, dog.String())
	assert.Equal(t, int64(0), byID[dog.String()].ChildCount)
	require.Contains(t, byID, cat.String())
	assert.Equal(t, int64(0), byID[cat.String()].ChildCount)
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
	results, metadata, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), refProp, nil, "germ*", enabledLanguages, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []search.RefFilterResult{
		{ID: germany.String(), Count: 1, ChildCount: 0, Paths: nil},
	}, results)
	assert.Equal(t, "1", metadata["total"])

	// Matching is over all naming strings, not just the display label: Germany's alternative name
	// "Deutschland" is found even though its display label is "Germany".
	results, _, errE = f.Get(ctx, getSearchService, session.ToQuery(nil), refProp, nil, "deutsch*", enabledLanguages, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []search.RefFilterResult{
		{ID: germany.String(), Count: 1, ChildCount: 0, Paths: nil},
	}, results)

	// A bare "*" matches everything, including this property's own name, so the whole facet is shown (all
	// values plus the missing bucket), still scoped to this property.
	results, metadata, errE = f.Get(ctx, getSearchService, session.ToQuery(nil), refProp, nil, "*", enabledLanguages, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.ElementsMatch(t, []search.RefFilterResult{
		{ID: germany.String(), Count: 1, ChildCount: 0, Paths: nil},
		{ID: france.String(), Count: 1, ChildCount: 0, Paths: nil},
		{ID: search.MissingValueID, Count: 2, ChildCount: 0, Paths: nil},
	}, results)
	assert.Equal(t, "3", metadata["total"])

	// An empty value query restores all values, including the missing bucket.
	results, metadata, errE = f.Get(ctx, getSearchService, session.ToQuery(nil), refProp, nil, "", enabledLanguages, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.ElementsMatch(t, []search.RefFilterResult{
		{ID: germany.String(), Count: 1, ChildCount: 0, Paths: nil},
		{ID: france.String(), Count: 1, ChildCount: 0, Paths: nil},
		{ID: search.MissingValueID, Count: 2, ChildCount: 0, Paths: nil},
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

	expected := []search.RefFilterResult{{ID: alice.String(), Count: 1, ChildCount: 0, Paths: nil}}

	// Matched by the parent property's name ("has location").
	results, _, errE := f.GetSubRef(ctx, getSearchService, session.ToQuery(nil), parentProp, subProp, nil, nil, "has location*", enabledLanguages, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, expected, results)

	// Matched by the sub-property's name ("has user").
	results, _, errE = f.GetSubRef(ctx, getSearchService, session.ToQuery(nil), parentProp, subProp, nil, nil, "has user*", enabledLanguages, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, expected, results)

	// Matched by the value's name ("Alice").
	results, _, errE = f.GetSubRef(ctx, getSearchService, session.ToQuery(nil), parentProp, subProp, nil, nil, "alic*", enabledLanguages, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, expected, results)

	// A query that matches neither the parent, sub-property, nor value names returns nothing.
	results, _, errE = f.GetSubRef(ctx, getSearchService, session.ToQuery(nil), parentProp, subProp, nil, nil, "zzz*", enabledLanguages, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Empty(t, results)
}

// refResultsByID indexes reference filter results by their value id for assertions.
func refResultsByID(results []search.RefFilterResult) map[string]search.RefFilterResult {
	out := make(map[string]search.RefFilterResult, len(results))
	for _, r := range results {
		out[r.ID] = r
	}
	return out
}

// TestRefFilterGetSelectedValuesWithAncestorsIntegration verifies that an active reference filter always shows
// its selected values together with their ancestor chain, even when a selection matches no document under the
// rest of the search. It also covers the deselection regression: with two selected values where one matches
// and one does not, both remain present (so deselecting the matching one cannot silently drop the other).
func TestRefFilterGetSelectedValuesWithAncestorsIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	instanceOf := identifier.From("instanceOf")
	hierProp := identifier.From("hierProp")
	vocabulary := identifier.From("vocabulary")
	unit := identifier.From("unit")
	class := identifier.From("class")

	// Hierarchy: vocabulary > {unit, class}. Paths follow the indexed "<hierProp>:<root>/.../<this>" form.
	vocabularyPath := hierProp.String() + ":" + vocabulary.String()
	unitPath := vocabularyPath + "/" + unit.String()
	classPath := vocabularyPath + "/" + class.String()

	// unitDoc references unit (expanded to unit + vocabulary); classDoc references class (expanded likewise).
	indexDocument(t, ctx, esClient, index, refDoc("unitDoc", internalSearch.ReferenceClaims{
		hierRefClaim(instanceOf, unit, []string{unitPath}, []string{unitPath}),
		hierRefClaim(instanceOf, vocabulary, []string{vocabularyPath}, []string{unitPath}),
	}))
	indexDocument(t, ctx, esClient, index, refDoc("classDoc", internalSearch.ReferenceClaims{
		hierRefClaim(instanceOf, class, []string{classPath}, []string{classPath}),
		hierRefClaim(instanceOf, vocabulary, []string{vocabularyPath}, []string{classPath}),
	}))
	refreshIndex(t, ctx, esClient, index)

	// The rest of the search matches only classDoc, so unit has zero documents here. Both unit and class are
	// selected; unit must still appear (at count 0) together with its ancestor vocabulary.
	restOfSearch := esdsl.NewNestedQuery(
		esdsl.NewTermQuery("claims.ref.to", esdsl.NewFieldValue().String(class.String())),
	).Path("claims.ref")
	f := search.RefFilter{To: []search.ToValue{{ID: class}, {ID: unit}}} //nolint:exhaustruct
	resolver := newPathResolver(map[identifier.Identifier][]string{
		unit:  {unitPath},
		class: {classPath},
	})
	results, _, errE := f.Get(ctx, getSearchService, restOfSearch, instanceOf, nil, "", nil, resolver)
	require.NoError(t, errE, "% -+#.1v", errE)

	byID := refResultsByID(results)
	// unit is shown at count 0 with vocabulary as its ancestor, even though no matching document has it.
	require.Contains(t, byID, unit.String())
	assert.Equal(t, int64(0), byID[unit.String()].Count)
	assert.Equal(t, [][]string{{vocabulary.String()}}, byID[unit.String()].Paths)
	// class (selected and matched) keeps its real count, also under vocabulary.
	require.Contains(t, byID, class.String())
	assert.Equal(t, int64(1), byID[class.String()].Count)
	assert.Equal(t, [][]string{{vocabulary.String()}}, byID[class.String()].Paths)
	// vocabulary (the shared ancestor) is present so the tree can render vocabulary -> {unit, class}.
	require.Contains(t, byID, vocabulary.String())
	assert.Empty(t, byID[vocabulary.String()].Paths)
}

// TestRefFilterGetSelectedValueVanishedIntegration verifies that a selected value with no indexed hierarchy
// anywhere (it references no document at all) still appears flat at count 0, so it stays deselectable.
func TestRefFilterGetSelectedValueVanishedIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	instanceOf := identifier.From("instanceOf")
	hierProp := identifier.From("hierProp")
	vocabulary := identifier.From("vocabulary")
	class := identifier.From("class")
	ghost := identifier.From("ghost")

	vocabularyPath := hierProp.String() + ":" + vocabulary.String()
	classPath := vocabularyPath + "/" + class.String()

	indexDocument(t, ctx, esClient, index, refDoc("classDoc", internalSearch.ReferenceClaims{
		hierRefClaim(instanceOf, class, []string{classPath}, []string{classPath}),
		hierRefClaim(instanceOf, vocabulary, []string{vocabularyPath}, []string{classPath}),
	}))
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	// ghost is selected but referenced by no document, so it has no indexed toPath. It must still be returned
	// flat (no ancestors) at count 0.
	f := search.RefFilter{To: []search.ToValue{{ID: ghost}}} //nolint:exhaustruct
	results, _, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), instanceOf, nil, "", nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	byID := refResultsByID(results)
	require.Contains(t, byID, ghost.String())
	assert.Equal(t, int64(0), byID[ghost.String()].Count)
	assert.Empty(t, byID[ghost.String()].Paths)
}

// TestRefFilterGetSubRefSelectedValueWithAncestorsIntegration verifies the same selected-value surfacing for
// sub-reference filters: an active sub-ref selection is always shown together with its ancestor chain, even
// when it matches no document under the rest of the search.
func TestRefFilterGetSubRefSelectedValueWithAncestorsIntegration(t *testing.T) {
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
	cat := identifier.From("cat")

	animalPath := hierProp.String() + ":" + animal.String()
	mammalPath := animalPath + "/" + mammal.String()
	dogPath := mammalPath + "/" + dog.String()
	catPath := mammalPath + "/" + cat.String()

	subHierClaim := func(to identifier.Identifier, toPath, fullPath string) internalSearch.SubRefClaim {
		return internalSearch.SubRefClaim{ //nolint:exhaustruct
			ParentProp: parentProp, ParentTo: parentTo,
			ReferenceClaim: internalSearch.ReferenceClaim{ //nolint:exhaustruct
				Prop: subProp, To: to, ToPath: []string{toPath}, ToFullPath: []string{fullPath},
			},
		}
	}

	// subDog references dog (expanded to dog, mammal, animal); subCat references cat (expanded likewise).
	indexDocument(t, ctx, esClient, index, internalSearch.Document{ //nolint:exhaustruct
		ID: identifier.From("subDog"),
		Claims: internalSearch.ClaimTypes{ //nolint:exhaustruct
			SubRef: internalSearch.SubRefClaims{
				subHierClaim(dog, dogPath, dogPath),
				subHierClaim(mammal, mammalPath, dogPath),
				subHierClaim(animal, animalPath, dogPath),
			},
		},
	})
	indexDocument(t, ctx, esClient, index, internalSearch.Document{ //nolint:exhaustruct
		ID: identifier.From("subCat"),
		Claims: internalSearch.ClaimTypes{ //nolint:exhaustruct
			SubRef: internalSearch.SubRefClaims{
				subHierClaim(cat, catPath, catPath),
				subHierClaim(mammal, mammalPath, catPath),
				subHierClaim(animal, animalPath, catPath),
			},
		},
	})
	refreshIndex(t, ctx, esClient, index)

	// The rest of the search matches only subCat, so dog has zero documents here. dog is selected; it must
	// still appear at count 0 with its full ancestor chain (animal -> mammal -> dog).
	restOfSearch := esdsl.NewNestedQuery(
		esdsl.NewTermQuery("claims.subRef.to", esdsl.NewFieldValue().String(cat.String())),
	).Path("claims.subRef")
	f := search.RefFilter{To: []search.ToValue{{ID: dog}}} //nolint:exhaustruct
	resolver := newPathResolver(map[identifier.Identifier][]string{dog: {dogPath}})
	results, _, errE := f.GetSubRef(ctx, getSearchService, restOfSearch, parentProp, subProp, nil, nil, "", nil, resolver)
	require.NoError(t, errE, "% -+#.1v", errE)

	byID := refResultsByID(results)
	require.Contains(t, byID, dog.String())
	assert.Equal(t, int64(0), byID[dog.String()].Count)
	assert.Equal(t, [][]string{{animal.String(), mammal.String()}}, byID[dog.String()].Paths)
	// The ancestors are present so the tree can render animal -> mammal -> dog.
	require.Contains(t, byID, mammal.String())
	require.Contains(t, byID, animal.String())
	// cat (from the rest of the search) keeps its real count.
	require.Contains(t, byID, cat.String())
	assert.Equal(t, int64(1), byID[cat.String()].Count)
}

// TestRefFilterGetMissingOnlySelectionIntegration verifies that a missing-only selection that matches nothing
// still produces the missing row (at count 0) so it can be unchecked, without needing the selected-values
// aggregation.
func TestRefFilterGetMissingOnlySelectionIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	instanceOf := identifier.From("instanceOf")
	class := identifier.From("class")

	// Every indexed document has the property, so the missing count is zero and the existing code would not add
	// a missing row on its own.
	indexDocument(t, ctx, esClient, index, refDoc("classDoc", internalSearch.ReferenceClaims{
		hierRefClaim(instanceOf, class, nil, nil),
	}))
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	f := search.RefFilter{Missing: true} //nolint:exhaustruct
	results, _, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), instanceOf, nil, "", nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	byID := refResultsByID(results)
	require.Contains(t, byID, search.MissingValueID)
	assert.Equal(t, int64(0), byID[search.MissingValueID].Count)
}

// TestRefFilterGetValueSearchHierarchyIntegration verifies the interaction between an active selection and a
// filter-pane value search: the search only changes which values are shown, never their counts; a matched
// value's ancestors are shown for tree context with their real (no-search) counts; and selected values are not
// force-shown unless they match the search.
func TestRefFilterGetValueSearchHierarchyIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	instanceOf := identifier.From("instanceOf")
	hierProp := identifier.From("hierProp")
	vocabulary := identifier.From("vocabulary")
	unit := identifier.From("unit")
	language := identifier.From("language")
	class := identifier.From("class")

	vocabularyPath := hierProp.String() + ":" + vocabulary.String()
	unitPath := vocabularyPath + "/" + unit.String()
	languagePath := vocabularyPath + "/" + language.String()
	classPath := hierProp.String() + ":" + class.String()

	// A reference claim carrying a display label (so the value-query label match can find it) and its toPath.
	hierClaim := func(to identifier.Identifier, display, toPath, fullPath string) internalSearch.ReferenceClaim {
		return internalSearch.ReferenceClaim{ //nolint:exhaustruct
			Prop: instanceOf, To: to, ToDisplay: map[string]string{"en": display},
			ToPath: []string{toPath}, ToFullPath: []string{fullPath},
		}
	}

	// Hierarchy: vocabulary > {unit, language}; class is a separate root. Counts: vocabulary 3 (two unit docs
	// plus one language doc), unit 2, language 1, class 1.
	indexDocument(t, ctx, esClient, index, refDoc("unitDoc1", internalSearch.ReferenceClaims{
		hierClaim(unit, "unit", unitPath, unitPath),
		hierClaim(vocabulary, "vocabulary", vocabularyPath, unitPath),
	}))
	indexDocument(t, ctx, esClient, index, refDoc("unitDoc2", internalSearch.ReferenceClaims{
		hierClaim(unit, "unit", unitPath, unitPath),
		hierClaim(vocabulary, "vocabulary", vocabularyPath, unitPath),
	}))
	indexDocument(t, ctx, esClient, index, refDoc("languageDoc", internalSearch.ReferenceClaims{
		hierClaim(language, "language", languagePath, languagePath),
		hierClaim(vocabulary, "vocabulary", vocabularyPath, languagePath),
	}))
	indexDocument(t, ctx, esClient, index, refDoc("classDoc", internalSearch.ReferenceClaims{
		hierClaim(class, "class", classPath, classPath),
	}))
	refreshIndex(t, ctx, esClient, index)

	enabledLanguages := internalSearch.EnabledLanguages(nil)
	query := createSession(t, ctx, search.SessionData{}).ToQuery(enabledLanguages)
	// unit is the active selection; this must not force it to show during a search that it does not match.
	f := search.RefFilter{To: []search.ToValue{{ID: unit}}} //nolint:exhaustruct
	resolver := newPathResolver(map[identifier.Identifier][]string{unit: {unitPath}})

	// Searching the value name "unit" shows unit and, for tree context, its ancestor vocabulary with its real
	// (no-search) count of 3, not 0. The sibling language and the unrelated class are not shown.
	results, metadata, errE := f.Get(ctx, getSearchService, query, instanceOf, nil, "unit*", enabledLanguages, resolver)
	require.NoError(t, errE, "% -+#.1v", errE)
	byID := refResultsByID(results)
	require.Contains(t, byID, unit.String())
	assert.Equal(t, int64(2), byID[unit.String()].Count)
	assert.Equal(t, [][]string{{vocabulary.String()}}, byID[unit.String()].Paths)
	require.Contains(t, byID, vocabulary.String())
	assert.Equal(t, int64(3), byID[vocabulary.String()].Count)
	assert.NotContains(t, byID, language.String())
	assert.NotContains(t, byID, class.String())
	assert.Equal(t, "2", metadata["total"])

	// Searching "voca" shows vocabulary (real count 3). unit does not match and is not force-shown, even though
	// it is the active selection; vocabulary's other descendants are not shown either.
	results, metadata, errE = f.Get(ctx, getSearchService, query, instanceOf, nil, "voca*", enabledLanguages, resolver)
	require.NoError(t, errE, "% -+#.1v", errE)
	byID = refResultsByID(results)
	require.Contains(t, byID, vocabulary.String())
	assert.Equal(t, int64(3), byID[vocabulary.String()].Count)
	assert.NotContains(t, byID, unit.String())
	assert.NotContains(t, byID, language.String())
	assert.NotContains(t, byID, class.String())
	assert.Equal(t, "1", metadata["total"])

	// Searching "class" shows only class. The selected unit and its ancestor vocabulary are not force-shown.
	results, _, errE = f.Get(ctx, getSearchService, query, instanceOf, nil, "class*", enabledLanguages, resolver)
	require.NoError(t, errE, "% -+#.1v", errE)
	byID = refResultsByID(results)
	require.Contains(t, byID, class.String())
	assert.Equal(t, int64(1), byID[class.String()].Count)
	assert.NotContains(t, byID, vocabulary.String())
	assert.NotContains(t, byID, unit.String())
}

// TestRefFilterGetSelectedAugmentValueSearchIntegration verifies that an active reference filter's augmented
// values (its selection plus their ancestors), which have zero documents in the current search scope, are
// searchable in the filter pane by the SAME Elasticsearch label matcher real values use: a selected value
// matches by its display label or any naming string, and an ancestor matches only because its descendant is
// selected (so searching the ancestor surfaces it without pulling in the descendant). A non-matching term
// hides the augment; outside a search the whole augment is shown at count 0.
func TestRefFilterGetSelectedAugmentValueSearchIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	instanceOf := identifier.From("instanceOf")
	hierProp := identifier.From("hierProp")
	vocabulary := identifier.From("vocabulary")
	unit := identifier.From("unit")
	class := identifier.From("class")

	vocabularyPath := hierProp.String() + ":" + vocabulary.String()
	unitPath := vocabularyPath + "/" + unit.String()
	classPath := hierProp.String() + ":" + class.String()

	// A reference claim carrying a display label and optional naming strings (so the value-query label match can
	// find the value by either), plus its toPath.
	hierNamedClaim := func(to identifier.Identifier, display string, naming []string, toPath, fullPath string) internalSearch.ReferenceClaim {
		var toNaming map[string][]string
		if naming != nil {
			toNaming = map[string][]string{"en": naming}
		}
		return internalSearch.ReferenceClaim{ //nolint:exhaustruct
			Prop: instanceOf, To: to, ToDisplay: map[string]string{"en": display}, ToNaming: toNaming,
			ToPath: []string{toPath}, ToFullPath: []string{fullPath},
		}
	}

	// unitDoc references unit (expanded to unit + vocabulary); classDoc references class. The search scope below
	// matches only classDoc, so unit and vocabulary have zero documents in scope, yet exist globally.
	indexDocument(t, ctx, esClient, index, refDoc("unitDoc", internalSearch.ReferenceClaims{
		hierNamedClaim(unit, "unit", []string{"metre"}, unitPath, unitPath),
		hierNamedClaim(vocabulary, "vocabulary", nil, vocabularyPath, unitPath),
	}))
	indexDocument(t, ctx, esClient, index, refDoc("classDoc", internalSearch.ReferenceClaims{
		hierNamedClaim(class, "class", nil, classPath, classPath),
	}))
	refreshIndex(t, ctx, esClient, index)

	enabledLanguages := internalSearch.EnabledLanguages(nil)
	// The rest of the search matches only classDoc, so the selected unit is not in scope.
	restOfSearch := esdsl.NewNestedQuery(
		esdsl.NewTermQuery("claims.ref.to", esdsl.NewFieldValue().String(class.String())),
	).Path("claims.ref")
	// unit is the active selection; its augment is unit plus its ancestor vocabulary.
	f := search.RefFilter{To: []search.ToValue{{ID: unit}}} //nolint:exhaustruct
	resolver := newPathResolver(map[identifier.Identifier][]string{unit: {unitPath}})

	// Searching unit's display label surfaces unit (at count 0) and its ancestor vocabulary for tree context,
	// even though neither is in the search scope. The in-scope class value does not match and is not shown.
	results, _, errE := f.Get(ctx, getSearchService, restOfSearch, instanceOf, nil, "unit*", enabledLanguages, resolver)
	require.NoError(t, errE, "% -+#.1v", errE)
	byID := refResultsByID(results)
	require.Contains(t, byID, unit.String())
	assert.Equal(t, int64(0), byID[unit.String()].Count)
	assert.Equal(t, [][]string{{vocabulary.String()}}, byID[unit.String()].Paths)
	require.Contains(t, byID, vocabulary.String())
	assert.Equal(t, int64(0), byID[vocabulary.String()].Count)
	assert.NotContains(t, byID, class.String())

	// Searching unit by one of its naming strings ("metre") surfaces it too: the augment is matched by the full
	// value matcher (display plus naming), not only the display label.
	results, _, errE = f.Get(ctx, getSearchService, restOfSearch, instanceOf, nil, "metr*", enabledLanguages, resolver)
	require.NoError(t, errE, "% -+#.1v", errE)
	byID = refResultsByID(results)
	require.Contains(t, byID, unit.String())
	require.Contains(t, byID, vocabulary.String())

	// Searching the ancestor's label ("voca") surfaces vocabulary only because its descendant unit is selected;
	// unit itself does not match and is not pulled in.
	results, _, errE = f.Get(ctx, getSearchService, restOfSearch, instanceOf, nil, "voca*", enabledLanguages, resolver)
	require.NoError(t, errE, "% -+#.1v", errE)
	byID = refResultsByID(results)
	require.Contains(t, byID, vocabulary.String())
	assert.Equal(t, int64(0), byID[vocabulary.String()].Count)
	assert.NotContains(t, byID, unit.String())

	// Searching "class" matches the real in-scope class value and hides the augment entirely.
	results, _, errE = f.Get(ctx, getSearchService, restOfSearch, instanceOf, nil, "class*", enabledLanguages, resolver)
	require.NoError(t, errE, "% -+#.1v", errE)
	byID = refResultsByID(results)
	require.Contains(t, byID, class.String())
	assert.Equal(t, int64(1), byID[class.String()].Count)
	assert.NotContains(t, byID, unit.String())
	assert.NotContains(t, byID, vocabulary.String())

	// Outside a value search the whole augment (unit plus vocabulary) is force-shown at count 0 alongside the
	// in-scope class value.
	results, _, errE = f.Get(ctx, getSearchService, restOfSearch, instanceOf, nil, "", enabledLanguages, resolver)
	require.NoError(t, errE, "% -+#.1v", errE)
	byID = refResultsByID(results)
	require.Contains(t, byID, unit.String())
	assert.Equal(t, int64(0), byID[unit.String()].Count)
	assert.Equal(t, [][]string{{vocabulary.String()}}, byID[unit.String()].Paths)
	require.Contains(t, byID, vocabulary.String())
	assert.Equal(t, int64(0), byID[vocabulary.String()].Count)
	require.Contains(t, byID, class.String())
	assert.Equal(t, int64(1), byID[class.String()].Count)
}

// TestRefFilterGetSubRefSelectedAugmentValueSearchIntegration verifies the same augment searchability for
// sub-reference filters: an active sub-ref selection (plus its ancestors), which has zero documents in the
// current search scope, is searchable by display label or naming string, an ancestor surfaces only because
// its selected descendant pulls it into the augment, a non-matching term hides the augment, and outside a
// search the whole augment is shown at count 0.
func TestRefFilterGetSubRefSelectedAugmentValueSearchIntegration(t *testing.T) {
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
	other := identifier.From("other")

	animalPath := hierProp.String() + ":" + animal.String()
	mammalPath := animalPath + "/" + mammal.String()
	dogPath := mammalPath + "/" + dog.String()
	otherPath := hierProp.String() + ":" + other.String()

	subNamedClaim := func(to identifier.Identifier, display string, naming []string, toPath, fullPath string) internalSearch.SubRefClaim {
		var toNaming map[string][]string
		if naming != nil {
			toNaming = map[string][]string{"en": naming}
		}
		return internalSearch.SubRefClaim{ //nolint:exhaustruct
			ParentProp: parentProp, ParentTo: parentTo,
			ReferenceClaim: internalSearch.ReferenceClaim{ //nolint:exhaustruct
				Prop: subProp, To: to, ToDisplay: map[string]string{"en": display}, ToNaming: toNaming,
				ToPath: []string{toPath}, ToFullPath: []string{fullPath},
			},
		}
	}

	// subDog references dog (expanded to dog, mammal, animal); subOther references the unrelated other root. The
	// search scope below matches only subOther, so dog and its ancestors have zero documents in scope.
	indexDocument(t, ctx, esClient, index, internalSearch.Document{ //nolint:exhaustruct
		ID: identifier.From("subDog"),
		Claims: internalSearch.ClaimTypes{ //nolint:exhaustruct
			SubRef: internalSearch.SubRefClaims{
				subNamedClaim(dog, "dog", []string{"canine"}, dogPath, dogPath),
				subNamedClaim(mammal, "mammal", nil, mammalPath, dogPath),
				subNamedClaim(animal, "animal", nil, animalPath, dogPath),
			},
		},
	})
	indexDocument(t, ctx, esClient, index, internalSearch.Document{ //nolint:exhaustruct
		ID: identifier.From("subOther"),
		Claims: internalSearch.ClaimTypes{ //nolint:exhaustruct
			SubRef: internalSearch.SubRefClaims{
				subNamedClaim(other, "other", nil, otherPath, otherPath),
			},
		},
	})
	refreshIndex(t, ctx, esClient, index)

	enabledLanguages := internalSearch.EnabledLanguages(nil)
	restOfSearch := esdsl.NewNestedQuery(
		esdsl.NewTermQuery("claims.subRef.to", esdsl.NewFieldValue().String(other.String())),
	).Path("claims.subRef")
	f := search.RefFilter{To: []search.ToValue{{ID: dog}}} //nolint:exhaustruct
	resolver := newPathResolver(map[identifier.Identifier][]string{dog: {dogPath}})

	// Searching dog's display label surfaces dog (count 0) with its full ancestor chain, even though dog is not
	// in scope.
	results, _, errE := f.GetSubRef(ctx, getSearchService, restOfSearch, parentProp, subProp, nil, nil, "dog*", enabledLanguages, resolver)
	require.NoError(t, errE, "% -+#.1v", errE)
	byID := refResultsByID(results)
	require.Contains(t, byID, dog.String())
	assert.Equal(t, int64(0), byID[dog.String()].Count)
	assert.Equal(t, [][]string{{animal.String(), mammal.String()}}, byID[dog.String()].Paths)
	require.Contains(t, byID, mammal.String())
	require.Contains(t, byID, animal.String())
	assert.NotContains(t, byID, other.String())

	// Searching dog by a naming string ("canine") surfaces it too.
	results, _, errE = f.GetSubRef(ctx, getSearchService, restOfSearch, parentProp, subProp, nil, nil, "canin*", enabledLanguages, resolver)
	require.NoError(t, errE, "% -+#.1v", errE)
	byID = refResultsByID(results)
	require.Contains(t, byID, dog.String())

	// Searching the ancestor's label ("anim") surfaces animal only because its descendant dog is selected; dog
	// and the intermediate mammal are not pulled in.
	results, _, errE = f.GetSubRef(ctx, getSearchService, restOfSearch, parentProp, subProp, nil, nil, "anim*", enabledLanguages, resolver)
	require.NoError(t, errE, "% -+#.1v", errE)
	byID = refResultsByID(results)
	require.Contains(t, byID, animal.String())
	assert.Equal(t, int64(0), byID[animal.String()].Count)
	assert.NotContains(t, byID, dog.String())
	assert.NotContains(t, byID, mammal.String())

	// Searching "other" matches the real in-scope value and hides the augment.
	results, _, errE = f.GetSubRef(ctx, getSearchService, restOfSearch, parentProp, subProp, nil, nil, "other*", enabledLanguages, resolver)
	require.NoError(t, errE, "% -+#.1v", errE)
	byID = refResultsByID(results)
	require.Contains(t, byID, other.String())
	assert.Equal(t, int64(1), byID[other.String()].Count)
	assert.NotContains(t, byID, dog.String())
	assert.NotContains(t, byID, animal.String())

	// Outside a value search the whole augment (dog plus its ancestors) is force-shown at count 0.
	results, _, errE = f.GetSubRef(ctx, getSearchService, restOfSearch, parentProp, subProp, nil, nil, "", enabledLanguages, resolver)
	require.NoError(t, errE, "% -+#.1v", errE)
	byID = refResultsByID(results)
	require.Contains(t, byID, dog.String())
	assert.Equal(t, int64(0), byID[dog.String()].Count)
	require.Contains(t, byID, mammal.String())
	require.Contains(t, byID, animal.String())
	require.Contains(t, byID, other.String())
}
