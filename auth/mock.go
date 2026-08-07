package auth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
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

// mockSubjectPrefix is the prefix of the sub claim of mock-minted JWTs. The rest of the subject spells
// out the roles the session was signed in with, each after a dash and in alphabetical order, and ends
// with the site domain, so the UserInfo header surfaces both which site a session was signed into and as
// whom: "mock-user@example.test" holds no role, "mock-user-admin-editor@example.test" holds two. The
// mock signs in as many users as there are combinations of the site's roles, and the subject is what
// tells them apart.
//
// A role name containing a dash cannot be told apart from two roles, so a subject spelling one out is
// not recognized (see rolesFromSubject): the site's role names are what a mock subject is read against.
const mockSubjectPrefix = "mock-user"

// The code the mock "issuer" hands to the callback: the prefix followed by the roles the sign-in claims,
// separated by mockRolesSeparator and empty for a sign-in with no role at all. It is what the page
// choosing the roles makes out of them (see MockAuthenticator.SignIn), and the only code the mock mints.
const (
	mockCodePrefix           = "mock:"
	mockRolesSeparator       = ","
	mockSubjectRoleSeparator = "-"
)

// mockUsernamePrefix is the prefix of the preferred_username surfaced to the frontend in the UserInfo
// header. The rest names the roles the user was signed in with, the same way the subject does, so the
// mock's users are told apart by their name as well: "mock" holds no role, "mock-admin-editor" holds
// two. Shared across sites is fine - the per-site distinction already lives in the subject.
const mockUsernamePrefix = "mock"

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

	issuer        string
	clientID      string
	subjectSuffix string
	privateKey    *rsa.PrivateKey
	keyID         string
	grantedRoles  func() []string
	redirectURI   func() string
	signInURI     func() string
}

// mockName spells the roles out after the prefix, each after a dash and in alphabetical order, which is
// how both the subject and the username of a mock user are made (see mockSubjectPrefix). The roles are
// expected to be known to the site already.
func mockName(prefix string, roles []string) string {
	var name strings.Builder
	name.WriteString(prefix)
	for _, role := range slices.Sorted(slices.Values(roles)) {
		name.WriteString(mockSubjectRoleSeparator)
		name.WriteString(role)
	}
	return name.String()
}

// mockSubject returns the sub claim of a session signed in with the given roles: the roles spelled out
// between the mock prefix and the site domain (see mockSubjectPrefix).
func mockSubject(roles []string, subjectSuffix string) string {
	return mockName(mockSubjectPrefix, roles) + subjectSuffix
}

// mockUsername returns the preferred_username of a session signed in with the given roles (see
// mockUsernamePrefix).
func mockUsername(roles []string) string {
	return mockName(mockUsernamePrefix, roles)
}

// rolesFromSubject returns the roles the mock subject was signed in with, and whether it is a subject of
// this mock at all: the site's own role names are what its dash-separated part is read against, so a
// subject naming a role the site does not have is nobody's (see mockSubjectPrefix).
func rolesFromSubject(subject string, subjectSuffix string, granted []string) ([]string, bool) {
	rest, ok := strings.CutSuffix(subject, subjectSuffix)
	if !ok {
		return nil, false
	}
	rest, ok = strings.CutPrefix(rest, mockSubjectPrefix)
	if !ok {
		return nil, false
	}
	if rest == "" {
		return nil, true
	}
	rest, ok = strings.CutPrefix(rest, mockSubjectRoleSeparator)
	if !ok {
		return nil, false
	}
	roles := strings.Split(rest, mockSubjectRoleSeparator)
	for _, role := range roles {
		if !slices.Contains(granted, role) {
			return nil, false
		}
	}
	return roles, true
}

