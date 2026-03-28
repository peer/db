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

	"gitlab.com/peerdb/peerdb/core"
	"gitlab.com/peerdb/peerdb/document"
	internalStore "gitlab.com/peerdb/peerdb/internal/store"
)

// Well-known IDs used only in tests.
//
//nolint:gochecknoglobals
var subclassOfPropID = identifier.From(core.Namespace, "SUBCLASS_OF")

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
		Prop:      document.Reference{ID: instanceOfPropID},
		To:        document.Reference{ID: propertyClassID},
	})
	if subpropertyOf != nil {
		claims.Reference = append(claims.Reference, document.ReferenceClaim{
			CoreClaim: makeCoreClaim(document.HighConfidence, nil),
			Prop:      document.Reference{ID: subpropertyOfPropID},
			To:        document.Reference{ID: *subpropertyOf},
		})
	}
	if inversePropertyOf != nil {
		claims.Reference = append(claims.Reference, document.ReferenceClaim{
			CoreClaim: makeCoreClaim(document.HighConfidence, nil),
			Prop:      document.Reference{ID: inversePropertyOfPropID},
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
		Prop:      document.Reference{ID: namingPropID},
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
		Prop:      document.Reference{ID: instanceOfPropID},
		To:        document.Reference{ID: languageClassID},
	})
	claims.Identifier = append(claims.Identifier, document.IdentifierClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: codePropID},
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
		Prop:      document.Reference{ID: namingPropID},
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
	return newTestConverterWithPriority(t, properties, languages, extraDocs, nil)
}

// newTestConverterWithPriority creates a Converter with custom language priority.
func newTestConverterWithPriority(
	t *testing.T,
	properties, languages []*document.D,
	extraDocs map[identifier.Identifier]*document.D,
	priority map[string][]string,
) *Converter {
	t.Helper()
	getDocument := func(_ context.Context, id identifier.Identifier) (*document.D, errors.E) {
		if doc, ok := extraDocs[id]; ok {
			return doc, nil
		}
		return nil, errors.New("document not found")
	}
	c, errE := NewConverter(properties, languages, priority, getDocument)
	require.NoError(t, errE, "% -+#.1v", errE)
	return c
}

func TestIsInstanceOf(t *testing.T) {
	t.Parallel()

	doc := makePropertyDoc(testPropID, nil)
	assert.True(t, isInstanceOf(doc, propertyClassID))
	assert.False(t, isInstanceOf(doc, languageClassID))

	// Document with no claims.
	emptyDoc := &document.D{
		CoreDocument: document.CoreDocument{ID: identifier.New()}, //nolint:exhaustruct
	}
	assert.False(t, isInstanceOf(emptyDoc, propertyClassID))
}

func TestBuildPropertyHierarchy(t *testing.T) {
	t.Parallel()

	// Build a chain: testPropID2 is a subproperty of testPropID.
	child := makePropertyDoc(testPropID2, &testPropID)
	parent := makePropertyDoc(testPropID, nil)

	properties := []*document.D{parent, child}
	c := &Converter{ //nolint:exhaustruct
		documentInfoCache: make(map[identifier.Identifier]documentInfo),
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
		documentInfoCache: make(map[identifier.Identifier]documentInfo),
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
					Prop:      document.Reference{ID: subpropertyOfPropID},
					To:        document.Reference{ID: testPropID},
				},
			},
		},
	}

	c := &Converter{ //nolint:exhaustruct
		documentInfoCache: make(map[identifier.Identifier]documentInfo),
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
	subentityDoc := makePropertyDoc(subentityOfPropID, nil)
	subclassDoc := makePropertyDoc(subclassOfPropID, &subentityOfPropID)
	subpropDoc := makePropertyDoc(subpropertyOfPropID, &subentityOfPropID)
	instanceDoc := makePropertyDoc(instanceOfPropID, &subentityOfPropID)
	properties := []*document.D{subentityDoc, subclassDoc, subpropDoc, instanceDoc}

	grandparent := identifier.New()
	parent := identifier.New()
	child := identifier.New()

	gpDoc := makeHierarchyDoc(grandparent, "Grandparent", subclassOfPropID, nil)
	pDoc := makeHierarchyDoc(parent, "Parent", subclassOfPropID, &grandparent)
	cDoc := makeHierarchyDoc(child, "Child", subclassOfPropID, &parent)

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
	assert.Contains(t, info.Ancestors[subclassOfPropID], parent)
	assert.Contains(t, info.Ancestors[subclassOfPropID], grandparent)

	// Child should have a hierarchy path: grandparent/parent/child.
	require.Len(t, info.IDPaths[subclassOfPropID], 1)
	expectedIDPath := grandparent.String() + "/" + parent.String() + "/" + child.String()
	assert.Equal(t, expectedIDPath, info.IDPaths[subclassOfPropID][0])

	// Display path should use null-byte separated display names.
	require.Contains(t, info.DisplayPaths[subclassOfPropID], "und")
	require.Len(t, info.DisplayPaths[subclassOfPropID]["und"], 1)
	assert.Equal(t, "Grandparent\x00Parent\x00Child", info.DisplayPaths[subclassOfPropID]["und"][0])

	// Parent should have a single-level path: grandparent/parent.
	parentInfo, errE := c.getDocumentInfo(ctx, parent)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, parentInfo.IDPaths[subclassOfPropID], 1)
	assert.Equal(t, grandparent.String()+"/"+parent.String(), parentInfo.IDPaths[subclassOfPropID][0])

	// Grandparent has no hierarchy paths (it's a root).
	gpInfo, errE := c.getDocumentInfo(ctx, grandparent)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Empty(t, gpInfo.IDPaths[subclassOfPropID])
}

