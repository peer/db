import { Identifier } from "@tozd/identifier"
import { assert, describe, expect, test } from "vitest"

import { LIST, ORDER_IN_LIST } from "@/core"
import {
  AmountClaim,
  AmountIntervalClaim,
  ClaimTypes,
  D,
  getAllClaimsOfType,
  getAllClaimsOfTypeWithConfidence,
  getBestClaimOfType,
  getClaimsListsOfType,
  getClaimsOfType,
  getClaimsOfTypeWithConfidence,
  HasClaim,
  HighConfidence,
  HTMLClaim,
  IdentifierClaim,
  LinkClaim,
  LowConfidence,
  MediumConfidence,
  NoneClaim,
  ReferenceClaim,
  StringClaim,
  TimeClaim,
  TimeIntervalClaim,
  UnknownClaim,
} from "@/document"

test("CoreDocument GetID", () => {
  const base = ["testdoc"]
  const id = Identifier.new().toString()
  const doc = new D({ id, base })
  assert.equal(doc.GetID(), id)
})

test("CoreClaim GetConfidence", () => {
  const claim = new NoneClaim({
    id: Identifier.new().toString(),
    confidence: MediumConfidence,
    prop: { id: Identifier.new().toString() },
  })
  assert.equal(claim.GetConfidence(), MediumConfidence)
})

test("CoreClaim methods (Get, Remove, Size, AllClaims on meta)", () => {
  const prop = Identifier.new().toString()
  const otherProp = Identifier.new().toString()

  const claim = new NoneClaim({
    id: Identifier.new().toString(),
    confidence: HighConfidence,
    prop: { id: prop },
  })

  // Initially empty meta.
  assert.equal(claim.Size(), 0)
  assert.deepEqual(claim.AllClaims(), [])
  assert.deepEqual(claim.Get(prop), [])

  // Add two meta claims.
  const metaClaim1 = new StringClaim({
    id: Identifier.new().toString(),
    confidence: HighConfidence,
    prop: { id: prop },
    string: "meta1",
  })
  const metaClaim2 = new UnknownClaim({
    id: Identifier.new().toString(),
    confidence: HighConfidence,
    prop: { id: otherProp },
  })
  claim.Add(metaClaim1)
  claim.Add(metaClaim2)

  assert.equal(claim.Size(), 2)
  assert.equal(claim.AllClaims().length, 2)

  // Get by prop returns only matching.
  const got = claim.Get(prop)
  assert.equal(got.length, 1)
  assert.equal(got[0].GetID(), metaClaim1.GetID())

  // Remove by prop.
  const removed = claim.Remove(prop)
  assert.equal(removed.length, 1)
  assert.equal(claim.Size(), 1)

  // Remove non-existent prop returns empty.
  const removedNone = claim.Remove(Identifier.new().toString())
  assert.equal(removedNone.length, 0)
})

