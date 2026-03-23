package store_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/tozd/identifier"

	internalStore "gitlab.com/peerdb/peerdb/internal/store"
	"gitlab.com/peerdb/peerdb/store"
)

// testCommitMetadata implements store.ChangesetID so that auto-committing
// view methods use the caller-supplied changeset ID.
type testCommitMetadata struct {
	internalStore.TestMetadata

	ID identifier.Identifier
}

// ChangesetID returns the caller-supplied changeset ID.
func (m testCommitMetadata) ChangesetID() identifier.Identifier {
	return m.ID
}

func TestChangesetIDInsert(t *testing.T) {
	t.Parallel()

	ctx, s, channelContents := initDatabase[
		*internalStore.TestData, *internalStore.TestMetadata, *internalStore.TestMetadata,
		*internalStore.TestMetadata, *testCommitMetadata, *internalStore.TestPatch,
	](t, "jsonb")

	expectedID := identifier.New()
	changesetID := identifier.New()

	commitMeta := &testCommitMetadata{
		TestMetadata: internalStore.TestMetadata{Metadata: "commit"},
		ID:           changesetID,
	}

	insertVersion, errE := s.Insert(
		ctx, expectedID,
		&internalStore.TestData{Data: 1, Patch: false},
		&internalStore.TestMetadata{Metadata: "m"},
		commitMeta,
	)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, int64(1), insertVersion.Revision)
	assert.Equal(t, changesetID, insertVersion.Changeset)

	// Verify through GetLatest.
	data, metadata, version, parentChangesets, errE := s.GetLatest(ctx, expectedID)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, 1, data.Data)
		assert.Equal(t, "m", metadata.Metadata)
		assert.Equal(t, insertVersion, version)
		assert.Empty(t, parentChangesets)
	}

	// Verify notification carries the specified changeset ID.
	require.Eventually(t, func() bool {
		return channelContents.Len() >= 1
	}, 5*time.Second, 10*time.Millisecond)
	c := channelContents.Prune()
	if assert.Len(t, c, 1) {
		assert.Equal(t, changesetID, c[0].Changesets[0].ID())
	}
}

func TestChangesetIDReplace(t *testing.T) {
	t.Parallel()

	ctx, s, _ := initDatabase[
		*internalStore.TestData, *internalStore.TestMetadata, *internalStore.TestMetadata,
		*internalStore.TestMetadata, *testCommitMetadata, *internalStore.TestPatch,
	](t, "jsonb")

	expectedID := identifier.New()
	insertChangesetID := identifier.New()

	// Insert first.
	insertVersion, errE := s.Insert(
		ctx, expectedID,
		&internalStore.TestData{Data: 1, Patch: false},
		&internalStore.TestMetadata{Metadata: "m"},
		&testCommitMetadata{
			TestMetadata: internalStore.TestMetadata{Metadata: "insert"},
			ID:           insertChangesetID,
		},
	)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, insertChangesetID, insertVersion.Changeset)

	// Replace with a specified changeset ID.
	replaceChangesetID := identifier.New()
	replaceVersion, errE := s.Replace(
		ctx, expectedID, insertVersion.Changeset,
		&internalStore.TestData{Data: 2, Patch: false},
		&internalStore.TestMetadata{Metadata: "m2"},
		&testCommitMetadata{
			TestMetadata: internalStore.TestMetadata{Metadata: "replace"},
			ID:           replaceChangesetID,
		},
	)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, int64(1), replaceVersion.Revision)
	assert.Equal(t, replaceChangesetID, replaceVersion.Changeset)

	// Verify.
	data, _, version, parentChangesets, errE := s.GetLatest(ctx, expectedID)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, 2, data.Data)
		assert.Equal(t, replaceVersion, version)
		if assert.Len(t, parentChangesets, 1) {
			assert.Equal(t, insertChangesetID, parentChangesets[0].Changeset)
		}
	}
}

func TestChangesetIDUpdate(t *testing.T) {
	t.Parallel()

	ctx, s, _ := initDatabase[
		*internalStore.TestData, *internalStore.TestMetadata, *internalStore.TestMetadata,
		*internalStore.TestMetadata, *testCommitMetadata, *internalStore.TestPatch,
	](t, "jsonb")

	expectedID := identifier.New()
	insertChangesetID := identifier.New()

	insertVersion, errE := s.Insert(
		ctx, expectedID,
		&internalStore.TestData{Data: 1, Patch: false},
		&internalStore.TestMetadata{Metadata: "m"},
		&testCommitMetadata{
			TestMetadata: internalStore.TestMetadata{Metadata: "insert"},
			ID:           insertChangesetID,
		},
	)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Update with a specified changeset ID.
	updateChangesetID := identifier.New()
	updateVersion, errE := s.Update(
		ctx, expectedID, insertVersion.Changeset,
		&internalStore.TestData{Data: 1, Patch: true},
		&internalStore.TestPatch{Patch: true},
		&internalStore.TestMetadata{Metadata: "m2"},
		&testCommitMetadata{
			TestMetadata: internalStore.TestMetadata{Metadata: "update"},
			ID:           updateChangesetID,
		},
	)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, int64(1), updateVersion.Revision)
	assert.Equal(t, updateChangesetID, updateVersion.Changeset)
}

