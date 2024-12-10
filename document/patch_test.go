package document_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/document"
)

func TestPatchJSON(t *testing.T) {
	t.Parallel()

	id1 := identifier.MustFromString("LpcGdCUThc22mhuBwQJQ5Z")
	id2 := identifier.MustFromString("AyNNP5CVsSx3w9b75erF1m")
	prop1 := identifier.MustFromString("XkbTJqwFCFkfoxMBXow4HU")
	prop2 := identifier.MustFromString("3EL2nZdWVbw85XG1zTH2o5")
	confidence := document.Confidence(1.0)
	amount := 42.1
	value := "foobar"
	unit := document.AmountUnitCelsius
	changes := document.Changes{
		document.AddClaimChange{
			ID:    id1,
			Under: nil,
			Patch: document.AmountClaimPatch{
				Confidence: &confidence,
				Prop:       &prop1,
				Amount:     &amount,
				Unit:       &unit,
			},
		},
		document.AddClaimChange{
			ID:    id2,
			Under: &id1,
			Patch: document.IdentifierClaimPatch{
				Confidence: &confidence,
				Prop:       &prop2,
				Value:      &value,
			},
		},
	}
	out, errE := x.MarshalWithoutEscapeHTML(changes)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, `[{"type":"add","id":"LpcGdCUThc22mhuBwQJQ5Z","patch":{"type":"amount","confidence":1,"prop":"XkbTJqwFCFkfoxMBXow4HU","amount":42.1,"unit":"°C"}},{"type":"add","under":"LpcGdCUThc22mhuBwQJQ5Z","id":"AyNNP5CVsSx3w9b75erF1m","patch":{"type":"id","confidence":1,"prop":"3EL2nZdWVbw85XG1zTH2o5","value":"foobar"}}]`, string(out)) //nolint:lll

	var changes2 document.Changes
	errE = x.UnmarshalWithoutUnknownFields(out, &changes2)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, changes, changes2)

	id := identifier.New()
	doc := &document.D{
		CoreDocument: document.CoreDocument{
			ID:    id,
			Score: 1.0,
		},
	}
	base := identifier.MustFromString("TqtRsbk7rTKviW3TJapTim")
	errE = changes.Validate(context.Background(), base)
	require.NoError(t, errE, "% -+#.1v", errE)
	errE = changes.Apply(doc)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, &document.D{
		CoreDocument: document.CoreDocument{
			ID:    id,
			Score: 1.0,
		},
		Claims: &document.ClaimTypes{
			Amount: []document.AmountClaim{
				{
					CoreClaim: document.CoreClaim{
						ID:         id1,
						Confidence: 1.0,
						Meta: &document.ClaimTypes{
							Identifier: []document.IdentifierClaim{
								{
									CoreClaim: document.CoreClaim{
										ID:         id2,
										Confidence: 1.0,
									},
									Prop: document.Reference{
										ID: &prop2,
									},
									Value: value,
								},
							},
						},
					},
					Prop: document.Reference{
						ID: &prop1,
					},
					Amount: amount,
					Unit:   unit,
				},
			},
		},
	}, doc)
}