func TestGetDocumentInfoMultiplePaths(t *testing.T) {
	t.Parallel()

	subentityDoc := makePropertyDoc(subentityOfPropID, nil)
	subclassDoc := makePropertyDoc(subclassOfPropID, &subentityOfPropID)
	subpropDoc := makePropertyDoc(subpropertyOfPropID, &subentityOfPropID)
	instanceDoc := makePropertyDoc(instanceOfPropID, &subentityOfPropID)
	properties := []*document.D{subentityDoc, subclassDoc, subpropDoc, instanceDoc}

	// Diamond hierarchy: child has two parents, both share a grandparent.
	grandparent := identifier.New()
	parentA := identifier.New()
	parentB := identifier.New()
	child := identifier.New()

	gpDoc := makeNamingDoc(grandparent, "Root")
	paDoc := makeHierarchyDoc(parentA, "ParentA", subclassOfPropID, &grandparent)
	pbDoc := makeHierarchyDoc(parentB, "ParentB", subclassOfPropID, &grandparent)

	// Child with two parents.
	childClaims := &document.ClaimTypes{}
	childClaims.String = append(childClaims.String, document.StringClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: namingPropID},
		String:    "Leaf",
	})
	childClaims.Reference = append(childClaims.Reference,
		document.ReferenceClaim{
			CoreClaim: makeCoreClaim(document.HighConfidence, nil),
			Prop:      document.Reference{ID: subclassOfPropID},
			To:        document.Reference{ID: parentA},
		},
		document.ReferenceClaim{
			CoreClaim: makeCoreClaim(document.HighConfidence, nil),
			Prop:      document.Reference{ID: subclassOfPropID},
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
	require.Len(t, info.IDPaths[subclassOfPropID], 2)
	pathA := grandparent.String() + "/" + parentA.String() + "/" + child.String()
	pathB := grandparent.String() + "/" + parentB.String() + "/" + child.String()
	assert.ElementsMatch(t, info.IDPaths[subclassOfPropID], []string{pathA, pathB})
	// Two display paths.
	require.Len(t, info.DisplayPaths[subclassOfPropID]["und"], 2)
	assert.ElementsMatch(t, info.DisplayPaths[subclassOfPropID]["und"], []string{
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
	c, errE := NewConverter(nil, nil, nil, getDocument)
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
		documentInfoCache: make(map[identifier.Identifier]documentInfo),
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
		documentInfoCache: make(map[identifier.Identifier]documentInfo),
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
		documentInfoCache: make(map[identifier.Identifier]documentInfo),
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
	subentityDoc := makePropertyDoc(subentityOfPropID, nil)
	subclassDoc := makePropertyDoc(subclassOfPropID, &subentityOfPropID)
	subpropDoc := makePropertyDoc(subpropertyOfPropID, &subentityOfPropID)
	instanceDoc := makePropertyDoc(instanceOfPropID, &subentityOfPropID)
	properties := []*document.D{subentityDoc, subclassDoc, subpropDoc, instanceDoc}

	// Document with SUBCLASS_OF pointing to itself.
	selfID := identifier.New()
	selfDoc := makeHierarchyDoc(selfID, "Self", subclassOfPropID, &selfID)
	extraDocs := map[identifier.Identifier]*document.D{
		selfID: selfDoc,
	}
	c := newTestConverter(t, properties, nil, extraDocs)

	ctx := t.Context()
	info, errE := c.getDocumentInfo(ctx, selfID)
	require.NoError(t, errE, "% -+#.1v", errE)
	// Self-reference is excluded to avoid duplicates.
	assert.Empty(t, info.Ancestors[subclassOfPropID])
}

func TestGetDocumentInfoMutualCycle(t *testing.T) {
	t.Parallel()

	subentityDoc := makePropertyDoc(subentityOfPropID, nil)
	subclassDoc := makePropertyDoc(subclassOfPropID, &subentityOfPropID)
	subpropDoc := makePropertyDoc(subpropertyOfPropID, &subentityOfPropID)
	instanceDoc := makePropertyDoc(instanceOfPropID, &subentityOfPropID)
	properties := []*document.D{subentityDoc, subclassDoc, subpropDoc, instanceDoc}

	a := identifier.New()
	b := identifier.New()
	aDoc := makeHierarchyDoc(a, "A", subclassOfPropID, &b)
	bDoc := makeHierarchyDoc(b, "B", subclassOfPropID, &a)
	extraDocs := map[identifier.Identifier]*document.D{
		a: aDoc,
		b: bDoc,
	}
	c := newTestConverter(t, properties, nil, extraDocs)

	ctx := t.Context()
	infoA, errE := c.getDocumentInfo(ctx, a)
	require.NoError(t, errE, "% -+#.1v", errE)
	// A should have B as ancestor (cycle handled without infinite loop).
	assert.Contains(t, infoA.Ancestors[subclassOfPropID], b)
}

func TestGetDocumentInfoMultipleHierarchies(t *testing.T) {
	t.Parallel()

	// Custom hierarchy property PART_OF as a sub-property of SUBENTITY_OF.
	partOfPropID := identifier.New()
	subentityDoc := makePropertyDoc(subentityOfPropID, nil)
	subclassDoc := makePropertyDoc(subclassOfPropID, &subentityOfPropID)
	partOfDoc := makePropertyDoc(partOfPropID, &subentityOfPropID)
	subpropDoc := makePropertyDoc(subpropertyOfPropID, &subentityOfPropID)
	instanceDoc := makePropertyDoc(instanceOfPropID, &subentityOfPropID)
	properties := []*document.D{subentityDoc, subclassDoc, partOfDoc, subpropDoc, instanceDoc}

	classParent := identifier.New()
	partParent := identifier.New()
	child := identifier.New()

	// Child has SUBCLASS_OF -> classParent and PART_OF -> partParent.
	childClaims := &document.ClaimTypes{}
	childClaims.String = append(childClaims.String, document.StringClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: namingPropID},
		String:    "Child",
	})
	childClaims.Reference = append(childClaims.Reference,
		document.ReferenceClaim{
			CoreClaim: makeCoreClaim(document.HighConfidence, nil),
			Prop:      document.Reference{ID: subclassOfPropID},
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
	assert.Contains(t, info.Ancestors[subclassOfPropID], classParent)
	assert.Contains(t, info.Ancestors[partOfPropID], partParent)
	// Each hierarchy should only contain its own ancestors.
	assert.NotContains(t, info.Ancestors[subclassOfPropID], partParent)
	assert.NotContains(t, info.Ancestors[partOfPropID], classParent)
}

func TestGetDocumentInfoOverlappingHierarchies(t *testing.T) {
	t.Parallel()

	// Custom hierarchy property PART_OF as a sub-property of SUBENTITY_OF.
	partOfPropID := identifier.New()
	subentityDoc := makePropertyDoc(subentityOfPropID, nil)
	subclassDoc := makePropertyDoc(subclassOfPropID, &subentityOfPropID)
	partOfDoc := makePropertyDoc(partOfPropID, &subentityOfPropID)
	subpropDoc := makePropertyDoc(subpropertyOfPropID, &subentityOfPropID)
	instanceDoc := makePropertyDoc(instanceOfPropID, &subentityOfPropID)
	properties := []*document.D{subentityDoc, subclassDoc, partOfDoc, subpropDoc, instanceDoc}

	// Same parent reachable via both SUBCLASS_OF and PART_OF.
	sharedParent := identifier.New()
	child := identifier.New()

	childClaims := &document.ClaimTypes{}
	childClaims.String = append(childClaims.String, document.StringClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: namingPropID},
		String:    "Child",
	})
	childClaims.Reference = append(childClaims.Reference,
		document.ReferenceClaim{
			CoreClaim: makeCoreClaim(document.HighConfidence, nil),
			Prop:      document.Reference{ID: subclassOfPropID},
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
	assert.Contains(t, info.Ancestors[subclassOfPropID], sharedParent)
	assert.Contains(t, info.Ancestors[partOfPropID], sharedParent)
}

func TestBuildNamingProperties(t *testing.T) {
	t.Parallel()

	// namingPropID is NAMING, testPropID is a subproperty of NAMING.
	namingDoc := makePropertyDoc(namingPropID, nil)
	subNaming := makePropertyDoc(testPropID, &namingPropID)

	c := &Converter{ //nolint:exhaustruct
		documentInfoCache: make(map[identifier.Identifier]documentInfo),
	}
	c.buildPropertyHierarchy([]*document.D{namingDoc, subNaming})
	c.buildNamingProperties()

	assert.Contains(t, c.namingProperties, namingPropID)
	assert.Contains(t, c.namingProperties, testPropID)
	assert.NotContains(t, c.namingProperties, testPropID2)
}

func TestDiscoverValueHierarchyProperties(t *testing.T) {
	t.Parallel()

	// Standard properties: SUBENTITY_OF with INSTANCE_OF, SUBCLASS_OF, SUBPROPERTY_OF as children.
	subentityDoc := makePropertyDoc(subentityOfPropID, nil)
	subclassDoc := makePropertyDoc(subclassOfPropID, &subentityOfPropID)
	subpropDoc := makePropertyDoc(subpropertyOfPropID, &subentityOfPropID)
	instanceDoc := makePropertyDoc(instanceOfPropID, &subentityOfPropID)
	properties := []*document.D{subentityDoc, subclassDoc, subpropDoc, instanceDoc}

	c := &Converter{ //nolint:exhaustruct
		documentInfoCache: make(map[identifier.Identifier]documentInfo),
	}
	c.buildPropertyHierarchy(properties)
	c.discoverValueHierarchyProperties()

	// Only SUBCLASS_OF should be a value hierarchy property.
	// INSTANCE_OF and SUBPROPERTY_OF are excluded.
	assert.Contains(t, c.valueHierarchyProperties, subclassOfPropID)
	assert.NotContains(t, c.valueHierarchyProperties, instanceOfPropID)
	assert.NotContains(t, c.valueHierarchyProperties, subpropertyOfPropID)
	assert.Len(t, c.valueHierarchyProperties, 1)
}

func TestDiscoverValueHierarchyPropertiesCustom(t *testing.T) {
	t.Parallel()

	// Add a custom PART_OF property as a sub-property of SUBENTITY_OF.
	partOfPropID := identifier.New()
	subentityDoc := makePropertyDoc(subentityOfPropID, nil)
	subclassDoc := makePropertyDoc(subclassOfPropID, &subentityOfPropID)
	subpropDoc := makePropertyDoc(subpropertyOfPropID, &subentityOfPropID)
	instanceDoc := makePropertyDoc(instanceOfPropID, &subentityOfPropID)
	partOfDoc := makePropertyDoc(partOfPropID, &subentityOfPropID)
	properties := []*document.D{subentityDoc, subclassDoc, subpropDoc, instanceDoc, partOfDoc}

	c := &Converter{ //nolint:exhaustruct
		documentInfoCache: make(map[identifier.Identifier]documentInfo),
	}
	c.buildPropertyHierarchy(properties)
	c.discoverValueHierarchyProperties()

	// Both SUBCLASS_OF and PART_OF should be value hierarchy properties.
	assert.Contains(t, c.valueHierarchyProperties, subclassOfPropID)
	assert.Contains(t, c.valueHierarchyProperties, partOfPropID)
	assert.NotContains(t, c.valueHierarchyProperties, instanceOfPropID)
	assert.NotContains(t, c.valueHierarchyProperties, subpropertyOfPropID)
	assert.Len(t, c.valueHierarchyProperties, 2)
}

func TestConvertRelationMultipleHierarchies(t *testing.T) {
	t.Parallel()

	// Custom hierarchy property PART_OF as a sub-property of SUBENTITY_OF.
	partOfPropID := identifier.New()
	subentityDoc := makePropertyDoc(subentityOfPropID, nil)
	subclassDoc := makePropertyDoc(subclassOfPropID, &subentityOfPropID)
	partOfDoc := makePropertyDoc(partOfPropID, &subentityOfPropID)
	subpropDoc := makePropertyDoc(subpropertyOfPropID, &subentityOfPropID)
	instanceDoc := makePropertyDoc(instanceOfPropID, &subentityOfPropID)
	properties := []*document.D{subentityDoc, subclassDoc, partOfDoc, subpropDoc, instanceDoc}

	classParent := identifier.New()
	partParent := identifier.New()
	target := identifier.New()

	// Target has SUBCLASS_OF -> classParent and PART_OF -> partParent.
	targetClaims := &document.ClaimTypes{}
	targetClaims.String = append(targetClaims.String, document.StringClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: namingPropID},
		String:    "Target",
	})
	targetClaims.Reference = append(targetClaims.Reference,
		document.ReferenceClaim{
			CoreClaim: makeCoreClaim(document.HighConfidence, nil),
			Prop:      document.Reference{ID: subclassOfPropID},
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
	result, errE := c.convertReference(ctx, claim)
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
	subentityDoc := makePropertyDoc(subentityOfPropID, nil)
	subclassDoc := makePropertyDoc(subclassOfPropID, &subentityOfPropID)
	partOfDoc := makePropertyDoc(partOfPropID, &subentityOfPropID)
	subpropDoc := makePropertyDoc(subpropertyOfPropID, &subentityOfPropID)
	instanceDoc := makePropertyDoc(instanceOfPropID, &subentityOfPropID)
	properties := []*document.D{subentityDoc, subclassDoc, partOfDoc, subpropDoc, instanceDoc}

	// Same ancestor reachable via both SUBCLASS_OF and PART_OF.
	sharedAncestor := identifier.New()
	target := identifier.New()

	targetClaims := &document.ClaimTypes{}
	targetClaims.String = append(targetClaims.String, document.StringClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: namingPropID},
		String:    "Target",
	})
	targetClaims.Reference = append(targetClaims.Reference,
		document.ReferenceClaim{
			CoreClaim: makeCoreClaim(document.HighConfidence, nil),
			Prop:      document.Reference{ID: subclassOfPropID},
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
	result, errE := c.convertReference(ctx, claim)
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
		documentInfoCache: make(map[identifier.Identifier]documentInfo),
	}
	c.buildLanguageCodes([]*document.D{enDoc, slDoc})

	assert.Equal(t, "en", c.languageCodes[testLangDocID])
	assert.Equal(t, "sl", c.languageCodes[slID])
}

func TestBuildLanguageCodesSubtag(t *testing.T) {
	t.Parallel()

	// Language code "en-US" should be cut to "en".
	langDoc := makeLanguageDoc(testLangDocID, "en-US")

	c := &Converter{ //nolint:exhaustruct
		documentInfoCache: make(map[identifier.Identifier]documentInfo),
	}
	c.buildLanguageCodes([]*document.D{langDoc})

	assert.Equal(t, "en", c.languageCodes[testLangDocID])
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
					Prop:      document.Reference{ID: codePropID},
					Value:     "en",
				},
			},
		},
	}

	c := &Converter{ //nolint:exhaustruct
		documentInfoCache: make(map[identifier.Identifier]documentInfo),
	}
	c.buildLanguageCodes([]*document.D{notLang})

	assert.Empty(t, c.languageCodes)
}

