package transform_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/peerdb/peerdb/core"
	"gitlab.com/peerdb/peerdb/transform"
)

// fieldsTestMnemonics returns a mnemonics map for testing Fields.
func fieldsTestMnemonics() map[string][]string {
	return map[string][]string{
		"NAME":         {"test", "NAME"},
		"DESCRIPTION":  {"test", "DESCRIPTION"},
		"AGE":          {"test", "AGE"},
		"HEIGHT":       {"test", "HEIGHT"},
		"BORN":         {"test", "BORN"},
		"PARENT":       {"test", "PARENT"},
		"CODE":         {"test", "CODE"},
		"HOMEPAGE":     {"test", "HOMEPAGE"},
		"PERIOD":       {"test", "PERIOD"},
		"WEIGHT":       {"test", "WEIGHT"},
		"STATUS":       {"test", "STATUS"},
		"SOMETHING":    {"test", "SOMETHING"},
		"BAR":          {"test", "BAR"},
		"FIRST":        {"test", "FIRST"},
		"SECOND":       {"test", "SECOND"},
		"THIRD":        {"test", "THIRD"},
		"AMOUNT":       {"test", "AMOUNT"},
		"NOTES":        {"test", "NOTES"},
		"ABSENT":       {"test", "ABSENT"},
		"UNKNOWN_PROP": {"test", "UNKNOWN_PROP"},
		"SCORE":        {"test", "SCORE"},
		"RANGE":        {"test", "RANGE"},
		"CREATED":      {"test", "CREATED"},
		"DATA":         {"test", "DATA"},
		"RAW":          {"test", "RAW"},
		"CHOICE":       {"test", "CHOICE"},
	}
}

type SimpleFields struct {
	Name string `cardinality:"1.."  json:"name" property:"NAME"`
	Age  *int   `cardinality:"0..1" json:"age"  property:"AGE"`
}

type NestedSection struct {
	Something string `cardinality:"1.." json:"something" property:"SOMETHING"`
}

type FieldsWithSection struct {
	NestedSection `section:"my-section"`

	Bar string `cardinality:"1.." json:"bar" property:"BAR"`
}

type SectionA struct {
	First  core.HTML `cardinality:"1"    json:"first"  property:"FIRST"`
	Second core.Ref  `cardinality:"0..1" json:"second" property:"SECOND" values:"test.example.com,YES_NO"`
}

type SectionB struct {
	Third *core.Time `cardinality:"0..1" json:"third" property:"THIRD"`
}

type MultipleSections struct {
	SectionA `section:"section-a"`
	SectionB `section:"section-b"`
}

type EmbeddedBase struct {
	Name string `cardinality:"1.." json:"name" property:"NAME"`
}

type FieldsWithEmbedded struct {
	EmbeddedBase

	Description string `cardinality:"0..1" json:"description" property:"DESCRIPTION"`
}

type RefFieldWithValues struct {
	Choice  core.Ref   `cardinality:"1"   json:"choice"  property:"CHOICE" values:"ns.example.com,OPT_A;ns.example.com,OPT_B"`
	Choices []core.Ref `cardinality:"0.." json:"choices" property:"STATUS" values:"ns.example.com,STATUS_A"`
}

type AllTypeFields struct {
	Name        string                          `cardinality:"1"    json:"name"                         property:"NAME"`
	Code        core.Identifier                 `cardinality:"1"    json:"code"                         property:"CODE"`
	Homepage    core.Link                       `cardinality:"0..1" json:"homepage"                     property:"HOMEPAGE"`
	Description core.HTML                       `cardinality:"0..1" json:"description"                  property:"DESCRIPTION"`
	Notes       core.RawHTML                    `cardinality:"0..1" json:"notes"                        property:"NOTES"`
	Age         int                             `cardinality:"1"    json:"age"         precision:"1"    property:"AGE"`
	Height      float64                         `cardinality:"0..1" json:"height"      precision:"0.01" property:"HEIGHT"`
	Born        time.Time                       `cardinality:"0..1" json:"born"        precision:"d"    property:"BORN"`
	Created     core.Time                       `cardinality:"0..1" json:"created"                      property:"CREATED"`
	Period      core.Interval[core.Time]        `cardinality:"0..1" json:"period"                       property:"PERIOD"`
	Amount      core.Amount[int]                `cardinality:"0..1" json:"amount"                       property:"AMOUNT"`
	Score       core.Amount[float64]            `cardinality:"0..1" json:"score"                        property:"SCORE"`
	Range       core.Interval[core.Amount[int]] `cardinality:"0..1" json:"range"                        property:"RANGE"`
	Weight      *float64                        `cardinality:"0..1" json:"weight"      precision:"0.1"  property:"WEIGHT"`
	Parent      core.Ref                        `cardinality:"0..1" json:"parent"                       property:"PARENT"`
	Absent      core.None                       `cardinality:"0..1" json:"absent"                       property:"ABSENT"`
	Unknown     core.Unknown                    `cardinality:"0..1" json:"unknown"                      property:"UNKNOWN_PROP"`
}

type FieldsWithStringTypes struct {
	Code     string `cardinality:"1"    json:"code"     property:"CODE"     type:"id"`
	Homepage string `cardinality:"0..1" json:"homepage" property:"HOMEPAGE" type:"link"`
	Data     string `cardinality:"0..1" json:"data"     property:"DATA"     type:"html"`
	Raw      string `cardinality:"0..1" json:"raw"      property:"RAW"      type:"rawhtml"`
}

type FieldsWithBoolNone struct {
	Absent bool `cardinality:"0..1" json:"absent" property:"ABSENT" type:"none"`
}

type FieldsWithBoolUnknown struct {
	Unknown bool `cardinality:"0..1" json:"unknown" property:"UNKNOWN_PROP" type:"unknown"`
}

type FieldsWithBoolHas struct {
	Published bool `cardinality:"0..1" json:"published" property:"NAME"`
}

