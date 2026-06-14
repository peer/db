import type { Claim } from "@/document/claims"
import type { Amount, Confidence, Time, TimePrecision } from "@/document/types"

import { Identifier } from "@tozd/identifier"

import { validateAmount } from "@/document/amount"
import {
  AmountClaim,
  AmountIntervalClaim,
  HasClaim,
  HTMLClaim,
  IdentifierClaim,
  LinkClaim,
  NoneClaim,
  ReferenceClaim,
  StringClaim,
  TimeClaim,
  TimeIntervalClaim,
  UnknownClaim,
} from "@/document/claims"
import { D } from "@/document/document"
import { VALID_TIME_PRECISIONS, validateTime } from "@/document/time"
import { isCanonicalHTML } from "@/partials/input/InputHTML.schema"
import { equals, parseUrl } from "@/utils"

// changeFrom creates a Change from a plain object.
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
  // eslint-disable-next-line @typescript-eslint/restrict-template-expressions
  throw new Error(`change type not supported: ${obj.type}`)
}

// Changes is a slice of Change operations to apply to a document.
export class Changes implements Change {
  changes: Change[]

  constructor(...objs: object[]) {
    this.changes = objs.map(changeFrom)
  }

  // Apply applies all changes in order to the given document.
  async Apply(doc: D): Promise<void> {
    for (const change of this.changes) {
      await change.Apply(doc)
    }
  }

  // Validate validates all changes in the slice.
  async Validate(base: string[]): Promise<void> {
    for (const [i, change] of this.changes.entries()) {
      await change.Validate(base, i + 1)
    }
  }

  toJSON(): Change[] {
    return this.changes
  }
}

// claimPatchFrom creates a ClaimPatch from a plain object.
export function claimPatchFrom(obj: object): ClaimPatch {
  if (!("type" in obj)) {
    throw new Error(`patch missing type`)
  }
  switch (obj.type) {
    case "id":
      return new IdentifierClaimPatch(obj)
    case "string":
      return new StringClaimPatch(obj)
    case "html":
      return new HTMLClaimPatch(obj)
    case "amount":
      return new AmountClaimPatch(obj)
    case "amountInterval":
      return new AmountIntervalClaimPatch(obj)
    case "time":
      return new TimeClaimPatch(obj)
    case "timeInterval":
      return new TimeIntervalClaimPatch(obj)
    case "link":
      return new LinkClaimPatch(obj)
    case "ref":
      return new ReferenceClaimPatch(obj)
    case "has":
      return new HasClaimPatch(obj)
    case "none":
      return new NoneClaimPatch(obj)
    case "unknown":
      return new UnknownClaimPatch(obj)
  }
  // eslint-disable-next-line @typescript-eslint/restrict-template-expressions
  throw new Error(`patch type not supported: ${obj.type}`)
}

// Change represents a modification operation that can be applied to a document.
export interface Change {
  Apply(doc: D): Promise<void>
  Validate(base: string[], operation: number): Promise<void>
}

// ClaimPatch represents a modification that can be applied to create or update a claim.
interface ClaimPatch {
  New(id: string): Claim
  Apply(claim: Claim): Promise<void>

  // Validate checks the patch on its own, without access to the target claim: at least
  // one field has to be set and fields which are set have to be valid by themselves.
  // This allows rejecting an invalid patch already when it is appended to an edit
  // session, instead of when the session completes. Completeness is checked by New and
  // constraints which need the target claim are checked by Apply. Mirrors the backend's
  // ClaimPatch.Validate.
  Validate(): void
}

// validatePatchConfidence validates an optional confidence field of a patch.
function validatePatchConfidence(confidence?: Confidence): void {
  if (confidence === undefined) {
    return
  }
  if (confidence < -1 || confidence > 1 || !isFinite(confidence)) {
    throw new Error("confidence out of range [-1, 1]")
  }
}

// validatePatchAmount validates optional amount and precision fields of a patch.
// When only one of them is set, the combination with the other value from the
// target claim is validated by Apply.
function validatePatchAmount(amount?: Amount, precision?: number): void {
  if (precision !== undefined) {
    if (!isFinite(precision) || precision <= 0) {
      throw new Error("Precision must be a finite positive number")
    }
    if (amount) {
      validateAmount(amount, precision)
    }
  }
}

