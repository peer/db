package search_test

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/elastic/go-elasticsearch/v9/typedapi/esdsl"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"

	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
	"gitlab.com/peerdb/peerdb/search"
)

// relDoc builds an indexable document carrying only the given rel records.
func relDoc(id string, records internalSearch.RelClaims) internalSearch.Document {
	return claimsDoc(id, internalSearch.ClaimTypes{ //nolint:exhaustruct
		Rel: records,
	})
}

// relSub builds a Sub container holding only the given rel records.
func relSub(records ...internalSearch.RelClaim) *internalSearch.ClaimTypes {
	return &internalSearch.ClaimTypes{ //nolint:exhaustruct
		Rel: records,
	}
}

// pathParents returns the immediate-parent value ids for hierarchy paths, matching what the converter stamps
// as ToParent: for each path "<hierProp>:<root>/.../<self>" the id segment before the last; a self or root
// path (a single id) contributes none. The result preserves first-seen order and is nil when none has a parent.
func pathParents(toPath []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, raw := range toPath {
		_, chain, ok := strings.Cut(raw, ":")
		if !ok {
			continue
		}
		parts := strings.Split(chain, "/")
		if len(parts) < 2 {
			continue
		}
		parent := parts[len(parts)-2]
		if seen[parent] {
			continue
		}
		seen[parent] = true
		out = append(out, parent)
	}
	return out
}

// hierRelRecord builds one expanded ref rel record: the record for value to with its own hierarchy paths,
// with ToParent derived from the paths, as convertReference produces at index time. isLeaf marks the value
// as most-specific in its scope.
func hierRelRecord(prop, to identifier.Identifier, toPath []string, isLeaf bool) internalSearch.RelClaim {
	target := to
	return internalSearch.RelClaim{
		ClaimType:     internalSearch.ClaimTypeRef,
		Prop:          prop,
		PropDisplay:   nil,
		PropNaming:    nil,
		PropSortKey:   nil,
		To:            &target,
		ToDisplay:     nil,
		ToNaming:      nil,
		ToSortKey:     nil,
		ToPath:        toPath,
		ToParent:      pathParents(toPath),
		ToDisplayPath: nil,
		ToPathSortKey: nil,
		IsLeaf:        isLeaf,
		Sub:           nil,
	}
}

// namedRefRecord builds a flat ref rel record for prop pointing at to, carrying an English display label
// and optional naming strings so value-label matching can find the value.
func namedRefRecord(prop, to identifier.Identifier, display string, naming []string) internalSearch.RelClaim {
	rec := refRecord(prop, to, nil)
	rec.ToDisplay = map[string]string{"en": display}
	if naming != nil {
		rec.ToNaming = map[string][]string{"en": naming}
	}
	return rec
}

// refResultsByID indexes reference filter results by their value id for assertions.
func refResultsByID(results []search.RefFilterResult) map[string]search.RefFilterResult {
	out := make(map[string]search.RefFilterResult, len(results))
	for _, r := range results {
		out[r.ID] = r
	}
	return out
}

func TestRefFilterGetIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	refProp := identifier.From("refProp")
	target1 := identifier.From("target1")
	target2 := identifier.From("target2")

	indexDocument(t, ctx, esClient, index, relDoc("refDoc1", internalSearch.RelClaims{refRecord(refProp, target1, nil)}))
	indexDocument(t, ctx, esClient, index, relDoc("refDoc2", internalSearch.RelClaims{refRecord(refProp, target1, nil)}))
	indexDocument(t, ctx, esClient, index, relDoc("refDoc3", internalSearch.RelClaims{refRecord(refProp, target2, nil)}))
	refreshIndex(t, ctx, esClient, index)

	// Create a session with a ref filter so we can look up the filter by ID.
	session := createSession(t, ctx, search.SessionData{ //nolint:exhaustruct
		Filters: []search.Filter{{ //nolint:exhaustruct
			Prop: []identifier.Identifier{refProp},
			Ref: &search.RefFilter{
				To:     []search.ToValue{{ID: target1}},
				Direct: nil,
			},
		}},
	})

	// The facet's own filter (and the path's specials filter, of which there is none) is excluded from the
	// query, exactly as the handler does, so the facet lists all available values.
	excludeIDs := session.FacetExcludeIDs(session.Filters[0].Prop, session.Filters[0].ID)
	results, metadata, errE := session.Filters[0].Ref.Get(
		ctx, getSearchService, session.ToQueryExcluding(excludeIDs, nil), session.Filters[0].Prop[0], nil, "", nil, nil,
	)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Results are sorted by count descending: target1 (count 2) first, target2 (count 1) second.
	assert.Equal(t, []search.RefFilterResult{
		{ID: target1.String(), Count: 2, ChildCount: 0, Paths: nil},
		{ID: target2.String(), Count: 1, ChildCount: 0, Paths: nil},
	}, results)
	assert.Equal(t, "2", metadata["total"])
	// All three documents are in the facet's universe and none has an amount or time claim for the property.
	assert.Equal(t, "3", metadata["universe"])
	assert.Equal(t, "0", metadata["other_types"])
}

func TestRefFilterGetInactiveIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	refProp := identifier.From("refProp")
	target1 := identifier.From("target1")
	target2 := identifier.From("target2")

	indexDocument(t, ctx, esClient, index, relDoc("refDoc1", internalSearch.RelClaims{refRecord(refProp, target1, nil)}))
	indexDocument(t, ctx, esClient, index, relDoc("refDoc2", internalSearch.RelClaims{refRecord(refProp, target2, nil)}))
	refreshIndex(t, ctx, esClient, index)

	// Create a session without any filters (inactive filter scenario).
	session := createSession(t, ctx, search.SessionData{})

	// Query for ref filter values using the session's full query and prop from outside the session.
	f := search.RefFilter{To: nil, Direct: nil}
	results, metadata, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), refProp, nil, "", nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Results order is non-deterministic when counts are equal.
	assert.ElementsMatch(t, []search.RefFilterResult{
		{ID: target1.String(), Count: 1, ChildCount: 0, Paths: nil},
		{ID: target2.String(), Count: 1, ChildCount: 0, Paths: nil},
	}, results)
	assert.Equal(t, "2", metadata["total"])
	assert.Equal(t, "2", metadata["universe"])
}

func TestRefFilterGetMissingIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	refProp := identifier.From("refProp")
	target1 := identifier.From("target1")

	// Doc with the ref prop.
	indexDocument(t, ctx, esClient, index, relDoc("refDoc1", internalSearch.RelClaims{refRecord(refProp, target1, nil)}))
	// Two docs without any claim for the ref prop.
	indexDocument(t, ctx, esClient, index, claimsDoc("refDoc2", internalSearch.ClaimTypes{}))
	indexDocument(t, ctx, esClient, index, claimsDoc("refDoc3", internalSearch.ClaimTypes{}))
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	f := search.RefFilter{To: nil, Direct: nil}
	results, metadata, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), refProp, nil, "", nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Results should include target1 (count 1) and __MISSING__ (count 2), sorted by count descending.
	assert.Equal(t, []search.RefFilterResult{
		{ID: search.MissingValueID, Count: 2, ChildCount: 0, Paths: nil},
		{ID: target1.String(), Count: 1, ChildCount: 0, Paths: nil},
	}, results)
	// Total includes the missing entry.
	assert.Equal(t, "2", metadata["total"])
	// The identity closes: value (1) plus missing (2) equals the universe (3).
	assert.Equal(t, "3", metadata["universe"])
}

// TestRefFilterGetMissingUniverseRuleIntegration verifies the universe rule for the top-level missing count:
// a document is missing the property only when it has no rel, no amount, and no time claim for it. Documents
// stating the property in another value type are not missing; they are surfaced through the otherTypes
// metadata instead, closing the facet's identity.
func TestRefFilterGetMissingUniverseRuleIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	refProp := identifier.From("refProp")
	target1 := identifier.From("target1")

	amountFrom := 9.5
	amountTo := 10.5
	timeFrom := float64(1000)
	timeTo := float64(1001)

	// A doc with a ref claim for the property.
	indexDocument(t, ctx, esClient, index, relDoc("refDoc1", internalSearch.RelClaims{refRecord(refProp, target1, nil)}))
	// A doc stating the property only as an amount claim: not missing, counted in otherTypes.
	indexDocument(t, ctx, esClient, index, claimsDoc("amountDoc", internalSearch.ClaimTypes{ //nolint:exhaustruct
		Amount: internalSearch.AmountClaims{amountRecord(refProp, nil, &amountFrom, &amountTo, nil)},
	}))
	// A doc stating the property only as a time claim: not missing, counted in otherTypes.
	indexDocument(t, ctx, esClient, index, claimsDoc("timeDoc", internalSearch.ClaimTypes{ //nolint:exhaustruct
		Time: internalSearch.TimeClaims{timeRecord(refProp, &timeFrom, &timeTo, nil)},
	}))
	// A doc with no claim for the property at all: the only missing one.
	indexDocument(t, ctx, esClient, index, claimsDoc("emptyDoc", internalSearch.ClaimTypes{}))
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	f := search.RefFilter{To: nil, Direct: nil}
	results, metadata, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), refProp, nil, "", nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	assert.Equal(t, []search.RefFilterResult{
		{ID: target1.String(), Count: 1, ChildCount: 0, Paths: nil},
		{ID: search.MissingValueID, Count: 1, ChildCount: 0, Paths: nil},
	}, results)
	assert.Equal(t, "2", metadata["total"])
	assert.Equal(t, "4", metadata["universe"])
	// The amount-only and time-only documents are reachable through the property's other value types.
	assert.Equal(t, "2", metadata["other_types"])
}

func TestRefFilterGetNoMissingIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	refProp := identifier.From("refProp")
	target1 := identifier.From("target1")

	// All docs have the ref prop.
	indexDocument(t, ctx, esClient, index, relDoc("refDoc1", internalSearch.RelClaims{refRecord(refProp, target1, nil)}))
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	f := search.RefFilter{To: nil, Direct: nil}
	results, metadata, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), refProp, nil, "", nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	// No missing entry since all documents have the prop.
	assert.Equal(t, []search.RefFilterResult{
		{ID: target1.String(), Count: 1, ChildCount: 0, Paths: nil},
	}, results)
	assert.Equal(t, "1", metadata["total"])
}

