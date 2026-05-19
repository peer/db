import type { Attrs, DOMOutputSpec, MarkSpec, NodeSpec, Node as PMNode } from "prosemirror-model"

import { DOMSerializer, DOMParser as PMDOMParser, Schema } from "prosemirror-model"

// TODO: Add image support.
//       The backend sanitizer (document/sanitize.go) permits <img src alt>, but we do not yet have UI for inserting images.
//       When we bring it back, restore the node spec (inline, draggable, src/alt attrs, toDOM/parseDOM mirroring the
//       sanitizer) and add a corresponding toolbar button. Until then, any <img> in loaded HTML is dropped on parse into
//       the editor.

// Levels supported by the heading node.
export const HEADING_LEVELS = [1, 2, 3, 4] as const

// Schema mirrors the bluemonday allowlist in document/sanitize.go so the
// editor cannot produce HTML that the backend would later strip.
const nodes: Record<string, NodeSpec> = {
  // Only the top-level doc accepts hr. doc's content expression
  // explicitly mentions "horizontal_rule"; every other container in
  // the schema either uses a narrower spec or references the "block"
  // group - and horizontal_rule deliberately does not declare a "block"
  // group below, so the group reference would still exclude it. Either
  // way, hr can never end up nested inside a list, blockquote, or
  // anything else.
  doc: { content: "(block | horizontal_rule)+" },
  paragraph: {
    group: "block",
    content: "inline*",
    toDOM: (): DOMOutputSpec => ["p", 0],
    parseDOM: [{ tag: "p" }],
  },
  blockquote: {
    group: "block",
    // blockquote_paragraph (not paragraph): blockquote is a wrapper rather
    // than a textblock, so Enter splits to a new paragraph inside the
    // blockquote (matching the way list_item with "paragraph block*"
    // works) instead of creating a fresh sibling blockquote. Marks /
    // links live on the inner paragraph's inline content, so they are
    // still supported - except italic, which blockquote_paragraph omits
    // from its allowed marks (the prose stylesheet already renders
    // blockquote contents in italic via CSS, so an italic mark is redundant).
    content: "blockquote_paragraph+",
    defining: true,
    attrs: { cite: { default: null } },
    toDOM: (node): DOMOutputSpec => (node.attrs.cite ? ["blockquote", { cite: node.attrs.cite as string }, 0] : ["blockquote", 0]),
    parseDOM: [
      {
        tag: "blockquote",
        getAttrs: (dom): Attrs => ({ cite: dom.getAttribute("cite") }),
      },
    ],
  },
  // The textblock used inside blockquote. Same shape as paragraph but with
  // an explicit marks allowlist that excludes italic. Not in any group
  // (not "block") so it can only sit inside blockquote, whose content
  // spec is "blockquote_paragraph+". In HTML we still serialize this
  // block as regular <p>. Parsing detects <p> inside <blockquote> via a
  // context filter (higher priority than paragraph's parse rule wins
  // when both match).
  blockquote_paragraph: {
    content: "inline*",
    marks: "link bold underline strikethrough monospace",
    toDOM: (): DOMOutputSpec => ["p", 0],
    parseDOM: [{ tag: "p", context: "blockquote/", priority: 60 }],
  },
  horizontal_rule: {
    // Intentionally not in the "block" group - that would let it slip
    // into any container whose content spec references "block".
    toDOM: (): DOMOutputSpec => ["hr"],
    parseDOM: [{ tag: "hr" }],
  },
  heading: {
    attrs: { level: { default: 1 } },
    group: "block",
    // Plain text content only - no marks (bold / italic / link / etc.)
    // inside a heading. Toolbar buttons in the formatting and insertion
    // pills disable themselves while the cursor is in a heading; the
    // schema-level marks: "" is the source of truth (a paste, drop or
    // toggleMark dry-run also respects it).
    content: "inline*",
    marks: "",
    defining: true,
    toDOM: (node): DOMOutputSpec => [`h${node.attrs.level as number}`, 0],
    parseDOM: HEADING_LEVELS.map((level) => ({ tag: `h${level}`, attrs: { level } })),
  },
  preformatted: {
    group: "block",
    content: "text*",
    marks: "",
    code: true,
    defining: true,
    toDOM: (): DOMOutputSpec => ["pre", 0],
    parseDOM: [{ tag: "pre", preserveWhitespace: "full" }],
  },
  bullet_list: {
    group: "block",
    content: "list_item+",
    toDOM: (): DOMOutputSpec => ["ul", 0],
    parseDOM: [{ tag: "ul" }],
  },
  ordered_list: {
    group: "block",
    content: "list_item+",
    toDOM: (): DOMOutputSpec => ["ol", 0],
    parseDOM: [{ tag: "ol" }],
  },
  list_item: {
    // Content restricted to paragraphs (with marks/links via the paragraph
    // node) and sub-lists - the same set the toolbar can produce inside a list.
    content: "paragraph (paragraph | bullet_list | ordered_list)*",
    defining: true,
    toDOM: (): DOMOutputSpec => ["li", 0],
    parseDOM: [{ tag: "li" }],
  },
  text: { group: "inline" },
  hard_break: {
    inline: true,
    group: "inline",
    selectable: false,
    toDOM: (): DOMOutputSpec => ["br"],
    parseDOM: [{ tag: "br" }],
  },
}

const marks: Record<string, MarkSpec> = {
  // Non-inclusive so typing past the end of a link does not extend it.
  link: {
    attrs: { href: {} },
    inclusive: false,
    toDOM: (mark): DOMOutputSpec => ["a", { href: mark.attrs.href as string }, 0],
    parseDOM: [
      {
        tag: "a[href]",
        getAttrs: (dom): Attrs => ({ href: dom.getAttribute("href") }),
      },
    ],
  },
  bold: {
    toDOM: (): DOMOutputSpec => ["b", 0],
    parseDOM: [{ tag: "b" }, { tag: "strong" }],
  },
  italic: {
    toDOM: (): DOMOutputSpec => ["i", 0],
    parseDOM: [{ tag: "i" }, { tag: "em" }],
  },
  underline: {
    toDOM: (): DOMOutputSpec => ["u", 0],
    parseDOM: [{ tag: "u" }],
  },
  strikethrough: {
    toDOM: (): DOMOutputSpec => ["strike", 0],
    parseDOM: [{ tag: "strike" }, { tag: "s" }, { tag: "del" }],
  },
  monospace: {
    toDOM: (): DOMOutputSpec => ["tt", 0],
    parseDOM: [{ tag: "tt" }],
  },
}

export const schema = new Schema({ nodes, marks })

const domSerializer = DOMSerializer.fromSchema(schema)

export function htmlToDoc(html: string): PMNode {
  const container = document.createElement("div")
  container.innerHTML = html
  return PMDOMParser.fromSchema(schema).parse(container)
}

export function docToHtml(doc: PMNode): string {
  const fragment = domSerializer.serializeFragment(doc.content)
  const container = document.createElement("div")
  container.appendChild(fragment)
  return container.innerHTML
}
