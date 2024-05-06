import type { TranslatableHTMLString, AmountUnit, TimePrecision } from "@/types"

import { Identifier } from "@tozd/identifier"
import { v5 as uuidv5 } from "uuid"

class CoreClaim implements ClaimsContainer {
  id!: string
  confidence!: number
  meta?: ClaimTypes

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
      this.meta = new ClaimTypes()
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

  static from(obj: object): IdentifierClaim {
    const claim = Object.assign(new IdentifierClaim(), obj)
    if (typeof claim.meta !== "undefined") {
      claim.meta = ClaimTypes.from(claim.meta)
    }
    return claim
  }
}

class ReferenceClaim extends CoreClaim {
  prop!: DocumentReference
  iri!: string

  static from(obj: object): ReferenceClaim {
    const claim = Object.assign(new ReferenceClaim(), obj)
    if (typeof claim.meta !== "undefined") {
      claim.meta = ClaimTypes.from(claim.meta)
    }
    return claim
  }
}

class TextClaim extends CoreClaim {
  prop!: DocumentReference
  html!: TranslatableHTMLString

  static from(obj: object): TextClaim {
    const claim = Object.assign(new TextClaim(), obj)
    if (typeof claim.meta !== "undefined") {
      claim.meta = ClaimTypes.from(claim.meta)
    }
    return claim
  }
}

class StringClaim extends CoreClaim {
  prop!: DocumentReference
  string!: string

  static from(obj: object): StringClaim {
    const claim = Object.assign(new StringClaim(), obj)
    if (typeof claim.meta !== "undefined") {
      claim.meta = ClaimTypes.from(claim.meta)
    }
    return claim
  }
}

class AmountClaim extends CoreClaim {
  prop!: DocumentReference
  amount!: number
  unit!: AmountUnit

  static from(obj: object): AmountClaim {
    const claim = Object.assign(new AmountClaim(), obj)
    if (typeof claim.meta !== "undefined") {
      claim.meta = ClaimTypes.from(claim.meta)
    }
    return claim
  }
}

class AmountRangeClaim extends CoreClaim {
  prop!: DocumentReference
  lower!: number
  upper!: number
  unit!: AmountUnit

  static from(obj: object): AmountRangeClaim {
    const claim = Object.assign(new AmountRangeClaim(), obj)
    if (typeof claim.meta !== "undefined") {
      claim.meta = ClaimTypes.from(claim.meta)
    }
    return claim
  }
}

class RelationClaim extends CoreClaim {
  prop!: DocumentReference
  to!: DocumentReference

  static from(obj: object): RelationClaim {
    const claim = Object.assign(new RelationClaim(), obj)
    if (typeof claim.meta !== "undefined") {
      claim.meta = ClaimTypes.from(claim.meta)
    }
    return claim
  }
}

class FileClaim extends CoreClaim {
  prop!: DocumentReference
  mediaType!: string
  url!: string
  preview?: string[]

  static from(obj: object): FileClaim {
    const claim = Object.assign(new FileClaim(), obj)
    if (typeof claim.meta !== "undefined") {
      claim.meta = ClaimTypes.from(claim.meta)
    }
    return claim
  }
}

class NoValueClaim extends CoreClaim {
  prop!: DocumentReference

  static from(obj: object): NoValueClaim {
    const claim = Object.assign(new NoValueClaim(), obj)
    if (typeof claim.meta !== "undefined") {
      claim.meta = ClaimTypes.from(claim.meta)
    }
    return claim
  }
}

class UnknownValueClaim extends CoreClaim {
  prop!: DocumentReference

  static from(obj: object): UnknownValueClaim {
    const claim = Object.assign(new UnknownValueClaim(), obj)
    if (typeof claim.meta !== "undefined") {
      claim.meta = ClaimTypes.from(claim.meta)
    }
    return claim
  }
}

class TimeClaim extends CoreClaim {
  prop!: DocumentReference
  timestamp!: string
  precision!: TimePrecision

  static from(obj: object): TimeClaim {
    const claim = Object.assign(new TimeClaim(), obj)
    if (typeof claim.meta !== "undefined") {
      claim.meta = ClaimTypes.from(claim.meta)
    }
    return claim
  }
}

class TimeRangeClaim extends CoreClaim {
  prop!: DocumentReference
  lower!: string
  upper!: string
  precision!: TimePrecision

  static from(obj: object): TimeRangeClaim {
    const claim = Object.assign(new TimeRangeClaim(), obj)
    if (typeof claim.meta !== "undefined") {
      claim.meta = ClaimTypes.from(claim.meta)
    }
    return claim
  }
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

  static from(obj: object): ClaimTypes {
    const claimTypes = Object.assign(new ClaimTypes(), obj)
    for (const [name, claimType] of Object.entries(claimTypesMap)) {
      for (const [i, claim] of (claimTypes[name] || []).entries()) {
        claimTypes[name][i] = claimType.from(claim)
      }
    }
    return claimTypes
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
      this.claims = new ClaimTypes()
    }

