// @vitest-environment jsdom
// HTMLClaimPatch.Validate checks HTML canonicality through a parse/serialize round trip
// in the editor schema, which needs a DOM implementation.

import { Identifier } from "@tozd/identifier"
import { assert, describe, expect, test } from "vitest"

import {
  AddClaimChange,
  AmountClaim,
  AmountClaimPatch,
  AmountIntervalClaim,
  AmountIntervalClaimPatch,
  CastClaimChange,
  Changes,
  D,
  HasClaimPatch,
  HTMLClaim,
  HTMLClaimPatch,
  IdentifierClaim,
  IdentifierClaimPatch,
  LinkClaim,
  LinkClaimPatch,
  LowConfidence,
  NoneClaim,
  NoneClaimPatch,
  ReferenceClaim,
  ReferenceClaimPatch,
  RemoveClaimChange,
  SetClaimChange,
  StringClaim,
  StringClaimPatch,
  TimeClaim,
  TimeClaimPatch,
  TimeIntervalClaim,
  TimeIntervalClaimPatch,
  UnknownClaimPatch,
  changeFrom,
} from "@/document"

describe("patch New and Apply", () => {
  const prop = Identifier.new().toString()

  test("IdentifierClaimPatch", async () => {
    const p = new IdentifierClaimPatch({ type: "id", prop, value: "Q42", confidence: 1.0 })
    const claim = p.New(Identifier.new().toString()) as IdentifierClaim
    assert.equal(claim.value, "Q42")
    assert.equal(claim.prop.id, prop)

    await p.Apply(claim)
    await new IdentifierClaimPatch({ type: "id", value: "P31" }).Apply(claim)
    assert.equal(claim.value, "P31")
  })

  test("StringClaimPatch", async () => {
    const p = new StringClaimPatch({ type: "string", prop, string: "hello world", confidence: 1.0 })
    const claim = p.New(Identifier.new().toString()) as StringClaim
    assert.equal(claim.string, "hello world")

    await new StringClaimPatch({ type: "string", string: "updated" }).Apply(claim)
    assert.equal(claim.string, "updated")
  })

  test("HTMLClaimPatch", async () => {
    const p = new HTMLClaimPatch({ type: "html", prop, html: "<p><b>bold</b></p>", confidence: 1.0 })
    const claim = p.New(Identifier.new().toString()) as HTMLClaim
    assert.equal(claim.html, "<p><b>bold</b></p>")

    await new HTMLClaimPatch({ type: "html", html: "<p><i>italic</i></p>" }).Apply(claim)
    assert.equal(claim.html, "<p><i>italic</i></p>")
  })

  test("AmountClaimPatch", async () => {
    const p = new AmountClaimPatch({ type: "amount", prop, amount: "42", precision: 1, confidence: 1.0 })
    const claim = p.New(Identifier.new().toString()) as AmountClaim
    assert.equal(claim.amount, "42")
    assert.equal(claim.precision, 1)

    await new AmountClaimPatch({ type: "amount", amount: "99" }).Apply(claim)
    assert.equal(claim.amount, "99")
  })

  test("TimeClaimPatch", async () => {
    const p = new TimeClaimPatch({ type: "time", prop, time: "2025-06-15", precision: "d", confidence: 1.0 })
    const claim = p.New(Identifier.new().toString()) as TimeClaim
    assert.equal(claim.time, "2025-06-15")
    assert.equal(claim.precision, "d")

    await new TimeClaimPatch({ type: "time", time: "2026-01-01" }).Apply(claim)
    assert.equal(claim.time, "2026-01-01")
  })

  test("LinkClaimPatch", async () => {
    const p = new LinkClaimPatch({ type: "link", prop, iri: "https://example.com/resource", confidence: 1.0 })
    const claim = p.New(Identifier.new().toString()) as LinkClaim
    assert.equal(claim.iri, "https://example.com/resource")

    await new LinkClaimPatch({ type: "link", iri: "https://example.org/other" }).Apply(claim)
    assert.equal(claim.iri, "https://example.org/other")
  })

  test("ReferenceClaimPatch", async () => {
    const target = Identifier.new().toString()
    const p = new ReferenceClaimPatch({ type: "ref", prop, to: target, confidence: 1.0 })
    const claim = p.New(Identifier.new().toString()) as ReferenceClaim
    assert.equal(claim.to.id, target)

    const newTarget = Identifier.new().toString()
    await new ReferenceClaimPatch({ type: "ref", to: newTarget }).Apply(claim)
    assert.equal(claim.to.id, newTarget)
  })

  test("HasClaimPatch", async () => {
    const p = new HasClaimPatch({ type: "has", prop, confidence: 1.0 })
    const claim = p.New(Identifier.new().toString())
    assert.equal(claim.prop.id, prop)

    await new HasClaimPatch({ type: "has", confidence: 0.5 }).Apply(claim)
    assert.equal(claim.confidence, 0.5)
  })

  test("NoneClaimPatch", async () => {
    const p = new NoneClaimPatch({ type: "none", prop, confidence: 1.0 })
    const claim = p.New(Identifier.new().toString())
    assert.equal(claim.prop.id, prop)

    const newProp = Identifier.new().toString()
    await new NoneClaimPatch({ type: "none", prop: newProp }).Apply(claim)
    assert.equal(claim.prop.id, newProp)
  })

  test("UnknownClaimPatch", async () => {
    const p = new UnknownClaimPatch({ type: "unknown", prop, confidence: 1.0 })
    const claim = p.New(Identifier.new().toString())
    assert.equal(claim.confidence, 1.0)

    await new UnknownClaimPatch({ type: "unknown", confidence: 0.75 }).Apply(claim)
    assert.equal(claim.confidence, 0.75)
  })

  test("AmountIntervalClaimPatch New", () => {
    const p = new AmountIntervalClaimPatch({
      type: "amountInterval",
      prop,
      confidence: 1.0,
      from: "1.5",
      fromPrecision: 0.1,
      to: "9.5",
      toPrecision: 0.1,
    })
    const claim = p.New(Identifier.new().toString()) as AmountIntervalClaim
    assert.equal(claim.from, "1.5")
    assert.equal(claim.fromPrecision, 0.1)
    assert.equal(claim.to, "9.5")
    assert.equal(claim.toPrecision, 0.1)
    assert.equal(claim.confidence, 1.0)
    assert.equal(claim.prop.id, prop)
  })

  test("TimeIntervalClaimPatch New", () => {
    const p = new TimeIntervalClaimPatch({
      type: "timeInterval",
      prop,
      confidence: 1.0,
      from: "2020-01-01",
      fromPrecision: "d",
      to: "2021-01-01",
      toPrecision: "d",
    })
    const claim = p.New(Identifier.new().toString()) as TimeIntervalClaim
    assert.equal(claim.from, "2020-01-01")
    assert.equal(claim.fromPrecision, "d")
    assert.equal(claim.to, "2021-01-01")
    assert.equal(claim.toPrecision, "d")
    assert.equal(claim.confidence, 1.0)
    assert.equal(claim.prop.id, prop)
  })

  test("LinkClaimPatch Apply confidence only", async () => {
    const p = new LinkClaimPatch({ type: "link", prop, iri: "https://example.com", confidence: 1.0 })
    const claim = p.New(Identifier.new().toString()) as LinkClaim
    assert.equal(claim.confidence, 1.0)

    await new LinkClaimPatch({ type: "link", confidence: LowConfidence }).Apply(claim)
    assert.equal(claim.confidence, LowConfidence)
    assert.equal(claim.iri, "https://example.com") // Unchanged.
  })
})

