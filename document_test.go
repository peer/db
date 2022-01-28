package search_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	"gitlab.com/peerdb/search"
	"gitlab.com/peerdb/search/identifier"
)

func TestTimestampMarshal(t *testing.T) {
	tests := []string{
		`"2006-12-04T12:34:45Z"`,
		`"0206-12-04T12:34:45Z"`,
		`"0001-12-04T12:34:45Z"`,
		`"20006-12-04T12:34:45Z"`,
		`"0000-12-04T12:34:45Z"`,
		`"-0001-12-04T12:34:45Z"`,
		`"-0206-12-04T12:34:45Z"`,
		`"-2006-12-04T12:34:45Z"`,
		`"-20006-12-04T12:34:45Z"`,
	}
	for _, test := range tests {
		t.Run(test, func(t *testing.T) {
			var time search.Timestamp
			in := []byte(test)
			err := json.Unmarshal(in, &time)
			assert.NoError(t, err)
			out, err := json.Marshal(time)
			assert.NoError(t, err)
			assert.Equal(t, in, out)
		})
	}
}

func TestDocument(t *testing.T) {
	doc := search.Document{}
	assert.Equal(t, search.Document{}, doc)

	id := search.Identifier(identifier.NewRandom())

	err := doc.Add(&search.NoValueClaim{
		CoreClaim: search.CoreClaim{
			ID: id,
		},
		Prop: search.GetStandardPropertyReference("ARTICLE"),
	})
	assert.NoError(t, err)
	assert.Equal(t, search.Document{
		Active: &search.ClaimTypes{
			NoValue: search.NoValueClaims{
				{
					CoreClaim: search.CoreClaim{
						ID: id,
					},
					Prop: search.GetStandardPropertyReference("ARTICLE"),
				},
			},
		},
	}, doc)
	claim := doc.GetByID(id)
	assert.Equal(t, &search.NoValueClaim{
		CoreClaim: search.CoreClaim{
			ID: id,
		},
		Prop: search.GetStandardPropertyReference("ARTICLE"),
	}, claim)
	claims := doc.Get(search.GetStandardPropertyID("ARTICLE"))
	assert.Equal(t, []search.Claim{
		&search.NoValueClaim{
			CoreClaim: search.CoreClaim{
				ID: id,
			},
			Prop: search.GetStandardPropertyReference("ARTICLE"),
		},
	}, claims)
	claim = doc.RemoveByID(id)
	assert.Equal(t, &search.NoValueClaim{
		CoreClaim: search.CoreClaim{
			ID: id,
		},
		Prop: search.GetStandardPropertyReference("ARTICLE"),
	}, claim)
	assert.Equal(t, search.Document{}, doc)

	id2 := search.Identifier(identifier.NewRandom())

	err = claim.AddMeta(&search.UnknownValueClaim{
		CoreClaim: search.CoreClaim{
			ID: id2,
		},
		Prop: search.GetStandardPropertyReference("ARTICLE"),
	})
	assert.NoError(t, err)
	assert.Equal(t, &search.NoValueClaim{
		CoreClaim: search.CoreClaim{
			ID: id,
			Meta: &search.ClaimTypes{
				UnknownValue: search.UnknownValueClaims{
					{
						CoreClaim: search.CoreClaim{
							ID: id2,
						},
						Prop: search.GetStandardPropertyReference("ARTICLE"),
					},
				},
			},
		},
		Prop: search.GetStandardPropertyReference("ARTICLE"),
	}, claim)
	metaClaim := claim.GetMetaByID(id2)
	assert.Equal(t, &search.UnknownValueClaim{
		CoreClaim: search.CoreClaim{
			ID: id2,
		},
		Prop: search.GetStandardPropertyReference("ARTICLE"),
	}, metaClaim)
	metaClaim = claim.RemoveMetaByID(id2)
	assert.Equal(t, &search.UnknownValueClaim{
		CoreClaim: search.CoreClaim{
			ID: id2,
		},
		Prop: search.GetStandardPropertyReference("ARTICLE"),
	}, metaClaim)
	assert.Equal(t, &search.NoValueClaim{
		CoreClaim: search.CoreClaim{
			ID: id,
		},
		Prop: search.GetStandardPropertyReference("ARTICLE"),
	}, claim)
}