type FieldsWithFileType struct {
	Upload string `cardinality:"0..1" json:"upload" property:"DATA" type:"file"`
}

type FieldsWithCoreFile struct {
	Upload core.File `cardinality:"0..1" json:"upload" property:"DATA"`
}

type FieldsWithValuesOnNonRef struct {
	Name string `cardinality:"1.." json:"name" property:"NAME" values:"test.example.com,FOO"`
}

type ValueStruct struct {
	Value core.Amount[int] `json:"value"                 value:""`
	Name  string           `json:"name"  property:"NAME"`
}

type FieldsWithValueStruct struct {
	Data ValueStruct `cardinality:"1" json:"data" property:"DATA"`
}

type FieldsWithDocumentID struct {
	ID   []string `                  documentid:"" json:"id"`
	Name string   `cardinality:"1.."               json:"name" property:"NAME"`
}

type FieldsWithSkippedField struct {
	Name     string `cardinality:"1.." json:"name"     property:"NAME"`
	Internal string `                  json:"internal" property:"-"`
}

type NestedSectionWithSection struct {
	First string `cardinality:"1" json:"first" property:"FIRST"`
}

type OuterSection struct {
	NestedSectionWithSection `section:"nested"`
}

type FieldsWithNestedSections struct {
	OuterSection `section:"outer"`
}

type EmbeddedInSection struct {
	First string `cardinality:"1" json:"first" property:"FIRST"`
}

type SectionWithEmbedded struct {
	EmbeddedInSection

	Second string `cardinality:"1" json:"second" property:"SECOND"`
}

type FieldsWithSectionEmbedded struct {
	SectionWithEmbedded `section:"embedded-section"`
}

type FieldsWithOrderTag struct {
	First  string `cardinality:"1" json:"first"  order:"10.5" property:"FIRST"`
	Second string `cardinality:"1" json:"second"              property:"SECOND"`
	Third  string `cardinality:"1" json:"third"  order:"5"    property:"THIRD"`
}

type FieldsWithOrderSkip struct {
	Name    string `cardinality:"1.." json:"name"              property:"NAME"`
	Skipped string `cardinality:"1"   json:"skipped" order:"-" property:"CODE"`
	Other   string `cardinality:"1"   json:"other"             property:"DESCRIPTION"`
}

type SectionWithOrder struct {
	First string `cardinality:"1" json:"first" property:"FIRST"`
}

type FieldsWithSectionOrder struct {
	SectionWithOrder `order:"99" section:"ordered-section"`

	Bar string `cardinality:"1.." json:"bar" property:"BAR"`
}

// NestedWithSubFields is a struct with a value field and sub-fields.
type NestedWithSubFields struct {
	Value  string `json:"value"                    value:""`
	Period string `json:"period" property:"PERIOD"`
	Note   string `json:"note"   property:"NOTES"`
}

type FieldsWithSubFields struct {
	Data NestedWithSubFields `cardinality:"1" json:"data" property:"DATA"`
}

// NestedNoValue is a struct without a value field (maps to has claim), with sub-fields.
type NestedNoValue struct {
	Location string `json:"location" property:"HOMEPAGE"`
	Note     string `json:"note"     property:"NOTES"`
}

type FieldsWithNestedNoValue struct {
	Address NestedNoValue `cardinality:"1" json:"address" property:"DATA"`
}

// NestedWithSkippedSubField has sub-fields where one is skipped via order:"-".
type NestedWithSkippedSubField struct {
	Value   string `json:"value"                               value:""`
	Visible string `json:"visible"           property:"NOTES"`
	Hidden  string `json:"hidden"  order:"-" property:"PERIOD"`
}

type FieldsWithSkippedSubField struct {
	Data NestedWithSkippedSubField `cardinality:"1" json:"data" property:"DATA"`
}

// NestedAllSkipped has all sub-fields skipped.
type NestedAllSkipped struct {
	Value  string `json:"value"                             value:""`
	Hidden string `json:"hidden" order:"-" property:"NOTES"`
}

type FieldsWithAllSubFieldsSkipped struct {
	Data NestedAllSkipped `cardinality:"1" json:"data" property:"DATA"`
}

// NestedWithOrderedSubFields has sub-fields with explicit order.
type NestedWithOrderedSubFields struct {
	Value  string `json:"value"                              value:""`
	First  string `json:"first"  order:"5" property:"NOTES"`
	Second string `json:"second"           property:"PERIOD"`
}

type FieldsWithOrderedSubFields struct {
	Data NestedWithOrderedSubFields `cardinality:"1" json:"data" property:"DATA"`
}

// SliceOfNestedWithSubFields tests sub-fields on a slice of structs.
type FieldsWithSliceSubFields struct {
	Items []NestedWithSubFields `cardinality:"0.." json:"items" property:"DATA"`
}

// RecursiveStruct is a struct that references itself, causing recursion.
type RecursiveStruct struct {
	Name  string           `json:"name"  property:"NAME"`
	Child *RecursiveStruct `json:"child" property:"DATA"`
}

type FieldsWithRecursion struct {
	Data RecursiveStruct `cardinality:"1" json:"data" property:"DATA"`
}

// SharedSubStruct is used by multiple fields in the same parent.
type SharedSubStruct struct {
	Value string `json:"value"                  value:""`
	Note  string `json:"note"  property:"NOTES"`
}

type FieldsWithInverseProperty struct {
	Parent core.Ref `cardinality:"0..1" inverseProperty:"FIRST" json:"parent" property:"PARENT"`
	Name   string   `cardinality:"1.."                          json:"name"   property:"NAME"`
}

type FieldsWithSharedSubStruct struct {
	First  SharedSubStruct `cardinality:"1" json:"first"  property:"FIRST"`
	Second SharedSubStruct `cardinality:"1" json:"second" property:"SECOND"`
}

type FieldsWithDefaultCardinality struct {
	Names  []string  `json:"names"                property:"NAME"`
	Age    int       `json:"age"    precision:"1" property:"AGE"`
	Parent *core.Ref `json:"parent"               property:"PARENT"`
}

