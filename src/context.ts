import type { SiteContext } from "@/types"

// TODO: Use import with import assertion?
//       See: https://github.com/vitejs/vite/issues/4934
export default (await fetch("/context.json", {
  method: "GET",
  headers: {
    Accept: "application/json",
  },
  mode: "same-origin",
  credentials: "omit",
  redirect: "error",
  referrer: document.location.href,
  referrerPolicy: "strict-origin-when-cross-origin",
}).then((response) => response.json())) as SiteContext
