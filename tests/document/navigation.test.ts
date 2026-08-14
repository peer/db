import type { Locator, Page } from "@playwright/test"

import type { DocumentClass } from "../peerdb_utils"

import { documentIdOf, LANGUAGES, PROPERTY_IDS, readTestData, searchByClass } from "../peerdb_utils"
import {
  checkpoint,
  checkpointElement,
  documentId,
  expect,
  expectDocument,
  expectResults,
  goHome,
  loadAllResults,
  openDocument,
  openFirstResult,
  PEERDB_URL,
  resultIds,
  settle,
  settleDocument,
  settleFilters,
  switchLanguage,
  test,
  volatile,
} from "../utils"

// The class whose results are walked. It is the class which holds the fewest documents while still holding
// more than a handful, so the whole result set is one page of results and the walk can be compared against it
// without loading further pages, and it is prefiltered by class rather than searched for by text, so the order
// the results come back in is the same on every run.
const WALK_CLASS: DocumentClass = "INSTITUTE"

// The documents the naming of a document is asserted on, one per kind of name the test data schema declares:
// a place, which is one of a chain of places containing one another, a person, whose name is split into a
// given name and a family name, and a record, which is told apart from the other records by its date.
const PLACE = { file: "site/G1_BEACON_ARCHIVE.json", id: await documentIdOf("SITE", "G1_BEACON_ARCHIVE") }
const PARENT_PLACE = { file: "region/G1_BEACON_PLAIN.json", id: await documentIdOf("REGION", "G1_BEACON_PLAIN") }
const GRANDPARENT_PLACE = { file: "moon/G1_FOOTNOTE.json", id: await documentIdOf("MOON", "G1_FOOTNOTE") }
const PERSON = { file: "researcher/RES_HALVORSEN.json", id: await documentIdOf("RESEARCHER", "RES_HALVORSEN") }
const RECORD = { file: "observation/OBSA_CORD_READING_RECORDER.json", id: await documentIdOf("OBSERVATION", "OBSA_CORD_READING_RECORDER") }

// One value of a field of a test data document, as its JSON file records it. A naming value is either written
// in a language or in none at all, and a time value carries the moment and how precisely it is known.
interface TestDataValue {
  value?: string
  time?: string
}

// The string one field of a test data document is written with. The fields read here are stated once, in
// English or in no language at all, and both are what the view shows whatever the interface language is: a
// value with no language of its own is in the bucket every fallback chain ends at, and the chains of the two
// other languages of the site pass through English on the way there (languagePriority in config.yml).
function stringOf(document: Record<string, unknown>, field: string): string {
  const values = document[field] as Array<TestDataValue>
  expect(values.length, `the ${field} of the test data document`).toBeGreaterThan(0)
  return values[0].value!
}

// The year a time field of a test data document records, which is the part of a date a record is told apart by.
function yearOf(document: Record<string, unknown>, field: string): string {
  return (document[field] as TestDataValue).time!.split("-")[0]
}

// The buttons which walk the results of the search a document was opened from. They are rendered into the
// navbar and only while the document view knows which search it was reached through.
function prevButton(page: Page): Locator {
  return page.locator("#documentget-button-prev")
}

function nextButton(page: Page): Locator {
  return page.locator("#documentget-button-next")
}

// Asserts that a walking button leads somewhere, which it does by being a link at all: the view renders the
// button as a plain block instead of a link once there is no result on that side to go to, so that it cannot
// be followed by a click or by the keyboard.
async function expectWalkable(button: Locator, target: string, what: string): Promise<void> {
  await expect(button, `the ${what} button`).toBeVisible()
  await expect(button, `the ${what} button is a link`).toHaveJSProperty("tagName", "A")
  await expect(button, `the ${what} button leads to the next document of the search`).toHaveAttribute("href", new RegExp(`/d/${target}(\\?|$)`))
}

async function expectNotWalkable(button: Locator, what: string): Promise<void> {
  await expect(button, `the ${what} button`).toBeVisible()
  await expect(button, `the ${what} button is not a link`).not.toHaveJSProperty("tagName", "A")
  await expect(button, `the ${what} button leads nowhere`).not.toHaveAttribute("href", /./)
}

// Follows one of the walking buttons and waits for the document it leads to.
async function walk(page: Page, button: Locator, target: string, what: string): Promise<void> {
  await button.click()
  await expectDocument(page)
  await settle(page)
  expect(documentId(page), `the ${what} button opens the neighbouring result`).toBe(target)
}

// The value cell of the first row of one property of the class tab of the document view.
function fieldValue(page: Page, propertyId: string): Locator {
  return page.locator(`.pd-documentget-panel-properties .pd-fieldsview-row-${propertyId}`).first().locator(".pd-fieldsview-value")
}

