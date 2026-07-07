import type { TimePrecision } from "@/document"

import { parseTimeString, TIME_PRECISIONS_ORDERED } from "@/document/time"

type DisplayTimePart = { text: string; precise: boolean }

// Number of trailing zeros that are imprecise for each precision level.
const TRAILING_ZEROS: Record<string, number> = {
  G: 9,
  "100M": 8,
  "10M": 7,
  M: 6,
  "100k": 5,
  "10k": 4,
  k: 3,
  "100y": 2,
  "10y": 1,
}

type ParsedTime = ReturnType<typeof parseTimeString>

/**
 * Parses a time string in the new claim format. Returns null on failure.
 */
export function parseTimestamp(timestamp: string): ParsedTime | null {
  try {
    return parseTimeString(timestamp)
  } catch {
    return null
  }
}

/**
 * Returns the index of a precision level in the hierarchy.
 * Lower index means less precise (e.g., G=0, ns=17).
 */
export function getPrecisionIndex(precision: TimePrecision): number {
  return TIME_PRECISIONS_ORDERED.indexOf(precision)
}

/**
 * Checks if a specific level is precise given the current precision.
 */
export function isPrecise(level: TimePrecision, precision: TimePrecision): boolean {
  const levelIndex = getPrecisionIndex(level)
  const precisionIndex = getPrecisionIndex(precision)
  return levelIndex <= precisionIndex
}

/**
 * Formats the year with grayed out imprecise trailing zeros.
 */
export function formatYearParts(yearStr: string, precision: TimePrecision): DisplayTimePart[] {
  const parts: DisplayTimePart[] = []
  const yearPrecise = isPrecise("y", precision)

  if (yearPrecise) {
    parts.push({ text: yearStr, precise: true })
    return parts
  }

  // For precisions less than "y", we need to gray out trailing zeros.
  const trailingZeros = TRAILING_ZEROS[precision] ?? 0
  const absYear = yearStr.startsWith("-") ? yearStr.slice(1) : yearStr
  const sign = yearStr.startsWith("-") ? "-" : ""

  if (trailingZeros > 0 && absYear.length > trailingZeros) {
    const preciseLen = absYear.length - trailingZeros
    parts.push({ text: sign + absYear.slice(0, preciseLen), precise: true })
    parts.push({ text: absYear.slice(preciseLen), precise: false })
  } else {
    parts.push({ text: yearStr, precise: true })
  }

  return parts
}

// Formats nanoseconds into a fixed-width subseconds string at the requested
// number of digits (3 = ms, 6 = us, 9 = ns).
function formatSubseconds(nanoseconds: number, digits: 3 | 6 | 9): string {
  const divisor = digits === 3 ? 1_000_000 : digits === 6 ? 1_000 : 1
  const value = Math.floor(nanoseconds / divisor)
  return String(value).padStart(digits, "0")
}

/**
 * Formats an absolute timestamp into display parts with precision indicators.
 */
export function formatAbsoluteParts(parsed: ParsedTime, precision: TimePrecision): DisplayTimePart[] {
  const parts: DisplayTimePart[] = []
  const precisionIndex = getPrecisionIndex(precision)

  // Year parts with grayed out imprecise trailing zeros.
  parts.push(...formatYearParts(parsed.yearStr, precision))

  // For year-only precisions, stop here.
  if (precisionIndex <= getPrecisionIndex("y")) {
    return parts
  }

  // Month.
  const monthPrecise = isPrecise("m", precision)
  parts.push({ text: "-", precise: monthPrecise })
  if (monthPrecise && parsed.hasMonth) {
    parts.push({ text: String(parsed.parts.month).padStart(2, "0"), precise: true })
  } else {
    parts.push({ text: "00", precise: false })
  }

  // Day.
  const dayPrecise = isPrecise("d", precision)
  parts.push({ text: "-", precise: dayPrecise })
  if (dayPrecise && parsed.hasDay) {
    parts.push({ text: String(parsed.parts.day).padStart(2, "0"), precise: true })
  } else {
    parts.push({ text: "00", precise: false })
  }

  if (precisionIndex < getPrecisionIndex("h")) {
    return parts
  }

  // Hours.
  const hourPrecise = isPrecise("h", precision)
  parts.push({ text: " ", precise: true })
  if (hourPrecise && parsed.hasHours) {
    parts.push({ text: String(parsed.parts.hours).padStart(2, "0"), precise: true })
  } else {
    parts.push({ text: "00", precise: false })
  }

  // Minutes.
  const minPrecise = isPrecise("min", precision)
  parts.push({ text: ":", precise: minPrecise })
  if (minPrecise && parsed.hasHours) {
    parts.push({ text: String(parsed.parts.minutes).padStart(2, "0"), precise: true })
  } else {
    parts.push({ text: "00", precise: false })
  }

  if (precisionIndex < getPrecisionIndex("s")) {
    return parts
  }

  // Seconds.
  const secPrecise = isPrecise("s", precision)
  parts.push({ text: ":", precise: secPrecise })
  if (secPrecise && parsed.hasSeconds) {
    parts.push({ text: String(parsed.parts.seconds).padStart(2, "0"), precise: true })
  } else {
    parts.push({ text: "00", precise: false })
  }

  // Subseconds (ms/us/ns).
  if (precisionIndex < getPrecisionIndex("ms")) {
    return parts
  }

  const digits: 3 | 6 | 9 = precision === "ms" ? 3 : precision === "us" ? 6 : 9
  parts.push({ text: ".", precise: true })
  if (parsed.subsecondsLen > 0) {
    parts.push({ text: formatSubseconds(parsed.parts.nanoseconds, digits), precise: true })
  } else {
    parts.push({ text: "0".repeat(digits), precise: false })
  }

  return parts
}

