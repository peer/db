import type { NamedValue } from "vue-i18n"

import type { TimePrecision } from "@/document"

import { assert, describe, test } from "vitest"

import {
  applyPrecision,
  clampToMax,
  getStructuredTime,
  inferPrecisionFromNormalized,
  inferYearPrecision,
  normalizeForParsing,
  progressiveValidate,
  toCanonicalString,
} from "@/partials/input/InputTime.format"

describe("normalizeForParsing", () => {
  test("returns empty string for empty or whitespace input", () => {
    assert.equal(normalizeForParsing(""), "")
    assert.equal(normalizeForParsing("   "), "")
  })

  test("normalizes excessive whitespace", () => {
    assert.equal(normalizeForParsing("  2022   "), "2022")
    assert.equal(normalizeForParsing("2022-01-01    12:00"), "2022-01-01 12:00")

    assert.notEqual(normalizeForParsing("2022   -   01   -   01"), "2022-01-01")
  })

  test("normalizes date–time boundary T", () => {
    assert.equal(normalizeForParsing("2022-01-01T00:00:00"), "2022-01-01 00:00:00")
    assert.equal(normalizeForParsing("2022-01-01t00:00:00"), "2022-01-01 00:00:00")
    assert.equal(normalizeForParsing("2022-1-1T9:5"), "2022-1-1 9:5")
  })

  test("normalizes lowercase t globally (documented behavior)", () => {
    assert.equal(normalizeForParsing("TEST"), "TEST")
    assert.equal(normalizeForParsing("test"), "Test")
    assert.equal(normalizeForParsing("t2022-01-01"), "T2022-01-01")

    assert.notEqual(normalizeForParsing("test"), "TesT")
  })

  test("normalizes spaced date separators", () => {
    assert.equal(normalizeForParsing("2022-01-01    12"), "2022-01-01 12")

    assert.notEqual(normalizeForParsing("-2022 - 1 - 1T1"), "-2022-1-1 1")
  })

  test("removes trailing dash after year or year-month", () => {
    assert.equal(normalizeForParsing("2022-"), "2022")
    assert.equal(normalizeForParsing("2022-01-"), "2022-01")

    assert.notEqual(normalizeForParsing("2022 - "), "2022-")
    assert.notEqual(normalizeForParsing("2022 - 01 - "), "2022-01")
  })

  test("removes trailing T after full date", () => {
    assert.equal(normalizeForParsing("2022-01-01T"), "2022-01-01")
    assert.equal(normalizeForParsing("2022-1-1T   "), "2022-1-1")

    assert.notEqual(normalizeForParsing("2022-1-1T   23"), "2022-1-1 23")
  })

  test("removes trailing colon in time", () => {
    assert.equal(normalizeForParsing("2025-12-12 22:"), "2025-12-12 22")
    assert.equal(normalizeForParsing("2025-12-12 22:12:"), "2025-12-12 22:12")

    assert.notEqual(normalizeForParsing("2025:"), "12")
    assert.notEqual(normalizeForParsing("2025:   "), "12")
    assert.notEqual(normalizeForParsing("2025:12:"), "12:34")
    assert.notEqual(normalizeForParsing("  2025:12:   "), "12:34")
    assert.notEqual(normalizeForParsing("2025-12-12 22:12:12:"), "2025-12-12 22:12")
  })

  test("handles combined messy input", () => {
    assert.equal(normalizeForParsing("2022-01-01T12:34:   "), "2022-01-01 12:34")

    assert.notEqual(normalizeForParsing(" 2022 - 01 - 01 t 12 : 34 : "), "2022-01-01 12:34")
  })

  test("is idempotent for already-normalized input", () => {
    assert.equal(normalizeForParsing("2022-01-01"), "2022-01-01")
    assert.equal(normalizeForParsing("2022-01-01 12:34"), "2022-01-01 12:34")
    assert.equal(normalizeForParsing("-2022-01-01 12:34:21"), "-2022-01-01 12:34:21")
  })
})

