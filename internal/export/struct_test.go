package export_test

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/core"
	"gitlab.com/peerdb/peerdb/document"
	internalCore "gitlab.com/peerdb/peerdb/internal/core"
	"gitlab.com/peerdb/peerdb/internal/export"
	"gitlab.com/peerdb/peerdb/transform"
)

// Ensure core init() runs to register types.
var _ = core.Namespace

func makeTestMnemonics() map[string]identifier.Identifier {
	return map[string]identifier.Identifier{
		"NAME":                     internalCore.NamePropID,
		"MNEMONIC":                 internalCore.MnemonicPropID,
		"DESCRIPTION":              internalCore.DescriptionPropID,
		"SHORT_NAME":               internalCore.ShortNamePropID,
		"ALTERNATIVE_NAME":         internalCore.AlternativeNamePropID,
		"IN_LANGUAGE":              internalCore.InLanguagePropID,
		"INSTANCE_OF":              internalCore.InstanceOfPropID,
		"SUBCLASS_OF":              internalCore.SubclassOfPropID,
		"ABSTRACT_CLASS":           internalCore.AbstractClassPropID,
		"DISPLAY_LABEL_TEMPLATE":   internalCore.DisplayLabelTemplatePropID,
		"SEARCH_SHORTCUT":          internalCore.SearchShortcutPropID,
		"FIELDS":                   internalCore.FieldsPropID,
		"SUBPROPERTY_OF":           internalCore.SubpropertyOfPropID,
		"INVERSE_PROPERTY_OF":      internalCore.InversePropertyOfPropID,
		"INSTRUCTION":              internalCore.InstructionPropID,
		"IDENTIFIER_LINK_TEMPLATE": internalCore.IdentifierLinkTemplatePropID,
		"CODE":                     internalCore.CodePropID,
	}
}

// makePropertyDoc creates a minimal Property document with NAME and MNEMONIC claims.
func makePropertyDoc(base []string, name string, mnemonic string) *document.D {
	docID := identifier.From(base...)
	propClassID := identifier.From(core.Namespace, "PROPERTY")
	classBase := []string{core.Namespace, "PROPERTY"}

	doc := &document.D{
		CoreDocument: document.CoreDocument{
			ID:   docID,
			Base: base,
		},
		Claims: &document.ClaimTypes{},
	}

	// Add INSTANCE_OF claim.
	_ = doc.Add(&document.ReferenceClaim{
		CoreClaim: document.CoreClaim{
			ID:         identifier.New(),
			Confidence: document.HighConfidence,
			Sub:        nil,
		},
		Prop: document.Reference{ID: internalCore.InstanceOfPropID},
		To:   document.Reference{ID: propClassID},
	})

	// Add NAME claim.
	_ = doc.Add(&document.StringClaim{
		CoreClaim: document.CoreClaim{
			ID:         identifier.New(),
			Confidence: document.HighConfidence,
			Sub:        nil,
		},
		Prop:   document.Reference{ID: internalCore.NamePropID},
		String: name,
	})

	// Add MNEMONIC claim.
	if mnemonic != "" {
		_ = doc.Add(&document.StringClaim{
			CoreClaim: document.CoreClaim{
				ID:         identifier.New(),
				Confidence: document.HighConfidence,
				Sub:        nil,
			},
			Prop:   document.Reference{ID: internalCore.MnemonicPropID},
			String: mnemonic,
		})
	}

	// Store class base for reference resolution.
	_ = classBase

	return doc
}

// makeClassDoc creates the PROPERTY class document (so INSTANCE_OF references can be resolved).
func makeClassDoc() *document.D {
	classBase := []string{core.Namespace, "PROPERTY"}
	classID := identifier.From(classBase...)
	return &document.D{
		CoreDocument: document.CoreDocument{
			ID:   classID,
			Base: classBase,
		},
		Claims: &document.ClaimTypes{},
	}
}

