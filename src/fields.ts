import type { DeepReadonly, InjectionKey } from "vue"

import type { Claim, Claims, ClaimTypeName } from "@/document"

import {
  CARDINALITY,
  FIELD,
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
  selectClaimsByLanguage,
  StringClaim,
  TimeClaim,
  TimeIntervalClaim,
  UnknownClaim,
} from "@/document"
import { ABSTRACT_CLASS } from "./core/properties"

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
  subFields: FieldData[]
  // Path from the root field to this field, using property IDs.
  // Top-level fields have a single-element path. Sub-fields have
  // [parentPropertyId, ..., thisPropertyId]. Used as a unique key
  // to distinguish sub-fields with the same propertyId under different parents.
  path: string[]
}

// fieldKey returns a unique string key for a field, derived from its path.
export function fieldKey(field: FieldData): string {
  return field.path.join("/")
}

// SectionData represents a section of fields with a name and ordering.
export interface SectionData {
  // Section name.
  name: string
  // Numeric order for sorting.
  orderInList: number
  // Fields within this section.
  fields: FieldData[]
}

// FieldsData represents all fields and sections.
export interface FieldsData {
  // Named sections containing fields.
  sections: SectionData[]
  // Top-level fields not in any section.
  fields: FieldData[]
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

  return {
    propertyId: propRef.to.id,
    valueType: valueTypeRef.to.id,
    orderInList: orderClaim ? parseFloat(orderClaim.amount) || 0 : 0,
    minCardinality,
    maxCardinality,
    subFields,
    path: thisPath,
  }
}

// extractFieldsFromClaims extracts FieldsData from a class document's claims.
export function extractFieldsFromClaims(claims: DeepReadonly<Claims> | undefined | null, language: string): FieldsData | null {
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
    const nameClaims = selectClaimsByLanguage(sectionClaim.sub, "string", NAME, language, (claims) => claims.length > 0)
    const sectionName = nameClaims && nameClaims.length > 0 ? nameClaims[0].string : ""
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
      name: sectionName,
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
        // Check if we already have a section with the same name.
        const existingSection = mergedSections.find((s) => s.name === section.name)
        if (existingSection) {
          existingSection.fields.push(...newFields)
          existingSection.fields.sort((a, b) => a.orderInList - b.orderInList)
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
export function hasFields(claims: DeepReadonly<Claims> | undefined | null): boolean {
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
export function isAbstractClass(claims: DeepReadonly<Claims> | undefined | null): boolean {
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

// ExistingClaimValue represents an existing claim's value extracted for display in form fields.
export interface ExistingClaimValue {
  claimId: string
  // Primary value (or "from" for interval types).
  value: string
  // Secondary value ("to" for interval types, empty for non-interval types).
  valueTo: string
}

// getClaimValues extracts the primary and secondary (for intervals) string values from a claim.
function getClaimValues(claim: DeepReadonly<Claim>): { value: string; valueTo: string } {
  if (claim instanceof IdentifierClaim) {
    return { value: claim.value, valueTo: "" }
  }
  if (claim instanceof StringClaim) {
    return { value: claim.string, valueTo: "" }
  }
  if (claim instanceof HTMLClaim) {
    return { value: claim.html, valueTo: "" }
  }
  if (claim instanceof AmountClaim) {
    return { value: claim.amount, valueTo: "" }
  }
  if (claim instanceof AmountIntervalClaim) {
    return { value: claim.from ?? "", valueTo: claim.to ?? "" }
  }
  if (claim instanceof TimeClaim) {
    return { value: claim.time, valueTo: "" }
  }
  if (claim instanceof TimeIntervalClaim) {
    return { value: claim.from ?? "", valueTo: claim.to ?? "" }
  }
  if (claim instanceof LinkClaim) {
    return { value: claim.iri, valueTo: "" }
  }
  if (claim instanceof ReferenceClaim) {
    return { value: claim.to.id, valueTo: "" }
  }
  if (claim instanceof HasClaim || claim instanceof NoneClaim || claim instanceof UnknownClaim) {
    return { value: "", valueTo: "" }
  }
  throw new Error("unsupported claim type")
}

// getExistingClaimValues finds existing claims for a field and returns their IDs and string values.
export function getExistingClaimValues(claims: DeepReadonly<Claims> | undefined | null, field: FieldData): ExistingClaimValue[] {
  if (!claims) {
    return []
  }
  const claimType = valueTypeToClaimType(field.valueType)
  const existing = getClaimsOfTypeWithConfidence(claims, claimType, field.propertyId)
  return existing.map((claim) => {
    const { value, valueTo } = getClaimValues(claim)
    return { claimId: claim.GetID(), value, valueTo }
  })
}

// isIntervalField returns true if the field's value type is an interval (amount interval or time interval).
export function isIntervalField(field: FieldData): boolean {
  const claimType = valueTypeToClaimType(field.valueType)
  return claimType === "amountInterval" || claimType === "timeInterval"
}

// makePatchForField creates a patch object for a field based on its value type.
// For interval fields, valueTo is the "to" bound.
export function makePatchForField(field: FieldData, value: string, valueTo?: string): object {
  const claimType = valueTypeToClaimType(field.valueType)
  const base = { type: claimType, confidence: HighConfidence, prop: field.propertyId }
  switch (claimType) {
    case "id":
      return { ...base, value }
    case "string":
      return { ...base, string: value }
    case "html":
      return { ...base, html: value }
    case "amount":
      // TODO: Handle precision properly.
      return { ...base, amount: value, precision: 1 }
    case "amountInterval": {
      const patch: Record<string, unknown> = { ...base }
      if (value) {
        patch.from = value
        // TODO: Handle precision properly.
        patch.fromPrecision = 1
      } else {
        patch.fromIsNone = true
      }
      if (valueTo) {
        patch.to = valueTo
        // TODO: Handle precision properly.
        patch.toPrecision = 1
      } else {
        patch.toIsNone = true
      }
      return patch
    }
    case "time":
      // TODO: Handle precision properly.
      return { ...base, time: value, precision: "d" }
    case "timeInterval": {
      const patch: Record<string, unknown> = { ...base }
      if (value) {
        patch.from = value
        // TODO: Handle precision properly.
        patch.fromPrecision = "d"
      } else {
        patch.fromIsNone = true
      }
      if (valueTo) {
        patch.to = valueTo
        // TODO: Handle precision properly.
        patch.toPrecision = "d"
      } else {
        patch.toIsNone = true
      }
      return patch
    }
    case "link":
      return { ...base, iri: value }
    case "ref":
      return { ...base, to: value }
    case "has":
    case "none":
    case "unknown":
      return base
    default:
      // eslint-disable-next-line @typescript-eslint/restrict-template-expressions
      throw new Error(`unsupported claim type: ${claimType}`)
  }
}
