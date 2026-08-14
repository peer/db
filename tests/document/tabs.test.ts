import type { Page } from "@playwright/test"

import { documentIdOf, LANGUAGES } from "../peerdb_utils"
import {
  checkpoint,
  checkpointElement,
  clearRefusedRequestErrors,
  expect,
  expectDocument,
  goHome,
  openDocument,
  openDocumentTab,
  PEERDB_URL,
  settle,
  switchLanguage,
  test,
  volatile,
} from "../utils"

// The document whose tabs are looked at, addressed by its document identifier so that the same document is
// opened on every run. It is an observation which carries permission claims of its own, so all four tabs have
// something to show: the fields its class declares, every claim it holds, the users it grants access to, and
// the changeset it was written in.
const OBSERVATION_ID = await documentIdOf("OBSERVATION", "OBSA_CORD_READING_RECORDER")

// A document which grants nobody anything, used to look at the other state of the permissions tab. A planet is
// public to read through the site's roles alone, so it carries no permission claim.
const PLANET_ID = await documentIdOf("PLANET", "G1_SURVEY_GRID_44_B")

// Every tab a document of this site has. The class tab carries "-tab-properties" and the "all properties" tab
// carries "-tab-allproperties", which is the reverse of what their names suggest, so the list is written out
// once here and the tests address a tab through it.
const TABS = ["properties", "allproperties", "permissions", "history"] as const

type Tab = (typeof TABS)[number]

// Asserts that exactly one tab panel is shown, which is what the tabs are for: switching to a tab has to show
// its panel and hide the panel of the previous one. The panels which are not selected stay in the document, so
// they are asserted to be hidden rather than to be gone.
async function expectOnlyPanel(page: Page, tab: Tab): Promise<void> {
  for (const other of TABS) {
    const panel = page.locator(`.pd-documentget-panel-${other}`)
    if (other === tab) {
      await expect(panel, `panel of the ${other} tab`).toBeVisible()
    } else {
      await expect(panel, `panel of the ${other} tab is hidden`).toBeHidden()
    }
  }
}

// Asserts which tab the address of the document view names. The slug of the first tab is never written, so the
// class tab is the one address without the parameter, and the slug of the "all properties" tab is "properties"
// while the class tab's own slug would be the identifier of the class. Passing null asserts that no tab is
// named at all.
function expectTabInURL(page: Page, slug: string | null, what: string): void {
  const tab = new URL(page.url()).searchParams.get("tab")
  expect(tab, `the address of the document view after ${what}`).toBe(slug)
}

