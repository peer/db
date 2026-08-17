import type { Locator } from "@playwright/test"

import { coreDocumentIdOf, documentIdOf, LANGUAGES, openClassSearch, PROPERTY_IDS } from "../peerdb_utils"
import {
  checkpoint,
  checkpointElement,
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
  settleFilters,
  switchLanguage,
  test,
  volatile,
} from "../utils"

// The units the amounts of the test data are measured in. The ones the test data declares itself live in
// its own namespace under the UNIT segment, while the ones every site is populated with (the metre, the
// kilogram, the degree Celsius and the dollar) live in the core namespace. A unit is a document like any
// other, which is what lets a facet render its name in the language of the interface.
const UNIT_IDS = {
  EARTH_DAY: await documentIdOf("UNIT", "EARTH_DAY"),
  EARTH_MASS: await documentIdOf("UNIT", "EARTH_MASS"),
  EARTH_RADIUS: await documentIdOf("UNIT", "EARTH_RADIUS"),
  EARTH_YEAR: await documentIdOf("UNIT", "EARTH_YEAR"),
  HOUR: await documentIdOf("UNIT", "HOUR"),
  INDIVIDUAL: await documentIdOf("UNIT", "INDIVIDUAL"),
  MINUTE: await documentIdOf("UNIT", "MINUTE"),
  PARSEC: await documentIdOf("UNIT", "PARSEC"),
  PERCENT: await documentIdOf("UNIT", "PERCENT"),
  SQUARE_KILOMETRE: await documentIdOf("UNIT", "SQUARE_KILOMETRE"),
  STANDARD_GRAVITY: await documentIdOf("UNIT", "STANDARD_GRAVITY"),
  DOLLAR: await coreDocumentIdOf("UNIT", "$"),
  // The mnemonic of this unit is the degree sign followed by C, written as an escape so that this file
  // stays plain ASCII.
  DEGREE_CELSIUS: await coreDocumentIdOf("UNIT", "\u00b0C"),
  KILOGRAM: await coreDocumentIdOf("UNIT", "kg"),
  METRE: await coreDocumentIdOf("UNIT", "m"),
}

// Every amount facet a search over all documents offers, in the order the panel lists them, which is by how
// many documents the facet covers. An amount facet is per property and unit rather than per property alone,
// so each entry carries the unit its amounts are measured in, and the three properties which count things
// rather than measure them carry none. The last entry with two properties is a nested facet: the duration
// of an audio recording attached to a document rather than a duration of the document itself.
const AMOUNT_FACETS: Array<{ props: Array<string>; unit?: string }> = [
  { props: [PROPERTY_IDS.POPULATION_ESTIMATE], unit: UNIT_IDS.INDIVIDUAL },
  { props: [PROPERTY_IDS.TYPICAL_MASS], unit: UNIT_IDS.KILOGRAM },
  { props: [PROPERTY_IDS.SURFACE_GRAVITY], unit: UNIT_IDS.STANDARD_GRAVITY },
  { props: [PROPERTY_IDS.ORBITAL_PERIOD], unit: UNIT_IDS.EARTH_DAY },
  { props: [PROPERTY_IDS.RADIUS], unit: UNIT_IDS.EARTH_RADIUS },
  { props: [PROPERTY_IDS.DAY_LENGTH], unit: UNIT_IDS.HOUR },
  { props: [PROPERTY_IDS.MEAN_TEMPERATURE], unit: UNIT_IDS.DEGREE_CELSIUS },
  { props: [PROPERTY_IDS.HYDROSPHERE], unit: UNIT_IDS.PERCENT },
  { props: [PROPERTY_IDS.MASS], unit: UNIT_IDS.EARTH_MASS },
  { props: [PROPERTY_IDS.DIMENSION], unit: UNIT_IDS.METRE },
  { props: [PROPERTY_IDS.PARTICIPANT_COUNT], unit: UNIT_IDS.INDIVIDUAL },
  { props: [PROPERTY_IDS.AREA], unit: UNIT_IDS.SQUARE_KILOMETRE },
  { props: [PROPERTY_IDS.ELEVATION_RANGE], unit: UNIT_IDS.METRE },
  { props: [PROPERTY_IDS.LIFESPAN], unit: UNIT_IDS.EARTH_YEAR },
  { props: [PROPERTY_IDS.TYPICAL_HEIGHT], unit: UNIT_IDS.METRE },
  { props: [PROPERTY_IDS.TYPICAL_SIZE], unit: UNIT_IDS.METRE },
  { props: [PROPERTY_IDS.STAR_COUNT] },
  { props: [PROPERTY_IDS.PLANET_COUNT] },
  { props: [PROPERTY_IDS.DISTANCE_FROM_SOL], unit: UNIT_IDS.PARSEC },
  { props: [PROPERTY_IDS.SPEAKER_ESTIMATE], unit: UNIT_IDS.INDIVIDUAL },
  { props: [PROPERTY_IDS.BUDGET], unit: UNIT_IDS.DOLLAR },
  { props: [PROPERTY_IDS.MEMBER_COUNT], unit: UNIT_IDS.INDIVIDUAL },
  { props: [PROPERTY_IDS.STAFF_COUNT] },
  { props: [PROPERTY_IDS.DIAMETER], unit: UNIT_IDS.PARSEC },
  { props: [PROPERTY_IDS.AUDIO, PROPERTY_IDS.DURATION], unit: UNIT_IDS.MINUTE },
]

