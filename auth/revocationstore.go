package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"gitlab.com/tozd/go/errors"

	internalStore "gitlab.com/peerdb/peerdb/internal/store"
)

// notRevokedCacheTTL is how long a "this token is not revoked" decision is
// trusted before we re-check the database. A revoked token whose JWT is
// still inside this window can keep authenticating until the cached
// "not revoked" entry expires. The trade-off for not hitting the database
// on every authenticated request.
const notRevokedCacheTTL = 1 * time.Hour

// revocationCacheEntry pairs a revocation status with the moment it stops
// being trustworthy. For "revoked" entries expiresAt mirrors the token's
// exp claim. After that the JWT validator rejects the token anyway, so
// the cached decision becomes irrelevant. For "not revoked" entries
// expiresAt is now + notRevokedCacheTTL.
type revocationCacheEntry struct {
	revoked   bool
	expiresAt time.Time
}

// revocationStore tracks revoked access tokens. The persistent half is a
// per-site PostgreSQL table that only stores revoked-token rows (so absence
// of a row is "not revoked"). A process-local cache sits on top so the
// per-request Authenticate path does not hit the database every time.
//
// Lookup order: cache first; on miss or stale entry, the database. The
// result is then written back to the cache. The cache stores both
// outcomes. Revoked rows are cached until the token expires anyway,
// not-revoked answers for notRevokedCacheTTL.
type revocationStore struct {
	dbpool *pgxpool.Pool

	mu    sync.Mutex
	items map[string]revocationCacheEntry

	now func() time.Time
}

// newRevocationStore returns a revocationStore backed by the given
// connection pool. The caller is responsible for ensuring ctx carries
// the per-site schema when Init / IsRevoked / Revoke run outside of a
// WAF-routed request.
func newRevocationStore(dbpool *pgxpool.Pool) *revocationStore {
	return &revocationStore{ //nolint:exhaustruct // mu is a zero-value sync.Mutex.
		dbpool: dbpool,
		items:  map[string]revocationCacheEntry{},
		now:    time.Now,
	}
}

// Init creates the RevokedTokens table in the schema configured on the
// connection. Idempotent on re-init.
func (s *revocationStore) Init(ctx context.Context) errors.E {
	errE := internalStore.RetryTransaction(ctx, s.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		_, err := tx.Exec(ctx, `
			CREATE TABLE "RevokedTokens" (
				"tokenHash" text STORAGE PLAIN COLLATE "C" NOT NULL,
				"expiresAt" timestamptz NOT NULL,
				"revokedAt" timestamptz NOT NULL DEFAULT now(),
				PRIMARY KEY ("tokenHash")
			);
			CREATE INDEX ON "RevokedTokens" ("expiresAt");
		`)
		return internalStore.WithPgxError(err)
	})
	if pgError, ok := errors.AsType[*pgconn.PgError](errE); ok {
		switch pgError.Code {
		case pgerrcode.UniqueViolation:
			// Nothing.
		case pgerrcode.DuplicateFunction:
			// Nothing.
		case pgerrcode.DuplicateTable:
			// Nothing.
			return nil
		}
	}
	return errE
}

// hashToken returns the storage key for a raw JWT. We never persist the
// token itself (a stolen DB dump would otherwise yield a list of all
// recently-revoked but unexpired bearer credentials). The hash is enough
// to recognise a previously-revoked token on lookup.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (s *revocationStore) get(hash string) (revocationCacheEntry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.items[hash]
	return entry, ok
}

func (s *revocationStore) set(hash string, entry revocationCacheEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.items[hash] = entry
}

// IsRevoked reports whether the given token has been revoked. tokenExp is
// the JWT's exp claim, used to bound the cache entry's lifetime when we
// cache a "revoked" result. For "not revoked" results we use
// notRevokedCacheTTL.
func (s *revocationStore) IsRevoked(ctx context.Context, token string, tokenExp time.Time) (bool, errors.E) {
	hash := hashToken(token)

	// Hot path: cache hit and entry not yet stale.
	entry, ok := s.get(hash)
	if ok && s.now().Before(entry.expiresAt) {
		return entry.revoked, nil
	}

	// Cold path: ask the database.
	var revoked bool
	errE := internalStore.RetryTransaction(ctx, s.dbpool, pgx.ReadOnly, func(ctx context.Context, tx pgx.Tx) errors.E {
		// We do not filter by expiresAt because once a token is revoked it is revoked
		// and it does not matter if it is still in a database not cleaned up.
		row := tx.QueryRow(ctx, `
			SELECT 1 FROM "RevokedTokens" WHERE "tokenHash" = $1
		`, hash)
		var seen int
		err := row.Scan(&seen)
		if errors.Is(err, pgx.ErrNoRows) {
			revoked = false
			return nil
		}
		if err != nil {
			return internalStore.WithPgxError(err)
		}
		revoked = true
		return nil
	})
	if errE != nil {
		return false, errE
	}

	// Write the decision back to the cache. Revoked rows live until the
	// JWT exp; not-revoked answers expire much sooner so a subsequent
	// revocation propagates within notRevokedCacheTTL.
	now := s.now()
	cacheExp := tokenExp
	if !revoked {
		cacheExp = now.Add(notRevokedCacheTTL)
	}
	s.set(hash, revocationCacheEntry{revoked: revoked, expiresAt: cacheExp})

	return revoked, nil
}

// Revoke marks the given token as revoked until tokenExp. The persistent
// row and the cache entry are both written.
func (s *revocationStore) Revoke(ctx context.Context, token string, tokenExp time.Time) errors.E {
	hash := hashToken(token)

	errE := internalStore.RetryTransaction(ctx, s.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		_, err := tx.Exec(ctx, `
			INSERT INTO "RevokedTokens" ("tokenHash", "expiresAt")
			VALUES ($1, $2)
			ON CONFLICT ("tokenHash") DO NOTHING
		`, hash, tokenExp)
		return internalStore.WithPgxError(err)
	})
	if errE != nil {
		return errE
	}

	// Cache the revocation so even tokens cached as "not revoked" within
	// the current notRevokedCacheTTL window are kicked out immediately.
	s.set(hash, revocationCacheEntry{revoked: true, expiresAt: tokenExp})

	return nil
}

// cleanupExpired removes rows for tokens that have already expired. Safe
// to call concurrently with Revoke / IsRevoked.
func (s *revocationStore) cleanupExpired(ctx context.Context) errors.E {
	return internalStore.RetryTransaction(ctx, s.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		_, err := tx.Exec(ctx, `DELETE FROM "RevokedTokens" WHERE "expiresAt" <= now()`)
		return internalStore.WithPgxError(err)
	})
}
