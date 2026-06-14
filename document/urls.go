package document

import (
	"net/url"
	"regexp"
	"strings"

	"gitlab.com/tozd/go/errors"
)

// TODO: We should also use Cross-Origin-Opener-Policy response headers and CSP headers to ensure that URLs/links cannot be misused.

// linkHrefPattern matches values acceptable in <a href>: leading-slash
// same-origin paths ("/foo" but not "//host/foo") and absolute URLs in the
// http/https/mailto allowlist. It is the documented semantics of the linkURL
// validator in the shared schema dialect (the schema's link mark validates
// href against it), and mirrors parseUrl in src/utils.ts and validateIRI. The
// case-insensitive flag covers uppercase scheme variants like "HTTPS://"; URL
// paths themselves stay case-sensitive (the regex does not constrain them past
// the prefix). After each scheme prefix the next character must be a non-slash
// so degenerate URLs like "http:///x" (which url.Parse otherwise accepts) are
// rejected.
var linkHrefPattern = regexp.MustCompile(`(?i)^(?:/(?:[^/]|$)|https?://[^/]|mailto:[^/])`)

// resourceURLPattern matches values acceptable in <blockquote cite>. It is the
// documented semantics of the resourceURL validator in the shared schema
// dialect. The set is the same as linkHrefPattern minus mailto. The frontend
// makes the same distinction: InputHTML.vue passes allow-mailto=false to
// InputLink when editing a blockquote cite.
var resourceURLPattern = regexp.MustCompile(`(?i)^(?:/(?:[^/]|$)|https?://[^/])`)

// allowedLinkClaimSchemes is the set of URI schemes accepted for LinkClaim
// IRIs. Mirrors ALLOWED_LINK_CLAIM_SCHEMES in src/utils.ts on the frontend.
// HTML link validation uses the same set for <a href> and, in sync with the
// frontend, the set minus mailto for <blockquote cite> (resourceURLPattern).
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