// validatePatchTime validates optional time and precision fields of a patch.
// When only one of them is set, the combination with the other value from the
// target claim is validated by Apply.
function validatePatchTime(time?: Time, precision?: TimePrecision): void {
  if (precision !== undefined) {
    if (!VALID_TIME_PRECISIONS.has(precision)) {
      throw new Error("unknown Precision")
    }
    if (time) {
      validateTime(time, precision)
    }
  }
}

// AddClaimChange represents a change that adds a new claim to a document.
export class AddClaimChange implements Change {
  type: "add"
  under?: string
  id!: string
  base!: string[]
  patch!: ClaimPatch

  constructor(obj: object) {
    if ("type" in obj && obj.type !== "add") {
      // eslint-disable-next-line @typescript-eslint/restrict-template-expressions
      throw new Error(`invalid type: ${obj.type}`)
    }
    this.type = "add"
    Object.assign(this, obj)
    this.patch = claimPatchFrom(this.patch)

    if (!this.id) {
      throw new Error("id is required")
    }
    if (!this.base) {
      throw new Error("base is required")
    }
    if (!this.patch) {
      throw new Error("patch is required")
    }
  }

  // Apply applies the add claim change to the document.
  async Apply(doc: D): Promise<void> {
    const newClaim = this.patch.New(this.id)
    await newClaim.Validate()

    if (!this.under) {
      doc.Add(newClaim)
      return
    }

    const claim = doc.GetByID(this.under)
    if (!claim) {
      throw new Error(`claim not found: ${this.under}`)
    }

    claim.Add(newClaim)
  }

  // Validate validates the add claim change.
  async Validate(base: string[], operation: number): Promise<void> {
    const expectedBase = [...base, String(operation)]
    if (!equals(this.base, expectedBase)) {
      throw new Error(`invalid base: expected ${JSON.stringify(expectedBase)}, base ${JSON.stringify(this.base)}`)
    }
    const expectedID = (await Identifier.from(...this.base)).toString()
    if (this.id !== expectedID) {
      throw new Error(`invalid ID: expected ${expectedID}, id ${this.id}`)
    }
    // Constructing the claim from the patch checks that the patch is complete; validating it checks
    // the resulting claim is valid. This rejects an invalid add already when it is appended to an edit
    // session, instead of when the session completes.
    await this.patch.New(this.id).Validate()
  }
}

// SetClaimChange represents a change that modifies an existing claim in a document.
export class SetClaimChange implements Change {
  type: "set"
  id!: string
  patch!: ClaimPatch

  constructor(obj: object) {
    if ("type" in obj && obj.type !== "set") {
      // eslint-disable-next-line @typescript-eslint/restrict-template-expressions
      throw new Error(`invalid type: ${obj.type}`)
    }
    this.type = "set"
    Object.assign(this, obj)
    this.patch = claimPatchFrom(this.patch)

    if (!this.id) {
      throw new Error("id is required")
    }
    if (!this.patch) {
      throw new Error("patch is required")
    }
  }

  // Apply applies the set claim change to the document.
  async Apply(doc: D): Promise<void> {
    const claim = doc.GetByID(this.id)
    if (!claim) {
      throw new Error(`claim not found: ${this.id}`)
    }
    await this.patch.Apply(claim)
  }

  // Validate validates the set claim change.
  // eslint-disable-next-line @typescript-eslint/require-await
  async Validate(base: string[], operation: number): Promise<void> {
    // Patches in set changes can be partial, so only the fields which are set are
    // checked here. The full result is validated by Apply when the session completes.
    this.patch.Validate()
  }
}

// RemoveClaimChange represents a change that removes a claim from a document.
export class RemoveClaimChange implements Change {
  type: "remove"
  id!: string

