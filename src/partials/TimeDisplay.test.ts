import { assert, describe, test } from "vitest"

import {
  calculateTimeUnits,
  convertParsedToUtc,
  formatAbsoluteLocalizedParts,
  formatAbsoluteParts,
  formatYearParts,
  getPrecisionIndex,
  getRelativeTimeInfo,
  isPrecise,
  parseTimestamp,
  zonedEpochMs,
} from "@/partials/TimeDisplay.utils"

describe("parseTimestamp", () => {
  test("parses full date-time", () => {
    const result = parseTimestamp("2025-03-02 10:30:45")
    assert.isNotNull(result)
    assert.equal(result.yearStr, "2025")
    assert.equal(result.parts.year, 2025)
    assert.equal(result.parts.month, 3)
    assert.equal(result.parts.day, 2)
    assert.equal(result.parts.hours, 10)
    assert.equal(result.parts.minutes, 30)
    assert.equal(result.parts.seconds, 45)
    assert.equal(result.parts.nanoseconds, 0)
    assert.equal(result.subsecondsLen, 0)
    assert.isTrue(result.hasMonth)
    assert.isTrue(result.hasDay)
    assert.isTrue(result.hasHours)
    assert.isTrue(result.hasSeconds)
  })

  test("parses date with milliseconds", () => {
    const result = parseTimestamp("2025-03-02 10:30:45.123")
    assert.isNotNull(result)
    assert.equal(result.subsecondsLen, 3)
    assert.equal(result.parts.nanoseconds, 123_000_000)
  })

  test("parses date with microseconds", () => {
    const result = parseTimestamp("2025-03-02 10:30:45.123456")
    assert.isNotNull(result)
    assert.equal(result.subsecondsLen, 6)
    assert.equal(result.parts.nanoseconds, 123_456_000)
  })

  test("parses date with nanoseconds", () => {
    const result = parseTimestamp("2025-03-02 10:30:45.123456789")
    assert.isNotNull(result)
    assert.equal(result.subsecondsLen, 9)
    assert.equal(result.parts.nanoseconds, 123_456_789)
  })

  test("parses year-only", () => {
    const result = parseTimestamp("2025")
    assert.isNotNull(result)
    assert.equal(result.yearStr, "2025")
    assert.isFalse(result.hasMonth)
    assert.isFalse(result.hasDay)
    assert.isFalse(result.hasHours)
    assert.isFalse(result.hasSeconds)
  })

  test("parses date-only", () => {
    const result = parseTimestamp("2025-03-02")
    assert.isNotNull(result)
    assert.isTrue(result.hasMonth)
    assert.isTrue(result.hasDay)
    assert.isFalse(result.hasHours)
  })

  test("parses month-only (day=00)", () => {
    const result = parseTimestamp("2025-03-00")
    assert.isNotNull(result)
    assert.isTrue(result.hasMonth)
    assert.isFalse(result.hasDay)
  })

  test("parses date with hours and minutes only", () => {
    const result = parseTimestamp("2025-03-02 10:30")
    assert.isNotNull(result)
    assert.isTrue(result.hasHours)
    assert.isFalse(result.hasSeconds)
  })

  test("parses negative year", () => {
    const result = parseTimestamp("-4500000000")
    assert.isNotNull(result)
    assert.equal(result.yearStr, "-4500000000")
    assert.equal(result.parts.year, -4500000000)
  })

  test("parses long year", () => {
    const result = parseTimestamp("20006-12-04")
    assert.isNotNull(result)
    assert.equal(result.yearStr, "20006")
    assert.equal(result.parts.year, 20006)
  })

  test("returns null for invalid timestamp", () => {
    assert.isNull(parseTimestamp("invalid"))
    assert.isNull(parseTimestamp(""))
    // T delimiter is no longer accepted.
    assert.isNull(parseTimestamp("2025-03-02T10:30:45Z"))
    // Trailing Z is no longer accepted.
    assert.isNull(parseTimestamp("2025-03-02 10:30:45Z"))
    // Year must be at least 4 digits.
    assert.isNull(parseTimestamp("206-01-01"))
  })
})

describe("getPrecisionIndex", () => {
  test("returns correct index for each precision level", () => {
    assert.equal(getPrecisionIndex("G"), 0)
    assert.equal(getPrecisionIndex("y"), 9)
    assert.equal(getPrecisionIndex("s"), 14)
    assert.equal(getPrecisionIndex("ms"), 15)
    assert.equal(getPrecisionIndex("us"), 16)
    assert.equal(getPrecisionIndex("ns"), 17)
  })
})

