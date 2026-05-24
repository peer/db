// Package auth verifies OIDC-issued JWT access tokens presented by API clients
// and attaches the resulting identity (subject and roles) to the request context.
//
// Tokens are validated against the JSON Web Key Set discovered from the
// configured OIDC issuer. The expected audience matches the configured client ID.
// Roles are extracted from the scope claim, taking every scope under the
// "role." namespace (for example, "role.admin" becomes "admin"); the wildcard
// "role.*" is ignored if encountered.
//
// The package also drives the backend-side OIDC authorization code flow used
// by the sign-in routes via Start and Callback. Both methods are backed by an
// internal per-site flow store so callers do not need to thread flow state
// around. Identity gathered from a validated token (subject, roles, profile)
// is exposed to downstream responses as SFV-encoded HTTP headers ("Roles"
// and "UserInfo", prefixed by the WAF service's MetadataHeaderPrefix).
//
// Two implementations of the Authenticator interface are provided.
// OIDCAuthenticator drives a real OpenID Connect authorization-code flow
// against an external issuer. MockAuthenticator short-circuits the flow
// for development by minting JWTs against an in-process key pair. It is
// intended for development only.
package auth

import (
	"bytes"
	"context"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"
	"gitlab.com/tozd/waf"
	"golang.org/x/oauth2"
)

// roleScopePrefix uses Charon's scope convention: every scope starting with
// this prefix grants the named role to the caller.
const roleScopePrefix = "role."

// roleScopeWildcard is the namespace wildcard that Charon expands into individual
// "role.<key>" scopes. It should never appear in granted scopes, but we filter
// it out defensively in case some OIDC providers pass it through.
const roleScopeWildcard = "role.*"

// rolesHeader and userInfoHeader are the names the auth middleware writes
// onto every response with a validated access token. The WAF service's
// MetadataHeaderPrefix (if configured) is prepended in front of each.
const (
	rolesHeader    = "Roles"
	userInfoHeader = "UserInfo"
)

// ErrSignInFailed marks every client-side failure from Callback: malformed
// callback parameters, an "error" response from the issuer, a replayed or
// expired flow row, or a token-exchange / JWT-validation failure. Route
// handlers should map errors that wrap this sentinel to HTTP 400 and treat
// any other Callback error as an internal-server (500) condition.
var ErrSignInFailed = errors.Base("sign-in failed")

// upstreamRevokeTimeout caps how long the background upstream
// revocation call may run. A slow or dead issuer cannot accumulate
// blocked goroutines beyond this bound.
const upstreamRevokeTimeout = 30 * time.Second

// Authenticator validates access-token credentials and drives the
// backend-side sign-in flow. Concrete implementations: OIDCAuthenticator
// (real OpenID Connect against an external issuer) and MockAuthenticator
// (in-process JWT minting for development). One Authenticator is built per
// site because each site has its own client and per-domain redirect URI.
type Authenticator interface {
	// Authenticate validates the caller's access token and, on success,
	// returns the request context enriched with subject and roles AND
	// writes the Roles / UserInfo response headers consumed by the
	// frontend. On failure the original ctx is returned unchanged and no
	// headers are written.
	Authenticate(w http.ResponseWriter, req *http.Request, metadataHeaderPrefix string, allowedRoles map[string][]string) context.Context

	// SignIn begins a fresh sign-in flow.
	SignIn(ctx context.Context, redirect string) (authURL string, errE errors.E)

	// Callback finishes a sign-in flow.
	//
	// Every client-side failure is wrapped with ErrSignInFailed. Internal errors
	// are returned without that wrapping.
	Callback(ctx context.Context, values url.Values) (token string, expiry time.Time, redirect string, errE errors.E)

	// SignOut revokes the access token the caller presented.
	SignOut(w http.ResponseWriter, req *http.Request) errors.E

	// CleanupExpired prunes rows that have aged out of the
	// Authenticator's internal flow and revocation stores. It is meant
	// to be called from a periodic background job. Errors from either
	// store are joined so a partial failure is still surfaced.
	CleanupExpired(ctx context.Context) errors.E
}

// baseAuthenticator holds the state shared between OIDCAuthenticator and
// MockAuthenticator: the token verifier (which key set differs but the
// validation contract is the same), the userinfo cache (mock primes it at
// sign-in so the cache is always warm, OIDC re-fetches from the issuer on
// miss), the per-site flow store, and the per-site revocation store that
// remembers which tokens were explicitly signed out.
type baseAuthenticator struct {
	tokenVerifier   *oidc.IDTokenVerifier
	userInfoCache   *userInfoCache
	flowStore       *flowStore
	revocationStore *revocationStore
}