func TestExtractInLanguages(t *testing.T) {
	t.Parallel()

	c := &Converter{ //nolint:exhaustruct
		languageCodes: map[identifier.Identifier]string{
			testLangDocID: "en",
		},
	}

	// Sub-claims with IN_LANGUAGE relation to a known language.
	sub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: inLanguagePropID},
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
				Prop:      document.Reference{ID: inLanguagePropID},
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
		languageCodes: map[identifier.Identifier]string{
			xxLangID: "xx",
		},
	}

	sub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: inLanguagePropID},
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
		languageCodes: map[identifier.Identifier]string{
			enLangID: "en",
			slLangID: "sl",
		},
	}

	sub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: inLanguagePropID},
				To:        document.Reference{ID: enLangID},
			},
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: inLanguagePropID},
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
				Prop:      document.Reference{ID: inUnitPropID},
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
}

func TestNamingStrings(t *testing.T) {
	t.Parallel()

	c := &Converter{ //nolint:exhaustruct
		namingProperties: []identifier.Identifier{namingPropID},
		languageCodes:    map[identifier.Identifier]string{},
	}

	doc := makeNamingDoc(testDocID, "Test Document")
	result := c.namingStrings(doc)
	require.NotNil(t, result)
	assert.Equal(t, []string{"Test Document"}, result["und"])
}

func TestNamingStringsEmpty(t *testing.T) {
	t.Parallel()

	c := &Converter{ //nolint:exhaustruct
		namingProperties: []identifier.Identifier{namingPropID},
		languageCodes:    map[identifier.Identifier]string{},
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
		namingProperties: []identifier.Identifier{namingPropID},
		languageCodes:    map[identifier.Identifier]string{},
	}

	// Two naming strings with different confidences.
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(document.MediumConfidence, nil),
					Prop:      document.Reference{ID: namingPropID},
					String:    "Medium",
				},
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: namingPropID},
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
		namingProperties: []identifier.Identifier{namingPropID},
		languageCodes:    map[identifier.Identifier]string{},
	}

	// Two naming strings: first becomes Display, Naming contains all strings.
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: namingPropID},
					String:    "Primary",
				},
				{
					CoreClaim: makeCoreClaim(document.MediumConfidence, nil),
					Prop:      document.Reference{ID: namingPropID},
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
		namingProperties: []identifier.Identifier{namingPropID},
		languageCodes:    map[identifier.Identifier]string{},
	}

	// Naming string with null byte should have it stripped.
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: namingPropID},
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

	namingDoc := makePropertyDoc(namingPropID, nil)
	subProp := makePropertyDoc(testPropID, &namingPropID)
	langDoc := makeLanguageDoc(testLangDocID, "en")

	extraDocs := map[identifier.Identifier]*document.D{}
	c := newTestConverter(t, []*document.D{namingDoc, subProp}, []*document.D{langDoc}, extraDocs)

	assert.Contains(t, c.namingProperties, namingPropID)
	assert.Contains(t, c.namingProperties, testPropID)
	assert.Equal(t, "en", c.languageCodes[testLangDocID])
	assert.NotNil(t, c.documentInfoCache)
}

func TestConvertIdentifier(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "My Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	claim := &document.IdentifierClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: testPropID},
		Value:     "Q42",
	}
	result, errE := c.convertIdentifier(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, result, 1)
	assert.Equal(t, testPropID, result[0].Prop)
	assert.Equal(t, "Q42", result[0].Value)
	assert.Equal(t, "My Prop", result[0].PropDisplay["und"])
}

func TestConvertIdentifierWithPropagation(t *testing.T) {
	t.Parallel()

	parentPropDoc := makeNamingDoc(testParentProp, "Parent Prop")
	childPropDoc := makeNamingDoc(testPropID, "Child Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID:     childPropDoc,
		testParentProp: parentPropDoc,
	}

	c := newTestConverter(t, nil, nil, extraDocs)
	c.propertyAncestors = map[identifier.Identifier][]identifier.Identifier{
		testPropID: {testParentProp},
	}

	ctx := t.Context()
	claim := &document.IdentifierClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: testPropID},
		Value:     "Q42",
	}
	result, errE := c.convertIdentifier(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, result, 2)
	assert.Equal(t, testPropID, result[0].Prop)
	assert.Equal(t, testParentProp, result[1].Prop)
}

func TestConvertIdentifierGetDocumentError(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	claim := &document.IdentifierClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: identifier.New()},
		Value:     "Q42",
	}
	_, errE := c.convertIdentifier(ctx, claim)
	assert.EqualError(t, errE, "document not found")
}

func TestConvertString(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Str Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	claim := &document.StringClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: testPropID},
		String:    "hello world",
	}
	result, errE := c.convertString(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, result, 1)
	assert.Equal(t, testPropID, result[0].Prop)
	assert.Equal(t, "hello world", result[0].String["und"])
}

