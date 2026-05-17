import { assert, describe, test } from "vitest"

import { timeFloat64, validateTime } from "@/document/time"
import { timePrecisionForRange, timePrecisionForValue, timeStringFromFloat64 } from "@/utils"

// Unix seconds for 2025-03-02 10:30:45 UTC.
const SAMPLE_SECONDS = Date.UTC(2025, 2, 2, 10, 30, 45) / 1000

describe("timePrecisionForRange", () => {
  test("returns s for spans under an hour", () => {
    assert.equal(timePrecisionForRange(0, 0), "s")
    assert.equal(timePrecisionForRange(0, 30), "s")
    assert.equal(timePrecisionForRange(0, 60), "s")
    assert.equal(timePrecisionForRange(0, 60 * 59), "s")
  })

  test("returns min for spans from an hour up to a day", () => {
    assert.equal(timePrecisionForRange(0, 60 * 60), "min")
    assert.equal(timePrecisionForRange(0, 60 * 60 * 12), "min")
  })

  test("returns h for spans from a day up to a month", () => {
    assert.equal(timePrecisionForRange(0, 60 * 60 * 24), "h")
    assert.equal(timePrecisionForRange(0, 60 * 60 * 24 * 15), "h")
  })

  test("returns d for spans from a month up to a year", () => {
    assert.equal(timePrecisionForRange(0, 60 * 60 * 24 * 30), "d")
    assert.equal(timePrecisionForRange(0, 60 * 60 * 24 * 200), "d")
  })

  test("returns m for spans from a year up to a decade", () => {
    const year = 60 * 60 * 24 * 365
    assert.equal(timePrecisionForRange(0, year), "m")
    assert.equal(timePrecisionForRange(0, year * 5), "m")
  })

  test("returns coarser precisions for larger spans", () => {
    const year = 60 * 60 * 24 * 365
    assert.equal(timePrecisionForRange(0, year * 50), "y")
    assert.equal(timePrecisionForRange(0, year * 500), "10y")
    assert.equal(timePrecisionForRange(0, year * 5_000), "100y")
    assert.equal(timePrecisionForRange(0, year * 50_000), "k")
    assert.equal(timePrecisionForRange(0, year * 500_000), "10k")
    assert.equal(timePrecisionForRange(0, year * 5_000_000), "100k")
    assert.equal(timePrecisionForRange(0, year * 50_000_000), "M")
    assert.equal(timePrecisionForRange(0, year * 500_000_000), "10M")
    assert.equal(timePrecisionForRange(0, year * 5_000_000_000), "100M")
  })

  test("ignores argument order", () => {
    // 2 hours falls in the "min" tier under the current mapping.
    assert.equal(timePrecisionForRange(60 * 60 * 2, 0), "min")
    assert.equal(timePrecisionForRange(0, -60 * 60 * 2), "min")
  })
})

