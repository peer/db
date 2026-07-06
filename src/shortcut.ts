import type { JustResultsFilters, QueryValues, ShortcutPair } from "@/types"

import { Identifier } from "@tozd/identifier"

import { encodeQuery } from "@/utils"

// Reserved tokens in the search shortcut grammar.
export const RESERVED_REVERSE = "reverse"
// RESERVED_ID is the key whose values are document IDs the search is scoped to (documents whose own ID
// is one of them). Unlike "reverse" it may repeat, one entry per allowed document.
export const RESERVED_ID = "id"
const RESERVED_SELF = "self"
// RESERVED_MISSING is the value token that selects a property's "missing" bucket.
export const RESERVED_MISSING = "missing"
// RESERVED_DIRECT_PREFIX prefixes a value to select its identifier as a "direct" (most-specific) match.
export const RESERVED_DIRECT_PREFIX = "direct:"
// RESERVED_LIMIT is a create-shortcut key (not a search-shortcut one): it restricts the create view to its
// value class and that class's descendants. It is recognized only by createShortcutToQuery below; the
// generic grammar parser treats it like any other literal key.
export const RESERVED_LIMIT = "limit"

// Search shortcut query parameter keys read from the SearchShortcut route query alongside RESERVED_REVERSE.
// Unlike the grammar tokens above they are not part of the shortcut string: they carry the session language
// and full-text query that augment a shortcut.
// RESERVED_LANGUAGE is the query parameter key that sets the search session language.
export const RESERVED_LANGUAGE = "language"
// RESERVED_QUERY is the query parameter key that sets the search session full-text query.
export const RESERVED_QUERY = "q"

// resolveShortcutID resolves a search shortcut identifier token into its
// canonical 22-character form. Multi-part tokens (comma-separated) are hashed
// via Identifier.from; single tokens are returned as-is and are expected to
// already be a valid 22-character identifier.
export async function resolveShortcutID(token: string): Promise<string> {
  if (token.includes(",")) {
    return (await Identifier.from(...token.split(","))).toString()
  }
  return token
}

// requireSelf returns the supplied self ID, throwing when a shortcut references "self" but none was given.
function requireSelf(self?: string): string {
  if (self === undefined) {
    throw new Error(`search shortcut uses "self" but no self ID was provided`)
  }
  return self
}

// parseShortcut splits a search shortcut string into raw key/value pairs and
// validates the structural rules shared with the backend validator in
// transform/shortcut.go:
//   - first "=" separates a non-empty key from a non-empty value,
//   - the key contains at most one ":" (nested "parent:prop" form),
//   - "reverse" and "id" are not allowed inside a nested key.
// Throws on the first violation.
export function parseShortcut(s: string): ShortcutPair[] {
  if (s === "") {
    throw new Error("search shortcut must not be empty")
  }
  const pairs: ShortcutPair[] = []
  for (const part of s.split("&")) {
    const eq = part.indexOf("=")
    if (eq <= 0 || eq === part.length - 1) {
      throw new Error(`search shortcut part must have a non-empty key and value separated by '=': ${part}`)
    }
    const key = part.substring(0, eq)
    const value = part.substring(eq + 1)
    const keyParts = key.split(":")
    if (keyParts.length > 2) {
      throw new Error(`search shortcut key must contain at most one ':': ${key}`)
    }
    if (keyParts.length === 2 && (keyParts[0] === RESERVED_REVERSE || keyParts[1] === RESERVED_REVERSE)) {
      throw new Error(`"reverse" is not allowed inside a nested key: ${key}`)
    }
    if (keyParts.length === 2 && (keyParts[0] === RESERVED_ID || keyParts[1] === RESERVED_ID)) {
      throw new Error(`"id" is not allowed inside a nested key: ${key}`)
    }
    pairs.push({ key, value })
  }
  return pairs
}

// resolvedPair is one shortcut pair with every identifier token already substituted to its canonical
// 22-character form (and "self" substituted with the supplied self ID). Reverse pairs have prop = [] and
// reverse = true; ID pairs have prop = [] and id = true. kind classifies a property pair's value: a plain
// target ("to"), a most-specific target ("direct"), or the missing bucket ("missing", for which value is
// unused).
type resolvedPair = { reverse: boolean; id: boolean; prop: string[]; kind: "to" | "direct" | "missing"; value: string }

