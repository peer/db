package document_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/peerdb/peerdb/document"
)

// TestCanonicalizeHTMLBehavior pins how PeerDB calls CanonicalizeHTML/IsCanonicalHTML: with
// whitespace preservation (PreserveWhitespaceTrue), so the editor can store HTML faithfully. Runs
// of spaces and leading/trailing spaces are kept and are canonical; newlines collapse to spaces;
// preformatted content keeps everything; and disallowed content is still stripped. Canonicalization
// stays idempotent (one pass reaches a fixed point), which IsCanonicalHTML relies on.
func TestCanonicalizeHTMLBehavior(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		input     string
		canonical string
		valid     bool
	}{
		// Whitespace is preserved (the point of the preserve option).
		{"double space preserved", "<p>a  b</p>", "<p>a  b</p>", true},
		{"triple space preserved", "<p>a   b</p>", "<p>a   b</p>", true},
		{"leading space preserved", "<p> a</p>", "<p> a</p>", true},
		{"trailing space preserved", "<p>a </p>", "<p>a </p>", true},
		{"space after hard break preserved", "<p>a<br> b</p>", "<p>a<br> b</p>", true},
		{"non-breaking space preserved", "<p>a\u00a0b</p>", "<p>a\u00a0b</p>", true},
		// Newlines collapse to spaces (no linebreak replacement node configured).
		{"newline becomes space", "<p>a\nb</p>", "<p>a b</p>", false},
		// Each newline becomes a space and the resulting run is preserved (preserve mode does not
		// collapse), so two newlines and two spaces become four spaces.
		{"newline run becomes spaces", "<p>a\n\n  b</p>", "<p>a    b</p>", false},
		// Preformatted keeps all whitespace including newlines, idempotently.
		{"pre keeps double space", "<pre>a  b</pre>", "<pre>a  b</pre>", true},
		{"pre keeps newline", "<pre>a\nb</pre>", "<pre>a\nb</pre>", true},
		// Sanitization is unchanged: disallowed content is stripped.
		{"script stripped", "<p>x</p><script>alert(1)</script>", "<p>x</p>", false},
		{"image stripped", "<p>a</p><img src=\"/x.png\" alt=\"y\"><p>b</p>", "<p>a</p><p>b</p>", false},
		{"javascript href dropped", "<p><a href=\"javascript:alert(1)\">x</a></p>", "<p>x</p>", false},
		{"uppercase tag normalized", "<P>x</P>", "<p>x</p>", false},
		// Empty and whitespace-only inputs canonicalize to the empty document.
		{"empty input", "", "<p></p>", false},
		{"whitespace-only input", "   ", "<p></p>", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			canonical, errE := document.CanonicalizeHTML(c.input)
			require.NoError(t, errE, "% -+#.1v", errE)
			assert.Equal(t, c.canonical, canonical, "CanonicalizeHTML")

			valid, errE := document.IsCanonicalHTML(c.input)
			require.NoError(t, errE, "% -+#.1v", errE)
			assert.Equal(t, c.valid, valid, "IsCanonicalHTML")
			assert.Equal(t, c.input == c.canonical, c.valid, "valid flag matches input==canonical")

			// Canonicalization is idempotent: the canonical form is itself a fixed point.
			recanonical, errE := document.CanonicalizeHTML(canonical)
			require.NoError(t, errE, "% -+#.1v", errE)
			assert.Equal(t, canonical, recanonical, "canonical form is a fixed point")
		})
	}
}
