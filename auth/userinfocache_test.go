package auth_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/go-cleanhttp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/peerdb/peerdb/auth"
)

// TestUserInfoCacheSetThenGet covers the primed-cache path: a value
// written with set is served on the next Get without ever fetching
// upstream.
func TestUserInfoCacheSetThenGet(t *testing.T) {
	t.Parallel()

	c := auth.TestingNewUserInfoCache("", nil)
	c.TestingSet("user-1", auth.TestingUserInfo{Subject: "user-1", Username: "alice", Roles: nil})

	info, errE := c.GetSelf(t.Context(), "user-1", "any-token", nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, "user-1", info.Subject)
	assert.Equal(t, "alice", info.Username)
}

// TestUserInfoCacheMissNoEndpoint covers the "Authenticate ignores
// userinfo errors" contract from the production side: when the cache
// misses and the issuer advertises no userinfo endpoint, Get returns an
// error that the caller is expected to log and fall back from.
func TestUserInfoCacheMissNoEndpoint(t *testing.T) {
	t.Parallel()

	c := auth.TestingNewUserInfoCache("", nil)
	_, errE := c.GetSelf(t.Context(), "user-1", "any-token", nil)
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

	c := auth.TestingNewUserInfoCache(ts.URL, cleanhttp.DefaultPooledClient())

	info, errE := c.GetSelf(t.Context(), "user-1", "my-token", nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, "user-1", info.Subject)
	assert.Equal(t, "alice", info.Username)
	assert.Equal(t, int32(1), calls.Load())

	// Second call must be served from the cache without hitting the
	// upstream endpoint.
	info, errE = c.GetSelf(t.Context(), "user-1", "my-token", nil)
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

	c := auth.TestingNewUserInfoCache(ts.URL, cleanhttp.DefaultPooledClient())

	_, errE := c.GetSelf(t.Context(), "user-1", "my-token", nil)
	require.Error(t, errE)

	_, errE = c.GetSelf(t.Context(), "user-1", "my-token", nil)
	require.Error(t, errE)
	assert.Equal(t, int32(2), calls.Load(), "failed fetch must not be cached; second Get must retry")
}

// TestUserInfoCacheRejectsSubjectMismatch covers the answer which is about
// another user: it says nothing about the subject looked up, so it is an
// error rather than an entry cached under them.
func TestUserInfoCacheRejectsSubjectMismatch(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"sub":                "user-2",
			"preferred_username": "bob",
		})
	}))
	t.Cleanup(ts.Close)

	c := auth.TestingNewUserInfoCache(ts.URL, cleanhttp.DefaultPooledClient())

	_, errE := c.GetSelf(t.Context(), "user-1", "my-token", nil)
	assert.EqualError(t, errE, "lookup returned a different subject")

	// Nothing was cached, so the next Get asks again.
	_, errE = c.GetSelf(t.Context(), "user-1", "my-token", nil)
	assert.EqualError(t, errE, "lookup returned a different subject")
	assert.Equal(t, int32(2), calls.Load())
}

// TestUserInfoCacheRejectsMissingSubject covers the "issuer omits sub"
// case: a response without one does not say who it is about, which OIDC
// does not allow of a userinfo response, so it is an error rather than an
// answer assumed to be about the subject looked up.
func TestUserInfoCacheRejectsMissingSubject(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"preferred_username": "alice",
		})
	}))
	t.Cleanup(ts.Close)

	c := auth.TestingNewUserInfoCache(ts.URL, cleanhttp.DefaultPooledClient())

	_, errE := c.GetSelf(t.Context(), "user-1", "tok", nil)
	assert.EqualError(t, errE, "lookup returned no subject")
}
