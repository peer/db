import type { Locator, Page } from "@playwright/test"

import { CLASS_IDS, coreDocumentIdOf, LANGUAGES, PROPERTY_IDS, searchByClass, searchByCoreClass } from "../peerdb_utils"
import {
  applyFilterValue,
  checkpoint,
  checkpointElement,
  documentId,
  expect,
  expectResults,
  fetchFromPage,
  filter,
  filterValue,
  goHome,
  LOADING_TIMEOUT,
  openDocumentTab,
  openFilters,
  openFirstResult,
  propertyRow,
  resultCount,
  resultIds,
  searchAgain,
  searchWithQuery,
  settle,
  settleDocument,
  settleFilters,
  switchLanguage,
  test,
  volatile,
} from "../utils"

// The synthetic identifiers a facet gives its special rows, which stand for a state of the property path
// rather than for a document, and the prefix a value's "direct" row carries in front of the identifier of
// the value it belongs to. They are the ids of src/utils.ts (MISSING_VALUE_ID and the others), which the
// facet builds its checkbox ids out of, so a test addresses a special row the same way it addresses a row
// standing for a document.
const MISSING = "__MISSING__"
const NONE = "__NONE__"
const UNKNOWN = "__UNKNOWN__"
const HAS_PROPERTY = "__HAS__"
const DIRECT = "__DIRECT__:"

// The classes whose documents state the biosphere property in all three ways: with a value, with the
// statement that there is no value (none) and with the statement that the value is unknown. The value is
// written as text, which a reference facet has no way to offer, so the facet on that property is made of
// special rows only: a document which states the property as text is one which states nothing the facet can
// offer, and so belongs to the missing row together with the documents which do not state it at all.
const WORLD_CLASSES = ["PLANET", "MOON"] as const

// The class the "direct" tests work over. A class states which class it is a subclass of, and the classes
// of the test data form a tree several levels deep, so the facet on that property has values which stand
// over other values, which is what makes a "direct" row appear next to them.
const CLASS_OF_CLASSES = "CLASS"

// The property whose facet carries a has property row. It is a core property of the schema itself: a class
// states it as a reference for some of its documents and as the bare statement of having it for others, and
// a facet grows a has property row exactly for a property which is stated both ways. It is not among the
// properties the test data declares, so its identifier is derived from the core namespace.
const SETTING = await coreDocumentIdOf("SETTING")

// The count a facet's row promises, read out of the parentheses the row renders it in, so a test can assert
// that selecting the row finds exactly as many documents as the row said it would. The count is grouped for
// the locale, so only its digits are read.
async function rowCount(count: Locator, what: string): Promise<number> {
  await expect(count, `the count of ${what}`).toBeVisible({ timeout: LOADING_TIMEOUT })
  const digits = ((await count.textContent()) || "").replace(/\D/g, "")
  expect(digits, `the count of ${what} is a number`).not.toBe("")
  return Number(digits)
}

// The label and the count of one special row of a reference facet, both of which are rendered as labels of
// the row's own checkbox and so are addressed by the id that checkbox carries.
function specialLabel(page: Page, props: Array<string>, value: string): Locator {
  return page.locator(`label[for="${["ref", ...props, value].join("/")}"].pd-reffiltertreerow-label-special`)
}

function specialCount(page: Page, props: Array<string>, value: string): Locator {
  return page.locator(`label[for="${["ref", ...props, value].join("/")}"].pd-reffiltertreerow-count-special`)
}

// The same for a "direct" row, which the facet renders with a label of its own rather than as one of the
// special rows, because it belongs to the value it is listed under.
function directLabel(page: Page, props: Array<string>, value: string): Locator {
  return page.locator(`label[for="${["ref", ...props, DIRECT + value].join("/")}"].pd-reffiltertreerow-label-direct`)
}

function directCount(page: Page, props: Array<string>, value: string): Locator {
  return page.locator(`label[for="${["ref", ...props, DIRECT + value].join("/")}"].pd-reffiltertreerow-count-direct`)
}

// Shows a facet which sits below the ones the panel shows at first: the panel is settled, which adds every
// facet it has and waits for its list to stop being replaced, and the facet is then asserted to be among
// them. A facet revealed without that wait can be taken away again by a list which arrives afterwards,
// because the panel starts its batches over whenever one does.
async function showFilter(page: Page, facet: Locator, what: string): Promise<void> {
  await settleFilters(page)
  await expect(facet, what).toBeVisible({ timeout: LOADING_TIMEOUT })
}

