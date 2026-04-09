import type { Page } from "@playwright/test"

import { checkpoint, expect, PEERDB_URL } from "./utils"

// Verify search input and button exist, proceed with custom query and take a screenshot.
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
