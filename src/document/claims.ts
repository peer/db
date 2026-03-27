import type { DeepReadonly } from "vue"

import type { Amount, Confidence, Reference, TimePrecision, Timestamp } from "@/document/types"
import type { Constructee, Constructor, Required } from "@/types"

import siteContext from "@/context"
import { IN_LANGUAGE, LIST, ORDER_IN_LIST } from "@/core"
import { LowConfidence } from "@/document/confidence"

// VALID_TIME_PRECISIONS is the set of valid TimePrecision values.
// TODO: Add "ms" | "us" | "ns".
const VALID_TIME_PRECISIONS: Set<string> = new Set(["G", "100M", "10M", "M", "100k", "10k", "k", "100y", "10y", "y", "m", "d", "h", "min", "s"])

// Claims is the interface for types that hold and manipulate a collection of claims.
export interface Claims {
  Get(propID: string): Claim[]
  Remove(propID: string): Claim[]
  GetByID(id: string): Claim | undefined
  RemoveByID(id: string): Claim | undefined
  Add(claim: Claim): void
  Size(): number
  AllClaims(): Claim[]
  Validate(): Promise<void>
}

// ClaimsContainer defines the interface for types that can hold and manipulate claims.
export interface ClaimsContainer extends Claims {
  GetID(): string
}

// CoreClaim contains fields common to all claim types.
class CoreClaim implements ClaimsContainer {
  id!: string
  confidence!: Confidence
  meta?: ClaimTypes

  GetID(): string {
    return this.id
  }

  GetConfidence(): Confidence {
    return this.confidence
  }

  Get(propID: string): Claim[] {
    if (this.meta === undefined) {
      return []
    }
    return this.meta.Get(propID)
  }

  Remove(propID: string): Claim[] {
    if (this.meta === undefined) {
      return []
    }
    return this.meta.Remove(propID)
  }

  GetByID(id: string): Claim | undefined {
    if (this.meta === undefined) {
      return
    }
    return this.meta.GetByID(id)
  }

  RemoveByID(id: string): Claim | undefined {
    if (this.meta === undefined) {
      return
    }
    return this.meta.RemoveByID(id)
  }

  Add(claim: Claim): void {
    if (this.meta === undefined) {
      this.meta = new ClaimTypes({})
    }
    this.meta.Add(claim)
  }

  Size(): number {
    if (this.meta === undefined) {
      return 0
    }
    return this.meta.Size()
  }

  AllClaims(): Claim[] {
    if (this.meta === undefined) {
      return []
    }
    return this.meta.AllClaims()
  }

  async Validate(): Promise<void> {
    if (this.confidence < -1 || this.confidence > 1 || !isFinite(this.confidence)) {
      throw new Error("confidence out of range [-1, 1]")
    }
    if (this.meta !== undefined) {
      await this.meta.Validate()
    }
  }
}

export class IdentifierClaim extends CoreClaim {
  prop!: Reference
  value!: string

  constructor(obj: object) {
    super()
    Object.assign(this, obj)
    if (!this.id) {
      throw new Error("id is required")
    }
    if (this.confidence === undefined) {
      throw new Error("confidence is required")
    }
    if (!this.prop) {
      throw new Error("prop is required")
    }
    if (!this.value) {
      throw new Error("value is required")
    }
    if (this.meta !== undefined) {
      this.meta = new ClaimTypes(this.meta)
    }
  }

  // Validate checks that the identifier claim has a non-empty value and valid confidence.
  async Validate(): Promise<void> {
    await super.Validate()
    if (!this.value) {
      throw new Error("empty value")
    }
  }
}

export class StringClaim extends CoreClaim {
  prop!: Reference
  string!: string

  constructor(obj: object) {
    super()
    Object.assign(this, obj)
    if (!this.id) {
      throw new Error("id is required")
    }
    if (this.confidence === undefined) {
      throw new Error("confidence is required")
    }
    if (!this.prop) {
      throw new Error("prop is required")
    }
    if (!this.string) {
      throw new Error("string is required")
    }
    if (this.meta !== undefined) {
      this.meta = new ClaimTypes(this.meta)
    }
  }

