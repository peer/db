package search_test

import (
	"os"
	"testing"

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
		ID:      identifier.From("refDoc1"),
		Display: nil,
		Text:    nil,
		Claims: internalSearch.ClaimTypes{
			Amount: nil, Time: nil,
			Reference: internalSearch.ReferenceClaims{{
				Prop: refProp, PropDisplay: nil, PropNaming: nil,
				To: target1, ToDisplay: nil, ToNaming: nil, ToPath: nil, ToDisplayPath: nil,
			}},
			Has: nil, None: nil, Unknown: nil,
			SubRef:    nil,
			SubAmount: nil,
			SubTime:   nil,
			SubHas:    nil,
		},
	})
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID:      identifier.From("refDoc2"),
		Display: nil,
		Text:    nil,
		Claims: internalSearch.ClaimTypes{
			Amount: nil, Time: nil,
			Reference: internalSearch.ReferenceClaims{{
				Prop: refProp, PropDisplay: nil, PropNaming: nil,
				To: target1, ToDisplay: nil, ToNaming: nil, ToPath: nil, ToDisplayPath: nil,
			}},
			Has: nil, None: nil, Unknown: nil,
			SubRef:    nil,
			SubAmount: nil,
			SubTime:   nil,
			SubHas:    nil,
		},
	})
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID:      identifier.From("refDoc3"),
		Display: nil,
		Text:    nil,
		Claims: internalSearch.ClaimTypes{
			Amount: nil, Time: nil,
			Reference: internalSearch.ReferenceClaims{{
				Prop: refProp, PropDisplay: nil, PropNaming: nil,
				To: target2, ToDisplay: nil, ToNaming: nil, ToPath: nil, ToDisplayPath: nil,
			}},
			Has: nil, None: nil, Unknown: nil,
			SubRef:    nil,
			SubAmount: nil,
			SubTime:   nil,
			SubHas:    nil,
		},
	})
	refreshIndex(t, ctx, esClient, index)

	// Create a session with a ref filter so we can look up the filter by ID.
	session := createSession(t, ctx, search.SessionData{
		View:  "",
		Query: "",
		Filters: []search.Filter{{ //nolint:exhaustruct
			Prop: []identifier.Identifier{refProp},
			Ref: &search.RefFilter{
				To:      []search.ToValue{{ID: target1}},
				Missing: false,
			},
		}},
		Reverse: nil,
	})

	results, metadata, errE := session.Filters[0].Ref.Get(ctx, getSearchService, session.ToQueryExcluding(*session.Filters[0].ID, nil), session.Filters[0].Prop[0])
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
		ID:      identifier.From("refDoc1"),
		Display: nil,
		Text:    nil,
		Claims: internalSearch.ClaimTypes{
			Amount: nil, Time: nil,
			Reference: internalSearch.ReferenceClaims{{
				Prop: refProp, PropDisplay: nil, PropNaming: nil,
				To: target1, ToDisplay: nil, ToNaming: nil, ToPath: nil, ToDisplayPath: nil,
			}},
			Has: nil, None: nil, Unknown: nil,
			SubRef:    nil,
			SubAmount: nil,
			SubTime:   nil,
			SubHas:    nil,
		},
	})
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID:      identifier.From("refDoc2"),
		Display: nil,
		Text:    nil,
		Claims: internalSearch.ClaimTypes{
			Amount: nil, Time: nil,
			Reference: internalSearch.ReferenceClaims{{
				Prop: refProp, PropDisplay: nil, PropNaming: nil,
				To: target2, ToDisplay: nil, ToNaming: nil, ToPath: nil, ToDisplayPath: nil,
			}},
			Has: nil, None: nil, Unknown: nil,
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
	results, metadata, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), refProp)
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
		ID:      identifier.From("refDoc1"),
		Display: nil,
		Text:    nil,
		Claims: internalSearch.ClaimTypes{
			Amount: nil, Time: nil,
			Reference: internalSearch.ReferenceClaims{{
				Prop: refProp, PropDisplay: nil, PropNaming: nil,
				To: target1, ToDisplay: nil, ToNaming: nil, ToPath: nil, ToDisplayPath: nil,
			}},
			Has: nil, None: nil, Unknown: nil,
			SubRef:    nil,
			SubAmount: nil,
			SubTime:   nil,
			SubHas:    nil,
		},
	})
	// Doc without the ref prop.
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID:      identifier.From("refDoc2"),
		Display: nil,
		Text:    nil,
		Claims: internalSearch.ClaimTypes{
			Amount: nil, Time: nil,
			Reference: nil,
			Has:       nil, None: nil, Unknown: nil,
			SubRef:    nil,
			SubAmount: nil,
			SubTime:   nil,
			SubHas:    nil,
		},
	})
	// Another doc without the ref prop.
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID:      identifier.From("refDoc3"),
		Display: nil,
		Text:    nil,
		Claims: internalSearch.ClaimTypes{
			Amount: nil, Time: nil,
			Reference: nil,
			Has:       nil, None: nil, Unknown: nil,
			SubRef:    nil,
			SubAmount: nil,
			SubTime:   nil,
			SubHas:    nil,
		},
	})
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	f := search.RefFilter{}
	results, metadata, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), refProp)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Results should include target1 (count 1) and __MISSING__ (count 2), sorted by count descending.
	assert.Equal(t, []search.RefFilterResult{
		{ID: search.MissingRefFilterID, Count: 2, Paths: nil},
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
		ID:      identifier.From("refDoc1"),
		Display: nil,
		Text:    nil,
		Claims: internalSearch.ClaimTypes{
			Amount: nil, Time: nil,
			Reference: internalSearch.ReferenceClaims{{
				Prop: refProp, PropDisplay: nil, PropNaming: nil,
				To: target1, ToDisplay: nil, ToNaming: nil, ToPath: nil, ToDisplayPath: nil,
			}},
			Has: nil, None: nil, Unknown: nil,
			SubRef:    nil,
			SubAmount: nil,
			SubTime:   nil,
			SubHas:    nil,
		},
	})
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	f := search.RefFilter{}
	results, metadata, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), refProp)
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
		ID:      identifier.From("dogDoc"),
		Display: nil,
		Text:    nil,
		Claims: internalSearch.ClaimTypes{
			Amount: nil, Time: nil,
			Reference: internalSearch.ReferenceClaims{
				{Prop: refProp, PropDisplay: nil, PropNaming: nil, To: dog, ToDisplay: nil, ToNaming: nil, ToPath: []string{dogPath}, ToDisplayPath: nil},
				{Prop: refProp, PropDisplay: nil, PropNaming: nil, To: mammal, ToDisplay: nil, ToNaming: nil, ToPath: []string{mammalPath}, ToDisplayPath: nil},
				{Prop: refProp, PropDisplay: nil, PropNaming: nil, To: animal, ToDisplay: nil, ToNaming: nil, ToPath: []string{animalPath}, ToDisplayPath: nil},
			},
			Has: nil, None: nil, Unknown: nil,
			SubRef:    nil,
			SubAmount: nil,
			SubTime:   nil,
			SubHas:    nil,
		},
	})
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	f := search.RefFilter{}
	results, metadata, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), refProp)
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
		ID:      identifier.From("leafDoc"),
		Display: nil,
		Text:    nil,
		Claims: internalSearch.ClaimTypes{
			Amount: nil, Time: nil,
			Reference: internalSearch.ReferenceClaims{
				{Prop: refProp, PropDisplay: nil, PropNaming: nil, To: leaf, ToDisplay: nil, ToNaming: nil, ToPath: []string{leafPathA, leafPathB}, ToDisplayPath: nil},
			},
			Has: nil, None: nil, Unknown: nil,
			SubRef:    nil,
			SubAmount: nil,
			SubTime:   nil,
			SubHas:    nil,
		},
	})
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	f := search.RefFilter{}
	results, _, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), refProp)
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
		ID:      identifier.From("leafDoc"),
		Display: nil,
		Text:    nil,
		Claims: internalSearch.ClaimTypes{
			Amount: nil, Time: nil,
			Reference: internalSearch.ReferenceClaims{
				{Prop: refProp, PropDisplay: nil, PropNaming: nil, To: leaf, ToDisplay: nil, ToNaming: nil, ToPath: []string{leafViaDeep, leafViaShallow}, ToDisplayPath: nil},
				{Prop: refProp, PropDisplay: nil, PropNaming: nil, To: deepParent, ToDisplay: nil, ToNaming: nil, ToPath: []string{deepParentPath}, ToDisplayPath: nil},
				{Prop: refProp, PropDisplay: nil, PropNaming: nil, To: shallowParent, ToDisplay: nil, ToNaming: nil, ToPath: []string{shallowParentPath}, ToDisplayPath: nil},
				{Prop: refProp, PropDisplay: nil, PropNaming: nil, To: mid2, ToDisplay: nil, ToNaming: nil, ToPath: []string{mid2Path}, ToDisplayPath: nil},
				{Prop: refProp, PropDisplay: nil, PropNaming: nil, To: mid1, ToDisplay: nil, ToNaming: nil, ToPath: []string{mid1Path}, ToDisplayPath: nil},
				{Prop: refProp, PropDisplay: nil, PropNaming: nil, To: root, ToDisplay: nil, ToNaming: nil, ToPath: []string{rootPath}, ToDisplayPath: nil},
			},
			Has: nil, None: nil, Unknown: nil,
			SubRef:    nil,
			SubAmount: nil,
			SubTime:   nil,
			SubHas:    nil,
		},
	})
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	f := search.RefFilter{}
	results, metadata, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), refProp)
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
		ID:      identifier.From("subDog"),
		Display: nil,
		Text:    nil,
		Claims: internalSearch.ClaimTypes{
			Amount: nil, Time: nil,
			Reference: nil,
			Has:       nil, None: nil, Unknown: nil,
			SubRef: internalSearch.SubRefClaims{
				{
					ParentProp: parentProp, ParentTo: parentTo,
					Prop: subProp, PropDisplay: nil, PropNaming: nil,
					To: dog, ToDisplay: nil, ToNaming: nil,
					ToPath: []string{dogPath}, ToDisplayPath: nil,
				},
				{
					ParentProp: parentProp, ParentTo: parentTo,
					Prop: subProp, PropDisplay: nil, PropNaming: nil,
					To: mammal, ToDisplay: nil, ToNaming: nil,
					ToPath: []string{mammalPath}, ToDisplayPath: nil,
				},
				{
					ParentProp: parentProp, ParentTo: parentTo,
					Prop: subProp, PropDisplay: nil, PropNaming: nil,
					To: animal, ToDisplay: nil, ToNaming: nil,
					ToPath: []string{animalPath}, ToDisplayPath: nil,
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
	results, metadata, errE := f.GetSubRef(ctx, getSearchService, session.ToQuery(nil), parentProp, subProp, nil)
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
