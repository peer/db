import type { DeepReadonly, InjectionKey } from "vue"

import type { Claim, ClaimTypeName, TimePrecision } from "@/document"

import {
  CARDINALITY,
  FIELD,
  FIELD_VALUES,
  FIELDS,
  HAS_PROPERTY,
  HAS_VALUE_TYPE,
  NAME,
  ORDER_IN_LIST,
  SECTION,
  SUB_FIELD,
  VT_AMOUNT,
  VT_AMOUNT_INTERVAL,
  VT_FILE,
  VT_HAS,
  VT_HTML,
  VT_IDENTIFIER,
  VT_LINK,
  VT_NONE,
  VT_REFERENCE,
  VT_STRING,
  VT_TIME,
  VT_TIME_INTERVAL,
  VT_UNKNOWN,
} from "@/core"
import { ABSTRACT_CLASS } from "@/core/properties"
import {
  AmountClaim,
  AmountIntervalClaim,
  ClaimTypes,
  getBestClaimOfType,
  getClaimsOfTypeWithConfidence,
  HasClaim,
  HighConfidence,
  HTMLClaim,
  IdentifierClaim,
  LinkClaim,
  NoneClaim,
  ReferenceClaim,
  StringClaim,
  TimeClaim,
  TimeIntervalClaim,
  UnknownClaim,
} from "@/document"

// FieldData represents a single field.
export interface FieldData {
  // Property document ID for this field.
  propertyId: string
  // Value type document ID (determines claim type).
  valueType: string
  // Numeric order for sorting.
  orderInList: number
  // Minimum number of values (0 means optional).
  minCardinality: number
  // Maximum number of values (Infinity means unlimited).
  maxCardinality: number
  // Nested sub-fields.
  subFields: readonly FieldData[]
  // Path from the root field to this field, using property IDs.
  // Top-level fields have a single-element path. Sub-fields have
  // [parentPropertyId, ..., thisPropertyId]. Used as a unique key
  // to distinguish sub-fields with the same propertyId under different parents.
  path: readonly string[]
  // Highest-confidence FIELD_VALUES search shortcut string, if any. Consumed
  // by InputRef as a filter that constrains which documents may be picked.
  values?: string
}

// fieldKey returns a unique string key for a field, derived from its path.
export function fieldKey(field: FieldData): string {
  return field.path.join("/")
}

// SectionData represents a section of fields with an identifier and ordering.
export interface SectionData {
  // Section identifier.
  id: string
  // Numeric order for sorting.
  orderInList: number
  // Fields within this section.
  fields: readonly FieldData[]
}

// FieldsData represents all fields and sections.
export interface FieldsData {
  // Named sections containing fields.
  sections: readonly SectionData[]
  // Top-level fields not in any section.
  fields: readonly FieldData[]
}

// extractFieldData extracts FieldData from claims. parentPath is the path from the root.
function extractFieldData(claimsTypes: DeepReadonly<ClaimTypes> | undefined, parentPath: string[]): FieldData | null {
  if (!claimsTypes) {
    return null
  }

  const propRef = getBestClaimOfType(claimsTypes, "ref", HAS_PROPERTY)
  const valueTypeRef = getBestClaimOfType(claimsTypes, "ref", HAS_VALUE_TYPE)
  const orderClaim = getBestClaimOfType(claimsTypes, "amount", ORDER_IN_LIST)
  const cardinalityClaim = getBestClaimOfType(claimsTypes, "amountInterval", CARDINALITY)

  if (!propRef || !valueTypeRef) {
    return null
  }

  const thisPath = [...parentPath, propRef.to.id]

  let minCardinality = 0
  let maxCardinality = Infinity
  if (cardinalityClaim) {
    if (cardinalityClaim.from) {
      minCardinality = parseFloat(cardinalityClaim.from)
      if (isNaN(minCardinality)) {
        throw Error(`invalid min cardinality: ${cardinalityClaim.from}`)
      }
    }
    if (cardinalityClaim.to) {
      maxCardinality = parseFloat(cardinalityClaim.to)
      if (isNaN(maxCardinality)) {
        throw Error(`invalid max cardinality: ${cardinalityClaim.to}`)
      }
    }
  }

  const subFieldClaims = getClaimsOfTypeWithConfidence(claimsTypes, "has", SUB_FIELD)
  const subFields: FieldData[] = []
  for (const subFieldClaim of subFieldClaims) {
    const subData = extractFieldData(subFieldClaim.sub, thisPath)
    if (subData) {
      subFields.push(subData)
    }
  }
  subFields.sort((a, b) => a.orderInList - b.orderInList)

  const valueClaim = getBestClaimOfType(claimsTypes, "string", FIELD_VALUES)

  return {
    propertyId: propRef.to.id,
    valueType: valueTypeRef.to.id,
    orderInList: orderClaim ? parseFloat(orderClaim.amount) || 0 : 0,
    minCardinality,
    maxCardinality,
    subFields,
    path: thisPath,
    values: valueClaim?.string || undefined,
  }
}

