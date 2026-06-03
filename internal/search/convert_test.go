package search //nolint:testpackage

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/document"
	internalCore "gitlab.com/peerdb/peerdb/internal/core"
	"gitlab.com/peerdb/peerdb/store"
)

// Helper IDs for tests.
//
//nolint:gochecknoglobals
var (
	testPropID      = identifier.New()
	testPropID2     = identifier.New()
	testParentProp  = identifier.New()
	testParentClass = identifier.New()
	testDocID       = identifier.New()
	testLangDocID   = identifier.New()
	testUnitDocID   = identifier.New()
	testTargetDocID = identifier.New()
)

// makeCoreClaim creates a CoreClaim with the given confidence and optional sub-claims.
func makeCoreClaim(confidence document.Confidence, sub *document.ClaimTypes) document.CoreClaim {
	return document.CoreClaim{
		ID:         identifier.New(),
		Confidence: confidence,
		Sub:        sub,
	}
}

// newIR creates an InverseRelation with the given fields.
func newIR(claim, source, sourceProp, targetProp, target identifier.Identifier, confidence document.Confidence) store.InverseRelation {
	return store.InverseRelation{
		InverseRelationKey: store.InverseRelationKey{Claim: claim, Source: source, TargetProp: targetProp},
		SourceProp:         sourceProp,
		Target:             target,
		Confidence:         confidence,
	}
}

// makePropertyDoc creates a property document (instance of PROPERTY class) with optional SUBPROPERTY_OF relation.
func makePropertyDoc(id identifier.Identifier, subpropertyOf *identifier.Identifier) *document.D {
	return makePropertyDocFull(id, subpropertyOf, nil)
}

// makePropertyDocFull creates a property document with optional SUBPROPERTY_OF and INVERSE_PROPERTY_OF relations.
func makePropertyDocFull(id identifier.Identifier, subpropertyOf, inversePropertyOf *identifier.Identifier) *document.D {
	claims := &document.ClaimTypes{}
	// INSTANCE_OF -> PROPERTY.
	claims.Reference = append(claims.Reference, document.ReferenceClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: internalCore.InstanceOfPropID},
		To:        document.Reference{ID: internalCore.PropertyClassID},
	})
	if subpropertyOf != nil {
		claims.Reference = append(claims.Reference, document.ReferenceClaim{
			CoreClaim: makeCoreClaim(document.HighConfidence, nil),
			Prop:      document.Reference{ID: internalCore.SubpropertyOfPropID},
			To:        document.Reference{ID: *subpropertyOf},
		})
	}
	if inversePropertyOf != nil {
		claims.Reference = append(claims.Reference, document.ReferenceClaim{
			CoreClaim: makeCoreClaim(document.HighConfidence, nil),
			Prop:      document.Reference{ID: internalCore.InversePropertyOfPropID},
			To:        document.Reference{ID: *inversePropertyOf},
		})
	}
	return &document.D{
		CoreDocument: document.CoreDocument{ //nolint:exhaustruct
			ID: id,
		},
		Claims: claims,
	}
}

// makeHierarchyDoc creates a document with a naming string and an optional hierarchy relation.
func makeHierarchyDoc(id identifier.Identifier, name string, hierProp identifier.Identifier, parentID *identifier.Identifier) *document.D {
	claims := &document.ClaimTypes{}
	claims.String = append(claims.String, document.StringClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: internalCore.NamingPropID},
		String:    name,
	})
	if parentID != nil {
		claims.Reference = append(claims.Reference, document.ReferenceClaim{
			CoreClaim: makeCoreClaim(document.HighConfidence, nil),
			Prop:      document.Reference{ID: hierProp},
			To:        document.Reference{ID: *parentID},
		})
	}
	return &document.D{
		CoreDocument: document.CoreDocument{ //nolint:exhaustruct
			ID: id,
		},
		Claims: claims,
	}
}

// makeLanguageDoc creates a language document (instance of LANGUAGE class) with a CODE identifier.
func makeLanguageDoc(id identifier.Identifier, code string) *document.D {
	claims := &document.ClaimTypes{}
	claims.Reference = append(claims.Reference, document.ReferenceClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: internalCore.InstanceOfPropID},
		To:        document.Reference{ID: internalCore.LanguageClassID},
	})
	claims.Identifier = append(claims.Identifier, document.IdentifierClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: internalCore.CodePropID},
		Value:     code,
	})
	return &document.D{
		CoreDocument: document.CoreDocument{ //nolint:exhaustruct
			ID: id,
		},
		Claims: claims,
	}
}

// idTmpl returns a template expression that resolves an identifier.Identifier from its string representation.
func idTmpl(id identifier.Identifier) string {
	return `(identifierString "` + id.String() + `")`
}

// makeNamingDoc creates a document with a naming string claim.
func makeNamingDoc(id identifier.Identifier, name string) *document.D {
	claims := &document.ClaimTypes{}
	claims.String = append(claims.String, document.StringClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: internalCore.NamingPropID},
		String:    name,
	})
	return &document.D{
		CoreDocument: document.CoreDocument{ //nolint:exhaustruct
			ID: id,
		},
		Claims: claims,
	}
}

// newTestConverter creates a Converter for testing with the given properties, languages, and extra documents.
func newTestConverter(
	t *testing.T,
	properties, languages []*document.D,
	extraDocs map[identifier.Identifier]*document.D,
) *Converter {
	t.Helper()
	return newTestConverterFull(t, properties, languages, nil, extraDocs, nil)
}

// newTestConverterWithClasses creates a Converter with class documents.
func newTestConverterWithClasses(
	t *testing.T,
	properties, classes []*document.D,
	extraDocs map[identifier.Identifier]*document.D,
) *Converter {
	t.Helper()
	return newTestConverterFull(t, properties, nil, classes, extraDocs, nil)
}

// newTestConverterWithPriority creates a Converter with custom language priority.
func newTestConverterWithPriority(
	t *testing.T,
	properties, languages []*document.D,
	extraDocs map[identifier.Identifier]*document.D,
	priority map[string][]string,
) *Converter {
	t.Helper()
	return newTestConverterFull(t, properties, languages, nil, extraDocs, priority)
}

// newTestConverterFull creates a Converter with all options.
func newTestConverterFull(
	t *testing.T,
	properties, languages, classes []*document.D,
	extraDocs map[identifier.Identifier]*document.D,
	priority map[string][]string,
) *Converter {
	t.Helper()
	coreStubDocs := map[identifier.Identifier]*document.D{
		internalCore.InLanguagePropID:  makeNamingDoc(internalCore.InLanguagePropID, "in language"),
		internalCore.InUnitPropID:      makeNamingDoc(internalCore.InUnitPropID, "in unit"),
		internalCore.LanguageClassID:   makeNamingDoc(internalCore.LanguageClassID, "language"),
		internalCore.ClassClassID:      makeNamingDoc(internalCore.ClassClassID, "class"),
		internalCore.VocabularyClassID: makeNamingDoc(internalCore.VocabularyClassID, "vocabulary"),
		internalCore.PropertyClassID:   makeNamingDoc(internalCore.PropertyClassID, "property"),
	}
	getDocument := func(_ context.Context, id identifier.Identifier) (*document.D, errors.E) {
		if doc, ok := extraDocs[id]; ok {
			return doc, nil
		}
		// Properties, languages, and classes registered with the converter are
		// also resolvable.
		for _, list := range [][]*document.D{properties, languages, classes} {
			for _, doc := range list {
				if doc.ID == id {
					return doc, nil
				}
			}
		}
		// Provide stubs for core metadata properties referenced by sub-claims.
		if stub, ok := coreStubDocs[id]; ok {
			return stub, nil
		}
		return nil, errors.New("document not found")
	}
	c, errE := NewConverter(properties, languages, classes, priority, getDocument)
	require.NoError(t, errE, "% -+#.1v", errE)
	return c
}

func TestIsInstanceOf(t *testing.T) {
	t.Parallel()

	doc := makePropertyDoc(testPropID, nil)
	assert.True(t, isInstanceOf(doc, internalCore.PropertyClassID))
	assert.False(t, isInstanceOf(doc, internalCore.LanguageClassID))

	// Document with no claims.
	emptyDoc := &document.D{
		CoreDocument: document.CoreDocument{ID: identifier.New()}, //nolint:exhaustruct
	}
	assert.False(t, isInstanceOf(emptyDoc, internalCore.PropertyClassID))
}

func TestBuildPropertyHierarchy(t *testing.T) {
	t.Parallel()

	// Build a chain: testPropID2 is a subproperty of testPropID.
	child := makePropertyDoc(testPropID2, &testPropID)
	parent := makePropertyDoc(testPropID, nil)

	properties := []*document.D{parent, child}
	c := &Converter{ //nolint:exhaustruct
		documentInfoCache: map[identifier.Identifier]documentInfo{},
	}
	c.buildPropertyHierarchy(properties)

	// Parent should have child as descendant.
	assert.Contains(t, c.propertyDescendants[testPropID], testPropID2)
	// Child should have parent as ancestor.
	assert.Contains(t, c.propertyAncestors[testPropID2], testPropID)
	// Parent has no ancestors.
	assert.Empty(t, c.propertyAncestors[testPropID])
	// Child has no descendants.
	assert.Empty(t, c.propertyDescendants[testPropID2])
}

func TestBuildPropertyHierarchyTransitive(t *testing.T) {
	t.Parallel()

	grandparent := identifier.New()
	parent := identifier.New()
	child := identifier.New()

	gpDoc := makePropertyDoc(grandparent, nil)
	pDoc := makePropertyDoc(parent, &grandparent)
	cDoc := makePropertyDoc(child, &parent)

	properties := []*document.D{gpDoc, pDoc, cDoc}
	c := &Converter{ //nolint:exhaustruct
		documentInfoCache: map[identifier.Identifier]documentInfo{},
	}
	c.buildPropertyHierarchy(properties)

	// Child should have both parent and grandparent as ancestors.
	assert.Contains(t, c.propertyAncestors[child], parent)
	assert.Contains(t, c.propertyAncestors[child], grandparent)
	// Grandparent should have both parent and child as descendants.
	assert.Contains(t, c.propertyDescendants[grandparent], parent)
	assert.Contains(t, c.propertyDescendants[grandparent], child)
}

func TestBuildPropertyHierarchySkipsNonProperty(t *testing.T) {
	t.Parallel()

	// Document that is NOT an instance of PROPERTY.
	notProp := &document.D{
		CoreDocument: document.CoreDocument{ID: identifier.New()}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			Reference: []document.ReferenceClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: internalCore.SubpropertyOfPropID},
					To:        document.Reference{ID: testPropID},
				},
			},
		},
	}

	c := &Converter{ //nolint:exhaustruct
		documentInfoCache: map[identifier.Identifier]documentInfo{},
	}
	c.buildPropertyHierarchy([]*document.D{notProp})

	assert.Empty(t, c.propertyDescendants)
	assert.Empty(t, c.propertyAncestors)
}

func TestGetDocumentInfoBasic(t *testing.T) {
	t.Parallel()

	// Document with no hierarchy claims.
	doc := makeNamingDoc(testDocID, "Test Doc")
	extraDocs := map[identifier.Identifier]*document.D{
		testDocID: doc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	info, errE := c.getDocumentInfo(ctx, testDocID)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, "Test Doc", info.Display.Display["und"])
	assert.Empty(t, info.Ancestors)
}

func TestGetDocumentInfoWithClassAncestors(t *testing.T) {
	t.Parallel()

	// Set up SUBENTITY_OF hierarchy so SUBCLASS_OF is discovered.
	subentityDoc := makePropertyDoc(internalCore.SubentityOfPropID, nil)
	subclassDoc := makePropertyDoc(internalCore.SubclassOfPropID, &internalCore.SubentityOfPropID)
	subpropDoc := makePropertyDoc(internalCore.SubpropertyOfPropID, &internalCore.SubentityOfPropID)
	instanceDoc := makePropertyDoc(internalCore.InstanceOfPropID, &internalCore.SubentityOfPropID)
	properties := []*document.D{subentityDoc, subclassDoc, subpropDoc, instanceDoc}

	grandparent := identifier.New()
	parent := identifier.New()
	child := identifier.New()

	gpDoc := makeHierarchyDoc(grandparent, "Grandparent", internalCore.SubclassOfPropID, nil)
	pDoc := makeHierarchyDoc(parent, "Parent", internalCore.SubclassOfPropID, &grandparent)
	cDoc := makeHierarchyDoc(child, "Child", internalCore.SubclassOfPropID, &parent)

	extraDocs := map[identifier.Identifier]*document.D{
		grandparent: gpDoc,
		parent:      pDoc,
		child:       cDoc,
	}
	c := newTestConverter(t, properties, nil, extraDocs)

	ctx := t.Context()
	info, errE := c.getDocumentInfo(ctx, child)
	require.NoError(t, errE, "% -+#.1v", errE)
	// Child should have parent and grandparent as ancestors via SUBCLASS_OF.
	assert.Contains(t, info.Ancestors[internalCore.SubclassOfPropID], parent)
	assert.Contains(t, info.Ancestors[internalCore.SubclassOfPropID], grandparent)

	// Child should have a hierarchy path: grandparent/parent/child.
	require.Len(t, info.IDPaths[internalCore.SubclassOfPropID], 1)
	expectedIDPath := grandparent.String() + "/" + parent.String() + "/" + child.String()
	assert.Equal(t, expectedIDPath, info.IDPaths[internalCore.SubclassOfPropID][0])

	// Display path should use null-byte separated display names.
	require.Contains(t, info.DisplayPaths[internalCore.SubclassOfPropID], "und")
	require.Len(t, info.DisplayPaths[internalCore.SubclassOfPropID]["und"], 1)
	assert.Equal(t, "Grandparent\x00Parent\x00Child", info.DisplayPaths[internalCore.SubclassOfPropID]["und"][0])

	// Parent should have a single-level path: grandparent/parent.
	parentInfo, errE := c.getDocumentInfo(ctx, parent)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, parentInfo.IDPaths[internalCore.SubclassOfPropID], 1)
	assert.Equal(t, grandparent.String()+"/"+parent.String(), parentInfo.IDPaths[internalCore.SubclassOfPropID][0])

	// Grandparent has no hierarchy paths (it's a root).
	gpInfo, errE := c.getDocumentInfo(ctx, grandparent)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Empty(t, gpInfo.IDPaths[internalCore.SubclassOfPropID])
}

func TestGetDocumentInfoMultiplePaths(t *testing.T) {
	t.Parallel()

	subentityDoc := makePropertyDoc(internalCore.SubentityOfPropID, nil)
	subclassDoc := makePropertyDoc(internalCore.SubclassOfPropID, &internalCore.SubentityOfPropID)
	subpropDoc := makePropertyDoc(internalCore.SubpropertyOfPropID, &internalCore.SubentityOfPropID)
	instanceDoc := makePropertyDoc(internalCore.InstanceOfPropID, &internalCore.SubentityOfPropID)
	properties := []*document.D{subentityDoc, subclassDoc, subpropDoc, instanceDoc}

	// Diamond hierarchy: child has two parents, both share a grandparent.
	grandparent := identifier.New()
	parentA := identifier.New()
	parentB := identifier.New()
	child := identifier.New()

	gpDoc := makeNamingDoc(grandparent, "Root")
	paDoc := makeHierarchyDoc(parentA, "ParentA", internalCore.SubclassOfPropID, &grandparent)
	pbDoc := makeHierarchyDoc(parentB, "ParentB", internalCore.SubclassOfPropID, &grandparent)

	// Child with two parents.
	childClaims := &document.ClaimTypes{}
	childClaims.String = append(childClaims.String, document.StringClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: internalCore.NamingPropID},
		String:    "Leaf",
	})
	childClaims.Reference = append(childClaims.Reference,
		document.ReferenceClaim{
			CoreClaim: makeCoreClaim(document.HighConfidence, nil),
			Prop:      document.Reference{ID: internalCore.SubclassOfPropID},
			To:        document.Reference{ID: parentA},
		},
		document.ReferenceClaim{
			CoreClaim: makeCoreClaim(document.HighConfidence, nil),
			Prop:      document.Reference{ID: internalCore.SubclassOfPropID},
			To:        document.Reference{ID: parentB},
		},
	)
	childDoc := &document.D{
		CoreDocument: document.CoreDocument{ID: child}, //nolint:exhaustruct
		Claims:       childClaims,
	}

	extraDocs := map[identifier.Identifier]*document.D{
		grandparent: gpDoc,
		parentA:     paDoc,
		parentB:     pbDoc,
		child:       childDoc,
	}
	c := newTestConverter(t, properties, nil, extraDocs)

	ctx := t.Context()
	info, errE := c.getDocumentInfo(ctx, child)
	require.NoError(t, errE, "% -+#.1v", errE)
	// Child should have two ID paths (one through each parent).
	require.Len(t, info.IDPaths[internalCore.SubclassOfPropID], 2)
	pathA := grandparent.String() + "/" + parentA.String() + "/" + child.String()
	pathB := grandparent.String() + "/" + parentB.String() + "/" + child.String()
	assert.ElementsMatch(t, info.IDPaths[internalCore.SubclassOfPropID], []string{pathA, pathB})
	// Two display paths.
	require.Len(t, info.DisplayPaths[internalCore.SubclassOfPropID]["und"], 2)
	assert.ElementsMatch(t, info.DisplayPaths[internalCore.SubclassOfPropID]["und"], []string{
		"Root\x00ParentA\x00Leaf",
		"Root\x00ParentB\x00Leaf",
	})
}

func TestGetDocumentInfoCaching(t *testing.T) {
	t.Parallel()

	doc := makeNamingDoc(testDocID, "Test Doc")
	callCount := 0
	getDocument := func(_ context.Context, id identifier.Identifier) (*document.D, errors.E) {
		if id == testDocID {
			callCount++
			return doc, nil
		}
		return nil, errors.New("document not found")
	}
	c, errE := NewConverter(nil, nil, nil, nil, getDocument)
	require.NoError(t, errE, "% -+#.1v", errE)

	ctx := t.Context()
	info1, errE := c.getDocumentInfo(ctx, testDocID)
	require.NoError(t, errE, "% -+#.1v", errE)
	info2, errE := c.getDocumentInfo(ctx, testDocID)
	require.NoError(t, errE, "% -+#.1v", errE)

	assert.Equal(t, info1.Display.Display["und"], info2.Display.Display["und"])
	// getDocument should only have been called once.
	assert.Equal(t, 1, callCount)
}

func TestBuildPropertyHierarchySelfCycle(t *testing.T) {
	t.Parallel()

	// Property that is a subproperty of itself.
	selfRef := makePropertyDoc(testPropID, &testPropID)

	c := &Converter{ //nolint:exhaustruct
		documentInfoCache: map[identifier.Identifier]documentInfo{},
	}
	c.buildPropertyHierarchy([]*document.D{selfRef})

	// Self-reference is excluded to avoid duplicates in consuming code.
	assert.Empty(t, c.propertyDescendants[testPropID])
	assert.Empty(t, c.propertyAncestors[testPropID])
}

func TestBuildPropertyHierarchyMutualCycle(t *testing.T) {
	t.Parallel()

	// A is subproperty of B, B is subproperty of A.
	a := identifier.New()
	b := identifier.New()
	aDoc := makePropertyDoc(a, &b)
	bDoc := makePropertyDoc(b, &a)

	c := &Converter{ //nolint:exhaustruct
		documentInfoCache: map[identifier.Identifier]documentInfo{},
	}
	c.buildPropertyHierarchy([]*document.D{aDoc, bDoc})

	// Both should appear as descendants and ancestors of each other.
	assert.Contains(t, c.propertyDescendants[a], b)
	assert.Contains(t, c.propertyDescendants[b], a)
	assert.Contains(t, c.propertyAncestors[a], b)
	assert.Contains(t, c.propertyAncestors[b], a)
}

func TestBuildPropertyHierarchyLongerCycle(t *testing.T) {
	t.Parallel()

	// A -> B -> C -> A (cycle of length 3).
	a := identifier.New()
	b := identifier.New()
	cc := identifier.New()
	aDoc := makePropertyDoc(a, &b)
	bDoc := makePropertyDoc(b, &cc)
	cDoc := makePropertyDoc(cc, &a)

	c := &Converter{ //nolint:exhaustruct
		documentInfoCache: map[identifier.Identifier]documentInfo{},
	}
	c.buildPropertyHierarchy([]*document.D{aDoc, bDoc, cDoc})

	// Every node should have the other two (but not itself) as ancestors.
	assert.ElementsMatch(t, c.propertyAncestors[a], []identifier.Identifier{b, cc})
	assert.ElementsMatch(t, c.propertyAncestors[b], []identifier.Identifier{a, cc})
	assert.ElementsMatch(t, c.propertyAncestors[cc], []identifier.Identifier{a, b})

	// Every node should have the other two (but not itself) as descendants.
	assert.ElementsMatch(t, c.propertyDescendants[a], []identifier.Identifier{b, cc})
	assert.ElementsMatch(t, c.propertyDescendants[b], []identifier.Identifier{a, cc})
	assert.ElementsMatch(t, c.propertyDescendants[cc], []identifier.Identifier{a, b})
}

func TestGetDocumentInfoSelfCycle(t *testing.T) {
	t.Parallel()

	// Set up SUBENTITY_OF hierarchy so SUBCLASS_OF is discovered.
	subentityDoc := makePropertyDoc(internalCore.SubentityOfPropID, nil)
	subclassDoc := makePropertyDoc(internalCore.SubclassOfPropID, &internalCore.SubentityOfPropID)
	subpropDoc := makePropertyDoc(internalCore.SubpropertyOfPropID, &internalCore.SubentityOfPropID)
	instanceDoc := makePropertyDoc(internalCore.InstanceOfPropID, &internalCore.SubentityOfPropID)
	properties := []*document.D{subentityDoc, subclassDoc, subpropDoc, instanceDoc}

	// Document with SUBCLASS_OF pointing to itself.
	selfID := identifier.New()
	selfDoc := makeHierarchyDoc(selfID, "Self", internalCore.SubclassOfPropID, &selfID)
	extraDocs := map[identifier.Identifier]*document.D{
		selfID: selfDoc,
	}
	c := newTestConverter(t, properties, nil, extraDocs)

	ctx := t.Context()
	info, errE := c.getDocumentInfo(ctx, selfID)
	require.NoError(t, errE, "% -+#.1v", errE)
	// Self-reference is excluded to avoid duplicates.
	assert.Empty(t, info.Ancestors[internalCore.SubclassOfPropID])
}

func TestGetDocumentInfoMutualCycle(t *testing.T) {
	t.Parallel()

	subentityDoc := makePropertyDoc(internalCore.SubentityOfPropID, nil)
	subclassDoc := makePropertyDoc(internalCore.SubclassOfPropID, &internalCore.SubentityOfPropID)
	subpropDoc := makePropertyDoc(internalCore.SubpropertyOfPropID, &internalCore.SubentityOfPropID)
	instanceDoc := makePropertyDoc(internalCore.InstanceOfPropID, &internalCore.SubentityOfPropID)
	properties := []*document.D{subentityDoc, subclassDoc, subpropDoc, instanceDoc}

	a := identifier.New()
	b := identifier.New()
	aDoc := makeHierarchyDoc(a, "A", internalCore.SubclassOfPropID, &b)
	bDoc := makeHierarchyDoc(b, "B", internalCore.SubclassOfPropID, &a)
	extraDocs := map[identifier.Identifier]*document.D{
		a: aDoc,
		b: bDoc,
	}
	c := newTestConverter(t, properties, nil, extraDocs)

	ctx := t.Context()
	infoA, errE := c.getDocumentInfo(ctx, a)
	require.NoError(t, errE, "% -+#.1v", errE)
	// A should have B as ancestor (cycle handled without infinite loop).
	assert.Contains(t, infoA.Ancestors[internalCore.SubclassOfPropID], b)
}

func TestGetDocumentInfoMultipleHierarchies(t *testing.T) {
	t.Parallel()

	// Custom hierarchy property PART_OF as a sub-property of SUBENTITY_OF.
	partOfPropID := identifier.New()
	subentityDoc := makePropertyDoc(internalCore.SubentityOfPropID, nil)
	subclassDoc := makePropertyDoc(internalCore.SubclassOfPropID, &internalCore.SubentityOfPropID)
	partOfDoc := makePropertyDoc(partOfPropID, &internalCore.SubentityOfPropID)
	subpropDoc := makePropertyDoc(internalCore.SubpropertyOfPropID, &internalCore.SubentityOfPropID)
	instanceDoc := makePropertyDoc(internalCore.InstanceOfPropID, &internalCore.SubentityOfPropID)
	properties := []*document.D{subentityDoc, subclassDoc, partOfDoc, subpropDoc, instanceDoc}

	classParent := identifier.New()
	partParent := identifier.New()
	child := identifier.New()

	// Child has SUBCLASS_OF -> classParent and PART_OF -> partParent.
	childClaims := &document.ClaimTypes{}
	childClaims.String = append(childClaims.String, document.StringClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: internalCore.NamingPropID},
		String:    "Child",
	})
	childClaims.Reference = append(childClaims.Reference,
		document.ReferenceClaim{
			CoreClaim: makeCoreClaim(document.HighConfidence, nil),
			Prop:      document.Reference{ID: internalCore.SubclassOfPropID},
			To:        document.Reference{ID: classParent},
		},
		document.ReferenceClaim{
			CoreClaim: makeCoreClaim(document.HighConfidence, nil),
			Prop:      document.Reference{ID: partOfPropID},
			To:        document.Reference{ID: partParent},
		},
	)
	childDoc := &document.D{
		CoreDocument: document.CoreDocument{ID: child}, //nolint:exhaustruct
		Claims:       childClaims,
	}
	classParentDoc := makeNamingDoc(classParent, "ClassParent")
	partParentDoc := makeNamingDoc(partParent, "PartParent")

	extraDocs := map[identifier.Identifier]*document.D{
		child:       childDoc,
		classParent: classParentDoc,
		partParent:  partParentDoc,
	}
	c := newTestConverter(t, properties, nil, extraDocs)

	ctx := t.Context()
	info, errE := c.getDocumentInfo(ctx, child)
	require.NoError(t, errE, "% -+#.1v", errE)
	// Ancestors should be computed separately for each hierarchy.
	assert.Contains(t, info.Ancestors[internalCore.SubclassOfPropID], classParent)
	assert.Contains(t, info.Ancestors[partOfPropID], partParent)
	// Each hierarchy should only contain its own ancestors.
	assert.NotContains(t, info.Ancestors[internalCore.SubclassOfPropID], partParent)
	assert.NotContains(t, info.Ancestors[partOfPropID], classParent)
}

func TestGetDocumentInfoOverlappingHierarchies(t *testing.T) {
	t.Parallel()

	// Custom hierarchy property PART_OF as a sub-property of SUBENTITY_OF.
	partOfPropID := identifier.New()
	subentityDoc := makePropertyDoc(internalCore.SubentityOfPropID, nil)
	subclassDoc := makePropertyDoc(internalCore.SubclassOfPropID, &internalCore.SubentityOfPropID)
	partOfDoc := makePropertyDoc(partOfPropID, &internalCore.SubentityOfPropID)
	subpropDoc := makePropertyDoc(internalCore.SubpropertyOfPropID, &internalCore.SubentityOfPropID)
	instanceDoc := makePropertyDoc(internalCore.InstanceOfPropID, &internalCore.SubentityOfPropID)
	properties := []*document.D{subentityDoc, subclassDoc, partOfDoc, subpropDoc, instanceDoc}

	// Same parent reachable via both SUBCLASS_OF and PART_OF.
	sharedParent := identifier.New()
	child := identifier.New()

	childClaims := &document.ClaimTypes{}
	childClaims.String = append(childClaims.String, document.StringClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: internalCore.NamingPropID},
		String:    "Child",
	})
	childClaims.Reference = append(childClaims.Reference,
		document.ReferenceClaim{
			CoreClaim: makeCoreClaim(document.HighConfidence, nil),
			Prop:      document.Reference{ID: internalCore.SubclassOfPropID},
			To:        document.Reference{ID: sharedParent},
		},
		document.ReferenceClaim{
			CoreClaim: makeCoreClaim(document.HighConfidence, nil),
			Prop:      document.Reference{ID: partOfPropID},
			To:        document.Reference{ID: sharedParent},
		},
	)
	childDoc := &document.D{
		CoreDocument: document.CoreDocument{ID: child}, //nolint:exhaustruct
		Claims:       childClaims,
	}
	sharedParentDoc := makeNamingDoc(sharedParent, "SharedParent")

	extraDocs := map[identifier.Identifier]*document.D{
		child:        childDoc,
		sharedParent: sharedParentDoc,
	}
	c := newTestConverter(t, properties, nil, extraDocs)

	ctx := t.Context()
	info, errE := c.getDocumentInfo(ctx, child)
	require.NoError(t, errE, "% -+#.1v", errE)
	// Shared parent appears in both hierarchies.
	assert.Contains(t, info.Ancestors[internalCore.SubclassOfPropID], sharedParent)
	assert.Contains(t, info.Ancestors[partOfPropID], sharedParent)
}

func TestBuildNamingProperties(t *testing.T) {
	t.Parallel()

	// internalCore.NamingPropID is NAMING, testPropID is a subproperty of NAMING.
	namingDoc := makePropertyDoc(internalCore.NamingPropID, nil)
	subNaming := makePropertyDoc(testPropID, &internalCore.NamingPropID)

	c := &Converter{ //nolint:exhaustruct
		documentInfoCache: map[identifier.Identifier]documentInfo{},
	}
	c.buildPropertyHierarchy([]*document.D{namingDoc, subNaming})
	c.buildNamingProperties()

	assert.Contains(t, c.namingProperties, internalCore.NamingPropID)
	assert.Contains(t, c.namingProperties, testPropID)
	assert.NotContains(t, c.namingProperties, testPropID2)
}

func TestDiscoverValueHierarchyProperties(t *testing.T) {
	t.Parallel()

	// Standard properties: SUBENTITY_OF with INSTANCE_OF, SUBCLASS_OF, SUBPROPERTY_OF as children.
	subentityDoc := makePropertyDoc(internalCore.SubentityOfPropID, nil)
	subclassDoc := makePropertyDoc(internalCore.SubclassOfPropID, &internalCore.SubentityOfPropID)
	subpropDoc := makePropertyDoc(internalCore.SubpropertyOfPropID, &internalCore.SubentityOfPropID)
	instanceDoc := makePropertyDoc(internalCore.InstanceOfPropID, &internalCore.SubentityOfPropID)
	properties := []*document.D{subentityDoc, subclassDoc, subpropDoc, instanceDoc}

	c := &Converter{ //nolint:exhaustruct
		documentInfoCache: map[identifier.Identifier]documentInfo{},
	}
	c.buildPropertyHierarchy(properties)
	c.discoverValueHierarchyProperties()

	// Only SUBCLASS_OF should be a value hierarchy property.
	// INSTANCE_OF and SUBPROPERTY_OF are excluded.
	assert.Contains(t, c.valueHierarchyProperties, internalCore.SubclassOfPropID)
	assert.NotContains(t, c.valueHierarchyProperties, internalCore.InstanceOfPropID)
	assert.NotContains(t, c.valueHierarchyProperties, internalCore.SubpropertyOfPropID)
	assert.Len(t, c.valueHierarchyProperties, 1)
}

