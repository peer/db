package document_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/document"
)

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
						Meta: &document.ClaimTypes{
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
	from := document.Timestamp("2020-01-01")
	fromPrecision := document.TimePrecisionDay
	to := document.Timestamp("2021-01-01")
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