// Wrapper to keep test call sites short: supplies a stub translator that
// echoes back the key (and any named values) so each assertion just compares
// the i18n key produced by progressiveValidate.
const progressiveValidateExposed = (value: string) =>
  progressiveValidate(value, (key: string, named?: NamedValue) => {
    if (named) {
      return `${key} ${JSON.stringify(named)}`
    } else {
      return key
    }
  })

describe("progressiveValidate", () => {
  test("returns empty string for empty input", () => {
    assert.equal(progressiveValidateExposed(""), "")

    assert.notEqual(progressiveValidateExposed(""), "partials.input.InputTime.errors.invalid")
  })

  test("allows year in progress", () => {
    assert.equal(progressiveValidateExposed("2"), "")
    assert.equal(progressiveValidateExposed("20"), "")
    assert.equal(progressiveValidateExposed("202"), "")
    assert.equal(progressiveValidateExposed("2023"), "")

    assert.notEqual(progressiveValidateExposed("2023"), "partials.input.InputTime.errors.months0")
  })

  test("validates month in progress", () => {
    assert.equal(progressiveValidateExposed("2023"), "")
    assert.equal(progressiveValidateExposed("2023-0"), "")
    assert.equal(progressiveValidateExposed("2023-1"), "")
    assert.equal(progressiveValidateExposed("2023-12"), "")
    assert.equal(progressiveValidateExposed("2023-13"), "partials.input.InputTime.errors.months0")

    assert.notEqual(progressiveValidateExposed("2023-12"), "partials.input.InputTime.errors.invalid")
    assert.notEqual(progressiveValidateExposed("2023-13"), "")
  })

  test("validates day in progress", () => {
    assert.equal(progressiveValidateExposed("2023-1"), "")
    assert.equal(progressiveValidateExposed("2023-1-0"), "")
    assert.equal(progressiveValidateExposed("2023-1-1"), "")
    assert.equal(progressiveValidateExposed("2023-1-1 1"), "")
    assert.equal(progressiveValidateExposed("2023-0-1"), "partials.input.InputTime.errors.daysNotZero")
    assert.equal(progressiveValidateExposed("2023-00-01"), "partials.input.InputTime.errors.daysNotZero")
    assert.equal(progressiveValidateExposed("2023-13-1"), "partials.input.InputTime.errors.months0")
    assert.equal(progressiveValidateExposed("2023-2-30"), `partials.input.InputTime.errors.days0 {"maxDay":28}`)
    assert.equal(progressiveValidateExposed("2015-2-30"), `partials.input.InputTime.errors.days0 {"maxDay":28}`)

    assert.notEqual(progressiveValidateExposed("2023-1-1"), "partials.input.InputTime.errors.invalid")
    assert.notEqual(progressiveValidateExposed("2023-0-1"), "")
    assert.notEqual(progressiveValidateExposed("2023-13-1"), "")
    assert.notEqual(progressiveValidateExposed("2023-2-30"), "")
  })

  test("validates hours", () => {
    assert.equal(progressiveValidateExposed("2023-12-31 0"), "")
    assert.equal(progressiveValidateExposed("2023-12-31 23"), "")
    assert.equal(progressiveValidateExposed("2023-12-31 24"), "partials.input.InputTime.errors.hours")
    assert.equal(progressiveValidateExposed("2023-13-31 12"), "partials.input.InputTime.errors.months")
    assert.equal(progressiveValidateExposed("2023-2-30 12"), `partials.input.InputTime.errors.days {"maxDay":28}`)

    assert.notEqual(progressiveValidateExposed("2023-12-31 23"), "partials.input.InputTime.errors.invalid")
    assert.notEqual(progressiveValidateExposed("2023-12-31 24"), "")
    assert.notEqual(progressiveValidateExposed("2023-13-31 12"), "")
    assert.notEqual(progressiveValidateExposed("2023-2-30 12"), "")
  })

  test("validates minutes", () => {
    assert.equal(progressiveValidateExposed("2023-12-31 12:0"), "")
    assert.equal(progressiveValidateExposed("2023-12-31 12:59"), "")
    assert.equal(progressiveValidateExposed("2023-12-31 12:60"), "partials.input.InputTime.errors.minutes")
    assert.equal(progressiveValidateExposed("2023-12-31 24:00"), "partials.input.InputTime.errors.hours")

    assert.notEqual(progressiveValidateExposed("2023-12-31 12:59"), "partials.input.InputTime.errors.invalid")
    assert.notEqual(progressiveValidateExposed("2023-12-31 12:60"), "")
    assert.notEqual(progressiveValidateExposed("2023-12-31 24:00"), "")
  })

  test("validates seconds", () => {
    assert.equal(progressiveValidateExposed("2023-12-31 12:34:0"), "")
    assert.equal(progressiveValidateExposed("2023-12-31 12:34:59"), "")
    assert.equal(progressiveValidateExposed("2023-12-31 12:34:60"), "partials.input.InputTime.errors.seconds")
    assert.equal(progressiveValidateExposed("2023-12-31 12:60:00"), "partials.input.InputTime.errors.minutes")

    assert.notEqual(progressiveValidateExposed("2023-12-31 12:34:59"), "partials.input.InputTime.errors.invalid")
    assert.notEqual(progressiveValidateExposed("2023-12-31 12:34:60"), "")
    assert.notEqual(progressiveValidateExposed("2023-12-31 12:60:00"), "")
  })

  test("rejects invalid time structure", () => {
    assert.equal(progressiveValidateExposed("foo"), "partials.input.InputTime.errors.invalid")
    assert.equal(progressiveValidateExposed("2023--12"), "partials.input.InputTime.errors.invalid")

    assert.notEqual(progressiveValidateExposed("foo"), "")
    assert.notEqual(progressiveValidateExposed("2023--12"), "")
  })
})

