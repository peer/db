import type { Page } from "@playwright/test"

import { CLASS_IDS, documentIdOf, LANGUAGES, PROPERTY_IDS, searchByCoreClass, type Language } from "../peerdb_utils"
import {
  checkpoint,
  checkpointElement,
  expect,
  expectResults,
  goHome,
  loadAllResults,
  LOADING_TIMEOUT,
  resultCount,
  resultIds,
  searchAgain,
  searchWithQuery,
  settle,
  settleFilters,
  switchLanguage,
  test,
  volatile,
} from "../utils"

// How many documents the populated instance holds for a visitor who is not signed in: everything of the test
// data except the interviews, which are closed, plus the schema documents the site is built out of. The
// searches below are read-only and the project they run in is ordered before every project which writes, so
// the number is the same on every run.
const EVERYTHING_TOTAL = 1421

// A query which narrows the search to one corner of the catalogue: the name of a star system, which its
// planets, moons and the records taken there carry as the place they belong to.
const NARROWING_QUERY = "Kephra"
const NARROWING_TOTAL = 21

// A word which the test data uses in exactly one document, in the middle of a sentence of its description.
// It is what a visitor who remembers a phrase rather than a name searches for.
const SINGLE_QUERY = "contradictory"

// A word which appears only inside descriptions and in no name, so a search for it can only have matched
// through the text of a document rather than through what the document is called.
const DESCRIPTION_QUERY = "colleagues"
const DESCRIPTION_TOTAL = 2

// The name one vocabulary entry of the test data is given in each of the three languages the site is served
// in. Every document which classifies itself by that entry carries its name in all three, so the same query
// finds the same documents whichever of the three it is written in. These are values of the test data and
// not labels of the interface, which is what makes hard-coding them here right: they are what is typed into
// the search box, and the point of the test is that each of the three finds the same thing.
const LANGUAGE_QUERIES: Record<Language, string> = {
  en: "Hydrothermal vent field",
  sl: "Hidrotermalno polje",
  pt: "Campo hidrotermal",
}
const LANGUAGE_TOTAL = 41

// The documents the queries above are expected to find.
const VENT_FIELD = await documentIdOf("BIOME", "VENT_FIELD")
const MILKY_WAY = await documentIdOf("GALAXY", "G1_MILKY_WAY")
const HALVORSEN = await documentIdOf("RESEARCHER", "RES_HALVORSEN")

// The identifier of the search session the browser is on, which is what says whether a search which was
// changed from inside the result page went on using the session it started in or opened a new one.
function sessionId(page: Page): string {
  const match = /\/s\/([0-9A-Za-z]+)/.exec(page.url())
  expect(match, `the browser is on a search session: ${page.url()}`).not.toBeNull()
  return match![1]
}

// Runs a query from the search box of the navbar, which edits the query of the running session rather than
// starting a new search, and waits until the results of the changed search have rendered.
async function searchAgainWithQuery(page: Page, query: string): Promise<void> {
  await searchAgain(page, async () => {
    const searchInput = page.locator("#search-input-text")
    await expect(searchInput, "the search box of the navbar").toBeVisible()
    await searchInput.fill(query)
    const searchButton = page.locator(".pd-navbarsearch-button")
    await expect(searchButton, "the search button of the navbar").toBeVisible()
    await searchButton.click()
  })
}

// The query the results header reports it searched for. The sentence around it is written in the language of
// the page, while the query itself is set in italics inside it, so the query is read out of that element and
// the sentence is left alone.
function reportedQuery(page: Page) {
  return page.locator(".pd-searchresultsheader-text-query i")
}