  constructor(obj: object) {
    if ("type" in obj && obj.type !== "remove") {
      // eslint-disable-next-line @typescript-eslint/restrict-template-expressions
      throw new Error(`invalid type: ${obj.type}`)
    }
    this.type = "remove"
    Object.assign(this, obj)

    if (!this.id) {
      throw new Error("id is required")
    }
  }

  // Apply applies the remove claim change to the document.
  // eslint-disable-next-line @typescript-eslint/require-await
  async Apply(doc: D): Promise<void> {
    const claim = doc.RemoveByID(this.id)
    if (!claim) {
      throw new Error(`claim not found: ${this.id}`)
    }
  }

  // Validate validates the remove claim change.
  async Validate(base: string[], operation: number): Promise<void> {
    // No validation needed.
  }
}

// IdentifierClaimPatch represents a patch for an identifier claim.
export class IdentifierClaimPatch implements ClaimPatch {
  type: "id"
  confidence?: Confidence
  prop?: string
  value?: string

  constructor(obj: object) {
    if ("type" in obj && obj.type !== "id") {
      // eslint-disable-next-line @typescript-eslint/restrict-template-expressions
      throw new Error(`invalid type: ${obj.type}`)
    }
    this.type = "id"
    Object.assign(this, obj)
  }

  // New creates a new identifier claim from the patch.
  New(id: string): Claim {
    if (this.confidence === undefined || !this.prop || !this.value) {
      throw new Error("incomplete patch")
    }

    return new IdentifierClaim({ id, confidence: this.confidence, prop: { id: this.prop }, value: this.value })
  }

  // Validate checks the patch fields which are set, without access to the target claim.
  Validate(): void {
    if (this.confidence === undefined && !this.prop && !this.value) {
      throw new Error("empty patch")
    }
    validatePatchConfidence(this.confidence)
  }

  // Apply applies the patch to an existing identifier claim.
  async Apply(claim: Claim): Promise<void> {
    this.Validate()

    if (!(claim instanceof IdentifierClaim)) {
      throw new Error("not identifier claim")
    }

    if (this.confidence !== undefined) claim.confidence = this.confidence
    if (this.prop) claim.prop.id = this.prop
    if (this.value) claim.value = this.value

    await claim.Validate()
  }
}

// StringClaimPatch represents a patch for a string claim.
export class StringClaimPatch implements ClaimPatch {
  type: "string"
  confidence?: Confidence
  prop?: string
  string?: string

  constructor(obj: object) {
    if ("type" in obj && obj.type !== "string") {
      // eslint-disable-next-line @typescript-eslint/restrict-template-expressions
      throw new Error(`invalid type: ${obj.type}`)
    }
    this.type = "string"
    Object.assign(this, obj)
  }

  // New creates a new string claim from the patch.
  New(id: string): Claim {
    if (this.confidence === undefined || !this.prop || !this.string) {
      throw new Error("incomplete patch")
    }

    return new StringClaim({ id, confidence: this.confidence, prop: { id: this.prop }, string: this.string })
  }

  // Validate checks the patch fields which are set, without access to the target claim.
  Validate(): void {
    if (this.confidence === undefined && !this.prop && !this.string) {
      throw new Error("empty patch")
    }
    validatePatchConfidence(this.confidence)
  }

  // Apply applies the patch to an existing string claim.
  async Apply(claim: Claim): Promise<void> {
    this.Validate()

    if (!(claim instanceof StringClaim)) {
      throw new Error("not string claim")
    }

    if (this.confidence !== undefined) claim.confidence = this.confidence
    if (this.prop) claim.prop.id = this.prop
    if (this.string) claim.string = this.string

    await claim.Validate()
  }
}

// HTMLClaimPatch represents a patch for an HTML claim.
export class HTMLClaimPatch implements ClaimPatch {
  type: "html"
  confidence?: Confidence
  prop?: string
  html?: string

  constructor(obj: object) {
    if ("type" in obj && obj.type !== "html") {
      // eslint-disable-next-line @typescript-eslint/restrict-template-expressions
      throw new Error(`invalid type: ${obj.type}`)
    }
    this.type = "html"
    Object.assign(this, obj)
  }

