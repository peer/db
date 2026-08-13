import { CLASS_IDS, PROPERTY_IDS } from "../peerdb_utils"
import { checkpoint, expect, expectResults, goHome, openFilters, overrideSiteFeatures, PEERDB_URL, settleFilters, test, volatile } from "../utils"

// The widths the chrome is looked at in. The navbar folds its search box away on a narrow screen and lays it
// out inline on a wide one, so both sides of that fold are exercised. The third is the viewport the rest of
// the suite runs at (playwright.config.ts), which lies between them: what the navbar does there is what every
// other screenshot in the suite is taken of, and neither of the other two widths says anything about it.
const NARROW = { width: 375, height: 720 }
const DEFAULT = { width: 1280, height: 720 }
const WIDE = { width: 1440, height: 900 }

// The search the layout tests are run over. A class search fills the page with results and gives the filters
// column something to hold, which is what the sticky header and the skip links are for.
const SEARCH_PATH = `/s?${PROPERTY_IDS.INSTANCE_OF}=${CLASS_IDS.INDIVIDUAL}`

test.describe("PeerDB Layout Flows", () => {
  test("Test the home page puts the caret in the search box", async ({ context }) => {
    const page = await context.newPage()

    await goHome(page)

    // The home view is a search box and nothing else, so it takes the caret itself: a visitor arrives able
    // to type. Losing this is not visible in a screenshot, which is why it is asserted.
    await expect.poll(async () => await page.evaluate(() => document.activeElement?.id), { message: "the home view focuses its search box" }).toBe("home-input-search")

    // Typing and pressing Enter submits the form the box is in, which starts a search session.
    await page.keyboard.type("weir")
    await expect(page.locator("#home-input-search"), "the search box takes what is typed").toHaveValue("weir")
    await page.keyboard.press("Enter")
    await expectResults(page)
    expect(new URL(page.url()).pathname, "pressing enter in the search box starts a search session").toMatch(/^\/s\/[0-9A-Za-z]+$/)

    console.log("Successfully verified that the home page focuses its search box and that enter submits it.")
  })

  test("Test the navbar folds its search box away on a narrow screen", async ({ context }) => {
    const page = await context.newPage()

    // Wide, the search box sits in the navbar next to its button.
    await page.setViewportSize(WIDE)
    await page.goto(`${PEERDB_URL}${SEARCH_PATH}`)
    await expectResults(page)
    const searchInput = page.locator("#search-input-text")
    await expect(searchInput, "the search box of a wide navbar").toBeVisible()
    const wideBox = await searchInput.boundingBox()
    expect(wideBox?.width ?? 0, "the search box of a wide navbar has room to type in").toBeGreaterThan(100)
    await settleFilters(page)
    await checkpoint(page, "layout-navbar-wide", { mask: volatile(page) })

    // At the width the suite runs at the box is still inline and still usable. It is what a long site title
    // takes the room for if the title does not give way, and the box is then squeezed to nothing rather than
    // folded away: it stays in the navbar, reports itself as hidden and cannot be typed into.
    await page.setViewportSize(DEFAULT)
    await expect(searchInput, "the search box at the width the suite runs at").toBeVisible()
    const defaultBox = await searchInput.boundingBox()
    expect(defaultBox?.width ?? 0, "the search box at the width the suite runs at has room to type in").toBeGreaterThan(100)

    // Narrow, the box folds away and the button stands in for it, so the navbar still fits.
    await page.setViewportSize(NARROW)
    await expect(page.locator(".pd-navbarsearch-button"), "the search button of a narrow navbar").toBeVisible()
    await expect
      .poll(async () => (await searchInput.boundingBox())?.width ?? 0, { message: "the search box folds away on a narrow screen" })
      .toBeLessThan(wideBox?.width ?? 0)
    await settleFilters(page)
    await checkpoint(page, "layout-navbar-narrow", { mask: volatile(page) })

    console.log(
      `Successfully verified that the navbar search box is ${Math.round(wideBox?.width ?? 0)} pixels wide at ${WIDE.width}, ${Math.round(defaultBox?.width ?? 0)} pixels wide at ${DEFAULT.width} and folds away at ${NARROW.width}.`,
    )
  })

  test("Test the navbar stays put while scrolling under reduced motion", async ({ context }) => {
    const page = await context.newPage()

    // The navbar hides itself as the page is scrolled down, which is an animation, so it is turned off for a
    // visitor who asked for reduced motion: the navbar then simply stays at the top of the viewport. The whole
    // suite runs under reduced motion, which is the state asserted here.
    //
    // The site is served to the tests with the navbar in the document flow instead, so that a full page
    // screenshot does not depend on where the page happened to be scrolled, and that is the one thing this
    // test cannot have: a navbar in the flow scrolls away with everything else. It is asked for at the
    // viewport top for this test alone.
    await overrideSiteFeatures(page, { navbarPosition: "fixed" })
    await page.goto(`${PEERDB_URL}${SEARCH_PATH}`)
    await expectResults(page)

    const navbar = page.locator("#navbar")
    await expect(navbar, "navbar").toBeVisible()
    const before = await navbar.boundingBox()

    await page.evaluate(() => window.scrollBy({ top: 2000, behavior: "instant" }))
    await expect(navbar, "the navbar is still shown after scrolling").toBeVisible()
    const after = await navbar.boundingBox()
    expect(after?.y, "the navbar does not move while scrolling").toBe(before?.y)

    // The results header sticks below the navbar, so the count and the toolbar stay reachable however far
    // down the feed the visitor is.
    await expect(page.locator(".pd-searchresultsheader"), "the results header stays with the navbar").toBeVisible()

    console.log("Successfully verified that the navbar and the results header stay put while scrolling.")
  })

  test("Test the skip links reach the filters and the results", async ({ context }) => {
    const page = await context.newPage()

    await page.goto(`${PEERDB_URL}${SEARCH_PATH}`)
    await expectResults(page)

    // The first two things the keyboard reaches on a results page are the links which jump over the chrome,
    // straight to the filters and to the results. They are hidden until they are focused.
    const toFilters = page.locator(".pd-searchresultsfeed-link-skipfilters")
    const toResults = page.locator(".pd-searchresultsfeed-link-skipresults")
    await expect(toFilters, "skip to filters link").toHaveCount(1)
    await expect(toResults, "skip to results link").toHaveCount(1)

    await page.evaluate(() => document.body.scrollIntoView({ block: "start", behavior: "instant" }))
    await toFilters.focus()
    await expect(toFilters, "the skip link shows itself once it is focused").toBeVisible()
    await checkpoint(page, "layout-skip-links", { mask: volatile(page) })
    await page.keyboard.press("Enter")
    await expect(page.locator("#search-filters"), "the filters column the skip link leads to").toBeVisible()

    await toResults.focus()
    await expect(toResults, "the second skip link shows itself once it is focused").toBeVisible()
    await page.keyboard.press("Enter")
    await expect(page.locator("#search-results"), "the results column the skip link leads to").toBeVisible()

    console.log("Successfully verified that the skip links reach the filters column and the results column.")
  })

  test("Test the keyboard walks the filter values without stopping at their links", async ({ context }) => {
    const page = await context.newPage()

    await page.goto(`${PEERDB_URL}${SEARCH_PATH}`)
    await expectResults(page)
    await openFilters(page)

    // Every value of a facet carries a link to the document it stands for, next to its checkbox. The link is
    // kept out of the tab order, so walking a facet with the keyboard goes from one value to the next
    // instead of stopping twice per value.
    const checkboxes = page.locator(".pd-reffiltertreerow-checkbox")
    await expect(checkboxes.first(), "the first value of a facet").toBeVisible()
    await checkboxes.first().focus()

    for (let step = 0; step < 4; step++) {
      await page.keyboard.press("Tab")
      const focused = await page.evaluate(() => {
        const el = document.activeElement
        return el ? [...el.classList].filter((c) => c.startsWith("pd-")).join(" ") : ""
      })
      expect(focused, `what the keyboard reached after ${step + 1} steps`).not.toContain("pd-reffiltertreerow-link")
    }

    console.log("Successfully verified that the keyboard walks the values of a facet without stopping at their links.")
  })
})
