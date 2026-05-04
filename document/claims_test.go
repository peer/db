package document_test

import (
	"math"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/document"
	internalCore "gitlab.com/peerdb/peerdb/internal/core"
)

// TestCoreDocumentGetID tests CoreDocument.GetID.
func TestCoreDocumentGetID(t *testing.T) {
	t.Parallel()

	base := []string{"testdoc"}
	id := identifier.From(base...)
	cd := document.CoreDocument{ID: id, Base: base}
	assert.Equal(t, id, cd.GetID())
}

// TestCoreDocumentValidate tests CoreDocument.Validate.
func TestCoreDocumentValidate(t *testing.T) {
	t.Parallel()

	t.Run("valid", func(t *testing.T) {
		t.Parallel()

		base := []string{"TqtRsbk7rTKviW3TJapTim"}
		cd := document.CoreDocument{
			ID:   identifier.From(base...),
			Base: base,
		}
		errE := cd.Validate()
		require.NoError(t, errE, "% -+#.1v", errE)
	})

	t.Run("valid_multi_segment", func(t *testing.T) {
		t.Parallel()

		base := []string{"TqtRsbk7rTKviW3TJapTim", "0"}
		cd := document.CoreDocument{
			ID:   identifier.From(base...),
			Base: base,
		}
		errE := cd.Validate()
		require.NoError(t, errE, "% -+#.1v", errE)
	})

	t.Run("invalid_id", func(t *testing.T) {
		t.Parallel()

		base := []string{"TqtRsbk7rTKviW3TJapTim"}
		cd := document.CoreDocument{
			ID:   identifier.New(),
			Base: base,
		}
		errE := cd.Validate()
		assert.EqualError(t, errE, "invalid ID")
	})
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

// TestCoreClaimMethods tests CoreClaim.Get, CoreClaim.Remove, CoreClaim.Size, and CoreClaim.AllClaims.
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
	assert.Empty(t, slices.Collect(claim.AllClaims()))
	assert.Empty(t, claim.Get(prop))

	subClaim1 := &document.StringClaim{
		CoreClaim: document.CoreClaim{
			ID:         id1,
			Confidence: 1.0,
		},
		Prop:   document.Reference{ID: prop},
		String: "first",
	}
	subClaim2 := &document.UnknownClaim{
		CoreClaim: document.CoreClaim{
			ID:         id2,
			Confidence: 1.0,
		},
		Prop: document.Reference{ID: otherProp},
	}

	errE := claim.Add(subClaim1)
	require.NoError(t, errE, "% -+#.1v", errE)
	errE = claim.Add(subClaim2)
	require.NoError(t, errE, "% -+#.1v", errE)

	assert.Equal(t, 2, claim.Size())

	// AllClaims returns all sub-claims.
	all := slices.Collect(claim.AllClaims())
	assert.Len(t, all, 2)

	// Get returns only claims matching prop.
	got := claim.Get(prop)
	assert.Len(t, got, 1)
	assert.Equal(t, subClaim1, got[0])

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
	from := document.Amount("1.0")
	fromP := 0.1
	to := document.Amount("9.0")
	toP := 0.1
	fromTS := document.Time("2020-01-01")
	fromPrec := document.TimePrecisionDay
	toTS := document.Time("2021-01-01")
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
	amtClaim := &document.AmountClaim{CoreClaim: newCore(), Prop: ref, Amount: document.Amount("42"), Precision: 1.0}
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
		Time:      "2025-01-01",
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
	linkClaim := &document.LinkClaim{CoreClaim: newCore(), Prop: ref, IRI: "https://example.com"}
	refClaim := &document.ReferenceClaim{CoreClaim: newCore(), Prop: ref, To: ref}
	hasClaim := &document.HasClaim{CoreClaim: newCore(), Prop: ref}
	noneClaim := &document.NoneClaim{CoreClaim: newCore(), Prop: ref}
	unknownClaim := &document.UnknownClaim{CoreClaim: newCore(), Prop: ref}

	doc := &document.D{}

	for _, c := range []document.Claim{
		idClaim, strClaim, htmlClaim, amtClaim, amtIntervalClaim,
		timeClaim, timeIntervalClaim, linkClaim, refClaim, hasClaim, noneClaim, unknownClaim,
	} {
		errE := doc.Add(c)
		require.NoError(t, errE, "% -+#.1v", errE)
	}

	assert.Equal(t, 12, doc.Size())

	// AllClaims covers AllClaimsVisitor.VisitX for all 12 types.
	all := slices.Collect(doc.AllClaims())
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
	assert.Equal(t, linkClaim, doc.GetByID(linkClaim.ID))
	assert.Equal(t, refClaim, doc.GetByID(refClaim.ID))
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

	t.Run("LinkClaim/empty", func(t *testing.T) {
		t.Parallel()
		c := &document.LinkClaim{CoreClaim: core, Prop: ref, IRI: ""}
		assert.EqualError(t, c.Validate(), "empty IRI")
	})
	t.Run("LinkClaim/valid", func(t *testing.T) {
		t.Parallel()
		c := &document.LinkClaim{CoreClaim: core, Prop: ref, IRI: "https://example.com"}
		require.NoError(t, c.Validate(), "% -+#.1v")
	})

	t.Run("TimeClaim/invalid_precision", func(t *testing.T) {
		t.Parallel()
		c := &document.TimeClaim{CoreClaim: core, Prop: ref, Time: "2025-01-01", Precision: 0}
		assert.EqualError(t, c.Validate(), "unknown Precision")
	})
	t.Run("TimeClaim/valid", func(t *testing.T) {
		t.Parallel()
		c := &document.TimeClaim{
			CoreClaim: core,
			Prop:      ref,
			Time:      "2025-01-01",
			Precision: document.TimePrecisionDay,
		}
		require.NoError(t, c.Validate())
	})
	t.Run("TimeClaim/invalid_time", func(t *testing.T) {
		t.Parallel()
		c := &document.TimeClaim{
			CoreClaim: core,
			Prop:      ref,
			Time:      "not-a-date",
			Precision: document.TimePrecisionDay,
		}
		assert.EqualError(t, c.Validate(), "unable to parse time")
	})

	from := document.Time("2020-01-01")
	fromPrec := document.TimePrecisionDay
	to := document.Time("2021-01-01")
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
		assert.EqualError(t, c.Validate(), "only one of ToIsOpen, ToIsUnknown, ToIsNone can be set")
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

	t.Run("TimeIntervalClaim/empty_interval_from_open", func(t *testing.T) {
		t.Parallel()
		c := &document.TimeIntervalClaim{ //nolint:exhaustruct
			CoreClaim:     core,
			Prop:          ref,
			From:          &from,
			FromPrecision: &fromPrec,
			FromIsOpen:    true,
			To:            &from,
			ToPrecision:   &fromPrec,
		}
		assert.EqualError(t, c.Validate(), "interval is empty")
	})
	t.Run("TimeIntervalClaim/empty_interval_to_open", func(t *testing.T) {
		t.Parallel()
		c := &document.TimeIntervalClaim{ //nolint:exhaustruct
			CoreClaim:     core,
			Prop:          ref,
			From:          &from,
			FromPrecision: &fromPrec,
			To:            &from,
			ToPrecision:   &fromPrec,
			ToIsOpen:      true,
		}
		assert.EqualError(t, c.Validate(), "interval is empty")
	})
	t.Run("TimeIntervalClaim/empty_interval_both_open", func(t *testing.T) {
		t.Parallel()
		// from == to with both bounds open: un-swapped start = WindowEnd >
		// end = WindowStart, so un-swapped is empty. Swapped is the same
		// case (symmetric), also empty. Validate must reject.
		c := &document.TimeIntervalClaim{ //nolint:exhaustruct
			CoreClaim:     core,
			Prop:          ref,
			From:          &from,
			FromPrecision: &fromPrec,
			FromIsOpen:    true,
			To:            &from,
			ToPrecision:   &fromPrec,
			ToIsOpen:      true,
		}
		assert.EqualError(t, c.Validate(), "interval is empty")
	})
	t.Run("TimeIntervalClaim/single_point_default_flags_valid", func(t *testing.T) {
		t.Parallel()
		c := &document.TimeIntervalClaim{ //nolint:exhaustruct
			CoreClaim:     core,
			Prop:          ref,
			From:          &from,
			FromPrecision: &fromPrec,
			To:            &from,
			ToPrecision:   &fromPrec,
		}
		require.NoError(t, c.Validate())
	})
	t.Run("TimeIntervalClaim/empty_overlapping_windows_open", func(t *testing.T) {
		t.Parallel()
		// from window [2020-01-01, 2021-01-01) excluded, to window
		// [2020-01-01, 2020-01-02) included. Both un-swapped and swapped
		// orientations collapse to start == end -> empty.
		yearStr := document.Time("2020")
		yearPrec := document.TimePrecisionYear
		dayStr := document.Time("2020-01-01")
		dayPrec := document.TimePrecisionDay
		c := &document.TimeIntervalClaim{ //nolint:exhaustruct
			CoreClaim:     core,
			Prop:          ref,
			From:          &yearStr,
			FromPrecision: &yearPrec,
			FromIsOpen:    true,
			To:            &dayStr,
			ToPrecision:   &dayPrec,
		}
		assert.EqualError(t, c.Validate(), "interval is empty")
	})
	t.Run("TimeIntervalClaim/directed_decreasing_adjacent_valid", func(t *testing.T) {
		t.Parallel()
		// from=2025, to=2024 (both year, both closed). Adjacent year-windows
		// touch at 2025-01-01: un-swapped start = 2025-01-01 = end,
		// so un-swapped is empty. Swapped (lo=2024, hi=2025) gives
		// [2024-01-01, 2026-01-01), non-empty. Validate accepts.
		fromY := document.Time("2025")
		toY := document.Time("2024")
		yearPrec := document.TimePrecisionYear
		c := &document.TimeIntervalClaim{ //nolint:exhaustruct
			CoreClaim:     core,
			Prop:          ref,
			From:          &fromY,
			FromPrecision: &yearPrec,
			To:            &toY,
			ToPrecision:   &yearPrec,
		}
		require.NoError(t, c.Validate())
	})
	t.Run("TimeIntervalClaim/directed_decreasing_adjacent_both_open_empty", func(t *testing.T) {
		t.Parallel()
		// from=2025, to=2024 (both year), both open. Adjacent year-windows
		// touch at 2025-01-01. Un-swapped is empty. Swapped (from=2024
		// open, to=2025 open) gives effective [2025-01-01, 2025-01-01) -
		// also empty. Both orientations empty -> reject.
		fromY := document.Time("2025")
		toY := document.Time("2024")
		yearPrec := document.TimePrecisionYear
		c := &document.TimeIntervalClaim{ //nolint:exhaustruct
			CoreClaim:     core,
			Prop:          ref,
			From:          &fromY,
			FromPrecision: &yearPrec,
			FromIsOpen:    true,
			To:            &toY,
			ToPrecision:   &yearPrec,
			ToIsOpen:      true,
		}
		assert.EqualError(t, c.Validate(), "interval is empty")
	})
}

