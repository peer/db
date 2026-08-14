import type { Locator, Page } from "@playwright/test"

import type { Language } from "../peerdb_utils"

import { CLASS_IDS, coreDocumentIdOf, documentIdOf, LANGUAGES, PROPERTY_IDS, startCreate } from "../peerdb_utils"
import {
  checkpoint,
  checkpointElement,
  expect,
  expectNothingLoading,
  field,
  fieldRow,
  fieldSlots,
  fieldValues,
  fillHtmlField,
  fillSlot,
  hideDuplicates,
  LOADING_TIMEOUT,
  openDocumentTab,
  PEERDB_URL,
  propertyValues,
  saveEdit,
  settle,
  settleEdit,
  signIn,
  slotInput,
  startEdit,
  switchLanguage,
  test,
  valueField,
  volatile,
} from "../utils"

// The prefix every document this file creates is named with, so that the documents of one test file never
// collide with another's and a stray document says which file made it.
const PREFIX = "E2E Lang"

// Everything here is done on a species, because that class is the one which carries all three of the
// things this file is about at once: instructions written for editors in every language the site speaks,
// fields grouped into sections which are named per language, and two repeated string fields whose values
// can each state the language they are written in.
const SPECIES_CLASS = "SPECIES"

// The role the site grants creating a species to (roles in config.yml).
const SPECIES_ROLE = "researcher"

// The name of each language in English, used to name the tests and the documents they create, so that a
// failure says which language it is about without carrying a translated word into a test name.
const LANGUAGE_NAMES: Record<Language, string> = { en: "English", sl: "Slovenian", pt: "Portuguese" }

// The languages the site is served in besides the default one, which are the ones an interface has to be
// translated into for this file to have anything to check.
const TRANSLATED_LANGUAGES: ReadonlyArray<Language> = LANGUAGES.filter((language) => language !== "en")

// The language documents of the core vocabulary (core/vocabularies.go), by the code the interface
// switcher uses. A value says which language it is written in by referring to one of these, so a test
// which writes a value in a language picks it by identifier and not by the name of the language, which is
// itself translated.
const LANGUAGE_IDS: Record<Language, string> = {
  en: await coreDocumentIdOf("LANGUAGE", "en-GB"),
  sl: await coreDocumentIdOf("LANGUAGE", "sl-SI"),
  pt: await coreDocumentIdOf("LANGUAGE", "pt-PT"),
}

// The entry of the mode of individuality vocabulary the tests pick, by its document identifier, because
// the vocabularies are data and are named in all three languages: picking one by its name would say
// nothing about the language of the page.
const BOUNDED_MODE = await documentIdOf("INDIVIDUALITY_MODE", "BOUNDED")

// The name of that entry in each language, as the test data states it
// (testdata/individuality_mode/BOUNDED.json), so that a document a test points at is asserted to be named
// in the language of the interface rather than in whichever language it happens to be listed under.
const BOUNDED_MODE_NAME: Record<Language, string> = { en: "Bounded individual", sl: "Zamejen posameznik", pt: "Indivíduo delimitado" }

// The name of the species class and of the abstract class the research records are gathered under, in
// each language, as the schema declares them (class(...) calls in internal/xeno/classes.go). The create
// page names a button with the first and a heading with the second, and the document view names the tab
// of the class with the first.
const SPECIES_CLASS_NAME: Record<Language, string> = { en: "species", sl: "vrsta", pt: "espécie" }
const RESEARCH_RECORD_CLASS_NAME: Record<Language, string> = { en: "research record", sl: "raziskovalni zapis", pt: "registo de investigação" }

// The title of the create page in each language (views.DocumentCreate.title in src/locales), which is the
// interface itself rather than the data, so it says the whole page and not only the documents on it
// follows the chosen language.
const CREATE_PAGE_TITLE: Record<Language, string> = { en: "Create a new document", sl: "Ustvari nov dokument", pt: "Criar um novo documento" }

// The badge a required field carries in each language (common.labels.required in src/locales).
const REQUIRED_BADGE: Record<Language, string> = { en: "required", sl: "obvezno", pt: "obrigatório" }