func TestConvertStringWithLanguage(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Str Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)
	c.languageCodes = map[identifier.Identifier]string{
		testLangDocID: "en",
	}

	ctx := t.Context()
	sub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: inLanguagePropID},
				To:        document.Reference{ID: testLangDocID},
			},
		},
	}
	claim := &document.StringClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, sub),
		Prop:      document.Reference{ID: testPropID},
		String:    "hello",
	}
	result, errE := c.convertString(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, result, 1)
	assert.Equal(t, "hello", result[0].String["en"])
	_, hasUnd := result[0].String["und"]
	assert.False(t, hasUnd)
}

func TestConvertHTML(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "HTML Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	claim := &document.HTMLClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: testPropID},
		HTML:      "<p>hello</p>",
	}
	result, errE := c.convertHTML(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, result, 1)
	assert.Equal(t, "<p>hello</p>", result[0].HTML["und"])
}

func TestConvertHTMLWithLanguage(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "HTML Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)
	c.languageCodes = map[identifier.Identifier]string{
		testLangDocID: "sl",
	}

	ctx := t.Context()
	sub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: inLanguagePropID},
				To:        document.Reference{ID: testLangDocID},
			},
		},
	}
	claim := &document.HTMLClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, sub),
		Prop:      document.Reference{ID: testPropID},
		HTML:      "<b>test</b>",
	}
	result, errE := c.convertHTML(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, result, 1)
	assert.Equal(t, "<b>test</b>", result[0].HTML["sl"])
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
	result, errE := c.convertAmount(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, result, 1)
	assert.Equal(t, testPropID, result[0].Prop)
	require.NotNil(t, result[0].From)
	assert.InDelta(t, 99.5, *result[0].From, 0.001)
	require.NotNil(t, result[0].To)
	assert.InDelta(t, 100.5, *result[0].To, 0.001)
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
				Prop:      document.Reference{ID: inUnitPropID},
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
	amountClaims, unknownClaims, errE := c.convertAmountInterval(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, amountClaims, 1)
	assert.Empty(t, unknownClaims)
	assert.NotNil(t, amountClaims[0].Range.GreaterThanOrEqual)
	assert.NotNil(t, amountClaims[0].Range.LessThan)
}

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
		ToIsClosed:    true,
	}
	amountClaims, unknownClaims, errE := c.convertAmountInterval(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, amountClaims, 1)
	assert.Empty(t, unknownClaims)
	assert.NotNil(t, amountClaims[0].Range.GreaterThan)
	assert.Nil(t, amountClaims[0].Range.GreaterThanOrEqual)
	assert.NotNil(t, amountClaims[0].Range.LessThanOrEqual)
	assert.Nil(t, amountClaims[0].Range.LessThan)
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
	amountClaims, unknownClaims, errE := c.convertAmountInterval(ctx, claim)
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
	amountClaims, unknownClaims, errE := c.convertAmountInterval(ctx, claim)
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
	// Should treat as single point at To.
	amountClaims, unknownClaims, errE := c.convertAmountInterval(ctx, claim)
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
	// Should treat as single point at From.
	amountClaims, unknownClaims, errE := c.convertAmountInterval(ctx, claim)
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
	// Both unknown: should become unknown claim.
	amountClaims, unknownClaims, errE := c.convertAmountInterval(ctx, claim)
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
	// From is None, To is Unknown with known From: becomes unknown.
	amountClaims, unknownClaims, errE := c.convertAmountInterval(ctx, claim)
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
	_, _, errE := c.convertAmountInterval(ctx, claim)
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
	_, _, errE := c.convertAmountInterval(ctx, claim)
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
	_, _, errE := c.convertAmountInterval(ctx, claim)
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
	_, _, errE := c.convertAmountInterval(ctx, claim)
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
		Timestamp: document.Timestamp("2024-01-15"),
		Precision: document.TimePrecisionDay,
	}
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
	fromTS := document.Timestamp("2024-01-01")
	toTS := document.Timestamp("2024-12-31")
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
	timeClaims, unknownClaims, errE := c.convertTimeInterval(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, timeClaims, 1)
	assert.Empty(t, unknownClaims)
	assert.NotNil(t, timeClaims[0].Range.GreaterThanOrEqual)
	assert.NotNil(t, timeClaims[0].Range.LessThan)
}

func TestConvertTimeIntervalOpen(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Time Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	fromTS := document.Timestamp("2024-01-01")
	toTS := document.Timestamp("2024-12-31")
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
		ToIsClosed:    true,
	}
	timeClaims, unknownClaims, errE := c.convertTimeInterval(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, timeClaims, 1)
	assert.Empty(t, unknownClaims)
	assert.NotNil(t, timeClaims[0].Range.GreaterThan)
	assert.Nil(t, timeClaims[0].Range.GreaterThanOrEqual)
	assert.NotNil(t, timeClaims[0].Range.LessThanOrEqual)
	assert.Nil(t, timeClaims[0].Range.LessThan)
}

func TestConvertTimeIntervalFromNone(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Time Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	toTS := document.Timestamp("2024-12-31")
	toPrec := document.TimePrecisionDay
	claim := &document.TimeIntervalClaim{ //nolint:exhaustruct
		CoreClaim:   makeCoreClaim(document.HighConfidence, nil),
		Prop:        document.Reference{ID: testPropID},
		FromIsNone:  true,
		To:          &toTS,
		ToPrecision: &toPrec,
	}
	timeClaims, unknownClaims, errE := c.convertTimeInterval(ctx, claim)
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
	fromTS := document.Timestamp("2024-01-01")
	fromPrec := document.TimePrecisionDay
	claim := &document.TimeIntervalClaim{ //nolint:exhaustruct
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: testPropID},
		From:          &fromTS,
		FromPrecision: &fromPrec,
		ToIsNone:      true,
	}
	timeClaims, unknownClaims, errE := c.convertTimeInterval(ctx, claim)
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
	toTS := document.Timestamp("2024-06-15")
	toPrec := document.TimePrecisionDay
	claim := &document.TimeIntervalClaim{ //nolint:exhaustruct
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: testPropID},
		FromIsUnknown: true,
		To:            &toTS,
		ToPrecision:   &toPrec,
	}
	timeClaims, unknownClaims, errE := c.convertTimeInterval(ctx, claim)
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
	fromTS := document.Timestamp("2024-06-15")
	fromPrec := document.TimePrecisionDay
	claim := &document.TimeIntervalClaim{ //nolint:exhaustruct
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: testPropID},
		From:          &fromTS,
		FromPrecision: &fromPrec,
		ToIsUnknown:   true,
	}
	timeClaims, unknownClaims, errE := c.convertTimeInterval(ctx, claim)
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
	timeClaims, unknownClaims, errE := c.convertTimeInterval(ctx, claim)
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
	// FromNone sets range gte, then ToUnknown with known From (but From is set through range) -
	// this hits the default case.
	timeClaims, unknownClaims, errE := c.convertTimeInterval(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Empty(t, timeClaims)
	require.Len(t, unknownClaims, 1)
}

func TestConvertTimeIntervalMissingFromPrecision(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	fromTS := document.Timestamp("2024-01-01")
	toTS := document.Timestamp("2024-12-31")
	toPrec := document.TimePrecisionDay
	claim := &document.TimeIntervalClaim{ //nolint:exhaustruct
		CoreClaim:   makeCoreClaim(document.HighConfidence, nil),
		Prop:        document.Reference{ID: testPropID},
		From:        &fromTS,
		To:          &toTS,
		ToPrecision: &toPrec,
	}
	_, _, errE := c.convertTimeInterval(ctx, claim)
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
	fromTS := document.Timestamp("2024-01-01")
	fromPrec := document.TimePrecisionDay
	toTS := document.Timestamp("2024-12-31")
	claim := &document.TimeIntervalClaim{ //nolint:exhaustruct
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: testPropID},
		From:          &fromTS,
		FromPrecision: &fromPrec,
		To:            &toTS,
	}
	_, _, errE := c.convertTimeInterval(ctx, claim)
	assert.EqualError(t, errE, "missing to precision in claim")
}

func TestConvertTimeIntervalFromUnknownMissingToPrecision(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	toTS := document.Timestamp("2024-12-31")
	claim := &document.TimeIntervalClaim{ //nolint:exhaustruct
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: testPropID},
		FromIsUnknown: true,
		To:            &toTS,
	}
	_, _, errE := c.convertTimeInterval(ctx, claim)
	assert.EqualError(t, errE, "missing to precision in claim")
}