  // New creates a new HTML claim from the patch.
  New(id: string): Claim {
    if (this.confidence === undefined || !this.prop || !this.html) {
      throw new Error("incomplete patch")
    }

    return new HTMLClaim({ id, confidence: this.confidence, prop: { id: this.prop }, html: this.html })
  }

  // Validate checks the patch fields which are set, without access to the target claim.
  // HTML has to be in the canonical form produced by docToHtml, which the backend
  // accepts unchanged, so that the stored HTML stays byte for byte equal to what the
  // client serialized. This mirrors the backend HTMLClaimPatch.Validate, including its
  // error message.
  Validate(): void {
    if (this.confidence === undefined && !this.prop && !this.html) {
      throw new Error("empty patch")
    }
    validatePatchConfidence(this.confidence)
    if (this.html && !isCanonicalHTML(this.html)) {
      throw new Error("HTML is not canonical")
    }
  }

  // Apply applies the patch to an existing HTML claim.
  async Apply(claim: Claim): Promise<void> {
    this.Validate()

    if (!(claim instanceof HTMLClaim)) {
      throw new Error("not HTML claim")
    }

    if (this.confidence !== undefined) claim.confidence = this.confidence
    if (this.prop) claim.prop.id = this.prop
    if (this.html) claim.html = this.html

    await claim.Validate()
  }
}

// AmountClaimPatch represents a patch for an amount claim.
export class AmountClaimPatch implements ClaimPatch {
  type: "amount"
  confidence?: Confidence
  prop?: string
  amount?: Amount
  precision?: number

  constructor(obj: object) {
    if ("type" in obj && obj.type !== "amount") {
      // eslint-disable-next-line @typescript-eslint/restrict-template-expressions
      throw new Error(`invalid type: ${obj.type}`)
    }
    this.type = "amount"
    Object.assign(this, obj)
  }

  // New creates a new amount claim from the patch.
  New(id: string): Claim {
    if (this.confidence === undefined || !this.prop || !this.amount || this.precision === undefined) {
      throw new Error("incomplete patch")
    }

    return new AmountClaim({ id, confidence: this.confidence, prop: { id: this.prop }, amount: this.amount, precision: this.precision })
  }

  // Validate checks the patch fields which are set, without access to the target claim.
  Validate(): void {
    if (this.confidence === undefined && !this.prop && !this.amount && this.precision === undefined) {
      throw new Error("empty patch")
    }
    validatePatchConfidence(this.confidence)
    validatePatchAmount(this.amount, this.precision)
  }

  // Apply applies the patch to an existing amount claim.
  async Apply(claim: Claim): Promise<void> {
    this.Validate()

    if (!(claim instanceof AmountClaim)) {
      throw new Error("not amount claim")
    }

    if (this.confidence !== undefined) claim.confidence = this.confidence
    if (this.prop) claim.prop.id = this.prop
    if (this.amount) claim.amount = this.amount
    if (this.precision !== undefined) claim.precision = this.precision

    await claim.Validate()
  }
}

