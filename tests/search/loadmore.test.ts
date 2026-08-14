import type { Locator, Page } from "@playwright/test"

import { PROPERTY_IDS, searchByClass, searchByCoreClass } from "../peerdb_utils"
import {
  checkpoint,
  expect,
  loadAllResults,
  LOADING_TIMEOUT,
  loadMoreButton,
  openSortDialog,
  PEERDB_URL,
  resultCount,
  resultIds,
  searchResults,
  settle,
  settleFilters,
  test,
  volatile,
} from "../utils"

// The class the results feed is paged through. It needs clearly more documents than the feed renders at
// once, its documents have to be small enough that a screenshot of all of them is worth comparing, and no
// role of this site may start a new document of it other than the one which may create anything, so a run
// loads the same result set as the run before it.
const PAGED_CLASS = "UNIT"

// How many results one page of the feed holds: how many it renders to begin with, how many it adds each time
// it is asked for more, and how many it puts between two pager rows (SEARCH_INITIAL_LIMIT and SEARCH_INCREASE
// in SearchResultsFeed.vue).
//
// The site is served to the tests with loading while scrolling turned off (disableLoadingOnScroll, see
// test-e2e.sh), so the feed adds a page when it is asked for one and at no other time. What it shows is
// therefore a count these tests can name rather than one which follows how tall the results happened to
// render.
const PAGE_SIZE = 10

// How few results a page may end with before the feed stops holding them back and reveals them together with
// the page before them (SKIP_TO_END in src/utils.ts). It is why the last step of the paging is not a whole
// page and why the search these tests page through needs more results than the two whole pages they assert,
// and that many more on top.
const SKIP_TO_END = 2

// The property the grouped run below groups by. Every star system either records the species native to it or
// records none, so grouping by it yields a heading per species and one heading for the systems which name no
// species at all, which is the group the feed has to render without trying to resolve a document for it.
const GROUPED_CLASS = "STAR_SYSTEM"
const GROUPED_PROPERTY = PROPERTY_IDS.HOME_TO_SPECIES

// How many of those star systems name no species at all, which is what the heading for the missing value
// gathers.
const NO_VALUE_TOTAL = 18

// How many pager rows precede the given number of shown results: one before every result which starts a page
// other than the first.
function pagerRowsFor(shown: number): number {
  return Math.floor((shown - 1) / PAGE_SIZE)
}

// The numbers a pager row or the end bar reports: how far down the results the row sits and how many results
// there are. The sentence around them is written in the language of the page and its numbers are grouped for
// the locale, so the digit runs are read out of it rather than the sentence being matched.
async function reportedNumbers(row: Locator): Promise<Array<number>> {
  await expect(row, "the row whose numbers are read").toBeVisible()
  const text = (await row.textContent()) || ""
  return (text.match(/\d(?:[\d.,\s]*\d)?/g) || []).map((run) => Number(run.replace(/\D/g, "")))
}

// Asks the feed for one more page of results and waits until it has rendered them.
//
// The button is pressed by dispatching the click rather than by clicking it, which leaves the page where it
// is: a click first scrolls the button into view, and a site served with loading while scrolling left on (the
// site's own behaviour, which these tests turn off) loads a page through that scrolling as well as through
// the press. Dispatching delivers the same click to the same button and loads exactly one page whichever way
// the site is served.
async function loadNextPage(page: Page): Promise<void> {
  const loadMore = loadMoreButton(page)
  await expect(loadMore, "the feed offers to load more results").toBeVisible()
  const shown = await searchResults(page).count()
  await loadMore.dispatchEvent("click")
  await expect.poll(() => searchResults(page).count(), { message: "the feed adds results when more are asked for" }).toBeGreaterThan(shown)
  await settle(page)
}

// Runs the search which has more results than the feed shows at once and reports how many it found.
async function searchPaged(page: Page): Promise<number> {
  await searchByCoreClass(page, PAGED_CLASS)
  await settle(page)

  const total = await resultCount(page)
  expect(total, `the search for ${PAGED_CLASS} documents finds more results than the two pages loaded below`).toBeGreaterThan(2 * PAGE_SIZE + SKIP_TO_END)
  return total
}

