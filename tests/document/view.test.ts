import type { Locator, Page } from "@playwright/test"

import { coreDocumentIdOf, documentIdOf, LANGUAGES, PROPERTY_IDS, readTestData } from "../peerdb_utils"
import {
  checkpoint,
  checkpointElement,
  expect,
  expectResults,
  fetchFromPage,
  fieldsPanel,
  goHome,
  openDocument,
  resultCount,
  settle,
  settleFilters,
  switchLanguage,
  test,
  volatile,
} from "../utils"

// The documents this file looks at, addressed by their document identifier so that the same document is
// opened on every run regardless of how a search happens to rank things. They are the documents of the test
// data which carry the most fields for their class, so their view exercises every kind of field row: strings,
// identifiers, rich text, amounts with units, intervals, times, references, links to attachments and the
// sub-fields hanging off them. The first two belong to classes which group their fields into sections and
// the third to a class which does not, so both layouts of the fields panel are covered.
const RICH_DOCUMENTS = [
  { what: "planet", id: await documentIdOf("PLANET", "G1_SURVEY_GRID_44_B"), sections: ["identification", "physical", "environment", "survey"] },
  { what: "species", id: await documentIdOf("SPECIES", "G1_IELUARO"), sections: ["identification", "biology", "society", "contact"] },
  { what: "culture", id: await documentIdOf("CULTURE", "G4_CU_LADDER_GORGE"), sections: [] },
] as const

// The document the language test is run on. It is the one document of the test data whose name is written in
// two languages, whose description is written in all three, and which also carries claims with no language of
// their own, so a single view shows what each of the three cases does when the interface language changes.
const INSTITUTE_FILE = "institute/INST_ANCHOR.json"
const INSTITUTE_ID = await documentIdOf("INSTITUTE", "INST_ANCHOR")

// The document the identifier link is checked on. Its researcher code is the identifier value of the test
// data which carries a link template and no character a URL would have to escape.
const RESEARCHER_FILE = "researcher/RES_HALVORSEN.json"
const RESEARCHER_ID = await documentIdOf("RESEARCHER", "RES_HALVORSEN")

// The property a class states the template with, which is a property of the core schema every site is built
// on rather than one the test data declares.
const LINK_TEMPLATE_ID = await coreDocumentIdOf("IDENTIFIER_LINK_TEMPLATE")

// One value of a field of a test data document, as its JSON file records it.
interface TestDataValue {
  value: string
  inLanguage?: Array<{ id: Array<string> }>
}

// The text a rich text value renders as. The test data writes descriptions and notes as HTML, while the view
// renders them as text, so the markup is dropped before the two are compared.
function textOf(value: string): string {
  return value.replace(/<[^>]*>/g, "").trim()
}

// The values one field of a test data document carries, keyed by the language they are written in. A value
// with no language of its own is filed under "und", which is the bucket every fallback chain of the site ends
// at (languagePriority in config.yml), so such a value is shown whatever the interface language is.
function valuesByLanguage(document: Record<string, unknown>, field: string): Record<string, string> {
  const byLanguage: Record<string, string> = {}
  for (const value of document[field] as Array<TestDataValue>) {
    // A language is recorded as a reference to the document of that language, whose base is the core
    // namespace, the class and the tag, so the tag is the last part of it. The tags of the test data name a
    // region as well ("en-GB"), while the interface languages do not, so only the language part is kept.
    const tag = value.inLanguage?.[0]?.id?.[2] ?? "und"
    byLanguage[tag.split("-")[0]] = textOf(value.value)
  }
  return byLanguage
}

// What the view shows for such a field in one interface language: the value written in that language when
// there is one, and the English one otherwise, which is where the fallback chains of the two other languages
// go next, and finally the value which carries no language at all.
function inLanguage(values: Record<string, string>, language: string): string {
  return values[language] ?? values.en ?? values.und
}

// The rows of the fields panel which hold one property. A property stated more than once renders one row per
// claim, with the label written into the first of them only, so this matches as many rows as there are values.
function fieldRows(page: Page, propertyId: string): Locator {
  return fieldsPanel(page).locator(`.pd-fieldsview-row-${propertyId}`)
}

// The value cell of the first row of one property.
function fieldValue(page: Page, propertyId: string): Locator {
  return fieldRows(page, propertyId).first().locator(".pd-fieldsview-value")
}