// Authenticate validates the caller's access token (Authorization Bearer
// first, falling back to the session cookie) and, on success, returns
// the request context enriched with subject and roles AND writes two
// response headers consumed by the frontend:
//
//   - "<prefix>Roles": the role list as an SFV inner-list.
//   - "<prefix>UserInfo": an SFV dictionary with subject (always) and
//     username (when known).
//
// metadataHeaderPrefix should be the WAF service's MetadataHeaderPrefix so
// the auth headers stack with the existing Metadata header pattern.
//
// allowedRoles is the allowlist of role names the caller is permitted to
// receive. Any role granted by the token that is not a key in this map is
// silently dropped. Only keys are consulted, values are ignored. A nil or
// empty map yields an empty role set even when the token carries role
// scopes.
//
// The userinfo for the UserInfo header is read from an in-memory cache.
// Concurrent requests for the same subject coalesce into a single upstream
// call to the issuer's userinfo endpoint (singleflight).
//
// On any validation failure the original ctx is returned unchanged and no
// headers are written. Callers should treat that as an unauthenticated request
// and continue handling.
func (b *baseAuthenticator) Authenticate(w http.ResponseWriter, req *http.Request, metadataHeaderPrefix string, allowedRoles map[string][]string) context.Context {
	ctx := req.Context()
	token, _ := resolveAccessToken(w, req)
	if token == "" {
		return ctx
	}
	// The token is an access token (what the cookie / Bearer header
	// carries), not an ID token. go-oidc only exposes IDTokenVerifier
	// for JWT validation, so the returned *oidc.IDToken is just a
	// parsed-JWT struct here. The validation contract (signature,
	// issuer, audience, expiry) is the same either way.
	claims, err := b.tokenVerifier.Verify(ctx, token)
	if err != nil {
		return ctx
	}
	// Revocation check: a token that passed the JWT signature/exp gate
	// may still have been explicitly signed out.
	revoked, errE := b.revocationStore.IsRevoked(ctx, token, claims.Expiry)
	if errE == nil && revoked {
		return ctx
	}
	// Database errors fail open: trust the JWT validation we already
	// passed rather than locking everyone out on a transient outage.
	// IsRevoked deliberately does not cache the result on error so
	// the next request will try again.
	if errE != nil {
		zerolog.Ctx(ctx).Warn().Err(errE).Msg("revocation store error")
	}
	roles, errE := extractRoles(claims, allowedRoles)
	if errE != nil {
		return ctx
	}
	ctx = withSubject(ctx, claims.Subject)
	ctx = withRoles(ctx, roles)
	b.writeRolesHeader(w, metadataHeaderPrefix, roles)
	b.writeUserInfoHeader(ctx, w, metadataHeaderPrefix, claims.Subject, token)
	// Authenticated responses carry per-user data, keep them out of
	// shared caches. Browser caches still store them (keyed by
	// Authorization / Cookie via the Vary headers resolveAccessToken
	// sets).
	w.Header().Set("Cache-Control", "private")
	return ctx
}

// writeRolesHeader emits the Roles response header as an SFV list of
// strings (one entry per role). Empty role sets do not emit a header.
// The frontend should use the presence of the UserInfo header (always
// set when authenticated) to tell "unauthenticated" from "signed in" and not
// Roles header.
func (b *baseAuthenticator) writeRolesHeader(w http.ResponseWriter, prefix string, roles []string) {
	if len(roles) == 0 {
		return
	}
	list := make([]any, len(roles))
	for i, r := range roles {
		list[i] = r
	}
	buf := &bytes.Buffer{}
	errE := waf.EncodeMetadataList(list, buf)
	if errE != nil {
		return
	}
	w.Header().Add(prefix+rolesHeader, buf.String())
}

// writeUserInfoHeader emits the UserInfo response header, falling back to
// a subject-only payload when the upstream userinfo lookup fails or has not
// yet populated the cache. Subject is guaranteed to be present so the
// frontend can always learn the identity of the signed-in user, even when
// the issuer is unreachable.
func (b *baseAuthenticator) writeUserInfoHeader(ctx context.Context, w http.ResponseWriter, prefix, subject, token string) {
	info, _ := b.userInfoCache.Get(ctx, subject, token)
	if info.Subject == "" {
		info.Subject = subject
	}

	metadata := map[string]any{"subject": info.Subject}
	if info.Username != "" {
		metadata["username"] = info.Username
	}

	buf := &bytes.Buffer{}
	errE := waf.EncodeMetadata(metadata, buf)
	if errE != nil {
		return
	}
	w.Header().Add(prefix+userInfoHeader, buf.String())
}

