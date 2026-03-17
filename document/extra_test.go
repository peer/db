package document_test

import (
	"context"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/document"
)

// customChange is a custom Change type for testing unsupported type errors.
type customChange struct{}

func (c customChange) Apply(_ *document.D) errors.E                             { return nil }
func (c customChange) Validate(_ context.Context, _ []string, _ int64) errors.E { return nil }

// customPatch is a custom ClaimPatch type for testing unsupported type errors.
type customPatch struct{}

func (p customPatch) New(_ []string) (document.Claim, errors.E) { return nil, nil } //nolint:ireturn,nilnil
func (p customPatch) Apply(_ document.Claim) errors.E           { return nil }

// TestCoreDocumentGetID tests CoreDocument.GetID.
func TestCoreDocumentGetID(t *testing.T) {
	t.Parallel()

	id := identifier.New()
	cd := document.CoreDocument{ID: id}
	assert.Equal(t, id, cd.GetID())
}

// TestCoreClaimGetConfidence tests CoreClaim.GetConfidence.
func TestCoreClaimGetConfidence(t *testing.T) {
	t.Parallel()

	cc := document.CoreClaim{
		ID:         identifier.New(),
		Confidence: document.MediumConfidence,
	}
	assert.Equal(t, document.MediumConfidence, cc.GetConfidence()) //nolint:testifylint
}

// TestCoreClaimMethods tests CoreClaim.Get, CoreClaim.Remove, CoreClaim.Size, CoreClaim.AllClaims.
func TestCoreClaimMethods(t *testing.T) {
	t.Parallel()

	prop := identifier.New()
	otherProp := identifier.New()
	id1 := identifier.New()
	id2 := identifier.New()

	claim := &document.NoneClaim{
		CoreClaim: document.CoreClaim{
			ID:         identifier.New(),
			Confidence: 1.0,
		},
		Prop: document.Reference{ID: prop},
	}

	// Initially empty.
	assert.Equal(t, 0, claim.Size())
	assert.Empty(t, claim.AllClaims())
	assert.Empty(t, claim.Get(prop))

	metaClaim1 := &document.StringClaim{
		CoreClaim: document.CoreClaim{
			ID:         id1,
			Confidence: 1.0,
		},
		Prop:   document.Reference{ID: prop},
		String: "first",
	}
	metaClaim2 := &document.UnknownClaim{
		CoreClaim: document.CoreClaim{
			ID:         id2,
			Confidence: 1.0,
		},
		Prop: document.Reference{ID: otherProp},
	}

	errE := claim.Add(metaClaim1)
	require.NoError(t, errE, "% -+#.1v", errE)
	errE = claim.Add(metaClaim2)
	require.NoError(t, errE, "% -+#.1v", errE)

	assert.Equal(t, 2, claim.Size())

	// AllClaims returns all meta claims.
	all := claim.AllClaims()
	assert.Len(t, all, 2)

	// Get returns only claims matching prop.
	got := claim.Get(prop)
	assert.Len(t, got, 1)
	assert.Equal(t, metaClaim1, got[0])

	// Remove removes and returns matching claims.
	removed := claim.Remove(prop)
	assert.Len(t, removed, 1)
	assert.Equal(t, 1, claim.Size())

	// Remove non-existing prop returns empty.
	removed = claim.Remove(identifier.New())
	assert.Empty(t, removed)
}

// TestDocumentWithAllClaimTypes exercises all 12 claim types in a document with all visitor methods.
func TestDocumentWithAllClaimTypes(t *testing.T) {
	t.Parallel()

	prop := identifier.New()
	ref := document.Reference{ID: prop}
	conf := document.HighConfidence
	from := 1.0
	fromP := 0.1
	to := 9.0
	toP := 0.1
	fromTS := document.Timestamp("2020-01-01")
	fromPrec := document.TimePrecisionDay
	toTS := document.Timestamp("2021-01-01")
	toPrec := document.TimePrecisionDay

	newCore := func() document.CoreClaim {
		return document.CoreClaim{
			ID:         identifier.New(),
			Confidence: conf,
		}
	}

	idClaim := &document.IdentifierClaim{CoreClaim: newCore(), Prop: ref, Value: "ext-id"}
	strClaim := &document.StringClaim{CoreClaim: newCore(), Prop: ref, String: "text"}
	htmlClaim := &document.HTMLClaim{CoreClaim: newCore(), Prop: ref, HTML: "<b>html</b>"}
	amtClaim := &document.AmountClaim{CoreClaim: newCore(), Prop: ref, Amount: 42.0, Precision: 1.0}
	amtIntervalClaim := &document.AmountIntervalClaim{ //nolint:exhaustruct
		CoreClaim:     newCore(),
		Prop:          ref,
		From:          &from,
		FromPrecision: &fromP,
		To:            &to,
		ToPrecision:   &toP,
	}
	timeClaim := &document.TimeClaim{
		CoreClaim: newCore(),
		Prop:      ref,
		Timestamp: "2025-01-01",
		Precision: document.TimePrecisionDay,
	}
	timeIntervalClaim := &document.TimeIntervalClaim{ //nolint:exhaustruct
		CoreClaim:     newCore(),
		Prop:          ref,
		From:          &fromTS,
		FromPrecision: &fromPrec,
		To:            &toTS,
		ToPrecision:   &toPrec,
	}
	refClaim := &document.ReferenceClaim{CoreClaim: newCore(), Prop: ref, IRI: "https://example.com"}
	relClaim := &document.RelationClaim{CoreClaim: newCore(), Prop: ref, To: ref}
	hasClaim := &document.HasClaim{CoreClaim: newCore(), Prop: ref}
	noneClaim := &document.NoneClaim{CoreClaim: newCore(), Prop: ref}
	unknownClaim := &document.UnknownClaim{CoreClaim: newCore(), Prop: ref}

	doc := &document.D{}

	for _, c := range []document.Claim{
		idClaim, strClaim, htmlClaim, amtClaim, amtIntervalClaim,
		timeClaim, timeIntervalClaim, refClaim, relClaim, hasClaim, noneClaim, unknownClaim,
	} {
		errE := doc.Add(c)
		require.NoError(t, errE, "% -+#.1v", errE)
	}

	assert.Equal(t, 12, doc.Size())

	// AllClaims covers AllClaimsVisitor.VisitX for all 12 types.
	all := doc.AllClaims()
	assert.Len(t, all, 12)

	// Get with matching prop covers GetByPropIDVisitor.VisitX for all 12 types.
	got := doc.Get(prop)
	assert.Len(t, got, 12)

	// Get with non-matching prop traverses all 12 types without matching.
	got = doc.Get(identifier.New())
	assert.Empty(t, got)

	// GetByID with non-existent ID traverses all 12 types via GetByIDVisitor.
	result := doc.GetByID(identifier.New())
	assert.Nil(t, result)

	// GetByID finds an existing claim.
	result = doc.GetByID(strClaim.ID)
	assert.Equal(t, strClaim, result)

	// GetByID finds an existing claim of each type.
	assert.Equal(t, idClaim, doc.GetByID(idClaim.ID))
	assert.Equal(t, htmlClaim, doc.GetByID(htmlClaim.ID))
	assert.Equal(t, amtClaim, doc.GetByID(amtClaim.ID))
	assert.Equal(t, amtIntervalClaim, doc.GetByID(amtIntervalClaim.ID))
	assert.Equal(t, timeClaim, doc.GetByID(timeClaim.ID))
	assert.Equal(t, timeIntervalClaim, doc.GetByID(timeIntervalClaim.ID))
	assert.Equal(t, refClaim, doc.GetByID(refClaim.ID))
	assert.Equal(t, relClaim, doc.GetByID(relClaim.ID))
	assert.Equal(t, hasClaim, doc.GetByID(hasClaim.ID))
	assert.Equal(t, noneClaim, doc.GetByID(noneClaim.ID))
	assert.Equal(t, unknownClaim, doc.GetByID(unknownClaim.ID))

	// Remove with matching prop removes all 12.
	removed := doc.Remove(prop)
	assert.Len(t, removed, 12)
	assert.Equal(t, 0, doc.Size())
	// Claims is set to nil when empty after removing.
	assert.Nil(t, doc.Claims)
}

// TestDocumentReference tests D.Reference.
func TestDocumentReference(t *testing.T) {
	t.Parallel()

	id := identifier.New()
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: id},
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
	assert.Empty(t, doc.AllClaims())

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
	assert.Len(t, doc.AllClaims(), 2)
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