// extractFieldsFromClaims extracts FieldsData from a class document's claims.
export function extractFieldsFromClaims(claims: DeepReadonly<ClaimTypes> | undefined | null): FieldsData | null {
  if (!claims) {
    return null
  }

  // Use the first (highest confidence) FIELDS claim.
  const fieldsClaim = getBestClaimOfType(claims, "has", FIELDS)
  if (!fieldsClaim) {
    return null
  }

  const sections: SectionData[] = []
  const fields: FieldData[] = []

  // Extract sections.
  const sectionClaims = getClaimsOfTypeWithConfidence(fieldsClaim.sub, "has", SECTION)
  for (const sectionClaim of sectionClaims) {
    const idClaim = getBestClaimOfType(sectionClaim.sub, "id", NAME)
    const sectionId = idClaim ? idClaim.value : ""
    const orderClaim = getBestClaimOfType(sectionClaim.sub, "amount", ORDER_IN_LIST)

    const sectionFields: FieldData[] = []
    const fieldClaims = getClaimsOfTypeWithConfidence(sectionClaim.sub, "has", FIELD)
    for (const fieldClaim of fieldClaims) {
      const field = extractFieldData(fieldClaim.sub, [])
      if (field) {
        sectionFields.push(field)
      }
    }
    sectionFields.sort((a, b) => a.orderInList - b.orderInList)

    sections.push({
      id: sectionId,
      orderInList: orderClaim ? parseFloat(orderClaim.amount) || 0 : 0,
      fields: sectionFields,
    })
  }

  // Extract top-level fields.
  const fieldClaims = getClaimsOfTypeWithConfidence(fieldsClaim.sub, "has", FIELD)
  for (const fieldClaim of fieldClaims) {
    const field = extractFieldData(fieldClaim.sub, [])
    if (field) {
      fields.push(field)
    }
  }

  sections.sort((a, b) => a.orderInList - b.orderInList)
  fields.sort((a, b) => a.orderInList - b.orderInList)

  return { sections, fields }
}

// mergeFields merges multiple FieldsData into a single union, deduplicating by property ID.
export function mergeFields(allFields: FieldsData[]): FieldsData {
  const seenProperties = new Set<string>()
  const mergedSections: SectionData[] = []
  const mergedFields: FieldData[] = []

  for (const fieldsData of allFields) {
    for (const section of fieldsData.sections) {
      // Deduplicate fields within sections.
      const newFields: FieldData[] = []
      for (const field of section.fields) {
        if (!seenProperties.has(field.propertyId)) {
          seenProperties.add(field.propertyId)
          newFields.push(field)
        } else {
          // TODO: Do something better?
          console.error("duplicate field", field.propertyId)
        }
      }
      if (newFields.length > 0) {
        // Check if we already have a section with the same ID.
        const existingIdx = mergedSections.findIndex((s) => s.id === section.id)
        if (existingIdx >= 0) {
          mergedSections[existingIdx] = {
            ...mergedSections[existingIdx],
            fields: [...mergedSections[existingIdx].fields, ...newFields].sort((a, b) => a.orderInList - b.orderInList),
          }
        } else {
          mergedSections.push({ ...section, fields: newFields })
        }
      }
    }

    for (const field of fieldsData.fields) {
      if (!seenProperties.has(field.propertyId)) {
        seenProperties.add(field.propertyId)
        mergedFields.push(field)
      }
    }
  }

  mergedSections.sort((a, b) => a.orderInList - b.orderInList)
  mergedFields.sort((a, b) => a.orderInList - b.orderInList)

  return { sections: mergedSections, fields: mergedFields }
}

