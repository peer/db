import { CLASS_IDS, PROPERTY_IDS } from "../peerdb_utils"
import { expect, expectDocument, expectNoConsoleErrors, expectResults, PEERDB_URL, resultIds, settle, test } from "../utils"

// The search whose results are walked quickly. Individuals are numerous and each of them points at a
// species, a culture and a birthplace, so opening one starts several fetches which a fast next click has to
// abandon cleanly.
const SEARCH_PATH = `/s?${PROPERTY_IDS.INSTANCE_OF}=${CLASS_IDS.INDIVIDUAL}`

// The tabs of a document view, switched between without waiting, so that the fetch each of them starts is
// abandoned by the next.
const TABS = ["allproperties", "history", "permissions", "properties"] as const

test.describe("PeerDB Abandoned Request Flows", () => {
  test("Test walking results faster than they load leaves no error behind", async ({ context }) => {
    const page = await context.newPage()

    await page.goto(`${PEERDB_URL}${SEARCH_PATH}`)
    await expectResults(page)
    const ids = await resultIds(page)
    expect(ids.length, "the search finds results to walk").toBeGreaterThan(3)

    // Three documents are opened one after another without waiting for any of them to finish loading. Each
    // view starts fetching the document, its class, its display label and everything it references, and the
    // next navigation abandons all of it. What is asserted is that nothing of the abandoned work reaches the
    // screen: no error, and no unhandled rejection in the console.
    for (const id of ids.slice(0, 3)) {
      await page.goto(`${PEERDB_URL}/d/${id}`, { waitUntil: "commit" })
    }
    await expectDocument(page)
    await settle(page)
    await expect(page.locator(".pd-withdocument-error"), "nothing was left standing in for an abandoned fetch").toHaveCount(0)
    await expect(page.locator(".pd-displaylabel-error"), "no display label was left in an error state").toHaveCount(0)

    // Going back twice replays the same race in the other direction.
    await page.goBack()
    await page.goBack()
    await expectDocument(page)
    await settle(page)
    await expect(page.locator(".pd-withdocument-error"), "going back leaves nothing standing in for an abandoned fetch").toHaveCount(0)
    await expectNoConsoleErrors(page)

    console.log(`Successfully walked ${ids.slice(0, 3).length} documents faster than they load, and back again, without an error.`)
  })

  test("Test switching document tabs faster than they load leaves no error behind", async ({ context }) => {
    const page = await context.newPage()

    await page.goto(`${PEERDB_URL}${SEARCH_PATH}`)
    await expectResults(page)
    const ids = await resultIds(page)
    await page.goto(`${PEERDB_URL}/d/${ids[0]}`)
    await expectDocument(page)

    // Each tab fetches what it shows, so switching through all of them without waiting abandons every fetch
    // but the last. The last one still has to render.
    for (const tab of TABS) {
      await page.locator(`.pd-documentget-tab-${tab}`).click()
    }
    await expect(page.locator(".pd-documentget-panel-properties"), "the tab which was switched to last").toBeVisible()
    await settle(page)
    await expect(page.locator(".pd-documenthistory-error"), "the history was not left in an error state").toHaveCount(0)
    await expectNoConsoleErrors(page)

    console.log(`Successfully switched through ${TABS.length} document tabs faster than they load, without an error.`)
  })

  test("Test starting a search and navigating away before it lands leaves no error behind", async ({ context }) => {
    const page = await context.newPage()

    await page.goto(`${PEERDB_URL}${SEARCH_PATH}`)
    await expectResults(page)

    // A query is typed into the navbar box and submitted, and the browser is taken home before the results
    // of it arrive, which abandons the search in flight.
    const searchInput = page.locator("#search-input-text")
    await expect(searchInput, "the search box of the navbar").toBeVisible()
    await searchInput.fill("basin")
    await page.locator(".pd-navbarsearch-button").click()
    await page.goto(PEERDB_URL, { waitUntil: "commit" })
    await expect(page.locator("#home-input-search"), "the home view is reached").toBeVisible()
    await settle(page)
    await expectNoConsoleErrors(page)

    console.log("Successfully abandoned a search in flight without an error.")
  })
})
