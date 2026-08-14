import type { Page } from "@playwright/test"

import type { Role } from "../peerdb_utils"

import { documentIdOf, PROPERTY_IDS, RESTRICTED_CLASS, startCreate } from "../peerdb_utils"
import {
  checkpoint,
  checkpointElement,
  clearRefusedRequestErrors,
  discardEdit,
  documentValues,
  expect,
  expectNothingLoading,
  fetchFromPage,
  fillSlot,
  goHome,
  hideDuplicates,
  LOADING_TIMEOUT,
  mockUsername,
  openDocument,
  openDocumentTab,
  PEERDB_URL,
  saveEdit,
  settle,
  signIn,
  slotInput,
  startEdit,
  test,
  volatile,
} from "../utils"

// The class every test here creates its own document in. A galaxy is the smallest class which carries two
// fields worth changing, a name and a catalogue code, so one version of it can be told apart from the next
// by what the document view shows, with neither value ambiguous.
const GALAXY = "GALAXY"

// The role the documents are written by. It may start a galaxy (roles in config.yml) and it may update
// every document of the site, and creating a document grants its creator every action on it, so the same
// role can write the document again and take it away once the test is done with it.
const WRITING_ROLE: Role = "surveyor"

// The two sets of values every test writes. No value of one set is a substring of a value of the other, so
// a page which shows one set provably does not show the other. Every name begins with the same invented
// prefix, which no document of the test data and no other test file carries.
const ORIGINAL_NAME = "PDEVERSION Galaxy Original"
const ORIGINAL_CODE = "PDEVERSION-ORIGINAL"
const CURRENT_NAME = "PDEVERSION Galaxy Current"
const CURRENT_CODE = "PDEVERSION-CURRENT"

// The two sets the test which writes a document three times uses on its last write, so that the entry the
// third save adds is about values neither of the other two wrote.
const THIRD_NAME = "PDEVERSION Galaxy Third"
const THIRD_CODE = "PDEVERSION-THIRD"

// The versions a document created and then written once more has, newest first, which is the order the
// history lists them in. Creating a document is two changesets and not one: opening a create session
// inserts the document empty, without even a class, and saving the create form then writes the class and
// the values into it. The edit which follows is the third.
const NEWEST = 0
const ORIGINAL = 1
const OLDEST = 2
const VERSIONS = 3

// How many slots a field of the create and edit form shows once its first slot is filled. The name of a
// galaxy may be stated more than once, so filling its only slot grows a fresh empty one below it, while
// the catalogue code may be stated once and keeps the single slot it has.
const NAME_SLOTS = 2
const CODE_SLOTS = 1

// The interview of the test data whose history is asked for by callers who may not read it. The site keeps
// the class it belongs to out of the public read scope, and this document names one user of its own, so
// what its history answers differs per caller without any test having to write a permission claim.
const INTERVIEW = await documentIdOf(RESTRICTED_CLASS, "INTA_ASELUNE_FOUR_TABLES")

// The role which is granted reading and reading history on the restricted class, and the roles which are
// granted neither, so the history of the same document is asked for by a caller who holds it and by
// callers who do not.
const HISTORIC_ROLE: Role = "ethics"
const UNGRANTED_ROLES: ReadonlyArray<Role> = ["surveyor", "curator"]

// What the server answers a caller who asks for something they hold no action for.
const FORBIDDEN = 403

// The version a history entry links to, read out of the query of its href. A version is minted when the
// document is written, so a test which writes the document cannot know it in advance.
function versionOf(href: string | null, what: string): string {
  expect(href, `href of ${what}`).not.toBeNull()
  const version = new URL(href!, PEERDB_URL).searchParams.get("version")
  expect(version, `version in the href of ${what}`).not.toBeNull()
  return version!
}

