package export_test

import (
	"reflect"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/core"
	"gitlab.com/peerdb/peerdb/internal/export"
)

// Fixtures for walkSubFields. Each top-level "Field" struct represents a
// property-tagged outer field whose type the walker will descend into.

// walkSimpleStruct exercises a few mixed sub-fields.
type walkSimpleStruct struct {
	Name   string   `cardinality:"1.."  property:"NAME"`
	Code   string   `cardinality:"0..1" property:"CODE"`
	Tags   []string `cardinality:"0.."  property:"TAG"`
	Hidden bool     `                   property:"-"`
	Active bool     `cardinality:"0..1"`
}

// walkWithDocAndValue verifies that documentid and value:"" fields are
// skipped (documentid emits a PK row; value:"" is represented by the parent's
// type and so isn't re-emitted as a sub-field row here).
type walkWithDocAndValue struct {
	ID    []string `                   documentid:""`
	Value string   `                                                  value:""`
	Title string   `cardinality:"0..1"               property:"TITLE"`
}

// walkRefTarget exercises an outer Ref sub-field carrying a values tag - it
// should emit an edge to the resolved target.
type walkRefTarget struct {
	Lang []core.Ref `cardinality:"0.." property:"IN_LANGUAGE" values:"core.peerdb.org,INSTANCE_OF=core.peerdb.org,LANGUAGE"`
}

// walkInnerEmbed embeds a struct anonymously so its property fields surface
// at the outer level.
type walkInnerCommon struct {
	Mnemonic string `cardinality:"0..1" property:"MNEMONIC"`
}

type walkInnerEmbed struct {
	walkInnerCommon

	Note string `cardinality:"0..1" property:"NOTE"`
}

// walkRecursive demonstrates the self-referential type guard. The walker
// must not loop forever even though SubField points to itself.
type walkRecursive struct {
	Label    string          `cardinality:"0..1" property:"LABEL"`
	SubField []walkRecursive `cardinality:"0.."  property:"SUB_FIELD"`
}

// walkNested has a property-tagged sub-field whose type is another struct
// with its own property fields - we expect PARENT[CHILD][LEAF] naming.
type walkNestedLeaf struct {
	Code string `cardinality:"0..1" property:"CODE"`
}

type walkNestedMid struct {
	Leaf walkNestedLeaf `cardinality:"0..1" property:"LEAF"`
}

type walkNested struct {
	Mid walkNestedMid `cardinality:"0..1" property:"MID"`
}

// TestWalkSubFields_Simple walks a flat struct and verifies one row per
// property-tagged field (skipping property:"-").
func TestWalkSubFields_Simple(t *testing.T) {
	t.Parallel()
	rows, relations := export.TestingWalkSubFields("ENT", "PARENT", reflect.TypeFor[walkSimpleStruct](), nil, zerolog.Nop())

	assert.Equal(t, []string{
		`string PARENT[NAME] "1..*"`,
		`string PARENT[CODE] "0..1"`,
		`string PARENT[TAG] "0..*"`,
	}, rows, "rows")
	assert.Empty(t, relations, "no Ref fields, no edges")
}

// TestWalkSubFields_SkipsDocumentIDAndValue verifies the walker doesn't emit
// rows for documentid and value:"" fields (those are handled at the parent).
func TestWalkSubFields_SkipsDocumentIDAndValue(t *testing.T) {
	t.Parallel()
	rows, relations := export.TestingWalkSubFields("ENT", "PARENT", reflect.TypeFor[walkWithDocAndValue](), nil, zerolog.Nop())

	assert.Equal(t, []string{
		`string PARENT[TITLE] "0..1"`,
	}, rows)
	assert.Empty(t, relations)
}

// TestWalkSubFields_EmitsRefEdge verifies that a Ref sub-field with a values
// tag produces both a row and an edge labelled with the compound name.
func TestWalkSubFields_EmitsRefEdge(t *testing.T) {
	t.Parallel()
	idToName := map[identifier.Identifier]string{
		identifier.From(core.Namespace, "LANGUAGE"): "LANGUAGE",
	}
	rows, relations := export.TestingWalkSubFields("CLASS", "NAME", reflect.TypeFor[walkRefTarget](), idToName, zerolog.Nop())

	assert.Equal(t, []string{
		`reference NAME[IN_LANGUAGE] FK "0..*"`,
	}, rows)
	assert.Equal(t, []string{
		`CLASS }o--o{ LANGUAGE : "NAME[IN_LANGUAGE]"`,
	}, relations)
}

// TestWalkSubFields_AnonymousEmbed verifies that fields from anonymously
// embedded structs are emitted as if they were declared at the outer level.
func TestWalkSubFields_AnonymousEmbed(t *testing.T) {
	t.Parallel()
	rows, _ := export.TestingWalkSubFields("ENT", "PARENT", reflect.TypeFor[walkInnerEmbed](), nil, zerolog.Nop())

	assert.Equal(t, []string{
		`string PARENT[MNEMONIC] "0..1"`,
		`string PARENT[NOTE] "0..1"`,
	}, rows)
}

// TestWalkSubFields_RecursionGuard verifies the visited-set prevents infinite
// recursion on self-referential types. The walker emits one row per nested
// level it visits, then stops.
func TestWalkSubFields_RecursionGuard(t *testing.T) {
	t.Parallel()
	// This test simply must terminate. We don't assert on row count beyond
	// "non-empty" because the exact recursion depth depends on the visited
	// set semantics, but it must not loop forever.
	rows, _ := export.TestingWalkSubFields("ENT", "PARENT", reflect.TypeFor[walkRecursive](), nil, zerolog.Nop())
	assert.NotEmpty(t, rows)
	// Sanity check: we should have at least the top-level LABEL row.
	assert.Contains(t, rows, `string PARENT[LABEL] "0..1"`)
}

// TestWalkSubFields_NestedCompoundName verifies that compound names accumulate
// across deeply nested struct sub-fields.
func TestWalkSubFields_NestedCompoundName(t *testing.T) {
	t.Parallel()
	rows, _ := export.TestingWalkSubFields("ENT", "PARENT", reflect.TypeFor[walkNested](), nil, zerolog.Nop())

	// Sub-field rows must include the full PARENT[MID][LEAF][CODE] path.
	assert.Contains(t, rows, `string PARENT[MID][LEAF][CODE] "0..1"`)
}
