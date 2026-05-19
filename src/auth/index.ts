import type { ComputedRef, Ref } from "vue"

import * as client from "openid-client"
import { computed, ref, watch } from "vue"

// siteContext is fetched eagerly by @/context (the server already sends a
// preload header for context.json), so importing it here does not add any
// extra request - we just read the OIDC config it already exposes.
import siteContext from "@/context"
import { currentAbsoluteURL, redirectServerSide, replaceLocationSearch } from "@/utils"

// State persisted in localStorage between the sign-in redirect and the
// callback. It is keyed by the OAuth state string we generated, so concurrent
// sign-in attempts (or a stale attempt left behind) do not collide.
type State = {
  redirect: string
  codeVerifier: string
  nonce: string
}

// localStorage keys for the persisted session. We namespace them with "peerdb-"
// so they cannot collide with the per-flow OAuth State entries (which are keyed
// by a random stateId).
const accessTokenStorageKey = "peerdb-access-token"
const currentIdentityIdStorageKey = "peerdb-current-identity-id"

// TODO: Move off localStorage.
//       Persisting the bearer token here lets the session survive page reloads, but it also makes the token an XSS target.

// accessToken is the bearer token attached to API requests in @/api. It is
// reactive so components can react to sign-in / sign-out, and is mirrored to
// localStorage so the session survives reloads.
//
// TODO: Wrap fetch in @/api so it transparently refreshes the access token
//       instead of forcing callers to react to expiry.
export const accessToken = ref(localStorage.getItem(accessTokenStorageKey) ?? "")

// currentIdentityId mirrors the sub claim from the ID token returned by the
// issuer. Useful to components that want to know who is signed in without
// decoding the access token themselves.
export const currentIdentityId = ref(localStorage.getItem(currentIdentityIdStorageKey) ?? "")

// currentRoles is the reactive list of roles granted to the signed-in user,
// derived from the access token's scope claim (role.<key> entries). It updates
// automatically on sign-in, sign-out, and on reload once accessToken is rehydrated
// from localStorage. Use it directly in templates (Vue tracks the .value access)
// for things like v-if="currentRoles.includes('admin')".
export const currentRoles: ComputedRef<string[]> = computed(() => extractRolesFromClaims(decodeJWTPayload(accessToken.value)))

// hasRole is the symmetric counterpart of auth.HasRole on the backend: a small
// convenience for callers that just want to check one role. It is reactive
// when called from a template or another computed because it reads
// currentRoles.value, which Vue tracks.
export function hasRole(role: string): boolean {
  return currentRoles.value.includes(role)
}

// Mirror in-memory writes back to localStorage so a reload picks up the same
// session. Empty values remove the entry rather than storing "" so we do not
// leak stale keys after sign-out.
watch(accessToken, (value) => {
  if (value) {
    localStorage.setItem(accessTokenStorageKey, value)
  } else {
    localStorage.removeItem(accessTokenStorageKey)
  }
})
watch(currentIdentityId, (value) => {
  if (value) {
    localStorage.setItem(currentIdentityIdStorageKey, value)
  } else {
    localStorage.removeItem(currentIdentityIdStorageKey)
  }
})

// isOIDCConfigured reports whether the server exposed an OIDC config in
// context.json. Components use this to decide whether to render sign-in UI.
export function isOIDCConfigured(): boolean {
  return siteContext.oidc !== undefined
}

// Charon's role-scope convention: every granted scope under "role." names a
// role the signed-in user holds. We mirror the backend's auth package so the
// frontend reads the same role set the server will enforce.
const roleScopePrefix = "role."
const roleScopeWildcard = "role.*"

// decodeJWTPayload decodes the middle segment of a JWT into a claims object.
// We intentionally do not verify the signature here - the backend re-validates
// the token on every API request, and the frontend only needs the claims for
// UI hints (e.g. which menus to show). Returns null if the token does not look
// like a JWT or the payload cannot be decoded.
function decodeJWTPayload(token: string): Record<string, unknown> | null {
  const parts = token.split(".")
  if (parts.length !== 3) {
    return null
  }
  try {
    // JWT uses base64url. atob expects standard base64, so swap the alphabet
    // and re-pad to a multiple of 4. Then decode UTF-8 explicitly so non-ASCII
    // claims (e.g. names) round-trip correctly.
    const b64 = parts[1].replace(/-/g, "+").replace(/_/g, "/")
    const padded = b64.padEnd(Math.ceil(b64.length / 4) * 4, "=")
    const binary = atob(padded)
    const bytes = Uint8Array.from(binary, (c) => c.charCodeAt(0))
    const json = new TextDecoder().decode(bytes)
    return JSON.parse(json) as Record<string, unknown>
  } catch {
    return null
  }
}