  // Validate checks that the string claim has a non-empty string and valid confidence.
  async Validate(): Promise<void> {
    await super.Validate()
    if (!this.string) {
      throw new Error("empty string")
    }
  }
}

export class HTMLClaim extends CoreClaim {
  prop!: Reference
  html!: string

  constructor(obj: object) {
    super()
    Object.assign(this, obj)
    if (!this.id) {
      throw new Error("id is required")
    }
    if (this.confidence === undefined) {
      throw new Error("confidence is required")
    }
    if (!this.prop) {
      throw new Error("prop is required")
    }
    if (!this.html) {
      throw new Error("html is required")
    }
    if (this.meta !== undefined) {
      this.meta = new ClaimTypes(this.meta)
    }
  }

  // Validate checks that the HTML claim has non-empty HTML and valid confidence.
  async Validate(): Promise<void> {
    await super.Validate()
    if (!this.html) {
      throw new Error("empty HTML")
    }
  }
}

export class AmountClaim extends CoreClaim {
  prop!: Reference
  amount!: Amount
  precision!: number

  constructor(obj: object) {
    super()
    Object.assign(this, obj)
    if (!this.id) {
      throw new Error("id is required")
    }
    if (this.confidence === undefined) {
      throw new Error("confidence is required")
    }
    if (!this.prop) {
      throw new Error("prop is required")
    }
    if (!this.amount) {
      throw new Error("amount is required")
    }
    if (this.precision === undefined) {
      throw new Error("precision is required")
    }
    if (this.meta !== undefined) {
      this.meta = new ClaimTypes(this.meta)
    }
  }

  // Validate checks that the amount claim has valid amount, precision, and confidence.
  async Validate(): Promise<void> {
    await super.Validate()
    if (!isFinite(this.precision) || this.precision <= 0) {
      throw new Error(`Precision must be a finite positive number`)
    }
    // TODO: Validate amount string format against precision.
  }
}

export class AmountIntervalClaim extends CoreClaim {
  prop!: Reference
  from?: Amount
  fromPrecision?: number
  fromIsOpen?: boolean
  fromIsUnknown?: boolean
  fromIsNone?: boolean
  to?: Amount
  toPrecision?: number
  toIsClosed?: boolean
  toIsUnknown?: boolean
  toIsNone?: boolean

  constructor(obj: object) {
    super()
    Object.assign(this, obj)
    if (!this.id) {
      throw new Error("id is required")
    }
    if (this.confidence === undefined) {
      throw new Error("confidence is required")
    }
    if (!this.prop) {
      throw new Error("prop is required")
    }
    if (this.meta !== undefined) {
      this.meta = new ClaimTypes(this.meta)
    }
  }

  // Validate checks that the amount interval claim has valid bounds and valid confidence.
  async Validate(): Promise<void> {
    await super.Validate()

    let fromIsCount = 0
    if (this.fromIsOpen) fromIsCount++
    if (this.fromIsUnknown) fromIsCount++
    if (this.fromIsNone) fromIsCount++
    if (fromIsCount > 1) {
      throw new Error("only one of FromIsOpen, FromIsUnknown, FromIsNone can be set")
    }
    if (!this.from !== (this.fromPrecision === undefined)) {
      throw new Error("From and FromPrecision must be set together")
    }
    if (!this.from && !this.fromIsUnknown && !this.fromIsNone) {
      throw new Error("one of From, FromIsUnknown, or FromIsNone must be set")
    }
    if (this.from && (this.fromIsUnknown || this.fromIsNone)) {
      throw new Error("From must not be set when FromIsUnknown or FromIsNone is true")
    }
    if (this.fromPrecision !== undefined) {
      if (!isFinite(this.fromPrecision) || this.fromPrecision <= 0) {
        throw new Error("FromPrecision must be finite positive number")
      }
      // TODO: Validate this.from against this.fromPrecision.
    }

    let toIsCount = 0
    if (this.toIsClosed) toIsCount++
    if (this.toIsUnknown) toIsCount++
    if (this.toIsNone) toIsCount++
    if (toIsCount > 1) {
      throw new Error("only one of ToIsClosed, ToIsUnknown, ToIsNone can be set")
    }
    if (!this.to !== (this.toPrecision === undefined)) {
      throw new Error("To and ToPrecision must be set together")
    }
    if (!this.to && !this.toIsUnknown && !this.toIsNone) {
      throw new Error("one of To, ToIsUnknown, or ToIsNone must be set")
    }
    if (this.to && (this.toIsUnknown || this.toIsNone)) {
      throw new Error("To must not be set when ToIsUnknown or ToIsNone is true")
    }
    if (this.toPrecision !== undefined) {
      if (!isFinite(this.toPrecision) || this.toPrecision <= 0) {
        throw new Error("ToPrecision must be finite positive number")
      }
      // TODO: Validate this.to against this.toPrecision.
    }
  }
}

