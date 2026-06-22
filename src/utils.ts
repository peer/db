import type { ComputedRef, DeepReadonly, InjectionKey, Ref } from "vue"

import type { TimePrecision } from "@/document"
import type { GetDisplayLabel, Mutable, QueryValues, QueryValuesWithOptional, Result } from "@/types"

import { Identifier } from "@tozd/identifier"
import { prng_alea } from "esm-seedrandom"
import { cloneDeep, isEqual } from "lodash-es"
import { onBeforeUnmount, onMounted, readonly, ref, shallowRef, toRaw, watch, watchEffect } from "vue"

import { INSTANCE_OF, NAME, TITLE } from "@/core"
import { getClaimsOfTypeWithConfidence, selectClaimsByLanguage } from "@/document/claims"
import { AddClaimChange } from "@/document/patch"
import { yearPrecisionMultiple } from "@/document/time"
import { getDisplayLabelFunctions } from "@/registry/display-label"
import { hour, minute, second, toDate } from "@/time"

// Wildcard to see if a string ends with unicode letter or number.
const WILDCARD_SEARCH_REGEX = /[\p{L}\p{N}]$/u

// addPrefixWildcard appends "*" to a search query that ends with a unicode letter or number so the last
// term is matched as a prefix. A query ending in whitespace or punctuation (or empty) is returned unchanged.
export function addPrefixWildcard(query: string): string {
  if (WILDCARD_SEARCH_REGEX.test(query)) {
    return query + "*"
  }
  return query
}

// If the last increase would be equal or less than this number, just skip to the end.
export const SKIP_TO_END = 2

// searchPagerKey carries, from SearchResultsFeed down to the nested SearchResultGroup tree, the per-node data
// the grouped view needs. pagerBefore maps each node before which a progress pager should appear (one every 10
// unique results) to the count of unique results preceding it; the node is the leaf the pager precedes, or the
// group it opens when the pager lands at a group's start (so the pager renders above the heading). shown is the
// total number of unique results rendered; total is the number of matching documents; duplicates holds the leaf
// nodes whose document already appeared earlier (a multi-placed document beyond its first occurrence), which are
// rendered as a back-reference to the first occurrence instead of in full. Counting unique results means a
// multi-placed document is tallied only on its first appearance, so a pager can span more than 10 cards yet
// still mark 10 new results.
export const searchPagerKey: InjectionKey<ComputedRef<{ pagerBefore: Map<object, number>; shown: number; total: number; duplicates: Set<object> }>> =
  process.env.NODE_ENV !== "production" ? Symbol.for("peerdb-search-pager") : Symbol()

// searchExpandKey carries, from SearchResultsFeed down to the nested SearchResultGroup tree, a callback that
// switches a grouping level to its expanded form (each group value shown as a full result card instead of a
// one-line heading). It takes the depth of the group level to expand, the same change the sort dialog's Expand
// checkbox makes; undoing it is done from the sort dialog. The default is a no-op for trees without a provider.
export const searchExpandKey: InjectionKey<(depth: number) => void> = process.env.NODE_ENV !== "production" ? Symbol.for("peerdb-search-expand") : Symbol()

// limitGroupedResults truncates a grouped result tree to the leaves that appear before its (limit+1)th unique
// result in document order, pruning any group left empty, and returns that tree with the number of unique
// results it contains. The backend sends the whole tree at once, so this drives the grouped view's client
// side load-more. Counting unique documents (a multi-placed document once) makes revealing 10 more line up
// with the every-10 unique-result pagers: a leaf already seen is kept, a new leaf beyond the limit stops the
// walk. Group nodes are shallow-copied with their kept children; leaf nodes are returned as-is so their
// identity still matches the pager map.
export function limitGroupedResults(nodes: DeepReadonly<Result[]>, limit: number): { results: DeepReadonly<Result>[]; shown: number } {
  const seen = new Set<string>()
  let stopped = false
  const walk = (input: DeepReadonly<Result[]>): DeepReadonly<Result>[] => {
    const out: DeepReadonly<Result>[] = []
    for (const node of input) {
      if (stopped) {
        break
      }
      if (node.group) {
        const kept = walk(node.group)
        if (kept.length > 0) {
          out.push({ ...node, group: kept })
        }
      } else if (seen.has(node.id)) {
        out.push(node)
      } else if (seen.size < limit) {
        seen.add(node.id)
        out.push(node)
      } else {
        stopped = true
      }
    }
    return out
  }
  return { results: walk(nodes), shown: seen.size }
}

