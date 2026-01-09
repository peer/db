import type { NamedValue } from "vue-i18n"

import type { TimePrecision } from "@/types"

import { assert, describe, test } from "vitest"

import { clampToMax, inferPrecisionFromNormalized, inferYearPrecision, normalizeForParsing, progressiveValidate } from "@/components/InputTime.vue"

// TODO: Enable once eslint parser for extra files is used.
//       See: https://github.com/ota-meshi/typescript-eslint-parser-for-extra-files/issues/162
// eslint-disable-next-line @typescript-eslint/no-unsafe-call
const normalizedForParsingExposed = (value: string) => normalizeForParsing(value)

describe("normalizeForParsing", () => {
  test("returns empty string for empty or whitespace input", () => {
    assert.equal(normalizedForParsingExposed(""), "")
    assert.equal(normalizedForParsingExposed("   "), "")
  })

  test("normalizes excessive whitespace", () => {
    assert.equal(normalizedForParsingExposed("  2022   "), "2022")
    assert.equal(normalizedForParsingExposed("2022-01-01    12:00"), "2022-01-01 12:00")

    assert.notEqual(normalizedForParsingExposed("2022   -   01   -   01"), "2022-01-01")
  })

  test("normalizes date–time boundary T", () => {
    assert.equal(normalizedForParsingExposed("2022-01-01T00:00:00"), "2022-01-01 00:00:00")
    assert.equal(normalizedForParsingExposed("2022-01-01t00:00:00"), "2022-01-01 00:00:00")
    assert.equal(normalizedForParsingExposed("2022-1-1T9:5"), "2022-1-1 9:5")
  })

  test("normalizes lowercase t globally (documented behavior)", () => {
    assert.equal(normalizedForParsingExposed("TEST"), "TEST")
    assert.equal(normalizedForParsingExposed("test"), "Test")
    assert.equal(normalizedForParsingExposed("t2022-01-01"), "T2022-01-01")

    assert.notEqual(normalizedForParsingExposed("test"), "TesT")
  })

  test("normalizes spaced date separators", () => {
    assert.equal(normalizedForParsingExposed("2022-01-01    12"), "2022-01-01 12")

    assert.notEqual(normalizedForParsingExposed("-2022 - 1 - 1T1"), "-2022-1-1 1")
  })

  test("removes trailing dash after year or year-month", () => {
    assert.equal(normalizedForParsingExposed("2022-"), "2022")
    assert.equal(normalizedForParsingExposed("2022-01-"), "2022-01")

    assert.notEqual(normalizedForParsingExposed("2022 - "), "2022-")
    assert.notEqual(normalizedForParsingExposed("2022 - 01 - "), "2022-01")
  })

  test("removes trailing T after full date", () => {
    assert.equal(normalizedForParsingExposed("2022-01-01T"), "2022-01-01")
    assert.equal(normalizedForParsingExposed("2022-1-1T   "), "2022-1-1")

    assert.notEqual(normalizedForParsingExposed("2022-1-1T   23"), "2022-1-1 23")
  })

  test("removes trailing colon in time", () => {
    assert.equal(normalizedForParsingExposed("2025-12-12 22:"), "2025-12-12 22")
    assert.equal(normalizedForParsingExposed("2025-12-12 22:12:"), "2025-12-12 22:12")

    assert.notEqual(normalizedForParsingExposed("2025:"), "12")
    assert.notEqual(normalizedForParsingExposed("2025:   "), "12")
    assert.notEqual(normalizedForParsingExposed("2025:12:"), "12:34")
    assert.notEqual(normalizedForParsingExposed("  2025:12:   "), "12:34")
    assert.notEqual(normalizedForParsingExposed("2025-12-12 22:12:12:"), "2025-12-12 22:12")
  })

  test("handles combined messy input", () => {
    assert.equal(normalizedForParsingExposed("2022-01-01T12:34:   "), "2022-01-01 12:34")

    assert.notEqual(normalizedForParsingExposed(" 2022 - 01 - 01 t 12 : 34 : "), "2022-01-01 12:34")
  })

  test("is idempotent for already-normalized input", () => {
    assert.equal(normalizedForParsingExposed("2022-01-01"), "2022-01-01")
    assert.equal(normalizedForParsingExposed("2022-01-01 12:34"), "2022-01-01 12:34")
    assert.equal(normalizedForParsingExposed("-2022-01-01 12:34:21"), "-2022-01-01 12:34:21")
  })
})

