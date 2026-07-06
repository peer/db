import type { Filter } from "@/types"

import { assert, describe, test } from "vitest"

import { prefiltersMatch, queryToPrefilterPayloads } from "@/search"

describe("queryToPrefilterPayloads", () => {
  test("parses to, direct, and missing values for one property", () => {
    const payloads = queryToPrefilterPayloads({ prop: ["a", "direct:b", "missing", "c"] })
    assert.deepEqual(payloads, [{ prop: ["prop"], to: [{ id: "a" }, { id: "c" }], direct: [{ id: "b" }], missing: true }])
  })

  test("splits nested keys on ':' and skips reverse/id/language", () => {
    const payloads = queryToPrefilterPayloads({ "parent:prop": ["x"], reverse: ["doc"], id: ["doc1", "doc2"], language: ["sl"] })
    assert.deepEqual(payloads, [{ prop: ["parent", "prop"], to: [{ id: "x" }], direct: [], missing: false }])
  })

  test("treats a bare string value as a single target", () => {
    const payloads = queryToPrefilterPayloads({ prop: "a" })
    assert.deepEqual(payloads, [{ prop: ["prop"], to: [{ id: "a" }], direct: [], missing: false }])
  })
})

describe("prefiltersMatch", () => {
  test("matches a to/direct/missing prefilter against its payload", () => {
    const payloads = queryToPrefilterPayloads({ prop: ["a", "direct:b", "missing"] })
    const prefilters: Filter[] = [{ id: "f", base: [], prop: ["prop"], ref: { to: [{ id: "a" }], direct: [{ id: "b" }], missing: true } }]
    assert.isTrue(prefiltersMatch(prefilters, payloads))
  })

  test("ignores value order within a selection", () => {
    const payloads = queryToPrefilterPayloads({ prop: ["a", "b"] })
    const prefilters: Filter[] = [{ id: "f", base: [], prop: ["prop"], ref: { to: [{ id: "b" }, { id: "a" }] } }]
    assert.isTrue(prefiltersMatch(prefilters, payloads))
  })

  test("does not match when missing differs", () => {
    const payloads = queryToPrefilterPayloads({ prop: ["a"] })
    const prefilters: Filter[] = [{ id: "f", base: [], prop: ["prop"], ref: { to: [{ id: "a" }], missing: true } }]
    assert.isFalse(prefiltersMatch(prefilters, payloads))
  })

  test("does not match when a value is direct on one side only", () => {
    const payloads = queryToPrefilterPayloads({ prop: ["direct:b"] })
    const prefilters: Filter[] = [{ id: "f", base: [], prop: ["prop"], ref: { to: [{ id: "b" }] } }]
    assert.isFalse(prefiltersMatch(prefilters, payloads))
  })
})
