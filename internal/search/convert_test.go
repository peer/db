package search

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/document"
)

// Helper IDs for tests.
//
//nolint:gochecknoglobals
var (
	testPropID      = identifier.New()
	testPropID2     = identifier.New()
	testParentProp  = identifier.New()
	testClassID     = identifier.New()
	testParentClass = identifier.New()
	testDocID       = identifier.New()
	testLangDocID   = identifier.New()
	testUnitDocID   = identifier.New()
	testTargetDocID = identifier.New()
)

// makeCoreClaim creates a CoreClaim with the given confidence and optional meta.
func makeCoreClaim(confidence document.Confidence, meta *document.ClaimTypes) document.CoreClaim {
	return document.CoreClaim{
		ID:         identifier.New(),
		Confidence: confidence,
		Meta:       meta,
	}
}

// makePropertyDoc creates a property document (instance of PROPERTY class) with optional SUBPROPERTY_OF relation.
func makePropertyDoc(id identifier.Identifier, subpropertyOf *identifier.Identifier) *document.D {
	claims := &document.ClaimTypes{} //nolint:exhaustruct
	// INSTANCE_OF -> PROPERTY.
	claims.Relation = append(claims.Relation, document.RelationClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: instanceOfPropID},
		To:        document.Reference{ID: propertyClassID},
	})
	if subpropertyOf != nil {
		claims.Relation = append(claims.Relation, document.RelationClaim{
			CoreClaim: makeCoreClaim(document.HighConfidence, nil),
			Prop:      document.Reference{ID: subpropertyOfPropID},
			To:        document.Reference{ID: *subpropertyOf},
		})
	}
	return &document.D{
		CoreDocument: document.CoreDocument{
			ID: id,
		},
		Claims: claims,
	}
}

// makeClassDoc creates a class document (instance of CLASS class) with optional SUBCLASS_OF relation.
func makeClassDoc(id identifier.Identifier, subclassOf *identifier.Identifier) *document.D {
	claims := &document.ClaimTypes{} //nolint:exhaustruct
	claims.Relation = append(claims.Relation, document.RelationClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: instanceOfPropID},
		To:        document.Reference{ID: classClassID},
	})
	if subclassOf != nil {
		claims.Relation = append(claims.Relation, document.RelationClaim{
			CoreClaim: makeCoreClaim(document.HighConfidence, nil),
			Prop:      document.Reference{ID: subclassOfPropID},
			To:        document.Reference{ID: *subclassOf},
		})
	}
	return &document.D{
		CoreDocument: document.CoreDocument{
			ID: id,
		},
		Claims: claims,
	}
}

// makeLanguageDoc creates a language document (instance of LANGUAGE class) with a CODE identifier.
func makeLanguageDoc(id identifier.Identifier, code string) *document.D {
	claims := &document.ClaimTypes{} //nolint:exhaustruct
	claims.Relation = append(claims.Relation, document.RelationClaim{
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
		CoreDocument: document.CoreDocument{
			ID: id,
		},
		Claims: claims,
	}
}

// makeNamingDoc creates a document with a naming string claim.
func makeNamingDoc(id identifier.Identifier, name string) *document.D {
	claims := &document.ClaimTypes{} //nolint:exhaustruct
	claims.String = append(claims.String, document.StringClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: namingPropID},
		String:    name,
	})
	return &document.D{
		CoreDocument: document.CoreDocument{
			ID: id,
		},
		Claims: claims,
	}
}

// newTestConverter creates a Converter for testing with the given properties, classes, and vocabularies.
func newTestConverter(
	t *testing.T,
	properties, classes, vocabularies []*document.D,
	extraDocs map[identifier.Identifier]*document.D,
) *Converter {
	t.Helper()
	getDocument := func(_ context.Context, id identifier.Identifier) (*document.D, errors.E) {
		if doc, ok := extraDocs[id]; ok {
			return doc, nil
		}
		return nil, errors.New("document not found")
	}
	return NewConverter(properties, classes, vocabularies, nil, getDocument)
}

func TestIsInstanceOf(t *testing.T) {
	t.Parallel()

	doc := makePropertyDoc(testPropID, nil)
	assert.True(t, isInstanceOf(doc, propertyClassID))
	assert.False(t, isInstanceOf(doc, classClassID))

	// Document with no claims.
	emptyDoc := &document.D{
		CoreDocument: document.CoreDocument{ID: identifier.New()},
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
		displayCache: make(map[identifier.Identifier]displayStrings),
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
		displayCache: make(map[identifier.Identifier]displayStrings),
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
		CoreDocument: document.CoreDocument{ID: identifier.New()},
		Claims: &document.ClaimTypes{ //nolint:exhaustruct
			Relation: []document.RelationClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: subpropertyOfPropID},
					To:        document.Reference{ID: testPropID},
				},
			},
		},
	}

	c := &Converter{ //nolint:exhaustruct
		displayCache: make(map[identifier.Identifier]displayStrings),
	}
	c.buildPropertyHierarchy([]*document.D{notProp})

	assert.Empty(t, c.propertyDescendants)
	assert.Empty(t, c.propertyAncestors)
}

func TestBuildClassHierarchy(t *testing.T) {
	t.Parallel()

	child := makeClassDoc(testClassID, &testParentClass)
	parent := makeClassDoc(testParentClass, nil)

	classes := []*document.D{parent, child}
	c := &Converter{ //nolint:exhaustruct
		displayCache: make(map[identifier.Identifier]displayStrings),
	}
	c.buildClassHierarchy(classes)

	assert.Contains(t, c.classAncestors[testClassID], testParentClass)
	assert.Empty(t, c.classAncestors[testParentClass])
}

func TestBuildClassHierarchyTransitive(t *testing.T) {
	t.Parallel()

	grandparent := identifier.New()
	parent := identifier.New()
	child := identifier.New()

	gpDoc := makeClassDoc(grandparent, nil)
	pDoc := makeClassDoc(parent, &grandparent)
	cDoc := makeClassDoc(child, &parent)

	c := &Converter{ //nolint:exhaustruct
		displayCache: make(map[identifier.Identifier]displayStrings),
	}
	c.buildClassHierarchy([]*document.D{gpDoc, pDoc, cDoc})

	assert.Contains(t, c.classAncestors[child], parent)
	assert.Contains(t, c.classAncestors[child], grandparent)
}

func TestBuildClassHierarchySkipsNonClass(t *testing.T) {
	t.Parallel()

	notClass := &document.D{
		CoreDocument: document.CoreDocument{ID: identifier.New()},
		Claims: &document.ClaimTypes{ //nolint:exhaustruct
			Relation: []document.RelationClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: subclassOfPropID},
					To:        document.Reference{ID: testParentClass},
				},
			},
		},
	}

	c := &Converter{ //nolint:exhaustruct
		displayCache: make(map[identifier.Identifier]displayStrings),
	}
	c.buildClassHierarchy([]*document.D{notClass})

	assert.Empty(t, c.classAncestors)
}

func TestBuildNamingProperties(t *testing.T) {
	t.Parallel()

	// namingPropID is NAMING, testPropID is a subproperty of NAMING.
	namingDoc := makePropertyDoc(namingPropID, nil)
	subNaming := makePropertyDoc(testPropID, &namingPropID)

	c := &Converter{ //nolint:exhaustruct
		displayCache: make(map[identifier.Identifier]displayStrings),
	}
	c.buildPropertyHierarchy([]*document.D{namingDoc, subNaming})
	c.buildNamingProperties()

	assert.True(t, c.namingProperties[namingPropID])
	assert.True(t, c.namingProperties[testPropID])
	assert.False(t, c.namingProperties[testPropID2])
}

func TestBuildLanguageCodes(t *testing.T) {
	t.Parallel()

	enDoc := makeLanguageDoc(testLangDocID, "en")
	slID := identifier.New()
	slDoc := makeLanguageDoc(slID, "sl")

	c := &Converter{ //nolint:exhaustruct
		displayCache: make(map[identifier.Identifier]displayStrings),
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
		displayCache: make(map[identifier.Identifier]displayStrings),
	}
	c.buildLanguageCodes([]*document.D{langDoc})

	assert.Equal(t, "en", c.languageCodes[testLangDocID])
}

