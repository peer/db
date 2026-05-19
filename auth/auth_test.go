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

	verifier, errE := auth.New(t.Context(), ts.URL, testAudience)
	require.NoError(t, errE, "% -+#.1v", errE)
	return verifier, ts.URL, priv
}

func TestNewRequiresIssuerAndClientID(t *testing.T) {
	t.Parallel()

	_, errE := auth.New(t.Context(), "", "client")
	require.Error(t, errE)

	_, errE = auth.New(t.Context(), "https://example.test", "")
	require.Error(t, errE)
}

func TestRequireAuthenticatedHappyPath(t *testing.T) {
	t.Parallel()

	const audience = "peerdb"
	verifier, issuer, priv := newTestVerifier(t)

	token := signedToken(t, priv, map[string]any{
		"iss":   issuer,
		"aud":   audience,
		"sub":   "user-123",
		"exp":   strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10),
		"scope": "openid profile email role.admin role.editor",
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/whatever", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	ctx := verifier.RequireAuthenticated(w, req)
	require.NotNil(t, ctx, "expected authentication to succeed; response: %d %s", w.Code, w.Body.String())

	subject, ok := auth.Subject(ctx)
	require.True(t, ok)
	assert.Equal(t, "user-123", subject)
	assert.ElementsMatch(t, []string{"admin", "editor"}, auth.Roles(ctx))
	assert.True(t, auth.HasRole(ctx, "admin"))
	assert.False(t, auth.HasRole(ctx, "viewer"))

	// Responses depend on the Authorization header, so the Verifier must say so.
	assert.Equal(t, []string{"Authorization"}, w.Header().Values("Vary"))
}

func TestRequireAuthenticatedRejectsWrongAudience(t *testing.T) {
	t.Parallel()

	verifier, issuer, priv := newTestVerifier(t)

	token := signedToken(t, priv, map[string]any{
		"iss": issuer,
		"aud": "someone-else",
		"sub": "user-123",
		"exp": strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10),
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/whatever", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	ctx := verifier.RequireAuthenticated(w, req)
	assert.Nil(t, ctx)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRequireAuthenticatedRejectsExpiredToken(t *testing.T) {
	t.Parallel()

	const audience = "peerdb"
	verifier, issuer, priv := newTestVerifier(t)

	token := signedToken(t, priv, map[string]any{
		"iss": issuer,
		"aud": audience,
		"sub": "user-123",
		"exp": strconv.FormatInt(time.Now().Add(-time.Hour).Unix(), 10),
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/whatever", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	ctx := verifier.RequireAuthenticated(w, req)
	assert.Nil(t, ctx)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRequireAuthenticatedRejectsMissingHeader(t *testing.T) {
	t.Parallel()

	verifier, _, _ := newTestVerifier(t)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/whatever", nil)
	w := httptest.NewRecorder()

	ctx := verifier.RequireAuthenticated(w, req)
	assert.Nil(t, ctx)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRequireAuthenticatedRejectsMalformedToken(t *testing.T) {
	t.Parallel()

	verifier, _, _ := newTestVerifier(t)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/whatever", nil)
	req.Header.Set("Authorization", "Bearer not-a-jwt")
	w := httptest.NewRecorder()

	ctx := verifier.RequireAuthenticated(w, req)
	assert.Nil(t, ctx)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRequireAuthenticatedNoRoles(t *testing.T) {
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

	ctx := verifier.RequireAuthenticated(w, req)
	require.NotNil(t, ctx)
	assert.Empty(t, auth.Roles(ctx))
}

func TestRequireAuthenticatedFiltersRoleWildcard(t *testing.T) {
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

	ctx := verifier.RequireAuthenticated(w, req)
	require.NotNil(t, ctx)
	assert.Equal(t, []string{"editor"}, auth.Roles(ctx))
}

func TestRequireAuthenticatedReadsScpArray(t *testing.T) {
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

	ctx := verifier.RequireAuthenticated(w, req)
	require.NotNil(t, ctx)
	assert.ElementsMatch(t, []string{"admin", "viewer"}, auth.Roles(ctx))
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

func TestMaybeAuthenticatedAttachesValidToken(t *testing.T) {
	t.Parallel()

	const audience = "peerdb"
	verifier, issuer, priv := newTestVerifier(t)

	token := signedToken(t, priv, map[string]any{
		"iss":   issuer,
		"aud":   audience,
		"sub":   "user-42",
		"exp":   strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10),
		"scope": "role.editor",
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/whatever", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	ctx := verifier.MaybeAuthenticated(w, req)

	subject, ok := auth.Subject(ctx)
	require.True(t, ok)
	assert.Equal(t, "user-42", subject)
	assert.Equal(t, []string{"editor"}, auth.Roles(ctx))
	// The response is left untouched apart from the Vary header.
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, []string{"Authorization"}, w.Header().Values("Vary"))
}

func TestMaybeAuthenticatedSilentlyDropsBadToken(t *testing.T) {
	t.Parallel()

	const audience = "peerdb"
	verifier, issuer, priv := newTestVerifier(t)

	tests := []struct {
		name   string
		header string
	}{
		{"no header", ""},
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
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			w := httptest.NewRecorder()

			ctx := verifier.MaybeAuthenticated(w, req)

			// No response written, ctx carries no identity.
			assert.Equal(t, http.StatusOK, w.Code)
			_, ok := auth.Subject(ctx)
			assert.False(t, ok)
			assert.Empty(t, auth.Roles(ctx))
			// Vary is still set even when no token was presented so cached
			// responses correctly key on Authorization.
			assert.Equal(t, []string{"Authorization"}, w.Header().Values("Vary"))
		})
	}
}

func TestMiddlewareAttachesIdentityToDownstreamHandler(t *testing.T) {
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

	var seenSubject string
	var seenRoles []string
	handler := verifier.Middleware()(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seenSubject, _ = auth.Subject(r.Context())
		seenRoles = auth.Roles(r.Context())
	}))

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/whatever", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "user-77", seenSubject)
	assert.Equal(t, []string{"admin"}, seenRoles)
}

func TestMiddlewareLeavesAnonymousRequestsAlone(t *testing.T) {
	t.Parallel()

	verifier, _, _ := newTestVerifier(t)

	called := false
	handler := verifier.Middleware()(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		called = true
		_, ok := auth.Subject(r.Context())
		assert.False(t, ok)
		assert.Empty(t, auth.Roles(r.Context()))
	}))

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/whatever", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.True(t, called, "downstream handler must run for anonymous requests")
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestIssuerAndClientIDExposedOnVerifier(t *testing.T) {
	t.Parallel()

	const audience = "peerdb"
	verifier, issuer, _ := newTestVerifier(t)
	assert.Equal(t, issuer, verifier.Issuer())
	assert.Equal(t, audience, verifier.ClientID())
}