describe("patch New incomplete", () => {
  const cases: [string, object][] = [
    ["IdentifierClaimPatch", { type: "id" }],
    ["StringClaimPatch", { type: "string" }],
    ["HTMLClaimPatch", { type: "html" }],
    ["AmountClaimPatch", { type: "amount" }],
    ["AmountIntervalClaimPatch", { type: "amountInterval" }],
    ["TimeClaimPatch", { type: "time" }],
    ["TimeIntervalClaimPatch", { type: "timeInterval" }],
    ["LinkClaimPatch", { type: "link" }],
    ["ReferenceClaimPatch", { type: "ref" }],
    ["HasClaimPatch", { type: "has" }],
    ["NoneClaimPatch", { type: "none" }],
    ["UnknownClaimPatch", { type: "unknown" }],
  ]

  for (const [name, patchObj] of cases) {
    test(`${name} throws on New with incomplete fields`, () => {
      const patches: Record<string, new (obj: object) => { New(id: string): unknown }> = {
        IdentifierClaimPatch,
        StringClaimPatch,
        HTMLClaimPatch,
        AmountClaimPatch,
        AmountIntervalClaimPatch,
        TimeClaimPatch,
        TimeIntervalClaimPatch,
        LinkClaimPatch,
        ReferenceClaimPatch,
        HasClaimPatch,
        NoneClaimPatch,
        UnknownClaimPatch,
      }
      const PatchClass = patches[name]
      const p = new PatchClass(patchObj)
      expect(() => p.New(Identifier.new().toString())).toThrow("incomplete patch")
    })
  }
})