// AmountIntervalClaimPatch represents a patch for an amount interval claim.
export class AmountIntervalClaimPatch implements ClaimPatch {
  type: "amountInterval"
  confidence?: Confidence
  prop?: string
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
    if ("type" in obj && obj.type !== "amountInterval") {
      // eslint-disable-next-line @typescript-eslint/restrict-template-expressions
      throw new Error(`invalid type: ${obj.type}`)
    }
    this.type = "amountInterval"
    Object.assign(this, obj)
  }

  // New creates a new amount interval claim from the patch.
  New(id: string): Claim {
    if (this.confidence === undefined || !this.prop) {
      throw new Error("incomplete patch")
    }

    return new AmountIntervalClaim({
      id,
      confidence: this.confidence,
      prop: { id: this.prop },
      from: this.from,
      fromPrecision: this.fromPrecision,
      fromIsOpen: this.fromIsOpen,
      fromIsUnknown: this.fromIsUnknown,
      fromIsNone: this.fromIsNone,
      to: this.to,
      toPrecision: this.toPrecision,
      toIsOpen: this.toIsOpen,
      toIsUnknown: this.toIsUnknown,
      toIsNone: this.toIsNone,
    })
  }

  // Validate checks the patch fields which are set, without access to the target claim.
  Validate(): void {
    if (
      this.confidence === undefined &&
      !this.prop &&
      !this.from &&
      this.fromPrecision === undefined &&
      this.fromIsOpen === undefined &&
      this.fromIsUnknown === undefined &&
      this.fromIsNone === undefined &&
      !this.to &&
      this.toPrecision === undefined &&
      this.toIsOpen === undefined &&
      this.toIsUnknown === undefined &&
      this.toIsNone === undefined
    ) {
      throw new Error("empty patch")
    }
    validatePatchConfidence(this.confidence)
    validatePatchAmount(this.from, this.fromPrecision)
    validatePatchAmount(this.to, this.toPrecision)
  }

  // Apply applies the patch to an existing amount interval claim.
  async Apply(claim: Claim): Promise<void> {
    this.Validate()

    if (!(claim instanceof AmountIntervalClaim)) {
      throw new Error("not amount interval claim")
    }

    if (this.confidence !== undefined) claim.confidence = this.confidence
    if (this.prop) claim.prop.id = this.prop
    if (this.from) {
      claim.from = this.from
      claim.fromIsUnknown = false
      claim.fromIsNone = false
    }
    if (this.fromPrecision !== undefined) claim.fromPrecision = this.fromPrecision
    if (this.fromIsOpen !== undefined) {
      claim.fromIsOpen = this.fromIsOpen
      if (this.fromIsOpen) {
        claim.fromIsUnknown = false
        claim.fromIsNone = false
      }
    }
    if (this.fromIsUnknown !== undefined) {
      claim.fromIsUnknown = this.fromIsUnknown
      if (this.fromIsUnknown) {
        claim.from = undefined
        claim.fromPrecision = undefined
        claim.fromIsOpen = false
        claim.fromIsNone = false
      }
    }
    if (this.fromIsNone !== undefined) {
      claim.fromIsNone = this.fromIsNone
      if (this.fromIsNone) {
        claim.from = undefined
        claim.fromPrecision = undefined
        claim.fromIsOpen = false
        claim.fromIsUnknown = false
      }
    }
    if (this.to) {
      claim.to = this.to
      claim.toIsUnknown = false
      claim.toIsNone = false
    }
    if (this.toPrecision !== undefined) claim.toPrecision = this.toPrecision
    if (this.toIsOpen !== undefined) {
      claim.toIsOpen = this.toIsOpen
      if (this.toIsOpen) {
        claim.toIsUnknown = false
        claim.toIsNone = false
      }
    }
    if (this.toIsUnknown !== undefined) {
      claim.toIsUnknown = this.toIsUnknown
      if (this.toIsUnknown) {
        claim.to = undefined
        claim.toPrecision = undefined
        claim.toIsOpen = false
        claim.toIsNone = false
      }
    }
    if (this.toIsNone !== undefined) {
      claim.toIsNone = this.toIsNone
      if (this.toIsNone) {
        claim.to = undefined
        claim.toPrecision = undefined
        claim.toIsOpen = false
        claim.toIsUnknown = false
      }
    }

    await claim.Validate()
  }
}

// TimeClaimPatch represents a patch for a time claim.
export class TimeClaimPatch implements ClaimPatch {
  type: "time"
  confidence?: Confidence
  prop?: string
  time?: Time
  precision?: TimePrecision

  constructor(obj: object) {
    if ("type" in obj && obj.type !== "time") {
      // eslint-disable-next-line @typescript-eslint/restrict-template-expressions
      throw new Error(`invalid type: ${obj.type}`)
    }
    this.type = "time"
    Object.assign(this, obj)
  }

  // New creates a new time claim from the patch.
  New(id: string): Claim {
    if (this.confidence === undefined || !this.prop || !this.time || this.precision === undefined) {
      throw new Error("incomplete patch")
    }

    return new TimeClaim({ id, confidence: this.confidence, prop: { id: this.prop }, time: this.time, precision: this.precision })
  }

