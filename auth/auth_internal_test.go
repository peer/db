package auth

import (
	"context"
	"time"

	"gitlab.com/tozd/go/errors"
)

var (
	TestingResolveAccessToken = resolveAccessToken //nolint:gochecknoglobals
	TestingWithSubject        = withSubject        //nolint:gochecknoglobals
	TestingWithRoles          = withRoles          //nolint:gochecknoglobals
)

const TestingAccessTokenCookieName = accessTokenCookieName

func (a *MockAuthenticator) TestingAuthCodeURL(state, codeVerifier, nonce string) string {
	return a.authCodeURL(state, codeVerifier, nonce)
}

func (a *MockAuthenticator) TestingExchangeCode(ctx context.Context, code, codeVerifier, expectedNonce string) (string, time.Time, errors.E) {
	return a.exchangeCode(ctx, code, codeVerifier, expectedNonce)
}
