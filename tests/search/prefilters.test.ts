import type { Locator, Page } from "@playwright/test"

import { CLASS_IDS, documentIdOf, LANGUAGES, PROPERTY_IDS, searchByClass } from "../peerdb_utils"
import {
  checkpoint,
  checkpointElement,
  expect,
  expectNothingLoading,
  expectResults,
  goHome,
  openDocument,
  PEERDB_URL,
  resultCount,
  searchAgain,
  settle,
  settleDocument,
  settleFilters,
  signIn,
  switchLanguage,
  test,
  volatile,
} from "../utils"

// The document the referencing searches are run from. A culture is referenced by everything the culture
// leaves behind (its practices, its artifacts, its narratives) and by everyone who belongs to it, so it is
// referenced by many documents of several classes, which is what a scope worth widening again needs.
const CULTURE = await documentIdOf("CULTURE", "G4_CU_LADDER_GORGE")

// The document the create shortcut is read from. A world declares a shortcut which lists its regions and
// offers creating another one, which is the only shortcut of the class that offers creating.
const PLANET = await documentIdOf("PLANET", "G1_SURVEY_GRID_44_B")

// The documents the explicit document scope is built from. Two galaxies are picked by name rather than off
// the top of a result list, so the scope is the same on every run.
const SCOPED = [await documentIdOf("GALAXY", "G1_MILKY_WAY"), await documentIdOf("GALAXY", "G2_UNDERCOUNT")]

// The role which may start a new region, which is what makes the create half of a search shortcut appear.
const CREATING_ROLE = "surveyor"

// The search the first shortcut of the culture opens, written out as the address it leads to, so that a test
// which is about the search alone reaches it without going through the document it is offered from.
const SHORTCUT_SEARCH = `${PEERDB_URL}/s?${PROPERTY_IDS.OF_CULTURE}=${CULTURE}&${PROPERTY_IDS.INSTANCE_OF}=${CLASS_IDS.PRACTICE}`

// How many results the feed reveals of a search which found more than fits on the first screen is decided by
// how tall the results it has already rendered are, which depends on how quickly the site answers while the
// suite runs next to itself, so it is not the same from one run to the next. A whole page screenshot of such
// a search would therefore differ for a reason which is not a regression. Those searches are captured as the
// viewport shows them, which is the same however many results are revealed below it, while a search whose
// results all fit into the first batch is captured whole.
const VIEWPORT_ONLY = { fullPage: false }

// The count a search shortcut shows in parentheses after its label. The count is fetched after the document
// renders, so this waits for it to appear before reading it.
async function linkCount(link: Locator, what: string): Promise<number> {
  await expect(link, `the count of ${what}`).toHaveText(/\(\d+\)/)
  const text = (await link.textContent()) || ""
  const match = /\((\d+)\)/.exec(text)
  expect(match, `the count of ${what} is a number`).not.toBeNull()
  return Number(match![1])
}

// Follows a search shortcut of the document currently shown. A shortcut which also offers creating holds two
// controls next to each other, so the link is addressed inside the shortcut rather than as the shortcut
// itself. The caller waits for the search the link opens.
async function followShortcut(shortcut: Locator): Promise<void> {
  const link = shortcut.locator(".pd-searchshortcutlink-link")
  await expect(link, "the link of the shortcut to follow").toBeVisible()
  await link.click()
}

// The line of the filters column which says what the search is scoped to. The scope is written twice, once
// for the screen next to the control which clears it and once into the printed layout, so it is addressed
// inside the filters column, which is the copy a user reads and the only one which is not print-only.
function scopeNotice(page: Page, kind: "scoped" | "referencing"): Locator {
  return page.locator(`.pd-searchresultsfeed-panel-filters .pd-searchresultsfeed-text-${kind}`)
}

