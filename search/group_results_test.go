package search_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/identifier"

	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
	"gitlab.com/peerdb/peerdb/search"
)

// hierRef builds an indexed reference claim for a value at a hierarchy path. toPath is
// "<prop>:<root>/.../<value>" and disp is the matching display labels joined by the null byte.
//
//nolint:exhaustruct
func hierRef(prop, to identifier.Identifier, toPath, disp string, isLeaf bool) internalSearch.ReferenceClaim {
	return internalSearch.ReferenceClaim{
		Prop:          prop,
		To:            to,
		ToPath:        []string{toPath},
		ToDisplayPath: map[string][]string{"en": {disp}},
		IsLeaf:        isLeaf,
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
