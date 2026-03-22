package store_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/document"
	internalStore "gitlab.com/peerdb/peerdb/internal/store"
)

func TestAddAppendsNew(t *testing.T) {
	t.Parallel()

	sourceA := identifier.New()
	claim1 := identifier.New()
	prop1 := identifier.New()

	m := &internalStore.DocumentMetadata{}
	m.AddInverseRelations([]internalStore.InverseRelation{
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

	m := &internalStore.DocumentMetadata{ //nolint:exhaustruct
		InverseRelations: []internalStore.InverseRelation{
			{Claim: claimA, Source: sourceA, Prop: propA, Target: identifier.Identifier{}, Confidence: document.HighConfidence},
		},
	}

	m.AddInverseRelations([]internalStore.InverseRelation{
		{Claim: claimB, Source: sourceB, Prop: propB, Target: identifier.Identifier{}, Confidence: document.HighConfidence},
	})

	assert.Len(t, m.InverseRelations, 2)
	assert.Equal(t, claimA, m.InverseRelations[0].Claim)
	assert.Equal(t, claimB, m.InverseRelations[1].Claim)
}

func TestAddEmpty(t *testing.T) {
	t.Parallel()

	m := &internalStore.DocumentMetadata{ //nolint:exhaustruct
		InverseRelations: []internalStore.InverseRelation{
			{Claim: identifier.New(), Source: identifier.New(), Prop: identifier.New(), Target: identifier.Identifier{}, Confidence: document.HighConfidence},
		},
	}

	m.AddInverseRelations(nil)
	assert.Len(t, m.InverseRelations, 1)

	m.AddInverseRelations([]internalStore.InverseRelation{})
	assert.Len(t, m.InverseRelations, 1)
}

func TestAddSkipsDuplicateClaimID(t *testing.T) {
	t.Parallel()

	sourceA := identifier.New()
	claim1 := identifier.New()
	prop1 := identifier.New()

	m := &internalStore.DocumentMetadata{ //nolint:exhaustruct
		InverseRelations: []internalStore.InverseRelation{
			{Claim: claim1, Source: sourceA, Prop: prop1, Target: identifier.Identifier{}, Confidence: document.HighConfidence},
		},
	}

	// Adding the same claim again should be a no-op.
	m.AddInverseRelations([]internalStore.InverseRelation{
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

	m := &internalStore.DocumentMetadata{}

	relations := []internalStore.InverseRelation{
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

	m := &internalStore.DocumentMetadata{ //nolint:exhaustruct
		InverseRelations: []internalStore.InverseRelation{
			{Claim: claim1, Source: sourceA, Prop: prop1, Target: identifier.Identifier{}, Confidence: document.HighConfidence},
		},
	}

	// Add a mix of existing and new claims.
	m.AddInverseRelations([]internalStore.InverseRelation{
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

	m := &internalStore.DocumentMetadata{ //nolint:exhaustruct
		InverseRelations: []internalStore.InverseRelation{
			{Claim: claimA, Source: sourceA, Prop: propA, Target: identifier.Identifier{}, Confidence: document.HighConfidence},
			{Claim: claimB, Source: sourceA, Prop: propA, Target: identifier.Identifier{}, Confidence: document.HighConfidence},
		},
	}

	// Remove only claimA.
	m.RemoveInverseRelations([]internalStore.InverseRelation{
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

	m := &internalStore.DocumentMetadata{ //nolint:exhaustruct
		InverseRelations: []internalStore.InverseRelation{
			{Claim: claimA, Source: sourceA, Prop: propA, Target: identifier.Identifier{}, Confidence: document.HighConfidence},
		},
	}

	m.RemoveInverseRelations([]internalStore.InverseRelation{
		{Claim: claimA, Source: sourceA, Prop: propA, Target: identifier.Identifier{}, Confidence: document.HighConfidence},
	})

	assert.Nil(t, m.InverseRelations)
}

func TestRemoveFromEmpty(t *testing.T) {
	t.Parallel()

	m := &internalStore.DocumentMetadata{}
	m.RemoveInverseRelations([]internalStore.InverseRelation{
		{Claim: identifier.New(), Source: identifier.New(), Prop: identifier.New(), Target: identifier.Identifier{}, Confidence: document.HighConfidence},
	})

	assert.Nil(t, m.InverseRelations)
}

func TestRemoveEmpty(t *testing.T) {
	t.Parallel()

	claimA := identifier.New()
	m := &internalStore.DocumentMetadata{ //nolint:exhaustruct
		InverseRelations: []internalStore.InverseRelation{
			{Claim: claimA, Source: identifier.New(), Prop: identifier.New(), Target: identifier.Identifier{}, Confidence: document.HighConfidence},
		},
	}

	m.RemoveInverseRelations(nil)
	assert.Len(t, m.InverseRelations, 1)

	m.RemoveInverseRelations([]internalStore.InverseRelation{})
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

	m := &internalStore.DocumentMetadata{ //nolint:exhaustruct
		InverseRelations: []internalStore.InverseRelation{
			{Claim: claimA, Source: sourceA, Prop: propA, Target: identifier.Identifier{}, Confidence: document.HighConfidence},
			{Claim: claimB, Source: sourceB, Prop: propB, Target: identifier.Identifier{}, Confidence: document.HighConfidence},
		},
	}

	// Remove only sourceA's claim.
	m.RemoveInverseRelations([]internalStore.InverseRelation{
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

	m := &internalStore.DocumentMetadata{}

	// Add two claims.
	m.AddInverseRelations([]internalStore.InverseRelation{
		{Claim: claimA, Source: sourceA, Prop: propA, Target: identifier.Identifier{}, Confidence: document.HighConfidence},
		{Claim: claimB, Source: sourceA, Prop: propA, Target: identifier.Identifier{}, Confidence: document.HighConfidence},
	})
	assert.Len(t, m.InverseRelations, 2)

	// Remove one.
	m.RemoveInverseRelations([]internalStore.InverseRelation{
		{Claim: claimA, Source: sourceA, Prop: propA, Target: identifier.Identifier{}, Confidence: document.HighConfidence},
	})
	assert.Len(t, m.InverseRelations, 1)
	assert.Equal(t, claimB, m.InverseRelations[0].Claim)

	// Remove the other.
	m.RemoveInverseRelations([]internalStore.InverseRelation{
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

	m := &internalStore.DocumentMetadata{}

	// Add from source A.
	m.AddInverseRelations([]internalStore.InverseRelation{
		{Claim: sharedClaimID, Source: sourceA, Prop: prop1, Target: targetX, Confidence: document.HighConfidence},
	})
	assert.Len(t, m.InverseRelations, 1)

	// Add from source B with the same claim ID but different source.
	// Should not be deduplicated because the (source, claim) pair differs.
	m.AddInverseRelations([]internalStore.InverseRelation{
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

	m := &internalStore.DocumentMetadata{}

	ir := internalStore.InverseRelation{Claim: claim1, Source: sourceA, Prop: prop1, Target: targetX, Confidence: document.HighConfidence}

	m.AddInverseRelations([]internalStore.InverseRelation{ir})
	assert.Len(t, m.InverseRelations, 1)

	// Adding the exact same relation again should be deduplicated.
	m.AddInverseRelations([]internalStore.InverseRelation{ir})
	assert.Len(t, m.InverseRelations, 1)
}

func TestTimeMarshalJSON(t *testing.T) {
	t.Parallel()

	ts := time.Date(2024, 3, 15, 10, 30, 45, 123000000, time.UTC)
	st := internalStore.Time(ts)

	b, err := x.MarshalWithoutEscapeHTML(st)
	require.NoError(t, err)
	assert.Equal(t, `"2024-03-15T10:30:45.123Z"`, string(b))
}

func TestTimeMarshalJSONZeroMillis(t *testing.T) {
	t.Parallel()

	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	st := internalStore.Time(ts)

	b, err := x.MarshalWithoutEscapeHTML(st)
	require.NoError(t, err)
	assert.Equal(t, `"2024-01-01T00:00:00.000Z"`, string(b))
}

func TestTimeUnmarshalJSON(t *testing.T) {
	t.Parallel()

	var st internalStore.Time
	err := x.UnmarshalWithoutUnknownFields([]byte(`"2024-03-15T10:30:45.123Z"`), &st)
	require.NoError(t, err)

	expected := time.Date(2024, 3, 15, 10, 30, 45, 123000000, time.UTC)
	assert.True(t, expected.Equal(time.Time(st)))
}

func TestTimeUnmarshalJSONNull(t *testing.T) {
	t.Parallel()

	var st internalStore.Time
	err := x.UnmarshalWithoutUnknownFields([]byte(`null`), &st)
	require.NoError(t, err)
	assert.True(t, time.Time(st).IsZero())
}

func TestTimeUnmarshalJSONInvalid(t *testing.T) {
	t.Parallel()

	var st internalStore.Time
	err := x.UnmarshalWithoutUnknownFields([]byte(`12345`), &st)
	assert.EqualError(t, err, "Time.UnmarshalJSON: input is not a JSON string")
}

func TestTimeUnmarshalJSONBadFormat(t *testing.T) {
	t.Parallel()

	var st internalStore.Time
	err := x.UnmarshalWithoutUnknownFields([]byte(`"not-a-date"`), &st)
	assert.EqualError(t, err, `parsing time "not-a-date" as "2006-01-02T15:04:05.000Z07:00": cannot parse "not-a-date" as "2006"`)
}

func TestTimeMarshalUnmarshalRoundTrip(t *testing.T) {
	t.Parallel()

	ts := time.Date(2025, 12, 31, 23, 59, 59, 999000000, time.UTC)
	original := internalStore.Time(ts)

	b, err := x.MarshalWithoutEscapeHTML(original)
	require.NoError(t, err)

	var decoded internalStore.Time
	err = x.UnmarshalWithoutUnknownFields(b, &decoded)
	require.NoError(t, err)

	assert.True(t, time.Time(original).Equal(time.Time(decoded)))
}

func TestTimeMarshalWithTimezone(t *testing.T) {
	t.Parallel()

	loc := time.FixedZone("EST", -5*60*60)
	ts := time.Date(2024, 6, 15, 14, 30, 0, 0, loc)
	st := internalStore.Time(ts)

	b, err := x.MarshalWithoutEscapeHTML(st)
	require.NoError(t, err)
	assert.Equal(t, `"2024-06-15T14:30:00.000-05:00"`, string(b))
}

func TestCarryOverNil(t *testing.T) {
	t.Parallel()

	m := &internalStore.DocumentMetadata{ //nolint:exhaustruct
		InverseRelations: []internalStore.InverseRelation{
			{Claim: identifier.New(), Source: identifier.New(), Prop: identifier.New(), Target: identifier.Identifier{}, Confidence: document.HighConfidence},
		},
	}

	m.CarryOver(nil)

	// InverseRelations should remain unchanged when old is nil.
	assert.Len(t, m.InverseRelations, 1)
}

func TestCarryOverCopiesInverseRelations(t *testing.T) {
	t.Parallel()

	claim1 := identifier.New()
	source1 := identifier.New()
	prop1 := identifier.New()

	old := &internalStore.DocumentMetadata{ //nolint:exhaustruct
		InverseRelations: []internalStore.InverseRelation{
			{Claim: claim1, Source: source1, Prop: prop1, Target: identifier.Identifier{}, Confidence: document.HighConfidence},
		},
	}

	m := &internalStore.DocumentMetadata{}
	m.CarryOver(old)

	require.Len(t, m.InverseRelations, 1)
	assert.Equal(t, claim1, m.InverseRelations[0].Claim)
	assert.Equal(t, source1, m.InverseRelations[0].Source)
}

func TestCarryOverReplacesExisting(t *testing.T) {
	t.Parallel()

	oldRelation := internalStore.InverseRelation{
		Claim: identifier.New(), Source: identifier.New(), Prop: identifier.New(), Target: identifier.Identifier{}, Confidence: document.HighConfidence,
	}

	old := &internalStore.DocumentMetadata{ //nolint:exhaustruct
		InverseRelations: []internalStore.InverseRelation{oldRelation},
	}

	newRelation := internalStore.InverseRelation{
		Claim: identifier.New(), Source: identifier.New(), Prop: identifier.New(), Target: identifier.Identifier{}, Confidence: document.HighConfidence,
	}

	m := &internalStore.DocumentMetadata{ //nolint:exhaustruct
		InverseRelations: []internalStore.InverseRelation{newRelation},
	}

	m.CarryOver(old)

	// CarryOver replaces InverseRelations with the old one.
	require.Len(t, m.InverseRelations, 1)
	assert.Equal(t, oldRelation.Claim, m.InverseRelations[0].Claim)
}

func TestCarryOverEmptyOld(t *testing.T) {
	t.Parallel()

	old := &internalStore.DocumentMetadata{}

	m := &internalStore.DocumentMetadata{ //nolint:exhaustruct
		InverseRelations: []internalStore.InverseRelation{
			{Claim: identifier.New(), Source: identifier.New(), Prop: identifier.New(), Target: identifier.Identifier{}, Confidence: document.HighConfidence},
		},
	}

	m.CarryOver(old)

	// CarryOver sets InverseRelations to old's value (nil).
	assert.Nil(t, m.InverseRelations)
}
