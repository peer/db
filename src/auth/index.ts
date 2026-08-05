import type { DeepReadonly, Ref } from "vue"
import type { Router } from "vue-router"

import type { ClaimTypes } from "@/document"
import type { UserInfo } from "@/types"

import { Identifier } from "@tozd/identifier"
import { computed, ref } from "vue"

import { clearCache, postJSON } from "@/api"
import siteContext, { initialRoles, initialUserInfo } from "@/context"
import {
  ACTION_CREATE,
  ACTION_DELETE,
  ACTION_READ,
  ACTION_READ_BULK,
  ACTION_READ_HISTORIC,
  ACTION_UPDATE,
  ACTION_UPDATE_PERMISSIONS,
  HAS_PERMISSION,
  PERMISSION_SCOPE,
  PERMISSION_USER,
} from "@/core"
import { getClaimsOfTypeWithConfidence } from "@/document"
import { ENTRY_SEPARATOR, parseEntry } from "@/shortcut"
import { currentAbsoluteURL, redirectServerSide } from "@/utils"

// currentUserInfo is the canonical reactive container for the signed-in
// user's identity. It is populated from the UserInfo response header
// the auth middleware emits on /context.json (read at boot in @/context).
//
// A null value means "unauthenticated": no validated access token cookie was
// presented when the boot fetch was made.
export const currentUserInfo = ref<UserInfo | null>(initialUserInfo)

// currentRoles tracks the role list parsed from the Roles response header.
export const currentRoles = ref<string[]>(initialRoles)

// authEpoch increments on every in-app identity change that is not accompanied
// by a full page load (currently only sign-out). The root <router-view> is keyed
// on it (see App.vue), so the whole component tree remounts and refetches
// results, filters and documents under the new roles. Sign-in does not need
// this: it round-trips through a server-side redirect that fully reloads the app.
export const authEpoch = ref(0)

// currentIdentityId mirrors the currentUserInfo's subject.
export const currentIdentityId = computed(() => currentUserInfo.value?.subject ?? "")

// currentUsername is the currentUserInfo's username.
export const currentUsername = computed(() => currentUserInfo.value?.username ?? "")

// hasRole is the symmetric counterpart of auth.HasRole on the backend.
export function hasRole(role: string): boolean {
  return currentRoles.value.includes(role)
}

// ROLE_EVERYONE is a reserved role name under which sites can declare permissions which apply
// to every caller, authenticated or not. It is not a real role and never appears in currentRoles.
//
// Keep in sync with auth/permissions.go.
export const ROLE_EVERYONE = ""

// Literal permission scopes. Besides them, a scope entry in role grants has the form "property=value"
// (both resolved document IDs in siteContext.roles), which scopes the action to documents carrying a
// reference claim with that property and value.
//
// Keep in sync with auth/permissions.go.

// SCOPE_ALL scopes an action to all objects (documents and files). It is valid only in role grants.
export const SCOPE_ALL = "all"
// SCOPE_FILES scopes an action to all files. It is valid only in role grants.
export const SCOPE_FILES = "files"
// SCOPE_DOCUMENTS scopes an action to all documents. It is valid only in role grants.
export const SCOPE_DOCUMENTS = "documents"
// SCOPE_SELF scopes an action to the document carrying the permission claim. It is valid only in
// document-level permission claims.
export const SCOPE_SELF = "self"

// Scope is one parsed entry of a permission scope expression. Either literal holds one of the literal
// scopes, or prop and value hold a claim scope: the scope then matches a document carrying a reference
// claim with that property and value. In sync with auth.Scope in auth/permissions.go.
export class Scope {
  readonly literal: string
  readonly prop: string
  readonly value: string

  constructor(literal: string, prop: string, value: string) {
    this.literal = literal
    this.prop = prop
    this.value = value
  }

