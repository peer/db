import type { DeepReadonly, Ref } from "vue"

import type { TimePrecision } from "@/document"
import type { GetDisplayLabel, Mutable, QueryValues, QueryValuesWithOptional } from "@/types"

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

// If the last increase would be equal or less than this number, just skip to the end.
const SKIP_TO_END = 2

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

// TODO: Use it in InputTime.vue.
export function formatYearStr(year: number): string {
  if (year < 0) {
    return "-" + String(-year).padStart(4, "0")
  }
  return String(year).padStart(4, "0")
}

// TODO: Use it in InputTime.vue.
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

export function redirectServerSide(url: string, replace: boolean, progress: Ref<number>) {
  // We increase the progress and never decrease it to wait for browser to do the redirect.
  progress.value += 1

  // We do not use Vue Router to force a server-side request which might return updated cookies
  // or redirect on its own somewhere because of new (or lack thereof) cookies.
  if (replace) {
    window.location.replace(url)
  } else {
    window.location.assign(url)
  }
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

export async function makeAddClaimChange(base: DeepReadonly<string[]>, session: string, changeIndex: number, patch: object) {
  const changeBase = [...base, "SESSION", session, String(changeIndex)]
  const claimID = (await Identifier.from(...changeBase)).toString()
  return new AddClaimChange({
    id: claimID,
    base: changeBase,
    patch,
  })
}