// TODO: Enable once eslint parser for extra files is used.
//       See: https://github.com/ota-meshi/typescript-eslint-parser-for-extra-files/issues/162
// eslint-disable-next-line @typescript-eslint/no-unsafe-call
const progressiveValidateExposed = (value: string) => progressiveValidate(value, (key: string, named?: NamedValue) => {
  if (named) {
    return `${key} ${JSON.stringify(named)}`
  } else {
    return key
  }
})

describe("progressiveValidate", () => {
  test("returns empty string for empty input", () => {
    assert.equal(progressiveValidateExposed(""), "")

    assert.notEqual(progressiveValidateExposed(""), "components.InputTime.errors.invalid")
  })

  test("allows year in progress", () => {
    assert.equal(progressiveValidateExposed("2"), "")
    assert.equal(progressiveValidateExposed("20"), "")
    assert.equal(progressiveValidateExposed("202"), "")
    assert.equal(progressiveValidateExposed("2023"), "")

    assert.notEqual(progressiveValidateExposed("2023"), "components.InputTime.errors.months0")
  })

  test("validates month in progress", () => {
    assert.equal(progressiveValidateExposed("2023"), "")
    assert.equal(progressiveValidateExposed("2023-0"), "")
    assert.equal(progressiveValidateExposed("2023-1"), "")
    assert.equal(progressiveValidateExposed("2023-12"), "")
    assert.equal(progressiveValidateExposed("2023-13"), "components.InputTime.errors.months0")

    assert.notEqual(progressiveValidateExposed("2023-12"), "components.InputTime.errors.invalid")
    assert.notEqual(progressiveValidateExposed("2023-13"), "")
  })

  test("validates day in progress", () => {
    assert.equal(progressiveValidateExposed("2023-1"), "")
    assert.equal(progressiveValidateExposed("2023-1-0"), "")
    assert.equal(progressiveValidateExposed("2023-1-1"), "")
    assert.equal(progressiveValidateExposed("2023-1-1 1"), "")
    assert.equal(progressiveValidateExposed("2023-0-1"), "components.InputTime.errors.daysNotZero")
    assert.equal(progressiveValidateExposed("2023-00-01"), "components.InputTime.errors.daysNotZero")
    assert.equal(progressiveValidateExposed("2023-13-1"), "components.InputTime.errors.months0")
    assert.equal(progressiveValidateExposed("2023-2-30"), `components.InputTime.errors.days0 {"maxDay":28}`)
    assert.equal(progressiveValidateExposed("2015-2-30"), `components.InputTime.errors.days0 {"maxDay":28}`)

    assert.notEqual(progressiveValidateExposed("2023-1-1"), "components.InputTime.errors.invalid")
    assert.notEqual(progressiveValidateExposed("2023-0-1"), "")
    assert.notEqual(progressiveValidateExposed("2023-13-1"), "")
    assert.notEqual(progressiveValidateExposed("2023-2-30"), "")
  })

  test("validates hours", () => {
    assert.equal(progressiveValidateExposed("2023-12-31 0"), "")
    assert.equal(progressiveValidateExposed("2023-12-31 23"), "")
    assert.equal(progressiveValidateExposed("2023-12-31 24"), "components.InputTime.errors.hours")
    assert.equal(progressiveValidateExposed("2023-13-31 12"), "components.InputTime.errors.months")
    assert.equal(progressiveValidateExposed("2023-2-30 12"), `components.InputTime.errors.days {"maxDay":28}`)

    assert.notEqual(progressiveValidateExposed("2023-12-31 23"), "components.InputTime.errors.invalid")
    assert.notEqual(progressiveValidateExposed("2023-12-31 24"), "")
    assert.notEqual(progressiveValidateExposed("2023-13-31 12"), "")
    assert.notEqual(progressiveValidateExposed("2023-2-30 12"), "")
  })

  test("validates minutes", () => {
    assert.equal(progressiveValidateExposed("2023-12-31 12:0"), "")
    assert.equal(progressiveValidateExposed("2023-12-31 12:59"), "")
    assert.equal(progressiveValidateExposed("2023-12-31 12:60"), "components.InputTime.errors.minutes")
    assert.equal(progressiveValidateExposed("2023-12-31 24:00"), "components.InputTime.errors.hours")

    assert.notEqual(progressiveValidateExposed("2023-12-31 12:59"), "components.InputTime.errors.invalid")
    assert.notEqual(progressiveValidateExposed("2023-12-31 12:60"), "")
    assert.notEqual(progressiveValidateExposed("2023-12-31 24:00"), "")
  })

  test("validates seconds", () => {
    assert.equal(progressiveValidateExposed("2023-12-31 12:34:0"), "")
    assert.equal(progressiveValidateExposed("2023-12-31 12:34:59"), "")
    assert.equal(progressiveValidateExposed("2023-12-31 12:34:60"), "components.InputTime.errors.seconds")
    assert.equal(progressiveValidateExposed("2023-12-31 12:60:00"), "components.InputTime.errors.minutes")

    assert.notEqual(progressiveValidateExposed("2023-12-31 12:34:59"), "components.InputTime.errors.invalid")
    assert.notEqual(progressiveValidateExposed("2023-12-31 12:34:60"), "")
    assert.notEqual(progressiveValidateExposed("2023-12-31 12:60:00"), "")
  })

  test("rejects invalid timestamp structure", () => {
    assert.equal(progressiveValidateExposed("foo"), "components.InputTime.errors.invalid")
    assert.equal(progressiveValidateExposed("2023--12"), "components.InputTime.errors.invalid")

    assert.notEqual(progressiveValidateExposed("foo"), "")
    assert.notEqual(progressiveValidateExposed("2023--12"), "")
  })
})

