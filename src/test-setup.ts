import { vi } from "vitest"

import schemaJSON from "@/../document/schema.json"

vi.mock("@/context")

// InputHTML.schema builds the editor schema by fetching /schema.json at module load (top-level
// await), the same file the backend serves and embeds. In tests we serve the local
// document/schema.json so the editor uses the real schema without a backend; other fetches fall
// through to the environment's fetch.
const realFetch: typeof fetch | undefined = globalThis.fetch
vi.stubGlobal("fetch", async (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
  const url = typeof input === "string" ? input : input instanceof URL ? input.href : input.url
  if (url === "/schema.json") {
    return new Response(JSON.stringify(schemaJSON), { status: 200, headers: { "Content-Type": "application/json" } })
  }
  if (realFetch) {
    return realFetch(input, init)
  }
  throw new Error(`unmocked fetch in test: ${url}`)
})
