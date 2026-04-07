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
}

export async function navigateToSearchResults(page: Page): Promise<void> {
    await searchWithQuery(page, "")

    const header = page.locator(".pd-searchresultsheader")
    await expect(header).toContainText("80 results found.")
    await expect(header).toContainText("Searching without query and with 0 active filters.")

    const results = page.locator("[id^='result-']")
    await expect(results).toHaveCount(50)
}
