package document_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/core"
	"gitlab.com/peerdb/peerdb/document"
	internalCore "gitlab.com/peerdb/peerdb/internal/core"
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
	claims := doc.Get(internalCore.NamePropID)
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

// TestDocumentSizeWithSub compares the shallow Size/AllClaims with the recursive
// SizeWithSub/AllClaimsWithSub on a document whose claims carry sub-claims.
func TestDocumentSizeWithSub(t *testing.T) {
	t.Parallel()

	prop := identifier.New()

	// One top-level String claim carrying two sub-claims, the first of which
	// carries a further sub-claim: 1 + 2 + 1 = 4 claims, only 1 at the top level.
	deepSub := &document.StringClaim{
		CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1.0},
		Prop:      document.Reference{ID: prop},
		String:    "deep",
	}
	sub1 := &document.StringClaim{
		CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1.0},
		Prop:      document.Reference{ID: prop},
		String:    "sub1",
	}
	require.NoError(t, sub1.Add(deepSub))
	sub2 := &document.UnknownClaim{
		CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1.0},
		Prop:      document.Reference{ID: prop},
	}
	top := &document.StringClaim{
		CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1.0},
		Prop:      document.Reference{ID: prop},
		String:    "top",
	}
	require.NoError(t, top.Add(sub1))
	require.NoError(t, top.Add(sub2))

	doc := &document.D{}
	require.NoError(t, doc.Add(top))

	// Shallow: only the single top-level claim.
	assert.Equal(t, 1, doc.Size())
	assert.Len(t, slices.Collect(doc.AllClaims()), 1)

	// Recursive: top + sub1 + deepSub + sub2.
	assert.Equal(t, 4, doc.SizeWithSub())
	assert.Len(t, slices.Collect(doc.AllClaimsWithSub()), 4)
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

func TestGetFallbackLanguages(t *testing.T) {
	t.Parallel()

	// With priority set.
	priority := map[string][]string{
		"en": {"sl", "und"},
		"sl": {"en"},
		"pt": {},
	}

	// Language with explicit fallbacks.
	assert.Equal(t, []string{"sl", "und"}, document.TestingGetFallbackLanguages("en", priority))
	assert.Equal(t, []string{"en"}, document.TestingGetFallbackLanguages("sl", priority))

	// Language with empty fallback list: no fallback at all.
	assert.Empty(t, document.TestingGetFallbackLanguages("pt", priority))

	// Language not in priority: fallback to "und".
	assert.Equal(t, []string{"und"}, document.TestingGetFallbackLanguages("fr", priority))

	// "und" not in priority: no fallback (it's already undetermined).
	assert.Nil(t, document.TestingGetFallbackLanguages("und", priority))

	// With nil priority.
	assert.Equal(t, []string{"und"}, document.TestingGetFallbackLanguages("en", nil))
	assert.Nil(t, document.TestingGetFallbackLanguages("und", nil))
}

func TestIsRecognizedLanguage(t *testing.T) {
	t.Parallel()

	// "en" and "sl" are keys; "und" appears only as a fallback target.
	priority := map[string][]string{
		"en": {"sl", "und"},
		"sl": {"en"},
	}

	// Keys are recognized.
	assert.True(t, document.TestingIsRecognizedLanguage("en", priority))
	assert.True(t, document.TestingIsRecognizedLanguage("sl", priority))

	// A fallback target that is not a key is still recognized.
	assert.True(t, document.TestingIsRecognizedLanguage("und", priority))

	// A language that is neither a key nor a fallback target is not recognized.
	assert.False(t, document.TestingIsRecognizedLanguage("pt", priority))

	// Nothing is recognized in an empty priority.
	assert.False(t, document.TestingIsRecognizedLanguage("en", nil))
	assert.False(t, document.TestingIsRecognizedLanguage("en", map[string][]string{}))
}

// TestGetClaimsOfType tests the generic GetClaimsOfType function.
func TestGetClaimsOfType(t *testing.T) {
	t.Parallel()

	prop := identifier.New()
	id1 := identifier.New()
	id2 := identifier.New()
	id3 := identifier.New()

	ct := &document.ClaimTypes{
		String: document.StringClaims{
			{
				CoreClaim: document.CoreClaim{ID: id1, Confidence: 0.5},
				Prop:      document.Reference{ID: prop},
				String:    "s1",
			},
			{
				CoreClaim: document.CoreClaim{ID: id2, Confidence: 1.0},
				Prop:      document.Reference{ID: prop},
				String:    "s2",
			},
		},
		None: document.NoneClaims{
			{
				CoreClaim: document.CoreClaim{ID: id3, Confidence: 0.75},
				Prop:      document.Reference{ID: prop},
			},
		},
	}

	// Get only StringClaims for prop.
	strings := document.TestingGetClaimsOfType[document.StringClaim](ct, prop)
	require.Len(t, strings, 2)
	assert.Equal(t, "s2", strings[0].String) // Higher confidence first.
	assert.Equal(t, "s1", strings[1].String)

	// Get NoneClaims for prop.
	nones := document.TestingGetClaimsOfType[document.NoneClaim](ct, prop)
	require.Len(t, nones, 1)

	// No AmountClaims for prop.
	amounts := document.TestingGetClaimsOfType[document.AmountClaim](ct, prop)
	assert.Empty(t, amounts)
}

