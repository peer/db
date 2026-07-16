import { Identifier } from "@tozd/identifier"
import { assert, describe, expect, test } from "vitest"

import { parseScopes, Scope, SCOPE_ALL, SCOPE_DOCUMENTS, SCOPE_FILES, SCOPE_SELF } from "@/auth"

const prop = (await Identifier.from("ns.example.com", "PROP")).toString()
const value = (await Identifier.from("ns.example.com", "VALUE")).toString()

describe("parseScopes", () => {
  test("parses a literal scope", () => {
    assert.deepEqual(parseScopes(SCOPE_ALL), [new Scope(SCOPE_ALL, "", "")])
    assert.deepEqual(parseScopes(SCOPE_SELF), [new Scope(SCOPE_SELF, "", "")])
  })

  test("parses a list of literal scopes", () => {
    assert.deepEqual(parseScopes(`${SCOPE_DOCUMENTS}&${SCOPE_FILES}`), [new Scope(SCOPE_DOCUMENTS, "", ""), new Scope(SCOPE_FILES, "", "")])
  })

  test("parses a claim scope", () => {
    assert.deepEqual(parseScopes(`${prop}=${value}`), [new Scope("", prop, value)])
  })

  test("parses a mixed expression", () => {
    assert.deepEqual(parseScopes(`${SCOPE_ALL}&${prop}=${value}`), [new Scope(SCOPE_ALL, "", ""), new Scope("", prop, value)])
  })

  test("throws on an entry without a value", () => {
    expect(() => parseScopes("garbage")).toThrowError(/non-empty key and value/)
  })

  test("throws on an empty expression", () => {
    expect(() => parseScopes("")).toThrowError(/non-empty key and value/)
  })

  test("throws on a non-identifier side", () => {
    expect(() => parseScopes(`self=${value}`)).toThrowError(/each a single identifier/)
    expect(() => parseScopes(`${prop}=missing`)).toThrowError(/each a single identifier/)
  })

  test("throws on an unresolved identifier token", () => {
    expect(() => parseScopes(`ns.example.com,PROP=${value}`)).toThrowError(/each a single identifier/)
  })

  test("throws on a value containing a further separator", () => {
    expect(() => parseScopes(`${prop}=${value}=${value}`)).toThrowError(/each a single identifier/)
  })

  test("throws on any invalid entry in the expression", () => {
    // A valid entry does not survive an invalid one: the whole expression contributes nothing, like on
    // the backend.
    expect(() => parseScopes(`${SCOPE_SELF}&garbage`)).toThrowError(/non-empty key and value/)
  })
})
