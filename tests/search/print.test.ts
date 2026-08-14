import type { Page } from "@playwright/test"

import { documentIdOf, PROPERTY_IDS, searchByClass } from "../peerdb_utils"
import {
  checkpoint,
  expect,
  expectResults,
  LOADING_TIMEOUT,
  openSortDialog,
  PEERDB_URL,
  printVolatile,
  resultCount,
  searchAgain,
  settle,
  settleFilters,
  sortColumn,
  test,
  volatile,
} from "../utils"

// The document the search of the referencing documents is run from. A culture is referenced by everything
// the culture leaves behind and by everyone who belongs to it, so the search has enough results for the
// print view to have something to reveal.
const CULTURE = await documentIdOf("CULTURE", "G4_CU_LADDER_GORGE")

// The suffix the sort dialog builds a column's CSS classes from: the type of the column followed by the
// identifiers of the property path it sorts on. Worlds are grouped by the type of world they are, because it is a
// reference column, which is the only kind of column results can be grouped by, and because few enough types
// are used for the grouped tree to stay readable.
const WORLD_TYPE_COLUMN = `ref-${PROPERTY_IDS.HAS_PLANET_TYPE}`

// How many results the feed reveals of a search which found more than fits on the first screen depends on how
// quickly the site answers, so it is not the same from one run to the next and a whole page screenshot of
// such a state would differ for a reason which is not a regression. Those states are captured as the viewport
// shows them; a state in which every result is revealed is captured whole.
const VIEWPORT_ONLY = { fullPage: false }

// Opens the print view, an in-app preview of how the results print. It hides the interactive chrome (the
// toolbar and the filters column) and shows the printed layout instead: the filter summary, a timestamp, and
// the controls which are part of the preview only.
async function openPrintView(page: Page): Promise<void> {
  const printButton = page.locator(".pd-searchresultsheader-button-print")
  await expect(printButton, "the print button").toBeVisible()
  await printButton.click()
  await expect(page.locator(".pd-searchresultsfeed-button-closeprint"), "the print view offers to be closed").toBeVisible()
  await expect(page.locator(".pd-searchresultsfeed-timestamp"), "the print view is stamped with the time").toBeVisible()
  // The chrome which is not printed goes away with the same class which reveals the printed layout, so this
  // tells the preview apart from the ordinary view rather than only asserting that its own controls appeared.
  await expect(page.locator(".pd-searchresultsfeed-panel-filters"), "the filters column is not printed").toBeHidden()
  await expect(printButton, "the print button is not printed").toBeHidden()
}

// Leaves the print view and waits until the ordinary view is back.
async function closePrintView(page: Page): Promise<void> {
  const closeButton = page.locator(".pd-searchresultsfeed-button-closeprint")
  await expect(closeButton, "the button which closes the print view").toBeVisible()
  await closeButton.click()
  await expect(closeButton, "the print view is gone").toBeHidden()
  await expect(page.locator(".pd-searchresultsfeed-panel-filters"), "the filters column is back").toBeVisible()
}

// Reveals every result which has been loaded, and every repeating claim value inside each of them, which is
// what a printout needs: the feed otherwise shows only the batches it revealed while it was scrolled and caps
// repeating values behind a button of their own. The button goes away once there is nothing left to reveal,
// and a search whose whole result set is already on the page never offers it at all, so a page which has
// nothing left to reveal is left as it is.
async function showAllResults(page: Page): Promise<void> {
  const loadAll = page.locator(".pd-searchresultsfeed-button-loadall")
  if (await loadAll.isVisible().catch(() => false)) {
    await loadAll.click()
    await expect(loadAll, "nothing is left to reveal").toBeHidden()
  }
  await settle(page)
}

