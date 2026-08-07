package auth

import (
	"context"
	"sync"
	"time"

	"gitlab.com/tozd/go/errors"
	"golang.org/x/sync/singleflight"
)

// userInfoCacheTTL is how long a userinfo lookup result is cached before we
// re-query the issuer.
const userInfoCacheTTL = 24 * time.Hour

// userInfo carries what is known about a user: the profile fields the auth middleware exposes on the
// response, and the roles the user holds of those the site recognizes (see filterRoles). Roles are
// cached with the rest, but they are not part of the UserInfo response header (the roles of the caller
// travel in the Roles header of their own).
//
// None of this authorizes anything. What a caller may do is decided by the roles of their own verified
// access token (see extractRoles and Roles), never by what is cached here: for the caller themselves
// their token is what GetSelf stamps onto the answer anyway, while about anybody else this is only
// what the issuer said, for as long as the entry lives. It is good for deciding what granting a user
// access has to add, where roles which are wrong give nobody a permission they should not have: the
// document ends up carrying claims which are redundant, or missing claims the user turns out to need.
type userInfo struct {
	Subject  string
	Username string
	Roles    []string
}

// userInfoCacheEntry pairs a value with its expiry so a single mutex guards both fields.
type userInfoCacheEntry struct {
	Info    userInfo
	Expires time.Time
}

// userInfoCache memoizes user lookups keyed by subject. Concurrent requests
// for the same subject coalesce via singleflight so the issuer sees at most
// one in-flight request per subject regardless of how many client connections
// we have.
//
// An entry is keyed by the subject alone and not by who looked them up, so what was fetched with one
// user's credentials answers every other user of the site as well. This assumes that the issuer
// describes a user the same way to all of them, which is what the endpoints used here do: userinfo is
// about the caller themselves, and the issuer's endpoint for another user is readable by any user of
// the organization (see OIDCAuthenticator.Identity).
type userInfoCache struct {
	// FetchSelf asks about the user the token belongs to and FetchOther about a user somebody else's
	// token wants to know about, each called with the subject it is asked about and the token to ask
	// with. They are what the authenticator does there: the OIDC one asks the issuer (its userinfo
	// endpoint about the caller, another endpoint about anybody else), while the mock one answers with
	// its own user, having no issuer to ask.
	FetchSelf  func(ctx context.Context, subject, token string) (userInfo, errors.E)
	FetchOther func(ctx context.Context, subject, token string) (userInfo, errors.E)

	TTL time.Duration
	Now func() time.Time

	mu    sync.Mutex
	items map[string]userInfoCacheEntry

	sf singleflight.Group
}

// newUserInfoCache builds a cache which asks the given lookups about a user whenever it has nothing
// fresh about them: fetchSelf about the caller themselves (see GetSelf) and fetchOther about anybody
// else (see GetOther).
func newUserInfoCache(fetchSelf, fetchOther func(ctx context.Context, subject, token string) (userInfo, errors.E)) *userInfoCache {
	return &userInfoCache{
		FetchSelf:  fetchSelf,
		FetchOther: fetchOther,
		TTL:        userInfoCacheTTL,
		Now:        time.Now,
		mu:         sync.Mutex{},
		items:      map[string]userInfoCacheEntry{},
		sf:         singleflight.Group{},
	}
}

// set stores info for subject with the standard TTL, overwriting any existing entry.
func (c *userInfoCache) set(subject string, info userInfo) {
	if subject == "" {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.items[subject] = userInfoCacheEntry{Info: info, Expires: c.Now().Add(c.TTL)}
}

func (c *userInfoCache) get(subject string) (userInfoCacheEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.items[subject]
	return entry, ok
}

// GetSelf returns the cached userInfo for the caller themselves, looking them up with FetchSelf when
// it is missing or expired. The subject is taken from the verified access token, which the lookup goes
// with as the credential, and roles are the ones that token grants of those the site recognizes (see
// extractRoles): a lookup with the caller's token is about the caller, so what their token says about
// them is what is cached with it.
//
// On lookup failure GetSelf does not cache.
func (c *userInfoCache) GetSelf(ctx context.Context, subject, token string, roles []string) (userInfo, errors.E) {
	info, errE := c.lookup(ctx, subject, token, func(ctx context.Context, subject, token string) (userInfo, errors.E) {
		info, errE := c.FetchSelf(ctx, subject, token)
		if errE != nil {
			return userInfo{}, errE
		}
		// A lookup of the caller says who they are but not which roles they hold, so the entry is
		// completed with the ones their token grants before it is cached, and a lookup of them by
		// somebody else (see GetOther) then finds an entry which describes them fully.
		info.Roles = roles
		return info, nil
	})
	if errE != nil {
		return userInfo{}, errE
	}
	// On a cache hit nothing above ran, and the entry may have been written a day ago or by a lookup
	// somebody else made. The caller's token is what grants their roles, so it says more about them
	// than any of that. It is also what decides everything they may do (see userInfo), which is why
	// their roles are never read out of the cache but always stamped from their token.
	info.Roles = roles
	return info, nil
}

// GetOther returns the cached userInfo for a user other than the caller, looking them up with
// FetchOther when it is missing or expired, with the caller's token as the credential. Of the roles
// the lookup reports it keeps the ones the site recognizes (see filterRoles), because what the cache
// holds is what the site knows about a user.
//
// On lookup failure GetOther does not cache.
func (c *userInfoCache) GetOther(ctx context.Context, subject, token string, allowedRoles map[string]RoleGrants) (userInfo, errors.E) {
	return c.lookup(ctx, subject, token, func(ctx context.Context, subject, token string) (userInfo, errors.E) {
		info, errE := c.FetchOther(ctx, subject, token)
		if errE != nil {
			return userInfo{}, errE
		}
		info.Roles = filterRoles(info.Roles, allowedRoles)
		return info, nil
	})
}

// lookup is the shared body of GetSelf and GetOther: a cached entry which has not expired is returned as
// is, and otherwise fetch is called and its result cached. Concurrent lookups of the same subject
// coalesce, so fetch is called at most once per subject at a time.
func (c *userInfoCache) lookup(
	ctx context.Context, subject, token string, fetch func(ctx context.Context, subject, token string) (userInfo, errors.E),
) (userInfo, errors.E) {
	if subject == "" {
		return userInfo{}, errors.New("subject is required")
	}

	entry, ok := c.get(subject)
	if ok && c.Now().Before(entry.Expires) {
		return entry.Info, nil
	}

	v, err, _ := c.sf.Do(subject, func() (any, error) {
		info, errE := fetch(ctx, subject, token)
		if errE != nil {
			return userInfo{}, errE
		}
		// The answer has to say who it is about, and it has to be the subject it was asked about: it is
		// cached under them, and an answer about somebody else says nothing about them. OIDC requires
		// both of a userinfo response (the subject is always returned, and it has to match the one of
		// the token), and an answer about another user is just as wrong for any other lookup.
		if info.Subject == "" {
			errE := errors.New("lookup returned no subject")
			errors.Details(errE)["subject"] = subject
			return userInfo{}, errE
		}
		if info.Subject != subject {
			errE := errors.New("lookup returned a different subject")
			errors.Details(errE)["subject"] = subject
			errors.Details(errE)["returned"] = info.Subject
			return userInfo{}, errE
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
