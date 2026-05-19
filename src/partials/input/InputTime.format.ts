import type { NamedValue } from "vue-i18n"

import type { TimePrecision } from "@/document"

import { PRECISION_LEVEL } from "@/document/time"
import { daysIn } from "@/time"
import { formatYearStr, pad2 } from "@/utils"

const DATE_TIME_WHITESPACE_TRIM_REGEX = /(-?\d+)\s*-\s*(\d{1,2})\s*-\s*(\d{1,2})\s+([0-9])/g
const FIRST_LOWERCASE_T_REGEX = /t/
const ALL_WHITESPACE_REGEX = /\s+/g
const T_TO_SPACE = /(\d{4}-\d{1,2}-\d{1,2})T(?=\d)/
const TRAILING_DASH_YEAR_MONTH = /^(\d{4}|\d{4}-\d{1,2})-\s*$/
const TRAILING_T = /(\d{4}-\d{1,2}-\d{1,2})T\s*$/
const TRAILING_SEMICOLON = /(?:^|\s)(\d{1,2}(?::\d{1,2})?):\s*$/

const YEAR_RE = /^(-?\d+)$/
const MONTH_RE = /^(-?\d+)-(\d{1,2})$/
const DAY_RE = /^(-?\d+)-(\d{1,2})-(\d{1,2})$/
const HOUR_RE = /^(-?\d+)-(\d{1,2})-(\d{1,2}) (\d{1,2})$/
const MINUTE_RE = /^(-?\d+)-(\d{1,2})-(\d{1,2}) (\d{1,2}):(\d{1,2})$/
const SECOND_RE = /^(-?\d+)-(\d{1,2})-(\d{1,2}) (\d{1,2}):(\d{1,2}):(\d{1,2})$/
const MS_RE = /^(-?\d+)-(\d{1,2})-(\d{1,2}) (\d{1,2}):(\d{1,2}):(\d{1,2})\.(\d{3})$/
const US_RE = /^(-?\d+)-(\d{1,2})-(\d{1,2}) (\d{1,2}):(\d{1,2}):(\d{1,2})\.(\d{6})$/
const NS_RE = /^(-?\d+)-(\d{1,2})-(\d{1,2}) (\d{1,2}):(\d{1,2}):(\d{1,2})\.(\d{9})$/

const YEAR_IN_PROGRESS_REGEX = /^-?\d*$/
const MONTH_IN_PROGRESS_REGEX = /^-?\d+-\d{0,2}$/
const DAY_IN_PROGRESS_REGEX = /^-?\d+-\d{1,2}-\d{0,2}$/
const MINUTES_IN_PROGRESS_REGEX = /^-?\d+-\d{1,2}-\d{1,2} \d{1,2}$/
const SECONDS_IN_PROGRESS_REGEX = /^-?\d+-\d{1,2}-\d{1,2} \d{1,2}:\d{1,2}$/
const SUBSECONDS_IN_PROGRESS_REGEX = /^-?\d+-\d{1,2}-\d{1,2} \d{1,2}:\d{1,2}:\d{1,2}\.\d{0,9}$/

const TRAILING_DASH_REGEX = /-$/

const matchToYear = (s: string) => s.match(YEAR_RE)
const matchToMonth = (s: string) => s.match(MONTH_RE)
const matchToDay = (s: string) => s.match(DAY_RE)
const matchToHour = (s: string) => s.match(HOUR_RE)
const matchToMinute = (s: string) => s.match(MINUTE_RE)
const matchToSecond = (s: string) => s.match(SECOND_RE)
const matchToMs = (s: string) => s.match(MS_RE)
const matchToUs = (s: string) => s.match(US_RE)
const matchToNs = (s: string) => s.match(NS_RE)