// Groups the results by the given reference column and renders each group value as a full result card instead
// of a one-line heading. A column has to be sorted by before it can be grouped by, and expanding is offered
// only once it is grouped by, so the three steps go in this order and each waits for the results of the
// search its change started.
async function groupByColumn(page: Page, column: string): Promise<void> {
  await openSortDialog(page)

  const addButton = page.locator(`.pd-searchsortdialog-button-add-${column}`)
  await expect(addButton, "the dialog offers the column to sort by").toBeVisible()
  await searchAgain(page, async () => {
    await addButton.click()
  })

  const groupCheckbox = sortColumn(page, column).locator(".pd-searchsortdialog-checkbox-group")
  await expect(groupCheckbox, "a reference column offers to group by it").toBeVisible()
  await searchAgain(page, async () => {
    await groupCheckbox.click()
  })
  await expect(groupCheckbox, "the column is grouped by").toBeChecked()

  const expandCheckbox = sortColumn(page, column).locator(".pd-searchsortdialog-checkbox-expand")
  await expect(expandCheckbox, "a grouped column offers to expand its values").toBeVisible()
  await searchAgain(page, async () => {
    await expandCheckbox.click()
  })
  await expect(expandCheckbox, "the group values are expanded").toBeChecked()

  const closeButton = page.locator(".pd-searchsortdialog-button-close")
  await expect(closeButton, "the sort dialog offers to be closed").toBeVisible()
  await closeButton.click()
  await expect(page.locator(".pd-searchsortdialog-panel"), "the sort dialog is closed").toBeHidden()
}