// rolesFromCode returns the roles a mock callback code stands for (see mockCodePrefix), keeping only
// those the site recognizes, without duplicates and in alphabetical order. It reports whether the code is
// a mock code at all.
func rolesFromCode(code string, granted []string) ([]string, bool) {
	rest, ok := strings.CutPrefix(code, mockCodePrefix)
	if !ok {
		return nil, false
	}
	roles := []string{}
	if rest != "" {
		for role := range strings.SplitSeq(rest, mockRolesSeparator) {
			if slices.Contains(granted, role) && !slices.Contains(roles, role) {
				roles = append(roles, role)
			}
		}
	}
	slices.Sort(roles)
	return roles, true
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
// grantedRoles is a thunk that resolves to the set of role names a mock
// sign-in can claim. It is evaluated at sign-in time, not at construction,
// so a caller may populate the site's roles after this authenticator has
// been built and the mock will still pick them up. Typically it returns the
// keys of the site's Roles map, so a mock user can hold every role the site
// recognises. redirectURI is a thunk that resolves to a URL the post-sign-in
// browser should land on, and signInURI one that resolves to the page where
// the roles to sign in with are chosen (see SignIn).
func NewMockAuthenticator(
	ctx context.Context, dbpool *pgxpool.Pool, siteDomain string, grantedRoles func() []string, redirectURI, signInURI func() string,
) (*MockAuthenticator, errors.E) {
	if siteDomain == "" {
		return nil, errors.New("site domain is required")
	}
	if redirectURI == nil {
		return nil, errors.New("redirect URI thunk is required")
	}
	if signInURI == nil {
		return nil, errors.New("sign-in URI thunk is required")
	}

	privateKey, err := rsa.GenerateKey(rand.Reader, mockKeyBits)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	issuer := mockIssuerScheme + siteDomain
	clientID := mockClientIDPrefix + siteDomain
	subjectSuffix := "@" + siteDomain

	keyID := identifier.New().String()
	keySet := &oidc.StaticKeySet{
		PublicKeys: []crypto.PublicKey{&privateKey.PublicKey},
	}
	tokenVerifier := oidc.NewVerifier(issuer, keySet, &oidc.Config{ //nolint:exhaustruct
		ClientID: clientID,
	})

	// There is no issuer to ask about a user, so a lookup is answered from what the mock signs users in
	// as. Its users are the site's role combinations, each named by a subject spelling its roles out
	// (see mockSubjectPrefix), so a subject which is not one of those finds nobody, the way a real
	// issuer answers about somebody it does not know.
	cache := newUserInfoCache(
		func(_ context.Context, lookedUp, _ string) (userInfo, errors.E) {
			// The caller themselves is answered without roles, because their own token is what says which they
			// hold and Get stamps those onto the answer. Callback primes their entry anyway, so a user who just
			// signed in is not looked up at all, and this answers the misses: an entry which expired, or a
			// process which restarted while the session cookie stayed valid. Their name says which roles they
			// signed in with, which their subject spells out, and stays the bare one for a subject which is
			// not the mock's own shape (the caller's token is what makes them the caller either way).
			var granted []string
			if grantedRoles != nil {
				granted = grantedRoles()
			}
			roles, _ := rolesFromSubject(lookedUp, subjectSuffix, granted)
			return userInfo{Subject: lookedUp, Username: mockUsername(roles), Roles: nil}, nil
		},
		func(_ context.Context, lookedUp, _ string) (userInfo, errors.E) {
			var granted []string
			if grantedRoles != nil {
				granted = grantedRoles()
			}
			roles, ok := rolesFromSubject(lookedUp, subjectSuffix, granted)
			if !ok {
				errE := errors.WithStack(ErrIdentityNotFound)
				errors.Details(errE)["subject"] = lookedUp
				return userInfo{}, errE
			}
			return userInfo{Subject: lookedUp, Username: mockUsername(roles), Roles: roles}, nil
		},
	)

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
			TokenVerifier:   tokenVerifier,
			UserInfoCache:   cache,
			FlowStore:       fs,
			RevocationStore: rs,
		},
		issuer:        issuer,
		clientID:      clientID,
		subjectSuffix: subjectSuffix,
		privateKey:    privateKey,
		keyID:         keyID,
		grantedRoles:  grantedRoles,
		redirectURI:   redirectURI,
		signInURI:     signInURI,
	}, nil
}

// SignIn begins a mock sign-in flow. It records flow state in the internal
// store (same as OIDC) so the callback round-trip exercises the same
// flow-store code path. The returned URL is a self-redirect to the page
// choosing the roles to sign in with, which stands in for the issuer's own
// sign-in page and sends the browser on to our callback handler, so the
// issuer is never contacted.
func (a *MockAuthenticator) SignIn(ctx context.Context, redirect, uiLocales string) (string, errors.E) {
	return signInFlow(ctx, a.FlowStore, redirect, uiLocales, a.authCodeURL)
}

