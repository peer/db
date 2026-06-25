package auth

import (
	"context"
	"net/http"
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

//nolint:gochecknoglobals
var (
	TestingResolveAccessToken = resolveAccessToken
	VisibilityForRoles        = visibilityForRoles
	TestingSafeRedirectPath   = safeRedirectPath
	TestingHashToken          = hashToken
	TestingErrFlowNotFound    = errFlowNotFound //nolint:errname
)

const (
	TestingAccessTokenCookieName = accessTokenCookieName
	TestingNotRevokedCacheTTL    = notRevokedCacheTTL
)

type (
	TestingFlowStore       = flowStore
	TestingFlowState       = flowState
	TestingRevocationStore = revocationStore
	TestingUserInfoCache   = userInfoCache
	TestingUserInfo        = userInfo
)

func (a *MockAuthenticator) TestingAuthCodeURL(state, codeVerifier, nonce string) string {
	return a.authCodeURL(state, codeVerifier, nonce)
}

func (a *MockAuthenticator) TestingExchangeCode(ctx context.Context, code, codeVerifier, expectedNonce string) (string, time.Time, errors.E) {
	return a.exchangeCode(ctx, code, codeVerifier, expectedNonce)
}

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

func TestingNewFlowStore(dbpool *pgxpool.Pool) *flowStore {
	return newFlowStore(dbpool)
}

func TestingNewRevocationStore(dbpool *pgxpool.Pool) *revocationStore {
	return newRevocationStore(dbpool)
}

func TestingNewUserInfoCache(endpoint string, client *http.Client) *userInfoCache {
	return newUserInfoCache(endpoint, client)
}

func (s *flowStore) TestingDBPool() *pgxpool.Pool {
	return s.dbpool
}

func (s *flowStore) TestingCleanupExpired(ctx context.Context) errors.E {
	return s.cleanupExpired(ctx)
}

func (s *revocationStore) TestingDBPool() *pgxpool.Pool {
	return s.dbpool
}

func (s *revocationStore) TestingSetNow(now func() time.Time) {
	s.now = now
}

func (s *revocationStore) TestingCleanupExpired(ctx context.Context) errors.E {
	return s.cleanupExpired(ctx)
}

func (c *userInfoCache) TestingSet(subject string, info userInfo) {
	c.set(subject, info)
}
