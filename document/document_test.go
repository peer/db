package document_test

import (
	"slices"
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
			Sub: &document.ClaimTypes{
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
	subClaim := claim.GetByID(id2)
	assert.Equal(t, &document.UnknownClaim{
		CoreClaim: document.CoreClaim{
			ID:         id2,
			Confidence: 1.0,
		},
		Prop: document.GetReference(core.Namespace, "NAME"),
	}, subClaim)
	subClaim = claim.RemoveByID(id2)
	assert.Equal(t, &document.UnknownClaim{
		CoreClaim: document.CoreClaim{
			ID:         id2,
			Confidence: 1.0,
		},
		Prop: document.GetReference(core.Namespace, "NAME"),
	}, subClaim)
	assert.Equal(t, &document.NoneClaim{
		CoreClaim: document.CoreClaim{
			ID:         id,
			Confidence: 1.0,
		},
		Prop: document.GetReference(core.Namespace, "NAME"),
	}, claim)
}

// TestDocumentValidate tests D.Validate.
func TestDocumentValidate(t *testing.T) {
	t.Parallel()

	t.Run("valid_no_claims", func(t *testing.T) {
		t.Parallel()

		base := []string{"TqtRsbk7rTKviW3TJapTim"}
		doc := document.D{
			CoreDocument: document.CoreDocument{
				ID:   identifier.From(base...),
				Base: base,
			},
		}
		errE := doc.Validate()
		require.NoError(t, errE, "% -+#.1v", errE)
	})

	t.Run("valid_with_claims", func(t *testing.T) {
		t.Parallel()

		base := []string{"TqtRsbk7rTKviW3TJapTim"}
		prop := identifier.New()
		doc := document.D{
			CoreDocument: document.CoreDocument{
				ID:   identifier.From(base...),
				Base: base,
			},
			Claims: &document.ClaimTypes{
				String: document.StringClaims{
					{
						CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1.0},
						Prop:      document.Reference{ID: prop},
						String:    "hello",
					},
				},
			},
		}
		errE := doc.Validate()
		require.NoError(t, errE, "% -+#.1v", errE)
	})

	t.Run("invalid_core_document", func(t *testing.T) {
		t.Parallel()

		base := []string{"TqtRsbk7rTKviW3TJapTim"}
		doc := document.D{
			CoreDocument: document.CoreDocument{
				ID:   identifier.New(),
				Base: base,
			},
		}
		errE := doc.Validate()
		assert.EqualError(t, errE, "invalid ID")
	})

	t.Run("invalid_claim", func(t *testing.T) {
		t.Parallel()

		base := []string{"TqtRsbk7rTKviW3TJapTim"}
		prop := identifier.New()
		doc := document.D{
			CoreDocument: document.CoreDocument{
				ID:   identifier.From(base...),
				Base: base,
			},
			Claims: &document.ClaimTypes{
				String: document.StringClaims{
					{
						CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1.0},
						Prop:      document.Reference{ID: prop},
						String:    "",
					},
				},
			},
		}
		errE := doc.Validate()
		assert.EqualError(t, errE, "empty string")
	})
}

// TestDocumentReference tests D.Reference.
func TestDocumentReference(t *testing.T) {
	t.Parallel()

	base := []string{"testdoc"}
	id := identifier.From(base...)
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: id, Base: base},
	}
	ref := doc.Reference()
	assert.Equal(t, document.Reference{ID: id}, ref)
}

// TestDocumentRemove tests D.Remove.
func TestDocumentRemove(t *testing.T) {
	t.Parallel()

	prop := identifier.New()
	doc := &document.D{}

	errE := doc.Add(&document.StringClaim{
		CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1.0},
		Prop:      document.Reference{ID: prop},
		String:    "first",
	})
	require.NoError(t, errE, "% -+#.1v", errE)
	errE = doc.Add(&document.StringClaim{
		CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1.0},
		Prop:      document.Reference{ID: prop},
		String:    "second",
	})
	require.NoError(t, errE, "% -+#.1v", errE)

	removed := doc.Remove(prop)
	assert.Len(t, removed, 2)
	assert.Equal(t, 0, doc.Size())
	// Claims is set to nil when all claims are removed.
	assert.Nil(t, doc.Claims)
}