// TestAmountIntervalClaimValidateExtra tests additional validation paths for AmountIntervalClaim.
func TestAmountIntervalClaimValidateExtra(t *testing.T) {
	t.Parallel()

	prop := identifier.New()
	ref := document.Reference{ID: prop}
	core := document.CoreClaim{ID: identifier.New(), Confidence: 1.0}
	from := document.Amount("1.0")
	fromP := 0.1
	to := document.Amount("9.0")
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
		assert.EqualError(t, c.Validate(), "only one of ToIsOpen, ToIsUnknown, ToIsNone can be set")
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
	t.Run("empty_interval_from_open", func(t *testing.T) {
		t.Parallel()
		c := &document.AmountIntervalClaim{ //nolint:exhaustruct
			CoreClaim:     core,
			Prop:          ref,
			From:          &from,
			FromPrecision: &fromP,
			FromIsOpen:    true,
			To:            &from,
			ToPrecision:   &fromP,
		}
		assert.EqualError(t, c.Validate(), "interval is empty")
	})
	t.Run("empty_interval_to_open", func(t *testing.T) {
		t.Parallel()
		c := &document.AmountIntervalClaim{ //nolint:exhaustruct
			CoreClaim:     core,
			Prop:          ref,
			From:          &from,
			FromPrecision: &fromP,
			To:            &from,
			ToPrecision:   &fromP,
			ToIsOpen:      true,
		}
		assert.EqualError(t, c.Validate(), "interval is empty")
	})
	t.Run("empty_interval_both_open", func(t *testing.T) {
		t.Parallel()
		// from == to with both bounds open: un-swapped start = WindowEnd >
		// end = WindowStart, so un-swapped is empty. Swapped is the same
		// case (symmetric), also empty. Validate must reject.
		c := &document.AmountIntervalClaim{ //nolint:exhaustruct
			CoreClaim:     core,
			Prop:          ref,
			From:          &from,
			FromPrecision: &fromP,
			FromIsOpen:    true,
			To:            &from,
			ToPrecision:   &fromP,
			ToIsOpen:      true,
		}
		assert.EqualError(t, c.Validate(), "interval is empty")
	})
	t.Run("single_point_default_flags_valid", func(t *testing.T) {
		t.Parallel()
		c := &document.AmountIntervalClaim{ //nolint:exhaustruct
			CoreClaim:     core,
			Prop:          ref,
			From:          &from,
			FromPrecision: &fromP,
			To:            &from,
			ToPrecision:   &fromP,
		}
		require.NoError(t, c.Validate())
	})
	t.Run("equal_value_different_precision_open_valid", func(t *testing.T) {
		t.Parallel()
		// Same value, different precisions -> different windows; open flag does
		// not produce an empty interval at the document level.
		intAmount := document.Amount("10")
		fineP := 1.0
		coarseP := 10.0
		c := &document.AmountIntervalClaim{ //nolint:exhaustruct
			CoreClaim:     core,
			Prop:          ref,
			From:          &intAmount,
			FromPrecision: &fineP,
			FromIsOpen:    true,
			To:            &intAmount,
			ToPrecision:   &coarseP,
		}
		require.NoError(t, c.Validate())
	})
	t.Run("directed_decreasing_adjacent_valid", func(t *testing.T) {
		t.Parallel()
		// from=11, to=10 (both prec=1, both closed). Adjacent windows touch
		// at 10.5: un-swapped start = 10.5 = end, so un-swapped is
		// empty. Swapped (lo=10, hi=11) gives [9.5, 11.5), non-empty.
		// Validate accepts.
		from11 := document.Amount("11")
		to10 := document.Amount("10")
		prec := 1.0
		c := &document.AmountIntervalClaim{ //nolint:exhaustruct
			CoreClaim:     core,
			Prop:          ref,
			From:          &from11,
			FromPrecision: &prec,
			To:            &to10,
			ToPrecision:   &prec,
		}
		require.NoError(t, c.Validate())
	})
	t.Run("directed_decreasing_adjacent_both_open_empty", func(t *testing.T) {
		t.Parallel()
		// from=11, to=10 (both prec=1), both open. Adjacent windows touch
		// at 10.5. Un-swapped is empty. Swapped (from=10 open, to=11 open)
		// gives effective [10.5, 10.5) - also empty. Both orientations
		// empty -> reject.
		from11 := document.Amount("11")
		to10 := document.Amount("10")
		prec := 1.0
		c := &document.AmountIntervalClaim{ //nolint:exhaustruct
			CoreClaim:     core,
			Prop:          ref,
			From:          &from11,
			FromPrecision: &prec,
			FromIsOpen:    true,
			To:            &to10,
			ToPrecision:   &prec,
			ToIsOpen:      true,
		}
		assert.EqualError(t, c.Validate(), "interval is empty")
	})
}