  // CoversDocuments reports whether the scope can cover any document at all. It is the optimistic
  // document check used when no concrete document is available, with the definitive
  // scopesAllowDocument check following once one is. In sync with auth.Scope.CoversDocuments in
  // auth/permissions.go.
  CoversDocuments(): boolean {
    switch (this.literal) {
      case SCOPE_ALL:
      case SCOPE_DOCUMENTS:
        // The all and documents scopes cover every document.
        return true
      case SCOPE_SELF:
        // The self scope never appears in role grants.
        throw new Error(`unexpected scope: ${this.literal}`)
      case SCOPE_FILES:
        // The files scope covers no document.
        return false
      case "":
        // A claim scope covers the documents carrying the matching reference claim.
        // We return true here and expect that the definitive scopesAllowDocument check follows this
        // call once the document is available.
        return true
      default:
        throw new Error(`unknown scope: ${this.literal}`)
    }
  }

  // MatchesFiles reports whether the scope covers files. In sync with auth.Scope.MatchesFiles in
  // auth/permissions.go.
  MatchesFiles(): boolean {
    switch (this.literal) {
      case SCOPE_ALL:
      case SCOPE_FILES:
        return true
      case SCOPE_DOCUMENTS:
        return false
      case SCOPE_SELF:
        // The self scope never appears in role grants.
        throw new Error(`unexpected scope: ${this.literal}`)
      case "":
        // A claim scope covers only documents.
        return false
      default:
        throw new Error(`unknown scope: ${this.literal}`)
    }
  }
}

// parseScopes parses a permission scope expression: "&"-separated entries, each either a literal scope
// (all, files, documents, or self) or a claim scope of the form "property=value". In sync with
// auth.ParseScopes in auth/permissions.go, with one difference: the backend resolves identifier
// tokens, while here both sides of a claim scope must already be resolved document IDs, because
// resolution is async and never needed (role grants arrive resolved from the backend and
// document-level permission claims validly contain only the self scope). Throws on the first invalid
// entry.
export function parseScopes(expression: string): Scope[] {
  const scopes: Scope[] = []
  for (const term of expression.split(ENTRY_SEPARATOR)) {
    if (term === SCOPE_ALL || term === SCOPE_FILES || term === SCOPE_DOCUMENTS || term === SCOPE_SELF) {
      scopes.push(new Scope(term, "", ""))
      continue
    }
    const { key, value } = parseEntry(term)
    if (!Identifier.valid(key) || !Identifier.valid(value)) {
      throw new Error(`scope entry must be a property and a value, each a single identifier: ${term}`)
    }
    scopes.push(new Scope("", key, value))
  }
  return scopes
}

// PermissionDocument is the structural subset of a document (the D class or a DeepReadonly view of it)
// that permission checks read: its claims.
type PermissionDocument = {
  claims?: DeepReadonly<ClaimTypes> | null
}

// scopesAllowDocument reports whether the scopes, evaluated together, allow an action on the
// document. The all and documents scopes allow every document on their own. Claim scopes have to be
// fully satisfied by the document: for every property among them which the document carries, every
// value the document carries has to be granted (so a document with multiple instance_of claims
// requires all of its classes to be granted), and at least one such property has to be present on
// the document. The files scope allows no document, and the self scope never appears here: it is
// valid only in document-level permission claims, which are evaluated by permissionClaimGrants, not
// as scopes. In sync with auth.scopesAllowDocument in auth/permissions.go.
function scopesAllowDocument(scopes: readonly Scope[], doc: PermissionDocument): boolean {
  const values = new Map<string, Set<string>>()
  for (const scope of scopes) {
    switch (scope.literal) {
      case SCOPE_ALL:
      case SCOPE_DOCUMENTS:
        return true
      case SCOPE_SELF:
        // The self scope never appears in role grants (the backend rejects it in role configuration).
        throw new Error(`unexpected scope: ${scope.literal}`)
      case SCOPE_FILES:
        continue
      case "": {
        let granted = values.get(scope.prop)
        if (!granted) {
          granted = new Set()
          values.set(scope.prop, granted)
        }
        granted.add(scope.value)
        break
      }
      default:
        throw new Error(`unknown scope: ${scope.literal}`)
    }
  }

  let present = false
  for (const [prop, granted] of values) {
    const claims = getClaimsOfTypeWithConfidence(doc.claims, "ref", prop)
    if (claims.length === 0) {
      continue
    }
    for (const claim of claims) {
      if (!granted.has(claim.to.id)) {
        return false
      }
    }
    present = true
  }
  return present
}

