package store_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/document"
	"gitlab.com/peerdb/peerdb/internal/store"
)

func TestAddAppendsNew(t *testing.T) {
	t.Parallel()

	sourceA := identifier.New()
	claim1 := identifier.New()
	prop1 := identifier.New()

	m := &store.DocumentMetadata{}
	m.AddInverseRelations([]store.InverseRelation{
		{Claim: claim1, Source: sourceA, Prop: prop1, Target: identifier.Identifier{}, Confidence: document.HighConfidence},
	})

	assert.Len(t, m.InverseRelations, 1)
	assert.Equal(t, claim1, m.InverseRelations[0].Claim)
}

func TestAddAppendsToExisting(t *testing.T) {
	t.Parallel()

	sourceA := identifier.New()
	sourceB := identifier.New()
	claimA := identifier.New()
	claimB := identifier.New()
	propA := identifier.New()
	propB := identifier.New()

	m := &store.DocumentMetadata{ //nolint:exhaustruct
		InverseRelations: []store.InverseRelation{
			{Claim: claimA, Source: sourceA, Prop: propA, Target: identifier.Identifier{}, Confidence: document.HighConfidence},
		},
	}

	m.AddInverseRelations([]store.InverseRelation{
		{Claim: claimB, Source: sourceB, Prop: propB, Target: identifier.Identifier{}, Confidence: document.HighConfidence},
	})

	assert.Len(t, m.InverseRelations, 2)
	assert.Equal(t, claimA, m.InverseRelations[0].Claim)
	assert.Equal(t, claimB, m.InverseRelations[1].Claim)
}

func TestAddEmpty(t *testing.T) {
	t.Parallel()

	m := &store.DocumentMetadata{ //nolint:exhaustruct
		InverseRelations: []store.InverseRelation{
			{Claim: identifier.New(), Source: identifier.New(), Prop: identifier.New(), Target: identifier.Identifier{}, Confidence: document.HighConfidence},
		},
	}

	m.AddInverseRelations(nil)
	assert.Len(t, m.InverseRelations, 1)

	m.AddInverseRelations([]store.InverseRelation{})
	assert.Len(t, m.InverseRelations, 1)
}

func TestAddSkipsDuplicateClaimID(t *testing.T) {
	t.Parallel()

	sourceA := identifier.New()
	claim1 := identifier.New()
	prop1 := identifier.New()

	m := &store.DocumentMetadata{ //nolint:exhaustruct
		InverseRelations: []store.InverseRelation{
			{Claim: claim1, Source: sourceA, Prop: prop1, Target: identifier.Identifier{}, Confidence: document.HighConfidence},
		},
	}

	// Adding the same claim again should be a no-op.
	m.AddInverseRelations([]store.InverseRelation{
		{Claim: claim1, Source: sourceA, Prop: prop1, Target: identifier.Identifier{}, Confidence: document.HighConfidence},
	})

	assert.Len(t, m.InverseRelations, 1)
}

func TestAddIdempotentOnRetry(t *testing.T) {
	t.Parallel()

	sourceA := identifier.New()
	claim1 := identifier.New()
	claim2 := identifier.New()
	prop1 := identifier.New()

	m := &store.DocumentMetadata{}

	relations := []store.InverseRelation{
		{Claim: claim1, Source: sourceA, Prop: prop1, Target: identifier.Identifier{}, Confidence: document.HighConfidence},
		{Claim: claim2, Source: sourceA, Prop: prop1, Target: identifier.Identifier{}, Confidence: document.HighConfidence},
	}

	// Simulate first call succeeding.
	m.AddInverseRelations(relations)
	assert.Len(t, m.InverseRelations, 2)

	// Simulate retry calling Add again with the same relations.
	m.AddInverseRelations(relations)
	assert.Len(t, m.InverseRelations, 2)
}

