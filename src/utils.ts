import type { DeepReadonly } from "vue"
import type { Mutable, Claim, ClaimTypes, Required, Router } from "@/types"

import { toRaw } from "vue"
import { cloneDeep, isEqual } from "lodash-es"
import { useRouter as useVueRouter } from "vue-router"
import { fromDate, toDate, hour, minute, second } from "@/time"
import { LIST, ORDER } from "@/props"

const timeRegex = /^([+-]?\d{4,})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})Z$/

// TODO: Improve by using size prefixes for some units (e.g., KB).
//       Both for large and small numbers (e.g., micro gram).
export function formatValue(value: number, unit: string): string {
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

export function useRouter(): Router {
  return useVueRouter() as Router
}
