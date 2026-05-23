package auth

import (
	"context"
	"time"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"gitlab.com/tozd/go/errors"

	internalStore "gitlab.com/peerdb/peerdb/internal/store"
)

// errFlowNotFound is returned by flowStore.ConsumeFlow when the state value
// has no matching unexpired row. Either the user lost their session, the row
// aged out, the row was already consumed, or the state was forged. It is
// surfaced to the route handler via ErrSignInFailed.
var errFlowNotFound = errors.Base("flow not found")

// flowTTL is how long a pending sign-in flow is allowed to sit in the store
// before its row is considered expired. It bounds the time between the
// authorize redirect and the callback. Users that take longer to authenticate
// at the issuer will see a fresh "lost flow" error rather than an old code
// being silently exchanged.
const flowTTL = 24 * time.Hour

// flowState is the per-flow data we need to remember between the authorize
// redirect (sent to the issuer) and the callback (issued back to us).
type flowState struct {
	// codeVerifier is the PKCE verifier matching the challenge sent to the issuer.
	codeVerifier string
	nonce        string
	// redirect is where the caller asked to land after a successful sign-in,
	// validated to be a same-site path by safeRedirectPath before being
	// stored.
	redirect string
}

// flowStore persists the authentication flow state in the per-site PostgreSQL
// schema. It is constructed once per Authenticator. Connections pick up the
// right schema via the search_path that the request context configures.
type flowStore struct {
	dbpool *pgxpool.Pool
}

// newFlowStore returns a flowStore backed by the given connection pool. The
// caller is responsible for ensuring ctx carries the per-site schema
// (typically via WithFallbackDBContext) when calling from the outside of
// a WAF-routed request.
func newFlowStore(dbpool *pgxpool.Pool) *flowStore {
	return &flowStore{dbpool: dbpool}
}

// Init creates the AuthFlows table in the schema configured on the
// connection. It is safe to call repeatedly: an already-existing table is
// treated as success so that re-runs during a site re-Init do not error.
func (s *flowStore) Init(ctx context.Context) errors.E {
	errE := internalStore.RetryTransaction(ctx, s.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		_, err := tx.Exec(ctx, `
			CREATE TABLE "AuthFlows" (
				"state" text STORAGE PLAIN COLLATE "C" NOT NULL,
				"codeVerifier" text NOT NULL,
				"nonce" text NOT NULL,
				"redirect" text NOT NULL,
				"createdAt" timestamptz NOT NULL DEFAULT now(),
				"expiresAt" timestamptz NOT NULL,
				PRIMARY KEY ("state")
			);
			CREATE INDEX ON "AuthFlows" ("expiresAt");
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

// BeginFlow stores a new flow row keyed by state. The state value must be
// unpredictable to a third party.
func (s *flowStore) BeginFlow(ctx context.Context, state string, fs flowState) errors.E {
	return internalStore.RetryTransaction(ctx, s.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		_, err := tx.Exec(ctx, `
			INSERT INTO "AuthFlows" ("state", "codeVerifier", "nonce", "redirect", "expiresAt")
			VALUES ($1, $2, $3, $4, now() + make_interval(secs => $5))
		`, state, fs.codeVerifier, fs.nonce, fs.redirect, flowTTL.Seconds())
		return internalStore.WithPgxError(err)
	})
}

// ConsumeFlow atomically deletes the row identified by state and returns its
// flowState. Single-use semantics: a second call with the same state returns
// errFlowNotFound. Expired rows are treated as absent.
func (s *flowStore) ConsumeFlow(ctx context.Context, state string) (flowState, errors.E) {
	var fs flowState
	errE := internalStore.RetryTransaction(ctx, s.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		row := tx.QueryRow(ctx, `
			DELETE FROM "AuthFlows"
			WHERE "state" = $1 AND "expiresAt" > now()
			RETURNING "codeVerifier", "nonce", "redirect"
		`, state)
		err := row.Scan(&fs.codeVerifier, &fs.nonce, &fs.redirect)
		if errors.Is(err, pgx.ErrNoRows) {
			return errors.WithStack(errFlowNotFound)
		}
		return internalStore.WithPgxError(err)
	})
	if errE != nil {
		return flowState{}, errE
	}
	return fs, nil
}

// cleanupExpired removes rows whose expiresAt is in the past. Safe to call
// concurrently with BeginFlow / ConsumeFlow because all writes go through
// serializable transactions.
func (s *flowStore) cleanupExpired(ctx context.Context) errors.E {
	return internalStore.RetryTransaction(ctx, s.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		_, err := tx.Exec(ctx, `DELETE FROM "AuthFlows" WHERE "expiresAt" <= now()`)
		return internalStore.WithPgxError(err)
	})
}
