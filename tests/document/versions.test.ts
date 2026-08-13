import type { Page } from "@playwright/test"

import { documentIdOf } from "../peerdb_utils"
import {
  checkpoint,
  checkpointElement,
  clearRefusedRequestErrors,
  expect,
  expectDocument,
  fetchFromPage,
  openDocument,
  openDocumentTab,
  PEERDB_URL,
  settle,
  test,
  volatile,
} from "../utils"

// The document whose history and past versions are looked at, addressed by its document identifier so that the
// same document is opened on every run. It is the planet of the test data which carries the most fields, so
// what a version of it renders is worth comparing against what the current one renders.
const PLANET_ID = await documentIdOf("PLANET", "G1_SURVEY_GRID_44_B")

// What one entry of the history the server reports carries.
interface HistoryEntry {
  changeset: string
  version: string
  at: string
}

// The history of a document as the server reports it, asked for from inside the page so that the request
// carries the same session as the view next to it.
async function historyFromServer(page: Page, id: string): Promise<Array<HistoryEntry>> {
  const response = await fetchFromPage(page, `/api/d/history/${id}`)
  expect(response.status, "the status the server answers the history of the document with").toBe(200)
  return JSON.parse(response.body) as Array<HistoryEntry>
}

// The address the document view read the document from, which is where the view records which version it is
// showing: the same address as for the current version, with the version added to it.
async function documentURL(page: Page): Promise<string> {
  const url = await page.locator(".pd-documentget").getAttribute("data-url")
  expect(url, "the address the document view read the document from").not.toBeNull()
  return url!
}

// Opens the history tab of a document and waits for the entries to be there.
async function openHistory(page: Page, id: string): Promise<void> {
  await openDocument(page, id)
  await settle(page)
  await openDocumentTab(page, "history")
  await settle(page)
  await expect(page.locator(".pd-documenthistory-loading"), "history is done loading").toHaveCount(0)
  await expect(page.locator(".pd-documenthistory-error"), "history loaded without an error").toHaveCount(0)
  await expect(page.locator(".pd-documenthistory-list"), "history table").toBeVisible()
}