test("ClaimTypes with all claim types", () => {
  const prop = Identifier.new().toString()
  const ct = new ClaimTypes({})

  const claims = [
    new IdentifierClaim({ id: Identifier.new().toString(), confidence: HighConfidence, prop: { id: prop }, value: "Q42" }),
    new StringClaim({ id: Identifier.new().toString(), confidence: HighConfidence, prop: { id: prop }, string: "hello" }),
    new HTMLClaim({ id: Identifier.new().toString(), confidence: HighConfidence, prop: { id: prop }, html: "<b>bold</b>" }),
    new AmountClaim({ id: Identifier.new().toString(), confidence: HighConfidence, prop: { id: prop }, amount: "42", precision: 1 }),
    new AmountIntervalClaim({ id: Identifier.new().toString(), confidence: HighConfidence, prop: { id: prop } }),
    new TimeClaim({ id: Identifier.new().toString(), confidence: HighConfidence, prop: { id: prop }, timestamp: "2025", precision: "y" }),
    new TimeIntervalClaim({ id: Identifier.new().toString(), confidence: HighConfidence, prop: { id: prop } }),
    new LinkClaim({ id: Identifier.new().toString(), confidence: HighConfidence, prop: { id: prop }, iri: "https://example.com" }),
    new ReferenceClaim({ id: Identifier.new().toString(), confidence: HighConfidence, prop: { id: prop }, to: { id: Identifier.new().toString() } }),
    new HasClaim({ id: Identifier.new().toString(), confidence: HighConfidence, prop: { id: prop } }),
    new NoneClaim({ id: Identifier.new().toString(), confidence: HighConfidence, prop: { id: prop } }),
    new UnknownClaim({ id: Identifier.new().toString(), confidence: HighConfidence, prop: { id: prop } }),
  ]

  for (const claim of claims) {
    ct.Add(claim)
  }

  assert.equal(ct.Size(), 12)
  assert.equal(ct.AllClaims().length, 12)

  // Get by prop returns all 12.
  assert.equal(ct.Get(prop).length, 12)

  // Get by random prop returns empty.
  assert.equal(ct.Get(Identifier.new().toString()).length, 0)

  // GetByID finds each claim.
  for (const claim of claims) {
    assert.equal(ct.GetByID(claim.GetID())?.GetID(), claim.GetID())
  }

  // GetByID returns undefined for non-existent ID.
  assert.equal(ct.GetByID(Identifier.new().toString()), undefined)

  // Remove by prop removes all 12.
  const removed = ct.Remove(prop)
  assert.equal(removed.length, 12)
  assert.equal(ct.Size(), 0)
})

test("ClaimTypes GetByID", () => {
  const prop = Identifier.new().toString()
  const id1 = Identifier.new().toString()
  const id2 = Identifier.new().toString()

  const ct = new ClaimTypes({
    string: [{ id: id1, confidence: 1.0, prop: { id: prop }, string: "s1" }],
    none: [{ id: id2, confidence: 1.0, prop: { id: prop } }],
  })

  assert.equal(ct.GetByID(id1)?.GetID(), id1)
  assert.equal(ct.GetByID(id2)?.GetID(), id2)
  assert.equal(ct.GetByID(Identifier.new().toString()), undefined)
})

test("ClaimTypes RemoveByID", () => {
  const prop = Identifier.new().toString()
  const id1 = Identifier.new().toString()
  const id2 = Identifier.new().toString()

  const ct = new ClaimTypes({
    string: [
      { id: id1, confidence: 1.0, prop: { id: prop }, string: "s1" },
      { id: id2, confidence: 1.0, prop: { id: prop }, string: "s2" },
    ],
  })

  assert.equal(ct.Size(), 2)
  const removed = ct.RemoveByID(id1)
  assert.equal(removed?.GetID(), id1)
  assert.equal(ct.Size(), 1)

  // Remove non-existent.
  assert.equal(ct.RemoveByID(Identifier.new().toString()), undefined)
})

test("ClaimTypes RemoveByID in meta", () => {
  const prop = Identifier.new().toString()
  const outerID = Identifier.new().toString()
  const innerID = Identifier.new().toString()

  const ct = new ClaimTypes({
    none: [
      {
        id: outerID,
        confidence: 1.0,
        prop: { id: prop },
        meta: {
          string: [{ id: innerID, confidence: 1.0, prop: { id: prop }, string: "inner" }],
        },
      },
    ],
  })

  // Find inner via GetByID.
  assert.equal(ct.GetByID(innerID)?.GetID(), innerID)

  // Remove inner.
  const removed = ct.RemoveByID(innerID)
  assert.equal(removed?.GetID(), innerID)
  assert.equal(ct.GetByID(innerID), undefined)
})