  // Validate checks the patch fields which are set, without access to the target claim.
  Validate(): void {
    if (this.confidence === undefined && !this.prop && !this.time && this.precision === undefined) {
      throw new Error("empty patch")
    }
    validatePatchConfidence(this.confidence)
    validatePatchTime(this.time, this.precision)
  }

  // Apply applies the patch to an existing time claim.
  async Apply(claim: Claim): Promise<void> {
    this.Validate()

    if (!(claim instanceof TimeClaim)) {
      throw new Error("not time claim")
    }

    if (this.confidence !== undefined) claim.confidence = this.confidence
    if (this.prop) claim.prop.id = this.prop
    if (this.time) claim.time = this.time
    if (this.precision !== undefined) claim.precision = this.precision

    await claim.Validate()
  }
}

// TimeIntervalClaimPatch represents a patch for a time interval claim.
export class TimeIntervalClaimPatch implements ClaimPatch {
  type: "timeInterval"
  confidence?: Confidence
  prop?: string
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
    if ("type" in obj && obj.type !== "timeInterval") {
      // eslint-disable-next-line @typescript-eslint/restrict-template-expressions
      throw new Error(`invalid type: ${obj.type}`)
    }
    this.type = "timeInterval"
    Object.assign(this, obj)
  }

  // New creates a new time interval claim from the patch.
  New(id: string): Claim {
    if (this.confidence === undefined || !this.prop) {
      throw new Error("incomplete patch")
    }

    return new TimeIntervalClaim({
      id,
      confidence: this.confidence,
      prop: { id: this.prop },
      from: this.from,
      fromPrecision: this.fromPrecision,
      fromIsOpen: this.fromIsOpen,
      fromIsUnknown: this.fromIsUnknown,
      fromIsNone: this.fromIsNone,
      to: this.to,
      toPrecision: this.toPrecision,
      toIsOpen: this.toIsOpen,
      toIsUnknown: this.toIsUnknown,
      toIsNone: this.toIsNone,
    })
  }

  // Validate checks the patch fields which are set, without access to the target claim.
  Validate(): void {
    if (
      this.confidence === undefined &&
      !this.prop &&
      !this.from &&
      this.fromPrecision === undefined &&
      this.fromIsOpen === undefined &&
      this.fromIsUnknown === undefined &&
      this.fromIsNone === undefined &&
      !this.to &&
      this.toPrecision === undefined &&
      this.toIsOpen === undefined &&
      this.toIsUnknown === undefined &&
      this.toIsNone === undefined
    ) {
      throw new Error("empty patch")
    }
    validatePatchConfidence(this.confidence)
    validatePatchTime(this.from, this.fromPrecision)
    validatePatchTime(this.to, this.toPrecision)
  }

  // Apply applies the patch to an existing time interval claim.
  async Apply(claim: Claim): Promise<void> {
    this.Validate()

    if (!(claim instanceof TimeIntervalClaim)) {
      throw new Error("not time interval claim")
    }

    if (this.confidence !== undefined) claim.confidence = this.confidence
    if (this.prop) claim.prop.id = this.prop
    if (this.from) {
      claim.from = this.from
      claim.fromIsUnknown = false
      claim.fromIsNone = false
    }
    if (this.fromPrecision !== undefined) claim.fromPrecision = this.fromPrecision
    if (this.fromIsOpen !== undefined) {
      claim.fromIsOpen = this.fromIsOpen
      if (this.fromIsOpen) {
        claim.fromIsUnknown = false
        claim.fromIsNone = false
      }
    }
    if (this.fromIsUnknown !== undefined) {
      claim.fromIsUnknown = this.fromIsUnknown
      if (this.fromIsUnknown) {
        claim.from = undefined
        claim.fromPrecision = undefined
        claim.fromIsOpen = false
        claim.fromIsNone = false
      }
    }
    if (this.fromIsNone !== undefined) {
      claim.fromIsNone = this.fromIsNone
      if (this.fromIsNone) {
        claim.from = undefined
        claim.fromPrecision = undefined
        claim.fromIsOpen = false
        claim.fromIsUnknown = false
      }
    }
    if (this.to) {
      claim.to = this.to
      claim.toIsUnknown = false
      claim.toIsNone = false
    }
    if (this.toPrecision !== undefined) claim.toPrecision = this.toPrecision
    if (this.toIsOpen !== undefined) {
      claim.toIsOpen = this.toIsOpen
      if (this.toIsOpen) {
        claim.toIsUnknown = false
        claim.toIsNone = false
      }
    }
    if (this.toIsUnknown !== undefined) {
      claim.toIsUnknown = this.toIsUnknown
      if (this.toIsUnknown) {
        claim.to = undefined
        claim.toPrecision = undefined
        claim.toIsOpen = false
        claim.toIsNone = false
      }
    }
    if (this.toIsNone !== undefined) {
      claim.toIsNone = this.toIsNone
      if (this.toIsNone) {
        claim.to = undefined
        claim.toPrecision = undefined
        claim.toIsOpen = false
        claim.toIsUnknown = false
      }
    }

    await claim.Validate()
  }
}