describe("patch Apply wrong type", () => {
  const wrongClaim = new NoneClaim({
    id: Identifier.new().toString(),
    confidence: 1.0,
    prop: { id: Identifier.new().toString() },
  })
  const prop = Identifier.new().toString()

  const cases: [string, object, string][] = [
    ["IdentifierClaimPatch", { type: "id", value: "Q42", confidence: 1.0 }, "not identifier claim"],
    ["StringClaimPatch", { type: "string", string: "x", confidence: 1.0 }, "not string claim"],
    ["HTMLClaimPatch", { type: "html", html: "<p>x</p>", confidence: 1.0 }, "not HTML claim"],
    ["AmountClaimPatch", { type: "amount", amount: "42", precision: 1, confidence: 1.0 }, "not amount claim"],
    ["AmountIntervalClaimPatch", { type: "amountInterval", from: "1.0", fromPrecision: 0.1, confidence: 1.0 }, "not amount interval claim"],
    ["TimeClaimPatch", { type: "time", time: "2025", confidence: 1.0 }, "not time claim"],
    ["TimeIntervalClaimPatch", { type: "timeInterval", from: "2020", fromPrecision: "y", confidence: 1.0 }, "not time interval claim"],
    ["LinkClaimPatch", { type: "link", iri: "https://example.com/", confidence: 1.0 }, "not link claim"],
    ["ReferenceClaimPatch", { type: "ref", to: "x", confidence: 1.0 }, "not reference claim"],
    ["HasClaimPatch", { type: "has", prop, confidence: 1.0 }, "not has claim"],
    ["UnknownClaimPatch", { type: "unknown", prop, confidence: 1.0 }, "not unknown claim"],
  ]

  for (const [name, patchObj, errorMsg] of cases) {
    test(`${name} throws "${errorMsg}"`, async () => {
      const patches: Record<string, new (obj: object) => { Apply(c: unknown): Promise<void> }> = {
        IdentifierClaimPatch,
        StringClaimPatch,
        HTMLClaimPatch,
        AmountClaimPatch,
        AmountIntervalClaimPatch,
        TimeClaimPatch,
        TimeIntervalClaimPatch,
        LinkClaimPatch,
        ReferenceClaimPatch,
        HasClaimPatch,
        UnknownClaimPatch,
      }
      const PatchClass = patches[name]
      const p = new PatchClass(patchObj)
      await expect(p.Apply(wrongClaim)).rejects.toThrow(errorMsg)
    })
  }
})

test("NoneClaimPatch Apply wrong type", async () => {
  const wrongNonNone = new StringClaim({
    id: Identifier.new().toString(),
    confidence: 1.0,
    prop: { id: Identifier.new().toString() },
    string: "x",
  })
  await expect(new NoneClaimPatch({ type: "none", confidence: 1.0 }).Apply(wrongNonNone)).rejects.toThrow("not none claim")
})