func TestDiscoverValueHierarchyPropertiesCustom(t *testing.T) {
	t.Parallel()

	// Add a custom PART_OF property as a sub-property of SUBENTITY_OF.
	partOfPropID := identifier.New()
	subentityDoc := makePropertyDoc(internalCore.SubentityOfPropID, nil)
	subclassDoc := makePropertyDoc(internalCore.SubclassOfPropID, &internalCore.SubentityOfPropID)
	subpropDoc := makePropertyDoc(internalCore.SubpropertyOfPropID, &internalCore.SubentityOfPropID)
	instanceDoc := makePropertyDoc(internalCore.InstanceOfPropID, &internalCore.SubentityOfPropID)
	partOfDoc := makePropertyDoc(partOfPropID, &internalCore.SubentityOfPropID)
	properties := []*document.D{subentityDoc, subclassDoc, subpropDoc, instanceDoc, partOfDoc}

	c := &Converter{ //nolint:exhaustruct
		documentInfoCache: map[identifier.Identifier]documentInfo{},
	}
	c.buildPropertyHierarchy(properties)
	c.discoverValueHierarchyProperties()

	// Both SUBCLASS_OF and PART_OF should be value hierarchy properties.
	assert.Contains(t, c.valueHierarchyProperties, internalCore.SubclassOfPropID)
	assert.Contains(t, c.valueHierarchyProperties, partOfPropID)
	assert.NotContains(t, c.valueHierarchyProperties, internalCore.InstanceOfPropID)
	assert.NotContains(t, c.valueHierarchyProperties, internalCore.SubpropertyOfPropID)
	assert.Len(t, c.valueHierarchyProperties, 2)
}

func TestConvertRelationMultipleHierarchies(t *testing.T) {
	t.Parallel()

	// Custom hierarchy property PART_OF as a sub-property of SUBENTITY_OF.
	partOfPropID := identifier.New()
	subentityDoc := makePropertyDoc(internalCore.SubentityOfPropID, nil)
	subclassDoc := makePropertyDoc(internalCore.SubclassOfPropID, &internalCore.SubentityOfPropID)
	partOfDoc := makePropertyDoc(partOfPropID, &internalCore.SubentityOfPropID)
	subpropDoc := makePropertyDoc(internalCore.SubpropertyOfPropID, &internalCore.SubentityOfPropID)
	instanceDoc := makePropertyDoc(internalCore.InstanceOfPropID, &internalCore.SubentityOfPropID)
	properties := []*document.D{subentityDoc, subclassDoc, partOfDoc, subpropDoc, instanceDoc}

	classParent := identifier.New()
	partParent := identifier.New()
	target := identifier.New()

	// Target has SUBCLASS_OF -> classParent and PART_OF -> partParent.
	targetClaims := &document.ClaimTypes{}
	targetClaims.String = append(targetClaims.String, document.StringClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: internalCore.NamingPropID},
		String:    "Target",
	})
	targetClaims.Reference = append(targetClaims.Reference,
		document.ReferenceClaim{
			CoreClaim: makeCoreClaim(document.HighConfidence, nil),
			Prop:      document.Reference{ID: internalCore.SubclassOfPropID},
			To:        document.Reference{ID: classParent},
		},
		document.ReferenceClaim{
			CoreClaim: makeCoreClaim(document.HighConfidence, nil),
			Prop:      document.Reference{ID: partOfPropID},
			To:        document.Reference{ID: partParent},
		},
	)
	targetDoc := &document.D{
		CoreDocument: document.CoreDocument{ID: target}, //nolint:exhaustruct
		Claims:       targetClaims,
	}

	propDoc := makeNamingDoc(testPropID, "Rel Prop")
	classParentDoc := makeNamingDoc(classParent, "ClassParent")
	partParentDoc := makeNamingDoc(partParent, "PartParent")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID:  propDoc,
		target:      targetDoc,
		classParent: classParentDoc,
		partParent:  partParentDoc,
	}
	c := newTestConverter(t, properties, nil, extraDocs)

	ctx := t.Context()
	claim := &document.ReferenceClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: testPropID},
		To:        document.Reference{ID: target},
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	result, _, errE := c.convertReference(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	// Target + classParent + partParent = 3 claims.
	require.Len(t, result, 3)
	toIDs := make([]identifier.Identifier, 0, len(result))
	for _, r := range result {
		toIDs = append(toIDs, r.To)
	}
	assert.Contains(t, toIDs, target)
	assert.Contains(t, toIDs, classParent)
	assert.Contains(t, toIDs, partParent)
}

func TestConvertRelationOverlappingAncestors(t *testing.T) {
	t.Parallel()

	// Custom hierarchy property PART_OF.
	partOfPropID := identifier.New()
	subentityDoc := makePropertyDoc(internalCore.SubentityOfPropID, nil)
	subclassDoc := makePropertyDoc(internalCore.SubclassOfPropID, &internalCore.SubentityOfPropID)
	partOfDoc := makePropertyDoc(partOfPropID, &internalCore.SubentityOfPropID)
	subpropDoc := makePropertyDoc(internalCore.SubpropertyOfPropID, &internalCore.SubentityOfPropID)
	instanceDoc := makePropertyDoc(internalCore.InstanceOfPropID, &internalCore.SubentityOfPropID)
	properties := []*document.D{subentityDoc, subclassDoc, partOfDoc, subpropDoc, instanceDoc}

	// Same ancestor reachable via both SUBCLASS_OF and PART_OF.
	sharedAncestor := identifier.New()
	target := identifier.New()

	targetClaims := &document.ClaimTypes{}
	targetClaims.String = append(targetClaims.String, document.StringClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: internalCore.NamingPropID},
		String:    "Target",
	})
	targetClaims.Reference = append(targetClaims.Reference,
		document.ReferenceClaim{
			CoreClaim: makeCoreClaim(document.HighConfidence, nil),
			Prop:      document.Reference{ID: internalCore.SubclassOfPropID},
			To:        document.Reference{ID: sharedAncestor},
		},
		document.ReferenceClaim{
			CoreClaim: makeCoreClaim(document.HighConfidence, nil),
			Prop:      document.Reference{ID: partOfPropID},
			To:        document.Reference{ID: sharedAncestor},
		},
	)
	targetDoc := &document.D{
		CoreDocument: document.CoreDocument{ID: target}, //nolint:exhaustruct
		Claims:       targetClaims,
	}

	propDoc := makeNamingDoc(testPropID, "Rel Prop")
	sharedDoc := makeNamingDoc(sharedAncestor, "Shared")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID:     propDoc,
		target:         targetDoc,
		sharedAncestor: sharedDoc,
	}
	c := newTestConverter(t, properties, nil, extraDocs)

	ctx := t.Context()
	claim := &document.ReferenceClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: testPropID},
		To:        document.Reference{ID: target},
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	result, _, errE := c.convertReference(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	// Target + sharedAncestor (deduplicated) = 2 claims.
	require.Len(t, result, 2)
	toIDs := make([]identifier.Identifier, 0, len(result))
	for _, r := range result {
		toIDs = append(toIDs, r.To)
	}
	assert.Contains(t, toIDs, target)
	assert.Contains(t, toIDs, sharedAncestor)
}

func TestBuildLanguageCodes(t *testing.T) {
	t.Parallel()

	enDoc := makeLanguageDoc(testLangDocID, "en")
	slID := identifier.New()
	slDoc := makeLanguageDoc(slID, "sl")

	c := &Converter{ //nolint:exhaustruct
		enabledLanguages:    SupportedLanguages,
		recognizedLanguages: SupportedLanguages,
		documentInfoCache:   map[identifier.Identifier]documentInfo{},
	}
	c.buildLanguageCodes([]*document.D{enDoc, slDoc})

	assert.Equal(t, "en", c.LanguageCodes[testLangDocID])
	assert.Equal(t, "sl", c.LanguageCodes[slID])
}

func TestBuildLanguageCodesSubtag(t *testing.T) {
	t.Parallel()

	// Language code "en-US" should be cut to "en".
	langDoc := makeLanguageDoc(testLangDocID, "en-US")

	c := &Converter{ //nolint:exhaustruct
		enabledLanguages:    SupportedLanguages,
		recognizedLanguages: SupportedLanguages,
		documentInfoCache:   map[identifier.Identifier]documentInfo{},
	}
	c.buildLanguageCodes([]*document.D{langDoc})

	assert.Equal(t, "en", c.LanguageCodes[testLangDocID])
}

func TestBuildLanguageCodesSkipsNonLanguage(t *testing.T) {
	t.Parallel()

	// Not an instance of LANGUAGE.
	notLang := &document.D{
		CoreDocument: document.CoreDocument{ID: identifier.New()}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			Identifier: []document.IdentifierClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: internalCore.CodePropID},
					Value:     "en",
				},
			},
		},
	}

	c := &Converter{ //nolint:exhaustruct
		documentInfoCache: map[identifier.Identifier]documentInfo{},
	}
	c.buildLanguageCodes([]*document.D{notLang})

	assert.Empty(t, c.LanguageCodes)
}

func TestExtractInLanguages(t *testing.T) {
	t.Parallel()

	c := &Converter{ //nolint:exhaustruct
		enabledLanguages:    SupportedLanguages,
		recognizedLanguages: SupportedLanguages,
		LanguageCodes: map[identifier.Identifier]string{
			testLangDocID: "en",
		},
	}

	// Sub-claims with IN_LANGUAGE relation to a known language.
	sub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: internalCore.InLanguagePropID},
				To:        document.Reference{ID: testLangDocID},
			},
		},
	}
	langs := c.extractInLanguages(sub)
	assert.Equal(t, []string{"en"}, langs)

	// No sub-claims.
	langs = c.extractInLanguages(nil)
	assert.Equal(t, []string{"und"}, langs)

	// Sub-claims with unknown language.
	unknownLangID := identifier.New()
	subUnknown := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: internalCore.InLanguagePropID},
				To:        document.Reference{ID: unknownLangID},
			},
		},
	}
	langs = c.extractInLanguages(subUnknown)
	assert.Equal(t, []string{"und"}, langs)
}

func TestExtractInLanguagesUnsupportedLanguage(t *testing.T) {
	t.Parallel()

	// Language code is "xx" which is not in SupportedLanguages.
	xxLangID := identifier.New()
	c := &Converter{ //nolint:exhaustruct
		LanguageCodes: map[identifier.Identifier]string{
			xxLangID: "xx",
		},
	}

	sub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: internalCore.InLanguagePropID},
				To:        document.Reference{ID: xxLangID},
			},
		},
	}
	langs := c.extractInLanguages(sub)
	assert.Equal(t, []string{"und"}, langs)
}

func TestExtractInLanguagesMultiple(t *testing.T) {
	t.Parallel()

	enLangID := identifier.New()
	slLangID := identifier.New()
	c := &Converter{ //nolint:exhaustruct
		enabledLanguages:    SupportedLanguages,
		recognizedLanguages: SupportedLanguages,
		LanguageCodes: map[identifier.Identifier]string{
			enLangID: "en",
			slLangID: "sl",
		},
	}

	sub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: internalCore.InLanguagePropID},
				To:        document.Reference{ID: enLangID},
			},
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: internalCore.InLanguagePropID},
				To:        document.Reference{ID: slLangID},
			},
		},
	}
	langs := c.extractInLanguages(sub)
	assert.Len(t, langs, 2)
	assert.Contains(t, langs, "en")
	assert.Contains(t, langs, "sl")
}

func TestExtractInUnit(t *testing.T) {
	t.Parallel()

	c := &Converter{}

	// Sub-claims with IN_UNIT relation.
	sub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: internalCore.InUnitPropID},
				To:        document.Reference{ID: testUnitDocID},
			},
		},
	}
	unit := c.extractInUnit(sub)
	require.NotNil(t, unit)
	assert.Equal(t, testUnitDocID, *unit)

	// No sub-claims.
	unit = c.extractInUnit(nil)
	assert.Nil(t, unit)

	// Empty sub-claims.
	unit = c.extractInUnit(&document.ClaimTypes{})
	assert.Nil(t, unit)
}

func TestPropagateProp(t *testing.T) {
	t.Parallel()

	c := &Converter{ //nolint:exhaustruct
		IndexAncestorProperties: true,
		propertyAncestors: map[identifier.Identifier][]identifier.Identifier{
			testPropID: {testParentProp},
		},
	}

	result := c.propagateProp(testPropID)
	assert.Len(t, result, 2)
	assert.Equal(t, testPropID, result[0])
	assert.Equal(t, testParentProp, result[1])

	// No ancestors.
	result = c.propagateProp(testPropID2)
	assert.Len(t, result, 1)
	assert.Equal(t, testPropID2, result[0])

	// With ancestor indexing disabled (the default), only the original
	// property is returned, even when ancestors are known.
	c.IndexAncestorProperties = false
	result = c.propagateProp(testPropID)
	assert.Equal(t, []identifier.Identifier{testPropID}, result)
	result = c.propagateProp(testPropID2)
	assert.Equal(t, []identifier.Identifier{testPropID2}, result)
}

func TestNamingStrings(t *testing.T) {
	t.Parallel()

	c := &Converter{ //nolint:exhaustruct
		namingProperties: []identifier.Identifier{internalCore.NamingPropID},
		LanguageCodes:    map[identifier.Identifier]string{},
	}

	doc := makeNamingDoc(testDocID, "Test Document")
	result := c.namingStrings(doc)
	require.NotNil(t, result)
	assert.Equal(t, []string{"Test Document"}, result["und"])
}

func TestNamingStringsEmpty(t *testing.T) {
	t.Parallel()

	c := &Converter{ //nolint:exhaustruct
		namingProperties: []identifier.Identifier{internalCore.NamingPropID},
		LanguageCodes:    map[identifier.Identifier]string{},
	}

	// Document with no naming strings.
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
	}
	result := c.namingStrings(doc)
	assert.Nil(t, result)
}

func TestNamingStringsSorted(t *testing.T) {
	t.Parallel()

	c := &Converter{ //nolint:exhaustruct
		namingProperties: []identifier.Identifier{internalCore.NamingPropID},
		LanguageCodes:    map[identifier.Identifier]string{},
	}

	// Two naming strings with different confidences.
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(document.MediumConfidence, nil),
					Prop:      document.Reference{ID: internalCore.NamingPropID},
					String:    "Medium",
				},
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: internalCore.NamingPropID},
					String:    "High",
				},
			},
		},
	}
	result := c.namingStrings(doc)
	require.NotNil(t, result)
	// Higher confidence should come first.
	assert.Equal(t, "High", result["und"][0])
	assert.Equal(t, "Medium", result["und"][1])
}

func TestMakeDisplayStrings(t *testing.T) {
	t.Parallel()

	c := &Converter{ //nolint:exhaustruct
		enabledLanguages:    SupportedLanguages,
		recognizedLanguages: SupportedLanguages,
		namingProperties:    []identifier.Identifier{internalCore.NamingPropID},
		LanguageCodes:       map[identifier.Identifier]string{},
	}

	// Two naming strings: first becomes Display, Naming contains all strings.
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: internalCore.NamingPropID},
					String:    "Primary",
				},
				{
					CoreClaim: makeCoreClaim(document.MediumConfidence, nil),
					Prop:      document.Reference{ID: internalCore.NamingPropID},
					String:    "Secondary",
				},
			},
		},
	}

	ctx := t.Context()
	display, errE := c.makeDisplayStrings(ctx, doc)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, "Primary", display.Display["und"])
	// Naming contains all naming strings, independent of Display.
	assert.Equal(t, []string{"Primary", "Secondary"}, display.Naming["und"])
}

func TestMakeDisplayStringsSanitizesNullBytes(t *testing.T) {
	t.Parallel()

	c := &Converter{ //nolint:exhaustruct
		enabledLanguages:    SupportedLanguages,
		recognizedLanguages: SupportedLanguages,
		namingProperties:    []identifier.Identifier{internalCore.NamingPropID},
		LanguageCodes:       map[identifier.Identifier]string{},
	}

	// Naming string with null byte should have it stripped.
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: internalCore.NamingPropID},
					String:    "Before\x00After",
				},
			},
		},
	}

	ctx := t.Context()
	display, errE := c.makeDisplayStrings(ctx, doc)
	require.NoError(t, errE, "% -+#.1v", errE)
	// Null byte should be removed from display string.
	assert.Equal(t, "BeforeAfter", display.Display["und"])
}

func TestGetDisplayStringsCache(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Test Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}

	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	ds1, errE := c.getDisplayStrings(ctx, testPropID)
	require.NoError(t, errE, "% -+#.1v", errE)
	ds2, errE := c.getDisplayStrings(ctx, testPropID)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, ds1, ds2)
}

func TestGetDisplayStringsNotFound(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	_, errE := c.getDisplayStrings(ctx, identifier.New())
	assert.EqualError(t, errE, "document not found")
}

func TestNewConverter(t *testing.T) {
	t.Parallel()

	namingDoc := makePropertyDoc(internalCore.NamingPropID, nil)
	subProp := makePropertyDoc(testPropID, &internalCore.NamingPropID)
	langDoc := makeLanguageDoc(testLangDocID, "en")

	extraDocs := map[identifier.Identifier]*document.D{}
	c := newTestConverter(t, []*document.D{namingDoc, subProp}, []*document.D{langDoc}, extraDocs)

	assert.Contains(t, c.namingProperties, internalCore.NamingPropID)
	assert.Contains(t, c.namingProperties, testPropID)
	assert.Equal(t, "en", c.LanguageCodes[testLangDocID])
	assert.NotNil(t, c.documentInfoCache)
}

func TestVisitIdentifierPopulatesText(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			Identifier: []document.IdentifierClaim{{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: testPropID},
				Value:     "Q42",
			}},
		},
	}
	result, errE := c.FromDocument(ctx, doc, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []string{testDocID.String(), "Q42"}, result.Text["und"])
}

func TestVisitStringPopulatesTextDefaultsToUnd(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			String: []document.StringClaim{{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: testPropID},
				String:    "hello world",
			}},
		},
	}
	result, errE := c.FromDocument(ctx, doc, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []string{testDocID.String(), "hello world"}, result.Text["und"])
	// The document ID and the String claim with no IN_LANGUAGE both go only to
	// "und", so no other language bucket exists.
	_, hasEn := result.Text["en"]
	assert.False(t, hasEn)
}

func TestVisitStringPopulatesTextWithLanguage(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, map[identifier.Identifier]*document.D{
		testLangDocID: makeNamingDoc(testLangDocID, "English"),
	})
	c.LanguageCodes = map[identifier.Identifier]string{testLangDocID: "en"}

	ctx := t.Context()
	sub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{{
			CoreClaim: makeCoreClaim(document.HighConfidence, nil),
			Prop:      document.Reference{ID: internalCore.InLanguagePropID},
			To:        document.Reference{ID: testLangDocID},
		}},
	}
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			String: []document.StringClaim{{
				CoreClaim: makeCoreClaim(document.HighConfidence, sub),
				Prop:      document.Reference{ID: testPropID},
				String:    "hello",
			}},
		},
	}
	result, errE := c.FromDocument(ctx, doc, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	// The English-tagged String claim goes to text["en"].
	// The IN_LANGUAGE sub-reference name folds into text["und"]
	// alongside the document ID.
	assert.Equal(t, []string{"hello"}, result.Text["en"])
	assert.Equal(t, []string{testDocID.String(), "English"}, result.Text["und"])
}

func TestVisitHTMLStripsAndPopulatesText(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			HTML: []document.HTMLClaim{{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: testPropID},
				HTML:      "<p>hello</p><p>world</p>",
			}},
		},
	}
	result, errE := c.FromDocument(ctx, doc, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	// HTML tags are stripped in Go and adjacent block elements get a separating
	// space so the tokenizer treats the words as distinct tokens. The first
	// entry is the document's own ID, always seeded under "und".
	require.Len(t, result.Text["und"], 2)
	assert.Equal(t, testDocID.String(), result.Text["und"][0])
	assert.Equal(t, "hello world", result.Text["und"][1])
}

// TestTextAggregatesAcrossClaims verifies that multiple textual claims of any
// type contribute separate values to the same per-language text bucket.
func TestTextAggregatesAcrossClaims(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			Identifier: []document.IdentifierClaim{{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: testPropID},
				Value:     "Q42",
			}},
			String: []document.StringClaim{{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: testPropID},
				String:    "alpha",
			}},
			Link: []document.LinkClaim{{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: testPropID},
				IRI:       "https://example.com",
			}},
		},
	}
	result, errE := c.FromDocument(ctx, doc, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.ElementsMatch(t, []string{testDocID.String(), "Q42", "alpha", "https://example.com"}, result.Text["und"])
}

func TestStripHTML(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		// Baseline.
		{"plain text", "hello world", "hello world"},
		{"empty", "", ""},
		{"tags only", "<br/><br/>", ""},
		{"single block element", "<p>hello</p>", "hello"},

		// Block tags insert a single space between text fragments.
		{"adjacent paragraphs", "<p>hello</p><p>world</p>", "hello world"},
		{"line break splits", "foo<br>bar", "foo bar"},
		{"horizontal rule splits", "foo<hr>bar", "foo bar"},
		{"list items split", "<ul><li>foo</li><li>bar</li></ul>", "foo bar"},
		{"heading and paragraph", "<h1>Title</h1><p>body</p>", "Title body"},
		{"blockquote splits", "intro<blockquote>quoted</blockquote>outro", "intro quoted outro"},
		{"pre splits", "before<pre>code</pre>after", "before code after"},

		// Inline tags do NOT insert a space: text fragments are concatenated.
		{"inline anchor", `foo<a href="">bar</a>`, "foobar"},
		{"inline bold then italic", "<b>foo</b><i>bar</i>", "foobar"},
		{"inline strike, tt, u", "<strike>a</strike><tt>b</tt><u>c</u>", "abc"},
		{"void img is inline", `text<img src="x" alt="y"/>more`, "textmore"},
		{"inline inside block", "<p>foo<b>bar</b></p>", "foobar"},
		{"block surrounding inline-only run", "<p><b>foo</b><i>bar</i></p><p><b>baz</b></p>", "foobar baz"},

		// Whitespace-only text tokens between tags do not add their own space;
		// the block tag still provides the single separator.
		{"whitespace between blocks collapses", "<p>foo</p>   <p>bar</p>", "foo bar"},
		{"inner whitespace preserved", "<p>foo bar</p>", "foo bar"},

		// Per HTML spec, vertical tab (\v) and NBSP are NOT whitespace, so
		// they should not be trimmed or treated as separator-only tokens.
		{"vertical tab is content not whitespace", "   \v   ", "\v"},
		{"vertical tab between blocks is content", "<p>foo</p>\v<p>bar</p>", "foo \v bar"},
		{"nbsp is content not whitespace", "     ", " "},

		// Source-side whitespace adjacent to an inline tag is significant: it's
		// the only signal that visually-rendered text had a gap.
		{"whitespace before inline", "foo<a>bar</a>", "foobar"},
		{"whitespace between inline elements", "<b>foo</b> <i>bar</i>", "foo bar"},
		{"trailing whitespace before inline tag", "foo  <a>bar</a>", "foo bar"},
		{"leading whitespace inside inline tag", "foo<a>  bar</a>", "foo bar"},

		// Real-world-ish: a sanitizer-shaped fragment with mixed inline.
		{"mixed inline run with link", `<b>Drago</b> <a href="x">Tršar</a>`, "Drago Tršar"},
		{"link concatenated to text", `text<a href="">link</a>more`, "textlinkmore"},

		// Unknown tags default to inserting a space (block-like). The
		// sanitizer normally strips these, but on raw input we prefer
		// over-tokenizing to silently merging unrelated words.
		{"unknown tag splits adjacent text", "foo<unknown>bar</unknown>baz", "foo bar baz"},
		{"unknown tag between inline runs", "<b>foo</b><span>bar</span><i>baz</i>", "foo bar baz"},
		{"adjacent unknown blocks split", "<div>foo</div><div>bar</div>", "foo bar"},
		{"unknown wrapper around inline run", "<div><b>foo</b><i>bar</i></div>", "foobar"},
		{"unknown wrapper with whitespace", "<div><b>foo</b> <i>bar</i></div>", "foo bar"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, stripHTML(tc.in, document.UndeterminedLanguage))
		})
	}
}

// TestStripHTMLNoBlockSpace exercises the no-block-space language branch by
// temporarily adding a synthetic language code to noBlockSpaceLanguages. It
// cannot run in parallel because it mutates a package-level map; for the same
// reason no other tests should mutate this map.
//
//nolint:paralleltest
func TestStripHTMLNoBlockSpace(t *testing.T) {
	const fakeLang = "test-noblockspace"

	noBlockSpaceLanguages[fakeLang] = true
	t.Cleanup(func() { delete(noBlockSpaceLanguages, fakeLang) })

	tests := []struct {
		name string
		in   string
		want string
	}{
		// Block tags do NOT insert a space for no-block-space languages.
		{"adjacent paragraphs concatenate", "<p>foo</p><p>bar</p>", "foobar"},
		{"line break does not split", "foo<br>bar", "foobar"},
		{"unknown tag does not split", "foo<div>bar</div>baz", "foobarbaz"},
		// Inline tags still concatenate (unchanged from the default branch).
		{"inline still concatenates", `foo<a href="">bar</a>`, "foobar"},
		// Source whitespace inside text tokens IS still preserved. The
		// language switch only changes the implicit-block-separator behavior,
		// not literal whitespace the author wrote.
		{"explicit whitespace preserved", "<p>foo</p> <p>bar</p>", "foo bar"},
		{"inner whitespace preserved", "<p>foo bar</p>", "foo bar"},
	}
	for _, tc := range tests {
		//nolint:paralleltest
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, stripHTML(tc.in, fakeLang))
		})
	}
}

func TestConvertAmount(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Amount Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	claim := &document.AmountClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: testPropID},
		Amount:    document.Amount("100"),
		Precision: 1,
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	result, errE := c.convertAmount(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, result, 1)
	assert.Equal(t, testPropID, result[0].Prop)
	require.NotNil(t, result[0].From)
	assert.Equal(t, 99.5, *result[0].From) //nolint:testifylint
	require.NotNil(t, result[0].To)
	assert.Equal(t, 100.5, *result[0].To) //nolint:testifylint
	assert.Equal(t, "100", result[0].FromDisplay)
	assert.Nil(t, result[0].Unit)
}

func TestConvertAmountWithUnit(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Amount Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	sub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: internalCore.InUnitPropID},
				To:        document.Reference{ID: testUnitDocID},
			},
		},
	}
	claim := &document.AmountClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, sub),
		Prop:      document.Reference{ID: testPropID},
		Amount:    document.Amount("42"),
		Precision: 1,
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	result, errE := c.convertAmount(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, result, 1)
	require.NotNil(t, result[0].Unit)
	assert.Equal(t, testUnitDocID, *result[0].Unit)
}

func TestConvertAmountInterval(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Amount Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	fromAmount := document.Amount("10")
	toAmount := document.Amount("20")
	fromPrec := 1.0
	toPrec := 1.0
	claim := &document.AmountIntervalClaim{ //nolint:exhaustruct
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: testPropID},
		From:          &fromAmount,
		FromPrecision: &fromPrec,
		To:            &toAmount,
		ToPrecision:   &toPrec,
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	amountClaims, unknownClaims, _, errE := c.convertAmountInterval(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, amountClaims, 1)
	assert.Empty(t, unknownClaims)
	require.NotNil(t, amountClaims[0].Range.GreaterThanOrEqual)
	assert.Nil(t, amountClaims[0].Range.GreaterThan)
	require.NotNil(t, amountClaims[0].Range.LessThan)
	assert.Nil(t, amountClaims[0].Range.LessThanOrEqual)

	// Default flags: from-window included -> lower = 10 - 0.5 = 9.5;
	// to-window included -> upper = 20 + 0.5 = 20.5.
	assert.Equal(t, 9.5, *amountClaims[0].Range.GreaterThanOrEqual) //nolint:testifylint
	assert.Equal(t, 20.5, *amountClaims[0].Range.LessThan)          //nolint:testifylint
	require.NotNil(t, amountClaims[0].From)
	require.NotNil(t, amountClaims[0].To)
	assert.Equal(t, *amountClaims[0].From, *amountClaims[0].Range.GreaterThanOrEqual) //nolint:testifylint
	assert.Equal(t, *amountClaims[0].To, *amountClaims[0].Range.LessThan)             //nolint:testifylint
	assert.Equal(t, "10", amountClaims[0].FromDisplay)
	assert.Equal(t, "20", amountClaims[0].ToDisplay)
}

// TestConvertAmountIntervalOpen verifies that with both *IsOpen flags set,
// each window is excluded: lower advances past from-window, upper retreats
// to before to-window.
func TestConvertAmountIntervalOpen(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Amount Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	fromAmount := document.Amount("10")
	toAmount := document.Amount("20")
	fromPrec := 1.0
	toPrec := 1.0
	claim := &document.AmountIntervalClaim{ //nolint:exhaustruct
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: testPropID},
		From:          &fromAmount,
		FromPrecision: &fromPrec,
		FromIsOpen:    true,
		To:            &toAmount,
		ToPrecision:   &toPrec,
		ToIsOpen:      true,
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	amountClaims, unknownClaims, _, errE := c.convertAmountInterval(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, amountClaims, 1)
	assert.Empty(t, unknownClaims)
	require.NotNil(t, amountClaims[0].Range.GreaterThanOrEqual)
	assert.Nil(t, amountClaims[0].Range.GreaterThan)
	require.NotNil(t, amountClaims[0].Range.LessThan)
	assert.Nil(t, amountClaims[0].Range.LessThanOrEqual)

	// FromIsOpen=true: lower advances to 10 + 0.5 = 10.5 (from-window excluded).
	// ToIsOpen=true:   upper retreats to 20 - 0.5 = 19.5 (to-window excluded).
	assert.Equal(t, 10.5, *amountClaims[0].Range.GreaterThanOrEqual) //nolint:testifylint
	assert.Equal(t, 19.5, *amountClaims[0].Range.LessThan)           //nolint:testifylint
	require.NotNil(t, amountClaims[0].From)
	require.NotNil(t, amountClaims[0].To)
	assert.Equal(t, *amountClaims[0].From, *amountClaims[0].Range.GreaterThanOrEqual) //nolint:testifylint
	assert.Equal(t, *amountClaims[0].To, *amountClaims[0].Range.LessThan)             //nolint:testifylint
}

// TestConvertAmountIntervalSinglePointSamePrecision verifies that an amount
// interval with from == to (same value, same precision) produces an
// identical indexed AmountClaim to what convertAmount produces for the same
// single point.
func TestConvertAmountIntervalSinglePointSamePrecision(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Amount Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	amount := document.Amount("100")
	prec := 1.0
	core := makeCoreClaim(document.HighConfidence, nil)
	prop := document.Reference{ID: testPropID}

	intervalClaim := &document.AmountIntervalClaim{ //nolint:exhaustruct
		CoreClaim:     core,
		Prop:          prop,
		From:          &amount,
		FromPrecision: &prec,
		To:            &amount,
		ToPrecision:   &prec,
	}
	errE := intervalClaim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	intervalClaims, unknownClaims, _, errE := c.convertAmountInterval(ctx, intervalClaim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, intervalClaims, 1)
	assert.Empty(t, unknownClaims)

	pointClaim := &document.AmountClaim{
		CoreClaim: core,
		Prop:      prop,
		Amount:    amount,
		Precision: prec,
	}
	errE = pointClaim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	pointClaims, errE := c.convertAmount(ctx, pointClaim)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Interval path with from == to must produce the same AmountClaim as the
	// single-point path.
	assert.Equal(t, pointClaims, intervalClaims)

	// Explicit value checks of the resulting bounds.
	require.NotNil(t, intervalClaims[0].From)
	require.NotNil(t, intervalClaims[0].To)
	require.NotNil(t, intervalClaims[0].Range.GreaterThanOrEqual)
	require.NotNil(t, intervalClaims[0].Range.LessThan)

	assert.Equal(t, 99.5, *intervalClaims[0].From)                                        //nolint:testifylint
	assert.Equal(t, 100.5, *intervalClaims[0].To)                                         //nolint:testifylint
	assert.Equal(t, *intervalClaims[0].From, *intervalClaims[0].Range.GreaterThanOrEqual) //nolint:testifylint
	assert.Equal(t, *intervalClaims[0].To, *intervalClaims[0].Range.LessThan)             //nolint:testifylint
	assert.Equal(t, "100", intervalClaims[0].FromDisplay)
	assert.Equal(t, "100", intervalClaims[0].ToDisplay)
}

