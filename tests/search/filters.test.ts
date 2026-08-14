import type { Locator, Page } from "@playwright/test"

import { CLASS_IDS, documentIdOf, PROPERTY_IDS, searchByClass } from "../peerdb_utils"
import {
  applyFilterValue,
  checkpoint,
  checkpointElement,
  expandFilter,
  expect,
  expectNoResults,
  filter,
  filterValue,
  hasValue,
  LOADING_TIMEOUT,
  openFilters,
  resultCount,
  searchAgain,
  settleFilters,
  test,
  volatile,
} from "../utils"

// The values the tests select. The contact status is a vocabulary entry documents of several classes are
// classified with, which is what a test combining a reference filter with a class filter needs, and the
// galaxy is the top of a containment chain, which is what a test about a facet whose values stand over
// other values needs.
const VALUE_NO_CONTACT = await documentIdOf("CONTACT_STATUS", "NO_CONTACT")
const VALUE_GALAXY = await documentIdOf("GALAXY", "G3_HOLLOW_BAR")
const VALUE_SECTOR = await documentIdOf("SECTOR", "G3_BELLWETHER")

// How many facets the panel shows before "more filters" is used, how many values a facet shows before its
// own "more" is used, and how many either of them adds. They mirror FILTERS_INITIAL_LIMIT and
// FILTERS_INCREASE in PeerDB.
const SHOWN = 10
const INCREASE = 10

// A query no document of the test data matches, used to drive the search to a state where a filter is
// active and nothing is left for it to filter.
const NO_MATCH_QUERY = "zzzznosuchthing"

// A term no facet and no value of the test data matches, used to empty the filters panel through its own
// search box.
const NO_MATCH_TERM = "qqqqnosuchfacet"

// Opens the search over the worlds of the catalogue with its filters panel showing. Every test here starts
// from the same search and only what happens in the filters panel afterwards is of interest, so the search
// is opened by its address rather than run from the home page. Worlds are what it is scoped to because that
// keeps the panel to a few dozen facets while still offering a facet of every kind: the classes the worlds
// fall into, the vocabularies they are classified with, the chain of places containing them, the amounts
// measured of them and the properties some of them merely state they have.
async function openSearch(page: Page): Promise<void> {
  await searchByClass(page, "WORLD")
  await openFilters(page)
  await expect(page.locator(".pd-filtersresult"), "the panel shows its first batch of facets").toHaveCount(SHOWN)
}

// The count a facet's row promises, read out of the parentheses the row renders it in, so a test can assert
// that selecting the row finds exactly as many documents as the row said it would. The count is grouped for
// the locale, so only its digits are read.
async function rowCount(count: Locator, what: string): Promise<number> {
  await expect(count, `the count of ${what}`).toBeVisible()
  const digits = ((await count.textContent()) || "").replace(/\D/g, "")
  expect(digits, `the count of ${what} is a number`).not.toBe("")
  return Number(digits)
}

// The count next to one value of a reference facet. The count is rendered as a label of the value's own
// checkbox, so it is addressed by the same id the checkbox carries.
function refCount(page: Page, props: Array<string>, value: string): Locator {
  return page.locator(`label[for="${["ref", ...props, value].join("/")}"].pd-reffiltertreerow-count`)
}

// Selects the vocabulary entry the reference filter tests filter on.
async function applyReferenceFilter(page: Page): Promise<Locator> {
  const contactStatus = filter(page, "ref", PROPERTY_IDS.HAS_CONTACT_STATUS)
  await expect(contactStatus, "the facet on the contact status").toBeVisible()
  await applyFilterValue(page, contactStatus, filterValue(page, "ref", [PROPERTY_IDS.HAS_CONTACT_STATUS], VALUE_NO_CONTACT))
  return contactStatus
}

// Selects planets in the facet on "instance of", which is the class filter.
async function applyClassFilter(page: Page): Promise<Locator> {
  const instanceOf = filter(page, "ref", PROPERTY_IDS.INSTANCE_OF)
  await expect(instanceOf, "the class facet").toBeVisible()
  await applyFilterValue(page, instanceOf, filterValue(page, "ref", [PROPERTY_IDS.INSTANCE_OF], CLASS_IDS.PLANET))
  return instanceOf
}