func TestBuildLanguageCodesSkipsNonLanguage(t *testing.T) {
	t.Parallel()

	// Not an instance of LANGUAGE.
	notLang := &document.D{
		CoreDocument: document.CoreDocument{ID: identifier.New()},
		Claims: &document.ClaimTypes{ //nolint:exhaustruct
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
		displayCache: make(map[identifier.Identifier]displayStrings),
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

	// Meta with IN_LANGUAGE relation to a known language.
	meta := &document.ClaimTypes{ //nolint:exhaustruct
		Relation: []document.RelationClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: inLanguagePropID},
				To:        document.Reference{ID: testLangDocID},
			},
		},
	}
	langs := c.extractInLanguages(meta)
	assert.Equal(t, []string{"en"}, langs)

	// No meta.
	langs = c.extractInLanguages(nil)
	assert.Equal(t, []string{"und"}, langs)

	// Meta with unknown language.
	unknownLangID := identifier.New()
	metaUnknown := &document.ClaimTypes{ //nolint:exhaustruct
		Relation: []document.RelationClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: inLanguagePropID},
				To:        document.Reference{ID: unknownLangID},
			},
		},
	}
	langs = c.extractInLanguages(metaUnknown)
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

	meta := &document.ClaimTypes{ //nolint:exhaustruct
		Relation: []document.RelationClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: inLanguagePropID},
				To:        document.Reference{ID: xxLangID},
			},
		},
	}
	langs := c.extractInLanguages(meta)
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

	meta := &document.ClaimTypes{ //nolint:exhaustruct
		Relation: []document.RelationClaim{
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
	langs := c.extractInLanguages(meta)
	assert.Len(t, langs, 2)
	assert.Contains(t, langs, "en")
	assert.Contains(t, langs, "sl")
}

func TestExtractInUnit(t *testing.T) {
	t.Parallel()

	c := &Converter{} //nolint:exhaustruct

	// Meta with IN_UNIT relation.
	meta := &document.ClaimTypes{ //nolint:exhaustruct
		Relation: []document.RelationClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: inUnitPropID},
				To:        document.Reference{ID: testUnitDocID},
			},
		},
	}
	unit := c.extractInUnit(meta)
	require.NotNil(t, unit)
	assert.Equal(t, testUnitDocID, *unit)

	// No meta.
	unit = c.extractInUnit(nil)
	assert.Nil(t, unit)

	// Empty meta.
	unit = c.extractInUnit(&document.ClaimTypes{}) //nolint:exhaustruct
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
		namingProperties: map[identifier.Identifier]bool{
			namingPropID: true,
		},
		languageCodes: map[identifier.Identifier]string{},
	}

	doc := makeNamingDoc(testDocID, "Test Document")
	result := c.namingStrings(doc)
	require.NotNil(t, result)
	assert.Equal(t, []string{"Test Document"}, result["und"])
}

func TestNamingStringsEmpty(t *testing.T) {
	t.Parallel()

	c := &Converter{ //nolint:exhaustruct
		namingProperties: map[identifier.Identifier]bool{
			namingPropID: true,
		},
		languageCodes: map[identifier.Identifier]string{},
	}

	// Document with no naming strings.
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID},
	}
	result := c.namingStrings(doc)
	assert.Nil(t, result)
}

func TestNamingStringsSorted(t *testing.T) {
	t.Parallel()

	c := &Converter{ //nolint:exhaustruct
		namingProperties: map[identifier.Identifier]bool{
			namingPropID: true,
		},
		languageCodes: map[identifier.Identifier]string{},
	}

	// Two naming strings with different confidences.
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID},
		Claims: &document.ClaimTypes{ //nolint:exhaustruct
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
		namingProperties: map[identifier.Identifier]bool{
			namingPropID: true,
		},
		languageCodes: map[identifier.Identifier]string{},
	}

	// Two naming strings: first becomes Display, rest become Naming.
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID},
		Claims: &document.ClaimTypes{ //nolint:exhaustruct
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
	assert.Equal(t, []string{"Secondary"}, display.Naming["und"])
}

func TestGetDisplayStringsCache(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Test Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}

	c := newTestConverter(t, nil, nil, nil, extraDocs)

	ctx := t.Context()
	ds1, errE := c.getDisplayStrings(ctx, testPropID)
	require.NoError(t, errE, "% -+#.1v", errE)
	ds2, errE := c.getDisplayStrings(ctx, testPropID)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, ds1, ds2)
}

func TestGetDisplayStringsNotFound(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	_, errE := c.getDisplayStrings(ctx, identifier.New())
	assert.Error(t, errE)
}

func TestNewConverter(t *testing.T) {
	t.Parallel()

	namingDoc := makePropertyDoc(namingPropID, nil)
	subProp := makePropertyDoc(testPropID, &namingPropID)
	classDoc := makeClassDoc(testClassID, nil)
	langDoc := makeLanguageDoc(testLangDocID, "en")

	extraDocs := map[identifier.Identifier]*document.D{}
	c := newTestConverter(t, []*document.D{namingDoc, subProp}, []*document.D{classDoc}, []*document.D{langDoc}, extraDocs)

	assert.True(t, c.namingProperties[namingPropID])
	assert.True(t, c.namingProperties[testPropID])
	assert.Equal(t, "en", c.languageCodes[testLangDocID])
	assert.NotNil(t, c.displayCache)
}

func TestConvertIdentifier(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "My Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, nil, extraDocs)

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

	c := newTestConverter(t, nil, nil, nil, extraDocs)
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

	c := newTestConverter(t, nil, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	claim := &document.IdentifierClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: identifier.New()},
		Value:     "Q42",
	}
	_, errE := c.convertIdentifier(ctx, claim)
	assert.Error(t, errE)
}

