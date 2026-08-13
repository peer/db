import type { Locator, Page } from "@playwright/test"

import type { EntityClass } from "../peerdb_utils"

import { documentIdOf, PROPERTY_IDS, searchByClass } from "../peerdb_utils"
import { checkpoint, expect, filterValue, LOADING_TIMEOUT, resultCount, resultIds, searchAgain, settle, settleFilters, test, volatile } from "../utils"

// The class the table is looked at over. Its documents carry a reference, an amount and a time claim each, so
// the table has a column of every kind it supports, and there are few enough of them for the whole table to
// be captured in one screenshot.
const TABLE_CLASS: EntityClass = "MOON"

// The class the table is scrolled and widened over: everything the catalogue treats as a place, which is
// several hundred documents with more facets than the table shows columns for at once.
const WIDE_CLASS: EntityClass = "PLACE"

// The value the table is filtered to from one of its columns. A world type is a reference claim, which is
// what a table column filters on, and this one is carried by several of the class's documents.
const WORLD_TYPE = await documentIdOf("PLANET_TYPE", "DENSE_SILICATE")

// The property whose claims are Has claims: the property is stated without a value at all, which is not
// something a table cell can render, so it gets no column of its own.
const HAS_CLAIM_PROPERTY = PROPERTY_IDS.TIDALLY_LOCKED

// How many results the table shows before it is asked for more (SEARCH_INITIAL_LIMIT in
// SearchResultsTable.vue).
const TABLE_INITIAL_LIMIT = 100

// A screenshot of the whole table of the wide search would be several times the viewport in both directions,
// and how much of it is revealed before it is asked for more depends on how quickly the site answers, so
// those states are captured as the viewport shows them.
const VIEWPORT_ONLY = { fullPage: false }

// The rows and the columns of the table, and the header cell of the column for one property.
function tableRows(page: Page): Locator {
  return page.locator(".pd-searchresultstable-row-result")
}

function tableColumns(page: Page): Locator {
  return page.locator("th.pd-searchresultstable-column-filter")
}

function tableColumn(page: Page, propertyId: string): Locator {
  return page.locator(`th.pd-searchresultstable-column-filter-${propertyId}`)
}

// The identifiers of the documents the table shows, in the order it shows them. Every row carries the
// identifier of its document in its own id, the same way a result card of the feed does.
async function tableRowIds(page: Page): Promise<Array<string>> {
  return await tableRows(page).evaluateAll((rows) => rows.map((row) => row.id.replace(/^result-/, "")))
}

// Runs an action which changes the search from inside the table and waits until the table shows what the
// changed search found. The shared helper for this waits for a result card of the feed, which the table does
// not render at all, so the response of the search is waited for here and then the rows of the table.
async function searchAgainInTable(page: Page, action: () => Promise<void>): Promise<void> {
  const results = page.waitForResponse((response) => response.url().includes("/api/s/results/"), { timeout: LOADING_TIMEOUT })
  await action()
  await results
  await expect(page.locator(".pd-searchresultsheader-count-results"), "the table reports what the changed search found").toBeVisible({ timeout: LOADING_TIMEOUT })
  await expect(tableRows(page).first(), "the table shows the results of the changed search").toBeVisible({ timeout: LOADING_TIMEOUT })
  await settle(page)
}

// Switches the results between the feed and the table. The switch changes the search session, which is read
// back before the view it asked for is rendered, so what says the switch landed is the view itself.
async function switchView(page: Page, view: "feed" | "table"): Promise<void> {
  const button = page.locator(`.pd-selectbutton-button-${view}`)
  await expect(button, `the button which switches to the ${view}`).toBeVisible()
  await button.click()
  await expectView(page, view)
}

// Waits until the results are rendered in the given view, and asserts that the other one is not rendered
// next to it.
async function expectView(page: Page, view: "feed" | "table"): Promise<void> {
  if (view === "table") {
    await expect(page.locator(".pd-searchresultstable-table"), "the table").toBeVisible({ timeout: LOADING_TIMEOUT })
    await expect(tableRows(page).first(), "the first row of the table").toBeVisible({ timeout: LOADING_TIMEOUT })
    await expect(page.locator(".pd-searchresultsfeed"), "the feed is replaced by the table").toHaveCount(0)
  } else {
    await expect(page.locator(".pd-searchresultsfeed"), "the feed").toBeVisible({ timeout: LOADING_TIMEOUT })
    await expect(page.locator(".pd-searchresult").first(), "the first result of the feed").toBeVisible({ timeout: LOADING_TIMEOUT })
    await expect(page.locator(".pd-searchresultstable-table"), "the table is replaced by the feed").toHaveCount(0)
  }
  await settle(page)
}