// TestRefFilterGetSpecialEntriesIntegration verifies the special-value entries of a value-list facet: has
// (a has claim, here one with sub-claims, which is indexed as a has rel record), unknown, none, and missing
// each surface as their synthetic entry, and together with the value entries they partition the universe.
func TestRefFilterGetSpecialEntriesIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	refProp := identifier.From("refProp")
	target1 := identifier.From("target1")
	subProp := identifier.From("subProp")
	subTarget := identifier.From("subTarget")

	indexDocument(t, ctx, esClient, index, relDoc("refClaimTypeDoc", internalSearch.RelClaims{refRecord(refProp, target1, nil)}))
	// A has claim WITH sub-claims: indexed as a has rel record with a Sub container, so it counts toward the
	// property's has special (and toward its sub facets), not toward a pooled facet of another property.
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
	results, metadata, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), refProp, nil, "", nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	// All entries have count 1; the value entry precedes the special entries, which keep their fixed order
	// (has property, unknown, none, missing).
	assert.Equal(t, []search.RefFilterResult{
		{ID: target1.String(), Count: 1, ChildCount: 0, Paths: nil},
		{ID: search.HasPropertyValueID, Count: 1, ChildCount: 0, Paths: nil},
		{ID: search.UnknownValueID, Count: 1, ChildCount: 0, Paths: nil},
		{ID: search.NoneValueID, Count: 1, ChildCount: 0, Paths: nil},
		{ID: search.MissingValueID, Count: 1, ChildCount: 0, Paths: nil},
	}, results)
	assert.Equal(t, "5", metadata["total"])
	// The identity closes: value (1) + has (1) + unknown (1) + none (1) + missing (1) = universe (5).
	assert.Equal(t, "5", metadata["universe"])
	assert.Equal(t, "0", metadata["other_types"])
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

	// One source doc with three ref records, one per target in the chain, as produced at index time by
	// ancestor expansion in convertReference; the stated value dog is the scope's leaf.
	indexDocument(t, ctx, esClient, index, relDoc("dogDoc", internalSearch.RelClaims{
		hierRelRecord(refProp, dog, []string{dogPath}, true),
		hierRelRecord(refProp, mammal, []string{mammalPath}, false),
		hierRelRecord(refProp, animal, []string{animalPath}, false),
	}))
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	f := search.RefFilter{To: nil, Direct: nil}
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
	// expansion (not most-specific, so isLeaf is false on the artist record).
	painterClaims := internalSearch.RelClaims{
		hierRelRecord(refProp, painter, []string{painterPath}, true),
		hierRelRecord(refProp, artist, []string{artistPath}, false),
	}
	// A sculptor document is most-specific sculptor (isLeaf), and also an artist via ancestor
	// expansion. There are more sculptors than artist-only documents, so the sculptor value
	// outcounts the artist "direct" entry, while painter undercounts it.
	sculptorClaims := internalSearch.RelClaims{
		hierRelRecord(refProp, sculptor, []string{sculptorPath}, true),
		hierRelRecord(refProp, artist, []string{artistPath}, false),
	}
	// An artist-only document is most-specific artist (isLeaf), with no narrower painter or sculptor.
	artistClaims := internalSearch.RelClaims{
		hierRelRecord(refProp, artist, []string{artistPath}, true),
	}

	indexDocument(t, ctx, esClient, index, relDoc("painterDoc1", painterClaims))
	indexDocument(t, ctx, esClient, index, relDoc("painterDoc2", painterClaims))
	indexDocument(t, ctx, esClient, index, relDoc("sculptorDoc1", sculptorClaims))
	indexDocument(t, ctx, esClient, index, relDoc("sculptorDoc2", sculptorClaims))
	indexDocument(t, ctx, esClient, index, relDoc("sculptorDoc3", sculptorClaims))
	indexDocument(t, ctx, esClient, index, relDoc("sculptorDoc4", sculptorClaims))
	indexDocument(t, ctx, esClient, index, relDoc("artistDoc1", artistClaims))
	indexDocument(t, ctx, esClient, index, relDoc("artistDoc2", artistClaims))
	indexDocument(t, ctx, esClient, index, relDoc("artistDoc3", artistClaims))
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	f := search.RefFilter{To: nil, Direct: nil}
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

	// Filter selections compile only through the session query. The "direct" selection selects exactly the
	// artist-only documents (most-specific artist), none of the painters or sculptors.
	directSession := createSession(t, ctx, search.SessionData{ //nolint:exhaustruct
		Filters: []search.Filter{{ //nolint:exhaustruct
			Prop: []identifier.Identifier{refProp},
			Ref:  &search.RefFilter{To: nil, Direct: []search.ToValue{{ID: artist}}},
		}},
	})
	assert.ElementsMatch(t, []string{artistDoc1.String(), artistDoc2.String(), artistDoc3.String()}, hitIDs(directSession.ToQuery(nil)))

	// The plain value selection selects every artist, painters and sculptors included.
	toSession := createSession(t, ctx, search.SessionData{ //nolint:exhaustruct
		Filters: []search.Filter{{ //nolint:exhaustruct
			Prop: []identifier.Identifier{refProp},
			Ref:  &search.RefFilter{To: []search.ToValue{{ID: artist}}, Direct: nil},
		}},
	})
	assert.ElementsMatch(t,
		[]string{
			painterDoc1.String(), painterDoc2.String(),
			sculptorDoc1.String(), sculptorDoc2.String(), sculptorDoc3.String(), sculptorDoc4.String(),
			artistDoc1.String(), artistDoc2.String(), artistDoc3.String(),
		},
		hitIDs(toSession.ToQuery(nil)),
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

	indexDocument(t, ctx, esClient, index, relDoc("leafDoc", internalSearch.RelClaims{
		hierRelRecord(refProp, leaf, []string{leafPathA, leafPathB}, true),
	}))
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	f := search.RefFilter{To: nil, Direct: nil}
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

	// One source doc, instance of leaf, expanded to a ref record per ancestor as convertReference
	// does at index time. Every bucket therefore has the same single-document count, so ordering is
	// decided entirely by hierarchy depth.
	indexDocument(t, ctx, esClient, index, relDoc("leafDoc", internalSearch.RelClaims{
		hierRelRecord(refProp, leaf, []string{leafViaDeep, leafViaShallow}, true),
		hierRelRecord(refProp, deepParent, []string{deepParentPath}, false),
		hierRelRecord(refProp, shallowParent, []string{shallowParentPath}, false),
		hierRelRecord(refProp, mid2, []string{mid2Path}, false),
		hierRelRecord(refProp, mid1, []string{mid1Path}, false),
		hierRelRecord(refProp, root, []string{rootPath}, false),
	}))
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	f := search.RefFilter{To: nil, Direct: nil}
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
	parentTarget := identifier.From("parentToValue")
	subProp := identifier.From("subProp")
	hierProp := identifier.From("hierProp")
	animal := identifier.From("animal")
	mammal := identifier.From("mammal")
	dog := identifier.From("dog")

	animalPath := hierProp.String() + ":" + animal.String()
	mammalPath := hierProp.String() + ":" + animal.String() + "/" + mammal.String()
	dogPath := hierProp.String() + ":" + animal.String() + "/" + mammal.String() + "/" + dog.String()

	// Three expanded sub ref records in the parent claim's Sub container, one per target in the chain.
	indexDocument(t, ctx, esClient, index, relDoc("subDog", internalSearch.RelClaims{
		refRecord(parentProp, parentTarget, relSub(
			hierRelRecord(subProp, dog, []string{dogPath}, true),
			hierRelRecord(subProp, mammal, []string{mammalPath}, false),
			hierRelRecord(subProp, animal, []string{animalPath}, false),
		)),
	}))
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	f := search.RefFilter{To: nil, Direct: nil}
	parentCtx := session.ParentContextFor(parentProp, subProp)
	results, metadata, errE := f.GetSubRef(ctx, getSearchService, session.ToQuery(nil), subProp, parentCtx, nil, "", nil, nil)
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
	// The sub facet's universe is the documents with a parent claim.
	assert.Equal(t, "1", metadata["universe"])
}

