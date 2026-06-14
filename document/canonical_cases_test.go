package document_test

import (
	_ "embed"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/peerdb/peerdb/document"
)

// canonicalCasesJSON is the shared claim-validation corpus run by both this Go backend test and
// the TypeScript frontend test (src/document/canonical-cases.test.ts). Each case fixes the
// canonical HTML and validity for an input, so the two implementations are held to the same
// contract: HTML claim validation has to agree between the backend and the editor.
//
//go:embed testdata/canonical-cases.json
var canonicalCasesJSON []byte

type canonicalCase struct {
	Name        string `json:"name"`
	Input       string `json:"input"`
	Canonical   string `json:"canonical"`
	Valid       bool   `json:"valid"`
	Recanonical string `json:"recanonical"`
}

func TestCanonicalCasesCorpus(t *testing.T) {
	t.Parallel()

	var corpus struct {
		Cases []canonicalCase `json:"cases"`
	}
	require.NoError(t, json.Unmarshal(canonicalCasesJSON, &corpus))
	require.NotEmpty(t, corpus.Cases)

	for _, c := range corpus.Cases {
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()

			canonical, errE := document.CanonicalizeHTML(c.Input)
			require.NoError(t, errE, "% -+#.1v", errE)
			assert.Equal(t, c.Canonical, canonical, "CanonicalizeHTML")

			valid, errE := document.IsCanonicalHTML(c.Input)
			require.NoError(t, errE, "% -+#.1v", errE)
			assert.Equal(t, c.Valid, valid, "IsCanonicalHTML")

			// The validity flag has to be consistent with whether canonicalization changed the input.
			assert.Equal(t, c.Input == c.Canonical, c.Valid, "valid flag matches input==canonical")

			// Re-canonicalizing the canonical form yields it again, unless the case records a
			// distinct recanonical form (canonical HTML which parses back into a different document).
			expectedRe := c.Canonical
			if c.Recanonical != "" {
				expectedRe = c.Recanonical
			}
			recanonical, errE := document.CanonicalizeHTML(c.Canonical)
			require.NoError(t, errE, "% -+#.1v", errE)
			assert.Equal(t, expectedRe, recanonical, "re-canonicalization")
		})
	}
}