// The name of the sections a species record is read in, in each language (speciesSections in
// internal/xeno/classes.go). Both the form and the saved document head their groups of fields with them.
const SECTION_NAMES: Record<string, Record<Language, string>> = {
  identification: { en: "Identification", sl: "Določitev", pt: "Identificação" },
  society: { en: "Society", sl: "Družba", pt: "Sociedade" },
}

// The label of a field, which is the name of the property document the field holds, in each language
// (properties.go in internal/xeno and core/properties.go). The same names label the rows of the saved
// document, because both are rendered from the property document itself.
const PROPERTY_LABELS: Record<string, Record<Language, string>> = {
  [PROPERTY_IDS.NAME]: { en: "name", sl: "ime", pt: "nome" },
  [PROPERTY_IDS.ALTERNATIVE_NAME]: { en: "alternative name", sl: "alternativno ime", pt: "nome alternativo" },
  [PROPERTY_IDS.TAXON_CODE]: { en: "taxon code", sl: "taksonska oznaka", pt: "código taxonómico" },
  [PROPERTY_IDS.BODY_PLAN]: { en: "body plan", sl: "telesni ustroj", pt: "plano corporal" },
  [PROPERTY_IDS.HAS_INDIVIDUALITY_MODE]: { en: "mode of individuality", sl: "način posameznosti", pt: "modo de individualidade" },
}

// The instruction written for editors on each field of a species which carries one (speciesInstructions
// in internal/xeno/classes.go), in each language. Only the leading sentence is compared, because the
// block a field renders holds its instruction together with the hints of the input below it.
const INSTRUCTIONS: Record<string, Record<Language, string>> = {
  [PROPERTY_IDS.BODY_PLAN]: {
    en: "Describe the body as a body, not as a comparison.",
    sl: "Telo opišite kot telo, ne kot primerjavo.",
    pt: "Descreva o corpo como corpo, não como comparação.",
  },
  [PROPERTY_IDS.HAS_INDIVIDUALITY_MODE]: {
    en: "Where the discipline cannot agree whether the species has individuals at all, say so with the contested term rather than picking a side.",
    sl: "Kadar se stroka ne more zediniti, ali ima vrsta sploh posameznike, to povejte s spornim izrazom, namesto da izberete stran.",
    pt: "Quando a disciplina não consegue chegar a acordo sobre se a espécie tem sequer indivíduos, diga-o com o termo contestado em vez de tomar partido.",
  },
}

// What the test which writes a value per language writes. The two names are stated in one language each
// and the two exonyms in one language and in none at all, which is what makes the view have something to
// choose between and something to fall back to.
const NAME_IN_ENGLISH = `${PREFIX} Value Species in English`
const NAME_IN_SLOVENIAN = `${PREFIX} Value Species in Slovenian`
const EXONYM_IN_SLOVENIAN = `${PREFIX} Value Exonym in Slovenian`
const EXONYM_WITHOUT_LANGUAGE = `${PREFIX} Value Exonym without a language`

// Which of those values the document view has to show in each language. The site falls back the way it is
// configured to (languagePriority in config.yml): Slovenian and Portuguese fall back to English and then
// to the language-neutral value, and English falls back to the language-neutral value alone. The name is
// therefore the English one for a Portuguese reader, who has no Portuguese name to be shown, while the
// exonym, which is stated in no English at all, falls all the way through to the value without a language.
const EXPECTED_NAME: Record<Language, string> = { en: NAME_IN_ENGLISH, sl: NAME_IN_SLOVENIAN, pt: NAME_IN_ENGLISH }
const EXPECTED_EXONYM: Record<Language, string> = { en: EXONYM_WITHOUT_LANGUAGE, sl: EXONYM_IN_SLOVENIAN, pt: EXONYM_WITHOUT_LANGUAGE }

// What the tests which drive the form in one language write into the fields they fill.
function speciesName(language: Language): string {
  return `${PREFIX} ${LANGUAGE_NAMES[language]} Species`
}

function bodyPlanText(language: Language): string {
  return `${PREFIX} body plan written while the interface was in ${LANGUAGE_NAMES[language]}.`
}

// How far below the navbar a checkpoint of the top of the editor leaves the view, in pixels.
const NAVBAR_CLEARANCE = 8

