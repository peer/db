package store_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/document"
	"gitlab.com/peerdb/peerdb/store"
)

// lvl and lvl2 are two visibility level names used to exercise the per-level inverse-relation buckets.
const (
	lvl  = "all"
	lvl2 = "editor"
)

// newIR creates an InverseRelation with the given fields.
func newIR(claim, source, sourceProp, targetProp, target identifier.Identifier) store.InverseRelation {
	return store.InverseRelation{
		InverseRelationKey: store.InverseRelationKey{Claim: claim, Source: source, TargetProp: targetProp},
		SourceProp:         sourceProp,
		Target:             target,
		Confidence:         document.HighConfidence,
	}
}

// newIRNew creates an InverseRelation with random IDs for fields that don't matter in the test.
func newIRNew() store.InverseRelation {
	p := identifier.New()
	return newIR(identifier.New(), identifier.New(), p, p, identifier.Identifier{})
}

func TestAddAppendsNew(t *testing.T) {
	t.Parallel()

	sourceA := identifier.New()
	claim1 := identifier.New()
	prop1 := identifier.New()

	m := &store.DocumentMetadata{}
	m.AddInverseRelations(lvl, []store.InverseRelation{
		newIR(claim1, sourceA, prop1, prop1, identifier.Identifier{}),
	})

	assert.Len(t, m.InverseRelations[lvl], 1)
	assert.Equal(t, claim1, m.InverseRelations[lvl][0].Claim)
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
		InverseRelations: map[string][]store.InverseRelation{
			lvl: {newIR(claimA, sourceA, propA, propA, identifier.Identifier{})},
		},
	}

	m.AddInverseRelations(lvl, []store.InverseRelation{
		newIR(claimB, sourceB, propB, propB, identifier.Identifier{}),
	})

	assert.Len(t, m.InverseRelations[lvl], 2)
	assert.Equal(t, claimA, m.InverseRelations[lvl][0].Claim)
	assert.Equal(t, claimB, m.InverseRelations[lvl][1].Claim)
}

func TestAddEmpty(t *testing.T) {
	t.Parallel()

	m := &store.DocumentMetadata{ //nolint:exhaustruct
		InverseRelations: map[string][]store.InverseRelation{lvl: {newIRNew()}},
	}

	m.AddInverseRelations(lvl, nil)
	assert.Len(t, m.InverseRelations[lvl], 1)

	m.AddInverseRelations(lvl, []store.InverseRelation{})
	assert.Len(t, m.InverseRelations[lvl], 1)
}

func TestAddSkipsDuplicateClaimID(t *testing.T) {
	t.Parallel()

	sourceA := identifier.New()
	claim1 := identifier.New()
	prop1 := identifier.New()

	ir := newIR(claim1, sourceA, prop1, prop1, identifier.Identifier{})

	m := &store.DocumentMetadata{ //nolint:exhaustruct
		InverseRelations: map[string][]store.InverseRelation{lvl: {ir}},
	}

	// Adding the same claim again should be a no-op.
	m.AddInverseRelations(lvl, []store.InverseRelation{ir})

	assert.Len(t, m.InverseRelations[lvl], 1)
}

func TestAddIdempotentOnRetry(t *testing.T) {
	t.Parallel()

	sourceA := identifier.New()
	claim1 := identifier.New()
	claim2 := identifier.New()
	prop1 := identifier.New()

	m := &store.DocumentMetadata{}

	relations := []store.InverseRelation{
		newIR(claim1, sourceA, prop1, prop1, identifier.Identifier{}),
		newIR(claim2, sourceA, prop1, prop1, identifier.Identifier{}),
	}

	// Simulate first call succeeding.
	m.AddInverseRelations(lvl, relations)
	assert.Len(t, m.InverseRelations[lvl], 2)

	// Simulate retry calling Add again with the same relations.
	m.AddInverseRelations(lvl, relations)
	assert.Len(t, m.InverseRelations[lvl], 2)
}

