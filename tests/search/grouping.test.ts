import type { Locator, Page } from "@playwright/test"

import { documentIdOf, LANGUAGES, PROPERTY_IDS, searchByClass } from "../peerdb_utils"
import {
  checkpoint,
  checkpointElement,
  expandCheckbox,
  expect,
  expectNothingLoading,
  goHome,
  groupCheckbox,
  LOADING_TIMEOUT,
  openSortDialog,
  resultCount,
  resultIds,
  searchAgain,
  searchByProperty,
  settle,
  settleFilters,
  sortColumn,
  switchLanguage,
  test,
  volatile,
} from "../utils"

// The suffix the CSS classes of a sortable column end in: its type followed by the property identifier for a
// column which comes from a facet of the search, and the type alone for a column the dialog always offers.
// Only a reference column can be grouped by, so every column these tests group by is a reference one.
const LABEL_COLUMN = "label"
const WORLD_TYPE_COLUMN = `ref-${PROPERTY_IDS.HAS_PLANET_TYPE}`
const CONTACT_STATUS_COLUMN = `ref-${PROPERTY_IDS.HAS_CONTACT_STATUS}`
const CONTAINED_IN_COLUMN = `ref-${PROPERTY_IDS.CONTAINED_IN}`
const CULTURE_COLUMN = `ref-${PROPERTY_IDS.BELONGS_TO_CULTURE}`
const BIOME_COLUMN = `ref-${PROPERTY_IDS.HAS_BIOME}`

// The vocabularies the values grouped by are entries of.
const WORLD_TYPE_VOCABULARY = "PLANET_TYPE"
const CONTACT_STATUS_VOCABULARY = "CONTACT_STATUS"

// The search most of these tests group: the worlds of one world type. It is small enough for the feed to show
// every one of its results at once, which is what makes what is on the page the same on every run. A feed
// which still has results to reveal reveals them for as long as its column is shorter than the page it has to
// fill (fillColumn in SearchResultsFeed.vue), and it measures that column while the documents of the results
// it already has are still arriving, so how much of a larger search ends up on the page follows how quickly
// the site answers rather than anything a test does.
const GROUPED_TYPE = "EYEBALL"

// How the worlds of that type are classified by the contact status of what lives on them, and how many of
// them carry each classification, read off the "contactStatus" field of their files in testdata. Every one of
// them carries one, so the classifications cover the whole search and each of them becomes a group.
const CONTACT_STATUSES = [
  { key: "INDIRECT", worlds: 1 },
  { key: "LIMITED", worlds: 3 },
  { key: "NO_CONTACT", worlds: 1 },
  { key: "REMOTE_OBSERVATION", worlds: 2 },
  { key: "SUSTAINED", worlds: 1 },
]

// The classes whose documents the tests which need something other than one page of results group. Collectives
// are grouped by the culture they belong to, which some of them state and some do not, and moons by their
// biomes, which a moon may have several of.
const MISSING_CLASS = "COLLECTIVE"
const REPEATED_CLASS = "MOON"

// The collectives of the test data which belong to no culture at all, which is what puts them in the group of
// the results the property grouped by is missing from.
const WITHOUT_CULTURE = ["G2_BASIN_SHEET", "G2_FRACTURE_BELT", "G3_UVVEI_FRAME"]

// Opens the search these tests group. It is reached through the shortcut route, which prefilters a session to
// the documents carrying the given value for the given property.
async function openGroupedSearch(page: Page): Promise<void> {
  await searchByProperty(page, PROPERTY_IDS.HAS_PLANET_TYPE, await documentIdOf(WORLD_TYPE_VOCABULARY, GROUPED_TYPE))
}

// Adds the given column to the sort order and waits for the results the change produced.
async function addSortColumn(page: Page, column: string): Promise<void> {
  const button = page.locator(`.pd-searchsortdialog-button-add-${column}`)
  await expect(button, `the button which adds the ${column} column`).toBeVisible()
  await searchAgain(page, async () => await button.click())
}

// Clicks the given checkbox of the sort dialog and waits for the results the change produced. The checkbox
// shows what the search session holds rather than what was clicked, so it is clicked once and the state it
// ends in is asserted afterwards.
async function toggleCheckbox(page: Page, checkbox: Locator, what: string): Promise<void> {
  await expect(checkbox, what).toBeVisible()
  await searchAgain(page, async () => await checkbox.click())
}

