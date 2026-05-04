// Time parsing and validation, mirroring document/time.go.

import type { TimePrecision } from "@/document/types"

import { daysIn, fromDate } from "@/time"

const timeRegex = /^(-?\d{4,})(?:-(\d{2})-(\d{2})(?: (\d{2}):(\d{2})(?::(\d{2})(?:\.(\d{3}(?:\d{3}(?:\d{3})?)?))?)?)?)?$/

// TIME_PRECISIONS_ORDERED lists precisions from coarsest to finest.
// Index in this array gives a comparable level (higher = finer precision).
const TIME_PRECISIONS_ORDERED: TimePrecision[] = ["G", "100M", "10M", "M", "100k", "10k", "k", "100y", "10y", "y", "m", "d", "h", "min", "s", "ms", "us", "ns"]

const PRECISION_LEVEL = new Map<TimePrecision, number>(TIME_PRECISIONS_ORDERED.map((p, i) => [p, i]))

// VALID_TIME_PRECISIONS is the set of valid TimePrecision values.
export const VALID_TIME_PRECISIONS = new Set<string>(TIME_PRECISIONS_ORDERED)

// precisionLevel returns the numeric level of a precision (0 = coarsest = "G",
// finer precisions have higher levels). Throws if the precision string is
// unknown (this is the runtime gate; the TimePrecision type narrows at
// compile time but JSON-decoded values can still slip an invalid string in).
function precisionLevel(p: TimePrecision): number {
  const level = PRECISION_LEVEL.get(p)
  if (level === undefined) {
    throw new Error("unknown precision: ${p}")
  }
  return level
}

// yearPrecisionMultiple returns the factor by which the year must be divisible
// for precisions coarser than a single year. Returns 1 for "y" and finer.
function yearPrecisionMultiple(p: TimePrecision): number {
  switch (p) {
    case "G":
      return 1_000_000_000
    case "100M":
      return 100_000_000
    case "10M":
      return 10_000_000
    case "M":
      return 1_000_000
    case "100k":
      return 100_000
    case "10k":
      return 10_000
    case "k":
      return 1_000
    case "100y":
      return 100
    case "10y":
      return 10
    case "y":
    case "m":
    case "d":
    case "h":
    case "min":
    case "s":
    case "ms":
    case "us":
    case "ns":
      return 1
  }
}

type TimeParts = {
  year: number
  month: number
  day: number
  hours: number
  minutes: number
  seconds: number
  nanoseconds: number
}

// parseTimeString parses the time string into its components. It does not
// validate against precision; pass the result through validatePrecision
// for that.
function parseTimeString(t: string): {
  parts: TimeParts
  // Tracks which components were present in the input (the raw match
  // structure mirrors the Go regex capture groups).
  hasMonth: boolean
  hasHours: boolean
  hasSeconds: boolean
  subsecondsLen: number
} {
  const match = timeRegex.exec(t)
  if (!match) {
    throw new Error("unable to parse time")
  }

  const year = parseInt(match[1], 10)
  if (!Number.isFinite(year)) {
    throw new Error("unable to parse year")
  }

  // Defaults match Go's "absent" markers.
  let month = -1
  let day = 0
  let hours = -1
  let minutes = -1
  let seconds = -1
  let nanoseconds = -1
  let subsecondsLen = 0

  if (match[2]) {
    month = parseInt(match[2], 10)
    if (month < 1 || month > 12) {
      throw new Error("month out of range")
    }
    day = parseInt(match[3], 10)
    if (day > daysIn(month, year)) {
      throw new Error("day out of range")
    }
    if (match[4]) {
      hours = parseInt(match[4], 10)
      if (hours > 23) {
        throw new Error("hours out of range")
      }
      minutes = parseInt(match[5], 10)
      if (minutes > 59) {
        throw new Error("minutes out of range")
      }
      if (match[6]) {
        seconds = parseInt(match[6], 10)
        if (seconds > 59) {
          throw new Error("seconds out of range")
        }
        if (match[7]) {
          subsecondsLen = match[7].length
          nanoseconds = parseInt(match[7], 10)
          // Match Go: 3-digit input is ms, 6-digit is us, 9-digit is ns.
          switch (subsecondsLen) {
            case 3:
              nanoseconds *= 1_000_000
              break
            case 6:
              nanoseconds *= 1_000
              break
            case 9:
              break
            default:
              // The regex guarantees one of these lengths.
              throw new Error("unexpected subseconds length")
          }
        }
      }
    }
  }

  // Replace absent parts with defaults for the time-construction step.
  const parts: TimeParts = {
    year,
    month: month === -1 ? 1 : month,
    day: day === 0 ? 1 : day,
    hours: hours === -1 ? 0 : hours,
    minutes: minutes === -1 ? 0 : minutes,
    seconds: seconds === -1 ? 0 : seconds,
    nanoseconds: nanoseconds === -1 ? 0 : nanoseconds,
  }

  return {
    parts,
    hasMonth: month !== -1,
    hasHours: hours !== -1,
    hasSeconds: seconds !== -1,
    subsecondsLen,
  }
}