describe("isPrecise", () => {
  test("year precision includes only year and above", () => {
    assert.isTrue(isPrecise("G", "y"))
    assert.isTrue(isPrecise("y", "y"))
    assert.isFalse(isPrecise("m", "y"))
    assert.isFalse(isPrecise("d", "y"))
  })

  test("month precision includes month and above", () => {
    assert.isTrue(isPrecise("y", "m"))
    assert.isTrue(isPrecise("m", "m"))
    assert.isFalse(isPrecise("d", "m"))
  })

  test("nanosecond precision includes all levels", () => {
    assert.isTrue(isPrecise("G", "ns"))
    assert.isTrue(isPrecise("y", "ns"))
    assert.isTrue(isPrecise("s", "ns"))
    assert.isTrue(isPrecise("ms", "ns"))
    assert.isTrue(isPrecise("us", "ns"))
    assert.isTrue(isPrecise("ns", "ns"))
  })

  test("millisecond precision excludes finer subseconds", () => {
    assert.isTrue(isPrecise("s", "ms"))
    assert.isTrue(isPrecise("ms", "ms"))
    assert.isFalse(isPrecise("us", "ms"))
    assert.isFalse(isPrecise("ns", "ms"))
  })
})

describe("formatYearParts", () => {
  test("returns full year for year precision", () => {
    const result = formatYearParts("2025", "y")
    assert.deepEqual(result, [{ text: "2025", precise: true }])
  })

  test("grays out trailing zeros for giga-year precision", () => {
    const result = formatYearParts("-4500000000", "G")
    assert.deepEqual(result, [
      { text: "-4", precise: true },
      { text: "500000000", precise: false },
    ])
  })

  test("grays out trailing zeros for mega-year precision", () => {
    const result = formatYearParts("100000000", "100M")
    assert.deepEqual(result, [
      { text: "1", precise: true },
      { text: "00000000", precise: false },
    ])
  })

  test("grays out trailing zeros for kilo-year precision", () => {
    const result = formatYearParts("10000", "k")
    assert.deepEqual(result, [
      { text: "10", precise: true },
      { text: "000", precise: false },
    ])
  })

  test("grays out trailing zeros for decade precision", () => {
    const result = formatYearParts("2020", "10y")
    assert.deepEqual(result, [
      { text: "202", precise: true },
      { text: "0", precise: false },
    ])
  })

  test("handles negative years correctly", () => {
    const result = formatYearParts("-5000", "k")
    assert.deepEqual(result, [
      { text: "-5", precise: true },
      { text: "000", precise: false },
    ])
  })

  test("splits year correctly for kilo-year precision", () => {
    const result = formatYearParts("2025", "k")
    assert.deepEqual(result, [
      { text: "2", precise: true },
      { text: "025", precise: false },
    ])
  })

  test("returns full year if shorter than trailing zeros count", () => {
    const result = formatYearParts("25", "k")
    assert.deepEqual(result, [{ text: "25", precise: true }])
  })
})