// TestRefFilterGetSubRefValuelessParentsIntegration verifies that sub facets aggregate sub-claims under
// parent claims of any rel claimType: sub records nested in a has and in an unknown parent claim both
// contribute to the (parentProp, subProp) facet, and both parents count toward its universe.
func TestRefFilterGetSubRefValuelessParentsIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	parentProp := identifier.From("parentProp")
	subProp := identifier.From("subProp")
	valueA := identifier.From("valueA")

	indexDocument(t, ctx, esClient, index, relDoc("hasParentDoc", internalSearch.RelClaims{
		simpleRelRecord(internalSearch.ClaimTypeHas, parentProp, relSub(refRecord(subProp, valueA, nil))),
	}))
	indexDocument(t, ctx, esClient, index, relDoc("unknownParentDoc", internalSearch.RelClaims{
		simpleRelRecord(internalSearch.ClaimTypeUnknown, parentProp, relSub(refRecord(subProp, valueA, nil))),
	}))
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	f := search.RefFilter{To: nil, Direct: nil}
	parentCtx := session.ParentContextFor(parentProp, subProp)
	results, metadata, errE := f.GetSubRef(ctx, getSearchService, session.ToQuery(nil), subProp, parentCtx, nil, "", nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	assert.Equal(t, []search.RefFilterResult{
		{ID: valueA.String(), Count: 2, ChildCount: 0, Paths: nil},
	}, results)
	assert.Equal(t, "1", metadata["total"])
	assert.Equal(t, "2", metadata["universe"])
}