export class TimeClaim extends CoreClaim {
  prop!: Reference
  timestamp!: Timestamp
  precision!: TimePrecision

  constructor(obj: object) {
    super()
    Object.assign(this, obj)
    if (!this.id) {
      throw new Error("id is required")
    }
    if (this.confidence === undefined) {
      throw new Error("confidence is required")
    }
    if (!this.prop) {
      throw new Error("prop is required")
    }
    if (!this.timestamp) {
      throw new Error("timestamp is required")
    }
    if (this.precision === undefined) {
      throw new Error("precision is required")
    }
    if (this.meta !== undefined) {
      this.meta = new ClaimTypes(this.meta)
    }
  }

  // Validate checks that the time claim has a valid precision, timestamp, and valid confidence.
  async Validate(): Promise<void> {
    await super.Validate()
    if (!VALID_TIME_PRECISIONS.has(this.precision)) {
      throw new Error("unknown Precision")
    }
    // TODO: Validate timestamp format against precision.
  }
}

export class TimeIntervalClaim extends CoreClaim {
  prop!: Reference
  from?: Timestamp
  fromPrecision?: TimePrecision
  fromIsOpen?: boolean
  fromIsUnknown?: boolean
  fromIsNone?: boolean
  to?: Timestamp
  toPrecision?: TimePrecision
  toIsClosed?: boolean
  toIsUnknown?: boolean
  toIsNone?: boolean

  constructor(obj: object) {
    super()
    Object.assign(this, obj)
    if (!this.id) {
      throw new Error("id is required")
    }
    if (this.confidence === undefined) {
      throw new Error("confidence is required")
    }
    if (!this.prop) {
      throw new Error("prop is required")
    }
    if (this.meta !== undefined) {
      this.meta = new ClaimTypes(this.meta)
    }
  }

  // Validate checks that the time interval claim has valid bounds and valid confidence.
  async Validate(): Promise<void> {
    await super.Validate()

    let fromIsCount = 0
    if (this.fromIsOpen) fromIsCount++
    if (this.fromIsUnknown) fromIsCount++
    if (this.fromIsNone) fromIsCount++
    if (fromIsCount > 1) {
      throw new Error("only one of FromIsOpen, FromIsUnknown, FromIsNone can be set")
    }
    if (!this.from !== (this.fromPrecision === undefined)) {
      throw new Error("From and FromPrecision must be set together")
    }
    if (!this.from && !this.fromIsUnknown && !this.fromIsNone) {
      throw new Error("one of From, FromIsUnknown, or FromIsNone must be set")
    }
    if (this.from && (this.fromIsUnknown || this.fromIsNone)) {
      throw new Error("From must not be set when FromIsUnknown or FromIsNone is true")
    }
    if (this.fromPrecision !== undefined) {
      if (!VALID_TIME_PRECISIONS.has(this.fromPrecision)) {
        throw new Error("unknown FromPrecision")
      }
      // TODO: Validate this.from against this.fromPrecision.
    }

    let toIsCount = 0
    if (this.toIsClosed) toIsCount++
    if (this.toIsUnknown) toIsCount++
    if (this.toIsNone) toIsCount++
    if (toIsCount > 1) {
      throw new Error("only one of ToIsClosed, ToIsUnknown, ToIsNone can be set")
    }
    if (!this.to !== (this.toPrecision === undefined)) {
      throw new Error("To and ToPrecision must be set together")
    }
    if (!this.to && !this.toIsUnknown && !this.toIsNone) {
      throw new Error("one of To, ToIsUnknown, or ToIsNone must be set")
    }
    if (this.to && (this.toIsUnknown || this.toIsNone)) {
      throw new Error("To must not be set when ToIsUnknown or ToIsNone is true")
    }
    if (this.toPrecision !== undefined) {
      if (!VALID_TIME_PRECISIONS.has(this.toPrecision)) {
        throw new Error("unknown ToPrecision")
      }
      // TODO: Validate this.to against this.toPrecision.
    }
  }
}