// Asks the filters panel for another batch of facets.
//
// The press is dispatched rather than clicked, because the panel replaces the button while it re-renders
// (it keeps its column filled and reveals the next batch by itself, see fillColumn in
// SearchResultsFeed.vue), and a click waits for an element which is being taken away from under it.
async function pressMoreFilters(page: Page): Promise<void> {
  const moreFilters = page.locator(".pd-searchresultsfeed-button-morefilters")
  await expect(moreFilters, "the button which shows more facets").toBeVisible()
  await moreFilters.dispatchEvent("click")
}

// Shows a facet which sits below the ones the panel shows at first: the panel is settled, which adds every
// facet it has and waits for its list to stop being replaced, and the facet is then asserted to be among
// them.
//
// Asking for batches until the facet appears is not enough on its own. The panel starts its batches over
// whenever a new list of facets arrives, so a facet revealed just before one lands is taken away again, and
// what is then waited for is a value of a facet which is no longer on the page.
async function showFilter(page: Page, facet: Locator, what: string): Promise<void> {
  await settleFilters(page)
  await expect(facet, what).toBeVisible({ timeout: LOADING_TIMEOUT })
}

// Selects the property the has filter tests filter on: documents which state that they have a ring system
// without stating anything more about it. The has facet sits below the facets shown at first, so the rest
// of the facets have to be shown before it can be used.
async function applyHasFilter(page: Page): Promise<Locator> {
  const hasFacet = filter(page, "has")
  await showFilter(page, hasFacet, "the has facet on the document itself")
  await applyFilterValue(page, hasFacet, hasValue(page, PROPERTY_IDS.HAS_RING_SYSTEM))
  return hasFacet
}

// Clears one filter through its own clear button, the way a user drops a single filter while keeping the
// others.
async function clearFilter(page: Page, applied: Locator): Promise<void> {
  const clearButton = applied.locator(".pd-filtersresult-button-clear")
  await expect(clearButton, "the clear button of the applied filter").toBeVisible()
  await searchAgain(page, async () => {
    await clearButton.click()
    await expect(clearButton, "the cleared facet no longer offers to be cleared").toHaveCount(0)
  })
}

// Clears every active filter. Active filters are sorted to the top of the panel, so they are all reachable
// at once, and each one is dropped by its own clear button until none is left.
async function clearAllFilters(page: Page): Promise<void> {
  const clearButtons = page.locator(".pd-filtersresult-button-clear")
  for (let remaining = await clearButtons.count(); remaining > 0; remaining--) {
    const clearButton = clearButtons.first()
    await expect(clearButton, "the clear button of the next active filter").toBeVisible()
    await searchAgain(page, async () => {
      await clearButton.click()
      await expect(clearButtons, "one filter fewer is active").toHaveCount(remaining - 1)
    })
  }
}