// TestRefFilterGetSubRefMissingIntegration verifies the sub facet's missing entry: it counts documents
// whose parent claims all lack a facetable sub-claim for the sub property, and it requires a parent claim
// to exist, so a document without any parent claim is outside the universe and is NOT missing.
func TestRefFilterGetSubRefMissingIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	parentProp := identifier.From("parentProp")
	parentTarget := identifier.From("parentToValue")
	otherProp := identifier.From("otherProp")
	otherTarget := identifier.From("otherTarget")
	subProp := identifier.From("subProp")
	valueA := identifier.From("valueA")

	// A parent claim carrying the sub property.
	indexDocument(t, ctx, esClient, index, relDoc("withSubDoc", internalSearch.RelClaims{
		refRecord(parentProp, parentTarget, relSub(refRecord(subProp, valueA, nil))),
	}))
	// A parent claim without any sub-claim for the sub property: missing.
	indexDocument(t, ctx, esClient, index, relDoc("parentOnlyDoc", internalSearch.RelClaims{
		refRecord(parentProp, parentTarget, nil),
	}))
	// No parent claim at all: outside the sub facet's universe, so not missing.
	indexDocument(t, ctx, esClient, index, relDoc("unrelatedDoc", internalSearch.RelClaims{
		refRecord(otherProp, otherTarget, nil),
	}))
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	f := search.RefFilter{To: nil, Direct: nil}
	parentCtx := session.ParentContextFor(parentProp, subProp)
	results, metadata, errE := f.GetSubRef(ctx, getSearchService, session.ToQuery(nil), subProp, parentCtx, nil, "", nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	assert.Equal(t, []search.RefFilterResult{
		{ID: valueA.String(), Count: 1, ChildCount: 0, Paths: nil},
		{ID: search.MissingValueID, Count: 1, ChildCount: 0, Paths: nil},
	}, results)
	assert.Equal(t, "2", metadata["total"])
	// Only the two documents with a parent claim are in the universe; value (1) plus missing (1) covers it.
	assert.Equal(t, "2", metadata["universe"])
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
	indexDocument(t, ctx, esClient, index, relDoc("dogDoc", internalSearch.RelClaims{
		hierRelRecord(refProp, dog, []string{dogViaMammal, dogViaPet}, true),
		hierRelRecord(refProp, mammal, []string{mammalPath}, false),
		hierRelRecord(refProp, pet, []string{petPath}, false),
	}))
	// catDoc references cat (expanded to cat plus its single parent mammal).
	indexDocument(t, ctx, esClient, index, relDoc("catDoc", internalSearch.RelClaims{
		hierRelRecord(refProp, cat, []string{catViaMammal}, true),
		hierRelRecord(refProp, mammal, []string{mammalPath}, false),
	}))
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	f := search.RefFilter{To: nil, Direct: nil}
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
	indexDocument(t, ctx, esClient, index, relDoc("refDoc1", internalSearch.RelClaims{
		namedRefRecord(refProp, germany, "Germany", []string{"Deutschland"}),
	}))
	indexDocument(t, ctx, esClient, index, relDoc("refDoc2", internalSearch.RelClaims{
		namedRefRecord(refProp, france, "France", nil),
	}))
	// A document referencing a value under a different property whose label also matches "germ*". The value
	// query on refProp must not leak this value, which guards against the per-property scope being dropped.
	indexDocument(t, ctx, esClient, index, relDoc("refDoc3", internalSearch.RelClaims{
		namedRefRecord(otherProp, germanium, "Germanium", nil),
	}))
	// A document without refProp contributes a missing entry that the value query must drop.
	indexDocument(t, ctx, esClient, index, claimsDoc("refDoc4", internalSearch.ClaimTypes{}))
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	enabledLanguages := internalSearch.EnabledLanguages(nil)
	f := search.RefFilter{To: nil, Direct: nil}

	// The value query (a prefix wildcard, as the frontend appends) narrows the facet to the matching value
	// under this property only. Germanium matches "germ*" too but belongs to otherProp, so it must not leak.
	// The missing entry is dropped because it has no display label to match.
	results, metadata, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), refProp, nil, "germ*", searchLangs(enabledLanguages), nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []search.RefFilterResult{
		{ID: germany.String(), Count: 1, ChildCount: 0, Paths: nil},
	}, results)
	assert.Equal(t, "1", metadata["total"])

	// Matching is over all naming strings, not just the display label: Germany's alternative name
	// "Deutschland" is found even though its display label is "Germany".
	results, _, errE = f.Get(ctx, getSearchService, session.ToQuery(nil), refProp, nil, "deutsch*", searchLangs(enabledLanguages), nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []search.RefFilterResult{
		{ID: germany.String(), Count: 1, ChildCount: 0, Paths: nil},
	}, results)

	// A bare "*" matches everything, including this property's own name, so the whole facet is shown (all
	// values plus the missing entry), still scoped to this property.
	results, metadata, errE = f.Get(ctx, getSearchService, session.ToQuery(nil), refProp, nil, "*", searchLangs(enabledLanguages), nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.ElementsMatch(t, []search.RefFilterResult{
		{ID: germany.String(), Count: 1, ChildCount: 0, Paths: nil},
		{ID: france.String(), Count: 1, ChildCount: 0, Paths: nil},
		{ID: search.MissingValueID, Count: 2, ChildCount: 0, Paths: nil},
	}, results)
	assert.Equal(t, "3", metadata["total"])

	// An empty value query restores all values, including the missing entry.
	results, metadata, errE = f.Get(ctx, getSearchService, session.ToQuery(nil), refProp, nil, "", searchLangs(enabledLanguages), nil)
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
	parentTarget := identifier.From("venue")
	subProp := identifier.From("hasUser")
	alice := identifier.From("alice")

	// A sub-reference facet "has location > has user" with value "Alice". The parent property's label lives
	// on the parent record and the sub-property's label on the sub record, so the facet can be matched by
	// either property name or by the value's name.
	subRecord := namedRefRecord(subProp, alice, "Alice", nil)
	subRecord.PropDisplay = map[string]string{"en": "has user"}
	parentRecord := refRecord(parentProp, parentTarget, relSub(subRecord))
	parentRecord.PropDisplay = map[string]string{"en": "has location"}
	indexDocument(t, ctx, esClient, index, relDoc("subDoc1", internalSearch.RelClaims{parentRecord}))
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})
	enabledLanguages := internalSearch.EnabledLanguages(nil)
	f := search.RefFilter{To: nil, Direct: nil}
	parentCtx := session.ParentContextFor(parentProp, subProp)

	expected := []search.RefFilterResult{{ID: alice.String(), Count: 1, ChildCount: 0, Paths: nil}}

	// Matched by the parent property's name ("has location").
	results, _, errE := f.GetSubRef(ctx, getSearchService, session.ToQuery(nil), subProp, parentCtx, nil, "has location*", searchLangs(enabledLanguages), nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, expected, results)

	// Matched by the sub-property's name ("has user").
	results, _, errE = f.GetSubRef(ctx, getSearchService, session.ToQuery(nil), subProp, parentCtx, nil, "has user*", searchLangs(enabledLanguages), nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, expected, results)

	// Matched by the value's name ("Alice").
	results, _, errE = f.GetSubRef(ctx, getSearchService, session.ToQuery(nil), subProp, parentCtx, nil, "alic*", searchLangs(enabledLanguages), nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, expected, results)

	// A query that matches neither the parent, sub-property, nor value names returns nothing.
	results, _, errE = f.GetSubRef(ctx, getSearchService, session.ToQuery(nil), subProp, parentCtx, nil, "zzz*", searchLangs(enabledLanguages), nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Empty(t, results)
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
	indexDocument(t, ctx, esClient, index, relDoc("unitDoc", internalSearch.RelClaims{
		hierRelRecord(instanceOf, unit, []string{unitPath}, true),
		hierRelRecord(instanceOf, vocabulary, []string{vocabularyPath}, false),
	}))
	indexDocument(t, ctx, esClient, index, relDoc("classDoc", internalSearch.RelClaims{
		hierRelRecord(instanceOf, class, []string{classPath}, true),
		hierRelRecord(instanceOf, vocabulary, []string{vocabularyPath}, false),
	}))
	refreshIndex(t, ctx, esClient, index)

	// The rest of the search matches only classDoc, so unit has zero documents here. Both unit and class are
	// selected; unit must still appear (at count 0) together with its ancestor vocabulary.
	restOfSearch := esdsl.NewNestedQuery(
		esdsl.NewTermQuery("claims.rel.to", esdsl.NewFieldValue().String(class.String())),
	).Path("claims.rel")
	f := search.RefFilter{To: []search.ToValue{{ID: class}, {ID: unit}}, Direct: nil}
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

	indexDocument(t, ctx, esClient, index, relDoc("classDoc", internalSearch.RelClaims{
		hierRelRecord(instanceOf, class, []string{classPath}, true),
		hierRelRecord(instanceOf, vocabulary, []string{vocabularyPath}, false),
	}))
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	// ghost is selected but referenced by no document, so it has no indexed toPath. It must still be returned
	// flat (no ancestors) at count 0.
	f := search.RefFilter{To: []search.ToValue{{ID: ghost}}, Direct: nil}
	results, _, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), instanceOf, nil, "", nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	byID := refResultsByID(results)
	require.Contains(t, byID, ghost.String())
	assert.Equal(t, int64(0), byID[ghost.String()].Count)
	assert.Empty(t, byID[ghost.String()].Paths)
}