// TestConvertAmountIntervalSinglePointDifferentPrecisions verifies that
// when from and to share the same value but have different precisions, the
// result reflects each side's own precision-window edge.
func TestConvertAmountIntervalSinglePointDifferentPrecisions(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Amount Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	amount := document.Amount("100")
	fromPrec := 10.0
	toPrec := 1.0
	claim := &document.AmountIntervalClaim{ //nolint:exhaustruct
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: testPropID},
		From:          &amount,
		FromPrecision: &fromPrec,
		To:            &amount,
		ToPrecision:   &toPrec,
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	amountClaims, unknownClaims, _, errE := c.convertAmountInterval(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, amountClaims, 1)
	assert.Empty(t, unknownClaims)

	// from-window (prec 10) start = 95, to-window (prec 1) end = 100.5.
	// Each side uses its own precision; the indexed range covers both.
	assert.Equal(t, 95.0, *amountClaims[0].From)                                      //nolint:testifylint
	assert.Equal(t, 100.5, *amountClaims[0].To)                                       //nolint:testifylint
	assert.Equal(t, *amountClaims[0].From, *amountClaims[0].Range.GreaterThanOrEqual) //nolint:testifylint
	assert.Equal(t, *amountClaims[0].To, *amountClaims[0].Range.LessThan)             //nolint:testifylint
	assert.Equal(t, "100", amountClaims[0].FromDisplay)
	assert.Equal(t, "100", amountClaims[0].ToDisplay)
}

// TestConvertAmountIntervalForwardAdjacent verifies the basic forward
// case with bounds exactly precision-apart, so their windows touch but
// don't overlap. from=10, to=20, both prec=10, both closed.
//   - from-window: [5, 15]; to-window: [15, 25]; share edge at 15.
//   - Expected indexed range: [5, 25) - union of both windows.
func TestConvertAmountIntervalForwardAdjacent(t *testing.T) { //nolint:dupl
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Amount Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	fromAmount := document.Amount("10")
	toAmount := document.Amount("20")
	prec := 10.0
	claim := &document.AmountIntervalClaim{ //nolint:exhaustruct
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: testPropID},
		From:          &fromAmount,
		FromPrecision: &prec,
		To:            &toAmount,
		ToPrecision:   &prec,
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	amountClaims, unknownClaims, _, errE := c.convertAmountInterval(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, amountClaims, 1)
	assert.Empty(t, unknownClaims)
	require.NotNil(t, amountClaims[0].Range.GreaterThanOrEqual)
	require.NotNil(t, amountClaims[0].Range.LessThan)

	assert.Equal(t, 5.0, *amountClaims[0].Range.GreaterThanOrEqual) //nolint:testifylint
	assert.Equal(t, 25.0, *amountClaims[0].Range.LessThan)          //nolint:testifylint
	require.NotNil(t, amountClaims[0].From)
	require.NotNil(t, amountClaims[0].To)
	assert.Equal(t, *amountClaims[0].From, *amountClaims[0].Range.GreaterThanOrEqual) //nolint:testifylint
	assert.Equal(t, *amountClaims[0].To, *amountClaims[0].Range.LessThan)             //nolint:testifylint
	assert.Equal(t, "10", amountClaims[0].FromDisplay)
	assert.Equal(t, "20", amountClaims[0].ToDisplay)
}

// TestConvertAmountIntervalOverlappingDirectedDecreasingRejected documents
// the behavior on inputs whose precision-windows would strictly overlap
// while values are directed-decreasing (e.g. from=12, to=10, prec=10:
// from-window [7, 17], to-window [5, 15], overlap [7, 15]).
//
// Such inputs are not constructible through normal validated claim flow,
// because Amount.Float64 enforces value-rounded-to-precision (12 is not
// rounded to precision 10 - only multiples of 10 are). This test bypasses
// AmountIntervalClaim.Validate and feeds the claim directly to the
// converter to confirm the convert layer rejects the same way: when its
// same-precision swap-criterion branch parses the values via Float64, the
// rounding check fires.
func TestConvertAmountIntervalOverlappingDirectedDecreasingRejected(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Amount Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	fromAmount := document.Amount("12")
	toAmount := document.Amount("10")
	prec := 10.0
	claim := &document.AmountIntervalClaim{ //nolint:exhaustruct
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: testPropID},
		From:          &fromAmount,
		FromPrecision: &prec,
		To:            &toAmount,
		ToPrecision:   &prec,
	}
	// Intentionally NO claim.Validate() call here - claim is deliberately
	// invalid and would not pass Validate().
	_, _, _, errE := c.convertAmountInterval(ctx, claim) //nolint:dogsled
	assert.ErrorContains(t, errE, "amount is not rounded to precision")
}

// TestConvertAmountIntervalDirectedDecreasingAdjacent verifies that an
// interval written in directed-decreasing form with adjacent windows
// (from=11, to=10, both prec=1, both closed) is swapped to ascending order.
// The un-swapped effective edges coincide (10.5 and 10.5), so the swap fires
// and the indexed range covers both windows: [9.5, 11.5).
func TestConvertAmountIntervalDirectedDecreasingAdjacent(t *testing.T) { //nolint:dupl
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Amount Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	fromAmount := document.Amount("11")
	toAmount := document.Amount("10")
	prec := 1.0
	claim := &document.AmountIntervalClaim{ //nolint:exhaustruct
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: testPropID},
		From:          &fromAmount,
		FromPrecision: &prec,
		To:            &toAmount,
		ToPrecision:   &prec,
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	amountClaims, unknownClaims, _, errE := c.convertAmountInterval(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, amountClaims, 1)
	assert.Empty(t, unknownClaims)
	require.NotNil(t, amountClaims[0].Range.GreaterThanOrEqual)
	require.NotNil(t, amountClaims[0].Range.LessThan)

	assert.Equal(t, 9.5, *amountClaims[0].Range.GreaterThanOrEqual) //nolint:testifylint
	assert.Equal(t, 11.5, *amountClaims[0].Range.LessThan)          //nolint:testifylint
	require.NotNil(t, amountClaims[0].From)
	require.NotNil(t, amountClaims[0].To)
	assert.Equal(t, *amountClaims[0].From, *amountClaims[0].Range.GreaterThanOrEqual) //nolint:testifylint
	assert.Equal(t, *amountClaims[0].To, *amountClaims[0].Range.LessThan)             //nolint:testifylint
	// Display strings follow the swapped orientation: smaller value is now From.
	assert.Equal(t, "10", amountClaims[0].FromDisplay)
	assert.Equal(t, "11", amountClaims[0].ToDisplay)
}

// TestConvertTimeIntervalDirectedDecreasingAdjacent verifies that an
// interval with from=2025 year, to=2024 year (both closed, adjacent
// year-windows) is swapped to ascending order. The un-swapped effective
// edges coincide (2025-01-01 = 2025-01-01), so the swap fires and the
// indexed range covers both years: [2024-01-01, 2026-01-01).
func TestConvertTimeIntervalDirectedDecreasingAdjacent(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Time Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	fromTS := document.Time("2025")
	toTS := document.Time("2024")
	prec := document.TimePrecisionYear
	claim := &document.TimeIntervalClaim{ //nolint:exhaustruct
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: testPropID},
		From:          &fromTS,
		FromPrecision: &prec,
		To:            &toTS,
		ToPrecision:   &prec,
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	timeClaims, unknownClaims, _, errE := c.convertTimeInterval(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, timeClaims, 1)
	assert.Empty(t, unknownClaims)
	require.NotNil(t, timeClaims[0].Range.GreaterThanOrEqual)
	require.NotNil(t, timeClaims[0].Range.LessThan)

	lowerTime := x.TimeFromFloat64(*timeClaims[0].Range.GreaterThanOrEqual).UTC()
	upperTime := x.TimeFromFloat64(*timeClaims[0].Range.LessThan).UTC()
	assert.Equal(t, time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC), lowerTime)
	assert.Equal(t, time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC), upperTime)
	require.NotNil(t, timeClaims[0].From)
	require.NotNil(t, timeClaims[0].To)
	assert.Equal(t, *timeClaims[0].From, *timeClaims[0].Range.GreaterThanOrEqual) //nolint:testifylint
	assert.Equal(t, *timeClaims[0].To, *timeClaims[0].Range.LessThan)             //nolint:testifylint
	// Display strings follow the swapped orientation.
	assert.Equal(t, "2024", timeClaims[0].FromDisplay)
	assert.Equal(t, "2025", timeClaims[0].ToDisplay)
}

// TestConvertAmountIntervalToIsOpenExcludesWindow verifies that
// ToIsOpen=true pulls the upper bound back to to_start (excluding the
// to-window) while the default (ToIsOpen=false) extends it to to_end
// (including the to-window).
func TestConvertAmountIntervalToIsOpenExcludesWindow(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Amount Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	fromAmount := document.Amount("0")
	toAmount := document.Amount("100")
	fromPrec := 1.0
	toPrec := 1.0
	mkClaim := func(toIsOpen bool) *document.AmountIntervalClaim {
		return &document.AmountIntervalClaim{ //nolint:exhaustruct
			CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
			Prop:          document.Reference{ID: testPropID},
			From:          &fromAmount,
			FromPrecision: &fromPrec,
			To:            &toAmount,
			ToPrecision:   &toPrec,
			ToIsOpen:      toIsOpen,
		}
	}

	defaultClaim := mkClaim(false)
	errE := defaultClaim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	defaultClaims, _, _, errE := c.convertAmountInterval(ctx, defaultClaim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, defaultClaims, 1)

	openClaim := mkClaim(true)
	errE = openClaim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	openClaims, _, _, errE := c.convertAmountInterval(ctx, openClaim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, openClaims, 1)

	// Default: upper = to_end = 100 + 0.5 = 100.5 (window included).
	require.NotNil(t, defaultClaims[0].Range.LessThan)
	assert.Equal(t, 100.5, *defaultClaims[0].Range.LessThan) //nolint:testifylint

	// ToIsOpen=true: upper = to_start = 100 - 0.5 = 99.5 (window excluded).
	require.NotNil(t, openClaims[0].Range.LessThan)
	assert.Equal(t, 99.5, *openClaims[0].Range.LessThan) //nolint:testifylint

	// Scalars coincide with range bounds.
	require.NotNil(t, defaultClaims[0].To)
	require.NotNil(t, openClaims[0].To)
	assert.Equal(t, *defaultClaims[0].To, *defaultClaims[0].Range.LessThan) //nolint:testifylint
	assert.Equal(t, *openClaims[0].To, *openClaims[0].Range.LessThan)       //nolint:testifylint
}

func TestConvertAmountIntervalFromNone(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Amount Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	toAmount := document.Amount("20")
	toPrec := 1.0
	claim := &document.AmountIntervalClaim{ //nolint:exhaustruct
		CoreClaim:   makeCoreClaim(document.HighConfidence, nil),
		Prop:        document.Reference{ID: testPropID},
		FromIsNone:  true,
		To:          &toAmount,
		ToPrecision: &toPrec,
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	amountClaims, unknownClaims, _, errE := c.convertAmountInterval(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, amountClaims, 1)
	assert.Empty(t, unknownClaims)
	// From should be nil since it's None.
	assert.Nil(t, amountClaims[0].From)
	assert.Equal(t, -math.MaxFloat64, *amountClaims[0].Range.GreaterThanOrEqual) //nolint:testifylint
}

func TestConvertAmountIntervalToNone(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Amount Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	fromAmount := document.Amount("10")
	fromPrec := 1.0
	claim := &document.AmountIntervalClaim{ //nolint:exhaustruct
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: testPropID},
		From:          &fromAmount,
		FromPrecision: &fromPrec,
		ToIsNone:      true,
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	amountClaims, unknownClaims, _, errE := c.convertAmountInterval(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, amountClaims, 1)
	assert.Empty(t, unknownClaims)
	assert.Nil(t, amountClaims[0].To)
	assert.Equal(t, math.MaxFloat64, *amountClaims[0].Range.LessThanOrEqual) //nolint:testifylint
}

func TestConvertAmountIntervalFromUnknownWithTo(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Amount Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	toAmount := document.Amount("20")
	toPrec := 1.0
	claim := &document.AmountIntervalClaim{ //nolint:exhaustruct
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: testPropID},
		FromIsUnknown: true,
		To:            &toAmount,
		ToPrecision:   &toPrec,
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	// Should treat as single point at To.
	amountClaims, unknownClaims, _, errE := c.convertAmountInterval(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, amountClaims, 1)
	assert.Empty(t, unknownClaims)
	assert.NotNil(t, amountClaims[0].From)
	assert.NotNil(t, amountClaims[0].To)
}

func TestConvertAmountIntervalToUnknownWithFrom(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Amount Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	fromAmount := document.Amount("10")
	fromPrec := 1.0
	claim := &document.AmountIntervalClaim{ //nolint:exhaustruct
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: testPropID},
		From:          &fromAmount,
		FromPrecision: &fromPrec,
		ToIsUnknown:   true,
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	// Should treat as single point at From.
	amountClaims, unknownClaims, _, errE := c.convertAmountInterval(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, amountClaims, 1)
	assert.Empty(t, unknownClaims)
}

func TestConvertAmountIntervalBothUnknown(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Amount Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	claim := &document.AmountIntervalClaim{ //nolint:exhaustruct
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: testPropID},
		FromIsUnknown: true,
		ToIsUnknown:   true,
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	// Both unknown: should become unknown claim.
	amountClaims, unknownClaims, _, errE := c.convertAmountInterval(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Empty(t, amountClaims)
	require.Len(t, unknownClaims, 1)
}

func TestConvertAmountIntervalFromNoneToUnknown(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Amount Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	claim := &document.AmountIntervalClaim{ //nolint:exhaustruct
		CoreClaim:   makeCoreClaim(document.HighConfidence, nil),
		Prop:        document.Reference{ID: testPropID},
		FromIsNone:  true,
		ToIsUnknown: true,
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	// From is None, To is Unknown with known From: becomes unknown.
	amountClaims, unknownClaims, _, errE := c.convertAmountInterval(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Empty(t, amountClaims)
	require.Len(t, unknownClaims, 1)
}

func TestConvertAmountIntervalMissingFromPrecision(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	fromAmount := document.Amount("10")
	toAmount := document.Amount("20")
	toPrec := 1.0
	claim := &document.AmountIntervalClaim{ //nolint:exhaustruct
		CoreClaim:   makeCoreClaim(document.HighConfidence, nil),
		Prop:        document.Reference{ID: testPropID},
		From:        &fromAmount,
		To:          &toAmount,
		ToPrecision: &toPrec,
	}
	// Intentionally NO claim.Validate() call here - claim is deliberately
	// invalid and would not pass Validate().
	_, _, _, errE := c.convertAmountInterval(ctx, claim) //nolint:dogsled
	assert.EqualError(t, errE, "missing from precision in claim")
}

func TestConvertAmountIntervalMissingToPrecision(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Amount Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	fromAmount := document.Amount("10")
	fromPrec := 1.0
	toAmount := document.Amount("20")
	claim := &document.AmountIntervalClaim{ //nolint:exhaustruct
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: testPropID},
		From:          &fromAmount,
		FromPrecision: &fromPrec,
		To:            &toAmount,
	}
	// Intentionally NO claim.Validate() call here - claim is deliberately
	// invalid and would not pass Validate().
	_, _, _, errE := c.convertAmountInterval(ctx, claim) //nolint:dogsled
	assert.EqualError(t, errE, "missing to precision in claim")
}

func TestConvertAmountIntervalFromUnknownMissingToPrecision(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	toAmount := document.Amount("20")
	claim := &document.AmountIntervalClaim{ //nolint:exhaustruct
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: testPropID},
		FromIsUnknown: true,
		To:            &toAmount,
	}
	// Intentionally NO claim.Validate() call here - claim is deliberately
	// invalid and would not pass Validate().
	_, _, _, errE := c.convertAmountInterval(ctx, claim) //nolint:dogsled
	assert.EqualError(t, errE, "missing to precision in claim")
}

func TestConvertAmountIntervalToUnknownMissingFromPrecision(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	fromAmount := document.Amount("10")
	claim := &document.AmountIntervalClaim{ //nolint:exhaustruct
		CoreClaim:   makeCoreClaim(document.HighConfidence, nil),
		Prop:        document.Reference{ID: testPropID},
		From:        &fromAmount,
		ToIsUnknown: true,
	}
	// Intentionally NO claim.Validate() call here - claim is deliberately
	// invalid and would not pass Validate().
	_, _, _, errE := c.convertAmountInterval(ctx, claim) //nolint:dogsled
	assert.EqualError(t, errE, "missing from precision in claim")
}

func TestConvertTime(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Time Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	claim := &document.TimeClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: testPropID},
		Time:      document.Time("2024-01-15"),
		Precision: document.TimePrecisionDay,
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	result, errE := c.convertTime(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, result, 1)
	assert.Equal(t, testPropID, result[0].Prop)
	require.NotNil(t, result[0].From)
	require.NotNil(t, result[0].To)
	// From should be start of day, To should be start of next day.
	fromTime := x.TimeFromFloat64(*result[0].From).UTC()
	toTime := x.TimeFromFloat64(*result[0].To).UTC()
	assert.Equal(t, 2024, fromTime.Year())
	assert.Equal(t, time.January, fromTime.Month())
	assert.Equal(t, 15, fromTime.Day())
	assert.Equal(t, 16, toTime.Day())
}

func TestConvertTimeInterval(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Time Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	fromTS := document.Time("2024-01-01")
	toTS := document.Time("2024-12-31")
	fromPrec := document.TimePrecisionDay
	toPrec := document.TimePrecisionDay
	claim := &document.TimeIntervalClaim{ //nolint:exhaustruct
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: testPropID},
		From:          &fromTS,
		FromPrecision: &fromPrec,
		To:            &toTS,
		ToPrecision:   &toPrec,
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	timeClaims, unknownClaims, _, errE := c.convertTimeInterval(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, timeClaims, 1)
	assert.Empty(t, unknownClaims)
	assert.NotNil(t, timeClaims[0].Range.GreaterThanOrEqual)
	assert.NotNil(t, timeClaims[0].Range.LessThan)
}

// TestConvertTimeIntervalOpen verifies that with both *IsOpen flags set,
// each window is excluded: lower advances past from-window, upper retreats
// to before to-window.
func TestConvertTimeIntervalOpen(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Time Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	fromTS := document.Time("2024-01-01")
	toTS := document.Time("2024-12-31")
	fromPrec := document.TimePrecisionDay
	toPrec := document.TimePrecisionDay
	claim := &document.TimeIntervalClaim{ //nolint:exhaustruct
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: testPropID},
		From:          &fromTS,
		FromPrecision: &fromPrec,
		FromIsOpen:    true,
		To:            &toTS,
		ToPrecision:   &toPrec,
		ToIsOpen:      true,
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	timeClaims, unknownClaims, _, errE := c.convertTimeInterval(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, timeClaims, 1)
	assert.Empty(t, unknownClaims)
	assert.NotNil(t, timeClaims[0].Range.GreaterThanOrEqual)
	assert.Nil(t, timeClaims[0].Range.GreaterThan)
	assert.NotNil(t, timeClaims[0].Range.LessThan)
	assert.Nil(t, timeClaims[0].Range.LessThanOrEqual)

	// FromIsOpen=true on day precision moves lower past 2024-01-01 to 2024-01-02.
	fromTime := x.TimeFromFloat64(*timeClaims[0].Range.GreaterThanOrEqual).UTC()
	assert.Equal(t, time.Date(2024, time.January, 2, 0, 0, 0, 0, time.UTC), fromTime)
	// ToIsOpen=true on day precision pulls upper back to start of 2024-12-31 (window excluded).
	toTime := x.TimeFromFloat64(*timeClaims[0].Range.LessThan).UTC()
	assert.Equal(t, time.Date(2024, time.December, 31, 0, 0, 0, 0, time.UTC), toTime)

	// Scalars coincide with range bounds.
	require.NotNil(t, timeClaims[0].From)
	require.NotNil(t, timeClaims[0].To)
	assert.Equal(t, *timeClaims[0].From, *timeClaims[0].Range.GreaterThanOrEqual) //nolint:testifylint
	assert.Equal(t, *timeClaims[0].To, *timeClaims[0].Range.LessThan)             //nolint:testifylint
}

// TestConvertTimeIntervalAppliesToPrecision verifies that the indexed range
// upper bound and scalar To are extended to the end of the to-precision window
// (e.g. to="2024" with year precision becomes 2025-01-01).
func TestConvertTimeIntervalAppliesToPrecision(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Time Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	fromTS := document.Time("2024-01-15")
	toTS := document.Time("2024-12-31")
	fromPrec := document.TimePrecisionDay
	toPrec := document.TimePrecisionDay
	claim := &document.TimeIntervalClaim{ //nolint:exhaustruct
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: testPropID},
		From:          &fromTS,
		FromPrecision: &fromPrec,
		To:            &toTS,
		ToPrecision:   &toPrec,
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	timeClaims, unknownClaims, _, errE := c.convertTimeInterval(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, timeClaims, 1)
	assert.Empty(t, unknownClaims)
	require.NotNil(t, timeClaims[0].From)
	require.NotNil(t, timeClaims[0].To)
	require.NotNil(t, timeClaims[0].Range.GreaterThanOrEqual)
	require.NotNil(t, timeClaims[0].Range.LessThan)

	fromTime := x.TimeFromFloat64(*timeClaims[0].From).UTC()
	assert.Equal(t, time.Date(2024, time.January, 15, 0, 0, 0, 0, time.UTC), fromTime)
	// To should be start of the day AFTER 2024-12-31, i.e. 2025-01-01.
	toTime := x.TimeFromFloat64(*timeClaims[0].To).UTC()
	assert.Equal(t, time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC), toTime)
	assert.Equal(t, *timeClaims[0].From, *timeClaims[0].Range.GreaterThanOrEqual) //nolint:testifylint
	assert.Equal(t, *timeClaims[0].To, *timeClaims[0].Range.LessThan)             //nolint:testifylint
	assert.Equal(t, "2024-01-15", timeClaims[0].FromDisplay)
	assert.Equal(t, "2024-12-31", timeClaims[0].ToDisplay)
}

// TestConvertTimeIntervalCoarseTo verifies that a precision-mismatched
// interval (fine-precision From, coarse-precision To) is not swapped:
// the to-precision window extends past From, so the range is
// well-formed and no swap occurs.
func TestConvertTimeIntervalCoarseTo(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Time Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	fromTS := document.Time("2025-10-21")
	toTS := document.Time("2025")
	fromPrec := document.TimePrecisionDay
	toPrec := document.TimePrecisionYear
	claim := &document.TimeIntervalClaim{ //nolint:exhaustruct
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: testPropID},
		From:          &fromTS,
		FromPrecision: &fromPrec,
		To:            &toTS,
		ToPrecision:   &toPrec,
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	timeClaims, unknownClaims, _, errE := c.convertTimeInterval(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, timeClaims, 1)
	assert.Empty(t, unknownClaims)
	require.NotNil(t, timeClaims[0].From)
	require.NotNil(t, timeClaims[0].To)
	require.NotNil(t, timeClaims[0].Range.GreaterThanOrEqual)
	require.NotNil(t, timeClaims[0].Range.LessThan)

	fromTime := x.TimeFromFloat64(*timeClaims[0].From).UTC()
	toTime := x.TimeFromFloat64(*timeClaims[0].To).UTC()
	assert.Equal(t, time.Date(2025, time.October, 21, 0, 0, 0, 0, time.UTC), fromTime)
	// To with year precision extends to start of 2026.
	assert.Equal(t, time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC), toTime)
	assert.Equal(t, *timeClaims[0].From, *timeClaims[0].Range.GreaterThanOrEqual) //nolint:testifylint
	assert.Equal(t, *timeClaims[0].To, *timeClaims[0].Range.LessThan)             //nolint:testifylint
	// Display strings keep the original user-visible form (no swap).
	assert.Equal(t, "2025-10-21", timeClaims[0].FromDisplay)
	assert.Equal(t, "2025", timeClaims[0].ToDisplay)
}

// TestConvertTimeIntervalToIsOpenExcludesWindow verifies that ToIsOpen=true
// pulls the upper bound back to to_start (excluding the to-window) while the
// default (ToIsOpen=false) extends it to to_end (including the to-window).
func TestConvertTimeIntervalToIsOpenExcludesWindow(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Time Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	fromTS := document.Time("2024")
	toTS := document.Time("2025")
	fromPrec := document.TimePrecisionYear
	toPrec := document.TimePrecisionYear
	mkClaim := func(toIsOpen bool) *document.TimeIntervalClaim {
		return &document.TimeIntervalClaim{ //nolint:exhaustruct
			CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
			Prop:          document.Reference{ID: testPropID},
			From:          &fromTS,
			FromPrecision: &fromPrec,
			To:            &toTS,
			ToPrecision:   &toPrec,
			ToIsOpen:      toIsOpen,
		}
	}

	defaultClaim := mkClaim(false)
	errE := defaultClaim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	defaultClaims, _, _, errE := c.convertTimeInterval(ctx, defaultClaim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, defaultClaims, 1)

	openClaim := mkClaim(true)
	errE = openClaim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	openClaims, _, _, errE := c.convertTimeInterval(ctx, openClaim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, openClaims, 1)

	// Default: upper = to_end = 2026-01-01 (window included).
	require.NotNil(t, defaultClaims[0].Range.LessThan)
	defaultUpper := x.TimeFromFloat64(*defaultClaims[0].Range.LessThan).UTC()
	assert.Equal(t, time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC), defaultUpper)

	// ToIsOpen=true: upper = to_start = 2025-01-01 (window excluded).
	require.NotNil(t, openClaims[0].Range.LessThan)
	openUpper := x.TimeFromFloat64(*openClaims[0].Range.LessThan).UTC()
	assert.Equal(t, time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC), openUpper)

	// Scalars coincide with range bounds.
	require.NotNil(t, defaultClaims[0].To)
	require.NotNil(t, openClaims[0].To)
	assert.Equal(t, *defaultClaims[0].To, *defaultClaims[0].Range.LessThan) //nolint:testifylint
	assert.Equal(t, *openClaims[0].To, *openClaims[0].Range.LessThan)       //nolint:testifylint
}

// TestConvertTimeIntervalCoarseFromOpen verifies that FromIsOpen=true with a
// coarse-precision From advances both the scalar from and the range lower
// to the end of the from-window.
func TestConvertTimeIntervalCoarseFromOpen(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Time Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	fromTS := document.Time("2024")
	toTS := document.Time("2026-06-15")
	fromPrec := document.TimePrecisionYear
	toPrec := document.TimePrecisionDay
	claim := &document.TimeIntervalClaim{ //nolint:exhaustruct
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: testPropID},
		From:          &fromTS,
		FromPrecision: &fromPrec,
		FromIsOpen:    true,
		To:            &toTS,
		ToPrecision:   &toPrec,
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	timeClaims, unknownClaims, _, errE := c.convertTimeInterval(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, timeClaims, 1)
	assert.Empty(t, unknownClaims)
	require.NotNil(t, timeClaims[0].Range.GreaterThanOrEqual)
	assert.Nil(t, timeClaims[0].Range.GreaterThan)
	require.NotNil(t, timeClaims[0].Range.LessThan)
	assert.Nil(t, timeClaims[0].Range.LessThanOrEqual)

	// from = 2024 (year), open: lower advances past the entire year to 2025-01-01.
	fromTime := x.TimeFromFloat64(*timeClaims[0].Range.GreaterThanOrEqual).UTC()
	assert.Equal(t, time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC), fromTime)

	// Scalars coincide with range bounds, displays preserve user input.
	require.NotNil(t, timeClaims[0].From)
	require.NotNil(t, timeClaims[0].To)
	assert.Equal(t, *timeClaims[0].From, *timeClaims[0].Range.GreaterThanOrEqual) //nolint:testifylint
	assert.Equal(t, *timeClaims[0].To, *timeClaims[0].Range.LessThan)             //nolint:testifylint
	assert.Equal(t, "2024", timeClaims[0].FromDisplay)
	assert.Equal(t, "2026-06-15", timeClaims[0].ToDisplay)
}

// TestConvertTimeIntervalSinglePointSamePrecision verifies that an interval
// with from == to (same time, same precision) produces an identical
// indexed TimeClaim to what convertTime produces for the same single point.
func TestConvertTimeIntervalSinglePointSamePrecision(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Time Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	ts := document.Time("2025")
	prec := document.TimePrecisionYear
	core := makeCoreClaim(document.HighConfidence, nil)
	prop := document.Reference{ID: testPropID}

	intervalClaim := &document.TimeIntervalClaim{ //nolint:exhaustruct
		CoreClaim:     core,
		Prop:          prop,
		From:          &ts,
		FromPrecision: &prec,
		To:            &ts,
		ToPrecision:   &prec,
	}
	errE := intervalClaim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	intervalClaims, unknownClaims, _, errE := c.convertTimeInterval(ctx, intervalClaim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, intervalClaims, 1)
	assert.Empty(t, unknownClaims)

	pointClaim := &document.TimeClaim{
		CoreClaim: core,
		Prop:      prop,
		Time:      ts,
		Precision: prec,
	}
	errE = pointClaim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	pointClaims, errE := c.convertTime(ctx, pointClaim)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Interval path with from == to must produce the same TimeClaim as the
	// single-point path.
	assert.Equal(t, pointClaims, intervalClaims)

	// Explicit value checks of the resulting bounds.
	require.NotNil(t, intervalClaims[0].From)
	require.NotNil(t, intervalClaims[0].To)
	require.NotNil(t, intervalClaims[0].Range.GreaterThanOrEqual)
	require.NotNil(t, intervalClaims[0].Range.LessThan)

	fromTime := x.TimeFromFloat64(*intervalClaims[0].From).UTC()
	toTime := x.TimeFromFloat64(*intervalClaims[0].To).UTC()
	assert.Equal(t, time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC), fromTime)
	assert.Equal(t, time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC), toTime)
	assert.Equal(t, *intervalClaims[0].From, *intervalClaims[0].Range.GreaterThanOrEqual) //nolint:testifylint
	assert.Equal(t, *intervalClaims[0].To, *intervalClaims[0].Range.LessThan)             //nolint:testifylint
	assert.Equal(t, "2025", intervalClaims[0].FromDisplay)
	assert.Equal(t, "2025", intervalClaims[0].ToDisplay)
}

// TestConvertTimeIntervalSinglePointToFinerPrecision verifies that when
// from and to share the same start instant but to has finer precision
// (e.g. from="2025" year, to="2025-01" month), the result reflects the
// to-window: range = [from_start, to_end), with each side's display
// preserved.
func TestConvertTimeIntervalSinglePointToFinerPrecision(t *testing.T) { //nolint:dupl
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Time Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	fromTS := document.Time("2025")
	toTS := document.Time("2025-01-00")
	fromPrec := document.TimePrecisionYear
	toPrec := document.TimePrecisionMonth
	claim := &document.TimeIntervalClaim{ //nolint:exhaustruct
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: testPropID},
		From:          &fromTS,
		FromPrecision: &fromPrec,
		To:            &toTS,
		ToPrecision:   &toPrec,
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	timeClaims, unknownClaims, _, errE := c.convertTimeInterval(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, timeClaims, 1)
	assert.Empty(t, unknownClaims)

	fromTime := x.TimeFromFloat64(*timeClaims[0].From).UTC()
	toTime := x.TimeFromFloat64(*timeClaims[0].To).UTC()
	// from_start = 2025-01-01, to_end = 2025-02-01 (one month past to_start).
	assert.Equal(t, time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC), fromTime)
	assert.Equal(t, time.Date(2025, time.February, 1, 0, 0, 0, 0, time.UTC), toTime)
	assert.Equal(t, *timeClaims[0].From, *timeClaims[0].Range.GreaterThanOrEqual) //nolint:testifylint
	assert.Equal(t, *timeClaims[0].To, *timeClaims[0].Range.LessThan)             //nolint:testifylint
	assert.Equal(t, "2025", timeClaims[0].FromDisplay)
	assert.Equal(t, "2025-01-00", timeClaims[0].ToDisplay)
}

