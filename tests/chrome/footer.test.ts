import { CLASS_IDS, LANGUAGES, PROPERTY_IDS } from "../peerdb_utils"
import {
  checkpoint,
  checkpointElement,
  expect,
  expectDocument,
  expectResults,
  goHome,
  loadAllResults,
  PEERDB_URL,
  settleFilters,
  switchLanguage,
  test,
  volatile,
} from "../utils"

// The two searches the footer is asserted on. A results page hands the footer to the end of its feed, so it
// is shown only once there is nothing left to load: the short search fits on one page and has its footer
// straight away, while the long one only gets it once every result is loaded.
const SHORT_SEARCH_PATH = `/s?${PROPERTY_IDS.INSTANCE_OF}=${CLASS_IDS.GALAXY}`
const LONG_SEARCH_PATH = `/s?${PROPERTY_IDS.INSTANCE_OF}=${CLASS_IDS.INDIVIDUAL}`

test.describe("PeerDB Footer Flows", () => {
  for (const language of LANGUAGES) {
    test(`Test the footer on the home page in ${language}`, async ({ context }) => {
      const page = await context.newPage()

      await goHome(page)
      await switchLanguage(page, language)

      // The footer is one bar with a list at each end. An application registers what goes into them, and
      // PeerDB itself registers nothing, so what is left is the credit it puts at the end of the trailing
      // list unless the application turns it off.
      //
      // The bar is what is asserted on and what is screenshotted. Its container is laid out with no width
      // of its own (see the comment on pd-footer in Footer.vue), so it counts as hidden and has no box.
      const footer = page.locator(".pd-footer-bar")
      await expect(page.locator(".pd-footer"), "footer").toHaveCount(1)
      await expect(footer, "footer bar").toBeVisible()
      // Both lists are rendered whether or not the application put anything in them, and PeerDB puts
      // nothing in the leading one, so an empty list has no box of its own and counts as hidden.
      await expect(page.locator(".pd-footer-list-start"), "leading list of the footer").toHaveCount(1)
      await expect(page.locator(".pd-footer-list-start > .pd-footer-item"), "PeerDB itself puts nothing in the leading list").toHaveCount(0)
      await expect(page.locator(".pd-footer-list-end"), "trailing list of the footer").toBeVisible()

      const credits = page.locator(".pd-footer-item-credits")
      await expect(credits, "credits of the footer").toBeVisible()
      await expect(credits, "the credits say what the site runs on").not.toHaveText(/^\s*$/)

      const peerdbLink = page.locator(".pd-footer-link-peerdb")
      await expect(peerdbLink, "link to PeerDB").toBeVisible()
      await expect(peerdbLink, "the link is named after the software").toHaveText("PeerDB")
      await expect(peerdbLink, "the link points at the PeerDB repository").toHaveAttribute("href", "https://gitlab.com/peerdb/peerdb")

      await checkpointElement(page, footer, `footer-home-${language}`)
      await checkpoint(page, `footer-home-page-${language}`)

      console.log(`Successfully verified the footer on the home page in ${language}.`)
    })

    test(`Test the footer on a search and on a document in ${language}`, async ({ context }) => {
      const page = await context.newPage()

      await goHome(page)
      await switchLanguage(page, language)

      // A results page hands the footer to the end of its feed rather than sticking it to the bottom of the
      // window, because the feed goes on loading as the visitor scrolls and a footer above what is still to
      // come would be a false end. A search whose results fit on one page has nothing left to load, so its
      // footer is there as soon as the results are.
      await page.goto(`${PEERDB_URL}${SHORT_SEARCH_PATH}`)
      await expectResults(page)
      await expect(page.locator(".pd-footer-bar"), "footer of the results page").toBeVisible()
      await expect(page.locator(".pd-footer-link-peerdb"), "link to PeerDB on the results page").toBeVisible()
      await settleFilters(page)
      await checkpointElement(page, page.locator(".pd-footer-bar"), `footer-search-${language}`)

      // A document view is not a feed, so it carries the footer whatever it shows.
      const firstResult = page.locator(".pd-searchresult-link-title").first()
      await expect(firstResult).toBeVisible()
      await firstResult.click()
      await expectDocument(page)
      await expect(page.locator(".pd-footer-bar"), "footer of the document view").toBeVisible()
      await expect(page.locator(".pd-footer-link-peerdb"), "link to PeerDB on the document view").toBeVisible()
      await checkpointElement(page, page.locator(".pd-footer-bar"), `footer-document-${language}`, { mask: volatile(page) })

      // A search with more results than one page holds gets its footer only once the last of them is loaded.
      await page.goto(`${PEERDB_URL}${LONG_SEARCH_PATH}`)
      await expectResults(page)
      await expect(page.locator(".pd-footer-bar"), "a feed which has more to load shows no footer yet").toHaveCount(0)
      await loadAllResults(page)
      await expect(page.locator(".pd-footer-bar"), "the footer arrives with the last of the results").toBeVisible()

      console.log(`Successfully verified the footer on a search and on a document in ${language}.`)
    })
  }
})
