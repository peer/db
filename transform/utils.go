package transform

import (
	"fmt"
	"html"
	"regexp"
	"strings"
	"unicode"

	internalDocument "gitlab.com/peerdb/peerdb/internal/document"
)

//nolint:gochecknoglobals
var (
	urlEmailRegex     = regexp.MustCompile(`(?i)(?:https?://(?:[^\s&]|&amp;)+)|(?:\bwww\.(?:[^\s&]|&amp;)+)|(?:\b[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}\b)`)
	trailingCharRegex *regexp.Regexp
)

// Only ASCII characters we want to remove.
// It includes all characters html.EscapeString escapes, except for &.
const trailingChars = `,.!?;:()[]{}<>|"'`

func init() { //nolint:gochecknoinits
	r := []string{} //nolint:prealloc
	// Adding trailingChars is mostly redundant because they are already matched by the \p{P} group,
	// but we want to make sure we can extend trailingChars even with characters which are not in \p{P}
	// and also we do not want to rely on which characters does html.EscapeString escape.
	for _, char := range trailingChars {
		// Some characters in trailingChars (like <, >, and ") are HTML escaped
		// before we try to match them. So we need to escape them here, too.
		// Adding HTML escaped characters is also redundant because urlEmailRegex does not really match
		// links with HTML entities, except for &amp;, but & is not in trailingChars.
		r = append(r, `(?:`+regexp.QuoteMeta(html.EscapeString(string(char)))+`)`)
	}
	// We append it at the end so that it matches last.
	r = append(r, `\p{P}`)
	trailingCharRegex = regexp.MustCompile(`(?:` + strings.Join(r, "|") + `)$`)
	trailingCharRegex.Longest()
}

func trimTrailing(input string) (string, string) {
	tailing := ""
	for {
		loc := trailingCharRegex.FindStringIndex(input)
		if loc == nil {
			// No more trailing characters we want to trim.
			break
		}
		match := input[loc[0]:loc[1]]
		// We do not check HTML escaped matches (they are multiple bytes).
		// Nor we check multi-byte unicode punctuation characters. We trim them all.
		if len(match) == 1 {
			r := rune(match[0])
			// Our regex matches also ASCII punctuation which we do not want to remove.
			// We want to remove only ASCII characters from trailingChars.
			if r <= unicode.MaxASCII && !strings.ContainsRune(trailingChars, r) {
				// We do not want to remove this ASCII punctuation. This also stops the trim.
				break
			}
		}
		input = input[:loc[0]]
		tailing = match + tailing
	}
	return input, tailing
}

// linkify replaces email addresses and URLs with HTML links.
func linkify(input string) string {
	return urlEmailRegex.ReplaceAllStringFunc(input, func(match string) string {
		link, trailing := trimTrailing(match)
		linkLower := strings.ToLower(link)
		if strings.HasPrefix(linkLower, "https://") || strings.HasPrefix(linkLower, "http://") {
			return fmt.Sprintf(`<a href="%s">%s</a>%s`, link, link, trailing)
		} else if strings.HasPrefix(linkLower, "www.") {
			return fmt.Sprintf(`<a href="http://%s">%s</a>%s`, link, link, trailing)
		}
		return fmt.Sprintf(`<a href="mailto:%s">%s</a>%s`, link, link, trailing)
	})
}

// TextToHTML converts plain text into HTML: it escapes HTML, linkifies URLs and email
// addresses, collapses whitespace, converts newlines to br tags, and wraps the result
// into a paragraph block. Whitespace-only input returns an empty string.
//
// The output is in the canonical form the frontend editor serializer produces: runs of
// collapsible whitespace become a single space and lines have no leading or trailing
// whitespace, mirroring how the editor parses HTML, so parsing the output into the
// editor and serializing it back is the identity.
//
// It does not sanitize HTML.
func TextToHTML(input string) string {
	if strings.TrimSpace(input) == "" {
		return ""
	}

	// Normalize newlines. Respect Windows and Mac newlines.
	input = strings.ReplaceAll(input, "\r\n", "\n")
	input = strings.ReplaceAll(input, "\r", "\n")

	lines := strings.Split(input, "\n")
	for i, line := range lines {
		// Collapses runs of HTML-collapsible whitespace into a single space and trims
		// the line. U+00A0 and other Unicode spaces are not collapsed, matching HTML
		// rendering.
		lines[i] = strings.Join(strings.FieldsFunc(line, internalDocument.IsHTMLWhitespace), " ")
	}

	result := html.EscapeString(strings.Join(lines, "\n"))

	result = linkify(result)

	return "<p>" + strings.ReplaceAll(result, "\n", "<br>") + "</p>"
}
