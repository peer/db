import type { Page } from "@playwright/test"

import { CLASS_IDS, PROPERTY_IDS } from "../peerdb_utils"
import { checkpoint, expect, expectResults, loadAllResults, overrideSiteFeatures, PEERDB_URL, test, volatile } from "../utils"

// The search the navbar is scrolled over. A class search fills the page with results, and every result is
// loaded before the scrolling starts, so the page is long enough for the navbar to hide in and come back out
// of without the feed loading anything while it is being scrolled.
const SEARCH_PATH = `/s?${PROPERTY_IDS.INSTANCE_OF}=${CLASS_IDS.INDIVIDUAL}`

// How far each step scrolls and how many steps are taken in each direction. The navbar hides while the page
// moves down and comes back while it moves up, and it does both gradually, so the steps are small enough for
// a screenshot to catch it part of the way and there are enough of them to reach the end of the movement.
const STEP = 5
const STEPS = 30

// How long the navbar is given to settle after a step. It is moved by a transition, so what a screenshot
// shows right after a scroll is the navbar on its way rather than where it came to rest.
const SETTLE = 500

// Scrolls the page by the given number of pixels and lets the navbar settle where the scroll left it.
async function scrollBy(page: Page, by: number): Promise<void> {
  await page.evaluate((top) => window.scrollBy({ top, behavior: "instant" }), by)
  await page.waitForTimeout(SETTLE)
}

// The top edge of the navbar in the viewport, which is what says how much of it is still shown: 0 while it is
// fully in view and its own negative height once it has hidden itself away.
async function navbarTop(page: Page): Promise<number> {
  const box = await page.locator("#navbar").boundingBox()
  expect(box, "the box of the navbar").not.toBeNull()
  return box!.y
}

test.describe("PeerDB Navbar Scrolling Flows", () => {
  // The auto-hide is an animation, so it is turned off for a visitor who asked for reduced motion, which the
  // rest of the suite runs under. This file is the one which is about the animation itself, so it asks for a
  // browser which did not ask for reduced motion.
  test.use({ contextOptions: { reducedMotion: "no-preference" } })

  test("Test the navbar hides itself while scrolling down and comes back while scrolling up", async ({ context }) => {
    // Sixty steps of scrolling, each settling and screenshotted, take longer than a test of the default
    // length is given, even with the checks which read the whole page left to the first capture.
    test.slow()

    const page = await context.newPage()

    // The site is served to the tests with the navbar in the document flow, which is the one mode this test
    // cannot use: the auto-hide exists only when the navbar is not in the flow. The site's own default, which
    // is the auto-hide, is asked for by dropping the feature.
    await overrideSiteFeatures(page, { navbarPosition: undefined })
    await page.goto(`${PEERDB_URL}${SEARCH_PATH}`)
    await expectResults(page)
    await loadAllResults(page)

    const shown = await navbarTop(page)
    expect(shown, "the navbar starts at the top of the viewport").toBe(0)
    // The first capture is the one which reads the whole page: what the duplicate identifiers and the
    // accessibility scan look at is the same page at every step of the scrolling below, so the steps ask
    // for the screenshot alone.
    await checkpoint(page, "navbar-scrolling-0", { fullPage: false, mask: volatile(page) })

    // Scrolling down takes the navbar away with the page and then leaves it behind, so what the screenshots
    // hold is the navbar moving up out of the viewport step by step.
    for (let step = 1; step <= STEPS; step++) {
      await scrollBy(page, STEP)
      await checkpoint(page, `navbar-scrolling-down-${step}`, { fullPage: false, mask: volatile(page), checks: false })
    }
    const hidden = await navbarTop(page)
    expect(hidden, "the navbar gave up its place at the top while the page was scrolled down").toBeLessThan(shown)

    // Scrolling back up brings it out again, which is what the auto-hide is for: the navbar is there as soon
    // as the visitor moves back towards what they came from, without waiting for the top of the page.
    for (let step = 1; step <= STEPS; step++) {
      await scrollBy(page, -STEP)
      await checkpoint(page, `navbar-scrolling-up-${step}`, { fullPage: false, mask: volatile(page), checks: false })
    }
    const back = await navbarTop(page)
    expect(back, "the navbar came back while the page was scrolled up").toBeGreaterThan(hidden)

    console.log(`Successfully verified that the navbar hides itself over ${STEPS} steps of ${STEP} pixels down and comes back over as many up.`)
  })
})