describe("formatAbsoluteParts", () => {
  const fullParsed = parseTimestamp("2025-03-02 10:30:45.123456789")!

  test("shows only year for year precision", () => {
    const result = formatAbsoluteParts(fullParsed, "y")
    assert.deepEqual(result, [{ text: "2025", precise: true }])
  })

  test("shows YYYY-MM with day as 00 for month precision", () => {
    const result = formatAbsoluteParts(fullParsed, "m")
    assert.deepEqual(result, [
      { text: "2025", precise: true },
      { text: "-", precise: true },
      { text: "03", precise: true },
      { text: "-", precise: false },
      { text: "00", precise: false },
    ])
  })

  test("shows full date for day precision", () => {
    const result = formatAbsoluteParts(fullParsed, "d")
    assert.deepEqual(result, [
      { text: "2025", precise: true },
      { text: "-", precise: true },
      { text: "03", precise: true },
      { text: "-", precise: true },
      { text: "02", precise: true },
    ])
  })

  test("shows date and HH:00 for hour precision", () => {
    const result = formatAbsoluteParts(fullParsed, "h")
    assert.deepEqual(result, [
      { text: "2025", precise: true },
      { text: "-", precise: true },
      { text: "03", precise: true },
      { text: "-", precise: true },
      { text: "02", precise: true },
      { text: " ", precise: true },
      { text: "10", precise: true },
      { text: ":", precise: false },
      { text: "00", precise: false },
    ])
  })

  test("shows date and HH:MM for minute precision", () => {
    const result = formatAbsoluteParts(fullParsed, "min")
    assert.deepEqual(result, [
      { text: "2025", precise: true },
      { text: "-", precise: true },
      { text: "03", precise: true },
      { text: "-", precise: true },
      { text: "02", precise: true },
      { text: " ", precise: true },
      { text: "10", precise: true },
      { text: ":", precise: true },
      { text: "30", precise: true },
    ])
  })

  test("shows full date and time for second precision", () => {
    const result = formatAbsoluteParts(fullParsed, "s")
    assert.deepEqual(result, [
      { text: "2025", precise: true },
      { text: "-", precise: true },
      { text: "03", precise: true },
      { text: "-", precise: true },
      { text: "02", precise: true },
      { text: " ", precise: true },
      { text: "10", precise: true },
      { text: ":", precise: true },
      { text: "30", precise: true },
      { text: ":", precise: true },
      { text: "45", precise: true },
    ])
  })

  test("shows milliseconds for ms precision", () => {
    const result = formatAbsoluteParts(fullParsed, "ms")
    assert.deepEqual(result, [
      { text: "2025", precise: true },
      { text: "-", precise: true },
      { text: "03", precise: true },
      { text: "-", precise: true },
      { text: "02", precise: true },
      { text: " ", precise: true },
      { text: "10", precise: true },
      { text: ":", precise: true },
      { text: "30", precise: true },
      { text: ":", precise: true },
      { text: "45", precise: true },
      { text: ".", precise: true },
      { text: "123", precise: true },
    ])
  })

  test("shows microseconds for us precision", () => {
    const result = formatAbsoluteParts(fullParsed, "us")
    const tail = result.slice(-2)
    assert.deepEqual(tail, [
      { text: ".", precise: true },
      { text: "123456", precise: true },
    ])
  })

  test("shows nanoseconds for ns precision", () => {
    const result = formatAbsoluteParts(fullParsed, "ns")
    const tail = result.slice(-2)
    assert.deepEqual(tail, [
      { text: ".", precise: true },
      { text: "123456789", precise: true },
    ])
  })

  test("grays out missing subseconds when precision requests them", () => {
    const noSub = parseTimestamp("2025-03-02 10:30:45")!
    const result = formatAbsoluteParts(noSub, "ms")
    const tail = result.slice(-2)
    assert.deepEqual(tail, [
      { text: ".", precise: true },
      { text: "000", precise: false },
    ])
  })

  test("grays out missing day when precision is day and input is month-only", () => {
    const monthOnly = parseTimestamp("2025-03-00")!
    const result = formatAbsoluteParts(monthOnly, "d")
    assert.deepEqual(result, [
      { text: "2025", precise: true },
      { text: "-", precise: true },
      { text: "03", precise: true },
      { text: "-", precise: true },
      { text: "00", precise: false },
    ])
  })
})

describe("calculateTimeUnits", () => {
  test("calculates correct units for seconds", () => {
    const result = calculateTimeUnits(5000)
    assert.equal(result.seconds, 5)
    assert.equal(result.minutes, 0)
  })

  test("calculates correct units for minutes", () => {
    const result = calculateTimeUnits(120000)
    assert.equal(result.seconds, 120)
    assert.equal(result.minutes, 2)
    assert.equal(result.hours, 0)
  })

  test("calculates correct units for hours", () => {
    const result = calculateTimeUnits(7200000)
    assert.equal(result.hours, 2)
    assert.equal(result.days, 0)
  })

  test("calculates correct units for days", () => {
    const result = calculateTimeUnits(172800000)
    assert.equal(result.days, 2)
    assert.equal(result.months, 0)
  })

  test("calculates correct units for years", () => {
    const result = calculateTimeUnits(365 * 24 * 60 * 60 * 1000 * 2)
    assert.equal(result.years, 2)
    assert.equal(result.kiloYears, 0)
  })

  test("handles negative time differences", () => {
    const result = calculateTimeUnits(-60000)
    assert.equal(result.seconds, 60)
    assert.equal(result.minutes, 1)
  })
})

describe("getRelativeTimeInfo", () => {
  test("returns seconds for small differences", () => {
    const result = getRelativeTimeInfo(5000)
    assert.equal(result.unit, "seconds")
    assert.equal(result.count, 5)
    assert.isTrue(result.isPast)
    assert.equal(result.nextUpdateMs, 1000)
  })

  test("returns minutes for medium differences", () => {
    const result = getRelativeTimeInfo(120000)
    assert.equal(result.unit, "minutes")
    assert.equal(result.count, 2)
  })

  test("returns hours for larger differences", () => {
    const result = getRelativeTimeInfo(7200000)
    assert.equal(result.unit, "hours")
    assert.equal(result.count, 2)
  })

  test("returns days for multi-day differences", () => {
    const result = getRelativeTimeInfo(172800000)
    assert.equal(result.unit, "days")
    assert.equal(result.count, 2)
  })

  test("returns years for multi-year differences", () => {
    const result = getRelativeTimeInfo(365 * 24 * 60 * 60 * 1000 * 2)
    assert.equal(result.unit, "years")
    assert.equal(result.count, 2)
  })

  test("indicates future time for negative differences", () => {
    const result = getRelativeTimeInfo(-60000)
    assert.isFalse(result.isPast)
  })

  test("returns kiloYears for very large differences", () => {
    const result = getRelativeTimeInfo(1000 * 365 * 24 * 60 * 60 * 1000)
    assert.equal(result.unit, "kiloYears")
    assert.equal(result.count, 1)
  })
})