export class LinkClaim extends CoreClaim {
  prop!: Reference
  iri!: string

  constructor(obj: object) {
    super()
    Object.assign(this, obj)
    if (!this.id) {
      throw new Error("id is required")
    }
    if (this.confidence === undefined) {
      throw new Error("confidence is required")
    }
    if (!this.prop) {
      throw new Error("prop is required")
    }
    if (!this.iri) {
      throw new Error("iri is required")
    }
    if (this.meta !== undefined) {
      this.meta = new ClaimTypes(this.meta)
    }
  }

  // Validate checks that the link claim has a non-empty IRI and valid confidence.
  async Validate(): Promise<void> {
    await super.Validate()
    if (!this.iri) {
      throw new Error("empty IRI")
    }
  }
}

export class ReferenceClaim extends CoreClaim {
  prop!: Reference
  to!: Reference

  constructor(obj: object) {
    super()
    Object.assign(this, obj)
    if (!this.id) {
      throw new Error("id is required")
    }
    if (this.confidence === undefined) {
      throw new Error("confidence is required")
    }
    if (!this.prop) {
      throw new Error("prop is required")
    }
    if (!this.to) {
      throw new Error("to is required")
    }
    if (this.meta !== undefined) {
      this.meta = new ClaimTypes(this.meta)
    }
  }
}

export class HasClaim extends CoreClaim {
  prop!: Reference

  constructor(obj: object) {
    super()
    Object.assign(this, obj)
    if (!this.id) {
      throw new Error("id is required")
    }
    if (this.confidence === undefined) {
      throw new Error("confidence is required")
    }
    if (!this.prop) {
      throw new Error("prop is required")
    }
    if (this.meta !== undefined) {
      this.meta = new ClaimTypes(this.meta)
    }
  }
}

export class NoneClaim extends CoreClaim {
  prop!: Reference

  constructor(obj: object) {
    super()
    Object.assign(this, obj)
    if (!this.id) {
      throw new Error("id is required")
    }
    if (this.confidence === undefined) {
      throw new Error("confidence is required")
    }
    if (!this.prop) {
      throw new Error("prop is required")
    }
    if (this.meta !== undefined) {
      this.meta = new ClaimTypes(this.meta)
    }
  }
}

export class UnknownClaim extends CoreClaim {
  prop!: Reference

  constructor(obj: object) {
    super()
    Object.assign(this, obj)
    if (!this.id) {
      throw new Error("id is required")
    }
    if (this.confidence === undefined) {
      throw new Error("confidence is required")
    }
    if (!this.prop) {
      throw new Error("prop is required")
    }
    if (this.meta !== undefined) {
      this.meta = new ClaimTypes(this.meta)
    }
  }
}

// ClaimTypes organizes claims by their type.
export class ClaimTypes implements Claims {
  id?: IdentifierClaim[]
  string?: StringClaim[]
  html?: HTMLClaim[]
  amount?: AmountClaim[]
  amountInterval?: AmountIntervalClaim[]
  time?: TimeClaim[]
  timeInterval?: TimeIntervalClaim[]
  link?: LinkClaim[]
  ref?: ReferenceClaim[]
  has?: HasClaim[]
  none?: NoneClaim[]
  unknown?: UnknownClaim[]

