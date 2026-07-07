import { Identifier } from "@tozd/identifier"
import { assert, describe, test } from "vitest"

import type { FieldData } from "@/fields"

import {
  CARDINALITY,
  FIELD,
  FIELDS,
  HAS_PROPERTY,
  HAS_VALUE_TYPE,
  IN_LANGUAGE,
  NAME,
  ORDER_IN_LIST,
  SECTION,
  SUB_FIELD,
  VT_FILE,
  VT_HAS,
  VT_HTML,
  VT_LINK,
  VT_REFERENCE,
} from "@/core"
import { ClaimTypes, HighConfidence, LinkClaim } from "@/document"
import { extractFieldsFromClaims, fieldKey, getClaimsForField, getSectionName, hasFields, makeDefaultPatchForField, mergeFields } from "@/fields"

const propA = Identifier.new().toString()
const propB = Identifier.new().toString()
const propC = Identifier.new().toString()
const valueTypeString = Identifier.new().toString()
const valueTypeAmount = Identifier.new().toString()

// Helpers to build raw JSON claim objects (mirrors how data comes from the API).
function id(): string {
  return Identifier.new().toString()
}

function rawRef(prop: string, to: string): object {
  return { id: id(), confidence: HighConfidence, prop: { id: prop }, to: { id: to } }
}

function rawAmount(prop: string, amount: string): object {
  return { id: id(), confidence: HighConfidence, prop: { id: prop }, amount, precision: 1 }
}

function rawCardinality(from: string | null, to: string | null): object {
  const obj: Record<string, unknown> = { id: id(), confidence: HighConfidence, prop: { id: CARDINALITY } }
  if (from !== null) {
    obj.from = from
    obj.fromPrecision = 1
  } else {
    obj.fromIsNone = true
  }
  if (to !== null) {
    obj.to = to
    obj.toPrecision = 1
  } else {
    obj.toIsNone = true
  }
  return obj
}

// Build a raw FIELD has claim.
function rawField(opts: { propertyId: string; valueType: string; order: string; from?: string | null; to?: string | null; subFields?: object[] }): object {
  const sub: Record<string, object[]> = {
    ref: [rawRef(HAS_PROPERTY, opts.propertyId), rawRef(HAS_VALUE_TYPE, opts.valueType)],
    amount: [rawAmount(ORDER_IN_LIST, opts.order)],
    amountInterval: [rawCardinality(opts.from ?? "0", opts.to ?? null)],
  }
  if (opts.subFields && opts.subFields.length > 0) {
    sub.has = opts.subFields
  }
  return { id: id(), confidence: HighConfidence, prop: { id: FIELD }, sub }
}

// Build a raw SUB_FIELD has claim.
function rawSubField(opts: { propertyId: string; valueType: string; order: string; from?: string | null; to?: string | null }): object {
  return {
    id: id(),
    confidence: HighConfidence,
    prop: { id: SUB_FIELD },
    sub: {
      ref: [rawRef(HAS_PROPERTY, opts.propertyId), rawRef(HAS_VALUE_TYPE, opts.valueType)],
      amount: [rawAmount(ORDER_IN_LIST, opts.order)],
      amountInterval: [rawCardinality(opts.from ?? "0", opts.to ?? null)],
    },
  }
}

// A fake language document ID. It is not in siteContext.languageCodes, so name claims
// referencing it group under the undetermined language, which every fallback chain ends with.
const languageDoc = Identifier.new().toString()

function rawIdentifier(prop: string, value: string): object {
  return { id: id(), confidence: HighConfidence, prop: { id: prop }, value }
}

function rawString(prop: string, value: string): object {
  return { id: id(), confidence: HighConfidence, prop: { id: prop }, string: value, sub: { ref: [rawRef(IN_LANGUAGE, languageDoc)] } }
}

// Build a raw SECTION has claim. The NAME property holds both the section identifier (an
// identifier claim) and the translated display name (a string claim); here the display name
// is derived from the identifier as "<sectionId> name".
function rawSection(sectionId: string, order: string, fields: object[]): object {
  const sub: Record<string, object[]> = {
    id: [rawIdentifier(NAME, sectionId)],
    string: [rawString(NAME, `${sectionId} name`)],
    amount: [rawAmount(ORDER_IN_LIST, order)],
  }
  if (fields.length > 0) {
    sub.has = fields
  }
  return { id: id(), confidence: HighConfidence, prop: { id: SECTION }, sub }
}