// TestClaimValidations tests Validate methods on all claim types.
func TestClaimValidations(t *testing.T) {
	t.Parallel()

	prop := identifier.New()
	ref := document.Reference{ID: prop}
	core := document.CoreClaim{ID: identifier.New(), Confidence: 1.0}

	t.Run("IdentifierClaim/empty", func(t *testing.T) {
		t.Parallel()
		c := &document.IdentifierClaim{CoreClaim: core, Prop: ref, Value: ""}
		assert.EqualError(t, c.Validate(), "empty value")
	})
	t.Run("IdentifierClaim/valid", func(t *testing.T) {
		t.Parallel()
		c := &document.IdentifierClaim{CoreClaim: core, Prop: ref, Value: "Q42"}
		require.NoError(t, c.Validate())
	})

	t.Run("StringClaim/empty", func(t *testing.T) {
		t.Parallel()
		c := &document.StringClaim{CoreClaim: core, Prop: ref, String: ""}
		assert.EqualError(t, c.Validate(), "empty string")
	})
	t.Run("StringClaim/valid", func(t *testing.T) {
		t.Parallel()
		c := &document.StringClaim{CoreClaim: core, Prop: ref, String: "hello"}
		require.NoError(t, c.Validate())
	})

	t.Run("HTMLClaim/empty", func(t *testing.T) {
		t.Parallel()
		c := &document.HTMLClaim{CoreClaim: core, Prop: ref, HTML: ""}
		assert.EqualError(t, c.Validate(), "empty HTML")
	})
	t.Run("HTMLClaim/valid", func(t *testing.T) {
		t.Parallel()
		c := &document.HTMLClaim{CoreClaim: core, Prop: ref, HTML: "<p>text</p>"}
		require.NoError(t, c.Validate())
	})

	t.Run("ReferenceClaim/empty", func(t *testing.T) {
		t.Parallel()
		c := &document.ReferenceClaim{CoreClaim: core, Prop: ref, IRI: ""}
		assert.EqualError(t, c.Validate(), "empty IRI")
	})
	t.Run("ReferenceClaim/valid", func(t *testing.T) {
		t.Parallel()
		c := &document.ReferenceClaim{CoreClaim: core, Prop: ref, IRI: "https://example.com"}
		require.NoError(t, c.Validate(), "% -+#.1v")
	})

	t.Run("TimeClaim/invalid_precision", func(t *testing.T) {
		t.Parallel()
		c := &document.TimeClaim{CoreClaim: core, Prop: ref, Timestamp: "2025-01-01", Precision: 0}
		assert.EqualError(t, c.Validate(), "unknown Precision")
	})
	t.Run("TimeClaim/valid", func(t *testing.T) {
		t.Parallel()
		c := &document.TimeClaim{
			CoreClaim: core,
			Prop:      ref,
			Timestamp: "2025-01-01",
			Precision: document.TimePrecisionDay,
		}
		require.NoError(t, c.Validate())
	})
	t.Run("TimeClaim/invalid_timestamp", func(t *testing.T) {
		t.Parallel()
		c := &document.TimeClaim{
			CoreClaim: core,
			Prop:      ref,
			Timestamp: "not-a-date",
			Precision: document.TimePrecisionDay,
		}
		assert.EqualError(t, c.Validate(), "unable to parse timestamp")
	})

	from := document.Timestamp("2020-01-01")
	fromPrec := document.TimePrecisionDay
	to := document.Timestamp("2021-01-01")
	toPrec := document.TimePrecisionDay

	t.Run("TimeIntervalClaim/valid", func(t *testing.T) {
		t.Parallel()
		c := &document.TimeIntervalClaim{ //nolint:exhaustruct
			CoreClaim:     core,
			Prop:          ref,
			From:          &from,
			FromPrecision: &fromPrec,
			To:            &to,
			ToPrecision:   &toPrec,
		}
		require.NoError(t, c.Validate())
	})
	t.Run("TimeIntervalClaim/missing_from", func(t *testing.T) {
		t.Parallel()
		c := &document.TimeIntervalClaim{ //nolint:exhaustruct
			CoreClaim:   core,
			Prop:        ref,
			To:          &to,
			ToPrecision: &toPrec,
		}
		assert.EqualError(t, c.Validate(), "one of From, FromIsUnknown, or FromIsNone must be set")
	})
	t.Run("TimeIntervalClaim/multiple_from_flags", func(t *testing.T) {
		t.Parallel()
		c := &document.TimeIntervalClaim{ //nolint:exhaustruct
			CoreClaim:     core,
			Prop:          ref,
			FromIsUnknown: true,
			FromIsNone:    true,
			To:            &to,
			ToPrecision:   &toPrec,
		}
		assert.EqualError(t, c.Validate(), "only one of FromIsOpen, FromIsUnknown, FromIsNone can be set")
	})
	t.Run("TimeIntervalClaim/from_and_unknown_flag", func(t *testing.T) {
		t.Parallel()
		c := &document.TimeIntervalClaim{ //nolint:exhaustruct
			CoreClaim:     core,
			Prop:          ref,
			From:          &from,
			FromPrecision: &fromPrec,
			FromIsUnknown: true,
			To:            &to,
			ToPrecision:   &toPrec,
		}
		assert.EqualError(t, c.Validate(), "From must not be set when FromIsUnknown or FromIsNone is true")
	})
	t.Run("TimeIntervalClaim/missing_to", func(t *testing.T) {
		t.Parallel()
		c := &document.TimeIntervalClaim{ //nolint:exhaustruct
			CoreClaim:     core,
			Prop:          ref,
			From:          &from,
			FromPrecision: &fromPrec,
		}
		assert.EqualError(t, c.Validate(), "one of To, ToIsUnknown, or ToIsNone must be set")
	})
	t.Run("TimeIntervalClaim/multiple_to_flags", func(t *testing.T) {
		t.Parallel()
		c := &document.TimeIntervalClaim{ //nolint:exhaustruct
			CoreClaim:     core,
			Prop:          ref,
			From:          &from,
			FromPrecision: &fromPrec,
			ToIsUnknown:   true,
			ToIsNone:      true,
		}
		assert.EqualError(t, c.Validate(), "only one of ToIsClosed, ToIsUnknown, ToIsNone can be set")
	})
	t.Run("TimeIntervalClaim/invalid_from_precision", func(t *testing.T) {
		t.Parallel()
		invalidPrec := document.TimePrecision(99)
		c := &document.TimeIntervalClaim{ //nolint:exhaustruct
			CoreClaim:     core,
			Prop:          ref,
			From:          &from,
			FromPrecision: &invalidPrec,
			To:            &to,
			ToPrecision:   &toPrec,
		}
		assert.EqualError(t, c.Validate(), "unknown FromPrecision")
	})
	t.Run("TimeIntervalClaim/invalid_to_precision", func(t *testing.T) {
		t.Parallel()
		invalidPrec := document.TimePrecision(99)
		c := &document.TimeIntervalClaim{ //nolint:exhaustruct
			CoreClaim:     core,
			Prop:          ref,
			From:          &from,
			FromPrecision: &fromPrec,
			To:            &to,
			ToPrecision:   &invalidPrec,
		}
		assert.EqualError(t, c.Validate(), "unknown ToPrecision")
	})
	t.Run("TimeIntervalClaim/from_and_none_flag", func(t *testing.T) {
		t.Parallel()
		c := &document.TimeIntervalClaim{ //nolint:exhaustruct
			CoreClaim:     core,
			Prop:          ref,
			From:          &from,
			FromPrecision: &fromPrec,
			FromIsNone:    true,
			To:            &to,
			ToPrecision:   &toPrec,
		}
		assert.EqualError(t, c.Validate(), "From must not be set when FromIsUnknown or FromIsNone is true")
	})
	t.Run("TimeIntervalClaim/to_and_unknown_flag", func(t *testing.T) {
		t.Parallel()
		c := &document.TimeIntervalClaim{ //nolint:exhaustruct
			CoreClaim:     core,
			Prop:          ref,
			From:          &from,
			FromPrecision: &fromPrec,
			To:            &to,
			ToPrecision:   &toPrec,
			ToIsUnknown:   true,
		}
		assert.EqualError(t, c.Validate(), "To must not be set when ToIsUnknown or ToIsNone is true")
	})
}

// TestAmountIntervalClaimValidateExtra tests additional validation paths for AmountIntervalClaim.
func TestAmountIntervalClaimValidateExtra(t *testing.T) {
	t.Parallel()

	prop := identifier.New()
	ref := document.Reference{ID: prop}
	core := document.CoreClaim{ID: identifier.New(), Confidence: 1.0}
	from := 1.0
	fromP := 0.1
	to := 9.0
	toP := 0.1

	t.Run("multiple_from_flags", func(t *testing.T) {
		t.Parallel()
		c := &document.AmountIntervalClaim{ //nolint:exhaustruct
			CoreClaim:     core,
			Prop:          ref,
			FromIsUnknown: true,
			FromIsNone:    true,
			To:            &to,
			ToPrecision:   &toP,
		}
		assert.EqualError(t, c.Validate(), "only one of FromIsOpen, FromIsUnknown, FromIsNone can be set")
	})
	t.Run("from_set_with_unknown_flag", func(t *testing.T) {
		t.Parallel()
		c := &document.AmountIntervalClaim{ //nolint:exhaustruct
			CoreClaim:     core,
			Prop:          ref,
			From:          &from,
			FromPrecision: &fromP,
			FromIsUnknown: true,
			To:            &to,
			ToPrecision:   &toP,
		}
		assert.EqualError(t, c.Validate(), "From must not be set when FromIsUnknown or FromIsNone is true")
	})
	t.Run("multiple_to_flags", func(t *testing.T) {
		t.Parallel()
		c := &document.AmountIntervalClaim{ //nolint:exhaustruct
			CoreClaim:     core,
			Prop:          ref,
			From:          &from,
			FromPrecision: &fromP,
			ToIsUnknown:   true,
			ToIsNone:      true,
		}
		assert.EqualError(t, c.Validate(), "only one of ToIsClosed, ToIsUnknown, ToIsNone can be set")
	})
	t.Run("to_set_with_none_flag", func(t *testing.T) {
		t.Parallel()
		c := &document.AmountIntervalClaim{ //nolint:exhaustruct
			CoreClaim:     core,
			Prop:          ref,
			From:          &from,
			FromPrecision: &fromP,
			To:            &to,
			ToPrecision:   &toP,
			ToIsNone:      true,
		}
		assert.EqualError(t, c.Validate(), "To must not be set when ToIsUnknown or ToIsNone is true")
	})
	t.Run("to_precision_mismatch", func(t *testing.T) {
		t.Parallel()
		c := &document.AmountIntervalClaim{ //nolint:exhaustruct
			CoreClaim:     core,
			Prop:          ref,
			From:          &from,
			FromPrecision: &fromP,
			To:            &to,
		}
		assert.EqualError(t, c.Validate(), "To and ToPrecision must be set together")
	})
	t.Run("from_precision_mismatch", func(t *testing.T) {
		t.Parallel()
		c := &document.AmountIntervalClaim{ //nolint:exhaustruct
			CoreClaim:   core,
			Prop:        ref,
			FromIsNone:  true,
			To:          &to,
			ToPrecision: &toP,
		}
		errE := c.Validate()
		require.NoError(t, errE, "% -+#.1v", errE)
	})
}