// Sorts the results by the given reference column and then groups them by it, which is the order a user goes
// in: the group checkbox is offered only for a column which is already sorted by.
async function groupByColumn(page: Page, column: string): Promise<void> {
  await addSortColumn(page, column)
  await expect(groupCheckbox(page, column), `the group checkbox of the ${column} column`).not.toBeChecked()
  await toggleCheckbox(page, groupCheckbox(page, column), `the group checkbox of the ${column} column`)
  await expect(groupCheckbox(page, column), `the grouped ${column} column`).toBeChecked()
}

// The groups the results are laid out in at the top of the results column, which for two grouped columns are
// the groups of the first of them.
function topGroups(page: Page): Locator {
  return page.locator("#search-results > .pd-searchresultgroup")
}

// The headings of the groups of the first grouped column, and of the groups nested inside them, which is what
// tells the two columns apart on the page.
function outerHeadings(page: Page): Locator {
  return page.locator("#search-results > .pd-searchresultgroup > .pd-searchresultgroup-header")
}

function innerHeadings(page: Page): Locator {
  return page.locator(".pd-searchresultgroup .pd-searchresultgroup > .pd-searchresultgroup-header")
}

// The result cards the groups of a column render themselves as while the column is expanded, which stand in
// place of its headings.
function innerCards(page: Page): Locator {
  return page.locator(".pd-searchresultgroup .pd-searchresultgroup > .pd-searchresult")
}

// The group of the results whose value for the property grouped by is the given document, addressed by the
// link its heading carries so that no label has to be named.
function groupOf(page: Page, valueId: string): Locator {
  return page.locator(`.pd-searchresultgroup:has(> .pd-searchresultgroup-header > a[href="/d/${valueId}"])`)
}

// The group of the results which state no value at all for the property grouped by. It stands for no document,
// so its heading names it in words instead of linking to one.
function missingGroup(page: Page): Locator {
  return page.locator(".pd-searchresultgroup:has(> .pd-searchresultgroup-header > i.pd-searchresultgroup-title)")
}

// The results a group holds, which is one entry per placement of a document in it.
function groupItems(group: Locator): Locator {
  return group.locator(":scope > .pd-searchresultgroup-list > .pd-searchresultgroup-item")
}

// How many results a group heading says the group holds. The number is rendered inside brackets, so only its
// digits are read, which also keeps this independent of the language the site is being read in.
async function groupCount(group: Locator, what: string): Promise<number> {
  const count = group.locator(":scope > .pd-searchresultgroup-header > .pd-searchresultgroup-count")
  await expect(count, `the count of ${what}`).toBeVisible()
  const text = (await count.textContent()) || ""
  const digits = text.replace(/\D/g, "")
  expect(digits, `the count of ${what} is a number`).not.toBe("")
  return Number(digits)
}

// The identifiers of the documents the given cards stand for, read off the link each of them offers to its
// own page.
async function cardIds(cards: Locator): Promise<Array<string>> {
  return await cards.locator(".pd-searchresult-link-details").evaluateAll((links) =>
    links.map((link) => {
      const match = /\/d\/([0-9A-Za-z]+)/.exec(link.getAttribute("href") || "")
      return match ? match[1] : ""
    }),
  )
}

// Reveals every result of a search which holds more of them than one page, so that a group which sorts after
// the ones the first page reaches can be asserted on. A search which is showing everything already has no
// button to press and is left as it is.
//
// The button is pressed until there is none left instead of using loadAllResults, which waits until the page
// holds as many result cards as the search found: a grouped feed renders a card for every placement of a
// document, and a document which is placed in several groups then has more than one card. The press is
// dispatched rather than clicked, because reaching the button with a click means scrolling the page, which
// reveals a page by itself and takes the button out from under the press, which is also how the filters panel
// is driven (settleFilters in tests/utils.ts).
async function revealAllResults(page: Page): Promise<void> {
  const loadMore = page.locator("#searchresultsfeed-button-loadmore")
  await expect
    .poll(
      async () => {
        // A button which is on its way out counts as gone: the feed is then showing everything it has.
        if (!(await loadMore.isVisible().catch(() => false))) {
          return false
        }
        await loadMore.dispatchEvent("click").catch(() => null)
        return true
      },
      { message: "the feed reveals every result it holds", timeout: 2 * LOADING_TIMEOUT },
    )
    .toBe(false)
  await settle(page)
}

