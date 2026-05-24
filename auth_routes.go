package peerdb

import (
	"io"
	"net/http"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/waf"

	"gitlab.com/peerdb/peerdb/auth"
)

// signInRedirectQueryParam is the name of the query parameter clients use to
// request a specific post-sign-in landing page (eg ?redirect=/d/abc).
const signInRedirectQueryParam = "redirect"

// AuthSignInGet starts the sign-in flow. It hands the caller-supplied redirect
// off to the per-site Authenticator and then redirects the user to the URL
// the Authenticator returned.
//
// The optional ?redirect=<path> query parameter records where to send the
// user after the callback completes.
func (s *Service) AuthSignInGet(w http.ResponseWriter, req *http.Request, _ waf.Params) {
	defer req.Body.Close()              //nolint:errcheck
	defer io.Copy(io.Discard, req.Body) //nolint:errcheck

	// no-store: this URL is a side-effect entry point (creates a authentication
	// flow, redirects to the issuer). Nothing about its response is safe to keep
	// in any cache.
	w.Header().Set("Cache-Control", "no-store")

	ctx := req.Context()
	site := waf.MustGetSite[*Site](ctx)

	authURL, errE := site.authenticator.SignIn(ctx, req.Form.Get(signInRedirectQueryParam))
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.TemporaryRedirectGetMethod(w, req, authURL)
}

// AuthCallbackGet receives the issuer's callback at the end of the sign-in flow.
// It hands the callback query string off to the per-site Authenticator which returns
// the access token plus the post-sign-in redirect path. The handler then sets the
// access-token cookie and redirects the user to that path.
func (s *Service) AuthCallbackGet(w http.ResponseWriter, req *http.Request, _ waf.Params) {
	defer req.Body.Close()              //nolint:errcheck
	defer io.Copy(io.Discard, req.Body) //nolint:errcheck

	// no-store: the URL potentially carries a one-time code or a token in its query string
	// and the response sets the session cookie. Caching any part of it would be a credential leak.
	w.Header().Set("Cache-Control", "no-store")

	ctx := req.Context()
	site := waf.MustGetSite[*Site](ctx)

	accessToken, expiry, redirect, errE := site.authenticator.Callback(ctx, req.Form)
	if errE != nil {
		if errors.Is(errE, auth.ErrSignInFailed) {
			s.BadRequestWithError(w, req, errE)
			return
		}
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	auth.SetSessionCookie(w, accessToken, expiry)
	s.TemporaryRedirectGetMethod(w, req, redirect)
}

// AuthSignOutPostAPI clears the access-token cookie.
func (s *Service) AuthSignOutPostAPI(w http.ResponseWriter, req *http.Request, _ waf.Params) {
	defer req.Body.Close()              //nolint:errcheck
	defer io.Copy(io.Discard, req.Body) //nolint:errcheck

	// no-store: the response clears the session cookie via Set-Cookie.
	// Caching it would let an unrelated request later replay the
	// invalidation (or, with stale caching, mask it).
	w.Header().Set("Cache-Control", "no-store")

	ctx := req.Context()
	site := waf.MustGetSite[*Site](ctx)

	errE := site.authenticator.SignOut(w, req)
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	auth.ClearSessionCookie(w)
	s.WriteJSON(w, req, []byte(`{"success":true}`), nil)
}