// RefValueLike is the minimal shape of a reference filter value the selection logic needs: the
// value id and its hierarchy paths. Each path is an ancestor chain from a root to the value's
// immediate parent; a "direct" entry's path ends with its own value, and the top-level "missing"
// entry has no paths. RefFilterResult satisfies this.
export type RefValueLike = { id: string; paths?: string[][] }

// RefCheckState is the tri-state a reference filter value renders as.
export type RefCheckState = { checked: boolean; indeterminate: boolean }

// refChildrenByValue maps each value id to the ids of its immediate children. A value's immediate
// parent is the last element of each of its hierarchy paths, so every value is registered as a
// child of those parents. The "direct" entry lists its own value as that last element, so it is
// a child of the value it belongs to. Values without paths (roots, the "missing" entry) are
// children of nothing.
function refChildrenByValue(values: readonly RefValueLike[]): Map<string, string[]> {
  const children = new Map<string, string[]>()
  for (const value of values) {
    for (const path of value.paths ?? []) {
      if (path.length === 0) {
        continue
      }
      const parent = path[path.length - 1]
      let list = children.get(parent)
      if (!list) {
        list = []
        children.set(parent, list)
      }
      if (!list.includes(value.id)) {
        list.push(value.id)
      }
    }
  }
  return children
}

// refSubtreeIds returns id together with every value reachable from it through the children map
// (its descendants in the value hierarchy, including the "direct" entry). It is cycle-safe.
function refSubtreeIds(id: string, children: ReadonlyMap<string, string[]>): Set<string> {
  const out = new Set<string>()
  const stack = [id]
  while (stack.length > 0) {
    const current = stack.pop() as string
    if (out.has(current)) {
      continue
    }
    out.add(current)
    for (const child of children.get(current) ?? []) {
      stack.push(child)
    }
  }
  return out
}

// computeRefCheckStates computes the tri-state checkbox state of every reference filter value. A
// value renders as a full checkmark when its own value is selected, when one of its ancestors is
// selected (selecting a value selects its whole subtree, including each narrower value and the
// "direct" entry), or when all of its children are checked. It is indeterminate when it is not
// fully checked but it or one of its descendants is selected. Selecting a parent and selecting all
// of its children therefore render identically.
export function computeRefCheckStates(values: readonly RefValueLike[], selected: ReadonlySet<string>): Map<string, RefCheckState> {
  const children = refChildrenByValue(values)
  const pathsById = new Map<string, string[][]>()
  for (const value of values) {
    pathsById.set(value.id, value.paths ?? [])
  }

  const ancestorSelected = (id: string): boolean => (pathsById.get(id) ?? []).some((path) => path.some((ancestor) => selected.has(ancestor)))

  const checkedMemo = new Map<string, boolean>()
  const isChecked = (id: string): boolean => {
    const cached = checkedMemo.get(id)
    if (cached !== undefined) {
      return cached
    }
    // Provisional false first, so an unexpected cycle in the value hierarchy cannot loop forever.
    checkedMemo.set(id, false)
    let result = selected.has(id) || ancestorSelected(id)
    if (!result) {
      const childIds = children.get(id)
      result = childIds !== undefined && childIds.length > 0 && childIds.every(isChecked)
    }
    checkedMemo.set(id, result)
    return result
  }

  const subtreeSelectedMemo = new Map<string, boolean>()
  const isSubtreeSelected = (id: string): boolean => {
    const cached = subtreeSelectedMemo.get(id)
    if (cached !== undefined) {
      return cached
    }
    subtreeSelectedMemo.set(id, false)
    let result = selected.has(id)
    if (!result) {
      result = (children.get(id) ?? []).some(isSubtreeSelected)
    }
    subtreeSelectedMemo.set(id, result)
    return result
  }

  const states = new Map<string, RefCheckState>()
  for (const value of values) {
    const checked = isChecked(value.id)
    states.set(value.id, { checked, indeterminate: !checked && isSubtreeSelected(value.id) })
  }
  return states
}

