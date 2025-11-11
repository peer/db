package peerdb

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"

	"gitlab.com/tozd/waf"
)

func hasher(s string) []byte {
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
				subtle.ConstantTimeCompare(hasher(user), usernameHash) != 1 ||
				subtle.ConstantTimeCompare(hasher(pass), passwordHash) != 1 {
				w.Header().Set("WWW-Authenticate", `Basic realm="`+realm+`"`)
				http.Error(w, "Unauthorized.", http.StatusUnauthorized)
				return
			}
			handler.ServeHTTP(w, r)
		})
	}
}