// RoleGrants maps permission actions (ACTION_* document IDs) to the scopes they are granted with. In
// sync with auth.RoleGrants in auth/permissions.go.
type RoleGrants = { [action: string]: Scope[] }

// parsedRoles caches the site's role grants with every scope string parsed, so permission checks
// evaluate parsed scopes like the backend does. The backend parses role configuration once at startup;
// the frontend mirrors that by parsing siteContext.roles (which is static after boot) once, on the
// first permission check.
let parsedRoles: { [roleName: string]: RoleGrants } | null = null

// siteGrants returns the site's role grants parsed into Scope lists, parsing and caching them on the
// first call.
function siteGrants(): { [roleName: string]: RoleGrants } {
  if (parsedRoles === null) {
    const roles: { [roleName: string]: RoleGrants } = {}
    for (const [roleName, actions] of Object.entries(siteContext.roles ?? {})) {
      const grants: RoleGrants = {}
      for (const [action, expressions] of Object.entries(actions)) {
        grants[action] = expressions.flatMap((expression) => parseScopes(expression))
      }
      roles[roleName] = grants
    }
    parsedRoles = roles
  }
  return parsedRoles
}

// siteRoles returns the names of the roles the site declares, in alphabetical order, without the
// reserved everyone entry, which is not a role anybody holds. In sync with siteRoleNames in serve.go.
export function siteRoles(): string[] {
  return Object.keys(siteGrants())
    .filter((role) => role !== ROLE_EVERYONE)
    .sort()
}

// permissionClaimGrants returns, per permission action, the users the document's own permission
// claims grant that action. A HAS_PERMISSION claim grants the action it references to the users its
// PERMISSION_USER sub-claims name when one of its PERMISSION_SCOPE sub-claims carries the self
// scope, the only scope valid in document-level permission claims (other scopes are ignored). The
// claim and both sub-claims count only at or above low confidence, and an empty user is ignored
// (permission claims always name a user). The create action is never granted: a document's own
// claims cannot grant creating it. In sync with auth.PermissionClaimGrants in auth/permissions.go,
// which additionally sorts the users per action, while here only membership is consulted, so users
// are sets.
function permissionClaimGrants(doc: PermissionDocument): Map<string, Set<string>> {
  const users = new Map<string, Set<string>>()
  for (const [claim, action] of permissionClaims(doc.claims, HAS_PERMISSION)) {
    // A document's own claims never grant creating it, so a claim of the create action grants nobody
    // anything.
    if (action === ACTION_CREATE) {
      continue
    }
    for (const user of permissionClaimUsers(claim)) {
      let granted = users.get(action)
      if (!granted) {
        granted = new Set()
        users.set(action, granted)
      }
      granted.add(user)
    }
  }
  return users
}

// PermissionClaim is one of a document's own permission claims: a reference claim of the
// HAS_PERMISSION or HAS_REQUESTED_PERMISSION property.
export type PermissionClaim = Required<DeepReadonly<ClaimTypes>>["ref"][number]

// hasSelfScope reports whether the permission claim is scoped to the document carrying it, the only
// scope which counts at document level. Other scopes are not allowed there but are ignored. In sync
// with auth.hasSelfScope in auth/permissions.go.
function hasSelfScope(claim: PermissionClaim): boolean {
  for (const sub of getClaimsOfTypeWithConfidence(claim.sub, "string", PERMISSION_SCOPE)) {
    let parsed: Scope[]
    try {
      parsed = parseScopes(sub.string)
    } catch {
      // A sub-claim which does not parse contributes nothing, like on the backend.
      continue
    }
    for (const scope of parsed) {
      if (scope.literal === SCOPE_SELF) {
        return true
      }
    }
  }
  return false
}

// permissionClaims iterates the document's own permission claims of the given property
// (HAS_PERMISSION or HAS_REQUESTED_PERMISSION) which count at document level, yielding each together
// with the action it references: the claim counts at or above low confidence, and a claim not scoped
// to the document carrying it is skipped. In sync with auth.permissionClaims in auth/permissions.go.
export function* permissionClaims(claims: DeepReadonly<ClaimTypes> | undefined | null, prop: string): Generator<[PermissionClaim, string]> {
  for (const claim of getClaimsOfTypeWithConfidence(claims, "ref", prop)) {
    if (!hasSelfScope(claim)) {
      continue
    }
    yield [claim, claim.to.id]
  }
}