// StandaloneFieldSection tests fields assigned to a section via section tag on individual fields.
type StandaloneFieldSectionDef struct{}

type FieldsWithStandaloneFieldSection struct {
	StandaloneFieldSectionDef `section:"standalone"`

	First  string `cardinality:"1" json:"first"  property:"FIRST"  section:"standalone"`
	Second string `cardinality:"1" json:"second" property:"SECOND" section:"standalone"`
	Third  string `cardinality:"1" json:"third"  property:"THIRD"`
}

// FieldSectionOverride tests that a field inside a section can override its section.
type FieldsInOverrideSection struct {
	First  string `cardinality:"1" json:"first"  property:"FIRST"`
	Second string `cardinality:"1" json:"second" property:"SECOND" section:"other"`
}

type OverrideSectionDef struct{}

type FieldsWithSectionOverride struct {
	FieldsInOverrideSection `section:"main"`
	OverrideSectionDef      `section:"other"`
}

// FieldsWithUndefinedSectionOrder tests that referencing a section without defining its order is an error.
type FieldsWithUndefinedSectionOrder struct {
	First string `cardinality:"1" json:"first" property:"FIRST" section:"undefined"`
}

// DuplicateSectionDefA and DuplicateSectionDefB both define the same section.
type DuplicateSectionDefA struct {
	First string `cardinality:"1" json:"first" property:"FIRST"`
}

type DuplicateSectionDefB struct {
	Second string `cardinality:"1" json:"second" property:"SECOND"`
}

type FieldsWithDuplicateSectionDef struct {
	DuplicateSectionDefA `section:"dup"`
	DuplicateSectionDefB `section:"dup"`
}

// SubFieldWithSection tests that sub-fields cannot have section tags.
type NestedWithSectionSubField struct {
	Value string `json:"value"                                      value:""`
	Bad   string `json:"bad"   property:"NOTES" section:"forbidden"`
}

type FieldsWithSubFieldSection struct {
	Data NestedWithSectionSubField `cardinality:"1" json:"data" property:"DATA"`
}

// MixedSectionFields tests fields from both embedded struct and standalone going to the same section.
type SectionFieldsBase struct {
	First string `cardinality:"1" json:"first" property:"FIRST"`
}

type FieldsWithMixedSectionFields struct {
	SectionFieldsBase `section:"mixed"`

	Second string `cardinality:"1" json:"second" property:"SECOND" section:"mixed"`
}

// SectionWithEmbeddedAndOrder tests that nil order is passed through correctly
// when a section contains a plain embedded struct (exercising the processLevel
// call with nil order for both the section and the plain embedded struct inside it).
type EmbeddedInsideSection struct {
	First string `cardinality:"1" json:"first" order:"10" property:"FIRST"`
}

type SectionWithEmbeddedAndOrder struct {
	EmbeddedInsideSection

	Second string `cardinality:"1" json:"second"           property:"SECOND"`
	Third  string `cardinality:"1" json:"third"  order:"5" property:"THIRD"`
}

type FieldsWithSectionEmbeddedOrder struct {
	SectionWithEmbeddedAndOrder `section:"sec"`

	Bar string `cardinality:"1" json:"bar" property:"BAR"`
}

func TestFieldsSimple(t *testing.T) {
	t.Parallel()

	mnemonics := fieldsTestMnemonics()

	result, errE := transform.Fields[SimpleFields](mnemonics)
	require.NoError(t, errE, "% -+#.1v", errE)

	assert.Empty(t, result.Section)
	require.Len(t, result.Field, 2)

	// Field 1: Name (string, cardinality 1..).
	f := result.Field[0]
	assert.Equal(t, core.Ref{ID: mnemonics["NAME"]}, f.Property)
	assert.Equal(t, core.Ref{ID: []string{core.Namespace, "VALUE_TYPE", "STRING"}}, f.ValueType)
	assert.Equal(t, core.Amount[float64]{Amount: 1, Precision: 1}, f.OrderInList)
	require.NotNil(t, f.Cardinality.From)
	assert.Equal(t, 1, f.Cardinality.From.Amount)
	assert.True(t, f.Cardinality.ToIsNone)
	assert.Nil(t, f.Cardinality.To)
	assert.Empty(t, f.Values)

	// Field 2: Age (pointer to int, cardinality 0..1).
	f = result.Field[1]
	assert.Equal(t, core.Ref{ID: mnemonics["AGE"]}, f.Property)
	assert.Equal(t, core.Ref{ID: []string{core.Namespace, "VALUE_TYPE", "AMOUNT"}}, f.ValueType)
	assert.Equal(t, core.Amount[float64]{Amount: 2, Precision: 1}, f.OrderInList)
	require.NotNil(t, f.Cardinality.From)
	assert.Equal(t, 0, f.Cardinality.From.Amount)
	require.NotNil(t, f.Cardinality.To)
	assert.Equal(t, 1, f.Cardinality.To.Amount)
	assert.True(t, f.Cardinality.ToIsClosed)
}

func TestFieldsWithSection(t *testing.T) {
	t.Parallel()

	mnemonics := fieldsTestMnemonics()

	result, errE := transform.Fields[FieldsWithSection](mnemonics)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Top-level: one section and one field (Bar).
	require.Len(t, result.Section, 1)
	require.Len(t, result.Field, 1)

	// Section (order 1).
	section := result.Section[0]
	assert.Equal(t, "my-section", string(section.ID))
	assert.Equal(t, core.Amount[float64]{Amount: 1, Precision: 1}, section.OrderInList)

	// Bar field (order 1, top-level fields have their own counter).
	assert.Equal(t, core.Amount[float64]{Amount: 1, Precision: 1}, result.Field[0].OrderInList)
	assert.Equal(t, core.Ref{ID: mnemonics["BAR"]}, result.Field[0].Property)
	assert.Equal(t, core.Ref{ID: []string{core.Namespace, "VALUE_TYPE", "STRING"}}, result.Field[0].ValueType)

	// Section fields.
	require.Len(t, section.Field, 1)
	assert.Equal(t, core.Amount[float64]{Amount: 1, Precision: 1}, section.Field[0].OrderInList)
	assert.Equal(t, core.Ref{ID: mnemonics["SOMETHING"]}, section.Field[0].Property)
}

