import type { DeepReadonly, InjectionKey } from "vue"

import type { Claim, ClaimTypeName, TimePrecision } from "@/document"
import type { FieldsFormFlush, SaveChangeResult, SaveChangeSpec } from "@/types"

import { equals } from "@/utils"

import {
  CARDINALITY,
  FIELD,
  FIELD_CONTEXT,
  FIELD_DEFAULT,
  FIELD_INSTRUCTION,
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
  selectClaimsByLanguage,
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
  // The FIELD_DEFAULT value type, if the field's value may be absent: "none" for a
  // none-value default, "unknown" for an unknown-value default. When set, the field's value
  // may be stored as a NoneClaim/UnknownClaim (carrying any sub-claims) instead of a value
  // claim.
  default?: "none" | "unknown"
  // The field claim's sub-claims, holding among others the FIELD_INSTRUCTION HTML claims
  // (with IN_LANGUAGE sub-claims) the instructions are picked from by language (see
  // getFieldInstructions).
  claims?: DeepReadonly<ClaimTypes>
  // FIELD_CONTEXT values: opaque context identifiers from the field's configuration.
  // The read-only views skip fields with the "edit" context (see fieldShownInView).
  context?: readonly string[]
  // Set when a LINK field and a FILE field share the same property at the same level (sibling
  // fields). Both value types produce link claims under the same property, so this flag makes
  // getClaimsForField route each claim to exactly one of the two fields by whether its IRI is
  // a file link (see the isFileLink parameter there).
  fileLinkSibling?: boolean
}

// fieldShownInView reports whether the read-only views render the field. A field
// marked with the "edit" context should be available only for editing, so only the
// edit form renders it.
export function fieldShownInView(field: DeepReadonly<FieldData>): boolean {
  return !field.context?.includes("edit")
}

// isSimpleField reports whether a field renders as a single (non-repeating)
// value with no sub-fields. Spacing in FieldsForm widens around non-simple
// fields: a group of sibling fields uses gap-8 when any member is non-simple
// (else gap-4); the repeated entries of a field use gap-8 when the field has
// sub-fields (else gap-4); sections are separated by gap-12.
export function isSimpleField(field: DeepReadonly<FieldData>): boolean {
  return field.maxCardinality <= 1 && field.subFields.length === 0
}

// fieldIsRequired reports whether the edit form treats the field as required: a positive min
// cardinality demands user-entered values only when the field has no default. With a default,
// the save flow fills the missing count with default (none/unknown) form claims (see
// computeCardinalityFills), so nothing is required of the user and no required badge or
// "Required value." validation is shown.
export function fieldIsRequired(field: DeepReadonly<FieldData>): boolean {
  return field.minCardinality > 0 && field.default === undefined
}

// fieldSignature encodes a field's identity beyond its propertyId: its value type, default, and
// the (recursive, order-independent) signature of its sub-fields. Fields that share a propertyId
// but differ in value type, default, or sub-field structure get distinct signatures.
function fieldSignature(field: DeepReadonly<FieldData>): string {
  const subs = field.subFields.map(fieldSignature).sort()
  return `${field.propertyId}:${field.valueType}:${field.default ?? ""}(${subs.join(",")})`
}

// fieldKey returns a unique string key for a field, derived from its path plus its signature.
// The path distinguishes sub-fields with the same propertyId under different parents, and the
// signature distinguishes sibling fields that intentionally share a propertyId.
export function fieldKey(field: DeepReadonly<FieldData>): string {
  return `${field.path.join("/")}#${fieldSignature(field)}`
}

// SectionData represents a section of fields with an identifier, translated names, and ordering.
export interface SectionData {
  // Section identifier (the NAME identifier claim). Sections declared by multiple classes
  // merge when they share the same identifier; also used as the render key.
  id: string
  // The section claim's sub-claims, holding the NAME string claims (with IN_LANGUAGE
  // sub-claims) the display name is picked from by language (see getSectionName).
  claims?: DeepReadonly<ClaimTypes>
  // Numeric order for sorting.
  orderInList: number
  // Fields within this section.
  fields: readonly FieldData[]
}