// validatePrecision checks that the parsed time matches the given precision
// (e.g. day precision requires day component, year precision rejects it).
//
// Mirrors the precision-validation block in Time.Time in document/time.go.
function validatePrecision(t: string, parsed: ReturnType<typeof parseTimeString>, precision: TimePrecision): void {
  const lvl = precisionLevel(precision)
  const lvlMonth = precisionLevel("m")
  const lvlDay = precisionLevel("d")
  const lvlHour = precisionLevel("h")
  const lvlMinute = precisionLevel("min")
  const lvlSecond = precisionLevel("s")
  const lvlMillisecond = precisionLevel("ms")

  const needsMonth = lvl >= lvlMonth
  const needsDay = lvl >= lvlDay
  const needsHours = lvl >= lvlHour
  const needsMinutes = lvl >= lvlMinute
  const needsSeconds = lvl >= lvlSecond
  const needsSubseconds = lvl >= lvlMillisecond

  // Year-precision divisibility check.
  const mult = yearPrecisionMultiple(precision)
  if (parsed.parts.year % mult !== 0) {
    throw new Error("year not rounded to precision")
  }

  if (parsed.hasMonth !== needsMonth) {
    throw new Error(needsMonth ? "month required for precision" : "month not allowed for precision")
  }

  // For "month" precision Go allows day=0 in the string and parses day=0 as
  // "absent for date-precision purposes". For "day" precision day must be set.
  // parseTimeString uses day===0 to flag "month-only".
  // We compare the raw match's day to determine "hasDay".
  // Re-run a small piece of the regex to recover it. The simpler approach:
  // detect from the original string whether day is present and non-zero.
  const dayMatch = timeRegex.exec(t)!
  const dayPresent = dayMatch[3] !== undefined && dayMatch[3] !== "00"
  if (dayPresent !== needsDay) {
    throw new Error(needsDay ? "day required for precision" : "day not allowed for precision")
  }

  if (parsed.hasHours !== needsHours) {
    throw new Error(needsHours ? "hours and minutes required for precision" : "hours and minutes not allowed for precision")
  }
  if (parsed.hasHours && !needsMinutes && parsed.parts.minutes !== 0) {
    throw new Error("minutes must be zero for hour precision")
  }
  if (parsed.hasSeconds !== needsSeconds) {
    throw new Error(needsSeconds ? "seconds required for precision" : "seconds not allowed for precision")
  }
  if (parsed.subsecondsLen > 0 !== needsSubseconds) {
    throw new Error(needsSubseconds ? "subseconds required for precision" : "subseconds not allowed for precision")
  }
  if (parsed.subsecondsLen > 0) {
    let requiredLen: number
    // Only ms/us/ns reach this branch because needsSubseconds is true only
    // for those; the other cases are unreachable.
    // eslint-disable-next-line @typescript-eslint/switch-exhaustiveness-check
    switch (precision) {
      case "ms":
        requiredLen = 3
        break
      case "us":
        requiredLen = 6
        break
      case "ns":
        requiredLen = 9
        break
      default:
        throw new Error("invalid precision")
    }
    if (parsed.subsecondsLen !== requiredLen) {
      throw new Error("subseconds length does not match precision")
    }
  }
}

// timeFloat64 parses a Time string and returns its float64 representation
// as seconds since the Unix epoch. Validates the format and (when
// precision is not 0) the precision-component-match invariants.
export function timeFloat64(t: string, precision: TimePrecision | 0): number {
  const parsed = parseTimeString(t)
  if (precision !== 0) {
    validatePrecision(t, parsed, precision)
  }
  return partsToFloat64(parsed.parts)
}

