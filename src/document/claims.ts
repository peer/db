import type { DeepReadonly } from "vue"

import type { Amount, Confidence, Reference, Time, TimePrecision } from "@/document/types"
import type { Constructee, Constructor, Required } from "@/types"

import siteContext from "@/context"
import { IN_LANGUAGE, LIST, ORDER_IN_LIST } from "@/core"
import { amountFloat64, amountWindowEnd, amountWindowStart, validateAmount } from "@/document/amount"
import { LowConfidence } from "@/document/confidence"
import { timeFloat64, timeWindowEnd, timeWindowStart, VALID_TIME_PRECISIONS, validateTime } from "@/document/time"

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
  sub?: ClaimTypes

  GetID(): string {
    return this.id
  }

  GetConfidence(): Confidence {
    return this.confidence
  }

  Get(propID: string): Claim[] {
    if (this.sub === undefined) {
      return []
    }
    return this.sub.Get(propID)
  }

  Remove(propID: string): Claim[] {
    if (this.sub === undefined) {
      return []
    }
    return this.sub.Remove(propID)
  }

  GetByID(id: string): Claim | undefined {
    if (this.sub === undefined) {
      return
    }
    return this.sub.GetByID(id)
  }

  RemoveByID(id: string): Claim | undefined {
    if (this.sub === undefined) {
      return
    }
    return this.sub.RemoveByID(id)
  }

  Add(claim: Claim): void {
    if (this.sub === undefined) {
      this.sub = new ClaimTypes({})
    }
    this.sub.Add(claim)
  }

  Size(): number {
    if (this.sub === undefined) {
      return 0
    }
    return this.sub.Size()
  }

  AllClaims(): Claim[] {
    if (this.sub === undefined) {
      return []
    }
    return this.sub.AllClaims()
  }

  async Validate(): Promise<void> {
    if (this.confidence < -1 || this.confidence > 1 || !isFinite(this.confidence)) {
      throw new Error("confidence out of range [-1, 1]")
    }
    if (this.sub !== undefined) {
      await this.sub.Validate()
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
    if (this.sub !== undefined) {
      this.sub = new ClaimTypes(this.sub)
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
    if (this.sub !== undefined) {
      this.sub = new ClaimTypes(this.sub)
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
    if (this.sub !== undefined) {
      this.sub = new ClaimTypes(this.sub)
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
    if (this.sub !== undefined) {
      this.sub = new ClaimTypes(this.sub)
    }
  }

  // Validate checks that the amount claim has valid amount, precision, and confidence.
  async Validate(): Promise<void> {
    await super.Validate()
    if (!isFinite(this.precision) || this.precision <= 0) {
      throw new Error(`Precision must be a finite positive number`)
    }
    validateAmount(this.amount, this.precision)
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
  toIsOpen?: boolean
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
    if (this.sub !== undefined) {
      this.sub = new ClaimTypes(this.sub)
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
      validateAmount(this.from!, this.fromPrecision)
    }

    let toIsCount = 0
    if (this.toIsOpen) toIsCount++
    if (this.toIsUnknown) toIsCount++
    if (this.toIsNone) toIsCount++
    if (toIsCount > 1) {
      throw new Error("only one of ToIsOpen, ToIsUnknown, ToIsNone can be set")
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
      validateAmount(this.to!, this.toPrecision)
    }

    // Empty-interval check. The swap criterion matches convertAmountInterval,
    // so the orientation here is the same as the indexed range.
    //
    // When both bounds share the same precision, the directed-decreasing
    // interpretation is unambiguous, so we use the simpler value-based
    // criterion: swap iff fromValue > toValue. After the swap the
    // orientation is ascending and the empty check is a forward
    // start(from) >= end(to).
    //
    // When precisions differ, value comparison would conflict with the
    // "precision-coarsening" pattern. In that case we fall back to the
    // un-swapped-empty criterion: only swap when the un-swapped form is
    // empty. If the swapped form is also empty, the interval is genuinely
    // empty.
    if (this.from !== undefined && this.to !== undefined && this.fromPrecision !== undefined && this.toPrecision !== undefined) {
      if (this.fromPrecision === this.toPrecision) {
        const fromValue = amountFloat64(this.from, this.fromPrecision)
        const toValue = amountFloat64(this.to, this.toPrecision)
        let loVal = this.from
        let loIsOpen = !!this.fromIsOpen
        let hiVal = this.to
        let hiIsOpen = !!this.toIsOpen
        if (fromValue > toValue) {
          loVal = this.to
          loIsOpen = !!this.toIsOpen
          hiVal = this.from
          hiIsOpen = !!this.fromIsOpen
        }
        const start = amountWindowStart(loVal, this.fromPrecision, loIsOpen)
        const end = amountWindowEnd(hiVal, this.fromPrecision, hiIsOpen)
        if (start >= end) {
          throw new Error("interval is empty")
        }
      } else {
        let start = amountWindowStart(this.from, this.fromPrecision, !!this.fromIsOpen)
        let end = amountWindowEnd(this.to, this.toPrecision, !!this.toIsOpen)
        if (start >= end) {
          start = amountWindowStart(this.to, this.toPrecision, !!this.toIsOpen)
          end = amountWindowEnd(this.from, this.fromPrecision, !!this.fromIsOpen)
          if (start >= end) {
            throw new Error("interval is empty")
          }
        }
      }
    }
  }
}

export class TimeClaim extends CoreClaim {
  prop!: Reference
  time!: Time
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
    if (!this.time) {
      throw new Error("time is required")
    }
    if (this.precision === undefined) {
      throw new Error("precision is required")
    }
    if (this.sub !== undefined) {
      this.sub = new ClaimTypes(this.sub)
    }
  }

  // Validate checks that the time claim has a valid precision, time, and valid confidence.
  async Validate(): Promise<void> {
    await super.Validate()
    if (!VALID_TIME_PRECISIONS.has(this.precision)) {
      throw new Error("unknown Precision")
    }
    validateTime(this.time, this.precision)
  }
}

export class TimeIntervalClaim extends CoreClaim {
  prop!: Reference
  from?: Time
  fromPrecision?: TimePrecision
  fromIsOpen?: boolean
  fromIsUnknown?: boolean
  fromIsNone?: boolean
  to?: Time
  toPrecision?: TimePrecision
  toIsOpen?: boolean
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
    if (this.sub !== undefined) {
      this.sub = new ClaimTypes(this.sub)
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
      validateTime(this.from!, this.fromPrecision)
    }

    let toIsCount = 0
    if (this.toIsOpen) toIsCount++
    if (this.toIsUnknown) toIsCount++
    if (this.toIsNone) toIsCount++
    if (toIsCount > 1) {
      throw new Error("only one of ToIsOpen, ToIsUnknown, ToIsNone can be set")
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
      validateTime(this.to!, this.toPrecision)
    }

    // Empty-interval check. Same dual criterion as AmountIntervalClaim.Validate:
    // for same precision, swap on value (fromValue > toValue) then forward
    // empty check; for different precision, swap iff un-swapped form is
    // empty, with a swapped retry. Matches convertTimeInterval.
    if (this.from !== undefined && this.to !== undefined && this.fromPrecision !== undefined && this.toPrecision !== undefined) {
      if (this.fromPrecision === this.toPrecision) {
        const fromValue = timeFloat64(this.from, this.fromPrecision)
        const toValue = timeFloat64(this.to, this.toPrecision)
        let loVal = this.from
        let loIsOpen = !!this.fromIsOpen
        let hiVal = this.to
        let hiIsOpen = !!this.toIsOpen
        if (fromValue > toValue) {
          loVal = this.to
          loIsOpen = !!this.toIsOpen
          hiVal = this.from
          hiIsOpen = !!this.fromIsOpen
        }
        const start = timeWindowStart(loVal, this.fromPrecision, loIsOpen)
        const end = timeWindowEnd(hiVal, this.fromPrecision, hiIsOpen)
        if (start >= end) {
          throw new Error("interval is empty")
        }
      } else {
        let start = timeWindowStart(this.from, this.fromPrecision, !!this.fromIsOpen)
        let end = timeWindowEnd(this.to, this.toPrecision, !!this.toIsOpen)
        if (start >= end) {
          start = timeWindowStart(this.to, this.toPrecision, !!this.toIsOpen)
          end = timeWindowEnd(this.from, this.fromPrecision, !!this.fromIsOpen)
          if (start >= end) {
            throw new Error("interval is empty")
          }
        }
      }
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
    if (this.sub !== undefined) {
      this.sub = new ClaimTypes(this.sub)
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
    if (this.sub !== undefined) {
      this.sub = new ClaimTypes(this.sub)
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
    if (this.sub !== undefined) {
      this.sub = new ClaimTypes(this.sub)
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
    if (this.sub !== undefined) {
      this.sub = new ClaimTypes(this.sub)
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
    if (this.sub !== undefined) {
      this.sub = new ClaimTypes(this.sub)
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

  // Validate validates all claims, including nested sub-claims.
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
//
// Exported only for testing.
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
//
// Claim has to have at least LowConfidence confidence.
// TODO: Support also negation claims (i.e., those with negative confidence).
export function getBestClaimOfType<K extends ClaimTypeName>(
  claimTypes: DeepReadonly<ClaimTypes> | undefined | null,
  claimType: K,
  propertyId: string | string[],
): Required<DeepReadonly<ClaimTypes>>[K][number] | null {
  const claims = getClaimsOfTypeWithConfidence(claimTypes, claimType, propertyId)
  if (claims.length > 0) {
    return claims[0]
  }
  return null
}

// getAllClaimsOfType returns all claims of a given type across all properties,
// sorted by decreasing confidence.
//
// Exported only for testing.
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

// getClaimsListsOfType groups claims by their LIST sub-claim and sorts within
// each list by the ORDER_IN_LIST sub-claim. Returns an array of lists, where each
// list is an array of claims sorted by order.
//
// Claim has to have at least LowConfidence confidence.
// TODO: Support also negation claims (i.e., those with negative confidence).
// TODO: Handle sub-lists. Children lists should be nested and not just added as additional lists to the list of lists.
// TODO: Sort lists between themselves by (average) confidence?
export function getClaimsListsOfType<K extends ClaimTypeName>(
  claimTypes: DeepReadonly<ClaimTypes> | undefined | null,
  claimType: K,
  propertyId: string | string[],
): Required<DeepReadonly<ClaimTypes>>[K][number][][] {
  const claims = getClaimsOfTypeWithConfidence(claimTypes, claimType, propertyId)
  const claimsPerList: Record<string, [Required<DeepReadonly<ClaimTypes>>[K][number], number][]> = {}
  for (const claim of claims) {
    const list = getBestClaimOfType(claim.sub, "id", LIST)?.value || "none"
    const order = parseFloat(getBestClaimOfType(claim.sub, "amount", ORDER_IN_LIST)?.amount ?? "") || Number.MAX_VALUE
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

// UNDETERMINED_LANGUAGE is the language code used for claims without a specific language.
export const UNDETERMINED_LANGUAGE = "und"

// extractClaimLanguages extracts language codes from a claim's IN_LANGUAGE sub-claim references.
//
// It maps language document IDs to codes using languageCodes, and checks that the code
// is a key in languagePriority (i.e., an enabled language).
//
// Returns [UNDETERMINED_LANGUAGE] if no languages are specified or none can be resolved.
function extractClaimLanguages(sub: DeepReadonly<ClaimTypes> | undefined | null): string[] {
  const refs = getClaimsOfTypeWithConfidence(sub, "ref", IN_LANGUAGE)
  const codes: string[] = []
  const languageCodes = siteContext.languageCodes ?? {}
  const languagePriority = siteContext.languagePriority ?? {}
  for (const ref of refs) {
    const code = languageCodes[ref.to.id]
    if (code && code in languagePriority) {
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
  const grouped: Record<string, { confidence: number; value: Required<DeepReadonly<ClaimTypes>>[K][number] }[]> = {}
  for (const claim of getClaimsOfTypeWithConfidence(claimTypes, claimType, propertyId, confidence)) {
    for (const lang of extractClaimLanguages(claim.sub)) {
      if (!grouped[lang]) {
        grouped[lang] = []
      }
      grouped[lang].push({ confidence: claim.confidence, value: claim })
    }
  }
  const result: Record<string, Required<DeepReadonly<ClaimTypes>>[K][number][]> = {}
  for (const lang in grouped) {
    // Sort by decreasing confidence.
    grouped[lang].sort((a, b) => b.confidence - a.confidence)
    result[lang] = grouped[lang].map((c) => c.value)
  }
  return result
}

// getFallbackLanguages returns the fallback language chain for a given language.
//
// If the language has an entry in languagePriority, that entry is used.
// Otherwise, the fallback is the undetermined language (unless the language is itself undetermined).
function getFallbackLanguages(lang: string): string[] {
  const fallbacks = (siteContext.languagePriority ?? {})[lang]
  if (fallbacks) {
    return fallbacks
  }
  // Default: try undetermined language, unless lang is already undetermined.
  if (lang !== UNDETERMINED_LANGUAGE) {
    return [UNDETERMINED_LANGUAGE]
  }
  return []
}

// selectClaimsByLanguage selects claims of a given type for the specified property IDs,
// filtered by minimum confidence, using the language fallback chain. It returns the
// first set of claims (grouped by language) for which the selector returns true,
// walking the language chain in order. Returns null if no language produces a match.
export function selectClaimsByLanguage<K extends ClaimTypeName>(
  claimTypes: DeepReadonly<ClaimTypes> | undefined | null,
  claimType: K,
  propertyId: string | string[],
  language: string,
  selector: (claims: Required<DeepReadonly<ClaimTypes>>[K][number][]) => boolean,
  confidence: Confidence = LowConfidence,
): Required<DeepReadonly<ClaimTypes>>[K][number][] | null {
  const claimsByLanguage = getClaimsAndLanguageOfTypeWithConfidence(claimTypes, claimType, propertyId, confidence)
  const chain = [language, ...getFallbackLanguages(language)]
  for (const tryLang of chain) {
    const claims = claimsByLanguage[tryLang]
    if (!claims) {
      continue
    }
    if (selector(claims)) {
      return claims
    }
  }
  return null
}
