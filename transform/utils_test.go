package transform_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"gitlab.com/peerdb/peerdb/transform"
)

func TestTextToHTML(t *testing.T) { //nolint:maintidx
	t.Parallel()

	//nolint:lll
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "PlainText",
			input:    "Hello, World!",
			expected: "<p>Hello, World!</p>",
		},
		{
			name:     "SpecialCharacters",
			input:    "<script>alert('xss')</script>",
			expected: "<p>&lt;script&gt;alert(&#39;xss&#39;)&lt;/script&gt;</p>",
		},
		{
			name:     "Ampersand",
			input:    "Tom & Jerry",
			expected: "<p>Tom &amp; Jerry</p>",
		},
		{
			name:     "Quotes",
			input:    `He said "Hello"`,
			expected: `<p>He said &#34;Hello&#34;</p>`,
		},
		{
			name:     "SingleURL",
			input:    "Visit https://example.com for info",
			expected: `<p>Visit <a href="https://example.com">https://example.com</a> for info</p>`,
		},
		{
			name:     "HTTPandHTTPS",
			input:    "HTTP: http://example.com and HTTPS: https://secure.example.com",
			expected: `<p>HTTP: <a href="http://example.com">http://example.com</a> and HTTPS: <a href="https://secure.example.com">https://secure.example.com</a></p>`,
		},
		{
			name:     "MultipleURLs",
			input:    "Check https://example.com and https://another.com",
			expected: `<p>Check <a href="https://example.com">https://example.com</a> and <a href="https://another.com">https://another.com</a></p>`,
		},
		{
			name:     "URLWithSpecialChars",
			input:    "Visit https://example.com?a=1&b=2 today",
			expected: `<p>Visit <a href="https://example.com?a=1&amp;b=2">https://example.com?a=1&amp;b=2</a> today</p>`,
		},
		{
			name:     "TextWithURLAndSpecialChars",
			input:    "<script>alert('xss')</script> Visit https://example.com & enjoy",
			expected: `<p>&lt;script&gt;alert(&#39;xss&#39;)&lt;/script&gt; Visit <a href="https://example.com">https://example.com</a> &amp; enjoy</p>`,
		},
		{
			name:     "UnixNewlines",
			input:    "Line 1\nLine 2\nLine 3",
			expected: "<p>Line 1<br>Line 2<br>Line 3</p>",
		},
		{
			name:     "WindowsNewlines",
			input:    "Line 1\r\nLine 2\r\nLine 3",
			expected: "<p>Line 1<br>Line 2<br>Line 3</p>",
		},
		{
			name:     "MacNewlines",
			input:    "Line 1\rLine 2\rLine 3",
			expected: "<p>Line 1<br>Line 2<br>Line 3</p>",
		},
		{
			name:     "MixedNewlines",
			input:    "Line 1\nLine 2\r\nLine 3\rLine 4",
			expected: "<p>Line 1<br>Line 2<br>Line 3<br>Line 4</p>",
		},
		{
			name:     "NewlinesWithSpecialChars",
			input:    "<b>Bold</b>\n<i>Italic</i>",
			expected: "<p>&lt;b&gt;Bold&lt;/b&gt;<br>&lt;i&gt;Italic&lt;/i&gt;</p>",
		},
		{
			name:     "NewlinesWithURL",
			input:    "Line 1\nhttps://example.com\nLine 3",
			expected: `<p>Line 1<br><a href="https://example.com">https://example.com</a><br>Line 3</p>`,
		},
		{
			name:     "URLAtStart",
			input:    "https://example.com is the website",
			expected: `<p><a href="https://example.com">https://example.com</a> is the website</p>`,
		},
		{
			name:     "URLAtEnd",
			input:    "Visit https://example.com",
			expected: `<p>Visit <a href="https://example.com">https://example.com</a></p>`,
		},
		{
			name:     "OnlyURL",
			input:    "https://example.com",
			expected: `<p><a href="https://example.com">https://example.com</a></p>`,
		},
		{
			name:     "EmptyString",
			input:    "",
			expected: "",
		},
		{
			name:     "OnlyNewlines",
			input:    "\n\n\n",
			expected: "",
		},
		{
			name:     "OnlyWhitespace",
			input:    " \t \n ",
			expected: "",
		},
		{
			name:     "CollapsedSpaces",
			input:    "a  b   c",
			expected: "<p>a b c</p>",
		},
		{
			name:     "CollapsedTabsAndFormFeeds",
			input:    "a\tb\f\fc \t d",
			expected: "<p>a b c d</p>",
		},
		{
			name:     "TrimmedLines",
			input:    "  a  \n\tb\t\nc",
			expected: "<p>a<br>b<br>c</p>",
		},
		{
			name:     "BlankLineBetweenText",
			input:    "a\n\nb",
			expected: "<p>a<br><br>b</p>",
		},
		{
			name:     "NonBreakingSpacePreserved",
			input:    "a\u00a0\u00a0b",
			expected: "<p>a\u00a0\u00a0b</p>",
		},
		{
			name:     "URLWithPath",
			input:    "Check https://example.com/path/to/page for details",
			expected: `<p>Check <a href="https://example.com/path/to/page">https://example.com/path/to/page</a> for details</p>`,
		},
		{
			name:     "URLWithFragment",
			input:    "See https://example.com#section here",
			expected: `<p>See <a href="https://example.com#section">https://example.com#section</a> here</p>`,
		},
		{
			name:     "ComplexExample",
			input:    "User <admin> said:\n\"Check https://example.com & https://test.com\"\nThanks!",
			expected: `<p>User &lt;admin&gt; said:<br>&#34;Check <a href="https://example.com">https://example.com</a> &amp; <a href="https://test.com">https://test.com</a>&#34;<br>Thanks!</p>`,
		},
		{
			name:     "URLsBackToBack",
			input:    "https://example.com https://another.com",
			expected: `<p><a href="https://example.com">https://example.com</a> <a href="https://another.com">https://another.com</a></p>`,
		},
		{
			name:     "URLWithQueryParameters",
			input:    "Search https://example.com?q=test&page=2 now",
			expected: `<p>Search <a href="https://example.com?q=test&amp;page=2">https://example.com?q=test&amp;page=2</a> now</p>`,
		},
		{
			name:     "Email",
			input:    "Contact us at test@example.com for help",
			expected: `<p>Contact us at <a href="mailto:test@example.com">test@example.com</a> for help</p>`,
		},
		{
			name:     "MultipleEmails",
			input:    "Send to alice@example.com or bob@test.org",
			expected: `<p>Send to <a href="mailto:alice@example.com">alice@example.com</a> or <a href="mailto:bob@test.org">bob@test.org</a></p>`,
		},
		{
			name:     "EmailWithPlusAndDash",
			input:    "Email: user+tag@sub-domain.example.com",
			expected: `<p>Email: <a href="mailto:user+tag@sub-domain.example.com">user+tag@sub-domain.example.com</a></p>`,
		},
		{
			name:     "WWWUrl",
			input:    "Visit www.example.com for more",
			expected: `<p>Visit <a href="http://www.example.com">www.example.com</a> for more</p>`,
		},
		{
			name:     "WWWUrlWithPath",
			input:    "Check www.example.com/path/to/page now",
			expected: `<p>Check <a href="http://www.example.com/path/to/page">www.example.com/path/to/page</a> now</p>`,
		},
		{
			name:     "WWWUrlWithQueryParams",
			input:    "Search www.example.com?q=test&page=1 here",
			expected: `<p>Search <a href="http://www.example.com?q=test&amp;page=1">www.example.com?q=test&amp;page=1</a> here</p>`,
		},
		{
			name:     "URLWithTrailingPeriod",
			input:    "Visit https://example.com.",
			expected: `<p>Visit <a href="https://example.com">https://example.com</a>.</p>`,
		},
		{
			name:     "URLWithTrailingComma",
			input:    "Check https://example.com, it's great",
			expected: `<p>Check <a href="https://example.com">https://example.com</a>, it&#39;s great</p>`,
		},
		{
			name:     "URLWithTrailingExclamation",
			input:    "Amazing site https://example.com!",
			expected: `<p>Amazing site <a href="https://example.com">https://example.com</a>!</p>`,
		},
		{
			name:     "URLWithTrailingQuestion",
			input:    "Have you seen https://example.com?",
			expected: `<p>Have you seen <a href="https://example.com">https://example.com</a>?</p>`,
		},
		{
			name:     "URLWithTrailingSemicolon",
			input:    "Links: https://example.com; https://test.com",
			expected: `<p>Links: <a href="https://example.com">https://example.com</a>; <a href="https://test.com">https://test.com</a></p>`,
		},
		{
			name:     "URLWithTrailingColon",
			input:    "The URL: https://example.com:",
			expected: `<p>The URL: <a href="https://example.com">https://example.com</a>:</p>`,
		},
		{
			name:     "URLInQuotes",
			input:    `Contact us at "https://example.com"`,
			expected: `<p>Contact us at &#34;<a href="https://example.com">https://example.com</a>&#34;</p>`,
		},
		{
			name:     "URLInParentheses",
			input:    "See the site (https://example.com) for details",
			expected: `<p>See the site (<a href="https://example.com">https://example.com</a>) for details</p>`,
		},
		{
			name:     "URLInBrackets",
			input:    "Click here [https://example.com]",
			expected: `<p>Click here [<a href="https://example.com">https://example.com</a>]</p>`,
		},
		{
			name:     "URLWithEmailInQueryParam",
			input:    "Contact form: https://example.com/contact?email=test@example.com",
			expected: `<p>Contact form: <a href="https://example.com/contact?email=test@example.com">https://example.com/contact?email=test@example.com</a></p>`,
		},
		{
			name:     "EmailWithTrailingPeriod",
			input:    "Email me at user@example.com.",
			expected: `<p>Email me at <a href="mailto:user@example.com">user@example.com</a>.</p>`,
		},
		{
			name:     "EmailWithTrailingComma",
			input:    "Send to admin@example.com, and we'll help",
			expected: `<p>Send to <a href="mailto:admin@example.com">admin@example.com</a>, and we&#39;ll help</p>`,
		},
		{
			name:     "MixedURLsEmailsAndWWW",
			input:    "Visit https://example.com, www.test.org or email us at support@example.com for help.",
			expected: `<p>Visit <a href="https://example.com">https://example.com</a>, <a href="http://www.test.org">www.test.org</a> or email us at <a href="mailto:support@example.com">support@example.com</a> for help.</p>`,
		},
		{
			name:     "URLWithMultipleTrailingChars",
			input:    "Check this: https://example.com!?",
			expected: `<p>Check this: <a href="https://example.com">https://example.com</a>!?</p>`,
		},
		{
			name:     "EmailInSentence",
			input:    "For questions, reach out to hello@example.com, we're here to help!",
			expected: `<p>For questions, reach out to <a href="mailto:hello@example.com">hello@example.com</a>, we&#39;re here to help!</p>`,
		},
		{
			name:     "UnicodeTrailingChars",
			input:    "Go to ”https://www.example.com/” and continue…",
			expected: `<p>Go to ”<a href="https://www.example.com/">https://www.example.com/</a>” and continue…</p>`,
		},
		{
			name:     "UppercaseHTTPS",
			input:    "Visit HTTPS://EXAMPLE.COM for info",
			expected: `<p>Visit <a href="HTTPS://EXAMPLE.COM">HTTPS://EXAMPLE.COM</a> for info</p>`,
		},
		{
			name:     "MixedCaseHTTP",
			input:    "Go to Http://Example.Com/Path",
			expected: `<p>Go to <a href="Http://Example.Com/Path">Http://Example.Com/Path</a></p>`,
		},
		{
			name:     "UppercaseWWW",
			input:    "Check WWW.EXAMPLE.COM now",
			expected: `<p>Check <a href="http://WWW.EXAMPLE.COM">WWW.EXAMPLE.COM</a> now</p>`,
		},
		{
			name:     "ExistingLink",
			input:    `See <a href="https://www.example.com/">https://www.example.com/</a>!`,
			expected: `<p>See &lt;a href=&#34;<a href="https://www.example.com/">https://www.example.com/</a>&#34;&gt;<a href="https://www.example.com/">https://www.example.com/</a>&lt;/a&gt;!</p>`,
		},
		{
			name:  "ExistingAmpEntity",
			input: `See <a href="https://www.example.com/?a=1&amp;B=2">https://www.example.com/?a=1&amp;B=2</a>!`,
			// This is not the best outcome, but it is good enough.
			expected: `<p>See &lt;a href=&#34;<a href="https://www.example.com/?a=1&amp;amp;B=2">https://www.example.com/?a=1&amp;amp;B=2</a>&#34;&gt;<a href="https://www.example.com/?a=1&amp;amp;B=2">https://www.example.com/?a=1&amp;amp;B=2</a>&lt;/a&gt;!</p>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := transform.TextToHTML(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