// Build a ClaimTypes with a FIELDS has claim containing sections and top-level fields.
// Validates all claims to ensure structural consistency.
async function makeClaimsWithFields(sections: object[], fields: object[]): Promise<ClaimTypes> {
  const sub: Record<string, object[]> = {}
  const hasClaims: object[] = [...sections, ...fields]
  if (hasClaims.length > 0) {
    sub.has = hasClaims
  }
  const ct = new ClaimTypes({
    has: [{ id: id(), confidence: HighConfidence, prop: { id: FIELDS }, sub }],
  })
  await ct.Validate()
  return ct
}

// Shared fixtures for the field-identity and claim-matching tests below.
const userProp = Identifier.new().toString()
const selectionProp = Identifier.new().toString()
const extraProp = Identifier.new().toString()
const user1 = Identifier.new().toString()
const user2 = Identifier.new().toString()

// makeField builds a FieldData; sub-field paths nest under the parent's propertyId.
function makeField(propertyId: string, valueType: string, subFields: FieldData[] = []): FieldData {
  return {
    propertyId,
    valueType,
    orderInList: 3,
    minCardinality: 0,
    maxCardinality: Infinity,
    subFields,
    path: [propertyId],
  }
}

const userField = (): FieldData => makeField(userProp, VT_REFERENCE)
const selectionField = (): FieldData => makeField(userProp, VT_HAS, [makeField(selectionProp, VT_HAS)])
const extraField = (): FieldData => makeField(userProp, VT_HAS, [makeField(extraProp, VT_HAS)])

// rawHas builds a raw HAS claim, optionally carrying HAS sub-claims for the given properties.
function rawHas(prop: string, subProps: string[] = []): object {
  const obj: Record<string, unknown> = { id: id(), confidence: HighConfidence, prop: { id: prop } }
  if (subProps.length > 0) {
    obj.sub = { has: subProps.map((p) => ({ id: id(), confidence: HighConfidence, prop: { id: p } })) }
  }
  return obj
}

describe("hasFields", () => {
  test("returns false for null/undefined claims", () => {
    assert.equal(hasFields(null), false)
    assert.equal(hasFields(undefined), false)
  })

  test("returns false for empty claims", () => {
    assert.equal(hasFields(new ClaimTypes({})), false)
  })

  test("returns false for FIELDS claim with no sections or fields", () => {
    const ct = new ClaimTypes({
      has: [{ id: id(), confidence: HighConfidence, prop: { id: FIELDS } }],
    })
    assert.equal(hasFields(ct), false)
  })

  test("returns true when FIELDS has a FIELD sub-claim", async () => {
    const ct = await makeClaimsWithFields([], [rawField({ propertyId: propA, valueType: valueTypeString, order: "1" })])
    assert.equal(hasFields(ct), true)
  })

  test("returns true when FIELDS has a SECTION sub-claim", async () => {
    const section = rawSection("basics", "1", [rawField({ propertyId: propA, valueType: valueTypeString, order: "1" })])
    const ct = await makeClaimsWithFields([section], [])
    assert.equal(hasFields(ct), true)
  })
})