test.describe("PeerDB Prefilter Flows", () => {
  test("Test referenced by opens a search of the referencing documents", async ({ context }) => {
    const page = await context.newPage()

    await openDocument(page, CULTURE)
    await settleDocument(page)

    const referencedBy = page.locator("#documentget-button-referencedby")
    await expect(referencedBy, "the document offers a search of the documents referencing it").toBeVisible()
    const promised = await linkCount(referencedBy, "the referenced by link")
    // The tests below widen this scope again, so the document has to be referenced by more than one document
    // for widening to have anything to add.
    expect(promised, "the culture is referenced by several documents").toBeGreaterThan(1)
    await checkpoint(page, "prefilters-culture-document", { mask: volatile(page) })

    await followShortcut(referencedBy)
    await expectResults(page)

    // The scope is named in the filters column, next to the control which clears it again.
    await expect(scopeNotice(page, "referencing"), "the search says which document it is scoped to").toBeVisible()
    await expect(page.locator(".pd-searchresultsfeed-button-clearreverse"), "the referencing scope offers to be cleared").toBeVisible()
    // Scoping to the referencing documents is not a filter, so nothing is prefiltered.
    await expect(page.locator(".pd-prefilterlabel"), "the referencing scope is not a prefilter").toHaveCount(0)
    // The search has to deliver exactly what the link promised.
    await expect.poll(() => resultCount(page), { message: "the search finds the promised referencing documents" }).toBe(promised)
    await settleFilters(page)
    await checkpoint(page, "prefilters-referenced-by-search", { mask: volatile(page), ...VIEWPORT_ONLY })

    console.log(`Successfully opened a search scoped to the ${promised} documents referencing a culture.`)
  })

  test("Test clearing the referencing scope widens the results", async ({ context }) => {
    const page = await context.newPage()

    await page.goto(`${PEERDB_URL}/s?reverse=${CULTURE}`)
    await expectResults(page)
    const scoped = await resultCount(page)
    expect(scoped, "the scoped search finds the referencing documents").toBeGreaterThan(1)
    const clearReverse = page.locator(".pd-searchresultsfeed-button-clearreverse")
    await expect(clearReverse, "the referencing scope offers to be cleared").toBeVisible()
    await settleFilters(page)
    await checkpoint(page, "prefilters-referencing-scope-before-clear", { mask: volatile(page), ...VIEWPORT_ONLY })

    await searchAgain(page, async () => {
      await clearReverse.click()
    })
    // The notice and its control go away together with the scope they describe.
    await expect(clearReverse, "the cleared scope no longer offers to be cleared").toBeHidden()
    await expect(scopeNotice(page, "referencing"), "the cleared scope is no longer named").toHaveCount(0)

    // Without the scope the same session searches every document, so it finds more than it did.
    await expect.poll(() => resultCount(page), { message: "clearing the referencing scope widens the results" }).toBeGreaterThan(scoped)
    const widened = await resultCount(page)
    // The filters of a search over every document are far taller than a screenshot should be, and the panel
    // stops growing on its own once it fills the page, so this is the one state which is captured as it is
    // seen rather than with every facet added to it.
    await settle(page)
    await checkpoint(page, "prefilters-referencing-scope-after-clear", { mask: volatile(page), ...VIEWPORT_ONLY })

    console.log(`Successfully cleared the referencing scope of a search and widened it from ${scoped} to ${widened} results.`)
  })

  test("Test a search shortcut of a class prefilters a search", async ({ context }) => {
    const page = await context.newPage()

    await openDocument(page, CULTURE)
    await settleDocument(page)

    // A class declares the saved searches its documents offer, and each of them is listed on the document
    // with the number of results it would find.
    const shortcuts = page.locator(".pd-documentget-link-shortcut")
    await expect(shortcuts, "the class declares four search shortcuts").toHaveCount(4)
    const first = shortcuts.first()
    const promised = await linkCount(first, "the first search shortcut")
    expect(promised, "the first shortcut of the culture finds something").toBeGreaterThan(0)
    await checkpointElement(page, page.locator(".pd-documentget-list-shortcuts"), "prefilters-culture-shortcuts")

    await followShortcut(first)
    await expectResults(page)

    // The shortcut of this class narrows the search by two filters at once, the property pointing back at the
    // document and the class of what is searched for, and both are named in the filters column so that it is
    // clear why the results are limited.
    const prefilterLabels = page.locator(".pd-prefilterlabel")
    await expect(prefilterLabels, "the search is limited by the two filters the shortcut carries").toHaveCount(2)
    for (let i = 0; i < 2; i++) {
      await expect(prefilterLabels.nth(i), `prefilter ${i} names what it filters on`).not.toBeEmpty()
    }
    const clearPrefilters = page.locator(".pd-searchresultsfeed-button-clearprefilters")
    await expect(clearPrefilters, "the prefilters offer to be cleared").toBeVisible()
    // The shortcut filters the documents instead of scoping the search to the ones referencing the document,
    // so the referencing scope is not in effect.
    await expect(page.locator(".pd-searchresultsfeed-button-clearreverse"), "a shortcut sets no referencing scope").toHaveCount(0)
    const prefiltered = await resultCount(page)
    expect(prefiltered, "the prefiltered search finds what the shortcut promised").toBe(promised)
    await settleFilters(page)
    // The block naming the prefilters is masked: the site lists the prefilters of a shortcut in a different
    // order on every visit, so the block is the one part of this page which does not look the same twice.
    // The test below is about that.
    await checkpoint(page, "prefilters-shortcut-search", { mask: [...volatile(page), page.locator(".pd-searchresultsfeed-text-prefilters")] })

    await searchAgain(page, async () => {
      await clearPrefilters.click()
    })
    await expect(prefilterLabels, "the cleared prefilters are no longer named").toHaveCount(0)
    await expect(clearPrefilters, "the cleared prefilters no longer offer to be cleared").toBeHidden()
    await expect.poll(() => resultCount(page), { message: "clearing the prefilters widens the results" }).toBeGreaterThan(prefiltered)
    const widened = await resultCount(page)
    // See the note on the cleared referencing scope above: a search over every document is captured as it is
    // seen rather than with every facet added to it.
    await settle(page)
    await checkpoint(page, "prefilters-shortcut-prefilters-cleared", { mask: volatile(page), ...VIEWPORT_ONLY })

    console.log(`Successfully followed a search shortcut into a search of ${prefiltered} documents and widened it to ${widened} by clearing its prefilters.`)
  })

  for (const language of LANGUAGES) {
    test(`Test the prefilter label of a search in ${language}`, async ({ context }) => {
      const page = await context.newPage()

      // A prefilter is named by the property it filters on and by the document it filters to, and both of
      // those are documents of the catalogue, so the whole line is written in the language the site is being
      // read in rather than in the one the search was built in. A search under a single prefilter is used,
      // because a search under several of them lists them in an order which is not the same twice, see the
      // test below.
      await goHome(page)
      await switchLanguage(page, language)

      await searchByClass(page, "PRACTICE")
      const prefilterLabel = page.locator(".pd-prefilterlabel")
      await expect(prefilterLabel, "the class search is limited by one prefilter").toHaveCount(1)
      await expect(prefilterLabel, `the prefilter names what it filters on in ${language}`).not.toBeEmpty()
      const found = await resultCount(page)
      await expectNothingLoading(page)
      await checkpointElement(page, page.locator(".pd-searchresultsfeed-text-prefilters"), `prefilters-label-${language}`)

      console.log(`Successfully read the prefilter label of a search of ${found} documents in ${language}.`)
    })
  }

  test("Test a search under several prefilters lists them in the same order every time", async ({ context }) => {
    const page = await context.newPage()

    // Which order the prefilters of a shortcut are listed in is what a reader of the line uses to tell one
    // search apart from another, so opening the same shortcut again has to give the same line. The shortcut
    // is opened several times because the order which comes back is one of the two orders at random, so a
    // single comparison would agree with itself half of the time.
    //
    // This currently fails: the session is built by iterating the map the shortcut was parsed into
    // (parseShortcutQueryGroups and the loop over its groups in parseSearchShortcutQuery, search.go), and Go
    // randomizes the iteration order of a map, so the same shortcut lists its prefilters in a different order
    // on every visit. The printed filter summary of the same search is listed from the same list.
    const attempts = 8
    const orders: Array<string> = []
    for (let attempt = 0; attempt < attempts; attempt++) {
      await page.goto(SHORTCUT_SEARCH)
      await expectResults(page)
      const labels = page.locator(".pd-prefilterlabel")
      await expect(labels, "the search is limited by the two filters the shortcut carries").toHaveCount(2)
      await expectNothingLoading(page)
      orders.push((await labels.allTextContents()).join(" | "))
    }
    expect(new Set(orders).size, `the same shortcut lists its prefilters in the same order on all ${attempts} visits`).toBe(1)

    console.log(`Successfully compared the prefilter order of ${attempts} visits to the same search shortcut.`)
  })

  test("Test a search shortcut which also offers creating", async ({ context }) => {
    const page = await context.newPage()

    // A visitor who may not create anything is offered the saved searches alone, and a shortcut which would
    // find nothing is not offered at all.
    await openDocument(page, PLANET)
    await settleDocument(page)
    const shortcuts = page.locator(".pd-documentget-link-shortcut")
    await expect(shortcuts.first(), "the world offers its saved searches to a visitor").toBeVisible()
    const offeredToVisitor = await shortcuts.count()
    await expect(page.locator(".pd-searchshortcutlink-button-create"), "a visitor is offered no way to create from a shortcut").toHaveCount(0)
    await checkpointElement(page, page.locator(".pd-documentget-list-shortcuts"), "prefilters-planet-shortcuts-visitor")

    // Signing in as the role which may start a new region adds the create half of the shortcut which lists
    // the regions of this world, and reveals the shortcuts which currently find nothing, because a search
    // which finds nothing is still where a first document of that kind is created from.
    await signIn(page, [CREATING_ROLE])
    await openDocument(page, PLANET)
    await settleDocument(page)
    await expect(shortcuts, "a caller who may create is offered every shortcut of the class").toHaveCount(5)
    expect(offeredToVisitor, "a shortcut which finds nothing is not offered to a visitor").toBeLessThan(5)

    const create = page.locator(".pd-searchshortcutlink-button-create")
    await expect(create, "one shortcut of the world offers creating").toHaveCount(1)
    // The create button leads to the create view, limited to the class the shortcut lists and carrying the
    // reference back to this world, so what is created is contained in it without the user filling that in.
    const href = await create.getAttribute("href")
    expect(href, "the create button leads to the create view").toContain("/d/create")
    expect(href, "the create button limits the create view to the class the shortcut lists").toContain(`limit=${CLASS_IDS.REGION}`)
    expect(href, "the create button carries the reference back to the world the shortcut is offered from").toContain(`${PROPERTY_IDS.CONTAINED_IN}=${PLANET}`)
    await checkpointElement(page, page.locator(".pd-documentget-list-shortcuts"), "prefilters-planet-shortcuts-creating")

    console.log(`Successfully verified the search shortcuts of a world: ${offeredToVisitor} offered to a visitor, 5 with one create button to a caller who may create.`)
  })

  test("Test the explicit document scope of a search", async ({ context }) => {
    const page = await context.newPage()

    // A search can also be scoped to an explicit set of documents, which the search shortcut route builds
    // from the identifiers of the documents to keep.
    await page.goto(`${PEERDB_URL}/s?${SCOPED.map((id) => `id=${id}`).join("&")}`)
    await expectResults(page)

    await expect(scopeNotice(page, "scoped"), "the search says it is scoped to a set of documents").toBeVisible()
    const clearIds = page.locator(".pd-searchresultsfeed-button-clearids")
    await expect(clearIds, "the document scope offers to be cleared").toBeVisible()
    await expect.poll(() => resultCount(page), { message: "the search finds only the documents it is scoped to" }).toBe(SCOPED.length)
    // A scope is not a filter, so nothing is prefiltered.
    await expect(page.locator(".pd-prefilterlabel"), "the document scope is not a prefilter").toHaveCount(0)
    await settleFilters(page)
    await checkpoint(page, "prefilters-ids-scoped-search", { mask: volatile(page) })

    await searchAgain(page, async () => {
      await clearIds.click()
    })
    await expect(clearIds, "the cleared scope no longer offers to be cleared").toBeHidden()
    await expect(scopeNotice(page, "scoped"), "the cleared scope is no longer named").toHaveCount(0)
    await expect.poll(() => resultCount(page), { message: "clearing the document scope widens the results" }).toBeGreaterThan(SCOPED.length)
    const widened = await resultCount(page)
    // See the note on the cleared referencing scope above: a search over every document is captured as it is
    // seen rather than with every facet added to it.
    await settle(page)
    await checkpoint(page, "prefilters-ids-scope-cleared", { mask: volatile(page), ...VIEWPORT_ONLY })

    console.log(`Successfully scoped a search to ${SCOPED.length} documents and widened it to ${widened} by clearing the scope.`)
  })
})
