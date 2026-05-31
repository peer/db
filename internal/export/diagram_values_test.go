package export_test

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/core"
	"gitlab.com/peerdb/peerdb/internal/export"
)

// Fixtures for the values/value tag resolution: each "Field" is the property
// field that the diagram walker would visit on a parent struct.

// refOuter exercises a Ref-typed property field carrying a values tag.
type refOuter struct {
	Field core.Ref `property:"X" values:"core.peerdb.org,INSTANCE_OF=core.peerdb.org,LANGUAGE"`
}

// sliceRefOuter exercises a []Ref property field.
type sliceRefOuter struct {
	Field []core.Ref `property:"X" values:"core.peerdb.org,INSTANCE_OF=core.peerdb.org,LANGUAGE"`
}

// ptrRefOuter exercises a *Ref property field.
type ptrRefOuter struct {
	Field *core.Ref `property:"X" values:"core.peerdb.org,INSTANCE_OF=core.peerdb.org,LANGUAGE"`
}

// refOuterNoTag exercises a Ref property field without a values tag.
type refOuterNoTag struct {
	Field core.Ref `property:"X"`
}

// wrapper is the canonical razume-style wrapper: a value:""-tagged Ref carrying
// the values tag, alongside a property-tagged sub-field.
type wrapper struct {
	Value      core.Ref   `                                         value:"" values:"core.peerdb.org,INSTANCE_OF=core.peerdb.org,UNIT"`
	InLanguage []core.Ref `cardinality:"0.." property:"IN_LANGUAGE"          values:"core.peerdb.org,INSTANCE_OF=core.peerdb.org,LANGUAGE"`
	Extras     []core.Ref `cardinality:"0.." property:"EXTRA"`
}

// wrapperOuter has a struct-typed property field whose values tag is on the
// inner value:"" Ref.
type wrapperOuter struct {
	Field wrapper `property:"X"`
}

// sliceWrapperOuter is the same dispatch but for a slice of wrappers.
type sliceWrapperOuter struct {
	Field []wrapper `property:"X"`
}

// ptrWrapperOuter is the same dispatch but for a pointer to a wrapper.
type ptrWrapperOuter struct {
	Field *wrapper `property:"X"`
}

// wrapperNoValues has a value:""-tagged Ref without a values tag.
type wrapperNoValues struct {
	Value core.Ref `value:""`
}

// wrapperNoValuesOuter exercises the empty-tag case for wrapper structs.
type wrapperNoValuesOuter struct {
	Field wrapperNoValues `property:"X"`
}

// innerEmbed embeds wrapper anonymously, so the value:"" field is reachable
// only through the embed.
type innerEmbed struct {
	wrapper

	Extra string `property:"E"`
}

// embeddedWrapperOuter checks that diagramStructValuesTag recurses through
// anonymous embeds to find the value:"" field.
type embeddedWrapperOuter struct {
	Field innerEmbed `property:"X"`
}

// fieldOf returns the "Field" struct field of typ, failing the test if
// missing. All fixtures in this file name the property field "Field" so the
// helper is fixed to that name.
func fieldOf(t *testing.T, typ reflect.Type) reflect.StructField {
	t.Helper()
	f, ok := typ.FieldByName("Field")
	if !ok {
		t.Fatalf("missing field %q on %s", "Field", typ)
	}
	return f
}

// TestDiagramValuesTag_OuterRef verifies that for a Ref-typed property field
// the values tag is read from the outer field itself (single, slice, pointer).
func TestDiagramValuesTag_OuterRef(t *testing.T) {
	t.Parallel()
	cases := []reflect.Type{
		reflect.TypeFor[refOuter](),
		reflect.TypeFor[sliceRefOuter](),
		reflect.TypeFor[ptrRefOuter](),
	}
	for _, typ := range cases {
		field := fieldOf(t, typ)
		got, _ := export.TestingDiagramValuesTag(field)
		assert.Equal(t, "core.peerdb.org,INSTANCE_OF=core.peerdb.org,LANGUAGE", got, "field type %s", typ)
	}
}