// TestClaimTypesAddUnsupported tests that adding an unsupported claim type returns an error.
func TestClaimTypesAddUnsupported(t *testing.T) {
	t.Parallel()

	ct := &document.ClaimTypes{}
	errE := ct.Add(nil)
	assert.EqualError(t, errE, "claim type not supported")
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
		Amount:    document.Amount("1"),
		Precision: math.Inf(1),
	}
	assert.EqualError(t, c.Validate(), "Precision must be a finite positive number")
}

// TestAmountClaimValidateNegativePrecision tests AmountClaim.Validate with negative precision.
func TestAmountClaimValidateNegativePrecision(t *testing.T) {
	t.Parallel()

	prop := identifier.New()
	ref := document.Reference{ID: prop}
	core := document.CoreClaim{ID: identifier.New(), Confidence: 1.0}

	c := &document.AmountClaim{
		CoreClaim: core,
		Prop:      ref,
		Amount:    document.Amount("1"),
		Precision: -0.1,
	}
	assert.EqualError(t, c.Validate(), "Precision must be a finite positive number")
}

// TestAmountIntervalClaimValidateNegativePrecision tests AmountIntervalClaim.Validate with negative precision.
func TestAmountIntervalClaimValidateNegativePrecision(t *testing.T) {
	t.Parallel()

	prop := identifier.New()
	ref := document.Reference{ID: prop}
	core := document.CoreClaim{ID: identifier.New(), Confidence: 1.0}

	from := document.Amount("1.0")
	to := document.Amount("10.0")
	negP := -0.1
	posP := 0.1

	// Invalid: negative FromPrecision.
	c := &document.AmountIntervalClaim{ //nolint:exhaustruct
		CoreClaim:     core,
		Prop:          ref,
		From:          &from,
		FromPrecision: &negP,
		To:            &to,
		ToPrecision:   &posP,
	}
	assert.EqualError(t, c.Validate(), "FromPrecision must be finite positive number")

	// Invalid: negative ToPrecision.
	c = &document.AmountIntervalClaim{ //nolint:exhaustruct
		CoreClaim:     core,
		Prop:          ref,
		From:          &from,
		FromPrecision: &posP,
		To:            &to,
		ToPrecision:   &negP,
	}
	assert.EqualError(t, c.Validate(), "ToPrecision must be finite positive number")
}

