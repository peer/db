import type { TranslatableHTMLString, AmountUnit, TimePrecision } from "@/types"

import { Identifier } from "@tozd/identifier"
import { v5 as uuidv5 } from "uuid"

class CoreClaim implements ClaimsContainer {
  id!: string
  confidence!: number
  meta?: ClaimTypes

  constructor(obj: object) {
    Object.assign(this, obj)
    if (typeof this.meta !== "undefined") {
      this.meta = new ClaimTypes(this.meta)
    }
  }

  GetID(): string {
    return this.id
  }

  GetByID(id: string): Claim | undefined {
    if (typeof this.meta === "undefined") {
      return
    }

    return this.meta.GetByID(id)
  }

  RemoveByID(id: string): Claim | undefined {
    if (typeof this.meta === "undefined") {
      return
    }

    return this.meta.RemoveByID(id)
  }

  Add(claim: Claim): void {
    if (typeof this.meta === "undefined") {
      this.meta = new ClaimTypes({})
    }

    this.meta.Add(claim)
  }
}

type DocumentReference = {
  id: string
  score: number
}

class IdentifierClaim extends CoreClaim {
  prop!: DocumentReference
  value!: string
}

class ReferenceClaim extends CoreClaim {
  prop!: DocumentReference
  iri!: string
}

class TextClaim extends CoreClaim {
  prop!: DocumentReference
  html!: TranslatableHTMLString
}

class StringClaim extends CoreClaim {
  prop!: DocumentReference
  string!: string
}

class AmountClaim extends CoreClaim {
  prop!: DocumentReference
  amount!: number
  unit!: AmountUnit
}

class AmountRangeClaim extends CoreClaim {
  prop!: DocumentReference
  lower!: number
  upper!: number
  unit!: AmountUnit
}

class RelationClaim extends CoreClaim {
  prop!: DocumentReference
  to!: DocumentReference
}

class FileClaim extends CoreClaim {
  prop!: DocumentReference
  mediaType!: string
  url!: string
  preview?: string[]
}

class NoValueClaim extends CoreClaim {
  prop!: DocumentReference
}

class UnknownValueClaim extends CoreClaim {
  prop!: DocumentReference
}

class TimeClaim extends CoreClaim {
  prop!: DocumentReference
  timestamp!: string
  precision!: TimePrecision
}

class TimeRangeClaim extends CoreClaim {
  prop!: DocumentReference
  lower!: string
  upper!: string
  precision!: TimePrecision
}

const claimTypesMap = {
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
}

class ClaimTypes {
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

  constructor(obj: object) {
    Object.assign(this, obj)
    for (const [name, claimType] of Object.entries(claimTypesMap)) {
      for (const [i, claim] of (this[name] || []).entries()) {
        this[name][i] = new claimType(claim)
      }
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
    for (const [name, claimType] of Object.entries(claimTypesMap)) {
      if (claim instanceof claimType) {
        if (!this[name]) {
          this[name] = []
        }
        this[name].push(claim)
        return
      }
    }

    const exhaustiveCheck: never = claim
    throw new Error(`claim of type ${(exhaustiveCheck as object).constructor.name} is not supported`, claim)
  }
}

type Claim =
  | IdentifierClaim
  | ReferenceClaim
  | TextClaim
  | StringClaim
  | AmountClaim
  | AmountRangeClaim
  | RelationClaim
  | FileClaim
  | NoValueClaim
  | UnknownValueClaim
  | TimeClaim
  | TimeRangeClaim

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
    if (typeof this.claims !== "undefined") {
      this.claims = new ClaimTypes(this.claims)
    }
  }

  GetID(): string {
    return this.id
  }

  GetByID(id: string): Claim | undefined {
    if (typeof this.claims === "undefined") {
      return
    }

    return this.claims.GetByID(id)
  }

  RemoveByID(id: string): Claim | undefined {
    if (typeof this.claims === "undefined") {
      return
    }

    return this.claims.RemoveByID(id)
  }

  Add(claim: Claim): void {
    if (typeof this.claims === "undefined") {
      this.claims = new ClaimTypes({})
    }

    this.claims.Add(claim)
  }
}

interface Change {
  Apply(doc: PeerDBDocument, id: string): void
}

export function changeFrom(obj: object): Change {
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
    if (obj.type !== "add") {
      throw new Error(`invalid type "${obj.type}"`)
    }
    Object.assign(this, obj)
    this.patch = claimPatchFrom(this.patch)
  }

  Apply(doc: PeerDBDocument, id: string): void {
    const newClaim = this.patch.New(id)

    if (typeof this.under === "undefined") {
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
    if (obj.type !== "set") {
      throw new Error(`invalid type "${obj.type}"`)
    }
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
    if (obj.type !== "remove") {
      throw new Error(`invalid type "${obj.type}"`)
    }
    Object.assign(this, obj)
  }

  Apply(doc: PeerDBDocument, id: string): void {
    const claim = doc.RemoveByID(this.id)
    if (!claim) {
      throw new Error(`claim with ID "${this.id}" not found`)
    }
  }
}

