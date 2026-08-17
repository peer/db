import type { Page } from "@playwright/test"

import { searchByClass } from "../peerdb_utils"
import {
  checkpoint,
  expect,
  LOADING_TIMEOUT,
  loadMoreButton,
  overrideSiteFeatures,
  resultCount,
  resultIds,
  searchResults,
  settle,
  settleFilters,
  test,
  volatile,
} from "../utils"

// The class the feed is scrolled through. It needs clearly more documents than one page holds so that
// reaching the end of the page has something to load several times over, few enough that loading all of
// them stays a test rather than a stress test, and no test which writes documents may create one of it, so
// the result set is the same from one run to the next. The test data holds 56 of them.
const SCROLLED_CLASS = "ORGANISM"

// How many results one page of the feed holds (SEARCH_INITIAL_LIMIT and SEARCH_INCREASE in
// SearchResultsFeed.vue), which is both how many it renders to begin with and how many each load adds.
const PAGE_SIZE = 10

// How many times the end of the page may be reached without the feed having loaded everything, before the
// test gives up on scrolling ever getting there. One pass loads at least one page, so a result set of any
// size the test data holds is covered many times over, and reaching the cap means scrolling stopped loading
// rather than that the search is large.
const SCROLL_PASSES = 30

// Scrolls to the end of the page, which is what a visitor reading down the results does, and returns how
// many results the feed shows once it has answered. Reaching the end is what the feed watches for, so this
// is the whole of the interaction: nothing in this test presses the load more button.
async function scrollToEnd(page: Page): Promise<number> {
  await page.evaluate(() => window.scrollTo({ top: document.body.scrollHeight, behavior: "instant" }))
  await settle(page)
  return await searchResults(page).count()
}

test.describe("PeerDB Scrolling Flows", () => {
  test("Test scrolling through a search loads its results a page at a time", async ({ context }) => {
    // Every page of results is loaded one at a time and each one fetches the documents it renders, which
    // takes more than the default budget for a search with this many results.
    test.slow()

    const page = await context.newPage()

    // The site is served to the tests with loading while scrolling turned off, so that what a screenshot of
    // a search shows is what the test asked for rather than how far the feed got on its own. This test is
    // the one about that loading, so it asks for the site's own behaviour back.
    await overrideSiteFeatures(page, { disableLoadingOnScroll: false })

    await searchByClass(page, SCROLLED_CLASS)
    await settle(page)

    const total = await resultCount(page)
    expect(total, `the search for ${SCROLLED_CLASS} documents finds more than two pages of results`).toBeGreaterThan(2 * PAGE_SIZE)

    // What the feed loads before anything is scrolled is what fills its column, which is whole pages of
    // results but not a fixed number of them: it stops once the column reaches about two viewports, and how
    // many pages that takes follows how tall the results have rendered. So it is asserted to be pages, and
    // to be less than everything, rather than to be a particular count.
    const onLoad = await searchResults(page).count()
    expect(onLoad, "the feed fills its column with whole pages of results").toBeGreaterThanOrEqual(PAGE_SIZE)
    expect(onLoad % PAGE_SIZE, "the feed fills its column with whole pages of results").toBe(0)
    expect(onLoad, "filling the column leaves results for the visitor to scroll to").toBeLessThan(total)
    await expect(loadMoreButton(page), "the feed offers the results it has not loaded").toBeVisible()

    // Reading down the page loads the rest of the results, a page or more at a time. Each pass is asserted
    // to add whole pages and to add them below what was already there, which is what says the feed extends
    // its list as the visitor goes rather than replacing it or repeating it.
    let shown = onLoad
    let ids = await resultIds(page)
    let passes = 0
    while (shown < total) {
      expect(passes, "reaching the end of the page keeps loading results").toBeLessThan(SCROLL_PASSES)
      passes += 1

      const before = ids
      await expect
        .poll(async () => await scrollToEnd(page), { message: "reaching the end of the page loads more results", timeout: LOADING_TIMEOUT })
        .toBeGreaterThan(before.length)

      shown = await searchResults(page).count()
      expect(shown, "scrolling never loads more results than the search found").toBeLessThanOrEqual(total)
      // Every page but the last is whole, and the last one is whatever is left of the result set.
      expect(shown % PAGE_SIZE === 0 || shown === total, "scrolling loads whole pages of results until the last one").toBeTruthy()

      ids = await resultIds(page)
      expect(ids.slice(0, before.length), "the results which were already loaded stay where they were").toEqual(before)
    }

    // At the end everything the search found is on the page, once each, and the feed says so: the button is
    // gone and the end bar is in its place.
    expect(shown, "scrolling to the end loads every result the search found").toBe(total)
    expect(ids, "every result which was loaded").toHaveLength(total)
    expect(new Set(ids).size, "no result is loaded twice however many times the page was scrolled").toBe(total)
    await expect(loadMoreButton(page), "nothing is left to load once every result is shown").toHaveCount(0)
    await expect(page.locator(".pd-searchresultsendbar"), "the end of the results is marked").toBeVisible()

    // Everything loaded is the one state of a feed which loads while scrolling that does not depend on how
    // far the scrolling had got when the screenshot was taken, so it is the only one worth comparing.
    await settleFilters(page)
    await checkpoint(page, "scrolling-loaded-all", { mask: volatile(page) })

    console.log(`Successfully scrolled through a search, which loaded all ${total} results in ${passes} passes after showing ${onLoad} of them.`)
  })
})