// TODO: Enable once eslint parser for extra files is used.
//       See: https://github.com/ota-meshi/typescript-eslint-parser-for-extra-files/issues/162
// eslint-disable-next-line @typescript-eslint/no-unsafe-call
const clampToMaxExposed = (p: TimePrecision, max: TimePrecision) => clampToMax(p, max)

describe("clampToMax", () => {
  test("returns precision when it is equal to max", () => {
    assert.equal(clampToMaxExposed("y", "y"), "y")
    assert.notEqual(clampToMaxExposed("y", "y"), "m")
  })

  test("returns precision when it is more precise than max", () => {
    assert.equal(clampToMaxExposed("d", "m"), "d")
    assert.equal(clampToMaxExposed("s", "h"), "s")

    assert.notEqual(clampToMaxExposed("d", "m"), "m")
    assert.notEqual(clampToMaxExposed("s", "h"), "h")
  })

  test("clamps to max when precision is less precise than max", () => {
    assert.equal(clampToMaxExposed("y", "m"), "m")
    assert.equal(clampToMaxExposed("G", "y"), "y")
    assert.equal(clampToMaxExposed("100M", "k"), "k")

    assert.notEqual(clampToMaxExposed("y", "m"), "y")
    assert.notEqual(clampToMaxExposed("G", "y"), "G")
    assert.notEqual(clampToMaxExposed("100M", "k"), "100M")
  })

  test("handles boundary neighbors correctly", () => {
    assert.equal(clampToMaxExposed("10y", "y"), "y")
    assert.equal(clampToMaxExposed("y", "10y"), "y")

    assert.notEqual(clampToMaxExposed("10y", "y"), "10y")
    assert.notEqual(clampToMaxExposed("y", "10y"), "10y")
  })

  test("handles extreme precision differences", () => {
    assert.equal(clampToMaxExposed("G", "s"), "s")
    assert.equal(clampToMaxExposed("s", "G"), "s")

    assert.notEqual(clampToMaxExposed("G", "s"), "G")
    assert.notEqual(clampToMaxExposed("s", "G"), "G")
  })

  test("throws for unknown precision", () => {
    assert.throws(() => clampToMaxExposed("unknown" as TimePrecision, "y"), /unknown precision/)
  })

  test("throws for unknown max precision", () => {
    assert.throws(() => clampToMaxExposed("y", "unknown" as TimePrecision), /unknown maxPrecision/)
  })
})

// TODO: Enable once eslint parser for extra files is used.
//       See: https://github.com/ota-meshi/typescript-eslint-parser-for-extra-files/issues/162
// eslint-disable-next-line @typescript-eslint/no-unsafe-call
const inferYearPrecisionExposed = (year: string, max: TimePrecision) => inferYearPrecision(year, max)