// TestConvertTimeIntervalSinglePointToCoarserPrecision verifies that when
// from and to share the same start instant but to has coarser precision
// (e.g. from="2025-01-01" day, to="2025" year), the result reflects the
// wider to-window: range = [from_start, to_end).
func TestConvertTimeIntervalSinglePointToCoarserPrecision(t *testing.T) { //nolint:dupl
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Time Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	fromTS := document.Time("2025-01-01")
	toTS := document.Time("2025")
	fromPrec := document.TimePrecisionDay
	toPrec := document.TimePrecisionYear
	claim := &document.TimeIntervalClaim{ //nolint:exhaustruct
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: testPropID},
		From:          &fromTS,
		FromPrecision: &fromPrec,
		To:            &toTS,
		ToPrecision:   &toPrec,
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	timeClaims, unknownClaims, _, errE := c.convertTimeInterval(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, timeClaims, 1)
	assert.Empty(t, unknownClaims)

	fromTime := x.TimeFromFloat64(*timeClaims[0].From).UTC()
	toTime := x.TimeFromFloat64(*timeClaims[0].To).UTC()
	// from_start = 2025-01-01, to_end = 2026-01-01 (one year past to_start).
	assert.Equal(t, time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC), fromTime)
	assert.Equal(t, time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC), toTime)
	assert.Equal(t, *timeClaims[0].From, *timeClaims[0].Range.GreaterThanOrEqual) //nolint:testifylint
	assert.Equal(t, *timeClaims[0].To, *timeClaims[0].Range.LessThan)             //nolint:testifylint
	assert.Equal(t, "2025-01-01", timeClaims[0].FromDisplay)
	assert.Equal(t, "2025", timeClaims[0].ToDisplay)
}

func TestConvertTimeIntervalFromNone(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Time Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	toTS := document.Time("2024-12-31")
	toPrec := document.TimePrecisionDay
	claim := &document.TimeIntervalClaim{ //nolint:exhaustruct
		CoreClaim:   makeCoreClaim(document.HighConfidence, nil),
		Prop:        document.Reference{ID: testPropID},
		FromIsNone:  true,
		To:          &toTS,
		ToPrecision: &toPrec,
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	timeClaims, unknownClaims, _, errE := c.convertTimeInterval(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, timeClaims, 1)
	assert.Empty(t, unknownClaims)
	assert.Nil(t, timeClaims[0].From)
	assert.Equal(t, -math.MaxFloat64, *timeClaims[0].Range.GreaterThanOrEqual) //nolint:testifylint
}

func TestConvertTimeIntervalToNone(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Time Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	fromTS := document.Time("2024-01-01")
	fromPrec := document.TimePrecisionDay
	claim := &document.TimeIntervalClaim{ //nolint:exhaustruct
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: testPropID},
		From:          &fromTS,
		FromPrecision: &fromPrec,
		ToIsNone:      true,
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	timeClaims, unknownClaims, _, errE := c.convertTimeInterval(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, timeClaims, 1)
	assert.Empty(t, unknownClaims)
	assert.Nil(t, timeClaims[0].To)
	assert.Equal(t, math.MaxFloat64, *timeClaims[0].Range.LessThanOrEqual) //nolint:testifylint
}

func TestConvertTimeIntervalFromUnknownWithTo(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Time Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	toTS := document.Time("2024-06-15")
	toPrec := document.TimePrecisionDay
	claim := &document.TimeIntervalClaim{ //nolint:exhaustruct
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: testPropID},
		FromIsUnknown: true,
		To:            &toTS,
		ToPrecision:   &toPrec,
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	timeClaims, unknownClaims, _, errE := c.convertTimeInterval(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, timeClaims, 1)
	assert.Empty(t, unknownClaims)
}

func TestConvertTimeIntervalToUnknownWithFrom(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Time Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	fromTS := document.Time("2024-06-15")
	fromPrec := document.TimePrecisionDay
	claim := &document.TimeIntervalClaim{ //nolint:exhaustruct
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: testPropID},
		From:          &fromTS,
		FromPrecision: &fromPrec,
		ToIsUnknown:   true,
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	timeClaims, unknownClaims, _, errE := c.convertTimeInterval(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, timeClaims, 1)
	assert.Empty(t, unknownClaims)
}

func TestConvertTimeIntervalBothUnknown(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Time Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	claim := &document.TimeIntervalClaim{ //nolint:exhaustruct
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: testPropID},
		FromIsUnknown: true,
		ToIsUnknown:   true,
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	timeClaims, unknownClaims, _, errE := c.convertTimeInterval(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Empty(t, timeClaims)
	require.Len(t, unknownClaims, 1)
}

func TestConvertTimeIntervalFromNoneToUnknown(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Time Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	claim := &document.TimeIntervalClaim{ //nolint:exhaustruct
		CoreClaim:   makeCoreClaim(document.HighConfidence, nil),
		Prop:        document.Reference{ID: testPropID},
		FromIsNone:  true,
		ToIsUnknown: true,
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	// FromNone sets range gte, then ToUnknown with known From (but From is set through range) -
	// this hits the default case.
	timeClaims, unknownClaims, _, errE := c.convertTimeInterval(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Empty(t, timeClaims)
	require.Len(t, unknownClaims, 1)
}

func TestConvertTimeIntervalMissingFromPrecision(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	fromTS := document.Time("2024-01-01")
	toTS := document.Time("2024-12-31")
	toPrec := document.TimePrecisionDay
	claim := &document.TimeIntervalClaim{ //nolint:exhaustruct
		CoreClaim:   makeCoreClaim(document.HighConfidence, nil),
		Prop:        document.Reference{ID: testPropID},
		From:        &fromTS,
		To:          &toTS,
		ToPrecision: &toPrec,
	}
	// Intentionally NO claim.Validate() call here - claim is deliberately
	// invalid and would not pass Validate().
	_, _, _, errE := c.convertTimeInterval(ctx, claim) //nolint:dogsled
	assert.EqualError(t, errE, "missing from precision in claim")
}

func TestConvertTimeIntervalMissingToPrecision(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Time Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	fromTS := document.Time("2024-01-01")
	fromPrec := document.TimePrecisionDay
	toTS := document.Time("2024-12-31")
	claim := &document.TimeIntervalClaim{ //nolint:exhaustruct
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: testPropID},
		From:          &fromTS,
		FromPrecision: &fromPrec,
		To:            &toTS,
	}
	// Intentionally NO claim.Validate() call here - claim is deliberately
	// invalid and would not pass Validate().
	_, _, _, errE := c.convertTimeInterval(ctx, claim) //nolint:dogsled
	assert.EqualError(t, errE, "missing to precision in claim")
}

func TestConvertTimeIntervalFromUnknownMissingToPrecision(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	toTS := document.Time("2024-12-31")
	claim := &document.TimeIntervalClaim{ //nolint:exhaustruct
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: testPropID},
		FromIsUnknown: true,
		To:            &toTS,
	}
	// Intentionally NO claim.Validate() call here - claim is deliberately
	// invalid and would not pass Validate().
	_, _, _, errE := c.convertTimeInterval(ctx, claim) //nolint:dogsled
	assert.EqualError(t, errE, "missing to precision in claim")
}

func TestConvertTimeIntervalToUnknownMissingFromPrecision(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	fromTS := document.Time("2024-01-01")
	claim := &document.TimeIntervalClaim{ //nolint:exhaustruct
		CoreClaim:   makeCoreClaim(document.HighConfidence, nil),
		Prop:        document.Reference{ID: testPropID},
		From:        &fromTS,
		ToIsUnknown: true,
	}
	// Intentionally NO claim.Validate() call here - claim is deliberately
	// invalid and would not pass Validate().
	_, _, _, errE := c.convertTimeInterval(ctx, claim) //nolint:dogsled
	assert.EqualError(t, errE, "missing from precision in claim")
}

func TestConvertRelation(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Rel Prop")
	targetDoc := makeNamingDoc(testTargetDocID, "Target")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID:      propDoc,
		testTargetDocID: targetDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	claim := &document.ReferenceClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: testPropID},
		To:        document.Reference{ID: testTargetDocID},
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	result, _, errE := c.convertReference(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, result, 1)
	assert.Equal(t, testPropID, result[0].Prop)
	assert.Equal(t, testTargetDocID, result[0].To)
	assert.Equal(t, "Target", result[0].ToDisplay["und"])
}

func TestConvertRelationWithClassAncestors(t *testing.T) {
	t.Parallel()

	// Set up hierarchy properties so SUBCLASS_OF is discovered.
	subentityDoc := makePropertyDoc(internalCore.SubentityOfPropID, nil)
	subclassDoc := makePropertyDoc(internalCore.SubclassOfPropID, &internalCore.SubentityOfPropID)
	subpropDoc := makePropertyDoc(internalCore.SubpropertyOfPropID, &internalCore.SubentityOfPropID)
	instanceDoc := makePropertyDoc(internalCore.InstanceOfPropID, &internalCore.SubentityOfPropID)
	properties := []*document.D{subentityDoc, subclassDoc, subpropDoc, instanceDoc}

	propDoc := makeNamingDoc(testPropID, "Rel Prop")
	// Target has SUBCLASS_OF -> parent.
	targetDoc := makeHierarchyDoc(testTargetDocID, "Target", internalCore.SubclassOfPropID, &testParentClass)
	parentDoc := makeNamingDoc(testParentClass, "Parent Class")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID:      propDoc,
		testTargetDocID: targetDoc,
		testParentClass: parentDoc,
	}
	c := newTestConverter(t, properties, nil, extraDocs)

	ctx := t.Context()
	claim := &document.ReferenceClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: testPropID},
		To:        document.Reference{ID: testTargetDocID},
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	result, _, errE := c.convertReference(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	// Should produce claims for both target and parent class.
	require.Len(t, result, 2)

	// Find the claim for the target and the claim for the parent.
	var targetClaim, parentClaim ReferenceClaim
	for _, r := range result {
		if r.To == testTargetDocID {
			targetClaim = r
		} else {
			parentClaim = r
		}
	}
	assert.Equal(t, testTargetDocID, targetClaim.To)
	assert.Equal(t, testParentClass, parentClaim.To)

	// Target claim should have a hierarchy path: <SUBCLASS_OF>:<parent>/<target>.
	require.Len(t, targetClaim.ToPath, 1)
	assert.Equal(t, internalCore.SubclassOfPropID.String()+":"+testParentClass.String()+"/"+testTargetDocID.String(), targetClaim.ToPath[0])
	require.Len(t, targetClaim.ToDisplayPath["und"], 1)
	assert.Equal(t, "Parent Class\x00Target", targetClaim.ToDisplayPath["und"][0])

	// Parent class claim has no hierarchy path (it's a root).
	assert.Empty(t, parentClaim.ToPath)
	assert.Empty(t, parentClaim.ToDisplayPath)
}

func TestConvertRelationWithClassSelfCycle(t *testing.T) {
	t.Parallel()

	subentityDoc := makePropertyDoc(internalCore.SubentityOfPropID, nil)
	subclassDoc := makePropertyDoc(internalCore.SubclassOfPropID, &internalCore.SubentityOfPropID)
	subpropDoc := makePropertyDoc(internalCore.SubpropertyOfPropID, &internalCore.SubentityOfPropID)
	instanceDoc := makePropertyDoc(internalCore.InstanceOfPropID, &internalCore.SubentityOfPropID)
	properties := []*document.D{subentityDoc, subclassDoc, subpropDoc, instanceDoc}

	propDoc := makeNamingDoc(testPropID, "Rel Prop")
	// Target has SUBCLASS_OF pointing to itself.
	targetDoc := makeHierarchyDoc(testTargetDocID, "Target", internalCore.SubclassOfPropID, &testTargetDocID)
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID:      propDoc,
		testTargetDocID: targetDoc,
	}
	c := newTestConverter(t, properties, nil, extraDocs)

	ctx := t.Context()
	claim := &document.ReferenceClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: testPropID},
		To:        document.Reference{ID: testTargetDocID},
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	// Self-reference excluded, so only one result claim.
	result, _, errE := c.convertReference(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, result, 1)
	assert.Equal(t, testTargetDocID, result[0].To)
}

func TestConvertRelationWithClassMutualCycle(t *testing.T) {
	t.Parallel()

	subentityDoc := makePropertyDoc(internalCore.SubentityOfPropID, nil)
	subclassDoc := makePropertyDoc(internalCore.SubclassOfPropID, &internalCore.SubentityOfPropID)
	subpropDoc := makePropertyDoc(internalCore.SubpropertyOfPropID, &internalCore.SubentityOfPropID)
	instanceDoc := makePropertyDoc(internalCore.InstanceOfPropID, &internalCore.SubentityOfPropID)
	properties := []*document.D{subentityDoc, subclassDoc, subpropDoc, instanceDoc}

	classA := identifier.New()
	classB := identifier.New()
	// A has SUBCLASS_OF -> B, B has SUBCLASS_OF -> A.
	aDoc := makeHierarchyDoc(classA, "Class A", internalCore.SubclassOfPropID, &classB)
	bDoc := makeHierarchyDoc(classB, "Class B", internalCore.SubclassOfPropID, &classA)

	propDoc := makeNamingDoc(testPropID, "Rel Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
		classA:     aDoc,
		classB:     bDoc,
	}
	c := newTestConverter(t, properties, nil, extraDocs)

	ctx := t.Context()
	claim := &document.ReferenceClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: testPropID},
		To:        document.Reference{ID: classA},
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	result, _, errE := c.convertReference(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	// Target classA + ancestor classB, no duplicates.
	require.Len(t, result, 2)
	toIDs := make([]identifier.Identifier, 0, len(result))
	for _, r := range result {
		toIDs = append(toIDs, r.To)
	}
	assert.Contains(t, toIDs, classA)
	assert.Contains(t, toIDs, classB)
}

func TestConvertRelationWithPropertySelfCycle(t *testing.T) {
	t.Parallel()

	// Property that is a subproperty of itself.
	propA := identifier.New()
	propADoc := makePropertyDoc(propA, &propA)

	propNaming := makeNamingDoc(propA, "Prop A")
	targetDoc := makeNamingDoc(testTargetDocID, "Target")
	extraDocs := map[identifier.Identifier]*document.D{
		propA:           propNaming,
		testTargetDocID: targetDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)
	c.buildPropertyHierarchy([]*document.D{propADoc})

	ctx := t.Context()
	claim := &document.ReferenceClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: propA},
		To:        document.Reference{ID: testTargetDocID},
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	result, _, errE := c.convertReference(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	// Self-cycle excluded: only one claim with propA, no duplicate.
	require.Len(t, result, 1)
	assert.Equal(t, propA, result[0].Prop)
	assert.Equal(t, testTargetDocID, result[0].To)
}

func TestConvertRelationWithPropertyMutualCycle(t *testing.T) {
	t.Parallel()

	// Two properties in a mutual cycle.
	propA := identifier.New()
	propB := identifier.New()
	propADoc := makePropertyDoc(propA, &propB)
	propBDoc := makePropertyDoc(propB, &propA)

	propANaming := makeNamingDoc(propA, "Prop A")
	propBNaming := makeNamingDoc(propB, "Prop B")
	targetDoc := makeNamingDoc(testTargetDocID, "Target")
	extraDocs := map[identifier.Identifier]*document.D{
		propA:           propANaming,
		propB:           propBNaming,
		testTargetDocID: targetDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)
	c.IndexAncestorProperties = true
	c.buildPropertyHierarchy([]*document.D{propADoc, propBDoc})

	ctx := t.Context()
	claim := &document.ReferenceClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: propA},
		To:        document.Reference{ID: testTargetDocID},
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	result, _, errE := c.convertReference(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	// propA (direct) + propB (ancestor), no duplicates.
	require.Len(t, result, 2)
	propIDs := make([]identifier.Identifier, 0, len(result))
	for _, r := range result {
		propIDs = append(propIDs, r.Prop)
	}
	assert.Contains(t, propIDs, propA)
	assert.Contains(t, propIDs, propB)
}

func TestConvertRelationWithSubRelations(t *testing.T) {
	t.Parallel()

	subPropID := identifier.New()
	subTargetID := identifier.New()
	propDoc := makeNamingDoc(testPropID, "Rel Prop")
	targetDoc := makeNamingDoc(testTargetDocID, "Target")
	subPropDoc := makeNamingDoc(subPropID, "Sub Prop")
	subTargetDoc := makeNamingDoc(subTargetID, "Sub Target")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID:      propDoc,
		testTargetDocID: targetDoc,
		subPropID:       subPropDoc,
		subTargetID:     subTargetDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	sub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: subPropID},
				To:        document.Reference{ID: subTargetID},
			},
		},
	}
	claim := &document.ReferenceClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, sub),
		Prop:      document.Reference{ID: testPropID},
		To:        document.Reference{ID: testTargetDocID},
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	result, subs, errE := c.convertReference(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, result, 1)
	require.Len(t, subs.Refs, 1)
	assert.Equal(t, subPropID, subs.Refs[0].Prop)
	assert.Equal(t, subTargetID, subs.Refs[0].To)
}

func TestConvertHas(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Has Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	claim := &document.HasClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: testPropID},
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	result, _, errE := c.convertHas(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, result, 1)
	assert.Equal(t, testPropID, result[0].Prop)
}

func TestConvertHasWithSubRelations(t *testing.T) {
	t.Parallel()

	subPropID := identifier.New()
	subTargetID := identifier.New()
	propDoc := makeNamingDoc(testPropID, "Has Prop")
	subPropDoc := makeNamingDoc(subPropID, "Sub Prop")
	subTargetDoc := makeNamingDoc(subTargetID, "Sub Target")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID:  propDoc,
		subPropID:   subPropDoc,
		subTargetID: subTargetDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	sub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: subPropID},
				To:        document.Reference{ID: subTargetID},
			},
		},
	}
	claim := &document.HasClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, sub),
		Prop:      document.Reference{ID: testPropID},
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	result, subs, errE := c.convertHas(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	// Has claims with sub-ref claims are not indexed as HasClaim entries.
	// They are recorded only in subs.Refs.
	require.Empty(t, result)
	require.Len(t, subs.Refs, 1)
}

func TestConvertNone(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "None Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	claim := &document.NoneClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: testPropID},
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	result, _, errE := c.convertNone(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, result, 1)
	assert.Equal(t, testPropID, result[0].Prop)
}

func TestConvertUnknown(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Unknown Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	claim := &document.UnknownClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: testPropID},
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	result, _, errE := c.convertUnknown(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, result, 1)
	assert.Equal(t, testPropID, result[0].Prop)
}

// TestConvertReferenceSubClaimFields verifies that sub-reference claims generated from a
// reference parent claim have ParentProp and ParentTo set to the parent's prop and target IDs,
// and that inner prop/to/display/naming fields are all populated.
func TestConvertReferenceSubClaimFields(t *testing.T) {
	t.Parallel()

	subPropID := identifier.New()
	subTargetID := identifier.New()
	propDoc := makeNamingDoc(testPropID, "Rel Prop")
	targetDoc := makeNamingDoc(testTargetDocID, "Target")
	subPropDoc := makeNamingDoc(subPropID, "Sub Prop")
	subTargetDoc := makeNamingDoc(subTargetID, "Sub Target")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID:      propDoc,
		testTargetDocID: targetDoc,
		subPropID:       subPropDoc,
		subTargetID:     subTargetDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	sub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: subPropID},
				To:        document.Reference{ID: subTargetID},
			},
		},
	}
	claim := &document.ReferenceClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, sub),
		Prop:      document.Reference{ID: testPropID},
		To:        document.Reference{ID: testTargetDocID},
	}
	_, subs, errE := c.convertReference(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)

	require.Len(t, subs.Refs, 1)
	sr := subs.Refs[0]
	assert.Equal(t, testPropID, sr.ParentProp)
	assert.Equal(t, testTargetDocID.String(), sr.ParentTo)
	assert.Equal(t, subPropID, sr.Prop)
	assert.Equal(t, subTargetID, sr.To)
	assert.NotEmpty(t, sr.PropDisplay)
	assert.NotEmpty(t, sr.ToDisplay)
}

// TestConvertReferenceMultipleSubClaims verifies that multiple reference sub-claims
// each produce a separate SubRefClaim entry.
func TestConvertReferenceMultipleSubClaims(t *testing.T) {
	t.Parallel()

	subPropID1 := identifier.New()
	subTargetID1 := identifier.New()
	subPropID2 := identifier.New()
	subTargetID2 := identifier.New()
	propDoc := makeNamingDoc(testPropID, "Rel Prop")
	targetDoc := makeNamingDoc(testTargetDocID, "Target")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID:      propDoc,
		testTargetDocID: targetDoc,
		subPropID1:      makeNamingDoc(subPropID1, "Sub Prop 1"),
		subTargetID1:    makeNamingDoc(subTargetID1, "Sub Target 1"),
		subPropID2:      makeNamingDoc(subPropID2, "Sub Prop 2"),
		subTargetID2:    makeNamingDoc(subTargetID2, "Sub Target 2"),
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	sub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: subPropID1},
				To:        document.Reference{ID: subTargetID1},
			},
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: subPropID2},
				To:        document.Reference{ID: subTargetID2},
			},
		},
	}
	claim := &document.ReferenceClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, sub),
		Prop:      document.Reference{ID: testPropID},
		To:        document.Reference{ID: testTargetDocID},
	}
	_, subs, errE := c.convertReference(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)

	require.Len(t, subs.Refs, 2)
	// All subs.Refs have the same parent.
	for _, sr := range subs.Refs {
		assert.Equal(t, testPropID, sr.ParentProp)
		assert.Equal(t, testTargetDocID.String(), sr.ParentTo)
	}
	// Collect the inner props to verify both are present.
	innerProps := []identifier.Identifier{subs.Refs[0].Prop, subs.Refs[1].Prop}
	assert.Contains(t, innerProps, subPropID1)
	assert.Contains(t, innerProps, subPropID2)
}

// TestConvertReferenceSubClaimHierarchyExpansion verifies that when the parent reference claim's
// target has hierarchy ancestors, a SubRefClaim is generated for each (expanded target × sub-ref)
// combination, with ParentTo set to each expanded target ID.
func TestConvertReferenceSubClaimHierarchyExpansion(t *testing.T) {
	t.Parallel()

	// Set up hierarchy property chain so SUBCLASS_OF is discovered as a value hierarchy property.
	subentityDoc := makePropertyDoc(internalCore.SubentityOfPropID, nil)
	subclassDoc := makePropertyDoc(internalCore.SubclassOfPropID, &internalCore.SubentityOfPropID)
	subpropDoc := makePropertyDoc(internalCore.SubpropertyOfPropID, &internalCore.SubentityOfPropID)
	instanceDoc := makePropertyDoc(internalCore.InstanceOfPropID, &internalCore.SubentityOfPropID)
	properties := []*document.D{subentityDoc, subclassDoc, subpropDoc, instanceDoc}

	classParent := identifier.New()
	target := identifier.New()
	subPropID := identifier.New()
	subTargetID := identifier.New()

	// Target has SUBCLASS_OF -> classParent.
	targetClaims := &document.ClaimTypes{}
	targetClaims.String = append(targetClaims.String, document.StringClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: internalCore.NamingPropID},
		String:    "Target",
	})
	targetClaims.Reference = append(targetClaims.Reference, document.ReferenceClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: internalCore.SubclassOfPropID},
		To:        document.Reference{ID: classParent},
	})
	targetDoc := &document.D{
		CoreDocument: document.CoreDocument{ID: target}, //nolint:exhaustruct
		Claims:       targetClaims,
	}

	propDoc := makeNamingDoc(testPropID, "Rel Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID:  propDoc,
		target:      targetDoc,
		classParent: makeNamingDoc(classParent, "ClassParent"),
		subPropID:   makeNamingDoc(subPropID, "Sub Prop"),
		subTargetID: makeNamingDoc(subTargetID, "Sub Target"),
	}
	c := newTestConverter(t, properties, nil, extraDocs)

	ctx := t.Context()
	sub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: subPropID},
				To:        document.Reference{ID: subTargetID},
			},
		},
	}
	claim := &document.ReferenceClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, sub),
		Prop:      document.Reference{ID: testPropID},
		To:        document.Reference{ID: target},
	}
	_, subs, errE := c.convertReference(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)

	// 1 sub-ref × 2 expanded targets (target + classParent) = 2 SubRefClaim entries.
	require.Len(t, subs.Refs, 2)
	parentTos := []string{subs.Refs[0].ParentTo, subs.Refs[1].ParentTo}
	assert.Contains(t, parentTos, target.String())
	assert.Contains(t, parentTos, classParent.String())
	// All subs.Refs share the same inner prop and to.
	for _, sr := range subs.Refs {
		assert.Equal(t, testPropID, sr.ParentProp)
		assert.Equal(t, subPropID, sr.Prop)
		assert.Equal(t, subTargetID, sr.To)
	}
}

// TestConvertHasSubClaimParentToSentinel verifies that has claims with reference sub-claims
// produce SubRefClaim entries with ParentTo set to the ParentToHas sentinel.
func TestConvertHasSubClaimParentToSentinel(t *testing.T) {
	t.Parallel()

	subPropID := identifier.New()
	subTargetID := identifier.New()
	propDoc := makeNamingDoc(testPropID, "Has Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID:  propDoc,
		subPropID:   makeNamingDoc(subPropID, "Sub Prop"),
		subTargetID: makeNamingDoc(subTargetID, "Sub Target"),
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	sub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: subPropID},
				To:        document.Reference{ID: subTargetID},
			},
		},
	}
	claim := &document.HasClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, sub),
		Prop:      document.Reference{ID: testPropID},
	}
	result, subs, errE := c.convertHas(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Has claim with sub-claims is not indexed as HasClaim.
	assert.Empty(t, result)
	require.Len(t, subs.Refs, 1)
	assert.Equal(t, testPropID, subs.Refs[0].ParentProp)
	assert.Equal(t, ParentToHas, subs.Refs[0].ParentTo)
	assert.Equal(t, subPropID, subs.Refs[0].Prop)
	assert.Equal(t, subTargetID, subs.Refs[0].To)
}

// TestConvertHasSkippedForNonRefSubClaim verifies that a has claim with any sub-claim
// (not necessarily a reference sub-claim) is still not indexed as a HasClaim entry.
func TestConvertHasSkippedForNonRefSubClaim(t *testing.T) {
	t.Parallel()

	nestedHasPropID := identifier.New()
	propDoc := makeNamingDoc(testPropID, "Has Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID:      propDoc,
		nestedHasPropID: makeNamingDoc(nestedHasPropID, "Nested Has Prop"),
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	// Sub-claims contain only a has claim, not a reference claim.
	sub := &document.ClaimTypes{
		Has: []document.HasClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: nestedHasPropID},
			},
		},
	}
	claim := &document.HasClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, sub),
		Prop:      document.Reference{ID: testPropID},
	}
	result, subs, errE := c.convertHas(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Has claim with ANY sub-claim (even non-ref) is not indexed.
	assert.Empty(t, result)
	// No reference sub-claims, so no SubRefClaim entries either.
	assert.Empty(t, subs.Refs)
}

// TestConvertNoneSubClaimParentToSentinel verifies that none claims with reference sub-claims
// produce SubRefClaim entries with ParentTo set to the ParentToNone sentinel.
//
//nolint:dupl
func TestConvertNoneSubClaimParentToSentinel(t *testing.T) {
	t.Parallel()

	subPropID := identifier.New()
	subTargetID := identifier.New()
	propDoc := makeNamingDoc(testPropID, "None Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID:  propDoc,
		subPropID:   makeNamingDoc(subPropID, "Sub Prop"),
		subTargetID: makeNamingDoc(subTargetID, "Sub Target"),
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	sub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: subPropID},
				To:        document.Reference{ID: subTargetID},
			},
		},
	}
	claim := &document.NoneClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, sub),
		Prop:      document.Reference{ID: testPropID},
	}
	result, subs, errE := c.convertNone(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Unlike has claims, none claims are still indexed even when they have sub-claims.
	require.Len(t, result, 1)
	require.Len(t, subs.Refs, 1)
	assert.Equal(t, testPropID, subs.Refs[0].ParentProp)
	assert.Equal(t, ParentToNone, subs.Refs[0].ParentTo)
	assert.Equal(t, subPropID, subs.Refs[0].Prop)
	assert.Equal(t, subTargetID, subs.Refs[0].To)
}

// TestConvertUnknownSubClaimParentToSentinel verifies that unknown claims with reference sub-claims
// produce SubRefClaim entries with ParentTo set to the ParentToUnknown sentinel.
//
//nolint:dupl
func TestConvertUnknownSubClaimParentToSentinel(t *testing.T) {
	t.Parallel()

	subPropID := identifier.New()
	subTargetID := identifier.New()
	propDoc := makeNamingDoc(testPropID, "Unknown Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID:  propDoc,
		subPropID:   makeNamingDoc(subPropID, "Sub Prop"),
		subTargetID: makeNamingDoc(subTargetID, "Sub Target"),
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	sub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: subPropID},
				To:        document.Reference{ID: subTargetID},
			},
		},
	}
	claim := &document.UnknownClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, sub),
		Prop:      document.Reference{ID: testPropID},
	}
	result, subs, errE := c.convertUnknown(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Unlike has claims, unknown claims are still indexed even when they have sub-claims.
	require.Len(t, result, 1)
	require.Len(t, subs.Refs, 1)
	assert.Equal(t, testPropID, subs.Refs[0].ParentProp)
	assert.Equal(t, ParentToUnknown, subs.Refs[0].ParentTo)
	assert.Equal(t, subPropID, subs.Refs[0].Prop)
	assert.Equal(t, subTargetID, subs.Refs[0].To)
}

// TestConvertSubClaimsEmpty verifies that convert functions return no SubRefClaim entries
// when the parent claim has no reference sub-claims.
func TestConvertSubClaimsEmpty(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Prop")
	targetDoc := makeNamingDoc(testTargetDocID, "Target")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID:      propDoc,
		testTargetDocID: targetDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()

	t.Run("Reference", func(t *testing.T) {
		t.Parallel()
		claim := &document.ReferenceClaim{
			CoreClaim: makeCoreClaim(document.HighConfidence, nil),
			Prop:      document.Reference{ID: testPropID},
			To:        document.Reference{ID: testTargetDocID},
		}
		_, subs, errE := c.convertReference(ctx, claim)
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.Empty(t, subs.Refs)
	})

	t.Run("Has", func(t *testing.T) {
		t.Parallel()
		claim := &document.HasClaim{
			CoreClaim: makeCoreClaim(document.HighConfidence, nil),
			Prop:      document.Reference{ID: testPropID},
		}
		result, subs, errE := c.convertHas(ctx, claim)
		require.NoError(t, errE, "% -+#.1v", errE)
		// Has claim without sub-claims IS indexed.
		require.Len(t, result, 1)
		assert.Empty(t, subs.Refs)
	})

	t.Run("None", func(t *testing.T) {
		t.Parallel()
		claim := &document.NoneClaim{
			CoreClaim: makeCoreClaim(document.HighConfidence, nil),
			Prop:      document.Reference{ID: testPropID},
		}
		_, subs, errE := c.convertNone(ctx, claim)
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.Empty(t, subs.Refs)
	})

	t.Run("Unknown", func(t *testing.T) {
		t.Parallel()
		claim := &document.UnknownClaim{
			CoreClaim: makeCoreClaim(document.HighConfidence, nil),
			Prop:      document.Reference{ID: testPropID},
		}
		_, subs, errE := c.convertUnknown(ctx, claim)
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.Empty(t, subs.Refs)
	})
}

