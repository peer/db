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

const prefix = siteContext.metadataHeaderPrefix ?? ""

// The auth middleware emits Roles and UserInfo response headers on every
// response, empty when the request is unauthenticated. We read them off the
// context.json fetch (which happens once on app boot) to seed the reactive
// auth state in @/auth without an extra round-trip. An empty (or absent)
// UserInfo subject is treated as "unauthenticated": empty roles and a null
// UserInfo.
function parseRoles(): string[] {
  const items = decodeMetadataListNamed(response.headers, prefix, "Roles")
  return items.filter((s): s is string => typeof s === "string")
}

function parseUserInfo(): UserInfo | null {
  const md = decodeMetadataNamed(response.headers, prefix, "UserInfo")
  const subject = md.subject
  if (typeof subject !== "string" || subject === "") {
    return null
  }
  const username = typeof md.username === "string" ? md.username : undefined
  return { subject, username }
}

export const initialRoles: string[] = parseRoles()
export const initialUserInfo: UserInfo | null = parseUserInfo()

// A logo variant: a logo path and the minimum viewport width (a CSS length) from which it is used.
export interface LogoVariant {
  minWidth: string
  src: string
}

// cssLengthToPx converts a logo breakpoint ("0", "48rem", "768px", "40em") to a pixel number for
// ordering. Unknown or missing units are treated as pixels.
function cssLengthToPx(length: string): number {
  const match = /^\s*([\d.]+)\s*(px|rem|em)?\s*$/.exec(length)
  if (!match) {
    return 0
  }
  const value = Number.parseFloat(match[1])
  return match[2] === "rem" || match[2] === "em" ? value * 16 : value
}

// logoVariants returns the configured logos ordered by their minimum viewport width ascending, so the
// first is the fallback (smallest) and the last is the full logo (largest). It is empty when no logo
// is configured, in which case callers fall back to showing the site title.
export function logoVariants(): LogoVariant[] {
  const logo = siteContext.logo
  if (!logo) {
    return []
  }
  return Object.entries(logo)
    .map(([minWidth, src]) => ({ minWidth, src }))
    .sort((a, b) => cssLengthToPx(a.minWidth) - cssLengthToPx(b.minWidth))
}

export default siteContext