describe("clampToMax", () => {
  test("returns precision when it is equal to max", () => {
    assert.equal(clampToMax("y", "y"), "y")
    assert.notEqual(clampToMax("y", "y"), "m")
  })

  test("returns precision when it is more precise than max", () => {
    assert.equal(clampToMax("d", "m"), "d")
    assert.equal(clampToMax("s", "h"), "s")

    assert.notEqual(clampToMax("d", "m"), "m")
    assert.notEqual(clampToMax("s", "h"), "h")
  })

  test("clamps to max when precision is less precise than max", () => {
    assert.equal(clampToMax("y", "m"), "m")
    assert.equal(clampToMax("G", "y"), "y")
    assert.equal(clampToMax("100M", "k"), "k")

    assert.notEqual(clampToMax("y", "m"), "y")
    assert.notEqual(clampToMax("G", "y"), "G")
    assert.notEqual(clampToMax("100M", "k"), "100M")
  })

  test("handles boundary neighbors correctly", () => {
    assert.equal(clampToMax("10y", "y"), "y")
    assert.equal(clampToMax("y", "10y"), "y")

    assert.notEqual(clampToMax("10y", "y"), "10y")
    assert.notEqual(clampToMax("y", "10y"), "10y")
  })

  test("handles extreme precision differences", () => {
    assert.equal(clampToMax("G", "s"), "s")
    assert.equal(clampToMax("s", "G"), "s")

    assert.notEqual(clampToMax("G", "s"), "G")
    assert.notEqual(clampToMax("s", "G"), "G")
  })

  test("throws for unknown precision", () => {
    assert.throws(() => clampToMax("unknown" as TimePrecision, "y"), /unknown precision/)
  })

  test("throws for unknown max precision", () => {
    assert.throws(() => clampToMax("y", "unknown" as TimePrecision), /unknown maxPrecision/)
  })
})

