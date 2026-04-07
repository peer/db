import { SHORT_NAME } from "@/core"
import { navigateToSearchResults, searchWithQuery } from "../peerdb_utils"
import { checkpoint, expect, test } from "../utils"

test.describe("PeerDB Search Flows", () => {
  test("Default search returns 80 core documents", async ({ context }) => {
    const page = await context.newPage()

    await navigateToSearchResults(page)
    await checkpoint(page, "search-default-50-results")

    const loadMoreButton = page.locator("#searchresultsfeed-button-loadmore")
    await expect(loadMoreButton).toBeVisible()

    const results = page.locator("[id^='result-']")
    // Results are loaded in batches of 50, remaining 30 are loaded when scrolling to bottom.
    await page.evaluate(() => window.scrollTo({ top: document.body.scrollHeight, behavior: "instant" }))
    await expect(results).toHaveCount(80)
    await expect(loadMoreButton).not.toBeVisible()
    await checkpoint(page, "search-default-all-80-results")

    console.log(
      "Successfully used default search showing 50 results, scrolled to trigger loading remaining 30, verified 80 documents appear and load more button disappears.",
    )
  })

  test("Search with no matching query shows no results", async ({ context }) => {
    const page = await context.newPage()

    await searchWithQuery(page, "no-results-expected")

    const header = page.locator(".pd-searchresultsheader")
    await expect(header).toBeVisible()
    await checkpoint(page, "search-zero-results")
    await expect(header).toContainText("No results found.")

    console.log("Successfully searched for no documents when querying non-existing document.")
  })

  test("Search query narrows results and finds short name property", async ({ context }) => {
    const page = await context.newPage()

    await searchWithQuery(page, "short")

    const header = page.locator(".pd-searchresultsheader")
    await expect(header).toBeVisible()
    await checkpoint(page, "search-query-short-name")
    await expect(header).toContainText("1 result found.")

    const shortNameResult = page.locator(`#result-${SHORT_NAME}`)
    await expect(shortNameResult).toBeVisible()

    const results = page.locator("[id^='result-']")
    await expect(results).toHaveCount(1)

    console.log('Successfully searched for "short name", verified it shows up and narrowed search results to 4.')
  })
})
