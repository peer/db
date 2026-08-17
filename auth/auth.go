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
// expired flow row, or a token-exchange/JWT-validation failure. Route
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
	// returns the request context enriched with subject, roles and the
	// resolved visibility level AND writes the Roles/UserInfo response
	// headers consumed by the frontend. On failure the original ctx is
	// returned unchanged and no headers are written.
	Authenticate(
		w http.ResponseWriter, req *http.Request, metadataHeaderPrefix string, allowedRoles map[string]RoleGrants, visibility []VisibilityLevel,
	) context.Context

	// SignIn begins a fresh sign-in flow. uiLocales is the user's preferred UI language (forwarded to the
	// issuer as the OIDC ui_locales request parameter, empty when unknown) so the issuer can render its UI to match.
	SignIn(ctx context.Context, redirect, uiLocales string) (authURL string, errE errors.E)

	// Callback finishes a sign-in flow. allowedRoles is the allowlist of role names the site recognizes.
	//
	// Every client-side failure is wrapped with ErrSignInFailed. Internal errors
	// are returned without that wrapping.
	Callback(ctx context.Context, values url.Values, allowedRoles map[string]RoleGrants) (token string, expiry time.Time, redirect string, errE errors.E)

	// SignOut revokes the access token the caller presented.
	SignOut(w http.ResponseWriter, req *http.Request) errors.E

	// Identity returns what is known about the user with the given subject: their profile fields and
	// the roles they hold, filtered by allowedRoles. The caller has to be authenticated, and their access
	// token is what authorizes the lookup upstream, but the answer is cached per subject and not per
	// caller, so what one user of the site learned about somebody answers every other user as well. It
	// assumes that the issuer describes a user the same way to all of them.
	//
	// It returns an error wrapping ErrIdentityNotFound when the issuer does not know the subject, and
	// one wrapping ErrAccessDenied when the request is not authenticated.
	Identity(w http.ResponseWriter, req *http.Request, subject string, allowedRoles map[string]RoleGrants) (*Identity, errors.E)

	// CleanupExpired prunes rows that have aged out of the
	// Authenticator's internal flow and revocation stores. It is meant
	// to be called from a periodic background job. Errors from either
	// store are joined so a partial failure is still surfaced.
	CleanupExpired(ctx context.Context) errors.E
}

// Identity is what a site knows about one of its users: who they are and which of the site's roles
// they hold. It is what the user API returns.
//
// It describes a user, it does not authorize them: what a caller may do follows from the roles of
// their own verified access token (see Roles), and never from what this says about anybody.
type Identity struct {
	Subject  string   `json:"subject"`
	Username string   `json:"username,omitempty"`
	Roles    []string `json:"roles"`
}

// ErrIdentityNotFound is returned when the issuer does not know the requested subject, or does not
// tell the caller about them, which is the same thing as far as the caller is concerned.
var ErrIdentityNotFound = errors.Base("identity not found")

// baseAuthenticator holds the state shared between OIDCAuthenticator and
// MockAuthenticator: the token verifier (which key set differs but the
// validation contract is the same), the userinfo cache (each of them gives it
// its own way of looking a user up), the per-site flow store, and the per-site
// revocation store that remembers which tokens were explicitly signed out.
type baseAuthenticator struct {
	TokenVerifier   *oidc.IDTokenVerifier
	UserInfoCache   *userInfoCache
	FlowStore       *flowStore
	RevocationStore *revocationStore
}