func TestStruct_SimpleProperty(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	propBase := []string{core.Namespace, "TEST_PROP"}
	doc := makePropertyDoc(propBase, "test property", "TEST_PROP")
	classDoc := makeClassDoc()
	docID := doc.ID

	docs := map[identifier.Identifier]*document.D{
		docID:       doc,
		classDoc.ID: classDoc,
	}
	getDoc := func(_ context.Context, id identifier.Identifier) (*document.D, errors.E) {
		return docs[id], nil
	}

	var buf bytes.Buffer
	errE := export.Struct(ctx, &buf, []identifier.Identifier{docID}, makeTestMnemonics(), getDoc)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Parse output JSON.
	var result map[string]any
	err := json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)

	// Check that the result has expected structure.
	// PropertyFields has Name ([]StringWithLanguage) and Mnemonic (string).
	assert.NotNil(t, result["name"])
	assert.Equal(t, "TEST_PROP", result["mnemonic"])

	// Check ID is set from base.
	idVal, ok := result["id"]
	require.True(t, ok, "expected id field in output")
	idSlice, ok := idVal.([]any)
	require.True(t, ok, "expected id to be an array")
	assert.Equal(t, core.Namespace, idSlice[0])
	assert.Equal(t, "TEST_PROP", idSlice[1])

	// Check instanceOf is set.
	instanceOf, ok := result["instanceOf"]
	require.True(t, ok, "expected instanceOf field")
	instanceOfSlice, ok := instanceOf.([]any)
	require.True(t, ok, "expected instanceOf to be array")
	require.Len(t, instanceOfSlice, 1)
}

func TestStruct_UnknownClass(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Create a document with an unregistered INSTANCE_OF class.
	unknownClassID := identifier.New()
	docBase := []string{"unknown", "doc1"}
	docID := identifier.From(docBase...)

	doc := &document.D{
		CoreDocument: document.CoreDocument{
			ID:   docID,
			Base: docBase,
		},
		Claims: &document.ClaimTypes{},
	}
	_ = doc.Add(&document.ReferenceClaim{
		CoreClaim: document.CoreClaim{
			ID:         identifier.New(),
			Confidence: document.HighConfidence,
			Sub:        nil,
		},
		Prop: document.Reference{ID: internalCore.InstanceOfPropID},
		To:   document.Reference{ID: unknownClassID},
	})

	getDoc := func(_ context.Context, id identifier.Identifier) (*document.D, errors.E) {
		if id == docID {
			return doc, nil
		}
		return nil, nil //nolint:nilnil
	}

	var buf bytes.Buffer
	errE := export.Struct(ctx, &buf, []identifier.Identifier{docID}, makeTestMnemonics(), getDoc)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Document should be skipped, so output should be empty.
	assert.Empty(t, buf.String())
}

func TestStruct_ExtraClaim(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	propBase := []string{core.Namespace, "TEST_EXTRA"}
	doc := makePropertyDoc(propBase, "test extra", "TEST_EXTRA")
	classDoc := makeClassDoc()
	docID := doc.ID

	// Add an extra claim with a property that is NOT on the Property struct.
	extraPropID := identifier.New()
	_ = doc.Add(&document.StringClaim{
		CoreClaim: document.CoreClaim{
			ID:         identifier.New(),
			Confidence: document.HighConfidence,
			Sub:        nil,
		},
		Prop:   document.Reference{ID: extraPropID},
		String: "extra value",
	})

	docs := map[identifier.Identifier]*document.D{
		docID:       doc,
		classDoc.ID: classDoc,
	}
	getDoc := func(_ context.Context, id identifier.Identifier) (*document.D, errors.E) {
		return docs[id], nil
	}

	var buf bytes.Buffer
	errE := export.Struct(ctx, &buf, []identifier.Identifier{docID}, makeTestMnemonics(), getDoc)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Struct should still be output despite extra claim.
	assert.NotEmpty(t, buf.String())

	var result map[string]any
	err := json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)
	assert.Equal(t, "TEST_EXTRA", result["mnemonic"])
}

