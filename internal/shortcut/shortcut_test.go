package shortcut_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/internal/shortcut"
)

// id builds an identifier segment from hashed base parts, idLit builds one from an already-resolved
// identifier, literal builds a literal segment, and sl is a terse []Segment constructor, for compact
// expectations.
func id(parts ...string) shortcut.Segment {
	return shortcut.Segment{Path: []identifier.Identifier{identifier.From(parts...)}, Literal: ""}
}

func idLit(i identifier.Identifier) shortcut.Segment {
	return shortcut.Segment{Path: []identifier.Identifier{i}, Literal: ""}
}

func literal(value string) shortcut.Segment {
	return shortcut.Segment{Path: nil, Literal: value}
}

func sl(segments ...shortcut.Segment) []shortcut.Segment {
	return segments
}

func TestParseEntry(t *testing.T) {
	t.Parallel()

	base58ID := identifier.New()
	base58 := base58ID.String()

	for _, tt := range []struct {
		name  string
		entry string
		want  shortcut.Entry
		errIs string
	}{
		{"simple", "ns,A=ns,B", shortcut.Entry{Key: sl(id("ns", "A")), Value: sl(id("ns", "B"))}, ""},
		{"value path", "ns,A=ns,B:ns,C", shortcut.Entry{Key: sl(id("ns", "A")), Value: sl(id("ns", "B"), id("ns", "C"))}, ""},
		{"nested key", "ns,A:ns,B=ns,C", shortcut.Entry{Key: sl(id("ns", "A"), id("ns", "B")), Value: sl(id("ns", "C"))}, ""},
		{"literal value", "ns,A=self", shortcut.Entry{Key: sl(id("ns", "A")), Value: sl(literal("self"))}, ""},
		{"literal key", "reverse=ns,A", shortcut.Entry{Key: sl(literal("reverse")), Value: sl(id("ns", "A"))}, ""},
		{"base58 identifier", base58 + "=ns,A", shortcut.Entry{Key: sl(idLit(base58ID)), Value: sl(id("ns", "A"))}, ""},
		{"missing equals", "ns,A", shortcut.Entry{}, "entry must have a non-empty key and value separated by '='"},
		{"empty value", "ns,A=", shortcut.Entry{}, "entry must have a non-empty key and value separated by '='"},
		{"empty key", "=ns,A", shortcut.Entry{}, "entry must have a non-empty key and value separated by '='"},
		{"empty", "", shortcut.Entry{}, "entry must have a non-empty key and value separated by '='"},
		{"empty part in identifier", "ns,A=ns,,B", shortcut.Entry{}, "empty identifier part"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, errE := shortcut.ParseEntry(tt.entry)
			if tt.errIs != "" {
				assert.EqualError(t, errE, tt.errIs)
				return
			}
			require.NoError(t, errE, "% -+#.1v", errE)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParse(t *testing.T) {
	t.Parallel()

	got, errE := shortcut.Parse("ns,A=ns,B&ns,C=ns,D")
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []shortcut.Entry{
		{Key: sl(id("ns", "A")), Value: sl(id("ns", "B"))},
		{Key: sl(id("ns", "C")), Value: sl(id("ns", "D"))},
	}, got)

	_, errE = shortcut.Parse("ns,A=ns,B&ns,,C=ns,D")
	assert.EqualError(t, errE, "empty identifier part")
}

func TestSegmentMethods(t *testing.T) {
	t.Parallel()

	identifierSegment := id("ns", "A")
	assert.True(t, identifierSegment.IsIdentifier())
	assert.Equal(t, identifier.From("ns", "A"), identifierSegment.Identifier())

	assert.False(t, literal("self").IsIdentifier())
}