// validateTime checks that the time is valid for the given precision.
// Passing 0 for precision skips precision checks and just checks the format.
export function validateTime(t: string, precision: TimePrecision | 0): void {
  timeFloat64(t, precision)
}

// partsToFloat64 mirrors x.TimeToFloat64 (Unix() + Nanosecond()/1e9), built
// on src/time.ts's fromDate (which produces unix seconds in bigint).
function partsToFloat64(p: TimeParts): number {
  const unixSec = fromDate(p.year, p.month, p.day, p.hours, p.minutes, p.seconds)
  return Number(unixSec) + p.nanoseconds / 1e9
}

// addTimePrecision returns the parts at the end of the precision window
// starting at `parts`. If the natural step doesn't survive the float64
// round-trip it widens to the next coarser precision.
//
// Mirrors addTimePrecision in document/time.go.
function addTimePrecision(parts: TimeParts, precision: TimePrecision): TimeParts {
  let stepped: TimeParts
  switch (precision) {
    case "G":
      stepped = { ...parts, year: parts.year + 1_000_000_000 }
      break
    case "100M":
      stepped = { ...parts, year: parts.year + 100_000_000 }
      break
    case "10M":
      stepped = { ...parts, year: parts.year + 10_000_000 }
      break
    case "M":
      stepped = { ...parts, year: parts.year + 1_000_000 }
      break
    case "100k":
      stepped = { ...parts, year: parts.year + 100_000 }
      break
    case "10k":
      stepped = { ...parts, year: parts.year + 10_000 }
      break
    case "k":
      stepped = { ...parts, year: parts.year + 1_000 }
      break
    case "100y":
      stepped = { ...parts, year: parts.year + 100 }
      break
    case "10y":
      stepped = { ...parts, year: parts.year + 10 }
      break
    case "y":
      stepped = { ...parts, year: parts.year + 1 }
      break
    case "m":
      stepped = { ...parts, month: parts.month + 1 }
      break
    case "d":
      stepped = { ...parts, day: parts.day + 1 }
      break
    case "h":
      stepped = { ...parts, hours: parts.hours + 1 }
      break
    case "min":
      stepped = { ...parts, minutes: parts.minutes + 1 }
      break
    case "s":
      stepped = { ...parts, seconds: parts.seconds + 1 }
      break
    case "ms":
      stepped = { ...parts, nanoseconds: parts.nanoseconds + 1_000_000 }
      break
    case "us":
      stepped = { ...parts, nanoseconds: parts.nanoseconds + 1_000 }
      break
    case "ns":
      stepped = { ...parts, nanoseconds: parts.nanoseconds + 1 }
      break
  }

  if (partsToFloat64(stepped) === partsToFloat64(parts)) {
    if (precision === "G") {
      // Nothing left to widen to.
      throw new Error("unsupported precision")
    }
    const idx = precisionLevel(precision)
    return addTimePrecision(parts, TIME_PRECISIONS_ORDERED[idx - 1])
  }
  return stepped
}

// timeWindowStart returns the lower edge that this bound contributes to a
// half-open indexed range. When the bound is closed (default,
// isOpen=false) this is the start of the precision window; when open
// (isOpen=true) the precision window is excluded and the edge advances to
// the end of the window.
export function timeWindowStart(t: string, precision: TimePrecision, isOpen: boolean): number {
  if (isOpen) {
    return timeWindowEndInternal(t, precision)
  }
  return timeWindowStartInternal(t, precision)
}

// timeWindowEnd returns the upper edge that this bound contributes to a
// half-open indexed range. When the bound is closed (default,
// isOpen=false) this is the end of the precision window; when open
// (isOpen=true) the precision window is excluded and the edge retreats to
// the start of the window.
export function timeWindowEnd(t: string, precision: TimePrecision, isOpen: boolean): number {
  if (isOpen) {
    return timeWindowStartInternal(t, precision)
  }
  return timeWindowEndInternal(t, precision)
}

function timeWindowStartInternal(t: string, precision: TimePrecision): number {
  return timeFloat64(t, precision)
}

function timeWindowEndInternal(t: string, precision: TimePrecision): number {
  const parsed = parseTimeString(t)
  validatePrecision(t, parsed, precision)
  return partsToFloat64(addTimePrecision(parsed.parts, precision))
}
