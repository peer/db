import type { Page } from "@playwright/test"

import { documentIdOf, LANGUAGES, PROPERTY_IDS, searchByClass } from "../peerdb_utils"
import {
  checkpoint,
  expect,
  expectNothingLoading,
  expectResults,
  goHome,
  openSortDialog,
  resultCount,
  resultIds,
  searchAgain,
  searchWithQuery,
  settleFilters,
  sortColumn,
  switchLanguage,
  test,
  volatile,
} from "../utils"

// The suffix the CSS classes of a sortable column end in: the column type alone for the three columns the
// dialog always offers, and the type followed by the property identifier for a column which comes from a
// facet of the search (colClass in SearchSortDialog.vue). An amount facet without a unit, which is what the
// staff count of an institute is, carries no unit in its CSS class either.
const RELEVANCE_COLUMN = "score"
const TIME_COLUMN = "time"
const LABEL_COLUMN = "label"
const BUILTIN_COLUMNS = [RELEVANCE_COLUMN, TIME_COLUMN, LABEL_COLUMN]
const INSTANCE_OF_COLUMN = `ref-${PROPERTY_IDS.INSTANCE_OF}`
const LOCATED_AT_COLUMN = `ref-${PROPERTY_IDS.LOCATED_AT}`
const FOUNDED_COLUMN = `time-${PROPERTY_IDS.FOUNDED}`
const STAFF_COUNT_COLUMN = `amount-${PROPERTY_IDS.STAFF_COUNT}`

// The class whose documents these tests reorder. Every institute of the catalogue fits on one page of results,
// so the feed shows all of them at once and the whole list can be both read back and screenshotted after each
// change, and an institute carries a reference, a time and an amount, each of which the dialog then offers as
// a sort column of its own.
const SEARCH_CLASS = "INSTITUTE"

// The query used where sorting has to be tested against real relevance: a search over the whole catalogue
// ranks its results by how well they match, while a search which only names a class matches every one of
// them equally.
const QUERY = "weir"

// The institutes of the test data in the order they were founded, which is the order the founded column puts
// them in. The years are the ones the "founded" field of each institute file in testdata records.
const FOUNDED_ORDER = ["INST_ANCHOR", "INST_LEDGER", "INST_EVENING", "INST_IELUARO", "INST_FARSIGHT", "INST_CANOPY", "INST_SUBSTRATE", "INST_BEACON"]

// Adds the given column to the sort order and waits for the results the change produced.
async function addSortColumn(page: Page, column: string): Promise<void> {
  const button = page.locator(`.pd-searchsortdialog-button-add-${column}`)
  await expect(button, `the button which adds the ${column} column`).toBeVisible()
  await searchAgain(page, async () => await button.click())
}

// Removes the given column from the sort order and waits for the results the change produced.
async function removeSortColumn(page: Page, column: string): Promise<void> {
  const button = sortColumn(page, column).locator(".pd-searchsortdialog-button-remove")
  await expect(button, `the button which removes the ${column} column`).toBeVisible()
  await searchAgain(page, async () => await button.click())
}

// Moves the given column one place up or down in the sort order and waits for the results the change
// produced.
async function moveSortColumn(page: Page, column: string, direction: "up" | "down"): Promise<void> {
  const button = sortColumn(page, column).locator(`.pd-searchsortdialog-button-move${direction}`)
  await expect(button, `the button which moves the ${column} column ${direction}`).toBeVisible()
  await expect(button, `the button which moves the ${column} column ${direction} is usable`).toBeEnabled()
  await searchAgain(page, async () => await button.click())
}

// Flips the given column between ascending and descending and waits for the results the change produced.
async function flipSortDirection(page: Page, column: string): Promise<void> {
  const button = sortColumn(page, column).locator(".pd-searchsortdialog-button-direction")
  await expect(button, `the button which flips the direction of the ${column} column`).toBeVisible()
  await searchAgain(page, async () => await button.click())
}