describe("GetAllClaimsOfType", () => {
  test("returns claims sorted by decreasing confidence", () => {
    const prop = Identifier.new().toString()
    const ct = new ClaimTypes({
      string: [
        { id: Identifier.new().toString(), confidence: 0.5, prop: { id: prop }, string: "low" },
        { id: Identifier.new().toString(), confidence: 1.0, prop: { id: prop }, string: "high" },
        { id: Identifier.new().toString(), confidence: 0.75, prop: { id: prop }, string: "medium" },
      ],
      html: [{ id: Identifier.new().toString(), confidence: 1.0, prop: { id: prop }, html: "<b>bold</b>" }],
    })

    const strings = getAllClaimsOfType(ct, "string")
    assert.equal(strings.length, 3)
    assert.equal(strings[0].string, "high")
    assert.equal(strings[1].string, "medium")
    assert.equal(strings[2].string, "low")

    const htmls = getAllClaimsOfType(ct, "html")
    assert.equal(htmls.length, 1)

    const refs = getAllClaimsOfType(ct, "ref")
    assert.equal(refs.length, 0)
  })

  test("returns empty for null/undefined", () => {
    assert.equal(getAllClaimsOfType(null, "string").length, 0)
    assert.equal(getAllClaimsOfType(undefined, "string").length, 0)
  })
})

describe("GetAllClaimsOfTypeWithConfidence", () => {
  test("filters by minimum confidence", () => {
    const prop = Identifier.new().toString()
    const ct = new ClaimTypes({
      string: [
        { id: Identifier.new().toString(), confidence: 0.3, prop: { id: prop }, string: "verylow" },
        { id: Identifier.new().toString(), confidence: 0.5, prop: { id: prop }, string: "low" },
        { id: Identifier.new().toString(), confidence: 0.75, prop: { id: prop }, string: "medium" },
        { id: Identifier.new().toString(), confidence: 1.0, prop: { id: prop }, string: "high" },
      ],
    })

    // LowConfidence (0.5) filters out 0.3.
    const low = getAllClaimsOfTypeWithConfidence(ct, "string", LowConfidence)
    assert.equal(low.length, 3)

    // MediumConfidence (0.75) keeps 2.
    const med = getAllClaimsOfTypeWithConfidence(ct, "string", MediumConfidence)
    assert.equal(med.length, 2)

    // HighConfidence (1.0) keeps 1.
    const high = getAllClaimsOfTypeWithConfidence(ct, "string", HighConfidence)
    assert.equal(high.length, 1)
  })

  test("returns empty for null/undefined", () => {
    assert.equal(getAllClaimsOfTypeWithConfidence(null, "string").length, 0)
  })
})

describe("GetClaimsOfType", () => {
  test("returns claims for property sorted by confidence", () => {
    const prop = Identifier.new().toString()
    const otherProp = Identifier.new().toString()

    const ct = new ClaimTypes({
      string: [
        { id: Identifier.new().toString(), confidence: 0.5, prop: { id: prop }, string: "s1" },
        { id: Identifier.new().toString(), confidence: 1.0, prop: { id: prop }, string: "s2" },
        { id: Identifier.new().toString(), confidence: 0.75, prop: { id: otherProp }, string: "other" },
      ],
    })

    const strings = getClaimsOfType(ct, "string", prop)
    assert.equal(strings.length, 2)
    assert.equal(strings[0].string, "s2") // Higher confidence first.
    assert.equal(strings[1].string, "s1")

    // No AmountClaims for prop.
    assert.equal(getClaimsOfType(ct, "amount", prop).length, 0)
  })

  test("accepts array of property IDs", () => {
    const prop1 = Identifier.new().toString()
    const prop2 = Identifier.new().toString()

    const ct = new ClaimTypes({
      string: [
        { id: Identifier.new().toString(), confidence: 1.0, prop: { id: prop1 }, string: "s1" },
        { id: Identifier.new().toString(), confidence: 1.0, prop: { id: prop2 }, string: "s2" },
      ],
    })

    const strings = getClaimsOfType(ct, "string", [prop1, prop2])
    assert.equal(strings.length, 2)
  })
})