describe("extractFieldsFromClaims", () => {
  test("returns null for null/undefined claims", () => {
    assert.equal(extractFieldsFromClaims(null), null)
    assert.equal(extractFieldsFromClaims(undefined), null)
  })

  test("returns null when no FIELDS claim exists", () => {
    assert.equal(extractFieldsFromClaims(new ClaimTypes({})), null)
  })

  test("extracts top-level fields sorted by orderInList", async () => {
    const ct = await makeClaimsWithFields(
      [],
      [rawField({ propertyId: propB, valueType: valueTypeString, order: "2" }), rawField({ propertyId: propA, valueType: valueTypeAmount, order: "1" })],
    )
    const result = extractFieldsFromClaims(ct)
    assert.notEqual(result, null)
    assert.equal(result!.sections.length, 0)
    assert.equal(result!.fields.length, 2)
    // Sorted by order.
    assert.equal(result!.fields[0].propertyId, propA)
    assert.equal(result!.fields[0].orderInList, 1)
    assert.equal(result!.fields[0].valueType, valueTypeAmount)
    assert.equal(result!.fields[1].propertyId, propB)
    assert.equal(result!.fields[1].orderInList, 2)
  })

  test("marks sibling LINK and FILE fields sharing a propertyId", async () => {
    const ct = await makeClaimsWithFields(
      [],
      [
        rawField({ propertyId: propA, valueType: VT_LINK, order: "1" }),
        rawField({ propertyId: propA, valueType: VT_FILE, order: "2" }),
        rawField({ propertyId: propB, valueType: VT_LINK, order: "3" }),
      ],
    )
    const result = extractFieldsFromClaims(ct)
    assert.notEqual(result, null)
    assert.equal(result!.fields.length, 3)
    assert.equal(result!.fields[0].fileLinkSibling, true)
    assert.equal(result!.fields[1].fileLinkSibling, true)
    assert.equal(result!.fields[2].fileLinkSibling, undefined)
  })

  test("extracts sections with fields sorted by orderInList", async () => {
    const ct = await makeClaimsWithFields(
      [
        rawSection("second", "2", [rawField({ propertyId: propB, valueType: valueTypeString, order: "1" })]),
        rawSection("first", "1", [rawField({ propertyId: propA, valueType: valueTypeString, order: "1" })]),
      ],
      [],
    )
    const result = extractFieldsFromClaims(ct)
    assert.notEqual(result, null)
    assert.equal(result!.fields.length, 0)
    assert.equal(result!.sections.length, 2)
    // Sections sorted by order; the display name is picked by language (here via the
    // undetermined-language fallback, since the test language document is unrecognized).
    assert.equal(result!.sections[0].id, "first")
    assert.equal(getSectionName(result!.sections[0], "en-US"), "first name")
    assert.equal(result!.sections[0].orderInList, 1)
    assert.equal(result!.sections[1].id, "second")
    assert.equal(getSectionName(result!.sections[1], "en-US"), "second name")
    assert.equal(result!.sections[1].orderInList, 2)
  })

  test("section name falls back to the identifier when no name matches", () => {
    assert.equal(getSectionName({ id: "plain", orderInList: 1, fields: [] }, "en-US"), "plain")
  })

  test("extracts cardinality from amount interval claim", async () => {
    const ct = await makeClaimsWithFields([], [rawField({ propertyId: propA, valueType: valueTypeString, order: "1", from: "1", to: "5" })])
    const result = extractFieldsFromClaims(ct)
    assert.notEqual(result, null)
    assert.equal(result!.fields[0].minCardinality, 1)
    assert.equal(result!.fields[0].maxCardinality, 5)
  })

  test("unbounded cardinality when to is absent", async () => {
    const ct = await makeClaimsWithFields([], [rawField({ propertyId: propA, valueType: valueTypeString, order: "1", from: "1", to: null })])
    const result = extractFieldsFromClaims(ct)
    assert.notEqual(result, null)
    assert.equal(result!.fields[0].minCardinality, 1)
    assert.equal(result!.fields[0].maxCardinality, Infinity)
  })

  test("extracts sub-fields", async () => {
    const subField = rawSubField({ propertyId: propB, valueType: valueTypeString, order: "1" })
    const ct = await makeClaimsWithFields([], [rawField({ propertyId: propA, valueType: valueTypeString, order: "1", subFields: [subField] })])
    const result = extractFieldsFromClaims(ct)
    assert.notEqual(result, null)
    assert.equal(result!.fields[0].subFields.length, 1)
    assert.equal(result!.fields[0].subFields[0].propertyId, propB)
  })

  test("extracts both sections and top-level fields", async () => {
    const section = rawSection("my-section", "1", [rawField({ propertyId: propA, valueType: valueTypeString, order: "1" })])
    const ct = await makeClaimsWithFields([section], [rawField({ propertyId: propB, valueType: valueTypeString, order: "2" })])
    const result = extractFieldsFromClaims(ct)
    assert.notEqual(result, null)
    assert.equal(result!.sections.length, 1)
    assert.equal(result!.sections[0].fields.length, 1)
    assert.equal(result!.sections[0].fields[0].propertyId, propA)
    assert.equal(result!.fields.length, 1)
    assert.equal(result!.fields[0].propertyId, propB)
  })

  test("fields within a section are sorted by orderInList", async () => {
    const section = rawSection("s", "1", [
      rawField({ propertyId: propC, valueType: valueTypeString, order: "3" }),
      rawField({ propertyId: propA, valueType: valueTypeString, order: "1" }),
      rawField({ propertyId: propB, valueType: valueTypeString, order: "2" }),
    ])
    const ct = await makeClaimsWithFields([section], [])
    const result = extractFieldsFromClaims(ct)
    assert.notEqual(result, null)
    const fields = result!.sections[0].fields
    assert.equal(fields[0].propertyId, propA)
    assert.equal(fields[1].propertyId, propB)
    assert.equal(fields[2].propertyId, propC)
  })

  test("default cardinality when no interval claim", () => {
    // Build a field without a cardinality claim.
    const ct = new ClaimTypes({
      has: [
        {
          id: id(),
          confidence: HighConfidence,
          prop: { id: FIELDS },
          sub: {
            has: [
              {
                id: id(),
                confidence: HighConfidence,
                prop: { id: FIELD },
                sub: {
                  ref: [rawRef(HAS_PROPERTY, propA), rawRef(HAS_VALUE_TYPE, valueTypeString)],
                  amount: [rawAmount(ORDER_IN_LIST, "1")],
                },
              },
            ],
          },
        },
      ],
    })
    const result = extractFieldsFromClaims(ct)
    assert.notEqual(result, null)
    assert.equal(result!.fields[0].minCardinality, 0)
    assert.equal(result!.fields[0].maxCardinality, Infinity)
  })

  test("returns null for field missing HAS_PROPERTY", () => {
    const ct = new ClaimTypes({
      has: [
        {
          id: id(),
          confidence: HighConfidence,
          prop: { id: FIELDS },
          sub: {
            has: [
              {
                id: id(),
                confidence: HighConfidence,
                prop: { id: FIELD },
                sub: {
                  // Missing HAS_PROPERTY ref.
                  ref: [rawRef(HAS_VALUE_TYPE, valueTypeString)],
                  amount: [rawAmount(ORDER_IN_LIST, "1")],
                },
              },
            ],
          },
        },
      ],
    })
    const result = extractFieldsFromClaims(ct)
    assert.notEqual(result, null)
    // Field is skipped because HAS_PROPERTY is missing.
    assert.equal(result!.fields.length, 0)
  })
})