/**
 * Calculates time difference in various units from milliseconds.
 */
export function calculateTimeUnits(diffMs: number): {
  seconds: number
  minutes: number
  hours: number
  days: number
  months: number
  years: number
  kiloYears: number
  megaYears: number
  gigaYears: number
} {
  const absDiff = Math.abs(diffMs)
  const seconds = Math.floor(absDiff / 1000)
  const minutes = Math.floor(seconds / 60)
  const hours = Math.floor(minutes / 60)
  const days = Math.floor(hours / 24)
  const months = Math.floor(days / 30)
  const years = Math.floor(days / 365)
  const kiloYears = Math.floor(years / 1000)
  const megaYears = Math.floor(years / 1_000_000)
  const gigaYears = Math.floor(years / 1_000_000_000)

  return { seconds, minutes, hours, days, months, years, kiloYears, megaYears, gigaYears }
}

/**
 * Determines which time unit to use for relative display and when to update.
 */
export function getRelativeTimeInfo(diffMs: number): {
  unit: "gigaYears" | "megaYears" | "kiloYears" | "years" | "months" | "days" | "hours" | "minutes" | "seconds"
  count: number
  isPast: boolean
  nextUpdateMs: number
} {
  const isPast = diffMs >= 0
  const units = calculateTimeUnits(diffMs)

  if (units.gigaYears >= 1) {
    return {
      unit: "gigaYears",
      count: units.gigaYears,
      isPast,
      nextUpdateMs: Number.MAX_SAFE_INTEGER,
    }
  }
  if (units.megaYears >= 1) {
    return {
      unit: "megaYears",
      count: units.megaYears,
      isPast,
      nextUpdateMs: Number.MAX_SAFE_INTEGER,
    }
  }
  if (units.kiloYears >= 1) {
    return {
      unit: "kiloYears",
      count: units.kiloYears,
      isPast,
      nextUpdateMs: Number.MAX_SAFE_INTEGER,
    }
  }
  if (units.years >= 1) {
    const remainingDays = units.days - units.years * 365
    return {
      unit: "years",
      count: units.years,
      isPast,
      nextUpdateMs: (365 - remainingDays) * 24 * 60 * 60 * 1000,
    }
  }
  if (units.months >= 1) {
    const remainingDays = units.days - units.months * 30
    return {
      unit: "months",
      count: units.months,
      isPast,
      nextUpdateMs: (30 - remainingDays) * 24 * 60 * 60 * 1000,
    }
  }
  if (units.days >= 1) {
    const remainingHours = units.hours - units.days * 24
    return {
      unit: "days",
      count: units.days,
      isPast,
      nextUpdateMs: (24 - remainingHours) * 60 * 60 * 1000,
    }
  }
  if (units.hours >= 1) {
    const remainingMinutes = units.minutes - units.hours * 60
    return {
      unit: "hours",
      count: units.hours,
      isPast,
      nextUpdateMs: (60 - remainingMinutes) * 60 * 1000,
    }
  }
  if (units.minutes >= 1) {
    const remainingSeconds = units.seconds - units.minutes * 60
    return {
      unit: "minutes",
      count: units.minutes,
      isPast,
      nextUpdateMs: (60 - remainingSeconds) * 1000,
    }
  }
  return {
    unit: "seconds",
    count: units.seconds,
    isPast,
    nextUpdateMs: 1000,
  }
}

/**
 * Returns the millisecond epoch of the parsed wall-clock fields interpreted as UTC, or null when
 * the year is outside the JavaScript Date range.
 */
function wallEpochMs(parsed: ParsedTime): number | null {
  const year = parseInt(parsed.yearStr, 10)
  if (!Number.isFinite(year) || year < -271820 || year > 275759) {
    return null
  }
  const date = new Date(0)
  date.setUTCFullYear(year, parsed.hasMonth ? parsed.parts.month - 1 : 0, parsed.hasDay ? parsed.parts.day : 1)
  date.setUTCHours(parsed.hasHours ? parsed.parts.hours : 0, parsed.hasHours ? parsed.parts.minutes : 0, parsed.hasSeconds ? parsed.parts.seconds : 0, 0)
  const ms = date.getTime()
  return Number.isFinite(ms) ? ms : null
}

/**
 * Returns the UTC offset of the IANA timezone at the given epoch in milliseconds, or null when the
 * timezone is unknown or the epoch cannot be formatted.
 */