// Screenshots the editor from its top, so that the words the interface puts around the form are captured
// next to the first fields of the form itself.
//
// The window is captured rather than the whole page. The editor of a class whose fields are grouped into
// sections opens on an address ending in an anchor to the first section, and capturing a whole page makes
// the browser lay the page out again, which sends it to that anchor a second time. That happens between
// one capture and the next, so a whole page capture of such a form catches it either before or after the
// jump depending on how quickly the capture runs. A window capture lays nothing out again and stays where
// the page was put.
//
// One field of the form is screenshotted with the shared checkpointElement instead, which clips a whole
// page capture to the field: what a clip holds does not depend on where the page happens to be scrolled,
// and a field whose box the window is scrolled to lands against the edge of the navbar, where the corner
// of an input is blended with the navbar's shadow a shade differently from one run to the next.
async function checkpointEditorTop(page: Page, name: string): Promise<void> {
  const editor = page.locator(".pd-documentedit")
  await expect(editor, `the editor for ${name}`).toBeVisible()
  // The navbar is fixed over the top of the window, so the page is scrolled to above the editor itself:
  // scrolling exactly to it would leave its first rows behind the navbar and capture everything but them.
  // How far above is the height of the navbar, read from the page rather than guessed, so a change to it
  // does not shift what the screenshots are of, plus a little clearance.
  //
  // The page is then left at a whole number of pixels, because a box which lands between two pixels is
  // drawn a shade differently, and a screenshot is compared pixel by pixel.
  const navbarHeight = await page.locator("#navbar").evaluate((element) => element.getBoundingClientRect().height)
  await editor.evaluate((element, offset) => {
    const top = element.getBoundingClientRect().top + window.scrollY - offset
    window.scrollTo({ top: Math.round(top), left: 0, behavior: "instant" })
    // Scrolling past the end of the page lands wherever the end is, which is a fraction as often as not,
    // so what was reached is rounded down rather than what was asked for being rounded.
    window.scrollTo({ top: Math.floor(window.scrollY), left: 0, behavior: "instant" })
  }, navbarHeight + NAVBAR_CLEARANCE)
  await checkpoint(page, name, { fullPage: false })
}

// Closes the menu of the signed-in user. Switching the language leaves it open, because the switcher of a
// signed-in user sits inside it and is read back through it, and a screenshot taken with it open is of a
// menu covering half of the view rather than of the view.
async function closeUserMenu(page: Page): Promise<void> {
  const panel = page.locator(".pd-navbarmenu-panel")
  if (await panel.isVisible()) {
    await page.locator(".pd-navbarmenu-button").click()
  }
  await expect(panel, "the menu of the signed-in user is closed").toBeHidden()
}

// The label of one field of the edit form, which is the display label of the property the field holds.
function formLabel(page: Page, propertyId: string): Locator {
  return field(page, propertyId).locator(".pd-fieldsformfield-label")
}

// Asserts that every field of the form is labelled with the name of its property in the given language.
async function expectFormLabels(page: Page, language: Language): Promise<void> {
  for (const [propertyId, labels] of Object.entries(PROPERTY_LABELS)) {
    await expect(formLabel(page, propertyId), `the label of the field for ${labels.en}, in ${LANGUAGE_NAMES[language]}`).toHaveText(labels[language])
  }
}

// Asserts that the instruction of a field is the one written in the given language and that the same
// instruction in either other language is nowhere on the form. Both halves are needed: without the first
// a dropped instruction would pass, and without the second an untranslated one would.
async function expectInstruction(page: Page, propertyId: string, language: Language): Promise<void> {
  const texts = INSTRUCTIONS[propertyId]
  const block = valueField(page, propertyId).locator(".pd-claimcardinality-text-hints")
  await expect(block, `the instruction of the field for ${PROPERTY_LABELS[propertyId].en}, in ${LANGUAGE_NAMES[language]}`).toContainText(texts[language])
  for (const other of LANGUAGES) {
    if (other === language) {
      continue
    }
    await expect(
      page.locator(".pd-fieldsform"),
      `the ${LANGUAGE_NAMES[other]} instruction of ${PROPERTY_LABELS[propertyId].en} is nowhere on the form`,
    ).not.toContainText(texts[other])
  }
}

// The label cell of the row of the saved document which is for the given property, which is the name of
// the property in the language of the interface.
function viewLabel(page: Page, propertyId: string): Locator {
  return fieldRow(page, propertyId).locator(".pd-fieldsview-label")
}