describe("timeStringFromFloat64", () => {
  test("formats at second precision", () => {
    assert.equal(timeStringFromFloat64(SAMPLE_SECONDS, "s"), "2025-03-02 10:30:45")
  })

  test("formats at minute precision (drops seconds)", () => {
    assert.equal(timeStringFromFloat64(SAMPLE_SECONDS, "min"), "2025-03-02 10:30")
  })

  test("formats at hour precision (minutes pinned to :00)", () => {
    assert.equal(timeStringFromFloat64(SAMPLE_SECONDS, "h"), "2025-03-02 10:00")
  })

  test("formats at day precision", () => {
    assert.equal(timeStringFromFloat64(SAMPLE_SECONDS, "d"), "2025-03-02")
  })

  test("formats at month precision with day=00", () => {
    assert.equal(timeStringFromFloat64(SAMPLE_SECONDS, "m"), "2025-03-00")
  })

  test("formats at year precision", () => {
    assert.equal(timeStringFromFloat64(SAMPLE_SECONDS, "y"), "2025")
  })

  test("rounds year down for decade precision", () => {
    assert.equal(timeStringFromFloat64(SAMPLE_SECONDS, "10y"), "2020")
  })

  test("rounds year down for century precision", () => {
    assert.equal(timeStringFromFloat64(SAMPLE_SECONDS, "100y"), "2000")
  })

  test("rounds year down for kiloyear precision", () => {
    assert.equal(timeStringFromFloat64(SAMPLE_SECONDS, "k"), "2000")
  })

  test("rounds year down for megayear precision", () => {
    assert.equal(timeStringFromFloat64(SAMPLE_SECONDS, "M"), "0000")
  })

  test("pads short positive years to four digits", () => {
    // Year 1 CE is unix epoch − ~62135596800 s.
    const seconds = -62_135_596_800 + 60 * 60 * 24
    const result = timeStringFromFloat64(seconds, "y")
    assert.equal(result, "0001")
  })

  test("formats negative years with leading minus and zero padding", () => {
    // Roughly -45 BCE (well before unix epoch).
    const year = 60 * 60 * 24 * 365
    const result = timeStringFromFloat64(-2_000 * year, "y")
    assert.match(result, /^-\d{4}$/)
  })

  test("output round-trips through the claim parser at the same precision", () => {
    for (const precision of ["s", "min", "h", "d", "m", "y", "10y", "100y", "k"] as const) {
      const s = timeStringFromFloat64(SAMPLE_SECONDS, precision)
      // validateTime throws on bad format or precision mismatch.
      validateTime(s, precision)
      // timeFloat64 (with explicit precision) re-derives a float that should
      // be the start of the precision window, i.e. <= the original.
      const roundTripped = timeFloat64(s, precision)
      assert.isAtMost(roundTripped, SAMPLE_SECONDS)
    }
  })

  test("throws for subsecond precisions", () => {
    assert.throws(() => timeStringFromFloat64(SAMPLE_SECONDS, "ms"), /subsecond/)
    assert.throws(() => timeStringFromFloat64(SAMPLE_SECONDS, "us"), /subsecond/)
    assert.throws(() => timeStringFromFloat64(SAMPLE_SECONDS, "ns"), /subsecond/)
  })
})

describe("timePrecisionForValue", () => {
  test("returns s for fractional-second values", () => {
    assert.equal(timePrecisionForValue(0.5), "s")
    assert.equal(timePrecisionForValue(0.001), "s")
    assert.equal(timePrecisionForValue(SAMPLE_SECONDS + 0.5), "s")
  })

  test("returns s for non-minute-divisible integer seconds", () => {
    assert.equal(timePrecisionForValue(Date.UTC(2025, 2, 2, 10, 30, 45) / 1000), "s")
  })

  test("returns min when divisible by 60 but not 3600", () => {
    assert.equal(timePrecisionForValue(Date.UTC(2025, 2, 2, 10, 30) / 1000), "min")
  })

  test("returns h when divisible by 3600 but not 86400", () => {
    assert.equal(timePrecisionForValue(Date.UTC(2025, 2, 2, 10) / 1000), "h")
  })

  test("returns d when divisible by 86400 and day > 1", () => {
    assert.equal(timePrecisionForValue(Date.UTC(2025, 2, 2) / 1000), "d")
  })

  test("returns m when on day 1 of a non-January month", () => {
    assert.equal(timePrecisionForValue(Date.UTC(2025, 2, 1) / 1000), "m")
  })

  test("returns y on Jan 1 of a year not divisible by 10", () => {
    assert.equal(timePrecisionForValue(Date.UTC(2025, 0, 1) / 1000), "y")
  })

  test("returns 10y on Jan 1 of a decade year not divisible by 100", () => {
    assert.equal(timePrecisionForValue(Date.UTC(2020, 0, 1) / 1000), "10y")
    // Unix epoch year 1970 is divisible by 10 but not by 100.
    assert.equal(timePrecisionForValue(0), "10y")
  })

  test("returns 100y on Jan 1 of a century year not divisible by 1000", () => {
    assert.equal(timePrecisionForValue(Date.UTC(2100, 0, 1) / 1000), "100y")
  })

  test("returns k on Jan 1 of a kiloyear not divisible by 10000", () => {
    assert.equal(timePrecisionForValue(Date.UTC(2000, 0, 1) / 1000), "k")
  })

  test("tolerates small float64 rounding error", () => {
    // 60 + 1e-9 should still be treated as exactly divisible by 60.
    assert.equal(timePrecisionForValue(60 + 1e-9), "min")
    // Likewise on the negative side.
    assert.equal(timePrecisionForValue(60 - 1e-9), "min")
  })
})
