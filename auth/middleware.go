package auth

import (
	"net/http"

	"gitlab.com/tozd/waf"
)

// Middleware returns an HTTP middleware that validates the request's
// access token via the per-request Authenticator.
//
// metadataHeaderPrefix is the WAF service's MetadataHeaderPrefix. It is
// prepended to the Roles/UserInfo header names the Authenticator
// writes.
//
// Register this once on the WAF Service. The caller's lookup is
// responsible for dispatching to the right per-site Authenticator.
//
//nolint:contextcheck
func Middleware(
	metadataHeaderPrefix string,
	lookup func(w http.ResponseWriter, req *http.Request) (a Authenticator, allowedRoles map[string][]string, visibility []VisibilityLevel, handled bool),
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := req.Context()
			waf.SetCanonicalLogMessage(ctx, "Auth")

			a, allowedRoles, visibility, handled := lookup(w, req)
			if handled {
				return
			}
			ctx = a.Authenticate(w, req, metadataHeaderPrefix, allowedRoles, visibility)
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	}
}