// TestStringClaimPatch tests StringClaimPatch New, Apply, and JSON roundtrip.
func TestStringClaimPatch(t *testing.T) {
	t.Parallel()

	prop := identifier.New()
	confidence := document.Confidence(1.0)
	str := "hello world"

	p := document.StringClaimPatch{
		Confidence: &confidence,
		Prop:       &prop,
		String:     str,
	}

	// Test New.
	claim, errE := p.New([]string{"test", "0"})
	require.NoError(t, errE, "% -+#.1v", errE)

	c, ok := claim.(*document.StringClaim)
	require.True(t, ok)
	assert.Equal(t, str, c.String)
	assert.Equal(t, confidence, c.Confidence) //nolint:testifylint

	// Test JSON roundtrip.
	out, errE := x.MarshalWithoutEscapeHTML(p)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Contains(t, string(out), `"type":"string"`)

	var p2 document.StringClaimPatch
	errE = x.UnmarshalWithoutUnknownFields(out, &p2)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, p, p2)

	// Test Apply updates the string.
	newStr := "updated"
	applyPatch := document.StringClaimPatch{String: newStr}
	errE = applyPatch.Apply(claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, newStr, c.String)
}

// TestHTMLClaimPatch tests HTMLClaimPatch New, Apply, and JSON roundtrip.
func TestHTMLClaimPatch(t *testing.T) { //nolint:dupl
	t.Parallel()

	prop := identifier.New()
	confidence := document.Confidence(1.0)
	html := "<b>bold</b>"

	p := document.HTMLClaimPatch{
		Confidence: &confidence,
		Prop:       &prop,
		HTML:       html,
	}

	// Test New.
	claim, errE := p.New([]string{"test", "0"})
	require.NoError(t, errE, "% -+#.1v", errE)

	c, ok := claim.(*document.HTMLClaim)
	require.True(t, ok)
	assert.Equal(t, html, c.HTML)

	// Test JSON roundtrip.
	out, errE := x.MarshalWithoutEscapeHTML(p)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Contains(t, string(out), `"type":"html"`)

	var p2 document.HTMLClaimPatch
	errE = x.UnmarshalWithoutUnknownFields(out, &p2)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, p, p2)

	// Test Apply updates the HTML.
	newHTML := "<i>italic</i>"
	applyPatch := document.HTMLClaimPatch{HTML: newHTML}
	errE = applyPatch.Apply(claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, newHTML, c.HTML)
}

// TestTimeClaimPatch tests TimeClaimPatch New, Apply, and JSON roundtrip.
func TestTimeClaimPatch(t *testing.T) {
	t.Parallel()

	prop := identifier.New()
	confidence := document.Confidence(1.0)
	ts := document.Timestamp("2025-06-15")
	prec := document.TimePrecisionDay

	p := document.TimeClaimPatch{
		Confidence: &confidence,
		Prop:       &prop,
		Timestamp:  &ts,
		Precision:  &prec,
	}

	// Test New.
	claim, errE := p.New([]string{"test", "0"})
	require.NoError(t, errE, "% -+#.1v", errE)

	c, ok := claim.(*document.TimeClaim)
	require.True(t, ok)
	assert.Equal(t, ts, c.Timestamp)
	assert.Equal(t, prec, c.Precision)

	// Test JSON roundtrip.
	out, errE := x.MarshalWithoutEscapeHTML(p)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Contains(t, string(out), `"type":"time"`)

	var p2 document.TimeClaimPatch
	errE = x.UnmarshalWithoutUnknownFields(out, &p2)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, p, p2)

	// Test Apply updates the timestamp.
	newTS := document.Timestamp("2026-01-01")
	applyPatch := document.TimeClaimPatch{Timestamp: &newTS}
	errE = applyPatch.Apply(claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, newTS, c.Timestamp)
}

// TestReferenceClaimPatch tests ReferenceClaimPatch New, Apply, and JSON roundtrip.
func TestReferenceClaimPatch(t *testing.T) { //nolint:dupl
	t.Parallel()

	prop := identifier.New()
	confidence := document.Confidence(1.0)
	iri := "https://example.com/resource"

	p := document.ReferenceClaimPatch{
		Confidence: &confidence,
		Prop:       &prop,
		IRI:        iri,
	}

	// Test New.
	claim, errE := p.New([]string{"test", "0"})
	require.NoError(t, errE, "% -+#.1v", errE)

	c, ok := claim.(*document.ReferenceClaim)
	require.True(t, ok)
	assert.Equal(t, iri, c.IRI)

	// Test JSON roundtrip.
	out, errE := x.MarshalWithoutEscapeHTML(p)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Contains(t, string(out), `"type":"ref"`)

	var p2 document.ReferenceClaimPatch
	errE = x.UnmarshalWithoutUnknownFields(out, &p2)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, p, p2)

	// Test Apply updates the IRI.
	newIRI := "https://example.org/other"
	applyPatch := document.ReferenceClaimPatch{IRI: newIRI}
	errE = applyPatch.Apply(claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, newIRI, c.IRI)
}

// TestRelationClaimPatch tests RelationClaimPatch New, Apply, and JSON roundtrip.
func TestRelationClaimPatch(t *testing.T) {
	t.Parallel()

	prop := identifier.New()
	target := identifier.New()
	confidence := document.Confidence(1.0)

	p := document.RelationClaimPatch{
		Confidence: &confidence,
		Prop:       &prop,
		To:         &target,
	}

	// Test New.
	claim, errE := p.New([]string{"test", "0"})
	require.NoError(t, errE, "% -+#.1v", errE)

	c, ok := claim.(*document.RelationClaim)
	require.True(t, ok)
	assert.Equal(t, target, c.To.ID)

	// Test JSON roundtrip.
	out, errE := x.MarshalWithoutEscapeHTML(p)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Contains(t, string(out), `"type":"rel"`)

	var p2 document.RelationClaimPatch
	errE = x.UnmarshalWithoutUnknownFields(out, &p2)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, p, p2)

	// Test Apply updates the target.
	newTarget := identifier.New()
	applyPatch := document.RelationClaimPatch{To: &newTarget}
	errE = applyPatch.Apply(claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, newTarget, c.To.ID)
}

// TestHasClaimPatch tests HasClaimPatch New, Apply, and JSON roundtrip.
func TestHasClaimPatch(t *testing.T) { //nolint:dupl
	t.Parallel()

	prop := identifier.New()
	confidence := document.Confidence(1.0)

	p := document.HasClaimPatch{
		Confidence: &confidence,
		Prop:       &prop,
	}

	// Test New.
	claim, errE := p.New([]string{"test", "0"})
	require.NoError(t, errE, "% -+#.1v", errE)

	c, ok := claim.(*document.HasClaim)
	require.True(t, ok)
	assert.Equal(t, confidence, c.Confidence) //nolint:testifylint

	// Test JSON roundtrip.
	out, errE := x.MarshalWithoutEscapeHTML(p)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Contains(t, string(out), `"type":"has"`)

	var p2 document.HasClaimPatch
	errE = x.UnmarshalWithoutUnknownFields(out, &p2)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, p, p2)

	// Test Apply updates the confidence.
	newConf := document.Confidence(0.5)
	applyPatch := document.HasClaimPatch{Confidence: &newConf}
	errE = applyPatch.Apply(claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, newConf, c.Confidence) //nolint:testifylint
}

// TestNoneClaimPatch tests NoneClaimPatch New, Apply, and JSON roundtrip.
func TestNoneClaimPatch(t *testing.T) {
	t.Parallel()

	prop := identifier.New()
	confidence := document.Confidence(1.0)

	p := document.NoneClaimPatch{
		Confidence: &confidence,
		Prop:       &prop,
	}

	// Test New.
	claim, errE := p.New([]string{"test", "0"})
	require.NoError(t, errE, "% -+#.1v", errE)

	c, ok := claim.(*document.NoneClaim)
	require.True(t, ok)
	assert.Equal(t, confidence, c.Confidence) //nolint:testifylint

	// Test JSON roundtrip.
	out, errE := x.MarshalWithoutEscapeHTML(p)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Contains(t, string(out), `"type":"none"`)

	var p2 document.NoneClaimPatch
	errE = x.UnmarshalWithoutUnknownFields(out, &p2)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, p, p2)

	// Test Apply updates the prop.
	newProp := identifier.New()
	applyPatch := document.NoneClaimPatch{Prop: &newProp}
	errE = applyPatch.Apply(claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, newProp, c.Prop.ID)
}

