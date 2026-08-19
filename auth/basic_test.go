package auth_test

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/peerdb/peerdb/auth"
)

const (
	basicAuthUsername = "testuser"
	basicAuthPassword = "testpass"
	basicAuthRealm    = "Test Realm"
)

// basicAuthHandler returns the basic-auth middleware around a handler which records that it ran and
// answers with 204, so that a test tells a request the gate let through from one it answered itself.
func basicAuthHandler(t *testing.T) (http.Handler, *int) {
	t.Helper()

	calls := 0
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusNoContent)
	})
	middleware := auth.BasicAuthMiddleware(basicAuthUsername, basicAuthPassword, func(_ *http.Request) string {
		return basicAuthRealm
	})
	return middleware(next), &calls
}

// basicAuthorization returns the value of the Authorization header for the credentials.
func basicAuthorization(username, password string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+password))
}

// TestBasicAuthMiddlewarePreflight checks that a CORS preflight passes the gate: a browser makes it
// without credentials, so gating it would fail every cross-origin request a browser makes rather than
// gate anything.
func TestBasicAuthMiddlewarePreflight(t *testing.T) {
	t.Parallel()

	handler, calls := basicAuthHandler(t)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodOptions, "/x", nil)
	req.Header.Set("Origin", "https://third-party.example")
	req.Header.Set("Access-Control-Request-Method", http.MethodGet)
	req.Header.Set("Access-Control-Request-Headers", "authorization")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, 1, *calls, "the preflight has to reach what answers it")
	assert.Equal(t, http.StatusNoContent, w.Result().StatusCode)
	assert.Empty(t, w.Header().Get("WWW-Authenticate"))
}

// TestBasicAuthMiddlewareNotPreflight checks that the exception is the preflight alone: an OPTIONS
// request which is not one, and a request of any other method stating the headers a preflight states,
// are gated like every other request.
func TestBasicAuthMiddlewareNotPreflight(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		Name   string
		Method string
		Header map[string]string
	}{
		{"plain OPTIONS", http.MethodOptions, nil},
		{"OPTIONS without origin", http.MethodOptions, map[string]string{"Access-Control-Request-Method": http.MethodGet}},
		{"OPTIONS without requested method", http.MethodOptions, map[string]string{"Origin": "https://third-party.example"}},
		{"GET with preflight headers", http.MethodGet, map[string]string{
			"Origin": "https://third-party.example", "Access-Control-Request-Method": http.MethodGet,
		}},
	} {
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()

			handler, calls := basicAuthHandler(t)

			req := httptest.NewRequestWithContext(t.Context(), tt.Method, "/x", nil)
			for name, value := range tt.Header {
				req.Header.Set(name, value)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			assert.Equal(t, 0, *calls, "the request must not reach past the gate")
			assert.Equal(t, http.StatusUnauthorized, w.Result().StatusCode)
			assert.Equal(t, `Basic realm="`+basicAuthRealm+`"`, w.Header().Get("WWW-Authenticate"))
		})
	}
}

// TestBasicAuthMiddlewareCredentials checks the gate itself: the request passes with the credentials it
// asks for and is answered with a challenge without them.
func TestBasicAuthMiddlewareCredentials(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		Name     string
		Username string
		Password string
		Status   int
	}{
		{"valid credentials", basicAuthUsername, basicAuthPassword, http.StatusNoContent},
		{"wrong password", basicAuthUsername, "wrongpass", http.StatusUnauthorized},
		{"wrong username", "wronguser", basicAuthPassword, http.StatusUnauthorized},
	} {
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()

			handler, calls := basicAuthHandler(t)

			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil)
			req.Header.Set("Authorization", basicAuthorization(tt.Username, tt.Password))
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			assert.Equal(t, tt.Status, w.Result().StatusCode)
			if tt.Status == http.StatusNoContent {
				require.Equal(t, 1, *calls)
				assert.Equal(t, "private", w.Header().Get("Cache-Control"))
			} else {
				require.Equal(t, 0, *calls)
			}
			assert.Contains(t, w.Header().Values("Vary"), "Authorization")
		})
	}
}
