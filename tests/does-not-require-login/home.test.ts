import { checkpoint, expect, PEERDB_URL, test } from "../utils"

test.describe("PeerDB Search Flows", () => {
  test("Show search input", async ({ context }) => {
    const page = await context.newPage()

    await page.goto(PEERDB_URL)

    // Open homepage and verify search input exists.
    const searchInput = page.locator("#home-input-search")
    await expect(searchInput).toBeVisible()

    await checkpoint(page, "home-page")
  })
})
