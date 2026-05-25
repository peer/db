import type { QueryValues, ShortcutPair, JustResultsFilters } from "@/types"

import { Identifier } from "@tozd/identifier"

import { encodeQuery } from "@/utils"

// Reserved tokens in the search shortcut grammar.
const RESERVED_REVERSE = "reverse"
const RESERVED_SELF = "self"

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

// parseShortcut splits a search shortcut string into raw key/value pairs and
// validates the structural rules shared with the backend validator in
// transform/shortcut.go:
//   - first "=" separates a non-empty key from a non-empty value,
//   - the key contains at most one ":" (nested "parent:prop" form),
//   - "reverse" is not allowed inside a nested key.
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
    pairs.push({ key, value })
  }
  return pairs
}

// resolvedPair is one shortcut pair with every identifier token already
// substituted to its canonical 22-character form (and "self" substituted with
// the supplied self ID). Reverse pairs have prop = [] and reverse = true.
type resolvedPair = { reverse: boolean; prop: string[]; value: string }

// resolveShortcut parses a search shortcut string and resolves every
// identifier token into resolvedPair entries. The "self" value is substituted
// with the supplied self ID; if self is undefined and the shortcut references
// "self", an Error is thrown.
async function resolveShortcut(s: string, self?: string): Promise<resolvedPair[]> {
  const pairs = parseShortcut(s)
  const resolved: resolvedPair[] = []
  for (const { key, value } of pairs) {
    let resolvedValue: string
    if (value === RESERVED_SELF) {
      if (self === undefined) {
        throw new Error(`search shortcut uses "self" but no self ID was provided`)
      }
      resolvedValue = self
    } else {
      resolvedValue = await resolveShortcutID(value)
    }
    if (key === RESERVED_REVERSE) {
      resolved.push({ reverse: true, prop: [], value: resolvedValue })
      continue
    }
    const prop: string[] = []
    if (key.includes(":")) {
      const [parentKey, nestedKey] = key.split(":")
      prop.push(await resolveShortcutID(parentKey), await resolveShortcutID(nestedKey))
    } else {
      prop.push(await resolveShortcutID(key))
    }
    resolved.push({ reverse: false, prop, value: resolvedValue })
  }
  return resolved
}

// shortcutToFilters parses a search shortcut string and resolves every
// identifier token into a payload suitable for the SearchJustResults POST
// endpoint. The "self" value is substituted with the supplied self ID; if
// self is undefined and the shortcut references "self", an Error is thrown.
export async function shortcutToFilters(s: string, self?: string): Promise<JustResultsFilters> {
  const payload: JustResultsFilters = {}
  const filters: NonNullable<JustResultsFilters["filters"]> = []
  for (const r of await resolveShortcut(s, self)) {
    if (r.reverse) {
      payload.reverse = r.value
      continue
    }
    filters.push({ prop: r.prop, ref: { to: [{ id: r.value }] } })
  }
  if (filters.length > 0) {
    payload.filters = filters
  }
  return payload
}

// shortcutToQuery parses a search shortcut string and resolves every
// identifier token into a URL query map suitable for routing to the
// SearchShortcut view (which posts the same shortcut grammar as URL params).
// Plain keys map to "<resolved>", nested keys to "<resolvedParent>:<resolvedProp>",
// and reverse maps to the "reverse" key. The "self" value is substituted with
// the supplied self ID; if self is undefined and the shortcut references
// "self", an Error is thrown.
export async function shortcutToQuery(s: string, self?: string): Promise<QueryValues> {
  const filter: Record<string, string> = {}
  for (const r of await resolveShortcut(s, self)) {
    const k = r.reverse ? RESERVED_REVERSE : r.prop.join(":")
    filter[k] = r.value
  }
  return encodeQuery(filter)
}
