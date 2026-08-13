import { LANGUAGES } from "../peerdb_utils"
import {
  checkpoint,
  expect,
  expectNoResults,
  expectResults,
  goHome,
  resultCount,
  resultIds,
  searchWithQuery,
  settleFilters,
  switchLanguage,
  test,
  volatile,
} from "../utils"

// A query which matches documents of the test data across several classes, and one which matches nothing at
// all. The first is a word the catalogue uses everywhere, the second is not a word of it.
const QUERY = "weir"
const NO_MATCH_QUERY = "zzzznosuchthing"

test.describe("PeerDB Home Page Flows", () => {
  for (const language of LANGUAGES) {
    test(`Test the home page in ${language}`, async ({ context }) => {
      const page = await context.newPage()

      await goHome(page)
      await switchLanguage(page, language)

      // The home view is the search box, the site's logo above it and nothing else: no results, no filters
      // and no navbar search box, because the view is itself the search box.
      await expect(page.locator(".pd-home"), "home view").toBeVisible()
      await expect(page.locator("#home-link-logo"), "logo links into the search").toBeVisible()
      await expect(page.locator(".pd-home-logo"), "logo").toBeVisible()
      await expect(page.locator("#home-input-search"), "search box").toBeVisible()
      await expect(page.locator("#home-button-search"), "search button").toBeVisible()
      await expect(page.locator("#home-button-search"), "the search button is labelled").not.toHaveText(/^\s*$/)
      await expect(page.locator(".pd-searchresult"), "the home view shows no results").toHaveCount(0)
      await expect(page.locator(".pd-searchresultsfeed"), "the home view shows no results feed").toHaveCount(0)
      // The home view has a slot an application fills with something of its own. PeerDB itself puts
      // nothing in it.
      await expect(page.locator(".pd-home > div").last().locator("*"), "the extra slot of the home view is empty").toHaveCount(0)

      await checkpoint(page, `home-page-${language}`)

      console.log(`Successfully verified the home page in ${language}.`)
    })

    test(`Test searching from the home page in ${language}`, async ({ context }) => {
      const page = await context.newPage()

      await goHome(page)
      await switchLanguage(page, language)

      // Submitting the box starts a search session and leaves the browser on its results.
      await searchWithQuery(page, QUERY)
      expect(new URL(page.url()).pathname, "the search lands on a session of its own").toMatch(/^\/s\/[0-9A-Za-z]+$/)
      const found = await resultCount(page)
      expect(found, "the query finds documents").toBeGreaterThan(0)
      const ids = await resultIds(page)
      expect(ids.length, "the feed shows the results it found").toBeGreaterThan(0)
      await settleFilters(page)
      await checkpoint(page, `home-search-query-${language}`, { mask: volatile(page) })

      console.log(`Successfully searched from the home page in ${language} and found ${found} documents.`)
    })

    test(`Test the empty query from the home page in ${language}`, async ({ context }) => {
      const page = await context.newPage()

      await goHome(page)
      await switchLanguage(page, language)

      // An empty query is a search over everything the caller may read rather than no search at all.
      await searchWithQuery(page, "")
      await expectResults(page)
      const found = await resultCount(page)
      expect(found, "the empty query finds every readable document").toBeGreaterThan(100)
      await settleFilters(page)
      await checkpoint(page, `home-search-empty-${language}`, { mask: volatile(page) })

      console.log(`Successfully searched with an empty query in ${language} and found ${found} documents.`)
    })

    test(`Test a query which finds nothing in ${language}`, async ({ context }) => {
      const page = await context.newPage()

      await goHome(page)
      await switchLanguage(page, language)

      // A search which finds nothing still reaches a session and reports what it found, rather than
      // failing or staying on the home page.
      await searchWithQuery(page, NO_MATCH_QUERY, { results: false })
      await expectNoResults(page)
      expect(await resultCount(page), "a query which matches nothing finds nothing").toBe(0)
      await checkpoint(page, `home-search-no-results-${language}`)

      console.log(`Successfully verified a query which finds nothing in ${language}.`)
    })

    test(`Test the logo leads to a search over everything in ${language}`, async ({ context }) => {
      const page = await context.newPage()

      await goHome(page)
      await switchLanguage(page, language)

      // The logo is a link into the search rather than a link back to the page it is on, so pressing it
      // opens the same search an empty query would.
      await page.locator("#home-link-logo").click()
      await expectResults(page)
      const found = await resultCount(page)
      expect(found, "the logo opens a search over everything").toBeGreaterThan(100)
      await settleFilters(page)
      await checkpoint(page, `home-logo-search-${language}`, { mask: volatile(page) })

      console.log(`Successfully followed the home logo into a search over ${found} documents in ${language}.`)
    })
  }
})
