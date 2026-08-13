import type { Locator, Page } from "@playwright/test"

import type { DocumentClass, Role, VocabularyClass } from "../peerdb_utils"

import { CLASS_IDS, DOCUMENT_CLASSES, documentIdOf, PROPERTY_IDS, RESTRICTED_CLASS, VOCABULARY_CLASSES } from "../peerdb_utils"
import { checkpoint, expect, openDocument, openDocumentTab, settle, signIn, test, volatile } from "../utils"

// The document of each class which fills in the most of the fields the class declares (the richest document
// per class of the test data). Opening the richest one is what makes a per-class view test say something: a
// document which happens to state three of its class's twenty fields would render three rows whatever the
// class declares, while these render as much of the class as the data has.
//
// The table is declared over every class which holds documents, so a class added to the schema without a
// document named for it here is a compile error rather than a class which quietly stops being viewed.
const RICHEST: Record<DocumentClass, string> = {
  GALAXY: "G4_SPILLWORK",
  SECTOR: "G2_COLD_MARGIN",
  STAR_SYSTEM: "G1_KEPHRA",
  PLANET: "G1_SURVEY_GRID_44_B",
  MOON: "G1_UNDERCOUNT",
  REGION: "G1_WARM_POOL",
  SITE: "G4_SUNK_RELAY",
  SPECIES: "G1_IELUARO",
  INDIVIDUAL: "G2_USSANESH",
  COLLECTIVE: "G2_FLOOR_SELF",
  CULTURE: "G4_CU_LADDER_GORGE",
  PRACTICE: "G4_PR_REFOUNDING",
  COMMUNICATION_SYSTEM: "G4_COM_TERRACE_REGISTER",
  ARTIFACT: "G2_RIB_LATTICE",
  NARRATIVE: "G2_UNCLOSED_LAMENT",
  ORGANISM: "G3_SWEEP_PURPLE",
  INSTITUTE: "INST_ANCHOR",
  RESEARCHER: "RES_HALVORSEN",
  EXPEDITION: "EXP_GRID_44_THIRD",
  OBSERVATION: "OBSA_CORD_READING_RECORDER",
  INTERVIEW: "INTA_RISER_SEQUENCE_DISPUTE",
  PUBLICATION: "PUB_COMPARATIVE_FERMENT",
}

// The sections the classes which group their fields declare, in the order the class declares them (the
// section maps in internal/xeno/classes.go). Every other class lays its fields out as one list, which is
// asserted just as much: a section header appearing on a class which declares none would mean the fields of
// one class leaked into another.
const SECTIONS: Partial<Record<DocumentClass, ReadonlyArray<string>>> = {
  PLANET: ["identification", "physical", "environment", "survey"],
  MOON: ["identification", "physical", "environment", "survey"],
  SPECIES: ["identification", "biology", "society", "contact"],
  INTERVIEW: ["subject", "record", "clearance"],
}

// One entry of each controlled vocabulary. The entries of a vocabulary are all of the same shape (a name, a
// description and a code, each of them written in all three languages), so one entry per vocabulary covers
// what the vocabulary renders as.
const VOCABULARY_ENTRIES: Record<VocabularyClass, string> = {
  PLANET_TYPE: "TIDAL_OCEAN",
  BIOME: "VENT_FIELD",
  SITE_TYPE: "RELAY_STATION",
  CONTACT_STATUS: "SUSTAINED",
  SENSORY_MODALITY: "ELECTRORECEPTION",
  SUBSISTENCE_MODE: "SILT_GARDENING",
  SOCIAL_ORGANISATION: "LINEAGE_HOUSES",
  KINSHIP_SYSTEM: "DEBT_KINSHIP",
  INDIVIDUALITY_MODE: "DISTRIBUTED_SELF",
  ORGANISM_CATEGORY: "CHEMOLITHO_MAT",
  ARTIFACT_CATEGORY: "RECORDING_LATTICE",
  PRACTICE_CATEGORY: "RITE_OF_PASSAGE",
  NARRATIVE_GENRE: "TRICKSTER_CYCLE",
  RESEARCH_METHOD: "PARTICIPANT_OBSERVATION",
  ETHICS_PROTOCOL: "PROTOCOL_1",
  COMMUNICATION_MODALITY: "SUBSTRATE_PERCUSSION",
}

