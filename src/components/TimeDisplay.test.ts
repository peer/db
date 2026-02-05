import type { TimePrecision } from "@/document"

import { assert, describe, test } from "vitest"

import {
  calculateTimeUnits,
  detectPrecision,
  formatAbsoluteParts,
  formatYearParts,
  getPrecisionIndex,
  getRelativeTimeInfo,
  isPrecise,
  parseTimestamp,
} from "@/components/TimeDisplay.vue"

// Expose functions to avoid eslint errors
// TODO: Enable once eslint parser for extra files is used.
//       See: https://github.com/ota-meshi/typescript-eslint-parser-for-extra-files/issues/162
// eslint-disable-next-line @typescript-eslint/no-unsafe-call
const parseTimestampExposed = (value: string) => parseTimestamp(value)
// eslint-disable-next-line @typescript-eslint/no-unsafe-call
const getPrecisionIndexExposed = (value: TimePrecision) => getPrecisionIndex(value)
// eslint-disable-next-line @typescript-eslint/no-unsafe-call
const isPreciseExposed = (level: TimePrecision, precision: TimePrecision) => isPrecise(level, precision)
// eslint-disable-next-line @typescript-eslint/no-unsafe-call
const formatYearPartsExposed = (yearStr: string, precision: TimePrecision) => formatYearParts(yearStr, precision)
// eslint-disable-next-line @typescript-eslint/no-unsafe-call
const calculateTimeUnitsExposed = (diffMs: number) => calculateTimeUnits(diffMs)
// eslint-disable-next-line @typescript-eslint/no-unsafe-call
const getRelativeTimeInfoExposed = (diffMs: number) => getRelativeTimeInfo(diffMs)

const formatAbsolutePartsExposed = (
  parsed: {
    year: string
    month: string
    day: string
    hour: string
    minute: string
    second: string
  },
  precision: TimePrecision,
) =>
  // eslint-disable-next-line @typescript-eslint/no-unsafe-call
  formatAbsoluteParts(parsed, precision)

const detectPrecisionExposed = (parsed: { year: string; month: string; day: string; hour: string; minute: string; second: string }) =>
  // eslint-disable-next-line @typescript-eslint/no-unsafe-call
  detectPrecision(parsed)

describe("parseTimestamp", () => {
  test("parses valid timestamp", () => {
    const result = parseTimestampExposed("2025-03-02T10:30:45Z")
    assert.deepEqual(result, {
      year: "2025",
      month: "03",
      day: "02",
      hour: "10",
      minute: "30",
      second: "45",
    })
  })

  test("parses negative year timestamp", () => {
    const result = parseTimestampExposed("-4500000000-01-01T00:00:00Z")
    assert.deepEqual(result, {
      year: "-4500000000",
      month: "01",
      day: "01",
      hour: "00",
      minute: "00",
      second: "00",
    })
  })

  test("returns null for invalid timestamp", () => {
    assert.isNull(parseTimestampExposed("invalid"))
    assert.isNull(parseTimestampExposed("2025-03-02"))
    assert.isNull(parseTimestampExposed("2025-03-02T10:30:45"))
  })
})

describe("getPrecisionIndex", () => {
  test("returns correct index for each precision level", () => {
    assert.equal(getPrecisionIndexExposed("G"), 0)
    assert.equal(getPrecisionIndexExposed("y"), 9)
    assert.equal(getPrecisionIndexExposed("s"), 14)
  })
})

describe("isPrecise", () => {
  test("year precision includes only year and above", () => {
    assert.isTrue(isPreciseExposed("G", "y"))
    assert.isTrue(isPreciseExposed("y", "y"))
    assert.isFalse(isPreciseExposed("m", "y"))
    assert.isFalse(isPreciseExposed("d", "y"))
  })

  test("month precision includes month and above", () => {
    assert.isTrue(isPreciseExposed("y", "m"))
    assert.isTrue(isPreciseExposed("m", "m"))
    assert.isFalse(isPreciseExposed("d", "m"))
  })

  test("second precision includes all levels", () => {
    assert.isTrue(isPreciseExposed("G", "s"))
    assert.isTrue(isPreciseExposed("y", "s"))
    assert.isTrue(isPreciseExposed("s", "s"))
  })
})