func TestFieldsMultipleSections(t *testing.T) {
	t.Parallel()

	mnemonics := fieldsTestMnemonics()

	result, errE := transform.Fields[MultipleSections](mnemonics)
	require.NoError(t, errE, "% -+#.1v", errE)

	assert.Empty(t, result.Field)
	require.Len(t, result.Section, 2)

	// Section A.
	sA := result.Section[0]
	assert.Equal(t, "section-a", string(sA.ID))
	assert.Equal(t, core.Amount[float64]{Amount: 1, Precision: 1}, sA.OrderInList)
	require.Len(t, sA.Field, 2)
	assert.Equal(t, core.Ref{ID: []string{core.Namespace, "VALUE_TYPE", "HTML"}}, sA.Field[0].ValueType)
	assert.Equal(t, core.Ref{ID: []string{core.Namespace, "VALUE_TYPE", "REFERENCE"}}, sA.Field[1].ValueType)

	// Check values on SECOND field.
	require.Len(t, sA.Field[1].Values, 1)
	assert.Equal(t, core.Ref{ID: []string{"test.example.com", "YES_NO"}}, sA.Field[1].Values[0])

	// Section B.
	sB := result.Section[1]
	assert.Equal(t, "section-b", string(sB.ID))
	assert.Equal(t, core.Amount[float64]{Amount: 2, Precision: 1}, sB.OrderInList)
	require.Len(t, sB.Field, 1)
	assert.Equal(t, core.Ref{ID: []string{core.Namespace, "VALUE_TYPE", "TIME"}}, sB.Field[0].ValueType)
}

func TestFieldsWithEmbedded(t *testing.T) {
	t.Parallel()

	mnemonics := fieldsTestMnemonics()

	result, errE := transform.Fields[FieldsWithEmbedded](mnemonics)
	require.NoError(t, errE, "% -+#.1v", errE)

	assert.Empty(t, result.Section)
	require.Len(t, result.Field, 2)

	// EmbeddedBase.Name comes first, then Description.
	assert.Equal(t, core.Amount[float64]{Amount: 1, Precision: 1}, result.Field[0].OrderInList)
	assert.Equal(t, core.Ref{ID: mnemonics["NAME"]}, result.Field[0].Property)
	assert.Equal(t, core.Amount[float64]{Amount: 2, Precision: 1}, result.Field[1].OrderInList)
	assert.Equal(t, core.Ref{ID: mnemonics["DESCRIPTION"]}, result.Field[1].Property)
}

func TestFieldsAllTypes(t *testing.T) {
	t.Parallel()

	mnemonics := fieldsTestMnemonics()

	result, errE := transform.Fields[AllTypeFields](mnemonics)
	require.NoError(t, errE, "% -+#.1v", errE)

	assert.Empty(t, result.Section)
	require.Len(t, result.Field, 17)

	expectedTypes := []string{
		"STRING",          // Name (string).
		"IDENTIFIER",      // Code (core.Identifier).
		"LINK",            // Homepage (core.Link).
		"HTML",            // Description (core.HTML).
		"HTML",            // Notes (core.RawHTML).
		"AMOUNT",          // Age (int).
		"AMOUNT",          // Height (float64).
		"TIME",            // Born (time.Time).
		"TIME",            // Created (core.Time).
		"TIME_INTERVAL",   // Period (core.Interval[core.Time]).
		"AMOUNT",          // Amount (core.Amount[int]).
		"AMOUNT",          // Score (core.Amount[float64]).
		"AMOUNT_INTERVAL", // Range (core.Interval[core.Amount[int]]).
		"AMOUNT",          // Weight (*float64).
		"REFERENCE",       // Parent (core.Ref).
		"NONE",            // Absent (core.None).
		"UNKNOWN",         // Unknown (core.Unknown).
	}

	for i, expected := range expectedTypes {
		assert.Equal(t, core.Ref{ID: []string{core.Namespace, "VALUE_TYPE", expected}}, result.Field[i].ValueType, "field %d", i)
		assert.Equal(t, core.Amount[float64]{Amount: float64(i + 1), Precision: 1}, result.Field[i].OrderInList, "field %d", i)
	}
}

func TestFieldsStringTypes(t *testing.T) {
	t.Parallel()

	mnemonics := fieldsTestMnemonics()

	result, errE := transform.Fields[FieldsWithStringTypes](mnemonics)
	require.NoError(t, errE, "% -+#.1v", errE)

	require.Len(t, result.Field, 4)
	assert.Equal(t, core.Ref{ID: []string{core.Namespace, "VALUE_TYPE", "IDENTIFIER"}}, result.Field[0].ValueType)
	assert.Equal(t, core.Ref{ID: []string{core.Namespace, "VALUE_TYPE", "LINK"}}, result.Field[1].ValueType)
	assert.Equal(t, core.Ref{ID: []string{core.Namespace, "VALUE_TYPE", "HTML"}}, result.Field[2].ValueType)
	assert.Equal(t, core.Ref{ID: []string{core.Namespace, "VALUE_TYPE", "HTML"}}, result.Field[3].ValueType)
}

func TestFieldsBoolNone(t *testing.T) {
	t.Parallel()

	mnemonics := fieldsTestMnemonics()

	result, errE := transform.Fields[FieldsWithBoolNone](mnemonics)
	require.NoError(t, errE, "% -+#.1v", errE)

	require.Len(t, result.Field, 1)
	assert.Equal(t, core.Ref{ID: []string{core.Namespace, "VALUE_TYPE", "NONE"}}, result.Field[0].ValueType)
}

