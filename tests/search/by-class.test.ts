import type { Locator, Page } from "@playwright/test"

import type { AbstractClass, DocumentClass, EntityClass, Role } from "../peerdb_utils"

import {
  CLASS_IDS,
  CORE_CLASS_IDS,
  DOCUMENT_CLASSES,
  LANGUAGES,
  PROPERTY_IDS,
  RESTRICTED_CLASS,
  searchByClass,
  searchByCoreClass,
  VOCABULARY_CLASSES,
} from "../peerdb_utils"
import {
  applyFilterValue,
  checkpoint,
  checkpointElement,
  expect,
  filter,
  filterValue,
  goHome,
  LOADING_TIMEOUT,
  openFilters,
  resultCount,
  searchWithQuery,
  settle,
  settleFilters,
  signIn,
  switchLanguage,
  test,
  volatile,
} from "../utils"

// The classes which hold no documents of their own but gather the classes below them, and what sits directly
// under each of them (classes.go in internal/xeno). peerdb_utils lists the mnemonics the schema declares but
// not how they nest, so the tree is written out here, typed against the schema so that a class which is
// renamed or dropped is a compile error rather than a test which quietly stops asserting anything.
const ABSTRACT_CLASSES: ReadonlyArray<AbstractClass> = ["PLACE", "WORLD", "BEING", "CULTURAL_ELEMENT", "RESEARCH_RECORD"]

const CHILD_CLASSES: Record<AbstractClass, ReadonlyArray<EntityClass>> = {
  PLACE: ["WORLD", "REGION", "SITE", "STAR_SYSTEM", "SECTOR", "GALAXY"],
  WORLD: ["PLANET", "MOON"],
  BEING: ["INDIVIDUAL", "COLLECTIVE"],
  CULTURAL_ELEMENT: ["ARTIFACT", "PRACTICE", "NARRATIVE"],
  RESEARCH_RECORD: ["OBSERVATION", "PUBLICATION", "INTERVIEW"],
}

// The classes of the schema itself which every site holds, whatever data it carries. The three covered here
// are the ones the test data fills: the classes it declares, the properties they are described by, and the
// units its amounts are measured in.
const CORE_CLASSES: ReadonlyArray<keyof typeof CORE_CLASS_IDS> = ["CLASS", "PROPERTY", "UNIT"]

// The class a search is narrowed to in every language the site is served in. It is one of the classes which
// hold the fewest documents, so the three result pages the language loop screenshots are the cheapest ones
// which still carry a class facet, a prefilter label and results of their own.
const LOCALIZED_CLASS: DocumentClass = "INSTITUTE"

// The role which is granted reading the class the site keeps out of the public read scope (roles in
// config.yml). A search for that class as a visitor who is not signed in finds nothing at all, because the
// documents are not readable and are therefore not in the visitor's index either, so the one test over it
// signs in first while every other class is covered as an anonymous visitor.
const RESTRICTED_CLASS_ROLE: Role = "ethics"

// The classes directly under the given one, or nothing when the class holds documents itself. The table is
// declared over the classes which have children so that every one of them has to be listed, and the lookup
// widens it to any class, which is what walking down the tree needs.
function childClassesOf(entityClass: EntityClass): ReadonlyArray<EntityClass> | undefined {
  return (CHILD_CLASSES as Partial<Record<EntityClass, ReadonlyArray<EntityClass>>>)[entityClass]
}

// The classes which hold documents anywhere below the given class, which is what the results of a search
// narrowed to a class gathering others are instances of. A document states the class it is an instance of and
// not the classes above that one, so a result is recognized by the leaf and never by the class filtered on.
function documentClassesBelow(entityClass: EntityClass): Array<EntityClass> {
  const children = childClassesOf(entityClass)
  if (children === undefined) {
    return [entityClass]
  }
  return children.flatMap(documentClassesBelow)
}

// The part of a screenshot name which identifies the class.
function slug(mnemonic: string): string {
  return mnemonic.toLowerCase().replaceAll("_", "-")
}