// States which language a value is written in. The language is a sub-claim of the value rather than a
// field of the document, so it is picked inside the slot holding the value, and the sub-field it is
// picked in appears only once that value has been committed, which is why this is called after the slot
// has been filled. Every language the site is served in is offered, so the wanted one is picked by the
// identifier of its language document.
async function stateLanguage(page: Page, propertyId: string, slot: number, language: Language, what: string): Promise<void> {
  const option = fieldSlots(page, propertyId)
    .nth(slot)
    .locator(`.pd-claimcardinality-${PROPERTY_IDS.IN_LANGUAGE} .pd-claimrefselect-item-${LANGUAGE_IDS[language]} .pd-claimrefselect-checkbox`)
  await expect(option, `the ${LANGUAGE_NAMES[language]} option of ${what}`).toBeVisible({ timeout: LOADING_TIMEOUT })
  await option.check()
  await expect(option, `${what} is stated to be in ${LANGUAGE_NAMES[language]}`).toBeChecked()
  await settleEdit(page)
}

// The identifiers of the classes the create page offers, read out of the identifier every class button
// carries in its own class name, so that what is offered is compared between languages as a set of
// documents rather than as a list of labels. A class which is a subclass of more than one class is listed
// once under each of them, so the identifiers are deduplicated.
async function offeredClasses(page: Page): Promise<Array<string>> {
  const ids = await page
    .locator(".pd-classtreelabel-button")
    .evaluateAll((buttons) =>
      buttons.map((button) => [...button.classList].map((name) => /^pd-classtreelabel-button-(.+)$/.exec(name)?.[1]).find((id) => id !== undefined) ?? ""),
    )
  return [...new Set(ids)].sort()
}