// TestCoreClaimValidateInvalidConfidence tests CoreClaim.Validate with invalid confidence values.
func TestCoreClaimValidateInvalidConfidence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		confidence document.Confidence
	}{
		{"NaN", document.Confidence(math.NaN())},
		{"positive_infinity", document.Confidence(math.Inf(1))},
		{"negative_infinity", document.Confidence(math.Inf(-1))},
		{"too_high", document.Confidence(1.5)},
		{"too_low", document.Confidence(-1.5)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cc := document.CoreClaim{
				ID:         identifier.New(),
				Confidence: tt.confidence,
			}
			claim := &document.NoneClaim{
				CoreClaim: cc,
				Prop:      document.Reference{ID: identifier.New()},
			}
			errE := claim.Validate()
			assert.EqualError(t, errE, "confidence out of range [-1, 1]")
		})
	}
}

// TestCoreClaimAddDuplicateID tests CoreClaim.Add with duplicate ID.
func TestCoreClaimAddDuplicateID(t *testing.T) {
	t.Parallel()

	subID := identifier.New()
	claim := &document.NoneClaim{
		CoreClaim: document.CoreClaim{
			ID:         identifier.New(),
			Confidence: 1.0,
		},
		Prop: document.Reference{ID: identifier.New()},
	}

	errE := claim.Add(&document.StringClaim{
		CoreClaim: document.CoreClaim{ID: subID, Confidence: 1.0},
		Prop:      document.Reference{ID: identifier.New()},
		String:    "first",
	})
	require.NoError(t, errE, "% -+#.1v", errE)

	// Adding with same ID should fail.
	errE = claim.Add(&document.StringClaim{
		CoreClaim: document.CoreClaim{ID: subID, Confidence: 1.0},
		Prop:      document.Reference{ID: identifier.New()},
		String:    "duplicate",
	})
	assert.EqualError(t, errE, "claim with ID already exists")
}

