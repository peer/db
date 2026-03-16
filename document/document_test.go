package document_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/core"
	"gitlab.com/peerdb/peerdb/document"
)

func TestDocument(t *testing.T) {
	t.Parallel()

	doc := document.D{}
	assert.Equal(t, document.D{}, doc)

	id := identifier.New()

	errE := doc.Add(&document.NoneClaim{
		CoreClaim: document.CoreClaim{
			ID:         id,
			Confidence: 1.0,
		},
		Prop: document.GetReference(core.Namespace, "NAME"),
	})
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, document.D{ //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			None: document.NoneClaims{
				{
					CoreClaim: document.CoreClaim{
						ID:         id,
						Confidence: 1.0,
					},
					Prop: document.GetReference(core.Namespace, "NAME"),
				},
			},
		},
	}, doc)
	claim := doc.GetByID(id)
	assert.Equal(t, &document.NoneClaim{
		CoreClaim: document.CoreClaim{
			ID:         id,
			Confidence: 1.0,
		},
		Prop: document.GetReference(core.Namespace, "NAME"),
	}, claim)
	claims := doc.Get(identifier.From(core.Namespace, "NAME"))
	assert.Equal(t, []document.Claim{
		&document.NoneClaim{
			CoreClaim: document.CoreClaim{
				ID:         id,
				Confidence: 1.0,
			},
			Prop: document.GetReference(core.Namespace, "NAME"),
		},
	}, claims)
	claim = doc.RemoveByID(id)
	assert.Equal(t, &document.NoneClaim{
		CoreClaim: document.CoreClaim{
			ID:         id,
			Confidence: 1.0,
		},
		Prop: document.GetReference(core.Namespace, "NAME"),
	}, claim)
	assert.Equal(t, document.D{}, doc)

	id2 := identifier.New()

	errE = claim.Add(&document.UnknownClaim{
		CoreClaim: document.CoreClaim{
			ID:         id2,
			Confidence: 1.0,
		},
		Prop: document.GetReference(core.Namespace, "NAME"),
	})
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, &document.NoneClaim{
		CoreClaim: document.CoreClaim{
			ID:         id,
			Confidence: 1.0,
			Meta: &document.ClaimTypes{
				Unknown: document.UnknownClaims{
					{
						CoreClaim: document.CoreClaim{
							ID:         id2,
							Confidence: 1.0,
						},
						Prop: document.GetReference(core.Namespace, "NAME"),
					},
				},
			},
		},
		Prop: document.GetReference(core.Namespace, "NAME"),
	}, claim)
	metaClaim := claim.GetByID(id2)
	assert.Equal(t, &document.UnknownClaim{
		CoreClaim: document.CoreClaim{
			ID:         id2,
			Confidence: 1.0,
		},
		Prop: document.GetReference(core.Namespace, "NAME"),
	}, metaClaim)
	metaClaim = claim.RemoveByID(id2)
	assert.Equal(t, &document.UnknownClaim{
		CoreClaim: document.CoreClaim{
			ID:         id2,
			Confidence: 1.0,
		},
		Prop: document.GetReference(core.Namespace, "NAME"),
	}, metaClaim)
	assert.Equal(t, &document.NoneClaim{
		CoreClaim: document.CoreClaim{
			ID:         id,
			Confidence: 1.0,
		},
		Prop: document.GetReference(core.Namespace, "NAME"),
	}, claim)
}