// toggleRefSelection returns the new selection after clicking the checkbox of value id. Clicking an
// unchecked or indeterminate value selects its whole subtree: the value, its narrower values and
// its "direct" entry. Clicking a fully checked value deselects that subtree: the selection is
// rewritten to the currently checked most-specific values (the leaves, "direct" and "missing"
// entries, i.e. values with no children) minus the deselected subtree. Re-expressing the selection
// at this granularity decomposes any broader ancestor selection that covered the value into its
// still-selected siblings, without changing which documents are matched. This makes deselecting a
// child behave the same whether the parent was stored explicitly (selected through the UI) or only
// as the parent value (for example a session created through the API): after the first change both
// yield the same selection and the same results.
export function toggleRefSelection(values: readonly RefValueLike[], id: string, selected: ReadonlySet<string>): Set<string> {
  const children = refChildrenByValue(values)
  const subtree = refSubtreeIds(id, children)
  const states = computeRefCheckStates(values, selected)

  if (!states.get(id)?.checked) {
    const next = new Set(selected)
    for (const valueId of subtree) {
      next.add(valueId)
    }
    return next
  }

  const next = new Set<string>()
  for (const value of values) {
    if ((children.get(value.id)?.length ?? 0) > 0) {
      continue
    }
    if (!states.get(value.id)?.checked || subtree.has(value.id)) {
      continue
    }
    next.add(value.id)
  }
  return next
}

// Approximate seconds-per-year used when picking a coarser-than-day precision.
// Exact-year math is unnecessary here. We only need the right order of magnitude.
const SECONDS_PER_YEAR = 60 * 60 * 24 * 365

export function formatValue(amount: number): string {
  return parseFloat(amount.toFixed(5)).toString()
}

export function clone<T>(input: T): Mutable<T> {
  // We are using lodash cloneDeep which supports symbols.
  return cloneDeep(toRaw(input))
}

export function equals<T>(a: T, b: T): boolean {
  return isEqual(a, b)
}

// timePrecisionForValue picks the coarsest display precision a single float64
// unix-second timestamp could plausibly be carrying. Anything with a fractional
// second part (beyond a small float64 tolerance) is sub-second and returns "s".
// Finer precisions are never returned. Otherwise we check divisibility by
// 60 / 3600 / 86400 for min/h/d, and then walk the calendar fields (and year
// divisibility) for the coarser tiers. Years within the four-digit range are
// never classified coarser than a year, mirroring inferYearPrecision in
// src/partials/input/InputTime.format.ts (and timePrecisionForValue on the
// backend): a value like 2000-01-01 comes from a year-precision claim in
// human-scale history, not a millennium one.
export function timePrecisionForValue(seconds: number): TimePrecision {
  // Tolerate small float64 rounding error when classifying "is this an integer
  // number of seconds?". For unix seconds in the human-relevant range the ULP
  // is well under this threshold.
  const tol = 1e-6
  if (Math.abs(seconds - Math.round(seconds)) >= tol) {
    return "s"
  }
  const sec = BigInt(Math.round(seconds))
  if (sec % 60n !== 0n) return "s"
  if (sec % (60n * 60n) !== 0n) return "min"
  if (sec % (60n * 60n * 24n) !== 0n) return "h"
  // Calendar units (months, years) do not have a fixed second count, so we
  // switch to inspecting the date components.
  const [year, month, day] = toDate(sec)
  if (day > 1) return "d"
  if (month > 1) return "m"
  if (year > -10_000 && year < 10_000) return "y"
  if (year % 10 !== 0) return "y"
  if (year % 100 !== 0) return "10y"
  if (year % 1_000 !== 0) return "100y"
  if (year % 10_000 !== 0) return "k"
  if (year % 100_000 !== 0) return "10k"
  if (year % 1_000_000 !== 0) return "100k"
  if (year % 10_000_000 !== 0) return "M"
  if (year % 100_000_000 !== 0) return "10M"
  if (year % 1_000_000_000 !== 0) return "100M"
  return "G"
}

