package auth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	jose "github.com/go-jose/go-jose/v4"
	"github.com/jackc/pgx/v5/pgxpool"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"
)

// mockIssuerScheme is the URL scheme used in the iss claim of mock-minted
// JWTs. The "mock://" scheme makes the value clearly non-routable so it can
// never collide with a real OIDC issuer URL.
const mockIssuerScheme = "mock://"

// mockClientIDPrefix is the prefix of the client ID claim of mock-minted JWTs
// which is also a base for the aud claim. The full audience is per-site so
// two sites' mocks reject each other's tokens at the aud check even before
// the signature check.
const mockClientIDPrefix = "peerdb-mock:"

// mockSubjectPrefix is the prefix of the sub claim of mock-minted JWTs.
// The full subject includes the site domain so the UserInfo header surfaces
// which site a session was signed into.
const mockSubjectPrefix = "mock-user@"

// mockUsername is the preferred_username surfaced to the frontend in the
// UserInfo header. Shared across sites is fine - the per-site distinction
// already lives in subject.
const mockUsername = "mock"

// mockTokenTTL is how long a mock-minted JWT is considered valid. The
// access-token cookie's lifetime matches it so the session ends at the
// same moment the cookie expires.
const mockTokenTTL = 24 * time.Hour

// mockKeyBits is the size of the RSA key generated at MockAuthenticator
// construction. 2048 is the smallest size the Go runtime still considers
// modern.
const mockKeyBits = 2048

// MockAuthenticator short-circuits the OIDC flow for development. At
// construction it generates an in-process RSA key pair, configures an OIDC
// token verifier against the public half, builds an internal per-site flow
// store, and remembers the role names a successful "sign-in" should grant.
// SignIn returns a self-redirect that loops the browser straight back at
// the callback.
//
// Each site that does not configure an OIDC issuer gets its own
// MockAuthenticator instance: the RSA key, the issuer/audience/subject
// claims, and the role list are all per-site, so a token minted for one
// site's mock is rejected at every layer (signature, issuer, audience) by
// any other site's mock.
//
// MockAuthenticator is intended for development. It is configured
// implicitly for any site whose Auth block does not set an OIDC issuer.
type MockAuthenticator struct {
	baseAuthenticator

	issuer       string
	clientID     string
	subject      string
	privateKey   *rsa.PrivateKey
	keyID        string
	grantedRoles []string
	redirectURI  func() string
}

// NewMockAuthenticator creates a MockAuthenticator scoped to the given site
// domain.
//
// The domain is baked into the issuer, audience, and subject claims
// so each site's mock is structurally distinct from every other site's
// mock (in addition to the per-instance RSA key that already isolates
// signatures).
//
// dbpool is used to construct and initialise the flow and revocation stores.
//
// grantedRoles is the set of role names a successful mock sign-in should
// claim. Typically the keys of the site's Roles map, so a mock user holds
// every role the site recognises. redirectURI is a thunk that resolves to
// a URL the post-sign-in browser should land on.
func NewMockAuthenticator(ctx context.Context, dbpool *pgxpool.Pool, siteDomain string, grantedRoles []string, redirectURI func() string) (*MockAuthenticator, errors.E) {
	if siteDomain == "" {
		return nil, errors.New("site domain is required")
	}
	if redirectURI == nil {
		return nil, errors.New("redirect URI thunk is required")
	}

	privateKey, err := rsa.GenerateKey(rand.Reader, mockKeyBits)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	issuer := mockIssuerScheme + siteDomain
	clientID := mockClientIDPrefix + siteDomain
	subject := mockSubjectPrefix + siteDomain

	keyID := identifier.New().String()
	keySet := &oidc.StaticKeySet{
		PublicKeys: []crypto.PublicKey{&privateKey.PublicKey},
	}
	tokenVerifier := oidc.NewVerifier(issuer, keySet, &oidc.Config{ //nolint:exhaustruct
		ClientID: clientID,
	})

	// The userinfo cache is primed at Callback time and never fetches
	// upstream: an empty endpoint short-circuits userInfoCache.fetch to an
	// error, but Authenticate ignores upstream failures and falls back to
	// the primed entry (or subject-only).
	cache := newUserInfoCache("", nil)

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

	return &MockAuthenticator{
		baseAuthenticator: baseAuthenticator{
			tokenVerifier:   tokenVerifier,
			userInfoCache:   cache,
			flowStore:       fs,
			revocationStore: rs,
		},
		issuer:       issuer,
		clientID:     clientID,
		subject:      subject,
		privateKey:   privateKey,
		keyID:        keyID,
		grantedRoles: append([]string(nil), grantedRoles...),
		redirectURI:  redirectURI,
	}, nil
}