func TestAddMixedNewAndExisting(t *testing.T) {
	t.Parallel()

	sourceA := identifier.New()
	claim1 := identifier.New()
	claim2 := identifier.New()
	prop1 := identifier.New()

	m := &store.DocumentMetadata{ //nolint:exhaustruct
		InverseRelations: []store.InverseRelation{
			{Claim: claim1, Source: sourceA, Prop: prop1, Target: identifier.Identifier{}, Confidence: document.HighConfidence},
		},
	}

	// Add a mix of existing and new claims.
	m.AddInverseRelations([]store.InverseRelation{
		{Claim: claim1, Source: sourceA, Prop: prop1, Target: identifier.Identifier{}, Confidence: document.HighConfidence},
		{Claim: claim2, Source: sourceA, Prop: prop1, Target: identifier.Identifier{}, Confidence: document.HighConfidence},
	})

	// Only the new claim should be added.
	assert.Len(t, m.InverseRelations, 2)
	assert.Equal(t, claim1, m.InverseRelations[0].Claim)
	assert.Equal(t, claim2, m.InverseRelations[1].Claim)
}

func TestRemoveByClaimID(t *testing.T) {
	t.Parallel()

	sourceA := identifier.New()
	claimA := identifier.New()
	claimB := identifier.New()
	propA := identifier.New()

	m := &store.DocumentMetadata{ //nolint:exhaustruct
		InverseRelations: []store.InverseRelation{
			{Claim: claimA, Source: sourceA, Prop: propA, Target: identifier.Identifier{}, Confidence: document.HighConfidence},
			{Claim: claimB, Source: sourceA, Prop: propA, Target: identifier.Identifier{}, Confidence: document.HighConfidence},
		},
	}

	// Remove only claimA.
	m.RemoveInverseRelations([]store.InverseRelation{
		{Claim: claimA, Source: sourceA, Prop: propA, Target: identifier.Identifier{}, Confidence: document.HighConfidence},
	})

	assert.Len(t, m.InverseRelations, 1)
	assert.Equal(t, claimB, m.InverseRelations[0].Claim)
}

func TestRemoveAllSetsNil(t *testing.T) {
	t.Parallel()

	sourceA := identifier.New()
	claimA := identifier.New()
	propA := identifier.New()

	m := &store.DocumentMetadata{ //nolint:exhaustruct
		InverseRelations: []store.InverseRelation{
			{Claim: claimA, Source: sourceA, Prop: propA, Target: identifier.Identifier{}, Confidence: document.HighConfidence},
		},
	}

	m.RemoveInverseRelations([]store.InverseRelation{
		{Claim: claimA, Source: sourceA, Prop: propA, Target: identifier.Identifier{}, Confidence: document.HighConfidence},
	})

	assert.Nil(t, m.InverseRelations)
}

func TestRemoveFromEmpty(t *testing.T) {
	t.Parallel()

	m := &store.DocumentMetadata{}
	m.RemoveInverseRelations([]store.InverseRelation{
		{Claim: identifier.New(), Source: identifier.New(), Prop: identifier.New(), Target: identifier.Identifier{}, Confidence: document.HighConfidence},
	})

	assert.Nil(t, m.InverseRelations)
}

func TestRemoveEmpty(t *testing.T) {
	t.Parallel()

	claimA := identifier.New()
	m := &store.DocumentMetadata{ //nolint:exhaustruct
		InverseRelations: []store.InverseRelation{
			{Claim: claimA, Source: identifier.New(), Prop: identifier.New(), Target: identifier.Identifier{}, Confidence: document.HighConfidence},
		},
	}

	m.RemoveInverseRelations(nil)
	assert.Len(t, m.InverseRelations, 1)

	m.RemoveInverseRelations([]store.InverseRelation{})
	assert.Len(t, m.InverseRelations, 1)
}