function tzOffsetMs(timeZone: string, epochMs: number): number | null {
  let parts: Intl.DateTimeFormatPart[]
  try {
    parts = new Intl.DateTimeFormat("en-US", {
      timeZone,
      era: "short",
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
      hourCycle: "h23",
    }).formatToParts(epochMs)
  } catch {
    return null
  }
  const get = (type: Intl.DateTimeFormatPartTypes) => parts.find((part) => part.type === type)?.value
  let year = parseInt(get("year") ?? "", 10)
  if (get("era") === "BC") {
    year = 1 - year
  }
  const month = parseInt(get("month") ?? "", 10)
  const day = parseInt(get("day") ?? "", 10)
  const hours = parseInt(get("hour") ?? "", 10)
  const minutes = parseInt(get("minute") ?? "", 10)
  const seconds = parseInt(get("second") ?? "", 10)
  if (![year, month, day, hours, minutes, seconds].every(Number.isFinite)) {
    return null
  }
  const date = new Date(0)
  date.setUTCFullYear(year, month - 1, day)
  date.setUTCHours(hours, minutes, seconds, 0)
  return date.getTime() - epochMs
}

/**
 * Returns the millisecond epoch of the parsed wall-clock fields interpreted in the given IANA
 * timezone (time claims carry their timezone in an IN_LOCATION sub claim; without one the claim is
 * in UTC). Returns null when the timezone is unknown or the timestamp is out of the Date range.
 * The offset is resolved iteratively so daylight saving transitions map correctly.
 */
export function zonedEpochMs(parsed: ParsedTime, timeZone: string): number | null {
  const wall = wallEpochMs(parsed)
  if (wall === null) {
    return null
  }
  const offset = tzOffsetMs(timeZone, wall)
  if (offset === null) {
    return null
  }
  let epoch = wall - offset
  const adjustedOffset = tzOffsetMs(timeZone, epoch)
  if (adjustedOffset === null) {
    return null
  }
  if (adjustedOffset !== offset) {
    epoch = wall - adjustedOffset
  }
  return epoch
}

/**
 * Converts parsed wall-clock fields known to be in the given IANA timezone into the equivalent
 * parsed fields in UTC, preserving subseconds (offsets are whole minutes). Returns null when the
 * conversion is not possible; callers should then fall back to the unconverted fields.
 */
export function convertParsedToUtc(parsed: ParsedTime, timeZone: string): ParsedTime | null {
  const epoch = zonedEpochMs(parsed, timeZone)
  if (epoch === null) {
    return null
  }
  const date = new Date(epoch)
  const year = date.getUTCFullYear()
  return {
    parts: {
      year,
      month: date.getUTCMonth() + 1,
      day: date.getUTCDate(),
      hours: date.getUTCHours(),
      minutes: date.getUTCMinutes(),
      seconds: date.getUTCSeconds(),
      nanoseconds: parsed.parts.nanoseconds,
    },
    yearStr: (year < 0 ? "-" : "") + String(Math.abs(year)).padStart(4, "0"),
    hasMonth: true,
    hasDay: true,
    hasHours: true,
    hasSeconds: parsed.hasSeconds,
    subsecondsLen: parsed.subsecondsLen,
  }
}

/**
 * Formats an absolute timestamp into a localized display part using Intl.DateTimeFormat while
 * respecting the claim precision: year precision renders only the year, month precision the month
 * name and year, day precision the locale date, and hour or finer precisions also the time to the
 * claim precision. Subsecond digits are not rendered. Precisions coarser than a year and years
 * outside the JavaScript Date range fall back to the plain rendering.
 *
 * Day and coarser precisions are calendar dates and render the stored fields without any timezone
 * conversion. Hour and finer precisions are instants: the fields are interpreted in the given IANA
 * timezone (UTC without one) and displayed in the timezone of the browser.
 */
export function formatAbsoluteLocalizedParts(parsed: ParsedTime, precision: TimePrecision, locale: string, timeZone?: string): DisplayTimePart[] {
  const precisionIndex = getPrecisionIndex(precision)
  if (precisionIndex <= getPrecisionIndex("y")) {
    return formatAbsoluteParts(parsed, precision)
  }

  const options: Intl.DateTimeFormatOptions = { year: "numeric" }
  if (isPrecise("d", precision)) {
    options.month = "numeric"
    options.day = "numeric"
  } else if (isPrecise("m", precision)) {
    options.month = "long"
  }

  let epoch: number | null
  if (precisionIndex < getPrecisionIndex("h")) {
    epoch = wallEpochMs(parsed)
    options.timeZone = "UTC"
  } else {
    epoch = timeZone ? zonedEpochMs(parsed, timeZone) : wallEpochMs(parsed)
    if (isPrecise("h", precision)) {
      options.hour = "numeric"
    }
    if (isPrecise("min", precision)) {
      options.minute = "2-digit"
    }
    if (isPrecise("s", precision)) {
      options.second = "2-digit"
    }
  }
  if (epoch === null) {
    return formatAbsoluteParts(parsed, precision)
  }

  try {
    return [{ text: new Intl.DateTimeFormat(locale, options).format(epoch), precise: true }]
  } catch {
    return formatAbsoluteParts(parsed, precision)
  }
}