test.describe("PeerDB Search Flows", () => {
  test("Test a search over everything", async ({ context }) => {
    const page = await context.newPage()

    // An empty query is not a query at all: it asks for the whole catalogue, which is how a visitor who does
    // not know what to look for starts.
    await searchWithQuery(page, "")

    await expect.poll(() => resultCount(page), { message: "a search over everything finds every document" }).toBe(EVERYTHING_TOTAL)
    await expect(reportedQuery(page), "the header reports a search which was given no query").toHaveCount(0)
    // The search box of the navbar carries the query of the session, so an empty query leaves it empty and
    // the visitor can type into it to narrow what they are looking at.
    await expect(page.locator("#search-input-text"), "the query of a search over everything").toHaveValue("")

    // The feed renders whole pages of results and leaves the rest behind the load more button, so what is in
    // front of the visitor is a fraction of what the header reports.
    const shown = await page.locator(".pd-searchresult").count()
    expect(shown, "the feed renders whole pages of results").toBeGreaterThanOrEqual(10)
    expect(shown % 10, "the feed renders whole pages of results").toBe(0)
    expect(shown, "what is rendered on load is less than the search found").toBeLessThan(EVERYTHING_TOTAL)
    await expect(page.locator("#searchresultsfeed-button-loadmore"), "the feed offers the results it did not render").toBeVisible()

    await settle(page)
    // The screenshot is of the viewport and not of the whole page: a search over everything has a facet for
    // every property of the schema, and a full page capture of the panel holding all of them would be many
    // times the height of the screen without showing anything the top of the page does not already.
    await checkpoint(page, "homesearch-everything", { fullPage: false, mask: volatile(page) })

    console.log(`Successfully ran a search over everything, which found ${EVERYTHING_TOTAL} documents and rendered ${shown} of them.`)
  })

  test("Test a query which narrows a search", async ({ context }) => {
    const page = await context.newPage()

    await searchWithQuery(page, "")
    const everything = await resultCount(page)
    const session = sessionId(page)

    // Typing into the search box of the result page edits the query of the session which is already running,
    // so the visitor narrows what they are looking at instead of starting again from the home page.
    await searchAgainWithQuery(page, NARROWING_QUERY)

    await expect.poll(() => resultCount(page), { message: "a query narrows what the search found" }).toBe(NARROWING_TOTAL)
    expect(NARROWING_TOTAL, "a narrowed search finds less than the whole catalogue").toBeLessThan(everything)
    expect(sessionId(page), "narrowing a search goes on using the session it started in").toBe(session)
    await expect(reportedQuery(page), "the header reports the query which was searched for").toHaveText(NARROWING_QUERY)
    await expect(page.locator("#search-input-text"), "the search box keeps the query it was given").toHaveValue(NARROWING_QUERY)

    // Everything the narrowed search found is loaded before the page is captured. The feed reveals whole
    // pages until its column is filled to about two viewports, so how much it renders on load follows how
    // tall the documents it happened to render are, which is not the same from one run to the next for a
    // result set holding documents of several classes.
    await loadAllResults(page)
    await expect(page.locator(".pd-searchresult"), "the feed shows every result the narrowed search found").toHaveCount(NARROWING_TOTAL)

    await settleFilters(page)
    await checkpoint(page, "homesearch-narrowed", { mask: volatile(page) })

    console.log(`Successfully narrowed a search from ${everything} documents to ${NARROWING_TOTAL} with the query ${NARROWING_QUERY}.`)
  })

  test("Test a query which matches exactly one document", async ({ context }) => {
    const page = await context.newPage()

    await searchWithQuery(page, SINGLE_QUERY)

    await expect.poll(() => resultCount(page), { message: "a query which matches one document finds one result" }).toBe(1)
    await expect(page.locator(".pd-searchresult"), "the feed renders the one result it found").toHaveCount(1)
    expect(await resultIds(page), "the document the query matched").toEqual([MILKY_WAY])
    // With everything the search found in front of the visitor there is nothing left to ask for, so the feed
    // marks the end of the results instead of offering to load more of them.
    await expect(page.locator("#searchresultsfeed-button-loadmore"), "nothing is left to load for a search which found one result").toHaveCount(0)
    await expect(page.locator(".pd-searchresultspager"), "a single result is not broken up by a pager").toHaveCount(0)
    await expect(page.locator(".pd-searchresultsendbar"), "the end of the results is marked").toBeVisible()

    await settleFilters(page)
    await checkpoint(page, "homesearch-single-result", { mask: volatile(page) })

    console.log(`Successfully ran the query ${SINGLE_QUERY}, which matched exactly one document.`)
  })

  test("Test a query which matches a word of a description", async ({ context }) => {
    const page = await context.newPage()

    await searchWithQuery(page, DESCRIPTION_QUERY)

    await expect.poll(() => resultCount(page), { message: "a query which matches inside descriptions finds results" }).toBe(DESCRIPTION_TOTAL)
    expect(await resultIds(page), "the documents whose description holds the word").toContain(HALVORSEN)

    // The word is in no name of the document, only in the text of it, so the card has to show the text the
    // match came from rather than only what the document is called.
    const matched = page.locator(`#result-${HALVORSEN}`)
    await expect(matched, "the card of the document whose description matched").toBeVisible()
    await expect(matched.locator(".pd-searchresult-link-title"), "the title of the matched document does not hold the word").not.toContainText(DESCRIPTION_QUERY, {
      ignoreCase: true,
    })
    const description = matched.locator(`.pd-fieldsview-row-${PROPERTY_IDS.DESCRIPTION} .pd-claimvaluehtml`)
    await expect(description, "the description of the matched document holds the word which was searched for").toContainText(DESCRIPTION_QUERY, {
      ignoreCase: true,
    })

    await settleFilters(page)
    await checkpointElement(page, matched, "homesearch-description-match", volatile(page))

    console.log(`Successfully ran the query ${DESCRIPTION_QUERY}, which matched ${DESCRIPTION_TOTAL} documents through their description.`)
  })

  for (const language of LANGUAGES) {
    test(`Test a query against language tagged values in ${language}`, async ({ context }) => {
      const page = await context.newPage()

      // The document the query names itself after carries its name in each of the three languages, and so does
      // every document which classifies itself by it, so the three queries are three ways of asking the same
      // thing and have to find the same documents.
      await goHome(page)
      await switchLanguage(page, language)
      await searchWithQuery(page, LANGUAGE_QUERIES[language])

      await expect.poll(() => resultCount(page), { message: `the query written in ${language} finds the documents named in it` }).toBe(LANGUAGE_TOTAL)
      const found = await resultIds(page)
      expect(found[0], `the document the query in ${language} names is what the search ranks first`).toBe(VENT_FIELD)

      // The card is captured on its own rather than the whole page: what the query found is the same in every
      // language while the order the rest of it comes in follows term statistics of that language, so only the
      // first card is the same from one run to the next.
      await settle(page)
      await checkpointElement(page, page.locator(`#result-${VENT_FIELD}`), `homesearch-language-${language}`, volatile(page))

      console.log(`Successfully ran a query written in ${language}, which found ${LANGUAGE_TOTAL} documents and ranked the document it names first.`)
    })
  }

  test("Test what a search result shows", async ({ context }) => {
    const page = await context.newPage()

    await searchWithQuery(page, SINGLE_QUERY)
    const session = sessionId(page)

    // A result stands for a document and has to offer every way into it: the title links to it, the details
    // button links to it for a document which has no title to link by, and both carry the search session so
    // that the document view can offer the way back to the results.
    const result = page.locator(`#result-${MILKY_WAY}`)
    await expect(result, "the card of the document which was found").toBeVisible()
    await expect(result.locator(".pd-searchresult-link-title"), "the title of the result links to the document").toHaveAttribute("href", `/d/${MILKY_WAY}?s=${session}`)
    await expect(result.locator(".pd-searchresult-link-details"), "the details button of the result links to the document").toHaveAttribute(
      "href",
      `/d/${MILKY_WAY}?s=${session}`,
    )

    // The badges say what the document is. They are addressed by the document they stand for and not by the
    // label they render, which is written in the language of the page.
    const badges = result.locator(".pd-searchresult-badge-type")
    await expect(badges, "the result is badged with the class of the document").toHaveCount(1)
    await expect(badges, "the badge stands for the class the document is an instance of").toHaveAttribute("data-url", `/api/d/${CLASS_IDS.GALAXY}`)

    // The body of the card is the document rendered through the fields its class declares, so the description
    // is one of the rows rather than a summary the result page writes itself.
    await expect(result.locator(`.pd-fieldsview-row-${PROPERTY_IDS.NAME}`), "the card shows the name of the document").toBeVisible()
    await expect(result.locator(`.pd-fieldsview-row-${PROPERTY_IDS.DESCRIPTION}`), "the card shows the description of the document").toBeVisible()

    await settle(page)
    await checkpointElement(page, result, "homesearch-result-card", volatile(page))

    console.log(`Successfully verified what a search result shows for the document ${MILKY_WAY}.`)
  })

  test("Test that a search session is reachable again by its address", async ({ context }) => {
    const page = await context.newPage()

    await searchWithQuery(page, SINGLE_QUERY)
    const session = sessionId(page)
    const found = await resultIds(page)

    // The address of a running search is what a visitor keeps or hands to somebody else, so opening it again
    // has to bring back the same search rather than an expired session or an empty result page.
    const reopened = await context.newPage()
    await reopened.goto(`${page.url().split("?")[0]}`)
    await expectResults(reopened)

    expect(sessionId(reopened), "the reopened address is the session the search was run in").toBe(session)
    await expect.poll(() => resultCount(reopened), { message: "the reopened search found what it found before" }).toBe(1)
    expect(await resultIds(reopened), "the reopened search shows the same documents").toEqual(found)
    await expect(reportedQuery(reopened), "the reopened search kept the query it was run with").toHaveText(SINGLE_QUERY)
    await expect(reopened.locator("#search-input-text"), "the search box of the reopened search holds the query").toHaveValue(SINGLE_QUERY)

    await settleFilters(reopened)
    await checkpoint(reopened, "homesearch-session-reopened", { mask: volatile(reopened) })

    console.log(`Successfully reopened the search session ${session} by its address and found the same result.`)
  })

  test("Test that the search session keeps which view was chosen", async ({ context }) => {
    const page = await context.newPage()

    // The choice between the feed and the table is part of the search and not of the browser, so it has to
    // survive being reloaded and has to be there for whoever the address is handed to.
    await searchByCoreClass(page, "UNIT")
    const session = sessionId(page)
    const address = page.url().split("?")[0]

    const table = page.locator(".pd-searchresultstable-list-results")
    await expect(table, "the results are shown as a feed before the view is switched").toHaveCount(0)
    await page.locator(".pd-selectbutton-button-table").click()
    await expect(table, "the results are shown as a table once the view is switched").toBeVisible({ timeout: LOADING_TIMEOUT })
    await settle(page)

    await page.goto(address)
    await expect(table, "the reloaded search is still shown as a table").toBeVisible({ timeout: LOADING_TIMEOUT })
    await expect(page.locator(".pd-searchresultsfeed"), "the reloaded search does not fall back to the feed").toHaveCount(0)
    expect(sessionId(page), "the view was kept by the session and not by the address").toBe(session)

    await settle(page)
    // The table renders the whole result set at once rather than a page of it, so what it holds is what the
    // header of the search reports.
    const rows = await resultCount(page)
    await expect.poll(() => page.locator(".pd-searchresultstable-row-result").count(), { message: "the table of the reloaded search holds every result" }).toBe(rows)
    await checkpoint(page, "homesearch-table-view", { fullPage: false, mask: volatile(page) })

    console.log(`Successfully kept the table view across a reload of the search session ${session}, which holds ${rows} rows.`)
  })
})