func TestRemovePreservesOtherSources(t *testing.T) {
	t.Parallel()

	sourceA := identifier.New()
	sourceB := identifier.New()
	claimA := identifier.New()
	claimB := identifier.New()
	propA := identifier.New()
	propB := identifier.New()

	m := &store.DocumentMetadata{ //nolint:exhaustruct
		InverseRelations: []store.InverseRelation{
			{Claim: claimA, Source: sourceA, Prop: propA, Target: identifier.Identifier{}, Confidence: document.HighConfidence},
			{Claim: claimB, Source: sourceB, Prop: propB, Target: identifier.Identifier{}, Confidence: document.HighConfidence},
		},
	}

	// Remove only sourceA's claim.
	m.RemoveInverseRelations([]store.InverseRelation{
		{Claim: claimA, Source: sourceA, Prop: propA, Target: identifier.Identifier{}, Confidence: document.HighConfidence},
	})

	assert.Len(t, m.InverseRelations, 1)
	assert.Equal(t, claimB, m.InverseRelations[0].Claim)
	assert.Equal(t, sourceB, m.InverseRelations[0].Source)
}

func TestAddThenRemove(t *testing.T) {
	t.Parallel()

	sourceA := identifier.New()
	claimA := identifier.New()
	claimB := identifier.New()
	propA := identifier.New()

	m := &store.DocumentMetadata{}

	// Add two claims.
	m.AddInverseRelations([]store.InverseRelation{
		{Claim: claimA, Source: sourceA, Prop: propA, Target: identifier.Identifier{}, Confidence: document.HighConfidence},
		{Claim: claimB, Source: sourceA, Prop: propA, Target: identifier.Identifier{}, Confidence: document.HighConfidence},
	})
	assert.Len(t, m.InverseRelations, 2)

	// Remove one.
	m.RemoveInverseRelations([]store.InverseRelation{
		{Claim: claimA, Source: sourceA, Prop: propA, Target: identifier.Identifier{}, Confidence: document.HighConfidence},
	})
	assert.Len(t, m.InverseRelations, 1)
	assert.Equal(t, claimB, m.InverseRelations[0].Claim)

	// Remove the other.
	m.RemoveInverseRelations([]store.InverseRelation{
		{Claim: claimB, Source: sourceA, Prop: propA, Target: identifier.Identifier{}, Confidence: document.HighConfidence},
	})
	assert.Nil(t, m.InverseRelations)
}

func TestAddSameClaimIDDifferentSources(t *testing.T) {
	t.Parallel()

	// Two different source documents happen to use the same claim ID.
	sharedClaimID := identifier.New()
	sourceA := identifier.New()
	sourceB := identifier.New()
	targetX := identifier.New()
	prop1 := identifier.New()

	m := &store.DocumentMetadata{}

	// Add from source A.
	m.AddInverseRelations([]store.InverseRelation{
		{Claim: sharedClaimID, Source: sourceA, Prop: prop1, Target: targetX, Confidence: document.HighConfidence},
	})
	assert.Len(t, m.InverseRelations, 1)

	// Add from source B with the same claim ID but different source.
	// Should not be deduplicated because the (source, claim) pair differs.
	m.AddInverseRelations([]store.InverseRelation{
		{Claim: sharedClaimID, Source: sourceB, Prop: prop1, Target: targetX, Confidence: document.HighConfidence},
	})
	assert.Len(t, m.InverseRelations, 2)
}

func TestAddSameClaimIDSameSourceIdempotent(t *testing.T) {
	t.Parallel()

	sourceA := identifier.New()
	targetX := identifier.New()
	claim1 := identifier.New()
	prop1 := identifier.New()

	m := &store.DocumentMetadata{}

	ir := store.InverseRelation{Claim: claim1, Source: sourceA, Prop: prop1, Target: targetX, Confidence: document.HighConfidence}

	m.AddInverseRelations([]store.InverseRelation{ir})
	assert.Len(t, m.InverseRelations, 1)

	// Adding the exact same relation again should be deduplicated.
	m.AddInverseRelations([]store.InverseRelation{ir})
	assert.Len(t, m.InverseRelations, 1)
}