// LinkClaimPatch represents a patch for a link claim.
export class LinkClaimPatch implements ClaimPatch {
  type: "link"
  confidence?: Confidence
  prop?: string
  iri?: string

  constructor(obj: object) {
    if ("type" in obj && obj.type !== "link") {
      // eslint-disable-next-line @typescript-eslint/restrict-template-expressions
      throw new Error(`invalid type: ${obj.type}`)
    }
    this.type = "link"
    Object.assign(this, obj)
  }

  // New creates a new link claim from the patch.
  New(id: string): Claim {
    if (this.confidence === undefined || !this.prop || !this.iri) {
      throw new Error("incomplete patch")
    }

    return new LinkClaim({ id, confidence: this.confidence, prop: { id: this.prop }, iri: this.iri })
  }

  // Validate checks the patch fields which are set, without access to the target claim.
  Validate(): void {
    if (this.confidence === undefined && !this.prop && !this.iri) {
      throw new Error("empty patch")
    }
    validatePatchConfidence(this.confidence)
    if (this.iri) {
      try {
        parseUrl(this.iri)
      } catch (err) {
        // eslint-disable-next-line @typescript-eslint/restrict-template-expressions
        throw new Error(`invalid IRI: ${err}`, { cause: err })
      }
    }
  }

  // Apply applies the patch to an existing link claim.
  async Apply(claim: Claim): Promise<void> {
    this.Validate()

    if (!(claim instanceof LinkClaim)) {
      throw new Error("not link claim")
    }

    if (this.confidence !== undefined) claim.confidence = this.confidence
    if (this.prop) claim.prop.id = this.prop
    if (this.iri) claim.iri = this.iri

    await claim.Validate()
  }
}

// ReferenceClaimPatch represents a patch for a reference claim.
export class ReferenceClaimPatch implements ClaimPatch {
  type: "ref"
  confidence?: Confidence
  prop?: string
  to?: string

  constructor(obj: object) {
    if ("type" in obj && obj.type !== "ref") {
      // eslint-disable-next-line @typescript-eslint/restrict-template-expressions
      throw new Error(`invalid type: ${obj.type}`)
    }
    this.type = "ref"
    Object.assign(this, obj)
  }

  // New creates a new reference claim from the patch.
  New(id: string): Claim {
    if (this.confidence === undefined || !this.prop || !this.to) {
      throw new Error("incomplete patch")
    }

    return new ReferenceClaim({ id, confidence: this.confidence, prop: { id: this.prop }, to: { id: this.to } })
  }

  // Validate checks the patch fields which are set, without access to the target claim.
  Validate(): void {
    if (this.confidence === undefined && !this.prop && !this.to) {
      throw new Error("empty patch")
    }
    validatePatchConfidence(this.confidence)
  }

  // Apply applies the patch to an existing reference claim.
  async Apply(claim: Claim): Promise<void> {
    this.Validate()

    if (!(claim instanceof ReferenceClaim)) {
      throw new Error("not reference claim")
    }

    if (this.confidence !== undefined) claim.confidence = this.confidence
    if (this.prop) claim.prop.id = this.prop
    if (this.to) claim.to.id = this.to

    await claim.Validate()
  }
}