describe("GetBestClaimOfType", () => {
  test("returns highest confidence claim", () => {
    const prop = Identifier.new().toString()
    const ct = new ClaimTypes({
      string: [
        { id: Identifier.new().toString(), confidence: 0.5, prop: { id: prop }, string: "low" },
        { id: Identifier.new().toString(), confidence: 1.0, prop: { id: prop }, string: "high" },
      ],
    })

    const best = getBestClaimOfType(ct, "string", prop)
    assert.equal(best?.string, "high")
  })

  test("returns null when no match", () => {
    const ct = new ClaimTypes({})
    assert.equal(getBestClaimOfType(ct, "string", Identifier.new().toString()), null)
    assert.equal(getBestClaimOfType(null, "string", Identifier.new().toString()), null)
  })
})

describe("GetClaimsOfTypeWithConfidence", () => {
  test("filters by confidence", () => {
    const prop = Identifier.new().toString()
    const ct = new ClaimTypes({
      string: [
        { id: Identifier.new().toString(), confidence: 0.3, prop: { id: prop }, string: "verylow" },
        { id: Identifier.new().toString(), confidence: 0.5, prop: { id: prop }, string: "low" },
        { id: Identifier.new().toString(), confidence: 1.0, prop: { id: prop }, string: "high" },
      ],
    })

    const claims = getClaimsOfTypeWithConfidence(ct, "string", prop, LowConfidence)
    assert.equal(claims.length, 2) // 0.5 and 1.0.
  })
})

describe("GetClaimsListsOfType", () => {
  test("groups by LIST and sorts by ORDER_IN_LIST", () => {
    const prop = Identifier.new().toString()
    const listA = Identifier.new().toString()
    const listB = Identifier.new().toString()

    const ct = new ClaimTypes({
      string: [
        {
          id: Identifier.new().toString(),
          confidence: 1.0,
          prop: { id: prop },
          string: "a2",
          meta: {
            id: [{ id: Identifier.new().toString(), confidence: 1.0, prop: { id: LIST }, value: listA }],
            amount: [{ id: Identifier.new().toString(), confidence: 1.0, prop: { id: ORDER_IN_LIST }, amount: "2", precision: 1 }],
          },
        },
        {
          id: Identifier.new().toString(),
          confidence: 1.0,
          prop: { id: prop },
          string: "a1",
          meta: {
            id: [{ id: Identifier.new().toString(), confidence: 1.0, prop: { id: LIST }, value: listA }],
            amount: [{ id: Identifier.new().toString(), confidence: 1.0, prop: { id: ORDER_IN_LIST }, amount: "1", precision: 1 }],
          },
        },
        {
          id: Identifier.new().toString(),
          confidence: 1.0,
          prop: { id: prop },
          string: "b1",
          meta: {
            id: [{ id: Identifier.new().toString(), confidence: 1.0, prop: { id: LIST }, value: listB }],
            amount: [{ id: Identifier.new().toString(), confidence: 1.0, prop: { id: ORDER_IN_LIST }, amount: "1", precision: 1 }],
          },
        },
      ],
    })

    const lists = getClaimsListsOfType(ct, "string", prop)
    assert.equal(lists.length, 2)

    // Find list A (2 items) and list B (1 item).
    const listAClaims = lists.find((l) => l.length === 2)!
    const listBClaims = lists.find((l) => l.length === 1)!

    assert.equal(listAClaims[0].string, "a1") // Order 1.
    assert.equal(listAClaims[1].string, "a2") // Order 2.
    assert.equal(listBClaims[0].string, "b1")
  })

  test("claims without LIST go into one group", () => {
    const prop = Identifier.new().toString()
    const ct = new ClaimTypes({
      string: [
        { id: Identifier.new().toString(), confidence: 1.0, prop: { id: prop }, string: "no-list-1" },
        { id: Identifier.new().toString(), confidence: 0.8, prop: { id: prop }, string: "no-list-2" },
      ],
    })

    const lists = getClaimsListsOfType(ct, "string", prop)
    assert.equal(lists.length, 1)
    assert.equal(lists[0].length, 2)
  })

  test("returns empty for no matching claims", () => {
    const ct = new ClaimTypes({})
    const lists = getClaimsListsOfType(ct, "string", Identifier.new().toString())
    assert.equal(lists.length, 0)
  })
})