  constructor(obj: Record<string, object[]> | ClaimTypes) {
    for (const [name, claimType] of Object.entries(CLAIM_TYPES_MAP) as ClaimTypesEntry[]) {
      if (!obj?.[name]) continue
      if (!Array.isArray(obj[name])) throw new Error(`is not an array: ${name}`)
      ;(this[name] as Constructee<typeof claimType>[]) = obj[name].map((claim) => new claimType(claim))
    }
  }

  Get(propID: string): Claim[] {
    const claims: Claim[] = []
    for (const claim of this.AllClaims()) {
      if (claim.prop.id === propID) {
        claims.push(claim)
      }
    }
    claims.sort((a, b) => b.confidence - a.confidence)
    return claims
  }

  Remove(propID: string): Claim[] {
    const removed: Claim[] = []
    for (const name of Object.keys(CLAIM_TYPES_MAP) as ClaimTypeName[]) {
      const claims = this[name]
      if (!claims) continue
      for (let i = claims.length - 1; i >= 0; i--) {
        if (claims[i].prop.id === propID) {
          removed.push(claims.splice(i, 1)[0])
        }
      }
    }
    return removed
  }

  GetByID(id: string): Claim | undefined {
    for (const claims of Object.values(this) as Claim[][]) {
      for (const claim of claims || []) {
        if (claim.GetID() === id) {
          return claim
        }
        const c = claim.GetByID(id)
        if (c) {
          return c
        }
      }
    }
  }

  RemoveByID(id: string): Claim | undefined {
    for (const claims of Object.values(this) as Claim[][]) {
      for (const [i, claim] of (claims || []).entries()) {
        if (claim.GetID() === id) {
          claims.splice(i, 1)
          return claim
        }
        const c = claim.RemoveByID(id)
        if (c) {
          return c
        }
      }
    }
  }

  Add(claim: Claim): void {
    for (const [name, claimType] of Object.entries(CLAIM_TYPES_MAP) as ClaimTypesEntry[]) {
      if (claim instanceof claimType) {
        if (!this[name]) {
          this[name] = []
        }
        ;(this[name] as Array<Constructee<typeof claimType>>).push(claim)
        return
      }
    }
  }

  Size(): number {
    let s = 0
    for (const name of Object.keys(CLAIM_TYPES_MAP) as ClaimTypeName[]) {
      s += this[name]?.length ?? 0
    }
    return s
  }

  AllClaims(): Claim[] {
    return (Object.keys(CLAIM_TYPES_MAP) as ClaimTypeName[]).flatMap((k) => this[k] ?? [])
  }

  // Validate validates all claims, including nested meta claims.
  async Validate(): Promise<void> {
    const ids = new Set<string>()
    for (const claim of this.AllClaims()) {
      if (ids.has(claim.GetID())) {
        throw new Error(`duplicate claim ID: ${claim.GetID()}`)
      }
      ids.add(claim.GetID())
      await claim.Validate()
    }
  }
}

export type ClaimTypeName = keyof typeof CLAIM_TYPES_MAP
type ClaimTypeConstructor = (typeof CLAIM_TYPES_MAP)[ClaimTypeName]
type ClaimTypesEntry = [ClaimTypeName, ClaimTypeConstructor]

// CLAIM_TYPES_MAP maps claim type JSON keys to their constructors.
// Order matches the backend.
export const CLAIM_TYPES_MAP: {
  [P in keyof ClaimTypes as ClaimTypes[P] extends CoreClaim[] | undefined ? P : never]-?: ClaimTypes[P] extends Array<infer U> | undefined ? Constructor<U> : never
} = {
  id: IdentifierClaim,
  string: StringClaim,
  html: HTMLClaim,
  amount: AmountClaim,
  amountInterval: AmountIntervalClaim,
  time: TimeClaim,
  timeInterval: TimeIntervalClaim,
  link: LinkClaim,
  ref: ReferenceClaim,
  has: HasClaim,
  none: NoneClaim,
  unknown: UnknownClaim,
} as const

// Claim is the union of all claim types.
export type Claim = Constructee<(typeof CLAIM_TYPES_MAP)[ClaimTypeName]>

// ClaimForType extracts the claim class for a given claim type name.
export type ClaimForType<T extends ClaimTypeName> = Constructee<(typeof CLAIM_TYPES_MAP)[T]>

