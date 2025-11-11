package peerdb

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"

	"gitlab.com/tozd/waf"
)

// hasherSHA256 computes the SHA256 hash of a string for credential comparison.
func hasherSHA256(s string) []byte {
	val := sha256.Sum256([]byte(s))
	return val[:]
}

func basicAuthHandler(usernameHash []byte, passwordHash []byte, realm string) func(http.Handler) http.Handler {
	return func(handler http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			site, ok := waf.GetSite[*Site](r.Context())
			if ok && site.Title != "" {
				realm = site.Title
			}

			user, pass, ok := r.BasicAuth()
			if !ok ||
				subtle.ConstantTimeCompare(hasherSHA256(user), usernameHash) != 1 ||
				subtle.ConstantTimeCompare(hasherSHA256(pass), passwordHash) != 1 {
				w.Header().Set("WWW-Authenticate", `Basic realm="`+realm+`"`)
				http.Error(w, "Unauthorized.", http.StatusUnauthorized)
				return
			}
			handler.ServeHTTP(w, r)
		})
	}
}