// Creates a galaxy holding the original values and returns its identifier. Every test creates the document
// it then writes a second time, so no document another test looks at is ever changed.
//
// Nothing is checkpointed here: while a document is being created the view also renders the panel of its
// potential duplicates, which from the second run on lists the documents the earlier runs created, so a
// screenshot taken during the creation would differ between runs.
async function createGalaxy(page: Page): Promise<string> {
  await startCreate(page, GALAXY)
  await hideDuplicates(page)
  await fillSlot(page, PROPERTY_IDS.NAME, 0, ".pd-inputstring", ORIGINAL_NAME, NAME_SLOTS, "name of the new document")
  await fillSlot(page, PROPERTY_IDS.CATALOGUE_CODE, 0, ".pd-inputidentifier", ORIGINAL_CODE, CODE_SLOTS, "catalogue code of the new document")
  const id = await saveEdit(page)

  await expect(page.locator("#documentget-title"), "title of the created document").toHaveText(ORIGINAL_NAME)
  await expect(documentValues(page), "values of the created document").toHaveText([ORIGINAL_NAME, ORIGINAL_CODE])

  return id
}

// Writes both values of the document and leaves the browser on the document view showing them. Both values
// change on every write, so a version of the document can be told from the one before it by either of them,
// and the document view is asserted to show what was written before the history is read.
async function writeValues(page: Page, name: string, code: string, what: string): Promise<void> {
  await startEdit(page)
  await fillSlot(page, PROPERTY_IDS.NAME, 0, ".pd-inputstring", name, NAME_SLOTS, `name of ${what}`)
  await fillSlot(page, PROPERTY_IDS.CATALOGUE_CODE, 0, ".pd-inputidentifier", code, CODE_SLOTS, `catalogue code of ${what}`)
  await saveEdit(page)

  await expect(page.locator("#documentget-title"), `title of ${what}`).toHaveText(name)
  await expect(documentValues(page), `values of ${what}`).toHaveText([name, code])
}

// Opens the history tab of the given document and waits until its entries have loaded.
async function openHistory(page: Page, id: string, entries: number): Promise<void> {
  await openDocument(page, id)
  await openDocumentTab(page, "history")
  await settle(page)
  await expect(page.locator(".pd-documenthistory-loading"), "history is done loading").toHaveCount(0)
  await expect(page.locator(".pd-documenthistory-error"), "history loaded without an error").toHaveCount(0)
  await expect(page.locator(".pd-documenthistory-list"), "history table").toBeVisible()
  await expect(page.locator(".pd-documenthistory-item"), `history entries of a document which was written ${entries} times`).toHaveCount(entries)
}

// Follows the version link of one history entry and waits for the document view it leads to. The oldest
// version of a document created this way holds no claim at all, so it has neither a display label to put in
// the title nor a class to build a class tab from. The view is therefore waited for by its tab list, which
// every version has, rather than by settleDocument, which waits for the title.
async function followVersion(page: Page, entry: number, version: string, what: string): Promise<void> {
  await page.locator(".pd-documenthistory-link-version").nth(entry).click()
  await expect(page.locator(".pd-documentget-tabs"), `tabs of ${what}`).toBeVisible()
  await settle(page)

  // The version has to reach both the URL, which is what makes the link shareable, and the request the view
  // makes for the document, which is what makes it show something other than the current document.
  expect(page.url(), `the URL of ${what} carries the version it links to`).toContain(`version=${version}`)
  await expect(page.locator(".pd-documentget"), `${what} was fetched at the version it links to`).toHaveAttribute("data-url", new RegExp(`version=${version}`))
}

// Deletes the document a test created, through the confirmation page the interface offers, so that a second
// run of the suite starts from the same data set as the first. Creating a document grants its creator every
// action on it, so the role which wrote the document is the one which takes it away.
async function deleteDocument(page: Page, id: string): Promise<void> {
  const opened = await page.goto(`${PEERDB_URL}/d/delete/${id}`)
  expect(opened?.status(), "the delete page of the document to clean up").toBe(200)
  await expect(page.locator("#documentdelete-button-delete"), "delete button of the confirmation").toBeVisible({ timeout: LOADING_TIMEOUT })
  await page.locator("#documentdelete-button-delete").click()
  await expect(page.locator(".pd-home"), "home page after deleting").toBeVisible({ timeout: LOADING_TIMEOUT })
}

// The history of a document as the server reports it, asked for from inside the page so that the request
// carries the session of the view next to it, together with the status it was answered with.
async function historyFromServer(page: Page, id: string): Promise<{ status: number; entries: Array<{ changeset: string; version: string; at: string }> }> {
  const response = await fetchFromPage(page, `/api/d/history/${id}`)
  return { status: response.status, entries: response.status === 200 ? (JSON.parse(response.body) as Array<{ changeset: string; version: string; at: string }>) : [] }
}

