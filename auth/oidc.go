package auth

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/hashicorp/go-cleanhttp"
	"github.com/jackc/pgx/v5/pgxpool"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"golang.org/x/oauth2"
)

// TODO: Support "OpenID Connect Back-Channel Logout" so that issuers (OPs) can inform us that the token is revoked.
//       We can set it as revoked in our revocationStore until it expires.
//       Or we can also implement "Security Event Tokens" instead for same purpose.

// signInScopes are the scopes the backend requests during the OIDC
// authorization flow. "role.*" is Charon's wildcard that expands into the
// individual "role.<key>" grants the access token actually carries.
var signInScopes = []string{ //nolint:gochecknoglobals
	oidc.ScopeOpenID,
	"profile",
	"email",
	"role.*",
}

// OIDCAuthenticator authenticates the user against an ODIC-compliant issue
// and validates its tokens.
type OIDCAuthenticator struct {
	baseAuthenticator

	clientID           string
	httpClient         *http.Client
	oauth              *oauth2.Config
	redirectURI        func() string
	revocationEndpoint string
}

// NewOIDCAuthenticator creates an Authenticator that uses OIDC discovery to
// fetch keys from issuer.
//
// organization is the issuer-side organization the site's users belong to, used to look up users
// other than the caller (see Identity). It is optional: without it such lookups fail, and the site
// then knows nothing about a user beyond what their own requests carry.
//
// clientID is the expected audience of presented access tokens.
// clientSecret authenticates the backend during the authorization-code exchange
// (the backend is a confidential client). redirectURI is a thunk that resolves
// to the absolute callback URL the issuer should send the user back to.
//
// dbpool is used to construct and initialise the flow and revocation stores.
//
// The returned OIDCAuthenticator holds a pooled HTTP client used for JWKS
// refreshes, userinfo lookups, and token exchanges. It does not own a
// shutdown hook because the underlying client uses idle connection pooling
// that releases resources passively.
func NewOIDCAuthenticator(
	ctx context.Context, dbpool *pgxpool.Pool, issuer, organization, clientID, clientSecretPath string, redirectURI func() string,
) (*OIDCAuthenticator, errors.E) {
	if issuer == "" {
		return nil, errors.New("issuer is required")
	}
	if clientID == "" {
		return nil, errors.New("client ID is required")
	}
	if clientSecretPath == "" {
		return nil, errors.New("client secret is required")
	}
	if redirectURI == nil {
		return nil, errors.New("redirect URI thunk is required")
	}

	clientSecret, err := os.ReadFile(clientSecretPath) //nolint:gosec
	if err != nil {
		errE := errors.WithStack(err)
		errors.Details(errE)["path"] = clientSecretPath
		return nil, errE
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

	// Discovery exposes userinfo_endpoint and revocation_endpoint as JSON claims on
	// the discovery document. The revocation endpoint is optional. Issuers that do
	// not advertise one leave it empty and SignOut's upstream call becomes a no-op.
	var discovered struct {
		UserInfoEndpoint   string `json:"userinfo_endpoint"`   //nolint:tagliatelle
		RevocationEndpoint string `json:"revocation_endpoint"` //nolint:tagliatelle
	}
	err = provider.Claims(&discovered)
	if err != nil {
		errE := errors.WithStack(err)
		errors.Details(errE)["issuer"] = issuer
		return nil, errE
	}

	if dbpool == nil {
		return nil, errors.New("dbpool is required")
	}
	fs := newFlowStore(dbpool)
	errE := fs.Init(ctx)
	if errE != nil {
		return nil, errE
	}
	rs := newRevocationStore(dbpool)
	errE = rs.Init(ctx)
	if errE != nil {
		return nil, errE
	}

	return &OIDCAuthenticator{
		baseAuthenticator: baseAuthenticator{
			TokenVerifier: provider.Verifier(&oidc.Config{ //nolint:exhaustruct
				ClientID: clientID,
			}),
			UserInfoCache:   newUserInfoCache(fetchUserInfo(discovered.UserInfoEndpoint, client), fetchIdentity(issuer, organization, client)),
			FlowStore:       fs,
			RevocationStore: rs,
		},
		clientID:           clientID,
		httpClient:         client,
		revocationEndpoint: discovered.RevocationEndpoint,
		oauth: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: strings.TrimSpace(string(clientSecret)),
			// RedirectURL is filled in per call via redirectURI().
			RedirectURL: "",
			Endpoint:    provider.Endpoint(),
			Scopes:      signInScopes,
		},
		redirectURI: redirectURI,
	}, nil
}