// getClaimsOfType returns all claims of a given type for the specified property ID(s),
// sorted by decreasing confidence.
export function getClaimsOfType<K extends ClaimTypeName>(
  claimTypes: DeepReadonly<ClaimTypes> | undefined | null,
  claimType: K,
  propertyId: string | string[],
): Required<DeepReadonly<ClaimTypes>>[K][number][] {
  if (!claimTypes) return []
  if (!Array.isArray(propertyId)) {
    propertyId = [propertyId]
  }
  const claims = []
  for (const claim of claimTypes[claimType] ?? []) {
    if (propertyId.includes(claim.prop.id)) {
      claims.push(claim)
    }
  }
  claims.sort((a, b) => b.confidence - a.confidence)
  return claims
}

// getBestClaimOfType returns the highest-confidence claim of a given type for the
// specified property ID(s), or null if none found.
export function getBestClaimOfType<K extends ClaimTypeName>(
  claimTypes: DeepReadonly<ClaimTypes> | undefined | null,
  claimType: K,
  propertyId: string | string[],
): Required<DeepReadonly<ClaimTypes>>[K][number] | null {
  const claims = getClaimsOfType(claimTypes, claimType, propertyId)
  if (claims.length > 0) {
    return claims[0]
  }
  return null
}

// getAllClaimsOfType returns all claims of a given type across all properties,
// sorted by decreasing confidence.
export function getAllClaimsOfType<K extends ClaimTypeName>(
  claimTypes: DeepReadonly<ClaimTypes> | undefined | null,
  claimType: K,
): Required<DeepReadonly<ClaimTypes>>[K][number][] {
  if (!claimTypes) return []
  const claims = [...(claimTypes[claimType] ?? [])]
  claims.sort((a, b) => b.confidence - a.confidence)
  return claims
}

// getClaimsOfTypeWithConfidence returns claims of a given type for the specified
// property ID(s), filtered by minimum confidence, sorted by decreasing confidence.
// TODO: Support also negation claims (i.e., those with negative confidence).
export function getClaimsOfTypeWithConfidence<K extends ClaimTypeName>(
  claimTypes: DeepReadonly<ClaimTypes> | undefined | null,
  claimType: K,
  propertyId: string | string[],
  confidence: Confidence = LowConfidence,
): Required<DeepReadonly<ClaimTypes>>[K][number][] {
  const claims = getClaimsOfType(claimTypes, claimType, propertyId)
  return claims.filter((claim) => claim.confidence >= confidence)
}

// getAllClaimsOfTypeWithConfidence returns all claims of a given type across all
// properties, filtered by minimum confidence, sorted by decreasing confidence.
// TODO: Support also negation claims (i.e., those with negative confidence).
export function getAllClaimsOfTypeWithConfidence<K extends ClaimTypeName>(
  claimTypes: DeepReadonly<ClaimTypes> | undefined | null,
  claimType: K,
  confidence: Confidence = LowConfidence,
): Required<DeepReadonly<ClaimTypes>>[K][number][] {
  const claims = getAllClaimsOfType(claimTypes, claimType)
  return claims.filter((claim) => claim.confidence >= confidence)
}

// getClaimsListsOfType groups claims by their LIST meta-claim and sorts within
// each list by the ORDER_IN_LIST meta-claim. Returns an array of lists, where each
// list is an array of claims sorted by order.
// TODO: Handle sub-lists. Children lists should be nested and not just added as additional lists to the list of lists.
// TODO: Sort lists between themselves by (average) confidence?
export function getClaimsListsOfType<K extends ClaimTypeName>(
  claimTypes: DeepReadonly<ClaimTypes> | undefined | null,
  claimType: K,
  propertyId: string | string[],
): Required<DeepReadonly<ClaimTypes>>[K][number][][] {
  const claims = getClaimsOfType(claimTypes, claimType, propertyId)
  const claimsPerList: Record<string, [Required<DeepReadonly<ClaimTypes>>[K][number], number][]> = {}
  for (const claim of claims) {
    const list = getBestClaimOfType(claim.meta, "id", LIST)?.value || "none"
    const order = parseFloat(getBestClaimOfType(claim.meta, "amount", ORDER_IN_LIST)?.amount ?? "") || Number.MAX_VALUE
    if (!(list in claimsPerList)) {
      claimsPerList[list] = []
    }
    claimsPerList[list].push([claim, order])
  }
  const res = []
  for (const c of Object.values(claimsPerList)) {
    res.push(c.sort(([_c1, o1], [_c2, o2]) => o1 - o2).map(([cl]) => cl))
  }
  return res
}

