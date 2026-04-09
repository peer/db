import { SHORT_NAME } from "@/core"
import { searchWithQuery } from "../peerdb_utils"
import { checkpoint, expect, test } from "../utils"

test.describe("PeerDB Document Navigation", () => {
  test("Prev/next navigation between documents in a search session", async ({ context }) => {
    const page = await context.newPage()

    await searchWithQuery(page, "")

    const shortName = page.locator(`#result-${SHORT_NAME} h2 .link`)
    await expect(shortName).toBeVisible()
    await shortName.click()

    const documentGet = page.locator(".pd-documentget")
    await expect(documentGet).toBeVisible()
    await checkpoint(page, "navigation-document-short-name")

    // First document does not have a previous one, so prev button is disabled.
    const prevButton = page.locator("#documentget-button-prev")
    await expect(prevButton).toBeVisible()
    await expect(prevButton).toHaveClass(/cursor-not-allowed/)
    const nextButton = page.locator("#documentget-button-next")
    await expect(nextButton).not.toHaveClass(/cursor-not-allowed/)
    await expect(nextButton).toBeVisible()
    await nextButton.click()

    await expect(documentGet).toBeVisible()
    await checkpoint(page, "navigation-next-document-list")

    // After navigating to next, prev becomes enabled.
    await expect(prevButton).toBeVisible()
    await expect(prevButton).not.toHaveClass(/cursor-not-allowed/)
    await prevButton.click()

    await expect(documentGet).toBeVisible()
    await checkpoint(page, "navigation-document-short-name")

    console.log('Successfully navigated to "short name" document, navigated to next document, then returned to original via prev button.')
  })
})