test.describe("PeerDB Search Filters Flows", () => {
  test("Test the filters panel lists the available filters", async ({ context }) => {
    const page = await context.newPage()

    await openSearch(page)

    // The panel offers more filters than it shows, so it counts them, gives a box to search them and a
    // button to show the next batch.
    await expect(page.locator(".pd-searchresultsfeed-count-filters"), "the panel counts the filters it offers").toBeVisible()
    await expect(page.locator(".pd-searchresultsfeed-input-filters"), "the box which searches the filters").toBeVisible()
    await expect(page.locator(".pd-searchresultsfeed-button-morefilters"), "the button which shows more facets").toBeVisible()

    // Every facet is headed by the property path it filters on and holds a list of what it offers.
    await expect(page.locator(".pd-filtersresult-title"), "every facet names the property it filters on").toHaveCount(SHOWN)
    await expect(page.locator(".pd-filtersresult-list"), "every facet lists what it offers").toHaveCount(SHOWN)

    // The facets are of every kind the data calls for: values to tick, amounts and moments in time to bound,
    // and the properties which are stated without a value at all.
    await expect(filter(page, "ref", PROPERTY_IDS.INSTANCE_OF), "the class facet").toBeVisible()
    await expect(filter(page, "ref", PROPERTY_IDS.HAS_CONTACT_STATUS), "a facet on a vocabulary").toBeVisible()
    await expect(filter(page, "amount", PROPERTY_IDS.RADIUS), "a facet on an amount").toBeVisible()
    await showFilter(page, filter(page, "time", PROPERTY_IDS.FIRST_SURVEYED), "a facet on a moment in time")
    await showFilter(page, filter(page, "has"), "the has facet on the document itself")

    // Nothing is filtered yet, so no facet offers to be cleared and the panel is not empty either.
    await expect(page.locator(".pd-filtersresult-button-clear"), "no facet offers to be cleared").toHaveCount(0)
    await expect(page.locator(".pd-searchresultsfeed-empty-filters"), "the panel does not report that it has no filters").toHaveCount(0)
    await expect(page.locator(".pd-searchresultsfeed-empty-nomatch"), "the panel does not report that nothing matched").toHaveCount(0)

    await settleFilters(page)
    const facets = await page.locator(".pd-filtersresult").count()
    await checkpoint(page, "filters-panel-initial", { mask: volatile(page) })

    console.log(`Successfully opened the filters panel of a search of ${await resultCount(page)} documents and read its ${facets} facets.`)
  })

  test("Test applying a single reference filter", async ({ context }) => {
    const page = await context.newPage()

    await openSearch(page)
    const unfiltered = await resultCount(page)

    // What the row promises is what the search has to find once the row is selected.
    const promised = await rowCount(refCount(page, [PROPERTY_IDS.HAS_CONTACT_STATUS], VALUE_NO_CONTACT), "the selected value of the contact status facet")
    const contactStatus = await applyReferenceFilter(page)

    await expect.poll(() => resultCount(page), { message: "the filtered search finds what the facet promised" }).toBe(promised)
    expect(promised, "filtering leaves fewer documents than the unfiltered search").toBeLessThan(unfiltered)
    await expect(page.locator(".pd-filtersresult-button-clear"), "exactly one filter is active").toHaveCount(1)
    await expect(filterValue(page, "ref", [PROPERTY_IDS.HAS_CONTACT_STATUS], VALUE_NO_CONTACT), "the selected value is checked").toBeChecked()
    await checkpointElement(page, contactStatus, "filters-single-applied-facet")
    await settleFilters(page)
    await checkpoint(page, "filters-single-applied", { mask: volatile(page) })

    console.log(`Successfully applied a single reference filter and narrowed ${unfiltered} documents to ${promised}.`)
  })

  test("Test clearing a single reference filter", async ({ context }) => {
    const page = await context.newPage()

    await openSearch(page)
    const unfiltered = await resultCount(page)

    const contactStatus = await applyReferenceFilter(page)
    const filtered = await resultCount(page)
    await clearFilter(page, contactStatus)

    // Dropping the only filter puts the search back where it started.
    await expect(page.locator(".pd-filtersresult-button-clear"), "no filter is active any more").toHaveCount(0)
    await expect(filterValue(page, "ref", [PROPERTY_IDS.HAS_CONTACT_STATUS], VALUE_NO_CONTACT), "the value is not checked any more").not.toBeChecked()
    await expect.poll(() => resultCount(page), { message: "the search is back to what it found unfiltered" }).toBe(unfiltered)
    await settleFilters(page)
    await checkpoint(page, "filters-single-cleared", { mask: volatile(page) })

    console.log(`Successfully cleared a single reference filter and went from ${filtered} documents back to ${unfiltered}.`)
  })

  test("Test combining a reference filter with a class filter", async ({ context }) => {
    const page = await context.newPage()

    await openSearch(page)
    const unfiltered = await resultCount(page)

    await applyReferenceFilter(page)
    const afterReference = await resultCount(page)
    expect(afterReference, "the reference filter narrows the search").toBeLessThan(unfiltered)

    // The class facet counts the documents the reference filter left, so what it promises for planets is
    // what the two filters together have to find.
    const promised = await rowCount(refCount(page, [PROPERTY_IDS.INSTANCE_OF], CLASS_IDS.PLANET), "planets in the class facet of the filtered search")
    await applyClassFilter(page)

    await expect(page.locator(".pd-filtersresult-button-clear"), "two filters are active").toHaveCount(2)
    await expect.poll(() => resultCount(page), { message: "the two filters together find what the class facet promised" }).toBe(promised)
    expect(promised, "the class filter narrows the search further").toBeLessThan(afterReference)
    await settleFilters(page)
    await checkpoint(page, "filters-two-applied", { mask: volatile(page) })

    console.log(`Successfully combined a reference filter with a class filter and narrowed ${unfiltered} documents through ${afterReference} to ${promised}.`)
  })

  test("Test clearing one of two filters individually", async ({ context }) => {
    const page = await context.newPage()

    await openSearch(page)

    const contactStatus = await applyReferenceFilter(page)
    const afterReference = await resultCount(page)
    const instanceOf = await applyClassFilter(page)
    const afterClass = await resultCount(page)

    await clearFilter(page, instanceOf)

    // Clearing one filter leaves the other one applied and the search where that one alone put it.
    await expect(page.locator(".pd-filtersresult-button-clear"), "one filter is left active").toHaveCount(1)
    await expect(contactStatus.locator(".pd-filtersresult-button-clear"), "the filter which was not cleared is the one left active").toBeVisible()
    await expect.poll(() => resultCount(page), { message: "the search is back where the remaining filter alone put it" }).toBe(afterReference)
    await settleFilters(page)
    await checkpoint(page, "filters-two-class-cleared", { mask: volatile(page) })

    console.log(`Successfully cleared one of two filters, going from ${afterClass} documents back to ${afterReference}, and kept the other.`)
  })

  test("Test combining a reference, a class and a has filter", async ({ context }) => {
    const page = await context.newPage()

    await openSearch(page)

    await applyReferenceFilter(page)
    await applyClassFilter(page)
    const afterClass = await resultCount(page)

    // A has filter keeps the documents which state the property without stating a value for it.
    const hasFacet = await applyHasFilter(page)

    await expect(page.locator(".pd-filtersresult-button-clear"), "three filters are active").toHaveCount(3)
    const afterHas = await resultCount(page)
    expect(afterHas, "the has filter narrows the search further").toBeLessThan(afterClass)
    expect(afterHas, "the three filters together still find something").toBeGreaterThan(0)
    await checkpointElement(page, hasFacet, "filters-three-applied-facet")
    await settleFilters(page)
    await checkpoint(page, "filters-three-applied", { mask: volatile(page) })

    console.log(`Successfully combined a reference, a class and a has filter and narrowed ${afterClass} documents to ${afterHas}.`)
  })

  test("Test clearing all filters at once", async ({ context }) => {
    const page = await context.newPage()

    await openSearch(page)
    const unfiltered = await resultCount(page)

    await applyReferenceFilter(page)
    await applyClassFilter(page)
    await applyHasFilter(page)
    await expect(page.locator(".pd-filtersresult-button-clear"), "three filters are active").toHaveCount(3)
    const filtered = await resultCount(page)

    await clearAllFilters(page)

    // With every filter gone the search is back to matching every document it started with.
    await expect(page.locator(".pd-filtersresult-button-clear"), "no filter is active any more").toHaveCount(0)
    await expect.poll(() => resultCount(page), { message: "the search is back to what it found unfiltered" }).toBe(unfiltered)
    await settleFilters(page)
    await checkpoint(page, "filters-all-cleared", { mask: volatile(page) })

    console.log(`Successfully cleared three filters of different kinds at once and went from ${filtered} documents back to ${unfiltered}.`)
  })

  test("Test showing more values of a filter", async ({ context }) => {
    const page = await context.newPage()

    await openSearch(page)

    // The facet on the classifying property has more values than it shows, so it offers to show the next
    // batch of them.
    const classifiedAs = filter(page, "ref", PROPERTY_IDS.CLASSIFIED_AS)
    await expect(classifiedAs, "the facet on the classifying property").toBeVisible()
    const values = classifiedAs.locator(".pd-reffiltertreerow-checkbox")
    await expect(values, "the facet shows its first batch of values").toHaveCount(SHOWN)
    await expect(classifiedAs.locator(".pd-filtersresult-more"), "the facet offers to show more values").toBeVisible()

    await expandFilter(page, classifiedAs)
    await expect(values, "the facet shows one batch of values more").toHaveCount(SHOWN + INCREASE)
    await page.waitForLoadState("networkidle")
    await checkpointElement(page, classifiedAs, "filters-values-expanded-facet")

    // How many values a facet shows is state of the view alone and there is no control to fold them back,
    // so the list collapses by loading the search again.
    await openSearch(page)
    await expect(classifiedAs.locator(".pd-reffiltertreerow-checkbox"), "the facet is back to its first batch of values").toHaveCount(SHOWN)
    await checkpointElement(page, classifiedAs, "filters-values-collapsed-again-facet")

    console.log(`Successfully showed ${SHOWN + INCREASE} values of a filter and collapsed them back to ${SHOWN}.`)
  })

  test("Test showing every value of a filter", async ({ context }) => {
    const page = await context.newPage()

    await openSearch(page)

    // A facet whose values run out asks for them until there is nothing left to ask for, and then says
    // nothing about values it is not showing: the count of what is left has to reach zero rather than
    // linger next to a list which is already complete.
    const hostsCulture = filter(page, "ref", PROPERTY_IDS.HOSTS_CULTURE)
    await showFilter(page, hostsCulture, "the facet on the culture a world hosts")
    const more = hostsCulture.locator(".pd-filtersresult-more")
    await expect(more, "the facet offers to show more values").toBeVisible()
    for (let batch = 0; batch < 30 && (await more.isVisible().catch(() => false)); batch++) {
      await expandFilter(page, hostsCulture)
    }

    await expect(more, "the facet has no more values to show").toHaveCount(0)
    await expect(hostsCulture.locator(".pd-filtersresult-text-notshown"), "the facet reports no values it is not showing").toHaveCount(0)

    // Every row which is left is a value to select. A row standing in for values the server never sent
    // renders no checkbox, so with none of those left every row has one.
    const rows = hostsCulture.locator(".pd-reffiltertreerow")
    const shown = await rows.count()
    expect(shown, "the facet shows more values than one batch").toBeGreaterThan(SHOWN)
    await expect(hostsCulture.locator(".pd-reffiltertreerow-checkbox"), "every value the facet shows can be selected").toHaveCount(shown)
    await expect(hostsCulture.locator(".pd-reffiltertreerow-text-notshown"), "no row stands in for values which were not sent").toHaveCount(0)

    console.log(`Successfully showed every one of the ${shown} values of a filter.`)
  })

  test("Test selecting a value which stands over other values", async ({ context }) => {
    const page = await context.newPage()

    await openSearch(page)

    // The facet on the containment chain renders the places as the tree they form, so selecting a place
    // selects everything under it and finds the documents contained anywhere in it.
    const containedIn = filter(page, "ref", PROPERTY_IDS.CONTAINED_IN)
    await expect(containedIn, "the facet on the containment chain").toBeVisible()
    const galaxyRow = filterValue(page, "ref", [PROPERTY_IDS.CONTAINED_IN], VALUE_GALAXY)
    const sectorRow = filterValue(page, "ref", [PROPERTY_IDS.CONTAINED_IN], VALUE_SECTOR)
    await expect(galaxyRow, "the place the others are under").toBeVisible()
    await expect(sectorRow, "a place under it").toBeVisible()
    await expect(galaxyRow, "nothing is selected yet").not.toBeChecked()
    await expect(sectorRow, "nothing is selected yet").not.toBeChecked()

    const promised = await rowCount(refCount(page, [PROPERTY_IDS.CONTAINED_IN], VALUE_GALAXY), "the place which stands over the others")
    await applyFilterValue(page, containedIn, galaxyRow)

    // Selecting the place selected everything under it, and the search found what the place promised.
    await expect(galaxyRow, "the selected place is checked").toBeChecked()
    await expect(sectorRow, "the places under the selected one are checked too").toBeChecked()
    await expect.poll(() => resultCount(page), { message: "the search finds what the selected place promised" }).toBe(promised)

    // A selected value is sorted to the top of its facet, so the facet opens on what is selected rather
    // than on wherever the value happened to rank.
    await expect(containedIn.locator(".pd-reffiltertreerow-checkbox").first(), "the selected value is the first the facet shows").toBeChecked()

    await checkpointElement(page, containedIn, "filters-subtree-selected-facet")
    await settleFilters(page)
    await checkpoint(page, "filters-subtree-selected", { mask: volatile(page) })

    console.log(`Successfully selected a place which stands over others and found the ${promised} documents contained in it.`)
  })

  test("Test showing more filters", async ({ context }) => {
    const page = await context.newPage()

    await openSearch(page)

    const facets = page.locator(".pd-filtersresult")
    await pressMoreFilters(page)
    // Asking for more facets adds at least one batch of them. It can add more than one: the panel keeps its
    // column filled to about two viewports (fillColumn in SearchResultsFeed.vue), so revealing a batch which
    // does not reach that far makes it reveal the next one by itself, and how many that is follows from how
    // tall the revealed facets are.
    await expect(facets, "the panel is no longer showing only its first batch").not.toHaveCount(SHOWN)
    await expect.poll(async () => await facets.count(), { message: "the panel adds at least one batch of facets" }).toBeGreaterThanOrEqual(SHOWN + INCREASE)
    await page.waitForLoadState("networkidle")
    await settleFilters(page)
    await checkpoint(page, "filters-facets-expanded", { mask: volatile(page) })

    // As with the values of a single facet, how many facets are shown is state of the view alone, so the
    // panel collapses by loading the search again.
    await openSearch(page)
    await expect(facets, "the panel is back to its first batch of facets").toHaveCount(SHOWN)

    console.log("Successfully showed more filters and collapsed them again.")
  })

  test("Test a filter which is left with nothing to filter", async ({ context }) => {
    const page = await context.newPage()

    await openSearch(page)

    const instanceOf = await applyClassFilter(page)
    const filtered = await resultCount(page)
    expect(filtered, "the class filter finds documents before the query is narrowed").toBeGreaterThan(0)

    // Narrowing the query until nothing matches leaves the filter active with nothing to filter. The panel
    // has to keep showing it anyway, because a filter which cannot be seen cannot be dropped.
    const searchInput = page.locator("#search-input-text")
    await expect(searchInput, "the search box of the results page").toBeVisible()
    await searchInput.fill(NO_MATCH_QUERY)
    await searchInput.press("Enter")
    await expectNoResults(page)

    await expect(instanceOf, "the active facet is still shown").toBeVisible({ timeout: LOADING_TIMEOUT })
    await expect(instanceOf.locator(".pd-filtersresult-button-clear"), "the active filter can still be cleared").toBeVisible()
    await expect(page.locator(".pd-searchresultsfeed-empty-filters"), "the panel does not replace the active filter with a message").toHaveCount(0)
    await expect(page.locator(".pd-searchresultsfeed-empty-nomatch"), "nothing is said about a search of the filters which was not made").toHaveCount(0)
    await checkpoint(page, "filters-nothing-left-to-filter", { mask: volatile(page) })

    // Searching the filters for something no facet matches empties the panel, which is the one case where
    // the panel says that nothing matched instead of showing an empty list.
    const filterInput = page.locator(".pd-searchresultsfeed-input-filters")
    await expect(filterInput, "the box which searches the filters").toBeVisible()
    await filterInput.fill(NO_MATCH_TERM)
    await expect(page.locator(".pd-searchresultsfeed-empty-nomatch"), "the panel says that no filter matched the term").toBeVisible({ timeout: LOADING_TIMEOUT })
    await expect(page.locator(".pd-filtersresult"), "no facet is left").toHaveCount(0)

    // Emptying the box brings the active filter back, and dropping it leaves a search which matches nothing
    // and has nothing to offer, which is the message the panel keeps for that.
    await filterInput.fill("")
    await expect(instanceOf, "the active facet is back").toBeVisible({ timeout: LOADING_TIMEOUT })
    const clearButton = instanceOf.locator(".pd-filtersresult-button-clear")
    await expect(clearButton, "the active filter can still be cleared").toBeVisible()
    await clearButton.click()
    await expect(page.locator(".pd-filtersresult-button-clear"), "no filter is active any more").toHaveCount(0)
    await expectNoResults(page)
    await expect(page.locator(".pd-searchresultsfeed-empty-filters"), "the panel reports that it has no filters to offer").toBeVisible({ timeout: LOADING_TIMEOUT })
    await checkpoint(page, "filters-nothing-left-empty", { mask: volatile(page) })

    console.log(`Successfully kept a filter over ${filtered} documents clearable after the search was narrowed to nothing.`)
  })
})