/**
 * Normalizes raw user input into a canonical, progressively-parseable
 * datetime string.
 *
 * This function is intentionally permissive. It accepts loosely formatted
 * date/time input and converts it into a stable canonical form suitable
 * for parsing.
 *
 * Canonical output format:
 *  - Format is `YYYY-MM-DD HH:MM:SS`.
 * - Date and time components may be partial.
 * - Date and time are separated by a single space.
 * - Output contains no `T`, repeated whitespace, or trailing separators.
 *
 * Guarantees:
 * - Collapses whitespace.
 * - Normalizes `T` / `t` to a space.
 * - Removes trailing `-`, `T`, and `:`.
 *
 * Non-goals:
 * - Does not validate semantic correctness.
 * - Does not pad numeric components.
 *
 * @param raw - Raw user input string.
 *
 * @returns A normalized datetime string in canonical format, or an empty
 *          string if the input is empty or whitespace.
 *
 * @example
 * normalizeForParsing("2022")                  // "2022"
 * normalizeForParsing("2022-")                 // "2022"
 * normalizeForParsing("2022-01-")              // "2022-01"
 *
 * @example
 * normalizeForParsing("2022-01-01T")            // "2022-01-01"
 * normalizeForParsing("2022-01-01T12:34:")      // "2022-01-01 12:34"
 * normalizeForParsing("2022-1-1t9:5")           // "2022-1-1 9:5"
 *
 * @example
 * normalizeForParsing("  2022   ")              // "2022"
 * normalizeForParsing("2022-01-01    12:00")    // "2022-01-01 12:00"
 */
export function normalizeForParsing(raw: string): string {
  if (!raw) return ""

  let r = raw

  // Normalize date + time boundary whitespace.
  r = r.replace(DATE_TIME_WHITESPACE_TRIM_REGEX, "$1-$2-$3 $4")

  // Normalize lowercase 't' to 'T'.
  r = r.replace(FIRST_LOWERCASE_T_REGEX, "T")

  // Convert only valid date–time boundary T to space.
  r = r.replace(T_TO_SPACE, "$1 ")

  // Cosmetic whitespace cleanup.
  r = r.replace(ALL_WHITESPACE_REGEX, " ").trim()

  // Accepts 'YYYY-' or 'YYYY-MM-'.
  r = r.replace(TRAILING_DASH_YEAR_MONTH, "$1")

  // Converts 'YYYY-MM-DDT' to 'YYYY-MM-DD'.
  r = r.replace(TRAILING_T, "$1")

  // Accepts 'HH:' or 'HH:MM:'.
  r = r.replace(TRAILING_SEMICOLON, " $1")

  return r
}

/**
 * Performs progressive validation of a partially entered
 * datetime string.
 *
 * This validator is designed for live input scenarios. It
 * allows **incomplete but structurally valid** timestamps and
 * only reports errors once a component’s intent is clear.
 *
 * IMPORTANT:
 * - The input must already be normalized using `normalizeForParsing`.
 * - This function assumes canonical formatting and does not attempt to
 *   sanitize input.
 *
 * @Example - Valid output.
 * progressiveValidate("")                    // ""
 * progressiveValidate("202")                 // ""
 * progressiveValidate("2023-1")              // ""
 * progressiveValidate("2023-12-31 12")       // ""
 * progressiveValidate("2023-12-31 12:34")    // ""
 *
 * @Example - Error messages.
 * progressiveValidate("2023-13")              // "Months need to be between 0-12."
 * progressiveValidate("2023-0-1")             // "Months cannot be 0 when days are not 0."
 * progressiveValidate("2023-2-30")            // "Day must be between 0-28."
 * progressiveValidate("2023-12-31 24")        // "Hours needs to be between 0-23."
 * progressiveValidate("foo")                  // "Invalid time structure."
 *
 *
 * @param normalized - Canonical datetime string produced by `normalizeForParsing`.
 * @param t - Translation function.
 *
 * @returns
 * - `""` if the input is valid or still incomplete.
 * - A descriptive error message if the input is invalid.
 */