// TestUnknownClaimPatch tests UnknownClaimPatch New, Apply, and JSON roundtrip.
func TestUnknownClaimPatch(t *testing.T) { //nolint:dupl
	t.Parallel()

	prop := identifier.New()
	confidence := document.Confidence(1.0)

	p := document.UnknownClaimPatch{
		Confidence: &confidence,
		Prop:       &prop,
	}

	// Test New.
	claim, errE := p.New([]string{"test", "0"})
	require.NoError(t, errE, "% -+#.1v", errE)

	c, ok := claim.(*document.UnknownClaim)
	require.True(t, ok)
	assert.Equal(t, confidence, c.Confidence) //nolint:testifylint

	// Test JSON roundtrip.
	out, errE := x.MarshalWithoutEscapeHTML(p)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Contains(t, string(out), `"type":"unknown"`)

	var p2 document.UnknownClaimPatch
	errE = x.UnmarshalWithoutUnknownFields(out, &p2)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, p, p2)

	// Test Apply updates the confidence.
	newConf := document.Confidence(0.75)
	applyPatch := document.UnknownClaimPatch{Confidence: &newConf}
	errE = applyPatch.Apply(claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, newConf, c.Confidence) //nolint:testifylint
}

// TestSetClaimChange tests SetClaimChange Apply, Validate, and JSON roundtrip.
func TestSetClaimChange(t *testing.T) {
	t.Parallel()

	base := []string{"TqtRsbk7rTKviW3TJapTim"}
	id1 := []string{"TqtRsbk7rTKviW3TJapTim", "0"}
	docID := identifier.From(base...)
	claimID := identifier.From(id1...)
	prop := identifier.String("XkbTJqwFCFkfoxMBXow4HU")
	confidence := document.Confidence(1.0)
	str := "original"

	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: docID},
	}

	// Add initial claim via AddClaimChange.
	addChange := document.AddClaimChange{ //nolint:exhaustruct
		ID: id1,
		Patch: document.StringClaimPatch{
			Confidence: &confidence,
			Prop:       &prop,
			String:     str,
		},
	}
	errE := addChange.Apply(doc)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Now set the claim with a new string value.
	newStr := "updated"
	setChange := document.SetClaimChange{
		ID: claimID,
		Patch: document.StringClaimPatch{
			String: newStr,
		},
	}

	// Test Validate (always returns nil).
	errE = setChange.Validate(t.Context(), base, 0)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Test JSON roundtrip.
	changes := document.Changes{setChange}
	out, errE := x.MarshalWithoutEscapeHTML(changes)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Contains(t, string(out), `"type":"set"`)

	var changes2 document.Changes
	errE = x.UnmarshalWithoutUnknownFields(out, &changes2)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, changes, changes2)

	// Test Apply changes the claim.
	errE = setChange.Apply(doc)
	require.NoError(t, errE, "% -+#.1v", errE)

	result := doc.GetByID(claimID)
	require.NotNil(t, result)
	c, ok := result.(*document.StringClaim)
	require.True(t, ok)
	assert.Equal(t, newStr, c.String)

	// Test Apply on non-existent claim returns error.
	setChangeNotFound := document.SetClaimChange{
		ID:    identifier.New(),
		Patch: document.StringClaimPatch{String: "x"},
	}
	errE = setChangeNotFound.Apply(doc)
	assert.EqualError(t, errE, "claim not found")
}

// TestRemoveClaimChange tests RemoveClaimChange Apply, Validate, and JSON roundtrip.
func TestRemoveClaimChange(t *testing.T) {
	t.Parallel()

	base := []string{"TqtRsbk7rTKviW3TJapTim"}
	id1 := []string{"TqtRsbk7rTKviW3TJapTim", "0"}
	docID := identifier.From(base...)
	claimID := identifier.From(id1...)
	prop := identifier.String("XkbTJqwFCFkfoxMBXow4HU")
	confidence := document.Confidence(1.0)

	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: docID},
	}

	// Add a claim first.
	addChange := document.AddClaimChange{ //nolint:exhaustruct
		ID: id1,
		Patch: document.NoneClaimPatch{
			Confidence: &confidence,
			Prop:       &prop,
		},
	}
	errE := addChange.Apply(doc)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, 1, doc.Size())

	// Test RemoveClaimChange.
	removeChange := document.RemoveClaimChange{
		ID: claimID,
	}

	// Test Validate (always returns nil).
	errE = removeChange.Validate(t.Context(), base, 0)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Test JSON roundtrip.
	changes := document.Changes{removeChange}
	out, errE := x.MarshalWithoutEscapeHTML(changes)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Contains(t, string(out), `"type":"remove"`)

	var changes2 document.Changes
	errE = x.UnmarshalWithoutUnknownFields(out, &changes2)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, changes, changes2)

	// Test Apply removes the claim.
	errE = removeChange.Apply(doc)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, 0, doc.Size())

	// Test Apply on non-existent claim returns error.
	errE = removeChange.Apply(doc)
	assert.EqualError(t, errE, "claim not found")
}

// TestClaimPatchMarshalUnmarshalJSON tests ClaimPatchMarshalJSON and ClaimPatchUnmarshalJSON.
func TestClaimPatchMarshalUnmarshalJSON(t *testing.T) {
	t.Parallel()

	prop := identifier.New()
	conf := document.Confidence(1.0)

	patches := []document.ClaimPatch{
		document.StringClaimPatch{Confidence: &conf, Prop: &prop, String: "test"},
		document.HTMLClaimPatch{Confidence: &conf, Prop: &prop, HTML: "<p>html</p>"},
		document.ReferenceClaimPatch{Confidence: &conf, Prop: &prop, IRI: "https://example.com"},
		document.RelationClaimPatch{Confidence: &conf, Prop: &prop, To: &prop},
		document.HasClaimPatch{Confidence: &conf, Prop: &prop},
		document.NoneClaimPatch{Confidence: &conf, Prop: &prop},
		document.UnknownClaimPatch{Confidence: &conf, Prop: &prop},
	}

	for _, patch := range patches {
		data, errE := document.ClaimPatchMarshalJSON(patch)
		require.NoError(t, errE, "% -+#.1v", errE)

		unmarshaled, errE := document.ClaimPatchUnmarshalJSON(data)
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.Equal(t, patch, unmarshaled)
	}
}

// TestChangeMarshalJSON tests the ChangeMarshalJSON and ChangeUnmarshalJSON functions.
func TestChangeMarshalJSON(t *testing.T) {
	t.Parallel()

	id := identifier.New()
	prop := identifier.New()
	conf := document.Confidence(1.0)

	changes := []document.Change{
		document.AddClaimChange{ //nolint:exhaustruct
			ID: []string{"base", "0"},
			Patch: document.HasClaimPatch{
				Confidence: &conf,
				Prop:       &prop,
			},
		},
		document.SetClaimChange{
			ID:    id,
			Patch: document.HasClaimPatch{Prop: &prop},
		},
		document.RemoveClaimChange{
			ID: id,
		},
	}

	for _, change := range changes {
		data, errE := document.ChangeMarshalJSON(change)
		require.NoError(t, errE, "% -+#.1v", errE)

		unmarshaled, errE := document.ChangeUnmarshalJSON(data)
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.Equal(t, change, unmarshaled)
	}
}

// TestTimestampMarshalText tests Timestamp.MarshalText.
func TestTimestampMarshalText(t *testing.T) {
	t.Parallel()

	ts := document.Timestamp("2025-03-17 10:30:00")
	text, err := ts.MarshalText()
	require.NoError(t, err)
	assert.Equal(t, []byte("2025-03-17 10:30:00"), text)

	var ts2 document.Timestamp
	err = ts2.UnmarshalText(text)
	require.NoError(t, err)
	assert.Equal(t, ts, ts2)
}

// TestTimePrecisionMarshalText tests TimePrecision.MarshalText.
func TestTimePrecisionMarshalText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		precision document.TimePrecision
		expected  string
	}{
		{document.TimePrecisionYear, "y"},
		{document.TimePrecisionDay, "d"},
		{document.TimePrecisionNanosecond, "ns"},
	}

	for _, test := range tests {
		t.Run(test.expected, func(t *testing.T) {
			t.Parallel()

			text, err := test.precision.MarshalText()
			require.NoError(t, err)
			assert.Equal(t, []byte(test.expected), text)
		})
	}
}

// TestTimestampLeapYear tests leap year handling in Timestamp.Time via isLeap and daysIn.
func TestTimestampLeapYear(t *testing.T) {
	t.Parallel()

	// Feb 29 in a 400-year leap (year 2000).
	ts := document.Timestamp("2000-02-29")
	errE := ts.Validate(document.TimePrecisionDay)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Feb 29 in a regular quadrennial leap year (2004).
	ts = document.Timestamp("2004-02-29")
	errE = ts.Validate(document.TimePrecisionDay)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Feb 29 in a century non-leap year (1900).
	ts = document.Timestamp("1900-02-29")
	errE = ts.Validate(document.TimePrecisionDay)
	assert.EqualError(t, errE, "day out of range")

	// Feb 28 in a non-leap year is valid.
	ts = document.Timestamp("2001-02-28")
	errE = ts.Validate(document.TimePrecisionDay)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Feb 29 in a non-leap year (2001) is invalid.
	ts = document.Timestamp("2001-02-29")
	errE = ts.Validate(document.TimePrecisionDay)
	assert.EqualError(t, errE, "day out of range")
}