// The class facet: the reference facet on the property every document states its class with. It is the only
// facet whose values are classes, and a facet is addressed by the property path it filters on, so a nested
// facet of the same property under another one does not match.
function classFacet(page: Page): Locator {
  return filter(page, "ref", PROPERTY_IDS.INSTANCE_OF)
}

// The checkbox selecting one class in the class facet, addressed by the identifier of the class document.
function classCheckbox(page: Page, classId: string): Locator {
  return filterValue(page, "ref", [PROPERTY_IDS.INSTANCE_OF], classId)
}

// The row of the class facet for one class. A row holds its own controls in a block of its own and the rows
// of the classes below it in a list next to that block, so a row is picked out by the checkbox of its own
// block and never by the checkbox of a class nested under it.
function classRow(page: Page, classId: string): Locator {
  return page.locator(`.pd-reffiltertreerow:has(> .pd-reffiltertreerow-row input[id="ref/${PROPERTY_IDS.INSTANCE_OF}/${classId}"])`)
}

// The checkbox of one class in a row of the class facet which sits directly under the row of another class,
// which is what says the facet nests the two. The whole path down to the checkbox is spelled out rather than
// matching the row and asking whether it holds the checkbox, because a row holds the rows of the classes
// below it as well and only the path says at which level the checkbox was found.
function nestedClassCheckbox(page: Page, parentId: string, classId: string): Locator {
  return classRow(page, parentId).locator(
    `> .pd-reffiltertreerow-list > .pd-reffiltertreerow > .pd-reffiltertreerow-row input[id="ref/${PROPERTY_IDS.INSTANCE_OF}/${classId}"]`,
  )
}

// How many documents the class facet reports for one class, read out of the count its row renders. The count
// is wrapped in parentheses and grouped for the locale, so only its digits are read.
async function classFacetCount(page: Page, classId: string, what: string): Promise<number> {
  const count = classRow(page, classId).locator("> .pd-reffiltertreerow-row > .pd-reffiltertreerow-count")
  await expect(count, `the class facet reports a count for ${what}`).toBeVisible()
  const digits = ((await count.textContent()) || "").replace(/\D/g, "")
  expect(digits, `the count the class facet reports for ${what}`).not.toBe("")
  return Number(digits)
}

// The results which are an instance of one of the given classes, recognized by the badge a result carries for
// each class it states.
function resultsOfClasses(page: Page, classIds: ReadonlyArray<string>): Locator {
  return page.locator(classIds.map((classId) => `.pd-searchresult:has(.pd-searchresult-badge-type[data-url="/api/d/${classId}"])`).join(", "))
}

// Asserts that every result the feed shows is an instance of one of the given classes and returns how many
// results that was. The feed renders only its first page of results, so this covers what is shown rather than
// the whole result set, whose size the results header reports separately.
async function expectResultsOfClasses(page: Page, classIds: ReadonlyArray<string>, what: string): Promise<number> {
  // The feed reveals a further page of results on its own whenever the end of the page comes near, so what is
  // shown is counted only once the page has stopped fetching.
  await settle(page)

  // Narrowing a search to a class has to leave results, otherwise the class is missing from the data or the
  // narrowing does not match what the class facet lists.
  const found = await page.locator(".pd-searchresult").count()
  expect(found, `results for ${what}`).toBeGreaterThan(0)

  await expect(resultsOfClasses(page, classIds), `every result for ${what} carries a class badge of ${what}`).toHaveCount(found)

  return found
}

// Takes a checkpoint of the settled result page. The feed is waited out by settleSearch and the filters panel
// by settleFilters, so what is captured is the page as it comes to rest rather than as far as it happened to
// get.
async function checkpointSearch(page: Page, name: string): Promise<void> {
  await settleFilters(page)
  await expect(page.locator(".pd-displaylabel-error"), "display labels rendered as an error").toHaveCount(0)
  await checkpoint(page, name, { mask: volatile(page) })
}

