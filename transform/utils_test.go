package transform_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"gitlab.com/peerdb/peerdb/transform"
)

func TestEscapeHTML(t *testing.T) {
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
			expected: "Hello, World!",
		},
		{
			name:     "SpecialCharacters",
			input:    "<script>alert('xss')</script>",
			expected: "&lt;script&gt;alert(&#39;xss&#39;)&lt;/script&gt;",
		},
		{
			name:     "Ampersand",
			input:    "Tom & Jerry",
			expected: "Tom &amp; Jerry",
		},
		{
			name:     "Quotes",
			input:    `He said "Hello"`,
			expected: `He said &#34;Hello&#34;`,
		},
		{
			name:     "SingleURL",
			input:    "Visit https://example.com for info",
			expected: `Visit <a href="https://example.com">https://example.com</a> for info`,
		},
		{
			name:     "HTTPandHTTPS",
			input:    "HTTP: http://example.com and HTTPS: https://secure.example.com",
			expected: `HTTP: <a href="http://example.com">http://example.com</a> and HTTPS: <a href="https://secure.example.com">https://secure.example.com</a>`,
		},
		{
			name:     "MultipleURLs",
			input:    "Check https://example.com and https://another.com",
			expected: `Check <a href="https://example.com">https://example.com</a> and <a href="https://another.com">https://another.com</a>`,
		},
		{
			name:     "URLWithSpecialChars",
			input:    "Visit https://example.com?a=1&b=2 today",
			expected: `Visit <a href="https://example.com?a=1&amp;b=2">https://example.com?a=1&amp;b=2</a> today`,
		},
		{
			name:     "TextWithURLAndSpecialChars",
			input:    "<script>alert('xss')</script> Visit https://example.com & enjoy",
			expected: `&lt;script&gt;alert(&#39;xss&#39;)&lt;/script&gt; Visit <a href="https://example.com">https://example.com</a> &amp; enjoy`,
		},
		{
			name:     "UnixNewlines",
			input:    "Line 1\nLine 2\nLine 3",
			expected: "Line 1<br/>Line 2<br/>Line 3",
		},
		{
			name:     "WindowsNewlines",
			input:    "Line 1\r\nLine 2\r\nLine 3",
			expected: "Line 1<br/>Line 2<br/>Line 3",
		},
		{
			name:     "MacNewlines",
			input:    "Line 1\rLine 2\rLine 3",
			expected: "Line 1<br/>Line 2<br/>Line 3",
		},
		{
			name:     "MixedNewlines",
			input:    "Line 1\nLine 2\r\nLine 3\rLine 4",
			expected: "Line 1<br/>Line 2<br/>Line 3<br/>Line 4",
		},
		{
			name:     "NewlinesWithSpecialChars",
			input:    "<b>Bold</b>\n<i>Italic</i>",
			expected: "&lt;b&gt;Bold&lt;/b&gt;<br/>&lt;i&gt;Italic&lt;/i&gt;",
		},
		{
			name:     "NewlinesWithURL",
			input:    "Line 1\nhttps://example.com\nLine 3",
			expected: `Line 1<br/><a href="https://example.com">https://example.com</a><br/>Line 3`,
		},
		{
			name:     "URLAtStart",
			input:    "https://example.com is the website",
			expected: `<a href="https://example.com">https://example.com</a> is the website`,
		},
		{
			name:     "URLAtEnd",
			input:    "Visit https://example.com",
			expected: `Visit <a href="https://example.com">https://example.com</a>`,
		},
		{
			name:     "OnlyURL",
			input:    "https://example.com",
			expected: `<a href="https://example.com">https://example.com</a>`,
		},
		{
			name:     "EmptyString",
			input:    "",
			expected: "",
		},
		{
			name:     "OnlyNewlines",
			input:    "\n\n\n",
			expected: "<br/><br/><br/>",
		},
		{
			name:     "URLWithPath",
			input:    "Check https://example.com/path/to/page for details",
			expected: `Check <a href="https://example.com/path/to/page">https://example.com/path/to/page</a> for details`,
		},
		{
			name:     "URLWithFragment",
			input:    "See https://example.com#section here",
			expected: `See <a href="https://example.com#section">https://example.com#section</a> here`,
		},
		{
			name:     "ComplexExample",
			input:    "User <admin> said:\n\"Check https://example.com & https://test.com\"\nThanks!",
			expected: `User &lt;admin&gt; said:<br/>&#34;Check <a href="https://example.com">https://example.com</a> &amp; <a href="https://test.com">https://test.com</a>&#34;<br/>Thanks!`,
		},
		{
			name:     "URLsBackToBack",
			input:    "https://example.com https://another.com",
			expected: `<a href="https://example.com">https://example.com</a> <a href="https://another.com">https://another.com</a>`,
		},
		{
			name:     "URLWithQueryParameters",
			input:    "Search https://example.com?q=test&page=2 now",
			expected: `Search <a href="https://example.com?q=test&amp;page=2">https://example.com?q=test&amp;page=2</a> now`,
		},
		{
			name:     "Email",
			input:    "Contact us at test@example.com for help",
			expected: `Contact us at <a href="mailto:test@example.com">test@example.com</a> for help`,
		},
		{
			name:     "MultipleEmails",
			input:    "Send to alice@example.com or bob@test.org",
			expected: `Send to <a href="mailto:alice@example.com">alice@example.com</a> or <a href="mailto:bob@test.org">bob@test.org</a>`,
		},
		{
			name:     "EmailWithPlusAndDash",
			input:    "Email: user+tag@sub-domain.example.com",
			expected: `Email: <a href="mailto:user+tag@sub-domain.example.com">user+tag@sub-domain.example.com</a>`,
		},
		{
			name:     "WWWUrl",
			input:    "Visit www.example.com for more",
			expected: `Visit <a href="http://www.example.com">www.example.com</a> for more`,
		},
		{
			name:     "WWWUrlWithPath",
			input:    "Check www.example.com/path/to/page now",
			expected: `Check <a href="http://www.example.com/path/to/page">www.example.com/path/to/page</a> now`,
		},
		{
			name:     "WWWUrlWithQueryParams",
			input:    "Search www.example.com?q=test&page=1 here",
			expected: `Search <a href="http://www.example.com?q=test&amp;page=1">www.example.com?q=test&amp;page=1</a> here`,
		},
		{
			name:     "URLWithTrailingPeriod",
			input:    "Visit https://example.com.",
			expected: `Visit <a href="https://example.com">https://example.com</a>.`,
		},
		{
			name:     "URLWithTrailingComma",
			input:    "Check https://example.com, it's great",
			expected: `Check <a href="https://example.com">https://example.com</a>, it&#39;s great`,
		},
		{
			name:     "URLWithTrailingExclamation",
			input:    "Amazing site https://example.com!",
			expected: `Amazing site <a href="https://example.com">https://example.com</a>!`,
		},
		{
			name:     "URLWithTrailingQuestion",
			input:    "Have you seen https://example.com?",
			expected: `Have you seen <a href="https://example.com">https://example.com</a>?`,
		},
		{
			name:     "URLWithTrailingSemicolon",
			input:    "Links: https://example.com; https://test.com",
			expected: `Links: <a href="https://example.com">https://example.com</a>; <a href="https://test.com">https://test.com</a>`,
		},
		{
			name:     "URLWithTrailingColon",
			input:    "The URL: https://example.com:",
			expected: `The URL: <a href="https://example.com">https://example.com</a>:`,
		},
		{
			name:     "URLInQuotes",
			input:    `Contact us at "https://example.com"`,
			expected: `Contact us at &#34;<a href="https://example.com">https://example.com</a>&#34;`,
		},
		{
			name:     "URLInParentheses",
			input:    "See the site (https://example.com) for details",
			expected: `See the site (<a href="https://example.com">https://example.com</a>) for details`,
		},
		{
			name:     "URLInBrackets",
			input:    "Click here [https://example.com]",
			expected: `Click here [<a href="https://example.com">https://example.com</a>]`,
		},
		{
			name:     "URLWithEmailInQueryParam",
			input:    "Contact form: https://example.com/contact?email=test@example.com",
			expected: `Contact form: <a href="https://example.com/contact?email=test@example.com">https://example.com/contact?email=test@example.com</a>`,
		},
		{
			name:     "EmailWithTrailingPeriod",
			input:    "Email me at user@example.com.",
			expected: `Email me at <a href="mailto:user@example.com">user@example.com</a>.`,
		},
		{
			name:     "EmailWithTrailingComma",
			input:    "Send to admin@example.com, and we'll help",
			expected: `Send to <a href="mailto:admin@example.com">admin@example.com</a>, and we&#39;ll help`,
		},
		{
			name:     "MixedURLsEmailsAndWWW",
			input:    "Visit https://example.com, www.test.org or email us at support@example.com for help.",
			expected: `Visit <a href="https://example.com">https://example.com</a>, <a href="http://www.test.org">www.test.org</a> or email us at <a href="mailto:support@example.com">support@example.com</a> for help.`,
		},
		{
			name:     "URLWithMultipleTrailingChars",
			input:    "Check this: https://example.com!?",
			expected: `Check this: <a href="https://example.com">https://example.com</a>!?`,
		},
		{
			name:     "EmailInSentence",
			input:    "For questions, reach out to hello@example.com, we're here to help!",
			expected: `For questions, reach out to <a href="mailto:hello@example.com">hello@example.com</a>, we&#39;re here to help!`,
		},
		{
			name:     "UnicodeTrailingChars",
			input:    "Go to ”https://www.example.com/” and continue…",
			expected: `Go to ”<a href="https://www.example.com/">https://www.example.com/</a>” and continue…`,
		},
		{
			name:     "UppercaseHTTPS",
			input:    "Visit HTTPS://EXAMPLE.COM for info",
			expected: `Visit <a href="HTTPS://EXAMPLE.COM">HTTPS://EXAMPLE.COM</a> for info`,
		},
		{
			name:     "MixedCaseHTTP",
			input:    "Go to Http://Example.Com/Path",
			expected: `Go to <a href="Http://Example.Com/Path">Http://Example.Com/Path</a>`,
		},
		{
			name:     "UppercaseWWW",
			input:    "Check WWW.EXAMPLE.COM now",
			expected: `Check <a href="http://WWW.EXAMPLE.COM">WWW.EXAMPLE.COM</a> now`,
		},
		{
			name:     "ExistingLink",
			input:    `See <a href="https://www.example.com/">https://www.example.com/</a>!`,
			expected: `See &lt;a href=&#34;<a href="https://www.example.com/">https://www.example.com/</a>&#34;&gt;<a href="https://www.example.com/">https://www.example.com/</a>&lt;/a&gt;!`,
		},
		{
			name:  "ExistingAmpEntity",
			input: `See <a href="https://www.example.com/?a=1&amp;B=2">https://www.example.com/?a=1&amp;B=2</a>!`,
			// This is not the best outcome, but it is good enough.
			expected: `See &lt;a href=&#34;<a href="https://www.example.com/?a=1&amp;amp;B=2">https://www.example.com/?a=1&amp;amp;B=2</a>&#34;&gt;<a href="https://www.example.com/?a=1&amp;amp;B=2">https://www.example.com/?a=1&amp;amp;B=2</a>&lt;/a&gt;!`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := transform.TestingEscapeHTML(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