describe("inferYearPrecision", () => {
  test("defaults to year when input is empty", () => {
    assert.equal(inferYearPrecisionExposed("", "s"), "s")
    assert.equal(inferYearPrecisionExposed("", "y"), "y")

    assert.notEqual(inferYearPrecisionExposed("", "s"), "y")
  })

  test("infers giga-years precision", () => {
    assert.equal(inferYearPrecisionExposed("1000000000", "G"), "G")

    assert.notEqual(inferYearPrecisionExposed("1000000000", "G"), "100M")
  })

  test("infers 10 year precision", () => {
    assert.equal(inferYearPrecisionExposed("10", "G"), "y")

    assert.notEqual(inferYearPrecisionExposed("10", "G"), "10y")
  })

  test("infers 10 kiloyears precision", () => {
    assert.equal(inferYearPrecisionExposed("10000", "G"), "10k")

    assert.notEqual(inferYearPrecisionExposed("10000", "G"), "y")
  })

  test("infers ten-million precision", () => {
    assert.equal(inferYearPrecisionExposed("30000000", "G"), "10M")

    assert.notEqual(inferYearPrecisionExposed("30000000", "G"), "M")
  })

  test("infers million precision", () => {
    assert.equal(inferYearPrecisionExposed("4000000", "G"), "M")

    assert.notEqual(inferYearPrecisionExposed("4000000", "G"), "100k")
  })

  test("infers thousand-scale precisions", () => {
    assert.equal(inferYearPrecisionExposed("500000", "G"), "100k")
    assert.equal(inferYearPrecisionExposed("60000", "G"), "10k")
    assert.equal(inferYearPrecisionExposed("7000", "G"), "y")

    assert.notEqual(inferYearPrecisionExposed("7000", "G"), "k")
    assert.notEqual(inferYearPrecisionExposed("7000", "G"), "100y")
  })

  test("infers century and decade precision", () => {
    assert.equal(inferYearPrecisionExposed("10100", "G"), "100y")
    assert.equal(inferYearPrecisionExposed("10110", "G"), "10y")

    assert.notEqual(inferYearPrecisionExposed("12000", "G"), "100y")
    assert.notEqual(inferYearPrecisionExposed("10110", "G"), "y")
  })

  test("does not infer higher precision for years <= 9999", () => {
    assert.equal(inferYearPrecisionExposed("9999", "G"), "y")
    assert.equal(inferYearPrecisionExposed("-9999", "G"), "y")

    assert.notEqual(inferYearPrecisionExposed("9999", "G"), "10y")
  })

  test("handles negative years symmetrically", () => {
    assert.equal(inferYearPrecisionExposed("-1000000", "G"), "M")

    assert.notEqual(inferYearPrecisionExposed("-1000000", "G"), "100k")
  })

  test("respects max precision clamp", () => {
    assert.equal(inferYearPrecisionExposed("1000000", "10k"), "10k")

    assert.notEqual(inferYearPrecisionExposed("1000000", "10k"), "M")
  })

  test("defaults to year when no candidate matches", () => {
    assert.equal(inferYearPrecisionExposed("12345", "G"), "y")

    assert.notEqual(inferYearPrecisionExposed("12345", "G"), "10y")
  })

  test("clamps inferred precision to max", () => {
    assert.equal(inferYearPrecisionExposed("1000000000", "y"), "y")
    assert.equal(inferYearPrecisionExposed("1000000000", "s"), "s")

    assert.notEqual(inferYearPrecisionExposed("1000000000", "s"), "G")
  })
})

// TODO: Enable once eslint parser for extra files is used.
//       See: https://github.com/ota-meshi/typescript-eslint-parser-for-extra-files/issues/162
const inferPrecisionFromNormalizedExposed = (
  normalized: string,
  timeStruct: { y: string; m: string; d: string; h: string; min: string; s: string },
  max: TimePrecision,
  precision: TimePrecision,
) =>
  // eslint-disable-next-line @typescript-eslint/no-unsafe-call
  inferPrecisionFromNormalized(normalized, timeStruct, max, precision)

