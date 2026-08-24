package document_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/document"
	internalCore "gitlab.com/peerdb/peerdb/internal/core"
)

func instanceOfClaim(confidence document.Confidence, to identifier.Identifier) document.ReferenceClaim {
	return document.ReferenceClaim{
		CoreClaim: document.CoreClaim{
			ID:         identifier.New(),
			Confidence: confidence,
			Sub:        nil,
		},
		Prop: document.Reference{ID: internalCore.InstanceOfPropID},
		To:   document.Reference{ID: to},
	}
}

func TestInstanceOf(t *testing.T) {
	t.Parallel()

	ties := sortIdentifiers(identifier.New(), identifier.New())
	high := identifier.New()
	low := identifier.New()
	excluded := identifier.New()

	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: identifier.New(), Base: nil},
		Claims: &document.ClaimTypes{
			Reference: document.ReferenceClaims{
				// Unordered on purpose: the IDs are returned by decreasing confidence, ties sorted by ID,
				// with the duplicate ranking by its highest confidence and the claim below LowConfidence
				// left out.
				instanceOfClaim(document.LowConfidence, low),
				instanceOfClaim(document.MediumConfidence, ties[1]),
				instanceOfClaim(document.HighConfidence, high),
				instanceOfClaim(document.MediumConfidence, ties[0]),
				instanceOfClaim(document.LowConfidence, high),
				instanceOfClaim(document.LowConfidence-0.01, excluded),
			},
		},
	}

	assert.Equal(t, []identifier.Identifier{high, ties[0], ties[1], low}, doc.InstanceOf())

	empty := &document.D{
		CoreDocument: document.CoreDocument{ID: identifier.New(), Base: nil},
	}
	assert.Nil(t, empty.InstanceOf())
}

func sortIdentifiers(ids ...identifier.Identifier) []identifier.Identifier {
	slices.SortFunc(ids, internalCore.CompareIdentifiers)
	return ids
}
