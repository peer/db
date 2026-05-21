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
// by the sign-in routes: it builds authorize URLs, exchanges authorization
// codes for tokens, and caches userinfo lookups. Identity gathered from a
// validated token (subject, roles, profile) is exposed to downstream
// responses as SFV-encoded HTTP headers ("Roles" and "UserInfo", prefixed by
// the WAF service's MetadataHeaderPrefix).
package auth

import (
	"bytes"
	"context"
	"net/http"
	"slices"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/hashicorp/go-cleanhttp"
	"gitlab.com/tozd/go/errors"
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

// Verifier validates OIDC bearer tokens against a configured issuer and audience.
// It also bundles the supporting state needed to drive the authorization-code
// flow and to enrich responses with identity headers (userinfo cache). One
// Verifier is built per site because each site has its own client ID and redirect URI.
type Verifier struct {
	issuer        string
	clientID      string
	verifier      *oidc.IDTokenVerifier
	httpClient    *http.Client
	oauth         *oauth2.Config
	redirectURI   func() string
	userInfoCache *userInfoCache
}

// New creates a Verifier that uses OIDC discovery to fetch keys from issuer.
// clientID is the expected audience of presented access tokens. clientSecret
// authenticates the backend during the authorization-code exchange (the
// backend is a confidential client). redirectURI is a thunk that resolves
// to the absolute callback URL the issuer should send the user back to.
//
// The returned Verifier holds a pooled HTTP client used for JWKS refreshes,
// userinfo lookups, and token exchanges; it does not own a shutdown hook
// because the underlying client uses idle connection pooling that releases
// resources passively.
func New(ctx context.Context, issuer, clientID, clientSecret string, redirectURI func() string) (*Verifier, errors.E) {
	if issuer == "" {
		return nil, errors.New("issuer is required")
	}
	if clientID == "" {
		return nil, errors.New("client ID is required")
	}
	if clientSecret == "" {
		return nil, errors.New("client secret is required")
	}
	if redirectURI == nil {
		return nil, errors.New("redirect URI thunk is required")
	}

	// We use a pooled client so that JWKS, userinfo, and token-exchange
	// refreshes can reuse connections.
	// TODO: Set User-Agent header.
	client := cleanhttp.DefaultPooledClient()
	ctx = oidc.ClientContext(ctx, client)

	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		errE := errors.WithStack(err)
		errors.Details(errE)["issuer"] = issuer
		return nil, errE
	}

	// Discovery exposes userinfo_endpoint as a JSON claim. We pull it once
	// at startup so the userinfo cache and the per-request middleware have
	// a stable URL even if Provider.Claims is later reshaped.
	var discovered struct {
		UserInfoEndpoint string `json:"userinfo_endpoint"` //nolint:tagliatelle
	}
	err = provider.Claims(&discovered)
	if err != nil {
		errE := errors.WithStack(err)
		errors.Details(errE)["issuer"] = issuer
		return nil, errE
	}

	return &Verifier{
		issuer:   issuer,
		clientID: clientID,
		verifier: provider.Verifier(&oidc.Config{ //nolint:exhaustruct
			ClientID: clientID,
		}),
		httpClient: client,
		oauth: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			// RedirectURL is filled in per call via redirectURI().
			RedirectURL: "",
			Endpoint:    provider.Endpoint(),
			Scopes:      signInScopes,
		},
		redirectURI:   redirectURI,
		userInfoCache: newUserInfoCache(discovered.UserInfoEndpoint, client),
	}, nil
}

// Issuer returns the issuer URL the Verifier was configured with.
func (v *Verifier) Issuer() string {
	return v.issuer
}

// ClientID returns the client ID the Verifier was configured with.
func (v *Verifier) ClientID() string {
	return v.clientID
}

// Authenticate validates the caller's access token (Authorization Bearer
// first, falling back to the access-token cookie) and, on success, returns
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
// The userinfo for the UserInfo header is read from an in-memory cache;
// concurrent requests for the same subject coalesce into a single upstream
// call to the issuer's userinfo endpoint (singleflight).
//
// On any validation failure the original ctx is returned unchanged and no
// headers are written; callers (eg. the PeerDB-level middleware) should
// treat that as an anonymous request and continue handling.
func (v *Verifier) Authenticate(w http.ResponseWriter, req *http.Request, metadataHeaderPrefix string) context.Context {
	ctx := req.Context()
	token, _ := resolveAccessToken(w, req)
	if token == "" {
		return ctx
	}
	idToken, err := v.verifier.Verify(ctx, token)
	if err != nil {
		return ctx
	}
	roles, errE := extractRoles(idToken)
	if errE != nil {
		return ctx
	}
	ctx = withSubject(ctx, idToken.Subject)
	ctx = withRoles(ctx, roles)
	v.writeRolesHeader(w, metadataHeaderPrefix, roles)
	v.writeUserInfoHeader(ctx, w, metadataHeaderPrefix, idToken.Subject, token)
	// Authenticated responses carry per-user data; keep them out of
	// shared caches. Browser caches still store them (keyed by
	// Authorization / Cookie via the Vary headers resolveAccessToken
	// sets).
	w.Header().Set("Cache-Control", "private")
	return ctx
}

// writeRolesHeader emits the Roles response header as an SFV list of
// strings (one entry per role). Empty role sets do not emit a header.
// The frontend should use the presence of the UserInfo header (always
// set when authenticated) to tell "anonymous" from "signed in" and not
// Roles header.
func (v *Verifier) writeRolesHeader(w http.ResponseWriter, prefix string, roles []string) {
	if len(roles) == 0 {
		return
	}
	list := make([]any, len(roles))
	for i, r := range roles {
		list[i] = r
	}
	b := &bytes.Buffer{}
	errE := waf.EncodeMetadataList(list, b)
	if errE != nil {
		return
	}
	w.Header().Add(prefix+rolesHeader, b.String())
}

// writeUserInfoHeader emits the UserInfo response header, falling back to
// a subject-only payload when the upstream userinfo lookup fails or has not
// yet populated the cache. Subject is guaranteed to be present so the
// frontend can always learn the identity of the signed-in user, even when
// the issuer is unreachable.
func (v *Verifier) writeUserInfoHeader(ctx context.Context, w http.ResponseWriter, prefix, subject, token string) {
	info, _ := v.userInfoCache.Get(ctx, subject, token)
	if info.Subject == "" {
		info.Subject = subject
	}

	metadata := map[string]any{"subject": info.Subject}
	if info.Username != "" {
		metadata["username"] = info.Username
	}

	b := &bytes.Buffer{}
	errE := waf.EncodeMetadata(metadata, b)
	if errE != nil {
		return
	}
	w.Header().Add(prefix+userInfoHeader, b.String())
}

// resolveAccessToken extracts the caller's access token from the request,
// preferring an Authorization: Bearer header and falling back to the
// session cookie.
//
// Tokens are returned as-is without further validation; callers feed them
// into the verifier. The second return value reports whether the token came
// from the cookie - useful for telemetry but not for auth decisions.
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
// role granted via the "role.<key>" namespace.
//
// We support both the standard OAuth 2.0 "scope" string claim (space-separated)
// and the RFC 8693 "scp" array claim. If neither is present we return an empty
// (non-nil) slice rather than an error so authenticated tokens without any
// roles still authorize.
func extractRoles(idToken *oidc.IDToken) ([]string, errors.E) {
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
	return roles, nil
}
