package auth

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"

	"gitlab.com/tozd/waf"
)

// hasherSHA256 computes the SHA256 hash of a string for constant-time
// credential comparison.
func hasherSHA256(s string) []byte {
	val := sha256.Sum256([]byte(s))
	return val[:]
}

// isCORSPreflight reports whether the request is a CORS preflight: the OPTIONS request a browser makes
// on its own before a cross-origin request it may not make unasked, stating the method, and any
// non-simple headers, of the request it is asking about. The Fetch standard has a browser send all of
// these, so an OPTIONS request without them is a request of its own and not a preflight of another.
func isCORSPreflight(req *http.Request) bool {
	return req.Method == http.MethodOptions &&
		req.Header.Get("Origin") != "" &&
		req.Header.Get("Access-Control-Request-Method") != ""
}

// BasicAuthMiddleware returns a middleware that gates requests with HTTP Basic
// auth. Every request must satisfy the basic-auth challenge, whether or not the
// caller also presents OIDC credentials, except a CORS preflight, which is let
// through (see isCORSPreflight). A route which answers OPTIONS itself, instead
// of leaving it to the CORS handling, therefore has to answer it with nothing a
// caller without the credentials may not see.
//
// realm is a callback that returns the WWW-Authenticate realm string for a
// given request (typically the per-site title) and is invoked only on
// the failure path.
//
// We declare Vary: Authorization on every response (cached responses must
// key on the Authorization header because the basic-auth check reads it)
// and Cache-Control: private on successful gates so shared caches do not
// store the protected content.
func BasicAuthMiddleware(username, password string, realm func(req *http.Request) string) func(http.Handler) http.Handler {
	usernameHash := hasherSHA256(username)
	passwordHash := hasherSHA256(password)
	return func(handler http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			// A browser makes the preflight with the credentials omitted (the Fetch standard has it so), so
			// a preflight cannot carry what this gate asks for. Gating it gates nothing: a preflight carries
			// no data, and what answers it says only which origins, methods and headers a route allows. What
			// it costs is every cross-origin request a browser makes, because the preflight failing is the
			// request it was about never being made, whatever the route allows. The request itself passes
			// here like any other, so it still has to carry the credentials. The middleware is left out of
			// the way entirely, so that what answers the preflight decides how the answer is cached and
			// logged.
			if isCORSPreflight(req) {
				handler.ServeHTTP(w, req)
				return
			}

			waf.SetCanonicalLogMessage(req.Context(), "BasicAuth")

			addVary(w, "Authorization")

			user, pass, ok := req.BasicAuth()
			userCompare := subtle.ConstantTimeCompare(hasherSHA256(user), usernameHash)
			passwordCompare := subtle.ConstantTimeCompare(hasherSHA256(pass), passwordHash)
			if !ok || userCompare+passwordCompare != 2 {
				w.Header().Set("WWW-Authenticate", `Basic realm="`+realm(req)+`"`)
				waf.Error(w, req, http.StatusUnauthorized)
				return
			}
			w.Header().Set("Cache-Control", "private")
			handler.ServeHTTP(w, req)
		})
	}
}