// Callback finishes a mock sign-in flow. It validates the callback
// parameters, consumes the matching flow row, mints a freshly-signed JWT
// (the access token the cookie should carry), and returns the token, its
// expiry, and the post-sign-in redirect recorded at SignIn time.
// Client-side failures wrap ErrSignInFailed.
func (a *MockAuthenticator) Callback(ctx context.Context, values url.Values, allowedRoles map[string]RoleGrants) (string, time.Time, string, errors.E) {
	return callbackFlow(ctx, a.FlowStore, values, allowedRoles, a.exchangeCode)
}

// SignOut revokes the request's access token. The mock has no upstream
// to notify, it only writes to the local revocation store (and its
// cache). The user is thereafter rejected by Authenticate even though
// the JWT signature/exp are still valid.
func (a *MockAuthenticator) SignOut(w http.ResponseWriter, req *http.Request) errors.E {
	return signOutFlow(w, req, a.TokenVerifier, a.RevocationStore, nil)
}

// authCodeURL returns a self-redirect URL: the local page choosing the roles
// to sign in with, with the state to carry on. That page mints the code out
// of the chosen roles (see mockCodePrefix) and sends the browser to our own
// AuthCallback handler, which consumes the flow row keyed by state and then
// calls exchangeCode. No external issuer is contacted.
func (a *MockAuthenticator) authCodeURL(state, _, _, _ string) string {
	base := a.signInURI()
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
	q.Set("state", state)
	u.RawQuery = q.Encode()
	return u.String()
}

// exchangeCode mints a freshly-signed JWT carrying the roles the code stands for (see mockCodePrefix), for
// the user those roles make (see mockSubjectPrefix). A code which is not the mock's own is refused as a
// failed sign-in, the way an issuer refuses a code it did not mint.
//
// The codeVerifier is ignored - the flow store round-trip has already validated state. expectedNonce is
// embedded into the JWT's nonce claim.
//
// Internal to the package; test code reaches it through
// TestingExchangeCode in auth_internal_test.go.
func (a *MockAuthenticator) exchangeCode(_ context.Context, code, _, _ string, allowedRoles map[string]RoleGrants) (string, time.Time, errors.E) {
	now := time.Now()
	cookieExpiry := now.Add(mockTokenTTL)

	// We extend cookieExpiry by a small grace so that the JWT's exp claim is
	// never validated as "already expired" against the very same time stamp
	// we set Max-Age from. The cookie anyway deletes itself first.
	jwtExpiry := cookieExpiry.Add(time.Minute)

	// The roles the site recognizes are resolved here, at sign-in time, so roles configured on the site
	// after this authenticator was built are still picked up. They are what the code is read against:
	// the sign-in claims those of them it names.
	var granted []string
	if a.grantedRoles != nil {
		granted = a.grantedRoles()
	}
	roles, ok := rolesFromCode(code, granted)
	if !ok {
		errE := errors.WithStack(ErrSignInFailed)
		errors.Details(errE)["code"] = code
		return "", time.Time{}, errE
	}
	subject := mockSubject(roles, a.subjectSuffix)

	// Base claims we always advertise plus one role.<key> entry per claimed role. Pre-sized so the
	// appends below do not grow the slice.
	baseScopes := []string{oidc.ScopeOpenID, "profile", "email"}
	scopes := make([]string, 0, len(baseScopes)+len(roles))
	scopes = append(scopes, baseScopes...)
	for _, role := range roles {
		scopes = append(scopes, roleScopePrefix+role)
	}

	claims := map[string]any{
		"iss":       a.issuer,
		"aud":       []string{a.clientID},
		"sub":       subject,
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
	// userinfo fetch (mock has no upstream endpoint). The roles are the
	// ones just minted into the token which the site recognizes, so the
	// entry describes the user fully and a lookup of them (see Identity)
	// needs no fetch either.
	a.UserInfoCache.set(subject, userInfo{Subject: subject, Username: mockUsername(roles), Roles: filterRoles(roles, allowedRoles)})

	return token, cookieExpiry, nil
}