describe("inferYearPrecision", () => {
  test("defaults to year when input is empty", () => {
    assert.equal(inferYearPrecision("", "s"), "s")
    assert.equal(inferYearPrecision("", "y"), "y")

    assert.notEqual(inferYearPrecision("", "s"), "y")
  })

  test("infers giga-years precision", () => {
    assert.equal(inferYearPrecision("1000000000", "G"), "G")

    assert.notEqual(inferYearPrecision("1000000000", "G"), "100M")
  })

  test("infers 10 year precision", () => {
    assert.equal(inferYearPrecision("10", "G"), "y")

    assert.notEqual(inferYearPrecision("10", "G"), "10y")
  })

  test("infers 10 kiloyears precision", () => {
    assert.equal(inferYearPrecision("10000", "G"), "10k")

    assert.notEqual(inferYearPrecision("10000", "G"), "y")
  })

  test("infers ten-million precision", () => {
    assert.equal(inferYearPrecision("30000000", "G"), "10M")

    assert.notEqual(inferYearPrecision("30000000", "G"), "M")
  })

  test("infers million precision", () => {
    assert.equal(inferYearPrecision("4000000", "G"), "M")

    assert.notEqual(inferYearPrecision("4000000", "G"), "100k")
  })

  test("infers thousand-scale precisions", () => {
    assert.equal(inferYearPrecision("500000", "G"), "100k")
    assert.equal(inferYearPrecision("60000", "G"), "10k")
    assert.equal(inferYearPrecision("7000", "G"), "y")

    assert.notEqual(inferYearPrecision("7000", "G"), "k")
    assert.notEqual(inferYearPrecision("7000", "G"), "100y")
  })

  test("infers century and decade precision", () => {
    assert.equal(inferYearPrecision("10100", "G"), "100y")
    assert.equal(inferYearPrecision("10110", "G"), "10y")

    assert.notEqual(inferYearPrecision("12000", "G"), "100y")
    assert.notEqual(inferYearPrecision("10110", "G"), "y")
  })

  test("does not infer higher precision for years <= 9999", () => {
    assert.equal(inferYearPrecision("9999", "G"), "y")
    assert.equal(inferYearPrecision("-9999", "G"), "y")

    assert.notEqual(inferYearPrecision("9999", "G"), "10y")
  })

  test("handles negative years symmetrically", () => {
    assert.equal(inferYearPrecision("-1000000", "G"), "M")

    assert.notEqual(inferYearPrecision("-1000000", "G"), "100k")
  })

  test("respects max precision clamp", () => {
    assert.equal(inferYearPrecision("1000000", "10k"), "10k")

    assert.notEqual(inferYearPrecision("1000000", "10k"), "M")
  })

  test("defaults to year when no candidate matches", () => {
    assert.equal(inferYearPrecision("12345", "G"), "y")

    assert.notEqual(inferYearPrecision("12345", "G"), "10y")
  })

  test("clamps inferred precision to max", () => {
    assert.equal(inferYearPrecision("1000000000", "y"), "y")
    assert.equal(inferYearPrecision("1000000000", "s"), "s")

    assert.notEqual(inferYearPrecision("1000000000", "s"), "G")
  })
})

// Wrapper to keep test call sites short: lets the test omit the sub field
// when it is irrelevant (which is the majority case for non-subsecond
// precisions) by defaulting it to "".
const inferPrecisionFromNormalizedExposed = (
  normalized: string,
  timeStruct: { y: string; m: string; d: string; h: string; min: string; s: string; sub?: string },
  max: TimePrecision,
  precision: TimePrecision,
) => inferPrecisionFromNormalized(normalized, { sub: "", ...timeStruct }, max, precision)

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

  test("infers ms precision (3 subsecond digits)", () => {
    assert.equal(inferPrecisionFromNormalizedExposed("2023-12-31 12:34:56.123", { y: "2023", m: "12", d: "31", h: "12", min: "34", s: "56", sub: "123" }, "G", "y"), "ms")
  })

  test("infers us precision (6 subsecond digits)", () => {
    assert.equal(
      inferPrecisionFromNormalizedExposed("2023-12-31 12:34:56.123456", { y: "2023", m: "12", d: "31", h: "12", min: "34", s: "56", sub: "123456" }, "G", "y"),
      "us",
    )
  })

  test("infers ns precision (9 subsecond digits)", () => {
    assert.equal(
      inferPrecisionFromNormalizedExposed("2023-12-31 12:34:56.123456789", { y: "2023", m: "12", d: "31", h: "12", min: "34", s: "56", sub: "123456789" }, "G", "y"),
      "ns",
    )
  })
})

