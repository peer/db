package document_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/document"
)

// TestVisitorStopBehavior tests that visitor stopping works across all claim types.
func TestVisitorStopBehavior(t *testing.T) {
	t.Parallel()

	prop := identifier.New()
	idStr := identifier.New()
	idHTML := identifier.New()

	ct := &document.ClaimTypes{
		String: document.StringClaims{
			{
				CoreClaim: document.CoreClaim{ID: idStr, Confidence: 1.0},
				Prop:      document.Reference{ID: prop},
				String:    "s1",
			},
		},
		HTML: document.HTMLClaims{
			{
				CoreClaim: document.CoreClaim{ID: idHTML, Confidence: 1.0},
				Prop:      document.Reference{ID: prop},
				HTML:      "<p>h1</p>",
			},
		},
	}

	// GetByID finds the string claim and stops before visiting HTML claims.
	found := ct.GetByID(idStr)
	require.NotNil(t, found)
	assert.Equal(t, idStr, found.GetID())

	// RemoveByID removes HTML claim.
	removed := ct.RemoveByID(idHTML)
	require.NotNil(t, removed)
	assert.Equal(t, idHTML, removed.GetID())
	assert.Equal(t, 1, ct.Size())
}
