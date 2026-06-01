package auth_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/coreos/go-oidc/v3/oidc/oidctest"
	jose "github.com/go-jose/go-jose/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/errors"

	"gitlab.com/peerdb/peerdb/auth"
)

// signedToken builds a JWT with the given claims signed by the test issuer's
// private key. We sign access tokens with the same key the OIDC server
// advertises so the Authenticator can validate them.
func signedToken(t *testing.T, priv *rsa.PrivateKey, claims map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(claims)
	require.NoError(t, err)
	return oidctest.SignIDToken(priv, "test-key", oidc.RS256, string(raw))
}

// testAudience is the audience the Authenticator expects in every test
// token. Each test redeclares it locally for readability.
const testAudience = "peerdb"

// newTestAuthenticator spins up an oidctest server and returns an
// Authenticator wired to it, along with the issuer URL and the private
// signing key callers can use to mint tokens. The Authenticator's flow
// store and revocation store are initialised against a per-test
// PostgreSQL schema. The test is skipped when POSTGRES is unavailable.
func newTestAuthenticator(t *testing.T) (*auth.OIDCAuthenticator, string, *rsa.PrivateKey) {
	t.Helper()

	ctx, dbpool := auth.TestingInitPool(t)

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	server := &oidctest.Server{ //nolint:exhaustruct
		PublicKeys: []oidctest.PublicKey{{
			PublicKey: priv.Public(),
			KeyID:     "test-key",
			Algorithm: oidc.RS256,
		}},
	}
	ts := httptest.NewServer(server)
	t.Cleanup(ts.Close)
	server.SetIssuer(ts.URL)

	secretPath := filepath.Join(t.TempDir(), "client-secret")
	require.NoError(t, os.WriteFile(secretPath, []byte("test-secret"), 0o600))

	cb := func() string { return "https://example.test/auth/callback" }
	a, errE := auth.NewOIDCAuthenticator(ctx, dbpool, ts.URL, testAudience, secretPath, cb)
	require.NoError(t, errE, "% -+#.1v", errE)
	return a, ts.URL, priv
}

func TestNewOIDCAuthenticatorRequiresIssuerClientIDSecretAndRedirect(t *testing.T) {
	t.Parallel()

	ctx, dbpool := auth.TestingInitPool(t)
	cb := func() string { return "https://example.test/cb" }

	_, errE := auth.NewOIDCAuthenticator(ctx, dbpool, "", "client", "secret", cb)
	require.Error(t, errE)

	_, errE = auth.NewOIDCAuthenticator(ctx, dbpool, "https://example.test", "", "secret", cb)
	require.Error(t, errE)

	_, errE = auth.NewOIDCAuthenticator(ctx, dbpool, "https://example.test", "client", "", cb)
	require.Error(t, errE)

	_, errE = auth.NewOIDCAuthenticator(ctx, dbpool, "https://example.test", "client", "secret", nil)
	require.Error(t, errE)

	_, errE = auth.NewOIDCAuthenticator(ctx, nil, "https://example.test", "client", "secret", cb)
	require.Error(t, errE)
}