func TestFieldsBoolUnknown(t *testing.T) {
	t.Parallel()

	mnemonics := fieldsTestMnemonics()

	result, errE := transform.Fields[FieldsWithBoolUnknown](mnemonics)
	require.NoError(t, errE, "% -+#.1v", errE)

	require.Len(t, result.Field, 1)
	assert.Equal(t, core.Ref{ID: []string{core.Namespace, "VALUE_TYPE", "UNKNOWN"}}, result.Field[0].ValueType)
}

func TestFieldsBoolHas(t *testing.T) {
	t.Parallel()

	mnemonics := fieldsTestMnemonics()

	result, errE := transform.Fields[FieldsWithBoolHas](mnemonics)
	require.NoError(t, errE, "% -+#.1v", errE)

	require.Len(t, result.Field, 1)
	assert.Equal(t, core.Ref{ID: []string{core.Namespace, "VALUE_TYPE", "HAS"}}, result.Field[0].ValueType)
}

func TestFieldsFileType(t *testing.T) {
	t.Parallel()

	mnemonics := fieldsTestMnemonics()

	result, errE := transform.Fields[FieldsWithFileType](mnemonics)
	require.NoError(t, errE, "% -+#.1v", errE)

	require.Len(t, result.Field, 1)
	assert.Equal(t, core.Ref{ID: []string{core.Namespace, "VALUE_TYPE", "FILE"}}, result.Field[0].ValueType)
}

func TestFieldsCoreFile(t *testing.T) {
	t.Parallel()

	mnemonics := fieldsTestMnemonics()

	result, errE := transform.Fields[FieldsWithCoreFile](mnemonics)
	require.NoError(t, errE, "% -+#.1v", errE)

	require.Len(t, result.Field, 1)
	assert.Equal(t, core.Ref{ID: []string{core.Namespace, "VALUE_TYPE", "FILE"}}, result.Field[0].ValueType)
}

func TestFieldsRefWithValues(t *testing.T) {
	t.Parallel()

	mnemonics := fieldsTestMnemonics()

	result, errE := transform.Fields[RefFieldWithValues](mnemonics)
	require.NoError(t, errE, "% -+#.1v", errE)

	require.Len(t, result.Field, 2)

	// Choice: two values.
	require.Len(t, result.Field[0].Values, 2)
	assert.Equal(t, core.Ref{ID: []string{"ns.example.com", "OPT_A"}}, result.Field[0].Values[0])
	assert.Equal(t, core.Ref{ID: []string{"ns.example.com", "OPT_B"}}, result.Field[0].Values[1])

	// Choices: one value.
	require.Len(t, result.Field[1].Values, 1)
	assert.Equal(t, core.Ref{ID: []string{"ns.example.com", "STATUS_A"}}, result.Field[1].Values[0])
}

func TestFieldsValuesOnNonRefError(t *testing.T) {
	t.Parallel()

	mnemonics := fieldsTestMnemonics()

	_, errE := transform.Fields[FieldsWithValuesOnNonRef](mnemonics)
	require.Error(t, errE)
	assert.EqualError(t, errE, "values tag can only be used with core.Ref field type")
}

func TestFieldsValueStruct(t *testing.T) {
	t.Parallel()

	mnemonics := fieldsTestMnemonics()

	result, errE := transform.Fields[FieldsWithValueStruct](mnemonics)
	require.NoError(t, errE, "% -+#.1v", errE)

	require.Len(t, result.Field, 1)
	// The value field is core.Amount[int], so value type should be AMOUNT.
	assert.Equal(t, core.Ref{ID: []string{core.Namespace, "VALUE_TYPE", "AMOUNT"}}, result.Field[0].ValueType)
}

func TestFieldsWithDocumentID(t *testing.T) {
	t.Parallel()

	mnemonics := fieldsTestMnemonics()

	result, errE := transform.Fields[FieldsWithDocumentID](mnemonics)
	require.NoError(t, errE, "% -+#.1v", errE)

	// documentid field is skipped, only Name field remains.
	require.Len(t, result.Field, 1)
	assert.Equal(t, core.Ref{ID: mnemonics["NAME"]}, result.Field[0].Property)
}

func TestFieldsWithSkippedField(t *testing.T) {
	t.Parallel()

	mnemonics := fieldsTestMnemonics()

	result, errE := transform.Fields[FieldsWithSkippedField](mnemonics)
	require.NoError(t, errE, "% -+#.1v", errE)

	// property:"-" field is skipped.
	require.Len(t, result.Field, 1)
}

func TestFieldsNestedSectionsError(t *testing.T) {
	t.Parallel()

	mnemonics := fieldsTestMnemonics()

	_, errE := transform.Fields[FieldsWithNestedSections](mnemonics)
	require.Error(t, errE)
	assert.EqualError(t, errE, "sections cannot be nested inside sections")
}

func TestFieldsSectionWithEmbedded(t *testing.T) {
	t.Parallel()

	mnemonics := fieldsTestMnemonics()

	result, errE := transform.Fields[FieldsWithSectionEmbedded](mnemonics)
	require.NoError(t, errE, "% -+#.1v", errE)

	require.Len(t, result.Section, 1)
	assert.Empty(t, result.Field)

	// Section has 2 fields: embedded First + Second.
	section := result.Section[0]
	assert.Equal(t, "embedded-section", string(section.ID))
	require.Len(t, section.Field, 2)
	assert.Equal(t, core.Amount[float64]{Amount: 1, Precision: 1}, section.Field[0].OrderInList)
	assert.Equal(t, core.Ref{ID: mnemonics["FIRST"]}, section.Field[0].Property)
	assert.Equal(t, core.Amount[float64]{Amount: 2, Precision: 1}, section.Field[1].OrderInList)
	assert.Equal(t, core.Ref{ID: mnemonics["SECOND"]}, section.Field[1].Property)
}