func TestConvertString(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Str Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, nil, extraDocs)

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
	c := newTestConverter(t, nil, nil, nil, extraDocs)
	c.languageCodes = map[identifier.Identifier]string{
		testLangDocID: "en",
	}

	ctx := t.Context()
	meta := &document.ClaimTypes{ //nolint:exhaustruct
		Relation: []document.RelationClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: inLanguagePropID},
				To:        document.Reference{ID: testLangDocID},
			},
		},
	}
	claim := &document.StringClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, meta),
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
	c := newTestConverter(t, nil, nil, nil, extraDocs)

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
	c := newTestConverter(t, nil, nil, nil, extraDocs)
	c.languageCodes = map[identifier.Identifier]string{
		testLangDocID: "sl",
	}

	ctx := t.Context()
	meta := &document.ClaimTypes{ //nolint:exhaustruct
		Relation: []document.RelationClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: inLanguagePropID},
				To:        document.Reference{ID: testLangDocID},
			},
		},
	}
	claim := &document.HTMLClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, meta),
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
	c := newTestConverter(t, nil, nil, nil, extraDocs)

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
	c := newTestConverter(t, nil, nil, nil, extraDocs)

	ctx := t.Context()
	meta := &document.ClaimTypes{ //nolint:exhaustruct
		Relation: []document.RelationClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: inUnitPropID},
				To:        document.Reference{ID: testUnitDocID},
			},
		},
	}
	claim := &document.AmountClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, meta),
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
	c := newTestConverter(t, nil, nil, nil, extraDocs)

	ctx := t.Context()
	fromAmount := document.Amount("10")
	toAmount := document.Amount("20")
	fromPrec := 1.0
	toPrec := 1.0
	claim := &document.AmountIntervalClaim{
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
	c := newTestConverter(t, nil, nil, nil, extraDocs)

	ctx := t.Context()
	fromAmount := document.Amount("10")
	toAmount := document.Amount("20")
	fromPrec := 1.0
	toPrec := 1.0
	claim := &document.AmountIntervalClaim{
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
	c := newTestConverter(t, nil, nil, nil, extraDocs)

	ctx := t.Context()
	toAmount := document.Amount("20")
	toPrec := 1.0
	claim := &document.AmountIntervalClaim{
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
	assert.Equal(t, -math.MaxFloat64, *amountClaims[0].Range.GreaterThanOrEqual)
}

func TestConvertAmountIntervalToNone(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Amount Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, nil, extraDocs)

	ctx := t.Context()
	fromAmount := document.Amount("10")
	fromPrec := 1.0
	claim := &document.AmountIntervalClaim{
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
	assert.Equal(t, math.MaxFloat64, *amountClaims[0].Range.LessThanOrEqual)
}

func TestConvertAmountIntervalFromUnknownWithTo(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Amount Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, nil, extraDocs)

	ctx := t.Context()
	toAmount := document.Amount("20")
	toPrec := 1.0
	claim := &document.AmountIntervalClaim{
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
	c := newTestConverter(t, nil, nil, nil, extraDocs)

	ctx := t.Context()
	fromAmount := document.Amount("10")
	fromPrec := 1.0
	claim := &document.AmountIntervalClaim{
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
	c := newTestConverter(t, nil, nil, nil, extraDocs)

	ctx := t.Context()
	claim := &document.AmountIntervalClaim{
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
	c := newTestConverter(t, nil, nil, nil, extraDocs)

	ctx := t.Context()
	claim := &document.AmountIntervalClaim{
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

	c := newTestConverter(t, nil, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	fromAmount := document.Amount("10")
	toAmount := document.Amount("20")
	toPrec := 1.0
	claim := &document.AmountIntervalClaim{
		CoreClaim:   makeCoreClaim(document.HighConfidence, nil),
		Prop:        document.Reference{ID: testPropID},
		From:        &fromAmount,
		To:          &toAmount,
		ToPrecision: &toPrec,
	}
	_, _, errE := c.convertAmountInterval(ctx, claim)
	assert.Error(t, errE)
	assert.Contains(t, errE.Error(), "missing from precision")
}

func TestConvertAmountIntervalMissingToPrecision(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Amount Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, nil, extraDocs)

	ctx := t.Context()
	fromAmount := document.Amount("10")
	fromPrec := 1.0
	toAmount := document.Amount("20")
	claim := &document.AmountIntervalClaim{
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: testPropID},
		From:          &fromAmount,
		FromPrecision: &fromPrec,
		To:            &toAmount,
	}
	_, _, errE := c.convertAmountInterval(ctx, claim)
	assert.Error(t, errE)
	assert.Contains(t, errE.Error(), "missing to precision")
}

func TestConvertAmountIntervalFromUnknownMissingToPrecision(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	toAmount := document.Amount("20")
	claim := &document.AmountIntervalClaim{
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: testPropID},
		FromIsUnknown: true,
		To:            &toAmount,
	}
	_, _, errE := c.convertAmountInterval(ctx, claim)
	assert.Error(t, errE)
	assert.Contains(t, errE.Error(), "missing to precision")
}

func TestConvertAmountIntervalToUnknownMissingFromPrecision(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	fromAmount := document.Amount("10")
	claim := &document.AmountIntervalClaim{
		CoreClaim:   makeCoreClaim(document.HighConfidence, nil),
		Prop:        document.Reference{ID: testPropID},
		From:        &fromAmount,
		ToIsUnknown: true,
	}
	_, _, errE := c.convertAmountInterval(ctx, claim)
	assert.Error(t, errE)
	assert.Contains(t, errE.Error(), "missing from precision")
}

func TestConvertTime(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Time Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, nil, extraDocs)

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
	fromTime := time.Unix(*result[0].From, 0).UTC()
	toTime := time.Unix(*result[0].To, 0).UTC()
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
	c := newTestConverter(t, nil, nil, nil, extraDocs)

	ctx := t.Context()
	fromTs := document.Timestamp("2024-01-01")
	toTs := document.Timestamp("2024-12-31")
	fromPrec := document.TimePrecisionDay
	toPrec := document.TimePrecisionDay
	claim := &document.TimeIntervalClaim{
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: testPropID},
		From:          &fromTs,
		FromPrecision: &fromPrec,
		To:            &toTs,
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
	c := newTestConverter(t, nil, nil, nil, extraDocs)

	ctx := t.Context()
	fromTs := document.Timestamp("2024-01-01")
	toTs := document.Timestamp("2024-12-31")
	fromPrec := document.TimePrecisionDay
	toPrec := document.TimePrecisionDay
	claim := &document.TimeIntervalClaim{
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: testPropID},
		From:          &fromTs,
		FromPrecision: &fromPrec,
		FromIsOpen:    true,
		To:            &toTs,
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
	c := newTestConverter(t, nil, nil, nil, extraDocs)

	ctx := t.Context()
	toTs := document.Timestamp("2024-12-31")
	toPrec := document.TimePrecisionDay
	claim := &document.TimeIntervalClaim{
		CoreClaim:   makeCoreClaim(document.HighConfidence, nil),
		Prop:        document.Reference{ID: testPropID},
		FromIsNone:  true,
		To:          &toTs,
		ToPrecision: &toPrec,
	}
	timeClaims, unknownClaims, errE := c.convertTimeInterval(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, timeClaims, 1)
	assert.Empty(t, unknownClaims)
	assert.Nil(t, timeClaims[0].From)
	assert.Equal(t, int64(math.MinInt64), *timeClaims[0].Range.GreaterThanOrEqual)
}

func TestConvertTimeIntervalToNone(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Time Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, nil, extraDocs)

	ctx := t.Context()
	fromTs := document.Timestamp("2024-01-01")
	fromPrec := document.TimePrecisionDay
	claim := &document.TimeIntervalClaim{
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: testPropID},
		From:          &fromTs,
		FromPrecision: &fromPrec,
		ToIsNone:      true,
	}
	timeClaims, unknownClaims, errE := c.convertTimeInterval(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, timeClaims, 1)
	assert.Empty(t, unknownClaims)
	assert.Nil(t, timeClaims[0].To)
	assert.Equal(t, int64(math.MaxInt64), *timeClaims[0].Range.LessThanOrEqual)
}

func TestConvertTimeIntervalFromUnknownWithTo(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Time Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, nil, extraDocs)

	ctx := t.Context()
	toTs := document.Timestamp("2024-06-15")
	toPrec := document.TimePrecisionDay
	claim := &document.TimeIntervalClaim{
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: testPropID},
		FromIsUnknown: true,
		To:            &toTs,
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
	c := newTestConverter(t, nil, nil, nil, extraDocs)

	ctx := t.Context()
	fromTs := document.Timestamp("2024-06-15")
	fromPrec := document.TimePrecisionDay
	claim := &document.TimeIntervalClaim{
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: testPropID},
		From:          &fromTs,
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
	c := newTestConverter(t, nil, nil, nil, extraDocs)

	ctx := t.Context()
	claim := &document.TimeIntervalClaim{
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
	c := newTestConverter(t, nil, nil, nil, extraDocs)

	ctx := t.Context()
	claim := &document.TimeIntervalClaim{
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

	c := newTestConverter(t, nil, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	fromTs := document.Timestamp("2024-01-01")
	toTs := document.Timestamp("2024-12-31")
	toPrec := document.TimePrecisionDay
	claim := &document.TimeIntervalClaim{
		CoreClaim:   makeCoreClaim(document.HighConfidence, nil),
		Prop:        document.Reference{ID: testPropID},
		From:        &fromTs,
		To:          &toTs,
		ToPrecision: &toPrec,
	}
	_, _, errE := c.convertTimeInterval(ctx, claim)
	assert.Error(t, errE)
	assert.Contains(t, errE.Error(), "missing from precision")
}

func TestConvertTimeIntervalMissingToPrecision(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Time Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, nil, extraDocs)

	ctx := t.Context()
	fromTs := document.Timestamp("2024-01-01")
	fromPrec := document.TimePrecisionDay
	toTs := document.Timestamp("2024-12-31")
	claim := &document.TimeIntervalClaim{
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: testPropID},
		From:          &fromTs,
		FromPrecision: &fromPrec,
		To:            &toTs,
	}
	_, _, errE := c.convertTimeInterval(ctx, claim)
	assert.Error(t, errE)
	assert.Contains(t, errE.Error(), "missing to precision")
}