// Asserts that the document view shows the fields of its class: a title, at least one field row, and a
// property label and a value on every row. The label cell is rendered once per field and the value cell once
// per claim, so there are never fewer values than labels. Returns how many field rows were shown.
async function expectFields(page: Page, what: string): Promise<number> {
  const title = page.locator("#documentget-title")
  await expect(title, `title of ${what}`).toBeVisible()
  await expect(title, `title of ${what} is not empty`).not.toHaveText(/^\s*$/)

  const panel = fieldsPanel(page)
  await expect(panel, `properties panel of ${what}`).toBeVisible()
  await expect(panel.locator(".pd-fieldsview").first(), `fields of ${what}`).toBeVisible()

  const rows = panel.locator(".pd-fieldsview-row")
  const labels = panel.locator(".pd-fieldsview-label")
  const values = panel.locator(".pd-fieldsview-value")

  const rowCount = await rows.count()
  expect(rowCount, `field rows of ${what}`).toBeGreaterThan(0)

  const labelTexts = await labels.allTextContents()
  expect(labelTexts.length, `property labels of ${what}`).toBeGreaterThan(0)
  for (const [i, text] of labelTexts.entries()) {
    expect(text.trim(), `property label ${i} of ${what}`).not.toBe("")
  }

  // The label is written into the first row of a field only, so a field stated several times is one label and
  // as many rows as it has values.
  expect(rowCount, `field rows of ${what} against the properties they are for`).toBeGreaterThanOrEqual(labelTexts.length)

  const valueTexts = await values.allTextContents()
  expect(valueTexts.length, `property values of ${what}`).toBeGreaterThan(0)
  for (const [i, text] of valueTexts.entries()) {
    expect(text.trim(), `property value ${i} of ${what}`).not.toBe("")
  }

  // A field whose claim only states that the property holds (a HAS claim without sub-fields, which is how a
  // yes-or-no field is stored) has nothing to put in the value column, so its row is a label next to an empty
  // cell. Every other row carries a value, and no row is empty on both sides.
  await expect(
    panel.locator(".pd-fieldsview-row:not(:has(.pd-fieldsview-label)):not(:has(.pd-fieldsview-value))"),
    `field rows of ${what} which show neither a property nor a value`,
  ).toHaveCount(0)

  return rowCount
}

// Asserts that the fields panel is split into exactly the sections the document's class declares, each of
// them headed by a name written in the interface language.
async function expectSections(page: Page, sections: ReadonlyArray<string>, what: string): Promise<void> {
  const headers = fieldsPanel(page).locator(".pd-fieldsview-header-section")
  await expect(headers, `section headers of ${what}`).toHaveCount(sections.length)
  for (const section of sections) {
    const header = fieldsPanel(page).locator(`.pd-fieldsview-header-section-${section}`)
    await expect(header, `the ${section} section of ${what}`).toBeVisible()
    await expect(header, `the ${section} section of ${what} is named`).not.toHaveText(/^\s*$/)
  }
}

// The count a link on the sidebar carries, which is how many documents the search behind it finds. Every such
// link is labelled by its name followed by the count in parentheses, and the count is grouped for the locale,
// so only its digits are read.
function countInLabel(label: string, what: string): number {
  const match = /\(([^)]*)\)\s*$/.exec(label.trim())
  expect(match, `the label of ${what} carries a count: ${label}`).not.toBeNull()
  const digits = match![1].replace(/\D/g, "")
  expect(digits, `the count in the label of ${what}`).not.toBe("")
  return Number(digits)
}

// Follows one of the links of the sidebar and waits for the search it opens. Every one of them is a search
// shortcut, which posts its query and redirects to the session it creates, so what is waited for is the result
// page at the end of that redirect and not the address the link points at.
async function followShortcut(page: Page, link: Locator, what: string): Promise<void> {
  const anchor = link.locator(".pd-searchshortcutlink-link")
  await expect(anchor, `the link of ${what}`).toBeVisible()
  await anchor.click()
  await expectResults(page)
}

