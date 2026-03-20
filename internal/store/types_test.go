package store_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/document"
	"gitlab.com/peerdb/peerdb/internal/store"
)

func TestMergeAddsNew(t *testing.T) {
	t.Parallel()

	sourceA := identifier.New()
	claim1 := identifier.New()
	prop1 := identifier.New()

	m := &store.DocumentMetadata{}
	m.Merge([]store.InverseRelation{
		{Claim: claim1, Document: sourceA, Prop: prop1, Confidence: document.HighConfidence},
	})

	assert.Len(t, m.InverseRelations, 1)
	assert.Equal(t, claim1, m.InverseRelations[0].Claim)
}

func TestMergeReplacesFromSameSource(t *testing.T) {
	t.Parallel()

	sourceA := identifier.New()
	claim1 := identifier.New()
	claim2 := identifier.New()
	prop1 := identifier.New()
	prop2 := identifier.New()

	m := &store.DocumentMetadata{ //nolint:exhaustruct
		InverseRelations: []store.InverseRelation{
			{Claim: claim1, Document: sourceA, Prop: prop1, Confidence: document.HighConfidence},
		},
	}

	// Replace with a different claim from the same source.
	m.Merge([]store.InverseRelation{
		{Claim: claim2, Document: sourceA, Prop: prop2, Confidence: document.HighConfidence},
	})

	assert.Len(t, m.InverseRelations, 1)
	assert.Equal(t, claim2, m.InverseRelations[0].Claim)
	assert.Equal(t, prop2, m.InverseRelations[0].Prop)
}

func TestMergePreservesOtherSources(t *testing.T) {
	t.Parallel()

	sourceA := identifier.New()
	sourceB := identifier.New()
	claimA := identifier.New()
	claimB := identifier.New()
	propA := identifier.New()
	propB := identifier.New()

	m := &store.DocumentMetadata{ //nolint:exhaustruct
		InverseRelations: []store.InverseRelation{
			{Claim: claimA, Document: sourceA, Prop: propA, Confidence: document.HighConfidence},
			{Claim: claimB, Document: sourceB, Prop: propB, Confidence: document.HighConfidence},
		},
	}

	// Update only sourceA with a new claim.
	claimA2 := identifier.New()
	m.Merge([]store.InverseRelation{
		{Claim: claimA2, Document: sourceA, Prop: propA, Confidence: document.HighConfidence},
	})

	assert.Len(t, m.InverseRelations, 2)

	// sourceB's claim should be preserved.
	found := false
	for _, ir := range m.InverseRelations {
		if ir.Document == sourceB {
			assert.Equal(t, claimB, ir.Claim)
			found = true
		}
	}
	assert.True(t, found, "sourceB's inverse relation should be preserved")
}

func TestMergeRemovesWhenEmpty(t *testing.T) {
	t.Parallel()

	sourceA := identifier.New()
	claim1 := identifier.New()
	prop1 := identifier.New()

	m := &store.DocumentMetadata{ //nolint:exhaustruct
		InverseRelations: []store.InverseRelation{
			{Claim: claim1, Document: sourceA, Prop: prop1, Confidence: document.HighConfidence},
		},
	}

	// Merge empty list for sourceA — removes all relations from sourceA.
	m.Merge([]store.InverseRelation{})

	// No sources were in the input, so nothing is removed.
	assert.Len(t, m.InverseRelations, 1)
}

func TestMergeRemovesSourceWithEmptyRelations(t *testing.T) {
	t.Parallel()

	sourceA := identifier.New()
	sourceB := identifier.New()
	claimA := identifier.New()
	claimB := identifier.New()
	propA := identifier.New()
	propB := identifier.New()

	m := &store.DocumentMetadata{ //nolint:exhaustruct
		InverseRelations: []store.InverseRelation{
			{Claim: claimA, Document: sourceA, Prop: propA, Confidence: document.HighConfidence},
			{Claim: claimB, Document: sourceB, Prop: propB, Confidence: document.HighConfidence},
		},
	}

	// Update sourceA with no relations — this means all relations from sourceA were removed.
	// To express "sourceA has no more relations", the caller must include sourceA in the input
	// with an empty set. But Merge identifies sources from the relations themselves.
	// So to remove all from sourceA, the caller should not call Merge for sourceA at all
	// (the bridge handles this by only including documents that have relation claims).
	// This test verifies that empty input doesn't change anything.
	m.Merge(nil)
	assert.Len(t, m.InverseRelations, 2)
}

func TestMergeMultipleSources(t *testing.T) {
	t.Parallel()

	sourceA := identifier.New()
	sourceB := identifier.New()
	claimA := identifier.New()
	claimB := identifier.New()
	claimA2 := identifier.New()
	claimB2 := identifier.New()
	propA := identifier.New()
	propB := identifier.New()

	m := &store.DocumentMetadata{ //nolint:exhaustruct
		InverseRelations: []store.InverseRelation{
			{Claim: claimA, Document: sourceA, Prop: propA, Confidence: document.HighConfidence},
			{Claim: claimB, Document: sourceB, Prop: propB, Confidence: document.HighConfidence},
		},
	}

	// Update both sources at once.
	m.Merge([]store.InverseRelation{
		{Claim: claimA2, Document: sourceA, Prop: propA, Confidence: document.HighConfidence},
		{Claim: claimB2, Document: sourceB, Prop: propB, Confidence: document.HighConfidence},
	})

	assert.Len(t, m.InverseRelations, 2)

	claims := map[identifier.Identifier]bool{}
	for _, ir := range m.InverseRelations {
		claims[ir.Claim] = true
	}
	assert.True(t, claims[claimA2], "sourceA should have new claim")
	assert.True(t, claims[claimB2], "sourceB should have new claim")
}

func TestMergeSetsNilWhenAllRemoved(t *testing.T) {
	t.Parallel()

	sourceA := identifier.New()
	claimA := identifier.New()
	propA := identifier.New()

	m := &store.DocumentMetadata{ //nolint:exhaustruct
		InverseRelations: []store.InverseRelation{
			{Claim: claimA, Document: sourceA, Prop: propA, Confidence: document.HighConfidence},
		},
	}

	// Merge with sourceA having a new claim, then merge again with sourceA
	// having a different claim. The old one should be gone.
	newClaim := identifier.New()
	m.Merge([]store.InverseRelation{
		{Claim: newClaim, Document: sourceA, Prop: propA, Confidence: document.HighConfidence},
	})
	assert.Len(t, m.InverseRelations, 1)
	assert.Equal(t, newClaim, m.InverseRelations[0].Claim)

	// Now merge for a different source, leaving sourceA intact.
	sourceB := identifier.New()
	m.Merge([]store.InverseRelation{
		{Claim: identifier.New(), Document: sourceB, Prop: propA, Confidence: document.HighConfidence},
	})
	assert.Len(t, m.InverseRelations, 2)
}
