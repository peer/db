import { expect, fetchFromPage, goHome, PEERDB_URL, test } from "../utils"

// The addresses the server answers itself, without the application: the legal texts it ships, the machine
// readable descriptions of itself, and what it tells a crawler.
const STATIC_PATHS = [
  { path: "/LICENSE", type: /^text\/plain/, contains: "Apache License" },
  { path: "/NOTICE", type: /^text\/plain/, contains: "" },
  { path: "/robots.txt", type: /^text\/plain/, contains: "User-agent" },
  { path: "/schema.json", type: /^application\/json/, contains: "" },
  { path: "/routes.json", type: /^application\/json/, contains: "DocumentGet" },
] as const

test.describe("PeerDB Static Endpoint Flows", () => {
  for (const { path, type, contains } of STATIC_PATHS) {
    test(`Test ${path} is served`, async ({ context }) => {
      const page = await context.newPage()

      // The request is made from inside the page rather than by the test's own request context, so that it
      // carries the browser's view of the site: its certificate, its origin and its cookies.
      await goHome(page)
      const response = await fetchFromPage(page, path)

      expect(response.status, `${path} is served`).toBe(200)
      expect(response.headers["content-type"], `content type of ${path}`).toMatch(type)
      expect(response.length, `${path} carries content`).toBeGreaterThan(0)
      if (contains) {
        expect(response.body, `${path} carries what it is for`).toContain(contains)
      }

      console.log(`Successfully fetched ${path}, ${response.length} bytes of ${response.headers["content-type"]}.`)
    })
  }

  test("Test the routes description lists the routes the application uses", async ({ context }) => {
    const page = await context.newPage()

    await goHome(page)
    const response = await fetchFromPage(page, "/routes.json")
    expect(response.status, "the routes description is served").toBe(200)

    // The application fetches this before it renders anything, and builds its router from it, so every
    // route it navigates to has to be in it.
    const routes = JSON.parse(response.body) as Record<string, { path: string }>
    for (const name of ["Home", "SearchGet", "SearchShortcut", "DocumentGet", "DocumentCreate", "DocumentEdit", "DocumentDelete", "DocumentSessions", "AuthMockSignIn"]) {
      expect(routes[name], `the ${name} route is described`).toBeDefined()
      expect(routes[name].path, `the path of the ${name} route`).toMatch(/^\//)
    }

    console.log(`Successfully verified the routes description, which lists ${Object.keys(routes).length} routes.`)
  })

  test("Test the schema description is served", async ({ context }) => {
    const page = await context.newPage()

    await goHome(page)
    const response = await fetchFromPage(page, "/schema.json")
    expect(response.status, "the schema description is served").toBe(200)

    // It has to parse: it is the machine readable description of the site's own document schema.
    const schema = JSON.parse(response.body) as unknown
    expect(schema, "the schema description parses").toBeTruthy()

    console.log(`Successfully verified the schema description, ${response.length} bytes of it.`)
  })

  test("Test an address the site does not serve", async ({ context }) => {
    const page = await context.newPage()

    await goHome(page)

    // An address which is no route of the site is refused by the server rather than handed to the
    // application, which would otherwise render an empty view for it.
    const response = await fetchFromPage(page, "/nosuchpage")
    expect(response.status, "an address which is no route of the site").toBe(404)

    console.log("Successfully verified that an address the site does not serve is answered with not found.")
  })

  test("Test a document identifier which cannot exist", async ({ context }) => {
    const page = await context.newPage()

    await goHome(page)

    // A document address whose identifier is not one the site could ever have issued is refused as a bad
    // request, before anything is looked up.
    const response = await fetchFromPage(page, `${PEERDB_URL}/d/nonexistent1234567890`)
    expect(response.status, "a document address carrying an impossible identifier").toBe(400)

    console.log("Successfully verified that a document address carrying an impossible identifier is refused.")
  })
})
