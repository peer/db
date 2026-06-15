package document

import (
	"net/url"
	"strings"

	"gitlab.com/tozd/go/errors"
)

// TODO: We should also use Cross-Origin-Opener-Policy response headers and CSP headers to ensure that URLs/links cannot be misused.

// validateURL returns nil if value is acceptable as a link target. Allowed forms:
//   - Same-origin path starting with "/" but not "//" (e.g. "/foo", "/a?b=c#d", "/").
//   - Absolute URL whose scheme is http, https, or (only when allowContact) the contact schemes mailto and tel.
//
// Rejected (with a descriptive error): empty input, unparseable input, protocol-relative URLs
// ("//host/path"), document-relative paths ("foo", "../foo"), fragment-only refs ("#section"), an
// http/https URL with no host, an empty mailto/tel, and any other scheme (javascript:, data:, ftp:, ...).
//
// This is the single URL validation used everywhere: LinkClaim IRIs and the editor schema's <a href>
// (linkURL) pass allowContact true; <blockquote cite> (resourceURL) passes allowContact false. It
// mirrors parseUrl in src/utils.ts on the frontend; the two are kept equivalent by parallel test
// corpora. All URLs go through this same parsing and classification, ignoring the parsed value when
// only validity matters.
func validateURL(value string, allowContact bool) errors.E {
	if value == "" {
		return errors.New("empty URL")
	}
	if strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "//") {
		u, err := url.Parse(value)
		if err != nil {
			return errors.WithMessage(err, "invalid URL")
		}
		if u.Scheme != "" || u.Host != "" {
			return errors.New("invalid URL")
		}
		return nil
	}
	u, err := url.Parse(value)
	if err != nil {
		return errors.WithMessage(err, "invalid URL")
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		// url.Parse accepts degenerate forms like "http:///path" with no host; reject them.
		if u.Host == "" {
			return errors.New("invalid URL: missing host")
		}
	case "mailto":
		if !allowContact {
			errE := errors.New("disallowed URL scheme")
			errors.Details(errE)["scheme"] = u.Scheme
			return errE
		}
		// url.Parse accepts "mailto:" with no address (empty Opaque); reject it.
		if u.Opaque == "" {
			return errors.New("invalid URL: missing address")
		}
	case "tel":
		if !allowContact {
			errE := errors.New("disallowed URL scheme")
			errors.Details(errE)["scheme"] = u.Scheme
			return errE
		}
		// url.Parse accepts "tel:" with no number (empty Opaque); reject it.
		if u.Opaque == "" {
			return errors.New("invalid URL: missing number")
		}
	case "":
		return errors.New("invalid URL")
	default:
		errE := errors.New("disallowed URL scheme")
		errors.Details(errE)["scheme"] = u.Scheme
		return errE
	}
	return nil
}
