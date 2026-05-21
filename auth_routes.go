package peerdb

import (
	"io"
	"net/http"
	"net/url"
	"strings"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"
	"gitlab.com/tozd/waf"
	"golang.org/x/oauth2"

	"gitlab.com/peerdb/peerdb/auth"
)

// signInRedirectQueryParam is the name of the query parameter clients use to
// request a specific post-sign-in landing page (eg ?redirect=/d/abc).
const signInRedirectQueryParam = "redirect"

// AuthSignInGet starts the OIDC authorization-code flow. It generates a
// fresh state / nonce / PKCE verifier, persists them in the per-site
// FlowStore, and 303-redirects the user to the issuer's authorize endpoint.
//
// The optional ?redirect=<path> query parameter records where to send the
// user after the callback completes. Only same-site paths (leading "/")
// are accepted to prevent open-redirect abuse.
func (s *Service) AuthSignInGet(w http.ResponseWriter, req *http.Request, _ waf.Params) {
	defer req.Body.Close()              //nolint:errcheck
	defer io.Copy(io.Discard, req.Body) //nolint:errcheck

	// no-store: this URL is a side-effect entry point (creates a flow row,
	// redirects to the issuer); nothing about its response is safe to keep
	// in any cache.
	w.Header().Set("Cache-Control", "no-store")

	ctx := req.Context()
	site := waf.MustGetSite[*Site](ctx)

	if !site.AuthEnabled || site.verifier == nil || site.flowStore == nil {
		// Sign-in disabled for this site: 404 looks the same to clients as
		// "this route does not exist", which is what we want.
		s.NotFound(w, req)
		return
	}

	redirect := safeRedirectPath(req.Form.Get(signInRedirectQueryParam))

	state := identifier.New().String()
	verifier := oauth2.GenerateVerifier()
	nonce := identifier.New().String()

	errE := site.flowStore.BeginFlow(ctx, state, auth.FlowState{
		CodeVerifier: verifier,
		Nonce:        nonce,
		Redirect:     redirect,
	})
	if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	authURL := site.verifier.AuthCodeURL(state, verifier, nonce)
	s.TemporaryRedirectGetMethod(w, req, authURL)
}

// AuthCallbackGet receives the issuer's redirect at the end of the
// authorization-code flow. It consumes the matching FlowStore row, exchanges
// the code for tokens (with PKCE verifier), sets the access-token cookie,
// and redirects to the post-sign-in landing page recorded at signin time.
func (s *Service) AuthCallbackGet(w http.ResponseWriter, req *http.Request, _ waf.Params) {
	defer req.Body.Close()              //nolint:errcheck
	defer io.Copy(io.Discard, req.Body) //nolint:errcheck

	// no-store: the URL carries the one-time authorization code in its
	// query string and the response sets the session cookie. Caching any
	// part of it would be a credential leak.
	w.Header().Set("Cache-Control", "no-store")

	ctx := req.Context()
	site := waf.MustGetSite[*Site](ctx)

	if !site.AuthEnabled || site.verifier == nil || site.flowStore == nil {
		s.NotFound(w, req)
		return
	}

	// If the issuer signals an error, surface it as a 400 rather than
	// pretending the flow succeeded. The "error" parameter is OIDC-standard.
	if issuerErr := req.Form.Get("error"); issuerErr != "" {
		errE := errors.New("issuer rejected the sign-in")
		errors.Details(errE)["error"] = issuerErr
		if desc := req.Form.Get("error_description"); desc != "" {
			errors.Details(errE)["description"] = desc
		}
		s.BadRequestWithError(w, req, errE)
		return
	}

	state := req.Form.Get("state")
	code := req.Form.Get("code")
	if state == "" || code == "" {
		s.BadRequestWithError(w, req, errors.New(`missing "state" or "code" in OIDC callback`))
		return
	}

	flow, errE := site.flowStore.ConsumeFlow(ctx, state)
	if errE != nil {
		if errors.Is(errE, auth.ErrFlowNotFound) {
			// Single-use, expired, or never existed: the user almost
			// certainly hit "back" / refreshed; report as a regular 400
			// so the browser does not retry.
			s.BadRequestWithError(w, req, errE)
			return
		}
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	accessToken, expiry, errE := site.verifier.ExchangeCode(ctx, code, flow.CodeVerifier, flow.Nonce)
	if errE != nil {
		s.BadRequestWithError(w, req, errE)
		return
	}

	auth.SetSessionCookie(w, accessToken, expiry)
	s.TemporaryRedirectGetMethod(w, req, flow.Redirect)
}

// AuthSignOutPostAPI clears the access-token cookie. It does not call the
// issuer's end_session_endpoint - RP-initiated sign-out is a v2 follow-up
// noted in the plan.
func (s *Service) AuthSignOutPostAPI(w http.ResponseWriter, req *http.Request, _ waf.Params) {
	defer req.Body.Close()              //nolint:errcheck
	defer io.Copy(io.Discard, req.Body) //nolint:errcheck

	// no-store: the response clears the session cookie via Set-Cookie;
	// caching it would let an unrelated request later replay the
	// invalidation (or, with stale caching, mask it).
	w.Header().Set("Cache-Control", "no-store")

	site := waf.MustGetSite[*Site](req.Context())
	if !site.AuthEnabled || site.verifier == nil {
		s.NotFound(w, req)
		return
	}

	auth.ClearSessionCookie(w)
	// A JSON body lets postJSON on the frontend stay happy (it rejects
	// non-JSON responses); the {"success":true} shape mirrors the other
	// API endpoints that have no real payload (eg. /d/discardEdit).
	s.WriteJSON(w, req, []byte(`{"success":true}`), nil)
}

// safeRedirectPath validates the caller-supplied post-sign-in landing path.
// Only relative same-site paths are accepted: anything starting with a
// scheme, a "//" authority, or empty falls back to "/" so a hostile
// signin URL cannot bounce the user off-site after the callback.
func safeRedirectPath(raw string) string {
	if raw == "" {
		return "/"
	}
	if !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") {
		return "/"
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "/"
	}
	if u.Scheme != "" || u.Host != "" {
		return "/"
	}
	// Re-stringify so any URL-decoded curiosities (escaped slashes, etc.)
	// land back in canonical form.
	return u.String()
}