// How a facet is written down for comparison: its property path and its unit, so that two facets of the
// same property measured in different units stay apart.
function facetKey(facet: { props?: Array<string>; unit?: string }): string {
  return `${(facet.props ?? []).join("-")}:${facet.unit ?? ""}`
}

// The path the facet loaded its values from, without the query string the version is passed as. A facet
// which has no filter of its own yet loads them from the route naming its property and its unit, which is
// what makes an amount facet a facet of the two together.
async function facetPath(facet: Locator): Promise<string> {
  const url = await facet.getAttribute("data-url")
  expect(url, "the facet publishes the URL it loaded its values from").toBeTruthy()
  return url!.split("?")[0]
}

// The reference to the unit the facet renders next to the name of the property. Both the property and the
// unit are rendered as inline references to their documents, and only the ones standing for a property of
// the path carry the class of a path segment, so what is left is the unit.
function unitReference(facet: Locator): Locator {
  return facet.locator(".pd-filtersresult-title .pd-documentrefinline:not(.pd-filterproplabel-value)")
}

// Moves one of the slider's handles by tapping the track. The slider is created with the "snap" behaviour,
// so a tap moves the handle nearest to the tapped position there: a fraction below the middle moves the
// lower handle (the start of the range) and one above it moves the upper handle (the end of the range).
async function tapRange(facet: Locator, fraction: number): Promise<void> {
  const slider = facet.locator(".pd-amountfiltersresult-input-range")
  await expect(slider, "the range slider of the amount facet").toBeVisible()

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
      { message: "the range slider of the amount facet has a box to tap", timeout: LOADING_TIMEOUT },
    )
    .toBe(true)

  await slider.click({ position: { x: box!.width * fraction, y: box!.height / 2 } })
}