// Groups the results of the running search by the values of one reference property, through the dialog the
// sort order and the grouping are set in.
async function groupBy(page: Page, propertyId: string): Promise<void> {
  await openSortDialog(page)
  const add = page.locator(`.pd-searchsortdialog-button-add-ref-${propertyId}`)
  await expect(add, "the property the results are grouped by is offered as a column").toBeVisible()
  await add.click()

  const group = page.locator(".pd-searchsortdialog-checkbox-group").first()
  await expect(group, "the added column can be grouped by").toBeVisible()
  await group.check()
  await page.locator(".pd-searchsortdialog-button-close").click()

  await expect(page.locator(".pd-searchresultgroup").first(), "the results are gathered into groups").toBeVisible({ timeout: LOADING_TIMEOUT })
  await settle(page)
}

// Reveals every group of a grouped feed.
//
// The rest is loaded by scrolling rather than by pressing the button, because the feed presses that button
// itself whenever the end of the list comes near the viewport, so a press and the scrolling a press needs
// would load a page each. What is waited for is the button going away, which is what says the feed has
// nothing left to reveal: how many elements a grouped feed renders is not the number of results, because a
// document which belongs under several headings is rendered under each of them.
async function loadAllGroups(page: Page): Promise<void> {
  await expect
    .poll(
      async () => {
        await page.evaluate(() => window.scrollTo({ top: document.body.scrollHeight, behavior: "instant" }))
        return await loadMoreButton(page).count()
      },
      { message: "the grouped feed runs out of results to reveal", timeout: 2 * LOADING_TIMEOUT },
    )
    .toBe(0)
  await settle(page)
}

// The heading of the group which gathers the results carrying no value for the property they are grouped by.
// It is a heading the feed writes itself rather than a document, so it is the one heading which does not
// stand for one, which is how it is told apart without reading the label it renders.
function missingGroupTitle(page: Page): Locator {
  return page.locator(".pd-searchresultgroup-title:not([data-url])")
}

