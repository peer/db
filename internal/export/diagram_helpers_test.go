package export_test

import (
	"bytes"
	"reflect"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/core"
	"gitlab.com/peerdb/peerdb/internal/export"
)

// TestClassifyDiagramValueType verifies the Go-type to value-type-label
// mapping. The boolean second return value reports whether the row is a
// reference and should get the FK flag.
func TestClassifyDiagramValueType(t *testing.T) {
	t.Parallel()

	type valueRefWrapper struct {
		Value core.Ref `value:""`
	}
	type valueStringWrapper struct {
		Value string `value:""`
	}
	type containerStruct struct {
		Name []core.StringWithLanguage `property:"NAME"`
	}

	tests := []struct {
		name      string
		typ       reflect.Type
		tag       string
		wantLabel string
		wantIsRef bool
	}{
		{"core.Ref", reflect.TypeFor[core.Ref](), "", "reference", true},
		{"slice of Ref", reflect.TypeFor[[]core.Ref](), "", "reference", true},
		{"pointer to Ref", reflect.TypeFor[*core.Ref](), "", "reference", true},
		{"core.Time", reflect.TypeFor[core.Time](), "", "time", false},
		{"time.Time", reflect.TypeFor[time.Time](), "", "time", false},
		{"core.Interval[Time]", reflect.TypeFor[core.Interval[core.Time]](), "", "time_interval", false},
		{"core.Identifier", reflect.TypeFor[core.Identifier](), "", "identifier", false},
		{"core.Link", reflect.TypeFor[core.Link](), "", "link", false},
		{"core.File", reflect.TypeFor[core.File](), "", "file", false},
		{"core.HTML", reflect.TypeFor[core.HTML](), "", "html", false},
		{"core.RawHTML", reflect.TypeFor[core.RawHTML](), "", "html", false},
		{"core.None", reflect.TypeFor[core.None](), "", "none", false},
		{"core.Unknown", reflect.TypeFor[core.Unknown](), "", "unknown", false},
		{"core.Amount[int]", reflect.TypeFor[core.Amount[int]](), "", "amount", false},
		{"core.Amount[float64]", reflect.TypeFor[core.Amount[float64]](), "", "amount", false},
		{"core.Interval[Amount[int]]", reflect.TypeFor[core.Interval[core.Amount[int]]](), "", "amount_interval", false},
		{"plain string", reflect.TypeFor[string](), "", "string", false},
		{"string + type:id", reflect.TypeFor[string](), "id", "identifier", false},
		{"string + type:html", reflect.TypeFor[string](), "html", "html", false},
		{"string + type:rawhtml", reflect.TypeFor[string](), "rawhtml", "html", false},
		{"string + type:link", reflect.TypeFor[string](), "link", "link", false},
		{"string + type:file", reflect.TypeFor[string](), "file", "file", false},
		{"int", reflect.TypeFor[int](), "", "amount", false},
		{"int64", reflect.TypeFor[int64](), "", "amount", false},
		{"uint32", reflect.TypeFor[uint32](), "", "amount", false},
		{"float32", reflect.TypeFor[float32](), "", "amount", false},
		{"bool", reflect.TypeFor[bool](), "", "has", false},
		{"bool + type:none", reflect.TypeFor[bool](), "none", "none", false},
		{"bool + type:unknown", reflect.TypeFor[bool](), "unknown", "unknown", false},
		{"struct with Ref value:\"\"", reflect.TypeFor[valueRefWrapper](), "", "reference", true},
		{"struct with string value:\"\"", reflect.TypeFor[valueStringWrapper](), "", "string", false},
		{"struct with no value:\"\" (HAS claim)", reflect.TypeFor[containerStruct](), "", "has", false},
		{"slice of struct with Ref value:\"\"", reflect.TypeFor[[]valueRefWrapper](), "", "reference", true},
		{"pointer to struct with string value:\"\"", reflect.TypeFor[*valueStringWrapper](), "", "string", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			label, isRef := export.TestingClassifyDiagramValueType(zerolog.Nop(), tc.typ, tc.tag)
			assert.Equal(t, tc.wantLabel, label)
			assert.Equal(t, tc.wantIsRef, isRef)
		})
	}
}

// TestCardinalityRightSymbol verifies the (min, max) -> Mermaid right-side
// cardinality symbol mapping.
func TestCardinalityRightSymbol(t *testing.T) {
	t.Parallel()
	tests := []struct {
		minC int
		maxC int
		want string
	}{
		{0, 1, "o|"},
		{1, 1, "||"},
		{0, -1, "o{"},
		{1, -1, "|{"},
		{0, 5, "o{"},
		{2, 5, "|{"},
	}
	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			t.Parallel()
			got := export.TestingCardinalityRightSymbol(tc.minC, tc.maxC)
			assert.Equal(t, tc.want, got, "(%d, %d)", tc.minC, tc.maxC)
		})
	}
}

// TestCardinalityLabel verifies the (min, max) -> "min..max" comment rendering.
func TestCardinalityLabel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		minC int
		maxC int
		want string
	}{
		{0, -1, "0..*"},
		{1, -1, "1..*"},
		{0, 1, "0..1"},
		{1, 1, "1"},
		{5, 5, "5"},
		{2, 5, "2..5"},
	}
	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			t.Parallel()
			got := export.TestingCardinalityLabel(tc.minC, tc.maxC)
			assert.Equal(t, tc.want, got, "(%d, %d)", tc.minC, tc.maxC)
		})
	}
}

// TestResolveDiagramShortcutID verifies the shortcut-grammar identifier
// resolver: 22-character base58 IDs, comma-separated base parts, and the
// reserved "self" / "reverse" / nested-key sentinels.
func TestResolveDiagramShortcutID(t *testing.T) {
	t.Parallel()

	// A real, well-formed 22-character base58 identifier.
	knownID := identifier.New()
	knownIDStr := knownID.String()

	tests := []struct {
		name   string
		token  string
		wantID identifier.Identifier
		wantOK bool
	}{
		{"empty token", "", identifier.Identifier{}, false},
		{"self sentinel", "self", identifier.Identifier{}, false},
		{"reverse sentinel", "reverse", identifier.Identifier{}, false},
		{"nested parent:prop key", "core.peerdb.org,X:foo", identifier.Identifier{}, false},
		{"comma-separated parts", "core.peerdb.org,UNIT", identifier.From("core.peerdb.org", "UNIT"), true},
		{"comma list with empty part", "core.peerdb.org,,UNIT", identifier.Identifier{}, false},
		{"22-char base58 id", knownIDStr, knownID, true},
		{"malformed bare token", "not-an-id", identifier.Identifier{}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			id, ok := export.TestingResolveDiagramShortcutID(tc.token)
			assert.Equal(t, tc.wantOK, ok)
			if tc.wantOK {
				assert.Equal(t, tc.wantID, id)
			}
		})
	}
}

// TestClassifyDiagramValueType_UnclassifiedFallbackLogs verifies a warning is
// emitted when classifyDiagramValueType cannot match the field type, and that
// the fallback label is "N/A". We use reflect.TypeFor[chan int]() because
// channels are intentionally unsupported.
func TestClassifyDiagramValueType_UnclassifiedFallbackLogs(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := zerolog.New(&buf)
	label, isRef := export.TestingClassifyDiagramValueType(logger, reflect.TypeFor[chan int](), "")
	assert.Equal(t, "N/A", label)
	assert.False(t, isRef)
	assert.Equal( //nolint:testifylint
		t,
		`{"level":"warn","type":"chan int","typeTag":"","message":"unable to classify field type; falling back to N/A"}`+"\n",
		buf.String(),
	)
}
