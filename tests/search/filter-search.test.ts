import type { Page } from "@playwright/test"

import { documentIdOf, PROPERTY_IDS, searchByClass } from "../peerdb_utils"
import {
  checkpoint,
  checkpointElement,
  expandFilter,
  expect,
  expectNothingLoading,
  filter,
  filterValue,
  hasValue,
  LOADING_TIMEOUT,
  openFilters,
  resultCount,
  settleFilters,
  test,
  volatile,
} from "../utils"

// The terms the tests type into the box which searches the filters. They are words of the test data rather
// than labels of the interface: the first is the name of a property, the second and the third are words of
// the names of values, and the last one appears in nothing at all. Everything the tests then assert on is
// addressed by the identifier of the property or of the value it belongs to, never by its label.
const FACET_TERM = "biome"
const VALUE_TERM = "Endolithic"
const HAS_TERM = "ring"
const NO_MATCH_TERM = "qqqqnosuchfilter"

// A value of the facet on the classifying property which the facet does not show until it is asked for more
// values, which is what the test about reaching a value beyond the shown ones selects.
const VALUE_BEYOND_FIRST_BATCH = await documentIdOf("BIOME", "CRYO_PLAIN")
// A value the same facet shows straight away, which is what the term narrowing values matches.
const VALUE_IN_FIRST_BATCH = await documentIdOf("BIOME", "ENDOLITH_ZONE")

// How many facets the panel shows before "more filters" is used and how many values a facet shows before
// its own "more" is used. Both mirror FILTERS_INITIAL_LIMIT in PeerDB.
const SHOWN = 10

// Opens the search over the worlds of the catalogue with its filters panel showing. Worlds are what the
// search is scoped to because their panel holds facets of every kind, which is what a test about which of
// them a term keeps needs, while staying small enough to be read as a whole.
async function openSearch(page: Page): Promise<void> {
  await searchByClass(page, "WORLD")
  await openFilters(page)
  await expect(page.locator(".pd-filtersresult"), "the panel shows its first batch of facets").toHaveCount(SHOWN)
}

// Waits until the panel has answered whatever was last typed into the box which searches the filters.
//
// Which facets the panel ends up with is what says a term has been answered, and there is nothing to wait
// for which is true for every term, so the wait is for the panel to stop changing: it is read until two
// readings agree, which also covers a term which leaves the panel as it is.
async function settlePanel(page: Page): Promise<void> {
  const facets = page.locator(".pd-filtersresult")
  let previous = -1
  await expect
    .poll(
      async () => {
        const count = await facets.count()
        const settled = count === previous
        previous = count
        return settled
      },
      { message: "the panel stops changing which facets it shows", timeout: LOADING_TIMEOUT },
    )
    .toBe(true)
  await page.waitForLoadState("networkidle")
  await expectNothingLoading(page)
}

// Types a term into the box which searches the filters and waits until the panel has answered it. The box
// narrows which facets and which of their values are shown, it never narrows the search itself, so what the
// search found has to stay what it was.
async function searchFilters(page: Page, term: string): Promise<void> {
  const input = page.locator(".pd-searchresultsfeed-input-filters")
  await expect(input, "the box which searches the filters").toBeVisible()
  await input.fill(term)
  await expect(input, "the box holds the typed term").toHaveValue(term)
  await settlePanel(page)
}