// extractRolesFromClaims mirrors auth.extractRoles on the backend: take every
// scope under the "role." namespace, strip the prefix, and dedupe; ignore the
// bare "role.*" wildcard if it ever shows up.
function extractRolesFromClaims(payload: Record<string, unknown> | null): string[] {
  if (!payload) {
    return []
  }
  const scopes: string[] = []
  if (typeof payload.scope === "string") {
    scopes.push(...payload.scope.split(/\s+/).filter(Boolean))
  }
  if (Array.isArray(payload.scp)) {
    for (const s of payload.scp) {
      if (typeof s === "string") {
        scopes.push(s)
      }
    }
  }
  const seen = new Set<string>()
  const roles: string[] = []
  for (const scope of scopes) {
    if (scope === roleScopeWildcard) {
      continue
    }
    if (!scope.startsWith(roleScopePrefix)) {
      continue
    }
    const role = scope.slice(roleScopePrefix.length)
    if (!role || seen.has(role)) {
      continue
    }
    seen.add(role)
    roles.push(role)
  }
  return roles
}

// isSignedIn reports whether we currently hold a valid-looking access token.
// "Valid-looking" because we do not check expiry here; the backend will reject
// expired tokens and the UI will surface a 401 like any other error.
export function isSignedIn(): boolean {
  return accessToken.value !== ""
}

// configPromise is lazy because openid-client's discovery() makes a network
// request and we want to defer it until the user actually clicks Sign In, not
// pay for it on every page load.
let configPromise: Promise<client.Configuration> | null = null

function getConfig(): Promise<client.Configuration> {
  const oidc = siteContext.oidc
  if (!oidc) {
    return Promise.reject(new Error("OIDC is not configured for this site"))
  }
  if (!configPromise) {
    configPromise = client.discovery(new URL(oidc.issuer), oidc.clientId)
  }
  return configPromise
}

export async function signIn(progress: Ref<number>) {
  const oidc = siteContext.oidc
  if (!oidc) {
    return
  }
  const config = await getConfig()

  const codeVerifier = client.randomPKCECodeVerifier()
  const codeChallenge = await client.calculatePKCECodeChallenge(codeVerifier)
  const stateId = client.randomState()
  const nonce = client.randomNonce()

  // role.* is the Charon wildcard scope. Requesting it tells the issuer to
  // expand it into one role.<key> grant for every role the signed-in user
  // actually holds in the organization; those grants are what auth.Roles on
  // the backend reads out of the access token's scope claim. Without it the
  // backend always sees an empty role set even for users with assigned roles.
  // The OAuth client must be registered with role.* in its allowed scopes
  // (Charon's app template "IDScopes") for this to be granted.
  const redirectTo = client
    .buildAuthorizationUrl(config, {
      redirect_uri: oidc.redirectUri,
      code_challenge: codeChallenge,
      code_challenge_method: "S256",
      scope: "openid profile email role.*",
      state: stateId,
      nonce,
    })
    .toString()

  const state: State = {
    redirect: currentAbsoluteURL(),
    codeVerifier,
    nonce,
  }
  localStorage.setItem(stateId, JSON.stringify(state))

  redirectServerSide(redirectTo, false, progress)
}

// processOIDCRedirect runs unconditionally on app startup and short-circuits
// when there is nothing to do. The callback URL is the same SPA URL the user
// signed in from, so we cannot route the callback to a dedicated component;
// instead we detect the "state" query parameter and consume it before Vue
// Router takes over.
export async function processOIDCRedirect() {
  if (!siteContext.oidc) {
    return
  }
  const stateId = new URLSearchParams(window.location.search).get("state")
  if (!stateId) {
    return
  }
  const url = new URL(window.location.href)
  replaceLocationSearch("")

  const stateJSON = localStorage.getItem(stateId)
  if (!stateJSON) {
    // TODO: Surface this to the user somehow - losing the localStorage entry
    //       means the callback cannot complete and we silently drop the
    //       sign-in attempt.
    return
  }
  localStorage.removeItem(stateId)
  const state = JSON.parse(stateJSON) as State

  const config = await getConfig()
  const tokens = await client.authorizationCodeGrant(config, url, {
    pkceCodeVerifier: state.codeVerifier,
    expectedState: stateId,
    expectedNonce: state.nonce,
  })

  const claims = tokens.claims()
  if (!claims) {
    throw new Error("missing ID token")
  }

  // TODO: Schedule clearing accessToken when the token expires (parse exp).
  accessToken.value = tokens.access_token
  currentIdentityId.value = claims.sub
}

// signOut clears the local token state. We do not try to revoke the token
// against the issuer here - PeerDB does not host its own OIDC introspection
// or revocation endpoint, so clearing the in-memory token is the strongest
// guarantee we can offer locally. Components that want issuer-side logout
// (RP-initiated logout) should add a redirect to the issuer's end_session
// endpoint on top of this.
export function signOut(_progress: Ref<number>) {
  accessToken.value = ""
  currentIdentityId.value = ""
}