func TestAddMixedNewAndExisting(t *testing.T) {
	t.Parallel()

	sourceA := identifier.New()
	claim1 := identifier.New()
	claim2 := identifier.New()
	prop1 := identifier.New()

	ir1 := newIR(claim1, sourceA, prop1, prop1, identifier.Identifier{})
	ir2 := newIR(claim2, sourceA, prop1, prop1, identifier.Identifier{})

	m := &store.DocumentMetadata{ //nolint:exhaustruct
		InverseRelations: map[string][]store.InverseRelation{lvl: {ir1}},
	}

	// Add a mix of existing and new claims.
	m.AddInverseRelations(lvl, []store.InverseRelation{ir1, ir2})

	// Only the new claim should be added.
	assert.Len(t, m.InverseRelations[lvl], 2)
	assert.Equal(t, claim1, m.InverseRelations[lvl][0].Claim)
	assert.Equal(t, claim2, m.InverseRelations[lvl][1].Claim)
}

func TestRemoveByClaimID(t *testing.T) {
	t.Parallel()

	sourceA := identifier.New()
	claimA := identifier.New()
	claimB := identifier.New()
	propA := identifier.New()

	irA := newIR(claimA, sourceA, propA, propA, identifier.Identifier{})
	irB := newIR(claimB, sourceA, propA, propA, identifier.Identifier{})

	m := &store.DocumentMetadata{ //nolint:exhaustruct
		InverseRelations: map[string][]store.InverseRelation{lvl: {irA, irB}},
	}

	// Remove only claimA.
	m.RemoveInverseRelations(lvl, []store.InverseRelation{irA})

	assert.Len(t, m.InverseRelations[lvl], 1)
	assert.Equal(t, claimB, m.InverseRelations[lvl][0].Claim)
}

func TestRemoveAllSetsNil(t *testing.T) {
	t.Parallel()

	sourceA := identifier.New()
	claimA := identifier.New()
	propA := identifier.New()

	ir := newIR(claimA, sourceA, propA, propA, identifier.Identifier{})

	m := &store.DocumentMetadata{ //nolint:exhaustruct
		InverseRelations: map[string][]store.InverseRelation{lvl: {ir}},
	}

	m.RemoveInverseRelations(lvl, []store.InverseRelation{ir})

	// The level's set is dropped and, being the only level, the whole map is reset to nil.
	assert.Nil(t, m.InverseRelations)
}

func TestRemoveFromEmpty(t *testing.T) {
	t.Parallel()

	m := &store.DocumentMetadata{}
	m.RemoveInverseRelations(lvl, []store.InverseRelation{newIRNew()})

	assert.Nil(t, m.InverseRelations)
}

func TestRemoveEmpty(t *testing.T) {
	t.Parallel()

	claimA := identifier.New()
	ir := newIR(claimA, identifier.New(), identifier.New(), identifier.New(), identifier.Identifier{})

	m := &store.DocumentMetadata{ //nolint:exhaustruct
		InverseRelations: map[string][]store.InverseRelation{lvl: {ir}},
	}

	m.RemoveInverseRelations(lvl, nil)
	assert.Len(t, m.InverseRelations[lvl], 1)

	m.RemoveInverseRelations(lvl, []store.InverseRelation{})
	assert.Len(t, m.InverseRelations[lvl], 1)
}

func TestRemovePreservesOtherSources(t *testing.T) {
	t.Parallel()

	sourceA := identifier.New()
	sourceB := identifier.New()
	claimA := identifier.New()
	claimB := identifier.New()
	propA := identifier.New()
	propB := identifier.New()

	irA := newIR(claimA, sourceA, propA, propA, identifier.Identifier{})
	irB := newIR(claimB, sourceB, propB, propB, identifier.Identifier{})

	m := &store.DocumentMetadata{ //nolint:exhaustruct
		InverseRelations: map[string][]store.InverseRelation{lvl: {irA, irB}},
	}

	// Remove only sourceA's claim.
	m.RemoveInverseRelations(lvl, []store.InverseRelation{irA})

	assert.Len(t, m.InverseRelations[lvl], 1)
	assert.Equal(t, claimB, m.InverseRelations[lvl][0].Claim)
	assert.Equal(t, sourceB, m.InverseRelations[lvl][0].Source)
}