test.describe("PeerDB Document View Flows", () => {
  for (const language of LANGUAGES) {
    for (const document of RICH_DOCUMENTS) {
      test(`Test viewing a fully populated ${document.what} in ${language}`, async ({ context }) => {
        const page = await context.newPage()

        // The language is switched on the home page and the document opened afterwards, because the view
        // renders the claims which match the interface language and is not re-read when the language changes
        // under it.
        await goHome(page)
        await switchLanguage(page, language)

        await openDocument(page, document.id)
        await settle(page)
        await checkpoint(page, `document-view-${document.what}-${language}`, { mask: volatile(page) })

        const rowCount = await expectFields(page, `the ${document.what} in ${language}`)
        await expectSections(page, document.sections, `the ${document.what} in ${language}`)

        await checkpointElement(page, fieldsPanel(page).locator(".pd-fieldsview").first(), `document-view-${document.what}-fields-${language}`)

        // The sidebar holds the links to the searches related to the document (the shortcuts its class
        // declares and the documents which point at it), and it is rendered only when there is something to
        // put in it, so a document of a class which declares shortcuts and which other documents reference
        // has one.
        const sidebar = page.locator("#documentget-sidebar")
        await expect(sidebar, `sidebar of the ${document.what}`).toBeVisible()
        const shortcuts = page.locator(".pd-documentget-link-shortcut")
        expect(await shortcuts.count(), `search shortcuts of the ${document.what}`).toBeGreaterThan(0)
        for (const label of await shortcuts.allTextContents()) {
          expect(label.trim(), `a search shortcut of the ${document.what} is labelled`).not.toBe("")
        }
        await expect(page.locator("#documentget-button-referencedby"), `referenced by link of the ${document.what}`).toBeVisible()
        await checkpointElement(page, sidebar, `document-view-${document.what}-sidebar-${language}`)

        console.log(
          `Successfully viewed a fully populated ${document.what} in ${language}, with ${rowCount} field rows in ${document.sections.length} sections and ${await shortcuts.count()} search shortcuts shown.`,
        )
      })
    }
  }

  // A claim which is written in a language is shown only while the interface is in that language, or in a
  // language whose fallback chain reaches it, while a claim with no language of its own is shown whatever the
  // interface language is. The document this runs on has all three cases at once: a name written in English
  // and Slovenian, a description written in all three languages, and a website and a staff count which carry
  // no language at all.
  test("Test the claims of a document follow the language of the interface", async ({ context }) => {
    const page = await context.newPage()

    const institute = readTestData(INSTITUTE_FILE)
    const names = valuesByLanguage(institute, "name")
    const descriptions = valuesByLanguage(institute, "description")
    const notes = valuesByLanguage(institute, "notes")
    const website = institute.website as Array<string>
    const founded = (institute.founded as { time: string }).time.split("-")[0]

    const shownNames: Array<string> = []
    const propertyLabels: Array<string> = []
    for (const language of LANGUAGES) {
      await goHome(page)
      await switchLanguage(page, language)

      await openDocument(page, INSTITUTE_ID)
      await settle(page)
      await checkpoint(page, `document-view-languages-${language}`, { mask: volatile(page) })

      // The title is the naming claim of the document, so it changes with the interface language just as the
      // name row below it does.
      await expect(page.locator("#documentget-title"), `the title in ${language}`).toHaveText(inLanguage(names, language))
      await expect(fieldValue(page, PROPERTY_IDS.NAME), `the name in ${language}`).toHaveText(inLanguage(names, language))
      await expect(fieldRows(page, PROPERTY_IDS.NAME), `the name is shown once in ${language}`).toHaveCount(1)
      await expect(fieldValue(page, PROPERTY_IDS.DESCRIPTION), `the description in ${language}`).toHaveText(inLanguage(descriptions, language))
      await expect(fieldRows(page, PROPERTY_IDS.DESCRIPTION), `the description is shown once in ${language}`).toHaveCount(1)

      // The notes are written in English only, so in the two other languages they are what the fallback chain
      // reaches rather than a translation, and they are shown all the same.
      await expect(fieldValue(page, PROPERTY_IDS.NOTES), `the notes in ${language}`).toHaveText(notes.en)

      // A claim which carries no language at all is not language-tagged data but a fact, so it is shown
      // unchanged in every language.
      await expect(fieldValue(page, PROPERTY_IDS.WEBSITE), `the website in ${language}`).toHaveText(website[0])
      await expect(fieldValue(page, PROPERTY_IDS.STAFF_COUNT), `the staff count in ${language}`).not.toHaveText(/^\s*$/)

      // A time carries no language either. The site does not turn on localized time display, so a date is
      // rendered part by part as precisely as it is known, which for a year known to the year is the year
      // alone, and it reads the same in every language.
      await expect(fieldValue(page, PROPERTY_IDS.FOUNDED), `the year the institute was founded in ${language}`).toHaveText(founded)

      shownNames.push(((await fieldValue(page, PROPERTY_IDS.NAME).textContent()) || "").trim())
      propertyLabels.push(((await fieldRows(page, PROPERTY_IDS.NAME).first().locator(".pd-fieldsview-label").textContent()) || "").trim())
    }

    // The property the row is for is named in the interface language as well, and the schema names it
    // differently in each of the three, so a label repeated across two of them would mean the language
    // reached the claims but not the schema which labels them.
    expect(new Set(propertyLabels).size, "the property label of the name row is written in each language").toBe(LANGUAGES.length)
    // Only two names are written, so the language which has none of its own shows the English one and the
    // three views show two distinct names between them.
    expect(new Set(shownNames).size, "the name shown follows the language when there is one for it").toBe(Object.keys(names).length)

    console.log(
      `Successfully verified that a document follows the interface language, with ${Object.keys(names).length} names and ${Object.keys(descriptions).length} descriptions shown across ${LANGUAGES.length} languages.`,
    )
  })

  // The sidebar link a document gets for the documents which point at it is labelled with how many there are,
  // and opens the search which finds them, so the number the visitor is shown before following the link has to
  // be the number of results they land on.
  test("Test the referenced by link opens the documents which point at the document", async ({ context }) => {
    const page = await context.newPage()

    await openDocument(page, RICH_DOCUMENTS[0].id)
    await settle(page)

    const referencedBy = page.locator("#documentget-button-referencedby")
    await expect(referencedBy, "the referenced by link").toBeVisible()
    const promised = countInLabel((await referencedBy.textContent()) || "", "the referenced by link")
    expect(promised, "the documents the referenced by link promises").toBeGreaterThan(0)

    await followShortcut(page, referencedBy, "the referenced by link")
    await settleFilters(page)
    await checkpoint(page, "document-view-referencedby-results", { mask: volatile(page) })

    expect(await resultCount(page), "the referenced by link finds as many documents as it promises").toBe(promised)

    console.log(`Successfully opened the ${promised} documents which point at a planet through its referenced by link.`)
  })

  // A class declares the searches which are worth running about one of its documents, and the sidebar offers
  // one link per search, labelled with how many documents it finds. A shortcut which finds nothing is left out
  // for a visitor who cannot create anything, so every link the sidebar does show has to lead to results.
  test("Test the search shortcuts of a class open the searches related to the document", async ({ context }) => {
    // Every shortcut of the document is followed, which is a result page each, and the document view is
    // returned to in between.
    test.slow()

    const page = await context.newPage()

    await openDocument(page, RICH_DOCUMENTS[0].id)
    await settle(page)

    const labels = await page.locator(".pd-documentget-link-shortcut").allTextContents()
    expect(labels.length, "the search shortcuts of the planet").toBeGreaterThan(0)

    for (const [i, label] of labels.entries()) {
      const promised = countInLabel(label, `search shortcut ${i}`)
      expect(promised, `the documents search shortcut ${i} promises`).toBeGreaterThan(0)

      await followShortcut(page, page.locator(".pd-documentget-link-shortcut").nth(i), `search shortcut ${i}`)
      if (i === 0) {
        await settleFilters(page)
        await checkpoint(page, "document-view-shortcut-results", { mask: volatile(page) })
      }
      expect(await resultCount(page), `search shortcut ${i} finds as many documents as it promises`).toBe(promised)

      await openDocument(page, RICH_DOCUMENTS[0].id)
      await settle(page)
    }

    console.log(`Successfully followed ${labels.length} search shortcuts of a planet, each of which found the number of documents its label promised.`)
  })

  // An identifier value is shown as a link whenever the property it is stated on declares a template to build
  // one from, so a catalogue code is not a dead string but a way out to the register which issued it. The
  // template is an RFC 6570 level 1 template with a single parameter, which the view fills in with the value.
  test("Test an identifier value links through the template its property declares", async ({ context }) => {
    const page = await context.newPage()

    const researcher = readTestData(RESEARCHER_FILE)
    const code = researcher.researcherCode as string

    await openDocument(page, RESEARCHER_ID)
    await settle(page)

    const identifier = fieldValue(page, PROPERTY_IDS.RESEARCHER_CODE).locator(".pd-claimvalueid")
    await expect(identifier, "the researcher code").toBeVisible()
    await expect(identifier, "the researcher code shows its value").toHaveText(code)
    // The value cell is what is captured and not the row it sits in: a row of the fields table is laid out as
    // display: contents so that the table reflows on narrow viewports, which leaves the row itself without a
    // box for a screenshot to be taken of.
    await checkpointElement(page, fieldValue(page, PROPERTY_IDS.RESEARCHER_CODE), "document-view-identifier-link")

    // The template is read from the property document itself, so the test asserts what the schema says rather
    // than a copy of it, and it is read through the page so that the request carries the same session as the
    // view next to it.
    const response = await fetchFromPage(page, `/api/d/${PROPERTY_IDS.RESEARCHER_CODE}`)
    expect(response.status, "the property document of the researcher code").toBe(200)
    const property = JSON.parse(response.body) as { claims: { string?: Array<{ prop: { id: string }; string: string }> } }
    const template = (property.claims.string ?? []).find((claim) => claim.prop.id === LINK_TEMPLATE_ID)?.string
    expect(template, "the link template the researcher code property declares").not.toBeUndefined()

    // Which parameter the template is written with is up to the schema, so what is asserted is only that the
    // view fills it in: a link which still carries the parameter leads nowhere.
    await expect(identifier, "the researcher code links through the template of its property").toHaveAttribute("href", template!.replace(/\{[^}]*\}/, code))

    console.log(`Successfully verified that the researcher code ${code} links through the template ${template} its property declares.`)
  })
})