// Runs the query-less search from the home page, which is the result page a visitor narrows down by ticking a
// class in the filters panel, and opens that panel.
async function searchEverything(page: Page): Promise<void> {
  await searchWithQuery(page, "")
  await openFilters(page)
  await expect(classFacet(page), "the class facet of a search over everything").toBeVisible()
}

// Expands the class facet until it lists the given class. The facet shows ten rows at a time behind a load
// more button and orders the classes by how many documents they hold, so a class with few documents is listed
// only after several expansions.
async function expandToClass(page: Page, classId: string, what: string): Promise<void> {
  const facet = classFacet(page)
  const rows = facet.locator(".pd-reffiltertreerow")
  const checkbox = classCheckbox(page, classId)

  while ((await checkbox.count()) === 0) {
    const more = facet.locator(".pd-filtersresult-more")
    await expect(more, `the class facet has further classes to load before it lists ${what}`).toBeVisible()
    const loaded = await rows.count()
    await more.click()
    await expect.poll(() => rows.count(), { message: `the class facet loads further classes before it lists ${what}` }).toBeGreaterThan(loaded)
  }
  await expect(checkbox, `the class facet lists ${what}`).toBeVisible()
}

// Whether an address is the one the facets of a search are listed from. The values of a single facet are
// fetched from addresses which start the same way and carry the kind and the property path after the session,
// so only an address which ends with the session counts.
function isFilterList(url: string): boolean {
  return /^\/api\/s\/filters\/[0-9A-Za-z]+$/.test(new URL(url).pathname)
}

// Ticks one class in the class facet and waits until the search has been narrowed to it. The selection updates
// the search session in place rather than navigating, so it is the narrowed search which has to be waited for
// and not a page load.
//
// Both the results and the facets of the narrowed search are waited for. The results alone say nothing about
// the filters panel: it goes on showing the facets of the previous search until the new ones arrive, and it
// folds itself back to its first batch when they do, so a screenshot taken in between is of a panel which is
// still about to change.
async function selectClass(page: Page, classId: string, what: string): Promise<void> {
  const facets = page.waitForResponse((response) => isFilterList(response.url()), { timeout: LOADING_TIMEOUT })
  await applyFilterValue(page, classFacet(page), classCheckbox(page, classId))
  await facets

  await expect(classCheckbox(page, classId), `the class facet keeps ${what} selected`).toBeChecked()
}

