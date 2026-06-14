package document_test

import (
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

func (c customChange) Apply(_ *document.D) errors.E          { return nil }
func (c customChange) Validate(_ []string, _ int64) errors.E { return nil }

// customPatch is a custom ClaimPatch type for testing unsupported type errors.
type customPatch struct{}

func (p customPatch) New(_ identifier.Identifier) (document.Claim, errors.E) { return nil, nil } //nolint:ireturn,nilnil
func (p customPatch) Apply(_ document.Claim) errors.E                        { return nil }
func (p customPatch) Validate() errors.E                                     { return nil }

func TestPatchJSON(t *testing.T) {
	t.Parallel()

	base := []string{"TqtRsbk7rTKviW3TJapTim"}
	id1 := []string{"TqtRsbk7rTKviW3TJapTim", "1"}
	id2 := []string{"TqtRsbk7rTKviW3TJapTim", "2"}
	id1Claim := identifier.From(id1...)
	id2Claim := identifier.From(id2...)
	prop1 := identifier.String("XkbTJqwFCFkfoxMBXow4HU")
	prop2 := identifier.String("3EL2nZdWVbw85XG1zTH2o5")
	confidence := document.Confidence(1.0)
	amount := document.Amount("42.1")
	precision := 0.1
	value := "foobar"

	changes := document.Changes{
		document.AddClaimChange{
			ID:    id1Claim,
			Base:  id1,
			Under: nil,
			Patch: document.AmountClaimPatch{
				Confidence: &confidence,
				Prop:       &prop1,
				Amount:     &amount,
				Precision:  &precision,
			},
		},
		document.AddClaimChange{
			ID:    id2Claim,
			Base:  id2,
			Under: &id1Claim,
			Patch: document.IdentifierClaimPatch{
				Confidence: &confidence,
				Prop:       &prop2,
				Value:      value,
			},
		},
	}

	out, errE := x.MarshalWithoutEscapeHTML(changes)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, `[{"id":"Cm2mEEMZHh6KB7CBvZLocZ","base":["TqtRsbk7rTKviW3TJapTim","1"],"patch":{"confidence":1,"prop":"XkbTJqwFCFkfoxMBXow4HU","amount":"42.1","precision":0.1,"type":"amount"},"type":"add"},{"under":"Cm2mEEMZHh6KB7CBvZLocZ","id":"Ah1k9c65Hhpuv9chpVZEeJ","base":["TqtRsbk7rTKviW3TJapTim","2"],"patch":{"confidence":1,"prop":"3EL2nZdWVbw85XG1zTH2o5","value":"foobar","type":"id"},"type":"add"}]`, string(out)) //nolint:lll,testifylint

	var changes2 document.Changes
	errE = x.UnmarshalWithoutUnknownFields(out, &changes2)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, changes, changes2)

	docID := identifier.From(base...)
	doc := &document.D{
		CoreDocument: document.CoreDocument{
			ID:   docID,
			Base: base,
		},
	}
	errE = changes.Validate(base)
	require.NoError(t, errE, "% -+#.1v", errE)
	errE = changes.Apply(doc)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, &document.D{
		CoreDocument: document.CoreDocument{
			ID:   docID,
			Base: base,
		},
		Claims: &document.ClaimTypes{
			Amount: []document.AmountClaim{
				{
					CoreClaim: document.CoreClaim{
						ID:         id1Claim,
						Confidence: 1.0,
						Sub: &document.ClaimTypes{
							Identifier: []document.IdentifierClaim{
								{
									CoreClaim: document.CoreClaim{
										ID:         id2Claim,
										Confidence: 1.0,
									},
									Prop: document.Reference{
										ID: prop2,
									},
									Value: value,
								},
							},
						},
					},
					Prop: document.Reference{
						ID: prop1,
					},
					Amount:    amount,
					Precision: precision,
				},
			},
		},
	}, doc)
}

