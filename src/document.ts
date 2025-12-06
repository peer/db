import type { TranslatableHTMLString, AmountUnit, TimePrecision, Constructee, Constructor } from "@/types"

import { Identifier } from "@tozd/identifier"
import { v5 as uuidv5 } from "uuid"

// TODO: Why does having a constructor only in CoreClaim not assign also child class properties?

class CoreClaim implements ClaimsContainer {
  id!: string
  confidence!: number
  meta?: ClaimTypes

  GetID(): string {
    return this.id
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
}

type DocumentReference = {
  id: string
}

class IdentifierClaim extends CoreClaim {
  readonly type = "id"

  prop!: DocumentReference
  value!: string

  constructor(obj: object) {
    super()
    Object.assign(this, obj)
    if (this.meta !== undefined) {
      this.meta = new ClaimTypes(this.meta)
    }
  }
}

class ReferenceClaim extends CoreClaim {
  readonly type = "ref"

  prop!: DocumentReference
  iri!: string

  constructor(obj: object) {
    super()
    Object.assign(this, obj)
    if (this.meta !== undefined) {
      this.meta = new ClaimTypes(this.meta)
    }
  }
}

class TextClaim extends CoreClaim {
  readonly type = "text"

  prop!: DocumentReference
  html!: TranslatableHTMLString

  constructor(obj: object) {
    super()
    Object.assign(this, obj)
    if (this.meta !== undefined) {
      this.meta = new ClaimTypes(this.meta)
    }
  }
}

class StringClaim extends CoreClaim {
  readonly type = "string"

  prop!: DocumentReference
  string!: string

  constructor(obj: object) {
    super()
    Object.assign(this, obj)
    if (this.meta !== undefined) {
      this.meta = new ClaimTypes(this.meta)
    }
  }
}

class AmountClaim extends CoreClaim {
  readonly type = "amount"

  prop!: DocumentReference
  amount!: number
  unit!: AmountUnit

  constructor(obj: object) {
    super()
    Object.assign(this, obj)
    if (this.meta !== undefined) {
      this.meta = new ClaimTypes(this.meta)
    }
  }
}

class AmountRangeClaim extends CoreClaim {
  readonly type = "amountRange"

  prop!: DocumentReference
  lower!: number
  upper!: number
  unit!: AmountUnit

  constructor(obj: object) {
    super()
    Object.assign(this, obj)
    if (this.meta !== undefined) {
      this.meta = new ClaimTypes(this.meta)
    }
  }
}

class RelationClaim extends CoreClaim {
  readonly type = "rel"

  prop!: DocumentReference
  to!: DocumentReference

  constructor(obj: object) {
    super()
    Object.assign(this, obj)
    if (this.meta !== undefined) {
      this.meta = new ClaimTypes(this.meta)
    }
  }
}

class FileClaim extends CoreClaim {
  readonly type = "file"

  prop!: DocumentReference
  mediaType!: string
  url!: string
  preview?: string[]

  constructor(obj: object) {
    super()
    Object.assign(this, obj)
    if (this.meta !== undefined) {
      this.meta = new ClaimTypes(this.meta)
    }
  }
}

class NoValueClaim extends CoreClaim {
  readonly type = "none"

  prop!: DocumentReference

  constructor(obj: object) {
    super()
    Object.assign(this, obj)
    if (this.meta !== undefined) {
      this.meta = new ClaimTypes(this.meta)
    }
  }
}

class UnknownValueClaim extends CoreClaim {
  readonly type = "unknown"

  prop!: DocumentReference

  constructor(obj: object) {
    super()
    Object.assign(this, obj)
    if (this.meta !== undefined) {
      this.meta = new ClaimTypes(this.meta)
    }
  }
}

class TimeClaim extends CoreClaim {
  readonly type = "time"

  prop!: DocumentReference
  timestamp!: string
  precision!: TimePrecision