func TestAddThenRemove(t *testing.T) {
	t.Parallel()

	sourceA := identifier.New()
	claimA := identifier.New()
	claimB := identifier.New()
	propA := identifier.New()

	irA := newIR(claimA, sourceA, propA, propA, identifier.Identifier{})
	irB := newIR(claimB, sourceA, propA, propA, identifier.Identifier{})

	m := &store.DocumentMetadata{}

	// Add two claims.
	m.AddInverseRelations(lvl, []store.InverseRelation{irA, irB})
	assert.Len(t, m.InverseRelations[lvl], 2)

	// Remove one.
	m.RemoveInverseRelations(lvl, []store.InverseRelation{irA})
	assert.Len(t, m.InverseRelations[lvl], 1)
	assert.Equal(t, claimB, m.InverseRelations[lvl][0].Claim)

	// Remove the other.
	m.RemoveInverseRelations(lvl, []store.InverseRelation{irB})
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
	m.AddInverseRelations(lvl, []store.InverseRelation{
		newIR(sharedClaimID, sourceA, prop1, prop1, targetX),
	})
	assert.Len(t, m.InverseRelations[lvl], 1)

	// Add from source B with the same claim ID but different source.
	// Should not be deduplicated because the (source, claim) pair differs.
	m.AddInverseRelations(lvl, []store.InverseRelation{
		newIR(sharedClaimID, sourceB, prop1, prop1, targetX),
	})
	assert.Len(t, m.InverseRelations[lvl], 2)
}

func TestAddSameClaimIDSameSourceIdempotent(t *testing.T) {
	t.Parallel()

	sourceA := identifier.New()
	targetX := identifier.New()
	claim1 := identifier.New()
	prop1 := identifier.New()

	m := &store.DocumentMetadata{}

	ir := newIR(claim1, sourceA, prop1, prop1, targetX)

	m.AddInverseRelations(lvl, []store.InverseRelation{ir})
	assert.Len(t, m.InverseRelations[lvl], 1)

	// Adding the exact same relation again should be deduplicated.
	m.AddInverseRelations(lvl, []store.InverseRelation{ir})
	assert.Len(t, m.InverseRelations[lvl], 1)
}

func TestAddRemovePerLevelSeparation(t *testing.T) {
	t.Parallel()

	source := identifier.New()
	prop := identifier.New()
	irA := newIR(identifier.New(), source, prop, prop, identifier.Identifier{})
	irB := newIR(identifier.New(), source, prop, prop, identifier.Identifier{})

	m := &store.DocumentMetadata{}

	// Different relations at different levels are kept separate.
	m.AddInverseRelations(lvl, []store.InverseRelation{irA})
	m.AddInverseRelations(lvl2, []store.InverseRelation{irA, irB})
	assert.Len(t, m.InverseRelations[lvl], 1)
	assert.Len(t, m.InverseRelations[lvl2], 2)

	// Removing from one level does not touch the other; the emptied level's key is dropped while the other remains.
	m.RemoveInverseRelations(lvl, []store.InverseRelation{irA})
	_, ok := m.InverseRelations[lvl]
	assert.False(t, ok, "emptied level key should be dropped")
	assert.Len(t, m.InverseRelations[lvl2], 2)
	assert.NotNil(t, m.InverseRelations)
}

func TestCarryOverNil(t *testing.T) {
	t.Parallel()

	m := &store.DocumentMetadata{ //nolint:exhaustruct
		InverseRelations: map[string][]store.InverseRelation{lvl: {newIRNew()}},
	}

	m.CarryOver(nil)

	// InverseRelations should remain unchanged when old is nil.
	assert.Len(t, m.InverseRelations[lvl], 1)
}

func TestCarryOverCopiesInverseRelations(t *testing.T) {
	t.Parallel()

	claim1 := identifier.New()
	source1 := identifier.New()
	prop1 := identifier.New()

	old := &store.DocumentMetadata{ //nolint:exhaustruct
		InverseRelations: map[string][]store.InverseRelation{
			lvl: {newIR(claim1, source1, prop1, prop1, identifier.Identifier{})},
		},
	}

	m := &store.DocumentMetadata{}
	m.CarryOver(old)

	require.Len(t, m.InverseRelations[lvl], 1)
	assert.Equal(t, claim1, m.InverseRelations[lvl][0].Claim)
	assert.Equal(t, source1, m.InverseRelations[lvl][0].Source)
}