func TestAmountIntervalClaimPatch(t *testing.T) {
	t.Parallel()

	prop := identifier.New()
	confidence := document.Confidence(1.0)
	from := document.Amount("1.5")
	fromPrecision := 0.1
	to := document.Amount("9.5")
	toPrecision := 0.1

	p := document.AmountIntervalClaimPatch{
		Confidence:    &confidence,
		Prop:          &prop,
		From:          &from,
		FromPrecision: &fromPrecision,
		To:            &to,
		ToPrecision:   &toPrecision,
	}

	claim, errE := p.New(identifier.From("test", "0"))
	require.NoError(t, errE, "% -+#.1v", errE)

	c, ok := claim.(*document.AmountIntervalClaim)
	require.True(t, ok)
	require.NotNil(t, c.From)
	assert.Equal(t, from, *c.From)
	require.NotNil(t, c.FromPrecision)
	assert.Equal(t, fromPrecision, *c.FromPrecision) //nolint:testifylint
	require.NotNil(t, c.To)
	assert.Equal(t, to, *c.To)
	require.NotNil(t, c.ToPrecision)
	assert.Equal(t, toPrecision, *c.ToPrecision) //nolint:testifylint

	// Test Apply: switch From to unknown.
	isUnknown := true
	applyPatch := document.AmountIntervalClaimPatch{
		FromIsUnknown: &isUnknown,
	}
	errE = applyPatch.Apply(claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.True(t, c.FromIsUnknown)
	assert.Nil(t, c.From)
	assert.Nil(t, c.FromPrecision)
}

func TestTimeIntervalClaimPatch(t *testing.T) {
	t.Parallel()

	prop := identifier.New()
	confidence := document.Confidence(1.0)
	from := document.Time("2020-01-01")
	fromPrecision := document.TimePrecisionDay
	to := document.Time("2021-01-01")
	toPrecision := document.TimePrecisionDay

	p := document.TimeIntervalClaimPatch{
		Confidence:    &confidence,
		Prop:          &prop,
		From:          &from,
		FromPrecision: &fromPrecision,
		To:            &to,
		ToPrecision:   &toPrecision,
	}

	claim, errE := p.New(identifier.From("test", "0"))
	require.NoError(t, errE, "% -+#.1v", errE)

	c, ok := claim.(*document.TimeIntervalClaim)
	require.True(t, ok)
	require.NotNil(t, c.From)
	assert.Equal(t, from, *c.From)
	require.NotNil(t, c.FromPrecision)
	assert.Equal(t, fromPrecision, *c.FromPrecision)
	require.NotNil(t, c.To)
	assert.Equal(t, to, *c.To)
	require.NotNil(t, c.ToPrecision)
	assert.Equal(t, toPrecision, *c.ToPrecision)

	// Test Apply: switch To to none.
	isNone := true
	applyPatch := document.TimeIntervalClaimPatch{
		ToIsNone: &isNone,
	}
	errE = applyPatch.Apply(claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.True(t, c.ToIsNone)
	assert.Nil(t, c.To)
	assert.Nil(t, c.ToPrecision)
}

// TestIntervalClaimPatchApplyClearsMarkers verifies that setting a concrete bound value
// via Apply clears a previously set unknown or none marker on that bound. This is the
// production case where a "set" fills in a bound that was previously none: the merged
// claim must not end up with both a value and the marker, which Validate rejects.
func TestIntervalClaimPatchApplyClearsMarkers(t *testing.T) {
	t.Parallel()

	t.Run("TimeInterval/ToIsNone", func(t *testing.T) {
		t.Parallel()

		from := document.Time("1984")
		yearPrec := document.TimePrecisionYear
		c := &document.TimeIntervalClaim{ //nolint:exhaustruct
			CoreClaim:     document.CoreClaim{ID: identifier.New(), Confidence: 1.0},
			Prop:          document.Reference{ID: identifier.New()},
			From:          &from,
			FromPrecision: &yearPrec,
			ToIsNone:      true,
		}
		require.NoError(t, c.Validate(), "% -+#.1v", c.Validate())

		to := document.Time("1950")
		applyPatch := document.TimeIntervalClaimPatch{
			To:          &to,
			ToPrecision: &yearPrec,
		}
		errE := applyPatch.Apply(c)
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.False(t, c.ToIsNone)
		require.NotNil(t, c.To)
		assert.Equal(t, to, *c.To)
	})

	t.Run("AmountInterval/FromIsUnknown", func(t *testing.T) {
		t.Parallel()

		to := document.Amount("9.5")
		toPrecision := 0.1
		c := &document.AmountIntervalClaim{ //nolint:exhaustruct
			CoreClaim:     document.CoreClaim{ID: identifier.New(), Confidence: 1.0},
			Prop:          document.Reference{ID: identifier.New()},
			To:            &to,
			ToPrecision:   &toPrecision,
			FromIsUnknown: true,
		}
		require.NoError(t, c.Validate(), "% -+#.1v", c.Validate())

		from := document.Amount("1.5")
		fromPrecision := 0.1
		applyPatch := document.AmountIntervalClaimPatch{
			From:          &from,
			FromPrecision: &fromPrecision,
		}
		errE := applyPatch.Apply(c)
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.False(t, c.FromIsUnknown)
		require.NotNil(t, c.From)
		assert.Equal(t, from, *c.From)
	})

	// Switching a bound directly from one marker to another must clear the previous
	// marker. The three markers (IsOpen, IsUnknown, IsNone) are mutually exclusive, so
	// leaving a stale one set fails validation just like the value-plus-marker case.
	t.Run("TimeInterval/UnknownToNone", func(t *testing.T) {
		t.Parallel()

		from := document.Time("1984")
		yearPrec := document.TimePrecisionYear
		c := &document.TimeIntervalClaim{ //nolint:exhaustruct
			CoreClaim:     document.CoreClaim{ID: identifier.New(), Confidence: 1.0},
			Prop:          document.Reference{ID: identifier.New()},
			From:          &from,
			FromPrecision: &yearPrec,
			ToIsUnknown:   true,
		}
		require.NoError(t, c.Validate(), "% -+#.1v", c.Validate())

		isNone := true
		applyPatch := document.TimeIntervalClaimPatch{
			ToIsNone: &isNone,
		}
		errE := applyPatch.Apply(c)
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.True(t, c.ToIsNone)
		assert.False(t, c.ToIsUnknown)
	})

	t.Run("AmountInterval/OpenToUnknown", func(t *testing.T) {
		t.Parallel()

		from := document.Amount("1.5")
		fromPrecision := 0.1
		to := document.Amount("9.5")
		toPrecision := 0.1
		c := &document.AmountIntervalClaim{ //nolint:exhaustruct
			CoreClaim:     document.CoreClaim{ID: identifier.New(), Confidence: 1.0},
			Prop:          document.Reference{ID: identifier.New()},
			From:          &from,
			FromPrecision: &fromPrecision,
			To:            &to,
			ToPrecision:   &toPrecision,
			ToIsOpen:      true,
		}
		require.NoError(t, c.Validate(), "% -+#.1v", c.Validate())

		isUnknown := true
		applyPatch := document.AmountIntervalClaimPatch{
			ToIsUnknown: &isUnknown,
		}
		errE := applyPatch.Apply(c)
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.True(t, c.ToIsUnknown)
		assert.False(t, c.ToIsOpen)
		assert.Nil(t, c.To)
		assert.Nil(t, c.ToPrecision)
	})
}

func TestAmountClaimValidate(t *testing.T) {
	t.Parallel()

	prop := identifier.New()
	id := identifier.New()
	core := document.CoreClaim{
		ID:         id,
		Confidence: 1.0,
	}
	ref := document.Reference{ID: prop}

	valid := []document.AmountClaim{
		{CoreClaim: core, Prop: ref, Amount: document.Amount("0"), Precision: 1},
		{CoreClaim: core, Prop: ref, Amount: document.Amount("-1.5"), Precision: 0.5},
	}
	for _, c := range valid {
		errE := c.Validate()
		require.NoError(t, errE, "% -+#.1v", errE)
	}

	nan := document.AmountClaim{CoreClaim: core, Prop: ref, Amount: document.Amount("abc"), Precision: 1}
	assert.EqualError(t, nan.Validate(), "unable to parse amount")
}

func TestAmountIntervalClaimValidate(t *testing.T) {
	t.Parallel()

	prop := identifier.New()
	id := identifier.New()
	from := document.Amount("1.0")
	fromP := 0.1
	to := document.Amount("9.0")
	toP := 0.1

	valid := &document.AmountIntervalClaim{ //nolint:exhaustruct
		CoreClaim:     document.CoreClaim{ID: id, Confidence: 1.0},
		Prop:          document.Reference{ID: prop},
		From:          &from,
		FromPrecision: &fromP,
		To:            &to,
		ToPrecision:   &toP,
	}
	errE := valid.Validate()
	require.NoError(t, errE, "% -+#.1v", errE)

	// Missing From and no flag set.
	invalid := &document.AmountIntervalClaim{ //nolint:exhaustruct
		CoreClaim:   document.CoreClaim{ID: id, Confidence: 1.0},
		Prop:        document.Reference{ID: prop},
		To:          &to,
		ToPrecision: &toP,
	}
	assert.EqualError(t, invalid.Validate(), "one of From, FromIsUnknown, or FromIsNone must be set")

	// From set with FromIsNone.
	conflicting := &document.AmountIntervalClaim{ //nolint:exhaustruct
		CoreClaim:     document.CoreClaim{ID: id, Confidence: 1.0},
		Prop:          document.Reference{ID: prop},
		From:          &from,
		FromPrecision: &fromP,
		FromIsNone:    true,
		To:            &to,
		ToPrecision:   &toP,
	}
	assert.EqualError(t, conflicting.Validate(), "From must not be set when FromIsUnknown or FromIsNone is true")
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
	claim, errE := p.New(identifier.From("test", "0"))
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
	html := "<p><b>bold</b></p>"

	p := document.HTMLClaimPatch{
		Confidence: &confidence,
		Prop:       &prop,
		HTML:       html,
	}

	// Test New.
	claim, errE := p.New(identifier.From("test", "0"))
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
	newHTML := "<p><i>italic</i></p>"
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
	ts := document.Time("2025-06-15")
	prec := document.TimePrecisionDay

	p := document.TimeClaimPatch{
		Confidence: &confidence,
		Prop:       &prop,
		Time:       &ts,
		Precision:  &prec,
	}

	// Test New.
	claim, errE := p.New(identifier.From("test", "0"))
	require.NoError(t, errE, "% -+#.1v", errE)

	c, ok := claim.(*document.TimeClaim)
	require.True(t, ok)
	assert.Equal(t, ts, c.Time)
	assert.Equal(t, prec, c.Precision)

	// Test JSON roundtrip.
	out, errE := x.MarshalWithoutEscapeHTML(p)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Contains(t, string(out), `"type":"time"`)

	var p2 document.TimeClaimPatch
	errE = x.UnmarshalWithoutUnknownFields(out, &p2)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, p, p2)

	// Test Apply updates the time.
	newTS := document.Time("2026-01-01")
	applyPatch := document.TimeClaimPatch{Time: &newTS}
	errE = applyPatch.Apply(claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, newTS, c.Time)
}

// TestLinkClaimPatch tests LinkClaimPatch New, Apply, and JSON roundtrip.
func TestLinkClaimPatch(t *testing.T) { //nolint:dupl
	t.Parallel()

	prop := identifier.New()
	confidence := document.Confidence(1.0)
	iri := "https://example.com/resource"

	p := document.LinkClaimPatch{
		Confidence: &confidence,
		Prop:       &prop,
		IRI:        iri,
	}

	// Test New.
	claim, errE := p.New(identifier.From("test", "0"))
	require.NoError(t, errE, "% -+#.1v", errE)

	c, ok := claim.(*document.LinkClaim)
	require.True(t, ok)
	assert.Equal(t, iri, c.IRI)

	// Test JSON roundtrip.
	out, errE := x.MarshalWithoutEscapeHTML(p)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Contains(t, string(out), `"type":"link"`)

	var p2 document.LinkClaimPatch
	errE = x.UnmarshalWithoutUnknownFields(out, &p2)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, p, p2)

	// Test Apply updates the IRI.
	newIRI := "https://example.org/other"
	applyPatch := document.LinkClaimPatch{IRI: newIRI}
	errE = applyPatch.Apply(claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, newIRI, c.IRI)
}