// permissionClaimUsers iterates the users a permission claim names through its PERMISSION_USER
// sub-claims at or above low confidence, skipping an empty one: permission claims always name a user.
// In sync with auth.permissionClaimUsers in auth/permissions.go.
export function* permissionClaimUsers(claim: PermissionClaim): Generator<string> {
  for (const sub of getClaimsOfTypeWithConfidence(claim.sub, "id", PERMISSION_USER)) {
    if (!sub.value) {
      continue
    }
    yield sub.value
  }
}

// hasPermissionClaim reports whether the document's own permission claims grant the user the action
// (see permissionClaimGrants). In sync with auth.HasPermissionClaim in auth/permissions.go.
function hasPermissionClaim(action: string, user: string, doc: PermissionDocument): boolean {
  return permissionClaimGrants(doc).get(action)?.has(user) ?? false
}

// allowsDocument reports whether one role's scopes for the action allow the document (see
// scopesAllowDocument). Without a document it reports whether the role's scopes allow the action on
// documents at all (any scope which can cover a document counts). The document's own permission
// claims are not consulted: they grant actions to named users independently of any role (see
// permissionClaimGrants), and hasDocumentPermission combines both. In sync with
// auth.RoleGrants.AllowsDocument in auth/permissions.go.
function allowsDocument(scopes: readonly Scope[] | undefined, doc?: PermissionDocument | null): boolean {
  if (!doc) {
    return (scopes ?? []).some((scope) => scope.CoversDocuments())
  }
  return scopesAllowDocument(scopes ?? [], doc)
}

// hasDocumentPermission returns true if the current user holds the permission action (an ACTION_*
// document ID from @/core), through either of two independent arms: the role arm (a grant of the
// reserved ROLE_EVERYONE entry or of one of the caller's roles allows the document, see
// allowsDocument) or, when doc is given, the claim arm (the document's own permission claims grant
// the user the action, see permissionClaimGrants). Without doc only role grants are checked, against
// documents in general. In sync with auth.HasDocumentPermission in auth/permissions.go, with the
// identity taken from the reactive auth state (like checkDocumentPermission in permissions.go takes
// it from the request context).
export function hasDocumentPermission(action: string, doc?: PermissionDocument | null): boolean {
  return hasUserDocumentPermission(action, doc, currentIdentityId.value, currentRoles.value)
}

// actionRequirements are, per permission action, the actions it directly requires: an action is
// meaningful only together with them, because it builds on what they allow. Reading is the base of
// every access to a document, and managing permissions goes through the ordinary edit path, so it
// requires updating. The create action requires nothing: it is about documents which do not exist yet.
// In sync with actionRequirements in auth/permissions.go.
export const actionRequirements: Record<string, string[]> = {
  [ACTION_READ_BULK]: [ACTION_READ],
  [ACTION_READ_HISTORIC]: [ACTION_READ],
  [ACTION_UPDATE]: [ACTION_READ],
  [ACTION_UPDATE_PERMISSIONS]: [ACTION_UPDATE],
  [ACTION_DELETE]: [ACTION_READ],
}

// actionsClosure returns the actions together with everything they require, transitively. In sync with
// auth.ActionsClosure in auth/permissions.go.
export function actionsClosure(actions: Iterable<string>): Set<string> {
  const closure = new Set<string>()
  const pending = [...actions]
  while (pending.length > 0) {
    const action = pending.pop()!
    if (closure.has(action)) {
      continue
    }
    closure.add(action)
    pending.push(...(actionRequirements[action] ?? []))
  }
  return closure
}

// hasUserDocumentAction reports whether the user with the given subject and roles is granted the
// permission action, through the role arm (the reserved ROLE_EVERYONE entry or one of their roles) or,
// when doc is given, through the document's own permission claims naming them. In sync with
// hasDocumentAction in auth/permissions.go.
function hasUserDocumentAction(action: string, doc: PermissionDocument | null | undefined, user: string, roles: readonly string[]): boolean {
  const grants = siteGrants()
  if (allowsDocument(grants[ROLE_EVERYONE]?.[action], doc)) {
    return true
  }
  for (const role of roles) {
    if (allowsDocument(grants[role]?.[action], doc)) {
      return true
    }
  }
  return !!doc && hasPermissionClaim(action, user, doc)
}