// TestDocumentSizeAllClaims tests D.Size and D.AllClaims on an empty and non-empty document.
func TestDocumentSizeAllClaims(t *testing.T) {
	t.Parallel()

	prop := identifier.New()
	doc := &document.D{}

	assert.Equal(t, 0, doc.Size())
	assert.Empty(t, slices.Collect(doc.AllClaims()))

	errE := doc.Add(&document.StringClaim{
		CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1.0},
		Prop:      document.Reference{ID: prop},
		String:    "test",
	})
	require.NoError(t, errE, "% -+#.1v", errE)
	errE = doc.Add(&document.HTMLClaim{
		CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1.0},
		Prop:      document.Reference{ID: prop},
		HTML:      "<p>test</p>",
	})
	require.NoError(t, errE, "% -+#.1v", errE)

	assert.Equal(t, 2, doc.Size())
	assert.Len(t, slices.Collect(doc.AllClaims()), 2)
}

// TestDocumentMergeFrom tests D.MergeFrom merging claims from multiple documents.
func TestDocumentMergeFrom(t *testing.T) {
	t.Parallel()

	prop := identifier.New()

	doc1 := &document.D{}
	errE := doc1.Add(&document.StringClaim{
		CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1.0},
		Prop:      document.Reference{ID: prop},
		String:    "from doc1",
	})
	require.NoError(t, errE, "% -+#.1v", errE)

	doc2 := &document.D{}
	errE = doc2.Add(&document.HTMLClaim{
		CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1.0},
		Prop:      document.Reference{ID: prop},
		HTML:      "<p>from doc2</p>",
	})
	require.NoError(t, errE, "% -+#.1v", errE)

	target := &document.D{}
	errE = target.MergeFrom(doc1, doc2)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, 2, target.Size())
}

// TestDocumentMergeFromDuplicateID tests D.MergeFrom returns error on duplicate claim ID.
func TestDocumentMergeFromDuplicateID(t *testing.T) {
	t.Parallel()

	prop := identifier.New()
	id := identifier.New()

	claimA := &document.StringClaim{
		CoreClaim: document.CoreClaim{ID: id, Confidence: 1.0},
		Prop:      document.Reference{ID: prop},
		String:    "in A",
	}
	claimB := &document.StringClaim{
		CoreClaim: document.CoreClaim{ID: id, Confidence: 0.5},
		Prop:      document.Reference{ID: prop},
		String:    "in B (same ID)",
	}

	doc1 := &document.D{}
	errE := doc1.Add(claimA)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc2 := &document.D{}
	errE = doc2.Add(claimB)
	require.NoError(t, errE, "% -+#.1v", errE)

	target := &document.D{}
	errE = target.MergeFrom(doc1)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Merging doc2 should fail because of duplicate claim ID.
	errE = target.MergeFrom(doc2)
	assert.EqualError(t, errE, "claim with ID already exists")
}

// TestDocumentGetByIDInMeta tests that GetByID searches inside claim metadata.
func TestDocumentGetByIDInMeta(t *testing.T) {
	t.Parallel()

	prop := identifier.New()
	outerID := identifier.New()
	innerID := identifier.New()

	outerClaim := &document.NoneClaim{
		CoreClaim: document.CoreClaim{ID: outerID, Confidence: 1.0},
		Prop:      document.Reference{ID: prop},
	}

	innerClaim := &document.StringClaim{
		CoreClaim: document.CoreClaim{ID: innerID, Confidence: 1.0},
		Prop:      document.Reference{ID: prop},
		String:    "metadata",
	}

	errE := outerClaim.Add(innerClaim)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc := &document.D{}
	errE = doc.Add(outerClaim)
	require.NoError(t, errE, "% -+#.1v", errE)

	// GetByID should find the inner claim inside the outer claim's metadata.
	result := doc.GetByID(innerID)
	assert.Equal(t, innerClaim, result)
}
