import type { Mutable, Claim, ClaimTypes, Required } from "@/types"

import { toRaw } from "vue"
import { v5 as uuidv5, parse as uuidParse } from "uuid"
import bs58 from "bs58"
import { fromDate, toDate, hour, minute, second } from "@/time"

const timeRegex = /^([+-]?\d{4,})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})Z$/
const idLength = 22
const nameSpaceStandardProperties = "34cd10b4-5731-46b8-a6dd-45444680ca62"
const nameSpaceWikidata = "8f8ba777-bcce-4e45-8dd4-a328e6722c82"

const LIST = getStandardPropertyID("LIST")
const ORDER = getStandardPropertyID("ORDER")

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
  if (typeof structuredClone !== "undefined") {
    return structuredClone(toRaw(input))
  } else {
    return JSON.parse(JSON.stringify(input))
  }
}

export function bigIntMax(a: bigint, b: bigint): bigint {
  if (a > b) {
    return a
  }
  return b
}

function identifierFromUUID(uuid: string): string {
  const res = bs58.encode(uuidParse(uuid) as Uint8Array)
  if (res.length < idLength) {
    return "1".repeat(idLength - res.length) + res
  }
  return res
}

function getID(namespace: string, ...args: string[]): string {
  let res = namespace
  for (const arg of args) {
    res = uuidv5(arg, res)
  }
  return identifierFromUUID(res)
}

export function getStandardPropertyID(mnemonic: string): string {
  return getID(nameSpaceStandardProperties, mnemonic)
}

export function getWikidataDocumentID(id: string): string {
  return getID(nameSpaceWikidata, id)
}

export function getBestClaim(claimTypes: ClaimTypes | undefined | null, propertyId: string | string[]): Claim | null {
  if (typeof propertyId === "string") {
    propertyId = [propertyId]
  }
  const claims: Claim[] = []
  for (const claims of Object.values(claimTypes ?? {})) {
    for (const claim of claims || []) {
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
  claimTypes: ClaimTypes | undefined | null,
  claimType: K,
  propertyId: string | string[],
): Required<ClaimTypes>[K][number][] {
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
  claimTypes: ClaimTypes | undefined | null,
  claimType: K,
  propertyId: string | string[],
): Required<ClaimTypes>[K][number] | null {
  const claims = getClaimsOfType(claimTypes, claimType, propertyId)
  if (claims.length > 0) {
    return claims[0]
  }
  return null
}

// TODO: Handle sub-lists. Children lists should be nested and not just added as additional lists to the list of lists.
// TODO: Sort lists between themselves by (average) confidence?
export function getClaimsListsOfType<K extends keyof ClaimTypes>(
  claimTypes: ClaimTypes | undefined | null,
  claimType: K,
  propertyId: string | string[],
): Required<ClaimTypes>[K][number][][] {
  const claims = getClaimsOfType(claimTypes, claimType, propertyId)
  const claimsPerList: Record<string, [Required<ClaimTypes>[K][number], number][]> = {}
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