  constructor(obj: object) {
    super()
    Object.assign(this, obj)
    if (this.meta !== undefined) {
      this.meta = new ClaimTypes(this.meta)
    }
  }
}

class TimeRangeClaim extends CoreClaim {
  readonly type = "timeRange"

  prop!: DocumentReference
  lower!: string
  upper!: string
  precision!: TimePrecision

  constructor(obj: object) {
    super()
    Object.assign(this, obj)
    if (this.meta !== undefined) {
      this.meta = new ClaimTypes(this.meta)
    }
  }
}

export class ClaimTypes {
  id?: IdentifierClaim[]
  ref?: ReferenceClaim[]
  text?: TextClaim[]
  string?: StringClaim[]
  amount?: AmountClaim[]
  amountRange?: AmountRangeClaim[]
  rel?: RelationClaim[]
  file?: FileClaim[]
  none?: NoValueClaim[]
  unknown?: UnknownValueClaim[]
  time?: TimeClaim[]
  timeRange?: TimeRangeClaim[]

  constructor(obj: Record<string, object> | ClaimTypes) {
    for (const [name, claimType] of Object.entries(CLAIM_TYPES_MAP) as ClaimTypeEntry[]) {
      if (!obj?.[name]) continue
      if (!Array.isArray(obj[name])) throw new Error(`"${name}" is not an array`)
      ;(this[name] as Constructee<typeof claimType>[]) = obj[name].map((claim) => new claimType(claim))
    }
  }

