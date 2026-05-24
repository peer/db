package auth

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/hashicorp/go-cleanhttp"
	"github.com/jackc/pgx/v5/pgxpool"
	"gitlab.com/tozd/go/errors"
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

	issuer             string
	clientID           string
	httpClient         *http.Client
	oauth              *oauth2.Config
	redirectURI        func() string
	revocationEndpoint string
}

// NewOIDCAuthenticator creates an Authenticator that uses OIDC discovery to
// fetch keys from issuer.
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
func NewOIDCAuthenticator(ctx context.Context, dbpool *pgxpool.Pool, issuer, clientID, clientSecret string, redirectURI func() string) (*OIDCAuthenticator, errors.E) {
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
			tokenVerifier: provider.Verifier(&oidc.Config{ //nolint:exhaustruct
				ClientID: clientID,
			}),
			userInfoCache:   newUserInfoCache(discovered.UserInfoEndpoint, client),
			flowStore:       fs,
			revocationStore: rs,
		},
		issuer:             issuer,
		clientID:           clientID,
		httpClient:         client,
		revocationEndpoint: discovered.RevocationEndpoint,
		oauth: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			// RedirectURL is filled in per call via redirectURI().
			RedirectURL: "",
			Endpoint:    provider.Endpoint(),
			Scopes:      signInScopes,
		},
		redirectURI: redirectURI,
	}, nil
}

// SignIn begins an authorization-code flow against the configured issuer.
func (a *OIDCAuthenticator) SignIn(ctx context.Context, redirect string) (string, errors.E) {
	return signInFlow(ctx, a.flowStore, redirect, a.authCodeURL)
}

// Callback finishes an authorization-code flow.
func (a *OIDCAuthenticator) Callback(ctx context.Context, values url.Values) (string, time.Time, string, errors.E) {
	return callbackFlow(ctx, a.flowStore, values, a.exchangeCode)
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
	return signOutFlow(w, req, a.tokenVerifier, a.revocationStore, a.revokeUpstream)
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
func (a *OIDCAuthenticator) authCodeURL(state, codeVerifier, nonce string) string {
	cfg := a.oauthConfig()
	return cfg.AuthCodeURL(
		state,
		oauth2.AccessTypeOnline,
		oauth2.S256ChallengeOption(codeVerifier),
		oidc.Nonce(nonce),
	)
}

// exchangeCode finishes the authorization-code flow.
func (a *OIDCAuthenticator) exchangeCode(ctx context.Context, code, codeVerifier, expectedNonce string) (string, time.Time, errors.E) {
	// The pooled HTTP client is shared with JWKS / userinfo so token
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

	idToken, err := a.tokenVerifier.Verify(ctx, rawIDToken)
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
	}
	err = idToken.Claims(&profile)
	if err != nil {
		return "", time.Time{}, errors.WithStack(err)
	}
	a.userInfoCache.set(idToken.Subject, userInfo{
		Subject:  idToken.Subject,
		Username: profile.PreferredUsername,
	})

	return response.AccessToken, response.Expiry, nil
}
