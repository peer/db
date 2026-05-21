import type { SiteContext, UserInfo } from "@/types"

import { decodeMetadataListNamed, decodeMetadataNamed } from "@/metadata"

// TODO: Use import with import assertion?
//       See: https://github.com/vitejs/vite/issues/4934
// redirect: "error" cannot be set because then preload is not matched in Firefox.
// We have a hard-coded URL here instead of resolving it from the router to simplify imports order.
const response = await fetch("/context.json", {
  method: "GET",
  // Mode and credentials match crossorigin=anonymous in link preload header.
  mode: "cors",
  credentials: "same-origin",
  referrer: document.location.href,
  referrerPolicy: "strict-origin-when-cross-origin",
})

const siteContext = (await response.json()) as SiteContext

// The auth middleware emits Roles and UserInfo response headers on every
// request that carries a validated access token. We read them off the
// context.json fetch (which happens once on app boot) to seed the reactive
// auth state in @/auth without an extra round-trip. Absent headers are
// treated as "anonymous": empty roles and a null UserInfo.
function parseRoles(): string[] {
  const items = decodeMetadataListNamed(response.headers, "Roles")
  return items.filter((s): s is string => typeof s === "string")
}

function parseUserInfo(): UserInfo | null {
  const md = decodeMetadataNamed(response.headers, "UserInfo")
  const subject = md.subject
  if (typeof subject !== "string" || subject === "") {
    return null
  }
  const username = typeof md.username === "string" ? md.username : undefined
  return { subject, username }
}

export const initialRoles: string[] = parseRoles()
export const initialUserInfo: UserInfo | null = parseUserInfo()

export default siteContext