// TestRefFilterGetSubRefSelectedValueWithAncestorsIntegration verifies the same selected-value surfacing for
// sub facets: an active sub-ref selection is always shown together with its ancestor chain, even when it
// matches no document under the rest of the search.
func TestRefFilterGetSubRefSelectedValueWithAncestorsIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	parentProp := identifier.From("parentProp")
	parentTarget := identifier.From("parentToValue")
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

	// subDog references dog (expanded to dog, mammal, animal); subCat references cat (expanded likewise).
	// The expanded records live in the parent claim's Sub container.
	indexDocument(t, ctx, esClient, index, relDoc("subDog", internalSearch.RelClaims{
		refRecord(parentProp, parentTarget, relSub(
			hierRelRecord(subProp, dog, []string{dogPath}, true),
			hierRelRecord(subProp, mammal, []string{mammalPath}, false),
			hierRelRecord(subProp, animal, []string{animalPath}, false),
		)),
	}))
	indexDocument(t, ctx, esClient, index, relDoc("subCat", internalSearch.RelClaims{
		refRecord(parentProp, parentTarget, relSub(
			hierRelRecord(subProp, cat, []string{catPath}, true),
			hierRelRecord(subProp, mammal, []string{mammalPath}, false),
			hierRelRecord(subProp, animal, []string{animalPath}, false),
		)),
	}))
	refreshIndex(t, ctx, esClient, index)

	// The rest of the search matches only subCat, so dog has zero documents here. dog is selected; it must
	// still appear at count 0 with its full ancestor chain (animal -> mammal -> dog).
	restOfSearch := esdsl.NewNestedQuery(
		esdsl.NewNestedQuery(
			esdsl.NewTermQuery("claims.rel.sub.rel.to", esdsl.NewFieldValue().String(cat.String())),
		).Path("claims.rel.sub.rel"),
	).Path("claims.rel")
	f := search.RefFilter{To: []search.ToValue{{ID: dog}}, Direct: nil}
	resolver := newPathResolver(map[identifier.Identifier][]string{dog: {dogPath}})
	var sessionData search.SessionData
	parentCtx := sessionData.ParentContextFor(parentProp, subProp)
	results, _, errE := f.GetSubRef(ctx, getSearchService, restOfSearch, subProp, parentCtx, nil, "", nil, resolver)
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