// How long one facet has to stay where it is before a screenshot of it is taken, and how often its box is
// read while waiting for that.
const FACET_SETTLED_READINGS = 4
const FACET_READING_INTERVAL = 500

// Waits until one facet of the filters panel has stopped moving.
//
// A screenshot of a single facet is a box on the page, so it is of that facet only for as long as the facet
// stays where the box was measured. Selecting a value sorts its facet to the top of the panel, and the panel
// learns that only when the facets it asked for after the search come back, which can be after everything
// else about the page has settled. The box is therefore read until it has been the same for a couple of
// seconds, rather than for two readings in a row.
async function settleFacet(page: Page, facet: Locator, what: string): Promise<void> {
  let previous: string | null = null
  let stable = 0
  await expect
    .poll(
      async () => {
        const box = await facet.boundingBox()
        const where = box === null ? "" : `${box.x},${box.y},${box.width},${box.height}`
        stable = where !== "" && where === previous ? stable + 1 : 0
        previous = where
        return stable
      },
      { message: `${what} stops moving`, timeout: LOADING_TIMEOUT, intervals: [FACET_READING_INTERVAL] },
    )
    .toBeGreaterThanOrEqual(FACET_SETTLED_READINGS)
}

// Types a term into the box which searches the filters and waits until the panel has answered it. What the
// panel ends up showing is what says the term was answered, so the panel is read until two readings agree.
async function searchFilters(page: Page, term: string): Promise<void> {
  const input = page.locator(".pd-searchresultsfeed-input-filters")
  await expect(input, "the box which searches the filters").toBeVisible()
  await input.fill(term)
  await expect(input, "the box holds the typed term").toHaveValue(term)

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
}

// Opens the search over the documents of one class of the test data with its filters panel showing, and
// hands back the facet on the biosphere property, which is the facet made of special rows only.
async function openBiosphereFacet(page: Page, entityClass: (typeof WORLD_CLASSES)[number]): Promise<Locator> {
  await searchByClass(page, entityClass)
  await openFilters(page)
  const biosphere = filter(page, "ref", PROPERTY_IDS.BIOSPHERE)
  await showFilter(page, biosphere, "the facet on the biosphere property")
  return biosphere
}

// Clears one filter through its own clear button and waits until the search it started has come back.
async function clearFilter(page: Page, applied: Locator): Promise<void> {
  const clearButton = applied.locator(".pd-filtersresult-button-clear")
  await expect(clearButton, "the clear button of the applied filter").toBeVisible()
  await searchAgain(page, async () => {
    await clearButton.click()
    await expect(clearButton, "the cleared facet no longer offers to be cleared").toHaveCount(0)
  })
}

// The claims a document makes about the biosphere property, counted by the kind of statement they are.
// The document is read back from the server as it is stored, because what a special row of a facet stands
// for is exactly a kind of claim, and no rendering of the document says which kind a claim is as plainly as
// the claim itself does.
interface BiosphereClaims {
  value: number
  none: number
  unknown: number
}

async function biosphereClaims(page: Page, id: string): Promise<BiosphereClaims> {
  const response = await fetchFromPage(page, `/api/d/${id}`)
  expect(response.status, `the document ${id} is readable`).toBe(200)
  const claims = (JSON.parse(response.body) as { claims: Record<string, Array<{ prop: { id: string } }>> }).claims
  const about = (kind: string) => (claims[kind] ?? []).filter((claim) => claim.prop.id === PROPERTY_IDS.BIOSPHERE).length
  return { value: about("html"), none: about("none"), unknown: about("unknown") }
}