// TestCoreClaimValidateWithInvalidSub tests CoreClaim.Validate with invalid sub-claims.
func TestCoreClaimValidateWithInvalidSub(t *testing.T) {
	t.Parallel()

	claim := &document.NoneClaim{
		CoreClaim: document.CoreClaim{
			ID:         identifier.New(),
			Confidence: 1.0,
			Sub: &document.ClaimTypes{
				String: document.StringClaims{
					{
						CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1.0},
						Prop:      document.Reference{ID: identifier.New()},
						String:    "", // Invalid: empty string.
					},
				},
			},
		},
		Prop: document.Reference{ID: identifier.New()},
	}

	errE := claim.Validate()
	assert.EqualError(t, errE, "empty string")
}

// TestClaimTypesGet tests ClaimTypes.Get directly.
func TestClaimTypesGet(t *testing.T) {
	t.Parallel()

	prop1 := identifier.New()
	prop2 := identifier.New()
	id1 := identifier.New()
	id2 := identifier.New()
	id3 := identifier.New()

	ct := &document.ClaimTypes{
		Identifier: document.IdentifierClaims{
			{
				CoreClaim: document.CoreClaim{ID: id1, Confidence: 0.5},
				Prop:      document.Reference{ID: prop1},
				Value:     "v1",
			},
		},
		String: document.StringClaims{
			{
				CoreClaim: document.CoreClaim{ID: id2, Confidence: 1.0},
				Prop:      document.Reference{ID: prop1},
				String:    "s1",
			},
		},
		None: document.NoneClaims{
			{
				CoreClaim: document.CoreClaim{ID: id3, Confidence: 0.75},
				Prop:      document.Reference{ID: prop2},
			},
		},
	}

	// Get by prop1 should return two claims sorted by decreasing confidence.
	result := ct.Get(prop1)
	require.Len(t, result, 2)
	assert.Equal(t, document.Confidence(1.0), result[0].GetConfidence()) //nolint:testifylint
	assert.Equal(t, document.Confidence(0.5), result[1].GetConfidence()) //nolint:testifylint

	// Get by prop2.
	result = ct.Get(prop2)
	require.Len(t, result, 1)
	assert.Equal(t, id3, result[0].GetID())

	// Get by unknown prop.
	result = ct.Get(identifier.New())
	assert.Empty(t, result)
}

// TestClaimTypesGetByID tests ClaimTypes.GetByID directly.
func TestClaimTypesGetByID(t *testing.T) {
	t.Parallel()

	id1 := identifier.New()
	id2 := identifier.New()

	ct := &document.ClaimTypes{
		Identifier: document.IdentifierClaims{
			{
				CoreClaim: document.CoreClaim{ID: id1, Confidence: 1.0},
				Prop:      document.Reference{ID: identifier.New()},
				Value:     "val",
			},
		},
		HTML: document.HTMLClaims{
			{
				CoreClaim: document.CoreClaim{ID: id2, Confidence: 0.5},
				Prop:      document.Reference{ID: identifier.New()},
				HTML:      "<p>hi</p>",
			},
		},
	}

	// Find existing claim.
	claim := ct.GetByID(id2)
	require.NotNil(t, claim)
	assert.Equal(t, id2, claim.GetID())

	// Not found.
	claim = ct.GetByID(identifier.New())
	assert.Nil(t, claim)
}

