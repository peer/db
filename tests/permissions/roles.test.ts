import type { Locator, Page } from "@playwright/test"

import type { Role } from "../peerdb_utils"

import { become, CLASS_IDS, documentIdOf, PROPERTY_IDS, RESTRICTED_CLASS, ROLES } from "../peerdb_utils"
import {
  checkpoint,
  checkpointElement,
  clearRefusedRequestErrors,
  expect,
  expectNoResults,
  fetchFromPage,
  filter,
  filterValue,
  goHome,
  LOADING_TIMEOUT,
  openDocument,
  openFilters,
  PEERDB_URL,
  resultCount,
  searchWithQuery,
  settleFilters,
  settleSearch,
  signIn,
  test,
} from "../utils"

// How many documents the instance holds altogether, and how many of them a caller reads who is granted
// every class the site declares public. The site grants reading per class through the reserved role which
// every request holds, signed in or not, and leaves exactly one class out of that list, so the difference
// between the two numbers is the interviews of the test data (see roles in config.yml).
const ALL_DOCUMENTS = 1461
const PUBLIC_DOCUMENTS = 1421

// Every interview of the test data, which is what a role granted the restricted class outright reaches.
const ALL_INTERVIEWS = ALL_DOCUMENTS - PUBLIC_DOCUMENTS

// The ethics protocol the test data files the widest spread of interviews under. Two expeditions record it
// as well, and those are public, so what a caller is told the protocol is referenced by is the two plus
// however many of the nine interviews under it that caller may read. It is where a count rendered in the
// interface follows the permissions rather than the data alone.
const ETHICS_PROTOCOL = await documentIdOf("ETHICS_PROTOCOL", "PROTOCOL_4")
const PUBLIC_PROTOCOL_REFERENCES = 2

// What the API answers a caller who may not have what they asked for.
const FORBIDDEN = 403

// One of the identities the site can be visited as, with everything about it which follows from the
// permission model. The roles are the ones to sign in with through the mock authenticator, or null for a
// visitor who does not sign in at all, which is the floor every other identity builds on.
interface Identity {
  // What names the identity in a test title and in what a test reports.
  what: string
  // The suffix which tells this identity's screenshots apart from the other identities'.
  slug: string
  // The roles to sign in with, or null for not signing in.
  roles: ReadonlyArray<Role> | null
  // How many interviews the identity reaches: none, unless a role opens the class or the interviews name
  // the identity's own subject in their permission claims (see section 5.4 of the test data).
  interviews: number
  // How many documents the identity is told reference the ethics protocol.
  protocolReferences: number
  // Whether the identity may enumerate the store, which the bulk read action alone allows.
  enumerates: boolean
}

// Every identity the site distinguishes: no sign-in, a sign-in carrying no role, and one per role the site
// declares. The mock authenticator makes a user out of the roles it is given, so the subject of a sign-in
// with a single role is the one the test data's permission claims name, which is why signing in as one
// role reaches interviews no role grant of the site opens.
const IDENTITIES: ReadonlyArray<Identity> = [
  // A visitor holds the reserved role alone, so they read the public classes and nothing else.
  { what: "a visitor who is not signed in", slug: "visitor", roles: null, interviews: 0, protocolReferences: PUBLIC_PROTOCOL_REFERENCES, enumerates: false },
  // Signing in without a role grants nothing on its own, but it makes the caller mock-user@localhost,
  // which ten interviews name, so it reaches documents a visitor cannot.
  { what: "a user signed in with no role", slug: "norole", roles: [], interviews: 10, protocolReferences: 5, enumerates: false },
  // Bulk reading is about enumerating rather than about a class, so it widens nothing.
  { what: "the bulk role", slug: "bulk", roles: ["bulk"], interviews: 0, protocolReferences: PUBLIC_PROTOCOL_REFERENCES, enumerates: true },
  { what: "the surveyor role", slug: "surveyor", roles: ["surveyor"], interviews: 0, protocolReferences: PUBLIC_PROTOCOL_REFERENCES, enumerates: false },
  // Nineteen interviews name mock-user-researcher@localhost, which is the subject of a sign-in holding
  // the researcher role alone. The role itself is granted no reading of the class.
  { what: "the researcher role", slug: "researcher", roles: ["researcher"], interviews: 19, protocolReferences: 3, enumerates: false },
  { what: "the author role", slug: "author", roles: ["author"], interviews: 0, protocolReferences: PUBLIC_PROTOCOL_REFERENCES, enumerates: false },
  // Fifteen interviews name mock-user-curator@localhost, eleven of them alone and four together with the
  // user holding no role.
  { what: "the curator role", slug: "curator", roles: ["curator"], interviews: 15, protocolReferences: 7, enumerates: false },
  // The ethics committee is granted the class itself, so it reaches every interview whatever the
  // interviews say themselves.
  { what: "the ethics role", slug: "ethics", roles: ["ethics"], interviews: ALL_INTERVIEWS, protocolReferences: 11, enumerates: false },
  { what: "the admin role", slug: "admin", roles: ["admin"], interviews: ALL_INTERVIEWS, protocolReferences: 11, enumerates: true },
]