// hasFields checks if claims have any FIELDS claims with actual field data.
export function hasFields(claims: DeepReadonly<ClaimTypes> | undefined | null): boolean {
  if (!claims) {
    return false
  }
  const fieldsClaims = getClaimsOfTypeWithConfidence(claims, "has", FIELDS)
  if (fieldsClaims.length === 0) {
    return false
  }
  // Check that there's at least one FIELD or SECTION.
  const fieldsClaim = fieldsClaims[0]
  const fieldCount = getClaimsOfTypeWithConfidence(fieldsClaim.sub, "has", FIELD).length
  const sectionCount = getClaimsOfTypeWithConfidence(fieldsClaim.sub, "has", SECTION).length
  return fieldCount > 0 || sectionCount > 0
}

// isAbstractClass checks if claims have an ABSTRACT_CLASS claim.
export function isAbstractClass(claims: DeepReadonly<ClaimTypes> | undefined | null): boolean {
  if (!claims) {
    return false
  }
  return getClaimsOfTypeWithConfidence(claims, "has", ABSTRACT_CLASS).length > 0
}

// VALUE_TYPE_TO_CLAIM_TYPE maps value type document IDs to claim type names.
const VALUE_TYPE_TO_CLAIM_TYPE: Record<string, ClaimTypeName> = {
  [VT_IDENTIFIER]: "id",
  [VT_STRING]: "string",
  [VT_HTML]: "html",
  [VT_AMOUNT]: "amount",
  [VT_AMOUNT_INTERVAL]: "amountInterval",
  [VT_TIME]: "time",
  [VT_TIME_INTERVAL]: "timeInterval",
  [VT_LINK]: "link",
  [VT_FILE]: "link",
  [VT_REFERENCE]: "ref",
  [VT_HAS]: "has",
  [VT_NONE]: "none",
  [VT_UNKNOWN]: "unknown",
}

// valueTypeToClaimType maps a value type document ID to the corresponding claim type name.
export function valueTypeToClaimType(valueTypeId: string): ClaimTypeName {
  const claimType = VALUE_TYPE_TO_CLAIM_TYPE[valueTypeId]
  if (claimType) {
    return claimType
  }
  throw new Error(`unsupported value type: ${valueTypeId}`)
}

// FieldsFormSaveChange is a fully constructed change object emitted by FieldsForm, ready to be posted.
export interface FieldsFormSaveChange {
  change: object
  changeNumber: number
}

// FlushFn is a function that flushes pending changes from a FieldsForm instance.
export type FlushFn = () => Promise<FieldsFormSaveChange[]>

// Injection keys for FieldsForm shared services (using Symbol.for for deduplication in dev).
// See progress.ts for the pattern.
export const getNextChangeNumberKey: InjectionKey<() => number> = process.env.NODE_ENV !== "production" ? Symbol.for("peerdb-getNextChangeNumber") : Symbol()
export const saveChangeKey: InjectionKey<(change: object, changeNumber: number) => Promise<void>> =
  process.env.NODE_ENV !== "production" ? Symbol.for("peerdb-saveChange") : Symbol()
export const registerForFlushKey: InjectionKey<(instance: FlushFn) => void> = process.env.NODE_ENV !== "production" ? Symbol.for("peerdb-registerForFlush") : Symbol()
export const unregisterForFlushKey: InjectionKey<(instance: FlushFn) => void> = process.env.NODE_ENV !== "production" ? Symbol.for("peerdb-unregisterForFlush") : Symbol()