// CleanupExpired prunes expired rows from both the flow store and the
// revocation store. Each store is asked to clean up independently so a
// transient failure in one does not leave the other unpruned. Errors
// from the two stores are joined so the caller learns about both.
func (b *baseAuthenticator) CleanupExpired(ctx context.Context) errors.E {
	errFlow := b.flowStore.cleanupExpired(ctx)
	errRevocation := b.revocationStore.cleanupExpired(ctx)
	return errors.Join(errFlow, errRevocation)
}

// signInFlow is the shared body of OIDCAuthenticator.SignIn and
// MockAuthenticator.SignIn. It sanitises the redirect, generates
// fresh state / PKCE verifier / nonce values, persists them, and
// delegates to the authenticator-specific authCodeURL builder
// for the final URL.
func signInFlow(
	ctx context.Context,
	fs *flowStore,
	redirect string,
	authCodeURL func(state, codeVerifier, nonce string) string,
) (string, errors.E) {
	if fs == nil {
		return "", errors.New("authenticator has no flow store")
	}

	redirect = safeRedirectPath(redirect)

	state := identifier.New().String()
	codeVerifier := oauth2.GenerateVerifier()
	nonce := identifier.New().String()

	errE := fs.BeginFlow(ctx, state, flowState{
		codeVerifier: codeVerifier,
		nonce:        nonce,
		redirect:     redirect,
	})
	if errE != nil {
		return "", errE
	}

	return authCodeURL(state, codeVerifier, nonce), nil
}

// callbackFlow is the shared body of OIDCAuthenticator.Callback and
// MockAuthenticator.Callback. It validates the query parameters, consumes
// the matching flow row, and delegates to the authenticator-specific
// exchangeCode for the actual code-to-token exchange.
//
// Every client-side failure is wrapped with ErrSignInFailed so the route
// handler can distinguish "user-induced" (HTTP 400) from "internal" (HTTP
// 500) without parsing the underlying cause.
func callbackFlow(
	ctx context.Context,
	fs *flowStore,
	values url.Values,
	exchangeCode func(ctx context.Context, code, codeVerifier, nonce string) (string, time.Time, errors.E),
) (string, time.Time, string, errors.E) {
	if fs == nil {
		return "", time.Time{}, "", errors.New("authenticator has no flow store")
	}

	// If the issuer signals an error, surface it as a 400 rather than
	// pretending the flow succeeded. The "error" and "error_description"
	// parameters are OIDC-standard.
	if issuerErr := values.Get("error"); issuerErr != "" {
		errE := errors.WithStack(ErrSignInFailed)
		errors.Details(errE)["error"] = issuerErr
		if desc := values.Get("error_description"); desc != "" {
			errors.Details(errE)["description"] = desc
		}
		return "", time.Time{}, "", errE
	}

	state := values.Get("state")
	code := values.Get("code")
	if state == "" || code == "" {
		errE := errors.WithStack(ErrSignInFailed)
		errors.Details(errE)["reason"] = `missing "state" or "code" in callback`
		return "", time.Time{}, "", errE
	}

	flow, errE := fs.ConsumeFlow(ctx, state)
	if errE != nil {
		if errors.Is(errE, errFlowNotFound) {
			// Single-use, expired, or never existed: surface as
			// client error so the handler does not 500.
			return "", time.Time{}, "", errors.WrapWith(errE, ErrSignInFailed)
		}
		// DB or other internal failure: pass through unwrapped so
		// the handler maps it to 500.
		return "", time.Time{}, "", errE
	}

	token, expiry, errE := exchangeCode(ctx, code, flow.codeVerifier, flow.nonce)
	if errE != nil {
		// Token exchange / JWT validation failures are caller-induced
		// (bad code, signature mismatch, nonce mismatch, ...).
		return "", time.Time{}, "", errors.WrapWith(errE, ErrSignInFailed)
	}

	return token, expiry, flow.redirect, nil
}

