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

func (a *MockAuthenticator) TestingSubject() string {
	return a.subject
}

func (a *MockAuthenticator) TestingAuthCodeURL(state, codeVerifier, nonce string) string {
	// The mock's authCodeURL ignores ui_locales (it self-redirects), so we pass an empty value.
	return a.authCodeURL(state, codeVerifier, nonce, "")
}

func (a *MockAuthenticator) TestingExchangeCode(
	ctx context.Context, code, codeVerifier, expectedNonce string, allowedRoles map[string]RoleGrants,
) (string, time.Time, errors.E) {
	return a.exchangeCode(ctx, code, codeVerifier, expectedNonce, allowedRoles)
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

// TestingNewUserInfoCache builds a cache over the OIDC lookups, the same way NewOIDCAuthenticator
// does, with the issuer of the other-user lookup left unconfigured: the tests are about the caller's
// own lookup and about the caching around it.
func TestingNewUserInfoCache(endpoint string, client *http.Client) *userInfoCache {
	return newUserInfoCache(fetchUserInfo(endpoint, client), fetchIdentity("", "", client))
}

func (s *flowStore) TestingCleanupExpired(ctx context.Context) errors.E {
	return s.cleanupExpired(ctx)
}

func (s *revocationStore) TestingCleanupExpired(ctx context.Context) errors.E {
	return s.cleanupExpired(ctx)
}

func (c *userInfoCache) TestingSet(subject string, info userInfo) {
	c.set(subject, info)
}

func (c *userInfoCache) TestingDelete(subject string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.items, subject)
}