test.describe("PeerDB Document Tabs Flows", () => {
  for (const language of LANGUAGES) {
    test(`Test the four document tabs in ${language}`, async ({ context }) => {
      const page = await context.newPage()

      await goHome(page)
      await switchLanguage(page, language)

      await openDocument(page, OBSERVATION_ID)
      await settle(page)

      // All four tabs a PeerDB document has are offered: the class tab (the fields the class declares, which is
      // the tab the view opens on), "all properties", "permissions" and "history". This site registers no
      // document component and the document is not a page, so there is neither a registry tab nor a content one.
      const tabs = page.locator(".pd-documentget-tabs")
      await expect(tabs, "tab list").toBeVisible()
      for (const tab of TABS) {
        await expect(page.locator(`.pd-documentget-tab-${tab}`), `${tab} tab`).toBeVisible()
      }
      await expect(page.locator(".pd-documentget-tab-registry"), "registry tabs").toHaveCount(0)
      await expect(page.locator(".pd-documentget-tab-content"), "content tab of a page document").toHaveCount(0)
      await checkpointElement(page, tabs, `tabs-observation-list-${language}`)

      // The class tab, which the view opens on: the fields the class declares, in the order the class gives
      // them. It is the tab whose slug is never written to the address, so the address carries no tab at all.
      await expectOnlyPanel(page, "properties")
      expectTabInURL(page, null, "opening the document")
      const propertiesPanel = page.locator(".pd-documentget-panel-properties")
      await expect(propertiesPanel.locator(".pd-fieldsview").first(), "fields of the class tab").toBeVisible()
      const labels = propertiesPanel.locator(".pd-fieldsview-label")
      const fieldCount = await labels.count()
      expect(fieldCount, "property labels of the class tab").toBeGreaterThan(0)
      await checkpoint(page, `tabs-observation-tab-properties-${language}`, { mask: volatile(page) })
      await checkpointElement(page, propertiesPanel, `tabs-observation-panel-properties-${language}`)

      // The "all properties" tab: every claim of the document as a property and value table, without the
      // class's grouping and without the class's filtering, so it lists at least as many properties as the
      // class tab does fields.
      await openDocumentTab(page, "allproperties")
      await settle(page)
      await expectOnlyPanel(page, "allproperties")
      expectTabInURL(page, "properties", "switching to the all properties tab")
      const allPanel = page.locator(".pd-documentget-panel-allproperties")
      const allRows = allPanel.locator(".pd-propertiesview-row")
      const rowCount = await allRows.count()
      expect(rowCount, "rows of the all properties tab").toBeGreaterThanOrEqual(fieldCount)
      const allLabels = await allPanel.locator(".pd-propertiesview-label").allTextContents()
      expect(allLabels.length, "property labels of the all properties tab").toBeGreaterThan(0)
      for (const [i, text] of allLabels.entries()) {
        expect(text.trim(), `property label ${i} of the all properties tab`).not.toBe("")
      }
      // A label is rendered once per property and a value once per claim, so a property stated more than once is
      // one label with several values under it and there are never fewer values than labels.
      expect(await allPanel.locator(".pd-propertiesview-value").count(), "property values of the all properties tab").toBeGreaterThanOrEqual(allLabels.length)
      await checkpoint(page, `tabs-observation-tab-allproperties-${language}`, { mask: volatile(page) })
      await checkpointElement(page, allPanel, `tabs-observation-panel-allproperties-${language}`)

      // The permissions tab: the users this document grants access to and what each of them may do, read out of
      // the permission claims the document carries. This observation hands updating to one user and deleting to
      // another, so two users are listed, each with the action it was given.
      await openDocumentTab(page, "permissions")
      await settle(page)
      await expectOnlyPanel(page, "permissions")
      expectTabInURL(page, "permissions", "switching to the permissions tab")
      const permissionsPanel = page.locator(".pd-documentget-panel-permissions")
      await expect(permissionsPanel.locator(".pd-permissionsview"), "permissions view").toBeVisible()
      await expect(permissionsPanel.locator(".pd-permissionsview-title-users"), "title of the list of users with access").toBeVisible()
      await expect(permissionsPanel.locator(".pd-permissionsview-empty-users"), "the document grants somebody access").toHaveCount(0)
      const users = permissionsPanel.locator(".pd-permissionsview-item-user")
      await expect(users, "users the document grants access to").toHaveCount(2)
      await expect(permissionsPanel.locator(".pd-permissionsview-badge-action"), "actions the granted users hold").toHaveCount(2)
      await expect(users.first().locator(".pd-permissionsview-label-user"), "the first granted user is named").not.toHaveText(/^\s*$/)
      // Naming a user resolves the identity behind the subject, which a caller who is not signed in may not
      // read, so the view falls back to the bare subject and the refused lookups are cleared before the page is
      // compared against its screenshot.
      await expect(permissionsPanel.locator(".pd-identityinline").first(), "the identity of the first granted user").toBeVisible()
      clearRefusedRequestErrors(page)
      await checkpoint(page, `tabs-observation-tab-permissions-${language}`, { mask: volatile(page) })
      await checkpointElement(page, permissionsPanel, `tabs-observation-panel-permissions-${language}`)

      // The history tab: the changesets the document went through. A populated document has been written at
      // least once, so its history is never empty.
      await openDocumentTab(page, "history")
      await settle(page)
      await expectOnlyPanel(page, "history")
      expectTabInURL(page, "history", "switching to the history tab")
      const historyPanel = page.locator(".pd-documentget-panel-history")
      await expect(historyPanel.locator(".pd-documenthistory-loading"), "history is done loading").toHaveCount(0)
      await expect(historyPanel.locator(".pd-documenthistory-error"), "history loaded without an error").toHaveCount(0)
      await expect(historyPanel.locator(".pd-documenthistory-empty"), "history is not empty").toHaveCount(0)
      await expect(historyPanel.locator(".pd-documenthistory-list"), "history table").toBeVisible()
      const historyItems = historyPanel.locator(".pd-documenthistory-item")
      const historyCount = await historyItems.count()
      expect(historyCount, "history entries").toBeGreaterThanOrEqual(1)
      // Every entry links to the version it recorded, which is how a past version of the document is reached
      // from here.
      await expect(historyPanel.locator(".pd-documenthistory-link-version"), "version links of the history entries").toHaveCount(historyCount)
      await checkpoint(page, `tabs-observation-tab-history-${language}`, { mask: volatile(page) })
      await checkpointElement(page, historyPanel, `tabs-observation-panel-history-${language}`, { mask: volatile(page) })

      // Going back to the tab the view opened on has to bring the class fields back, so that the tabs can be
      // switched between in either direction, and has to drop the tab from the address again.
      await openDocumentTab(page, "properties")
      await settle(page)
      await expectOnlyPanel(page, "properties")
      expectTabInURL(page, null, "switching back to the class tab")
      await expect(propertiesPanel.locator(".pd-fieldsview").first(), "fields of the class tab after coming back").toBeVisible()
      await checkpoint(page, `tabs-observation-tab-properties-again-${language}`, { mask: volatile(page) })

      console.log(
        `Successfully switched through all four document tabs in ${language}, with ${fieldCount} class fields, ${rowCount} property rows and ${historyCount} history entries listed.`,
      )
    })
  }

  test("Test a tab is reached by its address and the back button returns to the previous one", async ({ context }) => {
    const page = await context.newPage()

    // A tab is named in the address by a slug of its own, so a link can open the document on any of them
    // rather than only on the one the view opens by itself.
    await page.goto(`${PEERDB_URL}/d/${OBSERVATION_ID}?tab=history`)
    await expectDocument(page)
    await settle(page)
    await expectOnlyPanel(page, "history")
    await expect(page.locator(".pd-documenthistory-list"), "history table of the tab opened by its address").toBeVisible()

    // Switching a tab pushes an entry into the browser's history, so going back returns to the tab which was
    // shown before rather than leaving the document altogether.
    await openDocumentTab(page, "allproperties")
    await settle(page)
    await expectOnlyPanel(page, "allproperties")
    expectTabInURL(page, "properties", "switching to the all properties tab")

    await page.goBack()
    await expectDocument(page)
    await settle(page)
    await expectOnlyPanel(page, "history")
    expectTabInURL(page, "history", "going back")

    // A tab which no document has is not an error: the view falls back to the tab it opens on by itself.
    await page.goto(`${PEERDB_URL}/d/${OBSERVATION_ID}?tab=nosuchtab`)
    await expectDocument(page)
    await settle(page)
    await expectOnlyPanel(page, "properties")

    console.log(`Successfully opened a document on a tab named by its address, went back to it, and fell back to the class tab for an unknown tab name.`)
  })

  test("Test the permissions tab of a document which grants nobody anything", async ({ context }) => {
    const page = await context.newPage()

    // A document which nobody was given anything on still has the permissions tab: reading it is granted by the
    // roles the site declares rather than by the document, so the list of users is empty and says so instead of
    // being left out.
    await openDocument(page, PLANET_ID)
    await settle(page)
    await openDocumentTab(page, "permissions")
    await settle(page)
    await expectOnlyPanel(page, "permissions")

    const permissionsPanel = page.locator(".pd-documentget-panel-permissions")
    await expect(permissionsPanel.locator(".pd-permissionsview"), "permissions view").toBeVisible()
    await expect(permissionsPanel.locator(".pd-permissionsview-empty-users"), "the line saying nobody was granted anything").toBeVisible()
    await expect(permissionsPanel.locator(".pd-permissionsview-item-user"), "users the document grants access to").toHaveCount(0)
    // The requests section is rendered only when somebody has asked for access, and nobody has asked here.
    await expect(permissionsPanel.locator(".pd-permissionsview-section-requests"), "the section of pending access requests").toHaveCount(0)
    // Deciding access and asking for it are both offered only to somebody who is signed in, and this caller is
    // not, so the panel is the two lists and nothing else.
    await expect(permissionsPanel.locator(".pd-permissionsview-button-edit"), "the button to edit the granted access").toHaveCount(0)
    await expect(permissionsPanel.locator(".pd-permissionsview-button-request"), "the button to ask for access").toHaveCount(0)

    await checkpoint(page, "tabs-planet-tab-permissions-empty", { mask: volatile(page) })
    await checkpointElement(page, permissionsPanel, "tabs-planet-panel-permissions-empty")

    console.log("Successfully verified that the permissions tab of a document without permission claims lists no users and offers nothing to an anonymous caller.")
  })
})