test.describe("PeerDB Search Filter Search Flows", () => {
  test("Test searching the filters narrows which filters are shown", async ({ context }) => {
    const page = await context.newPage()

    await openSearch(page)
    const unfiltered = await resultCount(page)

    // Facets which have nothing to do with the term, and one which has. The one which has is not among the
    // facets the panel shows at first, so the term has to reach past them for it to be shown at all.
    const instanceOf = filter(page, "ref", PROPERTY_IDS.INSTANCE_OF)
    const contactStatus = filter(page, "ref", PROPERTY_IDS.HAS_CONTACT_STATUS)
    const radius = filter(page, "amount", PROPERTY_IDS.RADIUS)
    const biome = filter(page, "ref", PROPERTY_IDS.HAS_BIOME)
    await expect(instanceOf, "the class facet before the term is typed").toBeVisible()
    await expect(contactStatus, "the facet on the contact status before the term is typed").toBeVisible()
    await expect(radius, "the facet on an amount before the term is typed").toBeVisible()
    await expect(biome, "the facet named after the term is not shown before it is typed").toHaveCount(0)

    await searchFilters(page, FACET_TERM)

    // The facets which match neither by their own name nor by any of their values are gone, and the facet
    // named after the term takes their place. A facet reached by its own name is not narrowed inside: it
    // keeps every value it had, the special rows among them.
    await expect(instanceOf, "the class facet is gone").toHaveCount(0)
    await expect(contactStatus, "the facet on the contact status is gone").toHaveCount(0)
    await expect(biome, "the facet named after the term is shown").toBeVisible()
    await expect(biome.locator(".pd-reffiltertreerow-checkbox"), "the facet reached by its name keeps its values").toHaveCount(SHOWN)

    // A facet on an amount and a facet on a moment in time hold no values to search, so a term which does
    // not name one leaves none of them shown.
    await expect(page.locator(".pd-amountfiltersresult"), "no facet on an amount is left").toHaveCount(0)
    await expect(page.locator(".pd-timefiltersresult"), "no facet on a moment in time is left").toHaveCount(0)

    // Searching the filters narrows the panel only, so the search itself still found what it found.
    await expect.poll(() => resultCount(page), { message: "the search still finds what it found before the term" }).toBe(unfiltered)
    await settleFilters(page)
    await checkpoint(page, "filter-search-narrowed", { mask: volatile(page) })

    await searchFilters(page, "")

    // Emptying the box brings the whole list back.
    await expect(page.locator(".pd-filtersresult"), "the panel is back to its first batch of facets").toHaveCount(SHOWN)
    await expect(instanceOf, "the class facet is back").toBeVisible()
    await expect(contactStatus, "the facet on the contact status is back").toBeVisible()
    await expect(radius, "the facet on an amount is back").toBeVisible()
    await expect(biome, "the facet named after the term is out of sight again").toHaveCount(0)
    await expect.poll(() => resultCount(page), { message: "the search still finds what it found before the term" }).toBe(unfiltered)
    await settleFilters(page)
    await checkpoint(page, "filter-search-cleared", { mask: volatile(page) })

    console.log(`Successfully narrowed the filters of a search of ${unfiltered} documents to the one named after a term and brought the rest back.`)
  })

  test("Test searching the filters narrows the values inside a filter", async ({ context }) => {
    const page = await context.newPage()

    await openSearch(page)

    const classifiedAs = filter(page, "ref", PROPERTY_IDS.CLASSIFIED_AS)
    const biome = filter(page, "ref", PROPERTY_IDS.HAS_BIOME)
    await expect(classifiedAs, "the facet on the classifying property").toBeVisible()
    await expect(classifiedAs.locator(".pd-reffiltertreerow-checkbox"), "the facet shows its first batch of values").toHaveCount(SHOWN)
    await expect(biome, "the facet on the biome is not shown before the term is typed").toHaveCount(0)

    await searchFilters(page, VALUE_TERM)

    // The facets which keep a value named after the term are narrowed to that value instead of disappearing,
    // and that goes for a facet which was not shown before just as much as for one which was.
    await expect(classifiedAs, "the facet on the classifying property is still shown").toBeVisible()
    await expect(classifiedAs.locator(".pd-reffiltertreerow-checkbox"), "only the matching value is left in it").toHaveCount(1)
    await expect(filterValue(page, "ref", [PROPERTY_IDS.CLASSIFIED_AS], VALUE_IN_FIRST_BATCH), "the matching value is the one left").toBeVisible()
    await expect(biome, "the facet on the biome holds the matching value too").toBeVisible()
    await expect(biome.locator(".pd-reffiltertreerow-checkbox"), "only the matching value is left in it").toHaveCount(1)
    await expect(filterValue(page, "ref", [PROPERTY_IDS.HAS_BIOME], VALUE_IN_FIRST_BATCH), "the matching value is the one left").toBeVisible()
    await checkpointElement(page, classifiedAs, "filter-search-values-narrowed-facet")
    await settleFilters(page)
    await checkpoint(page, "filter-search-values-narrowed", { mask: volatile(page) })

    await searchFilters(page, "")

    await expect(page.locator(".pd-filtersresult"), "the panel is back to its first batch of facets").toHaveCount(SHOWN)
    await expect(classifiedAs.locator(".pd-reffiltertreerow-checkbox"), "the facet is back to its first batch of values").toHaveCount(SHOWN)
    await checkpointElement(page, classifiedAs, "filter-search-values-restored-facet")

    console.log(`Successfully narrowed a filter from ${SHOWN} values to the 1 named after a term with the filter search box, and brought the rest back.`)
  })

  test("Test searching the filters narrows the values of a has facet", async ({ context }) => {
    const page = await context.newPage()

    await openSearch(page)

    // The has facet on the document itself holds the properties documents state without stating a value for
    // them, and its values are searched by the names of those properties, the same way a reference facet's
    // values are searched by the names of the documents they stand for.
    const hasFacet = filter(page, "has")
    await expect(hasFacet, "the has facet is not shown before the term is typed").toHaveCount(0)

    await searchFilters(page, HAS_TERM)

    await expect(hasFacet, "the has facet holds a property named after the term").toBeVisible({ timeout: LOADING_TIMEOUT })
    await expect(hasFacet.locator(".pd-hasfiltersresult-checkbox"), "only the matching property is left in it").toHaveCount(1, { timeout: LOADING_TIMEOUT })
    await expect(hasValue(page, PROPERTY_IDS.HAS_RING_SYSTEM), "the matching property is the one left").toBeVisible()
    await checkpointElement(page, hasFacet, "filter-search-has-narrowed-facet")

    const properties = await hasFacet.locator(".pd-hasfiltersresult-checkbox").count()

    await searchFilters(page, "")
    await expect(hasFacet, "the has facet is out of sight again").toHaveCount(0)

    console.log(`Successfully narrowed a has facet to the ${properties} property named after a term with the filter search box.`)
  })

  test("Test searching the filters for a value beyond the ones a filter shows", async ({ context }) => {
    const page = await context.newPage()

    await openSearch(page)

    // A value which the facet does not show until it is asked for more values. Its label is read off the
    // facet once it is shown, so the term the box is given is the name the value is listed under rather
    // than a word written into the test.
    const classifiedAs = filter(page, "ref", PROPERTY_IDS.CLASSIFIED_AS)
    await expect(classifiedAs, "the facet on the classifying property").toBeVisible()
    const beyond = filterValue(page, "ref", [PROPERTY_IDS.CLASSIFIED_AS], VALUE_BEYOND_FIRST_BATCH)
    await expect(beyond, "the value is not among the ones the facet shows").toHaveCount(0)

    const more = classifiedAs.locator(".pd-filtersresult-more")
    for (let batch = 0; batch < 10 && (await beyond.count()) === 0 && (await more.isVisible().catch(() => false)); batch++) {
      await expandFilter(page, classifiedAs)
    }
    await expect(beyond, "the value is shown once the facet is asked for more values").toBeVisible()
    const label = page.locator(`label[for="${["ref", PROPERTY_IDS.CLASSIFIED_AS, VALUE_BEYOND_FIRST_BATCH].join("/")}"].pd-reffiltertreerow-label`)
    await expect(label, "the value is listed under a name").toBeVisible()
    const term = ((await label.textContent()) || "").trim()
    expect(term, "the name the value is listed under").not.toBe("")

    // Loading the search again puts the facet back to the values it shows on its own, so the value is out of
    // reach until the box is given its name.
    await openSearch(page)
    await expect(beyond, "the value is out of sight again").toHaveCount(0)

    await searchFilters(page, term)

    await expect(classifiedAs, "the facet holding the value is shown").toBeVisible()
    await expect(beyond, "the value beyond the ones the facet shows is reached by its name").toBeVisible()
    await expect(label, "the value is listed under the name it was searched by").toHaveText(term)
    // The row the value is listed in is screenshotted rather than the whole facet: how many other values the
    // term happens to match is not the same twice, while the row the term was typed for is.
    const row = page.locator(".pd-reffiltertreerow-row").filter({ has: label })
    await checkpointElement(page, row, "filter-search-value-beyond-row")

    await searchFilters(page, "")
    await expect(beyond, "the value is out of sight again once the term is dropped").toHaveCount(0)
    await expect(classifiedAs.locator(".pd-reffiltertreerow-checkbox"), "the facet is back to its first batch of values").toHaveCount(SHOWN)

    console.log(`Successfully reached a value beyond the ${SHOWN} a filter shows by typing the ${term.length} characters of its name into the filter search box.`)
  })

  test("Test the box which searches the filters stays usable while it answers", async ({ context }) => {
    const page = await context.newPage()

    await openSearch(page)

    // Every keystroke starts a search of its own while the panel is still answering the one before it. The
    // box is not locked while that happens, so a term typed at speed lands in it whole rather than losing
    // the characters typed while the panel was busy.
    const input = page.locator(".pd-searchresultsfeed-input-filters")
    await expect(input, "the box which searches the filters").toBeVisible()
    await expect(input, "the box takes typing").toBeEditable()
    await input.click()
    await input.pressSequentially(VALUE_TERM, { delay: 30 })
    await expect(input, "every character which was typed landed in the box").toHaveValue(VALUE_TERM)
    await expect(input, "the box still takes typing while it answers").toBeEditable()

    // A term which is being answered can be dropped through the button the box grows next to it.
    const clearQuery = page.locator(".pd-searchresultsfeed-button-clearfilterquery")
    await expect(clearQuery, "the box offers to be emptied").toBeVisible()
    await settlePanel(page)
    await checkpointElement(page, page.locator(".pd-searchresultsfeed-input-filters"), "filter-search-box-typed")

    await clearQuery.click()
    await expect(input, "the box is empty again").toHaveValue("")
    await expect(clearQuery, "the box no longer offers to be emptied").toHaveCount(0)
    await expect(page.locator(".pd-filtersresult"), "the panel is back to its first batch of facets").toHaveCount(SHOWN, { timeout: LOADING_TIMEOUT })

    console.log(`Successfully typed ${VALUE_TERM.length} characters into the filter search box while it was answering and emptied it again.`)
  })

  test("Test searching the filters for a term which matches nothing", async ({ context }) => {
    const page = await context.newPage()

    await openSearch(page)
    const unfiltered = await resultCount(page)

    await searchFilters(page, NO_MATCH_TERM)

    // No facet is left, so the panel says so instead of showing an empty list, and the box stays so that the
    // term can be changed or dropped.
    await expect(page.locator(".pd-filtersresult"), "no facet is left").toHaveCount(0)
    await expect(page.locator(".pd-searchresultsfeed-empty-nomatch"), "the panel says that no filter matched the term").toBeVisible()
    await expect(page.locator(".pd-searchresultsfeed-empty-filters"), "the panel does not say that it has no filters to offer").toHaveCount(0)
    await expect(page.locator(".pd-searchresultsfeed-input-filters"), "the box which searches the filters is still there").toBeVisible()
    await expect.poll(() => resultCount(page), { message: "the search still finds what it found before the term" }).toBe(unfiltered)
    await checkpoint(page, "filter-search-no-match", { mask: volatile(page) })

    await searchFilters(page, "")

    await expect(page.locator(".pd-searchresultsfeed-empty-nomatch"), "the message is gone once the term is dropped").toHaveCount(0)
    await expect(page.locator(".pd-filtersresult"), "the panel is back to its first batch of facets").toHaveCount(SHOWN)
    await expect.poll(() => resultCount(page), { message: "the search still finds what it found before the term" }).toBe(unfiltered)
    await checkpoint(page, "filter-search-no-match-cleared", { mask: volatile(page) })

    console.log(`Successfully searched the filters of a search of ${unfiltered} documents for a term which matches nothing.`)
  })
})
