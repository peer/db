import type { Ref } from "vue"
import type { Router } from "vue-router"

import type { UserInfo } from "@/types"

import { computed, ref } from "vue"

import { postJSON } from "@/api"
import siteContext, { initialRoles, initialUserInfo } from "@/context"
import { currentAbsoluteURL, redirectServerSide } from "@/utils"

// currentUserInfo is the canonical reactive container for the signed-in
// user's identity. It is populated from the UserInfo response header
// the auth middleware emits on /context.json (read at boot in @/context).
//
// A null value means "anonymous": no validated access token cookie was
// presented when the boot fetch was made.
export const currentUserInfo = ref<UserInfo | null>(initialUserInfo)

// currentRoles tracks the role list parsed from the Roles response header.
export const currentRoles = ref<string[]>(initialRoles)

// currentIdentityId mirrors the currentUserInfo's subject.
export const currentIdentityId = computed(() => currentUserInfo.value?.subject ?? "")

// currentUsername is the currentUserInfo's username.
export const currentUsername = computed(() => currentUserInfo.value?.username ?? "")

// PeerDB permissions.
//
// Keep in sync with auth/permissions.go.
export const CAN_EDIT = "canEdit"
export const CAN_DOWNLOAD = "canDownload"

type Permission = "canEdit" | "canDownload"

// hasRole is the symmetric counterpart of auth.HasRole on the backend.
export function hasRole(role: string): boolean {
  return currentRoles.value.includes(role)
}

// hasPermission returns true if the current user has the given permission.
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
  } finally {
    lock.value -= 1
  }
}