// TestClaimTypesRemove tests ClaimTypes.Remove directly.
func TestClaimTypesRemove(t *testing.T) {
	t.Parallel()

	prop := identifier.New()
	id1 := identifier.New()
	id2 := identifier.New()

	ct := &document.ClaimTypes{
		String: document.StringClaims{
			{
				CoreClaim: document.CoreClaim{ID: id1, Confidence: 1.0},
				Prop:      document.Reference{ID: prop},
				String:    "s1",
			},
			{
				CoreClaim: document.CoreClaim{ID: id2, Confidence: 0.5},
				Prop:      document.Reference{ID: prop},
				String:    "s2",
			},
		},
	}

	removed := ct.Remove(prop)
	assert.Len(t, removed, 2)
	assert.Equal(t, 0, ct.Size())
}

// TestClaimTypesRemoveByID tests ClaimTypes.RemoveByID directly.
func TestClaimTypesRemoveByID(t *testing.T) {
	t.Parallel()

	id1 := identifier.New()
	id2 := identifier.New()

	ct := &document.ClaimTypes{
		Amount: document.AmountClaims{
			{
				CoreClaim: document.CoreClaim{ID: id1, Confidence: 1.0},
				Prop:      document.Reference{ID: identifier.New()},
				Amount:    "42",
				Precision: 1,
			},
			{
				CoreClaim: document.CoreClaim{ID: id2, Confidence: 0.5},
				Prop:      document.Reference{ID: identifier.New()},
				Amount:    "100",
				Precision: 1,
			},
		},
	}

	removed := ct.RemoveByID(id1)
	require.NotNil(t, removed)
	assert.Equal(t, 1, ct.Size())

	// Remove non-existent.
	removed = ct.RemoveByID(identifier.New())
	assert.Nil(t, removed)
}

// TestClaimTypesGetWithAllTypes tests ClaimTypes.Get across all claim types.
func TestClaimTypesGetWithAllTypes(t *testing.T) {
	t.Parallel()

	prop := identifier.New()
	ts := document.NewTime(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), document.TimePrecisionYear, nil)

	ct := &document.ClaimTypes{
		Amount: document.AmountClaims{
			{
				CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1.0},
				Prop:      document.Reference{ID: prop},
				Amount:    "42",
				Precision: 1,
			},
		},
		AmountInterval: document.AmountIntervalClaims{
			{ //nolint:exhaustruct
				CoreClaim:   document.CoreClaim{ID: identifier.New(), Confidence: 1.0},
				Prop:        document.Reference{ID: prop},
				FromIsNone:  true,
				ToIsUnknown: true,
			},
		},
		Time: document.TimeClaims{
			{
				CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1.0},
				Prop:      document.Reference{ID: prop},
				Time:      ts,
				Precision: document.TimePrecisionYear,
			},
		},
		TimeInterval: document.TimeIntervalClaims{
			{ //nolint:exhaustruct
				CoreClaim:   document.CoreClaim{ID: identifier.New(), Confidence: 1.0},
				Prop:        document.Reference{ID: prop},
				FromIsNone:  true,
				ToIsUnknown: true,
			},
		},
		Link: document.LinkClaims{
			{
				CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1.0},
				Prop:      document.Reference{ID: prop},
				IRI:       "https://example.com",
			},
		},
		Reference: document.ReferenceClaims{
			{
				CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1.0},
				Prop:      document.Reference{ID: prop},
				To:        document.Reference{ID: identifier.New()},
			},
		},
		Has: document.HasClaims{
			{
				CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1.0},
				Prop:      document.Reference{ID: prop},
			},
		},
		Unknown: document.UnknownClaims{
			{
				CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1.0},
				Prop:      document.Reference{ID: prop},
			},
		},
	}

	result := ct.Get(prop)
	assert.Len(t, result, 8)
}

// TestGetBestClaimOfType tests the generic GetBestClaimOfType function.
func TestGetBestClaimOfType(t *testing.T) {
	t.Parallel()

	prop := identifier.New()

	ct := &document.ClaimTypes{
		String: document.StringClaims{
			{
				CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 0.5},
				Prop:      document.Reference{ID: prop},
				String:    "low",
			},
			{
				CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1.0},
				Prop:      document.Reference{ID: prop},
				String:    "high",
			},
		},
	}

	best := document.GetBestClaimOfType[document.StringClaim](ct, prop)
	require.NotNil(t, best)
	assert.Equal(t, "high", best.String)

	// No match returns zero value.
	bestAmount := document.GetBestClaimOfType[document.AmountClaim](ct, prop)
	assert.Nil(t, bestAmount)
}

