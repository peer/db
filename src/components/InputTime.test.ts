import { assert, describe, test } from "vitest"

import { normalizeForParsing, progressiveValidate } from "./InputTime.vue"

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
const progressiveValidateExposed = (value: string) => progressiveValidate(value)

describe("progressiveValidate", () => {
  test("returns empty string for empty input", () => {
    assert.equal(progressiveValidateExposed(""), "")

    assert.notEqual(progressiveValidateExposed(""), "Invalid timestamp structure.")
  })

  test("allows year in progress", () => {
    assert.equal(progressiveValidateExposed("2"), "")
    assert.equal(progressiveValidateExposed("20"), "")
    assert.equal(progressiveValidateExposed("202"), "")
    assert.equal(progressiveValidateExposed("2023"), "")

    assert.notEqual(progressiveValidateExposed("2023"), "Months need to be between 0-12.")
  })

  test("validates month in progress", () => {
    assert.equal(progressiveValidateExposed("2023-"), "")
    assert.equal(progressiveValidateExposed("2023-0"), "")
    assert.equal(progressiveValidateExposed("2023-1"), "")
    assert.equal(progressiveValidateExposed("2023-12"), "")
    assert.equal(progressiveValidateExposed("2023-13"), "Months need to be between 0-12.")

    assert.notEqual(progressiveValidateExposed("2023-12"), "Invalid timestamp structure.")
    assert.notEqual(progressiveValidateExposed("2023-13"), "")
  })

  test("validates day in progress", () => {
    assert.equal(progressiveValidateExposed("2023-1-"), "")
    assert.equal(progressiveValidateExposed("2023-1-0"), "")
    assert.equal(progressiveValidateExposed("2023-1-1"), "")
    assert.equal(progressiveValidateExposed("2023-1-1 1"), "")
    assert.equal(progressiveValidateExposed("2023-0-1"), "Months cannot be 0 when days are not 0.")
    assert.equal(progressiveValidateExposed("2023-00-01"), "Months cannot be 0 when days are not 0.")
    assert.equal(progressiveValidateExposed("2023-13-1"), "Months need to be between 0-12.")
    assert.equal(progressiveValidateExposed("2023-2-30"), "Day must be between 0-28.")
    assert.equal(progressiveValidateExposed("2015-2-30"), "Day must be between 0-28.")

    assert.notEqual(progressiveValidateExposed("2023-1-1"), "Invalid timestamp structure.")
    assert.notEqual(progressiveValidateExposed("2023-0-1"), "")
    assert.notEqual(progressiveValidateExposed("2023-13-1"), "")
    assert.notEqual(progressiveValidateExposed("2023-2-30"), "")
  })

  test("validates hours", () => {
    assert.equal(progressiveValidateExposed("2023-12-31 0"), "")
    assert.equal(progressiveValidateExposed("2023-12-31 23"), "")
    assert.equal(progressiveValidateExposed("2023-12-31 24"), "Hours needs to be between 0-23.")
    assert.equal(progressiveValidateExposed("2023-13-31 12"), "Months need to be between 1-12.")
    assert.equal(progressiveValidateExposed("2023-2-30 12"), "Day must be between 1-28.")

    assert.notEqual(progressiveValidateExposed("2023-12-31 23"), "Invalid timestamp structure.")
    assert.notEqual(progressiveValidateExposed("2023-12-31 24"), "")
    assert.notEqual(progressiveValidateExposed("2023-13-31 12"), "")
    assert.notEqual(progressiveValidateExposed("2023-2-30 12"), "")
  })

  test("do not allows minutes in progress", () => {
    assert.equal(progressiveValidateExposed("2023-12-31 12:"), "Invalid timestamp structure.")
  })

  test("validates minutes", () => {
    assert.equal(progressiveValidateExposed("2023-12-31 12:0"), "")
    assert.equal(progressiveValidateExposed("2023-12-31 12:59"), "")
    assert.equal(progressiveValidateExposed("2023-12-31 12:60"), "Minutes need to be between 0-59.")
    assert.equal(progressiveValidateExposed("2023-12-31 24:00"), "Hours needs to be between 0-23.")

    assert.notEqual(progressiveValidateExposed("2023-12-31 12:59"), "Invalid timestamp structure.")
    assert.notEqual(progressiveValidateExposed("2023-12-31 12:60"), "")
    assert.notEqual(progressiveValidateExposed("2023-12-31 24:00"), "")
  })

  test("do not allows seconds in progress", () => {
    assert.equal(progressiveValidateExposed("2023-12-31 12:34:"), "Invalid timestamp structure.")
  })

  test("validates seconds", () => {
    assert.equal(progressiveValidateExposed("2023-12-31 12:34:0"), "")
    assert.equal(progressiveValidateExposed("2023-12-31 12:34:59"), "")
    assert.equal(progressiveValidateExposed("2023-12-31 12:34:60"), "Seconds need to be between 0-59.")
    assert.equal(progressiveValidateExposed("2023-12-31 12:60:00"), "Minutes need to be between 0-59.")

    assert.notEqual(progressiveValidateExposed("2023-12-31 12:34:59"), "Invalid timestamp structure.")
    assert.notEqual(progressiveValidateExposed("2023-12-31 12:34:60"), "")
    assert.notEqual(progressiveValidateExposed("2023-12-31 12:60:00"), "")
  })

  test("rejects invalid timestamp structure", () => {
    assert.equal(progressiveValidateExposed("foo"), "Invalid timestamp structure.")
    assert.equal(progressiveValidateExposed("2023--12"), "Invalid timestamp structure.")

    assert.notEqual(progressiveValidateExposed("foo"), "")
    assert.notEqual(progressiveValidateExposed("2023--12"), "")
  })
})