describe("progressiveValidate (subseconds)", () => {
  // progressiveValidate returns "" for both valid and still-in-progress input;
  // it only returns a non-empty error message for definitively-invalid input.
  test("subseconds in progress (1-2 digits) accepted as still-typing", () => {
    assert.equal(progressiveValidateExposed("2023-12-31 12:34:56.1"), "")
    assert.equal(progressiveValidateExposed("2023-12-31 12:34:56.12"), "")
  })
  test("subseconds 3 digits valid (ms)", () => {
    assert.equal(progressiveValidateExposed("2023-12-31 12:34:56.123"), "")
  })
  test("subseconds 4-5 digits accepted as still-typing", () => {
    assert.equal(progressiveValidateExposed("2023-12-31 12:34:56.1234"), "")
    assert.equal(progressiveValidateExposed("2023-12-31 12:34:56.12345"), "")
  })
  test("subseconds 6 digits valid (us)", () => {
    assert.equal(progressiveValidateExposed("2023-12-31 12:34:56.123456"), "")
  })
  test("subseconds 7-8 digits accepted as still-typing", () => {
    assert.equal(progressiveValidateExposed("2023-12-31 12:34:56.1234567"), "")
    assert.equal(progressiveValidateExposed("2023-12-31 12:34:56.12345678"), "")
  })
  test("subseconds 9 digits valid (ns)", () => {
    assert.equal(progressiveValidateExposed("2023-12-31 12:34:56.123456789"), "")
  })

  // Invalid inputs that should produce a non-empty error key.
  test("letters in subseconds rejected", () => {
    assert.equal(progressiveValidateExposed("2023-12-31 12:34:56.abc"), "partials.input.InputTime.errors.invalid")
  })
  test("more than 9 subsecond digits rejected", () => {
    assert.equal(progressiveValidateExposed("2023-12-31 12:34:56.1234567890"), "partials.input.InputTime.errors.invalid")
  })
  test("month out of range with subseconds rejected", () => {
    assert.equal(progressiveValidateExposed("2023-13-31 12:34:56.123"), "partials.input.InputTime.errors.months")
  })
  test("day out of range with subseconds rejected", () => {
    assert.equal(progressiveValidateExposed("2023-12-32 12:34:56.123"), `partials.input.InputTime.errors.days {"maxDay":31}`)
  })
  test("hour out of range with subseconds rejected", () => {
    assert.equal(progressiveValidateExposed("2023-12-31 25:34:56.123"), "partials.input.InputTime.errors.hours")
  })
  test("minute out of range with subseconds rejected", () => {
    assert.equal(progressiveValidateExposed("2023-12-31 12:60:56.123"), "partials.input.InputTime.errors.minutes")
  })
  test("second out of range with subseconds rejected", () => {
    assert.equal(progressiveValidateExposed("2023-12-31 12:34:60.123"), "partials.input.InputTime.errors.seconds")
  })
})

describe("progressiveValidate (general invalid inputs)", () => {
  test("letters in year rejected", () => {
    // The first branch (YEAR_IN_PROGRESS_REGEX) tolerates partial input; "abc"
    // matches none of the well-formed shapes, falling through to the catch-all
    // "invalid" message.
    assert.equal(progressiveValidateExposed("abc"), "partials.input.InputTime.errors.invalid")
  })
  test("garbage tail after time rejected", () => {
    assert.equal(progressiveValidateExposed("2023-12-31 12:34:56xyz"), "partials.input.InputTime.errors.invalid")
  })
})

// Shorthand to build a struct with only the fields the test cares about.
// Mirrors getStructuredTime's shape so the resulting object can be passed
// directly to toCanonicalString and applyPrecision.
const struct = (s: Partial<{ y: string; m: string; d: string; h: string; min: string; s: string; sub: string }>) => ({
  y: "",
  m: "",
  d: "",
  h: "",
  min: "",
  s: "",
  sub: "",
  ...s,
})