test.describe("PeerDB Language Flows", () => {
  test("Test the create page and its class tree in each interface language", async ({ context }) => {
    test.setTimeout(300_000)

    const page = await context.newPage()

    await signIn(page, [SPECIES_ROLE])

    // What the page offers has to be the same set of classes whatever it is read in, and each of them has
    // to be labelled differently in each language, so both are collected per language and compared once
    // every language has been visited.
    const offered: Partial<Record<Language, Array<string>>> = {}
    const speciesLabels: Array<string> = []
    for (const language of LANGUAGES) {
      await switchLanguage(page, language)
      await page.goto(`${PEERDB_URL}/d/create`)
      await expect(page.locator(".pd-documentcreate"), `the create page in ${LANGUAGE_NAMES[language]}`).toBeVisible({ timeout: LOADING_TIMEOUT })
      // A class whose document has not resolved yet renders a placeholder instead of its label, so the
      // labels are read only once every class the page lists has arrived.
      await settle(page)

      await expect(page.locator("#documentcreate-title"), `the title of the create page in ${LANGUAGE_NAMES[language]}`).toHaveText(CREATE_PAGE_TITLE[language])
      const speciesButton = page.locator(`.pd-classtreelabel-button-${CLASS_IDS[SPECIES_CLASS]}`)
      await expect(speciesButton, `the species button in ${LANGUAGE_NAMES[language]}`).toHaveText(SPECIES_CLASS_NAME[language])
      speciesLabels.push(((await speciesButton.textContent()) ?? "").trim())
      // A class a document cannot be created for is a heading gathering the classes below it, and it is
      // named in the language of the page just as much as a button is.
      await expect(
        page.locator(`.pd-classtreelabel-title[data-url="/api/d/${CLASS_IDS.RESEARCH_RECORD}"]`),
        `the research record heading in ${LANGUAGE_NAMES[language]}`,
      ).toHaveText(RESEARCH_RECORD_CLASS_NAME[language])

      offered[language] = await offeredClasses(page)
      await checkpoint(page, `languages-classtree-${language}`)
    }

    for (const language of TRANSLATED_LANGUAGES) {
      expect(offered[language], `the classes the create page offers in ${LANGUAGE_NAMES[language]}`).toEqual(offered.en)
    }
    // The label one class was read under in each language is what says each language is a language of its
    // own, and not the default one served under another name.
    expect(new Set(speciesLabels).size, `the species button was read as ${speciesLabels.join(", ")}`).toBe(LANGUAGES.length)

    console.log(
      `Successfully checked the create page in ${LANGUAGES.length} languages, each offering the same ${offered.en?.length ?? 0} classes under translated labels.`,
    )
  })

  for (const language of TRANSLATED_LANGUAGES) {
    test(`Test creating and editing a species with the interface in ${LANGUAGE_NAMES[language]}`, async ({ context }) => {
      // The test drives a form field by field, saves, opens the document again for editing and saves once
      // more, which is a good deal more work than a view test does, so it gets a budget of its own.
      test.setTimeout(900_000)

      const page = await context.newPage()

      await signIn(page, [SPECIES_ROLE])
      await switchLanguage(page, language)

      await startCreate(page, SPECIES_CLASS)
      await hideDuplicates(page)

      // The form is translated in three separate ways, and all three have to hold: the fields are labelled
      // with the names of the properties they hold, the groups the fields are read in are named, and the
      // badges the interface annotates a field with are the interface's own words.
      await expectFormLabels(page, language)
      for (const [section, names] of Object.entries(SECTION_NAMES)) {
        await expect(page.locator(`#section-${section}`), `the ${section} section of the form, in ${LANGUAGE_NAMES[language]}`).toHaveText(names[language])
      }
      await expect(field(page, PROPERTY_IDS.NAME).locator(".pd-inputbadges-badge-required"), `the required badge, in ${LANGUAGE_NAMES[language]}`).toHaveText(
        REQUIRED_BADGE[language],
      )

      // The instructions are written for editors rather than derived from the field, so a missing
      // translation shows up as the English text on the form rather than as an empty block.
      for (const propertyId of Object.keys(INSTRUCTIONS)) {
        await expectInstruction(page, propertyId, language)
      }
      // They are raw HTML and are rendered as such, so an instruction arrives as a paragraph and not as
      // one run of escaped markup.
      await expect(
        valueField(page, PROPERTY_IDS.BODY_PLAN).locator(".pd-claimcardinality-text-hints p"),
        `the instruction of the body plan field is rendered as a paragraph in ${LANGUAGE_NAMES[language]}`,
      ).toHaveCount(1)
      await checkpointEditorTop(page, `languages-create-${language}-form-empty`)

      // The name is written without stating a language, which leaves it language-neutral and therefore
      // shown to every reader: what this test is about is the language of the interface and not of the
      // value, which the test below covers.
      const name = speciesName(language)
      await fillSlot(page, PROPERTY_IDS.NAME, 0, ".pd-inputstring", name, 2, `the name of the new species in ${LANGUAGE_NAMES[language]}`)
      await checkpointElement(page, field(page, PROPERTY_IDS.NAME), `languages-create-${language}-name`)

      await fillHtmlField(page, PROPERTY_IDS.BODY_PLAN, bodyPlanText(language), `the body plan of the new species in ${LANGUAGE_NAMES[language]}`)
      await checkpointElement(page, field(page, PROPERTY_IDS.BODY_PLAN), `languages-create-${language}-bodyplan`)

      const id = await saveEdit(page)

      // The saved document has to be rendered in the same language: the tab is named after the class, the
      // rows after the properties, and the groups after the sections of the class.
      await expect(page.locator("#documentget-title"), `the title of the created species in ${LANGUAGE_NAMES[language]}`).toHaveText(name)
      await expect(page.locator(".pd-documentget-tab-properties"), `the tab of the class, in ${LANGUAGE_NAMES[language]}`).toHaveText(SPECIES_CLASS_NAME[language])
      await expect(viewLabel(page, PROPERTY_IDS.NAME), `the label of the name row, in ${LANGUAGE_NAMES[language]}`).toHaveText(
        PROPERTY_LABELS[PROPERTY_IDS.NAME][language],
      )
      await expect(viewLabel(page, PROPERTY_IDS.BODY_PLAN), `the label of the body plan row, in ${LANGUAGE_NAMES[language]}`).toHaveText(
        PROPERTY_LABELS[PROPERTY_IDS.BODY_PLAN][language],
      )
      await expect(
        page.locator(".pd-fieldsview-header-section-identification"),
        `the identification section of the created species, in ${LANGUAGE_NAMES[language]}`,
      ).toHaveText(SECTION_NAMES.identification[language])
      await expect(fieldValues(page,PROPERTY_IDS.BODY_PLAN), "the body plan which was written").toContainText(bodyPlanText(language))
      await checkpoint(page, `languages-create-${language}-saved`, { mask: volatile(page) })

      // Editing a document which already exists has to be as translated as creating one, which is not the
      // same code path: the form is then filled from the document rather than from the class alone.
      await startEdit(page)
      await hideDuplicates(page)
      await expectFormLabels(page, language)
      await expectInstruction(page, PROPERTY_IDS.HAS_INDIVIDUALITY_MODE, language)
      await expect(
        slotInput(page, PROPERTY_IDS.NAME, 0, ".pd-inputstring"),
        `the form of the saved species holds the name it was created with in ${LANGUAGE_NAMES[language]}`,
      ).toHaveValue(name)

      // The mode of individuality is a vocabulary entry, so picking one adds a value which is data rather
      // than interface, and which is named in all three languages just like the interface is.
      const individuality = field(page, PROPERTY_IDS.HAS_INDIVIDUALITY_MODE)
      const bounded = individuality.locator(`.pd-claimrefselect-item-${BOUNDED_MODE}`)
      await expect(bounded.locator(".pd-claimrefselect-label"), `the option of the bounded mode, in ${LANGUAGE_NAMES[language]}`).toHaveText(BOUNDED_MODE_NAME[language])
      await bounded.locator(".pd-claimrefselect-radio").check()
      await expect(bounded.locator(".pd-claimrefselect-radio"), "the mode of individuality which was picked").toBeChecked()
      await settleEdit(page)
      await checkpointElement(page, individuality, `languages-edit-${language}-individuality`)

      const savedAgain = await saveEdit(page)
      expect(savedAgain, "editing a document leaves it under the identifier it was created with").toBe(id)

      await expect(fieldValues(page,PROPERTY_IDS.HAS_INDIVIDUALITY_MODE), `the mode of individuality which was picked, in ${LANGUAGE_NAMES[language]}`).toHaveText(
        BOUNDED_MODE_NAME[language],
      )
      await expect(page.locator(".pd-fieldsview-header-section-society"), `the society section of the edited species, in ${LANGUAGE_NAMES[language]}`).toHaveText(
        SECTION_NAMES.society[language],
      )
      await checkpoint(page, `languages-edit-${language}-saved`, { mask: volatile(page) })

      // Switching the interface to English has to bring the same document back in English, which is what
      // says that everything asserted above followed the language of the page rather than something which
      // was written into the document.
      await switchLanguage(page, "en")
      await closeUserMenu(page)
      await expectNothingLoading(page)
      await expect(page.locator(".pd-documentget-tab-properties"), "the tab of the class, in English").toHaveText(SPECIES_CLASS_NAME.en)
      await expect(viewLabel(page, PROPERTY_IDS.BODY_PLAN), "the label of the body plan row, in English").toHaveText(PROPERTY_LABELS[PROPERTY_IDS.BODY_PLAN].en)
      await expect(fieldValues(page,PROPERTY_IDS.HAS_INDIVIDUALITY_MODE), "the mode of individuality which was picked, in English").toHaveText(BOUNDED_MODE_NAME.en)
      await expect(page.locator("#documentget-title"), "a value stated in no language at all is shown to an English reader too").toHaveText(name)
      await checkpoint(page, `languages-edit-${language}-saved-in-english`, { mask: volatile(page) })

      const shownRows = await page.locator(".pd-documentget-panel-properties .pd-fieldsview-label").count()
      console.log(`Successfully created and then edited a species through the form in ${LANGUAGE_NAMES[language]}, saved as ${id} with ${shownRows} labelled rows on it.`)
    })
  }

  test("Test the language a value is stated in and how the document view falls back", async ({ context }) => {
    test.setTimeout(900_000)

    const page = await context.newPage()

    await signIn(page, [SPECIES_ROLE])

    // The document is written with the interface in its default language, because what this test is about
    // is the language of the values and not the language the editor happened to be reading.
    await startCreate(page, SPECIES_CLASS)
    await hideDuplicates(page)

    // The name is stated twice, once in English and once in Slovenian, which is what a catalogue does with
    // a name it has a translation of.
    await fillSlot(page, PROPERTY_IDS.NAME, 0, ".pd-inputstring", NAME_IN_ENGLISH, 2, "the English name")
    await stateLanguage(page, PROPERTY_IDS.NAME, 0, "en", "the English name")
    await fillSlot(page, PROPERTY_IDS.NAME, 1, ".pd-inputstring", NAME_IN_SLOVENIAN, 3, "the Slovenian name")
    await stateLanguage(page, PROPERTY_IDS.NAME, 1, "sl", "the Slovenian name")
    await checkpointElement(page, field(page, PROPERTY_IDS.NAME), "languages-value-name-field")

    // The exonym is stated in Slovenian and in no language at all, and never in English, so that a reader
    // of a language which has no value of its own has nothing to fall back to but the language-neutral one.
    await fillSlot(page, PROPERTY_IDS.ALTERNATIVE_NAME, 0, ".pd-inputstring", EXONYM_IN_SLOVENIAN, 2, "the Slovenian exonym")
    await stateLanguage(page, PROPERTY_IDS.ALTERNATIVE_NAME, 0, "sl", "the Slovenian exonym")
    await fillSlot(page, PROPERTY_IDS.ALTERNATIVE_NAME, 1, ".pd-inputstring", EXONYM_WITHOUT_LANGUAGE, 3, "the exonym without a language")
    await checkpointElement(page, field(page, PROPERTY_IDS.ALTERNATIVE_NAME), "languages-value-exonym-field")

    const id = await saveEdit(page)

    // What each reader is shown is the value of their own language where there is one, and what the site
    // is configured to fall back to where there is not.
    for (const language of LANGUAGES) {
      await switchLanguage(page, language)
      await closeUserMenu(page)
      await expectNothingLoading(page)

      await expect(fieldValues(page,PROPERTY_IDS.NAME), `the name shown to a reader of ${LANGUAGE_NAMES[language]}`).toHaveText(EXPECTED_NAME[language])
      await expect(fieldValues(page,PROPERTY_IDS.ALTERNATIVE_NAME), `the exonym shown to a reader of ${LANGUAGE_NAMES[language]}`).toHaveText(EXPECTED_EXONYM[language])
      // Only the value of the language being read is rendered, so the other one is not somewhere further
      // down the page either.
      for (const other of [NAME_IN_ENGLISH, NAME_IN_SLOVENIAN, EXONYM_IN_SLOVENIAN, EXONYM_WITHOUT_LANGUAGE]) {
        if (other === EXPECTED_NAME[language] || other === EXPECTED_EXONYM[language]) {
          continue
        }
        await expect(page.locator(".pd-documentget-panel-properties"), `${other} is not shown to a reader of ${LANGUAGE_NAMES[language]}`).not.toContainText(other)
      }
      // The title of the document is derived from its name, so it follows the same choice.
      await expect(page.locator("#documentget-title"), `the title shown to a reader of ${LANGUAGE_NAMES[language]}`).toHaveText(EXPECTED_NAME[language])
      await checkpoint(page, `languages-value-document-${language}`, { mask: volatile(page) })
    }

    // Which language a value is in is an editing detail rather than something to read, so the view does
    // not render it as a row of its own, even though it is a claim like any other.
    await expect(fieldRow(page,PROPERTY_IDS.IN_LANGUAGE), "the language of a value is not a row of the document view").toHaveCount(0)

    // The claims tab is not the document view: it lists what the document holds rather than what a reader
    // of one language is shown, so every name is there whatever language it is stated in.
    await openDocumentTab(page, "allproperties")
    await expect(propertyValues(page, PROPERTY_IDS.NAME), "every name the document holds, whatever its language").toHaveCount(2)
    await expect(propertyValues(page, PROPERTY_IDS.ALTERNATIVE_NAME), "every exonym the document holds, whatever its language").toHaveCount(2)
    await expect(page.locator(".pd-documentget-panel-allproperties"), "the claims tab lists the name which the current language does not show").toContainText(
      NAME_IN_SLOVENIAN,
    )
    await checkpoint(page, "languages-value-allproperties", { mask: volatile(page) })

    console.log(
      `Successfully verified that a species with 2 names and 2 exonyms stated in 2 languages and in none is read back correctly in ${LANGUAGES.length} languages, saved as ${id}.`,
    )
  })
})