func TestFromDocument(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "My Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			Identifier: []document.IdentifierClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: testPropID},
					Value:     "Q42",
				},
			},
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: testPropID},
					String:    "hello",
				},
			},
		},
	}

	result, errE := c.FromDocument(ctx, doc, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, testDocID, result.ID)
	assert.ElementsMatch(t, []string{testDocID.String(), "Q42", "hello"}, result.Text["und"])
}

func TestFromDocumentNilClaims(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
	}

	result, errE := c.FromDocument(ctx, doc, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, testDocID, result.ID)
	// The document ID goes only to the "und" bucket, so that is the sole entry.
	assert.Equal(t, map[string][]string{"und": {testDocID.String()}}, result.Text)
}

func makeDocWithAllClaimTypes(t *testing.T, confidence document.Confidence) *document.D {
	t.Helper()

	fromAmount := document.Amount("5")
	toAmount := document.Amount("10")
	fromPrec := 1.0
	toPrec := 1.0
	fromTS := document.Time("2024-01-01")
	toTS := document.Time("2024-12-31")
	fromTSPrec := document.TimePrecisionDay
	toTSPrec := document.TimePrecisionDay

	return &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			Identifier: []document.IdentifierClaim{
				{
					CoreClaim: makeCoreClaim(confidence, nil),
					Prop:      document.Reference{ID: testPropID},
					Value:     "ID1",
				},
			},
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(confidence, nil),
					Prop:      document.Reference{ID: testPropID},
					String:    "str",
				},
			},
			HTML: []document.HTMLClaim{
				{
					CoreClaim: makeCoreClaim(confidence, nil),
					Prop:      document.Reference{ID: testPropID},
					HTML:      "<p>html</p>",
				},
			},
			Amount: []document.AmountClaim{
				{
					CoreClaim: makeCoreClaim(confidence, nil),
					Prop:      document.Reference{ID: testPropID},
					Amount:    document.Amount("42"),
					Precision: 1,
				},
			},
			AmountInterval: []document.AmountIntervalClaim{
				{ //nolint:exhaustruct
					CoreClaim:     makeCoreClaim(confidence, nil),
					Prop:          document.Reference{ID: testPropID},
					From:          &fromAmount,
					FromPrecision: &fromPrec,
					To:            &toAmount,
					ToPrecision:   &toPrec,
				},
			},
			Time: []document.TimeClaim{
				{
					CoreClaim: makeCoreClaim(confidence, nil),
					Prop:      document.Reference{ID: testPropID},
					Time:      document.Time("2024-06-15"),
					Precision: document.TimePrecisionDay,
				},
			},
			TimeInterval: []document.TimeIntervalClaim{
				{ //nolint:exhaustruct
					CoreClaim:     makeCoreClaim(confidence, nil),
					Prop:          document.Reference{ID: testPropID},
					From:          &fromTS,
					FromPrecision: &fromTSPrec,
					To:            &toTS,
					ToPrecision:   &toTSPrec,
				},
			},
			Link: []document.LinkClaim{
				{
					CoreClaim: makeCoreClaim(confidence, nil),
					Prop:      document.Reference{ID: testPropID},
					IRI:       "https://example.com",
				},
			},
			Reference: []document.ReferenceClaim{
				{
					CoreClaim: makeCoreClaim(confidence, nil),
					Prop:      document.Reference{ID: testPropID},
					To:        document.Reference{ID: testTargetDocID},
				},
			},
			Has: []document.HasClaim{
				{
					CoreClaim: makeCoreClaim(confidence, nil),
					Prop:      document.Reference{ID: testPropID},
				},
			},
			None: []document.NoneClaim{
				{
					CoreClaim: makeCoreClaim(confidence, nil),
					Prop:      document.Reference{ID: testPropID},
				},
			},
			Unknown: []document.UnknownClaim{
				{
					CoreClaim: makeCoreClaim(confidence, nil),
					Prop:      document.Reference{ID: testPropID},
				},
			},
		},
	}
}

func TestFromDocumentAllClaimTypesConfidence(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "My Prop")
	targetDoc := makeNamingDoc(testTargetDocID, "Target")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID:      propDoc,
		testTargetDocID: targetDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	tests := []struct {
		name       string
		confidence document.Confidence
		expected   int
	}{
		{"high confidence", document.HighConfidence, 1},
		{"low confidence included", document.LowConfidence, 1},
		{"below low confidence skipped", document.Confidence(0.3), 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()
			doc := makeDocWithAllClaimTypes(t, tt.confidence)

			result, errE := c.FromDocument(ctx, doc, nil)
			require.NoError(t, errE, "% -+#.1v", errE)
			assert.Equal(t, testDocID, result.ID)
			// text["und"] aggregates the following, after dedupeResult drops
			// duplicates. Property labels of value claims (PropDisplay/PropNaming)
			// are not folded; only referenced-document labels and numeric/temporal
			// bounds are. This doc's referenced docs carry only "und" labels, so
			// every value resolves to a single "und" string.
			//   1   - the document ID (folded into "und")
			// per included-claim group (expected = 1 unless skipped):
			//   4   - Identifier+String+HTML(stripped)+Link source claims
			//   Amount point (From == To dedup) + AmountInterval (From, To)  = 3
			//   Time   point (From == To dedup) + TimeInterval   (From, To)  = 3
			//   1 Ref  (ToDisplay == ToNaming dedup)                         = 1
			//   1 Has  (PropDisplay == PropNaming dedup)                     = 1
			//   None+Unknown contribute nothing (absence assertions).
			assert.Len(t, result.Text["und"], 1+12*tt.expected)
			// Amount + AmountInterval each contribute one claim.
			assert.Len(t, result.Claims.Amount, 2*tt.expected)
			// Time + TimeInterval each contribute one claim.
			assert.Len(t, result.Claims.Time, 2*tt.expected)
			assert.Len(t, result.Claims.Reference, tt.expected)
			assert.Len(t, result.Claims.Has, tt.expected)
			assert.Len(t, result.Claims.None, tt.expected)
			assert.Len(t, result.Claims.Unknown, tt.expected)
		})
	}
}

// TestDedupeResultDeMirrorsUnd verifies the final dedup pass drops a value from
// "und" when the same value is already indexed in a language-specific text
// bucket. The search query unions each language field with "und", so the value
// stays reachable through its language field and the "und" copy is redundant.
func TestDedupeResultDeMirrorsUnd(t *testing.T) {
	t.Parallel()

	enLangDoc := makeLanguageDoc(testLangDocID, "en")
	c := newTestConverter(t, nil, []*document.D{enLangDoc}, map[identifier.Identifier]*document.D{})

	ctx := t.Context()

	enSub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: internalCore.InLanguagePropID},
				To:        document.Reference{ID: testLangDocID},
			},
		},
	}

	// The same value is tagged English on one claim (routes to text.en) and left
	// untagged on another (routes to text.und before the de-mirror pass).
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, enSub),
					Prop:      document.Reference{ID: testPropID},
					String:    "shared value",
				},
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: testPropID},
					String:    "shared value",
				},
			},
		},
	}
	result, errE := c.FromDocument(ctx, doc, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	assert.Equal(t, []string{"shared value"}, result.Text["en"])
	assert.NotContains(t, result.Text["und"], "shared value", "a value in text.en must be de-mirrored out of text.und")
	// und-only content (the document ID) is kept.
	assert.Contains(t, result.Text["und"], testDocID.String())
}

// TestEarliestClaimTime verifies the top-level Time field holds the earliest time
// value across all of a document's time claims: here a time interval's lower bound,
// which is earlier than a separate point timestamp on the same document.
func TestEarliestClaimTime(t *testing.T) {
	t.Parallel()

	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: makeNamingDoc(testPropID, "My Prop"),
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()

	fromTS := document.Time("2020-01-01")
	toTS := document.Time("2024-12-31")
	fromPrec := document.TimePrecisionDay
	toPrec := document.TimePrecisionDay

	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			Time: []document.TimeClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: testPropID},
					Time:      document.Time("2022-06-15"),
					Precision: document.TimePrecisionDay,
				},
			},
			TimeInterval: []document.TimeIntervalClaim{
				{ //nolint:exhaustruct
					CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
					Prop:          document.Reference{ID: testPropID},
					From:          &fromTS,
					FromPrecision: &fromPrec,
					To:            &toTS,
					ToPrecision:   &toPrec,
				},
			},
		},
	}

	result, errE := c.FromDocument(ctx, doc, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	// The earliest of all bounds across all indexed time claims.
	var expected float64
	found := false
	for _, tc := range result.Claims.Time {
		for _, bound := range []*float64{tc.From, tc.To} {
			if bound == nil {
				continue
			}
			if !found || *bound < expected {
				expected = *bound
				found = true
			}
		}
	}
	require.True(t, found)
	require.NotNil(t, result.Time)
	assert.InDelta(t, expected, *result.Time, 0)

	// The interval point claims differ, and the earliest (interval lower bound) won.
	require.Len(t, result.Claims.Time, 2)
	for _, tc := range result.Claims.Time {
		require.NotNil(t, tc.From)
		assert.LessOrEqual(t, *result.Time, *tc.From)
	}
}

// TestEarliestClaimTimeNone verifies a document with no time claims has no Time.
func TestEarliestClaimTimeNone(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()

	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: testPropID},
					String:    "no time here",
				},
			},
		},
	}

	result, errE := c.FromDocument(ctx, doc, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Nil(t, result.Time)
}

// TestEarliestClaimTimeOpenBoundsNotSentinel verifies that a time interval with a
// None bound does not leak the -MaxFloat64 / +MaxFloat64 range sentinels into the
// top-level Time field. convertTimeInterval uses those sentinels only for the
// searchable range of an absent bound; the From/To boundary values stay nil, so
// Time (derived from them) falls back to the interval's known bound. Unknown
// bounds never reach this path at all: paired with a known bound they collapse to
// a point, and otherwise they become an unknown claim with no time.
func TestEarliestClaimTimeOpenBoundsNotSentinel(t *testing.T) {
	t.Parallel()

	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: makeNamingDoc(testPropID, "My Prop"),
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()

	fromTS := document.Time("1990-01-01")
	fromPrec := document.TimePrecisionDay
	toTS := document.Time("2023-04-12")
	toPrec := document.TimePrecisionDay

	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			TimeInterval: []document.TimeIntervalClaim{
				{ //nolint:exhaustruct
					// No lower bound (None), known upper bound: range lower is the
					// -MaxFloat64 sentinel, From boundary stays nil.
					CoreClaim:   makeCoreClaim(document.HighConfidence, nil),
					Prop:        document.Reference{ID: testPropID},
					FromIsNone:  true,
					To:          &toTS,
					ToPrecision: &toPrec,
				},
				{ //nolint:exhaustruct
					// Known lower bound, no upper bound (None): range upper is the
					// +MaxFloat64 sentinel, To boundary stays nil.
					CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
					Prop:          document.Reference{ID: testPropID},
					From:          &fromTS,
					FromPrecision: &fromPrec,
					ToIsNone:      true,
				},
			},
		},
	}

	result, errE := c.FromDocument(ctx, doc, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, result.Claims.Time, 2)

	// The sentinels live only in the searchable range, never in the From/To
	// boundary values that Time is derived from.
	var sawLowerSentinel, sawUpperSentinel bool
	for _, tc := range result.Claims.Time {
		if tc.Range.GreaterThanOrEqual != nil && *tc.Range.GreaterThanOrEqual == -math.MaxFloat64 {
			sawLowerSentinel = true
		}
		if tc.Range.LessThanOrEqual != nil && *tc.Range.LessThanOrEqual == math.MaxFloat64 {
			sawUpperSentinel = true
		}
		if tc.From != nil {
			assert.Greater(t, *tc.From, -math.MaxFloat64)
			assert.Less(t, *tc.From, math.MaxFloat64)
		}
		if tc.To != nil {
			assert.Greater(t, *tc.To, -math.MaxFloat64)
			assert.Less(t, *tc.To, math.MaxFloat64)
		}
	}
	assert.True(t, sawLowerSentinel, "open-lower interval should carry the -MaxFloat64 range sentinel")
	assert.True(t, sawUpperSentinel, "open-upper interval should carry the +MaxFloat64 range sentinel")

	// Time is the known lower bound (1990), strictly between the sentinels.
	var earliest float64
	found := false
	for _, tc := range result.Claims.Time {
		for _, bound := range []*float64{tc.From, tc.To} {
			if bound == nil {
				continue
			}
			if !found || *bound < earliest {
				earliest = *bound
				found = true
			}
		}
	}
	require.True(t, found)
	require.NotNil(t, result.Time)
	assert.InDelta(t, earliest, *result.Time, 0)
	assert.Greater(t, *result.Time, -math.MaxFloat64)
	assert.Less(t, *result.Time, math.MaxFloat64)
}

// TestClaimsCountCountsRecursively verifies that ClaimsCount counts every claim
// in the document, including those nested as sub-claims.
func TestClaimsCountCountsRecursively(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()

	// One top-level String claim that carries a sub String claim: two claims total.
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, &document.ClaimTypes{
						String: []document.StringClaim{
							{
								CoreClaim: makeCoreClaim(document.HighConfidence, nil),
								Prop:      document.Reference{ID: testPropID},
								String:    "nested",
							},
						},
					}),
					Prop:   document.Reference{ID: testPropID},
					String: "top",
				},
			},
		},
	}

	result, errE := c.FromDocument(ctx, doc, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.NotNil(t, result.ClaimsCount)
	assert.Equal(t, 2, *result.ClaimsCount)
}

// TestReferencesCount verifies that CountReferences is recorded for ordinary
// documents and skipped for documents that are themselves classes.
func TestReferencesCount(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, map[identifier.Identifier]*document.D{
		internalCore.InstanceOfPropID: makeNamingDoc(internalCore.InstanceOfPropID, "instance of"),
	})
	c.CountReferences = func(_ context.Context, _ identifier.Identifier) (int, errors.E) {
		return 7, nil
	}

	ctx := t.Context()

	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: testPropID},
					String:    "ordinary",
				},
			},
		},
	}
	result, errE := c.FromDocument(ctx, doc, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.NotNil(t, result.ReferencesCount)
	assert.Equal(t, 7, *result.ReferencesCount)

	// A document that is an instance of CLASS is ignored for referencesCount.
	classDoc := &document.D{
		CoreDocument: document.CoreDocument{ID: identifier.New()}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			Reference: []document.ReferenceClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: internalCore.InstanceOfPropID},
					To:        document.Reference{ID: internalCore.ClassClassID},
				},
			},
		},
	}
	result, errE = c.FromDocument(ctx, classDoc, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Nil(t, result.ReferencesCount)

	// A document that is an instance of VOCABULARY is ignored too.
	vocabDoc := &document.D{
		CoreDocument: document.CoreDocument{ID: identifier.New()}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			Reference: []document.ReferenceClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: internalCore.InstanceOfPropID},
					To:        document.Reference{ID: internalCore.VocabularyClassID},
				},
			},
		},
	}
	result, errE = c.FromDocument(ctx, vocabDoc, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Nil(t, result.ReferencesCount)
}

// TestReferencesCountIgnoresTransitiveSubclass verifies that a document which is
// an instance of a transitive subclass of VOCABULARY is ignored for
// referencesCount via the class hierarchy.
func TestReferencesCountIgnoresTransitiveSubclass(t *testing.T) {
	t.Parallel()

	// Set up the SUBENTITY_OF hierarchy so SUBCLASS_OF is a value hierarchy property.
	properties := []*document.D{
		makePropertyDoc(internalCore.SubentityOfPropID, nil),
		makePropertyDoc(internalCore.SubclassOfPropID, &internalCore.SubentityOfPropID),
		makePropertyDoc(internalCore.InstanceOfPropID, &internalCore.SubentityOfPropID),
	}

	// A vocabulary subclass: SUBCLASS_OF VOCABULARY.
	vocabSubclass := identifier.New()
	vocabSubclassDoc := makeHierarchyDoc(vocabSubclass, "Discipline", internalCore.SubclassOfPropID, &internalCore.VocabularyClassID)

	c := newTestConverter(t, properties, nil, map[identifier.Identifier]*document.D{vocabSubclass: vocabSubclassDoc})
	c.CountReferences = func(_ context.Context, _ identifier.Identifier) (int, errors.E) {
		return 7, nil
	}

	// A document that is an instance of that vocabulary subclass.
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			Reference: []document.ReferenceClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: internalCore.InstanceOfPropID},
					To:        document.Reference{ID: vocabSubclass},
				},
			},
		},
	}
	result, errE := c.FromDocument(t.Context(), doc, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Nil(t, result.ReferencesCount)
}

// Tests for error propagation through Visit* methods and FromDocument.

func TestFromDocumentAmountError(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			Amount: []document.AmountClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: identifier.New()},
					Amount:    document.Amount("42"),
					Precision: 1,
				},
			},
		},
	}

	_, errE := c.FromDocument(ctx, doc, nil)
	assert.EqualError(t, errE, "document not found")
}

func TestFromDocumentAmountIntervalError(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	fromAmount := document.Amount("5")
	toAmount := document.Amount("10")
	fromPrec := 1.0
	toPrec := 1.0
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			AmountInterval: []document.AmountIntervalClaim{
				{ //nolint:exhaustruct
					CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
					Prop:          document.Reference{ID: identifier.New()},
					From:          &fromAmount,
					FromPrecision: &fromPrec,
					To:            &toAmount,
					ToPrecision:   &toPrec,
				},
			},
		},
	}

	_, errE := c.FromDocument(ctx, doc, nil)
	assert.EqualError(t, errE, "document not found")
}

func TestFromDocumentTimeError(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			Time: []document.TimeClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: identifier.New()},
					Time:      document.Time("2024-01-15"),
					Precision: document.TimePrecisionDay,
				},
			},
		},
	}

	_, errE := c.FromDocument(ctx, doc, nil)
	assert.EqualError(t, errE, "document not found")
}

func TestFromDocumentTimeIntervalError(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	fromTS := document.Time("2024-01-01")
	toTS := document.Time("2024-12-31")
	fromPrec := document.TimePrecisionDay
	toPrec := document.TimePrecisionDay
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			TimeInterval: []document.TimeIntervalClaim{
				{ //nolint:exhaustruct
					CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
					Prop:          document.Reference{ID: identifier.New()},
					From:          &fromTS,
					FromPrecision: &fromPrec,
					To:            &toTS,
					ToPrecision:   &toPrec,
				},
			},
		},
	}

	_, errE := c.FromDocument(ctx, doc, nil)
	assert.EqualError(t, errE, "document not found")
}

func TestFromDocumentRelationError(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			Reference: []document.ReferenceClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: identifier.New()},
					To:        document.Reference{ID: identifier.New()},
				},
			},
		},
	}

	_, errE := c.FromDocument(ctx, doc, nil)
	assert.EqualError(t, errE, "document not found")
}

func TestFromDocumentHasError(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			Has: []document.HasClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: identifier.New()},
				},
			},
		},
	}

	_, errE := c.FromDocument(ctx, doc, nil)
	assert.EqualError(t, errE, "document not found")
}

func TestFromDocumentNoneError(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			None: []document.NoneClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: identifier.New()},
				},
			},
		},
	}

	_, errE := c.FromDocument(ctx, doc, nil)
	assert.EqualError(t, errE, "document not found")
}

func TestFromDocumentUnknownError(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			Unknown: []document.UnknownClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: identifier.New()},
				},
			},
		},
	}

	_, errE := c.FromDocument(ctx, doc, nil)
	assert.EqualError(t, errE, "document not found")
}

func TestGetDisplayStringsMakeDisplayError(t *testing.T) {
	t.Parallel()

	// Document with no naming strings at all.
	docID := identifier.New()
	emptyDoc := &document.D{
		CoreDocument: document.CoreDocument{ID: docID}, //nolint:exhaustruct
	}
	extraDocs := map[identifier.Identifier]*document.D{
		docID: emptyDoc,
	}

	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	ds, errE := c.getDisplayStrings(ctx, docID)
	require.NoError(t, errE, "% -+#.1v", errE)
	// Display and Naming should be empty maps.
	assert.Empty(t, ds.Display)
	assert.Empty(t, ds.Naming)
}

func TestConvertAmountInvalidAmount(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	claim := &document.AmountClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: testPropID},
		Amount:    document.Amount("not-a-number"),
		Precision: 1,
	}
	// Intentionally NO claim.Validate() call here - claim is deliberately
	// invalid and would not pass Validate().
	_, errE := c.convertAmount(ctx, claim)
	assert.EqualError(t, errE, "unable to parse amount")
}

func TestConvertAmountPropagationError(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)
	c.IndexAncestorProperties = true
	unknownParent := identifier.New()
	c.propertyAncestors = map[identifier.Identifier][]identifier.Identifier{
		testPropID: {unknownParent},
	}

	ctx := t.Context()
	claim := &document.AmountClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: testPropID},
		Amount:    document.Amount("42"),
		Precision: 1,
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = c.convertAmount(ctx, claim)
	assert.EqualError(t, errE, "document not found")
}

func TestConvertAmountIntervalPropagationError(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)
	c.IndexAncestorProperties = true
	unknownParent := identifier.New()
	c.propertyAncestors = map[identifier.Identifier][]identifier.Identifier{
		testPropID: {unknownParent},
	}

	ctx := t.Context()
	fromAmount := document.Amount("10")
	toAmount := document.Amount("20")
	fromPrec := 1.0
	toPrec := 1.0
	claim := &document.AmountIntervalClaim{ //nolint:exhaustruct
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: testPropID},
		From:          &fromAmount,
		FromPrecision: &fromPrec,
		To:            &toAmount,
		ToPrecision:   &toPrec,
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	_, _, _, errE = c.convertAmountInterval(ctx, claim) //nolint:dogsled
	assert.EqualError(t, errE, "document not found")
}

func TestConvertAmountIntervalInvalidFromAmount(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	fromAmount := document.Amount("invalid")
	fromPrec := 1.0
	toAmount := document.Amount("20")
	toPrec := 1.0
	claim := &document.AmountIntervalClaim{ //nolint:exhaustruct
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: testPropID},
		From:          &fromAmount,
		FromPrecision: &fromPrec,
		To:            &toAmount,
		ToPrecision:   &toPrec,
	}
	// Intentionally NO claim.Validate() call here - claim is deliberately
	// invalid and would not pass Validate().
	_, _, _, errE := c.convertAmountInterval(ctx, claim) //nolint:dogsled
	assert.EqualError(t, errE, "unable to parse amount")
}

func TestConvertAmountIntervalInvalidToAmount(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	fromAmount := document.Amount("10")
	fromPrec := 1.0
	toAmount := document.Amount("invalid")
	toPrec := 1.0
	claim := &document.AmountIntervalClaim{ //nolint:exhaustruct
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: testPropID},
		From:          &fromAmount,
		FromPrecision: &fromPrec,
		To:            &toAmount,
		ToPrecision:   &toPrec,
	}
	// Intentionally NO claim.Validate() call here - claim is deliberately
	// invalid and would not pass Validate().
	_, _, _, errE := c.convertAmountInterval(ctx, claim) //nolint:dogsled
	assert.EqualError(t, errE, "unable to parse amount")
}

func TestConvertTimePropagationError(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)
	c.IndexAncestorProperties = true
	unknownParent := identifier.New()
	c.propertyAncestors = map[identifier.Identifier][]identifier.Identifier{
		testPropID: {unknownParent},
	}

	ctx := t.Context()
	claim := &document.TimeClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: testPropID},
		Time:      document.Time("2024-01-15"),
		Precision: document.TimePrecisionDay,
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = c.convertTime(ctx, claim)
	assert.EqualError(t, errE, "document not found")
}

func TestConvertTimeInvalidTime(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	claim := &document.TimeClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: testPropID},
		Time:      document.Time("not-a-time"),
		Precision: document.TimePrecisionDay,
	}
	// Intentionally NO claim.Validate() call here - claim is deliberately
	// invalid and would not pass Validate().
	_, errE := c.convertTime(ctx, claim)
	assert.EqualError(t, errE, "unable to parse time")
}

func TestConvertTimeIntervalPropagationError(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)
	c.IndexAncestorProperties = true
	unknownParent := identifier.New()
	c.propertyAncestors = map[identifier.Identifier][]identifier.Identifier{
		testPropID: {unknownParent},
	}

	ctx := t.Context()
	fromTS := document.Time("2024-01-01")
	toTS := document.Time("2024-12-31")
	fromPrec := document.TimePrecisionDay
	toPrec := document.TimePrecisionDay
	claim := &document.TimeIntervalClaim{ //nolint:exhaustruct
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: testPropID},
		From:          &fromTS,
		FromPrecision: &fromPrec,
		To:            &toTS,
		ToPrecision:   &toPrec,
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	_, _, _, errE = c.convertTimeInterval(ctx, claim) //nolint:dogsled
	assert.EqualError(t, errE, "document not found")
}

func TestConvertTimeIntervalInvalidFromTime(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	fromTS := document.Time("not-a-time")
	fromPrec := document.TimePrecisionDay
	toTS := document.Time("2024-12-31")
	toPrec := document.TimePrecisionDay
	claim := &document.TimeIntervalClaim{ //nolint:exhaustruct
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: testPropID},
		From:          &fromTS,
		FromPrecision: &fromPrec,
		To:            &toTS,
		ToPrecision:   &toPrec,
	}
	// Intentionally NO claim.Validate() call here - claim is deliberately
	// invalid and would not pass Validate().
	_, _, _, errE := c.convertTimeInterval(ctx, claim) //nolint:dogsled
	assert.EqualError(t, errE, "unable to parse time")
}

func TestConvertTimeIntervalInvalidToTime(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	fromTS := document.Time("2024-01-01")
	fromPrec := document.TimePrecisionDay
	toTS := document.Time("not-a-time")
	toPrec := document.TimePrecisionDay
	claim := &document.TimeIntervalClaim{ //nolint:exhaustruct
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: testPropID},
		From:          &fromTS,
		FromPrecision: &fromPrec,
		To:            &toTS,
		ToPrecision:   &toPrec,
	}
	// Intentionally NO claim.Validate() call here - claim is deliberately
	// invalid and would not pass Validate().
	_, _, _, errE := c.convertTimeInterval(ctx, claim) //nolint:dogsled
	assert.EqualError(t, errE, "unable to parse time")
}

func TestConvertRelationSubPropError(t *testing.T) {
	t.Parallel()

	// Sub-claim relation has unknown prop ID.
	targetDoc := makeNamingDoc(testTargetDocID, "Target")
	extraDocs := map[identifier.Identifier]*document.D{
		testTargetDocID: targetDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	sub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: identifier.New()}, // Unknown prop.
				To:        document.Reference{ID: testTargetDocID},
			},
		},
	}
	claim := &document.ReferenceClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, sub),
		Prop:      document.Reference{ID: testPropID},
		To:        document.Reference{ID: testTargetDocID},
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	_, _, errE = c.convertReference(ctx, claim)
	assert.EqualError(t, errE, "document not found")
}

func TestConvertRelationSubToError(t *testing.T) {
	t.Parallel()

	subPropDoc := makeNamingDoc(testPropID2, "Sub Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID2: subPropDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	sub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: testPropID2},
				To:        document.Reference{ID: identifier.New()}, // Unknown target.
			},
		},
	}
	claim := &document.ReferenceClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, sub),
		Prop:      document.Reference{ID: testPropID},
		To:        document.Reference{ID: testTargetDocID},
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	_, _, errE = c.convertReference(ctx, claim)
	assert.EqualError(t, errE, "document not found")
}

func TestConvertRelationToDisplayError(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Rel Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
		// testTargetDocID is NOT in extraDocs, so getDisplayStrings for it will fail.
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	claim := &document.ReferenceClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: testPropID},
		To:        document.Reference{ID: testTargetDocID},
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	_, _, errE = c.convertReference(ctx, claim)
	assert.EqualError(t, errE, "document not found")
}

func TestConvertHasSubPropError(t *testing.T) {
	t.Parallel()

	extraDocs := map[identifier.Identifier]*document.D{}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	sub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: identifier.New()}, // Unknown prop.
				To:        document.Reference{ID: identifier.New()},
			},
		},
	}
	claim := &document.HasClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, sub),
		Prop:      document.Reference{ID: testPropID},
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	_, _, errE = c.convertHas(ctx, claim)
	assert.EqualError(t, errE, "document not found")
}

func TestConvertHasSubToError(t *testing.T) {
	t.Parallel()

	subPropDoc := makeNamingDoc(testPropID2, "Sub Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID2: subPropDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	sub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: testPropID2},
				To:        document.Reference{ID: identifier.New()}, // Unknown target.
			},
		},
	}
	claim := &document.HasClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, sub),
		Prop:      document.Reference{ID: testPropID},
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	_, _, errE = c.convertHas(ctx, claim)
	assert.EqualError(t, errE, "document not found")
}

func TestConvertHasPropagationError(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)
	c.IndexAncestorProperties = true
	unknownParent := identifier.New()
	c.propertyAncestors = map[identifier.Identifier][]identifier.Identifier{
		testPropID: {unknownParent},
	}

	ctx := t.Context()
	claim := &document.HasClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: testPropID},
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	_, _, errE = c.convertHas(ctx, claim)
	assert.EqualError(t, errE, "document not found")
}

func TestConvertRelationPropagationPropError(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Rel Prop")
	targetDoc := makeNamingDoc(testTargetDocID, "Target")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID:      propDoc,
		testTargetDocID: targetDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)
	c.IndexAncestorProperties = true
	unknownParent := identifier.New()
	c.propertyAncestors = map[identifier.Identifier][]identifier.Identifier{
		testPropID: {unknownParent},
	}

	ctx := t.Context()
	claim := &document.ReferenceClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: testPropID},
		To:        document.Reference{ID: testTargetDocID},
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	_, _, errE = c.convertReference(ctx, claim)
	assert.EqualError(t, errE, "document not found")
}

func TestConvertNonePropagationError(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)
	c.IndexAncestorProperties = true
	unknownParent := identifier.New()
	c.propertyAncestors = map[identifier.Identifier][]identifier.Identifier{
		testPropID: {unknownParent},
	}

	ctx := t.Context()
	claim := &document.NoneClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: testPropID},
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	_, _, errE = c.convertNone(ctx, claim)
	assert.EqualError(t, errE, "document not found")
}

func TestConvertUnknownPropagationError(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)
	c.IndexAncestorProperties = true
	unknownParent := identifier.New()
	c.propertyAncestors = map[identifier.Identifier][]identifier.Identifier{
		testPropID: {unknownParent},
	}

	ctx := t.Context()
	claim := &document.UnknownClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: testPropID},
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	_, _, errE = c.convertUnknown(ctx, claim)
	assert.EqualError(t, errE, "document not found")
}

