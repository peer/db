package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/go-cleanhttp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUserInfoCacheSetThenGet covers the primed-cache path: a value
// written with set is served on the next Get without ever fetching
// upstream.
func TestUserInfoCacheSetThenGet(t *testing.T) {
	t.Parallel()

	c := newUserInfoCache("", nil)
	c.set("user-1", userInfo{Subject: "user-1", Username: "alice"})

	info, errE := c.Get(t.Context(), "user-1", "any-token")
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, "user-1", info.Subject)
	assert.Equal(t, "alice", info.Username)
}

// TestUserInfoCacheMissNoEndpoint covers the "Authenticate ignores
// userinfo errors" contract from the production side: when the cache
// misses and no upstream endpoint is configured (mock authenticator
// case), Get returns an error that the caller is expected to swallow.
func TestUserInfoCacheMissNoEndpoint(t *testing.T) {
	t.Parallel()

	c := newUserInfoCache("", nil)
	_, errE := c.Get(t.Context(), "user-1", "any-token")
	require.Error(t, errE, "missing endpoint must surface an error so the caller can fall back to subject-only")
}

// TestUserInfoCacheFetchesThenCaches covers the OIDC path: a miss
// triggers a single upstream call, the result is cached, and a
// subsequent Get for the same subject hits the cache (no second
// upstream call).
func TestUserInfoCacheFetchesThenCaches(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		assert.Equal(t, "Bearer my-token", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"sub":                "user-1",
			"preferred_username": "alice",
		})
	}))
	t.Cleanup(ts.Close)

	c := newUserInfoCache(ts.URL, cleanhttp.DefaultPooledClient())

	info, errE := c.Get(t.Context(), "user-1", "my-token")
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, "user-1", info.Subject)
	assert.Equal(t, "alice", info.Username)
	assert.Equal(t, int32(1), calls.Load())

	// Second call must be served from the cache without hitting the
	// upstream endpoint.
	info, errE = c.Get(t.Context(), "user-1", "my-token")
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, "alice", info.Username)
	assert.Equal(t, int32(1), calls.Load(), "second Get must come from cache")
}

// TestUserInfoCacheFetchFailureNotCached covers the failure path: an
// upstream non-200 must not leave a poisoned cache entry, so the next
// Get retries.
func TestUserInfoCacheFetchFailureNotCached(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(ts.Close)

	c := newUserInfoCache(ts.URL, cleanhttp.DefaultPooledClient())

	_, errE := c.Get(t.Context(), "user-1", "my-token")
	require.Error(t, errE)

	_, errE = c.Get(t.Context(), "user-1", "my-token")
	require.Error(t, errE)
	assert.Equal(t, int32(2), calls.Load(), "failed fetch must not be cached; second Get must retry")
}

// TestUserInfoCacheFallsBackToSubject covers the "issuer omits sub"
// case: when the response body has no sub field, the cache fills it
// in from the lookup key so the UserInfo header always carries one.
func TestUserInfoCacheFallsBackToSubject(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"preferred_username": "alice",
		})
	}))
	t.Cleanup(ts.Close)

	c := newUserInfoCache(ts.URL, cleanhttp.DefaultPooledClient())

	info, errE := c.Get(t.Context(), "user-1", "tok")
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, "user-1", info.Subject, "missing sub must be backfilled from the lookup key")
	assert.Equal(t, "alice", info.Username)
}
