import type { SiteContext } from "@/types"

// TODO: Use import with import assertion?
//       See: https://github.com/vitejs/vite/issues/4934
// redirect: "error" cannot be set because then preload is not matched in Firefox.
export default (await fetch("/context.json", {
  method: "GET",
  // Mode and credentials match crossorigin=anonymous in link preload header.
  mode: "cors",
  credentials: "same-origin",
  referrer: document.location.href,
  referrerPolicy: "strict-origin-when-cross-origin",
}).then((response) => response.json())) as SiteContext