// TestRefFilterGetMissingOnlySelectionIntegration verifies that a missing-only specials selection that
// matches nothing still produces the missing row (at count 0) so it can be unchecked, without needing the
// selected-values aggregation.
func TestRefFilterGetMissingOnlySelectionIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	instanceOf := identifier.From("instanceOf")
	class := identifier.From("class")

	// Every indexed document has the property, so the missing count is zero and the missing row is added only
	// because the path's specials selection selects it.
	indexDocument(t, ctx, esClient, index, relDoc("classDoc", internalSearch.RelClaims{
		refRecord(instanceOf, class, nil),
	}))
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	f := search.RefFilter{To: nil, Direct: nil}
	specials := &search.SpecialsFilter{Missing: true, None: false, Unknown: false, HasProperty: false}
	results, _, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), instanceOf, specials, "", nil, nil)
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

	// A ref record carrying a display label (so the value-query label match can find it) and its toPath.
	hierNamed := func(to identifier.Identifier, display, toPath string, isLeaf bool) internalSearch.RelClaim {
		rec := hierRelRecord(instanceOf, to, []string{toPath}, isLeaf)
		rec.ToDisplay = map[string]string{"en": display}
		return rec
	}

	// Hierarchy: vocabulary > {unit, language}; class is a separate root. Counts: vocabulary 3 (two unit docs
	// plus one language doc), unit 2, language 1, class 1.
	indexDocument(t, ctx, esClient, index, relDoc("unitDoc1", internalSearch.RelClaims{
		hierNamed(unit, "unit", unitPath, true),
		hierNamed(vocabulary, "vocabulary", vocabularyPath, false),
	}))
	indexDocument(t, ctx, esClient, index, relDoc("unitDoc2", internalSearch.RelClaims{
		hierNamed(unit, "unit", unitPath, true),
		hierNamed(vocabulary, "vocabulary", vocabularyPath, false),
	}))
	indexDocument(t, ctx, esClient, index, relDoc("languageDoc", internalSearch.RelClaims{
		hierNamed(language, "language", languagePath, true),
		hierNamed(vocabulary, "vocabulary", vocabularyPath, false),
	}))
	indexDocument(t, ctx, esClient, index, relDoc("classDoc", internalSearch.RelClaims{
		hierNamed(class, "class", classPath, true),
	}))
	refreshIndex(t, ctx, esClient, index)

	enabledLanguages := internalSearch.EnabledLanguages(nil)
	query := createSession(t, ctx, search.SessionData{}).ToQuery(enabledLanguages)
	// unit is the active selection; this must not force it to show during a search that it does not match.
	f := search.RefFilter{To: []search.ToValue{{ID: unit}}, Direct: nil}
	resolver := newPathResolver(map[identifier.Identifier][]string{unit: {unitPath}})

	// Searching the value name "unit" shows unit and, for tree context, its ancestor vocabulary with its real
	// (no-search) count of 3, not 0. The sibling language and the unrelated class are not shown.
	results, metadata, errE := f.Get(ctx, getSearchService, query, instanceOf, nil, "unit*", searchLangs(enabledLanguages), resolver)
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
	results, metadata, errE = f.Get(ctx, getSearchService, query, instanceOf, nil, "voca*", searchLangs(enabledLanguages), resolver)
	require.NoError(t, errE, "% -+#.1v", errE)
	byID = refResultsByID(results)
	require.Contains(t, byID, vocabulary.String())
	assert.Equal(t, int64(3), byID[vocabulary.String()].Count)
	assert.NotContains(t, byID, unit.String())
	assert.NotContains(t, byID, language.String())
	assert.NotContains(t, byID, class.String())
	assert.Equal(t, "1", metadata["total"])

	// Searching "class" shows only class. The selected unit and its ancestor vocabulary are not force-shown.
	results, _, errE = f.Get(ctx, getSearchService, query, instanceOf, nil, "class*", searchLangs(enabledLanguages), resolver)
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

	// A ref record carrying a display label and optional naming strings (so the value-query label match can
	// find the value by either), plus its toPath.
	hierNamed := func(to identifier.Identifier, display string, naming []string, toPath string, isLeaf bool) internalSearch.RelClaim {
		rec := hierRelRecord(instanceOf, to, []string{toPath}, isLeaf)
		rec.ToDisplay = map[string]string{"en": display}
		if naming != nil {
			rec.ToNaming = map[string][]string{"en": naming}
		}
		return rec
	}

	// unitDoc references unit (expanded to unit + vocabulary); classDoc references class. The search scope below
	// matches only classDoc, so unit and vocabulary have zero documents in scope, yet exist globally.
	indexDocument(t, ctx, esClient, index, relDoc("unitDoc", internalSearch.RelClaims{
		hierNamed(unit, "unit", []string{"metre"}, unitPath, true),
		hierNamed(vocabulary, "vocabulary", nil, vocabularyPath, false),
	}))
	indexDocument(t, ctx, esClient, index, relDoc("classDoc", internalSearch.RelClaims{
		hierNamed(class, "class", nil, classPath, true),
	}))
	refreshIndex(t, ctx, esClient, index)

	enabledLanguages := internalSearch.EnabledLanguages(nil)
	// The rest of the search matches only classDoc, so the selected unit is not in scope.
	restOfSearch := esdsl.NewNestedQuery(
		esdsl.NewTermQuery("claims.rel.to", esdsl.NewFieldValue().String(class.String())),
	).Path("claims.rel")
	// unit is the active selection; its augment is unit plus its ancestor vocabulary.
	f := search.RefFilter{To: []search.ToValue{{ID: unit}}, Direct: nil}
	resolver := newPathResolver(map[identifier.Identifier][]string{unit: {unitPath}})

	// Searching unit's display label surfaces unit (at count 0) and its ancestor vocabulary for tree context,
	// even though neither is in the search scope. The in-scope class value does not match and is not shown.
	results, _, errE := f.Get(ctx, getSearchService, restOfSearch, instanceOf, nil, "unit*", searchLangs(enabledLanguages), resolver)
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
	results, _, errE = f.Get(ctx, getSearchService, restOfSearch, instanceOf, nil, "metr*", searchLangs(enabledLanguages), resolver)
	require.NoError(t, errE, "% -+#.1v", errE)
	byID = refResultsByID(results)
	require.Contains(t, byID, unit.String())
	require.Contains(t, byID, vocabulary.String())

	// Searching the ancestor's label ("voca") surfaces vocabulary only because its descendant unit is selected;
	// unit itself does not match and is not pulled in.
	results, _, errE = f.Get(ctx, getSearchService, restOfSearch, instanceOf, nil, "voca*", searchLangs(enabledLanguages), resolver)
	require.NoError(t, errE, "% -+#.1v", errE)
	byID = refResultsByID(results)
	require.Contains(t, byID, vocabulary.String())
	assert.Equal(t, int64(0), byID[vocabulary.String()].Count)
	assert.NotContains(t, byID, unit.String())

	// Searching "class" matches the real in-scope class value and hides the augment entirely.
	results, _, errE = f.Get(ctx, getSearchService, restOfSearch, instanceOf, nil, "class*", searchLangs(enabledLanguages), resolver)
	require.NoError(t, errE, "% -+#.1v", errE)
	byID = refResultsByID(results)
	require.Contains(t, byID, class.String())
	assert.Equal(t, int64(1), byID[class.String()].Count)
	assert.NotContains(t, byID, unit.String())
	assert.NotContains(t, byID, vocabulary.String())

	// Outside a value search the whole augment (unit plus vocabulary) is force-shown at count 0 alongside the
	// in-scope class value.
	results, _, errE = f.Get(ctx, getSearchService, restOfSearch, instanceOf, nil, "", searchLangs(enabledLanguages), resolver)
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
// sub facets: an active sub-ref selection (plus its ancestors), which has zero documents in the current
// search scope, is searchable by display label or naming string, an ancestor surfaces only because its
// selected descendant pulls it into the augment, a non-matching term hides the augment, and outside a search
// the whole augment is shown at count 0.
func TestRefFilterGetSubRefSelectedAugmentValueSearchIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	parentProp := identifier.From("parentProp")
	parentTarget := identifier.From("parentToValue")
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

	subNamed := func(to identifier.Identifier, display string, naming []string, toPath string, isLeaf bool) internalSearch.RelClaim {
		rec := hierRelRecord(subProp, to, []string{toPath}, isLeaf)
		rec.ToDisplay = map[string]string{"en": display}
		if naming != nil {
			rec.ToNaming = map[string][]string{"en": naming}
		}
		return rec
	}

	// subDog references dog (expanded to dog, mammal, animal); subOther references the unrelated other root. The
	// search scope below matches only subOther, so dog and its ancestors have zero documents in scope.
	indexDocument(t, ctx, esClient, index, relDoc("subDog", internalSearch.RelClaims{
		refRecord(parentProp, parentTarget, relSub(
			subNamed(dog, "dog", []string{"canine"}, dogPath, true),
			subNamed(mammal, "mammal", nil, mammalPath, false),
			subNamed(animal, "animal", nil, animalPath, false),
		)),
	}))
	indexDocument(t, ctx, esClient, index, relDoc("subOther", internalSearch.RelClaims{
		refRecord(parentProp, parentTarget, relSub(
			subNamed(other, "other", nil, otherPath, true),
		)),
	}))
	refreshIndex(t, ctx, esClient, index)

	enabledLanguages := internalSearch.EnabledLanguages(nil)
	restOfSearch := esdsl.NewNestedQuery(
		esdsl.NewNestedQuery(
			esdsl.NewTermQuery("claims.rel.sub.rel.to", esdsl.NewFieldValue().String(other.String())),
		).Path("claims.rel.sub.rel"),
	).Path("claims.rel")
	f := search.RefFilter{To: []search.ToValue{{ID: dog}}, Direct: nil}
	resolver := newPathResolver(map[identifier.Identifier][]string{dog: {dogPath}})
	var sessionData search.SessionData
	parentCtx := sessionData.ParentContextFor(parentProp, subProp)

	// Searching dog's display label surfaces dog (count 0) with its full ancestor chain, even though dog is not
	// in scope.
	results, _, errE := f.GetSubRef(ctx, getSearchService, restOfSearch, subProp, parentCtx, nil, "dog*", searchLangs(enabledLanguages), resolver)
	require.NoError(t, errE, "% -+#.1v", errE)
	byID := refResultsByID(results)
	require.Contains(t, byID, dog.String())
	assert.Equal(t, int64(0), byID[dog.String()].Count)
	assert.Equal(t, [][]string{{animal.String(), mammal.String()}}, byID[dog.String()].Paths)
	require.Contains(t, byID, mammal.String())
	require.Contains(t, byID, animal.String())
	assert.NotContains(t, byID, other.String())

	// Searching dog by a naming string ("canine") surfaces it too.
	results, _, errE = f.GetSubRef(ctx, getSearchService, restOfSearch, subProp, parentCtx, nil, "canin*", searchLangs(enabledLanguages), resolver)
	require.NoError(t, errE, "% -+#.1v", errE)
	byID = refResultsByID(results)
	require.Contains(t, byID, dog.String())

	// Searching the ancestor's label ("anim") surfaces animal only because its descendant dog is selected; dog
	// and the intermediate mammal are not pulled in.
	results, _, errE = f.GetSubRef(ctx, getSearchService, restOfSearch, subProp, parentCtx, nil, "anim*", searchLangs(enabledLanguages), resolver)
	require.NoError(t, errE, "% -+#.1v", errE)
	byID = refResultsByID(results)
	require.Contains(t, byID, animal.String())
	assert.Equal(t, int64(0), byID[animal.String()].Count)
	assert.NotContains(t, byID, dog.String())
	assert.NotContains(t, byID, mammal.String())

	// Searching "other" matches the real in-scope value and hides the augment.
	results, _, errE = f.GetSubRef(ctx, getSearchService, restOfSearch, subProp, parentCtx, nil, "other*", searchLangs(enabledLanguages), resolver)
	require.NoError(t, errE, "% -+#.1v", errE)
	byID = refResultsByID(results)
	require.Contains(t, byID, other.String())
	assert.Equal(t, int64(1), byID[other.String()].Count)
	assert.NotContains(t, byID, dog.String())
	assert.NotContains(t, byID, animal.String())

	// Outside a value search the whole augment (dog plus its ancestors) is force-shown at count 0.
	results, _, errE = f.GetSubRef(ctx, getSearchService, restOfSearch, subProp, parentCtx, nil, "", searchLangs(enabledLanguages), resolver)
	require.NoError(t, errE, "% -+#.1v", errE)
	byID = refResultsByID(results)
	require.Contains(t, byID, dog.String())
	assert.Equal(t, int64(0), byID[dog.String()].Count)
	require.Contains(t, byID, mammal.String())
	require.Contains(t, byID, animal.String())
	require.Contains(t, byID, other.String())
}