func TestStruct_ExcessCardinality(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	propBase := []string{core.Namespace, "TEST_EXCESS"}
	doc := makePropertyDoc(propBase, "test excess", "TEST_EXCESS")
	classDoc := makeClassDoc()
	docID := doc.ID

	// Add a second MNEMONIC claim (Mnemonic is a string field, so only first should be used).
	_ = doc.Add(&document.StringClaim{
		CoreClaim: document.CoreClaim{
			ID:         identifier.New(),
			Confidence: document.HighConfidence,
			Sub:        nil,
		},
		Prop:   document.Reference{ID: internalCore.MnemonicPropID},
		String: "SECOND_MNEMONIC",
	})

	docs := map[identifier.Identifier]*document.D{
		docID:       doc,
		classDoc.ID: classDoc,
	}
	getDoc := func(_ context.Context, id identifier.Identifier) (*document.D, errors.E) {
		return docs[id], nil
	}

	var buf bytes.Buffer
	errE := export.Struct(ctx, &buf, []identifier.Identifier{docID}, makeTestMnemonics(), getDoc)
	require.NoError(t, errE, "% -+#.1v", errE)

	var result map[string]any
	err := json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)

	// Should use the first mnemonic (highest confidence, both are HighConfidence so insertion order).
	mnemonic, ok := result["mnemonic"].(string)
	require.True(t, ok)
	assert.True(t, mnemonic == "TEST_EXCESS" || mnemonic == "SECOND_MNEMONIC", "expected one of the mnemonic values, got: %s", mnemonic)
}

func TestStruct_TypeMismatch(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	propBase := []string{core.Namespace, "TEST_MISMATCH"}
	docID := identifier.From(propBase...)
	propClassID := identifier.From(core.Namespace, "PROPERTY")
	classDoc := makeClassDoc()

	doc := &document.D{
		CoreDocument: document.CoreDocument{
			ID:   docID,
			Base: propBase,
		},
		Claims: &document.ClaimTypes{},
	}

	// Add INSTANCE_OF.
	_ = doc.Add(&document.ReferenceClaim{
		CoreClaim: document.CoreClaim{
			ID:         identifier.New(),
			Confidence: document.HighConfidence,
			Sub:        nil,
		},
		Prop: document.Reference{ID: internalCore.InstanceOfPropID},
		To:   document.Reference{ID: propClassID},
	})

	// Add a MNEMONIC claim as an AmountClaim (type mismatch: field expects string).
	_ = doc.Add(&document.AmountClaim{
		CoreClaim: document.CoreClaim{
			ID:         identifier.New(),
			Confidence: document.HighConfidence,
			Sub:        nil,
		},
		Prop:      document.Reference{ID: internalCore.MnemonicPropID},
		Amount:    "42",
		Precision: 1,
	})

	// Add NAME so the struct has at least something.
	_ = doc.Add(&document.StringClaim{
		CoreClaim: document.CoreClaim{
			ID:         identifier.New(),
			Confidence: document.HighConfidence,
			Sub:        nil,
		},
		Prop:   document.Reference{ID: internalCore.NamePropID},
		String: "test",
	})

	docs := map[identifier.Identifier]*document.D{
		docID:       doc,
		classDoc.ID: classDoc,
	}
	getDoc := func(_ context.Context, id identifier.Identifier) (*document.D, errors.E) {
		return docs[id], nil
	}

	var buf bytes.Buffer
	errE := export.Struct(ctx, &buf, []identifier.Identifier{docID}, makeTestMnemonics(), getDoc)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Struct should still be output (mismatch is info-level, not an error).
	var result map[string]any
	err := json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)

	// Mnemonic field should be the amount as a string (AmountClaim on a string field).
	mnemonic, ok := result["mnemonic"].(string)
	require.True(t, ok)
	assert.Equal(t, "42", mnemonic)
}

func TestStruct_NilDoc(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	docID := identifier.New()

	getDoc := func(_ context.Context, _ identifier.Identifier) (*document.D, errors.E) {
		return nil, nil //nolint:nilnil
	}

	var buf bytes.Buffer
	errE := export.Struct(ctx, &buf, []identifier.Identifier{docID}, makeTestMnemonics(), getDoc)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Nil doc should be skipped, empty output.
	assert.Empty(t, buf.String())
}

