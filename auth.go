package peerdb

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"

	"gitlab.com/tozd/waf"
)

// hasherSHA256 computes the SHA256 hash of a string for constant-time credential comparison.
func hasherSHA256(s string) []byte {
	val := sha256.Sum256([]byte(s))
	return val[:]
}

func basicAuthHandler(username string, password string) func(http.Handler) http.Handler {
	usernameHash := hasherSHA256(username)
	passwordHash := hasherSHA256(password)
	return func(handler http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
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