// timePrecisionForRange picks a display precision that fits the span between
// two float64 unix-second timestamps. The result is capped at "s". Finer
// subsecond precisions are never returned even for very small spans.
export function timePrecisionForRange(from: number, to: number): TimePrecision {
  const delta = Math.abs(to - from)
  if (delta < 60 * 60) return "s"
  if (delta < 60 * 60 * 24) return "min"
  if (delta < 60 * 60 * 24 * 30) return "h"
  if (delta < SECONDS_PER_YEAR) return "d"
  const years = delta / SECONDS_PER_YEAR
  if (years < 10) return "m"
  if (years < 100) return "y"
  if (years < 1_000) return "10y"
  if (years < 10_000) return "100y"
  if (years < 100_000) return "k"
  if (years < 1_000_000) return "10k"
  if (years < 10_000_000) return "100k"
  if (years < 100_000_000) return "M"
  if (years < 1_000_000_000) return "10M"
  return "100M"
}

export function formatYearStr(year: number): string {
  if (year < 0) {
    return "-" + String(-year).padStart(4, "0")
  }
  return String(year).padStart(4, "0")
}

export function pad2(n: number | string): string {
  return String(n).padStart(2, "0")
}

// timeStringFromFloat64 converts a float64 unix-second timestamp into a claim
// Time string at the requested precision. Years coarser than "y" are rounded
// down so the result satisfies validatePrecision. Subsecond precisions are
// not supported.
export function timeStringFromFloat64(seconds: number, precision: TimePrecision): string {
  const sec = BigInt(Math.floor(seconds))
  const [year, month, day] = toDate(sec)
  const roundedYear = Math.floor(year / yearPrecisionMultiple(precision)) * yearPrecisionMultiple(precision)
  const yearStr = formatYearStr(roundedYear)
  switch (precision) {
    case "G":
    case "100M":
    case "10M":
    case "M":
    case "100k":
    case "10k":
    case "k":
    case "100y":
    case "10y":
    case "y":
      return yearStr
    case "m":
      return `${yearStr}-${pad2(month)}-00`
    case "d":
      return `${yearStr}-${pad2(month)}-${pad2(day)}`
    case "h":
      return `${yearStr}-${pad2(month)}-${pad2(day)} ${pad2(hour(sec))}:00`
    case "min":
      return `${yearStr}-${pad2(month)}-${pad2(day)} ${pad2(hour(sec))}:${pad2(minute(sec))}`
    case "s":
      return `${yearStr}-${pad2(month)}-${pad2(day)} ${pad2(hour(sec))}:${pad2(minute(sec))}:${pad2(second(sec))}`
    case "ms":
    case "us":
    case "ns":
      throw new Error(`subsecond precision "${precision}" is not supported`)
  }
}

// NAMING_PROPERTIES lists the properties considered for display labels.
// This matches the backend's naming properties (sub-properties of NAMING).
// TODO: Derive this dynamically from the property hierarchy instead of hard-coding.
const NAMING_PROPERTIES = [NAME, TITLE]

// getDisplayLabel returns the display label for a document's claims, using the
// current locale and language fallback chain.
//
// If claims contain an INSTANCE_OF claim which points to a class which has
// a display label function registered in the display label registry, then
// that function is used instead. In such case this same class should also have
// DISPLAY_LABEL_TEMPLATE defined to be used in the backend.
//
// This matches how makeDisplayStrings works in the backend, but for only one language.
export const getDisplayLabel: GetDisplayLabel = async function (claims, router, i18n, el, abortSignal, progress) {
  if (!claims) {
    return null
  }

  const displayLabelFunctions = getDisplayLabelFunctions()
  const refs = getClaimsOfTypeWithConfidence(claims, "ref", INSTANCE_OF)
  for (const ref of refs) {
    const displayLabelFunction = displayLabelFunctions.value.get(ref.to.id)
    if (displayLabelFunction) {
      return await displayLabelFunction(claims, router, i18n, el, abortSignal, progress)
    }
  }

  // Default implementation.
  return defaultDisplayLabel(claims, router, i18n, el, abortSignal, progress)
}

