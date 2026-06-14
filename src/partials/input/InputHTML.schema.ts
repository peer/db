import type { Node as PMNode } from "prosemirror-model"

import {
  buildSchema,
  docToHtml as docToHtmlImpl,
  htmlToDoc as htmlToDocImpl,
  isCanonicalHTML as isCanonicalHTMLImpl,
  type SchemaJSON,
  type Validator,
} from "@tozd/prosemirror"

import { validateUrl } from "@/utils"

// Levels supported by the heading node, used by the toolbar.
export const HEADING_LEVELS = [1, 2, 3, 4] as const

// Named attribute validators referenced by the schema JSON. They run link attributes through the same
// parseUrl-based validation (validateUrl) used for LinkClaim IRIs and matching validateURL in the
// backend's document/urls.go: linkURL (<a href>) allows a same-origin path or an http, https, or
// mailto URL; resourceURL (<blockquote cite>) is the same minus mailto. Non-string values are invalid.
const validators: Record<string, Validator> = {
  linkURL: (value) => typeof value === "string" && validateUrl(value, { allowMailto: true }),
  resourceURL: (value) => typeof value === "string" && validateUrl(value, { allowMailto: false }),
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
