package search_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/identifier"
)

// TestBridgeReindexDeletesPendingBacklog verifies that a reindex job collapses the redundant backlog:
// updateSeq schedules one reindex job per commit that enqueues, so a populate leaves a large number of
// pending jobs that each only refresh the index and find nothing to do. deletePendingReindexJobs (run at the
// start of every reindex job) removes this bridge's pending jobs, scoped to its own prefix so it never
// touches another bridge's jobs.
func TestBridgeReindexDeletesPendingBacklog(t *testing.T) {
	t.Parallel()

	ctx, env := setupBridge(t)
	b := env.bridge

	prefix := env.store.Prefix
	otherPrefix := identifier.New().String() + "_"

	// Build up a backlog of pending reindex jobs for this bridge, plus one for another bridge's prefix. The
	// river client is not started here, so nothing drains them and the state is deterministic.
	const backlog = 7
	for range backlog {
		errE := b.TestingScheduleReindexJob(ctx, prefix)
		require.NoError(t, errE, "% -+#.1v", errE)
	}
	errE := b.TestingScheduleReindexJob(ctx, otherPrefix)
	require.NoError(t, errE, "% -+#.1v", errE)

	available, errE := b.TestingCountAvailableReindexJobs(ctx, prefix)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Equal(t, backlog, available)

	// A reindex job run deletes this bridge's pending jobs before taking its seq snapshot.
	deleted, errE := b.TestingDeletePendingReindexJobs(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Equal(t, int64(backlog), deleted)

	available, errE = b.TestingCountAvailableReindexJobs(ctx, prefix)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Zero(t, available, "this bridge's pending reindex jobs should be collapsed")

	// Another bridge's pending jobs must be left untouched: the delete is scoped by prefix.
	other, errE := b.TestingCountAvailableReindexJobs(ctx, otherPrefix)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Equal(t, 1, other, "another bridge's pending reindex jobs must not be deleted")
}