// sectionElementId returns the DOM id of a section's rendered header, used as the
// scroll/hash target of the table of contents.
export function sectionElementId(section: DeepReadonly<SectionData>): string {
  return `section-${section.id}`
}

// getSectionName picks the section's display name for the given language, using the language
// fallback chain. When no language in the chain has a name, the section identifier is used.
export function getSectionName(section: DeepReadonly<SectionData>, language: string): string {
  const claims = selectClaimsByLanguage(section.claims, "string", NAME, language, (c) => c.length > 0 && !!c[0].string)
  if (claims && claims.length > 0) {
    return claims[0].string
  }
  return section.id
}

// FieldsData represents all fields and sections.
export interface FieldsData {
  // Named sections containing fields.
  sections: readonly SectionData[]
  // Top-level fields not in any section.
  fields: readonly FieldData[]
}

// markFileLinkSiblings sets fileLinkSibling on sibling fields (fields in the same level's list)
// which share a property while one of them has the LINK and another the FILE value type. Both
// value types produce link claims under the same property, so such siblings need claims routed
// between them (see getClaimsForField). Fields without such a sibling are left unmarked and
// keep matching all link claims of their property.
function markFileLinkSiblings(fields: readonly FieldData[]): void {
  const kindsByProperty = new Map<string, { link: boolean; file: boolean }>()
  for (const field of fields) {
    if (field.valueType !== VT_LINK && field.valueType !== VT_FILE) {
      continue
    }
    const kinds = kindsByProperty.get(field.propertyId) ?? { link: false, file: false }
    if (field.valueType === VT_LINK) {
      kinds.link = true
    } else {
      kinds.file = true
    }
    kindsByProperty.set(field.propertyId, kinds)
  }
  for (const field of fields) {
    if (field.valueType !== VT_LINK && field.valueType !== VT_FILE) {
      continue
    }
    const kinds = kindsByProperty.get(field.propertyId)
    if (kinds && kinds.link && kinds.file) {
      field.fileLinkSibling = true
    }
  }
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
  markFileLinkSiblings(subFields)

  const valueClaim = getBestClaimOfType(claimsTypes, "string", FIELD_VALUES)

  const contextClaims = getClaimsOfTypeWithConfidence(claimsTypes, "string", FIELD_CONTEXT)

  const defaultRef = getBestClaimOfType(claimsTypes, "ref", FIELD_DEFAULT)
  let fieldDefault: "none" | "unknown" | undefined
  if (defaultRef?.to.id === VT_NONE) {
    fieldDefault = "none"
  } else if (defaultRef?.to.id === VT_UNKNOWN) {
    fieldDefault = "unknown"
  }

  return {
    propertyId: propRef.to.id,
    valueType: valueTypeRef.to.id,
    orderInList: orderClaim ? parseFloat(orderClaim.amount) || 0 : 0,
    minCardinality,
    maxCardinality,
    subFields,
    path: thisPath,
    values: valueClaim?.string || undefined,
    default: fieldDefault,
    claims: claimsTypes,
    context: contextClaims.length > 0 ? contextClaims.map((claim) => claim.string) : undefined,
  }
}

