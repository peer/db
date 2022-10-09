import type { SiteContext } from "@/types"

// TODO: Use import with import assertion?
//       See: https://github.com/vitejs/vite/issues/4934
export default (await fetch("/context.json").then((response) => response.json())) as SiteContext
