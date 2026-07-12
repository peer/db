package search_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/identifier"

	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
	"gitlab.com/peerdb/peerdb/search"
)

// sortKey builds the indexed "en" toPathSortKey value for a value at idPath whose display path is disp,
// matching what the converter stamps: the display path, then SortKeySeparator, then the id chain encoded by
// EncodeSortKeyPath. ElasticSearch folds the display half via the keyword normalizer; the hex id half survives.
func sortKey(disp, idPath string) map[string][]string {
	return map[string][]string{"en": {disp + internalSearch.SortKeySeparator + internalSearch.EncodeSortKeyPath(idPath)}}
}

// hierRef builds an indexed reference claim for a value at a hierarchy path. toPath is
// "<prop>:<root>/.../<value>" and disp is the matching display labels joined by the null byte.
//
//nolint:exhaustruct
func hierRef(prop, to identifier.Identifier, toPath, disp string, isLeaf bool) internalSearch.ReferenceClaim {
	return internalSearch.ReferenceClaim{
		Prop:          prop,
		To:            to,
		ToPath:        []string{toPath},
		ToPathSortKey: sortKey(disp, toPath),
		IsLeaf:        isLeaf,
	}
}

// flatRef builds an indexed reference claim for a flat value (one in no value hierarchy), with the self
// path the indexer stamps onto such a value: toPath is "__SELF__:<value>" and its sort key carries disp.
//
//nolint:exhaustruct
func flatRef(prop, to identifier.Identifier, disp string) internalSearch.ReferenceClaim {
	selfPath := internalSearch.SelfHierarchyPathPrefix + to.String()
	return internalSearch.ReferenceClaim{
		Prop:          prop,
		To:            to,
		ToPath:        []string{selfPath},
		ToPathSortKey: sortKey(disp, selfPath),
		IsLeaf:        true,
	}
}

//nolint:exhaustruct
func hierDoc(id identifier.Identifier, refs []internalSearch.ReferenceClaim) internalSearch.Document {
	return internalSearch.Document{
		ID:     id,
		Claims: internalSearch.ClaimTypes{Reference: refs},
	}
}

// leafIDs collects the IDs of the leaf (non-group) results directly under a group node.
func leafIDs(group []search.Result) []string {
	ids := make([]string, 0, len(group))
	for _, r := range group {
		if r.Group == nil {
			ids = append(ids, r.ID)
		}
	}
	return ids
}

func TestResultsGetGroupedIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	loc := identifier.From("locationProp")
	eu := identifier.From("europe")
	paris := identifier.From("paris")
	berlin := identifier.From("berlin")
	docA := identifier.From("docA") // in Paris.
	docB := identifier.From("docB") // in Berlin.
	docC := identifier.From("docC") // in both Paris and Berlin.

	parisPath := loc.String() + ":" + eu.String() + "/" + paris.String()
	berlinPath := loc.String() + ":" + eu.String() + "/" + berlin.String()
	euPath := loc.String() + ":" + eu.String()

	parisLeaf := hierRef(loc, paris, parisPath, "Europe\x00Paris", true)
	berlinLeaf := hierRef(loc, berlin, berlinPath, "Europe\x00Berlin", true)
	euAncestor := hierRef(loc, eu, euPath, "Europe", false)

	indexDocument(t, ctx, esClient, index, hierDoc(docA, []internalSearch.ReferenceClaim{parisLeaf, euAncestor}))
	indexDocument(t, ctx, esClient, index, hierDoc(docB, []internalSearch.ReferenceClaim{berlinLeaf, euAncestor}))
	indexDocument(t, ctx, esClient, index, hierDoc(docC, []internalSearch.ReferenceClaim{parisLeaf, berlinLeaf, euAncestor}))
	refreshIndex(t, ctx, esClient, index)

	// Group by location (a ref column). View resolves to "feed", which enables grouping.
	session := createSession(t, ctx, search.SessionData{ //nolint:exhaustruct
		Sort: []search.SortKey{{Type: "ref", Prop: []string{loc.String()}, Group: true}}, //nolint:exhaustruct
	})

	results, metadata, errE := search.ResultsGet(ctx, getSearchService, &session.SessionData, []string{"en"}, 0)
	require.NoError(t, errE, "% -+#.1v", errE)

	// total counts matching documents (not group placements): docA, docB, docC.
	assert.Equal(t, int64(3), metadata["total"])

	// One top-level group: Europe, with the two cities nested under it, ordered by display label (Berlin, Paris).
	require.Len(t, results, 1)
	assert.Equal(t, eu.String(), results[0].ID)
	require.Len(t, results[0].Group, 2)

	berlinGroup := results[0].Group[0]
	parisGroup := results[0].Group[1]
	assert.Equal(t, berlin.String(), berlinGroup.ID)
	assert.Equal(t, paris.String(), parisGroup.ID)

	// Each city group counts its documents; docC is multi-placed under both cities.
	require.NotNil(t, berlinGroup.Count)
	assert.Equal(t, int64(2), *berlinGroup.Count)
	assert.ElementsMatch(t, []string{docB.String(), docC.String()}, leafIDs(berlinGroup.Group))

	require.NotNil(t, parisGroup.Count)
	assert.Equal(t, int64(2), *parisGroup.Count)
	assert.ElementsMatch(t, []string{docA.String(), docC.String()}, leafIDs(parisGroup.Group))
}