func TestConvertTimeIntervalFromUnknownMissingToPrecision(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	toTs := document.Timestamp("2024-12-31")
	claim := &document.TimeIntervalClaim{
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: testPropID},
		FromIsUnknown: true,
		To:            &toTs,
	}
	_, _, errE := c.convertTimeInterval(ctx, claim)
	assert.Error(t, errE)
	assert.Contains(t, errE.Error(), "missing to precision")
}

func TestConvertTimeIntervalToUnknownMissingFromPrecision(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	fromTs := document.Timestamp("2024-01-01")
	claim := &document.TimeIntervalClaim{
		CoreClaim:   makeCoreClaim(document.HighConfidence, nil),
		Prop:        document.Reference{ID: testPropID},
		From:        &fromTs,
		ToIsUnknown: true,
	}
	_, _, errE := c.convertTimeInterval(ctx, claim)
	assert.Error(t, errE)
	assert.Contains(t, errE.Error(), "missing from precision")
}

func TestConvertReference(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Ref Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, nil, extraDocs)

	ctx := t.Context()
	claim := &document.ReferenceClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: testPropID},
		IRI:       "https://example.com",
	}
	result, errE := c.convertReference(ctx, claim)
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
	c := newTestConverter(t, nil, nil, nil, extraDocs)

	ctx := t.Context()
	claim := &document.RelationClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: testPropID},
		To:        document.Reference{ID: testTargetDocID},
	}
	result, errE := c.convertRelation(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, result, 1)
	assert.Equal(t, testPropID, result[0].Prop)
	assert.Equal(t, testTargetDocID, result[0].To)
	assert.Equal(t, "Target", result[0].ToDisplay["und"])
}

func TestConvertRelationWithClassAncestors(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Rel Prop")
	targetDoc := makeNamingDoc(testTargetDocID, "Target")
	parentDoc := makeNamingDoc(testParentClass, "Parent Class")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID:      propDoc,
		testTargetDocID: targetDoc,
		testParentClass: parentDoc,
	}
	c := newTestConverter(t, nil, nil, nil, extraDocs)
	c.classAncestors = map[identifier.Identifier][]identifier.Identifier{
		testTargetDocID: {testParentClass},
	}

	ctx := t.Context()
	claim := &document.RelationClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: testPropID},
		To:        document.Reference{ID: testTargetDocID},
	}
	result, errE := c.convertRelation(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	// Should produce claims for both target and parent class.
	require.Len(t, result, 2)
	assert.Equal(t, testTargetDocID, result[0].To)
	assert.Equal(t, testParentClass, result[1].To)
}

func TestConvertRelationWithMetaRelations(t *testing.T) {
	t.Parallel()

	metaPropID := identifier.New()
	metaTargetID := identifier.New()
	propDoc := makeNamingDoc(testPropID, "Rel Prop")
	targetDoc := makeNamingDoc(testTargetDocID, "Target")
	metaPropDoc := makeNamingDoc(metaPropID, "Meta Prop")
	metaTargetDoc := makeNamingDoc(metaTargetID, "Meta Target")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID:      propDoc,
		testTargetDocID: targetDoc,
		metaPropID:      metaPropDoc,
		metaTargetID:    metaTargetDoc,
	}
	c := newTestConverter(t, nil, nil, nil, extraDocs)

	ctx := t.Context()
	meta := &document.ClaimTypes{ //nolint:exhaustruct
		Relation: []document.RelationClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: metaPropID},
				To:        document.Reference{ID: metaTargetID},
			},
		},
	}
	claim := &document.RelationClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, meta),
		Prop:      document.Reference{ID: testPropID},
		To:        document.Reference{ID: testTargetDocID},
	}
	result, errE := c.convertRelation(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, result, 1)
	require.Len(t, result[0].Relation, 1)
	assert.Equal(t, metaPropID, result[0].Relation[0].Prop)
	assert.Equal(t, metaTargetID, result[0].Relation[0].To)
}

func TestConvertHas(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Has Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, nil, extraDocs)

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

func TestConvertHasWithMetaRelations(t *testing.T) {
	t.Parallel()

	metaPropID := identifier.New()
	metaTargetID := identifier.New()
	propDoc := makeNamingDoc(testPropID, "Has Prop")
	metaPropDoc := makeNamingDoc(metaPropID, "Meta Prop")
	metaTargetDoc := makeNamingDoc(metaTargetID, "Meta Target")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID:   propDoc,
		metaPropID:   metaPropDoc,
		metaTargetID: metaTargetDoc,
	}
	c := newTestConverter(t, nil, nil, nil, extraDocs)

	ctx := t.Context()
	meta := &document.ClaimTypes{ //nolint:exhaustruct
		Relation: []document.RelationClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: metaPropID},
				To:        document.Reference{ID: metaTargetID},
			},
		},
	}
	claim := &document.HasClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, meta),
		Prop:      document.Reference{ID: testPropID},
	}
	result, errE := c.convertHas(ctx, claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, result, 1)
	require.Len(t, result[0].Relation, 1)
}

func TestConvertNone(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "None Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, nil, extraDocs)

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
	c := newTestConverter(t, nil, nil, nil, extraDocs)

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
	c := newTestConverter(t, nil, nil, nil, extraDocs)

	ctx := t.Context()
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID},
		Claims: &document.ClaimTypes{ //nolint:exhaustruct
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

	result, errE := c.FromDocument(ctx, doc)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, testDocID, result.ID)
	assert.Len(t, result.Claims.Identifier, 1)
	assert.Len(t, result.Claims.String, 1)
}

func TestFromDocumentNilClaims(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID},
	}

	result, errE := c.FromDocument(ctx, doc)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, testDocID, result.ID)
	assert.Empty(t, result.Claims.Identifier)
}

func TestFromDocumentAllClaimTypes(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "My Prop")
	targetDoc := makeNamingDoc(testTargetDocID, "Target")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID:      propDoc,
		testTargetDocID: targetDoc,
	}
	c := newTestConverter(t, nil, nil, nil, extraDocs)

	ctx := t.Context()
	fromAmount := document.Amount("5")
	toAmount := document.Amount("10")
	fromPrec := 1.0
	toPrec := 1.0
	fromTs := document.Timestamp("2024-01-01")
	toTs := document.Timestamp("2024-12-31")
	fromTsPrec := document.TimePrecisionDay
	toTsPrec := document.TimePrecisionDay

	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID},
		Claims: &document.ClaimTypes{ //nolint:exhaustruct
			Identifier: []document.IdentifierClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: testPropID},
					Value:     "ID1",
				},
			},
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: testPropID},
					String:    "str",
				},
			},
			HTML: []document.HTMLClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: testPropID},
					HTML:      "<p>html</p>",
				},
			},
			Amount: []document.AmountClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: testPropID},
					Amount:    document.Amount("42"),
					Precision: 1,
				},
			},
			AmountInterval: []document.AmountIntervalClaim{
				{
					CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
					Prop:          document.Reference{ID: testPropID},
					From:          &fromAmount,
					FromPrecision: &fromPrec,
					To:            &toAmount,
					ToPrecision:   &toPrec,
				},
			},
			Time: []document.TimeClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: testPropID},
					Timestamp: document.Timestamp("2024-06-15"),
					Precision: document.TimePrecisionDay,
				},
			},
			TimeInterval: []document.TimeIntervalClaim{
				{
					CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
					Prop:          document.Reference{ID: testPropID},
					From:          &fromTs,
					FromPrecision: &fromTsPrec,
					To:            &toTs,
					ToPrecision:   &toTsPrec,
				},
			},
			Reference: []document.ReferenceClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: testPropID},
					IRI:       "https://example.com",
				},
			},
			Relation: []document.RelationClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: testPropID},
					To:        document.Reference{ID: testTargetDocID},
				},
			},
			Has: []document.HasClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: testPropID},
				},
			},
			None: []document.NoneClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: testPropID},
				},
			},
			Unknown: []document.UnknownClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: testPropID},
				},
			},
		},
	}

	result, errE := c.FromDocument(ctx, doc)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, testDocID, result.ID)
	assert.Len(t, result.Claims.Identifier, 1)
	assert.Len(t, result.Claims.String, 1)
	assert.Len(t, result.Claims.HTML, 1)
	// Amount + AmountInterval.
	assert.Len(t, result.Claims.Amount, 2)
	// Time + TimeInterval.
	assert.Len(t, result.Claims.Time, 2)
	assert.Len(t, result.Claims.Reference, 1)
	assert.Len(t, result.Claims.Relation, 1)
	assert.Len(t, result.Claims.Has, 1)
	assert.Len(t, result.Claims.None, 1)
	assert.Len(t, result.Claims.Unknown, 1)
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
	c := newTestConverter(t, nil, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID},
		Claims: &document.ClaimTypes{ //nolint:exhaustruct
			Identifier: []document.IdentifierClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: identifier.New()},
					Value:     "Q42",
				},
			},
		},
	}

	_, errE := c.FromDocument(ctx, doc)
	assert.Error(t, errE)
}

