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

// SiteAuthConfig contains per-site configuration for OIDC-based authentication.
// Each site has its own client because the redirect URI is per-domain and
// most OIDC providers register redirect URIs as fixed strings rather than
// templates.
type SiteAuthConfig = internalSite.SiteAuthConfig

// VisibilityLevel is one entry in a site's ordered list of visibility levels.
// Each level has a unique, non-empty name and the roles (possibly none) that
// grant it.
type VisibilityLevel = internalSite.VisibilityLevel
