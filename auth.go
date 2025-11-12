package peerdb

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"

	"gitlab.com/tozd/waf"
)

// HasherSHA256 computes the SHA256 hash of a string for easier credential comparison.
func HasherSHA256(s string) []byte {
	val := sha256.Sum256([]byte(s))
	return val[:]
}

func BasicAuthHandler(usernameHash []byte, passwordHash []byte, realm string) func(http.Handler) http.Handler {
	return func(handler http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			site, ok := waf.GetSite[*Site](req.Context())
			if ok && site.Title != "" {
				realm = site.Title
			}

			user, pass, ok := req.BasicAuth()
			if !ok ||
				subtle.ConstantTimeCompare(HasherSHA256(user), usernameHash) != 1 ||
				subtle.ConstantTimeCompare(HasherSHA256(pass), passwordHash) != 1 {
				w.Header().Set("WWW-Authenticate", `Basic realm="`+realm+`"`)
				http.Error(w, "Unauthorized.", http.StatusUnauthorized)
				return
			}
			handler.ServeHTTP(w, req)
		})
	}
}