// Opens a search prefiltered to the restricted class and waits for it to settle, whether or not it found
// anything. The class is reached through the search shortcut route, which takes the property and the value
// as a query parameter, so the search is in the same scope as ticking the class in the class facet.
async function searchRestrictedClass(page: Page): Promise<void> {
  await page.goto(`${PEERDB_URL}/s?${PROPERTY_IDS.INSTANCE_OF}=${CLASS_IDS[RESTRICTED_CLASS]}`)
  await settleSearch(page)
}

// Adds every value one facet still keeps behind its row limit, so that a value which is not among the
// first rows is asserted on rather than reported as missing. The facet shows ten rows at a time and the
// button under it grows the list by ten more, so it is pressed until there is nothing left to add. The
// press is dispatched rather than clicked, because the facet replaces the button while it re-renders,
// which is also how the filters panel presses its own button (see settleFilters).
async function expandFilterFully(block: Locator): Promise<void> {
  const more = block.locator(".pd-filtersresult-more")
  await expect
    .poll(
      async () => {
        if (!(await more.isVisible().catch(() => false))) {
          return false
        }
        await more.dispatchEvent("click").catch(() => null)
        return true
      },
      { message: "the facet adds every value it has", timeout: 2 * LOADING_TIMEOUT },
    )
    .toBe(false)
}

