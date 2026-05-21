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

// ErrFlowNotFound is returned by ConsumeFlow when the state value has no
// matching unexpired row. Either the user lost their session, the row aged
// out, the row was already consumed, or the state was forged.
var ErrFlowNotFound = errors.Base("OIDC flow not found")

// flowTTL is how long a pending sign-in flow is allowed to sit in the store
// before its row is considered expired. It bounds the time between the
// authorize redirect and the callback; users that take longer to authenticate
// at the issuer will see a fresh "lost flow" error rather than an old code
// being silently exchanged.
const flowTTL = 24 * time.Hour

// FlowState is the per-flow data we need to remember between the authorize
// redirect (POSTed to the issuer) and the callback (issued back to us).
type FlowState struct {
	// CodeVerifier is the PKCE verifier matching the challenge sent to the issuer.
	CodeVerifier string
	Nonce        string
	// Redirect is where the caller asked to land after a successful sign-in,
	// validated to be a same-site path by the signin handler before being
	// stored.
	Redirect string
}

// FlowStore persists short-lived OIDC flow state in the per-site PostgreSQL
// schema. It is constructed once per site and used by the auth route
// handlers. Connections pick up the right schema via the search_path that the
// request context configures.
type FlowStore struct {
	dbpool *pgxpool.Pool
}

// NewFlowStore returns a FlowStore backed by the given connection pool. The
// caller is responsible for ensuring ctx carries the per-site schema
// (typically via WithFallbackDBContext) when Init / BeginFlow / ConsumeFlow
// run outside of a WAF-routed request.
func NewFlowStore(dbpool *pgxpool.Pool) *FlowStore {
	return &FlowStore{dbpool: dbpool}
}

// Init creates the OIDCFlows table in the schema configured on the
// connection. It is safe to call repeatedly: an already-existing table is
// treated as success so that re-runs during a site re-init do not error.
func (s *FlowStore) Init(ctx context.Context) errors.E {
	errE := internalStore.RetryTransaction(ctx, s.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		_, err := tx.Exec(ctx, `
			CREATE TABLE "OIDCFlows" (
				"state" text STORAGE PLAIN COLLATE "C" NOT NULL,
				"codeVerifier" text NOT NULL,
				"nonce" text NOT NULL,
				"redirect" text NOT NULL,
				"createdAt" timestamptz NOT NULL DEFAULT now(),
				"expiresAt" timestamptz NOT NULL,
				PRIMARY KEY ("state")
			);
			CREATE INDEX ON "OIDCFlows" ("expiresAt");
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
func (s *FlowStore) BeginFlow(ctx context.Context, state string, fs FlowState) errors.E {
	return internalStore.RetryTransaction(ctx, s.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		_, err := tx.Exec(ctx, `
			INSERT INTO "OIDCFlows" ("state", "codeVerifier", "nonce", "redirect", "expiresAt")
			VALUES ($1, $2, $3, $4, now() + make_interval(secs => $5))
		`, state, fs.CodeVerifier, fs.Nonce, fs.Redirect, flowTTL.Seconds())
		return internalStore.WithPgxError(err)
	})
}

// ConsumeFlow atomically deletes the row identified by state and returns its
// FlowState. Single-use semantics: a second call with the same state returns
// ErrFlowNotFound. Expired rows are treated as absent.
func (s *FlowStore) ConsumeFlow(ctx context.Context, state string) (FlowState, errors.E) {
	var fs FlowState
	errE := internalStore.RetryTransaction(ctx, s.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		row := tx.QueryRow(ctx, `
			DELETE FROM "OIDCFlows"
			WHERE "state" = $1 AND "expiresAt" > now()
			RETURNING "codeVerifier", "nonce", "redirect"
		`, state)
		err := row.Scan(&fs.CodeVerifier, &fs.Nonce, &fs.Redirect)
		if errors.Is(err, pgx.ErrNoRows) {
			return errors.WithStack(ErrFlowNotFound)
		}
		return internalStore.WithPgxError(err)
	})
	if errE != nil {
		return FlowState{}, errE
	}
	return fs, nil
}

// CleanupExpired removes rows whose expiresAt is in the past. Safe to call
// concurrently with Begin / Consume because all writes go through
// serializable transactions.
func (s *FlowStore) CleanupExpired(ctx context.Context) errors.E {
	return internalStore.RetryTransaction(ctx, s.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		_, err := tx.Exec(ctx, `DELETE FROM "OIDCFlows" WHERE "expiresAt" <= now()`)
		return internalStore.WithPgxError(err)
	})
}