// The role which is granted reading the class the site keeps out of the public read scope (roles in
// config.yml). A document of that class is not readable by a visitor who is not signed in, so the one test
// over it signs in first while every other class is viewed as an anonymous visitor.
const RESTRICTED_CLASS_ROLE: Role = "ethics"

// The part of a screenshot name which identifies the class.
function slug(mnemonic: string): string {
  return mnemonic.toLowerCase().replaceAll("_", "-")
}

// The panel of the document view which renders the fields the document's class declares, which is the tab the
// view opens on.
function fieldsPanel(page: Page): Locator {
  return page.locator(".pd-documentget-panel-properties")
}

// Asserts that the document view shows the document: the title its naming claim gives it, and the fields its
// class declares, each row with a property label and a value. Returns how many field rows the class tab shows.
async function expectDocumentShown(page: Page, what: string): Promise<number> {
  const title = page.locator("#documentget-title")
  await expect(title, `title of the ${what} document`).toBeVisible()
  await expect(title, `title of the ${what} document is not empty`).not.toHaveText(/^\s*$/)

  const panel = fieldsPanel(page)
  await expect(panel, `properties panel of the ${what} document`).toBeVisible()
  await expect(panel.locator(".pd-fieldsview").first(), `fields of the ${what} document`).toBeVisible()

  const rowCount = await panel.locator(".pd-fieldsview-row").count()
  expect(rowCount, `field rows of the ${what} document`).toBeGreaterThan(0)

  const labelTexts = await panel.locator(".pd-fieldsview-label").allTextContents()
  expect(labelTexts.length, `property labels of the ${what} document`).toBeGreaterThan(0)
  for (const [i, text] of labelTexts.entries()) {
    expect(text.trim(), `property label ${i} of the ${what} document`).not.toBe("")
  }

  // The label is written into the first row of a field only, so a field stated several times is one label and
  // as many rows as it has values.
  expect(rowCount, `field rows of the ${what} document against the properties they are for`).toBeGreaterThanOrEqual(labelTexts.length)

  const valueTexts = await panel.locator(".pd-fieldsview-value").allTextContents()
  expect(valueTexts.length, `property values of the ${what} document`).toBeGreaterThan(0)
  for (const [i, text] of valueTexts.entries()) {
    expect(text.trim(), `property value ${i} of the ${what} document`).not.toBe("")
  }

  // A field whose claim only states that the property holds (a HAS claim without sub-fields, which is how a
  // yes-or-no field is stored) has nothing to put in the value column, so its row is a label next to an empty
  // cell. Every other row carries a value, and no row is empty on both sides.
  await expect(
    panel.locator(".pd-fieldsview-row:not(:has(.pd-fieldsview-label)):not(:has(.pd-fieldsview-value))"),
    `field rows of the ${what} document which show neither a property nor a value`,
  ).toHaveCount(0)

  return rowCount
}

// Asserts that the fields panel is split into exactly the sections the document's class declares, each of them
// headed by a name written in the interface language, and that a class which declares none renders no header.
async function expectSections(page: Page, sections: ReadonlyArray<string>, what: string): Promise<void> {
  await expect(fieldsPanel(page).locator(".pd-fieldsview-header-section"), `section headers of the ${what} document`).toHaveCount(sections.length)
  for (const section of sections) {
    const header = fieldsPanel(page).locator(`.pd-fieldsview-header-section-${section}`)
    await expect(header, `the ${section} section of the ${what} document`).toBeVisible()
    await expect(header, `the ${section} section of the ${what} document is named`).not.toHaveText(/^\s*$/)
  }
}