func TestConvertTimeIntervalToUnknownMissingFromPrecision(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	fromTS := document.Timestamp("2024-01-01")
	claim := &document.TimeIntervalClaim{ //nolint:exhaustruct
		CoreClaim:   makeCoreClaim(document.HighConfidence, nil),
		Prop:        document.Reference{ID: testPropID},
		From:        &fromTS,
		ToIsUnknown: true,
	}
	_, _, errE := c.convertTimeInterval(ctx, claim)
	assert.EqualError(t, errE, "missing from precision in claim")
}

func TestConvertReference(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Ref Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	claim := &document.LinkClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: testPropID},
		IRI:       "https://example.com",
	}
	result, errE := c.convertLink(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, result, 1)
	assert.Equal(t, "https://example.com", result[0].IRI)
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
	result, errE := c.convertReference(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, result, 1)
	assert.Equal(t, testPropID, result[0].Prop)
	assert.Equal(t, testTargetDocID, result[0].To)
	assert.Equal(t, "Target", result[0].ToDisplay["und"])
}

func TestConvertRelationWithClassAncestors(t *testing.T) {
	t.Parallel()

	// Set up hierarchy properties so SUBCLASS_OF is discovered.
	subentityDoc := makePropertyDoc(subentityOfPropID, nil)
	subclassDoc := makePropertyDoc(subclassOfPropID, &subentityOfPropID)
	subpropDoc := makePropertyDoc(subpropertyOfPropID, &subentityOfPropID)
	instanceDoc := makePropertyDoc(instanceOfPropID, &subentityOfPropID)
	properties := []*document.D{subentityDoc, subclassDoc, subpropDoc, instanceDoc}

	propDoc := makeNamingDoc(testPropID, "Rel Prop")
	// Target has SUBCLASS_OF -> parent.
	targetDoc := makeHierarchyDoc(testTargetDocID, "Target", subclassOfPropID, &testParentClass)
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
	result, errE := c.convertReference(ctx, claim)
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
	assert.Equal(t, subclassOfPropID.String()+":"+testParentClass.String()+"/"+testTargetDocID.String(), targetClaim.ToPath[0])
	require.Len(t, targetClaim.ToDisplayPath["und"], 1)
	assert.Equal(t, "Parent Class\x00Target", targetClaim.ToDisplayPath["und"][0])

	// Parent class claim has no hierarchy path (it's a root).
	assert.Empty(t, parentClaim.ToPath)
	assert.Empty(t, parentClaim.ToDisplayPath)
}

func TestConvertRelationWithClassSelfCycle(t *testing.T) {
	t.Parallel()

	subentityDoc := makePropertyDoc(subentityOfPropID, nil)
	subclassDoc := makePropertyDoc(subclassOfPropID, &subentityOfPropID)
	subpropDoc := makePropertyDoc(subpropertyOfPropID, &subentityOfPropID)
	instanceDoc := makePropertyDoc(instanceOfPropID, &subentityOfPropID)
	properties := []*document.D{subentityDoc, subclassDoc, subpropDoc, instanceDoc}

	propDoc := makeNamingDoc(testPropID, "Rel Prop")
	// Target has SUBCLASS_OF pointing to itself.
	targetDoc := makeHierarchyDoc(testTargetDocID, "Target", subclassOfPropID, &testTargetDocID)
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
	// Self-reference excluded, so only one result claim.
	result, errE := c.convertReference(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, result, 1)
	assert.Equal(t, testTargetDocID, result[0].To)
}

func TestConvertRelationWithClassMutualCycle(t *testing.T) {
	t.Parallel()

	subentityDoc := makePropertyDoc(subentityOfPropID, nil)
	subclassDoc := makePropertyDoc(subclassOfPropID, &subentityOfPropID)
	subpropDoc := makePropertyDoc(subpropertyOfPropID, &subentityOfPropID)
	instanceDoc := makePropertyDoc(instanceOfPropID, &subentityOfPropID)
	properties := []*document.D{subentityDoc, subclassDoc, subpropDoc, instanceDoc}

	classA := identifier.New()
	classB := identifier.New()
	// A has SUBCLASS_OF -> B, B has SUBCLASS_OF -> A.
	aDoc := makeHierarchyDoc(classA, "Class A", subclassOfPropID, &classB)
	bDoc := makeHierarchyDoc(classB, "Class B", subclassOfPropID, &classA)

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
	result, errE := c.convertReference(ctx, claim)
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
	result, errE := c.convertReference(ctx, claim)
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
	c.buildPropertyHierarchy([]*document.D{propADoc, propBDoc})

	ctx := t.Context()
	claim := &document.ReferenceClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: propA},
		To:        document.Reference{ID: testTargetDocID},
	}
	result, errE := c.convertReference(ctx, claim)
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

func TestConvertStringWithPropertyCycle(t *testing.T) {
	t.Parallel()

	// Two properties in a mutual cycle, used with string claim conversion.
	propA := identifier.New()
	propB := identifier.New()
	propADoc := makePropertyDoc(propA, &propB)
	propBDoc := makePropertyDoc(propB, &propA)

	propANaming := makeNamingDoc(propA, "Prop A")
	propBNaming := makeNamingDoc(propB, "Prop B")
	extraDocs := map[identifier.Identifier]*document.D{
		propA: propANaming,
		propB: propBNaming,
	}
	c := newTestConverter(t, nil, nil, extraDocs)
	c.buildPropertyHierarchy([]*document.D{propADoc, propBDoc})

	ctx := t.Context()
	claim := &document.StringClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: propA},
		String:    "hello",
	}
	result, errE := c.convertString(ctx, claim)
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
	result, errE := c.convertReference(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, result, 1)
	require.Len(t, result[0].Reference, 1)
	assert.Equal(t, subPropID, result[0].Reference[0].Prop)
	assert.Equal(t, subTargetID, result[0].Reference[0].To)
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
	result, errE := c.convertHas(ctx, claim)
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
	result, errE := c.convertHas(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, result, 1)
	require.Len(t, result[0].Reference, 1)
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
	result, errE := c.convertNone(ctx, claim)
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
	result, errE := c.convertUnknown(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, result, 1)
	assert.Equal(t, testPropID, result[0].Prop)
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
	assert.Len(t, result.Claims.Identifier, 1)
	assert.Len(t, result.Claims.String, 1)
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
	assert.Empty(t, result.Claims.Identifier)
}

func makeDocWithAllClaimTypes(t *testing.T, confidence document.Confidence) *document.D {
	t.Helper()

	fromAmount := document.Amount("5")
	toAmount := document.Amount("10")
	fromPrec := 1.0
	toPrec := 1.0
	fromTS := document.Timestamp("2024-01-01")
	toTS := document.Timestamp("2024-12-31")
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
					Timestamp: document.Timestamp("2024-06-15"),
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
			assert.Len(t, result.Claims.Identifier, tt.expected)
			assert.Len(t, result.Claims.String, tt.expected)
			assert.Len(t, result.Claims.HTML, tt.expected)
			// Amount + AmountInterval each contribute one claim.
			assert.Len(t, result.Claims.Amount, 2*tt.expected)
			// Time + TimeInterval each contribute one claim.
			assert.Len(t, result.Claims.Time, 2*tt.expected)
			assert.Len(t, result.Claims.Link, tt.expected)
			assert.Len(t, result.Claims.Reference, tt.expected)
			assert.Len(t, result.Claims.Has, tt.expected)
			assert.Len(t, result.Claims.None, tt.expected)
			assert.Len(t, result.Claims.Unknown, tt.expected)
		})
	}
}