test.describe("PeerDB Search by Class Flows", () => {
  // A search can be narrowed to a class in two ways, which reach the result page through different parts of
  // the search session and are therefore both covered for every class which holds documents: opening the
  // search shortcut route for the class, which scopes the session with a prefilter, and ticking the class in
  // the class facet of a search over everything, which adds a filter to the session instead. The two have to
  // find the same documents, so what each of them found is compared against the other.
  for (const documentClass of DOCUMENT_CLASSES) {
    test(`Test narrowing a search to ${documentClass} documents both ways`, async ({ context }) => {
      // Two result pages are driven, each of them screenshotted and read back, which is more than the default
      // budget covers while other tests run next to this one.
      test.slow()

      const page = await context.newPage()

      if (documentClass === RESTRICTED_CLASS) {
        await signIn(page, [RESTRICTED_CLASS_ROLE])
      }

      await searchByClass(page, documentClass)
      // The filters panel is opened so that this page is screenshotted in the same state as the one the class
      // facet reaches, which is what makes the two comparable by eye.
      await openFilters(page)
      await checkpointSearch(page, `by-class-prefilter-${slug(documentClass)}`)

      const throughPrefilter = await resultCount(page)
      const shown = await expectResultsOfClasses(page, [CLASS_IDS[documentClass]], documentClass)

      // A prefilter scopes the search session itself rather than filtering inside it, so instead of a ticked
      // class facet the results are headed by the prefilter's own label and by the control which clears it.
      // That is what tells the two ways of narrowing apart on the page.
      await expect(page.locator(".pd-prefilterlabel"), `the prefilter label for ${documentClass}`).toBeVisible()
      await expect(page.locator(".pd-prefilterlabel"), `the prefilter label for ${documentClass} names it`).not.toBeEmpty()
      await expect(page.locator(".pd-searchresultsfeed-button-clearprefilters"), `the control clearing the prefilter for ${documentClass}`).toBeVisible()

      await searchEverything(page)
      await expandToClass(page, CLASS_IDS[documentClass], documentClass)
      await selectClass(page, CLASS_IDS[documentClass], documentClass)
      await checkpointSearch(page, `by-class-filter-${slug(documentClass)}`)

      expect(await resultCount(page), `both ways of narrowing to ${documentClass} find the same number of documents`).toBe(throughPrefilter)
      await expectResultsOfClasses(page, [CLASS_IDS[documentClass]], documentClass)

      console.log(`Successfully narrowed a search to ${documentClass} both ways, with ${throughPrefilter} documents found by each and ${shown} of them shown.`)
    })
  }

  // What a result page narrowed to a class shows is written in the interface language: the class the results
  // are limited to is named in it, the facets are labelled in it, and the results header counts in it. What
  // the search finds does not depend on it, so the same narrowing is run in each language the site is served
  // in and both what it found and how it is labelled are compared across the three.
  test(`Test narrowing a search to ${LOCALIZED_CLASS} documents in every language`, async ({ context }) => {
    test.slow()

    const page = await context.newPage()

    let inFirstLanguage: number | null = null
    const labels: Array<string> = []
    for (const language of LANGUAGES) {
      // The language is switched on the home page rather than on the result page, because a search session
      // records the language it was created in and labels what it finds in that language.
      await goHome(page)
      await switchLanguage(page, language)

      await searchByClass(page, LOCALIZED_CLASS)
      await openFilters(page)
      await checkpointSearch(page, `by-class-language-${language}-${slug(LOCALIZED_CLASS)}`)

      const total = await resultCount(page)
      await expectResultsOfClasses(page, [CLASS_IDS[LOCALIZED_CLASS]], LOCALIZED_CLASS)
      if (inFirstLanguage === null) {
        inFirstLanguage = total
      } else {
        expect(total, `narrowing to ${LOCALIZED_CLASS} in ${language} finds what narrowing to it in ${LANGUAGES[0]} finds`).toBe(inFirstLanguage)
      }

      const label = page.locator(".pd-prefilterlabel")
      await expect(label, `the prefilter label for ${LOCALIZED_CLASS} in ${language}`).not.toBeEmpty()
      labels.push(((await label.textContent()) || "").trim())
    }

    // The class the results are limited to is named in each of the three languages, and the test data gives
    // it a different name in each, so a label repeated across two of them would mean the language reached the
    // interface but not the documents the interface renders.
    expect(new Set(labels).size, `the prefilter label naming ${LOCALIZED_CLASS} is written in each language`).toBe(LANGUAGES.length)

    console.log(`Successfully narrowed a search to ${LOCALIZED_CLASS} in ${LANGUAGES.length} languages, with ${inFirstLanguage} documents found in each.`)
  })

  // The entries of a controlled vocabulary are documents like any other, so each vocabulary is a class a
  // search can be narrowed to. They all hold the same shape of document, so one loop over them carrying the
  // assertions the loop above already makes for the shortcut route covers them all. What a vocabulary adds
  // over the classes above it is how few documents it holds, which makes these the searches whose every
  // result the feed shows at once.
  for (const vocabularyClass of VOCABULARY_CLASSES) {
    test(`Test searching for the entries of the ${vocabularyClass} vocabulary`, async ({ context }) => {
      const page = await context.newPage()

      await searchByClass(page, vocabularyClass)
      await openFilters(page)
      await checkpointSearch(page, `by-class-vocabulary-${slug(vocabularyClass)}`)

      const total = await resultCount(page)
      const shown = await expectResultsOfClasses(page, [CLASS_IDS[vocabularyClass]], vocabularyClass)
      expect(total, `entries of the ${vocabularyClass} vocabulary`).toBeGreaterThan(0)

      console.log(`Successfully searched for the entries of the ${vocabularyClass} vocabulary, with ${total} found and ${shown} of them shown.`)
    })
  }

  // The documents of the schema itself are reached the same way as the documents of the data: they are
  // instances of a core class, so the search shortcut route narrows to them just as it narrows to a class the
  // site declares.
  for (const coreClass of CORE_CLASSES) {
    test(`Test searching for the ${coreClass} documents of the schema`, async ({ context }) => {
      test.slow()

      const page = await context.newPage()

      await searchByCoreClass(page, coreClass)
      await openFilters(page)
      await checkpointSearch(page, `by-class-core-${slug(coreClass)}`)

      const total = await resultCount(page)
      const shown = await expectResultsOfClasses(page, [CORE_CLASS_IDS[coreClass]], coreClass)
      expect(total, `documents of the ${coreClass} core class`).toBeGreaterThan(0)

      console.log(`Successfully searched for the ${coreClass} documents of the schema, with ${total} found and ${shown} of them shown.`)
    })
  }

  // A class which gathers other classes holds no documents of its own, so it is in the class facet only
  // because the site indexes the classes above the one a document states (indexAncestorProperties in
  // config.yml). What that buys the visitor is asserted here: the facet nests the classes below such a class
  // under it, the counts it reports add up, and narrowing the search to it finds the documents of every class
  // below it.
  for (const abstractClass of ABSTRACT_CLASSES) {
    test(`Test the class facet nests the classes below ${abstractClass}`, async ({ context }) => {
      test.slow()

      const page = await context.newPage()

      const children = CHILD_CLASSES[abstractClass]
      const below = documentClassesBelow(abstractClass)
      if (below.includes(RESTRICTED_CLASS)) {
        await signIn(page, [RESTRICTED_CLASS_ROLE])
      }

      await searchEverything(page)
      await expandToClass(page, CLASS_IDS[abstractClass], abstractClass)
      for (const child of children) {
        await expandToClass(page, CLASS_IDS[child], child)
      }

      // Every class directly below has to be a row inside the row of the class above it, and the counts of
      // those rows have to add up to the count of the row above: no document is an instance of a class which
      // gathers others, so everything the facet reports for it comes from the classes below.
      let summed = 0
      for (const child of children) {
        await expect(nestedClassCheckbox(page, CLASS_IDS[abstractClass], CLASS_IDS[child]), `the class facet nests ${child} under ${abstractClass}`).toHaveCount(1)
        summed += await classFacetCount(page, CLASS_IDS[child], child)
      }
      const gathered = await classFacetCount(page, CLASS_IDS[abstractClass], abstractClass)
      expect(summed, `the counts of the classes directly below ${abstractClass} add up to its own`).toBe(gathered)

      // The facet is screenshotted on its own, and not as the top of the page the way a result page is: what
      // this test is about is the whole tree the facet draws, which reaches further down the page than the
      // slice a result page is compared by.
      await checkpointElement(page, classFacet(page), `by-class-nested-${slug(abstractClass)}-facet`)

      await selectClass(page, CLASS_IDS[abstractClass], abstractClass)
      await checkpointSearch(page, `by-class-nested-${slug(abstractClass)}-results`)

      expect(await resultCount(page), `narrowing to ${abstractClass} finds every document of the classes below it`).toBe(gathered)
      const shown = await expectResultsOfClasses(
        page,
        below.map((entityClass) => CLASS_IDS[entityClass]),
        `the classes below ${abstractClass}`,
      )

      // Selecting a class covers the classes below it, which the facet says by ticking their checkboxes as
      // well, so a visitor can see what the narrowed search includes.
      for (const child of children) {
        await expect(classCheckbox(page, CLASS_IDS[child]), `the class facet ticks ${child} while ${abstractClass} above it is selected`).toBeChecked()
      }

      console.log(
        `Successfully verified that the class facet nests ${children.length} classes under ${abstractClass}, whose ${gathered} documents narrowing to it finds, with ${shown} of them shown.`,
      )
    })
  }
})