export function progressiveValidate(normalized: string, t: (key: string, named?: NamedValue) => string): string {
  if (!normalized) return ""

  // Year in progress: "202", "2023".
  if (YEAR_IN_PROGRESS_REGEX.test(normalized)) return ""

  // Month in progress: "2023-1", "2023-12".
  if (MONTH_IN_PROGRESS_REGEX.test(normalized)) {
    const m = matchToMonth(normalized.replace(TRAILING_DASH_REGEX, ""))
    if (!m) return ""
    const month = Number(m[2])
    if (month === 0) return ""
    return month >= 0 && month <= 12 ? "" : t("partials.input.InputTime.errors.months0")
  }

  // Day in progress: "2023-1-1".
  if (DAY_IN_PROGRESS_REGEX.test(normalized)) {
    const asDay = matchToDay(normalized)
    if (!asDay) return ""
    const year = Number(asDay[1])
    const month = Number(asDay[2])
    const day = Number(asDay[3])

    if (month == 0 && day != 0) return t("partials.input.InputTime.errors.daysNotZero")
    if (month < 0 || month > 12) return t("partials.input.InputTime.errors.months0")

    if (day === 0) return ""
    const maxDay = daysIn(month, year)
    if (day < 0 || day > maxDay) return t("partials.input.InputTime.errors.days0", { maxDay })

    return ""
  }

  const toHour = matchToHour(normalized)
  if (toHour) {
    const year = Number(toHour[1])
    const month = Number(toHour[2])
    const day = Number(toHour[3])
    const hour = Number(toHour[4])

    if (month < 1 || month > 12) return t("partials.input.InputTime.errors.months")
    const maxDay = daysIn(month, year)
    if (day < 1 || day > maxDay) return t("partials.input.InputTime.errors.days", { maxDay })
    if (hour < 0 || hour > 23) return t("partials.input.InputTime.errors.hours")

    return ""
  }

  // Minutes in progress.
  if (MINUTES_IN_PROGRESS_REGEX.test(normalized)) return ""
  const toMinute = matchToMinute(normalized)
  if (toMinute) {
    const year = Number(toMinute[1])
    const month = Number(toMinute[2])
    const day = Number(toMinute[3])
    const hour = Number(toMinute[4])
    const minute = Number(toMinute[5])

    if (month < 1 || month > 12) return t("partials.input.InputTime.errors.months")
    const maxDay = daysIn(month, year)
    if (day < 1 || day > maxDay) return t("partials.input.InputTime.errors.days", { maxDay })
    if (hour < 0 || hour > 23) return t("partials.input.InputTime.errors.hours")
    if (minute < 0 || minute > 59) return t("partials.input.InputTime.errors.minutes")

    return ""
  }

  // Seconds in progress.
  if (SECONDS_IN_PROGRESS_REGEX.test(normalized)) return ""
  const toSecond = matchToSecond(normalized)
  if (toSecond) {
    const year = Number(toSecond[1])
    const month = Number(toSecond[2])
    const day = Number(toSecond[3])
    const hour = Number(toSecond[4])
    const minute = Number(toSecond[5])
    const second = Number(toSecond[6])

    if (month < 1 || month > 12) return t("partials.input.InputTime.errors.months")
    const maxDay = daysIn(month, year)
    if (day < 1 || day > maxDay) return t("partials.input.InputTime.errors.days", { maxDay })
    if (hour < 0 || hour > 23) return t("partials.input.InputTime.errors.hours")
    if (minute < 0 || minute > 59) return t("partials.input.InputTime.errors.minutes")
    if (second < 0 || second > 59) return t("partials.input.InputTime.errors.seconds")

    return ""
  }

  // Subseconds in progress (e.g. "2024-01-15 12:34:56." up to 9 digits).
  if (SUBSECONDS_IN_PROGRESS_REGEX.test(normalized)) {
    // Validate the date/time portion before the dot.
    const dotIdx = normalized.indexOf(".")
    const beforeDot = normalized.slice(0, dotIdx)
    const toSec = matchToSecond(beforeDot)
    if (toSec) {
      const year = Number(toSec[1])
      const month = Number(toSec[2])
      const day = Number(toSec[3])
      const hour = Number(toSec[4])
      const minute = Number(toSec[5])
      const second = Number(toSec[6])

      if (month < 1 || month > 12) return t("partials.input.InputTime.errors.months")
      const maxDay = daysIn(month, year)
      if (day < 1 || day > maxDay) return t("partials.input.InputTime.errors.days", { maxDay })
      if (hour < 0 || hour > 23) return t("partials.input.InputTime.errors.hours")
      if (minute < 0 || minute > 59) return t("partials.input.InputTime.errors.minutes")
      if (second < 0 || second > 59) return t("partials.input.InputTime.errors.seconds")
    }

    // Allow only fully-typed groups of 3, 6, 9 digits as completed input;
    // intermediate digit-counts are still "in progress".
    const subLen = normalized.length - dotIdx - 1
    if (subLen === 0 || subLen === 1 || subLen === 2 || subLen === 4 || subLen === 5 || subLen === 7 || subLen === 8) {
      // Still in progress.
      return ""
    }
    if (subLen === 3 || subLen === 6 || subLen === 9) {
      return ""
    }
    return t("partials.input.InputTime.errors.subseconds")
  }

  return t("partials.input.InputTime.errors.invalid")
}

