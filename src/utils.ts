import type { Mutable } from "@/types"

import { toRaw } from "vue"
import { fromDate, toDate, hour, minute, second } from "@/time"

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