// eslint-disable-next-line @typescript-eslint/require-await
export const defaultDisplayLabel: GetDisplayLabel = async function (claims, router, i18n, el, abortSignal, progress) {
  if (!claims) {
    return null
  }

  const { locale } = i18n

  const claim = selectClaimsByLanguage(claims, "string", NAMING_PROPERTIES, locale.value, (claims) => !!(claims.length > 0 && claims[0].string))
  return claim?.[0].string ?? null
}

export function useLimitResults<T>(
  results: DeepReadonly<Ref<T[]>>,
  initialLimit: number,
  increase: number,
): {
  limitedResults: DeepReadonly<Ref<T[]>>
  hasMore: DeepReadonly<Ref<boolean>>
  loadMore: () => void
  loadAll: () => void
} {
  let limit = 0

  const _limitedResults = ref<T[]>([]) as Ref<T[]>
  const _hasMore = ref(false)
  const limitedResults = process.env.NODE_ENV !== "production" ? readonly(_limitedResults) : (_limitedResults as unknown as Readonly<Ref<readonly DeepReadonly<T>[]>>)
  const hasMore = process.env.NODE_ENV !== "production" ? readonly(_hasMore) : _hasMore

  watchEffect(() => {
    limit = Math.min(initialLimit, results.value.length)
    // If the last increase would be equal or less than SKIP_TO_END, just skip to the end.
    if (limit + SKIP_TO_END >= results.value.length) {
      limit = results.value.length
    }
    _hasMore.value = limit < results.value.length
    _limitedResults.value = results.value.slice(0, limit) as T[]
  })

  return {
    limitedResults,
    hasMore,
    loadMore: () => {
      limit = Math.min(limit + increase, results.value.length)
      // If the last increase would be equal or less than SKIP_TO_END, just skip to the end.
      if (limit + SKIP_TO_END >= results.value.length) {
        limit = results.value.length
      }
      _hasMore.value = limit < results.value.length
      _limitedResults.value = results.value.slice(0, limit) as T[]
    },
    loadAll: () => {
      limit = results.value.length
      _hasMore.value = false
      _limitedResults.value = results.value.slice(0, limit) as T[]
    },
  }
}

// We have to use complete class names for Tailwind to detect used classes and generating the
// corresponding CSS and do not do string interpolation or concatenation of partial class names.
// See: https://tailwindcss.com/docs/content-configuration#dynamic-class-names
const widthClasses = ["w-24", "w-32", "w-40", "w-48"]
const widthLongClasses = ["w-24", "w-32", "w-40", "w-48", "w-56", "w-64", "w-72", "w-80", "w-96"]
const heightShortClasses = ["h-0", "h-1/5", "h-2/5", "h-3/5", "h-4/5", "h-full"]

export function loadingWidth(seed: string): string {
  const rand = prng_alea(seed)
  return widthClasses[Math.floor(widthClasses.length * rand.quick())]
}

export function loadingLongWidth(seed: string): string {
  const rand = prng_alea(seed)
  return widthLongClasses[Math.floor(widthLongClasses.length * rand.quick())]
}

export function loadingShortHeight(seed: string): string {
  const rand = prng_alea(seed)
  return heightShortClasses[Math.floor(heightShortClasses.length * rand.quick())]
}

export function loadingShortHeights(seed: string, count: number): string[] {
  const rand = prng_alea(seed)
  const res = []
  let fullAdded = false
  for (let i = 0; i < count; i++) {
    res.push(heightShortClasses[Math.floor(heightShortClasses.length * rand.quick())])
    if (res[i] === heightShortClasses[heightShortClasses.length - 1]) {
      fullAdded = true
    }
  }
  if (!fullAdded) {
    // We want to make sure that at least one class in results is for full height.
    res[Math.floor(res.length * rand.quick())] = heightShortClasses[heightShortClasses.length - 1]
  }
  return res
}

