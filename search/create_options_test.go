package search_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/document"
	internalCore "gitlab.com/peerdb/peerdb/internal/core"
	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
	"gitlab.com/peerdb/peerdb/search"
)

// classDoc builds a class document carrying just the claims classCreatable inspects: an ABSTRACT_CLASS
// has-claim when abstract, and a FIELDS has-claim with one FIELD when it defines fields.
func classDoc(t *testing.T, id identifier.Identifier, abstract, hasField bool) *document.D {
	t.Helper()

	doc := &document.D{}
	doc.ID = id
	if abstract {
		errE := doc.Add(&document.HasClaim{
			CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: document.HighConfidence},
			Prop:      document.Reference{ID: internalCore.AbstractClassPropID},
		})
		require.NoError(t, errE, "% -+#.1v", errE)
	}
	if hasField {
		fields := &document.HasClaim{
			CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: document.HighConfidence},
			Prop:      document.Reference{ID: internalCore.FieldsPropID},
		}
		errE := fields.Add(&document.HasClaim{
			CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: document.HighConfidence},
			Prop:      document.Reference{ID: internalCore.FieldPropID},
		})
		require.NoError(t, errE, "% -+#.1v", errE)
		errE = doc.Add(fields)
		require.NoError(t, errE, "% -+#.1v", errE)
	}
	return doc
}