test("CoreClaim Validate invalid confidence", async () => {
  const claim = new NoneClaim({
    id: Identifier.new().toString(),
    confidence: 2.0,
    prop: { id: Identifier.new().toString() },
  })
  await expect(claim.Validate()).rejects.toThrow("confidence out of range")
})

test("CoreClaim Validate with invalid meta", async () => {
  const outerClaim = new NoneClaim({
    id: Identifier.new().toString(),
    confidence: 1.0,
    prop: { id: Identifier.new().toString() },
  })
  const innerClaim = new StringClaim({
    id: Identifier.new().toString(),
    confidence: 5.0,
    prop: { id: Identifier.new().toString() },
    string: "bad",
  })
  outerClaim.Add(innerClaim)
  await expect(outerClaim.Validate()).rejects.toThrow("confidence out of range")
})

test("ClaimTypes Validate duplicate ID", async () => {
  const sharedID = Identifier.new().toString()
  const prop = Identifier.new().toString()
  const ct = new ClaimTypes({
    string: [
      { id: sharedID, confidence: 1.0, prop: { id: prop }, string: "first" },
      { id: sharedID, confidence: 0.5, prop: { id: prop }, string: "second" },
    ],
  })
  await expect(ct.Validate()).rejects.toThrow("duplicate claim ID")
})

test("ClaimTypes Get", () => {
  const prop = Identifier.new().toString()
  const otherProp = Identifier.new().toString()
  const ct = new ClaimTypes({
    string: [
      { id: Identifier.new().toString(), confidence: 0.5, prop: { id: prop }, string: "s1" },
      { id: Identifier.new().toString(), confidence: 1.0, prop: { id: prop }, string: "s2" },
    ],
    html: [{ id: Identifier.new().toString(), confidence: 0.75, prop: { id: prop }, html: "<b>h</b>" }],
  })

  // Get returns all 3 claims matching prop, sorted by confidence.
  const got = ct.Get(prop)
  assert.equal(got.length, 3)
  assert.equal(got[0].confidence, 1.0)
  assert.equal(got[1].confidence, 0.75)
  assert.equal(got[2].confidence, 0.5)

  // Get with a different prop returns empty.
  assert.deepEqual(ct.Get(otherProp), [])
})

test("ClaimTypes Remove", () => {
  const prop = Identifier.new().toString()
  const ct = new ClaimTypes({
    string: [
      { id: Identifier.new().toString(), confidence: 1.0, prop: { id: prop }, string: "s1" },
      { id: Identifier.new().toString(), confidence: 0.5, prop: { id: prop }, string: "s2" },
    ],
  })

  const removed = ct.Remove(prop)
  assert.equal(removed.length, 2)
  assert.equal(ct.Size(), 0)
})

test("CoreClaim Get and Remove with no meta", () => {
  const prop = Identifier.new().toString()
  const claim = new NoneClaim({
    id: Identifier.new().toString(),
    confidence: HighConfidence,
    prop: { id: prop },
  })

  // Get on a claim with no meta returns empty.
  assert.deepEqual(claim.Get(prop), [])

  // Remove on a claim with no meta returns empty.
  assert.deepEqual(claim.Remove(prop), [])
})

test("ClaimTypes Add non-matching object does nothing", () => {
  const ct = new ClaimTypes({})

  // Adding a plain object that is not an instanceof any claim type.
  ct.Add({ id: Identifier.new().toString(), confidence: 1.0, prop: { id: Identifier.new().toString() } } as never)
  assert.equal(ct.Size(), 0)
})