func TestFromDocumentStringError(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID},
		Claims: &document.ClaimTypes{ //nolint:exhaustruct
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: identifier.New()},
					String:    "str",
				},
			},
		},
	}

	_, errE := c.FromDocument(ctx, doc)
	assert.Error(t, errE)
}

func TestFromDocumentHTMLError(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID},
		Claims: &document.ClaimTypes{ //nolint:exhaustruct
			HTML: []document.HTMLClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: identifier.New()},
					HTML:      "<p>test</p>",
				},
			},
		},
	}

	_, errE := c.FromDocument(ctx, doc)
	assert.Error(t, errE)
}

func TestFromDocumentAmountError(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID},
		Claims: &document.ClaimTypes{ //nolint:exhaustruct
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

	_, errE := c.FromDocument(ctx, doc)
	assert.Error(t, errE)
}

func TestFromDocumentAmountIntervalError(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	fromAmount := document.Amount("5")
	toAmount := document.Amount("10")
	fromPrec := 1.0
	toPrec := 1.0
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID},
		Claims: &document.ClaimTypes{ //nolint:exhaustruct
			AmountInterval: []document.AmountIntervalClaim{
				{
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

	_, errE := c.FromDocument(ctx, doc)
	assert.Error(t, errE)
}

func TestFromDocumentTimeError(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID},
		Claims: &document.ClaimTypes{ //nolint:exhaustruct
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

	_, errE := c.FromDocument(ctx, doc)
	assert.Error(t, errE)
}

func TestFromDocumentTimeIntervalError(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	fromTs := document.Timestamp("2024-01-01")
	toTs := document.Timestamp("2024-12-31")
	fromPrec := document.TimePrecisionDay
	toPrec := document.TimePrecisionDay
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID},
		Claims: &document.ClaimTypes{ //nolint:exhaustruct
			TimeInterval: []document.TimeIntervalClaim{
				{
					CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
					Prop:          document.Reference{ID: identifier.New()},
					From:          &fromTs,
					FromPrecision: &fromPrec,
					To:            &toTs,
					ToPrecision:   &toPrec,
				},
			},
		},
	}

	_, errE := c.FromDocument(ctx, doc)
	assert.Error(t, errE)
}

func TestFromDocumentReferenceError(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID},
		Claims: &document.ClaimTypes{ //nolint:exhaustruct
			Reference: []document.ReferenceClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: identifier.New()},
					IRI:       "https://example.com",
				},
			},
		},
	}

	_, errE := c.FromDocument(ctx, doc)
	assert.Error(t, errE)
}

func TestFromDocumentRelationError(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID},
		Claims: &document.ClaimTypes{ //nolint:exhaustruct
			Relation: []document.RelationClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: identifier.New()},
					To:        document.Reference{ID: identifier.New()},
				},
			},
		},
	}

	_, errE := c.FromDocument(ctx, doc)
	assert.Error(t, errE)
}

func TestFromDocumentHasError(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID},
		Claims: &document.ClaimTypes{ //nolint:exhaustruct
			Has: []document.HasClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: identifier.New()},
				},
			},
		},
	}

	_, errE := c.FromDocument(ctx, doc)
	assert.Error(t, errE)
}

func TestFromDocumentNoneError(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID},
		Claims: &document.ClaimTypes{ //nolint:exhaustruct
			None: []document.NoneClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: identifier.New()},
				},
			},
		},
	}

	_, errE := c.FromDocument(ctx, doc)
	assert.Error(t, errE)
}

func TestFromDocumentUnknownError(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID},
		Claims: &document.ClaimTypes{ //nolint:exhaustruct
			Unknown: []document.UnknownClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: identifier.New()},
				},
			},
		},
	}

	_, errE := c.FromDocument(ctx, doc)
	assert.Error(t, errE)
}

func TestGetDisplayStringsMakeDisplayError(t *testing.T) {
	t.Parallel()

	// Document with no naming strings at all.
	docID := identifier.New()
	emptyDoc := &document.D{
		CoreDocument: document.CoreDocument{ID: docID},
	}
	extraDocs := map[identifier.Identifier]*document.D{
		docID: emptyDoc,
	}

	c := newTestConverter(t, nil, nil, nil, extraDocs)

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
	c := newTestConverter(t, nil, nil, nil, extraDocs)
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
	assert.Error(t, errE)
}

func TestConvertHTMLPropagationError(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, nil, extraDocs)
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
	assert.Error(t, errE)
}

func TestConvertAmountInvalidAmount(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, nil, extraDocs)

	ctx := t.Context()
	claim := &document.AmountClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: testPropID},
		Amount:    document.Amount("not-a-number"),
		Precision: 1,
	}
	_, errE := c.convertAmount(ctx, claim)
	assert.Error(t, errE)
}

func TestConvertAmountPropagationError(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, nil, extraDocs)
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
	assert.Error(t, errE)
}

func TestConvertAmountIntervalPropagationError(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, nil, extraDocs)
	unknownParent := identifier.New()
	c.propertyAncestors = map[identifier.Identifier][]identifier.Identifier{
		testPropID: {unknownParent},
	}

	ctx := t.Context()
	fromAmount := document.Amount("10")
	toAmount := document.Amount("20")
	fromPrec := 1.0
	toPrec := 1.0
	claim := &document.AmountIntervalClaim{
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: testPropID},
		From:          &fromAmount,
		FromPrecision: &fromPrec,
		To:            &toAmount,
		ToPrecision:   &toPrec,
	}
	_, _, errE := c.convertAmountInterval(ctx, claim)
	assert.Error(t, errE)
}

func TestConvertAmountIntervalInvalidFromAmount(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	fromAmount := document.Amount("invalid")
	fromPrec := 1.0
	toAmount := document.Amount("20")
	toPrec := 1.0
	claim := &document.AmountIntervalClaim{
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: testPropID},
		From:          &fromAmount,
		FromPrecision: &fromPrec,
		To:            &toAmount,
		ToPrecision:   &toPrec,
	}
	_, _, errE := c.convertAmountInterval(ctx, claim)
	assert.Error(t, errE)
}

func TestConvertAmountIntervalInvalidToAmount(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, nil, extraDocs)

	ctx := t.Context()
	fromAmount := document.Amount("10")
	fromPrec := 1.0
	toAmount := document.Amount("invalid")
	toPrec := 1.0
	claim := &document.AmountIntervalClaim{
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: testPropID},
		From:          &fromAmount,
		FromPrecision: &fromPrec,
		To:            &toAmount,
		ToPrecision:   &toPrec,
	}
	_, _, errE := c.convertAmountInterval(ctx, claim)
	assert.Error(t, errE)
}

func TestConvertTimePropagationError(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, nil, extraDocs)
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
	assert.Error(t, errE)
}