// TestGetClaimsOfTypeWithConfidence tests the generic GetClaimsOfTypeWithConfidence function.
func TestGetClaimsOfTypeWithConfidence(t *testing.T) {
	t.Parallel()

	prop := identifier.New()

	ct := &document.ClaimTypes{
		String: document.StringClaims{
			{
				CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 0.3},
				Prop:      document.Reference{ID: prop},
				String:    "low",
			},
			{
				CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 0.8},
				Prop:      document.Reference{ID: prop},
				String:    "high",
			},
			{
				CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1.0},
				Prop:      document.Reference{ID: prop},
				String:    "top",
			},
		},
	}

	// With explicit confidence threshold.
	result := document.GetClaimsOfTypeWithConfidence[document.StringClaim](ct, prop, 0.75)
	require.Len(t, result, 2)
	assert.Equal(t, "top", result[0].String)
	assert.Equal(t, "high", result[1].String)

	// With 0 confidence (defaults to LowConfidence = 0.5).
	result = document.GetClaimsOfTypeWithConfidence[document.StringClaim](ct, prop, 0)
	require.Len(t, result, 2)
	assert.Equal(t, "top", result[0].String)
	assert.Equal(t, "high", result[1].String)

	// High threshold excludes all but top.
	result = document.GetClaimsOfTypeWithConfidence[document.StringClaim](ct, prop, 1.0)
	require.Len(t, result, 1)
	assert.Equal(t, "top", result[0].String)
}

// TestGetClaimsListsOfType tests grouping claims by LIST and sorting by ORDER_IN_LIST.
func TestGetClaimsListsOfType(t *testing.T) {
	t.Parallel()

	prop := identifier.New()
	listA := identifier.New()
	listB := identifier.New()
	listProp := internalCore.ListPropID
	orderProp := internalCore.OrderInListPropID

	ct := &document.ClaimTypes{
		String: document.StringClaims{
			{
				CoreClaim: document.CoreClaim{
					ID: identifier.New(), Confidence: 1.0,
					Sub: &document.ClaimTypes{
						Identifier: document.IdentifierClaims{
							{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1.0}, Prop: document.Reference{ID: listProp}, Value: listA.String()},
						},
						Amount: document.AmountClaims{
							{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1.0}, Prop: document.Reference{ID: orderProp}, Amount: "2", Precision: 1},
						},
					},
				},
				Prop:   document.Reference{ID: prop},
				String: "a2",
			},
			{
				CoreClaim: document.CoreClaim{
					ID: identifier.New(), Confidence: 1.0,
					Sub: &document.ClaimTypes{
						Identifier: document.IdentifierClaims{
							{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1.0}, Prop: document.Reference{ID: listProp}, Value: listA.String()},
						},
						Amount: document.AmountClaims{
							{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1.0}, Prop: document.Reference{ID: orderProp}, Amount: "1", Precision: 1},
						},
					},
				},
				Prop:   document.Reference{ID: prop},
				String: "a1",
			},
			{
				CoreClaim: document.CoreClaim{
					ID: identifier.New(), Confidence: 1.0,
					Sub: &document.ClaimTypes{
						Identifier: document.IdentifierClaims{
							{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1.0}, Prop: document.Reference{ID: listProp}, Value: listB.String()},
						},
						Amount: document.AmountClaims{
							{CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1.0}, Prop: document.Reference{ID: orderProp}, Amount: "1", Precision: 1},
						},
					},
				},
				Prop:   document.Reference{ID: prop},
				String: "b1",
			},
		},
	}

	lists := document.GetClaimsListsOfType[document.StringClaim](ct, prop)
	require.Len(t, lists, 2)

	// Find list A and list B (order of lists is not guaranteed).
	var listAClaims, listBClaims []*document.StringClaim
	for _, list := range lists {
		if len(list) == 2 {
			listAClaims = list
		} else {
			listBClaims = list
		}
	}

	// List A should have two claims sorted by order: "a1" (order 1), "a2" (order 2).
	require.Len(t, listAClaims, 2)
	assert.Equal(t, "a1", listAClaims[0].String)
	assert.Equal(t, "a2", listAClaims[1].String)

	// List B should have one claim: "b1".
	require.Len(t, listBClaims, 1)
	assert.Equal(t, "b1", listBClaims[0].String)
}