export function useInitialLoad(progress: Ref<number>): { initialLoad: Ref<boolean>; laterLoad: Ref<boolean> } {
  const _initialLoad = ref<boolean>(false)
  const _laterLoad = ref<boolean>(false)
  const initialLoad = process.env.NODE_ENV !== "production" ? readonly(_initialLoad) : _initialLoad
  const laterLoad = process.env.NODE_ENV !== "production" ? readonly(_laterLoad) : _laterLoad

  let initialLoadDone = false
  watch(
    progress,
    (p) => {
      if (p > 0) {
        if (_initialLoad.value || _laterLoad.value) {
          return
        }
        if (initialLoadDone) {
          if (!_laterLoad.value) {
            _laterLoad.value = true
          }
        } else {
          if (!_initialLoad.value) {
            _initialLoad.value = true
          }
        }
      } else {
        if (_initialLoad.value) {
          _initialLoad.value = false
          initialLoadDone = true
        }
        if (_laterLoad.value) {
          _laterLoad.value = false
        }
      }
    },
    {
      immediate: true,
    },
  )

  return { initialLoad, laterLoad }
}

// encodeQuery should match implementation on the backend.
export function encodeQuery(query: QueryValuesWithOptional): QueryValues {
  const keys = []
  for (const key in query) {
    keys.push(key)
  }
  // We want keys in an alphabetical order (default in Go).
  keys.sort()

  const values: QueryValues = {}
  for (const key of keys) {
    const value = query[key]
    if (value === undefined) {
      continue
    } else if (value === null) {
      // In contrast with Vue Router, we convert null values to an empty string because Go
      // does not support bare parameters without = and waf would then normalize them anyway.
      values[key] = ""
    } else if (Array.isArray(value)) {
      const vs: string[] = []
      for (const v of value) {
        if (v === null) {
          vs.push("")
        } else {
          vs.push(v)
        }
      }
      if (vs.length > 0) {
        values[key] = vs
      }
    } else {
      values[key] = value
    }
  }

  return values
}

// delay resolves after ms milliseconds, or throws the signal's abort reason
// if the signal aborts (or is already aborted) before then.
export async function delay(ms: number, signal?: AbortSignal): Promise<void> {
  await new Promise<void>((resolve) => {
    if (signal?.aborted) {
      resolve()
      return
    }
    const t = setTimeout(() => {
      signal?.removeEventListener("abort", onAbort)
      resolve()
    }, ms)
    function onAbort() {
      clearTimeout(t)
      resolve()
    }
    signal?.addEventListener("abort", onAbort, { once: true })
  })
  signal?.throwIfAborted()
}

// Schemes accepted by parseUrl. Mirrors the schemes validateURL accepts in
// document/urls.go on the backend. Link validation uses this set for <a href>
// and the set minus the contact schemes (mailto and tel) for <blockquote cite>;
// callers (and validateUrl) make the distinction by passing { allowContact: false },
// keeping both sides in sync.
export const ALLOWED_LINK_CLAIM_SCHEMES = ["http:", "https:", "mailto:", "tel:"] as const

const URL_HOST_REGEX = /^https?:\/\/\//i

// Options accepted by parseUrl (and forwarded by normalizeUrl).
export type ParseUrlOptions = {
  // Defaults to true.
  allowContact?: boolean
}

