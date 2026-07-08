package peerdb

import (
	internalSite "gitlab.com/peerdb/peerdb/internal/site"
)

// Type aliases for types defined in internal/site.

// Site represents a single site in the PeerDB application with its configuration and state.
type Site = internalSite.Site

// Build contains version and build metadata.
type Build = internalSite.Build

// SiteFeatures contains enabled feature flags.
type SiteFeatures = internalSite.SiteFeatures

// Favicon configures the site favicon rendered into the page head. When Href is set, a
// <link rel="icon"> pointing at it is emitted in index.html. Type is the optional value of
// the link's type attribute (the icon's MIME type, e.g. "image/svg+xml"); it is omitted when empty.
type Favicon = internalSite.Favicon

// SiteAuthConfig contains per-site configuration for OIDC-based authentication.
// Each site has its own client because the redirect URI is per-domain and
// most OIDC providers register redirect URIs as fixed strings rather than
// templates.
type SiteAuthConfig = internalSite.SiteAuthConfig
