import type { Locator, Page } from "@playwright/test"

import { LANGUAGES, openClassSearch, PROPERTY_IDS } from "../peerdb_utils"
import {
  checkpoint,
  checkpointFacet,
  countDigits,
  expect,
  expectFacetBack,
  expectFewerResults,
  expectFilterActive,
  expectNoConsoleErrors,
  expectResultsCount,
  facetMetadata,
  fetchFromPage,
  filter,
  goHome,
  loadAllResults,
  LOADING_TIMEOUT,
  openAllDocumentsSearch,
  PEERDB_URL,
  settleFilters,
  switchLanguage,
  test,
  volatile,
} from "../utils"

// Every time facet a search over all documents offers, by the property it filters on, in the order the
// panel lists them, which is by how many documents the facet covers. All of them filter on a property of
// the document itself: no claim of the test data carries a time value of its own, so this search offers no
// nested time facet the way it offers a nested amount facet (the duration of an audio recording).
const TIME_FACETS: Array<Array<string>> = [
  [PROPERTY_IDS.PERIOD],
  [PROPERTY_IDS.FIRST_SURVEYED],
  [PROPERTY_IDS.BORN],
  [PROPERTY_IDS.SURVEY_PERIOD],
  [PROPERTY_IDS.DATE_MADE],
  [PROPERTY_IDS.FIRST_DOCUMENTED],
  [PROPERTY_IDS.OBSERVED_ON],
  [PROPERTY_IDS.FIRST_CONTACT],
  [PROPERTY_IDS.ACTIVE_PERIOD],
  [PROPERTY_IDS.OCCUPATION_PERIOD],
  [PROPERTY_IDS.RECORDED_ON],
  [PROPERTY_IDS.FOUNDED],
  [PROPERTY_IDS.PUBLISHED_ON],
  [PROPERTY_IDS.DIED],
]

// The length of a year in seconds. A time facet reports the bounds of its histogram in seconds, and this
// turns the span between them into years, closely enough to tell one spanning centuries from one spanning
// decades.
const YEAR = 365.2425 * 24 * 60 * 60

// Shows the facets beyond the first page of them, which is where every time facet other than the one for
// the period of a document lives.
//
// The press is dispatched rather than clicked, the way the panel's own handler presses it. Clicking scrolls
// the button into view first, and the panel adds its next facets whenever the end of the page comes near,
// so the scroll both presses the button by itself and takes it out from under the press which follows.
async function expandFilterList(page: Page): Promise<void> {
  const moreFilters = page.locator(".pd-searchresultsfeed-button-morefilters")
  await expect(moreFilters, "the button which adds the next facets").toBeVisible()
  await moreFilters.dispatchEvent("click")
  await page.waitForLoadState("networkidle")
}

// Moves one of the slider's handles by tapping the track. The slider is created with the "snap" behaviour,
// so a tap moves the handle nearest to the tapped position there: a fraction below the middle moves the
// lower handle (the start of the range) and one above it moves the upper handle (the end of the range).
async function tapRange(facet: Locator, fraction: number): Promise<void> {
  const slider = facet.locator(".pd-timefiltersresult-input-range")
  await expect(slider, "the range slider of the time facet").toBeVisible()

  // The box is polled rather than read once. The panel replaces a facet whenever the list of facets it
  // belongs to comes back, so a slider which was on the page a moment ago can be off it again by the time it
  // is measured, which yields no box at all. Waiting for one measures the slider which is there now.
  let box: Awaited<ReturnType<Locator["boundingBox"]>> = null
  await expect
    .poll(
      async () => {
        box = await slider.boundingBox()
        return box !== null
      },
      { message: "the range slider of the time facet has a box to tap", timeout: LOADING_TIMEOUT },
    )
    .toBe(true)

  await slider.click({ position: { x: box!.width * fraction, y: box!.height / 2 } })
}