// Asserts that the sort order consists of exactly the given columns, in the given order. The columns are
// identified by the CSS class each entry carries and not by their labels, which differ between languages.
async function expectSortOrder(page: Page, columns: Array<string>): Promise<void> {
  const items = page.locator(".pd-searchsortdialog-item-sort")
  await expect(items, "the number of columns sorted by").toHaveCount(columns.length)
  for (const [i, column] of columns.entries()) {
    await expect(items.nth(i), `the column at position ${i} of the sort order`).toHaveClass(new RegExp(`(^|\\s)pd-searchsortdialog-item-sort-${column}(\\s|$)`))
  }
}

// Closes the sort dialog so that the results behind it are shown unobstructed.
async function closeSortDialog(page: Page): Promise<void> {
  const closeButton = page.locator(".pd-searchsortdialog-button-close")
  await expect(closeButton, "the button which closes the sort dialog").toBeVisible()
  await closeButton.click()
  await expect(page.locator(".pd-searchsortdialog-panel"), "the closed sort dialog").toBeHidden()
}

// Screenshots the dialog itself. The screenshot covers the viewport rather than the whole page, because the
// dialog is drawn over the page and only dims what is behind it. A column which comes from a facet is named
// by the property it sorts on, which the dialog fetches, so the capture waits until every label on the page
// has resolved.
async function checkpointDialog(page: Page, name: string): Promise<void> {
  await expect(page.locator(".pd-searchsortdialog-panel"), "the sort dialog").toBeVisible()
  await expectNothingLoading(page)
  await checkpoint(page, name, { mask: volatile(page), fullPage: false })
}

// Screenshots the whole list of results the current sort order produces, which is what shows that the order
// really changed. The dialog is closed for it, both because it covers the results and because a full page
// screenshot is only stable without it, and opened again afterwards for the next change.
async function checkpointResults(page: Page, name: string): Promise<void> {
  await closeSortDialog(page)
  await settleFilters(page)
  await checkpoint(page, name, { mask: volatile(page) })
  await openSortDialog(page)
}

// The identifiers of the institutes of the test data, in the order the founded column puts them in.
async function foundedOrder(): Promise<Array<string>> {
  return await Promise.all(FOUNDED_ORDER.map(async (key) => await documentIdOf(SEARCH_CLASS, key)))
}