func TestFieldsMnemonicNotFound(t *testing.T) {
	t.Parallel()

	// Empty mnemonics.
	_, errE := transform.Fields[SimpleFields](map[string][]string{})
	require.Error(t, errE)
	assert.EqualError(t, errE, "mnemonic not found")
}

type EmptyFields struct{}

func TestFieldsEmptyStruct(t *testing.T) {
	t.Parallel()

	mnemonics := fieldsTestMnemonics()

	result, errE := transform.Fields[EmptyFields](mnemonics)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Nil(t, result)
}

func TestFieldsNotStruct(t *testing.T) {
	t.Parallel()

	mnemonics := fieldsTestMnemonics()

	_, errE := transform.Fields[string](mnemonics)
	require.Error(t, errE)
	assert.EqualError(t, errE, "expected struct")
}

func TestFieldsCardinality(t *testing.T) {
	t.Parallel()

	mnemonics := fieldsTestMnemonics()

	result, errE := transform.Fields[FieldsWithDefaultCardinality](mnemonics)
	require.NoError(t, errE, "% -+#.1v", errE)

	require.Len(t, result.Field, 3)

	// Names (slice, no cardinality tag): default 0..unbounded.
	f0 := result.Field[0]
	require.NotNil(t, f0.Cardinality.From)
	assert.Equal(t, 0, f0.Cardinality.From.Amount)
	assert.True(t, f0.Cardinality.ToIsNone)

	// Age (single value, no cardinality tag): default 0..1.
	f1 := result.Field[1]
	require.NotNil(t, f1.Cardinality.From)
	assert.Equal(t, 0, f1.Cardinality.From.Amount)
	require.NotNil(t, f1.Cardinality.To)
	assert.Equal(t, 1, f1.Cardinality.To.Amount)
	assert.True(t, f1.Cardinality.ToIsClosed)

	// Parent (pointer, no cardinality tag): default 0..1.
	f2 := result.Field[2]
	require.NotNil(t, f2.Cardinality.From)
	assert.Equal(t, 0, f2.Cardinality.From.Amount)
	require.NotNil(t, f2.Cardinality.To)
	assert.Equal(t, 1, f2.Cardinality.To.Amount)
	assert.True(t, f2.Cardinality.ToIsClosed)
}

func TestFieldsOrderTag(t *testing.T) {
	t.Parallel()

	mnemonics := fieldsTestMnemonics()

	result, errE := transform.Fields[FieldsWithOrderTag](mnemonics)
	require.NoError(t, errE, "% -+#.1v", errE)

	require.Len(t, result.Field, 3)

	// First has explicit order 10.5.
	assert.Equal(t, core.Ref{ID: mnemonics["FIRST"]}, result.Field[0].Property)
	assert.Equal(t, core.Amount[float64]{Amount: 10.5, Precision: 0.1}, result.Field[0].OrderInList)

	// Second has auto-increment order (1.0, since First used explicit order).
	assert.Equal(t, core.Ref{ID: mnemonics["SECOND"]}, result.Field[1].Property)
	assert.Equal(t, core.Amount[float64]{Amount: 1, Precision: 1}, result.Field[1].OrderInList)

	// Third has explicit order 5.
	assert.Equal(t, core.Ref{ID: mnemonics["THIRD"]}, result.Field[2].Property)
	assert.Equal(t, core.Amount[float64]{Amount: 5, Precision: 1}, result.Field[2].OrderInList)
}

func TestFieldsOrderSkip(t *testing.T) {
	t.Parallel()

	mnemonics := fieldsTestMnemonics()

	result, errE := transform.Fields[FieldsWithOrderSkip](mnemonics)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Skipped field (order:"-") should not appear.
	require.Len(t, result.Field, 2)

	assert.Equal(t, core.Ref{ID: mnemonics["NAME"]}, result.Field[0].Property)
	assert.Equal(t, core.Amount[float64]{Amount: 1, Precision: 1}, result.Field[0].OrderInList)

	assert.Equal(t, core.Ref{ID: mnemonics["DESCRIPTION"]}, result.Field[1].Property)
	assert.Equal(t, core.Amount[float64]{Amount: 2, Precision: 1}, result.Field[1].OrderInList)
}

func TestFieldsSectionOrderTag(t *testing.T) {
	t.Parallel()

	mnemonics := fieldsTestMnemonics()

	result, errE := transform.Fields[FieldsWithSectionOrder](mnemonics)
	require.NoError(t, errE, "% -+#.1v", errE)

	require.Len(t, result.Section, 1)
	require.Len(t, result.Field, 1)

	// Section has explicit order 99.
	assert.Equal(t, "ordered-section", string(result.Section[0].ID))
	assert.Equal(t, core.Amount[float64]{Amount: 99, Precision: 1}, result.Section[0].OrderInList)

	// Bar gets auto-increment (1.0, since section used explicit order).
	assert.Equal(t, core.Amount[float64]{Amount: 1, Precision: 1}, result.Field[0].OrderInList)
}

func TestFieldsSubFields(t *testing.T) {
	t.Parallel()

	mnemonics := fieldsTestMnemonics()

	result, errE := transform.Fields[FieldsWithSubFields](mnemonics)
	require.NoError(t, errE, "% -+#.1v", errE)

	require.Len(t, result.Field, 1)
	f := result.Field[0]
	assert.Equal(t, core.Ref{ID: mnemonics["DATA"]}, f.Property)
	// Value type comes from the value field (string).
	assert.Equal(t, core.Ref{ID: []string{core.Namespace, "VALUE_TYPE", "STRING"}}, f.ValueType)

	// Sub-fields: Period and Note (value field is not a sub-field).
	require.Len(t, f.SubField, 2)
	assert.Equal(t, core.Ref{ID: mnemonics["PERIOD"]}, f.SubField[0].Property)
	assert.Equal(t, core.Amount[float64]{Amount: 1, Precision: 1}, f.SubField[0].OrderInList)
	assert.Equal(t, core.Ref{ID: mnemonics["NOTES"]}, f.SubField[1].Property)
	assert.Equal(t, core.Amount[float64]{Amount: 2, Precision: 1}, f.SubField[1].OrderInList)
}