test.describe("PeerDB Time Filter Flows", () => {
  test("Test the whole result page of a search filtered by time", async ({ context }) => {
    const page = await context.newPage()

    // The institutes are the smallest class which records a date for every one of its documents, and a
    // search of them holds fewer results than the feed renders at once and fewer facets than the panel
    // shows at once. That is what makes a screenshot of the whole page worth taking: with nothing left for
    // either of them to load, the page is the same height on every run, while a search whose feed still has
    // results to add grows while the screenshot of it is being taken.
    await openClassSearch(page, "INSTITUTE")
    await settleFilters(page)
    await loadAllResults(page)
    await expect(page.locator(".pd-searchresultsfeed-button-morefilters"), "the panel has no more facets to add").toHaveCount(0)
    const unfiltered = await countDigits(page.locator(".pd-searchresultsheader-count-results"))
    await checkpoint(page, "time-filters-institute-search", { mask: volatile(page) })

    const founded = filter(page, "time", PROPERTY_IDS.FOUNDED)
    await expect(founded, "the facet for when an institute was founded").toBeVisible()
    await expectFilterActive(page, founded, false)

    // Selecting a range leaves the page holding the facet with its filter applied, the header reporting the
    // narrowed count and the feed showing what is left of the results, which is what the whole page has to
    // look like for a filtered search.
    await tapRange(founded, 0.4)
    await expectFilterActive(page, founded, true)
    const narrowed = await expectFewerResults(page, unfiltered)
    await settleFilters(page)
    await loadAllResults(page)
    await checkpoint(page, "time-filters-institute-search-filtered", { mask: volatile(page) })

    console.log(`Successfully filtered a whole result page by time, narrowing ${unfiltered} documents down to ${narrowed}.`)
  })

  test("Test the time facet renders a histogram and a range slider", async ({ context }) => {
    const page = await context.newPage()

    // The people of the catalogue are searched rather than everything, because every one of them carries a
    // date of birth: the facet is then the whole of what the panel says about the property.
    await openClassSearch(page, "INDIVIDUAL")

    // A time facet has no per-facet collapse the way a reference facet does: it has no list of values to
    // show more of, only a histogram and its checkboxes, all of which are always shown. What is collapsed
    // and expanded is therefore the list of facets itself, which a later test covers.
    const born = filter(page, "time", PROPERTY_IDS.BORN)
    await expect(born, "the facet for the date of birth").toBeVisible()
    await settleFilters(page)

    // The facet is a histogram: a chart, the two edges of the span it covers, and the slider selecting a
    // range within it. The row for a single value is not used here because the property has many distinct
    // values, so the histogram has more than one bucket to draw.
    await expect(born.locator(".pd-filtersresult-title"), "the facet names the property it filters on").toBeVisible()
    await expect(born.locator(".pd-timefiltersresult-row-histogram"), "the histogram row").toBeVisible()
    await expect(born.locator(".pd-timefiltersresult-label-from"), "the start of the span the histogram covers").toBeVisible()
    await expect(born.locator(".pd-timefiltersresult-label-to"), "the end of the span the histogram covers").toBeVisible()
    await expect(born.locator(".pd-timefiltersresult-input-range"), "the range slider").toBeVisible()
    await expect(born.locator(".pd-timefiltersresult-row-value"), "the row for a property with a single value").toHaveCount(0)
    await expect(born.locator(".pd-timefiltersresult-row-range"), "the row standing in for a range which cannot be drawn").toHaveCount(0)

    // The chart draws one bar per bucket of the histogram, so the bars are counted against the number of
    // buckets the server reported rather than against a number written down here.
    // The facet records the address its values came from, which for a time facet is the property alone: a
    // time is a moment and not a measurement, so nothing further narrows it the way a unit narrows an amount.
    const session = new URL(page.url()).pathname.split("/").pop()
    const source = await born.getAttribute("data-url")
    expect(source, "the facet names the address its values came from").not.toBeNull()
    expect(new URL(source!, PEERDB_URL).pathname, "the facet is the values of its property in this search").toBe(`/api/s/filters/${session}/time/${PROPERTY_IDS.BORN}`)

    const metadata = await facetMetadata(page, born)
    await expect(born.locator(".pd-timefiltersresult-chart rect"), "the chart draws one bar per bucket").toHaveCount(metadata.total)
    expect(metadata.total, "the histogram has more than one bucket to draw").toBeGreaterThan(1)

    // Documents which state the property with no value at all, or state that it has none, are counted
    // separately from the histogram. A date of birth is recorded for every person as a date and in no other
    // way, so none of those rows render here. The date of death is the property which is recorded in every
    // way at once, and the tests of the rows work over that one.
    await expect(born.locator(".pd-timefiltersresult-row-exists"), "the row for claims without a known endpoint").toHaveCount(0)
    await expect(born.locator(".pd-timefiltersresult-row-unknown"), "the row for an unknown value").toHaveCount(0)
    await expect(born.locator(".pd-timefiltersresult-row-none"), "the row for no value at all").toHaveCount(0)
    await expect(born.locator(".pd-timefiltersresult-row-hasproperty"), "the row for having the property").toHaveCount(0)

    await checkpointFacet(page, "time-filters-born-facet", born)

    console.log(`Successfully verified that a time facet renders a histogram of ${metadata.total} buckets and a range slider over ${metadata.exists} documents.`)
  })

  test("Test a time facet spanning centuries next to one spanning decades", async ({ context }) => {
    const page = await context.newPage()

    // The artifacts are the one class whose dates reach back before the era the catalogue records: the
    // oldest of them is dated to a century rather than to a year, and the youngest to a day, so the facet
    // has to cover several centuries at once.
    await openClassSearch(page, "ARTIFACT")
    const dateMade = filter(page, "time", PROPERTY_IDS.DATE_MADE)
    await expect(dateMade, "the facet for the date an artifact was made").toBeVisible()
    const wide = await facetMetadata(page, dateMade)
    const wideYears = (wide.to - wide.from) / YEAR
    await expect(dateMade.locator(".pd-timefiltersresult-row-histogram"), "the histogram of the wide facet").toBeVisible()
    await checkpointFacet(page, "time-filters-datemade-facet", dateMade)

    // The dates of the publications sit inside a single research era, so the same view has to render a span
    // of decades as readably as it renders one of centuries.
    await openClassSearch(page, "PUBLICATION")
    const publishedOn = filter(page, "time", PROPERTY_IDS.PUBLISHED_ON)
    await expect(publishedOn, "the facet for the date a publication came out").toBeVisible()
    const narrow = await facetMetadata(page, publishedOn)
    const narrowYears = (narrow.to - narrow.from) / YEAR
    await expect(publishedOn.locator(".pd-timefiltersresult-row-histogram"), "the histogram of the narrow facet").toBeVisible()
    await checkpointFacet(page, "time-filters-publishedon-facet", publishedOn)

    expect(narrowYears, "the publication dates span decades").toBeGreaterThan(10)
    expect(wideYears, "the dates the artifacts were made span centuries").toBeGreaterThan(200)
    expect(wideYears, "the wide facet covers several times what the narrow one covers").toBeGreaterThan(5 * narrowYears)

    console.log(`Successfully compared a time facet spanning ${Math.round(wideYears)} years with one spanning ${Math.round(narrowYears)} years.`)
  })

  test("Test expanding the filter list shows the remaining time facets", async ({ context }) => {
    const page = await context.newPage()

    await openAllDocumentsSearch(page)

    // The panel starts collapsed to the facets it shows first, one of which is a time facet: the period a
    // document covers, which every kind of period in the data is a subproperty of. It is named and counted,
    // so a time facet which starts being shown here, or stops being, fails the test rather than passing
    // unnoticed.
    await expect(filter(page, "time", PROPERTY_IDS.PERIOD), "the facet for the period of a document").toBeVisible()
    await expect(page.locator(".pd-timefiltersresult"), "the time facets shown before the list is expanded").toHaveCount(1)

    // The time facets the search offers beyond that one are not there yet.
    await expect(filter(page, "time", PROPERTY_IDS.FIRST_SURVEYED), "the facet for when a place was first surveyed").toHaveCount(0)
    await expect(filter(page, "time", PROPERTY_IDS.BORN), "the facet for the date of birth").toHaveCount(0)

    await expandFilterList(page)

    // Expanding the list brings in the facets for the dates the places and the people of the catalogue
    // carry.
    await expect(filter(page, "time", PROPERTY_IDS.FIRST_SURVEYED), "the facet for when a place was first surveyed").toBeVisible()
    await expect(filter(page, "time", PROPERTY_IDS.BORN), "the facet for the date of birth").toBeVisible()
    await expect(page.locator(".pd-timefiltersresult"), "the time facets shown after one expansion").toHaveCount(3)

    // Nothing is screenshotted here. Every screenshot is taken of the whole page, which moves it, and this
    // panel adds its next facets whenever the end of the page comes near, so a screenshot would have to
    // wait for every facet the catalogue has before it could be taken twice with the same result. The
    // facets themselves are screenshotted from the class searches of the other tests, where the panel is a
    // handful of facets rather than all of them.

    console.log("Successfully expanded the filter list from 1 to 3 time facets.")
  })

  test("Test selecting the start of a time range with the range slider", async ({ context }) => {
    const page = await context.newPage()

    await openClassSearch(page, "INDIVIDUAL")

    const born = filter(page, "time", PROPERTY_IDS.BORN)
    await expect(born, "the facet for the date of birth").toBeVisible()
    await expectFilterActive(page, born, false)

    // Without a filter every person is a result, and the facet counts the ones which carry the property at
    // all, so any range selected below is bound to leave fewer results than the facet counts.
    const unfiltered = await countDigits(page.locator(".pd-searchresultsheader-count-results"))
    const withProperty = await countDigits(born.locator(".pd-filtersresult-header"))
    expect(Number(withProperty), "documents with the property").toBeLessThanOrEqual(Number(unfiltered))

    // Move the start of the range into the histogram. The results are then limited to documents whose date
    // of birth reaches into the selected range, which is fewer than the documents having the property.
    await tapRange(born, 0.3)
    await expectFilterActive(page, born, true)
    const narrowed = await expectFewerResults(page, withProperty)

    await checkpointFacet(page, "time-filters-range-facet-start-selected", born)

    console.log(`Successfully selected the start of a time range and narrowed ${withProperty} documents down to ${narrowed}.`)
  })

  test("Test narrowing a time range from both ends", async ({ context }) => {
    const page = await context.newPage()

    await openClassSearch(page, "INDIVIDUAL")

    const born = filter(page, "time", PROPERTY_IDS.BORN)
    await expect(born, "the facet for the date of birth").toBeVisible()
    await expectFilterActive(page, born, false)
    const withProperty = await countDigits(born.locator(".pd-filtersresult-header"))

    await tapRange(born, 0.3)
    await expectFilterActive(page, born, true)
    const afterStart = await expectFewerResults(page, withProperty)

    // The results and the facets are fetched separately, so the results narrowing above says nothing about
    // the panel: the facet is reloaded with the range the first tap left, and the slider is rebuilt to span
    // it. A tap which lands before that rebuild is a tap on the slider of the range before it, so the same
    // pixel picks a different value, and the histogram of a range selected from both ends spans exactly the
    // values picked. The facets are therefore settled before the range is narrowed from the other end.
    await settleFilters(page)

    // Move the end of the range as well, so a bounded range is selected and the results narrow again. The
    // slider snaps to the handle nearest what was tapped, so a tap in the upper half moves the end.
    await tapRange(born, 0.7)
    await expectFilterActive(page, born, true)
    const afterEnd = await expectFewerResults(page, afterStart)

    // Both edges of the histogram now render the selected range rather than the span of the property, so
    // the two of them are what the facet says the selection is.
    await expect(born.locator(".pd-timefiltersresult-label-from"), "the start of the selected range").toBeVisible()
    await expect(born.locator(".pd-timefiltersresult-label-to"), "the end of the selected range").toBeVisible()

    await checkpointFacet(page, "time-filters-range-facet-both-selected", born)

    console.log(`Successfully narrowed a time range from both ends: ${withProperty} documents down to ${afterStart} and then to ${afterEnd}.`)
  })

  test("Test clearing a time range", async ({ context }) => {
    const page = await context.newPage()

    await openClassSearch(page, "INDIVIDUAL")

    const born = filter(page, "time", PROPERTY_IDS.BORN)
    await expect(born, "the facet for the date of birth").toBeVisible()
    await expectFilterActive(page, born, false)
    const unfiltered = await countDigits(page.locator(".pd-searchresultsheader-count-results"))
    const withProperty = await countDigits(born.locator(".pd-filtersresult-header"))

    await tapRange(born, 0.3)
    await expectFilterActive(page, born, true)
    await expectFewerResults(page, withProperty)

    // Clearing the facet drops the filter and brings every document back.
    const clear = born.locator(".pd-filtersresult-button-clear")
    await expect(clear, "the button clearing the facet").toBeVisible()
    await clear.click()
    await expectFilterActive(page, born, false)
    await expectResultsCount(page, unfiltered)

    await settleFilters(page)
    await checkpointFacet(page, "time-filters-range-facet-cleared", born)

    console.log(`Successfully cleared a time range and got all ${unfiltered} documents back.`)
  })

  test("Test selecting the missing values of a time filter", async ({ context }) => {
    const page = await context.newPage()

    // The date of death is recorded for only a few of the people, so its facet is the one which offers the
    // documents missing the property next to the histogram of the ones having it.
    await openClassSearch(page, "INDIVIDUAL")

    const died = filter(page, "time", PROPERTY_IDS.DIED)
    await expect(died, "the facet for the date of death").toBeVisible()
    await expectFilterActive(page, died, false)

    const missingCheckbox = died.locator(".pd-timefiltersresult-checkbox-missing")
    await expect(died.locator(".pd-timefiltersresult-label-missing"), "the label of the missing row").toBeVisible()
    await expect(missingCheckbox, "the checkbox of the missing row").toBeVisible()
    await expect(missingCheckbox, "the missing row starts unselected").not.toBeChecked()

    // The checkbox is addressed by the identity the facet gives it, which is the kind of the filter, the
    // property it filters on and the selection, joined by slashes.
    await expect(missingCheckbox, "the missing row is identified by its kind, property and selection").toHaveAttribute("id", `time/${PROPERTY_IDS.DIED}/missing`)
    const missing = await countDigits(died.locator(".pd-timefiltersresult-count-missing"))
    await checkpointFacet(page, "time-filters-missing-facet-unchecked", died)

    // Selecting missing keeps exactly the documents without the property, which is the count the row shows.
    await missingCheckbox.click()
    await expectResultsCount(page, missing)
    await expectFacetBack(page, died)
    await expectFilterActive(page, died, true)
    await expect(missingCheckbox, "the missing row is selected").toBeChecked()

    await checkpointFacet(page, "time-filters-missing-facet-checked", died)

    console.log(`Successfully selected the ${missing} documents missing a time property.`)
  })

  test("Test deselecting the missing values of a time filter", async ({ context }) => {
    const page = await context.newPage()

    await openClassSearch(page, "INDIVIDUAL")

    const died = filter(page, "time", PROPERTY_IDS.DIED)
    await expect(died, "the facet for the date of death").toBeVisible()
    await expectFilterActive(page, died, false)
    const unfiltered = await countDigits(page.locator(".pd-searchresultsheader-count-results"))

    const missingCheckbox = died.locator(".pd-timefiltersresult-checkbox-missing")
    await expect(missingCheckbox, "the checkbox of the missing row").toBeVisible()
    await missingCheckbox.click()
    await expectFacetBack(page, died)
    await expectFilterActive(page, died, true)

    // Unchecking leaves the filter without any selection, so it is removed from the search session
    // altogether and the clear button goes away with it.
    await expect(missingCheckbox, "the checkbox of the missing row after selecting it").toBeVisible()
    await missingCheckbox.click()
    await expectResultsCount(page, unfiltered)
    await expectFacetBack(page, died)
    await expectFilterActive(page, died, false)
    await expect(missingCheckbox, "the missing row is unselected again").not.toBeChecked()

    await checkpointFacet(page, "time-filters-missing-facet-unchecked-again", died)

    console.log(`Successfully deselected the documents missing a time property and got all ${unfiltered} back.`)
  })

  test("Test the missing row ticked together with a selected time range", async ({ context }) => {
    const page = await context.newPage()

    // A range and the special rows of the same property are two filters of the search session rather than
    // one, and the two are OR-ed together (SpecialsFilter in search/filter_model.go), so one search can ask
    // for the documents dated inside a range and the documents carrying no date at all. The date of death is
    // the property which offers both: a histogram to select a range in, and a missing row beside it.
    await openClassSearch(page, "INDIVIDUAL")

    const died = filter(page, "time", PROPERTY_IDS.DIED)
    await expect(died, "the facet for the date of death").toBeVisible()
    await expectFilterActive(page, died, false)
    const unfiltered = await countDigits(page.locator(".pd-searchresultsheader-count-results"))
    const withProperty = await countDigits(died.locator(".pd-filtersresult-header"))

    await tapRange(died, 0.3)
    await expectFilterActive(page, died, true)
    const inRange = await expectFewerResults(page, withProperty)

    // The count of the missing row is read while the range is selected, because that is the number the
    // reader sees beside the row at the moment they tick it.
    await expectFacetBack(page, died)
    const missingCheckbox = died.locator(".pd-timefiltersresult-checkbox-missing")
    await expect(missingCheckbox, "the checkbox of the missing row").toBeVisible()
    await expect(missingCheckbox, "selecting a range leaves the missing row unselected").not.toBeChecked()
    const missing = await countDigits(died.locator(".pd-timefiltersresult-count-missing"))

    // No document is both dated inside the range and carrying no date at all, so what the two of them stand
    // for adds up rather than overlapping.
    await missingCheckbox.click()
    await expectResultsCount(page, String(Number(inRange) + Number(missing)))
    await expectFacetBack(page, died)
    await expectFilterActive(page, died, true)
    await expect(missingCheckbox, "the missing row is checked beside the selected range").toBeChecked()

    // The range is still the one which was selected: ticking the row added documents to the search rather
    // than replacing the filter the facet already carried.
    await expect(died.locator(".pd-timefiltersresult-label-from"), "the start of the selected range").toBeVisible()
    await expect(died.locator(".pd-timefiltersresult-label-to"), "the end of the selected range").toBeVisible()

    await checkpointFacet(page, "time-filters-missing-and-range-facet", died)

    console.log(
      `Successfully ticked the missing row of a time facet beside a selected range: ${inRange} of ${unfiltered} documents fall in the range and ${missing} carry no date, ${Number(inRange) + Number(missing)} in all.`,
    )
  })

  test("Test the none, unknown and has property rows ticked together with a selected time range", async ({ context }) => {
    const page = await context.newPage()

    // The date of death is stated every way a time property can be stated, so its facet offers all four
    // rows beside the histogram. Each of them is a selection of the path's specials filter, which is OR-ed
    // with the range, so ticking them one after another only ever widens the search.
    await openClassSearch(page, "INDIVIDUAL")

    const died = filter(page, "time", PROPERTY_IDS.DIED)
    await expect(died, "the facet for the date of death").toBeVisible()
    await expectFilterActive(page, died, false)
    const withProperty = await countDigits(died.locator(".pd-filtersresult-header"))

    await expect(died.locator(".pd-timefiltersresult-row-missing"), "the row for the documents which state no date").toHaveCount(1)
    await expect(died.locator(".pd-timefiltersresult-row-none"), "the row for the documents which state that there is no date").toHaveCount(1)
    await expect(died.locator(".pd-timefiltersresult-row-unknown"), "the row for the documents whose date is unknown").toHaveCount(1)
    await expect(died.locator(".pd-timefiltersresult-row-hasproperty"), "the row for the documents which state only that there is a date").toHaveCount(1)
    await checkpointFacet(page, "time-filters-every-row-facet", died)

    await tapRange(died, 0.3)
    await expectFilterActive(page, died, true)
    let found = Number(await expectFewerResults(page, withProperty))
    const inRange = found

    // Each row is ticked in turn and the search grows by exactly what that row promised, because a document
    // of this data states the property in one way only and so is counted by one row only.
    const rows = [
      { kind: "none", what: "the documents which state that there is no date" },
      { kind: "unknown", what: "the documents whose date is unknown" },
      { kind: "hasproperty", what: "the documents which state only that there is a date" },
    ] as const
    const counts: Record<string, number> = {}
    for (const row of rows) {
      await expectFacetBack(page, died)
      const checkbox = died.locator(`.pd-timefiltersresult-checkbox-${row.kind}`)
      await expect(checkbox, `the checkbox of the row for ${row.what}`).toBeVisible()
      await expect(checkbox, `the row for ${row.what} is not ticked yet`).not.toBeChecked()
      const promised = Number(await countDigits(died.locator(`.pd-timefiltersresult-count-${row.kind}`)))
      expect(promised, `the row for ${row.what} stands for some of the documents`).toBeGreaterThan(0)
      counts[row.kind] = promised

      await checkbox.click()
      found += promised
      await expectResultsCount(page, String(found))
      await expectFacetBack(page, died)
      await expect(died.locator(`.pd-timefiltersresult-checkbox-${row.kind}`), `the row for ${row.what} is ticked`).toBeChecked()
    }

    // All three are ticked at once, next to a range which is still the one which was selected.
    for (const row of rows) {
      await expect(died.locator(`.pd-timefiltersresult-checkbox-${row.kind}`), `the row for ${row.what} is still ticked`).toBeChecked()
    }
    await expect(died.locator(".pd-timefiltersresult-checkbox-missing"), "the row for the documents which state no date is untouched").not.toBeChecked()
    await expect(died.locator(".pd-timefiltersresult-label-from"), "the start of the selected range").toBeVisible()
    await expect(died.locator(".pd-timefiltersresult-label-to"), "the end of the selected range").toBeVisible()

    await checkpointFacet(page, "time-filters-specials-and-range-facet", died)

    console.log(
      `Successfully ticked the none, unknown and has property rows of a time facet beside a selected range: ${inRange} documents in the range, ${counts.none} with no date, ${counts.unknown} with an unknown date and ${counts.hasproperty} with only a date stated, ${found} in all.`,
    )
  })

  test("Test the missing row is offered exactly when documents lack the property", async ({ context }) => {
    const page = await context.newPage()

    await openClassSearch(page, "INDIVIDUAL")

    // Both facets are of the same class of documents, and only one of the two properties is recorded for
    // every one of them, so the two together say when the row is offered and when it is left out. What is
    // asserted is the rule the view follows rather than the counts of the test data, so a person another
    // test adds cannot turn this into a failure.
    const born = filter(page, "time", PROPERTY_IDS.BORN)
    const died = filter(page, "time", PROPERTY_IDS.DIED)
    await expect(born, "the facet for the date of birth").toBeVisible()
    await expect(died, "the facet for the date of death").toBeVisible()

    const bornMetadata = await facetMetadata(page, born)
    const diedMetadata = await facetMetadata(page, died)
    await expect(born.locator(".pd-timefiltersresult-row-missing"), "the missing row of the property every document has").toHaveCount(bornMetadata.missing > 0 ? 1 : 0)
    await expect(died.locator(".pd-timefiltersresult-row-missing"), "the missing row of the property only some documents have").toHaveCount(
      diedMetadata.missing > 0 ? 1 : 0,
    )
    expect(diedMetadata.missing, "more documents lack a date of death than lack a date of birth").toBeGreaterThan(bornMetadata.missing)

    // The five counts a facet reports partition the documents it covers: every document either states a
    // date, states that there is none, states that it is unknown, states only that there is one, or states
    // nothing at all. The date of birth is stated one way only, so four of its counts are zero, while the
    // date of death is stated every way there is.
    for (const [what, metadata] of [
      ["the date of birth", bornMetadata],
      ["the date of death", diedMetadata],
    ] as const) {
      expect(
        metadata.exists + metadata.none + metadata.unknown + metadata.has_property + metadata.missing,
        `the counts of the facet on ${what} account for every document the search covers`,
      ).toBe(metadata.universe)
    }
    expect(bornMetadata.none + bornMetadata.unknown + bornMetadata.has_property, "a date of birth is stated as a date and in no other way").toBe(0)
    expect(diedMetadata.none, "some people are recorded as having no date of death").toBeGreaterThan(0)
    expect(diedMetadata.unknown, "some people are recorded as having a date of death which is not known").toBeGreaterThan(0)
    expect(diedMetadata.has_property, "some people are recorded as having a date of death and nothing more").toBeGreaterThan(0)

    // The two facets cover the same documents, whichever way each property is stated.
    expect(bornMetadata.universe, "the facets of one search cover the same documents").toBe(diedMetadata.universe)

    await settleFilters(page)
    await checkpointFacet(page, "time-filters-died-facet", died)

    console.log(`Successfully verified the missing row: ${diedMetadata.missing} documents lack a date of death and ${bornMetadata.missing} lack a date of birth.`)
  })

  test("Test every time facet the search offers", async ({ context }) => {
    const page = await context.newPage()

    await openAllDocumentsSearch(page)

    // The panel shows only its first facets and the rest on demand, so which facets the search offers at
    // all is asked of the API instead of counted on screen. The panel publishes the very URL it loaded them
    // from, so the answer is the one the panel is showing rather than one from a separately built request.
    const filtersUrl = await page.locator(".pd-searchresultsfeed-panel-filters").getAttribute("data-url")
    expect(filtersUrl, "the filters panel publishes the URL it loaded its facets from").toBeTruthy()
    const facets = JSON.parse((await fetchFromPage(page, filtersUrl!)).body) as Array<{ type: string; props?: Array<string>; count: number }>

    const timeFacets = facets.filter((facet) => facet.type === "time")
    expect(timeFacets.map((facet) => (facet.props ?? []).join("-")).sort(), "the time facets the search offers").toEqual(
      TIME_FACETS.map((props) => props.join("-")).sort(),
    )

    // Every one of them filters on a property of the document itself. A facet of a property of a claim
    // carries the whole path, so a nested one would be listed with two properties instead of one.
    for (const facet of timeFacets) {
      expect((facet.props ?? []).length, "a time facet of this data filters on a property of the document").toBe(1)
      expect(facet.count, "a facet is offered because documents carry the property").toBeGreaterThan(0)
    }

    // The catalogue records amounts as well as times, and those get facets of their own, which a file of
    // their own covers. Asserting that they are here keeps the two kinds of histogram facet apart: a change
    // which turned one into the other would fail here rather than pass unnoticed.
    expect(facets.filter((facet) => facet.type === "amount").length, "the same search offers amount facets next to the time ones").toBeGreaterThan(0)

    await expectNoConsoleErrors(page)

    console.log(`Successfully verified all ${TIME_FACETS.length} time facets the search over every document offers.`)
  })

  for (const language of LANGUAGES) {
    test(`Test the time facet in ${language}`, async ({ context }) => {
      const page = await context.newPage()

      await goHome(page)
      await switchLanguage(page, language)
      await openClassSearch(page, "INDIVIDUAL")

      // Everything the facet renders is written in the language of the interface: the name of the property
      // it filters on, the edges of the span it covers, and the label of the row for the documents missing
      // the property.
      const died = filter(page, "time", PROPERTY_IDS.DIED)
      await expect(died, "the facet for the date of death").toBeVisible()
      await expect(died.locator(".pd-filtersresult-title"), "the facet names the property it filters on").not.toHaveText(/^\s*$/)
      await expect(died.locator(".pd-timefiltersresult-label-from"), "the start of the span the histogram covers").not.toHaveText(/^\s*$/)
      await expect(died.locator(".pd-timefiltersresult-label-to"), "the end of the span the histogram covers").not.toHaveText(/^\s*$/)
      await expect(died.locator(".pd-timefiltersresult-label-missing"), "the label of the missing row").not.toHaveText(/^\s*$/)

      await checkpointFacet(page, `time-filters-facet-${language}`, died)

      console.log(`Successfully verified a time facet in ${language}.`)
    })
  }
})
