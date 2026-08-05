import { Identifier } from "@tozd/identifier"
import { assert, describe, test } from "vitest"

import { SCOPE_SELF } from "@/auth"
import { ACTION_READ, ACTION_UPDATE, HAS_PERMISSION, PERMISSION_SCOPE, PERMISSION_USER } from "@/core"
import { ClaimTypes, HighConfidence } from "@/document"
import { usersWithDocumentPermission } from "@/permissions"

function id(): string {
  return Identifier.new().toString()
}

// Build a raw HAS_PERMISSION claim granting the action to the users, scoped to the document itself.
function rawGrant(action: string, users: string[]): object {
  return {
    id: id(),
    confidence: HighConfidence,
    prop: { id: HAS_PERMISSION },
    to: { id: action },
    sub: {
      id: users.map((user) => ({ id: id(), confidence: HighConfidence, prop: { id: PERMISSION_USER }, value: user })),
      string: [{ id: id(), confidence: HighConfidence, prop: { id: PERMISSION_SCOPE }, string: SCOPE_SELF }],
    },
  }
}

async function makeClaims(grants: object[]): Promise<ClaimTypes> {
  const ct = new ClaimTypes({ ref: grants })
  await ct.Validate()
  return ct
}

const user1 = "one@example.com"
const user2 = "two@example.com"

describe("usersWithDocumentPermission", () => {
  test("returns the users a claim grants the action to", async () => {
    const ct = await makeClaims([rawGrant(ACTION_READ, [user1, user2])])
    assert.deepEqual(usersWithDocumentPermission(ct, ACTION_READ), [user1, user2])
  })

  test("leaves out users granted only another action", async () => {
    const ct = await makeClaims([rawGrant(ACTION_READ, [user1]), rawGrant(ACTION_UPDATE, [user2])])
    assert.deepEqual(usersWithDocumentPermission(ct, ACTION_READ), [user1])
    assert.deepEqual(usersWithDocumentPermission(ct, ACTION_UPDATE), [user2])
  })

  test("names a user once however many claims grant them the action", async () => {
    const ct = await makeClaims([rawGrant(ACTION_READ, [user1]), rawGrant(ACTION_READ, [user1])])
    assert.deepEqual(usersWithDocumentPermission(ct, ACTION_READ), [user1])
  })

  test("returns nothing when the document grants nothing", async () => {
    const ct = await makeClaims([])
    assert.deepEqual(usersWithDocumentPermission(ct, ACTION_READ), [])
  })
})