describe("patch Apply empty patch", () => {
  test("StringClaimPatch empty", async () => {
    const claim = new StringClaim({ id: Identifier.new().toString(), confidence: 1.0, prop: { id: Identifier.new().toString() }, string: "x" })
    await expect(new StringClaimPatch({ type: "string" }).Apply(claim)).rejects.toThrow("empty patch")
  })

  test("HTMLClaimPatch empty", async () => {
    const claim = new HTMLClaim({ id: Identifier.new().toString(), confidence: 1.0, prop: { id: Identifier.new().toString() }, html: "x" })
    await expect(new HTMLClaimPatch({ type: "html" }).Apply(claim)).rejects.toThrow("empty patch")
  })

  test("LinkClaimPatch empty", async () => {
    const claim = new LinkClaim({ id: Identifier.new().toString(), confidence: 1.0, prop: { id: Identifier.new().toString() }, iri: "x" })
    await expect(new LinkClaimPatch({ type: "link" }).Apply(claim)).rejects.toThrow("empty patch")
  })

  test("IdentifierClaimPatch empty", async () => {
    const claim = new IdentifierClaim({ id: Identifier.new().toString(), confidence: 1.0, prop: { id: Identifier.new().toString() }, value: "x" })
    await expect(new IdentifierClaimPatch({ type: "id" }).Apply(claim)).rejects.toThrow("empty patch")
  })

  test("AmountClaimPatch empty", async () => {
    const claim = new AmountClaim({ id: Identifier.new().toString(), confidence: 1.0, prop: { id: Identifier.new().toString() }, amount: "1", precision: 1 })
    await expect(new AmountClaimPatch({ type: "amount" }).Apply(claim)).rejects.toThrow("empty patch")
  })

  test("AmountIntervalClaimPatch empty", async () => {
    const claim = new AmountIntervalClaim({
      id: Identifier.new().toString(),
      confidence: 1.0,
      prop: { id: Identifier.new().toString() },
      from: "1",
      fromPrecision: 0.1,
      to: "9",
      toPrecision: 0.1,
    })
    await expect(new AmountIntervalClaimPatch({ type: "amountInterval" }).Apply(claim)).rejects.toThrow("empty patch")
  })

  test("TimeIntervalClaimPatch empty", async () => {
    const claim = new TimeIntervalClaim({
      id: Identifier.new().toString(),
      confidence: 1.0,
      prop: { id: Identifier.new().toString() },
      from: "2020",
      fromPrecision: "y",
      to: "2025",
      toPrecision: "y",
    })
    await expect(new TimeIntervalClaimPatch({ type: "timeInterval" }).Apply(claim)).rejects.toThrow("empty patch")
  })

  test("TimeClaimPatch empty", async () => {
    const claim = new TimeClaim({ id: Identifier.new().toString(), confidence: 1.0, prop: { id: Identifier.new().toString() }, time: "2025", precision: "y" })
    await expect(new TimeClaimPatch({ type: "time" }).Apply(claim)).rejects.toThrow("empty patch")
  })

  test("ReferenceClaimPatch empty", async () => {
    const claim = new ReferenceClaim({
      id: Identifier.new().toString(),
      confidence: 1.0,
      prop: { id: Identifier.new().toString() },
      to: { id: Identifier.new().toString() },
    })
    await expect(new ReferenceClaimPatch({ type: "ref" }).Apply(claim)).rejects.toThrow("empty patch")
  })
})

describe("SetClaimChange", () => {
  test("Apply updates existing claim", async () => {
    const base = [Identifier.new().toString()]
    const prop = Identifier.new().toString()
    const claimID = (await Identifier.from(...base, "1")).toString()
    const doc = new D({ id: (await Identifier.from(...base)).toString(), base })

    const changes = new Changes({
      type: "add",
      id: claimID,
      base: [...base, "1"],
      patch: { type: "string", prop, string: "original", confidence: 1.0 },
    })
    await changes.Apply(doc)
    assert.equal(doc.Size(), 1)

    const setChange = new SetClaimChange({
      type: "set",
      id: claimID,
      patch: { type: "string", string: "updated" },
    })
    await setChange.Apply(doc)

    const claim = doc.GetByID(claimID) as StringClaim
    assert.equal(claim.string, "updated")
  })

  test("Apply throws for non-existent ID", async () => {
    const base = [Identifier.new().toString()]
    const doc = new D({ id: (await Identifier.from(...base)).toString(), base })

    const setChange = new SetClaimChange({
      type: "set",
      id: Identifier.new().toString(),
      patch: { type: "string", string: "x" },
    })
    await expect(setChange.Apply(doc)).rejects.toThrow("claim not found")
  })
})