// TestAllChangePatchTypesViaChanges exercises all patch types through Changes JSON roundtrip.
func TestAllChangePatchTypesViaChanges(t *testing.T) {
	t.Parallel()

	base := []string{"TqtRsbk7rTKviW3TJapTim"}
	prop := identifier.New()
	target := identifier.New()
	conf := document.Confidence(1.0)
	amount := 5.0
	precision := 0.1
	from := 1.0
	fromP := 0.1
	to := 9.0
	toP := 0.1
	ts := document.Timestamp("2025-01-01")
	tsPrec := document.TimePrecisionDay
	fromTS := document.Timestamp("2020-01-01")
	fromTSPrec := document.TimePrecisionDay
	toTS := document.Timestamp("2021-01-01")
	toTSPrec := document.TimePrecisionDay

	makeID := func(i int) []string {
		return append(append([]string{}, base...), string(rune('0'+i)))
	}

	changes := document.Changes{
		document.AddClaimChange{ //nolint:exhaustruct
			ID: makeID(0),
			Patch: document.StringClaimPatch{
				Confidence: &conf, Prop: &prop, String: "hello",
			},
		},
		document.AddClaimChange{ //nolint:exhaustruct
			ID: makeID(1),
			Patch: document.HTMLClaimPatch{
				Confidence: &conf, Prop: &prop, HTML: "<p>hi</p>",
			},
		},
		document.AddClaimChange{ //nolint:exhaustruct
			ID: makeID(2),
			Patch: document.AmountClaimPatch{
				Confidence: &conf, Prop: &prop, Amount: &amount, Precision: &precision,
			},
		},
		document.AddClaimChange{ //nolint:exhaustruct
			ID: makeID(3),
			Patch: document.AmountIntervalClaimPatch{
				Confidence: &conf, Prop: &prop,
				From: &from, FromPrecision: &fromP,
				To: &to, ToPrecision: &toP,
			},
		},
		document.AddClaimChange{ //nolint:exhaustruct
			ID: makeID(4),
			Patch: document.TimeClaimPatch{
				Confidence: &conf, Prop: &prop, Timestamp: &ts, Precision: &tsPrec,
			},
		},
		document.AddClaimChange{ //nolint:exhaustruct
			ID: makeID(5),
			Patch: document.TimeIntervalClaimPatch{
				Confidence: &conf, Prop: &prop,
				From: &fromTS, FromPrecision: &fromTSPrec,
				To: &toTS, ToPrecision: &toTSPrec,
			},
		},
		document.AddClaimChange{ //nolint:exhaustruct
			ID: makeID(6),
			Patch: document.ReferenceClaimPatch{
				Confidence: &conf, Prop: &prop, IRI: "https://example.com",
			},
		},
		document.AddClaimChange{ //nolint:exhaustruct
			ID: makeID(7),
			Patch: document.RelationClaimPatch{
				Confidence: &conf, Prop: &prop, To: &target,
			},
		},
		document.AddClaimChange{ //nolint:exhaustruct
			ID: makeID(8),
			Patch: document.HasClaimPatch{
				Confidence: &conf, Prop: &prop,
			},
		},
		document.AddClaimChange{ //nolint:exhaustruct
			ID: makeID(9),
			Patch: document.NoneClaimPatch{
				Confidence: &conf, Prop: &prop,
			},
		},
		document.AddClaimChange{ //nolint:exhaustruct
			ID: makeID(10),
			Patch: document.UnknownClaimPatch{
				Confidence: &conf, Prop: &prop,
			},
		},
	}

	// JSON roundtrip.
	out, errE := x.MarshalWithoutEscapeHTML(changes)
	require.NoError(t, errE, "% -+#.1v", errE)

	var changes2 document.Changes
	errE = x.UnmarshalWithoutUnknownFields(out, &changes2)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, changes, changes2)

	// Apply to document.
	docID := identifier.From(base...)
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: docID},
	}
	errE = changes.Apply(doc)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, 11, doc.Size())
}

// TestClaimTypesAddUnsupported tests that adding an unsupported claim type returns an error.
func TestClaimTypesAddUnsupported(t *testing.T) {
	t.Parallel()

	ct := &document.ClaimTypes{}
	errE := ct.Add(nil)
	assert.EqualError(t, errE, "claim type not supported")
}

// TestPatchNewIncomplete tests that calling New with incomplete patch returns an error.
func TestPatchNewIncomplete(t *testing.T) {
	t.Parallel()

	t.Run("StringClaimPatch", func(t *testing.T) {
		t.Parallel()
		p := document.StringClaimPatch{}
		_, errE := p.New([]string{"base", "0"})
		assert.EqualError(t, errE, "incomplete patch")
	})
	t.Run("HTMLClaimPatch", func(t *testing.T) {
		t.Parallel()
		p := document.HTMLClaimPatch{}
		_, errE := p.New([]string{"base", "0"})
		assert.EqualError(t, errE, "incomplete patch")
	})
	t.Run("TimeClaimPatch", func(t *testing.T) {
		t.Parallel()
		p := document.TimeClaimPatch{}
		_, errE := p.New([]string{"base", "0"})
		assert.EqualError(t, errE, "incomplete patch")
	})
	t.Run("ReferenceClaimPatch", func(t *testing.T) {
		t.Parallel()
		p := document.ReferenceClaimPatch{}
		_, errE := p.New([]string{"base", "0"})
		assert.EqualError(t, errE, "incomplete patch")
	})
	t.Run("RelationClaimPatch", func(t *testing.T) {
		t.Parallel()
		p := document.RelationClaimPatch{}
		_, errE := p.New([]string{"base", "0"})
		assert.EqualError(t, errE, "incomplete patch")
	})
	t.Run("HasClaimPatch", func(t *testing.T) {
		t.Parallel()
		p := document.HasClaimPatch{}
		_, errE := p.New([]string{"base", "0"})
		assert.EqualError(t, errE, "incomplete patch")
	})
	t.Run("NoneClaimPatch", func(t *testing.T) {
		t.Parallel()
		p := document.NoneClaimPatch{}
		_, errE := p.New([]string{"base", "0"})
		assert.EqualError(t, errE, "incomplete patch")
	})
	t.Run("UnknownClaimPatch", func(t *testing.T) {
		t.Parallel()
		p := document.UnknownClaimPatch{}
		_, errE := p.New([]string{"base", "0"})
		assert.EqualError(t, errE, "incomplete patch")
	})
}

// TestPatchApplyWrongType tests that Apply returns error when claim has wrong type.
func TestPatchApplyWrongType(t *testing.T) {
	t.Parallel()

	prop := identifier.New()
	conf := document.Confidence(1.0)
	wrongClaim := &document.NoneClaim{
		CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1.0},
		Prop:      document.Reference{ID: prop},
	}

	t.Run("StringClaimPatch", func(t *testing.T) {
		t.Parallel()
		p := document.StringClaimPatch{String: "x"}
		assert.EqualError(t, p.Apply(wrongClaim), "not string claim")
	})
	t.Run("HTMLClaimPatch", func(t *testing.T) {
		t.Parallel()
		p := document.HTMLClaimPatch{HTML: "<p>x</p>"}
		assert.EqualError(t, p.Apply(wrongClaim), "not HTML claim")
	})
	t.Run("TimeClaimPatch", func(t *testing.T) {
		t.Parallel()
		ts := document.Timestamp("2025-01-01")
		p := document.TimeClaimPatch{Timestamp: &ts}
		assert.EqualError(t, p.Apply(wrongClaim), "not time claim")
	})
	t.Run("ReferenceClaimPatch", func(t *testing.T) {
		t.Parallel()
		p := document.ReferenceClaimPatch{IRI: "https://example.com"}
		assert.EqualError(t, p.Apply(wrongClaim), "not reference claim")
	})
	t.Run("RelationClaimPatch", func(t *testing.T) {
		t.Parallel()
		p := document.RelationClaimPatch{To: &prop}
		assert.EqualError(t, p.Apply(wrongClaim), "not relation claim")
	})
	t.Run("HasClaimPatch", func(t *testing.T) {
		t.Parallel()
		p := document.HasClaimPatch{Confidence: &conf}
		assert.EqualError(t, p.Apply(wrongClaim), "not has claim")
	})
	t.Run("NoneClaimPatch/wrong_type", func(t *testing.T) {
		t.Parallel()
		wrongNonNone := &document.StringClaim{
			CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1.0},
			Prop:      document.Reference{ID: prop},
			String:    "x",
		}
		p := document.NoneClaimPatch{Confidence: &conf}
		assert.EqualError(t, p.Apply(wrongNonNone), "not none claim")
	})
	t.Run("UnknownClaimPatch/wrong_type", func(t *testing.T) {
		t.Parallel()
		p := document.UnknownClaimPatch{Confidence: &conf}
		assert.EqualError(t, p.Apply(wrongClaim), "not unknown claim")
	})
}

// TestIdentifierClaimPatchApply tests IdentifierClaimPatch.Apply.
func TestIdentifierClaimPatchApply(t *testing.T) {
	t.Parallel()

	prop := identifier.New()
	confidence := document.Confidence(1.0)
	value := "Q42"

	p := document.IdentifierClaimPatch{
		Confidence: &confidence,
		Prop:       &prop,
		Value:      value,
	}

	claim, errE := p.New([]string{"test", "0"})
	require.NoError(t, errE, "% -+#.1v", errE)

	c, ok := claim.(*document.IdentifierClaim)
	require.True(t, ok)
	assert.Equal(t, value, c.Value)

	// Test Apply updates the value.
	newValue := "P31"
	applyPatch := document.IdentifierClaimPatch{Value: newValue}
	errE = applyPatch.Apply(claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, newValue, c.Value)

	// Test Apply with empty patch returns error.
	emptyPatch := document.IdentifierClaimPatch{}
	errE = emptyPatch.Apply(claim)
	assert.EqualError(t, errE, "empty patch")

	// Test Apply with wrong claim type returns error.
	wrongClaim := &document.NoneClaim{
		CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1.0},
		Prop:      document.Reference{ID: prop},
	}
	applyPatch2 := document.IdentifierClaimPatch{Value: "x"}
	errE = applyPatch2.Apply(wrongClaim)
	assert.EqualError(t, errE, "not identifier claim")
}