func TestCarryOverReplacesExisting(t *testing.T) {
	t.Parallel()

	oldRelation := newIRNew()
	old := &store.DocumentMetadata{ //nolint:exhaustruct
		InverseRelations: map[string][]store.InverseRelation{lvl: {oldRelation}},
	}

	newRelation := newIRNew()
	m := &store.DocumentMetadata{ //nolint:exhaustruct
		InverseRelations: map[string][]store.InverseRelation{lvl: {newRelation}},
	}

	m.CarryOver(old)

	// CarryOver replaces InverseRelations with the old one.
	require.Len(t, m.InverseRelations[lvl], 1)
	assert.Equal(t, oldRelation.Claim, m.InverseRelations[lvl][0].Claim)
}

func TestCarryOverEmptyOld(t *testing.T) {
	t.Parallel()

	old := &store.DocumentMetadata{}

	m := &store.DocumentMetadata{ //nolint:exhaustruct
		InverseRelations: map[string][]store.InverseRelation{lvl: {newIRNew()}},
	}

	m.CarryOver(old)

	// CarryOver sets InverseRelations to old's value (nil).
	assert.Nil(t, m.InverseRelations)
}

func TestTimeMarshalJSON(t *testing.T) {
	t.Parallel()

	ts := time.Date(2024, 3, 15, 10, 30, 45, 123000000, time.UTC)
	st := store.Time(ts)

	b, err := x.MarshalWithoutEscapeHTML(st)
	require.NoError(t, err)
	assert.Equal(t, `"2024-03-15T10:30:45.123Z"`, string(b))
}

func TestTimeMarshalJSONZeroMillis(t *testing.T) {
	t.Parallel()

	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	st := store.Time(ts)

	b, err := x.MarshalWithoutEscapeHTML(st)
	require.NoError(t, err)
	assert.Equal(t, `"2024-01-01T00:00:00.000Z"`, string(b))
}

func TestTimeUnmarshalJSON(t *testing.T) {
	t.Parallel()

	var st store.Time
	err := x.UnmarshalWithoutUnknownFields([]byte(`"2024-03-15T10:30:45.123Z"`), &st)
	require.NoError(t, err)

	expected := time.Date(2024, 3, 15, 10, 30, 45, 123000000, time.UTC)
	assert.True(t, expected.Equal(time.Time(st)))
}

func TestTimeUnmarshalJSONNull(t *testing.T) {
	t.Parallel()

	var st store.Time
	err := x.UnmarshalWithoutUnknownFields([]byte(`null`), &st)
	require.NoError(t, err)
	assert.True(t, time.Time(st).IsZero())
}

func TestTimeUnmarshalJSONInvalid(t *testing.T) {
	t.Parallel()

	var st store.Time
	err := x.UnmarshalWithoutUnknownFields([]byte(`12345`), &st)
	assert.EqualError(t, err, "Time.UnmarshalJSON: input is not a JSON string")
}

func TestTimeUnmarshalJSONBadFormat(t *testing.T) {
	t.Parallel()

	var st store.Time
	err := x.UnmarshalWithoutUnknownFields([]byte(`"not-a-date"`), &st)
	assert.EqualError(t, err, `parsing time "not-a-date" as "2006-01-02T15:04:05.000Z07:00": cannot parse "not-a-date" as "2006"`)
}

func TestTimeMarshalUnmarshalRoundTrip(t *testing.T) {
	t.Parallel()

	ts := time.Date(2025, 12, 31, 23, 59, 59, 999000000, time.UTC)
	original := store.Time(ts)

	b, err := x.MarshalWithoutEscapeHTML(original)
	require.NoError(t, err)

	var decoded store.Time
	err = x.UnmarshalWithoutUnknownFields(b, &decoded)
	require.NoError(t, err)

	assert.True(t, time.Time(original).Equal(time.Time(decoded)))
}

func TestTimeMarshalWithTimezone(t *testing.T) {
	t.Parallel()

	loc := time.FixedZone("EST", -5*60*60)
	ts := time.Date(2024, 6, 15, 14, 30, 0, 0, loc)
	st := store.Time(ts)

	b, err := x.MarshalWithoutEscapeHTML(st)
	require.NoError(t, err)
	assert.Equal(t, `"2024-06-15T14:30:00.000-05:00"`, string(b))
}

// Pinned identifier used by Version tests so that parsing and formatting are
// fully deterministic.
const versionTestChangesetStr = "11111111111111111111AB"

func TestVersionStringWithRevision(t *testing.T) {
	t.Parallel()

	cs, errE := identifier.MaybeString(versionTestChangesetStr)
	require.NoError(t, errE, "% -+#.1v", errE)

	v := store.Version{Changeset: cs, Revision: 7}
	assert.Equal(t, versionTestChangesetStr+"-7", v.String())
}