// hasUserDocumentPermission is hasDocumentPermission asked about somebody else: whether the user with
// the given subject and roles holds the permission action, which they do when they are granted it and
// everything it requires (see actionRequirements), each through either arm. The roles of another user
// are not part of the reactive auth state (they are the caller's), so they come from the user API (see
// UserGetAPI in user.go). In sync with auth.HasDocumentPermission in auth/permissions.go.
export function hasUserDocumentPermission(action: string, doc: PermissionDocument | null | undefined, user: string, roles: readonly string[]): boolean {
  for (const required of actionsClosure([action])) {
    if (!hasUserDocumentAction(required, doc, user, roles)) {
      return false
    }
  }
  return true
}

// hasUserRoleDocumentPermission is hasUserDocumentPermission through the role arm alone: whether the
// roles hold the permission action on the document, without what the document's own permission claims
// grant to anyone. It answers what a user holds independently of the document, which is what a claim
// granting them the action would have to add to. In sync with checkRoleDocumentPermission in
// permissions.go, which asks the same by checking with no subject: no permission claim names one.
export function hasUserRoleDocumentPermission(action: string, doc: PermissionDocument | null | undefined, roles: readonly string[]): boolean {
  return hasUserDocumentPermission(action, doc, "", roles)
}

// hasFilePermission returns true if the current user holds the permission action on files, through a
// role grant with a scope covering files (under the reserved ROLE_EVERYONE entry or one of the
// caller's roles). In sync with auth.HasFilePermission in auth/permissions.go, with the identity taken
// from the reactive auth state (like checkFilePermission in permissions.go takes it from the request
// context).
export function hasFilePermission(action: string): boolean {
  const grants = siteGrants()
  if ((grants[ROLE_EVERYONE]?.[action] ?? []).some((scope) => scope.MatchesFiles())) {
    return true
  }
  for (const role of currentRoles.value) {
    if ((grants[role]?.[action] ?? []).some((scope) => scope.MatchesFiles())) {
      return true
    }
  }
  return false
}

// scopeProperties returns the set of properties participating in claim scopes of any role grant of the
// site. Claims of these properties can be requested at document creation but cannot be changed as
// ordinary client changes. In sync with auth.ScopeProperties in auth/permissions.go.
export function scopeProperties(): Set<string> {
  const properties = new Set<string>()
  for (const grants of Object.values(siteGrants())) {
    for (const scopes of Object.values(grants)) {
      for (const scope of scopes) {
        if (scope.literal === "") {
          properties.add(scope.prop)
        }
      }
    }
  }
  return properties
}

// isSignedIn reports whether the user has a validated cookie session.
export function isSignedIn(): boolean {
  return currentUserInfo.value !== null
}

// signIn navigates the browser to the backend's AuthSignIn endpoint. The
// backend performs the authentication flow on its own and drops the browser
// back at the redirect target (default: the current page) with the session
// token cookie set. We use a server-side redirect because the browser must
// follow the issuer's 3xx to its sign-in form, which a fetch cannot do.
export function signIn(router: Router, lock: Ref<number>, redirect?: string) {
  if (!redirect) {
    redirect = currentAbsoluteURL()
  }

  const target = router.resolve({
    name: "AuthSignIn",
    query: { redirect },
  }).href
  redirectServerSide(target, false, lock)
}

// signOut POSTs to the backend's AuthSignOut API endpoint, which clears
// the cookie, then clears the local reactive state so the UI updates
// immediately. A 200/204 is the success case. We ignore the body.
export async function signOut(router: Router, abortSignal: AbortSignal, lock: Ref<number>) {
  lock.value += 1
  try {
    const url = router.apiResolve({ name: "AuthSignOut" }).href
    await postJSON(url, {}, abortSignal, null)
    if (abortSignal.aborted) {
      return
    }
    currentUserInfo.value = null
    currentRoles.value = []
    // Drop responses cached for the previous identity, then bump authEpoch to
    // remount the app so every component refetches under the new (signed-out)
    // roles.
    clearCache()
    authEpoch.value += 1
  } finally {
    lock.value -= 1
  }
}
