import type { Claim, ClaimsContainer } from "@/document/claims"
import type { Reference } from "@/document/types"

import { Identifier } from "@tozd/identifier"

import { ClaimTypes } from "@/document/claims"
import { clone } from "@/utils"

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
  claims!: ClaimTypes

  constructor(obj: object) {
    super()
    Object.assign(this, obj)
    if (!this.id) {
      throw new Error("id is required")
    }
    if (!this.base) {
      throw new Error("base is required")
    }
    // Wrap raw JSON claims (from Object.assign) or initialize empty.
    this.claims = new ClaimTypes(this.claims ?? {})
  }

  // Clone returns a deep copy of the document.
  Clone(): D {
    // clone preserves prototypes, so the clone is a real D instance holding real Claim instances
    // (the Mutable mapped type just does not carry that through), and it unwraps a document held
    // in reactive state to its raw target first.
    return clone(this) as unknown as D
  }

  GetByID(id: string): Claim | undefined {
    return this.claims.GetByID(id)
  }

  RemoveByID(id: string): Claim | undefined {
    return this.claims.RemoveByID(id)
  }

  ReplaceByID(id: string, newClaim: Claim): Claim | undefined {
    return this.claims.ReplaceByID(id, newClaim)
  }

  Add(claim: Claim): void {
    this.claims.Add(claim)
  }

  Get(propID: string): Claim[] {
    return this.claims.Get(propID)
  }

  Remove(propID: string): Claim[] {
    return this.claims.Remove(propID)
  }

  Size(): number {
    return this.claims.Size()
  }

  // SizeWithSub returns the total number of claims in the document, counting recursively into sub-claims.
  SizeWithSub(): number {
    return this.claims.SizeWithSub()
  }

  AllClaims(): Claim[] {
    return this.claims.AllClaims()
  }

  // Reference returns a Reference to this document.
  Reference(): Reference {
    return { id: this.id }
  }

  // Validate validates the document ID and all its claims.
  async Validate(): Promise<void> {
    await super.Validate()
    await this.claims.Validate()
  }
}