// SignIn begins a mock sign-in flow. It records flow state in the internal
// store (same as OIDC) so the callback round-trip exercises the same
// flow-store code path. The returned URL is a self-redirect to our own
// callback handler with synthetic code+state, so the issuer is never
// contacted.
func (a *MockAuthenticator) SignIn(ctx context.Context, redirect string) (string, errors.E) {
	return signInFlow(ctx, a.flowStore, redirect, a.authCodeURL)
}

// Callback finishes a mock sign-in flow. It validates the callback
// parameters, consumes the matching flow row, mints a freshly-signed JWT
// (the access token the cookie should carry), and returns the token, its
// expiry, and the post-sign-in redirect recorded at SignIn time.
// Client-side failures wrap ErrSignInFailed.
func (a *MockAuthenticator) Callback(ctx context.Context, values url.Values) (string, time.Time, string, errors.E) {
	return callbackFlow(ctx, a.flowStore, values, a.exchangeCode)
}

// SignOut revokes the request's access token. The mock has no upstream
// to notify, it only writes to the local revocation store (and its
// cache). The user is thereafter rejected by Authenticate even though
// the JWT signature/exp are still valid.
func (a *MockAuthenticator) SignOut(w http.ResponseWriter, req *http.Request) errors.E {
	return signOutFlow(w, req, a.tokenVerifier, a.revocationStore, nil)
}

// authCodeURL returns a self-redirect URL: the local callback path with
// code+state already filled in. The browser follows it back to our own
// AuthCallback handler, which consumes the flow row keyed by state and
// then calls exchangeCode. No external issuer is contacted.
func (a *MockAuthenticator) authCodeURL(state, _, _ string) string {
	base := a.redirectURI()
	u, err := url.Parse(base)
	if err != nil {
		// Caller built the URL out of safe parts (https + Host + reverse
		// route lookup), so a parse failure here means a programmer error
		// not a user-influenced one.
		errE := errors.New("invalid redirect URI")
		errors.Details(errE)["redirect"] = base
		errors.Details(errE)["error"] = err
		panic(errE)
	}
	q := u.Query()
	q.Set("code", "mock")
	q.Set("state", state)
	u.RawQuery = q.Encode()
	return u.String()
}

// exchangeCode mints a freshly-signed JWT carrying every granted role.
//
// The code, codeVerifier are ignored - the flow store round-trip has
// already validated state. expectedNonce is embedded into the JWT's nonce
// claim.
//
// Internal to the package; test code reaches it through
// TestingExchangeCode in auth_internal_test.go.
func (a *MockAuthenticator) exchangeCode(_ context.Context, _, _, _ string) (string, time.Time, errors.E) {
	now := time.Now()
	cookieExpiry := now.Add(mockTokenTTL)

	// We extend cookieExpiry by a small grace so that the JWT's exp claim is
	// never validated as "already expired" against the very same time stamp
	// we set Max-Age from. The cookie anyway deletes itself first.
	jwtExpiry := cookieExpiry.Add(time.Minute)

	// Base claims we always advertise plus one role.<key> entry per
	// granted role. Pre-sized so the appends below do not grow the slice.
	baseScopes := []string{oidc.ScopeOpenID, "profile", "email"}
	scopes := make([]string, 0, len(baseScopes)+len(a.grantedRoles))
	scopes = append(scopes, baseScopes...)
	for _, role := range a.grantedRoles {
		scopes = append(scopes, roleScopePrefix+role)
	}

	claims := map[string]any{
		"iss":       a.issuer,
		"aud":       []string{a.clientID},
		"sub":       a.subject,
		"iat":       now.Unix(),
		"nbf":       now.Unix(),
		"exp":       jwtExpiry.Unix(),
		"scope":     strings.Join(scopes, " "),
		"client_id": a.clientID,
		"jti":       identifier.New().String(),
		"sid":       identifier.New().String(),
	}

	payload, err := json.Marshal(claims)
	if err != nil {
		return "", time.Time{}, errors.WithStack(err)
	}

	signingKey := jose.SigningKey{Algorithm: jose.RS256, Key: a.privateKey}
	opts := (&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", a.keyID)
	signer, err := jose.NewSigner(signingKey, opts)
	if err != nil {
		return "", time.Time{}, errors.WithStack(err)
	}

	signature, err := signer.Sign(payload)
	if err != nil {
		return "", time.Time{}, errors.WithStack(err)
	}

	token, err := signature.CompactSerialize()
	if err != nil {
		return "", time.Time{}, errors.WithStack(err)
	}

	// Prime the userinfo cache so the very first authenticated request
	// after sign-in finds the username in cache rather than failing the
	// userinfo fetch (mock has no upstream endpoint).
	a.userInfoCache.set(a.subject, userInfo{Subject: a.subject, Username: mockUsername})

	return token, cookieExpiry, nil
}