// HasClaimPatch represents a patch for a has claim.
export class HasClaimPatch implements ClaimPatch {
  type: "has"
  confidence?: Confidence
  prop?: string

  constructor(obj: object) {
    if ("type" in obj && obj.type !== "has") {
      // eslint-disable-next-line @typescript-eslint/restrict-template-expressions
      throw new Error(`invalid type: ${obj.type}`)
    }
    this.type = "has"
    Object.assign(this, obj)
  }

  // New creates a new has claim from the patch.
  New(id: string): Claim {
    if (this.confidence === undefined || !this.prop) {
      throw new Error("incomplete patch")
    }

    return new HasClaim({ id, confidence: this.confidence, prop: { id: this.prop } })
  }

  // Validate checks the patch fields which are set, without access to the target claim.
  Validate(): void {
    if (this.confidence === undefined && !this.prop) {
      throw new Error("empty patch")
    }
    validatePatchConfidence(this.confidence)
  }

  // Apply applies the patch to an existing has claim.
  async Apply(claim: Claim): Promise<void> {
    this.Validate()

    if (!(claim instanceof HasClaim)) {
      throw new Error("not has claim")
    }

    if (this.confidence !== undefined) claim.confidence = this.confidence
    if (this.prop) claim.prop.id = this.prop

    await claim.Validate()
  }
}

// NoneClaimPatch represents a patch for a none claim.
export class NoneClaimPatch implements ClaimPatch {
  type: "none"
  confidence?: Confidence
  prop?: string

  constructor(obj: object) {
    if ("type" in obj && obj.type !== "none") {
      // eslint-disable-next-line @typescript-eslint/restrict-template-expressions
      throw new Error(`invalid type: ${obj.type}`)
    }
    this.type = "none"
    Object.assign(this, obj)
  }

  // New creates a new none claim from the patch.
  New(id: string): Claim {
    if (this.confidence === undefined || !this.prop) {
      throw new Error("incomplete patch")
    }

    return new NoneClaim({ id, confidence: this.confidence, prop: { id: this.prop } })
  }

  // Validate checks the patch fields which are set, without access to the target claim.
  Validate(): void {
    if (this.confidence === undefined && !this.prop) {
      throw new Error("empty patch")
    }
    validatePatchConfidence(this.confidence)
  }

  // Apply applies the patch to an existing none claim.
  async Apply(claim: Claim): Promise<void> {
    this.Validate()

    if (!(claim instanceof NoneClaim)) {
      throw new Error("not none claim")
    }

    if (this.confidence !== undefined) claim.confidence = this.confidence
    if (this.prop) claim.prop.id = this.prop

    await claim.Validate()
  }
}

// UnknownClaimPatch represents a patch for an unknown claim.
export class UnknownClaimPatch implements ClaimPatch {
  type: "unknown"
  confidence?: Confidence
  prop?: string

  constructor(obj: object) {
    if ("type" in obj && obj.type !== "unknown") {
      // eslint-disable-next-line @typescript-eslint/restrict-template-expressions
      throw new Error(`invalid type: ${obj.type}`)
    }
    this.type = "unknown"
    Object.assign(this, obj)
  }

  // New creates a new unknown claim from the patch.
  New(id: string): Claim {
    if (this.confidence === undefined || !this.prop) {
      throw new Error("incomplete patch")
    }

    return new UnknownClaim({ id, confidence: this.confidence, prop: { id: this.prop } })
  }

  // Validate checks the patch fields which are set, without access to the target claim.
  Validate(): void {
    if (this.confidence === undefined && !this.prop) {
      throw new Error("empty patch")
    }
    validatePatchConfidence(this.confidence)
  }

  // Apply applies the patch to an existing unknown claim.
  async Apply(claim: Claim): Promise<void> {
    this.Validate()
    if (!(claim instanceof UnknownClaim)) {
      throw new Error("not unknown claim")
    }

    if (this.confidence !== undefined) claim.confidence = this.confidence
    if (this.prop) claim.prop.id = this.prop

    await claim.Validate()
  }
}