func TestConvertTimeInvalidTimestamp(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, nil, extraDocs)

	ctx := t.Context()
	claim := &document.TimeClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: testPropID},
		Timestamp: document.Timestamp("not-a-time"),
		Precision: document.TimePrecisionDay,
	}
	_, errE := c.convertTime(ctx, claim)
	assert.Error(t, errE)
}

func TestConvertTimeIntervalPropagationError(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, nil, extraDocs)
	unknownParent := identifier.New()
	c.propertyAncestors = map[identifier.Identifier][]identifier.Identifier{
		testPropID: {unknownParent},
	}

	ctx := t.Context()
	fromTs := document.Timestamp("2024-01-01")
	toTs := document.Timestamp("2024-12-31")
	fromPrec := document.TimePrecisionDay
	toPrec := document.TimePrecisionDay
	claim := &document.TimeIntervalClaim{
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: testPropID},
		From:          &fromTs,
		FromPrecision: &fromPrec,
		To:            &toTs,
		ToPrecision:   &toPrec,
	}
	_, _, errE := c.convertTimeInterval(ctx, claim)
	assert.Error(t, errE)
}

func TestConvertTimeIntervalInvalidFromTimestamp(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	fromTs := document.Timestamp("not-a-time")
	fromPrec := document.TimePrecisionDay
	toTs := document.Timestamp("2024-12-31")
	toPrec := document.TimePrecisionDay
	claim := &document.TimeIntervalClaim{
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: testPropID},
		From:          &fromTs,
		FromPrecision: &fromPrec,
		To:            &toTs,
		ToPrecision:   &toPrec,
	}
	_, _, errE := c.convertTimeInterval(ctx, claim)
	assert.Error(t, errE)
}

func TestConvertTimeIntervalInvalidToTimestamp(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, nil, extraDocs)

	ctx := t.Context()
	fromTs := document.Timestamp("2024-01-01")
	fromPrec := document.TimePrecisionDay
	toTs := document.Timestamp("not-a-time")
	toPrec := document.TimePrecisionDay
	claim := &document.TimeIntervalClaim{
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: testPropID},
		From:          &fromTs,
		FromPrecision: &fromPrec,
		To:            &toTs,
		ToPrecision:   &toPrec,
	}
	_, _, errE := c.convertTimeInterval(ctx, claim)
	assert.Error(t, errE)
}

func TestConvertRelationMetaPropError(t *testing.T) {
	t.Parallel()

	// Meta relation has unknown prop ID.
	targetDoc := makeNamingDoc(testTargetDocID, "Target")
	extraDocs := map[identifier.Identifier]*document.D{
		testTargetDocID: targetDoc,
	}
	c := newTestConverter(t, nil, nil, nil, extraDocs)

	ctx := t.Context()
	meta := &document.ClaimTypes{ //nolint:exhaustruct
		Relation: []document.RelationClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: identifier.New()}, // Unknown prop.
				To:        document.Reference{ID: testTargetDocID},
			},
		},
	}
	claim := &document.RelationClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, meta),
		Prop:      document.Reference{ID: testPropID},
		To:        document.Reference{ID: testTargetDocID},
	}
	_, errE := c.convertRelation(ctx, claim)
	assert.Error(t, errE)
}

func TestConvertRelationMetaToError(t *testing.T) {
	t.Parallel()

	metaPropDoc := makeNamingDoc(testPropID2, "Meta Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID2: metaPropDoc,
	}
	c := newTestConverter(t, nil, nil, nil, extraDocs)

	ctx := t.Context()
	meta := &document.ClaimTypes{ //nolint:exhaustruct
		Relation: []document.RelationClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: testPropID2},
				To:        document.Reference{ID: identifier.New()}, // Unknown target.
			},
		},
	}
	claim := &document.RelationClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, meta),
		Prop:      document.Reference{ID: testPropID},
		To:        document.Reference{ID: testTargetDocID},
	}
	_, errE := c.convertRelation(ctx, claim)
	assert.Error(t, errE)
}

func TestConvertRelationToDisplayError(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Rel Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
		// testTargetDocID is NOT in extraDocs, so getDisplayStrings for it will fail.
	}
	c := newTestConverter(t, nil, nil, nil, extraDocs)

	ctx := t.Context()
	claim := &document.RelationClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: testPropID},
		To:        document.Reference{ID: testTargetDocID},
	}
	_, errE := c.convertRelation(ctx, claim)
	assert.Error(t, errE)
}

func TestConvertHasMetaPropError(t *testing.T) {
	t.Parallel()

	extraDocs := map[identifier.Identifier]*document.D{}
	c := newTestConverter(t, nil, nil, nil, extraDocs)

	ctx := t.Context()
	meta := &document.ClaimTypes{ //nolint:exhaustruct
		Relation: []document.RelationClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: identifier.New()}, // Unknown prop.
				To:        document.Reference{ID: identifier.New()},
			},
		},
	}
	claim := &document.HasClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, meta),
		Prop:      document.Reference{ID: testPropID},
	}
	_, errE := c.convertHas(ctx, claim)
	assert.Error(t, errE)
}

func TestConvertHasMetaToError(t *testing.T) {
	t.Parallel()

	metaPropDoc := makeNamingDoc(testPropID2, "Meta Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID2: metaPropDoc,
	}
	c := newTestConverter(t, nil, nil, nil, extraDocs)

	ctx := t.Context()
	meta := &document.ClaimTypes{ //nolint:exhaustruct
		Relation: []document.RelationClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: testPropID2},
				To:        document.Reference{ID: identifier.New()}, // Unknown target.
			},
		},
	}
	claim := &document.HasClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, meta),
		Prop:      document.Reference{ID: testPropID},
	}
	_, errE := c.convertHas(ctx, claim)
	assert.Error(t, errE)
}

func TestConvertHasPropagationError(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, nil, extraDocs)
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
	assert.Error(t, errE)
}

func TestConvertRelationPropagationPropError(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Rel Prop")
	targetDoc := makeNamingDoc(testTargetDocID, "Target")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID:      propDoc,
		testTargetDocID: targetDoc,
	}
	c := newTestConverter(t, nil, nil, nil, extraDocs)
	unknownParent := identifier.New()
	c.propertyAncestors = map[identifier.Identifier][]identifier.Identifier{
		testPropID: {unknownParent},
	}

	ctx := t.Context()
	claim := &document.RelationClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: testPropID},
		To:        document.Reference{ID: testTargetDocID},
	}
	_, errE := c.convertRelation(ctx, claim)
	assert.Error(t, errE)
}

func TestConvertReferencePropagationError(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, nil, extraDocs)
	unknownParent := identifier.New()
	c.propertyAncestors = map[identifier.Identifier][]identifier.Identifier{
		testPropID: {unknownParent},
	}

	ctx := t.Context()
	claim := &document.ReferenceClaim{
		CoreClaim: makeCoreClaim(document.HighConfidence, nil),
		Prop:      document.Reference{ID: testPropID},
		IRI:       "https://example.com",
	}
	_, errE := c.convertReference(ctx, claim)
	assert.Error(t, errE)
}

func TestConvertNonePropagationError(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, nil, extraDocs)
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
	assert.Error(t, errE)
}

func TestConvertUnknownPropagationError(t *testing.T) {
	t.Parallel()

	propDoc := makeNamingDoc(testPropID, "Prop")
	extraDocs := map[identifier.Identifier]*document.D{
		testPropID: propDoc,
	}
	c := newTestConverter(t, nil, nil, nil, extraDocs)
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
	assert.Error(t, errE)
}

func TestConvertAmountIntervalFromUnknownToError(t *testing.T) {
	t.Parallel()

	// FromIsUnknown with To: delegates to convertAmount, which needs prop display.
	// But prop is not found, so it errors.
	c := newTestConverter(t, nil, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	toAmount := document.Amount("20")
	toPrec := 1.0
	claim := &document.AmountIntervalClaim{
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: identifier.New()},
		FromIsUnknown: true,
		To:            &toAmount,
		ToPrecision:   &toPrec,
	}
	_, _, errE := c.convertAmountInterval(ctx, claim)
	assert.Error(t, errE)
}

