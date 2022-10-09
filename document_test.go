package search_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"gitlab.com/peerdb/search"
	"gitlab.com/peerdb/search/identifier"
)

func TestTimestampMarshal(t *testing.T) {
	tests := []struct {
		timestamp string
		unix      int64
	}{
		{`"2006-12-04T12:34:45Z"`, 1165235685},
		{`"0206-12-04T12:34:45Z"`, -55637321115},
		{`"0001-12-04T12:34:45Z"`, -62106434715},
		{`"20006-12-04T12:34:45Z"`, 569190371685},
		{`"0000-12-04T12:34:45Z"`, -62137970715},
		{`"-0001-12-04T12:34:45Z"`, -62169593115},
		{`"-0206-12-04T12:34:45Z"`, -68638706715},
		{`"-2006-12-04T12:34:45Z"`, -125441263515},
		{`"-20006-12-04T12:34:45Z"`, -693466399515},
		{`"-239999999-01-01T00:00:00Z"`, -7573730615596800},
	}
	for _, test := range tests {
		t.Run(test.timestamp, func(t *testing.T) {
			var timestamp search.Timestamp
			in := []byte(test.timestamp)
			err := json.Unmarshal(in, &timestamp)
			assert.NoError(t, err)
			assert.Equal(t, test.unix, time.Time(timestamp).Unix())
			out, err := json.Marshal(timestamp)
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
			ID:         id,
			Confidence: 1.0,
		},
		Prop: search.GetCorePropertyReference("ARTICLE"),
	})
	assert.NoError(t, err)
	assert.Equal(t, search.Document{
		Active: &search.ClaimTypes{
			NoValue: search.NoValueClaims{
				{
					CoreClaim: search.CoreClaim{
						ID:         id,
						Confidence: 1.0,
					},
					Prop: search.GetCorePropertyReference("ARTICLE"),
				},
			},
		},
	}, doc)
	claim := doc.GetByID(id)
	assert.Equal(t, &search.NoValueClaim{
		CoreClaim: search.CoreClaim{
			ID:         id,
			Confidence: 1.0,
		},
		Prop: search.GetCorePropertyReference("ARTICLE"),
	}, claim)
	claims := doc.Get(search.GetCorePropertyID("ARTICLE"))
	assert.Equal(t, []search.Claim{
		&search.NoValueClaim{
			CoreClaim: search.CoreClaim{
				ID:         id,
				Confidence: 1.0,
			},
			Prop: search.GetCorePropertyReference("ARTICLE"),
		},
	}, claims)
	claim = doc.RemoveByID(id)
	assert.Equal(t, &search.NoValueClaim{
		CoreClaim: search.CoreClaim{
			ID:         id,
			Confidence: 1.0,
		},
		Prop: search.GetCorePropertyReference("ARTICLE"),
	}, claim)
	assert.Equal(t, search.Document{}, doc)

	id2 := search.Identifier(identifier.NewRandom())

	err = claim.AddMeta(&search.UnknownValueClaim{
		CoreClaim: search.CoreClaim{
			ID:         id2,
			Confidence: 1.0,
		},
		Prop: search.GetCorePropertyReference("ARTICLE"),
	})
	assert.NoError(t, err)
	assert.Equal(t, &search.NoValueClaim{
		CoreClaim: search.CoreClaim{
			ID:         id,
			Confidence: 1.0,
			Meta: &search.ClaimTypes{
				UnknownValue: search.UnknownValueClaims{
					{
						CoreClaim: search.CoreClaim{
							ID:         id2,
							Confidence: 1.0,
						},
						Prop: search.GetCorePropertyReference("ARTICLE"),
					},
				},
			},
		},
		Prop: search.GetCorePropertyReference("ARTICLE"),
	}, claim)
	metaClaim := claim.GetMetaByID(id2)
	assert.Equal(t, &search.UnknownValueClaim{
		CoreClaim: search.CoreClaim{
			ID:         id2,
			Confidence: 1.0,
		},
		Prop: search.GetCorePropertyReference("ARTICLE"),
	}, metaClaim)
	metaClaim = claim.RemoveMetaByID(id2)
	assert.Equal(t, &search.UnknownValueClaim{
		CoreClaim: search.CoreClaim{
			ID:         id2,
			Confidence: 1.0,
		},
		Prop: search.GetCorePropertyReference("ARTICLE"),
	}, metaClaim)
	assert.Equal(t, &search.NoValueClaim{
		CoreClaim: search.CoreClaim{
			ID:         id,
			Confidence: 1.0,
		},
		Prop: search.GetCorePropertyReference("ARTICLE"),
	}, claim)
}