func TestVersionStringRevisionZero(t *testing.T) {
	t.Parallel()

	cs, errE := identifier.MaybeString(versionTestChangesetStr)
	require.NoError(t, errE, "% -+#.1v", errE)

	v := store.Version{Changeset: cs, Revision: 0}
	// Zero revision still serializes with explicit "-0" suffix.
	assert.Equal(t, versionTestChangesetStr+"-0", v.String())
}

func TestVersionMarshalUnmarshalRoundTrip(t *testing.T) {
	t.Parallel()

	cs, errE := identifier.MaybeString(versionTestChangesetStr)
	require.NoError(t, errE, "% -+#.1v", errE)

	for _, rev := range []int64{0, 1, 2, 100, 1<<62 - 1} {
		original := store.Version{Changeset: cs, Revision: rev}

		b, err := original.MarshalText()
		require.NoError(t, err)

		var decoded store.Version
		err = decoded.UnmarshalText(b)
		require.NoError(t, err)

		assert.Equal(t, original, decoded)
	}
}

func TestVersionFromStringWithoutDash(t *testing.T) {
	t.Parallel()

	// No "-" -> revision defaults to 0 (meaning "latest" in read paths).
	v, errE := store.VersionFromString(versionTestChangesetStr)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, versionTestChangesetStr, v.Changeset.String())
	assert.Equal(t, int64(0), v.Revision)
}

func TestVersionFromStringWithExplicitRevision(t *testing.T) {
	t.Parallel()

	v, errE := store.VersionFromString(versionTestChangesetStr + "-3")
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, versionTestChangesetStr, v.Changeset.String())
	assert.Equal(t, int64(3), v.Revision)
}

func TestVersionFromStringExplicitZero(t *testing.T) {
	t.Parallel()

	v, errE := store.VersionFromString(versionTestChangesetStr + "-0")
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, versionTestChangesetStr, v.Changeset.String())
	assert.Equal(t, int64(0), v.Revision)
}

func TestVersionFromStringEmptyRevisionAfterDash(t *testing.T) {
	t.Parallel()

	// "<cs>-" parses revisionStr as "" which fails strconv.ParseInt.
	_, errE := store.VersionFromString(versionTestChangesetStr + "-")
	require.Error(t, errE)
}

func TestVersionFromStringDoubleDash(t *testing.T) {
	t.Parallel()

	// strings.Cut splits on the first "-" so revisionStr is "-3", which parses
	// to a negative integer and is rejected.
	_, errE := store.VersionFromString(versionTestChangesetStr + "--3")
	require.Error(t, errE)
	assert.Contains(t, errE.Error(), "invalid version revision")
}

func TestVersionFromStringBadChangeset(t *testing.T) {
	t.Parallel()

	_, errE := store.VersionFromString("not-an-identifier")
	require.Error(t, errE)
}

func TestVersionFromStringEmpty(t *testing.T) {
	t.Parallel()

	_, errE := store.VersionFromString("")
	require.Error(t, errE)
}

func TestVersionFromStringOverflow(t *testing.T) {
	t.Parallel()

	// Revision larger than int64.
	_, errE := store.VersionFromString(versionTestChangesetStr + "-99999999999999999999")
	require.Error(t, errE)
}

func TestVersionFromStringNegativeRevision(t *testing.T) {
	t.Parallel()

	_, errE := store.VersionFromString(versionTestChangesetStr + "-" + "-1")
	require.Error(t, errE)
	// "-1" -> invalid (we reject negative).
	assert.Contains(t, errE.Error(), "invalid version revision")
}

func TestVersionUnmarshalTextRoundTripThroughMarshalText(t *testing.T) {
	t.Parallel()

	cs, errE := identifier.MaybeString(versionTestChangesetStr)
	require.NoError(t, errE, "% -+#.1v", errE)

	original := store.Version{Changeset: cs, Revision: 42}

	b, err := original.MarshalText()
	require.NoError(t, err)
	assert.Equal(t, versionTestChangesetStr+"-42", string(b))

	var decoded store.Version
	err = decoded.UnmarshalText(b)
	require.NoError(t, err)
	assert.Equal(t, original, decoded)
}