// Selects one special row of the facet on the biosphere property, checks that the search found exactly what
// the row promised, reads the first document it found and then drops the filter again. What is passed as
// "states" is what that document has to say about the property for the row to mean what it says: the none
// row and the unknown row each stand for a claim of that kind, while the missing row stands for a document
// which makes neither, whether because it states the property as text or because it does not state it.
async function selectSpecialRow(page: Page, special: string, states: (claims: BiosphereClaims) => boolean, name: string): Promise<number> {
  const biosphere = await openBiosphereFacet(page, "MOON")
  const total = await resultCount(page)

  const promised = await rowCount(specialCount(page, [PROPERTY_IDS.BIOSPHERE], special), `the ${name} row`)
  expect(promised, `the ${name} row stands for some of the documents`).toBeGreaterThan(0)
  expect(promised, `the ${name} row stands for less than every document`).toBeLessThan(total)

  await applyFilterValue(page, biosphere, filterValue(page, "ref", [PROPERTY_IDS.BIOSPHERE], special))
  await expect(filterValue(page, "ref", [PROPERTY_IDS.BIOSPHERE], special), `the ${name} row is checked`).toBeChecked()
  await expect.poll(() => resultCount(page), { message: `the search finds what the ${name} row promised` }).toBe(promised)
  await settle(page)
  await settleFacet(page, biosphere, `the facet holding the ${name} row`)
  await checkpointElement(page, biosphere, `specials-${name}-applied-facet`)

  // What the row means is a statement of the documents themselves, so the first one the search found is
  // opened and read: its claims say whether the search really found what the row stands for, and the tab
  // listing every property it states is what a reader of the document would look at to see the same. The
  // address of the search is kept so it can be opened again afterwards: going back would only undo the
  // switch to that tab, which is a step of its own in the history of the browser.
  const filtered = page.url()
  const first = (await resultIds(page))[0]
  await openFirstResult(page)
  await settleDocument(page)
  expect(documentId(page), `the first result of the search filtered by the ${name} row`).toBe(first)
  await openDocumentTab(page, "allproperties")
  await expect(page.locator(".pd-documentget-panel-allproperties .pd-propertiesview-row").first(), "the properties of the document").toBeVisible()
  const claims = await biosphereClaims(page, first)
  expect(states(claims), `what the document the ${name} row stands for states about the property: ${JSON.stringify(claims)}`).toBe(true)
  await expect(propertyRow(page, PROPERTY_IDS.BIOSPHERE), "the tab of every property lists the property as often as the document states it").toHaveCount(
    claims.value + claims.none + claims.unknown,
  )
  await checkpoint(page, `specials-${name}-document`, { mask: volatile(page) })

  // The filter is part of the search session rather than of the address, so opening the search again brings
  // it back as it was left. The panel is back to the facets it shows on its own, so the facet is asked for
  // again before the filter it holds is dropped.
  await page.goto(filtered)
  await expectResults(page)
  await expect.poll(() => resultCount(page), { message: `the search opened again still finds what the ${name} row promised` }).toBe(promised)
  await openFilters(page)
  await showFilter(page, biosphere, "the facet after coming back from the document")
  await clearFilter(page, biosphere)
  // A facet with something selected is sorted to the top of the panel, so dropping the filter sends this one
  // back among the facets the panel does not show at first and it has to be asked for once more.
  await showFilter(page, biosphere, "the facet once its filter is dropped")
  await expect(filterValue(page, "ref", [PROPERTY_IDS.BIOSPHERE], special), `the ${name} row is not checked any more`).not.toBeChecked()
  await expect.poll(() => resultCount(page), { message: "the search is back to what it found unfiltered" }).toBe(total)

  return promised
}

