import { Identifier } from "@tozd/identifier"
import { assert, expect, test } from "vitest"

import { Changes, D, NoneClaim, StringClaim, UnknownClaim } from "@/document"

test("Document lifecycle", () => {
  const prop = Identifier.new().toString()

  const doc = new D({ id: Identifier.new().toString(), base: ["base"] })
  assert.equal(doc.Size(), 0)

  // Add a NoneClaim.
  const claimID = Identifier.new().toString()
  const claim = new NoneClaim({
    id: claimID,
    confidence: 1.0,
    prop: { id: prop },
  })
  doc.Add(claim)

  // GetByID finds it.
  const found = doc.GetByID(claimID)
  assert.equal(found?.GetID(), claimID)

  // Get by prop returns it.
  const got = doc.Get(prop)
  assert.equal(got.length, 1)
  assert.equal(got[0].GetID(), claimID)

  // RemoveByID removes it.
  const removed = doc.RemoveByID(claimID)
  assert.equal(removed?.GetID(), claimID)
  assert.equal(doc.Size(), 0)

  // Now test sub-claims on a claim.
  const metaClaimID = Identifier.new().toString()
  const metaClaim = new UnknownClaim({
    id: metaClaimID,
    confidence: 1.0,
    prop: { id: prop },
  })
  claim.Add(metaClaim)

  // Verify sub-claim is accessible via the claim.
  assert.equal(claim.GetByID(metaClaimID)?.GetID(), metaClaimID)

  // RemoveByID on sub-claim.
  const removedMeta = claim.RemoveByID(metaClaimID)
  assert.equal(removedMeta?.GetID(), metaClaimID)
  assert.equal(claim.Size(), 0)
})

test("patch json", async () => {
  const base = ["TqtRsbk7rTKviW3TJapTim"]
  // Change indices are 1-based.
  const id1 = (await Identifier.from(...base, "1")).toString()
  const id2 = (await Identifier.from(...base, "2")).toString()
  const prop1 = "XkbTJqwFCFkfoxMBXow4HU"
  const prop2 = "3EL2nZdWVbw85XG1zTH2o5"

  const changes = new Changes(
    {
      type: "add",
      id: id1,
      base: [...base, "1"],
      patch: {
        type: "amount",
        confidence: 1.0,
        prop: prop1,
        amount: "42.1",
        precision: 0.1,
      },
    },
    {
      type: "add",
      id: id2,
      base: [...base, "2"],
      under: id1,
      patch: {
        type: "id",
        confidence: 1.0,
        prop: prop2,
        value: "foobar",
      },
    },
  )

  const out = JSON.stringify(changes)
  assert.equal(
    out,
    `[{"type":"add","id":"${id1}","base":["TqtRsbk7rTKviW3TJapTim","1"],"patch":{"type":"amount","confidence":1,"prop":"XkbTJqwFCFkfoxMBXow4HU","amount":"42.1","precision":0.1}},{"type":"add","under":"${id1}","id":"${id2}","base":["TqtRsbk7rTKviW3TJapTim","2"],"patch":{"type":"id","confidence":1,"prop":"3EL2nZdWVbw85XG1zTH2o5","value":"foobar"}}]`,
  )

  const changes2 = new Changes(...(JSON.parse(out) as object[]))
  assert.deepEqual(changes, changes2)

  const id = Identifier.new().toString()
  const doc = new D({
    id: id,
    base: base,
  })
  await changes.Apply(doc)
  assert.deepEqual(
    new D({
      id: id,
      base: base,
      claims: {
        amount: [
          {
            id: id1,
            confidence: 1.0,
            sub: {
              id: [
                {
                  id: id2,
                  confidence: 1.0,
                  prop: {
                    id: prop2,
                  },
                  value: "foobar",
                },
              ],
            },
            prop: {
              id: prop1,
            },
            amount: "42.1",
            precision: 0.1,
          },
        ],
      },
    }),
    doc,
  )
})

test("Document GetByID in sub-claims", () => {
  const prop = Identifier.new().toString()
  const innerID = Identifier.new().toString()

  const doc = new D({ id: Identifier.new().toString(), base: ["base"] })

  const outerClaim = new NoneClaim({
    id: Identifier.new().toString(),
    confidence: 1.0,
    prop: { id: prop },
  })
  const innerClaim = new StringClaim({
    id: innerID,
    confidence: 1.0,
    prop: { id: prop },
    string: "nested",
  })
  outerClaim.Add(innerClaim)
  doc.Add(outerClaim)

  // GetByID should find the inner claim inside sub-claims.
  const found = doc.GetByID(innerID)
  assert.equal(found?.GetID(), innerID)
})

test("Document Remove", () => {
  const prop = Identifier.new().toString()

  const doc = new D({ id: Identifier.new().toString(), base: ["base"] })
  doc.Add(new StringClaim({ id: Identifier.new().toString(), confidence: 1.0, prop: { id: prop }, string: "a" }))
  doc.Add(new StringClaim({ id: Identifier.new().toString(), confidence: 0.5, prop: { id: prop }, string: "b" }))

  assert.equal(doc.Size(), 2)

  const removed = doc.Remove(prop)
  assert.equal(removed.length, 2)
  assert.equal(doc.Size(), 0)
})

test("Document Size and AllClaims", () => {
  const prop = Identifier.new().toString()

  const doc = new D({ id: Identifier.new().toString(), base: ["base"] })

  // Empty document.
  assert.equal(doc.Size(), 0)
  assert.deepEqual(doc.AllClaims(), [])

  // Add 2 claims.
  doc.Add(new StringClaim({ id: Identifier.new().toString(), confidence: 1.0, prop: { id: prop }, string: "x" }))
  doc.Add(new NoneClaim({ id: Identifier.new().toString(), confidence: 0.5, prop: { id: prop } }))

  assert.equal(doc.Size(), 2)
  assert.equal(doc.AllClaims().length, 2)
})

test("Document SizeWithSub", () => {
  const prop = Identifier.new().toString()

  const doc = new D({ id: Identifier.new().toString(), base: ["base"] })

  // One top-level claim carrying two sub-claims, the first with a further sub-claim.
  const deepSub = new StringClaim({ id: Identifier.new().toString(), confidence: 1.0, prop: { id: prop }, string: "deep" })
  const sub1 = new StringClaim({ id: Identifier.new().toString(), confidence: 1.0, prop: { id: prop }, string: "sub1" })
  sub1.Add(deepSub)
  const sub2 = new NoneClaim({ id: Identifier.new().toString(), confidence: 1.0, prop: { id: prop } })
  const top = new StringClaim({ id: Identifier.new().toString(), confidence: 1.0, prop: { id: prop }, string: "top" })
  top.Add(sub1)
  top.Add(sub2)
  doc.Add(top)

  // Shallow: only the single top-level claim.
  assert.equal(doc.Size(), 1)
  assert.equal(doc.AllClaims().length, 1)

  // Recursive: top + sub1 + deepSub + sub2.
  assert.equal(doc.SizeWithSub(), 4)
  assert.equal(doc.claims.AllClaimsWithSub().length, 4)
})

test("Document Validate", async () => {
  const base = ["test", "doc"]
  const id = (await Identifier.from(...base)).toString()
  const doc = new D({ id, base })

  // Valid ID.
  await doc.Validate()

  // Invalid ID.
  const badDoc = new D({ id: Identifier.new().toString(), base })
  await expect(badDoc.Validate()).rejects.toThrow("invalid ID")
})