// Authenticate validates the caller's access token (Authorization Bearer
// first, falling back to the session cookie). On success it returns the
// request context enriched with subject, roles and the resolved visibility
// level. It always writes two response headers consumed by the frontend,
// empty when the request is unauthenticated:
//
//   - "<prefix>Roles": the role list as an SFV inner-list (empty when the
//     caller has no roles).
//   - "<prefix>UserInfo": an SFV dictionary with subject (empty for an
//     unauthenticated request) and username (when known).
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
// visibility is the ordered list of visibility levels. The caller's resolved
// roles are mapped to the highest applicable level, which is attached to the
// context. A level with no roles is a floor applied to every request, so an
// unauthenticated request still gets that level when one is configured.
//
// The userinfo for the UserInfo header is read from an in-memory cache.
// Concurrent requests for the same subject coalesce into a single upstream
// call to the issuer's userinfo endpoint (singleflight).
//
// On any validation failure the request is treated as unauthenticated: no
// subject or roles are attached to the context (only the no-roles floor
// visibility level, if configured), but the Roles and UserInfo headers are
// still written empty (an empty role list and an empty subject). They are
// emitted unconditionally, signed in or not, because a cacheable response
// carries the auth state in these headers while its body is identical for
// everyone. On a 304 revalidation the browser merges the response headers
// over its stored copy and keeps any header the 304 omits, so always
// emitting them lets a cached signed-in response self-correct to signed-out
// instead of retaining a stale identity. Callers should continue handling.
func (b *baseAuthenticator) Authenticate(
	w http.ResponseWriter, req *http.Request, metadataHeaderPrefix string, allowedRoles map[string]RoleGrants, visibility []VisibilityLevel,
) context.Context {
	ctx := req.Context()
	subject, roles, token, ok := b.authenticatedIdentity(ctx, w, req, allowedRoles)
	// Resolve the caller's visibility level. roles is nil for an unauthenticated
	// request, in which case the no-roles floor level (if any) applies.
	// An authenticated caller's roles may raise it above that floor.
	if level, found := visibilityForRoles(visibility, roles); found {
		ctx = WithVisibility(ctx, level.Name)
		// WithVisibility tags the context logger, so we here also tag the canonical logger.
		waf.SetCanonicalLogField(ctx, "visibility", level.Name)
	}
	if !ok {
		// Emit empty Roles and UserInfo headers for an unauthenticated request so
		// a cached signed-in response self-corrects on revalidation.
		b.writeRolesHeader(w, metadataHeaderPrefix, nil)
		b.writeUserInfoHeader(ctx, w, metadataHeaderPrefix, "", "", nil)
		return ctx
	}
	ctx = WithSubject(ctx, subject)
	// WithSubject tags the context logger, so we here also tag the canonical logger.
	waf.SetCanonicalLogField(ctx, "subject", subject)
	ctx = WithRoles(ctx, roles)
	// WithRoles tags the context logger, so we here also tag the canonical logger.
	waf.SetCanonicalLogField(ctx, "roles", roles)
	b.writeRolesHeader(w, metadataHeaderPrefix, roles)
	b.writeUserInfoHeader(ctx, w, metadataHeaderPrefix, subject, token, roles)
	// Authenticated responses carry per-user data, keep them out of
	// shared caches. Browser caches still store them (keyed by
	// Authorization/Cookie via the Vary headers resolveAccessToken
	// sets).
	w.Header().Set("Cache-Control", "private")
	return ctx
}

// authenticatedIdentity validates the caller's access token and, on success,
// returns the verified subject, the granted roles (filtered by allowedRoles)
// and the raw token. ok is false for an unauthenticated request: no token, an
// invalid or revoked token, or a role-extraction failure. The caller then
// treats the request as anonymous.
func (b *baseAuthenticator) authenticatedIdentity(
	ctx context.Context, w http.ResponseWriter, req *http.Request, allowedRoles map[string]RoleGrants,
) (string, []string, string, bool) {
	token, _ := resolveAccessToken(w, req)
	if token == "" {
		return "", nil, "", false
	}
	// The token is an access token (what the cookie/Bearer header
	// carries), not an ID token. go-oidc only exposes IDTokenVerifier
	// for JWT validation, so the returned *oidc.IDToken is just a
	// parsed-JWT struct here. The validation contract (signature,
	// issuer, audience, expiry) is the same either way.
	claims, err := b.TokenVerifier.Verify(ctx, token)
	if err != nil {
		return "", nil, "", false
	}
	// Revocation check: a token that passed the JWT signature/exp gate
	// may still have been explicitly signed out.
	revoked, errE := b.RevocationStore.IsRevoked(ctx, token, claims.Expiry)
	if errE == nil && revoked {
		return "", nil, "", false
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
		return "", nil, "", false
	}
	return claims.Subject, roles, token, true
}