func TestAuthenticateNoRoles(t *testing.T) {
	t.Parallel()

	const audience = "peerdb"
	authenticator, issuer, priv := newTestAuthenticator(t)

	token := signedToken(t, priv, map[string]any{
		"iss":   issuer,
		"aud":   audience,
		"sub":   "user-123",
		"exp":   strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10),
		"scope": "openid profile email",
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/whatever", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	ctx := authenticator.Authenticate(w, req, "", nil)
	subject, ok := auth.Subject(ctx)
	require.True(t, ok)
	assert.Equal(t, "user-123", subject)
	assert.Empty(t, auth.Roles(ctx))
}

func TestAuthenticateFiltersRoleWildcard(t *testing.T) {
	t.Parallel()

	// In practice a granted scope is never the bare "role.*" wildcard - Charon
	// expands it before issuing the token. The Authenticator still filters
	// it out defensively in case an OIDC provider passes it through.
	const audience = "peerdb"
	authenticator, issuer, priv := newTestAuthenticator(t)

	token := signedToken(t, priv, map[string]any{
		"iss":   issuer,
		"aud":   audience,
		"sub":   "user-123",
		"exp":   strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10),
		"scope": "role.* role.editor role.",
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/whatever", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	ctx := authenticator.Authenticate(w, req, "", map[string][]string{"editor": nil})
	assert.Equal(t, []string{"editor"}, auth.Roles(ctx))
}

func TestAuthenticateReadsScpArray(t *testing.T) {
	t.Parallel()

	const audience = "peerdb"
	authenticator, issuer, priv := newTestAuthenticator(t)

	token := signedToken(t, priv, map[string]any{
		"iss": issuer,
		"aud": audience,
		"sub": "user-123",
		"exp": strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10),
		"scp": []string{"role.admin", "openid", "role.viewer"},
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/whatever", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	ctx := authenticator.Authenticate(w, req, "", map[string][]string{"admin": nil, "viewer": nil})
	assert.ElementsMatch(t, []string{"admin", "viewer"}, auth.Roles(ctx))
}

func TestAuthenticateSilentlyDropsBadToken(t *testing.T) {
	t.Parallel()

	const audience = "peerdb"
	authenticator, issuer, priv := newTestAuthenticator(t)

	tests := []struct {
		name   string
		header string
	}{
		{"malformed bearer", "Bearer not-a-jwt"},
		{
			"expired",
			"Bearer " + signedToken(t, priv, map[string]any{
				"iss": issuer,
				"aud": audience,
				"sub": "user-42",
				"exp": strconv.FormatInt(time.Now().Add(-time.Hour).Unix(), 10),
			}),
		},
		{
			"wrong audience",
			"Bearer " + signedToken(t, priv, map[string]any{
				"iss": issuer,
				"aud": "someone-else",
				"sub": "user-42",
				"exp": strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10),
			}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/whatever", nil)
			req.Header.Set("Authorization", tt.header)
			w := httptest.NewRecorder()

			ctx := authenticator.Authenticate(w, req, "", nil)
			_, ok := auth.Subject(ctx)
			assert.False(t, ok)
			assert.Empty(t, auth.Roles(ctx))
			assert.Empty(t, w.Header().Get("Roles"))
			assert.Empty(t, w.Header().Get("Userinfo"))
		})
	}
}

func TestSubjectAndRolesEmptyContext(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	_, ok := auth.Subject(ctx)
	assert.False(t, ok)
	assert.Empty(t, auth.Roles(ctx))
	assert.False(t, auth.HasRole(ctx, "admin"))
}

func TestWithSubjectAndRoles(t *testing.T) {
	t.Parallel()

	ctx := auth.WithSubject(context.Background(), "user-42")
	ctx = auth.WithRoles(ctx, []string{"admin", "editor"})

	subject, ok := auth.Subject(ctx)
	require.True(t, ok)
	assert.Equal(t, "user-42", subject)
	assert.Equal(t, "user-42", auth.MustSubject(ctx))
	assert.Equal(t, []string{"admin", "editor"}, auth.Roles(ctx))
	assert.True(t, auth.HasRole(ctx, "editor"))
}

func TestMustSubjectPanics(t *testing.T) {
	t.Parallel()

	assert.Panics(t, func() {
		auth.MustSubject(context.Background())
	})
}

func TestAuthenticateAttachesIdentityToContext(t *testing.T) {
	t.Parallel()

	const audience = "peerdb"
	authenticator, issuer, priv := newTestAuthenticator(t)

	token := signedToken(t, priv, map[string]any{
		"iss":   issuer,
		"aud":   audience,
		"sub":   "user-77",
		"exp":   strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10),
		"scope": "role.admin",
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/whatever", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	ctx := authenticator.Authenticate(w, req, "", map[string][]string{"admin": nil})

	subject, ok := auth.Subject(ctx)
	require.True(t, ok)
	assert.Equal(t, "user-77", subject)
	assert.Equal(t, []string{"admin"}, auth.Roles(ctx))
}

func TestAuthenticateLeavesAnonymousRequestsAlone(t *testing.T) {
	t.Parallel()

	authenticator, _, _ := newTestAuthenticator(t)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/whatever", nil)
	w := httptest.NewRecorder()

	ctx := authenticator.Authenticate(w, req, "", nil)
	_, ok := auth.Subject(ctx)
	assert.False(t, ok)
	assert.Empty(t, auth.Roles(ctx))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestResolveAccessTokenPrefersAuthorizationOverCookie(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/whatever", nil)
	req.Header.Set("Authorization", "Bearer header-token")
	req.AddCookie(&http.Cookie{Name: auth.TestingAccessTokenCookieName, Value: "cookie-token"}) //nolint:exhaustruct,gosec
	w := httptest.NewRecorder()

	token, fromCookie := auth.TestingResolveAccessToken(w, req)
	assert.Equal(t, "header-token", token)
	assert.False(t, fromCookie)

	// The Bearer path short-circuits, so only Authorization is read and advertised.
	vary := w.Header().Values("Vary")
	assert.Contains(t, vary, "Authorization")
	assert.NotContains(t, vary, "Cookie")
}

func TestResolveAccessTokenFallsBackToCookie(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/whatever", nil)
	req.AddCookie(&http.Cookie{Name: auth.TestingAccessTokenCookieName, Value: "cookie-token"}) //nolint:exhaustruct,gosec
	w := httptest.NewRecorder()

	token, fromCookie := auth.TestingResolveAccessToken(w, req)
	assert.Equal(t, "cookie-token", token)
	assert.True(t, fromCookie)

	vary := w.Header().Values("Vary")
	assert.Contains(t, vary, "Authorization")
	assert.Contains(t, vary, "Cookie")
}

func TestResolveAccessTokenNothingPresent(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/whatever", nil)
	w := httptest.NewRecorder()
	token, fromCookie := auth.TestingResolveAccessToken(w, req)
	assert.Empty(t, token)
	assert.False(t, fromCookie)

	// Even without a token we declared the dependency, so a later
	// authenticated request to the same URL is not served from a stale
	// cached unauthenticated response.
	vary := w.Header().Values("Vary")
	assert.Contains(t, vary, "Authorization")
	assert.Contains(t, vary, "Cookie")
}

func TestAuthenticateValidatesCookieToken(t *testing.T) {
	t.Parallel()

	const audience = "peerdb"
	authenticator, issuer, priv := newTestAuthenticator(t)

	token := signedToken(t, priv, map[string]any{
		"iss":   issuer,
		"aud":   audience,
		"sub":   "user-cookie",
		"exp":   strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10),
		"scope": "role.editor",
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/whatever", nil)
	req.AddCookie(&http.Cookie{Name: auth.TestingAccessTokenCookieName, Value: token}) //nolint:exhaustruct,gosec
	w := httptest.NewRecorder()

	ctx := authenticator.Authenticate(w, req, "", map[string][]string{"editor": nil})

	subject, ok := auth.Subject(ctx)
	require.True(t, ok)
	assert.Equal(t, "user-cookie", subject)
	assert.Equal(t, []string{"editor"}, auth.Roles(ctx))

	// Identity headers are written for both header- and cookie-borne tokens.
	// Roles is a top-level SFV list of strings; UserInfo carries the
	// signed-in signal via the presence of subject.
	assert.Equal(t, `"editor"`, w.Header().Get("Roles"))
	userInfoHeader := w.Header().Get("Userinfo")
	assert.Contains(t, userInfoHeader, `subject="user-cookie"`)
}

func TestAuthenticateSkipsHeadersForAnonymousRequest(t *testing.T) {
	t.Parallel()

	authenticator, _, _ := newTestAuthenticator(t)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/whatever", nil)
	w := httptest.NewRecorder()

	ctx := authenticator.Authenticate(w, req, "", nil)
	_, ok := auth.Subject(ctx)
	assert.False(t, ok)
	assert.Empty(t, w.Header().Get("Roles"))
	assert.Empty(t, w.Header().Get("Userinfo"))
}

// TestAuthenticateDropsRolesNotInAllowedSet covers the allowlist behaviour:
// a token may claim arbitrary "role.<key>" scopes, but the Authenticator
// only surfaces those that the caller has declared as legitimate via the
// allowedRoles set. Roles the token claims but the site has not declared
// are silently dropped so they cannot leak into auth.Roles or the Roles
// response header.
func TestAuthenticateDropsRolesNotInAllowedSet(t *testing.T) {
	t.Parallel()

	const audience = "peerdb"
	authenticator, issuer, priv := newTestAuthenticator(t)

	token := signedToken(t, priv, map[string]any{
		"iss":   issuer,
		"aud":   audience,
		"sub":   "user-filter",
		"exp":   strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10),
		"scope": "role.admin role.editor role.unknown",
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/whatever", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	ctx := authenticator.Authenticate(w, req, "", map[string][]string{"admin": nil, "editor": nil})

	subject, ok := auth.Subject(ctx)
	require.True(t, ok)
	assert.Equal(t, "user-filter", subject)
	assert.ElementsMatch(t, []string{"admin", "editor"}, auth.Roles(ctx))
	assert.False(t, auth.HasRole(ctx, "unknown"))
}

// TestAuthenticateNilAllowedRolesDropsAll covers the secure-by-default case:
// a site that does not declare any roles passes nil for allowedRoles. The
// caller is authenticated (subject still attaches) but no role claim is
// honoured.
func TestAuthenticateNilAllowedRolesDropsAll(t *testing.T) {
	t.Parallel()

	const audience = "peerdb"
	authenticator, issuer, priv := newTestAuthenticator(t)

	token := signedToken(t, priv, map[string]any{
		"iss":   issuer,
		"aud":   audience,
		"sub":   "user-noroles",
		"exp":   strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10),
		"scope": "role.admin role.editor",
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/whatever", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	ctx := authenticator.Authenticate(w, req, "", nil)

	subject, ok := auth.Subject(ctx)
	require.True(t, ok)
	assert.Equal(t, "user-noroles", subject)
	assert.Empty(t, auth.Roles(ctx))
}

// TestMockAuthenticatorMintsValidJWT covers the round-trip:
// NewMockAuthenticator generates a key pair and a token verifier wired to
// it, ExchangeCode mints a JWT signed with the private half, Authenticate
// validates the JWT and surfaces the granted roles.
func TestMockAuthenticatorMintsValidJWT(t *testing.T) {
	t.Parallel()

	ctx, dbpool := auth.TestingInitPool(t)
	cb := func() string { return "https://example.test/auth/callback" }
	a, errE := auth.NewMockAuthenticator(ctx, dbpool, "example.test", func() []string { return []string{"admin", "editor"} }, cb)
	require.NoError(t, errE, "% -+#.1v", errE)

	token, expiry, errE := a.TestingExchangeCode(ctx, "mock", "verifier", "nonce-abc")
	require.NoError(t, errE, "% -+#.1v", errE)
	require.NotEmpty(t, token)
	assert.True(t, expiry.After(time.Now()))

	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/whatever", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	ctx = a.Authenticate(w, req, "", map[string][]string{"admin": nil, "editor": nil})

	_, ok := auth.Subject(ctx)
	require.True(t, ok)
	assert.ElementsMatch(t, []string{"admin", "editor"}, auth.Roles(ctx))

	// UserInfo header carries the mock's preferred_username pre-primed
	// during ExchangeCode, so a sign-in immediately surfaces a non-empty
	// username without ever touching an upstream userinfo endpoint.
	userInfoHeader := w.Header().Get("Userinfo")
	assert.Contains(t, userInfoHeader, `username="mock"`)
}

// TestMockAuthenticatorAuthCodeURLPointsAtCallback covers the self-redirect:
// the URL returned by AuthCodeURL is the same absolute callback URL the
// OIDC flow would use, with state baked into the query so the callback can
// consume the matching flow-store row.
func TestMockAuthenticatorAuthCodeURLPointsAtCallback(t *testing.T) {
	t.Parallel()

	ctx, dbpool := auth.TestingInitPool(t)
	cb := func() string { return "https://example.test/auth/callback" }
	a, errE := auth.NewMockAuthenticator(ctx, dbpool, "example.test", nil, cb)
	require.NoError(t, errE, "% -+#.1v", errE)

	url := a.TestingAuthCodeURL("state-xyz", "verifier", "nonce")
	assert.Contains(t, url, "https://example.test/auth/callback")
	assert.Contains(t, url, "state=state-xyz")
	assert.Contains(t, url, "code=")
}

// TestMockAuthenticatorFiltersRolesByAllowedSet covers the same allowlist
// behaviour as the OIDC path: a mock-minted token claims every role the
// site grants, but Authenticate still filters those down to the caller's
// allowedRoles set.
func TestMockAuthenticatorFiltersRolesByAllowedSet(t *testing.T) {
	t.Parallel()

	ctx, dbpool := auth.TestingInitPool(t)
	cb := func() string { return "https://example.test/auth/callback" }
	a, errE := auth.NewMockAuthenticator(ctx, dbpool, "example.test", func() []string { return []string{"admin", "editor"} }, cb)
	require.NoError(t, errE, "% -+#.1v", errE)

	token, _, errE := a.TestingExchangeCode(ctx, "mock", "", "nonce")
	require.NoError(t, errE, "% -+#.1v", errE)

	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/whatever", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	// Allowed set excludes "editor": even though the mock-minted JWT
	// claimed both, Authenticate must drop the unallowed one.
	ctx = a.Authenticate(w, req, "", map[string][]string{"admin": nil})

	assert.Equal(t, []string{"admin"}, auth.Roles(ctx))
}

// TestMockAuthenticatorResolvesRolesAtSignIn covers the lazy granted-roles
// thunk: roles configured on the site after the authenticator is built must
// still be claimed by a mock-minted token. This mirrors an application that
// populates its site roles in a setup step that runs after Init has already
// constructed the authenticator.
func TestMockAuthenticatorResolvesRolesAtSignIn(t *testing.T) {
	t.Parallel()

	ctx, dbpool := auth.TestingInitPool(t)
	cb := func() string { return "https://example.test/auth/callback" }

	// The thunk reads a variable that is still empty at construction time and
	// only populated afterwards. The mock must reflect the later value, not an
	// empty snapshot taken when it was built.
	var roles []string
	a, errE := auth.NewMockAuthenticator(ctx, dbpool, "example.test", func() []string { return roles }, cb)
	require.NoError(t, errE, "% -+#.1v", errE)

	roles = []string{"admin", "editor"}

	token, _, errE := a.TestingExchangeCode(ctx, "mock", "", "nonce")
	require.NoError(t, errE, "% -+#.1v", errE)

	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/whatever", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	ctx = a.Authenticate(w, req, "", map[string][]string{"admin": nil, "editor": nil})

	assert.ElementsMatch(t, []string{"admin", "editor"}, auth.Roles(ctx))
}

// TestMockAuthenticatorRequiresDomainAndRedirectURI covers the
// NewMockAuthenticator preconditions: both siteDomain and redirectURI must
// be non-empty.
func TestMockAuthenticatorRequiresDomainAndRedirectURI(t *testing.T) {
	t.Parallel()

	ctx, dbpool := auth.TestingInitPool(t)
	cb := func() string { return "https://example.test/auth/callback" }

	_, errE := auth.NewMockAuthenticator(ctx, dbpool, "", nil, cb)
	require.Error(t, errE)

	_, errE = auth.NewMockAuthenticator(ctx, dbpool, "example.test", nil, nil)
	require.Error(t, errE)

	_, errE = auth.NewMockAuthenticator(ctx, nil, "example.test", nil, cb)
	require.Error(t, errE)
}

// TestMockAuthenticatorIsolatesPerSite covers the per-site isolation: a
// token minted by one MockAuthenticator is rejected by another. The
// signatures disagree (different RSA keys), but even if signing were
// somehow shared the issuer/audience claims differ by domain, so
// validation fails at the iss/aud check first.
func TestMockAuthenticatorIsolatesPerSite(t *testing.T) {
	t.Parallel()

	ctx, dbpool := auth.TestingInitPool(t)
	cb := func() string { return "https://example.test/auth/callback" }
	siteA, errE := auth.NewMockAuthenticator(ctx, dbpool, "a.example", func() []string { return []string{"admin"} }, cb)
	require.NoError(t, errE, "% -+#.1v", errE)
	siteB, errE := auth.NewMockAuthenticator(ctx, dbpool, "b.example", func() []string { return []string{"admin"} }, cb)
	require.NoError(t, errE, "% -+#.1v", errE)

	tokenA, _, errE := siteA.TestingExchangeCode(ctx, "mock", "", "nonce")
	require.NoError(t, errE, "% -+#.1v", errE)

	// Site B sees the token but its tokenVerifier expects a different
	// issuer/audience and a different signing key, so the request is
	// treated as unauthenticated: no subject, no roles, no UserInfo header.
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/whatever", nil)
	req.Header.Set("Authorization", "Bearer "+tokenA)
	w := httptest.NewRecorder()

	ctx = siteB.Authenticate(w, req, "", map[string][]string{"admin": nil})

	_, ok := auth.Subject(ctx)
	assert.False(t, ok)
	assert.Empty(t, auth.Roles(ctx))
	assert.Empty(t, w.Header().Get("Userinfo"))
}

// TestSignOutRevokesToken covers the end-to-end revocation path: a
// freshly minted token authenticates, SignOut writes it to the
// revocation store (and primes the cache), and a subsequent
// Authenticate carrying the same token is treated as unauthenticated.
func TestSignOutRevokesToken(t *testing.T) {
	t.Parallel()

	ctx, dbpool := auth.TestingInitPool(t)
	cb := func() string { return "https://example.test/auth/callback" }
	a, errE := auth.NewMockAuthenticator(ctx, dbpool, "example.test", func() []string { return []string{"admin"} }, cb)
	require.NoError(t, errE, "% -+#.1v", errE)

	token, _, errE := a.TestingExchangeCode(ctx, "mock", "", "nonce")
	require.NoError(t, errE, "% -+#.1v", errE)

	// Baseline: Authenticate accepts the token (revocation store is
	// empty, IsRevoked returns false, cache memoises that as "not
	// revoked").
	{
		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/whatever", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		authCtx := a.Authenticate(w, req, "", map[string][]string{"admin": nil})
		_, ok := auth.Subject(authCtx)
		require.True(t, ok, "first Authenticate should accept the token")
	}

	// Revoke the same token via SignOut. The cookie carries the
	// access token in production - here we put it on Authorization
	// for test convenience; the extraction logic accepts either.
	soReq := httptest.NewRequestWithContext(ctx, http.MethodPost, "/auth/signOut", nil)
	soReq.Header.Set("Authorization", "Bearer "+token)
	soW := httptest.NewRecorder()
	errE = a.SignOut(soW, soReq)
	require.NoError(t, errE, "% -+#.1v", errE)

	// After SignOut: Authenticate must reject the same token. The
	// JWT itself is still signature-valid and unexpired (mockTokenTTL
	// is 24h); only the revocation entry blocks it.
	{
		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/whatever", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		authCtx := a.Authenticate(w, req, "", map[string][]string{"admin": nil})
		_, ok := auth.Subject(authCtx)
		assert.False(t, ok, "Authenticate should reject the revoked token")
		assert.Empty(t, auth.Roles(authCtx))
		assert.Empty(t, w.Header().Get("Userinfo"))
	}
}

// TestSignOutWithoutTokenIsNoOp covers the early-return path: a
// request that carries no access token at all (no Bearer header, no
// cookie) signs out without error and without writing anything.
func TestSignOutWithoutTokenIsNoOp(t *testing.T) {
	t.Parallel()

	ctx, dbpool := auth.TestingInitPool(t)
	cb := func() string { return "https://example.test/auth/callback" }
	a, errE := auth.NewMockAuthenticator(ctx, dbpool, "example.test", nil, cb)
	require.NoError(t, errE, "% -+#.1v", errE)

	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/auth/signOut", nil)
	w := httptest.NewRecorder()
	errE = a.SignOut(w, req)
	assert.NoError(t, errE, "SignOut with no token attached must not error")
}

// TestSignOutWithInvalidTokenIsNoOp covers the JWT-validation guard:
// SignOut never writes a revocation for a token that fails JWT
// validation (forged, signed with a different key, etc.) so we cannot
// be tricked into populating the revocation store with arbitrary
// caller-controlled hashes.
func TestSignOutWithInvalidTokenIsNoOp(t *testing.T) {
	t.Parallel()

	ctx, dbpool := auth.TestingInitPool(t)
	cb := func() string { return "https://example.test/auth/callback" }
	a, errE := auth.NewMockAuthenticator(ctx, dbpool, "example.test", nil, cb)
	require.NoError(t, errE, "% -+#.1v", errE)

	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/auth/signOut", nil)
	req.Header.Set("Authorization", "Bearer not-a-real-jwt")
	w := httptest.NewRecorder()
	errE = a.SignOut(w, req)
	assert.NoError(t, errE, "SignOut with an unparseable token must not error")
}

// TestMockAuthenticatorSignInCallbackRoundTrip exercises the full
// SignIn -> Callback round-trip via the mock, end-to-end through the
// flow store. SignIn returns a URL whose query carries the state and
// code; feeding the same query back into Callback consumes the flow
// row and yields a valid access token plus the original redirect.
func TestMockAuthenticatorSignInCallbackRoundTrip(t *testing.T) {
	t.Parallel()

	ctx, dbpool := auth.TestingInitPool(t)
	cb := func() string { return "https://example.test/auth/callback" }
	a, errE := auth.NewMockAuthenticator(ctx, dbpool, "example.test", func() []string { return []string{"admin"} }, cb)
	require.NoError(t, errE, "% -+#.1v", errE)

	authURL, errE := a.SignIn(ctx, "/landing")
	require.NoError(t, errE, "% -+#.1v", errE)

	u, err := url.Parse(authURL)
	require.NoError(t, err)
	require.NotEmpty(t, u.Query().Get("state"))
	require.NotEmpty(t, u.Query().Get("code"))

	token, expiry, redirect, errE := a.Callback(ctx, u.Query())
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.NotEmpty(t, token)
	assert.True(t, expiry.After(time.Now()))
	assert.Equal(t, "/landing", redirect)

	// Single-use: a second Callback with the same state must return
	// the client-error sentinel so the route handler maps it to 400.
	_, _, _, errE = a.Callback(ctx, u.Query()) //nolint:dogsled
	require.Error(t, errE)
	assert.True(t, errors.Is(errE, auth.ErrSignInFailed), "second consume must wrap ErrSignInFailed")
}

// TestMockAuthenticatorSignInUnsafeRedirectFallsBackToRoot covers the
// open-redirect guard at SignIn: an off-site post-sign-in path is
// silently rewritten to "/" before being stored. Callback then returns
// the safe path.
func TestMockAuthenticatorSignInUnsafeRedirectFallsBackToRoot(t *testing.T) {
	t.Parallel()

	ctx, dbpool := auth.TestingInitPool(t)
	cb := func() string { return "https://example.test/auth/callback" }
	a, errE := auth.NewMockAuthenticator(ctx, dbpool, "example.test", nil, cb)
	require.NoError(t, errE, "% -+#.1v", errE)

	authURL, errE := a.SignIn(ctx, "//evil.example/phish")
	require.NoError(t, errE, "% -+#.1v", errE)
	u, err := url.Parse(authURL)
	require.NoError(t, err)

	_, _, redirect, errE := a.Callback(ctx, u.Query())
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, "/", redirect, "protocol-relative redirect must collapse to /")
}

// TestMockAuthenticatorCallbackMissingParams covers two malformed
// callback inputs: no state, no code. Both must wrap ErrSignInFailed.
func TestMockAuthenticatorCallbackMissingParams(t *testing.T) {
	t.Parallel()

	ctx, dbpool := auth.TestingInitPool(t)
	cb := func() string { return "https://example.test/auth/callback" }
	a, errE := auth.NewMockAuthenticator(ctx, dbpool, "example.test", nil, cb)
	require.NoError(t, errE, "% -+#.1v", errE)

	cases := []url.Values{
		{},                       // neither state nor code.
		{"state": []string{"s"}}, // missing code.
		{"code": []string{"c"}},  // missing state.
		{"state": []string{""}, "code": []string{"c"}}, // empty state.
	}
	for _, q := range cases {
		_, _, _, errE := a.Callback(ctx, q)
		require.Error(t, errE)
		assert.True(t, errors.Is(errE, auth.ErrSignInFailed), "missing params must wrap ErrSignInFailed: %v", q)
	}
}

// TestMockAuthenticatorCallbackIssuerError covers the OIDC-spec error
// path: when the issuer reports back via ?error=...&error_description=...
// the Callback surfaces ErrSignInFailed with both fields attached to
// errors.Details so the handler can log them.
func TestMockAuthenticatorCallbackIssuerError(t *testing.T) {
	t.Parallel()

	ctx, dbpool := auth.TestingInitPool(t)
	cb := func() string { return "https://example.test/auth/callback" }
	a, errE := auth.NewMockAuthenticator(ctx, dbpool, "example.test", nil, cb)
	require.NoError(t, errE, "% -+#.1v", errE)

	values := url.Values{
		"error":             []string{"access_denied"},
		"error_description": []string{"user clicked no"},
	}
	_, _, _, errE = a.Callback(ctx, values) //nolint:dogsled
	require.Error(t, errE)
	assert.True(t, errors.Is(errE, auth.ErrSignInFailed))
	assert.Equal(t, "access_denied", errors.Details(errE)["error"])
	assert.Equal(t, "user clicked no", errors.Details(errE)["description"])
}

// TestOIDCAuthenticatorSignOutCallsRevocationEndpoint covers the
// RFC 7009 round-trip from the OIDCAuthenticator side: we stand up a
// minimal OIDC-issuer mock that advertises a revocation_endpoint and
// records the POST it receives, mint a JWT with the same key the mock
// publishes via JWKS, and call SignOut. The middleware-level local
// revocation is best-effort goroutine, so we wait via Eventually.
func TestOIDCAuthenticatorSignOutCallsRevocationEndpoint(t *testing.T) {
	t.Parallel()

	ctx, dbpool := auth.TestingInitPool(t)

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	var (
		revokeCalls    atomic.Int32
		lastAuthHeader string
		lastBody       url.Values
		lastBodyMu     sync.Mutex
	)

	mux := http.NewServeMux()
	var issuerURL string

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                issuerURL,
			"jwks_uri":                              issuerURL + "/keys",
			"authorization_endpoint":                issuerURL + "/auth",
			"token_endpoint":                        issuerURL + "/token",
			"revocation_endpoint":                   issuerURL + "/revoke",
			"response_types_supported":              []string{"code"},
			"subject_types_supported":               []string{"public"},
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})

	mux.HandleFunc("/keys", func(w http.ResponseWriter, _ *http.Request) {
		jwks := struct {
			Keys []jose.JSONWebKey `json:"keys"`
		}{
			Keys: []jose.JSONWebKey{{ //nolint:exhaustruct
				Key:       priv.Public(),
				KeyID:     "test-key",
				Algorithm: "RS256",
				Use:       "sig",
			}},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jwks)
	})

	mux.HandleFunc("/revoke", func(w http.ResponseWriter, r *http.Request) {
		revokeCalls.Add(1)
		// require.* calls FailNow on the test goroutine and is unsafe
		// from a handler goroutine; use assert here and let the rest of
		// the test surface the failure.
		assert.NoError(t, r.ParseForm())
		lastBodyMu.Lock()
		lastAuthHeader = r.Header.Get("Authorization")
		lastBody = r.Form
		lastBodyMu.Unlock()
		w.WriteHeader(http.StatusOK)
	})

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	issuerURL = ts.URL

	secretPath := filepath.Join(t.TempDir(), "client-secret")
	require.NoError(t, os.WriteFile(secretPath, []byte("test-secret"), 0o600))

	cb := func() string { return "https://example.test/auth/callback" }
	a, errE := auth.NewOIDCAuthenticator(ctx, dbpool, ts.URL, "test-client", secretPath, cb)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Mint an access token directly with claims the OIDCAuthenticator's
	// tokenVerifier will accept (issuer matches, audience matches the
	// configured clientID, unexpired).
	token := signedToken(t, priv, map[string]any{
		"iss": ts.URL,
		"aud": "test-client",
		"sub": "user-1",
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(time.Hour).Unix(),
	})

	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/auth/signOut", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	errE = a.SignOut(w, req)
	require.NoError(t, errE, "% -+#.1v", errE)

	// SignOut fires revokeUpstream in a goroutine so the response can
	// return immediately. Wait for the upstream POST to land.
	require.Eventually(t, func() bool {
		return revokeCalls.Load() == 1
	}, 5*time.Second, 50*time.Millisecond, "expected exactly one upstream revocation call")

	lastBodyMu.Lock()
	defer lastBodyMu.Unlock()

	assert.Equal(t, token, lastBody.Get("token"))
	assert.Equal(t, "access_token", lastBody.Get("token_type_hint"))

	// RFC 6749 §2.3.1: client_id and client_secret are URL-encoded
	// before being used as the Basic-auth credentials.
	expectedAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte(
		url.QueryEscape("test-client")+":"+url.QueryEscape("test-secret"),
	))
	assert.Equal(t, expectedAuth, lastAuthHeader)
}

// TestOIDCAuthenticatorSignOutSwallowsUpstreamFailure covers the
// best-effort contract: when the issuer's revocation_endpoint returns
// a non-200, the local revocation has already succeeded and SignOut
// must still return nil. The user is locally signed out regardless of
// whether the upstream cooperates.
func TestOIDCAuthenticatorSignOutSwallowsUpstreamFailure(t *testing.T) {
	t.Parallel()

	ctx, dbpool := auth.TestingInitPool(t)

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	var revokeCalls atomic.Int32
	mux := http.NewServeMux()
	var issuerURL string

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                issuerURL,
			"jwks_uri":                              issuerURL + "/keys",
			"authorization_endpoint":                issuerURL + "/auth",
			"token_endpoint":                        issuerURL + "/token",
			"revocation_endpoint":                   issuerURL + "/revoke",
			"response_types_supported":              []string{"code"},
			"subject_types_supported":               []string{"public"},
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/keys", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(struct {
			Keys []jose.JSONWebKey `json:"keys"`
		}{Keys: []jose.JSONWebKey{{ //nolint:exhaustruct
			Key: priv.Public(), KeyID: "test-key", Algorithm: "RS256", Use: "sig",
		}}})
	})
	mux.HandleFunc("/revoke", func(w http.ResponseWriter, _ *http.Request) {
		revokeCalls.Add(1)
		http.Error(w, "issuer is down", http.StatusServiceUnavailable)
	})

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	issuerURL = ts.URL

	secretPath := filepath.Join(t.TempDir(), "client-secret")
	require.NoError(t, os.WriteFile(secretPath, []byte("test-secret"), 0o600))

	cb := func() string { return "https://example.test/auth/callback" }
	a, errE := auth.NewOIDCAuthenticator(ctx, dbpool, ts.URL, "test-client", secretPath, cb)
	require.NoError(t, errE, "% -+#.1v", errE)

	token := signedToken(t, priv, map[string]any{
		"iss": ts.URL,
		"aud": "test-client",
		"sub": "user-1",
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(time.Hour).Unix(),
	})

	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/auth/signOut", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	// SignOut must not surface the upstream failure - local revocation
	// has already succeeded.
	errE = a.SignOut(w, req)
	require.NoError(t, errE, "upstream 503 must not propagate to SignOut: % -+#.1v", errE)

	require.Eventually(t, func() bool {
		return revokeCalls.Load() == 1
	}, 5*time.Second, 50*time.Millisecond, "upstream endpoint should still be called even when it will fail")
}