export function clampToMax(p: TimePrecision, max: TimePrecision): TimePrecision {
  const pr = PRECISION_LEVEL.get(p)
  const mr = PRECISION_LEVEL.get(max)

  if (pr == null) throw new Error(`unknown precision: ${p}`)
  if (mr == null) throw new Error(`unknown maxPrecision: ${max}`)

  return pr < mr ? max : p
}

export function inferYearPrecision(yearStr: string, max: TimePrecision): TimePrecision {
  if (!yearStr) return clampToMax("y", max)

  const year = BigInt(yearStr)
  const abs = year < 0n ? -year : year

  const candidates: Array<[TimePrecision, bigint]> = [
    ["G", 1_000_000_000n],
    ["100M", 100_000_000n],
    ["10M", 10_000_000n],
    ["M", 1_000_000n],
    ["100k", 100_000n],
    ["10k", 10_000n],
    ["k", 1_000n],
    ["100y", 100n],
    ["10y", 10n],
  ]

  for (const [p, factor] of candidates) {
    if (abs >= factor && year % factor === 0n && abs > 9999n) {
      return clampToMax(p, max)
    }
  }

  return clampToMax("y", max)
}

export function inferPrecisionFromNormalized(
  normalized: string,
  timeStruct: { y: string; m: string; d: string; h: string; min: string; s: string; sub: string },
  maxPrecision: TimePrecision,
  precision: TimePrecision,
): TimePrecision {
  let inferred: TimePrecision

  if (matchToNs(normalized)) {
    inferred = "ns"
  } else if (matchToUs(normalized)) {
    inferred = "us"
  } else if (matchToMs(normalized)) {
    inferred = "ms"
  } else if (matchToSecond(normalized)) {
    inferred = "s"
  } else if (matchToMinute(normalized)) {
    inferred = "min"
  } else if (matchToHour(normalized)) {
    inferred = "h"
  } else if (matchToDay(normalized)) {
    // Months are defined, but days are not.
    if (Number(timeStruct.m) > 0 && (!timeStruct.d || timeStruct.d == "0" || timeStruct.d == "00")) return "m"
    // Days can be "00" or "0" for year precision.
    else if (timeStruct.d == "00" || timeStruct.d == "0") inferred = "y"
    else inferred = "d"
  } else if (matchToMonth(normalized)) {
    // Months can be "00" or "0" for year precision.
    if (timeStruct.m == "00" || timeStruct.m == "0") inferred = "y"
    else inferred = "m"
  } else {
    const y = matchToYear(normalized)
    inferred = y ? inferYearPrecision(y[1], maxPrecision) : precision
  }

  return clampToMax(inferred, maxPrecision)
}

// Pads a year string to the minimum width (4 digits in the
// absolute value, preserving any leading "-"). Operates on strings
// (not numbers) so years above Number.MAX_SAFE_INTEGER round-trip
// without loss.
function padYear(yStr: string): string {
  if (!yStr) return "0000"
  if (yStr.startsWith("-")) {
    return "-" + yStr.slice(1).padStart(4, "0")
  }
  return yStr.padStart(4, "0")
}

function roundDown(value: number, factor: number): number {
  return Math.floor(value / factor) * factor
}

