import type { Ref } from "vue"
import type { Router } from "vue-router"

import type { UserInfo } from "@/types"

import { computed, ref } from "vue"

import { clearCache, postJSON } from "@/api"
import siteContext, { initialRoles, initialUserInfo } from "@/context"
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

// PeerDB permissions.
//
// Keep in sync with auth/permissions.go.
export const CAN_EDIT_DOCUMENT = "canEditDocument"
export const CAN_DELETE_DOCUMENT = "canDeleteDocument"
export const CAN_CHANGES_DOCUMENT = "canChangesDocument"
export const CAN_BULK_GET_FILE = "canBulkGetFile"
export const CAN_CHANGES_FILE = "canChangesFile"
export const CAN_EDIT_FILE = "canEditFile"
export const CAN_DELETE_FILE = "canDeleteFile"

type Permission = "canEditDocument" | "canDeleteDocument" | "canChangesDocument" | "canBulkGetFile" | "canChangesFile" | "canEditFile" | "canDeleteFile"

// hasRole is the symmetric counterpart of auth.HasRole on the backend.
export function hasRole(role: string): boolean {
  return currentRoles.value.includes(role)
}

// hasPermission returns true if the current user has the given permission.
// In sync with auth/permissions.go.
export function hasPermission(permission: Permission): boolean {
  const roles = siteContext.roles
  if (!roles) {
    return false
  }
  for (const role of currentRoles.value) {
    if (roles[role]?.includes(permission)) {
      return true
    }
  }
  return false
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