func TestFieldsSubFieldsNoValue(t *testing.T) {
	t.Parallel()

	mnemonics := fieldsTestMnemonics()

	result, errE := transform.Fields[FieldsWithNestedNoValue](mnemonics)
	require.NoError(t, errE, "% -+#.1v", errE)

	require.Len(t, result.Field, 1)
	f := result.Field[0]
	// No value field -> HAS value type (maps to HasClaim in Documents).
	assert.Equal(t, core.Ref{ID: []string{core.Namespace, "VALUE_TYPE", "HAS"}}, f.ValueType)

	// Sub-fields: Location and Note.
	require.Len(t, f.SubField, 2)
	assert.Equal(t, core.Ref{ID: mnemonics["HOMEPAGE"]}, f.SubField[0].Property)
	assert.Equal(t, core.Amount[float64]{Amount: 1, Precision: 1}, f.SubField[0].OrderInList)
	assert.Equal(t, core.Ref{ID: mnemonics["NOTES"]}, f.SubField[1].Property)
	assert.Equal(t, core.Amount[float64]{Amount: 2, Precision: 1}, f.SubField[1].OrderInList)
}

func TestFieldsSubFieldsSkipped(t *testing.T) {
	t.Parallel()

	mnemonics := fieldsTestMnemonics()

	result, errE := transform.Fields[FieldsWithSkippedSubField](mnemonics)
	require.NoError(t, errE, "% -+#.1v", errE)

	require.Len(t, result.Field, 1)
	f := result.Field[0]

	// Only Visible sub-field remains (Hidden is skipped via order:"-").
	require.Len(t, f.SubField, 1)
	assert.Equal(t, core.Ref{ID: mnemonics["NOTES"]}, f.SubField[0].Property)
	assert.Equal(t, core.Amount[float64]{Amount: 1, Precision: 1}, f.SubField[0].OrderInList)
}

func TestFieldsSubFieldsAllSkipped(t *testing.T) {
	t.Parallel()

	mnemonics := fieldsTestMnemonics()

	result, errE := transform.Fields[FieldsWithAllSubFieldsSkipped](mnemonics)
	require.NoError(t, errE, "% -+#.1v", errE)

	require.Len(t, result.Field, 1)
	f := result.Field[0]

	// All sub-fields skipped -> SubField should be nil.
	assert.Nil(t, f.SubField)
}

func TestFieldsSubFieldsWithOrder(t *testing.T) {
	t.Parallel()

	mnemonics := fieldsTestMnemonics()

	result, errE := transform.Fields[FieldsWithOrderedSubFields](mnemonics)
	require.NoError(t, errE, "% -+#.1v", errE)

	require.Len(t, result.Field, 1)
	f := result.Field[0]

	require.Len(t, f.SubField, 2)
	// First has explicit order 5.
	assert.Equal(t, core.Ref{ID: mnemonics["NOTES"]}, f.SubField[0].Property)
	assert.Equal(t, core.Amount[float64]{Amount: 5, Precision: 1}, f.SubField[0].OrderInList)
	// Second has auto-increment order 1.
	assert.Equal(t, core.Ref{ID: mnemonics["PERIOD"]}, f.SubField[1].Property)
	assert.Equal(t, core.Amount[float64]{Amount: 1, Precision: 1}, f.SubField[1].OrderInList)
}

func TestFieldsSubFieldsSlice(t *testing.T) {
	t.Parallel()

	mnemonics := fieldsTestMnemonics()

	result, errE := transform.Fields[FieldsWithSliceSubFields](mnemonics)
	require.NoError(t, errE, "% -+#.1v", errE)

	require.Len(t, result.Field, 1)
	f := result.Field[0]

	// Sub-fields from the element type (NestedWithSubFields).
	require.Len(t, f.SubField, 2)
	assert.Equal(t, core.Ref{ID: mnemonics["PERIOD"]}, f.SubField[0].Property)
	assert.Equal(t, core.Ref{ID: mnemonics["NOTES"]}, f.SubField[1].Property)
}

func TestFieldsRecursionDetected(t *testing.T) {
	t.Parallel()

	mnemonics := fieldsTestMnemonics()

	_, errE := transform.Fields[FieldsWithRecursion](mnemonics)
	require.Error(t, errE)
	assert.EqualError(t, errE, "recursive struct type detected")
}

func TestFieldsSharedSubStruct(t *testing.T) {
	t.Parallel()

	mnemonics := fieldsTestMnemonics()

	// Two sibling fields using the same struct type should not trigger recursion.
	result, errE := transform.Fields[FieldsWithSharedSubStruct](mnemonics)
	require.NoError(t, errE, "% -+#.1v", errE)

	require.Len(t, result.Field, 2)

	// Both fields should have the same sub-field structure.
	require.Len(t, result.Field[0].SubField, 1)
	assert.Equal(t, core.Ref{ID: mnemonics["NOTES"]}, result.Field[0].SubField[0].Property)
	require.Len(t, result.Field[1].SubField, 1)
	assert.Equal(t, core.Ref{ID: mnemonics["NOTES"]}, result.Field[1].SubField[0].Property)
}

func TestFieldsInverseProperty(t *testing.T) {
	t.Parallel()

	mnemonics := fieldsTestMnemonics()

	result, errE := transform.Fields[FieldsWithInverseProperty](mnemonics)
	require.NoError(t, errE, "% -+#.1v", errE)

	require.Len(t, result.Field, 2)

	// Parent has inverseProperty set.
	f := result.Field[0]
	assert.Equal(t, core.Ref{ID: mnemonics["PARENT"]}, f.Property)
	require.NotNil(t, f.InverseProperty)
	assert.Equal(t, core.Ref{ID: mnemonics["FIRST"]}, *f.InverseProperty)

	// Name has no inverseProperty.
	f = result.Field[1]
	assert.Equal(t, core.Ref{ID: mnemonics["NAME"]}, f.Property)
	assert.Nil(t, f.InverseProperty)
}

