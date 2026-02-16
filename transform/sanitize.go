package transform

import "github.com/microcosm-cc/bluemonday"

// TODO: We should also use Cross-Origin-Opener-Policy response headers and CSP headers.

//nolint:gochecknoglobals
var sanitizer *bluemonday.Policy

//nolint:gochecknoinits
func init() {
	sanitizer = bluemonday.NewPolicy()
	sanitizer.RequireParseableURLs(true)
	// It adds rel="noreferrer" (which implies "noopener", too) without target="_blank" to all external
	// links, which currently means that rel does not really have an effect, but we do not want to have
	// target="_blank" because we want to allow users to decide if they want to open links in a new tab
	// or not. Hopefully it will have an effect in the future.
	// See: https://github.com/whatwg/html/issues/5134
	sanitizer.RequireNoReferrerOnFullyQualifiedLinks(true)
	sanitizer.AllowURLSchemes("mailto", "http", "https")
	// TODO: Renumber headings.
	//       See: https://github.com/microcosm-cc/bluemonday/issues/222
	sanitizer.AllowElements("h1", "h2", "h3", "h4", "h5", "h6")
	sanitizer.AllowElements("br", "hr", "p")
	sanitizer.AllowAttrs("href").OnElements("a")
	sanitizer.AllowElements("b", "i", "pre", "strike", "tt", "u")
	sanitizer.AllowAttrs("cite").OnElements("blockquote")
	sanitizer.AllowAttrs("alt", "src").OnElements("img")
	// TODO: Require that li is under ul or ol.
	sanitizer.AllowElements("ul", "ol", "li")
}

func sanitizeHTML(input string) string {
	// TODO: Make so that <p> is always closed with </p>, same for <blockquote>.
	//       So all tags which can be closed should be closed (or self closed).
	return sanitizer.Sanitize(input)
}