    this.claims.Add(claim)
  }

  static from(obj: object): PeerDBDocument {
    const doc = Object.assign(new PeerDBDocument(), obj)
    if (typeof doc.claims !== "undefined") {
      doc.claims = ClaimTypes.from(doc.claims)
    }
    return doc
  }
}

interface Change {
  Apply(doc: PeerDBDocument, id: string): void
}

export function changeFrom(obj: object): Change {
  switch (obj.type) {
    case "add":
      return AddClaimChange.from(obj)
    case "set":
      return SetClaimChange.from(obj)
    case "remove":
      return RemoveClaimChange.from(obj)
  }
  throw new Error(`change of type "${obj.type}" is not supported`)
}

export class AddClaimChange implements Change {
  type: "add"
  under?: string
  patch!: ClaimPatch

  constructor() {
    this.type = "add"
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

  static from(obj: object): AddClaimChange {
    const change = Object.assign(new AddClaimChange(), obj)
    change.patch = claimPatchFrom(change.patch)
    return change
  }
}

export class SetClaimChange implements Change {
  type: "set"
  id!: string
  patch!: ClaimPatch

  constructor() {
    this.type = "set"
  }

  Apply(doc: PeerDBDocument, id: string): void {
    const claim = doc.GetByID(this.id)
    if (!claim) {
      throw new Error(`claim with ID "${this.id}" not found`)
    }
    this.patch.Apply(claim)
  }

  static from(obj: object): SetClaimChange {
    const change = Object.assign(new SetClaimChange(), obj)
    change.patch = claimPatchFrom(change.patch)
    return change
  }
}

export class RemoveClaimChange implements Change {
  type: "remove"
  id!: string

  constructor() {
    this.type = "remove"
  }

  Apply(doc: PeerDBDocument, id: string): void {
    const claim = doc.RemoveByID(this.id)
    if (!claim) {
      throw new Error(`claim with ID "${this.id}" not found`)
    }
  }

  static from(obj: object): RemoveClaimChange {
    return Object.assign(new RemoveClaimChange(), obj)
  }
}

export class Changes implements Change {
  changes: Change[]

  constructor(...changes: Change[]) {
    this.changes = changes
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

  static from(objs: object[]): Changes {
    const changes = objs.map(changeFrom)
    return new Changes(...changes)
  }
}

interface ClaimPatch {
  New(id: string): Claim
  Apply(claim: Claim): void
}

export function claimPatchFrom(obj: object): ClaimPatch {
  switch (obj.type) {
    case "id":
      return IdentifierClaimPatch.from(obj)
    case "ref":
      return ReferenceClaimPatch.from(obj)
    case "text":
      return TextClaimPatch.from(obj)
    case "string":
      return StringClaimPatch.from(obj)
    case "amount":
      return AmountClaimPatch.from(obj)
    case "amountRange":
      return AmountRangeClaimPatch.from(obj)
    case "rel":
      return RelationClaimPatch.from(obj)
    case "file":
      return FileClaimPatch.from(obj)
    case "none":
      return NoValueClaimPatch.from(obj)
    case "unknown":
      return UnknownValueClaimPatch.from(obj)
    case "time":
      return TimeClaimPatch.from(obj)
    case "timeRange":
      return TimeRangeClaimPatch.from(obj)
  }
  throw new Error(`patch of type "${obj.type}" is not supported`)
}

export class IdentifierClaimPatch implements ClaimPatch {
  type: "id"
  prop?: string
  value?: string

  constructor() {
    this.type = "id"
  }