describe("mergeFields", () => {
  test("empty input returns empty result", () => {
    const result = mergeFields([])
    assert.equal(result.sections.length, 0)
    assert.equal(result.fields.length, 0)
  })

  test("single FieldsData passes through", () => {
    const input = {
      sections: [
        {
          id: "s",
          orderInList: 1,
          fields: [{ propertyId: propA, valueType: valueTypeString, orderInList: 1, minCardinality: 0, maxCardinality: Infinity, subFields: [], path: [propA] }],
        },
      ],
      fields: [{ propertyId: propB, valueType: valueTypeString, orderInList: 2, minCardinality: 0, maxCardinality: Infinity, subFields: [], path: [propB] }],
    }
    const result = mergeFields([input])
    assert.equal(result.sections.length, 1)
    assert.equal(result.sections[0].fields[0].propertyId, propA)
    assert.equal(result.fields.length, 1)
    assert.equal(result.fields[0].propertyId, propB)
  })

  test("deduplicates top-level fields by propertyId", () => {
    const f = (prop: string, order: number) => ({
      propertyId: prop,
      valueType: valueTypeString,
      orderInList: order,
      minCardinality: 0,
      maxCardinality: Infinity,
      subFields: [],
      path: [prop],
    })
    const result = mergeFields([
      { sections: [], fields: [f(propA, 1), f(propB, 2)] },
      { sections: [], fields: [f(propA, 1), f(propC, 3)] },
    ])
    assert.equal(result.fields.length, 3)
    assert.equal(result.fields[0].propertyId, propA)
    assert.equal(result.fields[1].propertyId, propB)
    assert.equal(result.fields[2].propertyId, propC)
  })

  test("merges sections with the same id", () => {
    const f = (prop: string, order: number) => ({
      propertyId: prop,
      valueType: valueTypeString,
      orderInList: order,
      minCardinality: 0,
      maxCardinality: Infinity,
      subFields: [],
      path: [prop],
    })
    const result = mergeFields([
      { sections: [{ id: "basics", orderInList: 1, fields: [f(propA, 1)] }], fields: [] },
      { sections: [{ id: "basics", orderInList: 1, fields: [f(propB, 2)] }], fields: [] },
    ])
    assert.equal(result.sections.length, 1)
    assert.equal(result.sections[0].id, "basics")
    assert.equal(result.sections[0].fields.length, 2)
    assert.equal(result.sections[0].fields[0].propertyId, propA)
    assert.equal(result.sections[0].fields[1].propertyId, propB)
  })

  test("keeps sections with different ids separate", () => {
    const f = (prop: string) => ({
      propertyId: prop,
      valueType: valueTypeString,
      orderInList: 1,
      minCardinality: 0,
      maxCardinality: Infinity,
      subFields: [],
      path: [prop],
    })
    const result = mergeFields([
      { sections: [{ id: "alpha", orderInList: 1, fields: [f(propA)] }], fields: [] },
      { sections: [{ id: "beta", orderInList: 2, fields: [f(propB)] }], fields: [] },
    ])
    assert.equal(result.sections.length, 2)
    assert.equal(result.sections[0].id, "alpha")
    assert.equal(result.sections[1].id, "beta")
  })

  test("deduplicates across sections and top-level fields", () => {
    const f = (prop: string) => ({
      propertyId: prop,
      valueType: valueTypeString,
      orderInList: 1,
      minCardinality: 0,
      maxCardinality: Infinity,
      subFields: [],
      path: [prop],
    })
    const result = mergeFields([
      { sections: [{ id: "s", orderInList: 1, fields: [f(propA)] }], fields: [] },
      { sections: [], fields: [f(propA)] },
    ])
    // propA already seen in section, so top-level duplicate is skipped.
    assert.equal(result.sections[0].fields.length, 1)
    assert.equal(result.fields.length, 0)
  })

  test("keeps sibling fields that share a propertyId but differ in value type or sub-fields", () => {
    const result = mergeFields([{ sections: [], fields: [userField(), selectionField(), extraField()] }])
    assert.equal(result.fields.length, 3)
  })

  test("still deduplicates the same field declared by multiple classes", () => {
    const result = mergeFields([
      { sections: [], fields: [selectionField()] },
      { sections: [], fields: [selectionField()] },
    ])
    assert.equal(result.fields.length, 1)
  })

  test("marks sibling LINK and FILE fields sharing a propertyId, also across classes", () => {
    const result = mergeFields([
      { sections: [], fields: [makeField(propA, VT_LINK)] },
      { sections: [], fields: [makeField(propA, VT_FILE), makeField(propB, VT_LINK)] },
    ])
    assert.equal(result.fields.length, 3)
    const marked = result.fields.filter((f) => f.fileLinkSibling)
    assert.equal(marked.length, 2)
    assert.ok(marked.every((f) => f.propertyId === propA))
  })
})

