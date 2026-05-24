package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/errors"

	"gitlab.com/peerdb/peerdb/auth"
)

// fakeAuthenticator records Authenticate calls without doing any real
// token validation. Used by Middleware tests to assert the call path
// the middleware follows.
type fakeAuthenticator struct {
	authCalls         int
	lastPrefix        string
	lastAllowedRoles  map[string][]string
	lastSubjectMarker string
}

func (f *fakeAuthenticator) Authenticate(_ http.ResponseWriter, req *http.Request, prefix string, allowedRoles map[string][]string) context.Context {
	f.authCalls++
	f.lastPrefix = prefix
	f.lastAllowedRoles = allowedRoles
	// Annotate the context so the downstream handler can prove it
	// received the ctx Authenticate returned.
	type markerKey struct{}
	return context.WithValue(req.Context(), markerKey{}, f.lastSubjectMarker)
}

func (*fakeAuthenticator) SignIn(_ context.Context, _ string) (string, errors.E) {
	return "", errors.New("not implemented")
}

func (*fakeAuthenticator) Callback(_ context.Context, _ url.Values) (string, time.Time, string, errors.E) {
	return "", time.Time{}, "", errors.New("not implemented")
}

func (*fakeAuthenticator) SignOut(_ http.ResponseWriter, _ *http.Request) errors.E {
	return errors.New("not implemented")
}

func (*fakeAuthenticator) CleanupExpired(_ context.Context) errors.E {
	return errors.New("not implemented")
}

// TestMiddlewareCallsAuthenticateAndNext covers the happy path: lookup
// returns handled=false plus a non-nil Authenticator, Authenticate runs,
// and next.ServeHTTP fires with the request whose context Authenticate
// returned.
func TestMiddlewareCallsAuthenticateAndNext(t *testing.T) {
	t.Parallel()

	fake := &fakeAuthenticator{}
	allowed := map[string][]string{"admin": {"canEdit"}}

	var (
		nextCalls int
		nextReq   *http.Request
	)
	next := http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
		nextCalls++
		nextReq = req
	})

	mw := auth.Middleware("Prefix-", func(_ http.ResponseWriter, _ *http.Request) (auth.Authenticator, map[string][]string, bool) {
		return fake, allowed, false
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil)
	w := httptest.NewRecorder()
	mw(next).ServeHTTP(w, req)

	assert.Equal(t, 1, fake.authCalls)
	assert.Equal(t, "Prefix-", fake.lastPrefix)
	assert.Equal(t, allowed, fake.lastAllowedRoles)
	assert.Equal(t, 1, nextCalls)
	require.NotNil(t, nextReq)
	// next must receive the request whose context Authenticate returned,
	// not the original request context.
	assert.NotSame(t, req, nextReq, "next must see a request with the Authenticate-returned context")
	// Default 200 (the next handler wrote nothing else).
	assert.Equal(t, http.StatusOK, w.Result().StatusCode)
}

// TestMiddlewareShortCircuitsWhenHandled covers the lookup-error path:
// when the lookup returns handled=true it has already written the
// response, the middleware must not call Authenticate and must not call
// next. The lookup writing 500 simulates what serve.go's
// lookupSiteAuthenticator does via InternalServerErrorWithError.
func TestMiddlewareShortCircuitsWhenHandled(t *testing.T) {
	t.Parallel()

	fake := &fakeAuthenticator{}
	nextCalls := 0
	next := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		nextCalls++
	})

	mw := auth.Middleware("Prefix-", func(w http.ResponseWriter, _ *http.Request) (auth.Authenticator, map[string][]string, bool) {
		http.Error(w, "no site", http.StatusInternalServerError)
		return nil, nil, true
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil)
	w := httptest.NewRecorder()
	mw(next).ServeHTTP(w, req)

	assert.Equal(t, 0, fake.authCalls, "Authenticate must not run when lookup reports handled")
	assert.Equal(t, 0, nextCalls, "next must not run when lookup reports handled")
	assert.Equal(t, http.StatusInternalServerError, w.Result().StatusCode)
}