test.describe("PeerDB Document Navigation Flows", () => {
  // A document opened from a search is one of a list, and the view keeps that list: it offers a button to each
  // side of it, and following one opens the neighbouring result without going back to the results page. The
  // search is walked in every language the site is served in, because the buttons are labelled in it and the
  // walk has to reach the same documents in each.
  for (const language of LANGUAGES) {
    test(`Test walking the results of a search with the previous and next buttons in ${language}`, async ({ context }) => {
      const page = await context.newPage()

      await goHome(page)
      await switchLanguage(page, language)

      await searchByClass(page, WALK_CLASS)
      await settle(page)
      const found = await resultIds(page)
      expect(found.length, `results to walk in ${language}`).toBeGreaterThan(3)

      await openFirstResult(page)
      await settle(page)
      expect(documentId(page), `the first result opens its own document in ${language}`).toBe(found[0])

      // The first result has nothing before it, so the button to that side is there but leads nowhere, which is
      // what tells a visitor where in the result list they are.
      await expectNotWalkable(prevButton(page), "previous")
      await expectWalkable(nextButton(page), found[1], "next")
      await checkpoint(page, `document-navigation-walk-first-${language}`, { mask: volatile(page) })
      await checkpointElement(page, page.locator(".pd-documentget-group-prevnext"), `document-navigation-walk-buttons-${language}`)

      await walk(page, nextButton(page), found[1], "next")
      await expectWalkable(prevButton(page), found[0], "previous")
      await checkpoint(page, `document-navigation-walk-second-${language}`, { mask: volatile(page) })

      await walk(page, nextButton(page), found[2], "next")
      await walk(page, prevButton(page), found[1], "previous")
      await checkpoint(page, `document-navigation-walk-back-${language}`, { mask: volatile(page) })

      // The search a document was reached through stays in the navbar as the query which was run, so the walk
      // can be left at any point and the results returned to. Which result the page is at is recorded in the
      // address, and the result page keeps it up to date as the visitor scrolls (the topmost visible result,
      // see orderedResults in SearchResultsFeed.vue), so what is asserted is the search the link returns to and
      // that the document the walk was left at is among its results, and not which result the address names.
      const session = new URL(page.url()).searchParams.get("s")
      expect(session, `the document names the search it was opened from in ${language}`).not.toBeNull()
      const back = page.locator(".pd-documentget-link-query")
      await expect(back, `the link back to the search in ${language}`).toBeVisible()
      await back.click()
      await expectResults(page)
      expect(page.url(), `the link back to the search returns to the search it was opened from in ${language}`).toContain(`/s/${session}`)
      expect(page.url(), `the link back to the search records which result the page is at in ${language}`).toContain("at=")
      await expect(page.locator(`[id="result-${found[1]}"]`), `the result of the document the walk was left at in ${language}`).toBeVisible()
      await settleFilters(page)
      await checkpoint(page, `document-navigation-walk-return-${language}`, { mask: volatile(page) })

      console.log(`Successfully walked ${found.length} results of a ${WALK_CLASS} search with the previous and next buttons in ${language}.`)
    })
  }

  // The other end of the list behaves like the first one: the last result has nothing after it, so the button
  // to that side leads nowhere while the one to the other side does.
  test("Test the last result of a search offers no document to go to next", async ({ context }) => {
    const page = await context.newPage()

    await searchByClass(page, WALK_CLASS)
    await loadAllResults(page)
    const found = await resultIds(page)

    const last = page.locator(".pd-searchresult-link-title").nth(found.length - 1)
    await expect(last, "the title of the last result").toBeVisible()

    // The last result is opened by following the address it links to rather than by clicking it. The result
    // page records which result the visitor is at in the address and rewrites the address as the page scrolls
    // (useLocationAt in src/search.ts), while opening a result pushes the address of its document, and a click
    // on a result which is not on screen scrolls the page itself, so the rewrite can land while the click is
    // still being followed and swallow it. What this test is about is what the document view offers at the end
    // of a result list, which following the link reaches just as much and without the race.
    const href = await last.getAttribute("href")
    expect(href, "the last result links to its document").not.toBeNull()
    await page.goto(`${PEERDB_URL}${href}`)
    await settleDocument(page)
    expect(documentId(page), "the last result opens its own document").toBe(found[found.length - 1])

    await expectNotWalkable(nextButton(page), "next")
    await expectWalkable(prevButton(page), found[found.length - 2], "previous")
    await checkpoint(page, "document-navigation-walk-last", { mask: volatile(page) })
    await checkpointElement(page, page.locator(".pd-documentget-group-prevnext"), "document-navigation-walk-buttons-last")

    console.log(`Successfully verified that the last of ${found.length} results offers no document to go to next.`)
  })

  // A document opened by its identifier was reached through no search, so there is no list to walk and the
  // buttons are not rendered at all. The navbar then holds the ordinary search box instead of the search the
  // document was reached through.
  test("Test opening a document directly offers no walk through results", async ({ context }) => {
    const page = await context.newPage()

    await openDocument(page, PERSON.id)
    await settle(page)

    await expect(prevButton(page), "the previous button of a document opened directly").toHaveCount(0)
    await expect(nextButton(page), "the next button of a document opened directly").toHaveCount(0)
    await expect(page.locator(".pd-navbarshortcut"), "the search bar of a document opened directly").toHaveCount(0)
    await expect(page.locator("#search-input-text"), "the ordinary search box of the navbar").toBeVisible()
    await checkpoint(page, "document-navigation-direct", { mask: volatile(page) })

    console.log("Successfully verified that a document opened by its identifier offers no walk through results and keeps the ordinary search box.")
  })

  // A place is one of a chain of places containing one another, which is what its name has to be read against:
  // the view names it by its own name and states the place which contains it as a reference, so the chain the
  // catalogue tells two places of the same name apart by is walked one link at a time.
  //
  // The class also declares a template which composes the whole chain into one label. That template is
  // rendered by the server when it indexes a document, and the view applies it only when the application
  // registers a display label function for the class (getDisplayLabel in src/utils.ts), which the site under
  // test does not, so what the view shows is the naming claim of the document itself.
  test("Test a place is named by itself and states the place which contains it", async ({ context }) => {
    const page = await context.newPage()

    const place = stringOf(readTestData(PLACE.file), "name")
    const parent = stringOf(readTestData(PARENT_PLACE.file), "name")
    const grandparent = stringOf(readTestData(GRANDPARENT_PLACE.file), "name")

    await openDocument(page, PLACE.id)
    await settle(page)
    await expect(page.locator("#documentget-title"), "the name of the place").toHaveText(place)

    const containedIn = fieldValue(page, PROPERTY_IDS.CONTAINED_IN).locator(".pd-claimvalueref")
    await expect(containedIn, "the place which contains it").toHaveText(parent)
    await expect(containedIn, "the place which contains it is linked to").toHaveAttribute("data-url", `/api/d/${PARENT_PLACE.id}`)
    await checkpoint(page, "document-navigation-place", { mask: volatile(page) })

    // Following the reference is how a visitor climbs the chain, and the place one step up states the next one
    // in the same way, so the chain is walkable to its end and not only stated once.
    await containedIn.click()
    await settleDocument(page)
    expect(documentId(page), "the reference opens the place which contains it").toBe(PARENT_PLACE.id)
    await expect(page.locator("#documentget-title"), "the name of the place one step up").toHaveText(parent)

    const above = fieldValue(page, PROPERTY_IDS.CONTAINED_IN).locator(".pd-claimvalueref")
    await expect(above, "the place which contains that one").toHaveText(grandparent)
    await expect(above, "the place which contains that one is linked to").toHaveAttribute("data-url", `/api/d/${GRANDPARENT_PLACE.id}`)
    await checkpoint(page, "document-navigation-place-parent", { mask: volatile(page) })

    console.log(`Successfully walked the containment chain of a place: ${place}, ${parent}, ${grandparent}.`)
  })

  // A person is named by a given name and a family name, which the schema keeps as two properties rather than
  // as one string, so the view shows the given name as the name of the document and the family name as a field
  // of its own. The class composes the two into one label for the search index, the same way as for a place.
  test("Test a researcher is named by their given name and shows their family name", async ({ context }) => {
    const page = await context.newPage()

    const person = readTestData(PERSON.file)
    const given = stringOf(person, "name")
    const family = stringOf(person, "familyName")

    await openDocument(page, PERSON.id)
    await settle(page)

    await expect(page.locator("#documentget-title"), "the given name of the researcher").toHaveText(given)
    await expect(fieldValue(page, PROPERTY_IDS.NAME), "the given name of the researcher as a field").toHaveText(given)
    await expect(fieldValue(page, PROPERTY_IDS.FAMILY_NAME), "the family name of the researcher").toHaveText(family)
    await checkpoint(page, "document-navigation-researcher", { mask: volatile(page) })

    console.log(`Successfully verified that the researcher ${given} ${family} is named by their given name and states their family name.`)
  })

  // A record is one of many written down in the same field season, so its title alone does not say which one
  // it is: the date it was made on is what tells two of them apart, and the view states it as a field of the
  // record. The class composes the title and the year of that date into one label for the search index.
  test("Test a record is named by its title and states the date it was made on", async ({ context }) => {
    const page = await context.newPage()

    const record = readTestData(RECORD.file)
    const title = stringOf(record, "title")
    const year = yearOf(record, "observedOn")

    await openDocument(page, RECORD.id)
    await settle(page)

    await expect(page.locator("#documentget-title"), "the title of the record").toHaveText(title)
    const observedOn = fieldValue(page, PROPERTY_IDS.OBSERVED_ON)
    await expect(observedOn, "the date the record was made on").toContainText(year)
    await expect(observedOn.locator(".pd-claimvaluetime"), "the date the record was made on is rendered as a time").toBeVisible()
    await checkpoint(page, "document-navigation-record", { mask: volatile(page) })

    console.log(`Successfully verified that the record "${title}" is named by its title and states the date it was made on in ${year}.`)
  })
})
