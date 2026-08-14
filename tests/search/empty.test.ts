import type { Page } from "@playwright/test"

import en from "@/locales/en.json" with { type: "json" }
import pt from "@/locales/pt.json" with { type: "json" }
import sl from "@/locales/sl.json" with { type: "json" }
import { CLASS_IDS, documentIdOf, LANGUAGES, PROPERTY_IDS, RESTRICTED_CLASS, searchByClass } from "../peerdb_utils"
import {
  applyFilterValue,
  applySearchChange,
  checkpoint,
  expect,
  expectNoResults,
  expectResults,
  filter,
  filterValue,
  goHome,
  loadAllResults,
  LOADING_TIMEOUT,
  openFilters,
  PEERDB_URL,
  resultCount,
  searchWithQuery,
  settle,
  settleFilters,
  settleSearch,
  switchLanguage,
  test,
  volatile,
} from "../utils"

// A query which matches no document of the test data, in any of its fields. It is deliberately not a word of
// any of the three languages the data is written in, so that no stemmer or folding can make it match
// something.
const MISSING_QUERY = "zzzqqqxyzzy"

// A query which does match, used to show that a search which found nothing is not a dead end.
const MATCHING_QUERY = "Kephra"

// The messages the interface writes in place of results and in place of facets, read from the application's
// own translations rather than repeated here, so that a test which runs in three languages asserts what each
// of them actually says.
const LOCALES = { en, sl, pt }

// The synthetic value a reference facet offers for the documents which do not carry the property at all
// (MISSING_VALUE_ID in src/utils.ts). It is a sentinel of the interface and not a label, so it is the same in
// every language.
const MISSING_VALUE = "__MISSING__"

// The class which no document of the test data is an instance of on its own, and how many documents a search
// for it finds all the same: every document of the classes below it, because the index carries the ancestors
// of the class each document is an instance of.
const ABSTRACT_CLASS = "PLACE"
const ABSTRACT_CLASS_TOTAL = 300

// The star systems of the test data and how many of them record no species as native to them, which is the
// facet value the filter tests below select.
const STAR_SYSTEM_TOTAL = 44
const NO_HOME_SPECIES_TOTAL = 18

// A galaxy is contained in nothing, so a search for the galaxies which are contained in one is a pair of
// filters which cannot both hold, and finds nothing however much the catalogue holds.
const MILKY_WAY = await documentIdOf("GALAXY", "G1_MILKY_WAY")

// How many times an address which limits a search is opened when it is checked that one address always
// limits the search the same way. Two limits can be arranged in two ways, so a run which opens the address
// this many times and sees one arrangement every time is as good as a proof that the arrangement is fixed.
const REPEATED_OPENS = 10

// Opens a search which is limited by prefilters, which is what a shortcut address builds: it takes a property
// identifier as the name of a query parameter and a value identifier as its value, and every pair it carries
// limits the search further. Nothing is waited for beyond the search having finished, because the searches
// opened this way are the ones which are expected to find nothing.
async function openPrefiltered(page: Page, pairs: Array<[string, string]>): Promise<void> {
  await page.goto(`${PEERDB_URL}/s?${pairs.map(([prop, value]) => `${prop}=${value}`).join("&")}`)
  await settleSearch(page)
}

// Runs an action which changes the search from inside the result page, and waits until what the page shows is
// the result of the changed search.
//
// This is the searchAgain of the shared helpers with the one difference the tests here need: what is waited
// for once the results of the changed search have come back follows what that search is expected to find,
// because a search which found nothing renders no result to wait for and waiting for one could only time out.
async function changeSearch(page: Page, action: () => Promise<void>, found: boolean): Promise<void> {
  await applySearchChange(page, action)
  if (found) {
    await expectResults(page)
  } else {
    await expectNoResults(page)
  }
}

// Runs a query from the search box of the navbar, which edits the query of the running session rather than
// starting a new search, and waits until the results of the changed search have rendered.
async function searchAgainWithQuery(page: Page, query: string, found: boolean): Promise<void> {
  await changeSearch(
    page,
    async () => {
      const searchInput = page.locator("#search-input-text")
      await expect(searchInput, "the search box of the navbar").toBeVisible()
      await searchInput.fill(query)
      const searchButton = page.locator(".pd-navbarsearch-button")
      await expect(searchButton, "the search button of the navbar").toBeVisible()
      await searchButton.click()
    },
    found,
  )
}