// TestCreateOptionsIntegration drives search.CreateOptions over a small class hierarchy:
//
//	A (abstract root, no fields)
//	|- B (creatable)        -- E
//	|- C (creatable)        -- E   (E has two parents: a diamond)
//	|- D (no fields)               (leaf with no creatable descendant)
//	|- E (creatable, leaf)
//
// It asserts that the no-creatable branch (D) is pruned, abstract A survives only as a structural
// ancestor, every other class is creatable (including instance-less ones), the diamond child E carries a
// path under each parent, and the result is ordered by instance count descending (so A and B, with five
// instances each, precede C and E, with three).
func TestCreateOptionsIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	classA := identifier.From("createClassA")
	classB := identifier.From("createClassB")
	classC := identifier.From("createClassC")
	classD := identifier.From("createClassD")
	classE := identifier.From("createClassE")

	// indexInstanceOf indexes a document whose only claims are INSTANCE_OF ref records to each of tos.
	// Listing every ancestor explicitly mirrors the converter's index-time ancestor expansion, so the
	// instance-count aggregation rolls a document up under each of its class's ancestors.
	indexInstanceOf := func(id identifier.Identifier, tos ...identifier.Identifier) {
		rels := make(internalSearch.RelClaims, 0, len(tos))
		for _, to := range tos {
			rels = append(rels, refRecord(internalCore.InstanceOfPropID, to, nil))
		}
		indexDocument(t, ctx, esClient, index, idClaimsDoc(id, internalSearch.ClaimTypes{ //nolint:exhaustruct
			Rel: rels,
		}))
	}

	// Each class is an instance of the core CLASS so it is enumerated.
	for _, id := range []identifier.Identifier{classA, classB, classC, classD, classE} {
		indexInstanceOf(id, internalCore.ClassClassID)
	}
	// Instance documents: two of B (rolled up to A), three of E (rolled up to B, C and A).
	indexInstanceOf(identifier.From("createB1"), classB, classA)
	indexInstanceOf(identifier.From("createB2"), classB, classA)
	indexInstanceOf(identifier.From("createE1"), classE, classB, classC, classA)
	indexInstanceOf(identifier.From("createE2"), classE, classB, classC, classA)
	indexInstanceOf(identifier.From("createE3"), classE, classB, classC, classA)
	refreshIndex(t, ctx, esClient, index)

	sub := internalCore.SubclassOfPropID.String() + ":"
	fullPaths := map[identifier.Identifier][]string{
		classA: {sub + classA.String()},
		classB: {sub + classA.String() + "/" + classB.String()},
		classC: {sub + classA.String() + "/" + classC.String()},
		classD: {sub + classA.String() + "/" + classD.String()},
		classE: {
			sub + classA.String() + "/" + classB.String() + "/" + classE.String(),
			sub + classA.String() + "/" + classC.String() + "/" + classE.String(),
		},
	}
	docs := map[identifier.Identifier]*document.D{
		classA: classDoc(t, classA, true, false),
		classB: classDoc(t, classB, false, true),
		classC: classDoc(t, classC, false, true),
		classD: classDoc(t, classD, false, false),
		classE: classDoc(t, classE, false, true),
	}
	loadDocument := func(_ context.Context, id identifier.Identifier) (*document.D, errors.E) {
		return docs[id], nil
	}
	documentHierarchyPaths := func(_ context.Context, id identifier.Identifier) ([]string, errors.E) {
		return fullPaths[id], nil
	}

	options, errE := search.CreateOptions(ctx, getSearchService, nil, loadDocument, nil, documentHierarchyPaths, "")
	require.NoError(t, errE, "% -+#.1v", errE)

	ids := make([]string, 0, len(options))
	creatable := map[string]bool{}
	paths := map[string][][]string{}
	for _, o := range options {
		ids = append(ids, o.ID)
		creatable[o.ID] = o.Creatable
		paths[o.ID] = o.Paths
	}

	// classD has no fields and no creatable descendants, so its whole branch is pruned; the rest are
	// ordered by instance count descending then depth ascending (A and B have five, C and E have three).
	assert.Equal(t, []string{classA.String(), classB.String(), classC.String(), classE.String()}, ids)
	// Abstract classA is kept only as a structural ancestor; the others can be created.
	assert.False(t, creatable[classA.String()])
	assert.True(t, creatable[classB.String()])
	assert.True(t, creatable[classC.String()])
	assert.True(t, creatable[classE.String()])
	// classA is a root (no ancestor paths); classE renders under both of its parents.
	assert.Empty(t, paths[classA.String()])
	assert.ElementsMatch(t, [][]string{{classA.String(), classB.String()}, {classA.String(), classC.String()}}, paths[classE.String()])

	// With a limit on classB, only classB and its descendant classE are offered; the limit's ancestor classA
	// is kept as a non-creatable label, and the unrelated classC and classD are dropped.
	limited, errE := search.CreateOptions(ctx, getSearchService, nil, loadDocument, nil, documentHierarchyPaths, classB.String())
	require.NoError(t, errE, "% -+#.1v", errE)
	limitedIDs := make([]string, 0, len(limited))
	limitedCreatable := map[string]bool{}
	for _, o := range limited {
		limitedIDs = append(limitedIDs, o.ID)
		limitedCreatable[o.ID] = o.Creatable
	}
	assert.Equal(t, []string{classA.String(), classB.String(), classE.String()}, limitedIDs)
	assert.False(t, limitedCreatable[classA.String()])
	assert.True(t, limitedCreatable[classB.String()])
	assert.True(t, limitedCreatable[classE.String()])

	// An unknown limit id yields nothing.
	none, errE := search.CreateOptions(ctx, getSearchService, nil, loadDocument, nil, documentHierarchyPaths, identifier.From("createClassMissing").String())
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Empty(t, none)

	// A caller who may create only classE is offered it alone, with the classes which place it kept as
	// labels; classD's branch stays pruned as it has nothing creatable under it either way.
	canCreate := func(_ context.Context, id identifier.Identifier) bool {
		return id == classE
	}
	scoped, errE := search.CreateOptions(ctx, getSearchService, nil, loadDocument, canCreate, documentHierarchyPaths, "")
	require.NoError(t, errE, "% -+#.1v", errE)
	scopedCreatable := map[string]bool{}
	for _, o := range scoped {
		scopedCreatable[o.ID] = o.Creatable
	}
	assert.Equal(t, map[string]bool{classA.String(): false, classB.String(): false, classC.String(): false, classE.String(): true}, scopedCreatable)

	// A caller who may create nothing is offered nothing, not even the classes which would place it.
	denyAll := func(_ context.Context, _ identifier.Identifier) bool {
		return false
	}
	nothing, errE := search.CreateOptions(ctx, getSearchService, nil, loadDocument, denyAll, documentHierarchyPaths, "")
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Empty(t, nothing)
}