// getFieldInstructions returns the field's instructions for the given language, using the
// language fallback chain: the FIELD_INSTRUCTION HTML claims from the field's configuration,
// longer form guidance shown after the value input's hints. Returns an empty array when the
// field has no instructions.
export function getFieldInstructions(field: DeepReadonly<FieldData>, language: string): DeepReadonly<HTMLClaim>[] {
  return selectClaimsByLanguage(field.claims, "html", FIELD_INSTRUCTION, language, (c) => c.length > 0 && !!c[0].html) ?? []
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
    markFileLinkSiblings(sectionFields)

    sections.push({
      id: idClaim ? idClaim.value : "",
      claims: sectionClaim.sub,
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
  markFileLinkSiblings(fields)

  return { sections, fields }
}

// mergeFields merges multiple FieldsData into a single union, deduplicating by field identity
// (property, value type, and sub-field structure, see fieldKey). Keying on the full identity
// rather than the propertyId alone keeps sibling fields that intentionally share a property,
// while still collapsing the same field declared by multiple classes of a multi-class document.
export function mergeFields(allFields: FieldsData[]): FieldsData {
  const seenKeys = new Set<string>()
  const mergedSections: SectionData[] = []
  const mergedFields: FieldData[] = []

  for (const fieldsData of allFields) {
    for (const section of fieldsData.sections) {
      // Deduplicate fields within sections.
      const newFields: FieldData[] = []
      for (const field of section.fields) {
        const key = fieldKey(field)
        if (!seenKeys.has(key)) {
          seenKeys.add(key)
          newFields.push(field)
        } else {
          // TODO: Do something better?
          console.error("duplicate field", key)
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
      const key = fieldKey(field)
      if (!seenKeys.has(key)) {
        seenKeys.add(key)
        mergedFields.push(field)
      }
    }
  }

  mergedSections.sort((a, b) => a.orderInList - b.orderInList)
  mergedFields.sort((a, b) => a.orderInList - b.orderInList)

  // Merging can bring together sibling LINK and FILE fields declared by different classes,
  // so mark the merged lists again.
  for (const section of mergedSections) {
    markFileLinkSiblings(section.fields)
  }
  markFileLinkSiblings(mergedFields)

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

// ChangeDroppedError rejects a queued change which was dropped instead of committed:
// after losing its change number to a concurrent change it no longer applies to the
// current document, or the server rejected it as invalid. The slot holding the claim
// resyncs to the committed state when it observes this error.
export class ChangeDroppedError extends Error {}

// Injection keys for FieldsForm shared services (using Symbol.for for deduplication in dev).
// See progress.ts for the pattern.
export const saveChangeKey: InjectionKey<(spec: SaveChangeSpec) => Promise<SaveChangeResult>> =
  process.env.NODE_ENV !== "production" ? Symbol.for("peerdb-saveChange") : Symbol()
export const registerForFlushKey: InjectionKey<(instance: FieldsFormFlush) => void> =
  process.env.NODE_ENV !== "production" ? Symbol.for("peerdb-registerForFlush") : Symbol()
export const unregisterForFlushKey: InjectionKey<(instance: FieldsFormFlush) => void> =
  process.env.NODE_ENV !== "production" ? Symbol.for("peerdb-unregisterForFlush") : Symbol()

// getCommittedClaimKey provides a lookup of a claim by id in the document with all
// committed session changes applied. Slots use it to resync to the committed state after
// a dropped change or a remote conflict.
export const getCommittedClaimKey: InjectionKey<(id: string) => DeepReadonly<Claim> | null> =
  process.env.NODE_ENV !== "production" ? Symbol.for("peerdb-getCommittedClaim") : Symbol()

// Remote conflict handlers: DocumentEdit notifies these with the set of claim ids touched
// by committed changes from other session editors whenever the subscription applies
// them. The set also contains the ancestor claim ids of every touched claim. Each slot
// (ClaimInput) holding a touched claim resyncs to the committed state, discarding local
// work (server wins).
// TODO: Implement better conflict handling and change comment above.
export const registerRemoteConflictKey: InjectionKey<(handler: (claimIds: ReadonlySet<string>) => void) => void> =
  process.env.NODE_ENV !== "production" ? Symbol.for("peerdb-registerRemoteConflict") : Symbol()
export const unregisterRemoteConflictKey: InjectionKey<(handler: (claimIds: ReadonlySet<string>) => void) => void> =
  process.env.NODE_ENV !== "production" ? Symbol.for("peerdb-unregisterRemoteConflict") : Symbol()

// Remote add handlers: notified with the same touched set as the conflict handlers, but
// only after the render flush has propagated resynced claims into every cardinality's
// modelValue. Each ClaimCardinality then adds slots for remotely added claims of its
// field which no slot represents yet, reporting whether it added any. Handlers run in
// rounds (see loadChanges in DocumentEdit): a filled slot feeds its sub-cardinalities'
// modelValue only after the next render flush, so each round can reveal claims one
// nesting level deeper.
export const registerRemoteAddsKey: InjectionKey<(handler: (claimIds: ReadonlySet<string>) => boolean) => void> =
  process.env.NODE_ENV !== "production" ? Symbol.for("peerdb-registerRemoteAdds") : Symbol()
export const unregisterRemoteAddsKey: InjectionKey<(handler: (claimIds: ReadonlySet<string>) => boolean) => void> =
  process.env.NODE_ENV !== "production" ? Symbol.for("peerdb-unregisterRemoteAdds") : Symbol()

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

// claimMatchesFieldSubFields returns true if the claim carries at least one sub-claim for one
// of the field's direct sub-field properties (matching that sub-field's value type).
function claimMatchesFieldSubFields(claim: DeepReadonly<Claim>, field: DeepReadonly<FieldData>): boolean {
  if (!claim.sub) {
    return false
  }
  for (const subField of field.subFields) {
    const subClaimType = valueTypeToClaimType(subField.valueType)
    if (getClaimsOfTypeWithConfidence(claim.sub, subClaimType, subField.propertyId).length > 0) {
      return true
    }
  }
  return false
}

// isValuelessClaimType reports whether a claim type carries no value of its own (its meaning is
// its presence plus its sub-claims): HAS, NONE, and UNKNOWN.
function isValuelessClaimType(claimType: ClaimTypeName): boolean {
  return claimType === "has" || claimType === "none" || claimType === "unknown"
}

// getClaimsForField returns the claims that belong to a field.
//
// A field always matches claims of its declared value type. A value field with a default also
// matches the corresponding valueless claim type (NONE/UNKNOWN), because such a field stores an
// absent value as a NoneClaim/UnknownClaim that still carries its sub-claims (e.g. an artist
// studio whose location is unknown but which has notes).
//
// For valueless claim types (HAS/NONE/UNKNOWN) on a field with sub-fields, we keep only claims
// carrying one of the field's sub-field properties. A valueless claim has no value of its own,
// so its sub-claims are what identify it; this keeps sibling fields that share a propertyId
// from matching each other's claims. Claims of the field's own default claim type are exempt:
// they belong to the value field (sibling HAS-meta fields never match NONE/UNKNOWN claims), and
// a default form claim legitimately carries no sub-claims at all (a field left empty stores a
// bare default form claim, see ClaimInput's eager default claim).
//
// Sibling LINK and FILE fields sharing a property (see FieldData.fileLinkSibling) both hold
// link claims, so their claims are routed by the claim's IRI: file links go to the FILE field
// and all other links to the LINK field. The isFileLink predicate decides whether an IRI is a
// file link (classifyLink reporting LINK_CLASS_FILE); it needs the router, so callers rendering
// such fields must provide it. Without the predicate no routing happens and both siblings match
// all link claims of the property.
export function getClaimsForField(
  claims: DeepReadonly<ClaimTypes> | undefined | null,
  field: DeepReadonly<FieldData>,
  isFileLink?: (iri: string) => boolean,
): DeepReadonly<Claim>[] {
  const valueType = valueTypeToClaimType(field.valueType)
  const claimTypes = new Set<ClaimTypeName>([valueType])
  if (field.default === "none") {
    claimTypes.add("none")
  } else if (field.default === "unknown") {
    claimTypes.add("unknown")
  }

  const result: DeepReadonly<Claim>[] = []
  for (const claimType of claimTypes) {
    for (const claim of getClaimsOfTypeWithConfidence(claims, claimType, field.propertyId) as DeepReadonly<Claim>[]) {
      if (claimType !== field.default && isValuelessClaimType(claimType) && field.subFields.length > 0 && !claimMatchesFieldSubFields(claim, field)) {
        continue
      }
      if (claimType === "link" && field.fileLinkSibling && isFileLink && isFileLink((claim as DeepReadonly<LinkClaim>).iri) !== (field.valueType === VT_FILE)) {
        continue
      }
      result.push(claim)
    }
  }
  return result
}

// getExistingClaimValues finds existing claims for a field and returns their IDs
// and full FieldEntryValue state.
export function getExistingClaimValues(
  claims: DeepReadonly<ClaimTypes> | undefined | null,
  field: FieldData,
  isFileLink?: (iri: string) => boolean,
): ExistingClaimValue[] {
  if (!claims) {
    return []
  }
  const existing = getClaimsForField(claims, field, isFileLink)
  return existing.map((claim) => ({ claimId: claim.GetID(), ...getClaimValues(claim) }))
}

// isIntervalField returns true if the field's value type is an interval (amount interval or time interval).
export function isIntervalField(field: FieldData): boolean {
  const claimType = valueTypeToClaimType(field.valueType)
  return claimType === "amountInterval" || claimType === "timeInterval"
}

// makePatchForField creates a patch object for a field from a FieldEntryValue.
// Per-side missing-state flags (fromUnknown/fromNone/toUnknown/toNone) take
// precedence over a typed value for that side. An interval bound with no value
// and no flag defaults to unknown.
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
        patch.fromIsUnknown = true
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
        patch.toIsUnknown = true
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
        patch.fromIsUnknown = true
      }
      if (data.toUnknown) {
        patch.toIsUnknown = true
      } else if (data.toNone) {
        patch.toIsNone = true
      } else if (data.valueTo) {
        patch.to = data.valueTo
        patch.toPrecision = data.timePrecisionTo
      } else {
        patch.toIsUnknown = true
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

// makeDefaultPatchForField builds a patch for a field's default (none/unknown) value type, used
// to lazily create or cast to the valueless form of a value field that has a default. Throws if
// the field has no default.
export function makeDefaultPatchForField(field: DeepReadonly<FieldData>): object {
  if (!field.default) {
    throw new Error("field has no default")
  }
  return { type: field.default, confidence: HighConfidence, prop: field.propertyId }
}

// CardinalityFill describes one default form claim the save flow adds so that a field with a
// default satisfies its min cardinality (see computeCardinalityFills).
export interface CardinalityFill {
  // Patch for the default (none/unknown) form claim (see makeDefaultPatchForField).
  patch: object
  // Id of the existing claim to add this claim under. Undefined for a top-level fill and for
  // a fill nested under another fill (the executor uses the id of the just-posted parent).
  under?: string
  // Fills for the added claim's own sub-fields with defaults; they must be added under the
  // claim created from patch, whose id exists only once patch is posted.
  children: CardinalityFill[]
}

// computeCardinalityFills returns the default form claims to add so that every given field with
// a default satisfies its min cardinality: for each such field with fewer claims than its min,
// the missing count of bare default form claims. It recurses into sub-fields, both under every
// existing claim of a field and under every added fill. Fields without a default are never
// filled; the strict check reports them instead (see cardinalityViolations).
export function computeCardinalityFills(
  fields: readonly DeepReadonly<FieldData>[],
  claims: DeepReadonly<ClaimTypes> | undefined | null,
  isFileLink?: (iri: string) => boolean,
): CardinalityFill[] {
  const fills: CardinalityFill[] = []
  for (const field of fields) {
    const existing = getClaimsForField(claims ?? null, field, isFileLink)
    if (field.default !== undefined) {
      for (let i = existing.length; i < field.minCardinality; i++) {
        // A fresh bare default form claim, itself filled for its own sub-fields (none exist yet).
        fills.push({ patch: makeDefaultPatchForField(field), under: undefined, children: computeCardinalityFills(field.subFields, null, isFileLink) })
      }
    }
    for (const claim of existing) {
      for (const fill of computeCardinalityFills(field.subFields, claim.sub ?? null, isFileLink)) {
        // A fill directly below this claim gets its id; deeper fills already carry theirs.
        fills.push(fill.under === undefined ? { ...fill, under: claim.GetID() } : fill)
      }
    }
  }
  return fills
}

// CardinalityViolation reports a field whose claim count does not reach its min cardinality.
export interface CardinalityViolation {
  propertyId: string
  path: readonly string[]
  min: number
  count: number
}

// cardinalityViolations returns every given field (recursively, sub-fields checked under each of
// the parent field's claims) whose claim count is below its min cardinality. The check is
// strict: a field's default gives it no pass. The save flow fills defaults first (see
// computeCardinalityFills), so a violation reported for a default field means the fill logic
// failed and the save must not proceed.
export function cardinalityViolations(
  fields: readonly DeepReadonly<FieldData>[],
  claims: DeepReadonly<ClaimTypes> | undefined | null,
  isFileLink?: (iri: string) => boolean,
): CardinalityViolation[] {
  const violations: CardinalityViolation[] = []
  for (const field of fields) {
    const existing = getClaimsForField(claims ?? null, field, isFileLink)
    if (existing.length < field.minCardinality) {
      violations.push({ propertyId: field.propertyId, path: field.path, min: field.minCardinality, count: existing.length })
    }
    for (const claim of existing) {
      violations.push(...cardinalityViolations(field.subFields, claim.sub ?? null, isFileLink))
    }
  }
  return violations
}

// A claim as claimsEquivalent inspects it: an id, its own value fields, and optional nested
// sub-claims. Kept structural (not the Claim class) so raw JSON claims and reactive proxies both fit.
type ClaimLike = { id: string; sub?: DeepReadonly<ClaimTypes> | null }

// claimsEquivalent reports whether two claim sets hold the same claims, keyed by id at every level
// and independent of a claim's position in its type array. Drives net-change detection: an edit
// session whose posted changes cancel out (e.g. an edit and its Revert) compares equal to its
// baseline and has nothing to save. Order-independence must be recursive: a Revert may restore a
// claim's sub-claims in a different array order than the baseline, so comparing whole claims with a
// deep (order-sensitive) equals would falsely report a change. We instead key sub-claims by id too
// (see claimEquivalent) and compare each claim's own value fields with a deep equals that excludes
// sub. That structural compare (not a JSON string compare) also handles a claim reconstructed
// locally after a cast serializing its keys in a different order than the server-loaded one.
export function claimsEquivalent(a: DeepReadonly<ClaimTypes> | undefined | null, b: DeepReadonly<ClaimTypes> | undefined | null): boolean {
  // We iterate the (possibly reactive) claims directly rather than through toRaw: a reactive caller
  // (canSave) must track claim reads so it re-runs when the doc changes, and the deep equals
  // compares reactive proxies correctly by reading their values.
  const index = (claims: DeepReadonly<ClaimTypes> | undefined | null): Map<string, ClaimLike> => {
    const m = new Map<string, ClaimLike>()
    if (!claims) {
      return m
    }
    for (const list of Object.values(claims)) {
      if (!Array.isArray(list)) {
        continue
      }
      for (const claim of list as ClaimLike[]) {
        m.set(claim.id, claim)
      }
    }
    return m
  }
  const indexA = index(a)
  const indexB = index(b)
  if (indexA.size !== indexB.size) {
    return false
  }
  for (const [id, claim] of indexA) {
    const other = indexB.get(id)
    if (other === undefined || !claimEquivalent(claim, other)) {
      return false
    }
  }
  return true
}

// claimEquivalent compares two claims with the same id: their own value fields via a deep equals
// with sub excluded (spreading normalizes sub to undefined on both sides so equals ignores it),
// then their sub-claims recursively through claimsEquivalent so nested claim arrays are compared
// id-keyed and order-independent, like the top level.
function claimEquivalent(a: ClaimLike, b: ClaimLike): boolean {
  if (!equals({ ...a, sub: undefined }, { ...b, sub: undefined })) {
    return false
  }
  return claimsEquivalent(a.sub ?? null, b.sub ?? null)
}
