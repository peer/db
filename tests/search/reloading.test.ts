import type { Page } from "@playwright/test"

import { CLASS_IDS, PROPERTY_IDS, searchByClass } from "../peerdb_utils"
import { expect, filter, LOADING_TIMEOUT, openFilters, PEERDB_URL, settleFilters, test } from "../utils"

// The class a facet carries while it loads values it has loaded once before (pd-data-reloading, applied from
// the laterLoad of useInitialLoad in src/utils.ts). The facet goes on rendering the values of the search it
// answered last and the class greys them out, which is what says the counts in front of the reader belong to
// the search before this one. It is a state the rest of the suite waits out rather than looks at, so these
// tests hold the requests the facets make and read the panel while it is held.
const RELOADING = ".pd-data-reloading"

// The requests a facet makes for its values. The panel first asks which facets the search has (the bare
// "/api/s/filters/<session>") and then each facet asks for its own values under that path, so the pattern
// takes only the latter: holding the list as well would leave the panel with no facets to render at all.
const FACET_VALUES = /\/api\/s\/filters\/[^/]+\/.+/

// The class searched. It is small enough for the panel to hold a handful of facets rather than every facet
// the catalogue has, so what is held is a few requests and not a few hundred.
const SEARCH_CLASS = "MOON" as const

// Holds every request a facet makes for its values until the returned function is called. The facets are then
// stuck in whatever load they are in, which is what makes the class readable: it is asserted against a panel
// which cannot finish, instead of being raced for in a panel which can.
//
// Releasing leaves the route in place rather than taking it off. A held request is resumed by the handler it
// is parked in, and taking the route off resumes it too, so doing both makes the handler continue a request
// which has already been continued. With the route left in place every later request passes straight through
// the resolved promise instead.
async function holdFacetValues(page: Page): Promise<() => void> {
  let release: (() => void) | null = null
  const held = new Promise<void>((resolve) => {
    release = resolve
  })
  await page.route(FACET_VALUES, async (route) => {
    await held
    // The page may be gone by the time a held request is resumed, which is not a failure of what is tested.
    await route.continue().catch(() => null)
  })
  return () => release!()
}

// Waits until the panel has facets on it, which is what makes an assertion that none of them is marked mean
// something rather than being true of an empty panel.
async function expectFacetsRendered(page: Page): Promise<number> {
  const facets = page.locator(".pd-filtersresult")
  await expect(facets.first(), "the panel renders its facets").toBeVisible({ timeout: LOADING_TIMEOUT })
  const shown = await facets.count()
  expect(shown, "the panel renders facets to read the mark off").toBeGreaterThan(0)
  return shown
}

test.describe("PeerDB Filter Reloading Flows", () => {
  test("Test the reloading mark is not applied while the values of a facet load for the first time", async ({ context }) => {
    const page = await context.newPage()

    // The requests are held before the search is opened, so every facet of the panel is stuck in the load it
    // does when it first appears. That load is the one the mark is not for: there are no values on the page
    // to grey out, and a facet which greyed itself out here would be greying out its own emptiness.
    const release = await holdFacetValues(page)
    try {
      // The search is opened without the usual helper. Held requests never let the network go quiet, and that
      // helper ends by waiting for exactly that, so the page is waited for by what it renders instead.
      await page.goto(`${PEERDB_URL}/s?${PROPERTY_IDS.INSTANCE_OF}=${CLASS_IDS[SEARCH_CLASS]}`)
      await expect(page.locator(".pd-searchresultsheader"), "the header of the results").toBeVisible({ timeout: LOADING_TIMEOUT })
      await openFilters(page)

      const shown = await expectFacetsRendered(page)
      await expect(page.locator(RELOADING), "no facet is marked as reloading while it loads for the first time").toHaveCount(0)

      console.log(`Successfully verified that none of the ${shown} facets loading for the first time is marked as reloading.`)
    } finally {
      release()
    }

    // Once the held requests are let through the panel finishes its first load, still with nothing marked.
    await settleFilters(page)
    await expect(page.locator(RELOADING), "no facet is marked as reloading once the first load is done").toHaveCount(0)
  })

  test("Test the reloading mark is applied while the values of a facet load again", async ({ context }) => {
    const page = await context.newPage()

    // The panel is let load once in full, so every facet has values on the page and a later load is what
    // comes next.
    await searchByClass(page, SEARCH_CLASS)
    await openFilters(page)
    await settleFilters(page)
    await expect(page.locator(RELOADING), "nothing is marked before the search changes").toHaveCount(0)

    const contactStatus = filter(page, "ref", PROPERTY_IDS.HAS_CONTACT_STATUS)
    await expect(contactStatus, "the facet on the contact status of a world").toBeVisible()
    const value = contactStatus.locator(".pd-reffiltertreerow-checkbox").first()
    await expect(value, "a value of the facet to select").toBeVisible()
    await expect(value, "a value of the facet which is not locked").toBeEnabled({ timeout: LOADING_TIMEOUT })

    // The requests are held first, so the facets cannot finish the load which selecting a value starts and the
    // mark stays on the page to be read rather than having to be caught as it passes.
    const release = await holdFacetValues(page)
    try {
      await value.click()

      const marked = page.locator(RELOADING)
      await expect(marked.first(), "a facet is marked as reloading while it loads again").toBeVisible({ timeout: LOADING_TIMEOUT })
      const count = await marked.count()

      // The marked facet is still the one it was: it renders the values of the search before this one rather
      // than emptying itself while it waits.
      await expect(marked.first().locator(".pd-filtersresult-title"), "a reloading facet still names its property").toBeVisible()

      console.log(`Successfully verified that ${count} facets are marked as reloading while they load again.`)
    } finally {
      release()
    }

    // The mark is taken off once the values it was waiting for arrive.
    await settleFilters(page)
    await expect(page.locator(RELOADING), "nothing is marked once the later load is done").toHaveCount(0)
  })

  test("Test the reloading mark is gone once the filters panel has settled", async ({ context }) => {
    const page = await context.newPage()

    // What the two tests above assert while the panel is held is asserted here against a panel which was left
    // to finish by itself, which is the state every other test in the suite reads the panel in: a facet left
    // marked would show the whole suite greyed-out counts and nothing would say so.
    await searchByClass(page, SEARCH_CLASS)
    await openFilters(page)
    await settleFilters(page)
    const shown = await expectFacetsRendered(page)
    await expect(page.locator(RELOADING), "nothing is marked once the panel has settled").toHaveCount(0)

    // A search which changes and then settles ends in the same state, so the mark is tied to a load being in
    // flight rather than to the search having ever changed.
    const contactStatus = filter(page, "ref", PROPERTY_IDS.HAS_CONTACT_STATUS)
    await expect(contactStatus, "the facet on the contact status of a world").toBeVisible()
    const value = contactStatus.locator(".pd-reffiltertreerow-checkbox").first()
    await expect(value, "a value of the facet which is not locked").toBeEnabled({ timeout: LOADING_TIMEOUT })
    await value.click()
    await settleFilters(page)
    await expect(page.locator(RELOADING), "nothing is marked once the changed search has settled").toHaveCount(0)

    console.log(`Successfully verified that none of the ${shown} facets of a settled panel is marked as reloading.`)
  })
})