// SignIn begins an authorization-code flow against the configured issuer.
func (a *OIDCAuthenticator) SignIn(ctx context.Context, redirect, uiLocales string) (string, errors.E) {
	return signInFlow(ctx, a.FlowStore, redirect, uiLocales, a.authCodeURL)
}

// Callback finishes an authorization-code flow.
func (a *OIDCAuthenticator) Callback(ctx context.Context, values url.Values, allowedRoles map[string]RoleGrants) (string, time.Time, string, errors.E) {
	return callbackFlow(ctx, a.FlowStore, values, allowedRoles, a.exchangeCode)
}

// TODO: Consider invoking also the issuer-side session using RP-Initiated Logout (end_session_endpoint).
//       It requires a browser-side redirect so maybe we should use something non-standard like Keycloak and Auth0 use to kill
//       the session server-to-server using its sid. Then Charon can set its session's cookie as revoked in its revocation store.

// SignOut revokes the request's access token. The local revocation store
// records the token (so any future request presenting the same cookie or
// bearer credential is rejected) and, when the issuer advertises a
// revocation_endpoint, it also informs the issuer to revoke the token.
//
// Upstream revocation is best-effort: if the call fails (network error,
// endpoint not configured, 4xx response) the local revocation has
// already succeeded and SignOut returns nil. The user is signed out for
// us regardless of whether the issuer cooperates.
func (a *OIDCAuthenticator) SignOut(w http.ResponseWriter, req *http.Request) errors.E {
	return signOutFlow(w, req, a.TokenVerifier, a.RevocationStore, a.revokeUpstream)
}

// charonIdentityPath builds the path of the issuer's endpoint describing one user of the
// organization, given the organization and the user's (organization-scoped) subject.
//
// TODO: Support issuers other than Charon.
//
//	Looking up a user other than the caller is not part of OpenID Connect: its userinfo endpoint
//	describes the caller alone. Charon exposes the organization's users through this endpoint,
//	readable by any user of the same organization, but another issuer needs its own way (SCIM,
//	say), so this should become a configured URL template with per-issuer response mapping.
func charonIdentityPath(organization, subject string) string {
	return "/api/o/identity/" + url.PathEscape(organization) + "/" + url.PathEscape(subject)
}

// fetchUserInfo builds the lookup of the user a token belongs to which the userinfo cache misses on:
// a call to the issuer's userinfo endpoint with the token as the credential. An issuer which does not
// advertise the endpoint leaves the site with nothing to ask, which every lookup then says.
//
// The subject looked up is not part of the request, because the endpoint describes the holder of the
// token and nobody else; it is the response which says who that is.
//
// The HTTP client should be pooled so JWKS refreshes and userinfo lookups share connections.
func fetchUserInfo(endpoint string, client *http.Client) func(ctx context.Context, subject, token string) (userInfo, errors.E) {
	return func(ctx context.Context, _, token string) (userInfo, errors.E) {
		if endpoint == "" {
			return userInfo{}, errors.New("userinfo endpoint not configured")
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return userInfo{}, errors.WithStack(err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return userInfo{}, errors.WithStack(err)
		}
		defer resp.Body.Close()              //nolint:errcheck
		defer io.Copy(io.Discard, resp.Body) //nolint:errcheck

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			errE := errors.New("userinfo request failed")
			errors.Details(errE)["status"] = resp.StatusCode
			errors.Details(errE)["body"] = strings.TrimSpace(string(body))
			return userInfo{}, errE
		}

		var payload struct {
			Sub               string `json:"sub"`
			PreferredUsername string `json:"preferred_username"` //nolint:tagliatelle
			Name              string `json:"name"`
			GivenName         string `json:"given_name"` //nolint:tagliatelle
		}
		// We accept extra fields silently.
		errE := x.DecodeJSON(resp.Body, &payload)
		if errE != nil {
			return userInfo{}, errE
		}
		return userInfo{Subject: payload.Sub, Username: pickUsername(payload.PreferredUsername, payload.GivenName, payload.Name), Roles: nil}, nil
	}
}