// TestAmountClaimPatchApply tests AmountClaimPatch.Apply.
func TestAmountClaimPatchApply(t *testing.T) {
	t.Parallel()

	prop := identifier.New()
	confidence := document.Confidence(1.0)
	amount := 42.0
	precision := 1.0

	p := document.AmountClaimPatch{
		Confidence: &confidence,
		Prop:       &prop,
		Amount:     &amount,
		Precision:  &precision,
	}

	claim, errE := p.New([]string{"test", "0"})
	require.NoError(t, errE, "% -+#.1v", errE)

	c, ok := claim.(*document.AmountClaim)
	require.True(t, ok)
	assert.Equal(t, amount, c.Amount) //nolint:testifylint

	// Test Apply updates the amount.
	newAmount := 99.0
	applyPatch := document.AmountClaimPatch{Amount: &newAmount}
	errE = applyPatch.Apply(claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, newAmount, c.Amount) //nolint:testifylint

	// Test Apply with empty patch returns error.
	emptyPatch := document.AmountClaimPatch{}
	errE = emptyPatch.Apply(claim)
	assert.EqualError(t, errE, "empty patch")

	// Test Apply with wrong claim type returns error.
	wrongClaim := &document.NoneClaim{
		CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1.0},
		Prop:      document.Reference{ID: prop},
	}
	errE = applyPatch.Apply(wrongClaim)
	assert.EqualError(t, errE, "not amount claim")
}

// TestPatchApplyEmptyPatch tests that Apply returns error for all patch types when patch is empty.
func TestPatchApplyEmptyPatch(t *testing.T) {
	t.Parallel()

	prop := identifier.New()

	// Create valid claims for each type to test Apply against.
	makeStrClaim := func() document.Claim {
		return &document.StringClaim{
			CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1.0},
			Prop:      document.Reference{ID: prop},
			String:    "test",
		}
	}
	makeHTMLClaim := func() document.Claim {
		return &document.HTMLClaim{
			CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1.0},
			Prop:      document.Reference{ID: prop},
			HTML:      "<p>test</p>",
		}
	}
	makeTimeClaim := func() document.Claim {
		return &document.TimeClaim{
			CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1.0},
			Prop:      document.Reference{ID: prop},
			Timestamp: "2025-01-01",
			Precision: document.TimePrecisionDay,
		}
	}
	makeRefClaim := func() document.Claim {
		return &document.ReferenceClaim{
			CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1.0},
			Prop:      document.Reference{ID: prop},
			IRI:       "https://example.com",
		}
	}

	t.Run("StringClaimPatch/empty", func(t *testing.T) {
		t.Parallel()
		assert.EqualError(t, document.StringClaimPatch{}.Apply(makeStrClaim()), "empty patch")
	})
	t.Run("HTMLClaimPatch/empty", func(t *testing.T) {
		t.Parallel()
		assert.EqualError(t, document.HTMLClaimPatch{}.Apply(makeHTMLClaim()), "empty patch")
	})
	t.Run("TimeClaimPatch/empty", func(t *testing.T) {
		t.Parallel()
		assert.EqualError(t, document.TimeClaimPatch{}.Apply(makeTimeClaim()), "empty patch")
	})
	t.Run("ReferenceClaimPatch/empty", func(t *testing.T) {
		t.Parallel()
		assert.EqualError(t, document.ReferenceClaimPatch{}.Apply(makeRefClaim()), "empty patch")
	})
}

// TestChangeMarshalJSONUnsupportedType tests ChangeMarshalJSON with an unsupported type.
func TestChangeMarshalJSONUnsupportedType(t *testing.T) {
	t.Parallel()

	_, errE := document.ChangeMarshalJSON(customChange{})
	assert.EqualError(t, errE, "change type not supported")
}

// TestClaimPatchMarshalJSONUnsupportedType tests ClaimPatchMarshalJSON with an unsupported type.
func TestClaimPatchMarshalJSONUnsupportedType(t *testing.T) {
	t.Parallel()

	_, errE := document.ClaimPatchMarshalJSON(customPatch{})
	assert.EqualError(t, errE, "patch type not supported")
}

// TestChangeUnmarshalJSONErrors tests ChangeUnmarshalJSON error paths.
func TestChangeUnmarshalJSONErrors(t *testing.T) {
	t.Parallel()

	t.Run("unknown_type", func(t *testing.T) {
		t.Parallel()
		_, errE := document.ChangeUnmarshalJSON([]byte(`{"type":"unknown","id":["a","0"]}`))
		assert.EqualError(t, errE, "change type not supported")
	})
	t.Run("invalid_json", func(t *testing.T) {
		t.Parallel()
		_, errE := document.ChangeUnmarshalJSON([]byte(`not-json`))
		assert.EqualError(t, errE, "invalid character 'o' in literal null (expecting 'u')")
	})
}

// TestClaimPatchUnmarshalJSONErrors tests ClaimPatchUnmarshalJSON error paths.
func TestClaimPatchUnmarshalJSONErrors(t *testing.T) {
	t.Parallel()

	t.Run("unknown_type", func(t *testing.T) {
		t.Parallel()
		_, errE := document.ClaimPatchUnmarshalJSON([]byte(`{"type":"bogus","value":"x"}`))
		assert.EqualError(t, errE, "patch type not supported")
	})
	t.Run("invalid_json", func(t *testing.T) {
		t.Parallel()
		_, errE := document.ClaimPatchUnmarshalJSON([]byte(`not-json`))
		assert.EqualError(t, errE, "invalid character 'o' in literal null (expecting 'u')")
	})
}

// TestChangesApplyError tests Changes.Apply error propagation.
func TestChangesApplyError(t *testing.T) {
	t.Parallel()

	base := []string{"TqtRsbk7rTKviW3TJapTim"}
	id1 := []string{"TqtRsbk7rTKviW3TJapTim", "0"}
	docID := identifier.From(base...)
	prop := identifier.New()
	conf := document.Confidence(1.0)

	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: docID},
	}

	addChange := document.AddClaimChange{ //nolint:exhaustruct
		ID: id1,
		Patch: document.HasClaimPatch{
			Confidence: &conf,
			Prop:       &prop,
		},
	}

	// First apply succeeds.
	changes := document.Changes{addChange}
	errE := changes.Apply(doc)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Second apply with same ID fails (duplicate claim ID).
	changes = document.Changes{addChange}
	errE = changes.Apply(doc)
	assert.EqualError(t, errE, "claim with ID already exists")
}

// TestChangesValidateError tests Changes.Validate error propagation.
func TestChangesValidateError(t *testing.T) {
	t.Parallel()

	base := []string{"TqtRsbk7rTKviW3TJapTim"}
	prop := identifier.New()
	conf := document.Confidence(1.0)

	// An AddClaimChange with wrong ID format should fail validation.
	changes := document.Changes{
		document.AddClaimChange{ //nolint:exhaustruct
			ID: []string{"wrong", "format"},
			Patch: document.HasClaimPatch{
				Confidence: &conf,
				Prop:       &prop,
			},
		},
	}
	errE := changes.Validate(t.Context(), base)
	assert.EqualError(t, errE, "invalid ID")
}

// TestAddClaimChangeApplyUnderNotFound tests AddClaimChange.Apply when Under claim doesn't exist.
func TestAddClaimChangeApplyUnderNotFound(t *testing.T) {
	t.Parallel()

	prop := identifier.New()
	conf := document.Confidence(1.0)
	nonExistentID := identifier.New()

	doc := &document.D{}

	change := document.AddClaimChange{
		Under: &nonExistentID,
		ID:    []string{"base", "0"},
		Patch: document.HasClaimPatch{
			Confidence: &conf,
			Prop:       &prop,
		},
	}

	errE := change.Apply(doc)
	assert.EqualError(t, errE, "claim not found")
}

// TestChangesUnmarshalJSONError tests Changes.UnmarshalJSON when a change has invalid JSON.
func TestChangesUnmarshalJSONError(t *testing.T) {
	t.Parallel()

	// An array containing an element with an unknown change type should fail.
	var changes document.Changes
	errE := x.UnmarshalWithoutUnknownFields([]byte(`[{"type":"bogus"}]`), &changes)
	assert.EqualError(t, errE, "change type not supported")
}

// TestChangesMarshalJSONError tests Changes.MarshalJSON error propagation when a change can't be marshaled.
func TestChangesMarshalJSONError(t *testing.T) {
	t.Parallel()

	// A Changes slice with an unsupported type should fail to marshal.
	changes := document.Changes{customChange{}}
	_, err := changes.MarshalJSON()
	assert.EqualError(t, err, "change type not supported")
}