// signOutFlow is the shared body of OIDCAuthenticator.SignOut and
// MockAuthenticator.SignOut. It extracts the access token from the request,
// writes the revocation row + cache entry, and (if provided) delegates
// to the authenticator-specific upstreamRevoke which is called in a goroutine.
//
// A request with no token attached or a request that fails JWT
// validation (already expired/tampered) is a no-op. A failed upstream
// revocation does not fail the sign-out: the local revocation has
// already succeeded and the user is signed out for us regardless of
// whether the issuer cooperates.
func signOutFlow(
	w http.ResponseWriter,
	req *http.Request,
	tokenVerifier *oidc.IDTokenVerifier,
	rs *revocationStore,
	upstreamRevoke func(ctx context.Context, token string) errors.E,
) errors.E {
	if rs == nil {
		return errors.New("authenticator has no revocation store")
	}

	ctx := req.Context()
	token, _ := resolveAccessToken(w, req)
	if token == "" {
		return nil
	}

	// The token is the access token from the cookie / Bearer header.
	claims, err := tokenVerifier.Verify(ctx, token)
	if err != nil {
		// Token does not validate (expired or tampered): the JWT
		// validator will reject it on every subsequent request without
		// us needing to remember anything.
		return nil //nolint:nilerr
	}

	errE := rs.Revoke(ctx, token, claims.Expiry)
	if errE != nil {
		return errE
	}

	if upstreamRevoke != nil {
		// Run upstreamRevoke in a goroutine so it does not block the
		// HTTP response.
		go func() {
			backgroundCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), upstreamRevokeTimeout)
			defer cancel()
			errE := upstreamRevoke(backgroundCtx, token)
			if errE != nil {
				zerolog.Ctx(backgroundCtx).Warn().Err(errE).Msg("upstream revocation failed")
			}
		}()
	}

	return nil
}

// safeRedirectPath validates the caller-supplied post-sign-in landing path.
// Only relative same-site paths are accepted: anything starting with a
// scheme, a "//" authority, or empty falls back to "/" so a hostile sign-in
// URL cannot bounce the user off-site after the callback.
func safeRedirectPath(raw string) string {
	if raw == "" {
		return "/"
	}
	if !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") {
		return "/"
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "/"
	}
	if u.Scheme != "" || u.Host != "" {
		return "/"
	}
	// Re-stringify so any URL-decoded curiosities (escaped slashes, etc.)
	// land back in canonical form.
	return u.String()
}

// resolveAccessToken extracts the caller's access token from the request,
// preferring an Authorization: Bearer header and falling back to the
// session cookie.
//
// Tokens are returned as-is without further validation. Callers feed them
// into the authenticator. The second return value reports whether the
// token came from the cookie - useful for telemetry but not for auth
// decisions.
func resolveAccessToken(w http.ResponseWriter, req *http.Request) (string, bool) {
	const prefix = "Bearer "

	addVary(w, "Authorization")
	auth := req.Header.Get("Authorization")
	if len(auth) >= len(prefix) && strings.EqualFold(auth[:len(prefix)], prefix) {
		return auth[len(prefix):], false
	}

	addVary(w, "Cookie")
	cookie, err := req.Cookie(accessTokenCookieName)
	if err == nil && cookie.Value != "" {
		return cookie.Value, true
	}
	return "", false
}

// addVary records that the response depends on the named request header,
// without duplicating an entry that is already present.
func addVary(w http.ResponseWriter, header string) {
	h := w.Header()
	if !slices.Contains(h.Values("Vary"), header) {
		h.Add("Vary", header)
	}
}

// extractRoles parses the scope claim of the verified token and returns every
// role granted via the "role.<key>" namespace that is also present as a key
// in allowedRoles.
//
// We support both the standard OAuth 2.0 "scope" string claim (space-separated)
// and the RFC 8693 "scp" array claim. If neither is present we return an empty
// (non-nil) slice rather than an error so authenticated tokens without any
// roles still authorize.
//
// allowedRoles acts as a allowlist: only roles whose name is a key in the map
// pass through. Values are ignored. A nil or empty map drops every role. This
// guarantees that auth.Roles never carries a role the site has not declared.
func extractRoles(idToken *oidc.IDToken, allowedRoles map[string][]string) ([]string, errors.E) {
	var claims struct {
		Scope string   `json:"scope"`
		SCP   []string `json:"scp"`
	}
	err := idToken.Claims(&claims)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var scopes []string
	if claims.Scope != "" {
		scopes = strings.Fields(claims.Scope)
	}
	scopes = append(scopes, claims.SCP...)

	roles := make([]string, 0, len(scopes))
	seen := map[string]bool{}
	for _, scope := range scopes {
		if scope == roleScopeWildcard {
			continue
		}
		if !strings.HasPrefix(scope, roleScopePrefix) {
			continue
		}
		role := strings.TrimPrefix(scope, roleScopePrefix)
		if role == "" {
			continue
		}
		if _, ok := allowedRoles[role]; !ok {
			continue
		}
		if seen[role] {
			continue
		}
		seen[role] = true
		roles = append(roles, role)
	}
	return roles, nil
}