describe("CastClaimChange", () => {
  test("Apply changes type, preserving id and sub-claims", async () => {
    const base = [Identifier.new().toString()]
    const prop1 = Identifier.new().toString()
    const prop2 = Identifier.new().toString()
    const notesProp = Identifier.new().toString()
    const target = Identifier.new().toString()
    const claimID = (await Identifier.from(...base, "1")).toString()
    const subID = (await Identifier.from(...base, "2")).toString()
    const doc = new D({ id: (await Identifier.from(...base)).toString(), base })

    // Unknown claim (e.g. studio with an unknown location) with a notes sub-claim.
    await new Changes(
      { type: "add", id: claimID, base: [...base, "1"], patch: { type: "unknown", prop: prop1, confidence: 1.0 } },
      { type: "add", under: claimID, id: subID, base: [...base, "2"], patch: { type: "html", prop: notesProp, html: "<p>notes</p>", confidence: 1.0 } },
    ).Apply(doc)

    const castChange = new CastClaimChange({
      type: "cast",
      id: claimID,
      patch: { type: "ref", prop: prop2, to: target, confidence: 0.5 },
    })
    await castChange.Apply(doc)

    const claim = doc.GetByID(claimID)
    assert.instanceOf(claim, ReferenceClaim)
    assert.equal(claim.GetID(), claimID)
    assert.equal(claim.prop.id, prop2)
    assert.equal(claim.to.id, target)
    assert.equal(claim.confidence, 0.5)

    // Sub-claims preserved on the new claim.
    const sub = claim.GetByID(subID)
    assert.instanceOf(sub, HTMLClaim)
    assert.equal(sub.html, "<p>notes</p>")
  })

  test("Apply rejects a cast that does not change the type", async () => {
    const base = [Identifier.new().toString()]
    const prop = Identifier.new().toString()
    const target = Identifier.new().toString()
    const claimID = (await Identifier.from(...base, "1")).toString()
    const doc = new D({ id: (await Identifier.from(...base)).toString(), base })
    await new Changes({ type: "add", id: claimID, base: [...base, "1"], patch: { type: "ref", prop, to: target, confidence: 1.0 } }).Apply(doc)

    const castChange = new CastClaimChange({ type: "cast", id: claimID, patch: { type: "ref", prop, to: target, confidence: 1.0 } })
    await expect(castChange.Apply(doc)).rejects.toThrow("cast does not change claim type")
  })

  test("Apply throws for non-existent ID", async () => {
    const base = [Identifier.new().toString()]
    const doc = new D({ id: (await Identifier.from(...base)).toString(), base })
    const castChange = new CastClaimChange({ type: "cast", id: Identifier.new().toString(), patch: { type: "unknown", prop: Identifier.new().toString(), confidence: 1.0 } })
    await expect(castChange.Apply(doc)).rejects.toThrow("claim not found")
  })

  test("Validate rejects an incomplete patch", async () => {
    const castChange = new CastClaimChange({ type: "cast", id: Identifier.new().toString(), patch: { type: "ref" } })
    await expect(castChange.Validate([], 1)).rejects.toThrow()
  })

  test("changeFrom round-trips a cast", () => {
    const id = Identifier.new().toString()
    const prop = Identifier.new().toString()
    const to = Identifier.new().toString()
    const change = changeFrom({ type: "cast", id, patch: { type: "ref", prop, to, confidence: 1.0 } })
    assert.instanceOf(change, CastClaimChange)
    assert.equal(change.id, id)
  })
})

describe("RemoveClaimChange", () => {
  test("Apply removes existing claim", async () => {
    const base = [Identifier.new().toString()]
    const prop = Identifier.new().toString()
    const claimID = (await Identifier.from(...base, "1")).toString()
    const doc = new D({ id: (await Identifier.from(...base)).toString(), base })

    const changes = new Changes({
      type: "add",
      id: claimID,
      base: [...base, "1"],
      patch: { type: "none", prop, confidence: 1.0 },
    })
    await changes.Apply(doc)
    assert.equal(doc.Size(), 1)

    const removeChange = new RemoveClaimChange({ type: "remove", id: claimID })
    await removeChange.Apply(doc)
    assert.equal(doc.Size(), 0)
  })

  test("Apply throws for already removed claim", async () => {
    const base = [Identifier.new().toString()]
    const prop = Identifier.new().toString()
    const claimID = (await Identifier.from(...base, "1")).toString()
    const doc = new D({ id: (await Identifier.from(...base)).toString(), base })

    const changes = new Changes({
      type: "add",
      id: claimID,
      base: [...base, "1"],
      patch: { type: "none", prop, confidence: 1.0 },
    })
    await changes.Apply(doc)
    await new RemoveClaimChange({ type: "remove", id: claimID }).Apply(doc)

    await expect(new RemoveClaimChange({ type: "remove", id: claimID }).Apply(doc)).rejects.toThrow("claim not found")
  })
})