// Asserts everything the feed leaves out when the search found nothing: no result, and none of the controls
// which belong to a list of results.
async function expectEmptyFeed(page: Page): Promise<void> {
  await expectNoResults(page)
  await expect(page.locator(".pd-searchresult"), "a search which found nothing renders no result").toHaveCount(0)
  await expect(page.locator("#searchresultsfeed-button-loadmore"), "a search which found nothing offers nothing to load").toHaveCount(0)
  await expect(page.locator(".pd-searchresultspager"), "a search which found nothing is broken up by no pager").toHaveCount(0)
  await expect(page.locator(".pd-searchresultsendbar"), "a search which found nothing marks no end of results").toHaveCount(0)
}

test.describe("PeerDB Empty Search Flows", () => {
  for (const language of LANGUAGES) {
    test(`Test a search which finds nothing in ${language}`, async ({ context }) => {
      const page = await context.newPage()

      await goHome(page)
      await switchLanguage(page, language)
      await searchWithQuery(page, MISSING_QUERY, { results: false })

      await expectEmptyFeed(page)
      // In place of the count of results the header says that there were none, in the language of the page.
      await expect(page.locator(".pd-searchresultsheader-count-results"), "the header reports that nothing was found").toHaveText(
        LOCALES[language].partials.SearchResultsHeader.noResults,
      )
      await expect.poll(() => resultCount(page), { message: "a search which finds nothing reports no results" }).toBe(0)
      // The query is kept, so the visitor can see what was searched for and correct it.
      await expect(page.locator("#search-input-text"), "the query of the search which found nothing").toHaveValue(MISSING_QUERY)

      await settleFilters(page)
      await checkpoint(page, `emptysearch-nothing-found-${language}`, { mask: volatile(page) })

      console.log(`Successfully ran a search which finds nothing in ${language}, which reported 0 results.`)
    })
  }

  test("Test the filters of a search which finds nothing", async ({ context }) => {
    const page = await context.newPage()

    await searchWithQuery(page, MISSING_QUERY, { results: false })
    await expectEmptyFeed(page)

    // With nothing found there is nothing to build a facet out of, so the filters column has to say so rather
    // than render an empty panel or keep the facets of the previous search.
    await openFilters(page)
    await expect(page.locator(".pd-filtersresult"), "facets of a search which finds nothing").toHaveCount(0)
    await expect(page.locator(".pd-searchresultsfeed-empty-filters"), "the filters column reports that there are no filters").toHaveText(
      LOCALES.en.partials.SearchResultsFeed.noFilters,
    )
    await expect(page.locator(".pd-searchresultsfeed-button-morefilters"), "there is no further batch of facets to ask for").toHaveCount(0)

    await settle(page)
    await checkpoint(page, "emptysearch-filters", { mask: volatile(page) })

    console.log("Successfully verified the filters of a search which finds nothing.")
  })

  test("Test widening a search which finds nothing", async ({ context }) => {
    const page = await context.newPage()

    await searchWithQuery(page, MISSING_QUERY, { results: false })
    await expectEmptyFeed(page)

    // Editing the query of the session which found nothing has to bring results back, so that the empty state
    // is somewhere a visitor can leave rather than a dead end.
    await searchAgainWithQuery(page, MATCHING_QUERY, true)

    await expect.poll(() => resultCount(page), { message: "widening the query brings results back" }).toBeGreaterThan(0)
    await expect(page.locator("#search-input-text"), "the widened query").toHaveValue(MATCHING_QUERY)
    await expect(page.locator(".pd-searchresult").first(), "the widened search renders results again").toBeVisible()

    // Everything the widened search found is loaded before the page is captured. The feed reveals whole pages
    // until its column is filled to about two viewports, so how much it renders on load follows how tall the
    // documents it happened to render are, which is not the same from one run to the next for a result set
    // holding documents of several classes.
    const widened = await resultCount(page)
    await loadAllResults(page)
    await expect(page.locator(".pd-searchresult"), "the feed shows every result the widened search found").toHaveCount(widened)

    await settleFilters(page)
    await checkpoint(page, "emptysearch-widened", { mask: volatile(page) })

    console.log(`Successfully widened a search which found nothing to the query ${MATCHING_QUERY}, which found results again.`)
  })

  test("Test a class filtered search which finds nothing", async ({ context }) => {
    const page = await context.newPage()

    // Narrowing a search which is limited to a class by a query which matches nothing keeps the limit in
    // effect: the two are combined, so a class which holds documents yields none of them for this query.
    await searchByClass(page, "STAR_SYSTEM")
    await expect.poll(() => resultCount(page), { message: "the class the search is limited to holds documents" }).toBe(STAR_SYSTEM_TOTAL)

    await searchAgainWithQuery(page, MISSING_QUERY, false)

    await expectEmptyFeed(page)
    await expect.poll(() => resultCount(page), { message: "a limited search which finds nothing reports no results" }).toBe(0)
    // The limit stays, so the visitor can widen the query without losing the scope they were working in.
    await expect(page.locator(".pd-prefilterlabel"), "the class the search which found nothing is limited to").toHaveCount(1)
    await expect(page.locator(".pd-searchresultsfeed-button-clearprefilters"), "the limit of the search which found nothing can be cleared").toBeVisible()

    await settle(page)
    await checkpoint(page, "emptysearch-class-filtered", { mask: volatile(page) })

    console.log(`Successfully narrowed a search over ${STAR_SYSTEM_TOTAL} documents of one class to nothing.`)
  })

  test("Test a combination of filters which finds nothing", async ({ context }) => {
    const page = await context.newPage()

    // Each pair of the address limits the search further, and the two here cannot both hold of one document,
    // so the combination finds nothing while each of them on its own finds plenty.
    await openPrefiltered(page, [
      [PROPERTY_IDS.INSTANCE_OF, CLASS_IDS.GALAXY],
      [PROPERTY_IDS.CONTAINED_IN, MILKY_WAY],
    ])

    await expectEmptyFeed(page)
    await expect.poll(() => resultCount(page), { message: "a combination of filters which cannot both hold finds nothing" }).toBe(0)
    // Both limits are named, so the visitor can see which combination it was that found nothing. Each of them
    // names the property it is on and the value it is for, and both link to the document they stand for, so
    // they are asserted by the documents they link to rather than by the labels they render.
    const prefilters = page.locator(".pd-searchresultsfeed-text-prefilters")
    await expect(prefilters.locator(".pd-prefilterlabel"), "both limits of the search which found nothing are shown").toHaveCount(2)
    for (const named of [PROPERTY_IDS.INSTANCE_OF, CLASS_IDS.GALAXY, PROPERTY_IDS.CONTAINED_IN, MILKY_WAY]) {
      await expect(prefilters.locator(`a[href="/d/${named}"]`), `the limits name the document ${named}`).toHaveCount(1)
    }
    const clearPrefilters = page.locator(".pd-searchresultsfeed-button-clearprefilters")
    await expect(clearPrefilters, "the limits of the search which found nothing can be cleared").toBeVisible()

    await settle(page)
    // The block listing the limits is masked because the order they are listed in is not the same from one
    // opening of the address to the next, which the test below this one is about. Everything the block holds
    // is asserted above, so the masking costs the comparison nothing but the arrangement of two lines.
    await checkpoint(page, "emptysearch-filter-combination", { mask: [...volatile(page), prefilters] })

    // Dropping the limits is the way out of the empty state, and it has to leave a search which finds
    // everything rather than a session which cannot be used again.
    await changeSearch(page, async () => await clearPrefilters.click(), true)
    await expect.poll(() => resultCount(page), { message: "clearing the limits brings results back" }).toBeGreaterThan(0)
    await expect(page.locator(".pd-prefilterlabel"), "no limit is left after they were cleared").toHaveCount(0)

    console.log("Successfully ran a combination of two filters which cannot both hold, which found nothing, and cleared it again.")
  })

  test("Test that one address always limits a search the same way", async ({ context }) => {
    const page = await context.newPage()

    // An address which limits a search is what a visitor keeps or hands on, so opening it has to build the
    // same search every time. The limits are listed in the order the session holds them in, so the order they
    // are rendered in is what says whether two openings of one address built the same session.
    const orders = new Set<string>()
    for (let opened = 0; opened < REPEATED_OPENS; opened++) {
      await openPrefiltered(page, [
        [PROPERTY_IDS.INSTANCE_OF, CLASS_IDS.GALAXY],
        [PROPERTY_IDS.CONTAINED_IN, MILKY_WAY],
      ])
      const properties = page.locator(".pd-searchresultsfeed-text-prefilters .pd-prefilterlabel a.pd-filterproplabel-value")
      await expect(properties, "both limits of the search are shown").toHaveCount(2)
      orders.add((await properties.evaluateAll((links) => links.map((link) => link.getAttribute("href")))).join(" "))
    }

    expect([...orders], `the ${REPEATED_OPENS} openings of one address limit the search in one order`).toHaveLength(1)

    console.log(`Successfully opened one limiting address ${REPEATED_OPENS} times.`)
  })

  test("Test a search narrowed to nothing while a filter is active", async ({ context }) => {
    const page = await context.newPage()

    // A facet value is selected first, so that the search which finds nothing afterwards has a filter of its
    // own on top of the query.
    await searchByClass(page, "STAR_SYSTEM")
    await settleFilters(page)
    const facet = filter(page, "ref", PROPERTY_IDS.HOME_TO_SPECIES)
    await expect(facet, "the facet the filter is selected in").toBeVisible()
    await applyFilterValue(page, facet, filterValue(page, "ref", [PROPERTY_IDS.HOME_TO_SPECIES], MISSING_VALUE))
    await expect.poll(() => resultCount(page), { message: "the selected filter narrows the search" }).toBe(NO_HOME_SPECIES_TOTAL)

    await searchAgainWithQuery(page, MISSING_QUERY, false)
    await expectEmptyFeed(page)

    // The filter which is in effect has to stay in the panel even though the search it belongs to found
    // nothing to build facets out of, because it is the only place it can be turned off again.
    await expect(page.locator(".pd-searchresultsfeed-empty-filters"), "the panel does not report that there are no filters while one is active").toHaveCount(0)
    await expect(facet, "the facet whose value is selected is still shown").toBeVisible()
    const cleared = facet.locator(".pd-filtersresult-button-clear")
    await expect(cleared, "the filter which is in effect can still be cleared").toBeVisible()
    await expect(filterValue(page, "ref", [PROPERTY_IDS.HOME_TO_SPECIES], MISSING_VALUE), "the selected value is still selected").toBeChecked()

    await settle(page)
    await checkpoint(page, "emptysearch-active-filter", { mask: volatile(page) })

    // Turning the filter off is the other way out of the empty state, and the query which found nothing stays
    // in effect, so what comes back is the query on its own.
    await changeSearch(page, async () => await cleared.click(), false)
    await expect(page.locator("#search-input-text"), "the query stays in effect once the filter is cleared").toHaveValue(MISSING_QUERY)

    console.log(`Successfully narrowed a search of ${NO_HOME_SPECIES_TOTAL} filtered documents to nothing while keeping the filter in effect.`)
  })

  test("Test a search for a class which holds nothing the visitor may read", async ({ context }) => {
    const page = await context.newPage()

    // The interviews of the test data are closed: a visitor who is not signed in may read none of them, so a
    // search limited to that class is the empty state a class produces without any query at all.
    await openPrefiltered(page, [[PROPERTY_IDS.INSTANCE_OF, CLASS_IDS[RESTRICTED_CLASS]]])

    await expectEmptyFeed(page)
    await expect.poll(() => resultCount(page), { message: "a class the visitor may read nothing of finds no results" }).toBe(0)
    await expect(page.locator(".pd-prefilterlabel"), "the class the search is limited to").toHaveCount(1)
    await expect(page.locator(".pd-searchresultsfeed-empty-filters"), "the filters column reports that there are no filters").toBeVisible()

    await settle(page)
    await checkpoint(page, "emptysearch-restricted-class", { mask: volatile(page) })

    console.log(`Successfully ran a search limited to ${RESTRICTED_CLASS}, of which a visitor who is not signed in may read nothing.`)
  })

  test("Test a search for a class no document is an instance of", async ({ context }) => {
    const page = await context.newPage()

    // An abstract class is declared to gather the classes below it and no document is an instance of one, but
    // the index carries the ancestors of the class of each document, so a search for it is not an empty state
    // at all: it finds every document of every class below it. This is why the empty class of this test data
    // is the closed one above and not an abstract one.
    await searchByClass(page, ABSTRACT_CLASS)

    await expect.poll(() => resultCount(page), { message: "an abstract class finds the documents of the classes below it" }).toBe(ABSTRACT_CLASS_TOTAL)
    await expect(page.locator(".pd-searchresult").first(), "the search for an abstract class renders results").toBeVisible({ timeout: LOADING_TIMEOUT })
    await expect(page.locator(".pd-prefilterlabel"), "the class the search is limited to").toHaveCount(1)

    await settle(page)
    await checkpoint(page, "emptysearch-abstract-class", { fullPage: false, mask: volatile(page) })

    console.log(`Successfully ran a search for the abstract class ${ABSTRACT_CLASS}, which found the ${ABSTRACT_CLASS_TOTAL} documents of the classes below it.`)
  })
})