export class Changes implements Change {
  changes: Change[]

  constructor(...objs: object[]) {
    this.changes = objs.map(changeFrom)
  }

  Apply(doc: PeerDBDocument, base: string): void {
    // TODO: Allow exposing data from Identifier.
    const namespace = (Identifier.fromString(base) as unknown as { value: Uint8Array }).value

    for (const [i, change] of this.changes.entries()) {
      const res = uuidv5(String(i), namespace)
      const id = Identifier.fromUUID(res).toString()
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
    if (obj.type !== "id") {
      throw new Error(`invalid type "${obj.type}"`)
    }
    Object.assign(this, obj)
  }

  New(id: string): Claim {
    if (typeof this.prop === "undefined" || typeof this.value === "undefined") {
      throw new Error("incomplete patch")
    }

    return new IdentifierClaim({
      id: id,
      confidence: 1.0, // TODO How to make it configurable?
      prop: {
        id: this.prop,
        score: 1.0, // TODO: Fetch if from the store?
      },
      value: this.value,
    })
  }

  Apply(claim: Claim): void {
    if (typeof this.prop === "undefined" && typeof this.value === "undefined") {
      throw new Error("empty patch")
    }

    if (!(claim instanceof IdentifierClaim)) {
      throw new Error("not identifier claim")
    }

    if (typeof this.prop !== "undefined") {
      claim.prop.id = this.prop
    }
    if (typeof this.value !== "undefined") {
      claim.value = this.value
    }
  }
}

export class ReferenceClaimPatch implements ClaimPatch {
  type: "ref"
  prop?: string
  iri?: string

  constructor(obj: object) {
    if (obj.type !== "ref") {
      throw new Error(`invalid type "${obj.type}"`)
    }
    Object.assign(this, obj)
  }

  New(id: string): Claim {
    if (typeof this.prop === "undefined" || typeof this.iri === "undefined") {
      throw new Error("incomplete patch")
    }

    return new ReferenceClaim({
      id: id,
      confidence: 1.0, // TODO How to make it configurable?
      prop: {
        id: this.prop,
        score: 1.0, // TODO: Fetch if from the store?
      },
      iri: this.iri,
    })
  }