// TestAmountClaimValidateInfPrecision tests AmountClaim.Validate with infinite precision.
func TestAmountClaimValidateInfPrecision(t *testing.T) {
	t.Parallel()

	prop := identifier.New()
	ref := document.Reference{ID: prop}
	core := document.CoreClaim{ID: identifier.New(), Confidence: 1.0}

	c := &document.AmountClaim{
		CoreClaim: core,
		Prop:      ref,
		Amount:    1.0,
		Precision: math.Inf(1),
	}
	assert.EqualError(t, c.Validate(), "Precision must be a finite number")
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

// TestSetClaimChangeUnmarshalJSONWrongType tests SetClaimChange.UnmarshalJSON with wrong type.
func TestSetClaimChangeUnmarshalJSONWrongType(t *testing.T) {
	t.Parallel()

	// No "patch" field avoids ambiguity in the embedded struct, hitting the wrong-type branch.
	var c document.SetClaimChange
	errE := x.UnmarshalWithoutUnknownFields([]byte(`{"type":"remove","id":"XkbTJqwFCFkfoxMBXow4HU"}`), &c)
	assert.EqualError(t, errE, "invalid type")
}

// TestRemoveClaimChangeUnmarshalJSONWrongType tests RemoveClaimChange.UnmarshalJSON with wrong type.
func TestRemoveClaimChangeUnmarshalJSONWrongType(t *testing.T) {
	t.Parallel()

	var c document.RemoveClaimChange
	errE := x.UnmarshalWithoutUnknownFields([]byte(`{"type":"add","id":"XkbTJqwFCFkfoxMBXow4HU"}`), &c)
	assert.EqualError(t, errE, "invalid type")
}

// TestPatchUnmarshalJSONWrongType tests that each patch's UnmarshalJSON rejects a wrong type field.
// Uses clean JSON with no unknown fields so the wrong-type branch is exercised (not the bad-JSON branch).
func TestPatchUnmarshalJSONWrongType(t *testing.T) {
	t.Parallel()

	// Each test passes JSON with a valid structure but wrong "type" value.
	// This exercises the t.Type != "<expected>" branch in each UnmarshalJSON.
	t.Run("IdentifierClaimPatch", func(t *testing.T) {
		t.Parallel()
		var p document.IdentifierClaimPatch
		assert.EqualError(t, x.UnmarshalWithoutUnknownFields([]byte(`{"type":"html"}`), &p), "invalid type")
	})
	t.Run("StringClaimPatch", func(t *testing.T) {
		t.Parallel()
		var p document.StringClaimPatch
		assert.EqualError(t, x.UnmarshalWithoutUnknownFields([]byte(`{"type":"id"}`), &p), "invalid type")
	})
	t.Run("HTMLClaimPatch", func(t *testing.T) {
		t.Parallel()
		var p document.HTMLClaimPatch
		assert.EqualError(t, x.UnmarshalWithoutUnknownFields([]byte(`{"type":"string"}`), &p), "invalid type")
	})
	t.Run("AmountClaimPatch", func(t *testing.T) {
		t.Parallel()
		var p document.AmountClaimPatch
		assert.EqualError(t, x.UnmarshalWithoutUnknownFields([]byte(`{"type":"ref"}`), &p), "invalid type")
	})
	t.Run("AmountIntervalClaimPatch", func(t *testing.T) {
		t.Parallel()
		var p document.AmountIntervalClaimPatch
		assert.EqualError(t, x.UnmarshalWithoutUnknownFields([]byte(`{"type":"has"}`), &p), "invalid type")
	})
	t.Run("TimeClaimPatch", func(t *testing.T) {
		t.Parallel()
		var p document.TimeClaimPatch
		assert.EqualError(t, x.UnmarshalWithoutUnknownFields([]byte(`{"type":"none"}`), &p), "invalid type")
	})
	t.Run("TimeIntervalClaimPatch", func(t *testing.T) {
		t.Parallel()
		var p document.TimeIntervalClaimPatch
		assert.EqualError(t, x.UnmarshalWithoutUnknownFields([]byte(`{"type":"unknown"}`), &p), "invalid type")
	})
	t.Run("ReferenceClaimPatch", func(t *testing.T) {
		t.Parallel()
		var p document.ReferenceClaimPatch
		assert.EqualError(t, x.UnmarshalWithoutUnknownFields([]byte(`{"type":"rel"}`), &p), "invalid type")
	})
	t.Run("RelationClaimPatch", func(t *testing.T) {
		t.Parallel()
		var p document.RelationClaimPatch
		assert.EqualError(t, x.UnmarshalWithoutUnknownFields([]byte(`{"type":"has"}`), &p), "invalid type")
	})
	t.Run("HasClaimPatch", func(t *testing.T) {
		t.Parallel()
		var p document.HasClaimPatch
		assert.EqualError(t, x.UnmarshalWithoutUnknownFields([]byte(`{"type":"none"}`), &p), "invalid type")
	})
	t.Run("NoneClaimPatch", func(t *testing.T) {
		t.Parallel()
		var p document.NoneClaimPatch
		assert.EqualError(t, x.UnmarshalWithoutUnknownFields([]byte(`{"type":"has"}`), &p), "invalid type")
	})
	t.Run("UnknownClaimPatch", func(t *testing.T) {
		t.Parallel()
		var p document.UnknownClaimPatch
		assert.EqualError(t, x.UnmarshalWithoutUnknownFields([]byte(`{"type":"none"}`), &p), "invalid type")
	})
}

// TestAmountIntervalClaimPatchApplyBranches tests all branches in AmountIntervalClaimPatch.Apply.
func TestAmountIntervalClaimPatchApplyBranches(t *testing.T) {
	t.Parallel()

	newClaim := func(t *testing.T) *document.AmountIntervalClaim {
		t.Helper()
		prop := identifier.New()
		from := 1.0
		fromP := 0.1
		to := 9.0
		toP := 0.1
		conf := document.Confidence(1.0)
		p := document.AmountIntervalClaimPatch{
			Confidence:    &conf,
			Prop:          &prop,
			From:          &from,
			FromPrecision: &fromP,
			To:            &to,
			ToPrecision:   &toP,
		}
		claim, errE := p.New([]string{"test", "0"})
		require.NoError(t, errE, "% -+#.1v", errE)
		c, ok := claim.(*document.AmountIntervalClaim)
		require.True(t, ok)
		return c
	}

	t.Run("set_from_and_precision", func(t *testing.T) {
		t.Parallel()
		claim := newClaim(t)
		newFrom := 2.0
		newFromP := 0.2
		patch := document.AmountIntervalClaimPatch{
			From:          &newFrom,
			FromPrecision: &newFromP,
		}
		errE := patch.Apply(claim)
		require.NoError(t, errE, "% -+#.1v", errE)
		require.NotNil(t, claim.From)
		assert.Equal(t, newFrom, *claim.From) //nolint:testifylint
	})

	t.Run("set_from_is_open", func(t *testing.T) {
		t.Parallel()
		claim := newClaim(t)
		isOpen := true
		patch := document.AmountIntervalClaimPatch{FromIsOpen: &isOpen}
		errE := patch.Apply(claim)
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.True(t, claim.FromIsOpen)
	})

	t.Run("set_from_is_none", func(t *testing.T) {
		t.Parallel()
		claim := newClaim(t)
		isNone := true
		patch := document.AmountIntervalClaimPatch{FromIsNone: &isNone}
		errE := patch.Apply(claim)
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.True(t, claim.FromIsNone)
		assert.Nil(t, claim.From)
		assert.Nil(t, claim.FromPrecision)
	})

	t.Run("set_to_and_precision", func(t *testing.T) {
		t.Parallel()
		claim := newClaim(t)
		newTo := 15.0
		newToP := 0.5
		patch := document.AmountIntervalClaimPatch{
			To:          &newTo,
			ToPrecision: &newToP,
		}
		errE := patch.Apply(claim)
		require.NoError(t, errE, "% -+#.1v", errE)
		require.NotNil(t, claim.To)
		assert.Equal(t, newTo, *claim.To) //nolint:testifylint
	})

	t.Run("set_to_is_closed", func(t *testing.T) {
		t.Parallel()
		claim := newClaim(t)
		isClosed := true
		patch := document.AmountIntervalClaimPatch{ToIsClosed: &isClosed}
		errE := patch.Apply(claim)
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.True(t, claim.ToIsClosed)
	})

	t.Run("set_to_is_unknown", func(t *testing.T) {
		t.Parallel()
		claim := newClaim(t)
		isUnknown := true
		patch := document.AmountIntervalClaimPatch{ToIsUnknown: &isUnknown}
		errE := patch.Apply(claim)
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.True(t, claim.ToIsUnknown)
		assert.Nil(t, claim.To)
		assert.Nil(t, claim.ToPrecision)
	})

	t.Run("set_to_is_none", func(t *testing.T) {
		t.Parallel()
		claim := newClaim(t)
		isNone := true
		patch := document.AmountIntervalClaimPatch{ToIsNone: &isNone}
		errE := patch.Apply(claim)
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.True(t, claim.ToIsNone)
		assert.Nil(t, claim.To)
		assert.Nil(t, claim.ToPrecision)
	})

	t.Run("set_confidence_and_prop", func(t *testing.T) {
		t.Parallel()
		claim := newClaim(t)
		newConf := document.Confidence(0.5)
		newProp := identifier.New()
		patch := document.AmountIntervalClaimPatch{
			Confidence: &newConf,
			Prop:       &newProp,
		}
		errE := patch.Apply(claim)
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.Equal(t, newConf, claim.Confidence) //nolint:testifylint
		assert.Equal(t, newProp, claim.Prop.ID)
	})
}

// TestTimeIntervalClaimPatchApplyBranches tests all branches in TimeIntervalClaimPatch.Apply.
func TestTimeIntervalClaimPatchApplyBranches(t *testing.T) {
	t.Parallel()

	newClaim := func(t *testing.T) *document.TimeIntervalClaim {
		t.Helper()
		prop := identifier.New()
		from := document.Timestamp("2020-01-01")
		fromP := document.TimePrecisionDay
		to := document.Timestamp("2021-01-01")
		toP := document.TimePrecisionDay
		conf := document.Confidence(1.0)
		p := document.TimeIntervalClaimPatch{
			Confidence:    &conf,
			Prop:          &prop,
			From:          &from,
			FromPrecision: &fromP,
			To:            &to,
			ToPrecision:   &toP,
		}
		claim, errE := p.New([]string{"test", "0"})
		require.NoError(t, errE, "% -+#.1v", errE)
		c, ok := claim.(*document.TimeIntervalClaim)
		require.True(t, ok)
		return c
	}

	t.Run("set_from_and_precision", func(t *testing.T) {
		t.Parallel()
		claim := newClaim(t)
		newFrom := document.Timestamp("2022-06-01")
		newFromP := document.TimePrecisionDay
		patch := document.TimeIntervalClaimPatch{
			From:          &newFrom,
			FromPrecision: &newFromP,
		}
		errE := patch.Apply(claim)
		require.NoError(t, errE, "% -+#.1v", errE)
		require.NotNil(t, claim.From)
		assert.Equal(t, newFrom, *claim.From)
	})

	t.Run("set_from_is_open", func(t *testing.T) {
		t.Parallel()
		claim := newClaim(t)
		isOpen := true
		patch := document.TimeIntervalClaimPatch{FromIsOpen: &isOpen}
		errE := patch.Apply(claim)
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.True(t, claim.FromIsOpen)
	})

	t.Run("set_from_is_none", func(t *testing.T) {
		t.Parallel()
		claim := newClaim(t)
		isNone := true
		patch := document.TimeIntervalClaimPatch{FromIsNone: &isNone}
		errE := patch.Apply(claim)
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.True(t, claim.FromIsNone)
		assert.Nil(t, claim.From)
		assert.Nil(t, claim.FromPrecision)
	})

	t.Run("set_to_and_precision", func(t *testing.T) {
		t.Parallel()
		claim := newClaim(t)
		newTo := document.Timestamp("2023-12-31")
		newToP := document.TimePrecisionDay
		patch := document.TimeIntervalClaimPatch{
			To:          &newTo,
			ToPrecision: &newToP,
		}
		errE := patch.Apply(claim)
		require.NoError(t, errE, "% -+#.1v", errE)
		require.NotNil(t, claim.To)
		assert.Equal(t, newTo, *claim.To)
	})

	t.Run("set_to_is_closed", func(t *testing.T) {
		t.Parallel()
		claim := newClaim(t)
		isClosed := true
		patch := document.TimeIntervalClaimPatch{ToIsClosed: &isClosed}
		errE := patch.Apply(claim)
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.True(t, claim.ToIsClosed)
	})

	t.Run("set_to_is_unknown", func(t *testing.T) {
		t.Parallel()
		claim := newClaim(t)
		isUnknown := true
		patch := document.TimeIntervalClaimPatch{ToIsUnknown: &isUnknown}
		errE := patch.Apply(claim)
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.True(t, claim.ToIsUnknown)
		assert.Nil(t, claim.To)
		assert.Nil(t, claim.ToPrecision)
	})

	t.Run("set_to_is_none", func(t *testing.T) {
		t.Parallel()
		claim := newClaim(t)
		isNone := true
		patch := document.TimeIntervalClaimPatch{ToIsNone: &isNone}
		errE := patch.Apply(claim)
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.True(t, claim.ToIsNone)
		assert.Nil(t, claim.To)
		assert.Nil(t, claim.ToPrecision)
	})

	t.Run("set_confidence_and_prop", func(t *testing.T) {
		t.Parallel()
		claim := newClaim(t)
		newConf := document.Confidence(0.5)
		newProp := identifier.New()
		patch := document.TimeIntervalClaimPatch{
			Confidence: &newConf,
			Prop:       &newProp,
		}
		errE := patch.Apply(claim)
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.Equal(t, newConf, claim.Confidence) //nolint:testifylint
		assert.Equal(t, newProp, claim.Prop.ID)
	})

	t.Run("wrong_type", func(t *testing.T) {
		t.Parallel()
		wrongClaim := &document.NoneClaim{
			CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1.0},
			Prop:      document.Reference{ID: identifier.New()},
		}
		isOpen := true
		patch := document.TimeIntervalClaimPatch{FromIsOpen: &isOpen}
		assert.EqualError(t, patch.Apply(wrongClaim), "not time interval claim")
	})

	t.Run("empty_patch", func(t *testing.T) {
		t.Parallel()
		claim := newClaim(t)
		assert.EqualError(t, document.TimeIntervalClaimPatch{}.Apply(claim), "empty patch")
	})
}