// pickUsername is the name a user is known by: the username the issuer gives them, or a name it gives
// instead, the given name before the full name, because an issuer which knows no username for a user
// (or does not tell it) still names them better than their subject does.
func pickUsername(username, givenName, fullName string) string {
	if username != "" {
		return username
	}
	if givenName != "" {
		return givenName
	}
	return fullName
}

// fetchIdentity builds the lookup of a user other than the caller which the userinfo cache misses on:
// a call to the issuer's endpoint for one user of the organization, with the caller's access token as
// the credential, so the issuer decides what a caller may learn. It is readable by any user of the
// organization and describes a user the same way to each of them, which is what caching the answer
// assumes (see userInfoCache).
//
// A subject the issuer does not describe to this caller is reported as not found, so that a caller
// cannot tell "no such user" from "not for you". Without a configured organization there is no such
// endpoint to call, which every lookup then says.
func fetchIdentity(issuer, organization string, client *http.Client) func(ctx context.Context, subject, token string) (userInfo, errors.E) {
	return func(ctx context.Context, subject, token string) (userInfo, errors.E) {
		if organization == "" {
			errE := errors.New("organization is not configured")
			errors.Details(errE)["subject"] = subject
			return userInfo{}, errE
		}

		endpoint, err := url.JoinPath(issuer, charonIdentityPath(organization, subject))
		if err != nil {
			return userInfo{}, errors.WithStack(err)
		}

		// The URL is the site's configured issuer with the organization and the subject as escaped path
		// segments, so it addresses the issuer and nothing else.
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil) //nolint:gosec
		if err != nil {
			return userInfo{}, errors.WithStack(err)
		}
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("Accept", "application/json")

		response, err := client.Do(request) //nolint:gosec
		if err != nil {
			return userInfo{}, errors.WithStack(err)
		}
		defer response.Body.Close()              //nolint:errcheck
		defer io.Copy(io.Discard, response.Body) //nolint:errcheck

		if response.StatusCode == http.StatusNotFound || response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
			errE := errors.WithStack(ErrIdentityNotFound)
			errors.Details(errE)["subject"] = subject
			errors.Details(errE)["status"] = response.StatusCode
			return userInfo{}, errE
		}
		if response.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(response.Body)
			errE := errors.New("identity request failed")
			errors.Details(errE)["subject"] = subject
			errors.Details(errE)["status"] = response.StatusCode
			errors.Details(errE)["body"] = strings.TrimSpace(string(body))
			return userInfo{}, errE
		}

		var payload struct {
			ID        string   `json:"id"`
			Username  string   `json:"username"`
			FullName  string   `json:"fullName"`
			GivenName string   `json:"givenName"`
			Roles     []string `json:"roles"`
		}
		// We accept extra fields silently.
		errE := x.DecodeJSON(response.Body, &payload)
		if errE != nil {
			return userInfo{}, errE
		}
		return userInfo{Subject: payload.ID, Username: pickUsername(payload.Username, payload.GivenName, payload.FullName), Roles: payload.Roles}, nil
	}
}