describe("toCanonicalString", () => {
  // The forms in this suite mirror the backend's NewTime output (see
  // document/time.go); a mismatch here means the value InputTime emits
  // will be rejected by the backend's regex or Validate(precision).

  test("year precision emits just the year, padded to 4 digits", () => {
    assert.equal(toCanonicalString(struct({ y: "1995" }), "y"), "1995")
    assert.equal(toCanonicalString(struct({ y: "123" }), "y"), "0123")
    assert.equal(toCanonicalString(struct({ y: "0" }), "y"), "0000")
    assert.equal(toCanonicalString(struct({ y: "-1" }), "y"), "-0001")
    assert.equal(toCanonicalString(struct({ y: "12345" }), "y"), "12345")
    assert.equal(toCanonicalString(struct({ y: "-12345" }), "y"), "-12345")
  })

  test("missing year falls back to 0000", () => {
    assert.equal(toCanonicalString(struct({}), "y"), "0000")
    assert.equal(toCanonicalString(struct({ y: "" }), "y"), "0000")
  })

  test("coarse year precisions also emit just the year", () => {
    // toCanonicalString does NOT round - that is applyPrecision's job.
    // For coarse precisions the input struct.y is expected to already be
    // on the precision multiple. The output is still just the (padded)
    // year, with no month/day suffix.
    for (const p of ["G", "100M", "10M", "M", "100k", "10k", "k", "100y", "10y"] as const) {
      assert.equal(toCanonicalString(struct({ y: "1000000000" }), p), "1000000000")
      assert.equal(toCanonicalString(struct({ y: "0" }), p), "0000")
    }
  })

  test("month precision emits YYYY-MM-00 (backend day=0 marker)", () => {
    assert.equal(toCanonicalString(struct({ y: "2025", m: "03" }), "m"), "2025-03-00")
    // Bare single-digit month is left-padded.
    assert.equal(toCanonicalString(struct({ y: "2025", m: "3" }), "m"), "2025-03-00")
    // Missing month defaults to 01 (legal placeholder).
    assert.equal(toCanonicalString(struct({ y: "2025" }), "m"), "2025-01-00")
  })

  test("day precision emits YYYY-MM-DD", () => {
    assert.equal(toCanonicalString(struct({ y: "2025", m: "03", d: "15" }), "d"), "2025-03-15")
    assert.equal(toCanonicalString(struct({ y: "2025", m: "3", d: "5" }), "d"), "2025-03-05")
  })

  test("hour precision emits YYYY-MM-DD HH:00 (backend minute=0 marker)", () => {
    assert.equal(toCanonicalString(struct({ y: "2025", m: "03", d: "15", h: "10" }), "h"), "2025-03-15 10:00")
    assert.equal(toCanonicalString(struct({ y: "2025", m: "03", d: "15", h: "0" }), "h"), "2025-03-15 00:00")
    // Missing hour defaults to 00.
    assert.equal(toCanonicalString(struct({ y: "2025", m: "03", d: "15" }), "h"), "2025-03-15 00:00")
  })

  test("minute precision emits YYYY-MM-DD HH:MM", () => {
    assert.equal(toCanonicalString(struct({ y: "2025", m: "03", d: "15", h: "10", min: "30" }), "min"), "2025-03-15 10:30")
  })

  test("second precision emits YYYY-MM-DD HH:MM:SS", () => {
    assert.equal(toCanonicalString(struct({ y: "2025", m: "03", d: "15", h: "10", min: "30", s: "45" }), "s"), "2025-03-15 10:30:45")
  })

  test("ms precision pads subseconds to 3 digits", () => {
    assert.equal(toCanonicalString(struct({ y: "2025", m: "03", d: "15", h: "10", min: "30", s: "45", sub: "1" }), "ms"), "2025-03-15 10:30:45.100")
    assert.equal(toCanonicalString(struct({ y: "2025", m: "03", d: "15", h: "10", min: "30", s: "45", sub: "123" }), "ms"), "2025-03-15 10:30:45.123")
    // Over-length subseconds are truncated.
    assert.equal(toCanonicalString(struct({ y: "2025", m: "03", d: "15", h: "10", min: "30", s: "45", sub: "123456" }), "ms"), "2025-03-15 10:30:45.123")
  })

  test("us precision pads subseconds to 6 digits", () => {
    assert.equal(toCanonicalString(struct({ y: "2025", m: "03", d: "15", h: "10", min: "30", s: "45", sub: "123" }), "us"), "2025-03-15 10:30:45.123000")
    assert.equal(toCanonicalString(struct({ y: "2025", m: "03", d: "15", h: "10", min: "30", s: "45", sub: "123456" }), "us"), "2025-03-15 10:30:45.123456")
  })

  test("ns precision pads subseconds to 9 digits", () => {
    assert.equal(toCanonicalString(struct({ y: "2025", m: "03", d: "15", h: "10", min: "30", s: "45", sub: "123" }), "ns"), "2025-03-15 10:30:45.123000000")
    assert.equal(toCanonicalString(struct({ y: "2025", m: "03", d: "15", h: "10", min: "30", s: "45", sub: "123456789" }), "ns"), "2025-03-15 10:30:45.123456789")
  })

  test("negative year propagates into sub-year precisions", () => {
    assert.equal(toCanonicalString(struct({ y: "-44", m: "03", d: "15" }), "d"), "-0044-03-15")
    assert.equal(toCanonicalString(struct({ y: "-44", m: "03" }), "m"), "-0044-03-00")
  })
})