// TestGetAllClaimsOfType tests GetAllClaimsOfType returns claims of the requested type sorted by decreasing confidence.
func TestGetAllClaimsOfType(t *testing.T) {
	t.Parallel()

	prop1 := identifier.New()
	prop2 := identifier.New()

	doc := &document.D{}

	// Empty document returns nil.
	assert.Nil(t, document.TestingGetAllClaimsOfType[document.StringClaim](doc))

	// Add string claims on different properties with varying confidence.
	errE := doc.Add(&document.StringClaim{
		CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 0.5},
		Prop:      document.Reference{ID: prop1},
		String:    "low",
	})
	require.NoError(t, errE, "% -+#.1v", errE)
	errE = doc.Add(&document.StringClaim{
		CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1.0},
		Prop:      document.Reference{ID: prop2},
		String:    "high",
	})
	require.NoError(t, errE, "% -+#.1v", errE)
	errE = doc.Add(&document.StringClaim{
		CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 0.75},
		Prop:      document.Reference{ID: prop1},
		String:    "medium",
	})
	require.NoError(t, errE, "% -+#.1v", errE)

	// Add a different claim type to verify type filtering.
	errE = doc.Add(&document.HTMLClaim{
		CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1.0},
		Prop:      document.Reference{ID: prop1},
		HTML:      "<p>html</p>",
	})
	require.NoError(t, errE, "% -+#.1v", errE)

	// GetAllClaimsOfType returns only string claims, across all properties.
	strings := document.TestingGetAllClaimsOfType[document.StringClaim](doc)
	assert.Len(t, strings, 3)

	// Sorted by decreasing confidence.
	assert.Equal(t, "high", strings[0].String)
	assert.Equal(t, "medium", strings[1].String)
	assert.Equal(t, "low", strings[2].String)

	// GetAllClaimsOfType for HTML returns only the HTML claim.
	htmls := document.TestingGetAllClaimsOfType[document.HTMLClaim](doc)
	assert.Len(t, htmls, 1)
	assert.Equal(t, "<p>html</p>", htmls[0].HTML)

	// A type with no claims returns nil.
	assert.Nil(t, document.TestingGetAllClaimsOfType[document.ReferenceClaim](doc))
}

// TestGetAllClaimsOfTypeNilClaims tests GetAllClaimsOfType on a nil ClaimTypes.
func TestGetAllClaimsOfTypeNilClaims(t *testing.T) {
	t.Parallel()

	var claims *document.ClaimTypes
	assert.Nil(t, document.TestingGetAllClaimsOfType[document.StringClaim](claims))
}

// TestGetAllClaimsOfTypeWithConfidence tests GetAllClaimsOfTypeWithConfidence filters by confidence threshold.
func TestGetAllClaimsOfTypeWithConfidence(t *testing.T) {
	t.Parallel()

	prop1 := identifier.New()
	prop2 := identifier.New()

	doc := &document.D{}

	// Empty document returns empty.
	assert.Empty(t, document.GetAllClaimsOfTypeWithConfidence[document.StringClaim](doc, document.LowConfidence))

	errE := doc.Add(&document.StringClaim{
		CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 0.3},
		Prop:      document.Reference{ID: prop1},
		String:    "below-low",
	})
	require.NoError(t, errE, "% -+#.1v", errE)
	errE = doc.Add(&document.StringClaim{
		CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 0.5},
		Prop:      document.Reference{ID: prop2},
		String:    "at-low",
	})
	require.NoError(t, errE, "% -+#.1v", errE)
	errE = doc.Add(&document.StringClaim{
		CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 0.75},
		Prop:      document.Reference{ID: prop1},
		String:    "at-medium",
	})
	require.NoError(t, errE, "% -+#.1v", errE)
	errE = doc.Add(&document.StringClaim{
		CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1.0},
		Prop:      document.Reference{ID: prop2},
		String:    "at-high",
	})
	require.NoError(t, errE, "% -+#.1v", errE)

	// Add a different type to verify filtering.
	errE = doc.Add(&document.HTMLClaim{
		CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1.0},
		Prop:      document.Reference{ID: prop1},
		HTML:      "<p>html</p>",
	})
	require.NoError(t, errE, "% -+#.1v", errE)

	// LowConfidence (0.5) filters out below-low (0.3).
	low := document.GetAllClaimsOfTypeWithConfidence[document.StringClaim](doc, document.LowConfidence)
	assert.Len(t, low, 3)
	assert.Equal(t, "at-high", low[0].String)
	assert.Equal(t, "at-medium", low[1].String)
	assert.Equal(t, "at-low", low[2].String)

	// MediumConfidence (0.75) filters out at-low and below-low.
	medium := document.GetAllClaimsOfTypeWithConfidence[document.StringClaim](doc, document.MediumConfidence)
	assert.Len(t, medium, 2)
	assert.Equal(t, "at-high", medium[0].String)
	assert.Equal(t, "at-medium", medium[1].String)

	// HighConfidence (1.0) keeps only at-high.
	high := document.GetAllClaimsOfTypeWithConfidence[document.StringClaim](doc, document.HighConfidence)
	assert.Len(t, high, 1)
	assert.Equal(t, "at-high", high[0].String)

	// Zero confidence defaults to LowConfidence.
	zero := document.GetAllClaimsOfTypeWithConfidence[document.StringClaim](doc, 0)
	assert.Equal(t, low, zero)
}

// TestGetAllClaimsOfTypeWithConfidenceNilClaims tests GetAllClaimsOfTypeWithConfidence on a nil ClaimTypes.
func TestGetAllClaimsOfTypeWithConfidenceNilClaims(t *testing.T) {
	t.Parallel()

	var claims *document.ClaimTypes
	assert.Empty(t, document.GetAllClaimsOfTypeWithConfidence[document.ReferenceClaim](claims, document.LowConfidence))
}
