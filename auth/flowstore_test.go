package auth_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/auth"
	internalStore "gitlab.com/peerdb/peerdb/internal/store"
)

// newTestFlowStore returns a fully initialised flowStore scoped to a
// fresh per-test schema, plus the ctx the caller should thread into
// BeginFlow / ConsumeFlow.
func newTestFlowStore(t *testing.T) (context.Context, *auth.TestingFlowStore) {
	t.Helper()

	ctx, dbpool := auth.TestingInitPool(t)
	fs := auth.TestingNewFlowStore(dbpool)
	require.NoError(t, fs.Init(ctx), "%+v", fs.Init(ctx))
	return ctx, fs
}

// TestFlowStoreInitIsIdempotent covers the documented behaviour: a
// second Init against an already-initialised schema must succeed
// silently so a re-run during site re-init does not error.
func TestFlowStoreInitIsIdempotent(t *testing.T) {
	t.Parallel()

	ctx, fs := newTestFlowStore(t)
	errE := fs.Init(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)
}

// TestFlowStoreBeginAndConsume covers the basic round-trip: BeginFlow
// writes a row, ConsumeFlow reads it back with every field intact.
func TestFlowStoreBeginAndConsume(t *testing.T) {
	t.Parallel()

	ctx, fs := newTestFlowStore(t)
	state := identifier.New().String()
	want := auth.TestingFlowState{
		CodeVerifier: "verifier-" + identifier.New().String(),
		Nonce:        "nonce-" + identifier.New().String(),
		Redirect:     "/landing-" + identifier.New().String(),
	}

	errE := fs.BeginFlow(ctx, state, want)
	require.NoError(t, errE, "% -+#.1v", errE)

	got, errE := fs.ConsumeFlow(ctx, state)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, want, got)
}

// TestFlowStoreConsumeIsSingleUse covers the DELETE...RETURNING semantics:
// a second ConsumeFlow with the same state returns errFlowNotFound.
func TestFlowStoreConsumeIsSingleUse(t *testing.T) {
	t.Parallel()

	ctx, fs := newTestFlowStore(t)
	state := identifier.New().String()
	require.NoError(t, fs.BeginFlow(ctx, state, auth.TestingFlowState{CodeVerifier: "v", Nonce: "n", Redirect: "/"}))

	_, errE := fs.ConsumeFlow(ctx, state)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = fs.ConsumeFlow(ctx, state)
	require.Error(t, errE)
	assert.True(t, errors.Is(errE, auth.TestingErrFlowNotFound))
}

// TestFlowStoreConsumeMissingState covers the "lost session" path: a
// ConsumeFlow for a state that was never inserted returns
// errFlowNotFound.
func TestFlowStoreConsumeMissingState(t *testing.T) {
	t.Parallel()

	ctx, fs := newTestFlowStore(t)
	_, errE := fs.ConsumeFlow(ctx, "never-existed")
	require.Error(t, errE)
	assert.True(t, errors.Is(errE, auth.TestingErrFlowNotFound))
}

// TestFlowStoreCleanupExpired covers the cleanup path: rows whose
// expiresAt is in the past are deleted; rows still in their window stay.
// We forcibly age one row via a direct UPDATE so we do not have to wait
// out the real TTL.
func TestFlowStoreCleanupExpired(t *testing.T) {
	t.Parallel()

	ctx, fs := newTestFlowStore(t)

	expired := identifier.New().String()
	fresh := identifier.New().String()
	require.NoError(t, fs.BeginFlow(ctx, expired, auth.TestingFlowState{CodeVerifier: "v", Nonce: "n", Redirect: "/"}))
	require.NoError(t, fs.BeginFlow(ctx, fresh, auth.TestingFlowState{CodeVerifier: "v", Nonce: "n", Redirect: "/"}))

	// Age the first row out of its window.
	errE := internalStore.RetryTransaction(ctx, fs.TestingDBPool(), pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		_, err := tx.Exec(ctx, `UPDATE "AuthFlows" SET "expiresAt" = now() - interval '1 second' WHERE "state" = $1`, expired)
		return internalStore.WithPgxError(err)
	})
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = fs.TestingCleanupExpired(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Expired row is gone, fresh row still consumable.
	_, errE = fs.ConsumeFlow(ctx, expired)
	require.Error(t, errE)
	assert.True(t, errors.Is(errE, auth.TestingErrFlowNotFound))

	_, errE = fs.ConsumeFlow(ctx, fresh)
	require.NoError(t, errE, "% -+#.1v", errE)
}