// fieldLabelCellKey provides the field's label cell element. ClaimInput's
// focusout handler uses it to skip the per-slot commit when focus is on
// its way to a control inside the label cell (the field-level Revert
// button). Without that skip, the commit's async saveChange races with
// the Revert click and Revert sees stale state on the first click.
export const fieldLabelCellKey: InjectionKey<() => HTMLElement | null> = process.env.NODE_ENV !== "production" ? Symbol.for("peerdb-fieldLabelCell") : Symbol()

// FieldEntryValue is the per-entry state edited by FieldsForm. The fields
// are wide enough to cover every claim type the form handles - non-relevant
// fields stay at their default values. Intervals split into "from" (value/
// amountPrecision/timePrecision) and "to" (valueTo/amountPrecisionTo/
// timePrecisionTo); the missing-state flags pair with each side and are
// kept mutually exclusive by the InputMissing wrapper that drives them.
export interface FieldEntryValue {
  value: string
  valueTo: string
  amountPrecision: string
  amountPrecisionTo: string
  timePrecision: TimePrecision
  timePrecisionTo: TimePrecision
  fromUnknown: boolean
  fromNone: boolean
  toUnknown: boolean
  toNone: boolean
}

// emptyFieldEntryValue returns a fresh FieldEntryValue for a new (blank) entry.
export function emptyFieldEntryValue(): FieldEntryValue {
  return {
    value: "",
    valueTo: "",
    amountPrecision: "",
    amountPrecisionTo: "",
    timePrecision: "y",
    timePrecisionTo: "y",
    fromUnknown: false,
    fromNone: false,
    toUnknown: false,
    toNone: false,
  }
}

// ExistingClaimValue is FieldEntryValue with the claim's ID attached.
export interface ExistingClaimValue extends FieldEntryValue {
  claimId: string
}

// getClaimValues extracts a FieldEntryValue from an existing claim.
export function getClaimValues(claim: DeepReadonly<Claim>): FieldEntryValue {
  const v = emptyFieldEntryValue()
  if (claim instanceof IdentifierClaim) {
    v.value = claim.value
    return v
  }
  if (claim instanceof StringClaim) {
    v.value = claim.string
    return v
  }
  if (claim instanceof HTMLClaim) {
    v.value = claim.html
    return v
  }
  if (claim instanceof AmountClaim) {
    v.value = claim.amount
    v.amountPrecision = String(claim.precision)
    return v
  }
  if (claim instanceof AmountIntervalClaim) {
    v.value = claim.from ?? ""
    v.valueTo = claim.to ?? ""
    v.amountPrecision = claim.fromPrecision !== undefined ? String(claim.fromPrecision) : ""
    v.amountPrecisionTo = claim.toPrecision !== undefined ? String(claim.toPrecision) : ""
    v.fromUnknown = !!claim.fromIsUnknown
    v.fromNone = !!claim.fromIsNone
    v.toUnknown = !!claim.toIsUnknown
    v.toNone = !!claim.toIsNone
    return v
  }
  if (claim instanceof TimeClaim) {
    v.value = claim.time
    v.timePrecision = claim.precision
    return v
  }
  if (claim instanceof TimeIntervalClaim) {
    v.value = claim.from ?? ""
    v.valueTo = claim.to ?? ""
    v.timePrecision = claim.fromPrecision ?? "y"
    v.timePrecisionTo = claim.toPrecision ?? "y"
    v.fromUnknown = !!claim.fromIsUnknown
    v.fromNone = !!claim.fromIsNone
    v.toUnknown = !!claim.toIsUnknown
    v.toNone = !!claim.toIsNone
    return v
  }
  if (claim instanceof LinkClaim) {
    v.value = claim.iri
    return v
  }
  if (claim instanceof ReferenceClaim) {
    v.value = claim.to.id
    return v
  }
  if (claim instanceof HasClaim || claim instanceof NoneClaim || claim instanceof UnknownClaim) {
    return v
  }
  throw new Error("unsupported claim type")
}

