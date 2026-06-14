import { Identifier } from "@tozd/identifier"
import { assert, describe, expect, test } from "vitest"

import { LIST, ORDER_IN_LIST } from "@/core"
import {
  AmountClaim,
  AmountIntervalClaim,
  claimTypeName,
  ClaimTypes,
  D,
  getAllClaimsOfTypeWithConfidence,
  getBestClaimOfType,
  getClaimsListsOfType,
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
import { getAllClaimsOfType, getClaimsOfType } from "@/document/claims"

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

test("CoreClaim methods (Get, Remove, Size, AllClaims on sub-claims)", () => {
  const prop = Identifier.new().toString()
  const otherProp = Identifier.new().toString()

  const claim = new NoneClaim({
    id: Identifier.new().toString(),
    confidence: HighConfidence,
    prop: { id: prop },
  })

  // Initially empty sub-claims.
  assert.equal(claim.Size(), 0)
  assert.deepEqual(claim.AllClaims(), [])
  assert.deepEqual(claim.Get(prop), [])

  // Add two sub-claims.
  const subClaim1 = new StringClaim({
    id: Identifier.new().toString(),
    confidence: HighConfidence,
    prop: { id: prop },
    string: "meta1",
  })
  const subClaim2 = new UnknownClaim({
    id: Identifier.new().toString(),
    confidence: HighConfidence,
    prop: { id: otherProp },
  })
  claim.Add(subClaim1)
  claim.Add(subClaim2)

  assert.equal(claim.Size(), 2)
  assert.equal(claim.AllClaims().length, 2)

  // Get by prop returns only matching.
  const got = claim.Get(prop)
  assert.equal(got.length, 1)
  assert.equal(got[0].GetID(), subClaim1.GetID())

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
    new TimeClaim({ id: Identifier.new().toString(), confidence: HighConfidence, prop: { id: prop }, time: "2025", precision: "y" }),
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

test("ClaimTypes RemoveByID in sub-claims", () => {
  const prop = Identifier.new().toString()
  const outerID = Identifier.new().toString()
  const innerID = Identifier.new().toString()

  const ct = new ClaimTypes({
    none: [
      {
        id: outerID,
        confidence: 1.0,
        prop: { id: prop },
        sub: {
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
          sub: {
            id: [{ id: Identifier.new().toString(), confidence: 1.0, prop: { id: LIST }, value: listA }],
            amount: [{ id: Identifier.new().toString(), confidence: 1.0, prop: { id: ORDER_IN_LIST }, amount: "2", precision: 1 }],
          },
        },
        {
          id: Identifier.new().toString(),
          confidence: 1.0,
          prop: { id: prop },
          string: "a1",
          sub: {
            id: [{ id: Identifier.new().toString(), confidence: 1.0, prop: { id: LIST }, value: listA }],
            amount: [{ id: Identifier.new().toString(), confidence: 1.0, prop: { id: ORDER_IN_LIST }, amount: "1", precision: 1 }],
          },
        },
        {
          id: Identifier.new().toString(),
          confidence: 1.0,
          prop: { id: prop },
          string: "b1",
          sub: {
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

test("CoreClaim Validate with invalid sub-claims", async () => {
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

test("ClaimTypes Validate duplicate ID nested in a sub-claim", async () => {
  // The duplicate ID is shared between a top-level claim and a claim nested in another claim's
  // sub-claims, so it is caught only by checking uniqueness across the whole tree (AllClaimsWithSub).
  const sharedID = Identifier.new().toString()
  const prop = Identifier.new().toString()
  const ct = new ClaimTypes({
    string: [
      {
        id: Identifier.new().toString(),
        confidence: 1.0,
        prop: { id: prop },
        string: "parent",
        sub: { string: [{ id: sharedID, confidence: 1.0, prop: { id: prop }, string: "nested" }] },
      },
      { id: sharedID, confidence: 0.5, prop: { id: prop }, string: "sibling" },
    ],
  })
  await expect(ct.Validate()).rejects.toThrow("duplicate claim ID")
})

describe("AmountClaim Validate", () => {
  const id = Identifier.new().toString()
  const prop = Identifier.new().toString()

  test("valid integer", async () => {
    const claim = new AmountClaim({ id, confidence: 1.0, prop: { id: prop }, amount: "10", precision: 1 })
    await claim.Validate()
  })
  test("valid decimal", async () => {
    const claim = new AmountClaim({ id, confidence: 1.0, prop: { id: prop }, amount: "3.1", precision: 0.1 })
    await claim.Validate()
  })
  test("invalid format", async () => {
    const claim = new AmountClaim({ id, confidence: 1.0, prop: { id: prop }, amount: "not-a-number", precision: 1 })
    await expect(claim.Validate()).rejects.toThrow("unable to parse amount")
  })
  test("not rounded to precision", async () => {
    const claim = new AmountClaim({ id, confidence: 1.0, prop: { id: prop }, amount: "12", precision: 10 })
    await expect(claim.Validate()).rejects.toThrow("amount is not rounded to precision")
  })
  test("decimal count mismatch", async () => {
    const claim = new AmountClaim({ id, confidence: 1.0, prop: { id: prop }, amount: "3", precision: 0.1 })
    await expect(claim.Validate()).rejects.toThrow("number of decimal digits does not match precision")
  })
  test("zero precision rejected", async () => {
    const claim = new AmountClaim({ id, confidence: 1.0, prop: { id: prop }, amount: "10", precision: 0 })
    await expect(claim.Validate()).rejects.toThrow("Precision must be a finite positive number")
  })
})

describe("AmountIntervalClaim Validate", () => {
  const id = Identifier.new().toString()
  const prop = Identifier.new().toString()
  const make = (obj: Partial<ConstructorParameters<typeof AmountIntervalClaim>[0]>) => new AmountIntervalClaim({ id, confidence: 1.0, prop: { id: prop }, ...obj })

  test("simple forward interval valid", async () => {
    const claim = make({ from: "10", fromPrecision: 1, to: "20", toPrecision: 1 })
    await claim.Validate()
  })
  test("forward adjacent (prec=10) valid", async () => {
    const claim = make({ from: "10", fromPrecision: 10, to: "20", toPrecision: 10 })
    await claim.Validate()
  })
  test("directed-decreasing adjacent valid (swap)", async () => {
    const claim = make({ from: "11", fromPrecision: 1, to: "10", toPrecision: 1 })
    await claim.Validate()
  })
  test("same-point closed valid (single point)", async () => {
    const claim = make({ from: "10", fromPrecision: 1, to: "10", toPrecision: 1 })
    await claim.Validate()
  })
  test("same-point FromIsOpen empty", async () => {
    const claim = make({ from: "10", fromPrecision: 1, fromIsOpen: true, to: "10", toPrecision: 1 })
    await expect(claim.Validate()).rejects.toThrow("interval is empty")
  })
  test("same-point ToIsOpen empty", async () => {
    const claim = make({ from: "10", fromPrecision: 1, to: "10", toPrecision: 1, toIsOpen: true })
    await expect(claim.Validate()).rejects.toThrow("interval is empty")
  })
  test("same-point both open empty", async () => {
    const claim = make({ from: "10", fromPrecision: 1, fromIsOpen: true, to: "10", toPrecision: 1, toIsOpen: true })
    await expect(claim.Validate()).rejects.toThrow("interval is empty")
  })
  test("directed-decreasing adjacent both open empty", async () => {
    const claim = make({ from: "11", fromPrecision: 1, fromIsOpen: true, to: "10", toPrecision: 1, toIsOpen: true })
    await expect(claim.Validate()).rejects.toThrow("interval is empty")
  })
  test("equal value different precision FromIsOpen valid", async () => {
    const claim = make({ from: "10", fromPrecision: 1, fromIsOpen: true, to: "10", toPrecision: 10 })
    await claim.Validate()
  })
  test("FromIsNone with valid To", async () => {
    const claim = make({ fromIsNone: true, to: "20", toPrecision: 1 })
    await claim.Validate()
  })
})

describe("TimeClaim Validate", () => {
  const id = Identifier.new().toString()
  const prop = Identifier.new().toString()

  test("year valid", async () => {
    const claim = new TimeClaim({ id, confidence: 1.0, prop: { id: prop }, time: "2025", precision: "y" })
    await claim.Validate()
  })
  test("day valid", async () => {
    const claim = new TimeClaim({ id, confidence: 1.0, prop: { id: prop }, time: "2025-01-15", precision: "d" })
    await claim.Validate()
  })
  test("month valid (day=00)", async () => {
    const claim = new TimeClaim({ id, confidence: 1.0, prop: { id: prop }, time: "2025-06-00", precision: "m" })
    await claim.Validate()
  })
  test("invalid format", async () => {
    const claim = new TimeClaim({ id, confidence: 1.0, prop: { id: prop }, time: "not-a-time", precision: "d" })
    await expect(claim.Validate()).rejects.toThrow("unable to parse time")
  })
  test("year-only with day precision rejected", async () => {
    const claim = new TimeClaim({ id, confidence: 1.0, prop: { id: prop }, time: "2025", precision: "d" })
    await expect(claim.Validate()).rejects.toThrow()
  })
  test("year+month+day with year precision rejected", async () => {
    const claim = new TimeClaim({ id, confidence: 1.0, prop: { id: prop }, time: "2025-06-15", precision: "y" })
    await expect(claim.Validate()).rejects.toThrow()
  })
  test("decade precision requires year multiple of 10", async () => {
    const claim = new TimeClaim({ id, confidence: 1.0, prop: { id: prop }, time: "2025", precision: "10y" })
    await expect(claim.Validate()).rejects.toThrow("year not rounded to precision")
  })
  test("decade precision with year=2020 valid", async () => {
    const claim = new TimeClaim({ id, confidence: 1.0, prop: { id: prop }, time: "2020", precision: "10y" })
    await claim.Validate()
  })
})

describe("TimeIntervalClaim Validate", () => {
  const id = Identifier.new().toString()
  const prop = Identifier.new().toString()
  const make = (obj: Partial<ConstructorParameters<typeof TimeIntervalClaim>[0]>) => new TimeIntervalClaim({ id, confidence: 1.0, prop: { id: prop }, ...obj })

  test("simple forward year interval valid", async () => {
    const claim = make({ from: "2020", fromPrecision: "y", to: "2025", toPrecision: "y" })
    await claim.Validate()
  })
  test("directed-decreasing adjacent year valid (swap)", async () => {
    const claim = make({ from: "2025", fromPrecision: "y", to: "2024", toPrecision: "y" })
    await claim.Validate()
  })
  test("same-point year both open empty", async () => {
    const claim = make({ from: "2025", fromPrecision: "y", fromIsOpen: true, to: "2025", toPrecision: "y", toIsOpen: true })
    await expect(claim.Validate()).rejects.toThrow("interval is empty")
  })
  test("precision-coarsening (day from, year to) not swapped", async () => {
    // from=2025-10-21 day, to=2025 year. Different precisions: un-swapped-empty
    // criterion. start=2025-10-21, end=2026-01-01 -> not empty, no swap.
    const claim = make({ from: "2025-10-21", fromPrecision: "d", to: "2025", toPrecision: "y" })
    await claim.Validate()
  })
  test("directed-decreasing adjacent both open empty", async () => {
    const claim = make({ from: "2025", fromPrecision: "y", fromIsOpen: true, to: "2024", toPrecision: "y", toIsOpen: true })
    await expect(claim.Validate()).rejects.toThrow("interval is empty")
  })
  test("FromIsNone with valid To", async () => {
    const claim = make({ fromIsNone: true, to: "2024-12-31", toPrecision: "d" })
    await claim.Validate()
  })
  test("ToIsUnknown with valid From", async () => {
    const claim = make({ from: "2024-01-01", fromPrecision: "d", toIsUnknown: true })
    await claim.Validate()
  })
})

describe("LinkClaim Validate", () => {
  const id = Identifier.new().toString()
  const prop = Identifier.new().toString()

  // Bypasses the constructor's "iri is required" guard so we can exercise
  // the disallowed cases directly on Validate.
  function makeClaim(iri: string): LinkClaim {
    const claim = new LinkClaim({ id, confidence: 1.0, prop: { id: prop }, iri: "https://placeholder.invalid" })
    claim.iri = iri
    return claim
  }

  // IRI allow/deny rules match validateURL in document/urls.go (the same URL
  // validation used for the editor schema's link attributes via validateUrl).
  test.each([
    "https://example.com",
    "https://example.com/path?q=1#section",
    "http://example.com/foo",
    "HTTPS://Example.com",
    "mailto:test@example.com",
    "/foo",
    "/foo/bar?q=1#h",
    "/",
  ])("accepts %s", async (iri) => {
    const claim = makeClaim(iri)
    await claim.Validate()
  })

  test.each([
    ["", "empty URL"],
    ["#section", "invalid IRI"],
    ["../foo", "invalid IRI"],
    ["foo/bar", "invalid IRI"],
    ["//example.com/foo", "invalid IRI"],
    ["javascript:alert(1)", "disallowed URL scheme: javascript:"],
    ["ftp://example.com", "disallowed URL scheme: ftp:"],
    ["tel:+1234", "disallowed URL scheme: tel:"],
    ["data:text/html,<x>", "disallowed URL scheme: data:"],
    ["http:///example.com", "invalid URL: missing host"],
    ["mailto:", "invalid URL: missing address"],
  ])("rejects %s", async (iri, fragment) => {
    const claim = makeClaim(iri)
    await expect(claim.Validate()).rejects.toThrow(fragment)
  })
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

test("CoreClaim Get and Remove with no sub-claims", () => {
  const prop = Identifier.new().toString()
  const claim = new NoneClaim({
    id: Identifier.new().toString(),
    confidence: HighConfidence,
    prop: { id: prop },
  })

  // Get on a claim with no sub-claims returns empty.
  assert.deepEqual(claim.Get(prop), [])

  // Remove on a claim with no sub-claims returns empty.
  assert.deepEqual(claim.Remove(prop), [])
})

test("ClaimTypes Add non-matching object does nothing", () => {
  const ct = new ClaimTypes({})

  // Adding a plain object that is not an instanceof any claim type.
  ct.Add({ id: Identifier.new().toString(), confidence: 1.0, prop: { id: Identifier.new().toString() } } as never)
  assert.equal(ct.Size(), 0)
})

describe("ReplaceByID", () => {
  test("replaces a nested claim, preserving its container", () => {
    const prop = Identifier.new().toString()
    const topID = Identifier.new().toString()
    const subID = Identifier.new().toString()

    const ct = new ClaimTypes({})
    const top = new HasClaim({ id: topID, confidence: HighConfidence, prop: { id: prop } })
    ct.Add(top)
    top.Add(new StringClaim({ id: subID, confidence: HighConfidence, prop: { id: prop }, string: "x" }))

    const newSub = new UnknownClaim({ id: subID, confidence: HighConfidence, prop: { id: prop } })
    const old = ct.ReplaceByID(subID, newSub)
    assert.instanceOf(old, StringClaim)

    // Only `top` remains at the top level; the replacement stays nested under it.
    const topLevel = ct.AllClaims()
    assert.equal(topLevel.length, 1)
    assert.equal(topLevel[0].GetID(), topID)

    const nested = ct.GetByID(topID)!.GetByID(subID)
    assert.instanceOf(nested, UnknownClaim)
  })

  test("returns undefined for a non-existent ID", () => {
    const ct = new ClaimTypes({})
    ct.Add(new HasClaim({ id: Identifier.new().toString(), confidence: HighConfidence, prop: { id: Identifier.new().toString() } }))
    const old = ct.ReplaceByID(
      Identifier.new().toString(),
      new UnknownClaim({ id: Identifier.new().toString(), confidence: HighConfidence, prop: { id: Identifier.new().toString() } }),
    )
    assert.isUndefined(old)
  })
})

describe("claimTypeName", () => {
  test("returns the type name for a claim instance", () => {
    const prop = Identifier.new().toString()
    assert.equal(claimTypeName(new ReferenceClaim({ id: Identifier.new().toString(), confidence: HighConfidence, prop: { id: prop }, to: { id: Identifier.new().toString() } })), "ref")
    assert.equal(claimTypeName(new UnknownClaim({ id: Identifier.new().toString(), confidence: HighConfidence, prop: { id: prop } })), "unknown")
    assert.equal(claimTypeName(new StringClaim({ id: Identifier.new().toString(), confidence: HighConfidence, prop: { id: prop }, string: "x" })), "string")
  })
})