describe("formatYearParts", () => {
  test("returns full year for year precision", () => {
    const result = formatYearPartsExposed("2025", "y")
    assert.deepEqual(result, [{ text: "2025", precise: true }])
  })

  test("grays out trailing zeros for giga-year precision", () => {
    const result = formatYearPartsExposed("-4500000000", "G")
    assert.deepEqual(result, [
      { text: "-4", precise: true },
      { text: "500000000", precise: false },
    ])
  })

  test("grays out trailing zeros for mega-year precision", () => {
    const result = formatYearPartsExposed("100000000", "100M")
    assert.deepEqual(result, [
      { text: "1", precise: true },
      { text: "00000000", precise: false },
    ])
  })

  test("grays out trailing zeros for kilo-year precision", () => {
    const result = formatYearPartsExposed("10000", "k")
    assert.deepEqual(result, [
      { text: "10", precise: true },
      { text: "000", precise: false },
    ])
  })

  test("grays out trailing zeros for decade precision", () => {
    const result = formatYearPartsExposed("2020", "10y")
    assert.deepEqual(result, [
      { text: "202", precise: true },
      { text: "0", precise: false },
    ])
  })

  test("handles negative years correctly", () => {
    const result = formatYearPartsExposed("-5000", "k")
    assert.deepEqual(result, [
      { text: "-5", precise: true },
      { text: "000", precise: false },
    ])
  })

  test("splits year correctly for kilo-year precision", () => {
    // "2025" with kilo-year precision means "2" is the thousands digit, "025" is imprecise.
    const result = formatYearPartsExposed("2025", "k")
    assert.deepEqual(result, [
      { text: "2", precise: true },
      { text: "025", precise: false },
    ])
  })

  test("returns full year if shorter than trailing zeros count", () => {
    // "25" with kilo-year precision has only 2 digits, less than 3 trailing zeros.
    const result = formatYearPartsExposed("25", "k")
    assert.deepEqual(result, [{ text: "25", precise: true }])
  })
})