func TestFieldsStandaloneFieldSection(t *testing.T) {
	t.Parallel()

	mnemonics := fieldsTestMnemonics()

	result, errE := transform.Fields[FieldsWithStandaloneFieldSection](mnemonics)
	require.NoError(t, errE, "% -+#.1v", errE)

	// One section and one top-level field.
	require.Len(t, result.Section, 1)
	require.Len(t, result.Field, 1)

	// Section contains First and Second.
	section := result.Section[0]
	assert.Equal(t, "standalone", string(section.ID))
	assert.Equal(t, core.Amount[float64]{Amount: 1, Precision: 1}, section.OrderInList)
	require.Len(t, section.Field, 2)
	assert.Equal(t, core.Ref{ID: mnemonics["FIRST"]}, section.Field[0].Property)
	assert.Equal(t, core.Ref{ID: mnemonics["SECOND"]}, section.Field[1].Property)

	// Third is top-level.
	assert.Equal(t, core.Ref{ID: mnemonics["THIRD"]}, result.Field[0].Property)
}

func TestFieldsSectionOverride(t *testing.T) {
	t.Parallel()

	mnemonics := fieldsTestMnemonics()

	result, errE := transform.Fields[FieldsWithSectionOverride](mnemonics)
	require.NoError(t, errE, "% -+#.1v", errE)

	assert.Empty(t, result.Field)
	require.Len(t, result.Section, 2)

	// "main" section gets First (default from embedded struct).
	main := result.Section[0]
	assert.Equal(t, "main", string(main.ID))
	require.Len(t, main.Field, 1)
	assert.Equal(t, core.Ref{ID: mnemonics["FIRST"]}, main.Field[0].Property)

	// "other" section gets Second (overridden by field's section tag).
	other := result.Section[1]
	assert.Equal(t, "other", string(other.ID))
	require.Len(t, other.Field, 1)
	assert.Equal(t, core.Ref{ID: mnemonics["SECOND"]}, other.Field[0].Property)
}

func TestFieldsUndefinedSectionOrder(t *testing.T) {
	t.Parallel()

	mnemonics := fieldsTestMnemonics()

	_, errE := transform.Fields[FieldsWithUndefinedSectionOrder](mnemonics)
	require.Error(t, errE)
	assert.EqualError(t, errE, "section order not defined")
}

func TestFieldsDuplicateSectionDef(t *testing.T) {
	t.Parallel()

	mnemonics := fieldsTestMnemonics()

	_, errE := transform.Fields[FieldsWithDuplicateSectionDef](mnemonics)
	require.Error(t, errE)
	assert.EqualError(t, errE, "section defined more than once")
}

func TestFieldsSubFieldSectionError(t *testing.T) {
	t.Parallel()

	mnemonics := fieldsTestMnemonics()

	_, errE := transform.Fields[FieldsWithSubFieldSection](mnemonics)
	require.Error(t, errE)
	assert.EqualError(t, errE, "sub-fields cannot have sections")
}

func TestFieldsMixedSectionFields(t *testing.T) {
	t.Parallel()

	mnemonics := fieldsTestMnemonics()

	result, errE := transform.Fields[FieldsWithMixedSectionFields](mnemonics)
	require.NoError(t, errE, "% -+#.1v", errE)

	assert.Empty(t, result.Field)
	require.Len(t, result.Section, 1)

	// Both First (from embedded) and Second (standalone) are in "mixed" section.
	section := result.Section[0]
	assert.Equal(t, "mixed", string(section.ID))
	require.Len(t, section.Field, 2)
	assert.Equal(t, core.Ref{ID: mnemonics["FIRST"]}, section.Field[0].Property)
	assert.Equal(t, core.Ref{ID: mnemonics["SECOND"]}, section.Field[1].Property)

	// First gets inner order 1, Second continues at 2.
	assert.Equal(t, core.Amount[float64]{Amount: 1, Precision: 1}, section.Field[0].OrderInList)
	assert.Equal(t, core.Amount[float64]{Amount: 2, Precision: 1}, section.Field[1].OrderInList)
}

func TestFieldsSectionWithEmbeddedAndOrder(t *testing.T) {
	t.Parallel()

	mnemonics := fieldsTestMnemonics()

	result, errE := transform.Fields[FieldsWithSectionEmbeddedOrder](mnemonics)
	require.NoError(t, errE, "% -+#.1v", errE)

	require.Len(t, result.Section, 1)
	require.Len(t, result.Field, 1)

	// Section "sec" with fields from embedded struct and direct fields.
	section := result.Section[0]
	assert.Equal(t, "sec", string(section.ID))
	require.Len(t, section.Field, 3)

	// First comes from EmbeddedInsideSection with explicit order 10.
	assert.Equal(t, core.Ref{ID: mnemonics["FIRST"]}, section.Field[0].Property)
	assert.Equal(t, core.Amount[float64]{Amount: 10, Precision: 1}, section.Field[0].OrderInList)

	// Second has auto-increment order 1 (section field counter, unaffected by explicit orders).
	assert.Equal(t, core.Ref{ID: mnemonics["SECOND"]}, section.Field[1].Property)
	assert.Equal(t, core.Amount[float64]{Amount: 1, Precision: 1}, section.Field[1].OrderInList)

	// Third has explicit order 5.
	assert.Equal(t, core.Ref{ID: mnemonics["THIRD"]}, section.Field[2].Property)
	assert.Equal(t, core.Amount[float64]{Amount: 5, Precision: 1}, section.Field[2].OrderInList)

	// Top-level Bar field.
	assert.Equal(t, core.Ref{ID: mnemonics["BAR"]}, result.Field[0].Property)
}