// TestTimestampValidateExtraCases tests additional Timestamp.Validate branches.
func TestTimestampValidateExtraCases(t *testing.T) {
	t.Parallel()

	// "day not allowed for precision": month precision with non-zero day.
	errE := document.Timestamp("2025-03-15").Validate(document.TimePrecisionMonth)
	assert.EqualError(t, errE, "day not allowed for precision")

	// "hours and minutes not allowed for precision": day precision with hours present.
	errE = document.Timestamp("2025-03-15 10:00").Validate(document.TimePrecisionDay)
	assert.EqualError(t, errE, "hours and minutes not allowed for precision")

	// "seconds not allowed for precision": minute precision with seconds present.
	errE = document.Timestamp("2025-03-15 10:30:45").Validate(document.TimePrecisionMinute)
	assert.EqualError(t, errE, "seconds not allowed for precision")

	// "subseconds not allowed for precision": second precision with subseconds present.
	errE = document.Timestamp("2025-03-15 10:30:45.123").Validate(document.TimePrecisionSecond)
	assert.EqualError(t, errE, "subseconds not allowed for precision")
}

// TestTimePrecisionStringDefault tests TimePrecision.String for an unknown precision.
func TestTimePrecisionStringDefault(t *testing.T) {
	t.Parallel()

	p := document.TimePrecision(999)
	s := p.String()
	assert.Equal(t, "[999]", s)
}

// TestTimePrecisionUnmarshalTextUnknown tests TimePrecision.UnmarshalText with an unknown string.
func TestTimePrecisionUnmarshalTextUnknown(t *testing.T) {
	t.Parallel()

	var p document.TimePrecision
	errE := p.UnmarshalText([]byte("xyz"))
	assert.EqualError(t, errE, "unknown time precision")
}

// TestTimestampTimeNilLocation tests Timestamp.Time with a nil location.
func TestTimestampTimeNilLocation(t *testing.T) {
	t.Parallel()

	_, errE := document.Timestamp("2020-01-01").Time(0, nil)
	assert.EqualError(t, errE, "missing location")
}

// TestTimestampUnmarshalErrors tests error paths in Timestamp.UnmarshalText and UnmarshalJSON.
func TestTimestampUnmarshalErrors(t *testing.T) {
	t.Parallel()

	t.Run("unmarshal_text_invalid", func(t *testing.T) {
		t.Parallel()
		var ts document.Timestamp
		err := ts.UnmarshalText([]byte("not-a-timestamp"))
		assert.EqualError(t, err, "unable to parse timestamp")
	})

	t.Run("unmarshal_json_non_string", func(t *testing.T) {
		t.Parallel()
		var ts document.Timestamp
		err := ts.UnmarshalJSON([]byte("123"))
		assert.EqualError(t, err, "json: cannot unmarshal number into Go value of type string")
	})
}

// TestTimePrecisionUnmarshalJSONBadJSON tests TimePrecision.UnmarshalJSON with non-string JSON.
func TestTimePrecisionUnmarshalJSONBadJSON(t *testing.T) {
	t.Parallel()

	var p document.TimePrecision
	err := p.UnmarshalJSON([]byte("123"))
	assert.EqualError(t, err, "json: cannot unmarshal number into Go value of type string")
}

// TestReferenceClaimPatchApplyConfidenceOnly tests that ReferenceClaimPatch.Apply accepts a confidence-only patch.
func TestReferenceClaimPatchApplyConfidenceOnly(t *testing.T) {
	t.Parallel()

	prop := identifier.New()
	confidence := document.HighConfidence

	fullPatch := document.ReferenceClaimPatch{
		Confidence: &confidence,
		Prop:       &prop,
		IRI:        "https://example.com",
	}
	claim, errE := fullPatch.New([]string{"test", "0"})
	require.NoError(t, errE, "% -+#.1v", errE)

	newConfidence := document.LowConfidence
	confidenceOnlyPatch := document.ReferenceClaimPatch{
		Confidence: &newConfidence,
	}
	errE = confidenceOnlyPatch.Apply(claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, document.LowConfidence, claim.GetConfidence()) //nolint:testifylint
}

// TestRemoveByIDMetaClaim tests that RemoveByID removes a meta claim without removing its parent.
func TestRemoveByIDMetaClaim(t *testing.T) {
	t.Parallel()

	prop := identifier.New()
	topID := identifier.New()
	metaID := identifier.New()

	doc := document.D{
		CoreDocument: document.CoreDocument{ID: identifier.New()},
	}

	errE := doc.Add(&document.NoneClaim{
		CoreClaim: document.CoreClaim{ID: topID, Confidence: 1.0},
		Prop:      document.Reference{ID: prop},
	})
	require.NoError(t, errE, "% -+#.1v", errE)

	topClaim := doc.GetByID(topID)
	require.NotNil(t, topClaim)

	errE = topClaim.Add(&document.StringClaim{
		CoreClaim: document.CoreClaim{ID: metaID, Confidence: 1.0},
		Prop:      document.Reference{ID: prop},
		String:    "meta value",
	})
	require.NoError(t, errE, "% -+#.1v", errE)

	// RemoveByID on the meta claim ID must not remove the parent top-level claim.
	removed := doc.RemoveByID(metaID)
	require.NotNil(t, removed)
	assert.Equal(t, metaID, removed.GetID())

	// Parent claim must still exist in the document.
	assert.Equal(t, 1, doc.Size())
	parent := doc.GetByID(topID)
	assert.NotNil(t, parent)

	// Meta claim must be gone.
	meta := doc.GetByID(metaID)
	assert.Nil(t, meta)
}
