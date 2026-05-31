package export_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/peerdb/peerdb/internal/export"
)

// TestDiagram verifies the diagram generator includes registered entities,
// the canonical reference relationships between them, and a PK row per entity.
func TestDiagram(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	errE := export.Diagram(zerolog.Nop(), &buf, false)
	require.NoError(t, errE, "% -+#.1v", errE)

	out := buf.String()

	assert.True(t, strings.HasPrefix(out, "---\nconfig:\n  layout: elk\n---\nerDiagram\n"), "should begin with Mermaid header")

	// Every described core class should appear as an entity block under its mnemonic.
	for _, entity := range []string{"CLASS", "PROPERTY", "DOCUMENT", "VOCABULARY", "LANGUAGE", "UNIT", "VALUE_TYPE"} {
		assert.Contains(t, out, "\n  \""+entity+"\" {\n", "missing entity block for %s", entity)
	}
	// The PK row should live on DOCUMENT (which owns the documentid field).
	assert.Contains(t, out, "\n  \"DOCUMENT\" {\n    string ID PK \"\"\n", "DOCUMENT should own the PK row")

	// Reference fields with values tags should produce solid edges using mnemonic names.
	// INSTANCE_OF lives on DOCUMENT (since DocumentFields owns it), so only DOCUMENT
	// emits that edge.
	expectedRelations := []string{
		`"CLASS" }o--o{ "CLASS" : "SUBCLASS_OF"`,
		`"PROPERTY" }o--o{ "PROPERTY" : "SUBPROPERTY_OF"`,
		`"PROPERTY" }o--o| "PROPERTY" : "INVERSE_PROPERTY_OF"`,
		`"DOCUMENT" }o--o{ "CLASS" : "INSTANCE_OF"`,
	}
	for _, rel := range expectedRelations {
		assert.Contains(t, out, rel, "missing relation %q", rel)
	}

	// Class hierarchy edges should be dashed and link concrete vocabularies to the abstract parent.
	for _, rel := range []string{
		`"LANGUAGE" }o..|| "VOCABULARY" : "IS_SUBCLASS"`,
		`"UNIT" }o..|| "VOCABULARY" : "IS_SUBCLASS"`,
		`"VALUE_TYPE" }o..|| "VOCABULARY" : "IS_SUBCLASS"`,
	} {
		assert.Contains(t, out, rel, "missing IS_SUBCLASS edge %q", rel)
	}

	// Sub-claim relationships discovered through nested values tags should also appear.
	// DESCRIPTION is owned by CLASS (via ClassFields), so the edge originates from CLASS.
	assert.Contains(t, out, `"CLASS" }o--o{ "LANGUAGE" : "DESCRIPTION[IN_LANGUAGE]"`)

	// Vocabulary leaves inherit everything, so their entity blocks must be empty.
	for _, leaf := range []string{"LANGUAGE", "UNIT", "VALUE_TYPE"} {
		assert.Contains(t, out, "\n  \""+leaf+"\" {\n  }\n", "%s should have no own fields", leaf)
	}

	// Property rows from the class registry should carry their mnemonics and value types.
	for _, row := range []string{
		`string NAME "1..*"`,
		`string MNEMONIC "0..1"`,
		`html DESCRIPTION "0..*"`,
		`reference SUBCLASS_OF FK "0..*"`,
		`has ABSTRACT_CLASS "0..1"`,
		`reference INSTANCE_OF FK "0..*"`,
	} {
		assert.Contains(t, out, row, "missing row %q", row)
	}

	// Sub-fields should be emitted with PARENT[SUB] compound names.
	assert.Contains(t, out, `reference DESCRIPTION[IN_LANGUAGE] FK "0..*"`, "missing sub-field row")
}

// TestDiagram_SkipCore verifies that core entities, edges to them, and
// INSTANCE_OF rows are excluded when skipCore is true.
func TestDiagram_SkipCore(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	errE := export.Diagram(zerolog.Nop(), &buf, true)
	require.NoError(t, errE, "% -+#.1v", errE)

	out := buf.String()

	// Core entities must not appear as entity blocks.
	for _, entity := range []string{"CLASS", "PROPERTY", "DOCUMENT", "VOCABULARY", "LANGUAGE", "UNIT", "VALUE_TYPE"} {
		assert.NotContains(t, out, "\n  \""+entity+"\" {\n", "core entity %s should be excluded", entity)
	}

	// INSTANCE_OF rows must not be emitted in any remaining entity.
	assert.NotContains(t, out, "INSTANCE_OF", "INSTANCE_OF references should be excluded")

	// No edges should reference the skipped core entities.
	for _, target := range []string{`"CLASS"`, `"PROPERTY"`, `"DOCUMENT"`, `"VOCABULARY"`, `"LANGUAGE"`, `"UNIT"`, `"VALUE_TYPE"`} {
		for _, sep := range []string{"--", ".."} {
			for _, right := range []string{"o|", "o{", "||", "|{"} {
				assert.NotContains(t, out, sep+right+" "+target+" :", "edge to skipped %s should not appear", target)
			}
		}
	}

	// Header should still be emitted.
	assert.True(t, strings.HasPrefix(out, "---\nconfig:\n  layout: elk\n---\nerDiagram\n"))
}
