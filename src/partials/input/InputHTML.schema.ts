import type { Node as PMNode } from "prosemirror-model"

import {
  buildSchema,
  docToHtml as docToHtmlImpl,
  htmlToDoc as htmlToDocImpl,
  isCanonicalHTML as isCanonicalHTMLImpl,
  type SchemaJSON,
  type Validator,
} from "@tozd/prosemirror"

// Levels supported by the heading node, used by the toolbar.
export const HEADING_LEVELS = [1, 2, 3, 4] as const

// Named attribute validators referenced by the schema JSON, mirroring linkHrefPattern and
// resourceURLPattern in the backend's document/urls.go: linkURL allows a same-origin path, an
// absolute http or https URL, or a mailto URL; resourceURL is the same minus mailto. Non-string
// values are invalid.
const validators: Record<string, Validator> = {
  linkURL: (value) => typeof value === "string" && /^(?:\/(?:[^/]|$)|https?:\/\/[^/]|mailto:[^/])/i.test(value),
  resourceURL: (value) => typeof value === "string" && /^(?:\/(?:[^/]|$)|https?:\/\/[^/])/i.test(value),
}

// The editor schema is built from the shared schema JSON served at /schema.json, the same document
// the backend builds its schema from (document/schema.json). They cannot drift: the shared
// canonical-cases corpus test asserts the editor and the backend agree, and serving the schema
// from the backend is the future hook for per-site HTML configuration.
const response = await fetch("/schema.json", {
  method: "GET",
  // Mode and credentials match crossorigin=anonymous in link preload header.
  mode: "cors",
  credentials: "same-origin",
  // To support also non-browser environments.
  referrer: typeof document !== "undefined" ? document.location.href : undefined,
  referrerPolicy: "strict-origin-when-cross-origin",
})
const schemaJSON = (await response.json()) as SchemaJSON

export const schema = buildSchema(schemaJSON, { validators })

// preserveWhitespace matches the backend's CanonicalizeHTML (PreserveWhitespaceTrue): runs of spaces
// are kept so a user's spacing survives a round trip and stays canonical, while newlines collapse to
// spaces. The editor's paste path keeps the collapsing default, so formatting whitespace from
// imported HTML is not pulled in as content.
//
// With these options canonicalization is always idempotent for this schema (one pass reaches a
// fixed point), which is what makes isCanonicalHTML well-defined (canonical iff input equals its
// canonicalization). Preserved spaces and pre content round-trip unchanged, and the only
// whitespace transform, newline-to-space, leaves no convertible newlines for a second pass to
// change.
const parseOptions = { preserveWhitespace: true }

export function htmlToDoc(html: string): PMNode {
  return htmlToDocImpl(schema, html, parseOptions)
}

// docToHtml serializes the editor document to the canonical HTML the backend produces, so equal
// documents serialize to equal bytes and collaborating clients can compare HTML against their own
// editor state directly. isCanonicalHTML is the matching claim-validity check (the backend's
// IsCanonicalHTML). Both bind the editor schema and the preserveWhitespace option to the shared
// helpers from @tozd/prosemirror.
export function docToHtml(doc: PMNode): string {
  return docToHtmlImpl(schema, doc)
}

export function isCanonicalHTML(html: string): boolean {
  return isCanonicalHTMLImpl(schema, html, parseOptions)
}
