package auth

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	internalStore "gitlab.com/peerdb/peerdb/internal/store"
)

// newTestRevocationStore returns a fully initialised revocationStore
// scoped to a fresh per-test schema, plus the ctx the caller should
// thread into Revoke / IsRevoked.
func newTestRevocationStore(t *testing.T) (context.Context, *revocationStore) {
	t.Helper()

	ctx, dbpool := TestingInitPool(t)
	rs := newRevocationStore(dbpool)
	errE := rs.Init(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)
	return ctx, rs
}

// TestRevocationStoreRevokeAndIsRevoked covers the basic cycle: a
// fresh token is not revoked, Revoke writes the row and updates the
// cache, and IsRevoked then reports the token as revoked.
func TestRevocationStoreRevokeAndIsRevoked(t *testing.T) {
	t.Parallel()

	ctx, rs := newTestRevocationStore(t)
	token := "test-token-" + identifier.New().String()
	exp := time.Now().Add(time.Hour)

	revoked, errE := rs.IsRevoked(ctx, token, exp)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.False(t, revoked, "fresh token must not be reported as revoked")

	errE = rs.Revoke(ctx, token, exp)
	require.NoError(t, errE, "% -+#.1v", errE)

	revoked, errE = rs.IsRevoked(ctx, token, exp)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.True(t, revoked, "revoked token must be reported as revoked")
}

// TestRevocationStoreRevokeIsIdempotent covers ON CONFLICT DO NOTHING:
// calling Revoke twice for the same token must not error and must keep
// reporting the token as revoked.
func TestRevocationStoreRevokeIsIdempotent(t *testing.T) {
	t.Parallel()

	ctx, rs := newTestRevocationStore(t)
	token := "test-token-" + identifier.New().String()
	exp := time.Now().Add(time.Hour)

	for i := range 2 {
		errE := rs.Revoke(ctx, token, exp)
		require.NoError(t, errE, "Revoke call %d: %% -+#.1v", i, errE)
	}

	revoked, errE := rs.IsRevoked(ctx, token, exp)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.True(t, revoked)
}

// TestRevocationStoreNotRevokedCacheRefreshesAfterTTL covers the cache
// TTL semantics for the "not revoked" decision: the first lookup caches
// false for notRevokedCacheTTL, subsequent revocations of the same
// token by an out-of-band writer are NOT visible until the cached entry
// expires.
//
// We use a controllable now() to fast-forward past the cache TTL.
func TestRevocationStoreNotRevokedCacheRefreshesAfterTTL(t *testing.T) {
	t.Parallel()

	ctx, rs := newTestRevocationStore(t)
	token := "test-token-" + identifier.New().String()
	exp := time.Now().Add(time.Hour)

	clock := time.Now()
	rs.now = func() time.Time { return clock }

	// Cold lookup: not revoked, cached as "not revoked" for an hour.
	revoked, errE := rs.IsRevoked(ctx, token, exp)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.False(t, revoked)

	// Out-of-band write directly into the table, bypassing the store
	// (so the store's cache does not learn about it). This simulates
	// a revocation written by another process.
	errE = internalStore.RetryTransaction(ctx, rs.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		_, err := tx.Exec(ctx, `INSERT INTO "RevokedTokens" ("tokenHash", "expiresAt") VALUES ($1, $2)`, hashToken(token), exp)
		return internalStore.WithPgxError(err)
	})
	require.NoError(t, errE, "% -+#.1v", errE)

	// Within the cache TTL window: the out-of-band write is invisible.
	revoked, errE = rs.IsRevoked(ctx, token, exp)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.False(t, revoked, "cache hit should hide the out-of-band revocation")

	// Step past notRevokedCacheTTL: the cache entry is stale, IsRevoked
	// re-queries the database and sees the revocation.
	clock = clock.Add(notRevokedCacheTTL + time.Second)
	revoked, errE = rs.IsRevoked(ctx, token, exp)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.True(t, revoked, "after cache TTL the DB-side revocation must surface")
}

// TestRevocationStoreRevokePrimesCache covers the inverse direction:
// Revoke writes the cache entry directly so that even a token cached
// as "not revoked" within the current notRevokedCacheTTL window
// becomes recognised as revoked immediately.
func TestRevocationStoreRevokePrimesCache(t *testing.T) {
	t.Parallel()

	ctx, rs := newTestRevocationStore(t)
	token := "test-token-" + identifier.New().String()
	exp := time.Now().Add(time.Hour)

	// Seed the cache with a "not revoked" entry.
	revoked, errE := rs.IsRevoked(ctx, token, exp)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.False(t, revoked)

	// Revoke through the store API. Cache must flip immediately - no
	// TTL wait, no DB round-trip on the next IsRevoked.
	errE = rs.Revoke(ctx, token, exp)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Disable the underlying pool to prove the next IsRevoked is
	// served from cache without touching the DB. Closing the pool
	// would make any SQL fail. We Close after IsRevoked so the
	// deferred t.Cleanup pool-close still works (it's a no-op the
	// second time).
	rs.dbpool.Close()

	revoked, errE = rs.IsRevoked(ctx, token, exp)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.True(t, revoked, "Revoke must populate the cache so a follow-up IsRevoked needs no DB call")
}
