import type { Page } from "@playwright/test"

import { checkpoint, expect, PEERDB_URL } from "./utils"

export const TOTAL_CORE_DOCUMENTS = 80
export const SEARCH_DEFAULT_LIMIT = 50

// Verify search input and button exist, proceed with custom query, verify result count and take a screenshot.
export async function searchWithQuery(page: Page, query: string, expectedCount: number): Promise<void> {
  await page.goto(PEERDB_URL)

  const searchInput = page.locator("#home-input-search")
  await expect(searchInput).toBeVisible()
  const searchButton = page.locator("#home-button-search")
  await expect(searchButton).toBeVisible()
  await checkpoint(page, "home-page-before-search")

  await searchInput.fill(query)
  await searchButton.click()

  const header = page.locator(".pd-searchresultsheader")
  const results = page.locator("[id^='result-']")
  // A maximum of SEARCH_DEFAULT_LIMIT results are shown without scrolling, so we take the smaller number from the two.
  await expect(results).toHaveCount(Math.min(expectedCount, SEARCH_DEFAULT_LIMIT))
  await expect(header).toContainText(resultsFoundText(expectedCount))
  await expect(header).toContainText(searchingQueryAndFiltersText(query, 0))

  await checkpoint(page, query === "" ? "search-default-results" : `search-query-${query}`)
}

export function searchingQueryAndFiltersText(query: string, count: number): string {
  const queryText = query === "" ? "Searching without query" : `Searching query ${query}`
  const separator = query === "" ? " and with " : " and "
  const filtersText = count === 1 ? "1 active filter." : `${count} active filters.`
  return queryText + separator + filtersText
}

export function resultsFoundText(count: number): string {
  if (count === 0) return "No results found."
  if (count === 1) return "1 result found."
  return `${count} results found.`
}
