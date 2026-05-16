package document

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/microcosm-cc/bluemonday"
	"gitlab.com/tozd/go/errors"
)

// TODO: We should also use Cross-Origin-Opener-Policy response headers and CSP headers.

// linkHrefPattern matches values acceptable in <a href>: leading-slash
// same-origin paths ("/foo" but not "//host/foo") and absolute URLs in the
// http/https/mailto allowlist. Mirrors parseUrl in src/utils.ts and
// validateIRI. The case-insensitive flag covers uppercase scheme variants
// like "HTTPS://"; URL paths themselves stay case-sensitive (the regex
// does not constrain them past the prefix). After each scheme prefix the
// next character must be a non-slash so degenerate URLs like "http:///x"
// (which url.Parse otherwise accepts) are rejected.
var linkHrefPattern = regexp.MustCompile(`(?i)^(?:/(?:[^/]|$)|https?://[^/]|mailto:[^/])`)

// resourceURLPattern matches values acceptable in attributes that name a
// fetched/embedded resource: <img src> and <blockquote cite>. The set is
// the same as linkHrefPattern minus mailto: pointing an image source or a
// quote-of-origin at an email address makes no sense.
var resourceURLPattern = regexp.MustCompile(`(?i)^(?:/(?:[^/]|$)|https?://[^/])`)

//nolint:gochecknoglobals
var sanitizer *bluemonday.Policy

//nolint:gochecknoinits
func init() {
	sanitizer = bluemonday.NewPolicy()
	sanitizer.RequireParseableURLs(true)
	// AllowRelativeURLs lets bluemonday accept URLs without a scheme; the
	// per-attribute Matching regexes below then narrow that to leading-slash
	// paths only.
	sanitizer.AllowRelativeURLs(true)
	sanitizer.AllowURLSchemes("mailto", "http", "https")
	// TODO: Renumber headings.
	//       See: https://github.com/microcosm-cc/bluemonday/issues/222
	sanitizer.AllowElements("h1", "h2", "h3", "h4", "h5", "h6")
	sanitizer.AllowElements("br", "hr", "p")
	sanitizer.AllowAttrs("href").Matching(linkHrefPattern).OnElements("a")
	sanitizer.AllowElements("b", "i", "pre", "strike", "tt", "u")
	sanitizer.AllowAttrs("cite").Matching(resourceURLPattern).OnElements("blockquote")
	sanitizer.AllowAttrs("alt").OnElements("img")
	sanitizer.AllowAttrs("src").Matching(resourceURLPattern).OnElements("img")
	// TODO: Require that li is under ul or ol.
	sanitizer.AllowElements("ul", "ol", "li")
}

// SanitizeHTML strips disallowed elements, attributes, and URL forms from
// input and returns the canonicalized safe HTML. SanitizeHTML is idempotent
// on already-canonical input.
func SanitizeHTML(input string) string {
	// TODO: Make so that <p> is always closed with </p>, same for <blockquote>.
	//       So all tags which can be closed should be closed (or self closed).
	return sanitizer.Sanitize(input)
}

// allowedLinkClaimSchemes is the set of URI schemes accepted for LinkClaim
// IRIs. Mirrors ALLOWED_LINK_SCHEMES in src/utils.ts on the frontend. The
// HTML sanitizer uses a narrower set for <img src> and <blockquote cite>
// (no mailto) and the same set for <a href>.
//
//nolint:gochecknoglobals
var allowedLinkClaimSchemes = map[string]bool{
	"http":   true,
	"https":  true,
	"mailto": true,
}

// validateIRI returns nil if the IRI is acceptable as a LinkClaim target.
// Allowed forms:
//   - Same-origin path starting with "/" but not "//" (e.g. "/foo", "/a?b=c#d", "/").
//   - Absolute URL whose scheme is in allowedLinkClaimSchemes.
//
// Rejected (with a descriptive error): empty input, unparseable input,
// protocol-relative URLs ("//host/path"), document-relative paths ("foo",
// "../foo"), fragment-only refs ("#section"), and any other scheme
// (javascript:, data:, tel:, ftp:, ...). Matches parseUrl on the frontend.
func validateIRI(iri string) errors.E {
	if iri == "" {
		return errors.New("empty IRI")
	}
	if strings.HasPrefix(iri, "/") && !strings.HasPrefix(iri, "//") {
		u, err := url.Parse(iri)
		if err != nil {
			return errors.WithMessage(err, "invalid IRI")
		}
		if u.Scheme != "" || u.Host != "" {
			return errors.New("invalid IRI")
		}
		return nil
	}
	u, err := url.Parse(iri)
	if err != nil {
		return errors.WithMessage(err, "invalid IRI")
	}
	if u.Scheme == "" {
		return errors.New("invalid IRI")
	}
	if !allowedLinkClaimSchemes[strings.ToLower(u.Scheme)] {
		return errors.Errorf("disallowed IRI scheme: %s", u.Scheme)
	}
	// Reject degenerate forms like "http:///path" (no host) and "mailto:".
	// For http/https we require a non-empty Host; for mailto we require a
	// non-empty Opaque (the part after "mailto:").
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		if u.Host == "" {
			return errors.New("invalid IRI: missing host")
		}
	case "mailto":
		if u.Opaque == "" {
			return errors.New("invalid IRI: missing address")
		}
	}
	return nil
}
