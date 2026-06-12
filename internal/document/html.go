package document

// IsHTMLWhitespace reports whether r is one of the ASCII whitespace characters
// the HTML spec treats as collapsible whitespace: SPACE, TAB, LF, FF, CR.
// Notably this excludes vertical tab (U+000B) and all non-ASCII whitespace
// (NBSP, U+2028, U+2029, ...), those are regular characters per the spec
// and must not be trimmed or collapsed.
func IsHTMLWhitespace(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\r', '\f':
		return true
	}
	return false
}