func TestAddPrecision(t *testing.T) {
	t.Parallel()

	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		precision document.TimePrecision
		expected  time.Time
	}{
		{"year", document.TimePrecisionYear, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"month", document.TimePrecisionMonth, time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)},
		{"day", document.TimePrecisionDay, time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)},
		{"hour", document.TimePrecisionHour, time.Date(2024, 1, 1, 1, 0, 0, 0, time.UTC)},
		{"minute", document.TimePrecisionMinute, time.Date(2024, 1, 1, 0, 1, 0, 0, time.UTC)},
		{"second", document.TimePrecisionSecond, time.Date(2024, 1, 1, 0, 0, 1, 0, time.UTC)},
		{"ten years", document.TimePrecisionTenYears, time.Date(2034, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"hundred years", document.TimePrecisionHundredYears, time.Date(2124, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"kilo years", document.TimePrecisionKiloYears, time.Date(3024, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"ten kilo years", document.TimePrecisionTenKiloYears, time.Date(12024, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"hundred kilo years", document.TimePrecisionHundredKiloYears, time.Date(102024, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"mega years", document.TimePrecisionMegaYears, time.Date(1002024, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"ten mega years", document.TimePrecisionTenMegaYears, time.Date(10002024, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"hundred mega years", document.TimePrecisionHundredMegaYears, time.Date(100002024, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"giga years", document.TimePrecisionGigaYears, time.Date(1000002024, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"millisecond", document.TimePrecisionMillisecond, time.Date(2024, 1, 1, 0, 0, 1, 0, time.UTC)},
		{"microsecond", document.TimePrecisionMicrosecond, time.Date(2024, 1, 1, 0, 0, 1, 0, time.UTC)},
		{"nanosecond", document.TimePrecisionNanosecond, time.Date(2024, 1, 1, 0, 0, 1, 0, time.UTC)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := addPrecision(base, tt.precision)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Tests for error propagation through Visit* methods and FromDocument.

func TestFromDocumentVisitorError(t *testing.T) {
	t.Parallel()

	// getDocument will fail, causing convertIdentifier to fail,
	// which will cause VisitIdentifier to return an error.
	c := newTestConverter(t, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			Identifier: []document.IdentifierClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: identifier.New()},
					Value:     "Q42",
				},
			},
		},
	}

	_, errE := c.FromDocument(ctx, doc, nil)
	assert.EqualError(t, errE, "document not found")
}

func TestFromDocumentStringError(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: identifier.New()},
					String:    "str",
				},
			},
		},
	}

	_, errE := c.FromDocument(ctx, doc, nil)
	assert.EqualError(t, errE, "document not found")
}

func TestFromDocumentHTMLError(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			HTML: []document.HTMLClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: identifier.New()},
					HTML:      "<p>test</p>",
				},
			},
		},
	}

	_, errE := c.FromDocument(ctx, doc, nil)
	assert.EqualError(t, errE, "document not found")
}

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
					Timestamp: document.Timestamp("2024-01-15"),
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
	fromTS := document.Timestamp("2024-01-01")
	toTS := document.Timestamp("2024-12-31")
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

func TestFromDocumentReferenceError(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			Link: []document.LinkClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: identifier.New()},
					IRI:       "https://example.com",
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

func TestConvertStringPropagationError(t *testing.T) {
	t.Parallel()

	// Prop is known, but parent prop is not.
	propDoc := makeNamingDoc(testPropID, "Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)
	unknownParent := identifier.New()
	c.propertyAncestors = map[identifier.Identifier][]identifier.Identifier{
		testPropID: {unknownParent},
	}

	ctx := t.Context()
	claim := &document.StringClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: testPropID},
		String:    "hello",
	}
	_, errE := c.convertString(ctx, claim)
	assert.EqualError(t, errE, "document not found")
}

func TestConvertHTMLPropagationError(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)
	unknownParent := identifier.New()
	c.propertyAncestors = map[identifier.Identifier][]identifier.Identifier{
		testPropID: {unknownParent},
	}

	ctx := t.Context()
	claim := &document.HTMLClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: testPropID},
		HTML:      "<p>test</p>",
	}
	_, errE := c.convertHTML(ctx, claim)
	assert.EqualError(t, errE, "document not found")
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
	_, errE := c.convertAmount(ctx, claim)
	assert.EqualError(t, errE, "document not found")
}

func TestConvertAmountIntervalPropagationError(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)
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
	_, _, errE := c.convertAmountInterval(ctx, claim)
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
	_, _, errE := c.convertAmountInterval(ctx, claim)
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
	_, _, errE := c.convertAmountInterval(ctx, claim)
	assert.EqualError(t, errE, "unable to parse amount")
}

func TestConvertTimePropagationError(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)
	unknownParent := identifier.New()
	c.propertyAncestors = map[identifier.Identifier][]identifier.Identifier{
		testPropID: {unknownParent},
	}

	ctx := t.Context()
	claim := &document.TimeClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: testPropID},
		Timestamp: document.Timestamp("2024-01-15"),
		Precision: document.TimePrecisionDay,
	}
	_, errE := c.convertTime(ctx, claim)
	assert.EqualError(t, errE, "document not found")
}

func TestConvertTimeInvalidTimestamp(t *testing.T) {
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
		Timestamp: document.Timestamp("not-a-time"),
		Precision: document.TimePrecisionDay,
	}
	_, errE := c.convertTime(ctx, claim)
	assert.EqualError(t, errE, "unable to parse timestamp")
}

func TestConvertTimeIntervalPropagationError(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)
	unknownParent := identifier.New()
	c.propertyAncestors = map[identifier.Identifier][]identifier.Identifier{
		testPropID: {unknownParent},
	}

	ctx := t.Context()
	fromTS := document.Timestamp("2024-01-01")
	toTS := document.Timestamp("2024-12-31")
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
	_, _, errE := c.convertTimeInterval(ctx, claim)
	assert.EqualError(t, errE, "document not found")
}

func TestConvertTimeIntervalInvalidFromTimestamp(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	fromTS := document.Timestamp("not-a-time")
	fromPrec := document.TimePrecisionDay
	toTS := document.Timestamp("2024-12-31")
	toPrec := document.TimePrecisionDay
	claim := &document.TimeIntervalClaim{ //nolint:exhaustruct
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: testPropID},
		From:          &fromTS,
		FromPrecision: &fromPrec,
		To:            &toTS,
		ToPrecision:   &toPrec,
	}
	_, _, errE := c.convertTimeInterval(ctx, claim)
	assert.EqualError(t, errE, "unable to parse timestamp")
}

func TestConvertTimeIntervalInvalidToTimestamp(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)

	ctx := t.Context()
	fromTS := document.Timestamp("2024-01-01")
	fromPrec := document.TimePrecisionDay
	toTS := document.Timestamp("not-a-time")
	toPrec := document.TimePrecisionDay
	claim := &document.TimeIntervalClaim{ //nolint:exhaustruct
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: testPropID},
		From:          &fromTS,
		FromPrecision: &fromPrec,
		To:            &toTS,
		ToPrecision:   &toPrec,
	}
	_, _, errE := c.convertTimeInterval(ctx, claim)
	assert.EqualError(t, errE, "unable to parse timestamp")
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
	_, errE := c.convertReference(ctx, claim)
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
	_, errE := c.convertReference(ctx, claim)
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
	_, errE := c.convertReference(ctx, claim)
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
	_, errE := c.convertHas(ctx, claim)
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
	_, errE := c.convertHas(ctx, claim)
	assert.EqualError(t, errE, "document not found")
}

func TestConvertHasPropagationError(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)
	unknownParent := identifier.New()
	c.propertyAncestors = map[identifier.Identifier][]identifier.Identifier{
		testPropID: {unknownParent},
	}

	ctx := t.Context()
	claim := &document.HasClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: testPropID},
	}
	_, errE := c.convertHas(ctx, claim)
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
	_, errE := c.convertReference(ctx, claim)
	assert.EqualError(t, errE, "document not found")
}

func TestConvertReferencePropagationError(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)
	unknownParent := identifier.New()
	c.propertyAncestors = map[identifier.Identifier][]identifier.Identifier{
		testPropID: {unknownParent},
	}

	ctx := t.Context()
	claim := &document.LinkClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: testPropID},
		IRI:       "https://example.com",
	}
	_, errE := c.convertLink(ctx, claim)
	assert.EqualError(t, errE, "document not found")
}

func TestConvertNonePropagationError(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)
	unknownParent := identifier.New()
	c.propertyAncestors = map[identifier.Identifier][]identifier.Identifier{
		testPropID: {unknownParent},
	}

	ctx := t.Context()
	claim := &document.NoneClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: testPropID},
	}
	_, errE := c.convertNone(ctx, claim)
	assert.EqualError(t, errE, "document not found")
}

func TestConvertUnknownPropagationError(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)
	unknownParent := identifier.New()
	c.propertyAncestors = map[identifier.Identifier][]identifier.Identifier{
		testPropID: {unknownParent},
	}

	ctx := t.Context()
	claim := &document.UnknownClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: testPropID},
	}
	_, errE := c.convertUnknown(ctx, claim)
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
	_, _, errE := c.convertAmountInterval(ctx, claim)
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
	_, _, errE := c.convertAmountInterval(ctx, claim)
	assert.EqualError(t, errE, "document not found")
}