describe("fieldKey", () => {
  test("distinguishes sibling fields that share a propertyId", () => {
    const keys = new Set([fieldKey(userField()), fieldKey(selectionField()), fieldKey(extraField())])
    assert.equal(keys.size, 3)
  })

  test("is stable for structurally identical fields", () => {
    assert.equal(fieldKey(selectionField()), fieldKey(selectionField()))
  })
})

describe("getClaimsForField", () => {
  function claims(): ClaimTypes {
    return new ClaimTypes({
      ref: [rawRef(userProp, user1), rawRef(userProp, user2)],
      has: [rawHas(userProp, [selectionProp]), rawHas(userProp, [extraProp])],
    })
  }

  test("a relation field returns all relation claims", () => {
    assert.equal(getClaimsForField(claims(), userField()).length, 2)
  })

  test("a HAS meta field returns only claims carrying its sub-field", () => {
    const sel = getClaimsForField(claims(), selectionField())
    assert.equal(sel.length, 1)
    assert.equal(sel[0].Get(selectionProp).length, 1)
    assert.equal(sel[0].Get(extraProp).length, 0)

    const wok = getClaimsForField(claims(), extraField())
    assert.equal(wok.length, 1)
    assert.equal(wok[0].Get(extraProp).length, 1)
  })

  test("the SELECTION field does not match a document that only has EXTRA", () => {
    const onlyWeOnlyKnow = new ClaimTypes({ has: [rawHas(userProp, [extraProp])] })
    assert.equal(getClaimsForField(onlyWeOnlyKnow, selectionField()).length, 0)
    assert.equal(getClaimsForField(onlyWeOnlyKnow, extraField()).length, 1)
  })

  test("a HAS field without sub-fields is not filtered", () => {
    assert.equal(getClaimsForField(claims(), makeField(userProp, VT_HAS)).length, 2)
  })

  test("returns empty for null claims", () => {
    assert.equal(getClaimsForField(null, selectionField()).length, 0)
  })
})