// TestGetClaimsListsOfTypeNoList tests claims without LIST sub-claims.
func TestGetClaimsListsOfTypeNoList(t *testing.T) {
	t.Parallel()

	prop := identifier.New()

	ct := &document.ClaimTypes{
		String: document.StringClaims{
			{
				CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 1.0},
				Prop:      document.Reference{ID: prop},
				String:    "no-list-1",
			},
			{
				CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: 0.8},
				Prop:      document.Reference{ID: prop},
				String:    "no-list-2",
			},
		},
	}

	// All claims without LIST sub-claim should be grouped into one list keyed "none".
	lists := document.GetClaimsListsOfType[document.StringClaim](ct, prop)
	require.Len(t, lists, 1)
	require.Len(t, lists[0], 2)
	// Without ORDER_IN_LIST, order is MaxFloat64 for all, so original order is preserved.
	assert.Equal(t, "no-list-1", lists[0][0].String)
	assert.Equal(t, "no-list-2", lists[0][1].String)
}

// TestGetClaimsListsOfTypeEmpty tests with no matching claims.
func TestGetClaimsListsOfTypeEmpty(t *testing.T) {
	t.Parallel()

	prop := identifier.New()
	ct := &document.ClaimTypes{}

	lists := document.GetClaimsListsOfType[document.StringClaim](ct, prop)
	assert.Nil(t, lists)
}

// TestCoreClaimGetRemove tests CoreClaim.Get and CoreClaim.Remove.
func TestCoreClaimGetRemove(t *testing.T) {
	t.Parallel()

	prop := identifier.New()
	subProp := identifier.New()
	subID := identifier.New()

	claim := &document.NoneClaim{
		CoreClaim: document.CoreClaim{
			ID:         identifier.New(),
			Confidence: 1.0,
			Sub: &document.ClaimTypes{
				String: document.StringClaims{
					{
						CoreClaim: document.CoreClaim{ID: subID, Confidence: 0.8},
						Prop:      document.Reference{ID: subProp},
						String:    "sub",
					},
				},
			},
		},
		Prop: document.Reference{ID: prop},
	}

	// Get sub-claims by prop.
	got := claim.Get(subProp)
	require.Len(t, got, 1)
	assert.Equal(t, subID, got[0].GetID())

	// Get with non-matching prop.
	got = claim.Get(identifier.New())
	assert.Empty(t, got)

	// Remove sub-claims by prop.
	removed := claim.Remove(subProp)
	require.Len(t, removed, 1)
	assert.Equal(t, 0, claim.Size())
}

// TestCoreClaimGetRemoveNoSub tests CoreClaim.Get and CoreClaim.Remove with no sub-claims.
func TestCoreClaimGetRemoveNoSub(t *testing.T) {
	t.Parallel()

	claim := &document.NoneClaim{
		CoreClaim: document.CoreClaim{
			ID:         identifier.New(),
			Confidence: 1.0,
		},
		Prop: document.Reference{ID: identifier.New()},
	}

	got := claim.Get(identifier.New())
	assert.Empty(t, got)

	removed := claim.Remove(identifier.New())
	assert.Empty(t, removed)

	found := claim.GetByID(identifier.New())
	assert.Nil(t, found)

	removedByID := claim.RemoveByID(identifier.New())
	assert.Nil(t, removedByID)
}

// TestRemoveByIDSubClaim tests that RemoveByID removes a sub-claim without removing its parent.
func TestRemoveByIDSubClaim(t *testing.T) {
	t.Parallel()

	prop := identifier.New()
	topID := identifier.New()
	subID := identifier.New()

	docBase := []string{"testdoc"}
	doc := document.D{
		CoreDocument: document.CoreDocument{ID: identifier.From(docBase...), Base: docBase},
	}

	errE := doc.Add(&document.NoneClaim{
		CoreClaim: document.CoreClaim{ID: topID, Confidence: 1.0},
		Prop:      document.Reference{ID: prop},
	})
	require.NoError(t, errE, "% -+#.1v", errE)

	topClaim := doc.GetByID(topID)
	require.NotNil(t, topClaim)

	errE = topClaim.Add(&document.StringClaim{
		CoreClaim: document.CoreClaim{ID: subID, Confidence: 1.0},
		Prop:      document.Reference{ID: prop},
		String:    "sub value",
	})
	require.NoError(t, errE, "% -+#.1v", errE)

	// RemoveByID on the sub-claim ID must not remove the parent top-level claim.
	removed := doc.RemoveByID(subID)
	require.NotNil(t, removed)
	assert.Equal(t, subID, removed.GetID())

	// Parent claim must still exist in the document.
	assert.Equal(t, 1, doc.Size())
	parent := doc.GetByID(topID)
	assert.NotNil(t, parent)

	// Sub-claim must be gone.
	sub := doc.GetByID(subID)
	assert.Nil(t, sub)
}
