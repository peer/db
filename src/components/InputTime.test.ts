import { assert, describe, test } from "vitest"

import { normalizeForParsing } from "./InputTime.vue"

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