  GetByID(id: string): Claim | undefined {
    for (const claims of Object.values(this)) {
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
    for (const claims of Object.values(this)) {
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
    for (const [name, claimType] of Object.entries(CLAIM_TYPES_MAP) as ClaimTypeEntry[]) {
      if (claim instanceof claimType) {
        if (!this[name]) {
          this[name] = []
        }
        ;(this[name] as Array<Constructee<typeof claimType>>).push(claim)
        return
      }
    }
  }
}

type ClaimTypeEntry = [keyof typeof CLAIM_TYPES_MAP, (typeof CLAIM_TYPES_MAP)[keyof typeof CLAIM_TYPES_MAP]]
export type ClaimTypeProp = keyof typeof CLAIM_TYPES_MAP

const CLAIM_TYPES_MAP: {
  [P in keyof ClaimTypes as ClaimTypes[P] extends CoreClaim[] | undefined ? P : never]-?: ClaimTypes[P] extends Array<infer U> | undefined ? Constructor<U> : never
} = {
  id: IdentifierClaim,
  ref: ReferenceClaim,
  text: TextClaim,
  string: StringClaim,
  amount: AmountClaim,
  amountRange: AmountRangeClaim,
  rel: RelationClaim,
  file: FileClaim,
  none: NoValueClaim,
  unknown: UnknownValueClaim,
  time: TimeClaim,
  timeRange: TimeRangeClaim,
} as const

export type Claim = Constructee<(typeof CLAIM_TYPES_MAP)[keyof typeof CLAIM_TYPES_MAP]>

export function claimFrom(obj: object, type: ClaimTypeProp): Claim {
  switch (type) {
    case "id":
      return new IdentifierClaim(obj)
    case "ref":
      return new ReferenceClaim(obj)
    case "text":
      return new TextClaim(obj)
    case "string":
      return new StringClaim(obj)
    case "amount":
      return new AmountClaim(obj)
    case "amountRange":
      return new AmountRangeClaim(obj)
    case "rel":
      return new RelationClaim(obj)
    case "file":
      return new FileClaim(obj)
    case "none":
      return new NoValueClaim(obj)
    case "unknown":
      return new UnknownValueClaim(obj)
    case "time":
      return new TimeClaim(obj)
    case "timeRange":
      return new TimeRangeClaim(obj)
  }
  // @ts-expect-error all types should be handled above
  throw new Error(`claim of type "${type}" is not supported`)
}

// TODO: Sync interface with Go implementation.
interface ClaimsContainer {
  GetID(): string
  GetByID(id: string): Claim | undefined
  RemoveByID(id: string): Claim | undefined
  Add(claim: Claim): void
}

export class PeerDBDocument implements ClaimsContainer {
  id!: string
  // Score is optional on the frontend because
  // search results do not have it initially.
  score?: number
  scores?: Record<string, number>
  mnemonic?: string
  claims?: ClaimTypes

  constructor(obj: object) {
    Object.assign(this, obj)
    if (this.claims !== undefined) {
      this.claims = new ClaimTypes(this.claims)
    }
  }

  GetID(): string {
    return this.id
  }

  GetByID(id: string): Claim | undefined {
    if (this.claims === undefined) {
      return
    }

    return this.claims.GetByID(id)
  }

  RemoveByID(id: string): Claim | undefined {
    if (this.claims === undefined) {
      return
    }

    return this.claims.RemoveByID(id)
  }

  Add(claim: Claim): void {
    if (this.claims === undefined) {
      this.claims = new ClaimTypes({})
    }

    this.claims.Add(claim)
  }
}

export interface Change {
  Apply(doc: PeerDBDocument, id: string): void
}

export function changeFrom(obj: object): Change {
  if (!("type" in obj)) {
    throw new Error(`change missing type`)
  }
  switch (obj.type) {
    case "add":
      return new AddClaimChange(obj)
    case "set":
      return new SetClaimChange(obj)
    case "remove":
      return new RemoveClaimChange(obj)
  }
  throw new Error(`change of type "${obj.type}" is not supported`)
}

export class AddClaimChange implements Change {
  type: "add"
  under?: string
  patch!: ClaimPatch

  constructor(obj: object) {
    if ("type" in obj && obj.type !== "add") {
      throw new Error(`invalid type "${obj.type}"`)
    }
    this.type = "add"
    Object.assign(this, obj)
    this.patch = claimPatchFrom(this.patch)
  }

  Apply(doc: PeerDBDocument, id: string): void {
    const newClaim = this.patch.New(id)

    if (this.under === undefined) {
      doc.Add(newClaim)
      return
    }

    const claim = doc.GetByID(this.under)
    if (!claim) {
      throw new Error(`claim with ID "${this.under}" not found`)
    }

    claim.Add(newClaim)
  }
}

export class SetClaimChange implements Change {
  type: "set"
  id!: string
  patch!: ClaimPatch

  constructor(obj: object) {
    if ("type" in obj && obj.type !== "set") {
      throw new Error(`invalid type "${obj.type}"`)
    }
    this.type = "set"
    Object.assign(this, obj)
    this.patch = claimPatchFrom(this.patch)
  }

  Apply(doc: PeerDBDocument, id: string): void {
    const claim = doc.GetByID(this.id)
    if (!claim) {
      throw new Error(`claim with ID "${this.id}" not found`)
    }
    this.patch.Apply(claim)
  }
}

export class RemoveClaimChange implements Change {
  type: "remove"
  id!: string

  constructor(obj: object) {
    if ("type" in obj && obj.type !== "remove") {
      throw new Error(`invalid type "${obj.type}"`)
    }
    this.type = "remove"
    Object.assign(this, obj)
  }

  Apply(doc: PeerDBDocument, id: string): void {
    const claim = doc.RemoveByID(this.id)
    if (!claim) {
      throw new Error(`claim with ID "${this.id}" not found`)
    }
  }
}

export function idAtChange(base: string, i: number): string {
  // TODO: Allow exposing data from Identifier.
  const namespace = (Identifier.fromString(base) as unknown as { value: Uint8Array }).value
  const res = uuidv5(String(i), namespace)
  return Identifier.fromUUID(res).toString()
}

export class Changes implements Change {
  changes: Change[]

  constructor(...objs: object[]) {
    this.changes = objs.map(changeFrom)
  }

  Apply(doc: PeerDBDocument, base: string): void {
    for (const [i, change] of this.changes.entries()) {
      const id = idAtChange(base, i)
      change.Apply(doc, id)
    }
  }

  toJSON(): Change[] {
    return this.changes
  }
}

interface ClaimPatch {
  New(id: string): Claim
  Apply(claim: Claim): void
}

export function claimPatchFrom(obj: object): ClaimPatch {
  if (!("type" in obj)) {
    throw new Error(`patch missing type`)
  }
  switch (obj.type) {
    case "id":
      return new IdentifierClaimPatch(obj)
    case "ref":
      return new ReferenceClaimPatch(obj)
    case "text":
      return new TextClaimPatch(obj)
    case "string":
      return new StringClaimPatch(obj)
    case "amount":
      return new AmountClaimPatch(obj)
    case "amountRange":
      return new AmountRangeClaimPatch(obj)
    case "rel":
      return new RelationClaimPatch(obj)
    case "file":
      return new FileClaimPatch(obj)
    case "none":
      return new NoValueClaimPatch(obj)
    case "unknown":
      return new UnknownValueClaimPatch(obj)
    case "time":
      return new TimeClaimPatch(obj)
    case "timeRange":
      return new TimeRangeClaimPatch(obj)
  }
  throw new Error(`patch of type "${obj.type}" is not supported`)
}

export class IdentifierClaimPatch implements ClaimPatch {
  type: "id"
  prop?: string
  value?: string

  constructor(obj: object) {
    if ("type" in obj && obj.type !== "id") {
      throw new Error(`invalid type "${obj.type}"`)
    }
    this.type = "id"
    Object.assign(this, obj)
  }

  New(id: string): Claim {
    if (this.prop === undefined || this.value === undefined) {
      throw new Error("incomplete patch")
    }

    return new IdentifierClaim({
      id: id,
      confidence: 1.0, // TODO How to make it configurable?
      prop: {
        id: this.prop,
      },
      value: this.value,
    })
  }

  Apply(claim: Claim): void {
    if (this.prop === undefined && this.value === undefined) {
      throw new Error("empty patch")
    }

    if (!(claim instanceof IdentifierClaim)) {
      throw new Error("not identifier claim")
    }

    if (this.prop !== undefined) {
      claim.prop.id = this.prop
    }
    if (this.value !== undefined) {
      claim.value = this.value
    }
  }
}

export class ReferenceClaimPatch implements ClaimPatch {
  type: "ref"
  prop?: string
  iri?: string

  constructor(obj: object) {
    if ("type" in obj && obj.type !== "ref") {
      throw new Error(`invalid type "${obj.type}"`)
    }
    this.type = "ref"
    Object.assign(this, obj)
  }

  New(id: string): Claim {
    if (this.prop === undefined || this.iri === undefined) {
      throw new Error("incomplete patch")
    }

    return new ReferenceClaim({
      id: id,
      confidence: 1.0, // TODO How to make it configurable?
      prop: {
        id: this.prop,
      },
      iri: this.iri,
    })
  }

  Apply(claim: Claim): void {
    if (this.prop === undefined && this.iri === undefined) {
      throw new Error("empty patch")
    }

    if (!(claim instanceof ReferenceClaim)) {
      throw new Error("not reference claim")
    }

    if (this.prop !== undefined) {
      claim.prop.id = this.prop
    }
    if (this.iri !== undefined) {
      claim.iri = this.iri
    }
  }
}

export class TextClaimPatch implements ClaimPatch {
  type: "text"
  prop?: string
  html?: TranslatableHTMLString
  remove?: string[]

  constructor(obj: object) {
    if ("type" in obj && obj.type !== "text") {
      throw new Error(`invalid type "${obj.type}"`)
    }
    this.type = "text"
    Object.assign(this, obj)
  }

  New(id: string): Claim {
    // TODO: Check that there are properties in this.html.
    if (this.prop === undefined || this.html === undefined) {
      throw new Error("incomplete patch")
    }
    // TODO: Check that there are no items in this.remove, even if it exists.
    if (this.remove !== undefined) {
      throw new Error("invalid patch")
    }

    return new TextClaim({
      id: id,
      confidence: 1.0, // TODO How to make it configurable?
      prop: {
        id: this.prop,
      },
      html: this.html,
    })
  }

  Apply(claim: Claim): void {
    // TODO: Check that there are properties in this.html or items in this.remove.
    if (this.prop === undefined && this.html === undefined && this.remove === undefined) {
      throw new Error("empty patch")
    }

    if (!(claim instanceof TextClaim)) {
      throw new Error("not text claim")
    }

    if (this.prop !== undefined) {
      claim.prop.id = this.prop
    }
    for (const lang of this.remove || []) {
      delete claim.html[lang]
    }
    for (const [lang, value] of Object.entries(this.html || {})) {
      claim.html[lang] = value
    }
  }
}

export class StringClaimPatch implements ClaimPatch {
  type: "string"
  prop?: string
  string?: string

  constructor(obj: object) {
    if ("type" in obj && obj.type !== "string") {
      throw new Error(`invalid type "${obj.type}"`)
    }
    this.type = "string"
    Object.assign(this, obj)
  }

  New(id: string): Claim {
    if (this.prop === undefined || this.string === undefined) {
      throw new Error("incomplete patch")
    }

    return new StringClaim({
      id: id,
      confidence: 1.0, // TODO How to make it configurable?
      prop: {
        id: this.prop,
      },
      string: this.string,
    })
  }

  Apply(claim: Claim): void {
    if (this.prop === undefined && this.string === undefined) {
      throw new Error("empty patch")
    }

    if (!(claim instanceof StringClaim)) {
      throw new Error("not string claim")
    }

    if (this.prop !== undefined) {
      claim.prop.id = this.prop
    }
    if (this.string !== undefined) {
      claim.string = this.string
    }
  }
}

export class AmountClaimPatch implements ClaimPatch {
  type: "amount"
  prop?: string
  amount?: number
  unit?: AmountUnit

  constructor(obj: object) {
    if ("type" in obj && obj.type !== "amount") {
      throw new Error(`invalid type "${obj.type}"`)
    }
    this.type = "amount"
    Object.assign(this, obj)
  }

  New(id: string): Claim {
    if (this.prop === undefined || this.amount === undefined || this.unit === undefined) {
      throw new Error("incomplete patch")
    }

    return new AmountClaim({
      id: id,
      confidence: 1.0, // TODO How to make it configurable?
      prop: {
        id: this.prop,
      },
      amount: this.amount,
      unit: this.unit,
    })
  }

  Apply(claim: Claim): void {
    if (this.prop === undefined && this.amount === undefined && this.unit === undefined) {
      throw new Error("empty patch")
    }

    if (!(claim instanceof AmountClaim)) {
      throw new Error("not amount claim")
    }

    if (this.prop !== undefined) {
      claim.prop.id = this.prop
    }
    if (this.amount !== undefined) {
      claim.amount = this.amount
    }
    if (this.unit !== undefined) {
      claim.unit = this.unit
    }
  }
}

export class AmountRangeClaimPatch implements ClaimPatch {
  type: "amountRange"
  prop?: string
  lower?: number
  upper?: number
  unit?: AmountUnit

  constructor(obj: object) {
    if ("type" in obj && obj.type !== "amountRange") {
      throw new Error(`invalid type "${obj.type}"`)
    }
    this.type = "amountRange"
    Object.assign(this, obj)
  }

  New(id: string): Claim {
    if (this.prop === undefined || this.lower === undefined || this.upper === undefined || this.unit === undefined) {
      throw new Error("incomplete patch")
    }

    return new AmountRangeClaim({
      id: id,
      confidence: 1.0, // TODO How to make it configurable?
      prop: {
        id: this.prop,
      },
      lower: this.lower,
      upper: this.upper,
      unit: this.unit,
    })
  }

  Apply(claim: Claim): void {
    if (this.prop === undefined && this.lower === undefined && this.upper === undefined && this.unit === undefined) {
      throw new Error("empty patch")
    }

    if (!(claim instanceof AmountRangeClaim)) {
      throw new Error("not amount range claim")
    }

    if (this.prop !== undefined) {
      claim.prop.id = this.prop
    }
    if (this.lower !== undefined) {
      claim.lower = this.lower
    }
    if (this.upper !== undefined) {
      claim.upper = this.upper
    }
    if (this.unit !== undefined) {
      claim.unit = this.unit
    }
  }
}

export class RelationClaimPatch implements ClaimPatch {
  type: "rel"
  prop?: string
  to?: string

  constructor(obj: object) {
    if ("type" in obj && obj.type !== "rel") {
      throw new Error(`invalid type "${obj.type}"`)
    }
    this.type = "rel"
    Object.assign(this, obj)
  }

  New(id: string): Claim {
    if (this.prop === undefined || this.to === undefined) {
      throw new Error("incomplete patch")
    }

    return new RelationClaim({
      id: id,
      confidence: 1.0, // TODO How to make it configurable?
      prop: {
        id: this.prop,
      },
      to: {
        id: this.to,
      },
    })
  }

  Apply(claim: Claim): void {
    if (this.prop === undefined && this.to === undefined) {
      throw new Error("empty patch")
    }

    if (!(claim instanceof RelationClaim)) {
      throw new Error("not relation claim")
    }

    if (this.prop !== undefined) {
      claim.prop.id = this.prop
    }
    if (this.to !== undefined) {
      claim.to.id = this.to
    }
  }
}

export class FileClaimPatch implements ClaimPatch {
  type: "file"
  prop?: string
  mediaType?: string
  url?: string
  preview?: string[]

  constructor(obj: object) {
    if ("type" in obj && obj.type !== "file") {
      throw new Error(`invalid type "${obj.type}"`)
    }
    this.type = "file"
    Object.assign(this, obj)
  }

  New(id: string): Claim {
    if (this.prop === undefined || this.mediaType === undefined || this.url === undefined || this.preview === undefined) {
      throw new Error("incomplete patch")
    }

    return new FileClaim({
      id: id,
      confidence: 1.0, // TODO How to make it configurable?
      prop: {
        id: this.prop,
      },
      mediaType: this.mediaType,
      url: this.url,
      preview: this.preview,
    })
  }

  Apply(claim: Claim): void {
    if (this.prop === undefined && this.mediaType === undefined && this.url === undefined && this.preview === undefined) {
      throw new Error("empty patch")
    }

    if (!(claim instanceof FileClaim)) {
      throw new Error("not file claim")
    }

    if (this.prop !== undefined) {
      claim.prop.id = this.prop
    }
    if (this.mediaType !== undefined) {
      claim.mediaType = this.mediaType
    }
    if (this.url !== undefined) {
      claim.url = this.url
    }
    if (this.preview !== undefined) {
      claim.preview = this.preview
    }
  }
}

export class NoValueClaimPatch implements ClaimPatch {
  type: "none"
  prop?: string

  constructor(obj: object) {
    if ("type" in obj && obj.type !== "none") {
      throw new Error(`invalid type "${obj.type}"`)
    }
    this.type = "none"
    Object.assign(this, obj)
  }

  New(id: string): Claim {
    if (this.prop === undefined) {
      throw new Error("incomplete patch")
    }

    return new NoValueClaim({
      id: id,
      confidence: 1.0, // TODO How to make it configurable?
      prop: {
        id: this.prop,
      },
    })
  }

  Apply(claim: Claim): void {
    if (this.prop === undefined) {
      throw new Error("empty patch")
    }

    if (!(claim instanceof NoValueClaim)) {
      throw new Error("not no value claim")
    }

    if (this.prop !== undefined) {
      claim.prop.id = this.prop
    }
  }
}

export class UnknownValueClaimPatch implements ClaimPatch {
  type: "unknown"
  prop?: string

  constructor(obj: object) {
    if ("type" in obj && obj.type !== "unknown") {
      throw new Error(`invalid type "${obj.type}"`)
    }
    this.type = "unknown"
    Object.assign(this, obj)
  }

  New(id: string): Claim {
    if (this.prop === undefined) {
      throw new Error("incomplete patch")
    }

    return new UnknownValueClaim({
      id: id,
      confidence: 1.0, // TODO How to make it configurable?
      prop: {
        id: this.prop,
      },
    })
  }

  Apply(claim: Claim): void {
    if (this.prop === undefined) {
      throw new Error("empty patch")
    }

    if (!(claim instanceof UnknownValueClaim)) {
      throw new Error("not unknown value claim")
    }

    if (this.prop !== undefined) {
      claim.prop.id = this.prop
    }
  }
}

export class TimeClaimPatch implements ClaimPatch {
  type: "time"
  prop?: string
  timestamp?: string
  precision?: TimePrecision

  constructor(obj: object) {
    if ("type" in obj && obj.type !== "time") {
      throw new Error(`invalid type "${obj.type}"`)
    }
    this.type = "time"
    Object.assign(this, obj)
  }

  New(id: string): Claim {
    if (this.prop === undefined || this.timestamp === undefined || this.precision === undefined) {
      throw new Error("incomplete patch")
    }

    return new TimeClaim({
      id: id,
      confidence: 1.0, // TODO How to make it configurable?
      prop: {
        id: this.prop,
      },
      timestamp: this.timestamp,
      precision: this.precision,
    })
  }

  Apply(claim: Claim): void {
    if (this.prop === undefined && this.timestamp === undefined && this.precision === undefined) {
      throw new Error("empty patch")
    }

    if (!(claim instanceof TimeClaim)) {
      throw new Error("not time claim")
    }

    if (this.prop !== undefined) {
      claim.prop.id = this.prop
    }
    if (this.timestamp !== undefined) {
      claim.timestamp = this.timestamp
    }
    if (this.precision !== undefined) {
      claim.precision = this.precision
    }
  }
}

export class TimeRangeClaimPatch implements ClaimPatch {
  type: "timeRange"
  prop?: string
  lower?: string
  upper?: string
  precision?: TimePrecision

  constructor(obj: object) {
    if ("type" in obj && obj.type !== "timeRange") {
      throw new Error(`invalid type "${obj.type}"`)
    }
    this.type = "timeRange"
    Object.assign(this, obj)
  }

  New(id: string): Claim {
    if (this.prop === undefined || this.lower === undefined || this.upper === undefined || this.precision === undefined) {
      throw new Error("incomplete patch")
    }

    return new TimeRangeClaim({
      id: id,
      confidence: 1.0, // TODO How to make it configurable?
      prop: {
        id: this.prop,
      },
      lower: this.lower,
      upper: this.upper,
      precision: this.precision,
    })
  }

  Apply(claim: Claim): void {
    if (this.prop === undefined && this.lower === undefined && this.upper === undefined && this.precision === undefined) {
      throw new Error("empty patch")
    }

    if (!(claim instanceof TimeRangeClaim)) {
      throw new Error("not time range claim")
    }

    if (this.prop !== undefined) {
      claim.prop.id = this.prop
    }
    if (this.lower !== undefined) {
      claim.lower = this.lower
    }
    if (this.upper !== undefined) {
      claim.upper = this.upper
    }
    if (this.precision !== undefined) {
      claim.precision = this.precision
    }
  }
}
