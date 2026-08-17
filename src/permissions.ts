import type { DeepReadonly } from "vue"
import type { ComposerTranslation } from "vue-i18n"

import type { ClaimTypes } from "@/document"

import { actionRequirements, actionsClosure, permissionClaims, permissionClaimUsers } from "@/auth"
import { ACTION_DELETE, ACTION_READ, ACTION_READ_HISTORIC, ACTION_UPDATE, ACTION_UPDATE_PERMISSIONS, DESCRIPTION, HAS_PERMISSION, HAS_REQUESTED_PERMISSION } from "@/core"
import { getBestClaimOfType } from "@/document"

// PermissionAction is one permission action offered to the user: its document ID, its label and the
// hint describing what it allows, and the actions it directly requires.
//
// The label and the hint are t() call results and not message keys, so that a search for a translation
// finds where it is used.
export interface PermissionAction {
  id: string
  label: (t: ComposerTranslation) => string
  hint: (t: ComposerTranslation) => string
  requires: string[]
}

// permissionActions are the permission actions a document's own permission claims can grant: the
// actions the permissions tab manages and an access request can ask for. The create action is missing
// on purpose, because a document's own claims never grant it (see auth.PermissionClaimGrants in
// auth/permissions.go), and so is the bulk read action, which is not about a particular document.
//
// The requirements are what an action builds on, so they are chosen together with it (see
// permissionActionsWith). They are the rule the permission checks apply as well, so they are taken
// from there (see actionRequirements) rather than restated.
export const permissionActions: PermissionAction[] = [
  {
    id: ACTION_READ,
    label: (t) => t("common.permissionActions.read"),
    hint: (t) => t("common.permissionActionHints.read"),
    requires: actionRequirements[ACTION_READ] ?? [],
  },
  {
    id: ACTION_READ_HISTORIC,
    label: (t) => t("common.permissionActions.history"),
    hint: (t) => t("common.permissionActionHints.history"),
    requires: actionRequirements[ACTION_READ_HISTORIC] ?? [],
  },
  {
    id: ACTION_UPDATE,
    label: (t) => t("common.permissionActions.update"),
    hint: (t) => t("common.permissionActionHints.update"),
    requires: actionRequirements[ACTION_UPDATE] ?? [],
  },
  {
    id: ACTION_UPDATE_PERMISSIONS,
    label: (t) => t("common.permissionActions.permissions"),
    hint: (t) => t("common.permissionActionHints.permissions"),
    requires: actionRequirements[ACTION_UPDATE_PERMISSIONS] ?? [],
  },
  {
    id: ACTION_DELETE,
    label: (t) => t("common.permissionActions.delete"),
    hint: (t) => t("common.permissionActionHints.delete"),
    requires: actionRequirements[ACTION_DELETE] ?? [],
  },
]

// permissionActionsWith returns the actions with the given action added, together with everything the
// added action requires, transitively.
export function permissionActionsWith(actions: Iterable<string>, action: string): Set<string> {
  return actionsClosure([...actions, action])
}

// permissionActionsWithout returns the actions with the given action removed, together with every
// action which requires it, directly or through another action: an action cannot stay chosen without
// what it builds on.
export function permissionActionsWithout(actions: Iterable<string>, action: string): Set<string> {
  const result = new Set(actions)
  const pending = [action]
  while (pending.length > 0) {
    const current = pending.pop()!
    result.delete(current)
    for (const other of permissionActions) {
      if (result.has(other.id) && other.requires.includes(current)) {
        pending.push(other.id)
      }
    }
  }
  return result
}

// PermissionGrant is the access one user has been granted on a document: the actions granted to them,
// each with the IDs of the claims granting it (the same action can be granted by more than one claim).
export interface PermissionGrant {
  user: string
  actions: Map<string, string[]>
}

// PermissionRequest is one access request recorded on a document which has not been decided yet: who
// asked for which action, the note they attached (empty when they attached none), and the ID of the
// claim recording the request.
export interface PermissionRequest {
  claimID: string
  user: string
  action: string
  note: string
}

// permissionGrants returns the access the document's own permission claims grant, one entry per user,
// in the order in which the users first appear among the claims. It is what the permissions tab lists
// and manages, so it counts a claim exactly as the permission checks do (see permissionClaims): a
// claim which is not scoped to the document grants nobody anything and is left out, and a claim naming
// several users grants it to each of them.
export function permissionGrants(claims: DeepReadonly<ClaimTypes>): PermissionGrant[] {
  const grants: PermissionGrant[] = []
  for (const [claim, action] of permissionClaims(claims, HAS_PERMISSION)) {
    for (const user of permissionClaimUsers(claim)) {
      let grant = grants.find((g) => g.user === user)
      if (!grant) {
        grant = { user, actions: new Map() }
        grants.push(grant)
      }
      const claimIDs = grant.actions.get(action) ?? []
      claimIDs.push(claim.id)
      grant.actions.set(action, claimIDs)
    }
  }
  return grants
}

// usersWithDocumentPermission returns the subjects of the users the document's own permission claims
// grant the action to, in the order the claims name them (see permissionGrants). Only what the
// document grants counts: a user who holds the action through their roles was never added to the
// document and is not among them, which is what makes this the list of users somebody put on the
// document.
export function usersWithDocumentPermission(claims: DeepReadonly<ClaimTypes>, action: string): string[] {
  return permissionGrants(claims)
    .filter((grant) => grant.actions.has(action))
    .map((grant) => grant.user)
}

// permissionRequests returns the access requests recorded on the document, in the order of their
// claims, counted by the same rules as the grants (see permissionGrants), so a claim asking on behalf
// of several users is one request per user. Requests are removed when they are decided, so all of them
// are still pending.
export function permissionRequests(claims: DeepReadonly<ClaimTypes>): PermissionRequest[] {
  const requests: PermissionRequest[] = []
  for (const [claim, action] of permissionClaims(claims, HAS_REQUESTED_PERMISSION)) {
    // The note is written under the claim, so every user it asks on behalf of comes with it.
    const note = getBestClaimOfType(claim.sub, "string", DESCRIPTION)?.string ?? ""
    for (const user of permissionClaimUsers(claim)) {
      requests.push({ claimID: claim.id, user, action, note })
    }
  }
  return requests
}

// permissionActionLabel returns an action's label, or null for an action which is not one of
// permissionActions (nothing stops a client from requesting any action), which the caller then shows
// by its identifier instead.
export function permissionActionLabel(action: string, t: ComposerTranslation): string | null {
  const found = permissionActions.find((a) => a.id === action)
  return found ? found.label(t) : null
}

// permissionActionHint returns the hint describing what an action allows, or null for an action which
// is not one of permissionActions, which has no description to show.
export function permissionActionHint(action: string, t: ComposerTranslation): string | null {
  const found = permissionActions.find((a) => a.id === action)
  return found ? found.hint(t) : null
}

// permissionActionOrder returns the position of an action in permissionActions, and the length of the
// list for an action which is not on it, so that a sort by it lists actions the way the permissions tab
// lists them and puts any other action last.
export function permissionActionOrder(action: string): number {
  const index = permissionActions.findIndex((a) => a.id === action)
  return index < 0 ? permissionActions.length : index
}
