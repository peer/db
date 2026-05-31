import { Identifier } from "@tozd/identifier"
import { assert, describe, expect, test } from "vitest"

import { parseShortcut, resolveShortcutID, shortcutToFilters, shortcutToQuery } from "@/shortcut"

describe("parseShortcut", () => {
  test("parses a single key/value pair", () => {
    assert.deepEqual(parseShortcut("ns.example.com,KIND=ns.example.com,A"), [{ key: "ns.example.com,KIND", value: "ns.example.com,A" }])
  })

  test("parses multiple parts separated by &", () => {
    assert.deepEqual(parseShortcut("a=1,2&b:c=3,4"), [
      { key: "a", value: "1,2" },
      { key: "b:c", value: "3,4" },
    ])
  })

  test("preserves only the first '=' as separator", () => {
    assert.deepEqual(parseShortcut("key=value=with=equals"), [{ key: "key", value: "value=with=equals" }])
  })

  test("preserves 'self' value literally", () => {
    assert.deepEqual(parseShortcut("ns.example.com,KIND=self"), [{ key: "ns.example.com,KIND", value: "self" }])
  })

  test("preserves 'reverse' key literally", () => {
    assert.deepEqual(parseShortcut("reverse=ns.example.com,DOC"), [{ key: "reverse", value: "ns.example.com,DOC" }])
  })

  test("throws on empty input", () => {
    expect(() => parseShortcut("")).toThrowError("search shortcut must not be empty")
  })

  test("throws when '=' is missing", () => {
    expect(() => parseShortcut("ns.example.com,KIND")).toThrowError(/non-empty key and value/)
  })

  test("throws on empty value", () => {
    expect(() => parseShortcut("ns.example.com,KIND=")).toThrowError(/non-empty key and value/)
  })

  test("throws when key has more than one ':'", () => {
    expect(() => parseShortcut("a:b:c=ns.example.com,D")).toThrowError(/at most one ':'/)
  })

  test("throws when 'reverse' is the parent of a nested key", () => {
    expect(() => parseShortcut("reverse:ns.example.com,X=ns.example.com,Y")).toThrowError(/"reverse" is not allowed/)
  })

  test("throws when 'reverse' is the nested side of a key", () => {
    expect(() => parseShortcut("ns.example.com,X:reverse=ns.example.com,Y")).toThrowError(/"reverse" is not allowed/)
  })
})

describe("resolveShortcutID", () => {
  test("hashes comma-separated parts via Identifier.from", async () => {
    const want = (await Identifier.from("ns.example.com", "KIND")).toString()
    assert.equal(await resolveShortcutID("ns.example.com,KIND"), want)
  })

  test("returns single tokens unchanged", async () => {
    const id = Identifier.new().toString()
    assert.equal(await resolveShortcutID(id), id)
  })
})

describe("shortcutToFilters", () => {
  test("builds a ref filter from a multi-part key and value", async () => {
    const payload = await shortcutToFilters("ns.example.com,KIND=ns.example.com,A")
    const prop = (await Identifier.from("ns.example.com", "KIND")).toString()
    const value = (await Identifier.from("ns.example.com", "A")).toString()
    assert.deepEqual(payload, {
      filters: [{ prop: [prop], ref: { to: [{ id: value }] } }],
    })
  })

  test("builds a nested ref filter for 'parent:prop' keys", async () => {
    const payload = await shortcutToFilters("ns.example.com,P:ns.example.com,Q=ns.example.com,V")
    const parent = (await Identifier.from("ns.example.com", "P")).toString()
    const nested = (await Identifier.from("ns.example.com", "Q")).toString()
    const value = (await Identifier.from("ns.example.com", "V")).toString()
    assert.deepEqual(payload.filters, [{ prop: [parent, nested], ref: { to: [{ id: value }] } }])
  })

  test("substitutes 'self' with the supplied self ID", async () => {
    const self = Identifier.new().toString()
    const payload = await shortcutToFilters("ns.example.com,KIND=self", self)
    const prop = (await Identifier.from("ns.example.com", "KIND")).toString()
    assert.deepEqual(payload.filters, [{ prop: [prop], ref: { to: [{ id: self }] } }])
  })

  test("throws when 'self' is referenced without a self prop", async () => {
    await expect(shortcutToFilters("ns.example.com,KIND=self")).rejects.toThrowError(/no self ID was provided/)
  })

  test("sets reverse at the top level", async () => {
    const payload = await shortcutToFilters("reverse=ns.example.com,DOC")
    const value = (await Identifier.from("ns.example.com", "DOC")).toString()
    assert.deepEqual(payload, { reverse: value })
  })

  test("supports reverse with self", async () => {
    const self = Identifier.new().toString()
    const payload = await shortcutToFilters("reverse=self", self)
    assert.deepEqual(payload, { reverse: self })
  })

  test("combines reverse and filters in one payload", async () => {
    const payload = await shortcutToFilters("ns.example.com,KIND=ns.example.com,A&reverse=ns.example.com,DOC")
    const prop = (await Identifier.from("ns.example.com", "KIND")).toString()
    const value = (await Identifier.from("ns.example.com", "A")).toString()
    const rev = (await Identifier.from("ns.example.com", "DOC")).toString()
    assert.deepEqual(payload, {
      reverse: rev,
      filters: [{ prop: [prop], ref: { to: [{ id: value }] } }],
    })
  })

  test("omits filters when only reverse is present", async () => {
    const payload = await shortcutToFilters("reverse=ns.example.com,DOC")
    assert.notProperty(payload, "filters")
  })
})

describe("shortcutToQuery", () => {
  test("emits a single key with a one-element list of resolved identifiers", async () => {
    const query = await shortcutToQuery("ns.example.com,KIND=ns.example.com,A")
    const prop = (await Identifier.from("ns.example.com", "KIND")).toString()
    const value = (await Identifier.from("ns.example.com", "A")).toString()
    assert.deepEqual(query, { [prop]: [value] })
  })

  test("joins nested keys with ':'", async () => {
    const query = await shortcutToQuery("ns.example.com,P:ns.example.com,Q=ns.example.com,V")
    const parent = (await Identifier.from("ns.example.com", "P")).toString()
    const nested = (await Identifier.from("ns.example.com", "Q")).toString()
    const value = (await Identifier.from("ns.example.com", "V")).toString()
    assert.deepEqual(query, { [`${parent}:${nested}`]: [value] })
  })

  test("groups repeated keys into a list of values", async () => {
    const query = await shortcutToQuery("ns.example.com,KIND=ns.example.com,A&ns.example.com,KIND=ns.example.com,B")
    const prop = (await Identifier.from("ns.example.com", "KIND")).toString()
    const a = (await Identifier.from("ns.example.com", "A")).toString()
    const b = (await Identifier.from("ns.example.com", "B")).toString()
    assert.deepEqual(query, { [prop]: [a, b] })
  })

  test("preserves 'reverse' as the literal key", async () => {
    const query = await shortcutToQuery("reverse=ns.example.com,DOC")
    const value = (await Identifier.from("ns.example.com", "DOC")).toString()
    assert.deepEqual(query, { reverse: [value] })
  })

  test("substitutes 'self' with the supplied self ID", async () => {
    const self = Identifier.new().toString()
    const query = await shortcutToQuery("ns.example.com,KIND=self", self)
    const prop = (await Identifier.from("ns.example.com", "KIND")).toString()
    assert.deepEqual(query, { [prop]: [self] })
  })

  test("throws when 'self' is referenced without a self prop", async () => {
    await expect(shortcutToQuery("ns.example.com,KIND=self")).rejects.toThrowError(/no self ID was provided/)
  })
})
