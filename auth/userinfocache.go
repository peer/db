package auth

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"golang.org/x/sync/singleflight"
)

// userInfoCacheTTL is how long a userinfo lookup result is cached before we
// re-query the issuer.
const userInfoCacheTTL = 24 * time.Hour

// userInfo carries the profile fields the auth middleware exposes on the response.
type userInfo struct {
	Subject  string
	Username string
}

// userInfoCacheEntry pairs a value with its expiry so a single mutex guards both fields.
type userInfoCacheEntry struct {
	info    userInfo
	expires time.Time
}

// userInfoCache memoizes OIDC userinfo lookups keyed by subject. Concurrent
// requests for the same subject coalesce via singleflight so the issuer sees
// at most one in-flight request per subject regardless of how many client
// connections we have.
type userInfoCache struct {
	endpoint string
	client   *http.Client

	mu    sync.Mutex
	items map[string]userInfoCacheEntry

	sf singleflight.Group

	ttl time.Duration
	now func() time.Time
}

// newUserInfoCache builds a cache backed by the given userinfo endpoint. The
// HTTP client should be pooled so JWKS refreshes (in the authenticator) and
// userinfo lookups share connections.
func newUserInfoCache(endpoint string, client *http.Client) *userInfoCache {
	return &userInfoCache{ //nolint:exhaustruct
		endpoint: endpoint,
		client:   client,
		items:    map[string]userInfoCacheEntry{},
		ttl:      userInfoCacheTTL,
		now:      time.Now,
	}
}

// set stores info for subject with the standard TTL, overwriting any existing entry.
func (c *userInfoCache) set(subject string, info userInfo) {
	if subject == "" {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.items[subject] = userInfoCacheEntry{info: info, expires: c.now().Add(c.ttl)}
}

func (c *userInfoCache) get(subject string) (userInfoCacheEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.items[subject]
	return entry, ok
}

// Get returns the cached userInfo for subject, fetching it from the issuer
// when missing or expired. The subject is taken from the verified access
// token. The token is forwarded to the issuer as a Bearer credential.
//
// On upstream failure Get does not cache.
func (c *userInfoCache) Get(ctx context.Context, subject, token string) (userInfo, errors.E) {
	if subject == "" {
		return userInfo{}, errors.New("subject is required")
	}

	entry, ok := c.get(subject)
	if ok && c.now().Before(entry.expires) {
		return entry.info, nil
	}

	v, err, _ := c.sf.Do(subject, func() (any, error) {
		info, errE := c.fetch(ctx, token)
		if errE != nil {
			return userInfo{}, errE
		}
		// We use the lookup key when the response omits sub
		// (some issuers sometimes do).
		if info.Subject == "" {
			info.Subject = subject
		}

		// Cache might have been updated in meantime already by somebody else for
		// the same subject, but we do not care because it is probably the same or
		// at least both recent enough.
		c.set(subject, info)
		return info, nil
	})
	if err != nil {
		return userInfo{}, errors.WithStack(err)
	}
	info, _ := v.(userInfo)
	return info, nil
}

// fetch performs the actual upstream call to the OIDC userinfo endpoint.
func (c *userInfoCache) fetch(ctx context.Context, token string) (userInfo, errors.E) {
	if c.endpoint == "" {
		return userInfo{}, errors.New("userinfo endpoint not configured")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint, nil)
	if err != nil {
		return userInfo{}, errors.WithStack(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
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
	}
	// We accept extra fields silently.
	errE := x.DecodeJSON(resp.Body, &payload)
	if errE != nil {
		return userInfo{}, errE
	}
	return userInfo{Subject: payload.Sub, Username: payload.PreferredUsername}, nil
}