// Undetermined language code for claims without a specific language.
export const UNDETERMINED_LANGUAGE = "und"

// extractClaimLanguages extracts language codes from a claim's meta IN_LANGUAGE references.
// Returns [UNDETERMINED_LANGUAGE] if no languages are specified or none can be resolved.
function extractClaimLanguages(meta: DeepReadonly<ClaimTypes> | undefined | null): string[] {
  const refs = getClaimsOfTypeWithConfidence(meta, "ref", IN_LANGUAGE)
  const supportedLanguages = siteContext.languagePriority
  const codes: string[] = []
  for (const ref of refs) {
    const code = siteContext.languageCodes[ref.to.id]
    if (code && code in supportedLanguages) {
      codes.push(code)
    }
  }
  if (codes.length === 0) {
    return [UNDETERMINED_LANGUAGE]
  }
  return codes
}

// getClaimsAndLanguageOfTypeWithConfidence returns claims of a given type for the specified
// property ID(s), filtered by minimum confidence, sorted by decreasing confidence,
// grouped by language.
export function getClaimsAndLanguageOfTypeWithConfidence<K extends ClaimTypeName>(
  claimTypes: DeepReadonly<ClaimTypes> | undefined | null,
  claimType: K,
  propertyId: string | string[],
  confidence: Confidence = LowConfidence,
): Record<string, Required<DeepReadonly<ClaimTypes>>[K][number][]> {
  const claims: Record<string, { confidence: number; value: Required<DeepReadonly<ClaimTypes>>[K][number] }[]> = {}
  for (const claim of getClaimsOfTypeWithConfidence(claimTypes, claimType, propertyId, confidence)) {
    for (const lang of extractClaimLanguages(claim.meta)) {
      if (!claims[lang]) {
        claims[lang] = []
      }
      claims[lang].push({ confidence: claim.confidence, value: claim })
    }
  }
  const result: Record<string, Required<DeepReadonly<ClaimTypes>>[K][number][]> = {}
  for (const lang in claims) {
    // Sort by decreasing confidence.
    claims[lang].sort((a, b) => b.confidence - a.confidence)
    result[lang] = claims[lang].map((c) => c.value)
  }
  return result
}

// getFallbackLanguages returns the fallback language chain for a given language.
// If the language has an entry in languagePriority, that entry is used.
// Otherwise, the fallback is the undetermined language (unless the language is itself undetermined).
function getFallbackLanguages(lang: string): string[] {
  const fallbacks = siteContext.languagePriority[lang]
  if (fallbacks) {
    return fallbacks
  }
  if (lang !== UNDETERMINED_LANGUAGE) {
    return [UNDETERMINED_LANGUAGE]
  }
  return []
}

export function selectClaimsByLanguage<K extends ClaimTypeName>(
  claimTypes: DeepReadonly<ClaimTypes> | undefined | null,
  claimType: K,
  propertyId: string | string[],
  language: string,
  selector: (claims: Required<DeepReadonly<ClaimTypes>>[K][number][]) => boolean,
  confidence: Confidence = LowConfidence,
): Required<DeepReadonly<ClaimTypes>>[K][number][] | null {
  const claimsWithLanguage = getClaimsAndLanguageOfTypeWithConfidence(claimTypes, claimType, propertyId, confidence)
  const chain = [language, ...getFallbackLanguages(language)]
  for (const tryLang of chain) {
    const claims = claimsWithLanguage[tryLang]
    if (!claims) {
      continue
    }
    if (selector(claims)) {
      return claims
    }
  }
  return null
}