describe("AddClaimChange", () => {
  test("Apply under non-existent claim throws", async () => {
    const base = [Identifier.new().toString()]
    const doc = new D({ id: (await Identifier.from(...base)).toString(), base })

    const change = new AddClaimChange({
      type: "add",
      under: Identifier.new().toString(),
      id: Identifier.new().toString(),
      base: ["dummy"],
      patch: { type: "none", prop: Identifier.new().toString(), confidence: 1.0 },
    })
    await expect(change.Apply(doc)).rejects.toThrow("claim not found")
  })
})

describe("AmountIntervalClaimPatch Apply branches", () => {
  test("setting from/fromPrecision, fromIsUnknown, fromIsNone, toIsUnknown, toIsNone, confidence+prop", async () => {
    const prop = Identifier.new().toString()
    const claim = new AmountIntervalClaim({
      id: Identifier.new().toString(),
      confidence: 1.0,
      prop: { id: prop },
      from: "10",
      fromPrecision: 1,
      to: "20",
      toPrecision: 1,
    })

    // Set from and fromPrecision.
    await new AmountIntervalClaimPatch({ type: "amountInterval", from: "15.0", fromPrecision: 0.1 }).Apply(claim)
    assert.equal(claim.from, "15.0")
    assert.equal(claim.fromPrecision, 0.1)

    // fromIsUnknown clears from and fromPrecision.
    await new AmountIntervalClaimPatch({ type: "amountInterval", fromIsUnknown: true }).Apply(claim)
    assert.equal(claim.fromIsUnknown, true)
    assert.equal(claim.from, undefined)
    assert.equal(claim.fromPrecision, undefined)

    // Restore from for next test.
    await new AmountIntervalClaimPatch({ type: "amountInterval", from: "10", fromPrecision: 1, fromIsUnknown: false }).Apply(claim)
    assert.equal(claim.from, "10")

    // fromIsNone clears from and fromPrecision.
    await new AmountIntervalClaimPatch({ type: "amountInterval", fromIsNone: true }).Apply(claim)
    assert.equal(claim.fromIsNone, true)
    assert.equal(claim.from, undefined)
    assert.equal(claim.fromPrecision, undefined)

    // toIsUnknown clears to and toPrecision.
    await new AmountIntervalClaimPatch({ type: "amountInterval", toIsUnknown: true }).Apply(claim)
    assert.equal(claim.toIsUnknown, true)
    assert.equal(claim.to, undefined)
    assert.equal(claim.toPrecision, undefined)

    // Restore to for next test.
    await new AmountIntervalClaimPatch({ type: "amountInterval", to: "20", toPrecision: 1, toIsUnknown: false }).Apply(claim)
    assert.equal(claim.to, "20")

    // toIsNone clears to and toPrecision.
    await new AmountIntervalClaimPatch({ type: "amountInterval", toIsNone: true }).Apply(claim)
    assert.equal(claim.toIsNone, true)
    assert.equal(claim.to, undefined)
    assert.equal(claim.toPrecision, undefined)

    // Update confidence and prop.
    const newProp = Identifier.new().toString()
    await new AmountIntervalClaimPatch({ type: "amountInterval", confidence: 0.5, prop: newProp }).Apply(claim)
    assert.equal(claim.confidence, 0.5)
    assert.equal(claim.prop.id, newProp)
  })
})

