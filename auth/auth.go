// Package auth verifies OIDC-issued JWT access tokens presented by API clients
// and attaches the resulting identity (subject and roles) to the request context.
//
// Tokens are validated against the JSON Web Key Set discovered from the
// configured OIDC issuer. The expected audience matches the configured client ID.
// Roles are extracted from the scope claim, taking every scope under the
// "role." namespace (for example, "role.admin" becomes "admin"); the wildcard
// "role.*" is ignored if encountered.
package auth

import (
	"context"
	"net/http"
	"slices"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/hashicorp/go-cleanhttp"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/waf"
)

// ErrNoToken is returned by parseToken when the request has no bearer token.
var ErrNoToken = errors.Base("no bearer token in request")

// ErrInvalidToken wraps verification failures returned by the underlying OIDC verifier.
var ErrInvalidToken = errors.Base("invalid bearer token")

// roleScopePrefix uses Charon's scope convention: every scope starting with
// this prefix grants the named role to the caller.
const roleScopePrefix = "role."

// roleScopeWildcard is the namespace wildcard that Charon expands into individual
// "role.<key>" scopes. It should never appear in granted scopes, but we filter
// it out defensively in case some OIDC providers pass it through.
const roleScopeWildcard = "role.*"

// Verifier validates OIDC bearer tokens against a configured issuer and audience.
type Verifier struct {
	issuer   string
	clientID string
	verifier *oidc.IDTokenVerifier
}

// New creates a Verifier that uses OIDC discovery to fetch keys from issuer.
// clientID is the expected audience of presented access tokens.
//
// The returned Verifier holds an HTTP client used for JWKS refreshes; it does
// not own a shutdown hook because the underlying client uses idle connection
// pooling that releases resources passively.
func New(ctx context.Context, issuer, clientID string) (*Verifier, errors.E) {
	if issuer == "" {
		return nil, errors.New("issuer is required")
	}
	if clientID == "" {
		return nil, errors.New("client ID is required")
	}

	// We use a pooled client so that JWKS and userinfo refreshes can reuse connections.
	client := cleanhttp.DefaultPooledClient()
	dctx := oidc.ClientContext(ctx, client)

	provider, err := oidc.NewProvider(dctx, issuer)
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

// authenticate verifies the bearer token attached to req, if any. On success
// it returns the request ctx enriched with the subject and roles claimed by
// the token and ok=true. On any failure (no token, malformed, expired, wrong
// audience, signature mismatch, claim parse error) it returns the original
// request ctx unchanged and ok=false, without writing to w - the caller
// decides whether the failure is a 401 or just an anonymous request.
//
// Vary: Authorization is set regardless so that responses cached upstream
// reflect that they depend on whether the caller presented a token.
func (v *Verifier) authenticate(w http.ResponseWriter, req *http.Request) (context.Context, bool) {
	ctx := req.Context()

	if !slices.Contains(w.Header().Values("Vary"), "Authorization") {
		w.Header().Add("Vary", "Authorization")
	}

	token := getBearerToken(req)
	if token == "" {
		return ctx, false
	}

	idToken, err := v.verifier.Verify(ctx, token)
	if err != nil {
		return ctx, false
	}

	roles, errE := extractRoles(idToken)
	if errE != nil {
		return ctx, false
	}

	ctx = WithSubject(ctx, idToken.Subject)
	ctx = WithRoles(ctx, roles)
	return ctx, true
}

// RequireAuthenticated verifies the request's bearer token. On success it
// returns the request context enriched with the subject and roles claimed by
// the token. On failure it writes a 401 Unauthorized response and returns nil.
func (v *Verifier) RequireAuthenticated(w http.ResponseWriter, req *http.Request) context.Context {
	ctx, ok := v.authenticate(w, req)
	if !ok {
		waf.Error(w, req, http.StatusUnauthorized)
		return nil
	}
	return ctx
}

// MaybeAuthenticated is the non-rejecting counterpart of RequireAuthenticated.
// It verifies the bearer token if one is present and returns ctx with subject
// and roles attached, but treats any failure (missing, malformed, expired,
// wrong audience) as an anonymous request and returns the original ctx
// unchanged. No response is written. Use this for endpoints whose behaviour
// merely depends on who is signed in (and may also serve anonymous users).
func (v *Verifier) MaybeAuthenticated(w http.ResponseWriter, req *http.Request) context.Context {
	ctx, _ := v.authenticate(w, req)
	return ctx
}

// Middleware returns an http.Handler middleware that runs MaybeAuthenticated
// on every request and forwards the (possibly enriched) ctx to the wrapped
// handler. Use it as a global middleware so handlers downstream can rely on
// auth.Subject / auth.Roles being populated whenever the caller presented a
// valid token, without having to call the Verifier themselves.
func (v *Verifier) Middleware() func(http.Handler) http.Handler {
	return func(handler http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			waf.SetCanonicalLogMessage(req.Context(), "OIDCAuth")
			ctx := v.MaybeAuthenticated(w, req)
			handler.ServeHTTP(w, req.WithContext(ctx)) //nolint:contextcheck
		})
	}
}

// getBearerToken extracts the bearer token from the request's Authorization
// header. Returns the empty string if the header is missing or not a Bearer credential.
func getBearerToken(req *http.Request) string {
	const prefix = "Bearer "
	auth := req.Header.Get("Authorization")
	if len(auth) < len(prefix) || !strings.EqualFold(auth[:len(prefix)], prefix) {
		return ""
	}
	return auth[len(prefix):]
}

// HasBearerToken reports whether req carries an Authorization header using the
// Bearer scheme. The token itself is not validated; this only inspects the scheme.
func HasBearerToken(req *http.Request) bool {
	return getBearerToken(req) != ""
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