describe("formatAbsoluteLocalizedParts", () => {
  function localizedText(timestamp: string, precision: Parameters<typeof formatAbsoluteLocalizedParts>[1], locale: string, timeZone?: string): string {
    const parsed = parseTimestamp(timestamp)
    assert.isNotNull(parsed)
    return formatAbsoluteLocalizedParts(parsed, precision, locale, timeZone)
      .map((p) => p.text)
      .join("")
  }

  test("formats day precision per locale without timezone shifts", () => {
    assert.equal(localizedText("2020-06-17 10:02:37", "d", "sl"), "17. 6. 2020")
    assert.equal(localizedText("2020-06-17 10:02:37", "d", "en-GB"), "17/06/2020")
    // Calendar dates render as stored even with a location.
    assert.equal(localizedText("2020-06-17 00:00:00", "d", "en-GB", "Asia/Tokyo"), "17/06/2020")
  })

  test("formats month precision as month name and year", () => {
    assert.equal(localizedText("2020-06-01 00:00:00", "m", "sl"), "junij 2020")
    assert.equal(localizedText("2020-06-01 00:00:00", "m", "en"), "June 2020")
  })

  test("formats second precision with the time in the timezone of the environment", () => {
    // The same instant expressed in a location and in UTC must render identically, whatever the
    // timezone of the environment is (Europe/Ljubljana is UTC+2 in June).
    assert.equal(localizedText("2020-06-17 12:02:37", "s", "en-GB", "Europe/Ljubljana"), localizedText("2020-06-17 10:02:37", "s", "en-GB"))
    const text = localizedText("2020-06-17 10:02:37", "s", "en-GB")
    assert.match(text, /\d{2}:\d{2}:\d{2}/)
  })

  test("falls back to the plain rendering for year and coarser precisions", () => {
    const parsed = parseTimestamp("2020-01-01 00:00:00")
    assert.isNotNull(parsed)
    assert.deepEqual(formatAbsoluteLocalizedParts(parsed, "y", "sl"), formatAbsoluteParts(parsed, "y"))
    const coarse = parseTimestamp("2000-01-01 00:00:00")
    assert.isNotNull(coarse)
    assert.deepEqual(formatAbsoluteLocalizedParts(coarse, "100y", "sl"), formatAbsoluteParts(coarse, "100y"))
  })

  test("falls back to the plain rendering for years outside the Date range", () => {
    const parsed = parseTimestamp("1000000-01-01 00:00:00")
    assert.isNotNull(parsed)
    assert.deepEqual(formatAbsoluteLocalizedParts(parsed, "d", "sl"), formatAbsoluteParts(parsed, "d"))
  })
})

describe("zonedEpochMs", () => {
  test("interprets wall time in the given timezone", () => {
    const summer = parseTimestamp("2020-06-17 12:02:37")
    assert.isNotNull(summer)
    // Europe/Ljubljana is UTC+2 in June.
    assert.equal(zonedEpochMs(summer, "Europe/Ljubljana"), Date.UTC(2020, 5, 17, 10, 2, 37))
    const winter = parseTimestamp("2020-01-17 12:02:37")
    assert.isNotNull(winter)
    // Europe/Ljubljana is UTC+1 in January.
    assert.equal(zonedEpochMs(winter, "Europe/Ljubljana"), Date.UTC(2020, 0, 17, 11, 2, 37))
  })

  test("returns null for an unknown timezone", () => {
    const parsed = parseTimestamp("2020-06-17 12:02:37")
    assert.isNotNull(parsed)
    assert.isNull(zonedEpochMs(parsed, "Not/AZone"))
  })
})

describe("convertParsedToUtc", () => {
  test("converts wall time in a timezone to UTC fields", () => {
    const parsed = parseTimestamp("2020-06-17 00:30:00")
    assert.isNotNull(parsed)
    const utc = convertParsedToUtc(parsed, "Asia/Tokyo")
    assert.isNotNull(utc)
    // Asia/Tokyo is UTC+9, so the date rolls back to the previous day.
    assert.equal(utc.parts.year, 2020)
    assert.equal(utc.parts.month, 6)
    assert.equal(utc.parts.day, 16)
    assert.equal(utc.parts.hours, 15)
    assert.equal(utc.parts.minutes, 30)
    assert.equal(utc.parts.seconds, 0)
  })
})