describe("TimeIntervalClaimPatch Apply branches", () => {
  test("setting from/fromPrecision, fromIsUnknown, fromIsNone, toIsUnknown, toIsNone, confidence+prop", async () => {
    const prop = Identifier.new().toString()
    const claim = new TimeIntervalClaim({
      id: Identifier.new().toString(),
      confidence: 1.0,
      prop: { id: prop },
      from: "2020",
      fromPrecision: "y",
      to: "2025",
      toPrecision: "y",
    })

    // Set from and fromPrecision.
    await new TimeIntervalClaimPatch({ type: "timeInterval", from: "2021-06-00", fromPrecision: "m" }).Apply(claim)
    assert.equal(claim.from, "2021-06-00")
    assert.equal(claim.fromPrecision, "m")

    // fromIsUnknown clears from and fromPrecision.
    await new TimeIntervalClaimPatch({ type: "timeInterval", fromIsUnknown: true }).Apply(claim)
    assert.equal(claim.fromIsUnknown, true)
    assert.equal(claim.from, undefined)
    assert.equal(claim.fromPrecision, undefined)

    // Restore from for next test.
    await new TimeIntervalClaimPatch({ type: "timeInterval", from: "2020", fromPrecision: "y", fromIsUnknown: false }).Apply(claim)
    assert.equal(claim.from, "2020")

    // fromIsNone clears from and fromPrecision.
    await new TimeIntervalClaimPatch({ type: "timeInterval", fromIsNone: true }).Apply(claim)
    assert.equal(claim.fromIsNone, true)
    assert.equal(claim.from, undefined)
    assert.equal(claim.fromPrecision, undefined)

    // toIsUnknown clears to and toPrecision.
    await new TimeIntervalClaimPatch({ type: "timeInterval", toIsUnknown: true }).Apply(claim)
    assert.equal(claim.toIsUnknown, true)
    assert.equal(claim.to, undefined)
    assert.equal(claim.toPrecision, undefined)

    // Restore to for next test.
    await new TimeIntervalClaimPatch({ type: "timeInterval", to: "2025", toPrecision: "y", toIsUnknown: false }).Apply(claim)
    assert.equal(claim.to, "2025")

    // toIsNone clears to and toPrecision.
    await new TimeIntervalClaimPatch({ type: "timeInterval", toIsNone: true }).Apply(claim)
    assert.equal(claim.toIsNone, true)
    assert.equal(claim.to, undefined)
    assert.equal(claim.toPrecision, undefined)

    // Update confidence and prop.
    const newProp = Identifier.new().toString()
    await new TimeIntervalClaimPatch({ type: "timeInterval", confidence: 0.5, prop: newProp }).Apply(claim)
    assert.equal(claim.confidence, 0.5)
    assert.equal(claim.prop.id, newProp)
  })
})

// Setting a concrete bound value must clear a previously set unknown or none marker on
// that bound. This is the production case where a "set" fills in a bound that was
// previously none: the merged claim must not end up with both a value and the marker,
// which Validate rejects.
describe("interval patch Apply clears markers when a value is set", () => {
  test("TimeIntervalClaimPatch setting to clears toIsNone", async () => {
    const claim = new TimeIntervalClaim({
      id: Identifier.new().toString(),
      confidence: 1.0,
      prop: { id: Identifier.new().toString() },
      from: "1984",
      fromPrecision: "y",
      toIsNone: true,
    })

    await new TimeIntervalClaimPatch({ type: "timeInterval", to: "1950", toPrecision: "y" }).Apply(claim)
    assert.equal(claim.to, "1950")
    assert.equal(claim.toPrecision, "y")
    assert.equal(claim.toIsNone, false)
  })

  test("AmountIntervalClaimPatch setting from clears fromIsUnknown", async () => {
    const claim = new AmountIntervalClaim({
      id: Identifier.new().toString(),
      confidence: 1.0,
      prop: { id: Identifier.new().toString() },
      to: "20",
      toPrecision: 1,
      fromIsUnknown: true,
    })

    await new AmountIntervalClaimPatch({ type: "amountInterval", from: "10", fromPrecision: 1 }).Apply(claim)
    assert.equal(claim.from, "10")
    assert.equal(claim.fromPrecision, 1)
    assert.equal(claim.fromIsUnknown, false)
  })

  test("TimeIntervalClaimPatch switching toIsUnknown to toIsNone clears toIsUnknown", async () => {
    const claim = new TimeIntervalClaim({
      id: Identifier.new().toString(),
      confidence: 1.0,
      prop: { id: Identifier.new().toString() },
      from: "1984",
      fromPrecision: "y",
      toIsUnknown: true,
    })

    await new TimeIntervalClaimPatch({ type: "timeInterval", toIsNone: true }).Apply(claim)
    assert.equal(claim.toIsNone, true)
    assert.equal(claim.toIsUnknown, false)
  })

  test("AmountIntervalClaimPatch switching toIsOpen to toIsUnknown clears toIsOpen and value", async () => {
    const claim = new AmountIntervalClaim({
      id: Identifier.new().toString(),
      confidence: 1.0,
      prop: { id: Identifier.new().toString() },
      from: "1.5",
      fromPrecision: 0.1,
      to: "9.5",
      toPrecision: 0.1,
      toIsOpen: true,
    })

    await new AmountIntervalClaimPatch({ type: "amountInterval", toIsUnknown: true }).Apply(claim)
    assert.equal(claim.toIsUnknown, true)
    assert.equal(claim.toIsOpen, false)
    assert.equal(claim.to, undefined)
    assert.equal(claim.toPrecision, undefined)
  })
})