func TestResultsGetGroupedFlatMissing(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	medium := identifier.From("mediumProp")
	other := identifier.From("otherProp")
	poster := identifier.From("poster")
	book := identifier.From("book")
	otherVal := identifier.From("otherVal")
	docA := identifier.From("docFlatA") // medium: poster.
	docB := identifier.From("docFlatB") // medium: book.
	docC := identifier.From("docFlatC") // medium: both poster and book.
	docD := identifier.From("docFlatD") // no medium.

	posterRef := flatRef(medium, poster, "Poster")
	bookRef := flatRef(medium, book, "Book")

	indexDocument(t, ctx, esClient, index, hierDoc(docA, []internalSearch.ReferenceClaim{posterRef}))
	indexDocument(t, ctx, esClient, index, hierDoc(docB, []internalSearch.ReferenceClaim{bookRef}))
	indexDocument(t, ctx, esClient, index, hierDoc(docC, []internalSearch.ReferenceClaim{posterRef, bookRef}))
	indexDocument(t, ctx, esClient, index, hierDoc(docD, []internalSearch.ReferenceClaim{flatRef(other, otherVal, "Other")}))
	refreshIndex(t, ctx, esClient, index)

	// Group by a flat ref column (medium). View resolves to "feed", which enables grouping.
	session := createSession(t, ctx, search.SessionData{ //nolint:exhaustruct
		Sort: []search.SortKey{{Type: "ref", Prop: []string{medium.String()}, Group: true}}, //nolint:exhaustruct
	})

	results, metadata, errE := search.ResultsGet(ctx, getSearchService, &session.SessionData, []string{"en"}, 0)
	require.NoError(t, errE, "% -+#.1v", errE)

	// total counts matching documents (not group placements): docA, docB, docC, docD.
	assert.Equal(t, int64(4), metadata["total"])

	// Two flat single-level groups ordered by display label (Book, Poster), then the missing group last.
	require.Len(t, results, 3)
	bookGroup := results[0]
	posterGroup := results[1]
	missingGroup := results[2]

	assert.Equal(t, book.String(), bookGroup.ID)
	require.NotNil(t, bookGroup.Count)
	assert.Equal(t, int64(2), *bookGroup.Count)
	assert.ElementsMatch(t, []string{docB.String(), docC.String()}, leafIDs(bookGroup.Group))

	assert.Equal(t, poster.String(), posterGroup.ID)
	require.NotNil(t, posterGroup.Count)
	assert.Equal(t, int64(2), *posterGroup.Count)
	assert.ElementsMatch(t, []string{docA.String(), docC.String()}, leafIDs(posterGroup.Group))

	// docD has no medium value, so it lands in the trailing synthetic "missing" group.
	assert.Equal(t, search.MissingValueID, missingGroup.ID)
	require.NotNil(t, missingGroup.Count)
	assert.Equal(t, int64(1), *missingGroup.Count)
	assert.ElementsMatch(t, []string{docD.String()}, leafIDs(missingGroup.Group))
}