test.describe("PeerDB Search Special Filter Values Flows", () => {
  for (const worldClass of WORLD_CLASSES) {
    test(`Test a facet made of special rows only over ${worldClass} documents`, async ({ context }) => {
      const page = await context.newPage()

      const biosphere = await openBiosphereFacet(page, worldClass)
      const total = await resultCount(page)

      // The property is stated as text, which no reference facet can offer as a value, so the facet offers
      // exactly the three rows which say how the property is stated rather than what it says: no row
      // standing for a document and no "direct" row either.
      await expect(biosphere.locator(".pd-reffiltertreerow"), "the facet offers three rows").toHaveCount(3)
      await expect(biosphere.locator(".pd-reffiltertreerow-label-special"), "every row of the facet is a special row").toHaveCount(3)
      await expect(biosphere.locator(".pd-reffiltertreerow-label"), "no row of the facet stands for a document").toHaveCount(0)
      await expect(biosphere.locator(".pd-reffiltertreerow-label-direct"), "no row of the facet is a direct row").toHaveCount(0)
      await expect(biosphere.locator(".pd-filtersresult-more"), "the facet has no further values to show").toHaveCount(0)

      const missing = await rowCount(specialCount(page, [PROPERTY_IDS.BIOSPHERE], MISSING), "the missing row")
      const none = await rowCount(specialCount(page, [PROPERTY_IDS.BIOSPHERE], NONE), "the none row")
      const unknown = await rowCount(specialCount(page, [PROPERTY_IDS.BIOSPHERE], UNKNOWN), "the unknown row")

      // The three rows partition the documents the search found: each document states the property in
      // exactly one of the three ways.
      expect(missing + none + unknown, "the three rows together account for every document the search found").toBe(total)
      await expect(biosphere.locator(".pd-filtersresult-button-clear"), "nothing is selected in the facet yet").toHaveCount(0)

      await settle(page)
      await settleFacet(page, biosphere, "the facet on the biosphere property")
      await checkpointElement(page, biosphere, `specials-facet-${worldClass}`)

      console.log(`Successfully read the facet of a property over ${total} ${worldClass} documents: ${missing} missing, ${none} none and ${unknown} unknown.`)
    })
  }

  test("Test filtering by the missing row of a facet", async ({ context }) => {
    const page = await context.newPage()

    // A document the missing row stands for says nothing the facet could have offered as a value: it either
    // states the property as text, which a reference facet has no way to list, or does not state it at all.
    const found = await selectSpecialRow(page, MISSING, (claims) => claims.none === 0 && claims.unknown === 0, "missing")

    console.log(`Successfully filtered by the missing row of a facet and found ${found} documents which state nothing the facet can offer.`)
  })

  test("Test filtering by the none row of a facet", async ({ context }) => {
    const page = await context.newPage()

    const found = await selectSpecialRow(page, NONE, (claims) => claims.none > 0, "none")

    console.log(`Successfully filtered by the none row of a facet and found ${found} documents which state that the property has no value.`)
  })

  test("Test filtering by the unknown row of a facet", async ({ context }) => {
    const page = await context.newPage()

    const found = await selectSpecialRow(page, UNKNOWN, (claims) => claims.unknown > 0, "unknown")

    console.log(`Successfully filtered by the unknown row of a facet and found ${found} documents whose value for the property is unknown.`)
  })

  test("Test filtering by the has property row of a facet", async ({ context }) => {
    const page = await context.newPage()

    // The property this is about is stated by some documents as a reference to another document and by
    // others as nothing but the bare statement that they have it, which is what puts a has property row next
    // to the values of its facet. Only a search over everything holds documents of both kinds, so the facet
    // is asked for batch by batch, the name of the row is read off it, and the row is then found again
    // through the box which searches the filters, which is how a reader would reach it without scrolling
    // through every facet the search offers.
    await searchWithQuery(page, "")
    await openFilters(page)
    const total = await resultCount(page)

    const setting = filter(page, "ref", SETTING)
    await showFilter(page, setting, "the facet on the setting property")
    const label = specialLabel(page, [SETTING], HAS_PROPERTY)
    await expect(label, "the has property row of the facet").toBeVisible()
    const term = ((await label.textContent()) || "").trim()
    expect(term, "the name the has property row is listed under").not.toBe("")

    await searchFilters(page, term)
    await expect(setting, "the facet holding the has property row is found by its name").toBeVisible()
    await expect(setting.locator(".pd-reffiltertreerow"), "the facet is narrowed to the row which was searched for").toHaveCount(1)
    await expect(label, "the has property row is the row which is left").toBeVisible()

    const promised = await rowCount(specialCount(page, [SETTING], HAS_PROPERTY), "the has property row")
    await applyFilterValue(page, setting, filterValue(page, "ref", [SETTING], HAS_PROPERTY))
    await expect(filterValue(page, "ref", [SETTING], HAS_PROPERTY), "the has property row is checked").toBeChecked()
    await expect.poll(() => resultCount(page), { message: "the search finds what the has property row promised" }).toBe(promised)
    expect(promised, "the row stands for less than every document").toBeLessThan(total)
    await settle(page)
    await settleFacet(page, setting, "the facet on the setting property")
    await checkpointElement(page, setting, "specials-hasproperty-applied-facet")
    await settleFilters(page)
    await checkpoint(page, "specials-hasproperty-applied", { mask: volatile(page) })

    await clearFilter(page, setting)
    await expect(filterValue(page, "ref", [SETTING], HAS_PROPERTY), "the has property row is not checked any more").not.toBeChecked()
    await expect.poll(() => resultCount(page), { message: "the search is back to what it found unfiltered" }).toBe(total)

    console.log(`Successfully filtered ${total} documents by the has property row of a facet and found the ${promised} which state the property and nothing more.`)
  })

  test("Test a special row ticked together with a value of the same facet", async ({ context }) => {
    const page = await context.newPage()

    // The special rows of a property and its values are two filters of the search session rather than one,
    // and the two are OR-ed together (SpecialsFilter in search/filter_model.go), so a facet can be narrowed
    // to one of its values and to a state of the property at once. The missing row is the state used here:
    // it stands for the documents which state nothing the facet could offer, so no document it counts states
    // the value ticked next to it and the two counts add up exactly.
    await searchWithQuery(page, "")
    await openFilters(page)

    const setting = filter(page, "ref", SETTING)
    await showFilter(page, setting, "the facet on the setting property")

    // Which value is ticked is read off the facet rather than written down here, so the test follows the
    // data rather than one document of it: the first row standing for a document is the one which is used.
    const valueLabel = setting.locator(".pd-reffiltertreerow-label").first()
    await expect(valueLabel, "the first row of the facet standing for a document").toBeVisible()
    const valueFor = await valueLabel.getAttribute("for")
    expect(valueFor, "the row standing for a document names the checkbox it belongs to").toBeTruthy()
    const valueRow = page.locator(`[id="${valueFor}"]`)
    const valueCount = await rowCount(page.locator(`label[for="${valueFor}"].pd-reffiltertreerow-count`), "the row standing for a document")

    const missingRow = filterValue(page, "ref", [SETTING], MISSING)
    await expect(missingRow, "the missing row of the facet").toBeVisible()

    // The value on its own finds what its row promised, which is what the row next to it is then added to.
    await applyFilterValue(page, setting, valueRow)
    await expect(valueRow, "the value is checked").toBeChecked()
    await expect.poll(() => resultCount(page), { message: "the search finds what the value promised" }).toBe(valueCount)

    // The count of the missing row is read while the value is ticked, because that is the number the reader
    // sees next to the row at the moment they tick it.
    const missingCount = await rowCount(specialCount(page, [SETTING], MISSING), "the missing row")
    expect(missingCount, "the missing row stands for some of the documents").toBeGreaterThan(0)

    await applyFilterValue(page, setting, missingRow)
    await expect(valueRow, "the value is still checked next to the special row").toBeChecked()
    await expect(missingRow, "the special row is checked next to the value").toBeChecked()
    await expect
      .poll(() => resultCount(page), { message: "the search finds the documents of the value together with the documents of the missing row" })
      .toBe(valueCount + missingCount)

    await settle(page)
    await settleFacet(page, setting, "the facet on the setting property")
    await checkpointElement(page, setting, "specials-value-and-missing-facet")

    // Clearing the facet drops both of them, because the two filters belong to the one property the facet
    // is of. A facet with something selected is sorted to the top of the panel, so dropping its filters
    // sends this one back among the facets the panel does not show at first and it has to be asked for once
    // more before what it holds can be read.
    await clearFilter(page, setting)
    await showFilter(page, setting, "the facet once its filters are dropped")
    await expect(valueRow, "the value is not checked any more").not.toBeChecked()
    await expect(missingRow, "the special row is not checked any more").not.toBeChecked()

    console.log(
      `Successfully ticked a value of a facet together with its missing row: ${valueCount} documents state the value, ${missingCount} state nothing the facet offers, ${valueCount + missingCount} in all.`,
    )
  })

  test("Test filtering by the direct row of a facet", async ({ context }) => {
    const page = await context.newPage()

    // A class of the test data is a subclass of another class, and the classes form a tree, so the facet on
    // that property lists a class together with the classes under it. Its direct row is what separates the
    // documents which state that very class from the ones which state a class under it.
    await searchByCoreClass(page, CLASS_OF_CLASSES)
    await openFilters(page)
    const total = await resultCount(page)
    const subclassOf = filter(page, "ref", PROPERTY_IDS.SUBCLASS_OF)
    await showFilter(page, subclassOf, "the facet on the class a class is under")

    const placeRow = filterValue(page, "ref", [PROPERTY_IDS.SUBCLASS_OF], CLASS_IDS.PLACE)
    const worldRow = filterValue(page, "ref", [PROPERTY_IDS.SUBCLASS_OF], CLASS_IDS.WORLD)
    const directRow = filterValue(page, "ref", [PROPERTY_IDS.SUBCLASS_OF], DIRECT + CLASS_IDS.PLACE)
    await expect(placeRow, "the value which stands over the others").toBeVisible()
    await expect(worldRow, "a value under it").toBeVisible()
    await expect(directRow, "the direct row of the value which stands over the others").toBeVisible()

    const whole = await rowCount(
      page.locator(`label[for="${["ref", PROPERTY_IDS.SUBCLASS_OF, CLASS_IDS.PLACE].join("/")}"].pd-reffiltertreerow-count`),
      "the value which stands over the others",
    )
    const promised = await rowCount(directCount(page, [PROPERTY_IDS.SUBCLASS_OF], CLASS_IDS.PLACE), "the direct row of the value")
    expect(promised, "the direct row stands for fewer documents than the value it belongs to").toBeLessThan(whole)

    await applyFilterValue(page, subclassOf, directRow)

    // Only the direct row is selected: the value it belongs to holds more than it, so that value is only
    // partly selected, and the values under it are not selected at all.
    await expect(directRow, "the direct row is checked").toBeChecked()
    await expect(worldRow, "the values under the one the direct row belongs to are not checked").not.toBeChecked()
    await expect(placeRow, "the value the direct row belongs to is not fully checked").not.toBeChecked()
    expect(await placeRow.evaluate((el: HTMLInputElement) => el.indeterminate), "the value the direct row belongs to is partly checked").toBe(true)
    await expect.poll(() => resultCount(page), { message: "the search finds what the direct row promised" }).toBe(promised)
    await settle(page)
    await settleFacet(page, subclassOf, "the facet on the class a class is under")
    await checkpointElement(page, subclassOf, "specials-direct-applied-facet")
    await settleFilters(page)
    await checkpoint(page, "specials-direct-applied", { mask: volatile(page) })

    await clearFilter(page, subclassOf)
    await expect(directRow, "the direct row is not checked any more").not.toBeChecked()
    await expect.poll(() => resultCount(page), { message: "the search is back to what it found unfiltered" }).toBe(total)

    console.log(`Successfully filtered by the direct row of a facet and found the ${promised} documents of the ${whole} under that value which state it directly.`)
  })

  test("Test finding the direct row of a facet by its name", async ({ context }) => {
    const page = await context.newPage()

    await searchByCoreClass(page, CLASS_OF_CLASSES)
    await openFilters(page)
    const subclassOf = filter(page, "ref", PROPERTY_IDS.SUBCLASS_OF)
    await showFilter(page, subclassOf, "the facet on the class a class is under")
    const label = directLabel(page, [PROPERTY_IDS.SUBCLASS_OF], CLASS_IDS.PLACE)
    await expect(label, "the direct row of the value which stands over the others").toBeVisible()
    const term = ((await label.textContent()) || "").trim()
    expect(term, "the name the direct row is listed under").not.toBe("")

    await searchFilters(page, term)

    // Every facet which is left holds a direct row, and each of them is narrowed to that row together with
    // the value it belongs to, which has to stay so that the row has something to be listed under.
    const facets = page.locator(".pd-filtersresult")
    const direct = page.locator(".pd-reffiltertreerow-label-direct")
    await expect(subclassOf, "the facet on the class a class is under is one of them").toBeVisible()
    // The panel is still answering while this is read, so the two counts are compared as they settle rather
    // than one after the other: a facet which arrives between the two reads would otherwise be counted once.
    await expect
      .poll(async () => (await facets.count()) > 0 && (await facets.count()) === (await direct.count()), {
        message: "every facet which is left shows a direct row",
        timeout: LOADING_TIMEOUT,
      })
      .toBe(true)
    const left = await facets.count()
    await expect(subclassOf.locator(".pd-reffiltertreerow-label-special"), "no special row is left in the facet").toHaveCount(0)
    await expect(label, "the direct row is listed under the name it was searched by").toHaveText(term)
    await checkpoint(page, "specials-direct-searched", { mask: volatile(page) })

    await searchFilters(page, "")
    await expect(page.locator(".pd-filtersresult"), "the panel shows more than the facets holding a direct row again").not.toHaveCount(left)

    console.log(`Successfully found the direct rows of ${left} facets by typing the name they are listed under into the filter search box.`)
  })

  for (const language of LANGUAGES) {
    test(`Test finding the special rows of a facet by their name in ${language}`, async ({ context }) => {
      const page = await context.newPage()

      await goHome(page)
      await switchLanguage(page, language)

      // The search is started from the home page and then narrowed through the class facet, rather than
      // opened by an address which says which class to show. A session started here is created by the
      // interface, which gives it the language the interface is in, and it is that language the names of
      // the special rows are matched in.
      await searchWithQuery(page, "")
      await openFilters(page)
      const instanceOf = filter(page, "ref", PROPERTY_IDS.INSTANCE_OF)
      await expect(instanceOf, "the class facet").toBeVisible()
      await applyFilterValue(page, instanceOf, filterValue(page, "ref", [PROPERTY_IDS.INSTANCE_OF], CLASS_IDS.MOON))
      const unfiltered = await resultCount(page)
      const biosphere = filter(page, "ref", PROPERTY_IDS.BIOSPHERE)
      await showFilter(page, biosphere, "the facet on the biosphere property")

      // The names the special rows are listed under are the ones the interface renders in the chosen
      // language, and the box which searches the filters finds them by exactly those names, so each name is
      // read off the row it belongs to and then typed back in.
      for (const special of [NONE, UNKNOWN]) {
        const label = specialLabel(page, [PROPERTY_IDS.BIOSPHERE], special)
        await expect(label, `the ${special} row in ${language}`).toBeVisible()
        const term = ((await label.textContent()) || "").trim()
        expect(term, `the name the ${special} row is listed under in ${language}`).not.toBe("")

        await searchFilters(page, term)

        // The facet is kept and narrowed to the row which was searched for. Which other facets are kept
        // depends on the language, because a name of a row can also be the start of the name of a value:
        // what has to hold in every language is that the row is found and that its facet holds nothing but
        // it.
        await expect(biosphere, `the facet holding the ${special} row is shown`).toBeVisible()
        await expect(biosphere.locator(".pd-reffiltertreerow"), `the facet is narrowed to the ${special} row`).toHaveCount(1)
        await expect(label, `the ${special} row is listed under the name it was searched by`).toHaveText(term)
        await expect.poll(() => resultCount(page), { message: "searching the filters leaves the search itself alone" }).toBe(unfiltered)
        await checkpoint(page, `specials-search-${special === NONE ? "none" : "unknown"}-${language}`, { mask: volatile(page) })

        // Dropping the term brings every facet back, which puts the panel back to the ones it shows on its
        // own, so this facet has to be asked for again before the next name is read off it.
        await searchFilters(page, "")
        await showFilter(page, biosphere, "the facet once the term is dropped")
      }

      // The row standing for the documents which say nothing about a property is one every facet can have,
      // so searching for its name leaves more than one facet, this one among them.
      const missingLabel = specialLabel(page, [PROPERTY_IDS.BIOSPHERE], MISSING)
      await expect(missingLabel, `the missing row in ${language}`).toBeVisible()
      const missingTerm = ((await missingLabel.textContent()) || "").trim()
      await searchFilters(page, missingTerm)
      await expect(page.locator(".pd-filtersresult").first(), "the panel keeps the facets which hold a missing row").toBeVisible()
      expect(await page.locator(".pd-filtersresult").count(), "more than one facet holds a missing row").toBeGreaterThan(1)
      await expect(biosphere, "the facet on the biosphere property is one of them").toBeVisible()
      await expect(biosphere.locator(".pd-reffiltertreerow"), "that facet is narrowed to its missing row").toHaveCount(1)
      await checkpoint(page, `specials-search-missing-${language}`, { mask: volatile(page) })

      console.log(`Successfully found the special rows of a facet over ${unfiltered} documents by their names in ${language}.`)
    })
  }
})