// TestCreateOptionsTieOrderIntegration verifies that classes tying on everything the offering is ordered by
// are offered in id order. Two classes nothing is an instance of tie on count and on depth, and what is left
// to order them is the order the enumerating search returns them in, which is the index's internal document
// order unless the search asks for another. Two indexes holding the same documents do not have to agree on
// that order, so the same site would offer the same two classes in a different order from one index to the
// next: the class picker of the create view is where this shows.
func TestCreateOptionsTieOrderIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	lower := identifier.From("createTieClassOne")
	higher := identifier.From("createTieClassTwo")
	if lower.String() > higher.String() {
		lower, higher = higher, lower
	}

	// The class with the greater id is indexed first, so the document order of the index is the reverse of
	// the order the classes are expected in and cannot be what puts them in it.
	for _, id := range []identifier.Identifier{higher, lower} {
		indexDocument(t, ctx, esClient, index, idClaimsDoc(id, internalSearch.ClaimTypes{ //nolint:exhaustruct
			Rel: internalSearch.RelClaims{refRecord(internalCore.InstanceOfPropID, internalCore.ClassClassID, nil)},
		}))
	}
	refreshIndex(t, ctx, esClient, index)

	docs := map[identifier.Identifier]*document.D{
		lower:  classDoc(t, lower, false, true),
		higher: classDoc(t, higher, false, true),
	}
	loadDocument := func(_ context.Context, id identifier.Identifier) (*document.D, errors.E) {
		return docs[id], nil
	}
	// Neither class is placed under anything, so both are roots and tie on depth as well as on count.
	documentHierarchyPaths := func(_ context.Context, _ identifier.Identifier) ([]string, errors.E) {
		return nil, nil
	}

	options, errE := search.CreateOptions(ctx, getSearchService, nil, loadDocument, nil, documentHierarchyPaths, "")
	require.NoError(t, errE, "% -+#.1v", errE)

	ids := make([]string, 0, len(options))
	for _, o := range options {
		ids = append(ids, o.ID)
	}
	assert.Equal(t, []string{lower.String(), higher.String()}, ids)
}

func TestClassCreatable(t *testing.T) {
	t.Parallel()

	// hasClaim builds a property-only has-claim for prop with the given sub has-claims already attached.
	// Sub-claims are attached before the claim is added to its container because a container stores claims
	// by value, so mutating a claim after adding it would not be reflected.
	hasClaim := func(prop identifier.Identifier, subs ...*document.HasClaim) *document.HasClaim {
		claim := &document.HasClaim{
			CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: document.HighConfidence},
			Prop:      document.Reference{ID: prop},
		}
		for _, sub := range subs {
			errE := claim.Add(sub)
			require.NoError(t, errE, "% -+#.1v", errE)
		}
		return claim
	}
	classDoc := func(claims ...*document.HasClaim) *document.D {
		doc := &document.D{}
		doc.ID = identifier.New()
		for _, claim := range claims {
			errE := doc.Add(claim)
			require.NoError(t, errE, "% -+#.1v", errE)
		}
		return doc
	}

	assert.False(t, search.TestingClassCreatable(nil))
	assert.False(t, search.TestingClassCreatable(classDoc()))
	// A FIELDS has-claim with a FIELD or a SECTION makes the class creatable.
	assert.True(t, search.TestingClassCreatable(classDoc(hasClaim(internalCore.FieldsPropID, hasClaim(internalCore.FieldPropID)))))
	assert.True(t, search.TestingClassCreatable(classDoc(hasClaim(internalCore.FieldsPropID, hasClaim(internalCore.SectionPropID)))))
	// A FIELDS has-claim with neither a field nor a section is not enough.
	assert.False(t, search.TestingClassCreatable(classDoc(hasClaim(internalCore.FieldsPropID))))
	// An abstract class is never creatable, even if it defines fields.
	assert.False(t, search.TestingClassCreatable(classDoc(
		hasClaim(internalCore.FieldsPropID, hasClaim(internalCore.FieldPropID)),
		hasClaim(internalCore.AbstractClassPropID),
	)))
}

func TestAncestorChains(t *testing.T) {
	t.Parallel()

	prefix := internalCore.SubclassOfPropID.String() + ":"

	assert.Nil(t, search.TestingAncestorChains(nil))
	// A root value (single segment, no ancestors) yields no chain.
	assert.Nil(t, search.TestingAncestorChains([]string{prefix + "A"}))
	// A path without the property prefix is dropped.
	assert.Nil(t, search.TestingAncestorChains([]string{"A/B"}))
	assert.Equal(t, [][]string{{"A"}}, search.TestingAncestorChains([]string{prefix + "A/B"}))
	assert.Equal(t, [][]string{{"A", "B"}, {"X"}}, search.TestingAncestorChains([]string{prefix + "A/B/C", prefix + "X/C"}))
}