test.describe("PeerDB Search Print Flows", () => {
  test("Test the print view of a search", async ({ context }) => {
    const page = await context.newPage()

    // The search is scoped to one class by a prefilter, which is what the print layout summarizes above the
    // results, and the class has few enough documents for every one of them to be printed.
    await searchByClass(page, "COLLECTIVE")
    const total = await resultCount(page)
    expect(total, "the class search finds more results than the feed reveals at once").toBeGreaterThan(10)
    const results = page.locator(".pd-searchresult")
    const shown = await results.count()
    await settleFilters(page)
    await checkpoint(page, "print-plain-search-results", { mask: volatile(page), ...VIEWPORT_ONLY })

    await openPrintView(page)

    // The scope of the search is printed as a list of the filters it is under, so that a printout says what
    // it is a printout of.
    const printFilters = page.locator(".pd-searchprintfilters")
    await expect(printFilters, "the printed layout summarizes the filters").toBeVisible()
    await expect(printFilters.locator(".pd-searchprintfilters-item"), "the class prefilter is the one filter summarized").toHaveCount(1)
    await expect(printFilters, "the summarized filter is named").not.toBeEmpty()
    await checkpoint(page, "print-plain-print-view", { mask: printVolatile(page), ...VIEWPORT_ONLY })

    // The feed reveals its results in batches, and the print view offers to reveal the rest, so that a
    // printout is not limited to what happened to be on screen.
    await showAllResults(page)
    await expect(results, "showing all results reveals every result the search found").toHaveCount(total, { timeout: LOADING_TIMEOUT })
    expect(shown, "the feed had not revealed every result before").toBeLessThan(total)
    await expect(printFilters, "the filter summary stays once everything is revealed").toBeVisible()
    await checkpoint(page, "print-plain-print-view-all-results", { mask: printVolatile(page) })

    await closePrintView(page)
    await expect(printFilters, "the filter summary belongs to the printed layout alone").toBeHidden()
    await settle(page)
    await checkpoint(page, "print-plain-print-view-closed", { mask: volatile(page), ...VIEWPORT_ONLY })

    console.log(`Successfully printed a prefiltered search: ${shown} results revealed by the feed, all ${total} of them in the print view.`)
  })

  test("Test the print view of a grouped search", async ({ context }) => {
    const page = await context.newPage()

    // Worlds carry the type of world they are, so grouping by it gives the results a tree of groups, each
    // group value rendered as a full result card of its own because the column is expanded.
    await searchByClass(page, "MOON")
    const total = await resultCount(page)
    await groupByColumn(page, WORLD_TYPE_COLUMN)

    // How many groups are rendered follows how many results have been revealed, because the tree is limited
    // by unique result rather than by group, so at this point only that the results are grouped at all is
    // asserted. How many groups there are is asserted once the whole tree is revealed below.
    const groups = page.locator(".pd-searchresultgroup")
    await expect(groups.first(), "the results are grouped").toBeVisible()
    // Expanded means every heading offers to collapse it again and none of them offers to expand it.
    await expect(page.locator(".pd-searchresultgroup-button-collapse").first(), "an expanded group offers to be collapsed").toBeVisible()
    await expect(page.locator(".pd-searchresultgroup-button-expand"), "no group is left collapsed").toHaveCount(0)
    const shown = await page.locator(".pd-searchresult").count()
    await settleFilters(page)
    await checkpoint(page, "print-grouped-search-results", { mask: volatile(page), ...VIEWPORT_ONLY })

    await openPrintView(page)
    await expect(page.locator(".pd-searchprintfilters"), "the printed layout summarizes the filters").toBeVisible()
    await expect(page.locator(".pd-searchprintfilters-item"), "the class prefilter is the one filter summarized").toHaveCount(1)
    await checkpoint(page, "print-grouped-print-view", { mask: printVolatile(page), ...VIEWPORT_ONLY })

    // The grouped view is limited by unique result rather than by position in a list, so showing all results
    // has to fill in the whole tree and not only its beginning. Every group value is rendered as a result
    // card of its own on top of the results themselves, which is what an expanded column means.
    await showAllResults(page)
    const printed = page.locator(".pd-searchresult")
    // The tree is filled in when every result is under a group and every group value is a result card of its
    // own next to them. Both counts are read together, because the tree is still being rendered while the
    // first of them is read.
    await expect
      .poll(
        async () => {
          const groupsShown = await groups.count()
          return groupsShown > 1 && (await printed.count()) === total + groupsShown
        },
        { message: "the whole tree is printed, every group value with it", timeout: LOADING_TIMEOUT },
      )
      .toBe(true)
    const groupCount = await groups.count()
    expect(shown, "the tree had not been filled in before").toBeLessThan(await printed.count())
    // An expanded group value is rendered with the count of the results under it, and the results themselves
    // stay nested in the list below it.
    await expect(page.locator(".pd-searchresultgroup-count").first(), "an expanded group value carries the count of its results").toBeVisible()
    await expect(page.locator(".pd-searchresultgroup-list").first(), "the results of a group stay nested under it").toBeVisible()
    await expect(page.locator(".pd-searchresultgroup-item").first(), "a nested result of a group").toBeVisible()
    await checkpoint(page, "print-grouped-print-view-all-results", { mask: printVolatile(page) })

    await closePrintView(page)
    await settle(page)
    await checkpoint(page, "print-grouped-print-view-closed", { mask: volatile(page), ...VIEWPORT_ONLY })

    console.log(`Successfully printed a search of ${total} results grouped into ${groupCount} expanded groups.`)
  })

  test("Test the print view of a search of the referencing documents", async ({ context }) => {
    const page = await context.newPage()

    await page.goto(`${PEERDB_URL}/s?reverse=${CULTURE}`)
    await expectResults(page)
    const total = await resultCount(page)
    await expect(page.locator(".pd-searchresultsfeed-button-clearreverse"), "the search is scoped to the referencing documents").toBeVisible()
    // The documents referencing a culture come from several classes at once, so this search has a facet for
    // every property any of them carries. Adding all of them takes longer than the rest of this test and only
    // grows the part of the page below the fold, which a screenshot of the viewport does not reach, so the
    // panel is left the size it settles at by itself.
    await settle(page)
    await checkpoint(page, "print-referencing-search-results", { mask: volatile(page), ...VIEWPORT_ONLY })

    await openPrintView(page)
    // A search scoped to the documents referencing a target is not scoped by a filter, so the print layout
    // names the target on its own line instead of listing it in the filter summary.
    await expect(page.locator(".pd-searchprintfilters"), "a referencing scope is not summarized as a filter").toHaveCount(0)
    // The target is written twice, once into the filters column for the screen and once into the printed
    // layout, so the printed copy is the one asserted on here.
    await expect(page.locator(".pd-searchresultsfeed-text-referencing.pd-print-only"), "the printed layout names the target").toBeVisible()
    const expand = page.locator(".pd-searchresultsfeed-button-expandreferencing")
    const collapse = page.locator(".pd-searchresultsfeed-button-collapsereferencing")
    await expect(expand, "the named target offers to be expanded").toBeVisible()
    await expect(collapse, "the target is not expanded to begin with").toHaveCount(0)
    await checkpoint(page, "print-referencing-print-view", { mask: printVolatile(page), ...VIEWPORT_ONLY })

    // Expanding the target replaces the line naming it with its own full result card above the results, so
    // that a printout carries what the referencing documents are about.
    await expect(page.locator(".pd-searchresultsfeed-result-referencing"), "the target is named rather than rendered before it is expanded").toHaveCount(0)
    await expand.click()
    await expect(collapse, "the expanded target offers to be collapsed").toBeVisible()
    await expect(expand, "the expanded target does not offer to be expanded again").toHaveCount(0)
    // The target is asserted through the CSS class its own card carries rather than by counting the cards on
    // the page: the feed reveals results as the page moves, and a capture or a click moves it, so a count taken
    // before the expanding and one taken after it are not counts of the same set.
    const target = page.locator(".pd-searchresultsfeed-result-referencing")
    await expect(target, "the expanded target is a result card of its own").toHaveCount(1, { timeout: LOADING_TIMEOUT })
    await expect(target, "the card of the target is a result card like the others").toHaveClass(/(^|\s)pd-searchresult(\s|$)/)

    await showAllResults(page)
    await expect(collapse, "the target stays expanded once everything is revealed").toBeVisible()
    await expect(page.locator(".pd-searchresult"), "every referencing document is printed next to the target").toHaveCount(total + 1, { timeout: LOADING_TIMEOUT })
    await checkpoint(page, "print-referencing-target-expanded", { mask: printVolatile(page) })

    // Collapsing takes the target's card away again. Expanding and collapsing are changes to the search
    // session, so the results are fetched again and the feed reveals its first batch of them once more,
    // which is why what is asserted here is that the target is gone and that there is something left to
    // reveal, and not a number of results.
    await collapse.click()
    await expect(expand, "the collapsed target offers to be expanded again").toBeVisible()
    await expect(collapse, "the collapsed target does not offer to be collapsed again").toHaveCount(0)
    await expect(page.locator(".pd-searchresultsfeed-result-referencing"), "the card of the collapsed target is gone").toHaveCount(0)
    await expect(page.locator(".pd-searchresultsfeed-button-loadall"), "the feed offers to reveal its results again").toBeVisible()
    await expect.poll(() => resultCount(page), { message: "collapsing the target leaves the referencing documents alone" }).toBe(total)
    await checkpoint(page, "print-referencing-target-collapsed", { mask: printVolatile(page), ...VIEWPORT_ONLY })

    await closePrintView(page)
    // The controls of the printed layout belong to the preview alone, so they are gone once it is closed,
    // while the scope itself is still in effect and can still be cleared from the filters column.
    await expect(expand, "the expand control belongs to the printed layout alone").toBeHidden()
    await expect(page.locator(".pd-searchresultsfeed-button-clearreverse"), "the scope survives the print view").toBeVisible()
    await settle(page)
    await checkpoint(page, "print-referencing-print-view-closed", { mask: volatile(page), ...VIEWPORT_ONLY })

    console.log(`Successfully printed a search of the ${total} documents referencing a culture, with the target expanded and collapsed again.`)
  })
})
