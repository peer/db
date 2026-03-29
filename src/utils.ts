import type { DeepReadonly, Ref } from "vue"

import type { GetDisplayLabel, Mutable, QueryValues, QueryValuesWithOptional } from "@/types"

import { prng_alea } from "esm-seedrandom"
import { cloneDeep, isEqual } from "lodash-es"
import { onBeforeUnmount, onMounted, readonly, ref, shallowRef, toRaw, watch, watchEffect } from "vue"

import { INSTANCE_OF, NAME, TITLE } from "@/core"
import { getClaimsOfTypeWithConfidence, selectClaimsByLanguage } from "@/document"
import { getDisplayLabelFunctions } from "@/registry/display-label"
import { fromDate, hour, minute, second, toDate } from "@/time"

// If the last increase would be equal or less than this number, just skip to the end.
const SKIP_TO_END = 2

const timeRegex = /^([+-]?\d{4,})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})Z$/

export function formatValue(amount: number): string {
  return parseFloat(amount.toFixed(5)).toString()
}

export function formatTime(seconds: number): string {
  // TODO: Support also nanoseconds.
  return secondsToTime(BigInt(Math.round(seconds)))
}

export function parseTime(value: string): number {
  return Number(timeToSeconds(value))
}

// TODO: Support also nanoseconds.
// TODO: Return float.
export function timeToSeconds(value: string): bigint {
  const match = timeRegex.exec(value)
  if (!match) {
    throw new Error(`unable to parse time "${value}"`)
  }
  const year = parseInt(match[1], 10)
  if (isNaN(year)) {
    throw new Error(`unable to parse year "${value}"`)
  }
  const month = parseInt(match[2], 10)
  if (isNaN(month)) {
    throw new Error(`unable to parse month "${value}"`)
  }
  const day = parseInt(match[3], 10)
  if (isNaN(day)) {
    throw new Error(`unable to parse day "${value}"`)
  }
  const hour = parseInt(match[4], 10)
  if (isNaN(hour)) {
    throw new Error(`unable to parse hour "${value}"`)
  }
  const minute = parseInt(match[5], 10)
  if (isNaN(minute)) {
    throw new Error(`unable to parse minute "${value}"`)
  }
  const second = parseInt(match[6], 10)
  if (isNaN(second)) {
    throw new Error(`unable to parse second "${value}"`)
  }
  return fromDate(year, month, day, hour, minute, second)
}

export function secondsToTime(value: bigint): string {
  const [year, month, day] = toDate(value)
  let yearStr
  if (year < 0) {
    yearStr = "-" + String(-year).padStart(4, "0")
  } else {
    yearStr = String(year).padStart(4, "0")
  }
  return `${yearStr}-${String(month).padStart(2, "0")}-${String(day).padStart(2, "0")}T${String(hour(value)).padStart(2, "0")}:${String(minute(value)).padStart(
    2,
    "0",
  )}:${String(second(value)).padStart(2, "0")}Z`
}

export function clone<T>(input: T): Mutable<T> {
  // We are using lodash cloneDeep which supports symbols.
  return cloneDeep(toRaw(input))
}

export function equals<T>(a: T, b: T): boolean {
  return isEqual(a, b)
}

export function bigIntMax(a: bigint, b: bigint): bigint {
  if (a > b) {
    return a
  }
  return b
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
export const getDisplayLabel: GetDisplayLabel = async function (claims, language) {
  if (!claims) {
    return null
  }

  const displayLabelFunctions = getDisplayLabelFunctions()
  const refs = getClaimsOfTypeWithConfidence(claims, "ref", INSTANCE_OF)
  for (const ref of refs) {
    const displayLabelFunction = displayLabelFunctions.value.get(ref.to.id)
    if (displayLabelFunction) {
      return await displayLabelFunction(claims, language)
    }
  }

  // Default implementation.
  const claim = selectClaimsByLanguage(claims, "string", NAMING_PROPERTIES, language, (claims) => {
    if (claims.length > 0 && claims[0].string) {
      return true
    }
    return false
  })
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
