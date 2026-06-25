package core_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	internalCore "gitlab.com/peerdb/peerdb/internal/core"
)

// TestParseCardinalityTag covers the full cardinality-tag grammar:
// "N", "N..", "N..M", "0..", "1..1". An empty tag yields the permissive
// default (0, -1). Malformed inputs return a non-nil error.
func TestParseCardinalityTag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		tag     string
		wantMin int
		wantMax int
	}{
		{"empty defaults to 0..unbounded", "", 0, -1},
		{"exactly one", "1", 1, 1},
		{"exactly five", "5", 5, 5},
		{"zero or one", "0..1", 0, 1},
		{"zero or more", "0..", 0, -1},
		{"one or more", "1..", 1, -1},
		{"two to five", "2..5", 2, 5},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			minC, maxC, errE := internalCore.ParseCardinalityTag(tc.tag)
			require.NoError(t, errE, "% -+#.1v", errE)
			assert.Equal(t, tc.wantMin, minC)
			assert.Equal(t, tc.wantMax, maxC)
		})
	}
}

// TestParseCardinalityTag_Invalid verifies that malformed tags return errors
// rather than silently falling back.
func TestParseCardinalityTag_Invalid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tag  string
	}{
		{"non-numeric", "abc"},
		{"non-numeric min", "abc..5"},
		{"non-numeric max", "1..abc"},
		{"empty min", "..5"},
		{"zero exact", "0"},
		{"negative exact", "-1"},
		{"zero max", "1..0"},
		{"negative max", "1..-1"},
		{"max less than min", "5..2"},
		{"too many dots", "1..2..3"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, _, errE := internalCore.ParseCardinalityTag(tc.tag)
			assert.Error(t, errE, "expected error for %q", tc.tag)
		})
	}
}
