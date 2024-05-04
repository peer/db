package document_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/document"
)

func TestTimestampMarshal(t *testing.T) {
	t.Parallel()

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
		test := test
		t.Run(test.timestamp, func(t *testing.T) {
			t.Parallel()

			var timestamp document.Timestamp
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
	t.Parallel()

	doc := document.D{}                //nolint:exhaustruct
	assert.Equal(t, document.D{}, doc) //nolint:exhaustruct

	id := identifier.New()

	err := doc.Add(&document.NoValueClaim{
		CoreClaim: document.CoreClaim{
			ID:         id,
			Confidence: 1.0,
		},
		Prop: document.GetCorePropertyReference("ARTICLE"),
	})
	assert.NoError(t, err)
	assert.Equal(t, document.D{ //nolint:exhaustruct
		Claims: &document.ClaimTypes{
			NoValue: document.NoValueClaims{
				{
					CoreClaim: document.CoreClaim{
						ID:         id,
						Confidence: 1.0,
					},
					Prop: document.GetCorePropertyReference("ARTICLE"),
				},
			},
		},
	}, doc)
	claim := doc.GetByID(id)
	assert.Equal(t, &document.NoValueClaim{
		CoreClaim: document.CoreClaim{
			ID:         id,
			Confidence: 1.0,
		},
		Prop: document.GetCorePropertyReference("ARTICLE"),
	}, claim)
	claims := doc.Get(document.GetCorePropertyID("ARTICLE"))
	assert.Equal(t, []document.Claim{
		&document.NoValueClaim{
			CoreClaim: document.CoreClaim{
				ID:         id,
				Confidence: 1.0,
			},
			Prop: document.GetCorePropertyReference("ARTICLE"),
		},
	}, claims)
	claim = doc.RemoveByID(id)
	assert.Equal(t, &document.NoValueClaim{
		CoreClaim: document.CoreClaim{
			ID:         id,
			Confidence: 1.0,
		},
		Prop: document.GetCorePropertyReference("ARTICLE"),
	}, claim)
	assert.Equal(t, document.D{}, doc) //nolint:exhaustruct

	id2 := identifier.New()

	err = claim.Add(&document.UnknownValueClaim{
		CoreClaim: document.CoreClaim{
			ID:         id2,
			Confidence: 1.0,
		},
		Prop: document.GetCorePropertyReference("ARTICLE"),
	})
	assert.NoError(t, err)
	assert.Equal(t, &document.NoValueClaim{
		CoreClaim: document.CoreClaim{
			ID:         id,
			Confidence: 1.0,
			Meta: &document.ClaimTypes{
				UnknownValue: document.UnknownValueClaims{
					{
						CoreClaim: document.CoreClaim{
							ID:         id2,
							Confidence: 1.0,
						},
						Prop: document.GetCorePropertyReference("ARTICLE"),
					},
				},
			},
		},
		Prop: document.GetCorePropertyReference("ARTICLE"),
	}, claim)
	metaClaim := claim.GetByID(id2)
	assert.Equal(t, &document.UnknownValueClaim{
		CoreClaim: document.CoreClaim{
			ID:         id2,
			Confidence: 1.0,
		},
		Prop: document.GetCorePropertyReference("ARTICLE"),
	}, metaClaim)
	metaClaim = claim.RemoveByID(id2)
	assert.Equal(t, &document.UnknownValueClaim{
		CoreClaim: document.CoreClaim{
			ID:         id2,
			Confidence: 1.0,
		},
		Prop: document.GetCorePropertyReference("ARTICLE"),
	}, metaClaim)
	assert.Equal(t, &document.NoValueClaim{
		CoreClaim: document.CoreClaim{
			ID:         id,
			Confidence: 1.0,
		},
		Prop: document.GetCorePropertyReference("ARTICLE"),
	}, claim)
}
