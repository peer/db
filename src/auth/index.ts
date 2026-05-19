import type { Ref } from "vue"

import * as client from "openid-client"
import { ref } from "vue"

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

// accessToken is the bearer token attached to API requests in @/api. It is
// reactive so components can react to sign-in / sign-out, and it lives in
// memory only - refreshing the page wipes it, which the OIDC redirect dance
// will recover on its own when the issuer's session is still valid.
//
// TODO: Wrap fetch in @/api so it transparently refreshes the access token
//       instead of forcing callers to react to expiry.
export const accessToken = ref("")

// currentIdentityId mirrors the sub claim from the ID token returned by the
// issuer. Useful to components that want to know who is signed in without
// decoding the access token themselves.
export const currentIdentityId = ref("")

// isOIDCConfigured reports whether the server exposed an OIDC config in
// context.json. Components use this to decide whether to render sign-in UI.
export function isOIDCConfigured(): boolean {
  return siteContext.oidc !== undefined
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

  const redirectTo = client
    .buildAuthorizationUrl(config, {
      redirect_uri: oidc.redirectUri,
      code_challenge: codeChallenge,
      code_challenge_method: "S256",
      scope: "openid profile email",
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