  New(id: string): Claim {
    if (typeof this.prop === "undefined" || typeof this.value === "undefined") {
      throw new Error("incomplete patch")
    }

    return Object.assign(new IdentifierClaim(), {
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

  static from(obj: object): IdentifierClaimPatch {
    return Object.assign(new IdentifierClaimPatch(), obj)
  }
}

export class ReferenceClaimPatch implements ClaimPatch {
  type: "ref"
  prop?: string
  iri?: string

  constructor() {
    this.type = "ref"
  }

  New(id: string): Claim {
    if (typeof this.prop === "undefined" || typeof this.iri === "undefined") {
      throw new Error("incomplete patch")
    }

    return Object.assign(new ReferenceClaim(), {
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

  static from(obj: object): ReferenceClaimPatch {
    return Object.assign(new ReferenceClaimPatch(), obj)
  }
}

export class TextClaimPatch implements ClaimPatch {
  type: "text"
  prop?: string
  html?: TranslatableHTMLString
  remove?: string[]

  constructor() {
    this.type = "text"
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

    return Object.assign(new TextClaim(), {
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

  static from(obj: object): TextClaimPatch {
    return Object.assign(new TextClaimPatch(), obj)
  }
}

export class StringClaimPatch implements ClaimPatch {
  type: "string"
  prop?: string
  string?: string

  constructor() {
    this.type = "string"
  }

  New(id: string): Claim {
    if (typeof this.prop === "undefined" || typeof this.string === "undefined") {
      throw new Error("incomplete patch")
    }

    return Object.assign(new StringClaim(), {
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

  static from(obj: object): StringClaimPatch {
    return Object.assign(new StringClaimPatch(), obj)
  }
}

export class AmountClaimPatch implements ClaimPatch {
  type: "amount"
  prop?: string
  amount?: number
  unit?: AmountUnit

  constructor() {
    this.type = "amount"
  }

  New(id: string): Claim {
    if (typeof this.prop === "undefined" || typeof this.amount === "undefined" || typeof this.unit === "undefined") {
      throw new Error("incomplete patch")
    }

    return Object.assign(new AmountClaim(), {
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

  static from(obj: object): AmountClaimPatch {
    return Object.assign(new AmountClaimPatch(), obj)
  }
}

export class AmountRangeClaimPatch implements ClaimPatch {
  type: "amountRange"
  prop?: string
  lower?: number
  upper?: number
  unit?: AmountUnit

  constructor() {
    this.type = "amountRange"
  }

  New(id: string): Claim {
    if (typeof this.prop === "undefined" || typeof this.lower === "undefined" || typeof this.upper === "undefined" || typeof this.unit === "undefined") {
      throw new Error("incomplete patch")
    }

    return Object.assign(new AmountRangeClaim(), {
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

  static from(obj: object): AmountRangeClaimPatch {
    return Object.assign(new AmountRangeClaimPatch(), obj)
  }
}

export class RelationClaimPatch implements ClaimPatch {
  type: "rel"
  prop?: string
  to?: string

  constructor() {
    this.type = "rel"
  }

  New(id: string): Claim {
    if (typeof this.prop === "undefined" || typeof this.to === "undefined") {
      throw new Error("incomplete patch")
    }

    return Object.assign(new RelationClaim(), {
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

  static from(obj: object): RelationClaimPatch {
    return Object.assign(new RelationClaimPatch(), obj)
  }
}

export class FileClaimPatch implements ClaimPatch {
  type: "file"
  prop?: string
  mediaType?: string
  url?: string
  preview?: string[]

  constructor() {
    this.type = "file"
  }

  New(id: string): Claim {
    if (typeof this.prop === "undefined" || typeof this.mediaType === "undefined" || typeof this.url === "undefined" || typeof this.preview === "undefined") {
      throw new Error("incomplete patch")
    }

    return Object.assign(new FileClaim(), {
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

  static from(obj: object): FileClaimPatch {
    return Object.assign(new FileClaimPatch(), obj)
  }
}

export class NoValueClaimPatch implements ClaimPatch {
  type: "none"
  prop?: string

  constructor() {
    this.type = "none"
  }

  New(id: string): Claim {
    if (typeof this.prop === "undefined") {
      throw new Error("incomplete patch")
    }

    return Object.assign(new NoValueClaim(), {
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

  static from(obj: object): NoValueClaimPatch {
    return Object.assign(new NoValueClaimPatch(), obj)
  }
}

export class UnknownValueClaimPatch implements ClaimPatch {
  type: "unknown"
  prop?: string

  constructor() {
    this.type = "unknown"
  }

  New(id: string): Claim {
    if (typeof this.prop === "undefined") {
      throw new Error("incomplete patch")
    }

    return Object.assign(new UnknownValueClaim(), {
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

  static from(obj: object): UnknownValueClaimPatch {
    return Object.assign(new UnknownValueClaimPatch(), obj)
  }
}

export class TimeClaimPatch implements ClaimPatch {
  type: "time"
  prop?: string
  timestamp?: string
  precision?: TimePrecision

  constructor() {
    this.type = "time"
  }

  New(id: string): Claim {
    if (typeof this.prop === "undefined" || typeof this.timestamp === "undefined" || typeof this.precision === "undefined") {
      throw new Error("incomplete patch")
    }

    return Object.assign(new TimeClaim(), {
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

  static from(obj: object): TimeClaimPatch {
    return Object.assign(new TimeClaimPatch(), obj)
  }
}

export class TimeRangeClaimPatch implements ClaimPatch {
  type: "timeRange"
  prop?: string
  lower?: string
  upper?: string
  precision?: TimePrecision

  constructor() {
    this.type = "timeRange"
  }

  New(id: string): Claim {
    if (typeof this.prop === "undefined" || typeof this.lower === "undefined" || typeof this.upper === "undefined" || typeof this.precision === "undefined") {
      throw new Error("incomplete patch")
    }

    return Object.assign(new TimeRangeClaim(), {
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

  static from(obj: object): TimeRangeClaimPatch {
    return Object.assign(new TimeRangeClaimPatch(), obj)
  }
}
