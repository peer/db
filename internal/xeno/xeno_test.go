package xeno_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/peerdb/peerdb/core"
	"gitlab.com/peerdb/peerdb/internal/xeno"
	"gitlab.com/peerdb/peerdb/transform"
)

// schemaDocuments generates the core documents and the test data schema on top of them, the way the
// populate command does, and returns them together with the mnemonics they define.
func schemaDocuments(t *testing.T) ([]any, map[string][]string) {
	t.Helper()

	ctx := t.Context()

	documents, errE := core.Properties()
	require.NoError(t, errE, "% -+#.1v", errE)

	xenoProperties, errE := xeno.Properties()
	require.NoError(t, errE, "% -+#.1v", errE)
	documents = append(documents, xenoProperties...)

	mnemonics, errE := transform.Mnemonics(ctx, documents)
	require.NoError(t, errE, "% -+#.1v", errE)

	coreClasses, errE := core.Classes(mnemonics)
	require.NoError(t, errE, "% -+#.1v", errE)
	documents = append(documents, coreClasses...)

	xenoClasses, errE := xeno.Classes(mnemonics)
	require.NoError(t, errE, "% -+#.1v", errE)
	documents = append(documents, xenoClasses...)

	vocabularies, errE := core.Vocabularies()
	require.NoError(t, errE, "% -+#.1v", errE)
	documents = append(documents, vocabularies...)

	mnemonics, errE = transform.Mnemonics(ctx, documents)
	require.NoError(t, errE, "% -+#.1v", errE)

	return documents, mnemonics
}

// TestSchemaTransforms verifies that the schema transforms into documents. Transforming validates
// every claim, so a field whose tags do not agree with its Go type fails here.
func TestSchemaTransforms(t *testing.T) {
	t.Parallel()

	documents, mnemonics := schemaDocuments(t)

	transformed, errE := transform.Documents(t.Context(), mnemonics, documents)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Len(t, transformed, len(documents))
}

// TestPropertiesAreNamed verifies that every property is named in both languages and that no
// mnemonic is defined twice. A missing name leaves a claim with nothing to render as its label.
func TestPropertiesAreNamed(t *testing.T) {
	t.Parallel()

	properties, errE := xeno.Properties()
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.NotEmpty(t, properties)

	seen := map[string]bool{}
	for _, doc := range properties {
		property, ok := doc.(*core.Property)
		require.True(t, ok, "%T is not a property", doc)

		assert.NotEmpty(t, property.Mnemonic)
		assert.False(t, seen[property.Mnemonic], "duplicate mnemonic %q", property.Mnemonic)
		seen[property.Mnemonic] = true

		languages := map[string]bool{}
		for _, name := range property.Name {
			assert.NotEmpty(t, name.Value, "%s: empty name", property.Mnemonic)
			for _, language := range name.InLanguage {
				languages[language.ID[len(language.ID)-1]] = true
			}
		}
		assert.True(t, languages["en-GB"], "%s: no English name", property.Mnemonic)
		assert.True(t, languages["sl-SI"], "%s: no Slovenian name", property.Mnemonic)
	}
}

// TestClassesHaveFields verifies that every class which can be instantiated has a field schema, and
// that the abstract ones (which only group their subclasses) have none.
func TestClassesHaveFields(t *testing.T) {
	t.Parallel()

	_, mnemonics := schemaDocuments(t)

	classes, errE := xeno.Classes(mnemonics)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.NotEmpty(t, classes)

	for _, doc := range classes {
		class, ok := doc.(*core.Class)
		require.True(t, ok, "%T is not a class", doc)

		if class.AbstractClass {
			assert.Nil(t, class.Fields, "%s: abstract class has a field schema", class.Mnemonic)
			continue
		}
		require.NotNil(t, class.Fields, "%s: no field schema", class.Mnemonic)
		assert.NotEmpty(t, append(class.Fields.Field, fieldsOfSections(class.Fields.Section)...), "%s: empty field schema", class.Mnemonic)
	}
}

// fieldsOfSections flattens the fields of the sections of a class, so a class whose fields are all
// inside sections does not look empty.
func fieldsOfSections(sections []core.Section) []core.Field {
	fields := []core.Field{}
	for _, section := range sections {
		fields = append(fields, section.Field...)
	}
	return fields
}

// TestClassesWithoutMnemonics verifies that the classes are still complete without mnemonics, only
// missing their field schema. The populate command generates them twice, and the first pass has no
// mnemonics yet.
func TestClassesWithoutMnemonics(t *testing.T) {
	t.Parallel()

	classes, errE := xeno.Classes(nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.NotEmpty(t, classes)

	for _, doc := range classes {
		class, ok := doc.(*core.Class)
		require.True(t, ok, "%T is not a class", doc)
		assert.NotEmpty(t, class.Mnemonic)
		assert.NotEmpty(t, class.Name)
		assert.Nil(t, class.Fields, "%s: field schema generated without mnemonics", class.Mnemonic)
	}
}