// resolveShortcut parses a search shortcut string and resolves every identifier token into resolvedPair
// entries. The "self" value is substituted with the supplied self ID; if self is undefined and the shortcut
// references "self", an Error is thrown. A value of "missing" selects the missing bucket and a
// "direct:<value>" value selects the target as a most-specific match.
async function resolveShortcut(s: string, self?: string): Promise<resolvedPair[]> {
  const pairs = parseShortcut(s)
  const resolved: resolvedPair[] = []
  for (const { key, value } of pairs) {
    if (key === RESERVED_REVERSE) {
      const resolvedValue = value === RESERVED_SELF ? requireSelf(self) : await resolveShortcutID(value)
      resolved.push({ reverse: true, id: false, prop: [], kind: "to", value: resolvedValue })
      continue
    }
    if (key === RESERVED_ID) {
      const resolvedValue = value === RESERVED_SELF ? requireSelf(self) : await resolveShortcutID(value)
      resolved.push({ reverse: false, id: true, prop: [], kind: "to", value: resolvedValue })
      continue
    }
    const prop: string[] = []
    if (key.includes(":")) {
      const [parentKey, nestedKey] = key.split(":")
      prop.push(await resolveShortcutID(parentKey), await resolveShortcutID(nestedKey))
    } else {
      prop.push(await resolveShortcutID(key))
    }
    if (value === RESERVED_MISSING) {
      resolved.push({ reverse: false, id: false, prop, kind: "missing", value: "" })
      continue
    }
    let kind: "to" | "direct" = "to"
    let token = value
    if (value.startsWith(RESERVED_DIRECT_PREFIX)) {
      kind = "direct"
      token = value.slice(RESERVED_DIRECT_PREFIX.length)
    }
    const resolvedValue = token === RESERVED_SELF ? requireSelf(self) : await resolveShortcutID(token)
    resolved.push({ reverse: false, id: false, prop, kind, value: resolvedValue })
  }
  return resolved
}

// shortcutToFilters parses a search shortcut string and resolves every identifier token into a payload
// suitable for the SearchJustResults POST endpoint. The "self" value is substituted with the supplied self
// ID; if self is undefined and the shortcut references "self", an Error is thrown. Values for the same
// property are grouped into a single filter, OR-ed across its "to", "direct", and "missing" selections.
// Filters are ordered by first appearance. All "id" values are collected into the ids scope (documents
// whose own ID is one of them).
export async function shortcutToFilters(s: string, self?: string): Promise<JustResultsFilters> {
  const payload: JustResultsFilters = {}
  const filters: NonNullable<JustResultsFilters["filters"]> = []
  const byProp = new Map<string, NonNullable<JustResultsFilters["filters"]>[number]>()
  for (const r of await resolveShortcut(s, self)) {
    if (r.reverse) {
      payload.reverse = r.value
      continue
    }
    if (r.id) {
      payload.ids = payload.ids ?? []
      payload.ids.push(r.value)
      continue
    }
    const key = r.prop.join(":")
    let filter = byProp.get(key)
    if (!filter) {
      filter = { prop: r.prop, ref: {} }
      byProp.set(key, filter)
      filters.push(filter)
    }
    if (r.kind === "missing") {
      filter.ref.missing = true
    } else if (r.kind === "direct") {
      filter.ref.direct = filter.ref.direct ?? []
      filter.ref.direct.push({ id: r.value })
    } else {
      filter.ref.to = filter.ref.to ?? []
      filter.ref.to.push({ id: r.value })
    }
  }
  if (filters.length > 0) {
    payload.filters = filters
  }
  return payload
}

// shortcutToQuery parses a search shortcut string and resolves every identifier token into a URL query map
// suitable for routing to the SearchShortcut view (which posts the same shortcut grammar as URL params).
// Plain keys map to "<resolved>", nested keys to "<resolvedParent>:<resolvedProp>", reverse maps to the
// "reverse" key, and id to the "id" key. A value is "<resolved>", "missing", or "direct:<resolved>". The
// "self" value is substituted with the supplied self ID; if self is undefined and the shortcut references
// "self", an Error is thrown.
export async function shortcutToQuery(s: string, self?: string): Promise<QueryValues> {
  const filter: Record<string, string[]> = {}
  for (const r of await resolveShortcut(s, self)) {
    const k = r.reverse ? RESERVED_REVERSE : r.id ? RESERVED_ID : r.prop.join(":")
    let v = r.value
    if (r.kind === "missing") {
      v = RESERVED_MISSING
    } else if (r.kind === "direct") {
      v = RESERVED_DIRECT_PREFIX + r.value
    }
    ;(filter[k] ??= []).push(v)
  }
  return encodeQuery(filter)
}

// createShortcutToQuery parses a CREATE_SHORTCUT string (it uses the same grammar as a search shortcut) and
// resolves it into a URL query map for the create-document view. The "limit" key is kept verbatim (it
// restricts the offered classes to its value class and descendants); every other key is a property
// identifier (resolved). Each value is resolved to an identifier, with "self" substituted by the supplied
// self ID, so a non-limit entry becomes an initial reference claim (property=value). Throws if the shortcut
// references "self" but none was provided. Nested keys, "missing", and "direct:" are not used for creation.
export async function createShortcutToQuery(s: string, self?: string): Promise<QueryValues> {
  const query: Record<string, string[]> = {}
  for (const { key, value } of parseShortcut(s)) {
    const resolvedKey = key === RESERVED_LIMIT ? RESERVED_LIMIT : await resolveShortcutID(key)
    const resolvedValue = value === RESERVED_SELF ? requireSelf(self) : await resolveShortcutID(value)
    ;(query[resolvedKey] ??= []).push(resolvedValue)
  }
  return encodeQuery(query)
}