export function getStructuredTime(normalized: string): { y: string; m: string; d: string; h: string; min: string; s: string; sub: string } {
  const timeStruct = { y: "", m: "", d: "", h: "", min: "", s: "", sub: "" }
  if (!normalized) return timeStruct

  const toYear = matchToYear(normalized)
  if (toYear) {
    timeStruct.y = toYear[1]
    return timeStruct
  }

  const toMonth = matchToMonth(normalized)
  if (toMonth) {
    timeStruct.y = toMonth[1]
    timeStruct.m = toMonth[2]
    return timeStruct
  }

  const toDay = matchToDay(normalized)
  if (toDay) {
    timeStruct.y = toDay[1]
    timeStruct.m = toDay[2]
    timeStruct.d = toDay[3]
    return timeStruct
  }

  const toHour = matchToHour(normalized)
  if (toHour) {
    timeStruct.y = toHour[1]
    timeStruct.m = toHour[2]
    timeStruct.d = toHour[3]
    timeStruct.h = toHour[4]
    return timeStruct
  }

  const toMinute = matchToMinute(normalized)
  if (toMinute) {
    timeStruct.y = toMinute[1]
    timeStruct.m = toMinute[2]
    timeStruct.d = toMinute[3]
    timeStruct.h = toMinute[4]
    timeStruct.min = toMinute[5]
    return timeStruct
  }

  const toSecond = matchToSecond(normalized)
  if (toSecond) {
    timeStruct.y = toSecond[1]
    timeStruct.m = toSecond[2]
    timeStruct.d = toSecond[3]
    timeStruct.h = toSecond[4]
    timeStruct.min = toSecond[5]
    timeStruct.s = toSecond[6]
    return timeStruct
  }

  // Subsecond formats (3, 6, or 9 digits after the dot).
  const toSub = matchToMs(normalized) ?? matchToUs(normalized) ?? matchToNs(normalized)
  if (toSub) {
    timeStruct.y = toSub[1]
    timeStruct.m = toSub[2]
    timeStruct.d = toSub[3]
    timeStruct.h = toSub[4]
    timeStruct.min = toSub[5]
    timeStruct.s = toSub[6]
    timeStruct.sub = toSub[7]
    return timeStruct
  }

  return timeStruct
}

export function toCanonicalString(timeStruct: { y: string; m: string; d: string; h: string; min: string; s: string; sub: string }, precision: TimePrecision): string {
  const y = padYear(timeStruct.y)

  if (
    precision === "G" ||
    precision === "100M" ||
    precision === "10M" ||
    precision === "M" ||
    precision === "100k" ||
    precision === "10k" ||
    precision === "k" ||
    precision === "100y" ||
    precision === "10y" ||
    precision === "y"
  ) {
    return y
  }

  const m = pad2(timeStruct.m || "01")
  // Month precision is encoded as YYYY-MM-00.
  if (precision === "m") return `${y}-${m}-00`

  const d = pad2(timeStruct.d || "01")
  if (precision === "d") return `${y}-${m}-${d}`

  const h = pad2(timeStruct.h || "00")
  // Hour precision is encoded as YYYY-MM-DD HH:00.
  if (precision === "h") return `${y}-${m}-${d} ${h}:00`

  const min = pad2(timeStruct.min || "00")
  if (precision === "min") return `${y}-${m}-${d} ${h}:${min}`

  const s = pad2(timeStruct.s || "00")
  if (precision === "s") return `${y}-${m}-${d} ${h}:${min}:${s}`

  // For sub-second precisions, pad/truncate the subseconds field to the
  // required number of digits.
  const padSubs = (raw: string, len: number): string => {
    if (raw.length >= len) return raw.slice(0, len)
    return raw.padEnd(len, "0")
  }
  if (precision === "ms") return `${y}-${m}-${d} ${h}:${min}:${s}.${padSubs(timeStruct.sub, 3)}`
  if (precision === "us") return `${y}-${m}-${d} ${h}:${min}:${s}.${padSubs(timeStruct.sub, 6)}`
  if (precision === "ns") return `${y}-${m}-${d} ${h}:${min}:${s}.${padSubs(timeStruct.sub, 9)}`

  return ""
}

export function applyPrecision(timeStruct: { y: string; m: string; d: string; h: string; min: string; s: string; sub: string }, precision: TimePrecision): string {
  const year = parseInt(timeStruct.y || "0000", 10)

  switch (precision) {
    case "G":
      return formatYearStr(roundDown(year, 1_000_000_000))
    case "100M":
      return formatYearStr(roundDown(year, 100_000_000))
    case "10M":
      return formatYearStr(roundDown(year, 10_000_000))
    case "M":
      return formatYearStr(roundDown(year, 1_000_000))
    case "100k":
      return formatYearStr(roundDown(year, 100_000))
    case "10k":
      return formatYearStr(roundDown(year, 10_000))
    case "k":
      return formatYearStr(roundDown(year, 1_000))
    case "100y":
      return formatYearStr(roundDown(year, 100))
    case "10y":
      return formatYearStr(roundDown(year, 10))
    case "y":
      return padYear(timeStruct.y)
    case "m":
    case "d":
    case "h":
    case "min":
    case "s":
    case "ms":
    case "us":
    case "ns":
      return toCanonicalString(timeStruct, precision)
    default:
      return ""
  }
}
