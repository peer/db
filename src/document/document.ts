import type { Claim, ClaimsContainer } from "@/document/claims"
import type { Reference } from "@/document/types"

import { Identifier } from "@tozd/identifier"

import { ClaimTypes } from "@/document/claims"

// CoreDocument contains the core fields present in all PeerDB documents.
class CoreDocument {
  id!: string
  base!: string[]

  // GetID returns the document's identifier.
  GetID(): string {
    return this.id
  }

  // Validate checks that the document has a valid identifier.
  async Validate(): Promise<void> {
    const expectedID = (await Identifier.from(...this.base)).toString()
    if (this.id !== expectedID) {
      throw new Error(`invalid ID: expected ${expectedID}, id ${this.id}`)
    }
  }
}

// D represents a PeerDB document.
export class D extends CoreDocument implements ClaimsContainer {
  claims?: ClaimTypes

  constructor(obj: object) {
    super()
    Object.assign(this, obj)
    if (!this.id) {
      throw new Error("id is required")
    }
    if (!this.base) {
      throw new Error("base is required")
    }
    if (this.claims !== undefined) {
      this.claims = new ClaimTypes(this.claims)
    }
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

  Get(propID: string): Claim[] {
    if (this.claims === undefined) {
      return []
    }
    return this.claims.Get(propID)
  }

  Remove(propID: string): Claim[] {
    if (this.claims === undefined) {
      return []
    }
    return this.claims.Remove(propID)
  }

  Size(): number {
    if (this.claims === undefined) {
      return 0
    }
    return this.claims.Size()
  }

  AllClaims(): Claim[] {
    if (this.claims === undefined) {
      return []
    }
    return this.claims.AllClaims()
  }

  // Reference returns a Reference to this document.
  Reference(): Reference {
    return { id: this.id }
  }

  // Validate validates the document ID and all its claims.
  async Validate(): Promise<void> {
    await super.Validate()
    if (this.claims !== undefined) {
      await this.claims.Validate()
    }
  }
}
