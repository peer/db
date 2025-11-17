package peerdb

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"

	"gitlab.com/tozd/waf"
)

// HasherSHA256 computes the SHA256 hash of a string for constant-time credential comparison.
func HasherSHA256(s string) []byte {
	val := sha256.Sum256([]byte(s))
	return val[:]
}

func BasicAuthHandler(username, password, realm string) func(http.Handler) http.Handler {
	usernameHash := peerdb.HasherSHA256(testUsername)
	passwordHash := peerdb.HasherSHA256(testPassword)
	return func(handler http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			site, ok := waf.GetSite[*Site](req.Context())
			if ok && site.Title != "" {
				realm = site.Title
			}

			user, pass, ok := req.BasicAuth()
			userCompare := subtle.ConstantTimeCompare(HasherSHA256(user), usernameHash)
			passwordCompare := subtle.ConstantTimeCompare(HasherSHA256(pass), passwordHash)
			if !ok || userCompare + passwordCompare != 2 {
				w.Header().Set("WWW-Authenticate", `Basic realm="`+realm+`"`)
				waf.Error(w, req, http.StatusUnauthorized)
				return
			}
			handler.ServeHTTP(w, req)
		})
	}
}