// Opens the "all properties" tab, which lists every claim of the document whatever its class declares, and
// asserts that the document states the class it was picked for. The document was opened by its identifier, so
// this is what says the identifier really is the identifier of a document of that class.
async function expectInstanceOf(page: Page, classId: string, what: string): Promise<void> {
  await openDocumentTab(page, "allproperties")
  await settle(page)
  const panel = page.locator(".pd-documentget-panel-allproperties")
  await expect(
    panel.locator(`.pd-propertiesview-row-${PROPERTY_IDS.INSTANCE_OF} .pd-propertiesview-value .pd-claimvalueref[data-url="/api/d/${classId}"]`),
    `the ${what} document is an instance of the class it was opened for`,
  ).toHaveCount(1)
}

test.describe("PeerDB Document Per Class Flows", () => {
  // Every class which holds documents is viewed through the document of it which fills in the most fields, so
  // that the whole catalogue is covered rather than the handful of classes the other files happen to use. What
  // is asserted is the same for all of them, because it is what the class-driven view promises for any class:
  // a title, the fields the class declares with a label and a value each, and the sections it groups them into.
  for (const documentClass of DOCUMENT_CLASSES) {
    test(`Test viewing the richest ${documentClass} document`, async ({ context }) => {
      const page = await context.newPage()

      if (documentClass === RESTRICTED_CLASS) {
        await signIn(page, [RESTRICTED_CLASS_ROLE])
      }

      await openDocument(page, await documentIdOf(documentClass, RICHEST[documentClass]))
      await settle(page)

      // The tab the view opens on is the one the class contributes, named after the class itself, which is
      // what says the view is rendering the document through its class and not only as a list of claims.
      const classTab = page.locator(".pd-documentget-tab-properties")
      await expect(classTab, `the class tab of the ${documentClass} document`).toBeVisible()
      await expect(classTab, `the class tab of the ${documentClass} document is named`).not.toHaveText(/^\s*$/)

      const rowCount = await expectDocumentShown(page, documentClass)
      const sections = SECTIONS[documentClass] ?? []
      await expectSections(page, sections, documentClass)
      await checkpoint(page, `per-class-${slug(documentClass)}-document`, { mask: volatile(page) })

      await expectInstanceOf(page, CLASS_IDS[documentClass], documentClass)
      await checkpoint(page, `per-class-${slug(documentClass)}-allproperties`, { mask: volatile(page) })

      console.log(`Successfully viewed the richest ${documentClass} document, with ${rowCount} field rows in ${sections.length} sections shown.`)
    })
  }

  // The entries of a controlled vocabulary are documents like any other: they are instances of the vocabulary
  // class, they are rendered by the fields that class declares, and they are what everything else in the
  // catalogue is classified by. Each vocabulary is covered through one of its entries.
  for (const vocabularyClass of VOCABULARY_CLASSES) {
    test(`Test viewing an entry of the ${vocabularyClass} vocabulary`, async ({ context }) => {
      const page = await context.newPage()

      await openDocument(page, await documentIdOf(vocabularyClass, VOCABULARY_ENTRIES[vocabularyClass]))
      await settle(page)

      const rowCount = await expectDocumentShown(page, vocabularyClass)
      // A vocabulary lays its three fields out as one list, so an entry has no section header at all.
      await expectSections(page, [], vocabularyClass)

      // The code is what the entry is known by outside the catalogue, and it is the one field of a vocabulary
      // entry which carries no language, so it is there whatever the interface language is.
      const code = fieldsPanel(page).locator(`.pd-fieldsview-row-${PROPERTY_IDS.CODE} .pd-fieldsview-value`)
      await expect(code, `the code of the ${vocabularyClass} entry`).toBeVisible()
      await expect(code, `the code of the ${vocabularyClass} entry is not empty`).not.toHaveText(/^\s*$/)

      await checkpoint(page, `per-class-vocabulary-${slug(vocabularyClass)}-document`, { mask: volatile(page) })
      await expectInstanceOf(page, CLASS_IDS[vocabularyClass], vocabularyClass)

      console.log(`Successfully viewed an entry of the ${vocabularyClass} vocabulary, with ${rowCount} field rows shown.`)
    })
  }
})