// TestDiagramValuesTag_OuterRefNoTag returns empty when no values tag is set.
func TestDiagramValuesTag_OuterRefNoTag(t *testing.T) {
	t.Parallel()
	field := fieldOf(t, reflect.TypeFor[refOuterNoTag]())
	tag, _ := export.TestingDiagramValuesTag(field)
	assert.Empty(t, tag)
}

// TestDiagramValuesTag_InnerValueField verifies that for a struct-typed
// property field the values tag is read from the wrapper's value:"" field,
// covering single, slice, and pointer wrappers.
func TestDiagramValuesTag_InnerValueField(t *testing.T) {
	t.Parallel()
	cases := []reflect.Type{
		reflect.TypeFor[wrapperOuter](),
		reflect.TypeFor[sliceWrapperOuter](),
		reflect.TypeFor[ptrWrapperOuter](),
	}
	for _, typ := range cases {
		field := fieldOf(t, typ)
		got, _ := export.TestingDiagramValuesTag(field)
		assert.Equal(t, "core.peerdb.org,INSTANCE_OF=core.peerdb.org,UNIT", got, "field type %s", typ)
	}
}

// TestDiagramValuesTag_InnerNoValues returns empty when the wrapper's
// value:"" field has no values tag.
func TestDiagramValuesTag_InnerNoValues(t *testing.T) {
	t.Parallel()
	field := fieldOf(t, reflect.TypeFor[wrapperNoValuesOuter]())
	tag, _ := export.TestingDiagramValuesTag(field)
	assert.Empty(t, tag)
}

// TestDiagramValuesTag_InnerThroughEmbed verifies recursion through anonymous
// embedded structs when finding the value:"" field.
func TestDiagramValuesTag_InnerThroughEmbed(t *testing.T) {
	t.Parallel()
	field := fieldOf(t, reflect.TypeFor[embeddedWrapperOuter]())
	got, _ := export.TestingDiagramValuesTag(field)
	assert.Equal(t, "core.peerdb.org,INSTANCE_OF=core.peerdb.org,UNIT", got)
}

// TestDiagramValuesTag_DoesNotPickInnerPropertySubField verifies that a
// property-tagged Ref sub-field inside the wrapper (e.g. IN_LANGUAGE) is not
// confused with the value:"" field when reading the outer property's tag.
// The values tag on IN_LANGUAGE is for the sub-claim edge, not the parent.
func TestDiagramValuesTag_DoesNotPickInnerPropertySubField(t *testing.T) {
	t.Parallel()
	field := fieldOf(t, reflect.TypeFor[wrapperOuter]())
	got, _ := export.TestingDiagramValuesTag(field)
	assert.Equal(t, "core.peerdb.org,INSTANCE_OF=core.peerdb.org,UNIT", got,
		"outer dispatch must read value:\"\" tag, not the IN_LANGUAGE sub-claim tag")
}

// TestResolveDiagramRefTargets_OuterRef verifies the full path: a Ref-typed
// property field with a values tag resolves to the registered class name.
func TestResolveDiagramRefTargets_OuterRef(t *testing.T) {
	t.Parallel()
	field := fieldOf(t, reflect.TypeFor[refOuter]())
	idToName := map[identifier.Identifier]string{
		identifier.From(core.Namespace, "LANGUAGE"): "LANGUAGE",
	}
	targets := export.TestingResolveDiagramRefTargets(zerolog.Nop(), field, "X", idToName, nil)
	assert.Equal(t, []string{"LANGUAGE"}, targets)
}

