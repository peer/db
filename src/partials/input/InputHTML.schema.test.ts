// @vitest-environment jsdom
// docToHtml serializes through DOM nodes and htmlToDoc parses with innerHTML,
// so these tests need a DOM implementation.

import { describe, expect, test } from "vitest"

import { docToHtml, htmlToDoc, isCanonicalHTML, schema } from "@/partials/input/InputHTML.schema"

// Round trips html through htmlToDoc and docToHtml.
function canonicalize(html: string): string {
  return docToHtml(htmlToDoc(html))
}

describe("docToHtml canonical serialization", () => {
  test.each([
    // The five characters escaped by the backend sanitizer's renderer stay escaped.
    ["<p>it&#39;s fine</p>", "apostrophe in text"],
    ["<p>she said &#34;hello&#34;</p>", "double quote in text"],
    ["<p>R &amp; D, 1 &lt; 2 &gt; 0</p>", "ampersand and angle brackets in text"],
    // U+00A0 is emitted as a raw character, not as &nbsp;.
    ["<p>a\u00a0b</p>", "non-breaking space in text"],
    ['<p><a href="/x?a=1&amp;b=2">x</a></p>', "ampersand in attribute value"],
    ['<p><a href="/o&#39;brien">x</a></p>', "apostrophe in attribute value"],
    ['<p><a href="mailto:info@mg-lj.si">m</a></p>', "mailto link"],
    // Structure produced by the editor schema.
    ["<h1>a</h1><h2>b</h2><h3>c</h3><h4>d</h4>", "headings"],
    ["<p>a<br>b</p>", "hard break"],
    ["<p>a<br><br>b</p>", "blank line"],
    ["<p><br>a</p>", "leading hard break"],
    ["<p>a<br></p>", "trailing hard break"],
    ["<p>a</p><hr><p>b</p>", "horizontal rule"],
    ["<ul><li><p>a</p></li><li><p>b</p></li></ul>", "bullet list"],
    ["<ol><li><p>a</p><ul><li><p>b</p></li></ul></li></ol>", "nested lists"],
    ["<blockquote><p>q</p></blockquote>", "blockquote"],
    ['<blockquote cite="https://example.com/x?a=1&amp;b=2"><p>q</p></blockquote>', "blockquote with cite"],
    ["<pre>a\nb</pre>", "preformatted with newline"],
    ["<p><b>a</b><i>b</i><u>c</u><strike>d</strike><tt>e</tt></p>", "marks"],
  ])("%s is a fixed point (%s)", (html) => {
    expect(canonicalize(html)).toBe(html)
    expect(isCanonicalHTML(html)).toBe(true)
  })

  test.each([
    // The browser's innerHTML serialization of these forms differs from the canonical
    // form, which is why innerHTML cannot be used for serialization.
    ["<p>it's fine</p>", "<p>it&#39;s fine</p>", "raw apostrophe"],
    ['<p>she said "hello"</p>', "<p>she said &#34;hello&#34;</p>", "raw double quote"],
    ["<p>a&nbsp;b</p>", "<p>a\u00a0b</p>", "nbsp entity"],
    // Inline content gets wrapped into a paragraph by the schema.
    ["<b>bold</b>", "<p><b>bold</b></p>", "bare inline content"],
    // Elements the schema does not support are dropped.
    ["<p>a</p><script>alert(1)</script>", "<p>a</p>", "script"],
    ["<h5>x</h5>", "<p>x</p>", "unsupported heading level"],
    ['<p>a</p><img src="/x.png" alt="y"><p>b</p>', "<p>a</p><p>b</p>", "img"],
    // Anchors with hrefs outside the URL allowlist lose the mark but keep the text,
    // and an invalid cite is dropped while the blockquote is kept, matching the
    // backend sanitizer.
    ['<p><a href="javascript:alert(1)">x</a></p>', "<p>x</p>", "javascript link"],
    ['<p><a href="file:///C:/report.docx">report</a></p>', "<p>report</p>", "file link"],
    ['<p><a href="relative.html">x</a></p>', "<p>x</p>", "document-relative link"],
    ['<blockquote cite="javascript:alert(1)"><p>q</p></blockquote>', "<blockquote><p>q</p></blockquote>", "javascript cite"],
    ['<blockquote cite="mailto:info@mg-lj.si"><p>q</p></blockquote>', "<blockquote><p>q</p></blockquote>", "mailto cite"],
  ])("%s canonicalizes to %s (%s)", (html, canonical) => {
    expect(canonicalize(html)).toBe(canonical)
    expect(isCanonicalHTML(html)).toBe(false)
    expect(isCanonicalHTML(canonical)).toBe(true)
  })

  test("marks nest in schema rank order", () => {
    // ProseMirror sorts marks by schema rank, so both nestings parse into the same
    // document and serialize to the same bytes.
    expect(canonicalize("<p><i><b>x</b></i></p>")).toBe("<p><b><i>x</i></b></p>")
    expect(canonicalize("<p><b><i>x</i></b></p>")).toBe("<p><b><i>x</i></b></p>")
  })

  test("empty document serializes to an empty paragraph", () => {
    expect(canonicalize("")).toBe("<p></p>")
  })

  test("Word-flavored HTML normalizes into the supported subset", () => {
    // ProseMirror parses pasted clipboard HTML through this same schema, so this
    // is what pasting from Word produces: presentation markup dissolves to the
    // supported subset and invalid link targets degrade to plain text, all of
    // which the backend accepts as-is.
    const word =
      '<html xmlns:o="urn:schemas-microsoft-com:office:office"><body>' +
      '<p class="MsoNormal" style="margin-bottom:.0001pt"><span style="font-family:Calibri,sans-serif">Hello <b style="mso-bidi-font-weight:normal">world</b><o:p></o:p></span></p>' +
      '<p class="MsoListParagraph"><!--[if !supportLists]--><span style="mso-list:Ignore">1.</span><!--[endif]--> item</p>' +
      '<p class="MsoNormal"><a href="file:///C:/Users/x/report.docx">report</a></p>' +
      "</body></html>"
    const canonical = canonicalize(word)
    expect(canonical).toBe("<p>Hello <b>world</b></p><p>1. item</p><p>report</p>")
    expect(isCanonicalHTML(canonical)).toBe(true)
  })

  test("preformatted with a leading newline survives a parse round trip", () => {
    // The HTML parser drops one newline immediately after the pre open tag, so the
    // serializer doubles a leading newline.
    const doc = schema.node("doc", null, [schema.node("preformatted", null, [schema.text("\nx")])])
    const html = docToHtml(doc)
    expect(html).toBe("<pre>\n\nx</pre>")
    expect(isCanonicalHTML(html)).toBe(true)
    expect(htmlToDoc(html).eq(doc)).toBe(true)
  })
})