func TestStruct_SubClaims(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	propBase := []string{core.Namespace, "TEST_SUBCLAIM"}
	docID := identifier.From(propBase...)
	propClassID := identifier.From(core.Namespace, "PROPERTY")
	classDoc := makeClassDoc()

	doc := &document.D{
		CoreDocument: document.CoreDocument{
			ID:   docID,
			Base: propBase,
		},
		Claims: &document.ClaimTypes{},
	}

	// Add INSTANCE_OF.
	_ = doc.Add(&document.ReferenceClaim{
		CoreClaim: document.CoreClaim{
			ID:         identifier.New(),
			Confidence: document.HighConfidence,
			Sub:        nil,
		},
		Prop: document.Reference{ID: internalCore.InstanceOfPropID},
		To:   document.Reference{ID: propClassID},
	})

	// Create a language ref for the sub-claim.
	langBase := []string{core.Namespace, "LANGUAGE", "en-GB"}
	langID := identifier.From(langBase...)

	// Add NAME claim with IN_LANGUAGE sub-claim (StringWithLanguage).
	nameClaim := &document.StringClaim{
		CoreClaim: document.CoreClaim{
			ID:         identifier.New(),
			Confidence: document.HighConfidence,
			Sub:        nil,
		},
		Prop:   document.Reference{ID: internalCore.NamePropID},
		String: "test name",
	}
	// Add IN_LANGUAGE sub-claim.
	_ = nameClaim.Add(&document.ReferenceClaim{
		CoreClaim: document.CoreClaim{
			ID:         identifier.New(),
			Confidence: document.HighConfidence,
			Sub:        nil,
		},
		Prop: document.Reference{ID: internalCore.InLanguagePropID},
		To:   document.Reference{ID: langID},
	})
	_ = doc.Add(nameClaim)

	// Add MNEMONIC.
	_ = doc.Add(&document.StringClaim{
		CoreClaim: document.CoreClaim{
			ID:         identifier.New(),
			Confidence: document.HighConfidence,
			Sub:        nil,
		},
		Prop:   document.Reference{ID: internalCore.MnemonicPropID},
		String: "TEST_SUBCLAIM",
	})

	// Language doc for base lookup.
	langDoc := &document.D{
		CoreDocument: document.CoreDocument{
			ID:   langID,
			Base: langBase,
		},
		Claims: &document.ClaimTypes{},
	}

	docs := map[identifier.Identifier]*document.D{
		docID:       doc,
		classDoc.ID: classDoc,
		langID:      langDoc,
	}
	getDoc := func(_ context.Context, id identifier.Identifier) (*document.D, errors.E) {
		return docs[id], nil
	}

	var buf bytes.Buffer
	errE := export.Struct(ctx, &buf, []identifier.Identifier{docID}, makeTestMnemonics(), getDoc)
	require.NoError(t, errE, "% -+#.1v", errE)

	var result map[string]any
	err := json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)

	// Name should be an array of objects with value and inLanguage.
	nameVal, ok := result["name"]
	require.True(t, ok, "expected name field")
	nameArr, ok := nameVal.([]any)
	require.True(t, ok, "expected name to be array")
	require.Len(t, nameArr, 1)

	nameObj, ok := nameArr[0].(map[string]any)
	require.True(t, ok, "expected name element to be object")
	assert.Equal(t, "test name", nameObj["value"])

	// InLanguage should have the language Ref.
	inLang, ok := nameObj["inLanguage"]
	require.True(t, ok, "expected inLanguage field in name")
	inLangArr, ok := inLang.([]any)
	require.True(t, ok, "expected inLanguage to be array")
	require.Len(t, inLangArr, 1)

	// The Ref should have the language base.
	langRef, ok := inLangArr[0].(map[string]any)
	require.True(t, ok, "expected inLanguage element to be object")
	langRefID, ok := langRef["id"].([]any)
	require.True(t, ok, "expected id field in language ref")
	assert.Equal(t, core.Namespace, langRefID[0])
	assert.Equal(t, "LANGUAGE", langRefID[1])
	assert.Equal(t, "en-GB", langRefID[2])
}

// TestStruct_ClassRegistryFromTest verifies registry lookups work from test context.
func TestStruct_ClassRegistryFromTest(t *testing.T) {
	t.Parallel()

	propClassID := identifier.From(core.Namespace, "PROPERTY")
	typ, ok := transform.ClassRegistry[propClassID]
	require.True(t, ok, "PROPERTY class should be registered")
	assert.Equal(t, reflect.TypeFor[core.Property](), typ)
}