// TestRefFilterGetSubRefChildCountAtLeastIntegration verifies the childCount completeness marking of
// sub facets: a value whose children terms were truncated (a positive sum_other_doc_count) reports
// ChildCountAtLeast and serializes childCount as "<n>+", while a value with a complete child key set
// reports a plain exact number.
func TestRefFilterGetSubRefChildCountAtLeastIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	locationProp := identifier.From("locationProp")
	artistProp := identifier.From("artistProp")
	venue := identifier.From("venue")
	bigParent := identifier.From("bigParent")
	smallParent := identifier.From("smallParent")

	// bigParent has one more child value than the children terms can return; smallParent has two.
	childRecord := func(child, parent identifier.Identifier) internalSearch.RelClaim {
		rec := refRecord(artistProp, child, nil)
		rec.ToParent = []string{parent.String()}
		return rec
	}
	records := internalSearch.RelClaims{refRecord(artistProp, bigParent, nil), refRecord(artistProp, smallParent, nil)}
	for i := range search.MaxResultsCount + 1 {
		records = append(records, childRecord(identifier.From("bigChild", strconv.Itoa(i)), bigParent))
	}
	records = append(records,
		childRecord(identifier.From("smallChild1"), smallParent),
		childRecord(identifier.From("smallChild2"), smallParent),
	)
	indexDocument(t, ctx, esClient, index, relDoc("childCountDoc1", internalSearch.RelClaims{
		refRecord(locationProp, venue, &internalSearch.ClaimTypes{Rel: records}), //nolint:exhaustruct
	}))
	// A second document referencing only the two parents lifts them above the children in the
	// document-count ordering, so both are within the value list cap.
	indexDocument(t, ctx, esClient, index, relDoc("childCountDoc2", internalSearch.RelClaims{
		refRecord(locationProp, venue, &internalSearch.ClaimTypes{Rel: internalSearch.RelClaims{ //nolint:exhaustruct
			refRecord(artistProp, bigParent, nil), refRecord(artistProp, smallParent, nil),
		}}),
	}))
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	f := search.RefFilter{To: nil, Direct: nil}
	parentCtx := session.ParentContextFor(locationProp, artistProp)
	results, _, errE := f.GetSubRef(ctx, getSearchService, session.ToQuery(nil), artistProp, parentCtx, nil, "", nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	byID := map[string]search.RefFilterResult{}
	for _, r := range results {
		byID[r.ID] = r
	}

	// The truncated parent's key set holds the cap's worth of children and is marked as a lower
	// bound; on the wire childCount becomes the string "<n>+".
	big, ok := byID[bigParent.String()]
	require.True(t, ok)
	assert.Equal(t, int64(search.MaxResultsCount), big.ChildCount)
	assert.True(t, big.ChildCountAtLeast)
	data, errE := x.MarshalWithoutEscapeHTML(big)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Contains(t, string(data), `"childCount":"1000+"`)

	// The complete parent stays exact and numeric.
	small, ok := byID[smallParent.String()]
	require.True(t, ok)
	assert.Equal(t, int64(2), small.ChildCount)
	assert.False(t, small.ChildCountAtLeast)
	data, errE = x.MarshalWithoutEscapeHTML(small)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Contains(t, string(data), `"childCount":2`)
}