describe("applyPrecision", () => {
  // applyPrecision is what the precision-dropdown change handler runs:
  // it both rounds the year (for coarse precisions) and reformats the
  // value to the canonical shape. Sub-year cases delegate to
  // toCanonicalString and are exercised there; here we focus on the
  // rounding-and-padding paths that are unique to applyPrecision.

  test("coarse precisions round year down to multiple and pad", () => {
    assert.equal(applyPrecision(struct({ y: "1999" }), "k"), "1000")
    assert.equal(applyPrecision(struct({ y: "12345" }), "10y"), "12340")
    assert.equal(applyPrecision(struct({ y: "12345" }), "100y"), "12300")
    assert.equal(applyPrecision(struct({ y: "12345" }), "k"), "12000")
    assert.equal(applyPrecision(struct({ y: "1234567" }), "M"), "1000000")
    assert.equal(applyPrecision(struct({ y: "999" }), "k"), "0000")
  })

  test("year precision pads without rounding", () => {
    assert.equal(applyPrecision(struct({ y: "999" }), "y"), "0999")
    assert.equal(applyPrecision(struct({ y: "-1" }), "y"), "-0001")
    assert.equal(applyPrecision(struct({ y: "0" }), "y"), "0000")
  })

  test("negative years round toward -infinity (matches backend)", () => {
    // Math.floor for negative numbers floors toward -infinity, so -999/1000
    // floors to -1000, not 0. This matches the backend's expectation that
    // the rounded year is the floor on the precision lattice.
    assert.equal(applyPrecision(struct({ y: "-999" }), "k"), "-1000")
    assert.equal(applyPrecision(struct({ y: "-12345" }), "10y"), "-12350")
  })

  test("sub-year precisions delegate to canonical form", () => {
    // applyPrecision is the path the precision-dropdown change runs through.
    // For these precisions it should produce exactly the same string as
    // toCanonicalString does for the validator-on-blur path.
    const s = struct({ y: "2025", m: "03", d: "15", h: "10", min: "30", s: "45", sub: "123" })
    for (const p of ["m", "d", "h", "min", "s", "ms", "us", "ns"] as const) {
      assert.equal(applyPrecision(s, p), toCanonicalString(s, p))
    }
  })
})

describe("backend canonical round-trip", () => {
  // For each precision, the backend's NewTime canonical form is the
  // contract InputTime must preserve when loading and saving a claim.
  // getStructuredTime(canonical) followed by toCanonicalString(struct, p)
  // must yield the same string - otherwise a load+blur (with no user
  // edit) would silently rewrite the model to a value the backend
  // rejects.
  const cases: Array<[TimePrecision, string]> = [
    ["y", "1995"],
    ["y", "0001"],
    ["y", "-0044"],
    ["m", "2025-03-00"],
    ["d", "2025-03-15"],
    ["h", "2025-03-15 10:00"],
    ["min", "2025-03-15 10:30"],
    ["s", "2025-03-15 10:30:45"],
    ["ms", "2025-03-15 10:30:45.123"],
    ["us", "2025-03-15 10:30:45.123456"],
    ["ns", "2025-03-15 10:30:45.123456789"],
  ]

  for (const [precision, canonical] of cases) {
    test(`${precision}: ${canonical}`, () => {
      assert.equal(toCanonicalString(getStructuredTime(canonical), precision), canonical)
    })
  }
})