func TestResultsGetGroupedMultiLevelMissing(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	medium := identifier.From("mediumProp2")
	creator := identifier.From("creatorProp2")
	poster := identifier.From("poster2")
	alice := identifier.From("alice2")
	bob := identifier.From("bob2")
	doc1 := identifier.From("docML1") // medium: poster, creator: Alice.
	doc2 := identifier.From("docML2") // medium: poster, creator: Bob.
	doc3 := identifier.From("docML3") // medium: poster, no creator.
	doc4 := identifier.From("docML4") // no medium, creator: Alice.

	posterRef := flatRef(medium, poster, "Poster")
	aliceRef := flatRef(creator, alice, "Alice")
	bobRef := flatRef(creator, bob, "Bob")

	indexDocument(t, ctx, esClient, index, hierDoc(doc1, []internalSearch.ReferenceClaim{posterRef, aliceRef}))
	indexDocument(t, ctx, esClient, index, hierDoc(doc2, []internalSearch.ReferenceClaim{posterRef, bobRef}))
	indexDocument(t, ctx, esClient, index, hierDoc(doc3, []internalSearch.ReferenceClaim{posterRef}))
	indexDocument(t, ctx, esClient, index, hierDoc(doc4, []internalSearch.ReferenceClaim{aliceRef}))
	refreshIndex(t, ctx, esClient, index)

	// Group by medium, then by creator: two leading group columns.
	session := createSession(t, ctx, search.SessionData{ //nolint:exhaustruct
		Sort: []search.SortKey{
			{Type: "ref", Prop: []string{medium.String()}, Group: true},  //nolint:exhaustruct
			{Type: "ref", Prop: []string{creator.String()}, Group: true}, //nolint:exhaustruct
		},
	})

	results, metadata, errE := search.ResultsGet(ctx, getSearchService, &session.SessionData, []string{"en"}, 0)
	require.NoError(t, errE, "% -+#.1v", errE)

	assert.Equal(t, int64(4), metadata["total"])

	// Top level: the poster group, then the missing-medium group last.
	require.Len(t, results, 2)
	posterGroup := results[0]
	missingMedium := results[1]

	assert.Equal(t, poster.String(), posterGroup.ID)
	require.NotNil(t, posterGroup.Count)
	assert.Equal(t, int64(3), *posterGroup.Count)
	assert.Equal(t, 0, posterGroup.Col)

	// Within poster, by creator: Alice, Bob, then the missing-creator group last. Their headings carry the
	// second column's index.
	require.Len(t, posterGroup.Group, 3)
	aliceSub := posterGroup.Group[0]
	bobSub := posterGroup.Group[1]
	missingCreator := posterGroup.Group[2]

	assert.Equal(t, alice.String(), aliceSub.ID)
	assert.Equal(t, 1, aliceSub.Col)
	assert.ElementsMatch(t, []string{doc1.String()}, leafIDs(aliceSub.Group))
	assert.Equal(t, bob.String(), bobSub.ID)
	assert.Equal(t, 1, bobSub.Col)
	assert.ElementsMatch(t, []string{doc2.String()}, leafIDs(bobSub.Group))
	assert.Equal(t, search.MissingValueID, missingCreator.ID)
	assert.Equal(t, 1, missingCreator.Col)
	require.NotNil(t, missingCreator.Count)
	assert.Equal(t, int64(1), *missingCreator.Count)
	assert.ElementsMatch(t, []string{doc3.String()}, leafIDs(missingCreator.Group))

	// The missing-medium group is itself grouped by creator (doc4 under Alice).
	assert.Equal(t, search.MissingValueID, missingMedium.ID)
	assert.Equal(t, 0, missingMedium.Col)
	require.NotNil(t, missingMedium.Count)
	assert.Equal(t, int64(1), *missingMedium.Count)
	require.Len(t, missingMedium.Group, 1)
	assert.Equal(t, alice.String(), missingMedium.Group[0].ID)
	assert.Equal(t, 1, missingMedium.Group[0].Col)
	assert.ElementsMatch(t, []string{doc4.String()}, leafIDs(missingMedium.Group[0].Group))
}