// Closes the sort dialog. It covers the whole viewport, so the group headings behind it can only be expanded
// or collapsed in place once it is gone.
async function closeSortDialog(page: Page): Promise<void> {
  const closeButton = page.locator(".pd-searchsortdialog-button-close")
  await expect(closeButton, "the button which closes the sort dialog").toBeVisible()
  await closeButton.click()
  await expect(page.locator(".pd-searchsortdialog-panel"), "the closed sort dialog").toBeHidden()
}

// Screenshots the dialog itself. The screenshot covers the viewport rather than the whole page, because the
// dialog is drawn over the page and only dims what is behind it. A column which comes from a facet is named by
// the property it groups on, which the dialog fetches, so the capture waits until every label has resolved.
async function checkpointDialog(page: Page, name: string): Promise<void> {
  await expect(page.locator(".pd-searchsortdialog-panel"), "the sort dialog").toBeVisible()
  await expectNothingLoading(page)
  await checkpoint(page, name, { mask: volatile(page), fullPage: false })
}

test.describe("PeerDB Search Grouping Flows", () => {
  test("Test grouping the results by a reference column", async ({ context }) => {
    const page = await context.newPage()

    await openGroupedSearch(page)
    await openSortDialog(page)

    // A column has to be sorted by before it can be grouped by, and sorting alone still shows a flat list.
    await addSortColumn(page, CONTACT_STATUS_COLUMN)
    await expect(page.locator(".pd-searchresultgroup"), "sorting by a column alone groups nothing").toHaveCount(0)
    const flat = await resultIds(page)
    expect(flat.length, "the results to group").toBeGreaterThan(1)
    await checkpointDialog(page, "grouping-dialog-column-added")

    // Grouping turns the flat list into a tree: each group is a heading naming the value grouped by, with the
    // count of the results under it and the results themselves below.
    await toggleCheckbox(page, groupCheckbox(page, CONTACT_STATUS_COLUMN), "the group checkbox of the contact status column")
    await expect(groupCheckbox(page, CONTACT_STATUS_COLUMN), "the grouped column").toBeChecked()
    await expect(topGroups(page).first(), "the first group").toBeVisible()
    await expect(topGroups(page).first().locator(".pd-searchresultgroup-header").first(), "the heading of the first group").toBeVisible()
    await expect(topGroups(page).first().locator(".pd-searchresultgroup-title").first(), "the title of the first group").toBeVisible()
    await expect(topGroups(page).first().locator(".pd-searchresultgroup-count").first(), "the count of the first group").toBeVisible()
    await expect(topGroups(page).first().locator(".pd-searchresultgroup-list").first(), "the results of the first group").toBeVisible()
    // The same results are on the page, laid out under headings. Which order they come in inside a group is
    // not the order the flat list had them in: the flat list is ordered by the column alone, while a group
    // holds the results of one value of it and orders them among themselves.
    expect([...(await resultIds(page))].sort(), "grouping lays the same results out under headings").toEqual([...flat].sort())
    await checkpointDialog(page, "grouping-dialog-grouped")

    await closeSortDialog(page)
    await settleFilters(page)
    await checkpoint(page, "grouping-results-grouped", { mask: volatile(page) })

    // Ungrouping the column leaves it sorted by, so the results come back in the order they were in before
    // they were grouped.
    await openSortDialog(page)
    await toggleCheckbox(page, groupCheckbox(page, CONTACT_STATUS_COLUMN), "the group checkbox of the contact status column")
    await expect(groupCheckbox(page, CONTACT_STATUS_COLUMN), "the ungrouped column").not.toBeChecked()
    await expect(page.locator(".pd-searchresultgroup"), "the ungrouped results").toHaveCount(0)
    expect(await resultIds(page), "the results come back in the order they were in before they were grouped").toEqual(flat)
    await closeSortDialog(page)
    await settleFilters(page)
    await checkpoint(page, "grouping-results-ungrouped", { mask: volatile(page) })

    console.log(`Successfully grouped ${flat.length} results by a reference column and ungrouped them again.`)
  })

  test("Test the groups a reference column produces", async ({ context }) => {
    const page = await context.newPage()

    await openGroupedSearch(page)
    const total = await resultCount(page)
    await openSortDialog(page)
    await groupByColumn(page, CONTACT_STATUS_COLUMN)
    await closeSortDialog(page)

    // The values the results carry for the property grouped by are exactly the groups, each headed by a link
    // to the document standing for it, saying how many results it holds and holding that many of them.
    await expect(topGroups(page), "one group per value the results carry").toHaveCount(CONTACT_STATUSES.length)
    let grouped = 0
    for (const status of CONTACT_STATUSES) {
      const group = groupOf(page, await documentIdOf(CONTACT_STATUS_VOCABULARY, status.key))
      await expect(group, `the group of ${status.key}`).toBeVisible()
      expect(await groupCount(group, `the group of ${status.key}`), `the number of worlds classified as ${status.key}`).toBe(status.worlds)
      await expect(groupItems(group), `the results of the group of ${status.key}`).toHaveCount(status.worlds)
      grouped += status.worlds
    }
    expect(grouped, "the groups hold every result the search found").toBe(total)
    await expect(missingGroup(page), "every result carries a value, so nothing is grouped as missing").toHaveCount(0)

    // The largest group is the one worth looking at: a heading, a count and the results under it.
    const largest = groupOf(page, await documentIdOf(CONTACT_STATUS_VOCABULARY, "LIMITED"))
    await settleFilters(page)
    await checkpointElement(page, largest, "grouping-group-largest")

    console.log(`Successfully verified the ${CONTACT_STATUSES.length} groups a reference column laid ${total} results out in.`)
  })

  test("Test expanding and collapsing a group in place", async ({ context }) => {
    const page = await context.newPage()

    await openGroupedSearch(page)
    await openSortDialog(page)
    await groupByColumn(page, CONTACT_STATUS_COLUMN)
    await closeSortDialog(page)

    const headings = await topGroups(page).count()
    await expect(page.locator(".pd-searchresultgroup-button-expand"), "every heading offers to be expanded").toHaveCount(headings)
    await settleFilters(page)
    await checkpoint(page, "grouping-groups-collapsed", { mask: volatile(page) })

    // A heading expands in place into the full result card of the document it groups by. The control applies
    // to the whole group column, so every heading of the column is expanded at once and each of them then
    // offers to collapse it again.
    await searchAgain(page, async () => await page.locator(".pd-searchresultgroup-button-expand").first().click())
    await expect(page.locator(".pd-searchresultgroup-button-collapse"), "every heading of the column offers to collapse it").toHaveCount(headings)
    await expect(page.locator(".pd-searchresultgroup-button-expand"), "no heading of the column is collapsed any more").toHaveCount(0)
    await expect(topGroups(page).first().locator(".pd-searchresult").first(), "the expanded group value is rendered as a result card").toBeVisible()
    await settleFilters(page)
    await checkpoint(page, "grouping-groups-expanded", { mask: volatile(page) })

    // Collapsing brings the one-line headings back.
    await searchAgain(page, async () => await page.locator(".pd-searchresultgroup-button-collapse").first().click())
    await expect(page.locator(".pd-searchresultgroup-button-expand"), "the headings offer to be expanded again").toHaveCount(headings)
    await expect(page.locator(".pd-searchresultgroup-button-collapse"), "nothing is expanded any more").toHaveCount(0)
    await settleFilters(page)
    await checkpoint(page, "grouping-groups-collapsed-again", { mask: volatile(page) })

    console.log(`Successfully expanded and collapsed ${headings} group headings through the controls on them.`)
  })

  test("Test the expand checkbox of a grouped column", async ({ context }) => {
    const page = await context.newPage()

    await openGroupedSearch(page)
    await openSortDialog(page)
    await groupByColumn(page, CONTACT_STATUS_COLUMN)

    // The expand checkbox is offered only for a column which is grouped by, and starts out unchecked, with
    // every group rendered as a one-line heading offering to expand it.
    await expect(expandCheckbox(page, CONTACT_STATUS_COLUMN), "the expand checkbox of the grouped column").toBeVisible()
    await expect(expandCheckbox(page, CONTACT_STATUS_COLUMN), "the expand checkbox of a column which was just grouped").not.toBeChecked()
    await expect(page.locator(".pd-searchresultgroup-button-expand").first(), "the control which expands a group heading").toBeVisible()
    await checkpointDialog(page, "grouping-dialog-expand-unchecked")

    // Checking it renders every group of the column as the full result card of the document it groups by,
    // which is the same change the expand control on a heading makes.
    await toggleCheckbox(page, expandCheckbox(page, CONTACT_STATUS_COLUMN), "the expand checkbox of the grouped column")
    await expect(expandCheckbox(page, CONTACT_STATUS_COLUMN), "the checked expand checkbox").toBeChecked()
    await expect(page.locator(".pd-searchresultgroup-button-collapse").first(), "an expanded group offers to be collapsed").toBeVisible()
    await expect(page.locator(".pd-searchresultgroup-button-expand"), "nothing is left to expand").toHaveCount(0)
    await checkpointDialog(page, "grouping-dialog-expand-checked")

    // The card an expanded group renders is the card of the document the group gathers, so the values grouped
    // by are on the page as results of their own.
    await closeSortDialog(page)
    const expanded = await cardIds(topGroups(page).locator(":scope > .pd-searchresult"))
    const values = await Promise.all(CONTACT_STATUSES.map(async (status) => await documentIdOf(CONTACT_STATUS_VOCABULARY, status.key)))
    expect([...expanded].sort(), "each group is rendered as the card of the document it gathers").toEqual([...values].sort())
    await settleFilters(page)
    await checkpoint(page, "grouping-results-expanded", { mask: volatile(page) })

    // Unchecking it collapses every group of the column back into a one-line heading.
    await openSortDialog(page)
    await toggleCheckbox(page, expandCheckbox(page, CONTACT_STATUS_COLUMN), "the expand checkbox of the grouped column")
    await expect(expandCheckbox(page, CONTACT_STATUS_COLUMN), "the unchecked expand checkbox").not.toBeChecked()
    await expect(page.locator(".pd-searchresultgroup-button-expand").first(), "a collapsed group offers to be expanded").toBeVisible()
    await expect(page.locator(".pd-searchresultgroup-button-collapse"), "nothing is left expanded").toHaveCount(0)
    await closeSortDialog(page)
    await settleFilters(page)
    await checkpoint(page, "grouping-results-collapsed", { mask: volatile(page) })

    console.log(`Successfully expanded and collapsed ${values.length} groups through the expand checkbox of the sort dialog.`)
  })

  test("Test grouping by two reference columns", async ({ context }) => {
    const page = await context.newPage()

    await openGroupedSearch(page)
    await openSortDialog(page)

    await groupByColumn(page, CONTACT_STATUS_COLUMN)
    const outer = await topGroups(page).count()
    expect(outer, "the groups of the first column").toBe(CONTACT_STATUSES.length)

    // Grouping by a second column splits each group of the first one further, so the results end up under two
    // headings instead of one. The column grouped by here is the place each world is contained in, and a place
    // is contained in another one in turn, so its groups nest as deeply as the places do.
    await groupByColumn(page, CONTAINED_IN_COLUMN)
    await expect(topGroups(page), "the groups of the first column are still the outer ones").toHaveCount(outer)
    const nested = page.locator(".pd-searchresultgroup .pd-searchresultgroup")
    await expect(nested.first(), "the groups of the second column").toBeVisible()
    expect(await nested.count(), "the second column splits the groups of the first one further").toBeGreaterThan(outer)
    await expect(innerHeadings(page).first().locator(".pd-searchresultgroup-title"), "the title of a nested group").toBeVisible()
    for (const status of CONTACT_STATUSES) {
      const group = groupOf(page, await documentIdOf(CONTACT_STATUS_VOCABULARY, status.key))
      expect(await groupCount(group, `the group of ${status.key}`), `the number of worlds classified as ${status.key}`).toBe(status.worlds)
    }

    // Only a reference column can be grouped by, so a column which orders the results by something they carry
    // themselves rather than by a document they point at is offered no group checkbox at all.
    await addSortColumn(page, LABEL_COLUMN)
    await expect(sortColumn(page, LABEL_COLUMN), "the column sorting by the display label").toBeVisible()
    await expect(groupCheckbox(page, LABEL_COLUMN), "a column which is not a reference column cannot be grouped by").toHaveCount(0)
    await checkpointDialog(page, "grouping-dialog-two-columns")

    await closeSortDialog(page)
    await settleFilters(page)
    await checkpoint(page, "grouping-results-two-columns", { mask: volatile(page) })

    console.log(`Successfully grouped the results by two reference columns, which laid ${outer} groups out with ${await nested.count()} groups inside them.`)
  })

  test("Test expanding only the second grouped column", async ({ context }) => {
    const page = await context.newPage()

    await openGroupedSearch(page)
    await openSortDialog(page)
    // The search is of the worlds of one world type, so the column grouping by that type gathers every result
    // into a single group, and the column grouping by the contact status splits that group into one per value.
    await groupByColumn(page, WORLD_TYPE_COLUMN)
    await groupByColumn(page, CONTACT_STATUS_COLUMN)
    await closeSortDialog(page)

    const outer = await outerHeadings(page).count()
    const inner = await innerHeadings(page).count()
    expect(outer, "the headings of the first column").toBe(1)
    expect(inner, "the headings of the second column").toBe(CONTACT_STATUSES.length)

    // Expanding is a property of the group column and not of the level a heading happens to sit at, so
    // expanding through a heading of the second column turns the headings of that column into result cards and
    // leaves the heading of the first column as it is.
    await searchAgain(page, async () => await innerHeadings(page).locator(".pd-searchresultgroup-button-expand").first().click())
    await expect(outerHeadings(page), "the heading of the first column stays a heading").toHaveCount(outer)
    await expect(outerHeadings(page).locator(".pd-searchresultgroup-button-expand"), "the first column still offers to be expanded").toHaveCount(outer)
    await expect(topGroups(page).locator(":scope > .pd-searchresult"), "no group of the first column is rendered as a card").toHaveCount(0)
    await expect(innerHeadings(page), "no heading of the second column is left").toHaveCount(0)
    await expect(innerCards(page), "every group of the second column is rendered as a card").toHaveCount(inner)
    await expect(page.locator(".pd-searchresultgroup-button-collapse"), "every group of the second column offers to be collapsed").toHaveCount(inner)

    // The cards which took the place of the headings are the cards of the documents those headings named.
    const values = await Promise.all(CONTACT_STATUSES.map(async (status) => await documentIdOf(CONTACT_STATUS_VOCABULARY, status.key)))
    expect((await cardIds(innerCards(page))).sort(), "the cards of the second column are the documents it groups by").toEqual([...values].sort())
    await settleFilters(page)
    await checkpoint(page, "grouping-results-second-expanded", { mask: volatile(page) })

    console.log(`Successfully expanded ${inner} groups of the second column and verified that the ${outer} group of the first one stayed collapsed.`)
  })

  test("Test a group value which sits under more than one group", async ({ context }) => {
    const page = await context.newPage()

    await openGroupedSearch(page)
    await openSortDialog(page)
    // Grouping by the contact status first and by the world type second puts the same world type under every
    // one of the contact status groups, because the search is of the worlds of that one type.
    await groupByColumn(page, CONTACT_STATUS_COLUMN)
    await groupByColumn(page, WORLD_TYPE_COLUMN)
    await closeSortDialog(page)

    const outer = await outerHeadings(page).count()
    expect(outer, "the groups of the first column").toBe(CONTACT_STATUSES.length)
    await expect(innerHeadings(page), "the second column has a group under each of them").toHaveCount(outer)

    // A group value which sits under more than one group is rendered once under each of them, so expanding the
    // column puts the card of the same document on the page once per group it sits under.
    await searchAgain(page, async () => await innerHeadings(page).locator(".pd-searchresultgroup-button-expand").first().click())
    await expect(innerCards(page), "the value is rendered as a card under each group it sits under").toHaveCount(outer)
    const worldType = await documentIdOf(WORLD_TYPE_VOCABULARY, GROUPED_TYPE)
    expect(await cardIds(innerCards(page)), "every one of those cards is of the same document").toEqual(Array(outer).fill(worldType))

    // Every one of those cards carries the element identifier of the document it shows, so the page ends up
    // with the same identifier on it several times.
    //
    // TODO: Screenshot this page once a repeated card no longer repeats an element identifier.
    //       A result card is given id="result-<document id>" (SearchResult.vue) whatever placement it is
    //       rendered for, so a value sitting under several groups puts that identifier on the page once per
    //       group. It makes the markup invalid, and it makes the identifier unusable for reaching a card:
    //       the address of a walked result carries "at=<document id>" and the view scrolls to the element of
    //       that identifier, which can only ever be the first of them. Screenshotting is what would catch it
    //       from here, because checkpoint refuses a page with a repeated element identifier.
    await settleFilters(page)
    const repeated = await page.evaluate(() => {
      const ids = Array.from(document.querySelectorAll("[id^='result-']"), (el) => el.id)
      return ids.length - new Set(ids).size
    })
    expect(repeated, "the repeated cards repeat their element identifier, which is the defect above").toBe(outer - 1)

    console.log(`Successfully verified that a group value sitting under ${outer} groups is rendered as a card under each of them.`)
  })

  test("Test the group of the results which lack the grouped property", async ({ context }) => {
    const page = await context.newPage()

    await searchByClass(page, MISSING_CLASS)
    const total = await resultCount(page)
    await openSortDialog(page)
    await groupByColumn(page, CULTURE_COLUMN)
    await closeSortDialog(page)

    // The results which state no value for the property grouped by are gathered in a group of their own, which
    // sorts after every group which stands for a document. It is named in words rather than by a document, so
    // nothing is fetched for it and nothing fails to be fetched either.
    await revealAllResults(page)
    const missing = missingGroup(page)
    await expect(missing, "the group of the results which state no value").toHaveCount(1)
    await expect(
      topGroups(page).last().locator(":scope > .pd-searchresultgroup-header > i.pd-searchresultgroup-title"),
      "the group of the results which state no value is the last one",
    ).toHaveCount(1)
    await expect(missing.locator(":scope > .pd-searchresultgroup-header > i.pd-searchresultgroup-title"), "the heading which names the missing value").not.toHaveText(
      /^\s*$/,
    )
    await expect(missing.locator(":scope > .pd-searchresultgroup-header > a"), "the heading of the missing value links to no document").toHaveCount(0)
    await expect(page.locator(".pd-withdocument-error"), "nothing failed to be fetched for the missing value").toHaveCount(0)

    // The collectives of the test data which belong to no culture are the ones gathered there.
    const without = await Promise.all(WITHOUT_CULTURE.map(async (key) => await documentIdOf(MISSING_CLASS, key)))
    const inMissing = await cardIds(missing)
    for (const id of without) {
      expect(inMissing, `the collective ${id} which belongs to no culture is grouped as missing`).toContain(id)
    }
    expect(await groupCount(missing, "the group of the missing value"), "the group of the missing value counts the results it holds").toBe(inMissing.length)
    await checkpointElement(page, missing, "grouping-group-missing")

    // A group standing for a document renders as that document's card when the column is expanded, while the
    // group of the missing value has no document to render and stays a one-line heading.
    await openSortDialog(page)
    await toggleCheckbox(page, expandCheckbox(page, CULTURE_COLUMN), "the expand checkbox of the grouped column")
    await closeSortDialog(page)
    await revealAllResults(page)
    await expect(missingGroup(page), "the group of the missing value stays a heading while the column is expanded").toHaveCount(1)
    await expect(outerHeadings(page), "the group of the missing value is the only heading left").toHaveCount(1)
    await expect(page.locator(".pd-withdocument-error"), "nothing failed to be fetched for the expanded groups").toHaveCount(0)
    await checkpointElement(page, missingGroup(page), "grouping-group-missing-expanded")

    console.log(`Successfully verified the group of the ${inMissing.length} results of ${total} which state no value for the property grouped by.`)
  })

  test("Test the pagers inside the groups", async ({ context }) => {
    const page = await context.newPage()

    await searchByClass(page, MISSING_CLASS)
    const total = await resultCount(page)
    await openSortDialog(page)
    await groupByColumn(page, CULTURE_COLUMN)
    await closeSortDialog(page)

    // A pager stands before every tenth result, so a feed showing its first page carries none.
    await expect(page.locator(".pd-searchresultspager"), "the first page of results carries no pager").toHaveCount(0)

    // Revealing the rest brings a pager for each page boundary they crossed. A boundary which falls inside a
    // group is rendered as an entry of that group, and one which falls where a group starts is rendered above
    // that group's heading instead, so the heading keeps its results under it.
    await revealAllResults(page)
    const pagers = Math.floor((total - 1) / 10)
    expect(pagers, "the page boundaries the results cross").toBeGreaterThan(1)
    await expect(page.locator(".pd-searchresultspager"), "a pager stands at each page boundary").toHaveCount(pagers)
    await expect(page.locator(".pd-searchresultgroup-item-pager"), "a boundary inside a group is rendered as an entry of it").toHaveCount(pagers - 1)
    await expect(page.locator("#search-results > .pd-searchresultspager"), "a boundary where a group starts is rendered above it").toHaveCount(1)
    await expect(page.locator("#search-results > .pd-searchresultspager + .pd-searchresultgroup"), "the group whose heading such a pager stands above").toHaveCount(1)

    const pager = page.locator(".pd-searchresultgroup-item-pager").first()
    await expect(pager.locator(".pd-searchresultspager-count"), "the pager reports how far into the results it stands").not.toHaveText(/^\s*$/)
    await expect(pager.locator(".pd-searchresultspager-track"), "the track of the pager").toBeVisible()
    await expect(pager.locator(".pd-searchresultspager-thumb"), "the thumb of the pager").toBeVisible()
    await settleFilters(page)
    await checkpointElement(page, pager, "grouping-pager")

    console.log(`Successfully revealed ${total} grouped results and verified the ${pagers} pagers the page boundaries they crossed brought.`)
  })

  test("Test a document which belongs to several groups", async ({ context }) => {
    const page = await context.newPage()

    await searchByClass(page, REPEATED_CLASS)
    const total = await resultCount(page)
    await openSortDialog(page)
    await groupByColumn(page, BIOME_COLUMN)
    await closeSortDialog(page)
    await revealAllResults(page)

    // A moon may state several biomes, and it is then placed in a group for each of them. Its contents are
    // shown at the first of those placements only, and every other placement is a card which says so and links
    // back to the first one through the "at" query parameter of the page.
    //
    // What the cards say is asserted rather than screenshotted, because a page carrying a card per placement
    // carries the identifier of the placed document as many times, which the test above covers.
    const duplicates = page.locator(".pd-searchresult-link-duplicate")
    const repeated = await duplicates.count()
    expect(repeated, "a document with several values is placed in a group for each of them").toBeGreaterThan(0)
    const back = await duplicates.first().getAttribute("href")
    const at = new URL(back || "", "https://localhost").searchParams.get("at")
    expect(at, "the repeated placement links back to the first one").not.toBeNull()
    expect(await page.locator(`#result-${at}`).count(), "the document the repeated placement links back to is the one on the page more than once").toBeGreaterThan(1)
    const stub = page.locator(".pd-searchresultdocument-duplicate").first()
    await expect(stub.locator(".pd-searchresult-link-title"), "a repeated placement is headed by the title of its document").toBeVisible()
    await expect(stub.locator(".pd-fieldsview"), "a repeated placement shows none of the contents of its document").toHaveCount(0)
    expect(await page.locator(".pd-searchresult").count(), "the page holds a card for every placement").toBe(total + repeated)

    console.log(`Successfully verified that ${repeated} placements of the ${total} results are repeated ones which link back to their first placement.`)
  })

  for (const language of LANGUAGES) {
    test(`Test the grouped results in ${language}`, async ({ context }) => {
      const page = await context.newPage()

      await goHome(page)
      await switchLanguage(page, language)
      await openGroupedSearch(page)
      await openSortDialog(page)
      await groupByColumn(page, CONTACT_STATUS_COLUMN)
      await closeSortDialog(page)

      // Every group is headed by the document it gathers, named in the language the site is being read in.
      // What each of them says is not asserted here, only that every one of them says something.
      await expect(topGroups(page), `the groups shown in ${language}`).toHaveCount(CONTACT_STATUSES.length)
      for (let i = 0; i < CONTACT_STATUSES.length; i++) {
        const group = topGroups(page).nth(i)
        await expect(group.locator(".pd-searchresultgroup-title").first(), `the title of group ${i} in ${language}`).not.toHaveText(/^\s*$/)
        expect(await groupCount(group, `group ${i} in ${language}`), `the count of group ${i} in ${language}`).toBeGreaterThan(0)
      }
      await settleFilters(page)
      await checkpoint(page, `grouping-results-${language}`, { mask: volatile(page) })

      console.log(`Successfully verified ${CONTACT_STATUSES.length} groups of results in ${language}.`)
    })
  }
})