func TestConvertAmountIntervalToUnknownFromError(t *testing.T) {
	t.Parallel()

	// ToIsUnknown with From: delegates to convertAmount with From, which errors.
	c := newTestConverter(t, nil, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	fromAmount := document.Amount("10")
	fromPrec := 1.0
	claim := &document.AmountIntervalClaim{
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: identifier.New()},
		From:          &fromAmount,
		FromPrecision: &fromPrec,
		ToIsUnknown:   true,
	}
	_, _, errE := c.convertAmountInterval(ctx, claim)
	assert.Error(t, errE)
}

func TestConvertTimeIntervalFromUnknownToError(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	toTs := document.Timestamp("2024-06-15")
	toPrec := document.TimePrecisionDay
	claim := &document.TimeIntervalClaim{
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: identifier.New()},
		FromIsUnknown: true,
		To:            &toTs,
		ToPrecision:   &toPrec,
	}
	_, _, errE := c.convertTimeInterval(ctx, claim)
	assert.Error(t, errE)
}

func TestConvertTimeIntervalToUnknownFromError(t *testing.T) {
	t.Parallel()

	c := newTestConverter(t, nil, nil, nil, map[identifier.Identifier]*document.D{})

	ctx := t.Context()
	fromTs := document.Timestamp("2024-06-15")
	fromPrec := document.TimePrecisionDay
	claim := &document.TimeIntervalClaim{
		CoreClaim:     makeCoreClaim(document.HighConfidence, nil),
		Prop:          document.Reference{ID: identifier.New()},
		From:          &fromTs,
		FromPrecision: &fromPrec,
		ToIsUnknown:   true,
	}
	_, _, errE := c.convertTimeInterval(ctx, claim)
	assert.Error(t, errE)
}

func TestDisplayNameTemplates(t *testing.T) {
	t.Parallel()

	c := &Converter{ //nolint:exhaustruct
		languageCodes: map[identifier.Identifier]string{},
	}

	// Document with a DISPLAY_LABEL_TEMPLATE claim.
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID},
		Claims: &document.ClaimTypes{ //nolint:exhaustruct
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: displayLabelTemplatePropID},
					String:    `{{bestString "SHORT_NAME" .}}`,
				},
			},
		},
	}
	result := c.displayLabelTemplates(doc)
	require.NotNil(t, result)
	assert.Equal(t, `{{bestString "SHORT_NAME" .}}`, result["und"])
}

func TestDisplayNameTemplatesWithLanguage(t *testing.T) {
	t.Parallel()

	c := &Converter{ //nolint:exhaustruct
		languageCodes: map[identifier.Identifier]string{
			testLangDocID: "en",
		},
	}

	meta := &document.ClaimTypes{ //nolint:exhaustruct
		Relation: []document.RelationClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: inLanguagePropID},
				To:        document.Reference{ID: testLangDocID},
			},
		},
	}
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID},
		Claims: &document.ClaimTypes{ //nolint:exhaustruct
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, meta),
					Prop:      document.Reference{ID: displayLabelTemplatePropID},
					String:    `{{bestString "NAME" .}}`,
				},
			},
		},
	}
	result := c.displayLabelTemplates(doc)
	require.NotNil(t, result)
	assert.Equal(t, `{{bestString "NAME" .}}`, result["en"])
	_, hasUnd := result["und"]
	assert.False(t, hasUnd)
}

func TestDisplayNameTemplatesEmpty(t *testing.T) {
	t.Parallel()

	c := &Converter{ //nolint:exhaustruct
		languageCodes: map[identifier.Identifier]string{},
	}

	// Document without DISPLAY_LABEL_TEMPLATE.
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID},
	}
	result := c.displayLabelTemplates(doc)
	assert.Nil(t, result)
}

func TestMakeDisplayStringsWithTemplate(t *testing.T) {
	t.Parallel()

	shortNamePropID := identifier.New()

	c := &Converter{ //nolint:exhaustruct
		namingProperties: map[identifier.Identifier]bool{
			namingPropID: true,
		},
		languageCodes: map[identifier.Identifier]string{},
		mnemonics: map[string]identifier.Identifier{
			"SHORT_NAME": shortNamePropID,
		},
	}

	// Document with a DISPLAY_LABEL_TEMPLATE and naming + short name claims.
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID},
		Claims: &document.ClaimTypes{ //nolint:exhaustruct
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: displayLabelTemplatePropID},
					String:    `{{bestString "SHORT_NAME" .}}`,
				},
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

	ctx := t.Context()
	display, errE := c.makeDisplayStrings(ctx, doc)
	require.NoError(t, errE, "% -+#.1v", errE)
	// Template renders the short name as display.
	assert.Equal(t, "FN", display.Display["und"])
	// All naming strings become Naming.
	assert.Equal(t, []string{"Full Name"}, display.Naming["und"])
}

func TestMakeDisplayStringsWithTemplateFallback(t *testing.T) {
	t.Parallel()

	c := &Converter{ //nolint:exhaustruct
		namingProperties: map[identifier.Identifier]bool{
			namingPropID: true,
		},
		languageCodes: map[identifier.Identifier]string{},
		mnemonics:     map[string]identifier.Identifier{},
	}

	// Template with invalid syntax should return an error.
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID},
		Claims: &document.ClaimTypes{ //nolint:exhaustruct
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: displayLabelTemplatePropID},
					String:    `{{invalid syntax`,
				},
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: namingPropID},
					String:    "Fallback Name",
				},
			},
		},
	}

	ctx := t.Context()
	_, errE := c.makeDisplayStrings(ctx, doc)
	assert.Error(t, errE)
	assert.Contains(t, errE.Error(), "function \"invalid\" not defined")
}

func TestMakeDisplayStringsTemplatePerLanguage(t *testing.T) {
	t.Parallel()

	enLangID := identifier.New()
	slLangID := identifier.New()
	shortNamePropID := identifier.New()

	c := &Converter{ //nolint:exhaustruct
		namingProperties: map[identifier.Identifier]bool{
			namingPropID: true,
		},
		languageCodes: map[identifier.Identifier]string{
			enLangID: "en",
			slLangID: "sl",
		},
		mnemonics: map[string]identifier.Identifier{
			"SHORT_NAME": shortNamePropID,
		},
	}

	enMeta := &document.ClaimTypes{ //nolint:exhaustruct
		Relation: []document.RelationClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: inLanguagePropID},
				To:        document.Reference{ID: enLangID},
			},
		},
	}
	slMeta := &document.ClaimTypes{ //nolint:exhaustruct
		Relation: []document.RelationClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: inLanguagePropID},
				To:        document.Reference{ID: slLangID},
			},
		},
	}

	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID},
		Claims: &document.ClaimTypes{ //nolint:exhaustruct
			String: []document.StringClaim{
				{
					// Template only for English.
					CoreClaim: makeCoreClaim(document.HighConfidence, enMeta),
					Prop:      document.Reference{ID: displayLabelTemplatePropID},
					String:    `{{bestString "SHORT_NAME" .}}`,
				},
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, enMeta),
					Prop:      document.Reference{ID: namingPropID},
					String:    "English Name",
				},
				{
					// SHORT_NAME is not a naming property, just used by the template.
					CoreClaim: makeCoreClaim(document.HighConfidence, enMeta),
					Prop:      document.Reference{ID: shortNamePropID},
					String:    "EN",
				},
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, slMeta),
					Prop:      document.Reference{ID: namingPropID},
					String:    "Slovensko Ime",
				},
				{
					CoreClaim: makeCoreClaim(document.MediumConfidence, slMeta),
					Prop:      document.Reference{ID: namingPropID},
					String:    "Alternativno Ime",
				},
			},
		},
	}

	ctx := t.Context()
	display, errE := c.makeDisplayStrings(ctx, doc)
	require.NoError(t, errE, "% -+#.1v", errE)

	// English uses template.
	assert.Equal(t, "EN", display.Display["en"])
	assert.Equal(t, []string{"English Name"}, display.Naming["en"])

	// Slovenian has no template, uses existing logic.
	assert.Equal(t, "Slovensko Ime", display.Display["sl"])
	assert.Equal(t, []string{"Alternativno Ime"}, display.Naming["sl"])
}

