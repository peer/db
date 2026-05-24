package auth

import (
	"net/http"
	"time"
)

// accessTokenCookieName is the name of the HTTP-only cookie carrying the OIDC
// access token after a server-side sign-in.
//
// The __Host- prefix is a cookie-prefix browsers enforce: the cookie must
// be Secure, must omit Domain (so it is bound to the exact host that set
// it), and must have Path=/. SetSessionCookie meets all three. The prefix
// gives us subdomain isolation (a sibling host cannot shadow this cookie)
// and turns any future weakening of those attributes into an immediate
// browser-side rejection rather than a silent regression.
const accessTokenCookieName = "__Host-session" //nolint:gosec

// SetSessionCookie writes the access-token cookie with the given token
// value, scoped to the whole site (Path=/) and configured so that
// JavaScript cannot read it (HttpOnly) and modern browsers only attach it
// under HTTPS / same-site contexts (Secure, SameSite=Lax).
//
// expiresAt should equal the token's exp claim; when it has already
// passed MaxAge becomes 0, which deletes the cookie immediately.
func SetSessionCookie(w http.ResponseWriter, token string, expiresAt time.Time) {
	maxAge := max(int(time.Until(expiresAt).Seconds()), 0)
	http.SetCookie(w, &http.Cookie{ //nolint:exhaustruct
		Name:     accessTokenCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearSessionCookie deletes the access-token cookie. The browser drops
// the entry as soon as it observes Max-Age=-1.
func ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{ //nolint:exhaustruct
		Name:     accessTokenCookieName,
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}