// parseUrl parses an input URL and validates it against the project's link
// allowlist. It accepts:
//   - Same-origin paths starting with "/" (but not "//"): "/foo", "/a?b=c#d", "/"
//   - Absolute URLs whose scheme is in ALLOWED_LINK_CLAIM_SCHEMES (the contact
//     schemes mailto and tel excluded when options.allowContact is false).
//
// It throws on:
//   - Empty input
//   - Unparseable input (via the URL constructor)
//   - Protocol-relative URLs ("//host/path")
//   - Document-relative paths ("foo", "../foo")
//   - Fragment-only refs ("#section")
//   - Absolute URLs with any other scheme (javascript:, data:, ftp:, ...)
//   - Degenerate forms like "http:///x" (the WHATWG URL parser silently
//     normalizes those to "http://x/", moving the path into the host; we
//     reject before parsing so the backend, which does not normalize, sees
//     the same outcome)
//   - Bare "mailto:" with no address, or bare "tel:" with no number.
//
// Leading-slash paths are resolved against window.location.href when
// available so downstream same-origin checks (normalizeUrl, classifyLink,
// matchStorageRoute) compare against the current document's origin. In
// environments without window (Node, isolated tests) a synthetic base is
// used; the validation rules are syntactic, so the same-origin information
// is simply not meaningful there.
export function parseUrl(input: string, { allowContact = true }: ParseUrlOptions = {}): URL {
  if (!input) {
    throw new Error("empty URL")
  }
  if (input.startsWith("/") && !input.startsWith("//")) {
    // For claim validation we might want that it works also outside browser in other JS environments.
    // normalizeUrl which is used when displaying the link still uses only window.location.
    const base = typeof window !== "undefined" ? window.location.href : "http://example.invalid/"
    return new URL(input, base)
  }
  // The "///" guard is anchored to the raw input. The URL constructor
  // would otherwise rewrite "http:///x" to "http://x/" and we would lose
  // the chance to reject it.
  if (URL_HOST_REGEX.test(input)) {
    throw new Error("invalid URL: missing host")
  }
  const url = new URL(input)
  if (!ALLOWED_LINK_CLAIM_SCHEMES.includes(url.protocol)) {
    throw new Error(`disallowed URL scheme: ${url.protocol}`)
  }
  if (!allowContact && (url.protocol === "mailto:" || url.protocol === "tel:")) {
    throw new Error(`disallowed URL scheme: ${url.protocol}`)
  }
  // The URL constructor accepts "mailto:" with no address. Reject it.
  if (url.protocol === "mailto:" && !url.pathname) {
    throw new Error("invalid URL: missing address")
  }
  // The URL constructor accepts "tel:" with no number. Reject it.
  if (url.protocol === "tel:" && !url.pathname) {
    throw new Error("invalid URL: missing number")
  }
  return url
}

// validateUrl reports whether input is an acceptable URL, by parsing it with parseUrl and ignoring the
// result (true when parseUrl does not throw). It is the validity check for the editor schema's link
// attributes, so they go through the same parsing and classification as LinkClaim IRIs rather than a
// separate regex. It is the boolean counterpart of validateURL on the backend, which returns an error
// instead of a boolean following the Go validator convention.
export function validateUrl(input: string, options: ParseUrlOptions = {}): boolean {
  try {
    parseUrl(input, options)
    return true
  } catch {
    return false
  }
}

// normalizeUrl returns the canonical string for a URL. Same-origin URLs are
// collapsed to "/path?query#hash" so they match the leading-slash convention
// used by InputFile (which stores StorageGet routes as paths) and by Link.vue
// (which shows internal links as paths). External URLs are re-stringified
// through the URL constructor (lowercase host, default port stripped,
// trailing slash on bare origins, etc.). Idempotent: passing an already
// normalized value back through normalizeUrl returns it unchanged.
// Throws (via parseUrl) on input not in the allowed-link form.
export function normalizeUrl(input: string, options: ParseUrlOptions = {}): string {
  const url = parseUrl(input, options)
  if (url.origin === window.location.origin) {
    return url.pathname + url.search + url.hash
  }
  return url.toString()
}

// raceWithSignal settles as soon as the given promise settles or the signal
// aborts. A settling promise propagates its resolution or rejection through
// unchanged. An abort resolves with undefined (no error is raised).
export function raceWithSignal<T>(promise: Promise<T>, signal: AbortSignal): Promise<T | undefined> {
  if (signal.aborted) return Promise.resolve(undefined)
  let onAbort: (() => void) | undefined
  const abortPromise = new Promise<undefined>((resolve) => {
    onAbort = () => {
      onAbort = undefined
      resolve(undefined)
    }
    signal.addEventListener("abort", onAbort, { once: true })
  })
  return Promise.race<T | undefined>([promise, abortPromise]).finally(() => {
    if (onAbort) signal.removeEventListener("abort", onAbort)
  })
}