test.describe("PeerDB Amount Filter Flows", () => {
  test("Test the whole result page of a search filtered by amount", async ({ context }) => {
    const page = await context.newPage()

    // The galaxies are the smallest class of the catalogue, and a search of them holds fewer results than
    // the feed renders at once and fewer facets than the panel shows at once. That is what makes a
    // screenshot of the whole page worth taking: with nothing left for either of them to load, the page is
    // the same height on every run, while a search whose feed still has results to add grows while the
    // screenshot of it is being taken.
    await openClassSearch(page, "GALAXY")
    await settleFilters(page)
    await loadAllResults(page)
    await expect(page.locator(".pd-searchresultsfeed-button-morefilters"), "the panel has no more facets to add").toHaveCount(0)
    const unfiltered = await countDigits(page.locator(".pd-searchresultsheader-count-results"))
    await checkpoint(page, "amount-filters-galaxy-search", { mask: volatile(page) })

    const diameter = filter(page, "amount", PROPERTY_IDS.DIAMETER)
    await expect(diameter, "the facet for the diameter of a galaxy").toBeVisible()
    await expectFilterActive(page, diameter, false)

    // Selecting a range leaves the page holding the facet with its filter applied, the header reporting the
    // narrowed count and the feed showing what is left of the results, which is what the whole page has to
    // look like for a filtered search.
    await tapRange(diameter, 0.25)
    await expectFilterActive(page, diameter, true)
    const narrowed = await expectFewerResults(page, unfiltered)
    await settleFilters(page)
    await loadAllResults(page)
    await checkpoint(page, "amount-filters-galaxy-search-filtered", { mask: volatile(page) })

    console.log(`Successfully filtered a whole result page by amount, narrowing ${unfiltered} documents down to ${narrowed}.`)
  })

  test("Test the amount facet renders a histogram and a range slider", async ({ context }) => {
    const page = await context.newPage()

    // The star systems are searched rather than everything, because every one of them carries its distance
    // from Sol: the facet is then the whole of what the panel says about the property.
    await openClassSearch(page, "STAR_SYSTEM")

    // An amount facet has no per-facet collapse the way a reference facet does: it has no list of values to
    // show more of, only a histogram and its checkboxes, all of which are always shown. What is collapsed
    // and expanded is therefore the list of facets itself, which a later test covers.
    const distance = filter(page, "amount", PROPERTY_IDS.DISTANCE_FROM_SOL)
    await expect(distance, "the facet for the distance from Sol").toBeVisible()
    await settleFilters(page)

    // The facet is a histogram: a chart, the two edges of the span it covers, and the slider selecting a
    // range within it. The row for a single value is not used here because the property has many distinct
    // values, so the histogram has more than one bucket to draw.
    await expect(distance.locator(".pd-filtersresult-title"), "the facet names the property it filters on").toBeVisible()
    await expect(distance.locator(".pd-amountfiltersresult-row-histogram"), "the histogram row").toBeVisible()
    await expect(distance.locator(".pd-amountfiltersresult-label-from"), "the start of the span the histogram covers").toBeVisible()
    await expect(distance.locator(".pd-amountfiltersresult-label-to"), "the end of the span the histogram covers").toBeVisible()
    await expect(distance.locator(".pd-amountfiltersresult-input-range"), "the range slider").toBeVisible()
    await expect(distance.locator(".pd-amountfiltersresult-row-value"), "the row for a property with a single value").toHaveCount(0)
    await expect(distance.locator(".pd-amountfiltersresult-row-range"), "the row standing in for a range which cannot be drawn").toHaveCount(0)

    // The chart draws one bar per bucket of the histogram, so the bars are counted against the number of
    // buckets the server reported rather than against a number written down here.
    const metadata = await facetMetadata(page, distance)
    await expect(distance.locator(".pd-amountfiltersresult-chart rect"), "the chart draws one bar per bucket").toHaveCount(metadata.total)
    expect(metadata.total, "the histogram has more than one bucket to draw").toBeGreaterThan(1)
    expect(metadata.from, "the span of the histogram starts below where it ends").toBeLessThan(metadata.to)

    // Documents which state the property with no value at all, or state that it has none, are counted
    // separately from the histogram. The distance from Sol is recorded for every star system as a number
    // and in no other way, so none of those rows render here. The estimated population is the property
    // which is recorded in every way at once, and the tests of the rows work over that one.
    await expect(distance.locator(".pd-amountfiltersresult-row-exists"), "the row for claims without a known endpoint").toHaveCount(0)
    await expect(distance.locator(".pd-amountfiltersresult-row-unknown"), "the row for an unknown value").toHaveCount(0)
    await expect(distance.locator(".pd-amountfiltersresult-row-none"), "the row for no value at all").toHaveCount(0)
    await expect(distance.locator(".pd-amountfiltersresult-row-hasproperty"), "the row for having the property").toHaveCount(0)

    await checkpointFacet(page, "amount-filters-distance-facet", distance)

    console.log(`Successfully verified that an amount facet renders a histogram of ${metadata.total} buckets and a range slider over ${metadata.exists} documents.`)
  })

  test("Test an amount facet is a facet of a property and a unit", async ({ context }) => {
    const page = await context.newPage()

    // The places of the catalogue are searched, because they are the documents whose amounts are measured
    // (the radius of a world) next to ones which are counted (the number of stars of a system), and because
    // most of them carry neither, so both facets also offer the documents missing the property.
    await openClassSearch(page, "PLACE")
    await settleFilters(page)

    const radius = filter(page, "amount", PROPERTY_IDS.RADIUS)
    const starCount = filter(page, "amount", PROPERTY_IDS.STAR_COUNT)
    await expect(radius, "the facet for the radius of a world").toBeVisible()
    await expect(starCount, "the facet for the number of stars of a system").toBeVisible()

    // A measured property is a facet of the property and the unit together, so the unit is part of the
    // address its values are loaded from, and the facet renders it next to the name of the property. The
    // whole address is compared and not only its end, so the facet is pinned to this search as well as to
    // the property and the unit.
    const session = new URL(page.url()).pathname.split("/").pop()
    expect(await facetPath(radius), "the facet of a measured property is addressed by its property and its unit").toBe(
      `/api/s/filters/${session}/amount/${PROPERTY_IDS.RADIUS}/${UNIT_IDS.EARTH_RADIUS}`,
    )
    await expect(unitReference(radius), "the facet of a measured property names its unit").toHaveCount(1)
    await expect(unitReference(radius), "the unit named is the one the amounts are measured in").toHaveAttribute("data-url", `/api/d/${UNIT_IDS.EARTH_RADIUS}`)

    // A counted property has no unit at all, so neither its address nor its label carries one.
    expect(await facetPath(starCount), "the facet of a counted property is addressed by its property alone").toMatch(new RegExp(`/amount/${PROPERTY_IDS.STAR_COUNT}$`))
    await expect(unitReference(starCount), "the facet of a counted property names no unit").toHaveCount(0)

    // The same difference reaches the selections a facet offers: their identity is the kind of the filter,
    // the property path, the unit and the selection, and a facet without a unit leaves that part empty.
    await expect(radius.locator(".pd-amountfiltersresult-checkbox-missing"), "the missing row of the measured property carries its unit").toHaveAttribute(
      "id",
      `amount/${PROPERTY_IDS.RADIUS}/${UNIT_IDS.EARTH_RADIUS}/missing`,
    )
    await expect(starCount.locator(".pd-amountfiltersresult-checkbox-missing"), "the missing row of the counted property carries no unit").toHaveAttribute(
      "id",
      `amount/${PROPERTY_IDS.STAR_COUNT}//missing`,
    )

    await checkpointElement(page, radius, "amount-filters-radius-facet")
    await checkpointElement(page, starCount, "amount-filters-starcount-facet")

    console.log("Successfully verified that an amount facet is a facet of a property and a unit, and that a counted property has none.")
  })

  test("Test the amount facets of properties which are counted rather than measured", async ({ context }) => {
    const page = await context.newPage()

    // Three properties of the catalogue count things instead of measuring them, so their facets are the
    // only ones which render no unit. Two of them are of a star system and the third of an institute.
    await openClassSearch(page, "STAR_SYSTEM")
    await settleFilters(page)

    const starCount = filter(page, "amount", PROPERTY_IDS.STAR_COUNT)
    const planetCount = filter(page, "amount", PROPERTY_IDS.PLANET_COUNT)
    for (const [facet, what, property] of [
      [starCount, "the number of stars", PROPERTY_IDS.STAR_COUNT],
      [planetCount, "the number of planets", PROPERTY_IDS.PLANET_COUNT],
    ] as Array<[Locator, string, string]>) {
      await expect(facet, `the facet for ${what}`).toBeVisible()
      await expect(unitReference(facet), `the facet for ${what} names no unit`).toHaveCount(0)
      expect(await facetPath(facet), `the facet for ${what} is addressed by its property alone`).toMatch(new RegExp(`/amount/${property}$`))
      await expect(facet.locator(".pd-amountfiltersresult-row-histogram"), `the histogram for ${what}`).toBeVisible()
    }
    await checkpointElement(page, starCount, "amount-filters-starcount-facet-systems")
    await checkpointElement(page, planetCount, "amount-filters-planetcount-facet")

    await openClassSearch(page, "INSTITUTE")
    await settleFilters(page)

    const staffCount = filter(page, "amount", PROPERTY_IDS.STAFF_COUNT)
    await expect(staffCount, "the facet for the number of staff").toBeVisible()
    await expect(unitReference(staffCount), "the facet for the number of staff names no unit").toHaveCount(0)
    expect(await facetPath(staffCount), "the facet for the number of staff is addressed by its property alone").toMatch(
      new RegExp(`/amount/${PROPERTY_IDS.STAFF_COUNT}$`),
    )
    await checkpointElement(page, staffCount, "amount-filters-staffcount-facet")

    console.log("Successfully verified the three amount facets of properties which are counted rather than measured.")
  })

  test("Test the amount facet of a property measured in a currency", async ({ context }) => {
    const page = await context.newPage()

    // The budget of an expedition is the one amount of the catalogue measured in money, and its unit is one
    // of the units every site is populated with rather than one the test data declares.
    await openClassSearch(page, "EXPEDITION")
    await settleFilters(page)

    const budget = filter(page, "amount", PROPERTY_IDS.BUDGET)
    await expect(budget, "the facet for the budget of an expedition").toBeVisible()
    expect(await facetPath(budget), "the facet is addressed by its property and the currency").toMatch(new RegExp(`/amount/${PROPERTY_IDS.BUDGET}/${UNIT_IDS.DOLLAR}$`))
    await expect(unitReference(budget), "the facet names the currency the budgets are given in").toHaveAttribute("data-url", new RegExp(`/${UNIT_IDS.DOLLAR}$`))

    // The budgets run into the millions, so the two edges of the histogram are where the view has to render
    // a large amount readably.
    const metadata = await facetMetadata(page, budget)
    expect(metadata.to, "the largest budget is a large amount").toBeGreaterThan(1000000)
    await expect(budget.locator(".pd-amountfiltersresult-label-from"), "the smallest budget the histogram covers").not.toHaveText(/^\s*$/)
    await expect(budget.locator(".pd-amountfiltersresult-label-to"), "the largest budget the histogram covers").not.toHaveText(/^\s*$/)

    await checkpointElement(page, budget, "amount-filters-budget-facet")

    console.log(`Successfully verified the amount facet of a property measured in a currency, covering ${metadata.exists} expeditions.`)
  })

  test("Test the amount facet of an amount inside a claim", async ({ context }) => {
    const page = await context.newPage()

    // The duration of a recording is not a property of the document but of the audio attached to it, so its
    // facet filters on a path of two properties instead of one and loads its values from a route of its
    // own. Every recorded narrative carries its duration, so the facet offers no missing row.
    await openClassSearch(page, "NARRATIVE")
    await settleFilters(page)

    const duration = filter(page, "amount", PROPERTY_IDS.AUDIO, PROPERTY_IDS.DURATION)
    await expect(duration, "the facet for the duration of a recording").toBeVisible()
    expect(await facetPath(duration), "the nested facet is addressed by both properties and the unit").toMatch(
      new RegExp(`/subamount/${PROPERTY_IDS.AUDIO}/${PROPERTY_IDS.DURATION}/${UNIT_IDS.MINUTE}$`),
    )

    // The label of a nested facet is the whole path, so it renders one reference per property and the unit
    // next to them.
    await expect(duration.locator(".pd-filtersresult-title .pd-filterproplabel-value"), "the label of the facet is the whole property path").toHaveCount(2)
    await expect(unitReference(duration), "the facet names the unit the durations are measured in").toHaveAttribute("data-url", new RegExp(`/${UNIT_IDS.MINUTE}$`))

    const metadata = await facetMetadata(page, duration)
    await expect(duration.locator(".pd-amountfiltersresult-row-missing"), "the missing row of a property every recording carries").toHaveCount(
      metadata.missing > 0 ? 1 : 0,
    )
    await expect(duration.locator(".pd-amountfiltersresult-row-histogram"), "the histogram of the nested facet").toBeVisible()
    await checkpointElement(page, duration, "amount-filters-duration-facet")

    // Selecting a range of durations narrows the results to the documents whose recording is that long,
    // which is what makes the nested facet a filter on the document and not only a summary of the claims.
    // The panel is asked for every facet again afterwards, because a search which changed collapses it back
    // to the facets it shows first.
    const unfiltered = await countDigits(page.locator(".pd-searchresultsheader-count-results"))
    await tapRange(duration, 0.6)
    await settleFilters(page)
    await expectFilterActive(page, duration, true)
    const narrowed = await expectFewerResults(page, unfiltered)

    await checkpointFacet(page, "amount-filters-duration-facet-selected", duration)

    console.log(`Successfully filtered on an amount inside a claim, narrowing ${unfiltered} documents down to ${narrowed}.`)
  })

  test("Test expanding the filter list shows the remaining amount facets", async ({ context }) => {
    const page = await context.newPage()

    await openAllDocumentsSearch(page)

    // The panel starts collapsed to the facets it shows first, one of which is an amount facet: how many
    // people a place or a group is estimated to hold. It is named and counted, so an amount facet which
    // starts being shown here, or stops being, fails the test rather than passing unnoticed.
    await expect(filter(page, "amount", PROPERTY_IDS.POPULATION_ESTIMATE), "the facet for the estimated population").toBeVisible()
    await expect(page.locator(".pd-amountfiltersresult"), "the amount facets shown before the list is expanded").toHaveCount(1)

    // The amount facets the search offers beyond that one are not there yet.
    await expect(filter(page, "amount", PROPERTY_IDS.TYPICAL_MASS), "the facet for the typical mass of a species").toHaveCount(0)

    // The press is dispatched rather than clicked, the way the panel's own handler presses it. Clicking
    // scrolls the button into view first, and the panel adds its next facets whenever the end of the page
    // comes near, so the scroll both presses the button by itself and takes it out from under the press
    // which follows.
    const moreFilters = page.locator(".pd-searchresultsfeed-button-morefilters")
    await expect(moreFilters, "the button which adds the next facets").toBeVisible()
    await moreFilters.dispatchEvent("click")
    await page.waitForLoadState("networkidle")

    // Expanding the list brings in the facet for the mass the living things of the catalogue are measured
    // by.
    await expect(filter(page, "amount", PROPERTY_IDS.TYPICAL_MASS), "the facet for the typical mass of a species").toBeVisible()
    await expect(page.locator(".pd-amountfiltersresult"), "the amount facets shown after one expansion").toHaveCount(2)

    // Nothing is screenshotted here. Every screenshot is taken of the whole page, which moves it, and this
    // panel adds its next facets whenever the end of the page comes near, so a screenshot would have to
    // wait for every facet the catalogue has before it could be taken twice with the same result. The
    // facets themselves are screenshotted from the class searches of the other tests, where the panel is a
    // handful of facets rather than all of them.

    console.log("Successfully expanded the filter list from 1 to 2 amount facets.")
  })

  test("Test selecting the start of an amount range with the range slider", async ({ context }) => {
    const page = await context.newPage()

    await openClassSearch(page, "MOON")

    const radius = filter(page, "amount", PROPERTY_IDS.RADIUS)
    await expect(radius, "the facet for the radius of a moon").toBeVisible()
    await expectFilterActive(page, radius, false)

    // Without a filter every moon is a result, and the facet counts the ones which carry the property at
    // all, so any range selected below is bound to leave fewer results than the facet counts.
    const unfiltered = await countDigits(page.locator(".pd-searchresultsheader-count-results"))
    const withProperty = await countDigits(radius.locator(".pd-filtersresult-header"))
    expect(Number(withProperty), "documents with the property").toBeLessThanOrEqual(Number(unfiltered))

    // Move the start of the range into the histogram. The results are then limited to the documents whose
    // radius reaches into the selected range, which is fewer than the documents having the property.
    await tapRange(radius, 0.3)
    await expectFilterActive(page, radius, true)
    const narrowed = await expectFewerResults(page, withProperty)

    await checkpointFacet(page, "amount-filters-range-facet-start-selected", radius)

    console.log(`Successfully selected the start of an amount range and narrowed ${withProperty} documents down to ${narrowed}.`)
  })

  test("Test narrowing an amount range from both ends", async ({ context }) => {
    const page = await context.newPage()

    await openClassSearch(page, "MOON")

    const radius = filter(page, "amount", PROPERTY_IDS.RADIUS)
    await expect(radius, "the facet for the radius of a moon").toBeVisible()
    await expectFilterActive(page, radius, false)
    const withProperty = await countDigits(radius.locator(".pd-filtersresult-header"))

    await tapRange(radius, 0.3)
    await expectFilterActive(page, radius, true)
    const afterStart = await expectFewerResults(page, withProperty)

    // The results and the facets are fetched separately, so the results narrowing above says nothing about
    // the panel: the facet is reloaded with the range the first tap left, and the slider is rebuilt to span
    // it. A tap which lands before that rebuild is a tap on the slider of the range before it, so the same
    // pixel picks a different value, and the histogram of a range selected from both ends spans exactly the
    // values picked. The facets are therefore settled before the range is narrowed from the other end.
    await settleFilters(page)

    // Move the end of the range as well, so a bounded range is selected and the results narrow again. The
    // slider snaps to the handle nearest what was tapped, so a tap in the upper half moves the end.
    await tapRange(radius, 0.7)
    await expectFilterActive(page, radius, true)
    const afterEnd = await expectFewerResults(page, afterStart)

    // Both edges of the histogram now render the selected range rather than the span of the property, so
    // the two of them are what the facet says the selection is.
    await expect(radius.locator(".pd-amountfiltersresult-label-from"), "the start of the selected range").toBeVisible()
    await expect(radius.locator(".pd-amountfiltersresult-label-to"), "the end of the selected range").toBeVisible()

    await checkpointFacet(page, "amount-filters-range-facet-both-selected", radius)

    console.log(`Successfully narrowed an amount range from both ends: ${withProperty} documents down to ${afterStart} and then to ${afterEnd}.`)
  })

  test("Test clearing an amount range", async ({ context }) => {
    const page = await context.newPage()

    await openClassSearch(page, "MOON")

    const radius = filter(page, "amount", PROPERTY_IDS.RADIUS)
    await expect(radius, "the facet for the radius of a moon").toBeVisible()
    await expectFilterActive(page, radius, false)
    const unfiltered = await countDigits(page.locator(".pd-searchresultsheader-count-results"))
    const withProperty = await countDigits(radius.locator(".pd-filtersresult-header"))

    await tapRange(radius, 0.3)
    await expectFilterActive(page, radius, true)
    await expectFewerResults(page, withProperty)

    // Clearing the facet drops the filter and brings every document back.
    const clear = radius.locator(".pd-filtersresult-button-clear")
    await expect(clear, "the button clearing the facet").toBeVisible()
    await clear.click()
    await expectFilterActive(page, radius, false)
    await expectResultsCount(page, unfiltered)

    await settleFilters(page)
    await checkpointFacet(page, "amount-filters-range-facet-cleared", radius)

    console.log(`Successfully cleared an amount range and got all ${unfiltered} documents back.`)
  })

  test("Test selecting the missing values of an amount filter", async ({ context }) => {
    const page = await context.newPage()

    // The estimated population is recorded for only a few of the moons, so its facet is the one which
    // offers the documents missing the property next to the histogram of the ones having it. It covers the
    // fewest documents of the moons' facets, which is why the panel is asked for every facet it has before
    // the facet is looked for: the panel lists them by how many documents they cover.
    await openClassSearch(page, "MOON")
    await settleFilters(page)

    const population = filter(page, "amount", PROPERTY_IDS.POPULATION_ESTIMATE)
    await expect(population, "the facet for the estimated population").toBeVisible()
    await expectFilterActive(page, population, false)

    const missingCheckbox = population.locator(".pd-amountfiltersresult-checkbox-missing")
    await expect(population.locator(".pd-amountfiltersresult-label-missing"), "the label of the missing row").toBeVisible()
    await expect(missingCheckbox, "the checkbox of the missing row").toBeVisible()
    await expect(missingCheckbox, "the missing row starts unselected").not.toBeChecked()
    const missing = await countDigits(population.locator(".pd-amountfiltersresult-count-missing"))
    await checkpointFacet(page, "amount-filters-missing-facet-unchecked", population)

    // Selecting missing keeps exactly the documents without the property, which is the count the row shows.
    await missingCheckbox.click()
    await expectResultsCount(page, missing)
    await expectFacetBack(page, population)
    await expectFilterActive(page, population, true)
    await expect(missingCheckbox, "the missing row is selected").toBeChecked()

    await checkpointFacet(page, "amount-filters-missing-facet-checked", population)

    console.log(`Successfully selected the ${missing} documents missing an amount property.`)
  })

  test("Test deselecting the missing values of an amount filter", async ({ context }) => {
    const page = await context.newPage()

    // The facet covering the fewest documents is the last one the panel lists, so the panel is asked for
    // every facet it has before the facet is looked for.
    await openClassSearch(page, "MOON")
    await settleFilters(page)

    const population = filter(page, "amount", PROPERTY_IDS.POPULATION_ESTIMATE)
    await expect(population, "the facet for the estimated population").toBeVisible()
    await expectFilterActive(page, population, false)
    const unfiltered = await countDigits(page.locator(".pd-searchresultsheader-count-results"))

    const missingCheckbox = population.locator(".pd-amountfiltersresult-checkbox-missing")
    await expect(missingCheckbox, "the checkbox of the missing row").toBeVisible()
    await missingCheckbox.click()
    await expectFacetBack(page, population)
    await expectFilterActive(page, population, true)

    // Unchecking leaves the filter without any selection, so it is removed from the search session
    // altogether and the clear button goes away with it.
    await expect(missingCheckbox, "the checkbox of the missing row after selecting it").toBeVisible()
    await missingCheckbox.click()
    await expectResultsCount(page, unfiltered)
    await expectFacetBack(page, population)
    await expectFilterActive(page, population, false)
    await expect(missingCheckbox, "the missing row is unselected again").not.toBeChecked()

    await checkpointFacet(page, "amount-filters-missing-facet-unchecked-again", population)

    console.log(`Successfully deselected the documents missing an amount property and got all ${unfiltered} back.`)
  })

  test("Test the missing row ticked together with a selected amount range", async ({ context }) => {
    const page = await context.newPage()

    // A range and the special rows of the same property are two filters of the search session rather than
    // one, and the two are OR-ed together (SpecialsFilter in search/filter_model.go), so one search can ask
    // for the documents measuring inside a range and the documents carrying no measurement at all. The
    // estimated population is the property which offers both, and it covers the fewest documents of the
    // moons' facets, so the panel is asked for every facet it has before the facet is looked for.
    await openClassSearch(page, "MOON")
    await settleFilters(page)

    const population = filter(page, "amount", PROPERTY_IDS.POPULATION_ESTIMATE)
    await expect(population, "the facet for the estimated population").toBeVisible()
    await expectFilterActive(page, population, false)
    const unfiltered = await countDigits(page.locator(".pd-searchresultsheader-count-results"))
    const withProperty = await countDigits(population.locator(".pd-filtersresult-header"))
    await expect(population.locator(".pd-amountfiltersresult-input-range"), "the range slider of the facet").toBeVisible()

    await tapRange(population, 0.3)
    await expectFilterActive(page, population, true)
    const inRange = await expectFewerResults(page, withProperty)

    // The count of the missing row is read while the range is selected, because that is the number the
    // reader sees beside the row at the moment they tick it.
    await expectFacetBack(page, population)
    const missingCheckbox = population.locator(".pd-amountfiltersresult-checkbox-missing")
    await expect(missingCheckbox, "the checkbox of the missing row").toBeVisible()
    await expect(missingCheckbox, "selecting a range leaves the missing row unselected").not.toBeChecked()
    const missing = await countDigits(population.locator(".pd-amountfiltersresult-count-missing"))

    // No document both measures inside the range and carries no measurement at all, so what the two of them
    // stand for adds up rather than overlapping.
    await missingCheckbox.click()
    await expectResultsCount(page, String(Number(inRange) + Number(missing)))
    await expectFacetBack(page, population)
    await expectFilterActive(page, population, true)
    await expect(missingCheckbox, "the missing row is checked beside the selected range").toBeChecked()

    // The range is still the one which was selected: ticking the row added documents to the search rather
    // than replacing the filter the facet already carried.
    await expect(population.locator(".pd-amountfiltersresult-label-from"), "the start of the selected range").toBeVisible()
    await expect(population.locator(".pd-amountfiltersresult-label-to"), "the end of the selected range").toBeVisible()

    await checkpointFacet(page, "amount-filters-missing-and-range-facet", population)

    console.log(
      `Successfully ticked the missing row of an amount facet beside a selected range: ${inRange} of ${unfiltered} documents fall in the range and ${missing} carry no measurement, ${Number(inRange) + Number(missing)} in all.`,
    )
  })

  test("Test the none, unknown and has property rows ticked together with a selected amount range", async ({ context }) => {
    const page = await context.newPage()

    // The estimated population is stated every way an amount can be stated, so its facet offers all four
    // rows beside the histogram. Each of them is a selection of the path's specials filter, which is OR-ed
    // with the range, so ticking them one after another only ever widens the search.
    await openClassSearch(page, "MOON")
    await settleFilters(page)

    const population = filter(page, "amount", PROPERTY_IDS.POPULATION_ESTIMATE)
    await expect(population, "the facet for the estimated population").toBeVisible()
    await expectFilterActive(page, population, false)
    const withProperty = await countDigits(population.locator(".pd-filtersresult-header"))

    await expect(population.locator(".pd-amountfiltersresult-row-missing"), "the row for the documents which state no estimate").toHaveCount(1)
    await expect(population.locator(".pd-amountfiltersresult-row-none"), "the row for the documents which state that there is no estimate").toHaveCount(1)
    await expect(population.locator(".pd-amountfiltersresult-row-unknown"), "the row for the documents whose estimate is unknown").toHaveCount(1)
    await expect(population.locator(".pd-amountfiltersresult-row-hasproperty"), "the row for the documents which state only that there is an estimate").toHaveCount(1)
    await checkpointFacet(page, "amount-filters-every-row-facet", population)

    await tapRange(population, 0.3)
    await expectFilterActive(page, population, true)
    let found = Number(await expectFewerResults(page, withProperty))
    const inRange = found

    // Each row is ticked in turn and the search grows by exactly what that row promised, because a document
    // of this data states the property in one way only and so is counted by one row only.
    const rows = [
      { kind: "none", what: "the documents which state that there is no estimate" },
      { kind: "unknown", what: "the documents whose estimate is unknown" },
      { kind: "hasproperty", what: "the documents which state only that there is an estimate" },
    ] as const
    const counts: Record<string, number> = {}
    for (const row of rows) {
      await expectFacetBack(page, population)
      const checkbox = population.locator(`.pd-amountfiltersresult-checkbox-${row.kind}`)
      await expect(checkbox, `the checkbox of the row for ${row.what}`).toBeVisible()
      await expect(checkbox, `the row for ${row.what} is not ticked yet`).not.toBeChecked()
      const promised = Number(await countDigits(population.locator(`.pd-amountfiltersresult-count-${row.kind}`)))
      expect(promised, `the row for ${row.what} stands for some of the documents`).toBeGreaterThan(0)
      counts[row.kind] = promised

      await checkbox.click()
      found += promised
      await expectResultsCount(page, String(found))
      await expectFacetBack(page, population)
      await expect(population.locator(`.pd-amountfiltersresult-checkbox-${row.kind}`), `the row for ${row.what} is ticked`).toBeChecked()
    }

    // All three are ticked at once, next to a range which is still the one which was selected.
    for (const row of rows) {
      await expect(population.locator(`.pd-amountfiltersresult-checkbox-${row.kind}`), `the row for ${row.what} is still ticked`).toBeChecked()
    }
    await expect(population.locator(".pd-amountfiltersresult-checkbox-missing"), "the row for the documents which state no estimate is untouched").not.toBeChecked()
    await expect(population.locator(".pd-amountfiltersresult-label-from"), "the start of the selected range").toBeVisible()
    await expect(population.locator(".pd-amountfiltersresult-label-to"), "the end of the selected range").toBeVisible()

    await checkpointFacet(page, "amount-filters-specials-and-range-facet", population)

    console.log(
      `Successfully ticked the none, unknown and has property rows of an amount facet beside a selected range: ${inRange} documents in the range, ${counts.none} with no estimate, ${counts.unknown} with an unknown estimate and ${counts.hasproperty} with only an estimate stated, ${found} in all.`,
    )
  })

  test("Test every amount facet the search offers", async ({ context }) => {
    const page = await context.newPage()

    await openAllDocumentsSearch(page)

    // The panel shows only its first facets and the rest on demand, so which facets the search offers at
    // all is asked of the API instead of counted on screen. The panel publishes the very URL it loaded them
    // from, so the answer is the one the panel is showing rather than one from a separately built request.
    const filtersUrl = await page.locator(".pd-searchresultsfeed-panel-filters").getAttribute("data-url")
    expect(filtersUrl, "the filters panel publishes the URL it loaded its facets from").toBeTruthy()
    const facets = JSON.parse((await fetchFromPage(page, filtersUrl!)).body) as Array<{ type: string; props?: Array<string>; unit?: string; count: number }>

    // Each facet is compared by its property path and its unit together, so a property which started being
    // measured in another unit, or stopped carrying one, fails here.
    const amountFacets = facets.filter((facet) => facet.type === "amount")
    expect(amountFacets.map(facetKey).sort(), "the amount facets the search offers").toEqual(AMOUNT_FACETS.map(facetKey).sort())

    for (const facet of amountFacets) {
      expect(facet.count, "a facet is offered because documents carry the property").toBeGreaterThan(0)
    }
    expect(amountFacets.filter((facet) => facet.unit === undefined).length, "the amount facets of properties which are counted rather than measured").toBe(
      AMOUNT_FACETS.filter((facet) => facet.unit === undefined).length,
    )
    expect(amountFacets.filter((facet) => (facet.props ?? []).length === 2).length, "the amount facets filtering on an amount inside a claim").toBe(
      AMOUNT_FACETS.filter((facet) => facet.props.length === 2).length,
    )

    // The catalogue records times as well as amounts, and those get facets of their own, which a file of
    // their own covers. Asserting that they are here keeps the two kinds of histogram facet apart: a change
    // which turned one into the other would fail here rather than pass unnoticed.
    expect(facets.filter((facet) => facet.type === "time").length, "the same search offers time facets next to the amount ones").toBeGreaterThan(0)

    await expectNoConsoleErrors(page)

    console.log(`Successfully verified all ${AMOUNT_FACETS.length} amount facets the search over every document offers.`)
  })

  for (const language of LANGUAGES) {
    test(`Test the amount facet in ${language}`, async ({ context }) => {
      const page = await context.newPage()

      await goHome(page)
      await switchLanguage(page, language)
      await openClassSearch(page, "STAR_SYSTEM")

      // Everything the facet renders is written in the language of the interface: the name of the property
      // it filters on, the name of the unit its amounts are measured in, and the edges of the span it
      // covers, which are numbers grouped the way the language groups them.
      const distance = filter(page, "amount", PROPERTY_IDS.DISTANCE_FROM_SOL)
      await expect(distance, "the facet for the distance from Sol").toBeVisible()
      await expect(distance.locator(".pd-filtersresult-title"), "the facet names the property it filters on").not.toHaveText(/^\s*$/)
      await expect(unitReference(distance), "the facet names the unit the distances are measured in").not.toHaveText(/^\s*$/)
      await expect(distance.locator(".pd-amountfiltersresult-label-from"), "the start of the span the histogram covers").not.toHaveText(/^\s*$/)
      await expect(distance.locator(".pd-amountfiltersresult-label-to"), "the end of the span the histogram covers").not.toHaveText(/^\s*$/)

      // A property which is counted rather than measured renders its name alone in every language.
      const starCount = filter(page, "amount", PROPERTY_IDS.STAR_COUNT)
      await expect(starCount, "the facet for the number of stars of a system").toBeVisible()
      await expect(unitReference(starCount), "the facet of a counted property names no unit").toHaveCount(0)

      await checkpointFacet(page, `amount-filters-facet-${language}`, distance)
      await checkpointElement(page, starCount, `amount-filters-counted-facet-${language}`)

      console.log(`Successfully verified an amount facet in ${language}.`)
    })
  }
})