test.describe("PeerDB Document Versions Flows", () => {
  test("Test the history tab lists the changesets the document went through", async ({ context }) => {
    const page = await context.newPage()

    await openHistory(page, PLANET_ID)

    // A document which is in the store has been written at least once, so its history is never empty, and the
    // entries the tab lists are the changesets the server reports for it.
    const entries = page.locator(".pd-documenthistory-item")
    const count = await entries.count()
    expect(count, "history entries").toBeGreaterThanOrEqual(1)
    await expect(page.locator(".pd-documenthistory-empty"), "history is not empty").toHaveCount(0)
    const reported = await historyFromServer(page, PLANET_ID)
    expect(reported.length, "history entries the server reports").toBe(count)

    // Every entry says when the document was written and by whom. Nobody was signed in when the test data was
    // populated, so the author is the word for that rather than a name, which is still an answer and not a
    // blank.
    for (let i = 0; i < count; i++) {
      await expect(entries.nth(i).locator(".pd-documenthistory-text-time"), `the time of history entry ${i}`).not.toHaveText(/^\s*$/)
      await expect(entries.nth(i).locator(".pd-documenthistory-text-author"), `the author of history entry ${i}`).not.toHaveText(/^\s*$/)
    }

    // Every entry links to the version it recorded, which is how a past version of the document is reached, and
    // the version it links to is the one the server reports for that changeset.
    const links = page.locator(".pd-documenthistory-link-version")
    await expect(links, "version links of the history entries").toHaveCount(count)
    for (const [i, entry] of reported.entries()) {
      expect(entry.version, `the version the server reports for history entry ${i}`).toMatch(/^[0-9A-Za-z]+-\d+$/)
      expect(entry.version.split("-")[0], `the version of history entry ${i} names the changeset which recorded it`).toBe(entry.changeset)
      expect(Number.isNaN(Date.parse(entry.at)), `the time the server reports for history entry ${i} is a timestamp`).toBe(false)
      await expect(links.nth(i), `version link of history entry ${i}`).toHaveAttribute("href", `/d/${PLANET_ID}?version=${entry.version}`)
    }

    await checkpoint(page, "versions-planet-history", { mask: volatile(page) })
    await checkpointElement(page, page.locator(".pd-documenthistory-list"), "versions-planet-history-list", volatile(page))

    console.log(`Successfully verified that the history tab lists the ${count} changesets the server reports for the document, each linking to the version it recorded.`)
  })

  test("Test following a version link shows the document as it was at that version", async ({ context }) => {
    const page = await context.newPage()

    // What the document shows now is read first, so that what it shows at the version it was last written at
    // can be compared against it. The test data was populated in one changeset per document, so the version the
    // history entry links to is that same document, and what the comparison proves is that the view asked for
    // the version and rendered what came back rather than falling back to the current document or to nothing.
    await openDocument(page, PLANET_ID)
    await settle(page)
    const currentURL = await documentURL(page)
    expect(currentURL, "the address of the current document carries no version").not.toContain("version=")
    const title = (await page.locator("#documentget-title").textContent())!.trim()
    await openDocumentTab(page, "allproperties")
    await settle(page)
    const rows = await page.locator(".pd-documentget-panel-allproperties .pd-propertiesview-row").count()
    expect(rows, "property rows of the current document").toBeGreaterThan(0)

    await openHistory(page, PLANET_ID)
    const link = page.locator(".pd-documenthistory-link-version").first()
    const href = (await link.getAttribute("href"))!
    const version = new URL(href, PEERDB_URL).searchParams.get("version")
    expect(version, "the version the first history entry links to").not.toBeNull()

    await link.click()
    await expectDocument(page)
    await settle(page)

    // The view says which version it is showing by reading the document at that version: the address it read it
    // from is the one of the document with the version added to it.
    expect(page.url(), "the address of the document view after following a version link").toContain(`version=${version}`)
    expect(await documentURL(page), "the address the document view read the version from").toBe(`${currentURL}?version=${version}`)
    await expect(page.locator("#documentget-title"), "the title of the document at the version").toHaveText(title)
    await checkpoint(page, "versions-planet-at-version", { mask: volatile(page) })

    // The document at the version renders the same way the current one does, tabs and all, because it is a
    // document like any other and not a diff of one.
    await openDocumentTab(page, "allproperties")
    await settle(page)
    await expect(page.locator(".pd-documentget-panel-allproperties .pd-propertiesview-row"), "property rows of the document at the version").toHaveCount(rows)

    // Switching a tab keeps the version, so a reader who came from the history stays on the version they came
    // for instead of being dropped back onto the current document.
    expect(page.url(), "the address after switching a tab of the document at the version").toContain(`version=${version}`)
    expect(await documentURL(page), "the address the document view read the version from after switching a tab").toBe(`${currentURL}?version=${version}`)
    await checkpointElement(page, page.locator(".pd-documentget-panel-allproperties"), "versions-planet-at-version-allproperties")

    // The history is the history of the document and not of the version, so it is the same list whichever
    // version is being read.
    await openDocumentTab(page, "history")
    await settle(page)
    await expect(page.locator(".pd-documenthistory-link-version"), "version links listed while a version is being read").toHaveCount(
      (await historyFromServer(page, PLANET_ID)).length,
    )

    console.log(`Successfully followed a version link to version ${version} of a document and got the same ${rows} property rows the current version shows.`)
  })

  test("Test a version which does not exist and one which is not a version at all", async ({ context }) => {
    const page = await context.newPage()

    await openDocument(page, PLANET_ID)
    await settle(page)
    const reported = await historyFromServer(page, PLANET_ID)
    expect(reported.length, "history entries the server reports").toBeGreaterThanOrEqual(1)
    const version = reported[0].version

    // Asking for the version which is there is answered with the document at that version, and the answer says
    // which version it is, so a caller reading a version never has to trust that it got the one it asked for.
    const found = await fetchFromPage(page, `/api/d/${PLANET_ID}?version=${version}`)
    expect(found.status, "the status of asking for the version which is there").toBe(200)
    expect(found.headers["version"], "the version the server says it answered with").toBe(version)

    // A version which is well formed but which this document never had is not there, which is answered as a
    // missing document rather than as the current one. The identifier of another document stands in for a
    // changeset which never recorded this one.
    const unknown = await fetchFromPage(page, `/d/${PLANET_ID}?version=${PLANET_ID}-1`)
    expect(unknown.status, "the status of asking for a version the document never had").toBe(404)

    // A version which is not a version at all is refused as a bad request, so a mistyped address is told apart
    // from one which asks for something which is simply gone.
    const malformed = await fetchFromPage(page, `/d/${PLANET_ID}?version=notaversion`)
    expect(malformed.status, "the status of asking for something which is not a version").toBe(400)

    // Both refusals are logged by the browser as failed requests, which is what asking for them was for, so
    // they are cleared before the page is compared against its screenshot.
    clearRefusedRequestErrors(page)
    await checkpoint(page, "versions-planet-after-refused-versions", { mask: volatile(page) })

    console.log(
      `Successfully verified that version ${version} is served with the version it is, while an unknown version is answered with 404 and a malformed one with 400.`,
    )
  })
})