// TestResolveDiagramRefTargets_InnerValueField verifies the full path for the
// razume-style wrapper struct: the inner value:""'s values tag drives the
// target lookup.
func TestResolveDiagramRefTargets_InnerValueField(t *testing.T) {
	t.Parallel()
	field := fieldOf(t, reflect.TypeFor[wrapperOuter]())
	idToName := map[identifier.Identifier]string{
		identifier.From(core.Namespace, "UNIT"): "UNIT",
	}
	targets := export.TestingResolveDiagramRefTargets(zerolog.Nop(), field, "X", idToName, nil)
	assert.Equal(t, []string{"UNIT"}, targets)
}

// TestResolveDiagramRefTargets_UnregisteredTarget verifies that a values tag
// pointing to a class not in idToName produces no edge and logs a warning.
func TestResolveDiagramRefTargets_UnregisteredTarget(t *testing.T) {
	t.Parallel()
	field := fieldOf(t, reflect.TypeFor[wrapperOuter]())
	var buf bytes.Buffer
	logger := zerolog.New(&buf)
	targets := export.TestingResolveDiagramRefTargets(logger, field, "X", map[identifier.Identifier]string{}, nil)
	assert.Empty(t, targets)
	unitID := identifier.From(core.Namespace, "UNIT").String()
	assert.Equal(
		t,
		`{"level":"warn","property":"X","targetID":"`+unitID+
			`","targetToken":"core.peerdb.org,UNIT","message":"values tag points to a class not in the diagram; FK row will have no edge"}`+"\n",
		buf.String(),
	)
}

// TestResolveDiagramRefTargets_MissingTagOnOuterRef verifies that a Ref-typed
// property field without a values tag produces no edge and logs a warning.
func TestResolveDiagramRefTargets_MissingTagOnOuterRef(t *testing.T) {
	t.Parallel()
	field := fieldOf(t, reflect.TypeFor[refOuterNoTag]())
	var buf bytes.Buffer
	logger := zerolog.New(&buf)
	targets := export.TestingResolveDiagramRefTargets(logger, field, "MISSING", map[identifier.Identifier]string{}, nil)
	assert.Empty(t, targets)
	assert.Equal( //nolint:testifylint
		t,
		`{"level":"warn","property":"MISSING","type":"core.Ref","message":"Ref-typed field has no values tag; FK row will have no edge"}`+"\n",
		buf.String(),
	)
}

// TestResolveDiagramRefTargets_MissingTagOnInnerValue verifies the same warning
// fires when the wrapper's value:"" Ref carries no values tag.
func TestResolveDiagramRefTargets_MissingTagOnInnerValue(t *testing.T) {
	t.Parallel()
	field := fieldOf(t, reflect.TypeFor[wrapperNoValuesOuter]())
	var buf bytes.Buffer
	logger := zerolog.New(&buf)
	targets := export.TestingResolveDiagramRefTargets(logger, field, "MISSING_INNER", map[identifier.Identifier]string{}, nil)
	assert.Empty(t, targets)
	assert.Equal( //nolint:testifylint
		t,
		`{"level":"warn","property":"MISSING_INNER","type":"export_test.wrapperNoValues","message":"Ref-typed field has no values tag; FK row will have no edge"}`+"\n",
		buf.String(),
	)
}

// TestResolveDiagramRefTargets_SkippedTargetIsSilent verifies that when the
// values tag points to a class explicitly listed in skipIDs (typically because
// of --skip-core), no warning is emitted: the user asked for it to be gone.
func TestResolveDiagramRefTargets_SkippedTargetIsSilent(t *testing.T) {
	t.Parallel()
	field := fieldOf(t, reflect.TypeFor[wrapperOuter]())
	var buf bytes.Buffer
	logger := zerolog.New(&buf)
	skipIDs := map[identifier.Identifier]bool{
		identifier.From(core.Namespace, "UNIT"): true,
	}
	targets := export.TestingResolveDiagramRefTargets(logger, field, "X", map[identifier.Identifier]string{}, skipIDs)
	assert.Empty(t, targets)
	assert.Empty(t, buf.String(), "no warning expected when target is in skipIDs")
}