test.describe("PeerDB Role Permission Flows", () => {
  test("Test the identities asserted on cover every role the site declares", async ({ context }) => {
    const page = await context.newPage()

    // The table this file asserts against is written out rather than derived, so a role added to the site
    // without a line of its own here would otherwise be tested by nothing. Every identity holding exactly
    // one role is compared with the roles the site declares, which is what the mock sign-in page offers.
    const covered = IDENTITIES.filter((identity) => identity.roles?.length === 1).map((identity) => identity.roles![0])
    expect([...covered].sort(), "the identities cover exactly the roles the site declares").toEqual([...ROLES].sort())

    // The two identities left are the ones holding no role at all, which the site tells apart by whether
    // the caller signed in.
    const withoutRole = IDENTITIES.filter((identity) => (identity.roles?.length ?? 0) === 0)
    expect(withoutRole.length, "a visitor and a user holding no role are covered as well").toBe(2)

    await goHome(page)
    console.log(`Successfully verified that the ${IDENTITIES.length} identities asserted on cover the ${ROLES.length} roles the site declares.`)
  })

  for (const identity of IDENTITIES) {
    test(`Test what ${identity.what} may read`, async ({ context }) => {
      const page = await context.newPage()

      await become(page, identity)

      // A search with an empty query is over everything the caller may read, so its count is the size of
      // the part of the store the identity reaches. Every identity is granted the public classes, so the
      // count is that floor plus the interviews the identity's own subject or role opens.
      await searchWithQuery(page, "")
      const total = await resultCount(page)
      expect(total, `what a search over everything finds for ${identity.what}`).toBe(PUBLIC_DOCUMENTS + identity.interviews)

      // The facet counts are computed over the same filtered query as the results, so the class facet
      // offers the restricted class exactly when the identity reaches interviews, and says how many.
      await openFilters(page)
      const classes = filter(page, "ref", PROPERTY_IDS.INSTANCE_OF)
      await expect(classes, "the class facet").toBeVisible({ timeout: LOADING_TIMEOUT })
      await expandFilterFully(classes)
      const restricted = filterValue(page, "ref", [PROPERTY_IDS.INSTANCE_OF], CLASS_IDS[RESTRICTED_CLASS])
      if (identity.interviews === 0) {
        await expect(restricted, `the class facet does not offer the restricted class to ${identity.what}`).toHaveCount(0)
      } else {
        await expect(restricted, `the class facet offers the restricted class to ${identity.what}`).toHaveCount(1)
        const count = page.locator(`.pd-reffiltertreerow-count[for="${["ref", PROPERTY_IDS.INSTANCE_OF, CLASS_IDS[RESTRICTED_CLASS]].join("/")}"]`)
        await expect(count, `what the class facet says the restricted class holds for ${identity.what}`).toHaveText(`(${identity.interviews})`)
      }

      // Searching the restricted class on its own reports the same number, which is what says the count
      // above is the identity's reach and not an artefact of how the facet is aggregated.
      await searchRestrictedClass(page)
      if (identity.interviews === 0) {
        await expectNoResults(page)
      }
      expect(await resultCount(page), `how many interviews ${identity.what} finds`).toBe(identity.interviews)

      // Listing the store is an action of its own, which no class scope can grant: a bulk read is not
      // about a particular document. Only the role which exists for it and the one holding everything are
      // given it, whatever else a role may read.
      const listing = await fetchFromPage(page, "/api/d")
      expect(listing.status, `the store listing for ${identity.what}`).toBe(identity.enumerates ? 200 : FORBIDDEN)
      if (identity.enumerates) {
        expect(JSON.parse(listing.body), `what the store listing yields for ${identity.what}`).not.toHaveLength(0)
      }
      clearRefusedRequestErrors(page)

      // The count next to "referenced by" is a search of its own, run over the documents which reference
      // the one being looked at, so it is filtered by what the caller may read just as any other search
      // is. The protocol is referenced by two public expeditions and by nine interviews, which is why the
      // number differs between identities which all read the protocol itself.
      await openDocument(page, ETHICS_PROTOCOL)
      const referencedBy = page.locator("#documentget-button-referencedby")
      await expect(referencedBy, "the referenced by button").toBeVisible({ timeout: LOADING_TIMEOUT })
      await expect(referencedBy, `what the protocol says it is referenced by for ${identity.what}`).toHaveText(new RegExp(`\\(${identity.protocolReferences}\\)`), {
        timeout: LOADING_TIMEOUT,
      })

      console.log(
        `Successfully verified that ${identity.what} reads ${total} documents, ${identity.interviews} of them interviews, out of the ${ALL_DOCUMENTS} the site holds, ` +
          `${identity.enumerates ? "may" : "may not"} enumerate the store, and is told the ethics protocol is referenced by ${identity.protocolReferences} documents.`,
      )
    })
  }

  test("Test the restricted class search of a visitor who is not signed in", async ({ context }) => {
    const page = await context.newPage()

    // A class which the caller may read nothing of is not an error: the search runs and finds nothing, and
    // the page says so instead of offering a result list which would be empty.
    await goHome(page)
    await searchRestrictedClass(page)
    await expectNoResults(page)
    await checkpoint(page, "permissions-roles-interviews-visitor")

    console.log("Successfully verified that a search of the restricted class finds nothing for a visitor who is not signed in.")
  })

  for (const slug of ["norole", "researcher", "ethics"]) {
    const identity = IDENTITIES.find((candidate) => candidate.slug === slug)!

    test(`Test the restricted class search of ${identity.what}`, async ({ context }) => {
      const page = await context.newPage()

      await become(page, identity)
      await searchRestrictedClass(page)
      expect(await resultCount(page), `how many interviews ${identity.what} finds`).toBe(identity.interviews)

      // The panel keeps adding facets while the page is captured, so it is asked for all of them first.
      await settleFilters(page)
      await checkpoint(page, `permissions-roles-interviews-${identity.slug}`)

      console.log(`Successfully verified the restricted class search of ${identity.what}, which finds ${identity.interviews} interviews.`)
    })
  }

  test("Test the class facet of a visitor next to the one of the ethics role", async ({ context }) => {
    const page = await context.newPage()

    // The two facets are of the same search over everything, so what they differ in is the one class the
    // site keeps out of the public read scope. Capturing the facet itself rather than the whole page keeps
    // the comparison on the list of classes.
    await goHome(page)
    await searchWithQuery(page, "")
    await openFilters(page)
    const classes = filter(page, "ref", PROPERTY_IDS.INSTANCE_OF)
    await expect(classes, "the class facet").toBeVisible({ timeout: LOADING_TIMEOUT })
    await expandFilterFully(classes)
    await checkpointElement(page, classes, "permissions-roles-class-facet-visitor")

    await signIn(page, ["ethics"])
    await searchWithQuery(page, "")
    await openFilters(page)
    const withInterviews = filter(page, "ref", PROPERTY_IDS.INSTANCE_OF)
    await expect(withInterviews, "the class facet of the ethics role").toBeVisible({ timeout: LOADING_TIMEOUT })
    await expandFilterFully(withInterviews)
    await checkpointElement(page, withInterviews, "permissions-roles-class-facet-ethics")

    console.log("Successfully verified that the class facet gains the restricted class for the role which is granted it.")
  })
})