func TestConvertAmountIntervalFromUnknownToError(t *testing.T) {
	t.Parallel()

	// FromIsUnknown with To: delegates to convertAmount, which needs prop display.
	// But prop is not found, so it errors.
	c := newTestConverter(t, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	toAmount := document.Amount("20")
	toPrec := 1.0
	claim := &document.AmountIntervalClaim{ //nolint:exhaustruct
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: identifier.New()},
		FromIsUnknown: true,
		To:            &toAmount,
		ToPrecision:   &toPrec,
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	_, _, _, errE = c.convertAmountInterval(ctx, claim) //nolint:dogsled
	assert.EqualError(t, errE, "document not found")
}

func TestConvertAmountIntervalToUnknownFromError(t *testing.T) {
	t.Parallel()

	// ToIsUnknown with From: delegates to convertAmount with From, which errors.
	c := newTestConverter(t, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	fromAmount := document.Amount("10")
	fromPrec := 1.0
	claim := &document.AmountIntervalClaim{ //nolint:exhaustruct
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: identifier.New()},
		From:          &fromAmount,
		FromPrecision: &fromPrec,
		ToIsUnknown:   true,
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	_, _, _, errE = c.convertAmountInterval(ctx, claim) //nolint:dogsled
	assert.EqualError(t, errE, "document not found")
}

func TestConvertTimeIntervalFromUnknownToError(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	toTS := document.Time("2024-06-15")
	toPrec := document.TimePrecisionDay
	claim := &document.TimeIntervalClaim{ //nolint:exhaustruct
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: identifier.New()},
		FromIsUnknown: true,
		To:            &toTS,
		ToPrecision:   &toPrec,
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	_, _, _, errE = c.convertTimeInterval(ctx, claim) //nolint:dogsled
	assert.EqualError(t, errE, "document not found")
}

func TestConvertTimeIntervalToUnknownFromError(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	fromTS := document.Time("2024-06-15")
	fromPrec := document.TimePrecisionDay
	claim := &document.TimeIntervalClaim{ //nolint:exhaustruct
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: identifier.New()},
		From:          &fromTS,
		FromPrecision: &fromPrec,
		ToIsUnknown:   true,
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)
	_, _, _, errE = c.convertTimeInterval(ctx, claim) //nolint:dogsled
	assert.EqualError(t, errE, "document not found")
}

// TestConvertReferenceWithSubAmountClaim verifies that an amount sub-claim on
// a reference parent claim is flattened into SubAmountClaim entries with the
// parent's prop/target as parentProp/parentTo.
func TestConvertReferenceWithSubAmountClaim(t *testing.T) {
	t.Parallel()

	subPropID := identifier.New()
	propDoc := makeNamingDoc(testPropID, "Rel Prop")
	targetDoc := makeNamingDoc(testTargetDocID, "Target")
	subPropDoc := makeNamingDoc(subPropID, "Sub Amount Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID:      propDoc,
		testTargetDocID: targetDoc,
		subPropID:       subPropDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	sub := &document.ClaimTypes{
		Amount: []document.AmountClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: subPropID},
				Amount:    document.Amount("42"),
				Precision: 1,
			},
		},
	}
	claim := &document.ReferenceClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, sub),
		Prop:      document.Reference{ID: testPropID},
		To:        document.Reference{ID: testTargetDocID},
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)

	_, subs, errE := c.convertReference(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, subs.Amounts, 1)
	assert.Equal(t, testPropID, subs.Amounts[0].ParentProp)
	assert.Equal(t, testTargetDocID.String(), subs.Amounts[0].ParentTo)
	assert.Equal(t, subPropID, subs.Amounts[0].Prop)
	require.NotNil(t, subs.Amounts[0].From)
	assert.Equal(t, 41.5, *subs.Amounts[0].From) //nolint:testifylint
	require.NotNil(t, subs.Amounts[0].To)
	assert.Equal(t, 42.5, *subs.Amounts[0].To) //nolint:testifylint
	assert.Empty(t, subs.Refs)
	assert.Empty(t, subs.Times)
	assert.Empty(t, subs.Has)
}

// TestConvertReferenceWithSubTimeIntervalClaim verifies that a time-interval
// sub-claim on a reference parent claim is flattened into a SubTimeClaim
// (mapped to a range) with the parent's prop/target as parentProp/parentTo.
func TestConvertReferenceWithSubTimeIntervalClaim(t *testing.T) {
	t.Parallel()

	subPropID := identifier.New()
	propDoc := makeNamingDoc(testPropID, "Rel Prop")
	targetDoc := makeNamingDoc(testTargetDocID, "Target")
	subPropDoc := makeNamingDoc(subPropID, "Period")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID:      propDoc,
		testTargetDocID: targetDoc,
		subPropID:       subPropDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	fromTime := document.Time("2020")
	toTime := document.Time("2022")
	fromPrec := document.TimePrecisionYear
	toPrec := document.TimePrecisionYear
	sub := &document.ClaimTypes{
		TimeInterval: []document.TimeIntervalClaim{
			{
				CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
				Prop:          document.Reference{ID: subPropID},
				From:          &fromTime,
				FromPrecision: &fromPrec,
				FromIsOpen:    false,
				FromIsUnknown: false,
				FromIsNone:    false,
				To:            &toTime,
				ToPrecision:   &toPrec,
				ToIsOpen:      false,
				ToIsUnknown:   false,
				ToIsNone:      false,
			},
		},
	}
	claim := &document.ReferenceClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, sub),
		Prop:      document.Reference{ID: testPropID},
		To:        document.Reference{ID: testTargetDocID},
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)

	_, subs, errE := c.convertReference(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, subs.Times, 1)
	assert.Equal(t, testPropID, subs.Times[0].ParentProp)
	assert.Equal(t, testTargetDocID.String(), subs.Times[0].ParentTo)
	assert.Equal(t, subPropID, subs.Times[0].Prop)
	require.NotNil(t, subs.Times[0].From)
	require.NotNil(t, subs.Times[0].To)
	// From is start of 2020, To is start of 2023 (exclusive end of 2022).
	assert.Less(t, *subs.Times[0].From, *subs.Times[0].To)
	assert.Empty(t, subs.Refs)
	assert.Empty(t, subs.Amounts)
	assert.Empty(t, subs.Has)
}

// TestConvertReferenceWithSubHasClaim verifies that a simple has sub-claim on
// a reference parent claim is flattened into a SubHasClaim with the parent's
// prop/target as parentProp/parentTo.
func TestConvertReferenceWithSubHasClaim(t *testing.T) {
	t.Parallel()

	subPropID := identifier.New()
	propDoc := makeNamingDoc(testPropID, "Rel Prop")
	targetDoc := makeNamingDoc(testTargetDocID, "Target")
	subPropDoc := makeNamingDoc(subPropID, "Has Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID:      propDoc,
		testTargetDocID: targetDoc,
		subPropID:       subPropDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	sub := &document.ClaimTypes{
		Has: []document.HasClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: subPropID},
			},
		},
	}
	claim := &document.ReferenceClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, sub),
		Prop:      document.Reference{ID: testPropID},
		To:        document.Reference{ID: testTargetDocID},
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)

	_, subs, errE := c.convertReference(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, subs.Has, 1)
	assert.Equal(t, testPropID, subs.Has[0].ParentProp)
	assert.Equal(t, testTargetDocID.String(), subs.Has[0].ParentTo)
	assert.Equal(t, subPropID, subs.Has[0].Prop)
	assert.Empty(t, subs.Refs)
	assert.Empty(t, subs.Amounts)
	assert.Empty(t, subs.Times)
}

// TestConvertHasWithSubHasClaim verifies that a has sub-claim under a has
// parent claim is flattened into a SubHasClaim with ParentTo=ParentToHas.
func TestConvertHasWithSubHasClaim(t *testing.T) {
	t.Parallel()

	subPropID := identifier.New()
	propDoc := makeNamingDoc(testPropID, "Has Prop")
	subPropDoc := makeNamingDoc(subPropID, "Sub Has Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
		subPropID:  subPropDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	sub := &document.ClaimTypes{
		Has: []document.HasClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: subPropID},
			},
		},
	}
	claim := &document.HasClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, sub),
		Prop:      document.Reference{ID: testPropID},
	}
	errE := claim.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)

	result, subs, errE := c.convertHas(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	// Has claim with sub-claims is not indexed as a HasClaim entry.
	require.Empty(t, result)
	require.Len(t, subs.Has, 1)
	assert.Equal(t, testPropID, subs.Has[0].ParentProp)
	assert.Equal(t, ParentToHas, subs.Has[0].ParentTo)
	assert.Equal(t, subPropID, subs.Has[0].Prop)
}

// makeClassDocWithTemplate creates a class document with an INSTANCE_OF PROPERTY
// and a DISPLAY_LABEL_TEMPLATE claim.
func makeClassDocWithTemplate(id identifier.Identifier, tmpl string) *document.D {
	claims := &document.ClaimTypes{}
	claims.Reference = append(claims.Reference, document.ReferenceClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: internalCore.InstanceOfPropID},
		To:        document.Reference{ID: internalCore.PropertyClassID},
	})
	claims.String = append(claims.String, document.StringClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: internalCore.DisplayLabelTemplatePropID},
		String:    tmpl,
	})
	return &document.D{
		CoreDocument: document.CoreDocument{ID: id}, //nolint:exhaustruct
		Claims:       claims,
	}
}

// makeClassDocWithTemplateInLanguage creates a class document with an INSTANCE_OF PROPERTY
// and a DISPLAY_LABEL_TEMPLATE claim tagged with the given language.
func makeClassDocWithTemplateInLanguage(id identifier.Identifier, tmpl string, langID identifier.Identifier) *document.D {
	langSub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: internalCore.InLanguagePropID},
				To:        document.Reference{ID: langID},
			},
		},
	}
	claims := &document.ClaimTypes{}
	claims.Reference = append(claims.Reference, document.ReferenceClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: internalCore.InstanceOfPropID},
		To:        document.Reference{ID: internalCore.PropertyClassID},
	})
	claims.String = append(claims.String, document.StringClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, langSub),
		Prop:      document.Reference{ID: internalCore.DisplayLabelTemplatePropID},
		String:    tmpl,
	})
	return &document.D{
		CoreDocument: document.CoreDocument{ID: id}, //nolint:exhaustruct
		Claims:       claims,
	}
}

// makeClassDocWithTemplates creates a class document with an INSTANCE_OF PROPERTY
// and multiple DISPLAY_LABEL_TEMPLATE claims, each tagged with a language.
func makeClassDocWithTemplates(id identifier.Identifier, templates map[identifier.Identifier]string) *document.D {
	claims := &document.ClaimTypes{}
	claims.Reference = append(claims.Reference, document.ReferenceClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: internalCore.InstanceOfPropID},
		To:        document.Reference{ID: internalCore.PropertyClassID},
	})
	for langID, tmpl := range templates {
		langSub := &document.ClaimTypes{
			Reference: []document.ReferenceClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: internalCore.InLanguagePropID},
					To:        document.Reference{ID: langID},
				},
			},
		}
		claims.String = append(claims.String, document.StringClaim{
			CoreClaim: makeCoreClaim(document.HighConfidence, langSub),
			Prop:      document.Reference{ID: internalCore.DisplayLabelTemplatePropID},
			String:    tmpl,
		})
	}
	return &document.D{
		CoreDocument: document.CoreDocument{ID: id}, //nolint:exhaustruct
		Claims:       claims,
	}
}

// addInstanceOf adds an INSTANCE_OF relation claim to a document.
func addInstanceOf(doc *document.D, classID identifier.Identifier, confidence document.Confidence) {
	if doc.Claims == nil {
		doc.Claims = &document.ClaimTypes{}
	}
	doc.Claims.Reference = append(doc.Claims.Reference, document.ReferenceClaim{
		CoreClaim: makeCoreClaim(confidence, nil),
		Prop:      document.Reference{ID: internalCore.InstanceOfPropID},
		To:        document.Reference{ID: classID},
	})
}

func TestDisplayLabelTemplate(t *testing.T) {
	t.Parallel()

	classID := identifier.New()
	classDoc := makeClassDocWithTemplate(classID, `{{bestString "SHORT_NAME" .}}`)

	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
	}
	addInstanceOf(doc, classID, document.HighConfidence)

	extraDocs := map[identifier.Identifier]*document.D{
		classID: classDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	result, errE := c.displayLabelTemplate(ctx, doc)
	require.NoError(t, errE, "% -+#.1v", errE)
	// Template without IN_LANGUAGE is "und"; all languages resolve to it via fallback.
	for lang := range SupportedLanguages {
		assert.Equal(t, `{{bestString "SHORT_NAME" .}}`, result[lang])
	}
}

func TestDisplayLabelTemplateEmpty(t *testing.T) {
	t.Parallel()

	// Document without INSTANCE_OF.
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
	}

	c := newTestConverter(t, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	result, errE := c.displayLabelTemplate(ctx, doc)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Nil(t, result)
}

func TestDisplayLabelTemplateConfidenceSelection(t *testing.T) {
	t.Parallel()

	// Two classes, each with a template. The one with higher effective confidence wins.
	classA := identifier.New()
	classB := identifier.New()
	classADoc := makeClassDocWithTemplate(classA, `Template A`)
	classBDoc := makeClassDocWithTemplate(classB, `Template B`)

	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
	}
	// INSTANCE_OF classA with medium confidence: effective = 0.75 * 1.0 = 0.75.
	addInstanceOf(doc, classA, document.MediumConfidence)
	// INSTANCE_OF classB with high confidence: effective = 1.0 * 1.0 = 1.0.
	addInstanceOf(doc, classB, document.HighConfidence)

	extraDocs := map[identifier.Identifier]*document.D{
		classA: classADoc,
		classB: classBDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	result, errE := c.displayLabelTemplate(ctx, doc)
	require.NoError(t, errE, "% -+#.1v", errE)
	// Both templates are "und" (no IN_LANGUAGE); all languages resolve to the highest confidence one.
	for lang := range SupportedLanguages {
		assert.Equal(t, "Template B", result[lang])
	}
}

func TestMakeDisplayStringsWithTemplate(t *testing.T) {
	t.Parallel()

	shortNamePropID := identifier.New()
	classID := identifier.New()
	classDoc := makeClassDocWithTemplate(classID, `{{bestString `+idTmpl(shortNamePropID)+` .}}`)

	extraDocs := map[identifier.Identifier]*document.D{
		classID: classDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)
	c.namingProperties = []identifier.Identifier{internalCore.NamingPropID}

	// Document with INSTANCE_OF class and naming + short name claims.
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: internalCore.NamingPropID},
					String:    "Full Name",
				},
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: shortNamePropID},
					String:    "FN",
				},
			},
		},
	}
	addInstanceOf(doc, classID, document.HighConfidence)

	ctx := t.Context()
	display, errE := c.makeDisplayStrings(ctx, doc)
	require.NoError(t, errE, "% -+#.1v", errE)
	// Template renders the short name as display.
	assert.Equal(t, "FN", display.Display["und"])
	// All naming strings in Naming.
	assert.Equal(t, []string{"Full Name"}, display.Naming["und"])
}

func TestMakeDisplayStringsWithInvalidTemplate(t *testing.T) {
	t.Parallel()

	classID := identifier.New()
	classDoc := makeClassDocWithTemplate(classID, `{{invalid syntax`)

	extraDocs := map[identifier.Identifier]*document.D{
		classID: classDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)
	c.namingProperties = []identifier.Identifier{internalCore.NamingPropID}

	// Template with invalid syntax should return an error.
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: internalCore.NamingPropID},
					String:    "Fallback Name",
				},
			},
		},
	}
	addInstanceOf(doc, classID, document.HighConfidence)

	ctx := t.Context()
	_, errE := c.makeDisplayStrings(ctx, doc)
	assert.Error(t, errE)
	// Go template parsing error has an unpredictable format, so we use assert.Contains here instead of assert.EqualError.
	assert.Contains(t, errE.Error(), "function \"invalid\" not defined")
}

func TestMakeDisplayStringsTemplateAllLanguages(t *testing.T) {
	t.Parallel()

	enLangID := identifier.New()
	slLangID := identifier.New()
	shortNamePropID := identifier.New()
	classID := identifier.New()
	classDoc := makeClassDocWithTemplate(classID, `{{bestString `+idTmpl(shortNamePropID)+` .}}`)

	languages := []*document.D{
		makeLanguageDoc(enLangID, "en"),
		makeLanguageDoc(slLangID, "sl"),
	}
	extraDocs := map[identifier.Identifier]*document.D{
		classID: classDoc,
	}
	c := newTestConverter(t, nil, languages, extraDocs)
	c.namingProperties = []identifier.Identifier{internalCore.NamingPropID}

	enSub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: internalCore.InLanguagePropID},
				To:        document.Reference{ID: enLangID},
			},
		},
	}
	slSub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: internalCore.InLanguagePropID},
				To:        document.Reference{ID: slLangID},
			},
		},
	}

	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, enSub),
					Prop:      document.Reference{ID: internalCore.NamingPropID},
					String:    "English Name",
				},
				{
					// SHORT_NAME only in English, used by the template.
					CoreClaim: makeCoreClaim(document.HighConfidence, enSub),
					Prop:      document.Reference{ID: shortNamePropID},
					String:    "EN",
				},
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, slSub),
					Prop:      document.Reference{ID: shortNamePropID},
					String:    "SL",
				},
			},
		},
	}
	addInstanceOf(doc, classID, document.HighConfidence)

	ctx := t.Context()
	display, errE := c.makeDisplayStrings(ctx, doc)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Template without IN_LANGUAGE is "und"; all languages resolve to it via fallback.
	// English renders bestString for "en": finds "EN".
	assert.Equal(t, "EN", display.Display["en"])
	// Slovenian renders bestString for "sl": finds "SL".
	assert.Equal(t, "SL", display.Display["sl"])
}

func TestMakeDisplayStringsTemplateRelationTraversal(t *testing.T) {
	t.Parallel()

	shortNamePropID := identifier.New()
	parentRelPropID := identifier.New()
	yearPropID := identifier.New()
	parentDocID := identifier.New()
	classID := identifier.New()

	parentDoc := &document.D{
		CoreDocument: document.CoreDocument{ID: parentDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			Amount: []document.AmountClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: yearPropID},
					Amount:    document.Amount("1776"),
					Precision: 1,
				},
			},
		},
	}
	classDoc := makeClassDocWithTemplate(classID,
		`{{bestString `+idTmpl(shortNamePropID)+` .}} ({{bestReferenceDoc `+idTmpl(parentRelPropID)+` . | bestAmountString `+idTmpl(yearPropID)+`}})`,
	)

	extraDocs := map[identifier.Identifier]*document.D{
		parentDocID: parentDoc,
		classID:     classDoc,
	}

	c := newTestConverter(t, nil, nil, extraDocs)
	c.namingProperties = []identifier.Identifier{internalCore.NamingPropID}
	c.LanguageCodes = map[identifier.Identifier]string{}

	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: internalCore.NamingPropID},
					String:    "United States",
				},
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: shortNamePropID},
					String:    "US",
				},
			},
			Reference: []document.ReferenceClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: parentRelPropID},
					To:        document.Reference{ID: parentDocID},
				},
			},
		},
	}
	addInstanceOf(doc, classID, document.HighConfidence)

	ctx := t.Context()
	display, errE := c.makeDisplayStrings(ctx, doc)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, "US (1776)", display.Display["und"])
	assert.Equal(t, []string{"United States"}, display.Naming["und"])
}

func TestMakeDisplayStringsTemplateOnlyNoNaming(t *testing.T) {
	t.Parallel()

	shortNamePropID := identifier.New()
	classID := identifier.New()
	classDoc := makeClassDocWithTemplate(classID, `{{bestString `+idTmpl(shortNamePropID)+` .}}`)

	extraDocs := map[identifier.Identifier]*document.D{
		classID: classDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)
	c.namingProperties = []identifier.Identifier{internalCore.NamingPropID}

	// Document with template (via class) but no naming strings.
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: shortNamePropID},
					String:    "ShortVal",
				},
			},
		},
	}
	addInstanceOf(doc, classID, document.HighConfidence)

	ctx := t.Context()
	display, errE := c.makeDisplayStrings(ctx, doc)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, "ShortVal", display.Display["und"])
	assert.Empty(t, display.Naming["und"])
}

func TestTemplateBestStringLanguageFallback(t *testing.T) {
	t.Parallel()

	enLangID := identifier.New()
	namePropID := identifier.New()
	classID := identifier.New()
	classDoc := makeClassDocWithTemplate(classID, `{{bestString `+idTmpl(namePropID)+` .}}`)

	languages := []*document.D{
		makeLanguageDoc(enLangID, "en"),
	}
	extraDocs := map[identifier.Identifier]*document.D{
		classID: classDoc,
	}
	c := newTestConverter(t, nil, languages, extraDocs)
	c.namingProperties = []identifier.Identifier{internalCore.NamingPropID}

	enSub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: internalCore.InLanguagePropID},
				To:        document.Reference{ID: enLangID},
			},
		},
	}

	// Document with NAME claim only in "und". Template's bestString for "en"
	// should fall back to "und" when "en" is not found.
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, enSub),
					Prop:      document.Reference{ID: internalCore.NamingPropID},
					String:    "English Naming",
				},
				{
					// NAME claim without language (und).
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: namePropID},
					String:    "Universal Name",
				},
			},
		},
	}
	addInstanceOf(doc, classID, document.HighConfidence)

	ctx := t.Context()
	display, errE := c.makeDisplayStrings(ctx, doc)
	require.NoError(t, errE, "% -+#.1v", errE)
	// Template bestString for "en" falls back to "und" NAME claim.
	assert.Equal(t, "Universal Name", display.Display["en"])
}

func TestTemplateBestIdentifier(t *testing.T) {
	t.Parallel()

	codeProp := identifier.New()
	classID := identifier.New()
	classDoc := makeClassDocWithTemplate(classID, `ID: {{bestIdentifier `+idTmpl(codeProp)+` .}}`)

	extraDocs := map[identifier.Identifier]*document.D{
		classID: classDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)
	c.namingProperties = []identifier.Identifier{internalCore.NamingPropID}

	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: internalCore.NamingPropID},
					String:    "Some Name",
				},
			},
			Identifier: []document.IdentifierClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: codeProp},
					Value:     "Q42",
				},
			},
		},
	}
	addInstanceOf(doc, classID, document.HighConfidence)

	ctx := t.Context()
	display, errE := c.makeDisplayStrings(ctx, doc)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, "ID: Q42", display.Display["und"])
}

func TestTemplateNilDoc(t *testing.T) {
	t.Parallel()

	parentRelPropID := identifier.New()
	yearPropID := identifier.New()
	classID := identifier.New()
	classDoc := makeClassDocWithTemplate(classID,
		`Year: {{bestReferenceDoc `+idTmpl(parentRelPropID)+` . | bestAmountString `+idTmpl(yearPropID)+`}}`,
	)

	extraDocs := map[identifier.Identifier]*document.D{
		classID: classDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)
	c.namingProperties = []identifier.Identifier{internalCore.NamingPropID}
	c.LanguageCodes = map[identifier.Identifier]string{}

	// Template tries to follow a non-existent relation.
	// bestReferenceDoc returns nil, bestAmountString handles nil doc gracefully.
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: internalCore.NamingPropID},
					String:    "Test",
				},
			},
		},
	}
	addInstanceOf(doc, classID, document.HighConfidence)

	ctx := t.Context()
	display, errE := c.makeDisplayStrings(ctx, doc)
	require.NoError(t, errE, "% -+#.1v", errE)
	// Template renders with empty string for nil doc, trailing space trimmed.
	assert.Equal(t, "Year:", display.Display["und"])
}

func TestTemplateBestTimeString(t *testing.T) {
	t.Parallel()

	datePropID := identifier.New()
	classID := identifier.New()
	classDoc := makeClassDocWithTemplate(classID, `Date: {{bestTimeString `+idTmpl(datePropID)+` .}}`)

	extraDocs := map[identifier.Identifier]*document.D{
		classID: classDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)
	c.namingProperties = []identifier.Identifier{internalCore.NamingPropID}

	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: internalCore.NamingPropID},
					String:    "Some Name",
				},
			},
			Time: []document.TimeClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: datePropID},
					Time:      document.Time("2024-06-15"),
					Precision: document.TimePrecisionDay,
				},
			},
		},
	}
	addInstanceOf(doc, classID, document.HighConfidence)

	ctx := t.Context()
	display, errE := c.makeDisplayStrings(ctx, doc)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, "Date: 2024-06-15", display.Display["und"])
}

func TestTemplateGetDocumentByMnemonic(t *testing.T) {
	t.Parallel()

	otherDocID := identifier.New()
	classID := identifier.New()
	otherDoc := &document.D{
		CoreDocument: document.CoreDocument{ID: otherDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: internalCore.NamingPropID},
					String:    "Other Document",
				},
			},
		},
	}
	classDoc := makeClassDocWithTemplate(classID,
		`{{getDocument `+idTmpl(otherDocID)+` | bestString `+idTmpl(internalCore.NamingPropID)+`}}`,
	)

	extraDocs := map[identifier.Identifier]*document.D{
		otherDocID: otherDoc,
		classID:    classDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)
	c.namingProperties = []identifier.Identifier{internalCore.NamingPropID}
	c.LanguageCodes = map[identifier.Identifier]string{}

	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: internalCore.NamingPropID},
					String:    "My Doc",
				},
			},
		},
	}
	addInstanceOf(doc, classID, document.HighConfidence)

	ctx := t.Context()
	display, errE := c.makeDisplayStrings(ctx, doc)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, "Other Document", display.Display["und"])
}

func TestDisplayLabelTemplatePerLanguage(t *testing.T) {
	t.Parallel()

	enLangID := identifier.New()
	slLangID := identifier.New()
	classID := identifier.New()

	// Class with separate templates for English and Slovenian.
	classDoc := makeClassDocWithTemplates(classID, map[identifier.Identifier]string{
		enLangID: `EN: {{bestString "NAME" .}}`,
		slLangID: `SL: {{bestString "NAME" .}}`,
	})

	languages := []*document.D{
		makeLanguageDoc(enLangID, "en"),
		makeLanguageDoc(slLangID, "sl"),
	}
	extraDocs := map[identifier.Identifier]*document.D{
		classID: classDoc,
	}
	c := newTestConverter(t, nil, languages, extraDocs)

	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
	}
	addInstanceOf(doc, classID, document.HighConfidence)

	ctx := t.Context()
	result, errE := c.displayLabelTemplate(ctx, doc)
	require.NoError(t, errE, "% -+#.1v", errE)
	// English gets the English template.
	assert.Equal(t, `EN: {{bestString "NAME" .}}`, result["en"])
	// Slovenian gets the Slovenian template.
	assert.Equal(t, `SL: {{bestString "NAME" .}}`, result["sl"])
	// Portuguese and undetermined have no specific template and no "und" fallback template.
	assert.Empty(t, result["pt"])
	assert.Empty(t, result["und"])
}

func TestDisplayLabelTemplatePerLanguageWithFallback(t *testing.T) {
	t.Parallel()

	enLangID := identifier.New()
	classID := identifier.New()

	// Class with only an English template.
	classDoc := makeClassDocWithTemplateInLanguage(classID, `EN-only template`, enLangID)

	languages := []*document.D{
		makeLanguageDoc(enLangID, "en"),
	}
	extraDocs := map[identifier.Identifier]*document.D{
		classID: classDoc,
	}
	c := newTestConverter(t, nil, languages, extraDocs)

	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
	}
	addInstanceOf(doc, classID, document.HighConfidence)

	ctx := t.Context()
	result, errE := c.displayLabelTemplate(ctx, doc)
	require.NoError(t, errE, "% -+#.1v", errE)
	// English gets its own template.
	assert.Equal(t, "EN-only template", result["en"])
	// Other languages do not fall back to "en" (default fallback is "und", not other languages).
	assert.Empty(t, result["sl"])
	assert.Empty(t, result["pt"])
	assert.Empty(t, result["und"])
}

func TestDisplayLabelTemplateUndAndPerLanguage(t *testing.T) {
	t.Parallel()

	enLangID := identifier.New()
	classID := identifier.New()

	// Class with an undetermined template and an English-specific template.
	claims := &document.ClaimTypes{}
	claims.Reference = append(claims.Reference, document.ReferenceClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: internalCore.InstanceOfPropID},
		To:        document.Reference{ID: internalCore.PropertyClassID},
	})
	// Undetermined template (no IN_LANGUAGE).
	claims.String = append(claims.String, document.StringClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: internalCore.DisplayLabelTemplatePropID},
		String:    "default template",
	})
	// English-specific template.
	enSub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: internalCore.InLanguagePropID},
				To:        document.Reference{ID: enLangID},
			},
		},
	}
	claims.String = append(claims.String, document.StringClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, enSub),
		Prop:      document.Reference{ID: internalCore.DisplayLabelTemplatePropID},
		String:    "english template",
	})
	classDoc := &document.D{
		CoreDocument: document.CoreDocument{ID: classID}, //nolint:exhaustruct
		Claims:       claims,
	}

	languages := []*document.D{
		makeLanguageDoc(enLangID, "en"),
	}
	extraDocs := map[identifier.Identifier]*document.D{
		classID: classDoc,
	}
	c := newTestConverter(t, nil, languages, extraDocs)

	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
	}
	addInstanceOf(doc, classID, document.HighConfidence)

	ctx := t.Context()
	result, errE := c.displayLabelTemplate(ctx, doc)
	require.NoError(t, errE, "% -+#.1v", errE)
	// English gets its own specific template (first in fallback chain).
	assert.Equal(t, "english template", result["en"])
	// Other languages fall back to "und" template.
	assert.Equal(t, "default template", result["sl"])
	assert.Equal(t, "default template", result["pt"])
	assert.Equal(t, "default template", result["und"])
}

func TestMakeDisplayStringsPerLanguageTemplate(t *testing.T) {
	t.Parallel()

	enLangID := identifier.New()
	slLangID := identifier.New()
	shortNamePropID := identifier.New()
	classID := identifier.New()

	// Class with English and Slovenian templates using different formats.
	classDoc := makeClassDocWithTemplates(classID, map[identifier.Identifier]string{
		enLangID: `{{bestString ` + idTmpl(shortNamePropID) + ` .}} (EN)`,
		slLangID: `{{bestString ` + idTmpl(shortNamePropID) + ` .}} (SL)`,
	})

	languages := []*document.D{
		makeLanguageDoc(enLangID, "en"),
		makeLanguageDoc(slLangID, "sl"),
	}
	extraDocs := map[identifier.Identifier]*document.D{
		classID: classDoc,
	}
	c := newTestConverter(t, nil, languages, extraDocs)
	c.namingProperties = []identifier.Identifier{internalCore.NamingPropID}

	enSub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: internalCore.InLanguagePropID},
				To:        document.Reference{ID: enLangID},
			},
		},
	}
	slSub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: internalCore.InLanguagePropID},
				To:        document.Reference{ID: slLangID},
			},
		},
	}

	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, enSub),
					Prop:      document.Reference{ID: shortNamePropID},
					String:    "Short",
				},
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, slSub),
					Prop:      document.Reference{ID: shortNamePropID},
					String:    "Kratko",
				},
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: internalCore.NamingPropID},
					String:    "Full Name",
				},
			},
		},
	}
	addInstanceOf(doc, classID, document.HighConfidence)

	ctx := t.Context()
	display, errE := c.makeDisplayStrings(ctx, doc)
	require.NoError(t, errE, "% -+#.1v", errE)
	// English and Slovenian use their per-language templates.
	assert.Equal(t, "Short (EN)", display.Display["en"])
	assert.Equal(t, "Kratko (SL)", display.Display["sl"])
	// Portuguese and undetermined have no template, fall back to naming strings.
	assert.Equal(t, "Full Name", display.Display["pt"])
	assert.Equal(t, "Full Name", display.Display["und"])
}

