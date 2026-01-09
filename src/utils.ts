import type { DeepReadonly, Ref } from "vue"

import type { Claim, ClaimTypeName, ClaimTypes } from "@/document"
import type { AmountUnit, Mutable, QueryValues, QueryValuesWithOptional, Required } from "@/types"

import { prng_alea } from "esm-seedrandom"
import { cloneDeep, isEqual } from "lodash-es"
import { onBeforeUnmount, onMounted, readonly, ref, toRaw, watch, watchEffect } from "vue"

import { DESCRIPTION, LIST, NAME, ORDER } from "@/props"
import { fromDate, hour, minute, second, toDate } from "@/time"

// If the last increase would be equal or less than this number, just skip to the end.
const SKIP_TO_END = 2

const timeRegex = /^([+-]?\d{4,})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})Z$/

// TODO: Improve by using size prefixes for some units (e.g., KB).
//       Both for large and small numbers (e.g., micro gram).
export function formatValue(amount: number, unit: AmountUnit): string {
  let res = parseFloat(amount.toPrecision(5)).toString()
  if (unit !== "1") {
    res += " " + unit
  }
  return res
}

// TODO: Improve by using size prefixes for some units (e.g., KB).
//       Both for large and small numbers (e.g., micro gram).
export function formatRange(lower: number, upper: number, unit: AmountUnit): string {
  const l = parseFloat(lower.toPrecision(5)).toString()
  const u = parseFloat(lower.toPrecision(5)).toString()
  let res = l + "–" + u
  if (unit !== "1") {
    res += " " + unit
  }
  return res
}

export function formatTime(seconds: bigint): string {
  return secondsToTimestamp(seconds)
}

export function timestampToSeconds(value: string): bigint {
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

export function secondsToTimestamp(value: bigint): string {
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

export function getBestClaim(claimTypes: DeepReadonly<ClaimTypes> | undefined | null, propertyId: string | string[]): DeepReadonly<Claim> | null {
  if (!Array.isArray(propertyId)) {
    propertyId = [propertyId]
  }
  const claims: DeepReadonly<Claim>[] = []
  for (const claim of claimTypes?.AllClaims() ?? []) {
    if (propertyId.includes(claim.prop.id)) {
      claims.push(claim)
    }
  }
  claims.sort((a, b) => b.confidence - a.confidence)
  if (claims.length > 0) {
    return claims[0]
  }
  return null
}

export function getClaimsOfType<K extends ClaimTypeName>(
  claimTypes: DeepReadonly<ClaimTypes> | undefined | null,
  claimType: K,
  propertyId: string | string[],
): Required<DeepReadonly<ClaimTypes>>[K][number][] {
  if (!claimTypes) return []
  if (!Array.isArray(propertyId)) {
    propertyId = [propertyId]
  }
  const claims = []
  for (const claim of claimTypes[claimType] ?? []) {
    if (propertyId.includes(claim.prop.id)) {
      claims.push(claim)
    }
  }
  claims.sort((a, b) => b.confidence - a.confidence)
  return claims
}

export function getBestClaimOfType<K extends ClaimTypeName>(
  claimTypes: DeepReadonly<ClaimTypes> | undefined | null,
  claimType: K,
  propertyId: string | string[],
): Required<DeepReadonly<ClaimTypes>>[K][number] | null {
  const claims = getClaimsOfType(claimTypes, claimType, propertyId)
  if (claims.length > 0) {
    return claims[0]
  }
  return null
}

const LOW_CONFIDENCE = 0.5

// TODO: Support also negation claims (i.e., those with negative confidence).
export function getClaimsOfTypeWithConfidence<K extends ClaimTypeName>(
  claimTypes: DeepReadonly<ClaimTypes> | undefined | null,
  claimType: K,
  propertyId: string | string[],
  confidence: number = LOW_CONFIDENCE,
): Required<DeepReadonly<ClaimTypes>>[K][number][] {
  const claims = getClaimsOfType(claimTypes, claimType, propertyId)
  return claims.filter((claim) => claim.confidence >= confidence)
}

// TODO: Handle sub-lists. Children lists should be nested and not just added as additional lists to the list of lists.
// TODO: Sort lists between themselves by (average) confidence?
export function getClaimsListsOfType<K extends ClaimTypeName>(
  claimTypes: DeepReadonly<ClaimTypes> | undefined | null,
  claimType: K,
  propertyId: string | string[],
): Required<DeepReadonly<ClaimTypes>>[K][number][][] {
  const claims = getClaimsOfType(claimTypes, claimType, propertyId)
  const claimsPerList: Record<string, [Required<DeepReadonly<ClaimTypes>>[K][number], number][]> = {}
  for (const claim of claims) {
    const list = getBestClaimOfType(claim.meta, "id", LIST)?.id || "none"
    const order = getBestClaimOfType(claim.meta, "amount", ORDER)?.amount ?? Number.MAX_VALUE
    if (!(list in claimsPerList)) {
      claimsPerList[list] = []
    }
    claimsPerList[list].push([claim, order])
  }
  const res = []
  for (const c of Object.values(claimsPerList)) {
    res.push(c.sort(([c1, o1], [c2, o2]) => o1 - o2).map(([c, o]) => c))
  }
  return res
}

export function getName(claimTypes: DeepReadonly<ClaimTypes> | undefined | null): string | null {
  let claim = getBestClaimOfType(claimTypes, "text", NAME)
  if (claim) {
    return claim.html.en
  }

  claim = getBestClaimOfType(claimTypes, "text", DESCRIPTION)
  if (claim) {
    return claim.html.en
  }

  return null
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
  const limitedResults = import.meta.env.DEV ? readonly(_limitedResults) : (_limitedResults as unknown as Readonly<Ref<readonly DeepReadonly<T>[]>>)
  const hasMore = import.meta.env.DEV ? readonly(_hasMore) : _hasMore

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
  const initialLoad = import.meta.env.DEV ? readonly(_initialLoad) : _initialLoad
  const laterLoad = import.meta.env.DEV ? readonly(_laterLoad) : _laterLoad

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