describe("getClaimsForField sibling LINK and FILE fields", () => {
  const imageProp = Identifier.new().toString()
  const fileIri = "https://example.com/f/abc"
  const linkIri = "https://example.com/image.jpg"
  const isFileLink = (iri: string) => iri === fileIri

  function rawLink(prop: string, iri: string): object {
    return { id: id(), confidence: HighConfidence, prop: { id: prop }, iri }
  }

  function imageClaims(): ClaimTypes {
    return new ClaimTypes({ link: [rawLink(imageProp, fileIri), rawLink(imageProp, linkIri)] })
  }

  test("routes file links to the FILE field and other links to the LINK field", () => {
    const linkField: FieldData = { ...makeField(imageProp, VT_LINK), fileLinkSibling: true }
    const fileField: FieldData = { ...makeField(imageProp, VT_FILE), fileLinkSibling: true }

    const linkClaims = getClaimsForField(imageClaims(), linkField, isFileLink)
    assert.equal(linkClaims.length, 1)
    assert.equal((linkClaims[0] as LinkClaim).iri, linkIri)

    const fileClaims = getClaimsForField(imageClaims(), fileField, isFileLink)
    assert.equal(fileClaims.length, 1)
    assert.equal((fileClaims[0] as LinkClaim).iri, fileIri)
  })

  test("does not route for fields without the sibling flag", () => {
    assert.equal(getClaimsForField(imageClaims(), makeField(imageProp, VT_LINK), isFileLink).length, 2)
    assert.equal(getClaimsForField(imageClaims(), makeField(imageProp, VT_FILE), isFileLink).length, 2)
  })

  test("does not route without the isFileLink predicate", () => {
    const linkField: FieldData = { ...makeField(imageProp, VT_LINK), fileLinkSibling: true }
    assert.equal(getClaimsForField(imageClaims(), linkField).length, 2)
  })
})

describe("getClaimsForField default fields", () => {
  test("a value field with default:unknown also matches unknown claims carrying its sub-field", () => {
    const studioProp = Identifier.new().toString()
    const notesProp = Identifier.new().toString()
    const claims = new ClaimTypes({
      // A known-location studio.
      ref: [rawRef(studioProp, Identifier.new().toString())],
      unknown: [
        // An unknown-location studio that still has a notes sub-claim.
        {
          id: id(),
          confidence: HighConfidence,
          prop: { id: studioProp },
          sub: { html: [{ id: id(), confidence: HighConfidence, prop: { id: notesProp }, html: "<p>n</p>" }] },
        },
        // A bare unknown claim with no matching sub-field is excluded.
        { id: id(), confidence: HighConfidence, prop: { id: studioProp } },
      ],
    })
    const studioField: FieldData = { ...makeField(studioProp, VT_REFERENCE, [makeField(notesProp, VT_HTML)]), default: "unknown" }
    // The ref (known) claim plus the unknown claim with notes; the bare unknown is excluded.
    assert.equal(getClaimsForField(claims, studioField).length, 2)
  })

  test("a value field without a default does not match unknown/none claims", () => {
    const prop = Identifier.new().toString()
    const claims = new ClaimTypes({
      ref: [rawRef(prop, Identifier.new().toString())],
      unknown: [{ id: id(), confidence: HighConfidence, prop: { id: prop } }],
    })
    assert.equal(getClaimsForField(claims, makeField(prop, VT_REFERENCE)).length, 1)
  })
})

describe("makeDefaultPatchForField", () => {
  test("builds a none/unknown patch for a field with a default", () => {
    const prop = Identifier.new().toString()
    const noneField: FieldData = { ...makeField(prop, VT_REFERENCE), default: "none" }
    assert.deepEqual(makeDefaultPatchForField(noneField), { type: "none", confidence: HighConfidence, prop })
    const unknownField: FieldData = { ...makeField(prop, VT_REFERENCE), default: "unknown" }
    assert.deepEqual(makeDefaultPatchForField(unknownField), { type: "unknown", confidence: HighConfidence, prop })
  })

  test("throws for a field without a default", () => {
    assert.throws(() => makeDefaultPatchForField(makeField(Identifier.new().toString(), VT_REFERENCE)))
  })
})