func TestConvertTimeIntervalFromUnknownToError(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	toTS := document.Timestamp("2024-06-15")
	toPrec := document.TimePrecisionDay
	claim := &document.TimeIntervalClaim{ //nolint:exhaustruct
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: identifier.New()},
		FromIsUnknown: true,
		To:            &toTS,
		ToPrecision:   &toPrec,
	}
	_, _, errE := c.convertTimeInterval(ctx, claim)
	assert.EqualError(t, errE, "document not found")
}

func TestConvertTimeIntervalToUnknownFromError(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	fromTS := document.Timestamp("2024-06-15")
	fromPrec := document.TimePrecisionDay
	claim := &document.TimeIntervalClaim{ //nolint:exhaustruct
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: identifier.New()},
		From:          &fromTS,
		FromPrecision: &fromPrec,
		ToIsUnknown:   true,
	}
	_, _, errE := c.convertTimeInterval(ctx, claim)
	assert.EqualError(t, errE, "document not found")
}

// makeClassDocWithTemplate creates a class document with an INSTANCE_OF PROPERTY
// and a DISPLAY_LABEL_TEMPLATE claim.
func makeClassDocWithTemplate(id identifier.Identifier, tmpl string) *document.D {
	claims := &document.ClaimTypes{}
	claims.Reference = append(claims.Reference, document.ReferenceClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: instanceOfPropID},
		To:        document.Reference{ID: propertyClassID},
	})
	claims.String = append(claims.String, document.StringClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: displayLabelTemplatePropID},
		String:    tmpl,
	})
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
		Prop:      document.Reference{ID: instanceOfPropID},
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
	assert.Equal(t, `{{bestString "SHORT_NAME" .}}`, result)
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
	assert.Empty(t, result)
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
	assert.Equal(t, "Template B", result)
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
	c.namingProperties = []identifier.Identifier{namingPropID}

	// Document with INSTANCE_OF class and naming + short name claims.
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: namingPropID},
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
	c.namingProperties = []identifier.Identifier{namingPropID}

	// Template with invalid syntax should return an error.
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: namingPropID},
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
	c.namingProperties = []identifier.Identifier{namingPropID}

	enSub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: inLanguagePropID},
				To:        document.Reference{ID: enLangID},
			},
		},
	}
	slSub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: inLanguagePropID},
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
					Prop:      document.Reference{ID: namingPropID},
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

	// Template is not per-language; all languages render it.
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
	c.namingProperties = []identifier.Identifier{namingPropID}
	c.languageCodes = map[identifier.Identifier]string{}

	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: namingPropID},
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
	c.namingProperties = []identifier.Identifier{namingPropID}

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
	c.namingProperties = []identifier.Identifier{namingPropID}

	enSub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: inLanguagePropID},
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
					Prop:      document.Reference{ID: namingPropID},
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
	c.namingProperties = []identifier.Identifier{namingPropID}

	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: namingPropID},
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
	c.namingProperties = []identifier.Identifier{namingPropID}
	c.languageCodes = map[identifier.Identifier]string{}

	// Template tries to follow a non-existent relation.
	// bestReferenceDoc returns nil, bestAmountString handles nil doc gracefully.
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: namingPropID},
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
	c.namingProperties = []identifier.Identifier{namingPropID}

	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: namingPropID},
					String:    "Some Name",
				},
			},
			Time: []document.TimeClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: datePropID},
					Timestamp: document.Timestamp("2024-06-15"),
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
					Prop:      document.Reference{ID: namingPropID},
					String:    "Other Document",
				},
			},
		},
	}
	classDoc := makeClassDocWithTemplate(classID,
		`{{getDocument `+idTmpl(otherDocID)+` | bestString `+idTmpl(namingPropID)+`}}`,
	)

	extraDocs := map[identifier.Identifier]*document.D{
		otherDocID: otherDoc,
		classID:    classDoc,
	}
	c := newTestConverter(t, nil, nil, extraDocs)
	c.namingProperties = []identifier.Identifier{namingPropID}
	c.languageCodes = map[identifier.Identifier]string{}

	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: namingPropID},
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
	c, errE := NewConverter(nil, nil, map[string][]string{"en": {"sl"}}, getDocument)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.NotNil(t, c)

	// Invalid priority.
	c, errE = NewConverter(nil, nil, map[string][]string{"xx": {"en"}}, getDocument)
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
				Prop:      document.Reference{ID: inLanguagePropID},
				To:        document.Reference{ID: enLangID},
			},
		},
	}
	slSub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: inLanguagePropID},
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
					Prop:      document.Reference{ID: namingPropID},
					String:    "English Name",
				},
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, slSub),
					Prop:      document.Reference{ID: namingPropID},
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

	// Priority: en falls back to sl, pt has no fallback.
	priority := map[string][]string{
		"en": {"sl", "und"},
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

func TestDisplayPathsNoFallback(t *testing.T) {
	t.Parallel()

	enLangID := identifier.New()
	slLangID := identifier.New()

	enSub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: inLanguagePropID},
				To:        document.Reference{ID: enLangID},
			},
		},
	}
	slSub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: inLanguagePropID},
				To:        document.Reference{ID: slLangID},
			},
		},
	}

	// Set up hierarchy.
	subentityDoc := makePropertyDoc(subentityOfPropID, nil)
	subclassDoc := makePropertyDoc(subclassOfPropID, &subentityOfPropID)
	subpropDoc := makePropertyDoc(subpropertyOfPropID, &subentityOfPropID)
	instanceDoc := makePropertyDoc(instanceOfPropID, &subentityOfPropID)
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
					Prop:      document.Reference{ID: namingPropID},
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
					Prop:      document.Reference{ID: namingPropID},
					String:    "Child EN",
				},
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, slSub),
					Prop:      document.Reference{ID: namingPropID},
					String:    "Child SL",
				},
			},
			Reference: []document.ReferenceClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: subclassOfPropID},
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
		parentID: parentDoc,
		childID:  childDoc,
	}

	// Priority: sl falls back to en. No fallback for pt.
	priority := map[string][]string{
		"sl": {"en"},
		"pt": {},
	}
	c := newTestConverterWithPriority(t, properties, languages, extraDocs, priority)

	ctx := t.Context()
	info, errE := c.getDocumentInfo(ctx, childID)
	require.NoError(t, errE, "% -+#.1v", errE)

	// English path exists: parent has EN, child has EN.
	require.Contains(t, info.DisplayPaths[subclassOfPropID], "en")
	assert.Equal(t, []string{"Parent EN\x00Child EN"}, info.DisplayPaths[subclassOfPropID]["en"])

	// Slovenian path: parent resolved to "Parent EN" via sl->en fallback, child has "Child SL".
	require.Contains(t, info.DisplayPaths[subclassOfPropID], "sl")
	assert.Equal(t, []string{"Parent EN\x00Child SL"}, info.DisplayPaths[subclassOfPropID]["sl"])

	// Portuguese: pt has no fallback, both parent and child have empty pt display.
	// Path is still created with empty strings.
	require.Contains(t, info.DisplayPaths[subclassOfPropID], "pt")
	assert.Equal(t, []string{"\x00"}, info.DisplayPaths[subclassOfPropID]["pt"])
}

func TestDisplayPathsEmptyAppend(t *testing.T) {
	t.Parallel()

	enLangID := identifier.New()

	enSub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: inLanguagePropID},
				To:        document.Reference{ID: enLangID},
			},
		},
	}

	// Set up hierarchy.
	subentityDoc := makePropertyDoc(subentityOfPropID, nil)
	subclassDoc := makePropertyDoc(subclassOfPropID, &subentityOfPropID)
	subpropDoc := makePropertyDoc(subpropertyOfPropID, &subentityOfPropID)
	instanceDoc := makePropertyDoc(instanceOfPropID, &subentityOfPropID)
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
					Prop:      document.Reference{ID: namingPropID},
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
					Prop:      document.Reference{ID: namingPropID},
					String:    "Child UND",
				},
			},
			Reference: []document.ReferenceClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: subclassOfPropID},
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
	require.Contains(t, info.DisplayPaths[subclassOfPropID], "en")
	assert.Equal(t, []string{"Parent EN\x00"}, info.DisplayPaths[subclassOfPropID]["en"])
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
				Prop:      document.Reference{ID: inLanguagePropID},
				To:        document.Reference{ID: enLangID},
			},
		},
	}
	slSub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: inLanguagePropID},
				To:        document.Reference{ID: slLangID},
			},
		},
	}
	ptSub := &document.ClaimTypes{
		Reference: []document.ReferenceClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: inLanguagePropID},
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
	c.namingProperties = []identifier.Identifier{namingPropID}

	// Document with template (via class). NAME only exists in "sl".
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, ptSub),
					Prop:      document.Reference{ID: namingPropID},
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
		Prop:      document.Reference{ID: inversePropertyOfPropID},
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

	outgoing := OutgoingInverseRelations(doc)

	// Should have an entry for the target document.
	require.Contains(t, outgoing, testTargetDocID)
	require.Len(t, outgoing[testTargetDocID], 1)
	ir := outgoing[testTargetDocID][0]
	assert.Equal(t, claimID, ir.Claim)
	assert.Equal(t, testDocID, ir.Source)
	assert.Equal(t, testPropID, ir.Prop)
	assert.InDelta(t, float64(document.HighConfidence), float64(ir.Confidence), 0)
}