describe("inferPrecisionFromNormalized", () => {
  test("infers seconds precision", () => {
    assert.equal(inferPrecisionFromNormalizedExposed("2023-12-31 12:34:56", { y: "2023", m: "12", d: "31", h: "12", min: "34", s: "56" }, "G", "y"), "s")

    assert.notEqual(inferPrecisionFromNormalizedExposed("2023-12-31 12:34:56", { y: "2023", m: "12", d: "31", h: "12", min: "34", s: "56" }, "G", "y"), "min")
  })

  test("infers minutes precision", () => {
    assert.equal(inferPrecisionFromNormalizedExposed("2023-12-31 12:34", { y: "2023", m: "12", d: "31", h: "12", min: "34", s: "" }, "G", "y"), "min")

    assert.notEqual(inferPrecisionFromNormalizedExposed("2023-12-31 12:34", { y: "2023", m: "12", d: "31", h: "12", min: "34", s: "" }, "G", "y"), "h")
  })

  test("infers hour precision", () => {
    assert.equal(inferPrecisionFromNormalizedExposed("2023-12-31 12", { y: "2023", m: "12", d: "31", h: "12", min: "", s: "" }, "G", "y"), "h")

    assert.notEqual(inferPrecisionFromNormalizedExposed("2023-12-31 12", { y: "2023", m: "12", d: "31", h: "12", min: "", s: "" }, "G", "y"), "d")
  })

  test("infers day precision", () => {
    assert.equal(inferPrecisionFromNormalizedExposed("2023-12-31", { y: "2023", m: "12", d: "31", h: "", min: "", s: "" }, "G", "y"), "d")

    assert.notEqual(inferPrecisionFromNormalizedExposed("2023-12-31", { y: "2023", m: "12", d: "31", h: "", min: "", s: "" }, "G", "y"), "m")
  })

  test("returns month precision when days are zero", () => {
    assert.equal(inferPrecisionFromNormalizedExposed("2023-12-00", { y: "2023", m: "12", d: "00", h: "", min: "", s: "" }, "G", "y"), "m")

    assert.notEqual(inferPrecisionFromNormalizedExposed("2023-12-00", { y: "2023", m: "12", d: "00", h: "", min: "", s: "" }, "G", "y"), "d")
  })

  test("returns years precision when days and months are zero", () => {
    assert.equal(inferPrecisionFromNormalizedExposed("2023-00-00", { y: "2023", m: "00", d: "00", h: "", min: "", s: "" }, "G", "y"), "y")

    assert.notEqual(inferPrecisionFromNormalizedExposed("2023-00-00", { y: "2023", m: "00", d: "00", h: "", min: "", s: "" }, "G", "y"), "d")
  })

  test("infers month precision", () => {
    assert.equal(inferPrecisionFromNormalizedExposed("2023-12", { y: "2023", m: "12", d: "", h: "", min: "", s: "" }, "G", "y"), "m")

    assert.notEqual(inferPrecisionFromNormalizedExposed("2023-12", { y: "2023", m: "12", d: "", h: "", min: "", s: "" }, "G", "y"), "y")
  })

  test("infers year precision when month is zero", () => {
    assert.equal(inferPrecisionFromNormalizedExposed("2023-00", { y: "2023", m: "00", d: "", h: "", min: "", s: "" }, "G", "m"), "y")

    assert.notEqual(inferPrecisionFromNormalizedExposed("2023-00", { y: "2023", m: "00", d: "", h: "", min: "", s: "" }, "G", "m"), "m")
  })

  test("falls back to inferred year precision when only year is present", () => {
    assert.equal(inferPrecisionFromNormalizedExposed("1000000", { y: "1000000", m: "", d: "", h: "", min: "", s: "" }, "G", "y"), "M")

    assert.notEqual(inferPrecisionFromNormalizedExposed("1000000", { y: "1000000", m: "", d: "", h: "", min: "", s: "" }, "G", "y"), "y")
  })

  test("falls back to provided precision when no match is possible", () => {
    assert.equal(inferPrecisionFromNormalizedExposed("foo", { y: "", m: "", d: "", h: "", min: "", s: "" }, "G", "m"), "m")

    assert.notEqual(inferPrecisionFromNormalizedExposed("foo", { y: "", m: "", d: "", h: "", min: "", s: "" }, "G", "m"), "y")
  })
})
