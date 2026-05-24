package auth

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	internalStore "gitlab.com/peerdb/peerdb/internal/store"
)

var (
	TestingResolveAccessToken = resolveAccessToken //nolint:gochecknoglobals
	TestingWithSubject        = withSubject        //nolint:gochecknoglobals
	TestingWithRoles          = withRoles          //nolint:gochecknoglobals
)

const TestingAccessTokenCookieName = accessTokenCookieName

func (a *MockAuthenticator) TestingAuthCodeURL(state, codeVerifier, nonce string) string {
	return a.authCodeURL(state, codeVerifier, nonce)
}

func (a *MockAuthenticator) TestingExchangeCode(ctx context.Context, code, codeVerifier, expectedNonce string) (string, time.Time, errors.E) {
	return a.exchangeCode(ctx, code, codeVerifier, expectedNonce)
}

// TestingInitPool returns a per-test PostgreSQL pool scoped to a fresh schema.
//
//nolint:paralleltest
func TestingInitPool(t *testing.T) (context.Context, *pgxpool.Pool) {
	t.Helper()

	if os.Getenv("POSTGRES") == "" {
		t.Skip("POSTGRES is not available")
	}

	ctx := t.Context()

	logger := zerolog.New(zerolog.NewTestWriter(t)).With().Timestamp().Logger()
	ctx = logger.WithContext(ctx)

	schema := "s" + strings.ToLower(identifier.New().String())

	// context.WithoutCancel because we cancel the pool ourselves and not
	// when ctx is cancelled - cleanup code needs PostgreSQL access.
	dbCtx := internalStore.WithMaxDBPoolConnections(context.WithoutCancel(ctx), internalStore.TestMaxDBPoolConnections)
	dbpool, dbpoolCleanup, errE := internalStore.InitPostgres(dbCtx, os.Getenv("POSTGRES"), logger, func(context.Context) (string, string) {
		return schema, "tests"
	})
	require.NoError(t, errE, "% -+#.1v", errE)
	t.Cleanup(dbpoolCleanup)

	errE = internalStore.RetryTransaction(ctx, dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		return internalStore.EnsureSchema(ctx, tx, schema)
	})
	require.NoError(t, errE, "% -+#.1v", errE)

	return ctx, dbpool
}
