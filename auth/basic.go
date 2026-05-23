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

// BasicAuthMiddleware returns a middleware that gates requests with HTTP Basic
// auth. It is unconditional: when configured, every request must satisfy
// the basic-auth challenge regardless of whether the caller also presents
// OIDC credentials.
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