func TestMakeDisplayStringsTemplateRelationTraversal(t *testing.T) {
	t.Parallel()

	shortNamePropID := identifier.New()
	parentRelPropID := identifier.New()
	yearPropID := identifier.New()
	parentDocID := identifier.New()

	parentDoc := &document.D{
		CoreDocument: document.CoreDocument{ID: parentDocID},
		Claims: &document.ClaimTypes{ //nolint:exhaustruct
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

	extraDocs := map[identifier.Identifier]*document.D{
		parentDocID: parentDoc,
	}

	c := newTestConverter(t, nil, nil, nil, extraDocs)
	c.namingProperties = map[identifier.Identifier]bool{
		namingPropID: true,
	}
	c.languageCodes = map[identifier.Identifier]string{}
	c.mnemonics = map[string]identifier.Identifier{
		"SHORT_NAME": shortNamePropID,
		"PARENT_DOC": parentRelPropID,
		"YEAR":       yearPropID,
	}

	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID},
		Claims: &document.ClaimTypes{ //nolint:exhaustruct
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: displayLabelTemplatePropID},
					String:    `{{bestString "SHORT_NAME" .}} ({{bestRelationDoc "PARENT_DOC" . | bestAmountString "YEAR"}})`,
				},
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
			Relation: []document.RelationClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: parentRelPropID},
					To:        document.Reference{ID: parentDocID},
				},
			},
		},
	}

	ctx := t.Context()
	display, errE := c.makeDisplayStrings(ctx, doc)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, "US (1776)", display.Display["und"])
	assert.Equal(t, []string{"United States"}, display.Naming["und"])
}

func TestMakeDisplayStringsTemplateOnlyNoNaming(t *testing.T) {
	t.Parallel()

	shortNamePropID := identifier.New()

	c := &Converter{ //nolint:exhaustruct
		namingProperties: map[identifier.Identifier]bool{
			namingPropID: true,
		},
		languageCodes: map[identifier.Identifier]string{},
		mnemonics: map[string]identifier.Identifier{
			"SHORT_NAME": shortNamePropID,
		},
	}

	// Document with template but no naming strings.
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID},
		Claims: &document.ClaimTypes{ //nolint:exhaustruct
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: displayLabelTemplatePropID},
					String:    `{{bestString "SHORT_NAME" .}}`,
				},
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: shortNamePropID},
					String:    "ShortVal",
				},
			},
		},
	}

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

	c := &Converter{ //nolint:exhaustruct
		namingProperties: map[identifier.Identifier]bool{
			namingPropID: true,
		},
		languageCodes: map[identifier.Identifier]string{
			enLangID: "en",
		},
		mnemonics: map[string]identifier.Identifier{
			"NAME": namePropID,
		},
	}

	// Document with a NAME claim in "und" and a template for "en".
	// bestString should fall back to "und" when "en" is not found.
	enMeta := &document.ClaimTypes{ //nolint:exhaustruct
		Relation: []document.RelationClaim{
			{
				CoreClaim: makeCoreClaim(document.HighConfidence, nil),
				Prop:      document.Reference{ID: inLanguagePropID},
				To:        document.Reference{ID: enLangID},
			},
		},
	}

	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID},
		Claims: &document.ClaimTypes{ //nolint:exhaustruct
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, enMeta),
					Prop:      document.Reference{ID: displayLabelTemplatePropID},
					String:    `{{bestString "NAME" .}}`,
				},
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, enMeta),
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

	ctx := t.Context()
	display, errE := c.makeDisplayStrings(ctx, doc)
	require.NoError(t, errE, "% -+#.1v", errE)
	// Template for "en" should fall back to "und" NAME claim.
	assert.Equal(t, "Universal Name", display.Display["en"])
}

func TestTemplateBestIdentifier(t *testing.T) {
	t.Parallel()

	codeProp := identifier.New()

	c := &Converter{ //nolint:exhaustruct
		namingProperties: map[identifier.Identifier]bool{
			namingPropID: true,
		},
		languageCodes: map[identifier.Identifier]string{},
		mnemonics: map[string]identifier.Identifier{
			"CODE": codeProp,
		},
	}

	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID},
		Claims: &document.ClaimTypes{ //nolint:exhaustruct
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: displayLabelTemplatePropID},
					String:    `ID: {{bestIdentifier "CODE" .}}`,
				},
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

	ctx := t.Context()
	display, errE := c.makeDisplayStrings(ctx, doc)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, "ID: Q42", display.Display["und"])
}

func TestTemplateNilDoc(t *testing.T) {
	t.Parallel()

	parentRelPropID := identifier.New()
	yearPropID := identifier.New()

	// getDocument returns not found for any ID.
	c := newTestConverter(t, nil, nil, nil, map[identifier.Identifier]*document.D{})
	c.namingProperties = map[identifier.Identifier]bool{
		namingPropID: true,
	}
	c.languageCodes = map[identifier.Identifier]string{}
	c.mnemonics = map[string]identifier.Identifier{
		"PARENT_DOC": parentRelPropID,
		"YEAR":       yearPropID,
	}

	// Template that tries to follow a non-existent relation.
	// bestRelationDoc returns nil, bestAmountString handles nil doc gracefully.
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID},
		Claims: &document.ClaimTypes{ //nolint:exhaustruct
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: displayLabelTemplatePropID},
					String:    `Year: {{bestRelationDoc "PARENT_DOC" . | bestAmountString "YEAR"}}`,
				},
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: namingPropID},
					String:    "Test",
				},
			},
		},
	}

	ctx := t.Context()
	display, errE := c.makeDisplayStrings(ctx, doc)
	require.NoError(t, errE, "% -+#.1v", errE)
	// Template renders with empty string for nil doc, trailing space trimmed.
	assert.Equal(t, "Year:", display.Display["und"])
}

func TestTemplateBestTimeString(t *testing.T) {
	t.Parallel()

	datePropID := identifier.New()

	c := &Converter{ //nolint:exhaustruct
		namingProperties: map[identifier.Identifier]bool{
			namingPropID: true,
		},
		languageCodes: map[identifier.Identifier]string{},
		mnemonics: map[string]identifier.Identifier{
			"DATE": datePropID,
		},
	}

	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID},
		Claims: &document.ClaimTypes{ //nolint:exhaustruct
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: displayLabelTemplatePropID},
					String:    `Date: {{bestTimeString "DATE" .}}`,
				},
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

	ctx := t.Context()
	display, errE := c.makeDisplayStrings(ctx, doc)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, "Date: 2024-06-15", display.Display["und"])
}

func TestTemplateGetDocumentByMnemonic(t *testing.T) {
	t.Parallel()

	otherDocID := identifier.New()
	otherDoc := &document.D{
		CoreDocument: document.CoreDocument{ID: otherDocID},
		Claims: &document.ClaimTypes{ //nolint:exhaustruct
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: namingPropID},
					String:    "Other Document",
				},
			},
		},
	}

	extraDocs := map[identifier.Identifier]*document.D{
		otherDocID: otherDoc,
	}
	c := newTestConverter(t, nil, nil, nil, extraDocs)
	c.namingProperties = map[identifier.Identifier]bool{
		namingPropID: true,
	}
	c.languageCodes = map[identifier.Identifier]string{}
	c.mnemonics = map[string]identifier.Identifier{
		"OTHER":  otherDocID,
		"NAMING": namingPropID,
	}

	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: testDocID},
		Claims: &document.ClaimTypes{ //nolint:exhaustruct
			String: []document.StringClaim{
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: displayLabelTemplatePropID},
					String:    `{{getDocumentByMnemonic "OTHER" | bestString "NAMING"}}`,
				},
				{
					CoreClaim: makeCoreClaim(document.HighConfidence, nil),
					Prop:      document.Reference{ID: namingPropID},
					String:    "My Doc",
				},
			},
		},
	}

	ctx := t.Context()
	display, errE := c.makeDisplayStrings(ctx, doc)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, "Other Document", display.Display["und"])
}