test.describe("PeerDB Load More Flows", () => {
  test("Test the first page of a search which found more results than it shows", async ({ context }) => {
    const page = await context.newPage()

    const total = await searchPaged(page)

    // The header reports the whole result set while the feed renders the first page of it, so the two numbers
    // differ and the visitor is told there is more than what is in front of them.
    const shown = await searchResults(page).count()
    expect(shown, "the feed renders the first page of results").toBe(PAGE_SIZE)
    expect(shown, "what is rendered on load is less than the search found").toBeLessThan(total)

    await expect(loadMoreButton(page), "the feed offers to load the rest").toBeVisible()
    await expect(page.locator(".pd-searchresultsendbar"), "the feed does not mark the end while there is more to load").toHaveCount(0)
    // A pager row precedes every result which starts a page other than the first, so how many of them there
    // are follows from how much the feed rendered.
    await expect(page.locator(".pd-searchresultspager"), "a pager row precedes every page after the first").toHaveCount(pagerRowsFor(shown))

    await settleFilters(page)
    await checkpoint(page, "loadmore-first-page", { mask: volatile(page) })

    console.log(`Successfully verified the first page of a search, which shows ${shown} of ${total} results.`)
  })

  test("Test loading the next page of results", async ({ context }) => {
    const page = await context.newPage()

    const total = await searchPaged(page)
    const onLoad = await resultIds(page)
    expect(onLoad.length, "what the feed renders on load").toBe(PAGE_SIZE)

    await loadNextPage(page)

    const shown = await searchResults(page).count()
    expect(shown, "loading more adds a page of results").toBe(2 * PAGE_SIZE)
    expect(shown, "what is shown after loading more is still not all of it").toBeLessThan(total)
    await expect(loadMoreButton(page), "the feed keeps offering the results which are still not shown").toBeVisible()

    // What was added has to extend the list rather than replace it or repeat it: the results which were
    // already shown stay where they were, and every result which came with the new page is one of the
    // documents which were not shown before.
    const shownIds = await resultIds(page)
    expect(shownIds.slice(0, onLoad.length), "the results shown on load stay where they were").toEqual(onLoad)
    const added = shownIds.slice(onLoad.length)
    expect(
      added.filter((id) => onLoad.includes(id)),
      "the results which were loaded are documents which were not shown before",
    ).toEqual([])

    // With more than a page shown, the list is broken up by pager rows which say how far down the results the
    // reader has come and how many of them there are.
    const pagers = page.locator(".pd-searchresultspager")
    await expect(pagers, "a pager row precedes every page after the first").toHaveCount(pagerRowsFor(shown))
    expect(
      await reportedNumbers(pagers.last().locator(".pd-searchresultspager-count")),
      "the last pager reports how far down the results it sits, out of all of them",
    ).toEqual([PAGE_SIZE * pagerRowsFor(shown), total])
    await expect(pagers.last().locator(".pd-searchresultspager-thumb"), "the pager draws how far down the results it sits").toBeVisible()

    await settleFilters(page)
    await checkpoint(page, "loadmore-second-page", { mask: volatile(page) })

    console.log(`Successfully loaded more results, which brought the ${onLoad.length} shown to ${shown} of ${total}.`)
  })

  test("Test loading more results by scrolling", async ({ context }) => {
    const page = await context.newPage()

    const total = await searchPaged(page)
    const onLoad = await searchResults(page).count()
    const startedAt = new URL(page.url()).searchParams.get("at")

    // Nothing is pressed here: the feed watches the page and loads the next results by itself as soon as the
    // end of the list comes near the viewport, so a visitor who only scrolls never runs out of results.
    await page.evaluate(() => window.scrollTo({ top: document.body.scrollHeight, behavior: "instant" }))
    await expect.poll(() => searchResults(page).count(), { message: "scrolling to the end of the list loads more results" }).toBeGreaterThan(onLoad)
    await settle(page)

    const shown = await searchResults(page).count()
    expect(shown, "scrolling adds no more than the search found").toBeLessThanOrEqual(total)
    const shownIds = await resultIds(page)
    expect(shownIds, "scrolling shows no result twice").toHaveLength(new Set(shownIds).size)

    // Scrolling also records where the reader has come to in the address, so that reloading the page or
    // handing the address on comes back to the same place in the results.
    await expect.poll(() => new URL(page.url()).searchParams.get("at"), { message: "scrolling records in the address which result is being read" }).not.toBe(startedAt)
    expect(shownIds, "the result the address names is one of the results which are shown").toContain(new URL(page.url()).searchParams.get("at"))

    console.log(`Successfully loaded results by scrolling, which brought the ${onLoad} shown to ${shown} of ${total}.`)
  })

  test("Test that the header counts every result and not the ones which are shown", async ({ context }) => {
    const page = await context.newPage()

    // The count in the header is what the search found, so it has to stay where it is while the feed reveals
    // more of it: a header which counted the rendered results would climb as the reader scrolls.
    const total = await searchPaged(page)
    const onLoad = await searchResults(page).count()
    expect(onLoad, "the feed renders less than the search found").toBeLessThan(total)

    await loadNextPage(page)
    const shown = await searchResults(page).count()
    expect(shown, "loading more results renders more of them").toBeGreaterThan(onLoad)
    await expect.poll(() => resultCount(page), { message: "the header goes on reporting the whole result set" }).toBe(total)

    // The pager rows count against the same whole, so what the last of them reports as the total is what the
    // header reports.
    const pagers = page.locator(".pd-searchresultspager")
    expect((await reportedNumbers(pagers.last().locator(".pd-searchresultspager-count")))[1], "the pager counts against the whole result set").toBe(total)

    console.log(`Successfully verified that the header reports all ${total} results while the feed shows ${shown} of them.`)
  })

  test("Test loading every result of a search", async ({ context }) => {
    // Every page of results is loaded one at a time and each one fetches the documents it renders, which takes
    // more than the default budget for a search with this many results.
    test.slow()

    const page = await context.newPage()

    const total = await searchPaged(page)
    const onLoad = await resultIds(page)

    await loadAllResults(page)
    await settle(page)

    const shown = await searchResults(page).count()
    expect(shown, "loading everything shows every result the search found").toBe(total)
    await expect(loadMoreButton(page), "nothing is left to load once every result is shown").toHaveCount(0)

    // In place of the button the feed marks the end of the results, and says that all of them are shown.
    const endBar = page.locator(".pd-searchresultsendbar")
    await expect(endBar, "the end of the results is marked").toBeVisible()
    expect(await reportedNumbers(endBar.locator(".pd-searchresultsendbar-text")), "the end bar reports every result as shown").toEqual([total])

    const shownIds = await resultIds(page)
    expect(shownIds, "every result which was loaded").toHaveLength(total)
    expect(shownIds.slice(0, onLoad.length), "what was shown on load is still at the top of the loaded list").toEqual(onLoad)
    // Paging through the results may not show the same document twice, which is what a page loaded with an
    // offset the feed has already rendered would do.
    expect(new Set(shownIds).size, "no result is shown twice across the whole loaded list").toBe(shownIds.length)

    // The pager rows are what a reader counts pages by, so there has to be one before every result which
    // starts a page other than the first, and the last one has to report the same number of results as the
    // header does.
    const pagers = page.locator(".pd-searchresultspager")
    await expect(pagers, "a pager row precedes every page after the first").toHaveCount(pagerRowsFor(total))
    expect(await reportedNumbers(pagers.last().locator(".pd-searchresultspager-count")), "the last pager reports the results it sits after, out of all of them").toEqual([
      PAGE_SIZE * pagerRowsFor(total),
      total,
    ])

    await settleFilters(page)
    await checkpoint(page, "loadmore-all-loaded", { mask: volatile(page) })

    console.log(`Successfully loaded every result of a search, all ${total} of them, without showing any of them twice.`)
  })

  test("Test paging through grouped results", async ({ context }) => {
    test.slow()

    const page = await context.newPage()

    await searchByClass(page, GROUPED_CLASS)
    const total = await resultCount(page)
    await groupBy(page, GROUPED_PROPERTY)

    // A grouped feed reveals its results a page at a time just as a flat one does, counting the results
    // themselves rather than the headings which gather them.
    const groups = page.locator(".pd-searchresultgroup")
    const onLoad = await groups.count()
    expect(onLoad, "the grouped feed renders groups").toBeGreaterThan(0)
    await expect(loadMoreButton(page), "the grouped feed offers the results it did not render").toBeVisible()
    await expect(page.locator(".pd-withdocument-error"), "every group heading resolves").toHaveCount(0)

    await loadMoreButton(page).dispatchEvent("click")
    await expect.poll(() => groups.count(), { message: "asking for more results reveals more groups" }).toBeGreaterThan(onLoad)
    await settle(page)

    await loadAllGroups(page)

    // The results which carry no value for the property they are grouped by are gathered under a heading the
    // feed writes itself. It stands for no document, so it may not try to resolve one, and it counts what it
    // gathers the same way the headings which do stand for a document count theirs.
    const missing = missingGroupTitle(page)
    await expect(missing, "the results which carry no value are gathered under a heading of their own").toHaveCount(1)
    await expect(page.locator(".pd-withdocument-error"), "no group heading failed to resolve a document").toHaveCount(0)
    const missingGroup = page.locator(".pd-searchresultgroup").filter({ has: missing })
    expect(
      await reportedNumbers(missingGroup.locator(".pd-searchresultgroup-count").first()),
      "the heading which stands for no document counts the results it gathers",
    ).toEqual([NO_VALUE_TOTAL])

    // The pager rows count the results the groups hold, so a grouped feed which holds more than one page of
    // them is broken up the same way a flat one is.
    const pagers = page.locator(".pd-searchresultspager")
    await expect(pagers.first(), "the grouped results are broken up by pager rows").toBeVisible()
    expect((await reportedNumbers(pagers.last().locator(".pd-searchresultspager-count")))[1], "the pagers of a grouped feed count the whole result set").toBe(total)

    // Expanding the column turns every heading which stands for a document into the card of that document,
    // while the one which stands for none has no card to turn into and stays a heading. Everything has to be
    // revealed again first: expanding is a change of the search, which takes the feed back to its first page.
    await page.locator(".pd-searchresultgroup-button-expand").first().click()
    await loadAllGroups(page)
    await expect(page.locator(".pd-searchresultgroup-button-expand"), "no heading is left collapsed once the column is expanded").toHaveCount(0)
    await expect(missingGroupTitle(page), "the heading which stands for no document stays a heading when the column is expanded").toHaveCount(1)

    const loaded = await groups.count()
    console.log(`Successfully paged through a grouped search of ${total} results, which ended in ${loaded} groups including the one for the missing value.`)
  })

  test("Test a document which belongs under several group headings", async ({ context }) => {
    test.slow()

    const page = await context.newPage()

    await searchByClass(page, GROUPED_CLASS)
    await groupBy(page, GROUPED_PROPERTY)
    await loadAllGroups(page)

    // A document which carries several of the values it is grouped by belongs under each of their headings.
    // The first of them renders it in full and every later one renders a stub which points back to where it
    // was already shown, so a reader is told about the further placement instead of reading the document
    // again.
    const stub = page.locator(".pd-searchresult-link-duplicate").first()
    await expect(stub, "a document which belongs under several headings is rendered in full only once").toBeVisible()
    const at = new URL((await stub.getAttribute("href")) || "", PEERDB_URL).searchParams.get("at")
    expect(at, "the stub of a repeated result points back at the document it stands for").not.toBeNull()
    const full = page.locator(`[id="result-${at}"]`).filter({ hasNot: page.locator(".pd-searchresult-link-duplicate") })
    await expect(full, "the document the stub points back at is rendered in full somewhere on the page").toHaveCount(1)

    // Every result carries the identifier of the document it stands for as the identifier of its element
    // (SearchResult.vue), and the stub of a repeated result carries it as well, so a document which belongs
    // under two headings puts the same identifier on the page twice.
    //
    // TODO: Assert that no element identifier is used twice, and screenshot a grouped feed.
    //       An identifier has to name one element: the address of a walked result carries "at=<document id>"
    //       and the view scrolls to the element of that identifier, which can only ever be the first of
    //       them, which is exactly what the stub above links by. Until a repeated card stops repeating the
    //       identifier this asserts the repetition instead, and no grouped run here takes a screenshot,
    //       because every checkpoint refuses a page with a repeated element identifier.
    const repeated = await page.evaluate(() =>
      Object.entries(
        Array.from(document.querySelectorAll("[id]")).reduce<Record<string, number>>((counts, element) => {
          counts[element.id] = (counts[element.id] || 0) + 1
          return counts
        }, {}),
      )
        .filter(([, count]) => count > 1)
        .map(([id]) => id),
    )
    expect(repeated, "the stub repeats the element identifier of the card it points back at, which is the defect above").toContain(`result-${at}`)

    console.log(`Successfully verified that the document ${at} is rendered in full once and pointed back at from the group it belongs to as well.`)
  })

  test("Test expanding the groups of a grouped search", async ({ context }) => {
    const page = await context.newPage()

    await searchByClass(page, GROUPED_CLASS)
    await groupBy(page, GROUPED_PROPERTY)

    // A group heading is collapsed to the document it stands for and its count. Expanding is a property of
    // the column the results are grouped by rather than of the one heading which was clicked, so every
    // heading of that column turns into the full card of its document at once.
    const expand = page.locator(".pd-searchresultgroup-button-expand")
    const collapsed = await expand.count()
    expect(collapsed, "the grouped results are collapsed to headings").toBeGreaterThan(0)
    const cards = await searchResults(page).count()

    // What is asserted is that no heading of the column is left collapsed rather than an exact number of
    // expanded ones: an expanded heading is taller than a collapsed one, so expanding shortens the column and
    // the feed answers by revealing further groups, which are expanded as well.
    const collapse = page.locator(".pd-searchresultgroup-button-collapse")
    await expand.first().click()
    await expect.poll(() => collapse.count(), { message: "expanding a heading expands the column it belongs to" }).toBeGreaterThanOrEqual(collapsed)
    await expect(expand, "no heading of the expanded column is left collapsed").toHaveCount(0)
    await settle(page)
    expect(await searchResults(page).count(), "an expanded heading is rendered as the full card of its document").toBeGreaterThan(cards)

    // Collapsing takes the column back to its headings, so the choice is one a reader can undo.
    await collapse.first().click()
    await expect.poll(() => expand.count(), { message: "collapsing takes the column back to its headings" }).toBeGreaterThanOrEqual(collapsed)
    await expect(collapse, "no heading of the collapsed column is left expanded").toHaveCount(0)

    console.log(`Successfully expanded and collapsed the ${collapsed} group headings of a grouped search.`)
  })
})