// revokeUpstream POSTs to the issuer's revocation_endpoint. Returns the
// request error. The caller treats it as best-effort and only logs it.
//
// revokeUpstream is safe to run in a separate goroutine.
func (a *OIDCAuthenticator) revokeUpstream(ctx context.Context, token string) errors.E {
	if a.revocationEndpoint == "" {
		return nil
	}
	form := url.Values{
		"token":           {token},
		"token_type_hint": {"access_token"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.revocationEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return errors.WithStack(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.SetBasicAuth(url.QueryEscape(a.clientID), url.QueryEscape(a.oauth.ClientSecret))

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return errors.WithStack(err)
	}
	defer resp.Body.Close()              //nolint:errcheck
	defer io.Copy(io.Discard, resp.Body) //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		errE := errors.New("revocation endpoint returned non-200")
		errors.Details(errE)["status"] = resp.StatusCode
		return errE
	}
	return nil
}

// oauthConfig returns a copy of the stored oauth2.Config with the redirect
// URL resolved via the thunk. We copy rather than mutate because the
// underlying config is shared across goroutines.
func (a *OIDCAuthenticator) oauthConfig() oauth2.Config {
	c := *a.oauth
	c.RedirectURL = a.redirectURI()
	return c
}

// authCodeURL builds the issuer-bound URL the browser should be redirected
// to in order to start an authorization-code flow. state, codeVerifier, and
// nonce must be generated by the caller. The PKCE verifier should come from
// oauth2.GenerateVerifier so the S256 challenge derivation matches what the
// issuer expects.
func (a *OIDCAuthenticator) authCodeURL(state, codeVerifier, nonce, uiLocales string) string {
	cfg := a.oauthConfig()
	opts := []oauth2.AuthCodeOption{
		oauth2.AccessTypeOnline,
		oauth2.S256ChallengeOption(codeVerifier),
		oidc.Nonce(nonce),
	}
	// Forward the user's preferred UI language to the issuer so it can render its sign-in UI to match.
	if uiLocales != "" {
		opts = append(opts, oauth2.SetAuthURLParam("ui_locales", uiLocales))
	}
	return cfg.AuthCodeURL(state, opts...)
}

// exchangeCode finishes the authorization-code flow.
func (a *OIDCAuthenticator) exchangeCode(
	ctx context.Context, code, codeVerifier, expectedNonce string, allowedRoles map[string]RoleGrants,
) (string, time.Time, errors.E) {
	// The pooled HTTP client is shared with JWKS/userinfo so token
	// exchanges benefit from the same keep-alive pool.
	tokenCtx := oidc.ClientContext(ctx, a.httpClient)
	cfg := a.oauthConfig()
	response, err := cfg.Exchange(tokenCtx, code, oauth2.VerifierOption(codeVerifier))
	if err != nil {
		return "", time.Time{}, errors.WithStack(err)
	}

	rawIDToken, ok := response.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		return "", time.Time{}, errors.New("issuer did not return an id_token")
	}

	idToken, err := a.TokenVerifier.Verify(ctx, rawIDToken)
	if err != nil {
		return "", time.Time{}, errors.WithStack(err)
	}
	if idToken.Nonce != expectedNonce {
		return "", time.Time{}, errors.New("id_token nonce mismatch")
	}
	// If the issuer included an at_hash claim, verify it matches the
	// access token. The claim is optional in the authorization-code flow,
	// so we only verify when present.
	if idToken.AccessTokenHash != "" {
		err = idToken.VerifyAccessToken(response.AccessToken)
		if err != nil {
			return "", time.Time{}, errors.WithStack(err)
		}
	}

	// Prime the userinfo cache from the ID-token claims so the first
	// authenticated request after sign-in does not pay an extra
	// /auth/oidc/userinfo round-trip.
	var profile struct {
		PreferredUsername string `json:"preferred_username"` //nolint:tagliatelle
		Name              string `json:"name"`
		GivenName         string `json:"given_name"` //nolint:tagliatelle
	}
	err = idToken.Claims(&profile)
	if err != nil {
		return "", time.Time{}, errors.WithStack(err)
	}
	// The roles come from the access token just issued, which is what grants them, so the entry
	// describes the user fully and a lookup of them (see Identity) needs no fetch either. Priming is
	// best-effort: a token we cannot read here (a JWKS refresh may have just failed) does not
	// authenticate anything either, so we prime nothing instead of recording a user without roles.
	accessClaims, err := a.TokenVerifier.Verify(ctx, response.AccessToken)
	if err == nil {
		roles, errE := extractRoles(accessClaims, allowedRoles)
		if errE == nil {
			a.UserInfoCache.set(idToken.Subject, userInfo{
				Subject:  idToken.Subject,
				Username: pickUsername(profile.PreferredUsername, profile.GivenName, profile.Name),
				Roles:    roles,
			})
		}
	}

	return response.AccessToken, response.Expiry, nil
}