// Polyfill for AbortSignal.any.
export function anySignal(...signals: AbortSignal[]): AbortSignal {
  if ("any" in AbortSignal) {
    return AbortSignal.any(signals)
  }

  const controller = new AbortController()

  for (const signal of signals) {
    if (signal.aborted) {
      controller.abort()
      return signal
    }

    signal.addEventListener("abort", () => controller.abort(signal.reason), {
      signal: controller.signal,
    })
  }

  return controller.signal
}

export function useOnScrollOrResize(el: Ref<Element | null>, callback: () => void) {
  const resizeObserver = new ResizeObserver(callback)

  watch(el, (newEl, oldEl) => {
    if (oldEl) {
      resizeObserver.unobserve(oldEl)
    }
    if (newEl) {
      resizeObserver.observe(newEl)
    }
  })

  onMounted(() => {
    window.addEventListener("scroll", callback, { passive: true })
    window.addEventListener("resize", callback, { passive: true })
  })

  onBeforeUnmount(() => {
    window.removeEventListener("scroll", callback)
    window.removeEventListener("resize", callback)

    resizeObserver.disconnect()
  })
}

export function redirectServerSide(url: string, replace: boolean, lock: Ref<number>) {
  // We increase the lock and never decrease it to wait for browser to do the redirect.
  lock.value += 1

  // We do not use Vue Router to force a server-side request which might return updated cookies
  // or redirect on its own somewhere because of new (or lack thereof) cookies.
  if (replace) {
    window.location.replace(url)
  } else {
    window.location.assign(url)
  }
}

// currentAbsoluteURL returns the current location stripped of its origin (the
// path + search + hash). Sign-in flow persist this so the user lands back
// where they were after sign-in.
export function currentAbsoluteURL(): string {
  return document.location.href.slice(document.location.origin.length)
}

// replaceLocationSearch replaces the current URL's query string in-place via
// history.replaceState (no navigation, no page reload). The OIDC callback
// handler uses this to scrub the "state" and "code" params from the URL once
// they have been consumed.
export function replaceLocationSearch(search: string) {
  if (history.replaceState) {
    const url = new URL(window.location.href)
    url.search = search ? "?" + search : ""
    history.replaceState(null, "", url)
    return
  }
  window.location.search = search ? "?" + search : ""
}

// asyncToReactive converts an async function to a reactive value.
//
// Reactivity is tracked until the first await.
export function asyncToReactive<T>(fn: () => Promise<T>): Ref<{ loading: true } | { error: unknown } | T> {
  const result = shallowRef<{ loading: true } | { error: unknown } | T>({ loading: true })
  watchEffect(() => {
    fn()
      .then((value) => {
        result.value = value
      })
      .catch((error) => {
        result.value = { error: error }
      })
  })
  return result
}

// isLoading works on both Refs and unwrapped values.
export function isLoading(result: Ref<{ loading: true } | unknown> | { loading: true } | unknown) {
  if (!result) {
    return false
  }
  if (typeof result === "object" && "value" in result) {
    if (!result.value) {
      return false
    }
    if (typeof result.value !== "object") {
      return false
    }
    return "loading" in result.value && result.value.loading
  } else if (typeof result !== "object") {
    return false
  }
  return "loading" in result && result.loading
}

// getError works on both Refs and unwrapped values.
export function getError(result: Ref<{ error: unknown } | unknown> | { error: unknown } | unknown): unknown {
  if (!result) {
    return ""
  }
  if (typeof result === "object" && "value" in result) {
    if (!result.value) {
      return ""
    }
    if (typeof result.value !== "object") {
      return ""
    }
    if ("error" in result.value) {
      // A side effect, but still useful for debugging.
      console.error("getError", result.value.error)
      return result.value.error
    }
  } else if (typeof result !== "object") {
    return false
  } else if ("error" in result) {
    // A side effect, but still useful for debugging.
    console.error("getError", result.error)
    return result.error
  }
  return ""
}

export async function makeAddClaimChange(base: DeepReadonly<string[]>, session: string, changeIndex: number, patch: object, under?: string) {
  const changeBase = [...base, "SESSION", session, String(changeIndex)]
  const claimID = (await Identifier.from(...changeBase)).toString()
  return new AddClaimChange({
    id: claimID,
    base: changeBase,
    patch,
    ...(under ? { under } : {}),
  })
}