describe("formatAbsoluteParts", () => {
  const parsed = {
    year: "2025",
    month: "03",
    day: "02",
    hour: "10",
    minute: "30",
    second: "45",
  }

  test("shows only year for year precision", () => {
    const result = formatAbsolutePartsExposed(parsed, "y")
    assert.deepEqual(result, [{ text: "2025", precise: true }])
  })

  test("shows YYYY-MM with day as 00 for month precision", () => {
    const result = formatAbsolutePartsExposed(parsed, "m")
    assert.deepEqual(result, [
      { text: "2025", precise: true },
      { text: "-", precise: true },
      { text: "03", precise: true },
      { text: "-", precise: false },
      { text: "00", precise: false },
    ])
  })

  test("shows full date for day precision", () => {
    const result = formatAbsolutePartsExposed(parsed, "d")
    assert.deepEqual(result, [
      { text: "2025", precise: true },
      { text: "-", precise: true },
      { text: "03", precise: true },
      { text: "-", precise: true },
      { text: "02", precise: true },
    ])
  })

  test("shows date and HH:00 for hour precision", () => {
    const result = formatAbsolutePartsExposed(parsed, "h")
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
    const result = formatAbsolutePartsExposed(parsed, "min")
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
    const result = formatAbsolutePartsExposed(parsed, "s")
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
})

describe("calculateTimeUnits", () => {
  test("calculates correct units for seconds", () => {
    const result = calculateTimeUnitsExposed(5000)
    assert.equal(result.seconds, 5)
    assert.equal(result.minutes, 0)
  })

  test("calculates correct units for minutes", () => {
    const result = calculateTimeUnitsExposed(120000)
    assert.equal(result.seconds, 120)
    assert.equal(result.minutes, 2)
    assert.equal(result.hours, 0)
  })

  test("calculates correct units for hours", () => {
    const result = calculateTimeUnitsExposed(7200000)
    assert.equal(result.hours, 2)
    assert.equal(result.days, 0)
  })

  test("calculates correct units for days", () => {
    const result = calculateTimeUnitsExposed(172800000)
    assert.equal(result.days, 2)
    assert.equal(result.months, 0)
  })

  test("calculates correct units for years", () => {
    const result = calculateTimeUnitsExposed(365 * 24 * 60 * 60 * 1000 * 2)
    assert.equal(result.years, 2)
    assert.equal(result.kiloYears, 0)
  })

  test("handles negative time differences", () => {
    const result = calculateTimeUnitsExposed(-60000)
    assert.equal(result.seconds, 60)
    assert.equal(result.minutes, 1)
  })
})

describe("getRelativeTimeInfo", () => {
  test("returns seconds for small differences", () => {
    const result = getRelativeTimeInfoExposed(5000)
    assert.equal(result.unit, "seconds")
    assert.equal(result.count, 5)
    assert.isTrue(result.isPast)
    assert.equal(result.nextUpdateMs, 1000)
  })

  test("returns minutes for medium differences", () => {
    const result = getRelativeTimeInfoExposed(120000)
    assert.equal(result.unit, "minutes")
    assert.equal(result.count, 2)
  })

  test("returns hours for larger differences", () => {
    const result = getRelativeTimeInfoExposed(7200000)
    assert.equal(result.unit, "hours")
    assert.equal(result.count, 2)
  })

  test("returns days for multi-day differences", () => {
    const result = getRelativeTimeInfoExposed(172800000)
    assert.equal(result.unit, "days")
    assert.equal(result.count, 2)
  })

  test("returns years for multi-year differences", () => {
    const result = getRelativeTimeInfoExposed(365 * 24 * 60 * 60 * 1000 * 2)
    assert.equal(result.unit, "years")
    assert.equal(result.count, 2)
  })

  test("indicates future time for negative differences", () => {
    const result = getRelativeTimeInfoExposed(-60000)
    assert.isFalse(result.isPast)
  })

  test("returns kiloYears for very large differences", () => {
    const result = getRelativeTimeInfoExposed(1000 * 365 * 24 * 60 * 60 * 1000)
    assert.equal(result.unit, "kiloYears")
    assert.equal(result.count, 1)
  })
})

describe("detectPrecision", () => {
  test("detects second precision when seconds > 0", () => {
    const parsed = {
      year: "2025",
      month: "03",
      day: "02",
      hour: "10",
      minute: "30",
      second: "45",
    }
    assert.equal(detectPrecisionExposed(parsed), "s")
  })

  test("detects minute precision when minutes > 0 and seconds = 0", () => {
    const parsed = {
      year: "2025",
      month: "03",
      day: "02",
      hour: "10",
      minute: "30",
      second: "00",
    }
    assert.equal(detectPrecisionExposed(parsed), "min")
  })

  test("detects hour precision when hours > 0 and minutes/seconds = 0", () => {
    const parsed = {
      year: "2025",
      month: "03",
      day: "02",
      hour: "10",
      minute: "00",
      second: "00",
    }
    assert.equal(detectPrecisionExposed(parsed), "h")
  })

  test("detects day precision when day > 0 and time components = 0", () => {
    const parsed = {
      year: "2025",
      month: "03",
      day: "15",
      hour: "00",
      minute: "00",
      second: "00",
    }
    assert.equal(detectPrecisionExposed(parsed), "d")
  })

  test("detects day precision when month > 0 and day = 1, time = 0", () => {
    const parsed = {
      year: "2025",
      month: "06",
      day: "01",
      hour: "00",
      minute: "00",
      second: "00",
    }
    assert.equal(detectPrecisionExposed(parsed), "d")
  })

  test("detects day precision when all components are at their minimum", () => {
    const parsed = {
      year: "2025",
      month: "01",
      day: "01",
      hour: "00",
      minute: "00",
      second: "00",
    }
    assert.equal(detectPrecisionExposed(parsed), "d")
  })

  test("prioritizes seconds over minutes", () => {
    const parsed = {
      year: "2025",
      month: "03",
      day: "02",
      hour: "10",
      minute: "30",
      second: "01",
    }
    assert.equal(detectPrecisionExposed(parsed), "s")
  })

  test("prioritizes minutes over hours", () => {
    const parsed = {
      year: "2025",
      month: "03",
      day: "02",
      hour: "10",
      minute: "01",
      second: "00",
    }
    assert.equal(detectPrecisionExposed(parsed), "min")
  })

  test("prioritizes hours over days", () => {
    const parsed = {
      year: "2025",
      month: "03",
      day: "15",
      hour: "01",
      minute: "00",
      second: "00",
    }
    assert.equal(detectPrecisionExposed(parsed), "h")
  })

  test("prioritizes days over months", () => {
    const parsed = {
      year: "2025",
      month: "06",
      day: "15",
      hour: "00",
      minute: "00",
      second: "00",
    }
    assert.equal(detectPrecisionExposed(parsed), "d")
  })

  test("handles edge case: day = 1 should trigger day precision", () => {
    const parsed = {
      year: "2025",
      month: "03",
      day: "01",
      hour: "00",
      minute: "00",
      second: "00",
    }
    assert.equal(detectPrecisionExposed(parsed), "d")
  })

  test("handles edge case: month = 0 should not trigger month precision", () => {
    const parsed = {
      year: "2025",
      month: "00",
      day: "00",
      hour: "00",
      minute: "00",
      second: "00",
    }
    assert.equal(detectPrecisionExposed(parsed), "y")
  })

  test("handles edge case: all fields are 0, fallback to year precision", () => {
    const parsed = {
      year: "0",
      month: "00",
      day: "00",
      hour: "00",
      minute: "00",
      second: "00",
    }
    assert.equal(detectPrecisionExposed(parsed), "y")
  })
})
