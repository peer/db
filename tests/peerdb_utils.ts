import type { Page } from "@playwright/test"

import { checkpoint, expect, PEERDB_URL } from "./utils"

export const TOTAL_CORE_DOCUMENTS = 80
export const SEARCH_DEFAULT_LIMIT = 50

// Verify search input and button exist, proceed with custom query.
export async function searchWithQuery(page: Page, query: string): Promise<void> {
  await page.goto(PEERDB_URL)

  const searchInput = page.locator("#home-input-search")
  await expect(searchInput).toBeVisible()
  const searchButton = page.locator("#home-button-search")
  await expect(searchButton).toBeVisible()
  await checkpoint(page, "home-page-before-search")

  await searchInput.fill(query)
  await searchButton.click()

  await checkpoint(page, query === "" ? "search-default-results" : `search-query-${query}`)
}

export async function testDocumentPage(page: Page, id: string, expectedTitle: string, checkpointName: string): Promise<void> {
    await searchWithQuery(page, "")

    const result = page.locator(`#result-${id} .pd-searchresult-link-title`)
    await expect(result).toBeVisible()

    // Playwright's click() scrolls to the element. That updates the URL, updating router and cancelling previous
    // navigation. el.click() via evaluate() fires the click without scrolling.
    await Promise.all([page.waitForURL(new RegExp(`/d/${id}`)), result.evaluate((el: HTMLElement) => el.click())])

    const documentGet = page.locator(".pd-documentget")
    await expect(documentGet).toBeVisible()

    await expect(page.locator(".pd-documentget h1")).toContainText(expectedTitle)

    await checkpoint(page, checkpointName)
}