  Apply(claim: Claim): void {
    if (typeof this.prop === "undefined" && typeof this.iri === "undefined") {
      throw new Error("empty patch")
    }

    if (!(claim instanceof ReferenceClaim)) {
      throw new Error("not reference claim")
    }

    if (typeof this.prop !== "undefined") {
      claim.prop.id = this.prop
    }
    if (typeof this.iri !== "undefined") {
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
    if (obj.type !== "text") {
      throw new Error(`invalid type "${obj.type}"`)
    }
    Object.assign(this, obj)
  }

  New(id: string): Claim {
    // TODO: Check that there are properties in this.html.
    if (typeof this.prop === "undefined" || typeof this.html === "undefined") {
      throw new Error("incomplete patch")
    }
    // TODO: Check that there are no items in this.remove, even if it exists.
    if (typeof this.remove !== "undefined") {
      throw new Error("invalid patch")
    }

    return new TextClaim({
      id: id,
      confidence: 1.0, // TODO How to make it configurable?
      prop: {
        id: this.prop,
        score: 1.0, // TODO: Fetch if from the store?
      },
      html: this.html,
    })
  }

  Apply(claim: Claim): void {
    // TODO: Check that there are properties in this.html or items in this.remove.
    if (typeof this.prop === "undefined" && typeof this.html === "undefined" && typeof this.remove === "undefined") {
      throw new Error("empty patch")
    }

    if (!(claim instanceof TextClaim)) {
      throw new Error("not text claim")
    }

    if (typeof this.prop !== "undefined") {
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
    if (obj.type !== "string") {
      throw new Error(`invalid type "${obj.type}"`)
    }
    Object.assign(this, obj)
  }

  New(id: string): Claim {
    if (typeof this.prop === "undefined" || typeof this.string === "undefined") {
      throw new Error("incomplete patch")
    }

    return new StringClaim({
      id: id,
      confidence: 1.0, // TODO How to make it configurable?
      prop: {
        id: this.prop,
        score: 1.0, // TODO: Fetch if from the store?
      },
      string: this.string,
    })
  }

  Apply(claim: Claim): void {
    if (typeof this.prop === "undefined" && typeof this.string === "undefined") {
      throw new Error("empty patch")
    }

    if (!(claim instanceof StringClaim)) {
      throw new Error("not string claim")
    }

    if (typeof this.prop !== "undefined") {
      claim.prop.id = this.prop
    }
    if (typeof this.string !== "undefined") {
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
    if (obj.type !== "amount") {
      throw new Error(`invalid type "${obj.type}"`)
    }
    Object.assign(this, obj)
  }

  New(id: string): Claim {
    if (typeof this.prop === "undefined" || typeof this.amount === "undefined" || typeof this.unit === "undefined") {
      throw new Error("incomplete patch")
    }

    return new AmountClaim({
      id: id,
      confidence: 1.0, // TODO How to make it configurable?
      prop: {
        id: this.prop,
        score: 1.0, // TODO: Fetch if from the store?
      },
      amount: this.amount,
      unit: this.unit,
    })
  }

  Apply(claim: Claim): void {
    if (typeof this.prop === "undefined" && typeof this.amount === "undefined" && typeof this.unit === "undefined") {
      throw new Error("empty patch")
    }

    if (!(claim instanceof AmountClaim)) {
      throw new Error("not amount claim")
    }

    if (typeof this.prop !== "undefined") {
      claim.prop.id = this.prop
    }
    if (typeof this.amount !== "undefined") {
      claim.amount = this.amount
    }
    if (typeof this.unit !== "undefined") {
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
    if (obj.type !== "amountRange") {
      throw new Error(`invalid type "${obj.type}"`)
    }
    Object.assign(this, obj)
  }

  New(id: string): Claim {
    if (typeof this.prop === "undefined" || typeof this.lower === "undefined" || typeof this.upper === "undefined" || typeof this.unit === "undefined") {
      throw new Error("incomplete patch")
    }

    return new AmountRangeClaim({
      id: id,
      confidence: 1.0, // TODO How to make it configurable?
      prop: {
        id: this.prop,
        score: 1.0, // TODO: Fetch if from the store?
      },
      lower: this.lower,
      upper: this.upper,
      unit: this.unit,
    })
  }

  Apply(claim: Claim): void {
    if (typeof this.prop === "undefined" && typeof this.lower === "undefined" && typeof this.upper === "undefined" && typeof this.unit === "undefined") {
      throw new Error("empty patch")
    }

    if (!(claim instanceof AmountRangeClaim)) {
      throw new Error("not amount range claim")
    }

    if (typeof this.prop !== "undefined") {
      claim.prop.id = this.prop
    }
    if (typeof this.lower !== "undefined") {
      claim.lower = this.lower
    }
    if (typeof this.upper !== "undefined") {
      claim.upper = this.upper
    }
    if (typeof this.unit !== "undefined") {
      claim.unit = this.unit
    }
  }
}

export class RelationClaimPatch implements ClaimPatch {
  type: "rel"
  prop?: string
  to?: string

  constructor(obj: object) {
    if (obj.type !== "rel") {
      throw new Error(`invalid type "${obj.type}"`)
    }
    Object.assign(this, obj)
  }

  New(id: string): Claim {
    if (typeof this.prop === "undefined" || typeof this.to === "undefined") {
      throw new Error("incomplete patch")
    }

    return new RelationClaim({
      id: id,
      confidence: 1.0, // TODO How to make it configurable?
      prop: {
        id: this.prop,
        score: 1.0, // TODO: Fetch if from the store?
      },
      to: {
        id: this.to,
        score: 1.0, // TODO: Fetch if from the store?
      },
    })
  }

  Apply(claim: Claim): void {
    if (typeof this.prop === "undefined" && typeof this.to === "undefined") {
      throw new Error("empty patch")
    }

    if (!(claim instanceof RelationClaim)) {
      throw new Error("not relation claim")
    }

    if (typeof this.prop !== "undefined") {
      claim.prop.id = this.prop
    }
    if (typeof this.to !== "undefined") {
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
    if (obj.type !== "file") {
      throw new Error(`invalid type "${obj.type}"`)
    }
    Object.assign(this, obj)
  }

  New(id: string): Claim {
    if (typeof this.prop === "undefined" || typeof this.mediaType === "undefined" || typeof this.url === "undefined" || typeof this.preview === "undefined") {
      throw new Error("incomplete patch")
    }

    return new FileClaim({
      id: id,
      confidence: 1.0, // TODO How to make it configurable?
      prop: {
        id: this.prop,
        score: 1.0, // TODO: Fetch if from the store?
      },
      mediaType: this.mediaType,
      url: this.url,
      preview: this.preview,
    })
  }

  Apply(claim: Claim): void {
    if (typeof this.prop === "undefined" && typeof this.mediaType === "undefined" && typeof this.url === "undefined" && typeof this.preview === "undefined") {
      throw new Error("empty patch")
    }

    if (!(claim instanceof FileClaim)) {
      throw new Error("not file claim")
    }

    if (typeof this.prop !== "undefined") {
      claim.prop.id = this.prop
    }
    if (typeof this.mediaType !== "undefined") {
      claim.mediaType = this.mediaType
    }
    if (typeof this.url !== "undefined") {
      claim.url = this.url
    }
    if (typeof this.preview !== "undefined") {
      claim.preview = this.preview
    }
  }
}

export class NoValueClaimPatch implements ClaimPatch {
  type: "none"
  prop?: string

  constructor(obj: object) {
    if (obj.type !== "none") {
      throw new Error(`invalid type "${obj.type}"`)
    }
    Object.assign(this, obj)
  }

  New(id: string): Claim {
    if (typeof this.prop === "undefined") {
      throw new Error("incomplete patch")
    }

    return new NoValueClaim({
      id: id,
      confidence: 1.0, // TODO How to make it configurable?
      prop: {
        id: this.prop,
        score: 1.0, // TODO: Fetch if from the store?
      },
    })
  }

  Apply(claim: Claim): void {
    if (typeof this.prop === "undefined") {
      throw new Error("empty patch")
    }

    if (!(claim instanceof NoValueClaim)) {
      throw new Error("not no value claim")
    }

    if (typeof this.prop !== "undefined") {
      claim.prop.id = this.prop
    }
  }
}

export class UnknownValueClaimPatch implements ClaimPatch {
  type: "unknown"
  prop?: string

  constructor(obj: object) {
    if (obj.type !== "unknown") {
      throw new Error(`invalid type "${obj.type}"`)
    }
    Object.assign(this, obj)
  }

  New(id: string): Claim {
    if (typeof this.prop === "undefined") {
      throw new Error("incomplete patch")
    }

    return new UnknownValueClaim({
      id: id,
      confidence: 1.0, // TODO How to make it configurable?
      prop: {
        id: this.prop,
        score: 1.0, // TODO: Fetch if from the store?
      },
    })
  }

  Apply(claim: Claim): void {
    if (typeof this.prop === "undefined") {
      throw new Error("empty patch")
    }

    if (!(claim instanceof UnknownValueClaim)) {
      throw new Error("not unknown value claim")
    }

    if (typeof this.prop !== "undefined") {
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
    if (obj.type !== "time") {
      throw new Error(`invalid type "${obj.type}"`)
    }
    Object.assign(this, obj)
  }

  New(id: string): Claim {
    if (typeof this.prop === "undefined" || typeof this.timestamp === "undefined" || typeof this.precision === "undefined") {
      throw new Error("incomplete patch")
    }

    return new TimeClaim({
      id: id,
      confidence: 1.0, // TODO How to make it configurable?
      prop: {
        id: this.prop,
        score: 1.0, // TODO: Fetch if from the store?
      },
      timestamp: this.timestamp,
      precision: this.precision,
    })
  }

  Apply(claim: Claim): void {
    if (typeof this.prop === "undefined" && typeof this.timestamp === "undefined" && typeof this.precision === "undefined") {
      throw new Error("empty patch")
    }

    if (!(claim instanceof TimeClaim)) {
      throw new Error("not time claim")
    }

    if (typeof this.prop !== "undefined") {
      claim.prop.id = this.prop
    }
    if (typeof this.timestamp !== "undefined") {
      claim.timestamp = this.timestamp
    }
    if (typeof this.precision !== "undefined") {
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
    if (obj.type !== "timeRange") {
      throw new Error(`invalid type "${obj.type}"`)
    }
    Object.assign(this, obj)
  }

  New(id: string): Claim {
    if (typeof this.prop === "undefined" || typeof this.lower === "undefined" || typeof this.upper === "undefined" || typeof this.precision === "undefined") {
      throw new Error("incomplete patch")
    }

    return new TimeRangeClaim({
      id: id,
      confidence: 1.0, // TODO How to make it configurable?
      prop: {
        id: this.prop,
        score: 1.0, // TODO: Fetch if from the store?
      },
      lower: this.lower,
      upper: this.upper,
      precision: this.precision,
    })
  }

  Apply(claim: Claim): void {
    if (typeof this.prop === "undefined" && typeof this.lower === "undefined" && typeof this.upper === "undefined" && typeof this.precision === "undefined") {
      throw new Error("empty patch")
    }

    if (!(claim instanceof TimeRangeClaim)) {
      throw new Error("not time range claim")
    }

    if (typeof this.prop !== "undefined") {
      claim.prop.id = this.prop
    }
    if (typeof this.lower !== "undefined") {
      claim.lower = this.lower
    }
    if (typeof this.upper !== "undefined") {
      claim.upper = this.upper
    }
    if (typeof this.precision !== "undefined") {
      claim.precision = this.precision
    }
  }
}
