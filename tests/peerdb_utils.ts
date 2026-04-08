import type { Page } from "@playwright/test"

import { checkpoint, expect, PEERDB_URL } from "./utils"

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

export async function testDocumentPage(page: Page, id: string, expectedTitle: string): Promise<void> {
  await searchWithQuery(page, "")

  const result = page.locator(`#result-${id} h2 .link`)
  await expect(result).toBeVisible()

  // Playwright's click() scrolls to the element. That updates the URL, updating router and cancelling previous
  // navigation. el.click() via evaluate() fires the click without scrolling.
  await Promise.all([page.waitForURL(new RegExp(`/d/${id}`)), result.evaluate((el: HTMLElement) => el.click())])

  const documentGet = page.locator(".pd-documentget")
  await expect(documentGet).toBeVisible()

  // Confirm prev/next buttons are present.
  const prevButton = page.locator("#documentget-button-prev")
  await expect(prevButton).toBeVisible()
  const nextButton = page.locator("#documentget-button-next")
  await expect(nextButton).toBeVisible()

  await checkpoint(page, `document-navigation-${expectedTitle}`)
}

export async function testDocumentPageDirect(page: Page, id: string, expectedTitle: string): Promise<void> {
  await page.goto(`${PEERDB_URL}/d/${id}`)

  const documentGet = page.locator(".pd-documentget")
  await expect(documentGet).toBeVisible()

  // Without a search session the navbar shows a search button, not prev/next button navigation.
  const navBarSearch = page.locator(".pd-navbar-search")
  await expect(navBarSearch).toBeVisible()

  await checkpoint(page, `document-direct-${expectedTitle}`)

  // Search with empty query and confirm we return to full results.
  const searchButton = navBarSearch.locator("[type='submit']")
  await expect(searchButton).toBeVisible()
  await searchButton.click()

  await checkpoint(page, "search-default-results")
}
