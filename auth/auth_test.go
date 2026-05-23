package auth_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/coreos/go-oidc/v3/oidc/oidctest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/peerdb/peerdb/auth"
)

// signedToken builds a JWT with the given claims signed by the test issuer's
// private key. We sign access tokens with the same key the OIDC server
// advertises so the Verifier can validate them.
func signedToken(t *testing.T, priv *rsa.PrivateKey, claims map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(claims)
	require.NoError(t, err)
	return oidctest.SignIDToken(priv, "test-key", oidc.RS256, string(raw))
}

// testAudience is the audience the Verifier expects in every test token. Each
// test redeclares it locally for readability.
const testAudience = "peerdb"

// newTestVerifier spins up an oidctest server and returns a Verifier wired to it,
// along with the issuer URL and the private signing key callers can use to mint tokens.
func newTestVerifier(t *testing.T) (*auth.Verifier, string, *rsa.PrivateKey) {
	t.Helper()

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

	cb := func() string { return "https://example.test/auth/callback" }
	verifier, errE := auth.New(t.Context(), ts.URL, testAudience, "test-secret", cb)
	require.NoError(t, errE, "% -+#.1v", errE)
	return verifier, ts.URL, priv
}

func TestNewRequiresIssuerClientIDSecretAndRedirect(t *testing.T) {
	t.Parallel()

	cb := func() string { return "https://example.test/cb" }

	_, errE := auth.New(t.Context(), "", "client", "secret", cb)
	require.Error(t, errE)

	_, errE = auth.New(t.Context(), "https://example.test", "", "secret", cb)
	require.Error(t, errE)

	_, errE = auth.New(t.Context(), "https://example.test", "client", "", cb)
	require.Error(t, errE)

	_, errE = auth.New(t.Context(), "https://example.test", "client", "secret", nil)
	require.Error(t, errE)
}

func TestAuthenticateNoRoles(t *testing.T) {
	t.Parallel()

	const audience = "peerdb"
	verifier, issuer, priv := newTestVerifier(t)

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

	ctx := verifier.Authenticate(w, req, "", nil)
	subject, ok := auth.Subject(ctx)
	require.True(t, ok)
	assert.Equal(t, "user-123", subject)
	assert.Empty(t, auth.Roles(ctx))
}

func TestAuthenticateFiltersRoleWildcard(t *testing.T) {
	t.Parallel()

	// In practice a granted scope is never the bare "role.*" wildcard - Charon
	// expands it before issuing the token. The Verifier still filters it out
	// defensively in case an OIDC provider passes it through.
	const audience = "peerdb"
	verifier, issuer, priv := newTestVerifier(t)

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

	ctx := verifier.Authenticate(w, req, "", map[string][]string{"editor": nil})
	assert.Equal(t, []string{"editor"}, auth.Roles(ctx))
}

func TestAuthenticateReadsScpArray(t *testing.T) {
	t.Parallel()

	const audience = "peerdb"
	verifier, issuer, priv := newTestVerifier(t)

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

	ctx := verifier.Authenticate(w, req, "", map[string][]string{"admin": nil, "viewer": nil})
	assert.ElementsMatch(t, []string{"admin", "viewer"}, auth.Roles(ctx))
}

func TestAuthenticateSilentlyDropsBadToken(t *testing.T) {
	t.Parallel()

	const audience = "peerdb"
	verifier, issuer, priv := newTestVerifier(t)

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

			ctx := verifier.Authenticate(w, req, "", nil)
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

	ctx := auth.TestingWithSubject(context.Background(), "user-42")
	ctx = auth.TestingWithRoles(ctx, []string{"admin", "editor"})

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
	verifier, issuer, priv := newTestVerifier(t)

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

	ctx := verifier.Authenticate(w, req, "", map[string][]string{"admin": nil})

	subject, ok := auth.Subject(ctx)
	require.True(t, ok)
	assert.Equal(t, "user-77", subject)
	assert.Equal(t, []string{"admin"}, auth.Roles(ctx))
}

func TestAuthenticateLeavesAnonymousRequestsAlone(t *testing.T) {
	t.Parallel()

	verifier, _, _ := newTestVerifier(t)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/whatever", nil)
	w := httptest.NewRecorder()

	ctx := verifier.Authenticate(w, req, "", nil)
	_, ok := auth.Subject(ctx)
	assert.False(t, ok)
	assert.Empty(t, auth.Roles(ctx))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestIssuerAndClientIDExposedOnVerifier(t *testing.T) {
	t.Parallel()

	const audience = "peerdb"
	verifier, issuer, _ := newTestVerifier(t)
	assert.Equal(t, issuer, verifier.Issuer())
	assert.Equal(t, audience, verifier.ClientID())
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
	// cached anonymous response.
	vary := w.Header().Values("Vary")
	assert.Contains(t, vary, "Authorization")
	assert.Contains(t, vary, "Cookie")
}

func TestAuthenticateValidatesCookieToken(t *testing.T) {
	t.Parallel()

	const audience = "peerdb"
	verifier, issuer, priv := newTestVerifier(t)

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

	ctx := verifier.Authenticate(w, req, "", map[string][]string{"editor": nil})

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

	verifier, _, _ := newTestVerifier(t)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/whatever", nil)
	w := httptest.NewRecorder()

	ctx := verifier.Authenticate(w, req, "", nil)
	_, ok := auth.Subject(ctx)
	assert.False(t, ok)
	assert.Empty(t, w.Header().Get("Roles"))
	assert.Empty(t, w.Header().Get("Userinfo"))
}

// TestAuthenticateDropsRolesNotInAllowedSet covers the allowlist behaviour:
// a token may claim arbitrary "role.<key>" scopes, but the Verifier only
// surfaces those that the caller has declared as legitimate via the
// allowedRoles set. Roles the token claims but the site has not declared
// are silently dropped so they cannot leak into auth.Roles or the Roles
// response header.
func TestAuthenticateDropsRolesNotInAllowedSet(t *testing.T) {
	t.Parallel()

	const audience = "peerdb"
	verifier, issuer, priv := newTestVerifier(t)

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

	ctx := verifier.Authenticate(w, req, "", map[string][]string{"admin": nil, "editor": nil})

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
	verifier, issuer, priv := newTestVerifier(t)

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

	ctx := verifier.Authenticate(w, req, "", nil)

	subject, ok := auth.Subject(ctx)
	require.True(t, ok)
	assert.Equal(t, "user-noroles", subject)
	assert.Empty(t, auth.Roles(ctx))
}