// writeRolesHeader emits the Roles response header as an SFV list of strings
// (one entry per role). It is written for every request, an empty list when
// the caller has no roles, so a 304 revalidation merges over and replaces a
// stale Roles header rather than keeping it. The frontend tells "signed in"
// from "unauthenticated" by the UserInfo subject, not by this header.
func (b *baseAuthenticator) writeRolesHeader(w http.ResponseWriter, prefix string, roles []string) {
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

// writeUserInfoHeader emits the UserInfo response header as an SFV dictionary.
// For an authenticated caller (non-empty subject) it carries the subject plus
// the username from the userinfo cache, falling back to a subject-only payload
// when the upstream lookup fails or has not yet populated the cache, so the
// frontend can always learn the identity even when the issuer is unreachable.
// For an unauthenticated caller (empty subject) it emits subject="" with no
// upstream lookup; the header is still written so a 304 revalidation replaces
// a stale signed-in UserInfo rather than keeping it.
func (b *baseAuthenticator) writeUserInfoHeader(ctx context.Context, w http.ResponseWriter, prefix, subject, token string, roles []string) {
	metadata := map[string]any{"subject": subject}
	if subject != "" {
		info, errE := b.UserInfoCache.GetSelf(ctx, subject, token, roles)
		if errE != nil {
			// The header is written without the username rather than not at all, but the site is not
			// learning who its users are, so the failure is worth knowing about.
			zerolog.Ctx(ctx).Warn().Err(errE).Str("subject", subject).Msg("userinfo lookup failed")
		}
		if info.Subject != "" {
			metadata["subject"] = info.Subject
		}
		if info.Username != "" {
			metadata["username"] = info.Username
		}
	}

	buf := &bytes.Buffer{}
	errE := waf.EncodeMetadata(metadata, buf)
	if errE != nil {
		return
	}
	w.Header().Add(prefix+userInfoHeader, buf.String())
}

// Identity resolves what is known about subject for the caller of req (see Authenticator.Identity).
// The caller's own identity is answered from what their token says (the issuer's userinfo endpoint is
// about them, and their roles are in their token), while any other user is looked up on the caller's
// behalf, each with the lookup the authenticator gave the userinfo cache. Both are stored in that same
// cache, so a user shown repeatedly (say once per access request on a document) is fetched once.
func (b *baseAuthenticator) Identity(
	w http.ResponseWriter, req *http.Request, subject string, allowedRoles map[string]RoleGrants,
) (*Identity, errors.E) {
	ctx := req.Context()

	if subject == "" {
		return nil, errors.WithStack(ErrIdentityNotFound)
	}

	callerSubject, callerRoles, token, ok := b.authenticatedIdentity(ctx, w, req, allowedRoles)
	if !ok {
		return nil, errors.WithStack(ErrAccessDenied)
	}

	var info userInfo
	var errE errors.E
	if subject == callerSubject {
		info, errE = b.UserInfoCache.GetSelf(ctx, subject, token, callerRoles)
		if errE != nil {
			// The issuer's userinfo endpoint is not reachable, but the token itself already says who the
			// caller is, so they still learn about themselves, without their profile fields.
			zerolog.Ctx(ctx).Warn().Err(errE).Str("subject", subject).Msg("userinfo lookup failed")
			info = userInfo{Subject: callerSubject, Username: "", Roles: callerRoles}
		}
	} else {
		info, errE = b.UserInfoCache.GetOther(ctx, subject, token, allowedRoles)
		if errE != nil {
			return nil, errE
		}
	}

	return &Identity{
		Subject:  info.Subject,
		Username: info.Username,
		Roles:    info.Roles,
	}, nil
}

// filterRoles keeps only the roles the site recognizes: a role the site does not configure grants
// nothing, so it is not part of what the site knows about a user. The result is never nil, so that a
// user without any roles has an empty list of them and not a missing one.
func filterRoles(roles []string, allowedRoles map[string]RoleGrants) []string {
	filtered := make([]string, 0, len(roles))
	for _, role := range roles {
		if _, ok := allowedRoles[role]; ok {
			filtered = append(filtered, role)
		}
	}
	return filtered
}

// CleanupExpired prunes expired rows from both the flow store and the
// revocation store. Each store is asked to clean up independently so a
// transient failure in one does not leave the other unpruned. Errors
// from the two stores are joined so the caller learns about both.
func (b *baseAuthenticator) CleanupExpired(ctx context.Context) errors.E {
	errFlow := b.FlowStore.cleanupExpired(ctx)
	errRevocation := b.RevocationStore.cleanupExpired(ctx)
	return errors.Join(errFlow, errRevocation)
}

// signInFlow is the shared body of OIDCAuthenticator.SignIn and
// MockAuthenticator.SignIn. It sanitises the redirect, generates
// fresh state/PKCE verifier/nonce values, persists them, and
// delegates to the authenticator-specific authCodeURL builder
// for the final URL.
func signInFlow(
	ctx context.Context,
	fs *flowStore,
	redirect string,
	uiLocales string,
	authCodeURL func(state, codeVerifier, nonce, uiLocales string) string,
) (string, errors.E) {
	if fs == nil {
		return "", errors.New("authenticator has no flow store")
	}

	redirect = safeRedirectPath(redirect)

	state := identifier.New().String()
	codeVerifier := oauth2.GenerateVerifier()
	nonce := identifier.New().String()

	errE := fs.BeginFlow(ctx, state, flowState{
		CodeVerifier: codeVerifier,
		Nonce:        nonce,
		Redirect:     redirect,
	})
	if errE != nil {
		return "", errE
	}

	return authCodeURL(state, codeVerifier, nonce, uiLocales), nil
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
	allowedRoles map[string]RoleGrants,
	exchangeCode func(ctx context.Context, code, codeVerifier, nonce string, allowedRoles map[string]RoleGrants) (string, time.Time, errors.E),
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

	token, expiry, errE := exchangeCode(ctx, code, flow.CodeVerifier, flow.Nonce, allowedRoles)
	if errE != nil {
		// Token exchange/JWT validation failures are caller-induced
		// (bad code, signature mismatch, nonce mismatch, ...).
		return "", time.Time{}, "", errors.WrapWith(errE, ErrSignInFailed)
	}

	return token, expiry, flow.Redirect, nil
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

	// The token is the access token from the cookie/Bearer header.
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
// allowedRoles acts as an allowlist (see filterRoles): only roles whose name is
// a key in the map pass through. Values are ignored. A nil or empty map drops
// every role. This guarantees that auth.Roles never carries a role the site has
// not declared.
func extractRoles(idToken *oidc.IDToken, allowedRoles map[string]RoleGrants) ([]string, errors.E) {
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
		if seen[role] {
			continue
		}
		seen[role] = true
		roles = append(roles, role)
	}
	return filterRoles(roles, allowedRoles), nil
}
