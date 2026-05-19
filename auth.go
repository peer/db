package peerdb

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"

	"gitlab.com/tozd/waf"

	"gitlab.com/peerdb/peerdb/auth"
)

// hasherSHA256 computes the SHA256 hash of a string for constant-time credential comparison.
func hasherSHA256(s string) []byte {
	val := sha256.Sum256([]byte(s))
	return val[:]
}

// basicAuthHandler returns a middleware that gates requests with HTTP Basic auth.
//
// When skipBearer is true, requests carrying an Authorization header with the
// Bearer scheme bypass the basic-auth check and are passed to the next handler,
// where the OIDC verifier is expected to handle them.
func basicAuthHandler(username string, password string, skipBearer bool) func(http.Handler) http.Handler {
	usernameHash := hasherSHA256(username)
	passwordHash := hasherSHA256(password)
	return func(handler http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			waf.SetCanonicalLogMessage(req.Context(), "BasicAuth")

			if skipBearer && auth.HasBearerToken(req) {
				handler.ServeHTTP(w, req)
				return
			}

			site := waf.MustGetSite[*Site](req.Context())

			user, pass, ok := req.BasicAuth()
			userCompare := subtle.ConstantTimeCompare(hasherSHA256(user), usernameHash)
			passwordCompare := subtle.ConstantTimeCompare(hasherSHA256(pass), passwordHash)
			if !ok || userCompare+passwordCompare != 2 {
				w.Header().Set("WWW-Authenticate", `Basic realm="`+site.Title+`"`)
				waf.Error(w, req, http.StatusUnauthorized)
				return
			}
			handler.ServeHTTP(w, req)
		})
	}
}