func TestChangesetIDMerge(t *testing.T) {
	t.Parallel()

	ctx, s, _ := initDatabase[
		*internalStore.TestData, *internalStore.TestMetadata, *internalStore.TestMetadata,
		*internalStore.TestMetadata, *testCommitMetadata, *internalStore.TestPatch,
	](t, "jsonb")

	expectedID := identifier.New()

	// Insert.
	insertVersion, errE := s.Insert(
		ctx, expectedID,
		&internalStore.TestData{Data: 1, Patch: false},
		&internalStore.TestMetadata{Metadata: "m"},
		&testCommitMetadata{
			TestMetadata: internalStore.TestMetadata{Metadata: "insert"},
			ID:           identifier.New(),
		},
	)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Create two branches via manual changesets (uncommitted) to set up a merge.
	changesetA, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changesetA.Update(
		ctx, expectedID, insertVersion.Changeset,
		&internalStore.TestData{Data: 2, Patch: true},
		&internalStore.TestPatch{Patch: true},
		&internalStore.TestMetadata{Metadata: "b1"},
	)
	require.NoError(t, errE, "% -+#.1v", errE)

	changesetB, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changesetB.Update(
		ctx, expectedID, insertVersion.Changeset,
		&internalStore.TestData{Data: 3, Patch: true},
		&internalStore.TestPatch{Patch: true},
		&internalStore.TestMetadata{Metadata: "b2"},
	)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Merge with a specified changeset ID.
	mergeChangesetID := identifier.New()
	mergeVersion, errE := s.Merge(
		ctx, expectedID,
		[]identifier.Identifier{changesetA.ID(), changesetB.ID()},
		&internalStore.TestData{Data: 4, Patch: false},
		[]*internalStore.TestPatch{{Patch: true}, {Patch: true}},
		&internalStore.TestMetadata{Metadata: "merged"},
		&testCommitMetadata{
			TestMetadata: internalStore.TestMetadata{Metadata: "merge"},
			ID:           mergeChangesetID,
		},
	)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, mergeChangesetID, mergeVersion.Changeset)

	// Verify merge parents.
	_, _, _, parentChangesets, errE := s.GetLatest(ctx, expectedID) //nolint:dogsled
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Len(t, parentChangesets, 2)
	}
}

func TestChangesetIDDelete(t *testing.T) {
	t.Parallel()

	ctx, s, _ := initDatabase[
		*internalStore.TestData, *internalStore.TestMetadata, *internalStore.TestMetadata,
		*internalStore.TestMetadata, *testCommitMetadata, *internalStore.TestPatch,
	](t, "jsonb")

	expectedID := identifier.New()

	insertVersion, errE := s.Insert(
		ctx, expectedID,
		&internalStore.TestData{Data: 1, Patch: false},
		&internalStore.TestMetadata{Metadata: "m"},
		&testCommitMetadata{
			TestMetadata: internalStore.TestMetadata{Metadata: "insert"},
			ID:           identifier.New(),
		},
	)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Delete with a specified changeset ID.
	deleteChangesetID := identifier.New()
	deleteVersion, errE := s.Delete(
		ctx, expectedID, insertVersion.Changeset,
		&internalStore.TestMetadata{Metadata: "del"},
		&testCommitMetadata{
			TestMetadata: internalStore.TestMetadata{Metadata: "delete"},
			ID:           deleteChangesetID,
		},
	)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, int64(1), deleteVersion.Revision)
	assert.Equal(t, deleteChangesetID, deleteVersion.Changeset)

	// Verify value is deleted.
	_, _, _, _, errE = s.GetLatest(ctx, expectedID) //nolint:dogsled
	assert.ErrorIs(t, errE, store.ErrValueNotFound)
}

// TestChangesetIDWithoutInterface verifies that when commitMetadata does NOT
// implement ChangesetID, a random changeset ID is used (existing behavior).
func TestChangesetIDWithoutInterface(t *testing.T) {
	t.Parallel()

	ctx, s, _ := initDatabase[
		*internalStore.TestData, *internalStore.TestMetadata, *internalStore.TestMetadata,
		*internalStore.TestMetadata, *internalStore.TestMetadata, *internalStore.TestPatch,
	](t, "jsonb")

	expectedID := identifier.New()

	insertVersion, errE := s.Insert(
		ctx, expectedID,
		&internalStore.TestData{Data: 1, Patch: false},
		&internalStore.TestMetadata{Metadata: "m"},
		&internalStore.TestMetadata{Metadata: "commit"},
	)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Changeset should be a valid identifier (random), not zero.
	assert.NotEqual(t, identifier.Identifier{}, insertVersion.Changeset)
}
