import { DeepReadonly, Ref, watchEffect } from "vue"
import type { Mutable, Claim, ClaimTypes, Required, Router, AmountUnit } from "@/types"

import { toRaw, ref, readonly, watch } from "vue"
import { cloneDeep, isEqual } from "lodash-es"
import { prng_alea } from "esm-seedrandom"
import { useRouter as useVueRouter } from "vue-router"
import { fromDate, toDate, hour, minute, second } from "@/time"
import { LIST, ORDER, NAME } from "@/props"

// If the last increase would be equal or less than this number, just skip to the end.
const SKIP_TO_END = 2

const timeRegex = /^([+-]?\d{4,})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})Z$/

// TODO: Improve by using size prefixes for some units (e.g., KB).
//       Both for large and small numbers (e.g., micro gram).
export function formatValue(value: number, unit: AmountUnit): string {
  let res = parseFloat(value.toPrecision(5)).toString()
  if (unit !== "1") {
    res += unit
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
  if (typeof propertyId === "string") {
    propertyId = [propertyId]
  }
  const claims: DeepReadonly<Claim>[] = []
  for (const cs of Object.values(claimTypes ?? {})) {
    for (const claim of cs || []) {
      if (propertyId.includes(claim.prop._id)) {
        claims.push(claim)
      }
    }
  }
  claims.sort((a, b) => b.confidence - a.confidence)
  if (claims.length > 0) {
    return claims[0]
  }
  return null
}

export function getClaimsOfType<K extends keyof ClaimTypes>(
  claimTypes: DeepReadonly<ClaimTypes> | undefined | null,
  claimType: K,
  propertyId: string | string[],
): Required<DeepReadonly<ClaimTypes>>[K][number][] {
  if (typeof propertyId === "string") {
    propertyId = [propertyId]
  }
  const claims = []
  for (const claim of claimTypes?.[claimType] || []) {
    if (propertyId.includes(claim.prop._id)) {
      claims.push(claim)
    }
  }
  claims.sort((a, b) => b.confidence - a.confidence)
  return claims
}

export function getBestClaimOfType<K extends keyof ClaimTypes>(
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

// TODO: Handle sub-lists. Children lists should be nested and not just added as additional lists to the list of lists.
// TODO: Sort lists between themselves by (average) confidence?
export function getClaimsListsOfType<K extends keyof ClaimTypes>(
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
  const claim = getBestClaimOfType(claimTypes, "text", NAME)
  if (!claim) {
    return null
  }

  return claim.html.en
}

export function useRouter(): Router {
  return useVueRouter() as Router
}

export function useLimitResults<Type>(
  results: DeepReadonly<Ref<Type[]>>,
  initialLimit: number,
  increase: number,
): {
  limitedResults: DeepReadonly<Ref<Type[]>>
  hasMore: DeepReadonly<Ref<boolean>>
  loadMore: () => void
} {
  let limit = 0

  const _limitedResults = ref<Type[]>([]) as Ref<Type[]>
  const _hasMore = ref(false)
  const limitedResults = import.meta.env.DEV ? readonly(_limitedResults) : (_limitedResults as unknown as Readonly<Ref<readonly DeepReadonly<Type>[]>>)
  const hasMore = import.meta.env.DEV ? readonly(_hasMore) : _hasMore

  watchEffect(() => {
    limit = Math.min(initialLimit, results.value.length)
    // If the last increase would be equal or less than SKIP_TO_END, just skip to the end.
    if (limit + SKIP_TO_END >= results.value.length) {
      limit = results.value.length
    }
    _hasMore.value = limit < results.value.length
    _limitedResults.value = results.value.slice(0, limit) as Type[]
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
      _limitedResults.value = results.value.slice(0, limit) as Type[]
    },
  }
}

// We have to use complete class names for Tailwind to detect used classes and generating the
// corresponding CSS and do not do string interpolation or concatenation of partial class names.
// See: https://tailwindcss.com/docs/content-configuration#dynamic-class-names
const widthClasses = ["w-24", "w-32", "w-40", "w-48"]
const widthShortClasses = ["w-4", "w-8", "w-12", "w-16"]
const heightShortClasses = ["h-0", "h-1/5", "h-2/5", "h-3/5", "h-4/5", "h-full"]

export function loadingWidth(seed: string): string {
  const rand = prng_alea(seed)
  return widthClasses[Math.floor(widthClasses.length * rand.quick())]
}

export function loadingShortWidth(seed: string): string {
  const rand = prng_alea(seed)
  return widthShortClasses[Math.floor(widthShortClasses.length * rand.quick())]
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