func TestMakeDisplayStringsPerLanguageTemplateFallbackToNaming(t *testing.T) {
	t.Parallel()

	enLangID := identifier.New()
	classID := identifier.New()

	// Class with only an English template.
	classDoc := makeClassDocWithTemplateInLanguage(classID,
		`English Only`, enLangID)

	languages := []*document.D{
		makeLanguageDoc(enLangID, "en"),
	}
	extraDocs := map[identifier.Identifier]*document.D{
		classID: classDoc,
	}
	c := newTestConverter(t, nil, languages, extraDocs)
	c.namingProperties = []identifier.Identifier{internalCore.NamingPropID}

	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: internalCore.NamingPropID},
					String:    "Naming String",
				},
			},
		},
	}
	addInstanceOf(doc, classID, document.HighConfidence)

	ctx := t.Context()
	display, errE := c.makeDisplayStrings(ctx, doc)
	require.NoError(t, errE, "% -+#.1v", errE)
	// English uses the template.
	assert.Equal(t, "English Only", display.Display["en"])
	// Other languages have no template, fall back to naming strings.
	assert.Equal(t, "Naming String", display.Display["sl"])
	assert.Equal(t, "Naming String", display.Display["pt"])
	assert.Equal(t, "Naming String", display.Display["und"])
	// Naming is always populated independently.
	assert.Equal(t, []string{"Naming String"}, display.Naming["und"])
}

func TestMakeDisplayStringsPerLanguageTemplateWithUndFallback(t *testing.T) {
	t.Parallel()

	enLangID := identifier.New()
	shortNamePropID := identifier.New()
	classID := identifier.New()

	// Class with an English template and an undetermined (default) template.
	claims := &document.ClaimTypes{}
	claims.Reference = append(claims.Reference, document.ReferenceClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: internalCore.InstanceOfPropID},
		To:        document.Reference{ID: internalCore.PropertyClassID},
	})
	// English-specific template.
	enSub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: internalCore.InLanguagePropID},
				To:        document.Reference{ID: enLangID},
			},
		},
	}
	claims.String = append(claims.String, document.StringClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, enSub),
		Prop:      document.Reference{ID: internalCore.DisplayLabelTemplatePropID},
		String:    `EN: {{bestString ` + idTmpl(shortNamePropID) + ` .}}`,
	})
	// Undetermined template (no IN_LANGUAGE).
	claims.String = append(claims.String, document.StringClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: internalCore.DisplayLabelTemplatePropID},
		String:    `{{bestString ` + idTmpl(shortNamePropID) + ` .}}`,
	})
	classDoc := &document.D{
		CoreDocument: document.CoreDocument{ID: classID}, //nolint:exhaustruct
		Claims:       claims,
	}

	languages := []*document.D{
		makeLanguageDoc(enLangID, "en"),
	}
	extraDocs := map[identifier.Identifier]*document.D{
		classID: classDoc,
	}
	c := newTestConverter(t, nil, languages, extraDocs)
	c.namingProperties = []identifier.Identifier{internalCore.NamingPropID}

	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: shortNamePropID},
					String:    "Val",
				},
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: internalCore.NamingPropID},
					String:    "Full Name",
				},
			},
		},
	}
	addInstanceOf(doc, classID, document.HighConfidence)

	ctx := t.Context()
	display, errE := c.makeDisplayStrings(ctx, doc)
	require.NoError(t, errE, "% -+#.1v", errE)
	// English uses its specific template.
	assert.Equal(t, "EN: Val", display.Display["en"])
	// Other languages fall back to the "und" template.
	assert.Equal(t, "Val", display.Display["sl"])
	assert.Equal(t, "Val", display.Display["pt"])
	assert.Equal(t, "Val", display.Display["und"])
}

func TestValidateLanguagePriority(t *testing.T) {
	t.Parallel()

	// Valid priority.
	errE := validateLanguagePriority(map[string][]string{
		"en": {"sl", "und"},
		"sl": {"en"},
	})
	require.NoError(t, errE, "% -+#.1v", errE)

	// Nil priority is valid.
	errE = validateLanguagePriority(nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Empty priority is valid.
	errE = validateLanguagePriority(map[string][]string{})
	require.NoError(t, errE, "% -+#.1v", errE)

	// Empty fallback list is valid (means no fallback at all).
	errE = validateLanguagePriority(map[string][]string{
		"en": {},
	})
	require.NoError(t, errE, "% -+#.1v", errE)

	// "und" as key is valid.
	errE = validateLanguagePriority(map[string][]string{
		"und": {"en"},
	})
	require.NoError(t, errE, "% -+#.1v", errE)

	// Unsupported language in key.
	errE = validateLanguagePriority(map[string][]string{
		"xx": {"en"},
	})
	assert.EqualError(t, errE, "unsupported language in priority key")

	// Unsupported language in fallback.
	errE = validateLanguagePriority(map[string][]string{
		"en": {"xx"},
	})
	assert.EqualError(t, errE, "unsupported language in priority fallback")

	// Language as its own fallback.
	errE = validateLanguagePriority(map[string][]string{
		"en": {"sl", "en"},
	})
	assert.EqualError(t, errE, "language cannot be its own fallback")
}

func TestNewConverterValidation(t *testing.T) {
	t.Parallel()

	getDocument := func(_ context.Context, _ identifier.Identifier) (*document.D, errors.E) {
		return nil, errors.New("not found")
	}

	// Valid priority.
	c, errE := NewConverter(nil, nil, nil, map[string][]string{"en": {"sl"}}, getDocument)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.NotNil(t, c)

	// Invalid priority.
	c, errE = NewConverter(nil, nil, nil, map[string][]string{"xx": {"en"}}, getDocument)
	assert.EqualError(t, errE, "unsupported language in priority key")
	assert.Nil(t, c)
}

func TestGetDocumentInfoWithLanguagePriority(t *testing.T) {
	t.Parallel()

	enLangID := identifier.New()
	slLangID := identifier.New()

	enSub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: internalCore.InLanguagePropID},
				To:        document.Reference{ID: enLangID},
			},
		},
	}
	slSub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: internalCore.InLanguagePropID},
				To:        document.Reference{ID: slLangID},
			},
		},
	}

	docID := identifier.New()
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: docID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, enSub),
					Prop:      document.Reference{ID: internalCore.NamingPropID},
					String:    "English Name",
				},
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, slSub),
					Prop:      document.Reference{ID: internalCore.NamingPropID},
					String:    "Slovensko Ime",
				},
			},
		},
	}

	languages := []*document.D{
		makeLanguageDoc(enLangID, "en"),
		makeLanguageDoc(slLangID, "sl"),
	}

	extraDocs := map[identifier.Identifier]*document.D{
		docID: doc,
	}

	// Priority: en falls back to sl, sl enabled without fallback, pt enabled
	// without fallback. All three appear as keys so they are in the enabled
	// language set (LanguagePriority keys + "und" = enabled languages).
	priority := map[string][]string{
		"en": {"sl", "und"},
		"sl": {},
		"pt": {},
	}
	c := newTestConverterWithPriority(t, nil, languages, extraDocs, priority)

	ctx := t.Context()
	info, errE := c.getDocumentInfo(ctx, docID)
	require.NoError(t, errE, "% -+#.1v", errE)

	// "en" resolved directly.
	assert.Equal(t, "English Name", info.Display.Display["en"])
	// "sl" resolved directly.
	assert.Equal(t, "Slovensko Ime", info.Display.Display["sl"])
	// "pt" has empty fallback: no display.
	assert.Empty(t, info.Display.Display["pt"])
	// "und" not in priority: no fallback for "und" itself (no "und" naming exists).
	assert.Empty(t, info.Display.Display["und"])
}

// TestRecognizedNotIndexedLanguageFallback verifies that a language listed only as a
// fallback target (here "sl" in {en: {sl}}) is recognized but not indexed: its content
// resolves the display label of the language that falls back to it, but the raw content
// is dropped from the text buckets (no per-language field and not folded into "und").
func TestRecognizedNotIndexedLanguageFallback(t *testing.T) {
	t.Parallel()

	enLangID := identifier.New()
	slLangID := identifier.New()
	enLangDoc := makeLanguageDoc(enLangID, "en")
	slLangDoc := makeLanguageDoc(slLangID, "sl")

	slSub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{{
			CoreClaim: makeCoreClaim(document.HighConfidence, nil),
			Prop:      document.Reference{ID: internalCore.InLanguagePropID},
			To:        document.Reference{ID: slLangID},
		}},
	}

	docID := identifier.New()
	// The document has only a Slovenian-tagged name.
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: docID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			String: []document.StringClaim{{
				CoreClaim: makeCoreClaim(document.HighConfidence, slSub),
				Prop:      document.Reference{ID: internalCore.NamingPropID},
				String:    "Slovensko Ime",
			}},
		},
	}

	// "en" is enabled and indexed; "sl" is only a fallback target, so it is
	// recognized but not indexed.
	priority := map[string][]string{"en": {"sl"}}
	c := newTestConverterWithPriority(t, nil, []*document.D{enLangDoc, slLangDoc}, map[identifier.Identifier]*document.D{docID: doc}, priority)

	result, errE := c.FromDocument(t.Context(), doc, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	// The English display label resolves to the Slovenian name via the en->sl
	// fallback, even though "sl" is not indexed.
	assert.Equal(t, []string{"Slovensko Ime"}, result.Display["en"])

	// The Slovenian content is dropped from text: there is no "sl" bucket, and it
	// is not folded into "und". Only the document ID lands in "und".
	_, hasSl := result.Text["sl"]
	assert.False(t, hasSl)
	assert.Equal(t, map[string][]string{"und": {docID.String()}}, result.Text)
}

// TestConversionIndexesOnlyEnabledLanguages verifies that when LanguagePriority
// restricts the enabled languages, FromDocument produces per-language text only
// for those languages: content tagged in a non-enabled language collapses to
// "und" and no bucket is created for the non-enabled language.
func TestConversionIndexesOnlyEnabledLanguages(t *testing.T) {
	t.Parallel()

	enLangID := identifier.New()
	slLangID := identifier.New()
	ptLangID := identifier.New()
	langs := []*document.D{
		makeLanguageDoc(enLangID, "en"),
		makeLanguageDoc(slLangID, "sl"),
		makeLanguageDoc(ptLangID, "pt"),
	}

	inLang := func(langID identifier.Identifier) *document.ClaimTypes {
		return &document.ClaimTypes{
			Reference: []document.ReferenceClaim{{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: internalCore.InLanguagePropID},
				To:        document.Reference{ID: langID},
			}},
		}
	}

	docID := identifier.New()
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: docID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			String: []document.StringClaim{
				{CoreClaim: makeCoreClaim(document.HighConfidence, inLang(enLangID)), Prop: document.Reference{ID: testPropID}, String: "english"},
				{CoreClaim: makeCoreClaim(document.HighConfidence, inLang(slLangID)), Prop: document.Reference{ID: testPropID}, String: "slovensko"},
				{CoreClaim: makeCoreClaim(document.HighConfidence, inLang(ptLangID)), Prop: document.Reference{ID: testPropID}, String: "portuguese"},
			},
		},
	}

	// Only "en" is enabled (plus "und"); sl and pt are neither keys nor fallback
	// targets, so they are not recognized.
	priority := map[string][]string{"en": {"und"}}
	c := newTestConverterWithPriority(t, nil, langs, map[identifier.Identifier]*document.D{docID: doc}, priority)

	result, errE := c.FromDocument(t.Context(), doc, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	// English content is indexed under "en".
	assert.Equal(t, []string{"english"}, result.Text["en"])
	// sl and pt are not enabled, so no buckets exist for them.
	_, hasSl := result.Text["sl"]
	_, hasPt := result.Text["pt"]
	assert.False(t, hasSl, "non-enabled language sl should not be indexed")
	assert.False(t, hasPt, "non-enabled language pt should not be indexed")
	// Their content collapses into "und", alongside the document ID.
	assert.ElementsMatch(t, []string{docID.String(), "slovensko", "portuguese"}, result.Text["und"])
}

// TestDetectLanguageRoutesUntaggedContent verifies that, with DetectLanguages enabled, a long
// untagged String claim whose content is clearly one enabled language is routed to that
// language's text bucket instead of "und", while short content stays in "und".
func TestDetectLanguageRoutesUntaggedContent(t *testing.T) {
	t.Parallel()

	// Default priority enables en/sl/pt/und, so the detector covers en, sl, pt.
	c := newTestConverter(t, nil, nil, map[identifier.Identifier]*document.D{})
	c.DetectLanguages = true

	ctx := t.Context()

	// A clearly-English sentence (no IN_LANGUAGE) should land in text.en.
	enText := "The quick brown fox jumps over the lazy dog near the river."
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: identifier.New()}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			String: []document.StringClaim{{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: testPropID},
				String:    enText,
			}},
		},
	}
	result, errE := c.FromDocument(ctx, doc, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Contains(t, result.Text["en"], enText, "clearly-English content should route to text.en")
	assert.NotContains(t, result.Text["und"], enText, "detected content should not also land in und")

	// A short untagged value stays in "und" (below the detection length guard).
	short := "hello"
	shortDoc := &document.D{
		CoreDocument: document.CoreDocument{ID: identifier.New()}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			String: []document.StringClaim{{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: testPropID},
				String:    short,
			}},
		},
	}
	result, errE = c.FromDocument(ctx, shortDoc, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Contains(t, result.Text["und"], short, "short content stays in und")
	_, hasEn := result.Text["en"]
	assert.False(t, hasEn, "short content should not be language-detected")
}

// TestDetectLanguageSingleEnabled verifies that, when a site enables exactly one
// detector-supported language, untagged content is scored directly against that language with a
// confidence threshold: content that matches routes to it, while content that clearly does not
// (here Slovenian on an English-only site) stays in "und".
func TestDetectLanguageSingleEnabled(t *testing.T) {
	t.Parallel()

	// Only English is enabled, so detection uses the single-language confidence path.
	c := newTestConverterWithPriority(t, nil, nil, map[identifier.Identifier]*document.D{}, map[string][]string{"en": {}})
	c.DetectLanguages = true

	ctx := t.Context()

	enText := "The quick brown fox jumps over the lazy dog near the river."
	enDoc := &document.D{
		CoreDocument: document.CoreDocument{ID: identifier.New()}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			String: []document.StringClaim{{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: testPropID},
				String:    enText,
			}},
		},
	}
	result, errE := c.FromDocument(ctx, enDoc, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Contains(t, result.Text["en"], enText, "clearly-English content should route to text.en")
	assert.NotContains(t, result.Text["und"], enText, "detected content should not also land in und")

	// Clearly-Slovenian content on an English-only site does not match English well enough and
	// stays in "und" rather than being forced into the English analyzer.
	slText := "Danes je lep sončen dan in ptice veselo prepevajo na visokih drevesih ob reki."
	slDoc := &document.D{
		CoreDocument: document.CoreDocument{ID: identifier.New()}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			String: []document.StringClaim{{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: testPropID},
				String:    slText,
			}},
		},
	}
	result, errE = c.FromDocument(ctx, slDoc, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Contains(t, result.Text["und"], slText, "non-English content stays in und on an English-only site")
	_, hasEn := result.Text["en"]
	assert.False(t, hasEn, "non-English content should not route to text.en")
}

func TestDisplayPathsNoFallback(t *testing.T) {
	t.Parallel()

	enLangID := identifier.New()
	slLangID := identifier.New()

	enSub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: internalCore.InLanguagePropID},
				To:        document.Reference{ID: enLangID},
			},
		},
	}
	slSub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: internalCore.InLanguagePropID},
				To:        document.Reference{ID: slLangID},
			},
		},
	}

	// Set up hierarchy.
	subentityDoc := makePropertyDoc(internalCore.SubentityOfPropID, nil)
	subclassDoc := makePropertyDoc(internalCore.SubclassOfPropID, &internalCore.SubentityOfPropID)
	subpropDoc := makePropertyDoc(internalCore.SubpropertyOfPropID, &internalCore.SubentityOfPropID)
	instanceDoc := makePropertyDoc(internalCore.InstanceOfPropID, &internalCore.SubentityOfPropID)
	properties := []*document.D{subentityDoc, subclassDoc, subpropDoc, instanceDoc}

	parentID := identifier.New()
	childID := identifier.New()

	// Parent has only English name.
	parentDoc := &document.D{
		CoreDocument: document.CoreDocument{ID: parentID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, enSub),
					Prop:      document.Reference{ID: internalCore.NamingPropID},
					String:    "Parent EN",
				},
			},
		},
	}

	// Child has English and Slovenian names, and SUBCLASS_OF parent.
	childDoc := &document.D{
		CoreDocument: document.CoreDocument{ID: childID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, enSub),
					Prop:      document.Reference{ID: internalCore.NamingPropID},
					String:    "Child EN",
				},
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, slSub),
					Prop:      document.Reference{ID: internalCore.NamingPropID},
					String:    "Child SL",
				},
			},
			Reference: []document.ReferenceClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: internalCore.SubclassOfPropID},
					To:        document.Reference{ID: parentID},
				},
			},
		},
	}

	languages := []*document.D{
		makeLanguageDoc(enLangID, "en"),
		makeLanguageDoc(slLangID, "sl"),
	}

	extraDocs := map[identifier.Identifier]*document.D{
		parentID:                         parentDoc,
		childID:                          childDoc,
		internalCore.SubentityOfPropID:   subentityDoc,
		internalCore.SubclassOfPropID:    subclassDoc,
		internalCore.SubpropertyOfPropID: subpropDoc,
		internalCore.InstanceOfPropID:    instanceDoc,
		enLangID:                         makeLanguageDoc(enLangID, "en"),
		slLangID:                         makeLanguageDoc(slLangID, "sl"),
		//nolint:exhaustruct
		internalCore.PropertyClassID: {CoreDocument: document.CoreDocument{ID: internalCore.PropertyClassID}},
	}

	// Priority: en enabled (no fallback), sl falls back to en, pt enabled with
	// no fallback. All three appear as keys so they are part of the enabled
	// language set (LanguagePriority keys + "und" = enabled languages).
	priority := map[string][]string{
		"en": {},
		"sl": {"en"},
		"pt": {},
	}
	c := newTestConverterWithPriority(t, properties, languages, extraDocs, priority)

	ctx := t.Context()
	info, errE := c.getDocumentInfo(ctx, childID)
	require.NoError(t, errE, "% -+#.1v", errE)

	// English path exists: parent has EN, child has EN.
	require.Contains(t, info.DisplayPaths[internalCore.SubclassOfPropID], "en")
	assert.Equal(t, []string{"Parent EN\x00Child EN"}, info.DisplayPaths[internalCore.SubclassOfPropID]["en"])

	// Slovenian path: parent resolved to "Parent EN" via sl->en fallback, child has "Child SL".
	require.Contains(t, info.DisplayPaths[internalCore.SubclassOfPropID], "sl")
	assert.Equal(t, []string{"Parent EN\x00Child SL"}, info.DisplayPaths[internalCore.SubclassOfPropID]["sl"])

	// Portuguese: pt has no fallback, both parent and child have empty pt display.
	// Path is still created with empty strings.
	require.Contains(t, info.DisplayPaths[internalCore.SubclassOfPropID], "pt")
	assert.Equal(t, []string{"\x00"}, info.DisplayPaths[internalCore.SubclassOfPropID]["pt"])

	// FromDocument folds the document's own ancestor display labels into the
	// per-language display field: its own label plus its ancestors' labels.
	// ("Parent EN" also appears in text independently, via the SUBCLASS_OF
	// reference claim's ToDisplay/ToNaming, which is a separate mechanism.)
	result, errE := c.FromDocument(ctx, childDoc, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.ElementsMatch(t, []string{"Child EN", "Parent EN"}, result.Display["en"])
	assert.ElementsMatch(t, []string{"Child SL", "Parent EN"}, result.Display["sl"])
}

func TestDisplayPathsEmptyAppend(t *testing.T) {
	t.Parallel()

	enLangID := identifier.New()

	enSub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: internalCore.InLanguagePropID},
				To:        document.Reference{ID: enLangID},
			},
		},
	}

	// Set up hierarchy.
	subentityDoc := makePropertyDoc(internalCore.SubentityOfPropID, nil)
	subclassDoc := makePropertyDoc(internalCore.SubclassOfPropID, &internalCore.SubentityOfPropID)
	subpropDoc := makePropertyDoc(internalCore.SubpropertyOfPropID, &internalCore.SubentityOfPropID)
	instanceDoc := makePropertyDoc(internalCore.InstanceOfPropID, &internalCore.SubentityOfPropID)
	properties := []*document.D{subentityDoc, subclassDoc, subpropDoc, instanceDoc}

	parentID := identifier.New()
	childID := identifier.New()

	// Parent has English name.
	parentDoc := &document.D{
		CoreDocument: document.CoreDocument{ID: parentID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, enSub),
					Prop:      document.Reference{ID: internalCore.NamingPropID},
					String:    "Parent EN",
				},
			},
		},
	}

	// Child has NO English name, only undetermined.
	childDoc := &document.D{
		CoreDocument: document.CoreDocument{ID: childID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: internalCore.NamingPropID},
					String:    "Child UND",
				},
			},
			Reference: []document.ReferenceClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: internalCore.SubclassOfPropID},
					To:        document.Reference{ID: parentID},
				},
			},
		},
	}

	languages := []*document.D{
		makeLanguageDoc(enLangID, "en"),
	}

	extraDocs := map[identifier.Identifier]*document.D{
		parentID: parentDoc,
		childID:  childDoc,
	}

	// en has empty fallback: no fallback, not even "und".
	priority := map[string][]string{
		"en": {},
	}
	c := newTestConverterWithPriority(t, properties, languages, extraDocs, priority)

	ctx := t.Context()
	info, errE := c.getDocumentInfo(ctx, childID)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Parent has "en" display. Child has no "en" display (empty fallback means no resolution).
	// Path should still be created with empty child display appended.
	require.Contains(t, info.DisplayPaths[internalCore.SubclassOfPropID], "en")
	assert.Equal(t, []string{"Parent EN\x00"}, info.DisplayPaths[internalCore.SubclassOfPropID]["en"])
}

func TestBestStringLanguagePriority(t *testing.T) {
	t.Parallel()

	enLangID := identifier.New()
	slLangID := identifier.New()
	ptLangID := identifier.New()
	namePropID := identifier.New()
	classID := identifier.New()
	classDoc := makeClassDocWithTemplate(classID, `{{bestString `+idTmpl(namePropID)+` .}}`)

	enSub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: internalCore.InLanguagePropID},
				To:        document.Reference{ID: enLangID},
			},
		},
	}
	slSub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: internalCore.InLanguagePropID},
				To:        document.Reference{ID: slLangID},
			},
		},
	}
	ptSub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: internalCore.InLanguagePropID},
				To:        document.Reference{ID: ptLangID},
			},
		},
	}

	languages := []*document.D{
		makeLanguageDoc(enLangID, "en"),
		makeLanguageDoc(slLangID, "sl"),
		makeLanguageDoc(ptLangID, "pt"),
	}
	extraDocs := map[identifier.Identifier]*document.D{
		classID: classDoc,
	}
	// pt falls back to sl then und.
	priority := map[string][]string{
		"pt": {"sl", "und"},
	}
	c := newTestConverterWithPriority(t, nil, languages, extraDocs, priority)
	c.namingProperties = []identifier.Identifier{internalCore.NamingPropID}

	// Document with template (via class). NAME only exists in "sl".
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, ptSub),
					Prop:      document.Reference{ID: internalCore.NamingPropID},
					String:    "Nome PT",
				},
				{
					// NAME claim only in Slovenian.
					CoreClaim: makeCoreClaim(document.HighConfidence, slSub),
					Prop:      document.Reference{ID: namePropID},
					String:    "Slovenian Value",
				},
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, enSub),
					Prop:      document.Reference{ID: namePropID},
					String:    "English Value",
				},
			},
		},
	}
	addInstanceOf(doc, classID, document.HighConfidence)

	ctx := t.Context()
	display, errE := c.makeDisplayStrings(ctx, doc)
	require.NoError(t, errE, "% -+#.1v", errE)
	// Template bestString for "pt" falls back sl -> und.
	// "sl" has a NAME claim, so it should find "Slovenian Value".
	assert.Equal(t, "Slovenian Value", display.Display["pt"])
}

func TestBuildInverseProperties(t *testing.T) {
	t.Parallel()

	propA := identifier.New()
	propB := identifier.New()

	propADoc := makePropertyDocFull(propA, nil, &propB)
	propBDoc := makePropertyDocFull(propB, nil, nil)

	c := &Converter{}
	c.buildInverseProperties([]*document.D{propADoc, propBDoc})

	// A has inversePropertyOf B, so both directions should be recorded.
	assert.Contains(t, c.inverseProperties[propA], propB)
	assert.Contains(t, c.inverseProperties[propB], propA)
}

func TestBuildInversePropertiesNoDuplicates(t *testing.T) {
	t.Parallel()

	// Both A and B declare inversePropertyOf each other.
	propA := identifier.New()
	propB := identifier.New()

	propADoc := makePropertyDocFull(propA, nil, &propB)
	propBDoc := makePropertyDocFull(propB, nil, &propA)

	c := &Converter{}
	c.buildInverseProperties([]*document.D{propADoc, propBDoc})

	// Each should appear exactly once, not duplicated.
	assert.Len(t, c.inverseProperties[propA], 1)
	assert.Len(t, c.inverseProperties[propB], 1)
	assert.Contains(t, c.inverseProperties[propA], propB)
	assert.Contains(t, c.inverseProperties[propB], propA)
}

func TestBuildInversePropertiesMultiple(t *testing.T) {
	t.Parallel()

	// Both A and C have inversePropertyOf B.
	propA := identifier.New()
	propB := identifier.New()
	propC := identifier.New()

	propADoc := makePropertyDocFull(propA, nil, &propB)
	propBDoc := makePropertyDocFull(propB, nil, nil)
	propCDoc := makePropertyDocFull(propC, nil, &propB)

	c := &Converter{}
	c.buildInverseProperties([]*document.D{propADoc, propBDoc, propCDoc})

	// B should map to both A and C.
	assert.Contains(t, c.inverseProperties[propB], propA)
	assert.Contains(t, c.inverseProperties[propB], propC)
	// A maps to B.
	assert.Contains(t, c.inverseProperties[propA], propB)
	assert.Len(t, c.inverseProperties[propA], 1)
	// C maps to B.
	assert.Contains(t, c.inverseProperties[propC], propB)
	assert.Len(t, c.inverseProperties[propC], 1)
}

func TestBuildInversePropertiesSkipsNonProperties(t *testing.T) {
	t.Parallel()

	propA := identifier.New()
	propB := identifier.New()

	// A document that is NOT a property (no INSTANCE_OF -> PROPERTY) but has inversePropertyOf.
	claims := &document.ClaimTypes{}
	claims.Reference = append(claims.Reference, document.ReferenceClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: internalCore.InversePropertyOfPropID},
		To:        document.Reference{ID: propB},
	})
	notAProp := &document.D{
		CoreDocument: document.CoreDocument{ID: propA}, //nolint:exhaustruct
		Claims:       claims,
	}

	c := &Converter{}
	c.buildInverseProperties([]*document.D{notAProp})

	assert.Empty(t, c.inverseProperties)
}

func TestOutgoingInverseRelations(t *testing.T) {
	t.Parallel()

	// Property testPropID has inverse testPropID2.
	propDoc := makePropertyDocFull(testPropID, nil, &testPropID2)
	propDoc2 := makePropertyDocFull(testPropID2, nil, nil)
	c := newTestConverter(t, []*document.D{propDoc, propDoc2}, nil, nil)

	claimID := identifier.New()
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			Reference: []document.ReferenceClaim{
				{
					CoreClaim: document.CoreClaim{
						ID:         claimID,
						Confidence: document.HighConfidence,
					},
					Prop: document.Reference{ID: testPropID},
					To:   document.Reference{ID: testTargetDocID},
				},
			},
		},
	}

	outgoing, errE := c.OutgoingInverseRelations(t.Context(), doc)
	require.NoError(t, errE)

	// Should have an entry for the target document.
	require.Contains(t, outgoing, testTargetDocID)
	require.Len(t, outgoing[testTargetDocID], 1)
	ir := outgoing[testTargetDocID][0]
	assert.Equal(t, claimID, ir.Claim)
	assert.Equal(t, testDocID, ir.Source)
	assert.Equal(t, testPropID, ir.SourceProp)
	assert.Equal(t, testPropID2, ir.TargetProp)
	assert.Equal(t, float64(document.HighConfidence), float64(ir.Confidence)) //nolint:testifylint
}

func TestOutgoingInverseRelationsHierarchyExpansion(t *testing.T) {
	t.Parallel()

	// containedIn is a sub-property of SUBENTITY_OF, so it defines a value hierarchy and
	// targets referenced through it expand to their ancestors.
	containedIn := identifier.New()
	subentityDoc := makePropertyDoc(internalCore.SubentityOfPropID, nil)
	containedInDoc := makePropertyDoc(containedIn, &internalCore.SubentityOfPropID)

	// hasLocation has inverse inLocation.
	hasLocation := identifier.New()
	inLocation := identifier.New()
	hasLocationDoc := makePropertyDocFull(hasLocation, nil, &inLocation)
	inLocationDoc := makePropertyDoc(inLocation, nil)

	properties := []*document.D{subentityDoc, containedInDoc, hasLocationDoc, inLocationDoc}

	// city is contained in country.
	country := identifier.New()
	city := identifier.New()
	extraDocs := map[identifier.Identifier]*document.D{
		city:    makeHierarchyDoc(city, "City", containedIn, &country),
		country: makeHierarchyDoc(country, "Country", containedIn, nil),
	}
	c := newTestConverter(t, properties, nil, extraDocs)

	// Source document references the city via hasLocation.
	claimID := identifier.New()
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			Reference: []document.ReferenceClaim{
				{
					CoreClaim: document.CoreClaim{ID: claimID, Confidence: document.HighConfidence},
					Prop:      document.Reference{ID: hasLocation},
					To:        document.Reference{ID: city},
				},
			},
		},
	}

	outgoing, errE := c.OutgoingInverseRelations(t.Context(), doc)
	require.NoError(t, errE, "% -+#.1v", errE)

	// The inverse relation lands on both the direct target (city) and its ancestor (country).
	require.Contains(t, outgoing, city)
	require.Contains(t, outgoing, country)
	require.Len(t, outgoing[city], 1)
	require.Len(t, outgoing[country], 1)

	assert.Equal(t, inLocation, outgoing[city][0].TargetProp)
	assert.Equal(t, city, outgoing[city][0].Target)
	assert.Equal(t, testDocID, outgoing[city][0].Source)
	assert.Equal(t, claimID, outgoing[city][0].Claim)

	assert.Equal(t, inLocation, outgoing[country][0].TargetProp)
	assert.Equal(t, country, outgoing[country][0].Target)
	assert.Equal(t, testDocID, outgoing[country][0].Source)
	assert.Equal(t, claimID, outgoing[country][0].Claim)
}

func TestInverseRelationClaimIDDeterministic(t *testing.T) {
	t.Parallel()

	base := []string{"base"}
	irKey := store.InverseRelationKey{
		Claim:      identifier.New(),
		Source:     identifier.New(),
		TargetProp: identifier.New(),
	}

	id1 := inverseReferenceClaimID(base, irKey)
	id2 := inverseReferenceClaimID(base, irKey)

	assert.Equal(t, id1, id2)
}

func TestInverseRelationClaimIDDiffersPerSource(t *testing.T) {
	t.Parallel()

	base := []string{"base"}
	claim := identifier.New()
	targetProp := identifier.New()
	sourceA := identifier.New()
	sourceB := identifier.New()

	idA := inverseReferenceClaimID(base, store.InverseRelationKey{Claim: claim, Source: sourceA, TargetProp: targetProp})
	idB := inverseReferenceClaimID(base, store.InverseRelationKey{Claim: claim, Source: sourceB, TargetProp: targetProp})

	assert.NotEqual(t, idA, idB)
}