test.describe("PeerDB Document Version Flows", () => {
  test("Test that every save adds an entry to the history naming who made it", async ({ context }) => {
    // The document is written three times and its history is read back after each of them, which is more
    // than a test of the default length has time for.
    test.slow()

    const page = await context.newPage()

    await signIn(page, [WRITING_ROLE])
    const id = await createGalaxy(page)

    // Creating a document is two changesets: the session inserts the document empty and saving the form
    // writes the class and the values into it. Both are recorded, so the history of a document which was
    // only ever created already has two entries.
    await openHistory(page, id, 2)
    await checkpointElement(page, page.locator(".pd-documenthistory-list"), "editversions-history-created", volatile(page))

    // Every further save adds one entry and takes none away, so the history grows by exactly one per write
    // whatever the write changed.
    await openDocument(page, id)
    await writeValues(page, CURRENT_NAME, CURRENT_CODE, "the second version of the document")
    await openHistory(page, id, VERSIONS)
    await openDocument(page, id)
    await writeValues(page, THIRD_NAME, THIRD_CODE, "the third version of the document")
    await openHistory(page, id, VERSIONS + 1)

    // Every entry says when the document was written and by whom, and the who is the user who wrote it: the
    // same signed-in user made all four, so every entry names them rather than the word for a changeset
    // which nobody signed in made.
    const entries = page.locator(".pd-documenthistory-item")
    const author = mockUsername([WRITING_ROLE])
    for (let entry = 0; entry < VERSIONS + 1; entry++) {
      await expect(entries.nth(entry).locator(".pd-documenthistory-text-time"), `the time of history entry ${entry}`).not.toHaveText(/^\s*$/)
      await expect(entries.nth(entry).locator(".pd-documenthistory-text-author"), `the author of history entry ${entry}`).toHaveText(author)
    }
    await expect(page.locator(".pd-documenthistory-empty"), "the history of a document which was written is not empty").toHaveCount(0)
    await checkpoint(page, "editversions-history-tab", { mask: volatile(page) })
    await checkpointElement(page, page.locator(".pd-documenthistory-list"), "editversions-history-written", volatile(page))

    // The entries the tab lists are the changesets the server reports, each linking to the version that
    // changeset produced, and no two of them are the same version.
    const reported = await historyFromServer(page, id)
    expect(reported.status, "the status the server answers the history of the document with").toBe(200)
    expect(reported.entries.length, "history entries the server reports").toBe(VERSIONS + 1)
    const links = page.locator(".pd-documenthistory-link-version")
    await expect(links, "version links of the history entries").toHaveCount(VERSIONS + 1)
    for (const [entry, item] of reported.entries.entries()) {
      await expect(links.nth(entry), `version link of history entry ${entry}`).toHaveAttribute("href", `/d/${id}?version=${item.version}`)
    }
    expect(new Set(reported.entries.map((item) => item.version)).size, "the history entries link to as many versions as there are entries").toBe(VERSIONS + 1)

    await deleteDocument(page, id)

    console.log(`Successfully verified that 3 saves left ${VERSIONS + 1} history entries on the created document, each naming ${author} as its author.`)
  })

  test("Test following the version links of a document which has three versions", async ({ context }) => {
    // The document is written twice and then read back at every one of its three versions, with a
    // screenshot of each, which is more than a test of the default length has time for.
    test.slow()

    const page = await context.newPage()

    await signIn(page, [WRITING_ROLE])

    const id = await createGalaxy(page)
    await writeValues(page, CURRENT_NAME, CURRENT_CODE, "the newest version of the document")

    // Every write is recorded, so the history lists a changeset per write and offers a link to the version
    // each of them produced. The backend returns them newest first.
    await openHistory(page, id, VERSIONS)
    const links = page.locator(".pd-documenthistory-link-version")
    await expect(links, "version links of the history entries").toHaveCount(VERSIONS)
    await checkpointElement(page, page.locator(".pd-documentget-panel-history"), "editversions-links-panel", volatile(page))

    const versions = await Promise.all([NEWEST, ORIGINAL, OLDEST].map(async (entry) => versionOf(await links.nth(entry).getAttribute("href"), `history entry ${entry}`)))
    expect(new Set(versions).size, "the three history entries link to three different versions").toBe(VERSIONS)

    // Following the link of the entry which recorded the original values has to show the document as it was
    // then. This is the assertion the whole test exists for: a view which showed the current document while
    // the URL claimed an older version would be indistinguishable from a working version link unless both
    // the values of the older version and the absence of the current ones are checked.
    await followVersion(page, ORIGINAL, versions[ORIGINAL], "the version the document was created with")
    await expect(page.locator("#documentget-title"), "title of the version the document was created with").toHaveText(ORIGINAL_NAME)
    await expect(documentValues(page), "values of the version the document was created with").toHaveText([ORIGINAL_NAME, ORIGINAL_CODE])
    const card = page.locator(".pd-documentget-card")
    await expect(card, "the older version does not show the name of the current one").not.toContainText(CURRENT_NAME)
    await expect(card, "the older version does not show the code of the current one").not.toContainText(CURRENT_CODE)
    await checkpoint(page, "editversions-original", { mask: volatile(page) })

    // Following the link of the newest entry has to show what the document is now, so that the links are
    // shown to lead to different documents and not merely to different URLs.
    await openHistory(page, id, VERSIONS)
    await followVersion(page, NEWEST, versions[NEWEST], "the newest version")
    await expect(page.locator("#documentget-title"), "title of the newest version").toHaveText(CURRENT_NAME)
    await expect(documentValues(page), "values of the newest version").toHaveText([CURRENT_NAME, CURRENT_CODE])
    await expect(card, "the newest version does not show the name of the older one").not.toContainText(ORIGINAL_NAME)
    await expect(card, "the newest version does not show the code of the older one").not.toContainText(ORIGINAL_CODE)
    await checkpoint(page, "editversions-newest", { mask: volatile(page) })

    // The oldest version is the document as opening the create session inserted it, holding no claim at
    // all, so it has nothing to show: no class, and therefore no class tab, an empty table of all its
    // properties, and no display label to title itself with. A view which fell back to the current document
    // would show a class tab, two values and a title here.
    await openHistory(page, id, VERSIONS)
    await followVersion(page, OLDEST, versions[OLDEST], "the oldest version")
    await expect(page.locator(".pd-documentget-panel-properties"), "class tab of the oldest version").toHaveCount(0)
    // The all properties tab is the one the view opens on, and it is there but empty: a table of properties
    // with no property to put in it is not rendered at all, so the panel holding it has nothing to give it a
    // size and is attached rather than visible.
    await expect(page.locator(".pd-documentget-panel-allproperties"), "all properties tab of the oldest version").toBeAttached()
    await expect(page.locator(".pd-propertiesview"), "table of the properties of the oldest version").toHaveCount(0)
    await expect(page.locator(".pd-propertiesview-row"), "claims of the oldest version").toHaveCount(0)
    await expect(page.locator("#documentget-title"), "title of the oldest version").toBeHidden()
    await expect(page.locator(".pd-displaylabel-empty"), "the oldest version has no display label").toHaveCount(1)
    await expect(card, "the oldest version does not show the name of the current version").not.toContainText(CURRENT_NAME)
    await expect(card, "the oldest version does not show the name it was created with").not.toContainText(ORIGINAL_NAME)
    await checkpoint(page, "editversions-oldest", { mask: volatile(page) })

    await deleteDocument(page, id)

    console.log(`Successfully followed all ${VERSIONS} version links of the created document and read back the document each of them records.`)
  })

  test("Test what the document view shows while an older version is open", async ({ context }) => {
    test.slow()

    const page = await context.newPage()

    await signIn(page, [WRITING_ROLE])

    const id = await createGalaxy(page)
    await writeValues(page, CURRENT_NAME, CURRENT_CODE, "the newest version of the document")

    await openHistory(page, id, VERSIONS)
    const links = page.locator(".pd-documenthistory-link-version")
    const version = versionOf(await links.nth(ORIGINAL).getAttribute("href"), "the history entry of the original values")
    await followVersion(page, ORIGINAL, version, "the version the document was created with")
    await expect(documentValues(page), "values of the version the document was created with").toHaveText([ORIGINAL_NAME, ORIGINAL_CODE])

    // The view says nothing at all about the document being shown at an older version: the version reaches
    // the URL and the fetch URL the view records in data-url, and nothing the card renders mentions it. The
    // class tab is selected and the tabs are the same ones the current version offers.
    await expect(page.locator(".pd-documentget-card [class*='version']"), "elements of the card which are about the version being shown").toHaveCount(0)
    await expect(page.locator(".pd-documentget-tab-properties"), "class tab").toBeVisible()
    await expect(page.locator(".pd-documentget-tab-allproperties"), "all properties tab").toBeVisible()
    await expect(page.locator(".pd-documentget-tab-history"), "history tab").toBeVisible()

    // The sidebar offers the same actions as on the current version, editing and deleting included, even
    // though neither of them acts on what is being shown.
    await expect(page.locator("#documentget-sidebar"), "sidebar of the older version").toBeVisible()
    await expect(page.locator("#documentget-button-edit"), "edit button offered while an older version is shown").toBeVisible()
    await expect(page.locator("#documentget-button-delete"), "delete button offered while an older version is shown").toBeVisible()
    await checkpoint(page, "editversions-older-actions", { mask: volatile(page) })

    // The history tab lists the whole history of the document whichever version is being shown, because it
    // is given the document and not the version.
    await openDocumentTab(page, "history")
    await settle(page)
    await expect(page.locator(".pd-documenthistory-item"), "history entries listed while an older version is shown").toHaveCount(VERSIONS)
    await checkpoint(page, "editversions-older-history", { mask: volatile(page) })

    // Editing from an older version edits the document as it is now and not the version on screen, so the
    // form comes up holding the current values.
    await openDocumentTab(page, "properties")
    await startEdit(page)
    await expect(slotInput(page, PROPERTY_IDS.NAME, 0, ".pd-inputstring"), "name the edit form starts from").toHaveValue(CURRENT_NAME)
    await expect(slotInput(page, PROPERTY_IDS.CATALOGUE_CODE, 0, ".pd-inputidentifier"), "code the edit form starts from").toHaveValue(CURRENT_CODE)

    await discardEdit(page)
    await expect(documentValues(page), "values after the edit started from an older version was discarded").toHaveText([CURRENT_NAME, CURRENT_CODE])

    await deleteDocument(page, id)

    console.log(`Successfully checked what the document view shows while 1 of the ${VERSIONS} versions of the created document, the one it was created with, is open.`)
  })

  test("Test that reading the history of a document is refused where the grant is missing", async ({ context }) => {
    const page = await context.newPage()

    // The history of a document is served to whoever may read the document, which for a document of the
    // class the site keeps out of the public read scope is nobody but the roles granted that class and the
    // users the document itself names. A caller who may not read the document is refused its history rather
    // than answered with an empty one, so a refusal cannot be read as a document which was never written.
    await goHome(page)
    const visitor = await historyFromServer(page, INTERVIEW)
    expect(visitor.status, "the history of a restricted document asked for by a visitor who is not signed in").toBe(FORBIDDEN)
    clearRefusedRequestErrors(page)

    for (const role of UNGRANTED_ROLES) {
      await signIn(page, [role])
      const refused = await historyFromServer(page, INTERVIEW)
      expect(refused.status, `the history of a restricted document asked for by the ${role} role`).toBe(FORBIDDEN)
      clearRefusedRequestErrors(page)
      await goHome(page)
      await page.locator(".pd-navbarmenu-button").click()
      await page.locator("#navbar-button-signout").click()
      await expect(page.locator("#navbar-button-signin"), `the ${role} role is signed out again`).toBeVisible()
    }

    // The role which is granted reading the class is answered with the history, so the refusals above are
    // about the caller and not about the document.
    await signIn(page, [HISTORIC_ROLE])
    const granted = await historyFromServer(page, INTERVIEW)
    expect(granted.status, `the history of a restricted document asked for by the ${HISTORIC_ROLE} role`).toBe(200)
    expect(granted.entries.length, "history entries of the restricted document").toBeGreaterThanOrEqual(1)

    // The tab renders what the server answered: as many entries as it reported, and no error in their
    // place.
    await openHistory(page, INTERVIEW, granted.entries.length)
    await expectNothingLoading(page)
    await checkpointElement(page, page.locator(".pd-documenthistory-list"), "editversions-restricted-history", volatile(page))

    console.log(
      `Successfully verified that the history of a restricted document is refused to ${UNGRANTED_ROLES.length + 1} callers which are granted none of it and lists ` +
        `${granted.entries.length} entries for the role which is granted it.`,
    )
  })
})
