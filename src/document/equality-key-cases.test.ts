// The shared equality-key corpus, run against the frontend claim types. The same file is run by the
// Go backend (document/equality_key_cases_test.go), so this asserts that the frontend and the
// backend agree on what a claim says: one claim of every claim type, with the key it produces asked
// with and without its id and its sub-claims. Two sides disagreeing there would count repeats of a
// claim differently, and the keys are regenerated from the backend (see the Go test).

import { assert, describe, test } from "vitest"

// The corpus lives next to the Go backend test that also runs it; Vite resolves the JSON import
// from the project root.
import corpus from "@/../document/testdata/equality-key-cases.json"

import { ClaimTypes } from "@/document/claims"

interface EqualityKeyCase {
  name: string
  claims: object
  key: string
  keyWithID: string
  keyWithSub: string
  keyWithIDAndSub: string
}

const cases: EqualityKeyCase[] = corpus.cases

describe("equality key corpus", () => {
  for (const c of cases) {
    test(c.name, () => {
      const claims = new ClaimTypes(c.claims as Record<string, object[]>)
      const all = claims.AllClaims()
      assert.equal(all.length, 1)
      const claim = all[0]

      assert.equal(claim.EqualityKey(false, false), c.key)
      assert.equal(claim.EqualityKey(true, false), c.keyWithID)
      assert.equal(claim.EqualityKey(false, true), c.keyWithSub)
      assert.equal(claim.EqualityKey(true, true), c.keyWithIDAndSub)
    })
  }

  test("presence-only claims are told apart by their type alone", () => {
    const key = (name: string) => cases.find((c) => c.name === name)!.key
    assert.notEqual(key("has"), key("none"))
    assert.notEqual(key("has"), key("unknown"))
    assert.notEqual(key("none"), key("unknown"))
  })

  test("sub-claim order does not decide the key, the identities do", () => {
    const first = cases.find((c) => c.name === "claim with sub-claims")!
    const second = cases.find((c) => c.name === "claim with sub-claims in another order")!
    assert.equal(first.keyWithSub, second.keyWithSub)
    assert.notEqual(first.keyWithIDAndSub, second.keyWithIDAndSub)
  })
})