func TestOutgoingInverseRelationsEmpty(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, nil)

	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
	}

	outgoing, errE := c.OutgoingInverseRelations(t.Context(), doc)
	require.NoError(t, errE)
	assert.Empty(t, outgoing)
}

func TestOutgoingInverseRelationsNoInverse(t *testing.T) {
	t.Parallel()

	// Property with no inverse.
	propDoc := makePropertyDocFull(testPropID, nil, nil)
	c := newTestConverter(t, []*document.D{propDoc}, nil, nil)

	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			Reference: []document.ReferenceClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: testPropID},
					To:        document.Reference{ID: testTargetDocID},
				},
			},
		},
	}

	outgoing, errE := c.OutgoingInverseRelations(t.Context(), doc)
	require.NoError(t, errE)

	// No inverse property, so no outgoing relations should be created.
	assert.Empty(t, outgoing)
}

func TestFromDocumentIncomingInverseRelation(t *testing.T) {
	t.Parallel()

	// Property X has inversePropertyOf Y.
	propX := identifier.New()
	propY := identifier.New()
	propXDoc := makePropertyDocFull(propX, nil, &propY)
	propYDoc := makePropertyDocFull(propY, nil, nil)

	sourceDocID := identifier.New()
	sourceDoc := makeNamingDoc(sourceDocID, "Source")
	propXNamingDoc := makeNamingDoc(propX, "Prop X")
	propYNamingDoc := makeNamingDoc(propY, "Prop Y")

	extraDocs := map[identifier.Identifier]*document.D{
		sourceDocID: sourceDoc,
		propX:       propXNamingDoc,
		propY:       propYNamingDoc,
	}
	c := newTestConverter(t, []*document.D{propXDoc, propYDoc}, nil, extraDocs)

	ctx := t.Context()
	claimID := identifier.New()

	// Document B has no claims of its own, but has an incoming inverse relation
	// from source document A via property X.
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
	}
	// TargetProp is pre-resolved: property X has inverse Y.
	inverseRelations := []store.InverseRelation{
		newIR(claimID, sourceDocID, propX, propY, identifier.Identifier{}, document.HighConfidence),
	}

	result, errE := c.FromDocument(ctx, doc, inverseRelations)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Should have a reverse relation claim with property Y pointing to source document.
	require.Len(t, result.Claims.Reference, 1)
	rel := result.Claims.Reference[0]
	assert.Equal(t, propY, rel.Prop)
	assert.Equal(t, sourceDocID, rel.To)
}

func TestFromDocumentIncomingInverseRelationMultipleInverses(t *testing.T) {
	t.Parallel()

	// Both A and C have inversePropertyOf B.
	// Two separate InverseRelation entries (one with TargetProp=A, one with TargetProp=C)
	// should produce two reverse claims.
	propA := identifier.New()
	propB := identifier.New()
	propC := identifier.New()

	propADoc := makePropertyDocFull(propA, nil, &propB)
	propBDoc := makePropertyDocFull(propB, nil, nil)
	propCDoc := makePropertyDocFull(propC, nil, &propB)

	sourceDocID := identifier.New()
	extraDocs := map[identifier.Identifier]*document.D{
		sourceDocID: makeNamingDoc(sourceDocID, "Source"),
		propA:       makeNamingDoc(propA, "Prop A"),
		propB:       makeNamingDoc(propB, "Prop B"),
		propC:       makeNamingDoc(propC, "Prop C"),
	}
	c := newTestConverter(t, []*document.D{propADoc, propBDoc, propCDoc}, nil, extraDocs)

	ctx := t.Context()
	claimID := identifier.New()
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
	}
	inverseRelations := []store.InverseRelation{
		newIR(claimID, sourceDocID, propB, propA, identifier.Identifier{}, document.HighConfidence),
		newIR(claimID, sourceDocID, propB, propC, identifier.Identifier{}, document.HighConfidence),
	}

	result, errE := c.FromDocument(ctx, doc, inverseRelations)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Should have two reverse relation claims: one for A and one for C.
	require.Len(t, result.Claims.Reference, 2)
	relProps := map[identifier.Identifier]bool{
		result.Claims.Reference[0].Prop: true,
		result.Claims.Reference[1].Prop: true,
	}
	assert.True(t, relProps[propA], "expected reverse claim with property A")
	assert.True(t, relProps[propC], "expected reverse claim with property C")
	// Both should point to the source document.
	assert.Equal(t, sourceDocID, result.Claims.Reference[0].To)
	assert.Equal(t, sourceDocID, result.Claims.Reference[1].To)
}

func TestFromDocumentIncomingInverseRelationBidirectional(t *testing.T) {
	t.Parallel()

	// A has inversePropertyOf B. An incoming relation with property A should
	// produce a reverse claim with property B.
	propA := identifier.New()
	propB := identifier.New()

	propADoc := makePropertyDocFull(propA, nil, &propB)
	propBDoc := makePropertyDocFull(propB, nil, nil)

	sourceDocID := identifier.New()
	extraDocs := map[identifier.Identifier]*document.D{
		sourceDocID: makeNamingDoc(sourceDocID, "Source"),
		propA:       makeNamingDoc(propA, "Prop A"),
		propB:       makeNamingDoc(propB, "Prop B"),
	}
	c := newTestConverter(t, []*document.D{propADoc, propBDoc}, nil, extraDocs)

	ctx := t.Context()
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
	}

	// Incoming relation with property A, resolved TargetProp is B.
	inverseRelations := []store.InverseRelation{
		newIR(identifier.New(), sourceDocID, propA, propB, identifier.Identifier{}, document.HighConfidence),
	}

	result, errE := c.FromDocument(ctx, doc, inverseRelations)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Should produce a reverse claim with property B.
	require.Len(t, result.Claims.Reference, 1)
	assert.Equal(t, propB, result.Claims.Reference[0].Prop)
	assert.Equal(t, sourceDocID, result.Claims.Reference[0].To)
}

func TestDiffOutgoingInverseRelationsBothEmpty(t *testing.T) {
	t.Parallel()

	current := map[identifier.Identifier][]store.InverseRelation{}
	parent := map[identifier.Identifier][]store.InverseRelation{}

	added, removed := diffOutgoingInverseRelations(current, parent)
	assert.Empty(t, added)
	assert.Empty(t, removed)
}

func TestDiffOutgoingInverseRelationsNewClaim(t *testing.T) {
	t.Parallel()

	docA := identifier.New()
	targetB := identifier.New()
	claim1 := identifier.New()
	prop1 := identifier.New()

	current := map[identifier.Identifier][]store.InverseRelation{
		targetB: {newIR(claim1, docA, prop1, prop1, targetB, document.HighConfidence)},
	}
	parent := map[identifier.Identifier][]store.InverseRelation{}

	added, removed := diffOutgoingInverseRelations(current, parent)

	require.Len(t, added[targetB], 1)
	assert.Equal(t, claim1, added[targetB][0].Claim)
	assert.Empty(t, removed)
}

func TestDiffOutgoingInverseRelationsRemovedClaim(t *testing.T) {
	t.Parallel()

	docA := identifier.New()
	targetB := identifier.New()
	claim1 := identifier.New()
	prop1 := identifier.New()

	current := map[identifier.Identifier][]store.InverseRelation{}
	parent := map[identifier.Identifier][]store.InverseRelation{
		targetB: {newIR(claim1, docA, prop1, prop1, targetB, document.HighConfidence)},
	}

	added, removed := diffOutgoingInverseRelations(current, parent)

	assert.Empty(t, added)
	require.Len(t, removed[targetB], 1)
	assert.Equal(t, claim1, removed[targetB][0].Claim)
}

func TestDiffOutgoingInverseRelationsUnchanged(t *testing.T) {
	t.Parallel()

	docA := identifier.New()
	targetB := identifier.New()
	claim1 := identifier.New()
	prop1 := identifier.New()

	ir := newIR(claim1, docA, prop1, prop1, targetB, document.HighConfidence)
	current := map[identifier.Identifier][]store.InverseRelation{targetB: {ir}}
	parent := map[identifier.Identifier][]store.InverseRelation{targetB: {ir}}

	added, removed := diffOutgoingInverseRelations(current, parent)

	assert.Empty(t, added)
	assert.Empty(t, removed)
}

func TestDiffOutgoingInverseRelationsChangedTarget(t *testing.T) {
	t.Parallel()

	docA := identifier.New()
	targetB := identifier.New()
	targetC := identifier.New()
	claimOld := identifier.New()
	claimNew := identifier.New()
	prop1 := identifier.New()

	// Parent had A -> B, current has A -> C (different claim IDs because the claim was replaced).
	parent := map[identifier.Identifier][]store.InverseRelation{
		targetB: {newIR(claimOld, docA, prop1, prop1, targetB, document.HighConfidence)},
	}
	current := map[identifier.Identifier][]store.InverseRelation{
		targetC: {newIR(claimNew, docA, prop1, prop1, targetC, document.HighConfidence)},
	}

	added, removed := diffOutgoingInverseRelations(current, parent)

	require.Len(t, added[targetC], 1)
	assert.Equal(t, claimNew, added[targetC][0].Claim)
	assert.Empty(t, added[targetB])

	require.Len(t, removed[targetB], 1)
	assert.Equal(t, claimOld, removed[targetB][0].Claim)
	assert.Empty(t, removed[targetC])
}

func TestDiffOutgoingInverseRelationsMultipleParents(t *testing.T) {
	t.Parallel()

	docA := identifier.New()
	targetB := identifier.New()
	claim1 := identifier.New()
	claim2 := identifier.New()
	claimNew := identifier.New()
	prop1 := identifier.New()

	// Two parents contributed claims, current keeps claim1 and adds claimNew but drops claim2.
	parent := map[identifier.Identifier][]store.InverseRelation{
		targetB: {
			newIR(claim1, docA, prop1, prop1, targetB, document.HighConfidence),
			newIR(claim2, docA, prop1, prop1, targetB, document.HighConfidence),
		},
	}
	current := map[identifier.Identifier][]store.InverseRelation{
		targetB: {
			newIR(claim1, docA, prop1, prop1, targetB, document.HighConfidence),
			newIR(claimNew, docA, prop1, prop1, targetB, document.HighConfidence),
		},
	}

	added, removed := diffOutgoingInverseRelations(current, parent)

	require.Len(t, added[targetB], 1)
	assert.Equal(t, claimNew, added[targetB][0].Claim)

	require.Len(t, removed[targetB], 1)
	assert.Equal(t, claim2, removed[targetB][0].Claim)
}

func TestDiffOutgoingInverseRelationsSameClaimChangedTarget(t *testing.T) {
	t.Parallel()

	docA := identifier.New()
	targetB := identifier.New()
	targetC := identifier.New()
	claim1 := identifier.New()
	prop1 := identifier.New()

	// Same claim ID, but target changed from B to C.
	parent := map[identifier.Identifier][]store.InverseRelation{
		targetB: {newIR(claim1, docA, prop1, prop1, targetB, document.HighConfidence)},
	}
	current := map[identifier.Identifier][]store.InverseRelation{
		targetC: {newIR(claim1, docA, prop1, prop1, targetC, document.HighConfidence)},
	}

	added, removed := diffOutgoingInverseRelations(current, parent)

	// Should detect the move: removed from B, added to C.
	require.Len(t, added[targetC], 1)
	assert.Equal(t, claim1, added[targetC][0].Claim)
	assert.Empty(t, added[targetB])

	require.Len(t, removed[targetB], 1)
	assert.Equal(t, claim1, removed[targetB][0].Claim)
	assert.Empty(t, removed[targetC])
}

func TestDiffOutgoingInverseRelationsSameClaimChangedProp(t *testing.T) {
	t.Parallel()

	docA := identifier.New()
	targetB := identifier.New()
	claim1 := identifier.New()
	prop1 := identifier.New()
	prop2 := identifier.New()

	// Same claim ID and target, but property changed.
	parent := map[identifier.Identifier][]store.InverseRelation{
		targetB: {newIR(claim1, docA, prop1, prop1, targetB, document.HighConfidence)},
	}
	current := map[identifier.Identifier][]store.InverseRelation{
		targetB: {newIR(claim1, docA, prop2, prop2, targetB, document.HighConfidence)},
	}

	added, removed := diffOutgoingInverseRelations(current, parent)

	// Should detect the property change as removal + addition.
	require.Len(t, added[targetB], 1)
	assert.Equal(t, prop2, added[targetB][0].SourceProp)

	require.Len(t, removed[targetB], 1)
	assert.Equal(t, prop1, removed[targetB][0].SourceProp)
}

func TestDiffOutgoingInverseRelationsSameClaimChangedConfidence(t *testing.T) {
	t.Parallel()

	docA := identifier.New()
	targetB := identifier.New()
	claim1 := identifier.New()
	prop1 := identifier.New()

	// Same claim ID, target, and prop, but confidence changed.
	parent := map[identifier.Identifier][]store.InverseRelation{
		targetB: {newIR(claim1, docA, prop1, prop1, targetB, document.HighConfidence)},
	}
	current := map[identifier.Identifier][]store.InverseRelation{
		targetB: {newIR(claim1, docA, prop1, prop1, targetB, document.LowConfidence)},
	}

	added, removed := diffOutgoingInverseRelations(current, parent)

	// Should detect the confidence change as removal + addition.
	require.Len(t, added[targetB], 1)
	assert.Equal(t, float64(document.LowConfidence), float64(added[targetB][0].Confidence)) //nolint:testifylint

	require.Len(t, removed[targetB], 1)
	assert.Equal(t, float64(document.HighConfidence), float64(removed[targetB][0].Confidence)) //nolint:testifylint
}

// makeClassDocWithField creates a class document with a single top-level FIELD
// that has the given property and optional inverse property.
func makeClassDocWithField(id, fieldPropID identifier.Identifier, inversePropID *identifier.Identifier) *document.D {
	fieldSub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: internalCore.HasPropertyPropID},
				To:        document.Reference{ID: fieldPropID},
			},
		},
	}
	if inversePropID != nil {
		fieldSub.Reference = append(fieldSub.Reference, document.ReferenceClaim{
			CoreClaim: makeCoreClaim(document.HighConfidence, nil),
			Prop:      document.Reference{ID: internalCore.InversePropertyPropID},
			To:        document.Reference{ID: *inversePropID},
		})
	}
	claims := &document.ClaimTypes{
		Has: []document.HasClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, fieldSub),
				Prop:      document.Reference{ID: internalCore.FieldPropID},
			},
		},
	}
	return &document.D{
		CoreDocument: document.CoreDocument{ID: id}, //nolint:exhaustruct
		Claims:       claims,
	}
}

// makeClassDocWithSubField creates a class document with a top-level FIELD (parentPropID)
// containing a SUB_FIELD (childPropID) with optional inverse property.
func makeClassDocWithSubField(id, parentPropID, childPropID identifier.Identifier, inversePropID *identifier.Identifier) *document.D {
	subFieldSub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: internalCore.HasPropertyPropID},
				To:        document.Reference{ID: childPropID},
			},
		},
	}
	if inversePropID != nil {
		subFieldSub.Reference = append(subFieldSub.Reference, document.ReferenceClaim{
			CoreClaim: makeCoreClaim(document.HighConfidence, nil),
			Prop:      document.Reference{ID: internalCore.InversePropertyPropID},
			To:        document.Reference{ID: *inversePropID},
		})
	}
	fieldSub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: internalCore.HasPropertyPropID},
				To:        document.Reference{ID: parentPropID},
			},
		},
		Has: []document.HasClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, subFieldSub),
				Prop:      document.Reference{ID: internalCore.SubFieldPropID},
			},
		},
	}
	claims := &document.ClaimTypes{
		Has: []document.HasClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, fieldSub),
				Prop:      document.Reference{ID: internalCore.FieldPropID},
			},
		},
	}
	return &document.D{
		CoreDocument: document.CoreDocument{ID: id}, //nolint:exhaustruct
		Claims:       claims,
	}
}

func TestBuildFieldInverseProperties(t *testing.T) {
	t.Parallel()

	classID := identifier.New()
	fieldProp := identifier.New()
	inverseProp := identifier.New()

	classDoc := makeClassDocWithField(classID, fieldProp, &inverseProp)

	c := &Converter{}
	c.buildFieldInverseProperties([]*document.D{classDoc})

	// Should have field inverse for top-level field.
	key := fieldInverseKey{Path: "", SourceProp: fieldProp}
	assert.Equal(t, inverseProp, c.fieldInverseProperties[key])
}

func TestBuildFieldInversePropertiesNoInverse(t *testing.T) {
	t.Parallel()

	classID := identifier.New()
	fieldProp := identifier.New()

	classDoc := makeClassDocWithField(classID, fieldProp, nil)

	c := &Converter{}
	c.buildFieldInverseProperties([]*document.D{classDoc})

	assert.Empty(t, c.fieldInverseProperties)
}

func TestBuildFieldInversePropertiesSubField(t *testing.T) {
	t.Parallel()

	classID := identifier.New()
	parentProp := identifier.New()
	childProp := identifier.New()
	inverseProp := identifier.New()

	classDoc := makeClassDocWithSubField(classID, parentProp, childProp, &inverseProp)

	c := &Converter{}
	c.buildFieldInverseProperties([]*document.D{classDoc})

	// Should have field inverse for sub-field with parent path.
	key := fieldInverseKey{Path: parentProp.String(), SourceProp: childProp}
	assert.Equal(t, inverseProp, c.fieldInverseProperties[key])
}

func TestOutgoingInverseRelationsFieldLevel(t *testing.T) {
	t.Parallel()

	// Set up: class defines field P1 with inverse IP.
	classID := identifier.New()
	fieldProp := identifier.New()
	inverseProp := identifier.New()
	classDoc := makeClassDocWithField(classID, fieldProp, &inverseProp)

	// No property-level inverse for fieldProp.
	c := newTestConverterWithClasses(t, nil, []*document.D{classDoc}, nil)

	targetDocID := identifier.New()
	claimID := identifier.New()
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			Reference: []document.ReferenceClaim{
				{
					CoreClaim: document.CoreClaim{
						ID:         claimID,
						Confidence: document.HighConfidence,
					},
					Prop: document.Reference{ID: fieldProp},
					To:   document.Reference{ID: targetDocID},
				},
			},
		},
	}

	outgoing, errE := c.OutgoingInverseRelations(t.Context(), doc)
	require.NoError(t, errE)

	require.Contains(t, outgoing, targetDocID)
	require.Len(t, outgoing[targetDocID], 1)
	ir := outgoing[targetDocID][0]
	assert.Equal(t, claimID, ir.Claim)
	assert.Equal(t, testDocID, ir.Source)
	assert.Equal(t, fieldProp, ir.SourceProp)
	assert.Equal(t, inverseProp, ir.TargetProp)
}

func TestOutgoingInverseRelationsFieldLevelPrecedence(t *testing.T) {
	t.Parallel()

	// Set up: property P has property-level inverse propInv.
	propP := identifier.New()
	propInv := identifier.New()
	propPDoc := makePropertyDocFull(propP, nil, &propInv)
	propInvDoc := makePropertyDocFull(propInv, nil, nil)

	// But class field defines a different inverse: fieldInv.
	classID := identifier.New()
	fieldInv := identifier.New()
	classDoc := makeClassDocWithField(classID, propP, &fieldInv)

	extraDocs := map[identifier.Identifier]*document.D{}
	c := newTestConverterWithClasses(t, []*document.D{propPDoc, propInvDoc}, []*document.D{classDoc}, extraDocs)

	targetDocID := identifier.New()
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			Reference: []document.ReferenceClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: propP},
					To:        document.Reference{ID: targetDocID},
				},
			},
		},
	}

	outgoing, errE := c.OutgoingInverseRelations(t.Context(), doc)
	require.NoError(t, errE)

	// Field-level inverse takes precedence over property-level.
	require.Contains(t, outgoing, targetDocID)
	require.Len(t, outgoing[targetDocID], 1)
	assert.Equal(t, fieldInv, outgoing[targetDocID][0].TargetProp)
}

func TestOutgoingInverseRelationsSubFieldInverse(t *testing.T) {
	t.Parallel()

	// Class defines Has(P1) -> Ref(P2) with inverse IP2.
	classID := identifier.New()
	parentProp := identifier.New()
	childProp := identifier.New()
	inverseProp := identifier.New()
	classDoc := makeClassDocWithSubField(classID, parentProp, childProp, &inverseProp)

	c := newTestConverterWithClasses(t, nil, []*document.D{classDoc}, nil)

	targetDocID := identifier.New()
	claimID := identifier.New()

	// Document has Has(P1) containing Ref(P2).
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			Has: []document.HasClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, &document.ClaimTypes{
						Reference: []document.ReferenceClaim{
							{
								CoreClaim: document.CoreClaim{
									ID:         claimID,
									Confidence: document.HighConfidence,
								},
								Prop: document.Reference{ID: childProp},
								To:   document.Reference{ID: targetDocID},
							},
						},
					}),
					Prop: document.Reference{ID: parentProp},
				},
			},
		},
	}

	outgoing, errE := c.OutgoingInverseRelations(t.Context(), doc)
	require.NoError(t, errE)

	require.Contains(t, outgoing, targetDocID)
	require.Len(t, outgoing[targetDocID], 1)
	ir := outgoing[targetDocID][0]
	assert.Equal(t, claimID, ir.Claim)
	assert.Equal(t, childProp, ir.SourceProp)
	assert.Equal(t, inverseProp, ir.TargetProp)
}

func TestOutgoingInverseRelationsDifferentPathsSameProperty(t *testing.T) {
	t.Parallel()

	// Same property P2 appears under different parents with different inverses.
	classID := identifier.New()
	parentA := identifier.New()
	parentB := identifier.New()
	childProp := identifier.New()
	inverseA := identifier.New()
	inverseB := identifier.New()

	classDocA := makeClassDocWithSubField(classID, parentA, childProp, &inverseA)
	classDocB := makeClassDocWithSubField(identifier.New(), parentB, childProp, &inverseB)

	c := newTestConverterWithClasses(t, nil, []*document.D{classDocA, classDocB}, nil)

	targetDoc1 := identifier.New()
	targetDoc2 := identifier.New()
	claimID1 := identifier.New()
	claimID2 := identifier.New()

	// Document has both Has(parentA)/Ref(childProp) and Has(parentB)/Ref(childProp).
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			Has: []document.HasClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, &document.ClaimTypes{
						Reference: []document.ReferenceClaim{
							{
								CoreClaim: document.CoreClaim{ID: claimID1, Confidence: document.HighConfidence},
								Prop:      document.Reference{ID: childProp},
								To:        document.Reference{ID: targetDoc1},
							},
						},
					}),
					Prop: document.Reference{ID: parentA},
				},
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, &document.ClaimTypes{
						Reference: []document.ReferenceClaim{
							{
								CoreClaim: document.CoreClaim{ID: claimID2, Confidence: document.HighConfidence},
								Prop:      document.Reference{ID: childProp},
								To:        document.Reference{ID: targetDoc2},
							},
						},
					}),
					Prop: document.Reference{ID: parentB},
				},
			},
		},
	}

	outgoing, errE := c.OutgoingInverseRelations(t.Context(), doc)
	require.NoError(t, errE)

	// Each should have the correct inverse based on its path.
	require.Contains(t, outgoing, targetDoc1)
	require.Len(t, outgoing[targetDoc1], 1)
	assert.Equal(t, inverseA, outgoing[targetDoc1][0].TargetProp)

	require.Contains(t, outgoing, targetDoc2)
	require.Len(t, outgoing[targetDoc2], 1)
	assert.Equal(t, inverseB, outgoing[targetDoc2][0].TargetProp)
}

func TestOutgoingInverseRelationsPropertyFallback(t *testing.T) {
	t.Parallel()

	// Property-level inverse but no field-level inverse.
	propP := identifier.New()
	propInv := identifier.New()
	propPDoc := makePropertyDocFull(propP, nil, &propInv)
	propInvDoc := makePropertyDocFull(propInv, nil, nil)

	c := newTestConverter(t, []*document.D{propPDoc, propInvDoc}, nil, nil)

	targetDocID := identifier.New()
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			Reference: []document.ReferenceClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: propP},
					To:        document.Reference{ID: targetDocID},
				},
			},
		},
	}

	outgoing, errE := c.OutgoingInverseRelations(t.Context(), doc)
	require.NoError(t, errE)

	// Should use property-level inverse.
	require.Contains(t, outgoing, targetDocID)
	require.Len(t, outgoing[targetDocID], 1)
	assert.Equal(t, propInv, outgoing[targetDocID][0].TargetProp)
}

func TestOutgoingInverseRelationsStringSubClaimReference(t *testing.T) {
	t.Parallel()

	// Class defines field NAME (string) with sub-field IN_LANGUAGE (reference)
	// that has an inverse property.
	classID := identifier.New()
	nameProp := identifier.New()
	inLangProp := identifier.New()
	inverseProp := identifier.New()
	classDoc := makeClassDocWithSubField(classID, nameProp, inLangProp, &inverseProp)

	c := newTestConverterWithClasses(t, nil, []*document.D{classDoc}, nil)

	langDocID := identifier.New()
	refClaimID := identifier.New()

	// Document has a StringClaim(Prop=nameProp) with a reference sub-claim
	// ReferenceClaim(Prop=inLangProp, To=langDocID).
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, &document.ClaimTypes{
						Reference: []document.ReferenceClaim{
							{
								CoreClaim: document.CoreClaim{
									ID:         refClaimID,
									Confidence: document.HighConfidence,
								},
								Prop: document.Reference{ID: inLangProp},
								To:   document.Reference{ID: langDocID},
							},
						},
					}),
					Prop:   document.Reference{ID: nameProp},
					String: "hello",
				},
			},
		},
	}

	outgoing, errE := c.OutgoingInverseRelations(t.Context(), doc)
	require.NoError(t, errE)

	// Should find the reference inside the string claim's sub-claims.
	require.Contains(t, outgoing, langDocID)
	require.Len(t, outgoing[langDocID], 1)
	ir := outgoing[langDocID][0]
	assert.Equal(t, refClaimID, ir.Claim)
	assert.Equal(t, inLangProp, ir.SourceProp)
	assert.Equal(t, inverseProp, ir.TargetProp)
}

func TestEncodeFieldPath(t *testing.T) {
	t.Parallel()

	assert.Empty(t, encodeFieldPath(nil))
	assert.Empty(t, encodeFieldPath([]identifier.Identifier{}))

	id1 := identifier.New()
	assert.Equal(t, id1.String(), encodeFieldPath([]identifier.Identifier{id1}))

	id2 := identifier.New()
	assert.Equal(t, id1.String()+"/"+id2.String(), encodeFieldPath([]identifier.Identifier{id1, id2}))
}

func TestFromDocumentHookModifies(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "My Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	// Hook adds a string claim to the document.
	c.Hooks = []func(ctx context.Context, doc *document.D) (*document.D, errors.E){
		func(_ context.Context, doc *document.D) (*document.D, errors.E) {
			if doc.Claims == nil {
				doc.Claims = &document.ClaimTypes{}
			}
			doc.Claims.String = append(doc.Claims.String, document.StringClaim{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: testPropID},
				String:    "injected",
			})
			return doc, nil
		},
	}

	ctx := t.Context()
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
	}

	result, errE := c.FromDocument(ctx, doc, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, testDocID, result.ID)
	assert.Equal(t, []string{testDocID.String(), "injected"}, result.Text["und"])
}

func TestFromDocumentHookError(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, map[identifier.Identifier]*document.D{})

	c.Hooks = []func(ctx context.Context, doc *document.D) (*document.D, errors.E){
		func(_ context.Context, _ *document.D) (*document.D, errors.E) {
			return nil, errors.New("hook failed")
		},
	}

	ctx := t.Context()
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
	}

	_, errE := c.FromDocument(ctx, doc, nil)
	require.Error(t, errE)
	assert.EqualError(t, errE, "hook failed")
	assert.Equal(t, 0, errors.AllDetails(errE)["hook"])
}

func TestFromDocumentHookReturnsNil(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, map[identifier.Identifier]*document.D{})

	c.Hooks = []func(ctx context.Context, doc *document.D) (*document.D, errors.E){
		func(_ context.Context, _ *document.D) (*document.D, errors.E) {
			return nil, nil //nolint:nilnil
		},
	}

	ctx := t.Context()
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
	}

	_, errE := c.FromDocument(ctx, doc, nil)
	require.Error(t, errE)
	assert.EqualError(t, errE, "hook returned nil document")
	assert.Equal(t, 0, errors.AllDetails(errE)["hook"])
}

func TestFromDocumentMultipleHooks(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "My Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	// First hook adds a string claim.
	// Second hook adds another string claim.
	c.Hooks = []func(ctx context.Context, doc *document.D) (*document.D, errors.E){
		func(_ context.Context, doc *document.D) (*document.D, errors.E) {
			if doc.Claims == nil {
				doc.Claims = &document.ClaimTypes{}
			}
			doc.Claims.String = append(doc.Claims.String, document.StringClaim{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: testPropID},
				String:    "first",
			})
			return doc, nil
		},
		func(_ context.Context, doc *document.D) (*document.D, errors.E) {
			doc.Claims.String = append(doc.Claims.String, document.StringClaim{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: testPropID},
				String:    "second",
			})
			return doc, nil
		},
	}

	ctx := t.Context()
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
	}

	result, errE := c.FromDocument(ctx, doc, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.ElementsMatch(t, []string{testDocID.String(), "first", "second"}, result.Text["und"])
}

func TestFromDocumentHookReplaces(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "My Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	replacementID := identifier.New()

	// Hook replaces the entire document.
	c.Hooks = []func(ctx context.Context, doc *document.D) (*document.D, errors.E){
		func(_ context.Context, _ *document.D) (*document.D, errors.E) {
			return &document.D{
				CoreDocument: document.CoreDocument{ID: replacementID}, //nolint:exhaustruct
				Claims: &document.ClaimTypes{
					Identifier: []document.IdentifierClaim{
						{
							CoreClaim: makeCoreClaim(document.HighConfidence, nil),
							Prop:      document.Reference{ID: testPropID},
							Value:     "REPLACED",
						},
					},
				},
			}, nil
		},
	}

	ctx := t.Context()
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: testPropID},
					String:    "original",
				},
			},
		},
	}

	result, errE := c.FromDocument(ctx, doc, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	// Result should use the replacement document.
	assert.Equal(t, replacementID, result.ID)
	// The replacement carries the REPLACED identifier value, indexed into text["und"].
	// The original "original" string from the input document is gone because the hook
	// substituted the whole document. The seeded ID is the replacement's, not the input's,
	// because FromDocument seeds the ID after hooks have run.
	assert.Equal(t, []string{replacementID.String(), "REPLACED"}, result.Text["und"])
}

func TestFromDocumentMultipleHooksErrorInSecond(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, map[identifier.Identifier]*document.D{})

	// First hook succeeds, second fails.
	c.Hooks = []func(ctx context.Context, doc *document.D) (*document.D, errors.E){
		func(_ context.Context, doc *document.D) (*document.D, errors.E) {
			return doc, nil
		},
		func(_ context.Context, _ *document.D) (*document.D, errors.E) {
			return nil, errors.New("second hook failed")
		},
	}

	ctx := t.Context()
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
	}

	_, errE := c.FromDocument(ctx, doc, nil)
	require.Error(t, errE)
	assert.EqualError(t, errE, "second hook failed")
	// Hook index should be 1 (the second hook).
	assert.Equal(t, 1, errors.AllDetails(errE)["hook"])
}

func TestFromDocumentNilHooks(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "My Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)
	// Hooks is nil by default, confirm it works.

	ctx := t.Context()
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: testPropID},
					String:    "hello",
				},
			},
		},
	}

	result, errE := c.FromDocument(ctx, doc, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, testDocID, result.ID)
	assert.Equal(t, []string{testDocID.String(), "hello"}, result.Text["und"])
}