// equalFieldEntryValue returns true if two FieldEntryValues are materially
// equivalent (same value, same precision, same missing-state flags).
// Used by FieldsFormField to detect whether a property has any session-level
// modification relative to its pre-session baseline.
export function equalFieldEntryValue(a: FieldEntryValue, b: FieldEntryValue): boolean {
  return (
    a.value === b.value &&
    a.valueTo === b.valueTo &&
    a.amountPrecision === b.amountPrecision &&
    a.amountPrecisionTo === b.amountPrecisionTo &&
    a.timePrecision === b.timePrecision &&
    a.timePrecisionTo === b.timePrecisionTo &&
    a.fromUnknown === b.fromUnknown &&
    a.fromNone === b.fromNone &&
    a.toUnknown === b.toUnknown &&
    a.toNone === b.toNone
  )
}

// getExistingClaimValues finds existing claims for a field and returns their IDs
// and full FieldEntryValue state.
export function getExistingClaimValues(claims: DeepReadonly<ClaimTypes> | undefined | null, field: FieldData): ExistingClaimValue[] {
  if (!claims) {
    return []
  }
  const claimType = valueTypeToClaimType(field.valueType)
  const existing = getClaimsOfTypeWithConfidence(claims, claimType, field.propertyId)
  return existing.map((claim) => ({ claimId: claim.GetID(), ...getClaimValues(claim) }))
}

// isIntervalField returns true if the field's value type is an interval (amount interval or time interval).
export function isIntervalField(field: FieldData): boolean {
  const claimType = valueTypeToClaimType(field.valueType)
  return claimType === "amountInterval" || claimType === "timeInterval"
}

// makePatchForField creates a patch object for a field from a FieldEntryValue.
// Per-side missing-state flags (fromUnknown/fromNone/toUnknown/toNone) take
// precedence over a typed value for that side.
export function makePatchForField(field: FieldData, data: FieldEntryValue): object {
  const claimType = valueTypeToClaimType(field.valueType)
  const base = { type: claimType, confidence: HighConfidence, prop: field.propertyId }
  switch (claimType) {
    case "id":
      return { ...base, value: data.value }
    case "string":
      return { ...base, string: data.value }
    case "html":
      return { ...base, html: data.value }
    case "amount": {
      const p = parseFloat(data.amountPrecision)
      return { ...base, amount: data.value, precision: isFinite(p) && p > 0 ? p : 1 }
    }
    case "amountInterval": {
      const patch: Record<string, unknown> = { ...base }
      if (data.fromUnknown) {
        patch.fromIsUnknown = true
      } else if (data.fromNone) {
        patch.fromIsNone = true
      } else if (data.value) {
        patch.from = data.value
        const fp = parseFloat(data.amountPrecision)
        patch.fromPrecision = isFinite(fp) && fp > 0 ? fp : 1
      } else {
        patch.fromIsNone = true
      }
      if (data.toUnknown) {
        patch.toIsUnknown = true
      } else if (data.toNone) {
        patch.toIsNone = true
      } else if (data.valueTo) {
        patch.to = data.valueTo
        const tp = parseFloat(data.amountPrecisionTo)
        patch.toPrecision = isFinite(tp) && tp > 0 ? tp : 1
      } else {
        patch.toIsNone = true
      }
      return patch
    }
    case "time":
      return { ...base, time: data.value, precision: data.timePrecision }
    case "timeInterval": {
      const patch: Record<string, unknown> = { ...base }
      if (data.fromUnknown) {
        patch.fromIsUnknown = true
      } else if (data.fromNone) {
        patch.fromIsNone = true
      } else if (data.value) {
        patch.from = data.value
        patch.fromPrecision = data.timePrecision
      } else {
        patch.fromIsNone = true
      }
      if (data.toUnknown) {
        patch.toIsUnknown = true
      } else if (data.toNone) {
        patch.toIsNone = true
      } else if (data.valueTo) {
        patch.to = data.valueTo
        patch.toPrecision = data.timePrecisionTo
      } else {
        patch.toIsNone = true
      }
      return patch
    }
    case "link":
      return { ...base, iri: data.value }
    case "ref":
      return { ...base, to: data.value }
    case "has":
    case "none":
    case "unknown":
      return base
    default:
      // eslint-disable-next-line @typescript-eslint/restrict-template-expressions
      throw new Error(`unsupported claim type: ${claimType}`)
  }
}