test.describe("PeerDB Search Sorting Flows", () => {
  test("Test the sort dialog offers every sortable column", async ({ context }) => {
    const page = await context.newPage()

    await searchByClass(page, SEARCH_CLASS)
    await settleFilters(page)
    await checkpoint(page, "sorting-results-unsorted", { mask: volatile(page) })
    const facets = await page.locator(".pd-filtersresult").count()
    expect(facets, "the search offers facets to sort by").toBeGreaterThan(0)

    await openSortDialog(page)

    // Nothing has been sorted by yet, so the sort order is empty and every column is on offer.
    await expect(page.locator(".pd-searchsortdialog-title"), "the dialog is headed by its own title").toBeVisible()
    await expect(page.locator(".pd-searchsortdialog-header-sort"), "the heading of the sort order").toBeVisible()
    await expect(page.locator(".pd-searchsortdialog-empty"), "the message saying that nothing is sorted by").toBeVisible()
    await expect(page.locator(".pd-searchsortdialog-item-sort"), "the empty sort order").toHaveCount(0)
    await expect(page.locator(".pd-searchsortdialog-header-available"), "the heading of the columns which can be added").toBeVisible()
    await expect(page.locator(".pd-searchsortdialog-list-available"), "the list of the columns which can be added").toBeVisible()

    // The three columns the dialog always offers, and one for each facet of the search: a reference, a time
    // and an amount among them.
    for (const column of [...BUILTIN_COLUMNS, INSTANCE_OF_COLUMN, LOCATED_AT_COLUMN, FOUNDED_COLUMN, STAFF_COUNT_COLUMN]) {
      await expect(page.locator(`.pd-searchsortdialog-button-add-${column}`), `the ${column} column is offered`).toBeVisible()
      await expect(page.locator(`.pd-searchsortdialog-item-available-${column} .pd-searchsortdialog-label-available`), `the ${column} column is named`).not.toHaveText(
        /^\s*$/,
      )
    }
    // Every facet of this search sorts on a value of its own, so the dialog offers a column for each of them
    // next to the three it always offers, and nothing else.
    await expect(page.locator(".pd-searchsortdialog-button-add"), "the number of columns offered").toHaveCount(BUILTIN_COLUMNS.length + facets)
    await checkpointDialog(page, "sorting-dialog-empty")

    console.log(`Successfully opened the sort dialog and verified that it offers ${BUILTIN_COLUMNS.length} built-in columns and one for each of the ${facets} facets.`)
  })

  test("Test adding and removing sort columns", async ({ context }) => {
    const page = await context.newPage()

    await searchByClass(page, SEARCH_CLASS)
    const unsorted = await resultIds(page)
    expect(unsorted.length, "the results to reorder").toBeGreaterThan(1)

    await openSortDialog(page)

    // Sorting by the display label reorders the results and takes the column out of the offered ones.
    await addSortColumn(page, LABEL_COLUMN)
    await expectSortOrder(page, [LABEL_COLUMN])
    await expect(page.locator(`.pd-searchsortdialog-button-add-${LABEL_COLUMN}`), "a column which is sorted by is not offered again").toHaveCount(0)
    const byLabel = await resultIds(page)
    expect(byLabel, "sorting by the display label reorders the results").not.toEqual(unsorted)
    expect([...byLabel].sort(), "sorting changes the order of the results and not which documents were found").toEqual([...unsorted].sort())
    await checkpointDialog(page, "sorting-dialog-with-label")
    await checkpointResults(page, "sorting-results-by-label")

    // A column added while one is already there goes after it and only breaks its ties. No two institutes
    // share a display label, so the first column alone decides and the results stay where they were.
    await addSortColumn(page, FOUNDED_COLUMN)
    await expectSortOrder(page, [LABEL_COLUMN, FOUNDED_COLUMN])
    expect(await resultIds(page), "a secondary column only breaks the ties of the column before it").toEqual(byLabel)
    await checkpointDialog(page, "sorting-dialog-with-label-and-founded")

    // Removing the primary column leaves the secondary one ordering the results on its own, and offers the
    // removed column again.
    await removeSortColumn(page, LABEL_COLUMN)
    await expectSortOrder(page, [FOUNDED_COLUMN])
    await expect(page.locator(`.pd-searchsortdialog-button-add-${LABEL_COLUMN}`), "a removed column is offered again").toBeVisible()
    expect(await resultIds(page), "the remaining column orders the results on its own").not.toEqual(byLabel)
    await checkpointResults(page, "sorting-results-by-founded")

    // Removing the last column leaves the search in the order it was in before anything was sorted by.
    await removeSortColumn(page, FOUNDED_COLUMN)
    await expectSortOrder(page, [])
    await expect(page.locator(".pd-searchsortdialog-empty"), "the sort order is empty again").toBeVisible()
    expect(await resultIds(page), "removing every column restores the order the search started in").toEqual(unsorted)

    console.log(`Successfully added two sort columns to a search of ${unsorted.length} documents, removed both, and verified how each change reordered them.`)
  })

  test("Test moving a sort column up and down", async ({ context }) => {
    const page = await context.newPage()

    await searchByClass(page, SEARCH_CLASS)
    await openSortDialog(page)

    await addSortColumn(page, LABEL_COLUMN)
    await addSortColumn(page, LOCATED_AT_COLUMN)
    await expectSortOrder(page, [LABEL_COLUMN, LOCATED_AT_COLUMN])
    const byLabel = await resultIds(page)

    // A column at an end of the sort order cannot be moved past it.
    await expect(sortColumn(page, LABEL_COLUMN).locator(".pd-searchsortdialog-button-moveup"), "the first column cannot be moved up").toBeDisabled()
    await expect(sortColumn(page, LOCATED_AT_COLUMN).locator(".pd-searchsortdialog-button-movedown"), "the last column cannot be moved down").toBeDisabled()
    await checkpointDialog(page, "sorting-dialog-before-move")
    await checkpointResults(page, "sorting-results-before-move")

    // Moving the second column up makes it the primary one, which orders the results by it first.
    await moveSortColumn(page, LOCATED_AT_COLUMN, "up")
    await expectSortOrder(page, [LOCATED_AT_COLUMN, LABEL_COLUMN])
    const byLocatedAt = await resultIds(page)
    expect(byLocatedAt, "the column which was moved up orders the results").not.toEqual(byLabel)
    await checkpointDialog(page, "sorting-dialog-moved-up")
    await checkpointResults(page, "sorting-results-moved-up")

    // Moving it back down restores both the sort order and the results it produced.
    await moveSortColumn(page, LOCATED_AT_COLUMN, "down")
    await expectSortOrder(page, [LABEL_COLUMN, LOCATED_AT_COLUMN])
    expect(await resultIds(page), "moving the column back restores the order it had before").toEqual(byLabel)
    await closeSortDialog(page)
    await settleFilters(page)
    await checkpoint(page, "sorting-results-moved-down", { mask: volatile(page) })

    console.log(`Successfully moved a sort column up and down and verified that ${byLabel.length} results followed it both times.`)
  })

  test("Test flipping a sort column between ascending and descending", async ({ context }) => {
    const page = await context.newPage()

    await searchByClass(page, SEARCH_CLASS)
    await openSortDialog(page)

    // A column which comes from a facet is added ascending.
    await addSortColumn(page, FOUNDED_COLUMN)
    await expectSortOrder(page, [FOUNDED_COLUMN])
    const ascending = await resultIds(page)
    await checkpointResults(page, "sorting-results-ascending")

    // Every institute states one founding date, so flipping the direction turns the order around exactly,
    // rather than only sorting from the other end of a set of values.
    await flipSortDirection(page, FOUNDED_COLUMN)
    const descending = await resultIds(page)
    expect(descending, "sorting descending reverses an order in which every document has one value").toEqual([...ascending].reverse())
    await checkpointDialog(page, "sorting-dialog-descending")
    await checkpointResults(page, "sorting-results-descending")

    // Flipping it back brings the first order back.
    await flipSortDirection(page, FOUNDED_COLUMN)
    expect(await resultIds(page), "flipping the direction back restores the first order").toEqual(ascending)
    await closeSortDialog(page)
    await settleFilters(page)
    await checkpoint(page, "sorting-results-ascending-again", { mask: volatile(page) })

    console.log(`Successfully flipped a sort column between ascending and descending and back over ${ascending.length} results.`)
  })

  test("Test sorting by a time column follows the recorded dates", async ({ context }) => {
    const page = await context.newPage()

    await searchByClass(page, SEARCH_CLASS)
    await openSortDialog(page)

    // The founding dates of the institutes are all different, so the column puts them in exactly one order,
    // which is the order the test data records them in. A document which states no date at all sorts after
    // every document which states one, so the institutes of the test data lead the results whatever else the
    // catalogue has gained.
    await addSortColumn(page, FOUNDED_COLUMN)
    const expected = await foundedOrder()
    const ids = await resultIds(page)
    expect(ids.slice(0, expected.length), "the results follow the dates the test data records").toEqual(expected)
    await checkpointResults(page, "sorting-results-by-time-column")

    console.log(`Successfully sorted by a time column and verified that the ${expected.length} institutes came back in the order they were founded.`)
  })

  test("Test sorting by a reference column", async ({ context }) => {
    const page = await context.newPage()

    await searchByClass(page, SEARCH_CLASS)
    const unsorted = await resultIds(page)

    await openSortDialog(page)

    await addSortColumn(page, LABEL_COLUMN)
    const byLabel = await resultIds(page)
    await removeSortColumn(page, LABEL_COLUMN)

    // A reference column sorts on the document each result points at rather than on the result itself, so it
    // orders the results by something neither the display label nor the default order follows.
    await addSortColumn(page, LOCATED_AT_COLUMN)
    await expectSortOrder(page, [LOCATED_AT_COLUMN])
    const ascending = await resultIds(page)
    expect(ascending, "a reference column orders the results by the documents they point at").not.toEqual(byLabel)
    expect(ascending, "a reference column orders the results differently from the order the search started in").not.toEqual(unsorted)
    expect([...ascending].sort(), "sorting by a reference column changes the order and not which documents were found").toEqual([...unsorted].sort())
    await checkpointDialog(page, "sorting-dialog-by-reference")
    await checkpointResults(page, "sorting-results-by-reference")

    // The sort order is part of the search session, so asking for the same session again shows the same
    // results in the same order rather than the order the session started in.
    await closeSortDialog(page)
    await page.reload()
    await expectResults(page)
    expect(await resultIds(page), "the session keeps the sort order across a reload").toEqual(ascending)

    // Descending is not the ascending order read backwards. A reference column sorts on the sort key of the
    // whole path of the value, which reaches the documents the value is itself contained in, and a document
    // with several values on that path is placed by the smallest of them when ascending and by the largest
    // when descending.
    await openSortDialog(page)
    await flipSortDirection(page, LOCATED_AT_COLUMN)
    const descending = await resultIds(page)
    expect(descending, "flipping a reference column reorders the results").not.toEqual(ascending)
    expect([...descending].sort(), "flipping the direction changes the order and not which documents were found").toEqual([...ascending].sort())
    await checkpointResults(page, "sorting-results-by-reference-descending")

    console.log(`Successfully sorted ${ascending.length} results by a reference column in both directions and verified that the session kept the order.`)
  })

  test("Test sorting by relevance", async ({ context }) => {
    const page = await context.newPage()

    await searchByClass(page, SEARCH_CLASS)
    await openSortDialog(page)

    // The search names a class instead of asking for words, so every result matches it equally well and the
    // relevance column leaves every one of them tied. What orders them is then the document identifier, which
    // every sort ends in so that the order is a total one (idTieBreak in search/sort.go).
    await addSortColumn(page, RELEVANCE_COLUMN)
    await expectSortOrder(page, [RELEVANCE_COLUMN])
    const byRelevance = await resultIds(page)
    expect(byRelevance, "results which tie on relevance come back ordered by their identifier").toEqual([...byRelevance].sort())
    await checkpointDialog(page, "sorting-dialog-by-relevance")
    await checkpointResults(page, "sorting-results-by-relevance")

    // The tie-breaker is ascending whichever way the column above it is sorted, so flipping a column every
    // result ties on changes nothing at all.
    await flipSortDirection(page, RELEVANCE_COLUMN)
    expect(await resultIds(page), "flipping a column every result ties on leaves the tie-breaker deciding").toEqual(byRelevance)

    console.log(`Successfully sorted ${byRelevance.length} results by relevance and verified that the identifier broke every tie.`)
  })

  test("Test the identifier tie-breaker keeps tied results in a stable order", async ({ context }) => {
    const page = await context.newPage()

    await searchByClass(page, SEARCH_CLASS)
    await openSortDialog(page)

    // Every result of this search is an instance of the same class, so a column sorting on that reference
    // gives every one of them the same value and the identifier alone decides where each lands.
    await addSortColumn(page, INSTANCE_OF_COLUMN)
    await expectSortOrder(page, [INSTANCE_OF_COLUMN])
    const tied = await resultIds(page)
    expect(tied.length, "the tied results").toBeGreaterThan(1)
    expect(tied, "results which tie on the column sorted by come back ordered by their identifier").toEqual([...tied].sort())
    await closeSortDialog(page)

    // Asking for the same search again has to give the same order back. An order which rests on nothing but
    // the tie is where an index which returns its documents in whatever order it happens to hold them would
    // show, so the same search is run twice more.
    for (const attempt of [1, 2]) {
      await page.reload()
      await expectResults(page)
      expect(await resultIds(page), `the order of the tied results on attempt ${attempt}`).toEqual(tied)
    }

    console.log(`Successfully verified that ${tied.length} results which tie on the column sorted by keep the same order across three searches.`)
  })

  test("Test sorting a text query by the display label", async ({ context }) => {
    const page = await context.newPage()

    // A search which asks for words ranks its results by how well each matches, which is the order the
    // results come in when nothing has been sorted by.
    await searchWithQuery(page, QUERY)
    const found = await resultCount(page)
    expect(found, "the query finds documents").toBeGreaterThan(1)
    const byRelevance = await resultIds(page)

    await openSortDialog(page)

    // Sorting by the display label replaces that ranking, without changing what the search found.
    await addSortColumn(page, LABEL_COLUMN)
    await expectSortOrder(page, [LABEL_COLUMN])
    const byLabel = await resultIds(page)
    expect(byLabel, "sorting a ranked search by the display label reorders it").not.toEqual(byRelevance)
    expect(await resultCount(page), "sorting changes the order and not what the query found").toBe(found)

    // Taking the column away again leaves the search with no column of its own, which is the ranking it
    // started in. How many results the feed has rendered by then is not part of that: it reveals whole pages
    // until its column is filled (fillColumn in SearchResultsFeed.vue), so a feed which has just been rebuilt
    // may be a page behind the one the first order was read from, and what is there is compared against the
    // beginning of it.
    await removeSortColumn(page, LABEL_COLUMN)
    await expectSortOrder(page, [])
    const ranked = await resultIds(page)
    expect(ranked.length, "the results of the search without a sort column").toBeGreaterThan(0)
    expect(ranked, "a search with no sort column of its own is ranked by relevance again").toEqual(byRelevance.slice(0, ranked.length))

    console.log(`Successfully sorted a query which found ${found} documents by the display label and back to its ranking.`)
  })

  for (const language of LANGUAGES) {
    test(`Test the sort dialog in ${language}`, async ({ context }) => {
      const page = await context.newPage()

      await goHome(page)
      await switchLanguage(page, language)
      await searchByClass(page, SEARCH_CLASS)
      await settleFilters(page)
      await openSortDialog(page)

      // The dialog names the three columns it always offers itself, and a column which comes from a facet by
      // the property it sorts on, all of them in the language the site is being read in. What each of them
      // says is not asserted here, only that every one of them says something.
      await expect(page.locator(".pd-searchsortdialog-title"), "the title of the dialog is translated").not.toHaveText(/^\s*$/)
      await expect(page.locator(".pd-searchsortdialog-header-sort"), "the heading of the sort order is translated").not.toHaveText(/^\s*$/)
      await expect(page.locator(".pd-searchsortdialog-header-available"), "the heading of the columns which can be added is translated").not.toHaveText(/^\s*$/)
      for (const column of [...BUILTIN_COLUMNS, LOCATED_AT_COLUMN]) {
        await expect(
          page.locator(`.pd-searchsortdialog-item-available-${column} .pd-searchsortdialog-label-available`),
          `the ${column} column is named in ${language}`,
        ).not.toHaveText(/^\s*$/)
      }

      // A column which is sorted by is named the same way as one which is only offered.
      await addSortColumn(page, LOCATED_AT_COLUMN)
      await expect(sortColumn(page, LOCATED_AT_COLUMN).locator(".pd-searchsortdialog-label-column"), `the sorted column is named in ${language}`).not.toHaveText(/^\s*$/)
      await checkpointDialog(page, `sorting-dialog-${language}`)

      console.log(`Successfully verified the sort dialog in ${language}.`)
    })
  }
})
