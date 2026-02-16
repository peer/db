package transform_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"gitlab.com/peerdb/peerdb/transform"
)

func TestSanitizeHTML(t *testing.T) {
	t.Parallel()

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
			name:     "ExternalLinkHTTPS",
			input:    `<a href="https://example.com">Link</a>`,
			expected: `<a href="https://example.com" rel="noreferrer">Link</a>`,
		},
		{
			name:     "ExternalLinkHTTP",
			input:    `<a href="http://example.com">Link</a>`,
			expected: `<a href="http://example.com" rel="noreferrer">Link</a>`,
		},
		{
			name:     "MailtoLink",
			input:    `<a href="mailto:test@example.com">Email</a>`,
			expected: `<a href="mailto:test@example.com">Email</a>`,
		},
		{
			name:     "RemovesScriptTag",
			input:    `<p>Safe content</p><script>alert('xss')</script>`,
			expected: `<p>Safe content</p>`,
		},
		{
			name:     "RemovesScriptContent",
			input:    `<script>alert('xss')</script><p>After</p>`,
			expected: `<p>After</p>`,
		},
		{
			name:     "RemovesIframe",
			input:    `<p>Before</p><iframe src="evil.com"></iframe><p>After</p>`,
			expected: `<p>Before</p><p>After</p>`,
		},
		{
			name:     "RemovesStyle",
			input:    `<style>body { display: none; }</style><p>Content</p>`,
			expected: `<p>Content</p>`,
		},
		{
			name:     "RemovesOnclickAttribute",
			input:    `<a href="https://example.com" onclick="evil()">Link</a>`,
			expected: `<a href="https://example.com" rel="noreferrer">Link</a>`,
		},
		{
			name:     "RemovesOnerrorAttribute",
			input:    `<img src="https://example.com/img.jpg" onerror="evil()">`,
			expected: `<img src="https://example.com/img.jpg">`,
		},
		{
			name:     "PreservesImageSrcAlt",
			input:    `<img src="https://example.com/img.jpg" alt="Test Image">`,
			expected: `<img src="https://example.com/img.jpg" alt="Test Image">`,
		},
		{
			name:     "RemovesJavascriptScheme",
			input:    `<a href="javascript:alert('xss')">Click</a>`,
			expected: `Click`,
		},
		{
			name:     "RemovesDataScheme",
			input:    `<a href="data:text/html,<script>alert('xss')</script>">Click</a>`,
			expected: `Click`,
		},
		{
			name:     "AllowedFormattingTags",
			input:    `<p>Text with <b>bold</b>, <i>italic</i>, <u>underline</u>, and <strike>strikethrough</strike></p>`,
			expected: `<p>Text with <b>bold</b>, <i>italic</i>, <u>underline</u>, and <strike>strikethrough</strike></p>`,
		},
		{
			name:     "AllowedHeadings",
			input:    `<h1>H1</h1><h2>H2</h2><h3>H3</h3><h4>H4</h4><h5>H5</h5><h6>H6</h6>`,
			expected: `<h1>H1</h1><h2>H2</h2><h3>H3</h3><h4>H4</h4><h5>H5</h5><h6>H6</h6>`,
		},
		{
			name:     "AllowedListElements",
			input:    `<ul><li>Item 1</li><li>Item 2</li></ul><ol><li>First</li><li>Second</li></ol>`,
			expected: `<ul><li>Item 1</li><li>Item 2</li></ul><ol><li>First</li><li>Second</li></ol>`,
		},
		{
			name:     "Blockquote",
			input:    `<blockquote>Quote text</blockquote>`,
			expected: `<blockquote>Quote text</blockquote>`,
		},
		{
			name:     "PreformattedText",
			input:    `<pre>Code block</pre>`,
			expected: `<pre>Code block</pre>`,
		},
		{
			name:     "BreakAndHorizontalRule",
			input:    `Line 1<br>Line 2<hr>Line 3`,
			expected: `Line 1<br>Line 2<hr>Line 3`,
		},
		{
			name:     "RemovesFormElements",
			input:    `<form><input type="text" name="field"></form><p>After</p>`,
			expected: `<p>After</p>`,
		},
		{
			name:     "RemovesButtonElements",
			input:    `<button onclick="evil()">Click</button><p>Safe</p>`,
			expected: `Click<p>Safe</p>`,
		},
		{
			name:     "RemovesObjectEmbed",
			input:    `<object data="evil.swf"></object><embed src="evil.swf"><p>Content</p>`,
			expected: `<p>Content</p>`,
		},
		{
			name:     "NestedAllowedElements",
			input:    `<blockquote><p>Quote with <b>bold</b> and <i>italic</i></p><ul><li>Item 1</li><li>Item 2</li></ul></blockquote>`,
			expected: `<blockquote><p>Quote with <b>bold</b> and <i>italic</i></p><ul><li>Item 1</li><li>Item 2</li></ul></blockquote>`,
		},
		{
			name:     "MixedSafeAndDangerous",
			input:    `<p>Safe <script>evil()</script> text</p><b>Bold</b>`,
			expected: `<p>Safe  text</p><b>Bold</b>`,
		},
		{
			name:     "MultipleExternalLinks",
			input:    `<a href="https://example.com">Link 1</a> and <a href="http://test.com">Link 2</a>`,
			expected: `<a href="https://example.com" rel="noreferrer">Link 1</a> and <a href="http://test.com" rel="noreferrer">Link 2</a>`,
		},
		{
			name:     "EmptyString",
			input:    "",
			expected: "",
		},
		{
			name:     "OnlyWhitespace",
			input:    "   \n\t  ",
			expected: "   \n\t  ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := transform.TestingSanitizeHTML(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