// TestReferenceClaimPatch tests ReferenceClaimPatch New, Apply, and JSON roundtrip.
func TestReferenceClaimPatch(t *testing.T) {
	t.Parallel()

	prop := identifier.New()
	target := identifier.New()
	confidence := document.Confidence(1.0)

	p := document.ReferenceClaimPatch{
		Confidence: &confidence,
		Prop:       &prop,
		To:         &target,
	}

	// Test New.
	claim, errE := p.New(identifier.From("test", "0"))
	require.NoError(t, errE, "% -+#.1v", errE)

	c, ok := claim.(*document.ReferenceClaim)
	require.True(t, ok)
	assert.Equal(t, target, c.To.ID)

	// Test JSON roundtrip.
	out, errE := x.MarshalWithoutEscapeHTML(p)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Contains(t, string(out), `"type":"ref"`)

	var p2 document.ReferenceClaimPatch
	errE = x.UnmarshalWithoutUnknownFields(out, &p2)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, p, p2)

	// Test Apply updates the target.
	newTarget := identifier.New()
	applyPatch := document.ReferenceClaimPatch{To: &newTarget}
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
	claim, errE := p.New(identifier.From("test", "0"))
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
	claim, errE := p.New(identifier.From("test", "0"))
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
	claim, errE := p.New(identifier.From("test", "0"))
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
		CoreDocument: document.CoreDocument{ID: docID, Base: base},
	}

	// Add initial claim via AddClaimChange.
	addChange := document.AddClaimChange{ //nolint:exhaustruct
		ID:   claimID,
		Base: id1,
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
	errE = setChange.Validate(base, 0)
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
		CoreDocument: document.CoreDocument{ID: docID, Base: base},
	}

	// Add a claim first.
	addChange := document.AddClaimChange{ //nolint:exhaustruct
		ID:   claimID,
		Base: id1,
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
	errE = removeChange.Validate(base, 0)
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
		document.LinkClaimPatch{Confidence: &conf, Prop: &prop, IRI: "https://example.com"},
		document.ReferenceClaimPatch{Confidence: &conf, Prop: &prop, To: &prop},
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
			ID:   identifier.From("base", "0"),
			Base: []string{"base", "0"},
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

// TestAllChangePatchTypesViaChanges exercises all patch types through Changes JSON roundtrip.
func TestAllChangePatchTypesViaChanges(t *testing.T) {
	t.Parallel()

	base := []string{"TqtRsbk7rTKviW3TJapTim"}
	prop := identifier.New()
	target := identifier.New()
	conf := document.Confidence(1.0)
	amount := document.Amount("5.0")
	precision := 0.1
	from := document.Amount("1.0")
	fromP := 0.1
	to := document.Amount("9.0")
	toP := 0.1
	ts := document.Time("2025-01-01")
	tsPrec := document.TimePrecisionDay
	fromTS := document.Time("2020-01-01")
	fromTSPrec := document.TimePrecisionDay
	toTS := document.Time("2021-01-01")
	toTSPrec := document.TimePrecisionDay

	makeBase := func(i int) []string {
		return append(append([]string{}, base...), string(rune('0'+i))) //nolint:gosec
	}
	makeID := func(i int) identifier.Identifier {
		return identifier.From(makeBase(i)...)
	}

	changes := document.Changes{
		document.AddClaimChange{ //nolint:exhaustruct
			ID:   makeID(0),
			Base: makeBase(0),
			Patch: document.StringClaimPatch{
				Confidence: &conf, Prop: &prop, String: "hello",
			},
		},
		document.AddClaimChange{ //nolint:exhaustruct
			ID:   makeID(1),
			Base: makeBase(1),
			Patch: document.HTMLClaimPatch{
				Confidence: &conf, Prop: &prop, HTML: "<p>hi</p>",
			},
		},
		document.AddClaimChange{ //nolint:exhaustruct
			ID:   makeID(2),
			Base: makeBase(2),
			Patch: document.AmountClaimPatch{
				Confidence: &conf, Prop: &prop, Amount: &amount, Precision: &precision,
			},
		},
		document.AddClaimChange{ //nolint:exhaustruct
			ID:   makeID(3),
			Base: makeBase(3),
			Patch: document.AmountIntervalClaimPatch{
				Confidence: &conf, Prop: &prop,
				From: &from, FromPrecision: &fromP,
				To: &to, ToPrecision: &toP,
			},
		},
		document.AddClaimChange{ //nolint:exhaustruct
			ID:   makeID(4),
			Base: makeBase(4),
			Patch: document.TimeClaimPatch{
				Confidence: &conf, Prop: &prop, Time: &ts, Precision: &tsPrec,
			},
		},
		document.AddClaimChange{ //nolint:exhaustruct
			ID:   makeID(5),
			Base: makeBase(5),
			Patch: document.TimeIntervalClaimPatch{
				Confidence: &conf, Prop: &prop,
				From: &fromTS, FromPrecision: &fromTSPrec,
				To: &toTS, ToPrecision: &toTSPrec,
			},
		},
		document.AddClaimChange{ //nolint:exhaustruct
			ID:   makeID(6),
			Base: makeBase(6),
			Patch: document.LinkClaimPatch{
				Confidence: &conf, Prop: &prop, IRI: "https://example.com",
			},
		},
		document.AddClaimChange{ //nolint:exhaustruct
			ID:   makeID(7),
			Base: makeBase(7),
			Patch: document.ReferenceClaimPatch{
				Confidence: &conf, Prop: &prop, To: &target,
			},
		},
		document.AddClaimChange{ //nolint:exhaustruct
			ID:   makeID(8),
			Base: makeBase(8),
			Patch: document.HasClaimPatch{
				Confidence: &conf, Prop: &prop,
			},
		},
		document.AddClaimChange{ //nolint:exhaustruct
			ID:   makeID(9),
			Base: makeBase(9),
			Patch: document.NoneClaimPatch{
				Confidence: &conf, Prop: &prop,
			},
		},
		document.AddClaimChange{ //nolint:exhaustruct
			ID:   makeID(10),
			Base: makeBase(10),
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
		CoreDocument: document.CoreDocument{ID: docID, Base: base},
	}
	errE = changes.Apply(doc)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, 11, doc.Size())
}

// TestPatchNewIncomplete tests that calling New with incomplete patch returns an error.
func TestPatchNewIncomplete(t *testing.T) {
	t.Parallel()

	t.Run("StringClaimPatch", func(t *testing.T) {
		t.Parallel()
		p := document.StringClaimPatch{}
		_, errE := p.New(identifier.From("base", "0"))
		assert.EqualError(t, errE, "incomplete patch")
	})
	t.Run("HTMLClaimPatch", func(t *testing.T) {
		t.Parallel()
		p := document.HTMLClaimPatch{}
		_, errE := p.New(identifier.From("base", "0"))
		assert.EqualError(t, errE, "incomplete patch")
	})
	t.Run("TimeClaimPatch", func(t *testing.T) {
		t.Parallel()
		p := document.TimeClaimPatch{}
		_, errE := p.New(identifier.From("base", "0"))
		assert.EqualError(t, errE, "incomplete patch")
	})
	t.Run("LinkClaimPatch", func(t *testing.T) {
		t.Parallel()
		p := document.LinkClaimPatch{}
		_, errE := p.New(identifier.From("base", "0"))
		assert.EqualError(t, errE, "incomplete patch")
	})
	t.Run("ReferenceClaimPatch", func(t *testing.T) {
		t.Parallel()
		p := document.ReferenceClaimPatch{}
		_, errE := p.New(identifier.From("base", "0"))
		assert.EqualError(t, errE, "incomplete patch")
	})
	t.Run("HasClaimPatch", func(t *testing.T) {
		t.Parallel()
		p := document.HasClaimPatch{}
		_, errE := p.New(identifier.From("base", "0"))
		assert.EqualError(t, errE, "incomplete patch")
	})
	t.Run("NoneClaimPatch", func(t *testing.T) {
		t.Parallel()
		p := document.NoneClaimPatch{}
		_, errE := p.New(identifier.From("base", "0"))
		assert.EqualError(t, errE, "incomplete patch")
	})
	t.Run("UnknownClaimPatch", func(t *testing.T) {
		t.Parallel()
		p := document.UnknownClaimPatch{}
		_, errE := p.New(identifier.From("base", "0"))
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
		ts := document.Time("2025-01-01")
		p := document.TimeClaimPatch{Time: &ts}
		assert.EqualError(t, p.Apply(wrongClaim), "not time claim")
	})
	t.Run("LinkClaimPatch", func(t *testing.T) {
		t.Parallel()
		p := document.LinkClaimPatch{IRI: "https://example.com"}
		assert.EqualError(t, p.Apply(wrongClaim), "not link claim")
	})
	t.Run("ReferenceClaimPatch", func(t *testing.T) {
		t.Parallel()
		p := document.ReferenceClaimPatch{To: &prop}
		assert.EqualError(t, p.Apply(wrongClaim), "not reference claim")
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

	claim, errE := p.New(identifier.From("test", "0"))
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
	amount := document.Amount("42")
	precision := 1.0

	p := document.AmountClaimPatch{
		Confidence: &confidence,
		Prop:       &prop,
		Amount:     &amount,
		Precision:  &precision,
	}

	claim, errE := p.New(identifier.From("test", "0"))
	require.NoError(t, errE, "% -+#.1v", errE)

	c, ok := claim.(*document.AmountClaim)
	require.True(t, ok)
	assert.Equal(t, amount, c.Amount)

	// Test Apply updates the amount.
	newAmount := document.Amount("99")
	applyPatch := document.AmountClaimPatch{Amount: &newAmount}
	errE = applyPatch.Apply(claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, newAmount, c.Amount)

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
			Time:      "2025-01-01",
			Precision: document.TimePrecisionDay,
		}
	}
	makeRefClaim := func() document.Claim {
		return &document.LinkClaim{
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
	t.Run("LinkClaimPatch/empty", func(t *testing.T) {
		t.Parallel()
		assert.EqualError(t, document.LinkClaimPatch{}.Apply(makeRefClaim()), "empty patch")
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
		CoreDocument: document.CoreDocument{ID: docID, Base: base},
	}

	addChange := document.AddClaimChange{ //nolint:exhaustruct
		ID:   identifier.From(id1...),
		Base: id1,
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

	// An AddClaimChange with wrong ID should fail validation.
	changes := document.Changes{
		document.AddClaimChange{ //nolint:exhaustruct
			ID:   identifier.New(),
			Base: []string{"TqtRsbk7rTKviW3TJapTim", "1"},
			Patch: document.HasClaimPatch{
				Confidence: &conf,
				Prop:       &prop,
			},
		},
	}
	errE := changes.Validate(base)
	assert.EqualError(t, errE, "invalid ID")
}

// TestClaimPatchValidate tests that add and set changes validate their patches, so that
// changes which would deterministically fail when an edit session completes are rejected
// already when they are appended to the session.
func TestClaimPatchValidate(t *testing.T) {
	t.Parallel()

	prop := identifier.New()
	conf := document.Confidence(1.0)
	outOfRange := document.Confidence(2.0)

	// Non-canonical HTML (the browser's innerHTML serialization of a non-breaking space).
	change := document.SetClaimChange{ID: identifier.New(), Patch: document.HTMLClaimPatch{HTML: "<p>a&nbsp;b</p>"}}
	errE := change.Validate(nil, 1)
	assert.EqualError(t, errE, "HTML is not canonical")

	// Canonical HTML (a raw U+00A0 character) passes.
	change = document.SetClaimChange{ID: identifier.New(), Patch: document.HTMLClaimPatch{HTML: "<p>a\u00a0b</p>"}}
	errE = change.Validate(nil, 1)
	require.NoError(t, errE, "% -+#.1v", errE)

	change = document.SetClaimChange{ID: identifier.New(), Patch: document.HTMLClaimPatch{}}
	errE = change.Validate(nil, 1)
	assert.EqualError(t, errE, "empty patch")

	change = document.SetClaimChange{ID: identifier.New(), Patch: document.HasClaimPatch{Confidence: &outOfRange}}
	errE = change.Validate(nil, 1)
	assert.EqualError(t, errE, "confidence out of range [-1, 1]")

	change = document.SetClaimChange{ID: identifier.New(), Patch: document.LinkClaimPatch{IRI: "javascript:alert(1)"}}
	errE = change.Validate(nil, 1)
	assert.EqualError(t, errE, "disallowed URL scheme")

	// AddClaimChange.Validate validates the patch through New.
	base := []string{"TqtRsbk7rTKviW3TJapTim"}
	id1 := []string{"TqtRsbk7rTKviW3TJapTim", "1"}
	addChange := document.AddClaimChange{ //nolint:exhaustruct
		ID:    identifier.From(id1...),
		Base:  id1,
		Patch: document.HTMLClaimPatch{HTML: "<p>x</p>"},
	}
	errE = addChange.Validate(base, 1)
	assert.EqualError(t, errE, "incomplete patch")

	addChange = document.AddClaimChange{ //nolint:exhaustruct
		ID:   identifier.From(id1...),
		Base: id1,
		Patch: document.HTMLClaimPatch{
			Confidence: &conf,
			Prop:       &prop,
			HTML:       "<p>a&nbsp;b</p>",
		},
	}
	errE = addChange.Validate(base, 1)
	assert.EqualError(t, errE, "HTML is not canonical")

	addChange = document.AddClaimChange{ //nolint:exhaustruct
		ID:   identifier.From(id1...),
		Base: id1,
		Patch: document.HTMLClaimPatch{
			Confidence: &conf,
			Prop:       &prop,
			HTML:       "<p>x</p>",
		},
	}
	errE = addChange.Validate(base, 1)
	require.NoError(t, errE, "% -+#.1v", errE)
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
		ID:    identifier.From("base", "0"),
		Base:  []string{"base", "0"},
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
	t.Run("LinkClaimPatch", func(t *testing.T) {
		t.Parallel()
		var p document.LinkClaimPatch
		assert.EqualError(t, x.UnmarshalWithoutUnknownFields([]byte(`{"type":"rel"}`), &p), "invalid type")
	})
	t.Run("ReferenceClaimPatch", func(t *testing.T) {
		t.Parallel()
		var p document.ReferenceClaimPatch
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
		from := document.Amount("1.0")
		fromP := 0.1
		to := document.Amount("9.0")
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
		claim, errE := p.New(identifier.From("test", "0"))
		require.NoError(t, errE, "% -+#.1v", errE)
		c, ok := claim.(*document.AmountIntervalClaim)
		require.True(t, ok)
		return c
	}

	t.Run("set_from_and_precision", func(t *testing.T) {
		t.Parallel()
		claim := newClaim(t)
		newFrom := document.Amount("2.0")
		newFromP := 0.2
		patch := document.AmountIntervalClaimPatch{
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
		newTo := document.Amount("15.0")
		newToP := 0.5
		patch := document.AmountIntervalClaimPatch{
			To:          &newTo,
			ToPrecision: &newToP,
		}
		errE := patch.Apply(claim)
		require.NoError(t, errE, "% -+#.1v", errE)
		require.NotNil(t, claim.To)
		assert.Equal(t, newTo, *claim.To)
	})

	t.Run("set_to_is_open", func(t *testing.T) {
		t.Parallel()
		claim := newClaim(t)
		isOpen := true
		patch := document.AmountIntervalClaimPatch{ToIsOpen: &isOpen}
		errE := patch.Apply(claim)
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.True(t, claim.ToIsOpen)
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
		from := document.Time("2020-01-01")
		fromP := document.TimePrecisionDay
		to := document.Time("2021-01-01")
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
		claim, errE := p.New(identifier.From("test", "0"))
		require.NoError(t, errE, "% -+#.1v", errE)
		c, ok := claim.(*document.TimeIntervalClaim)
		require.True(t, ok)
		return c
	}

	t.Run("set_from_and_precision", func(t *testing.T) {
		t.Parallel()
		claim := newClaim(t)
		newFrom := document.Time("2022-06-01")
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
		newTo := document.Time("2023-12-31")
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

	t.Run("set_to_is_open", func(t *testing.T) {
		t.Parallel()
		claim := newClaim(t)
		isOpen := true
		patch := document.TimeIntervalClaimPatch{ToIsOpen: &isOpen}
		errE := patch.Apply(claim)
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.True(t, claim.ToIsOpen)
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

// TestLinkClaimPatchApplyConfidenceOnly tests that LinkClaimPatch.Apply accepts a confidence-only patch.
func TestLinkClaimPatchApplyConfidenceOnly(t *testing.T) {
	t.Parallel()

	prop := identifier.New()
	confidence := document.HighConfidence

	fullPatch := document.LinkClaimPatch{
		Confidence: &confidence,
		Prop:       &prop,
		IRI:        "https://example.com",
	}
	claim, errE := fullPatch.New(identifier.From("test", "0"))
	require.NoError(t, errE, "% -+#.1v", errE)

	newConfidence := document.LowConfidence
	confidenceOnlyPatch := document.LinkClaimPatch{
		Confidence: &newConfidence,
	}
	errE = confidenceOnlyPatch.Apply(claim)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, document.LowConfidence, claim.GetConfidence()) //nolint:testifylint
}