// Opens a search of every document of a class and switches it to the table.
async function openTable(page: Page, entityClass: EntityClass): Promise<number> {
  await searchByClass(page, entityClass)
  const total = await resultCount(page)
  await switchView(page, "table")
  return total
}

test.describe("PeerDB Search Table Flows", () => {
  test("Test switching between the feed and the table", async ({ context }) => {
    const page = await context.newPage()

    await searchByClass(page, TABLE_CLASS)
    const total = await resultCount(page)
    const viewSwitch = page.locator(".pd-searchresultsheader-select-view")
    await expect(viewSwitch, "the results offer both views").toBeVisible()
    await expect(viewSwitch.locator(".pd-selectbutton-button"), "the switch offers exactly the two views").toHaveCount(2)
    await settleFilters(page)
    await checkpoint(page, "table-feed-view", { mask: volatile(page), ...VIEWPORT_ONLY })

    await switchView(page, "table")
    // The table renders one row per result rather than the batch the feed reveals, so a search of this size
    // is shown whole as soon as it is switched to.
    await expect(tableRows(page), "the table shows every result the search found").toHaveCount(total)
    const sessionUrl = page.url()
    await checkpoint(page, "table-table-view", { mask: volatile(page) })

    // Which view the results are shown in belongs to the search session and not to the page, so coming back
    // to the same search comes back to the table.
    await page.goto(sessionUrl)
    await expectView(page, "table")
    await expect(tableRows(page), "the reopened session shows the table again").toHaveCount(total)

    await switchView(page, "feed")
    await settleFilters(page)
    await checkpoint(page, "table-feed-view-again", { mask: volatile(page), ...VIEWPORT_ONLY })

    console.log(`Successfully switched a search of ${total} results from the feed to the table and back.`)
  })

  test("Test the columns the table shows", async ({ context }) => {
    const page = await context.newPage()

    const total = await openTable(page, TABLE_CLASS)

    // The first column numbers the results and links each of them to its document, and every other column is
    // one facet of the search.
    await expect(page.locator("th.pd-searchresultstable-column-index"), "the table is headed by the index column").toHaveCount(1)
    await expect(tableColumns(page).first(), "the table has columns of its own").toBeVisible()
    const columns = await tableColumns(page).count()
    expect(columns, "the table shows a column per facet").toBeGreaterThan(1)
    await expect(page.locator(".pd-searchresultstable-link-document"), "every row links to the document it stands for").toHaveCount(total)

    // A column is offered for the three kinds of facet whose claims a cell can render.
    await expect(tableColumn(page, PROPERTY_IDS.INSTANCE_OF), "the reference column of the class of a document").toHaveCount(1)
    await expect(tableColumn(page, PROPERTY_IDS.HAS_PLANET_TYPE), "the reference column of the type of world").toHaveCount(1)
    await expect(tableColumn(page, PROPERTY_IDS.CONTAINED_IN), "the reference column of what a world is contained in").toHaveCount(1)
    await expect(tableColumn(page, PROPERTY_IDS.RADIUS), "the amount column of the radius").toHaveCount(1)
    await expect(tableColumn(page, PROPERTY_IDS.FIRST_SURVEYED), "the time column of when a world was first surveyed").toHaveCount(1)
    // A property stated without a value is not one of them, so it gets no column even though the search can
    // be filtered by it.
    await expect(tableColumn(page, HAS_CLAIM_PROPERTY), "a property whose claims carry no value gets no column").toHaveCount(0)

    // Each column header is a button which opens the filter of that facet, so a column says both what it
    // holds and how the results are narrowed by it.
    await expect(tableColumn(page, PROPERTY_IDS.HAS_PLANET_TYPE).locator(".pd-searchresultstable-button-filter"), "a column header opens its filter").toBeVisible()
    await expect(tableColumn(page, PROPERTY_IDS.HAS_PLANET_TYPE), "a column is named after the property it holds").not.toBeEmpty()

    // The rows are the documents the search found, in the order the search returned them.
    const rowIds = await tableRowIds(page)
    expect(rowIds.length, "every result is a row of the table").toBe(total)
    expect(new Set(rowIds).size, "no document is shown twice").toBe(total)
    await checkpoint(page, "table-columns", { mask: volatile(page) })

    console.log(`Successfully verified the table of a search of ${total} results: an index column and ${columns} facet columns.`)
  })

  test("Test adding a column to the table", async ({ context }) => {
    const page = await context.newPage()

    await openTable(page, WIDE_CLASS)

    // The table shows as many columns as fit next to each other and offers the rest behind a button, the way
    // the feed offers the facets it has not shown yet.
    const before = await tableColumns(page).count()
    expect(before, "the table starts with the columns which fit").toBeGreaterThan(0)
    const moreColumns = page.locator(".pd-searchresultstable-button-morecolumns")
    await expect(moreColumns, "the table offers the facets it has not made a column of yet").toBeVisible()
    await checkpoint(page, "table-columns-before-adding", { mask: volatile(page), ...VIEWPORT_ONLY })

    // The press is dispatched rather than clicked, for the same reason as the one which reveals more results:
    // the table widens itself whenever its right edge comes near the fold, so the sideways scrolling a click
    // needs in order to reach the button at that edge widens the table and takes the button out from under
    // the click.
    await moreColumns.dispatchEvent("click")
    await expect
      .poll(async () => await tableColumns(page).count(), { message: "asking for more columns adds facets to the table", timeout: LOADING_TIMEOUT })
      .toBeGreaterThan(before)
    const after = await tableColumns(page).count()
    await settle(page)
    await checkpoint(page, "table-columns-after-adding", { mask: volatile(page), ...VIEWPORT_ONLY })

    console.log(`Successfully added columns to the table of a wide search: ${before} columns before, ${after} after.`)
  })

  test("Test filtering the table from one of its columns", async ({ context }) => {
    const page = await context.newPage()

    const total = await openTable(page, TABLE_CLASS)

    // A column header opens the filter of the facet the column holds, in a dialog over the table.
    const header = tableColumn(page, PROPERTY_IDS.HAS_PLANET_TYPE).locator(".pd-searchresultstable-button-filter")
    await expect(header, "the header of the column to filter by").toBeVisible()
    // The funnel of a column says whether the results are narrowed by it, and it is drawn in the muted colour
    // while the filter is not in effect.
    await expect(header.locator("svg.text-primary-300"), "the funnel of a column nothing is filtered by is muted").toHaveCount(1)
    await header.click()

    const panel = page.locator(".pd-searchresultstable-panel-filter")
    await expect(panel, "the filter dialog of the column").toBeVisible()
    await expect(panel.locator(`.pd-filtersresult-ref-${PROPERTY_IDS.HAS_PLANET_TYPE}`), "the dialog holds the facet of the column it was opened from").toBeVisible()
    await checkpoint(page, "table-filter-dialog", { mask: volatile(page), ...VIEWPORT_ONLY })

    const value = filterValue(page, "ref", [PROPERTY_IDS.HAS_PLANET_TYPE], WORLD_TYPE)
    await expect(value, "the value to filter by").toBeVisible()
    await searchAgainInTable(page, async () => {
      await value.click()
    })
    await expect.poll(() => resultCount(page), { message: "filtering from a column narrows the results" }).toBeLessThan(total)
    const filtered = await resultCount(page)
    expect(filtered, "the filtered search still finds something").toBeGreaterThan(0)

    const closeFilter = page.locator(".pd-searchresultstable-button-closefilter")
    await expect(closeFilter, "the filter dialog offers to be closed").toBeVisible()
    await closeFilter.click()
    await expect(panel, "the filter dialog is closed").toHaveCount(0)

    await expect(tableRows(page), "the table shows the narrowed results").toHaveCount(filtered)
    await expect(header.locator("svg.text-primary-300"), "the funnel of the column the results are filtered by is no longer muted").toHaveCount(0)
    await checkpoint(page, "table-filtered", { mask: volatile(page) })

    console.log(`Successfully filtered a table of ${total} results down to ${filtered} from one of its columns.`)
  })

  test("Test loading more results inside the table", async ({ context }) => {
    const page = await context.newPage()

    const total = await openTable(page, WIDE_CLASS)
    expect(total, "the wide search finds more results than the table shows at once").toBeGreaterThan(TABLE_INITIAL_LIMIT)

    const before = await tableRows(page).count()
    expect(before, "the table shows the first batch of the results").toBeGreaterThanOrEqual(TABLE_INITIAL_LIMIT)
    expect(before, "the table does not show every result at once").toBeLessThan(total)
    const loadMore = page.locator("#searchresultstable-button-loadmore")
    await expect(loadMore, "the table offers the results it has not shown yet").toBeVisible()
    await checkpoint(page, "table-before-loading-more", { mask: volatile(page), ...VIEWPORT_ONLY })

    // The press is dispatched rather than clicked. The table reveals another batch by itself whenever its end
    // comes near the fold, so the scrolling a click needs in order to reach the button at the end of the table
    // reveals that batch and takes the button out from under the click. Dispatching reaches the button the way
    // the table reaches it itself.
    await loadMore.dispatchEvent("click")
    await expect
      .poll(async () => await tableRows(page).count(), { message: "the table shows another batch of results", timeout: LOADING_TIMEOUT })
      .toBeGreaterThan(before)
    // Every row of the new batch stands for a document of its own. The table goes on revealing batches by
    // itself for as long as its end is near the fold, so how many rows there are is read once and both
    // assertions are made about that one reading rather than about two readings taken a moment apart.
    const rowIds = await tableRowIds(page)
    const after = rowIds.length
    expect(after, "the table shows more rows than it did").toBeGreaterThan(before)
    expect(new Set(rowIds).size, "no document is shown twice after loading more").toBe(after)
    await settle(page)
    await checkpoint(page, "table-after-loading-more", { mask: volatile(page), ...VIEWPORT_ONLY })

    console.log(`Successfully loaded more results inside the table: ${before} rows of ${total} before, ${after} after.`)
  })

  test("Test the sort order of the search carries into the table", async ({ context }) => {
    const page = await context.newPage()

    // The table has no toolbar of its own for sorting or printing: both are offered by the feed, and what the
    // feed was set to is what the table shows.
    await searchByClass(page, TABLE_CLASS)
    const total = await resultCount(page)

    const sortButton = page.locator(".pd-searchresultsheader-button-sort")
    await expect(sortButton, "the feed offers the sort dialog").toBeVisible()
    await sortButton.click()
    await expect(page.locator(".pd-searchsortdialog-panel"), "the sort dialog").toBeVisible()
    const addLabel = page.locator(".pd-searchsortdialog-button-add-label")
    await expect(addLabel, "the dialog offers sorting by the display label").toBeVisible()
    await searchAgain(page, async () => {
      await addLabel.click()
    })
    await expect(page.locator(".pd-searchsortdialog-item-sort-label"), "the results are sorted by the display label").toBeVisible()
    await page.locator(".pd-searchsortdialog-button-close").click()
    await expect(page.locator(".pd-searchsortdialog-panel"), "the sort dialog is closed").toBeHidden()

    const feedOrder = await resultIds(page)
    expect(feedOrder.length, "the feed shows results to compare the table against").toBeGreaterThan(0)

    await switchView(page, "table")
    await expect(tableRows(page), "the table shows every result the search found").toHaveCount(total)
    await expect(sortButton, "the table offers no sort dialog").toHaveCount(0)
    await expect(page.locator(".pd-searchresultsheader-button-print"), "the table offers no print view").toHaveCount(0)

    // The order is the search session's and not the view's, so the rows of the table are the results of the
    // feed, in the same order, starting from the same first result.
    const tableOrder = await tableRowIds(page)
    expect(tableOrder.slice(0, feedOrder.length), "the table shows the results in the order the feed did").toEqual(feedOrder)
    await checkpoint(page, "table-sorted", { mask: volatile(page) })

    console.log(`Successfully carried a sort order into the table: ${feedOrder.length} results of ${total} compared row by row.`)
  })
})