func TestInverseRelationClaimIDDeterministic(t *testing.T) {
	t.Parallel()

	target := identifier.New()
	source := identifier.New()
	claim := identifier.New()

	id1 := inverseReferenceClaimID(target, source, claim)
	id2 := inverseReferenceClaimID(target, source, claim)

	assert.Equal(t, id1, id2)
}

func TestInverseRelationClaimIDDiffersPerSource(t *testing.T) {
	t.Parallel()

	target := identifier.New()
	sourceA := identifier.New()
	sourceB := identifier.New()
	claim := identifier.New()

	idA := inverseReferenceClaimID(target, sourceA, claim)
	idB := inverseReferenceClaimID(target, sourceB, claim)

	assert.NotEqual(t, idA, idB)
}

func TestOutgoingInverseRelationsEmpty(t *testing.T) {
	t.Parallel()

	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
	}

	outgoing := OutgoingInverseRelations(doc)
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
	inverseRelations := []internalStore.InverseRelation{
		{
			Claim:      claimID,
			Source:     sourceDocID,
			Prop:       propX,
			Target:     identifier.Identifier{},
			Confidence: document.HighConfidence,
		},
	}

	result, errE := c.FromDocument(ctx, doc, inverseRelations)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Should have a reverse relation claim with property Y pointing to source document.
	require.Len(t, result.Claims.Reference, 1)
	rel := result.Claims.Reference[0]
	assert.Equal(t, propY, rel.Prop)
	assert.Equal(t, sourceDocID, rel.To)
}

func TestFromDocumentIncomingInverseRelationNoInverse(t *testing.T) {
	t.Parallel()

	// Property X has no inverse.
	propX := identifier.New()
	propXDoc := makePropertyDocFull(propX, nil, nil)

	extraDocs := map[identifier.Identifier]*document.D{
		propX: makeNamingDoc(propX, "Prop X"),
	}
	c := newTestConverter(t, []*document.D{propXDoc}, nil, extraDocs)

	ctx := t.Context()
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
	}
	inverseRelations := []internalStore.InverseRelation{
		{
			Claim:      identifier.New(),
			Source:     identifier.New(),
			Prop:       propX,
			Target:     identifier.Identifier{},
			Confidence: document.HighConfidence,
		},
	}

	result, errE := c.FromDocument(ctx, doc, inverseRelations)
	require.NoError(t, errE, "% -+#.1v", errE)

	// No inverse property, so no relation claims should be added.
	assert.Empty(t, result.Claims.Reference)
}

func TestFromDocumentIncomingInverseRelationMultipleInverses(t *testing.T) {
	t.Parallel()

	// Both A and C have inversePropertyOf B.
	// An incoming relation with property B should produce reverse claims for both A and C.
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
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID}, //nolint:exhaustruct
	}
	inverseRelations := []internalStore.InverseRelation{
		{
			Claim:      identifier.New(),
			Source:     sourceDocID,
			Prop:       propB,
			Target:     identifier.Identifier{},
			Confidence: document.HighConfidence,
		},
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

	// Incoming relation with property A (the one that declared inversePropertyOf B).
	inverseRelations := []internalStore.InverseRelation{
		{
			Claim:      identifier.New(),
			Source:     sourceDocID,
			Prop:       propA,
			Target:     identifier.Identifier{},
			Confidence: document.HighConfidence,
		},
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

	current := map[identifier.Identifier][]internalStore.InverseRelation{}
	parent := map[identifier.Identifier][]internalStore.InverseRelation{}

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

	current := map[identifier.Identifier][]internalStore.InverseRelation{
		targetB: {{Claim: claim1, Source: docA, Prop: prop1, Target: targetB, Confidence: document.HighConfidence}},
	}
	parent := map[identifier.Identifier][]internalStore.InverseRelation{}

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

	current := map[identifier.Identifier][]internalStore.InverseRelation{}
	parent := map[identifier.Identifier][]internalStore.InverseRelation{
		targetB: {{Claim: claim1, Source: docA, Prop: prop1, Target: targetB, Confidence: document.HighConfidence}},
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

	ir := internalStore.InverseRelation{Claim: claim1, Source: docA, Prop: prop1, Target: targetB, Confidence: document.HighConfidence}
	current := map[identifier.Identifier][]internalStore.InverseRelation{targetB: {ir}}
	parent := map[identifier.Identifier][]internalStore.InverseRelation{targetB: {ir}}

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
	parent := map[identifier.Identifier][]internalStore.InverseRelation{
		targetB: {{Claim: claimOld, Source: docA, Prop: prop1, Target: targetB, Confidence: document.HighConfidence}},
	}
	current := map[identifier.Identifier][]internalStore.InverseRelation{
		targetC: {{Claim: claimNew, Source: docA, Prop: prop1, Target: targetC, Confidence: document.HighConfidence}},
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
	parent := map[identifier.Identifier][]internalStore.InverseRelation{
		targetB: {
			{Claim: claim1, Source: docA, Prop: prop1, Target: targetB, Confidence: document.HighConfidence},
			{Claim: claim2, Source: docA, Prop: prop1, Target: targetB, Confidence: document.HighConfidence},
		},
	}
	current := map[identifier.Identifier][]internalStore.InverseRelation{
		targetB: {
			{Claim: claim1, Source: docA, Prop: prop1, Target: targetB, Confidence: document.HighConfidence},
			{Claim: claimNew, Source: docA, Prop: prop1, Target: targetB, Confidence: document.HighConfidence},
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
	parent := map[identifier.Identifier][]internalStore.InverseRelation{
		targetB: {{Claim: claim1, Source: docA, Prop: prop1, Target: targetB, Confidence: document.HighConfidence}},
	}
	current := map[identifier.Identifier][]internalStore.InverseRelation{
		targetC: {{Claim: claim1, Source: docA, Prop: prop1, Target: targetC, Confidence: document.HighConfidence}},
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
	parent := map[identifier.Identifier][]internalStore.InverseRelation{
		targetB: {{Claim: claim1, Source: docA, Prop: prop1, Target: targetB, Confidence: document.HighConfidence}},
	}
	current := map[identifier.Identifier][]internalStore.InverseRelation{
		targetB: {{Claim: claim1, Source: docA, Prop: prop2, Target: targetB, Confidence: document.HighConfidence}},
	}

	added, removed := diffOutgoingInverseRelations(current, parent)

	// Should detect the property change as removal + addition.
	require.Len(t, added[targetB], 1)
	assert.Equal(t, prop2, added[targetB][0].Prop)

	require.Len(t, removed[targetB], 1)
	assert.Equal(t, prop1, removed[targetB][0].Prop)
}

func TestDiffOutgoingInverseRelationsSameClaimChangedConfidence(t *testing.T) {
	t.Parallel()

	docA := identifier.New()
	targetB := identifier.New()
	claim1 := identifier.New()
	prop1 := identifier.New()

	// Same claim ID, target, and prop, but confidence changed.
	parent := map[identifier.Identifier][]internalStore.InverseRelation{
		targetB: {{Claim: claim1, Source: docA, Prop: prop1, Target: targetB, Confidence: document.HighConfidence}},
	}
	current := map[identifier.Identifier][]internalStore.InverseRelation{
		targetB: {{Claim: claim1, Source: docA, Prop: prop1, Target: targetB, Confidence: document.LowConfidence}},
	}

	added, removed := diffOutgoingInverseRelations(current, parent)

	// Should detect the confidence change as removal + addition.
	require.Len(t, added[targetB], 1)
	assert.InDelta(t, float64(document.LowConfidence), float64(added[targetB][0].Confidence), 0)

	require.Len(t, removed[targetB], 1)
	assert.InDelta(t, float64(document.HighConfidence), float64(removed[targetB][0].Confidence), 0)
}
