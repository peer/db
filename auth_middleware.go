package peerdb

import (
	"net/http"

	"gitlab.com/tozd/waf"
)

// oidcAuthMiddleware is the global middleware that looks up the per-site
// auth.Verifier from the request context (one Verifier per site, because
// each site has its own OIDC client and redirect URI) and lets it
// validate the request's access token. On success it attaches
// auth.Subject / auth.Roles to ctx and writes the Roles / UserInfo
// response headers.
//
// We register this once on the Service and dispatch per-site internally
// rather than registering one middleware per site, because WAF's
// middleware stack is service-wide and the site is only resolved once the
// validateSite middleware downstream has matched the Host header.
//
//nolint:contextcheck
func (s *Service) oidcAuthMiddleware(metadataHeaderPrefix string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			waf.SetCanonicalLogMessage(req.Context(), "OIDCAuth")

			ctx := req.Context()
			site, ok := waf.GetSite[*Site](ctx)
			if ok && site.verifier != nil {
				ctx = site.verifier.Authenticate(w, req, metadataHeaderPrefix, site.Roles)
				next.ServeHTTP(w, req.WithContext(ctx))
			} else {
				next.ServeHTTP(w, req)
			}
		})
	}
}
