import type { Locator, Page } from "@playwright/test"

import type { EntityClass } from "../peerdb_utils"

import { coreDocumentIdOf, documentIdOf, PROPERTY_IDS, startCreate } from "../peerdb_utils"
import {
  changePosted,
  checkpoint,
  checkpointElement,
  expect,
  expectNothingLoading,
  expectNothingPending,
  field,
  fieldInput,
  fieldSlots,
  fieldValues,
  fillSlot,
  hideDuplicates,
  LOADING_TIMEOUT,
  pickReference,
  saveEdit,
  signIn,
  slotInput,
  startEdit,
  test,
  volatile,
} from "../utils"

// Every document these tests create is named with this prefix, so that the documents of this file
// never collide with the ones another test file creates and a document left in the data set says
// which file made it.
const NAME_PREFIX = "Claim Inputs"

// Most of the tests work on a species they create themselves, because that class declares a field
// for nearly every kind of value the field form can render (a string with a string sub-claim, an
// identifier, HTML, an amount with a unit, an amount interval, a time with a precision, a reference
// searched by query and two reference selects) and needs nothing but a name in order to be saved.
const SPECIES_CLASS = "SPECIES"
// The role which may create a species (ROLE_CREATES in peerdb_utils says which role opens which
// class).
const SPECIES_ROLE = "researcher"

// A species declares neither a link nor a time interval, so those two are driven on a researcher,
// which carries a website and a period of activity. A researcher's family name is required but has
// a default, which the form fills in when the document is saved, so a researcher too needs nothing
// but a name.
const RESEARCHER_CLASS = "RESEARCHER"
const RESEARCHER_ROLE = "curator"

// The documents the reference inputs are pointed at, each by its identifier, so that the same
// document is picked out of the ranked results on every run: the world a species is native to, and
// the units a height and a lifespan are measured in.
const HOMEWORLD_ID = await documentIdOf("PLANET", "G1_SURVEY_GRID_44_B")
const HOMEWORLD_QUERY = "Survey Grid 44-b"
const METRE_ID = await coreDocumentIdOf("UNIT", "m")
const METRE_QUERY = "metre"
const EARTH_YEAR_ID = await documentIdOf("UNIT", "EARTH_YEAR")
const EARTH_YEAR_QUERY = "Earth year"

// The vocabulary entries the two reference selects are asked to select, also by identifier: the two
// modes of individuality a species can be given (one at a time, so the field renders radio buttons)
// and two of the ways it can feed itself (several at a time, so that field renders checkboxes).
const BOUNDED_ID = await documentIdOf("INDIVIDUALITY_MODE", "BOUNDED")
const FISSIONING_ID = await documentIdOf("INDIVIDUALITY_MODE", "FISSIONING")
const SEALED_CULTIVATION_ID = await documentIdOf("SUBSISTENCE_MODE", "SEALED_CULTIVATION")
const TRADE_DEPENDENCY_ID = await documentIdOf("SUBSISTENCE_MODE", "TRADE_DEPENDENCY")

// The block of one sub-field of a field of the edit form, which is where the sub-claims of a value
// are edited: the gloss of an endonym, the unit of an amount. A sub-field is rendered only once the
// value it belongs to has been entered, and it carries the identifier of its own property.
function subField(page: Page, propertyId: string, subPropertyId: string): Locator {
  return field(page, propertyId).locator(`.pd-claimcardinality-${subPropertyId}`).first()
}

// One of the entries a reference select offers, addressed by the document the entry stands for.
function selectItem(page: Page, propertyId: string, documentId: string): Locator {
  return field(page, propertyId).locator(`.pd-claimrefselect-item-${documentId}`)
}

// Waits until the edit form has taken focus into a field of its own. The form focuses a field as
// soon as it has loaded, which scrolls that field into view, and waiting for it once before anything
// is typed keeps it from taking focus away from an input a test has just driven.
async function settleFormFocus(page: Page): Promise<void> {
  await expect
    .poll(async () => await page.evaluate(() => document.activeElement?.tagName ?? ""), { message: "the form takes focus into a field of its own" })
    .not.toBe("BODY")
}

// Puts the page back at the top and keeps the view from scrolling it away again, which is what makes
// a screenshot of a form comparable between runs. A screenshot is taken of the whole page, where a
// fixed navbar and a sticky table of contents are drawn wherever the page happens to be scrolled to,
// so a scroll which lands between the page being anchored and the screenshot being taken moves them,
// and the next run moves them by something else. What scrolls the page is the table of contents: it
// puts the section the address names back under the navbar whenever the page is moved away from it,
// which taking the screenshot provokes by itself. It finds that section by the identifier of the
// heading which opens it, so renaming those headings leaves the page looking exactly as it does and
// leaves the table of contents nothing to scroll to.
async function settleScroll(page: Page): Promise<void> {
  let stable = 0
  await expect
    .poll(
      async () => {
        const position = await page.evaluate(() => {
          for (const heading of document.querySelectorAll("[id^='section-']")) {
            heading.id = `unanchored-${heading.id}`
          }
          const scrolled = Math.round(window.scrollY)
          if (scrolled !== 0) {
            window.scrollTo({ top: 0, left: 0, behavior: "instant" })
          }
          return scrolled
        })
        stable = position === 0 ? stable + 1 : 0
        return stable
      },
      { message: "the page stays at the top of itself", timeout: LOADING_TIMEOUT },
    )
    .toBeGreaterThanOrEqual(4)
}

// A checkpoint of the whole page, taken once the page has stopped scrolling.
async function checkpointPage(page: Page, name: string, mask: Array<Locator> = []): Promise<void> {
  await settleScroll(page)
  await checkpoint(page, name, { mask })
}

// A checkpoint of one block of the page, taken once the page has stopped scrolling.
async function checkpointBlock(page: Page, locator: Locator, name: string): Promise<void> {
  await settleScroll(page)
  await checkpointElement(page, locator, name)
}

// Creates a document of the given class with nothing but its name filled in and leaves the browser on
// the document view of what was created, checkpointing the empty form (when asked for it) and the
// name field on the way. This is what createNamed does, driven here so that every checkpoint is taken
// through the wrappers above.
async function createDocument(page: Page, entityClass: EntityClass, name: string, prefix: string, options: { emptyForm?: boolean } = {}): Promise<void> {
  await startCreate(page, entityClass)
  await hideDuplicates(page)
  await settleFormFocus(page)
  if (options.emptyForm) {
    await checkpointPage(page, `${prefix}-create-form`)
  }

  const nameInput = fieldInput(page, PROPERTY_IDS.NAME, ".pd-inputstring")
  await expect(nameInput, `the name input of the new ${entityClass}`).toBeVisible({ timeout: LOADING_TIMEOUT })
  const posted = changePosted(page)
  await nameInput.fill(name)
  await nameInput.blur()
  await expect(nameInput, `the name input of the new ${entityClass} holds the entered name`).toHaveValue(name)
  // The form is headed by what the document will be called, which is written as the name is typed, so
  // the title carrying the name is what says the form has caught up with what was entered.
  await expect(page.locator("#documentedit-title"), `the title of the form of the new ${entityClass}`).toHaveText(name)
  // The title is written from what the input holds, which the session does not hold until the slot has
  // posted it, and the save below acts on the session: without the wait it can write a document with no
  // name, which the class requires.
  await posted
  await expectNothingPending(page)
  await checkpointBlock(page, field(page, PROPERTY_IDS.NAME), `${prefix}-name`)

  await saveEdit(page)
  await expect(page.locator("#documentget-title"), `the title of the created ${entityClass}`).toHaveText(name, { timeout: LOADING_TIMEOUT })
}

// Drives a reference input the way a user does and checkpoints the three states it goes through: the
// query typed into it, the results it offered, and the value it was left holding. The shared
// pickReference does the same and is used wherever those states are not what the test is about; it
// checkpoints the whole page directly, which cannot be compared here, and it types the query again
// until the wanted document turns up, which is not needed for a document of the test data set: the
// index holds those from the start, so a search which does not offer one is a failure.
async function pickReferenceShown(page: Page, scope: Locator, query: string, documentId: string, prefix: string, what: string): Promise<void> {
  const input = scope.locator(".pd-inputref-input").first()
  await expect(input, `the reference input for ${what}`).toBeVisible()
  await input.fill(query)
  await expect(input, `the reference input for ${what} holds the typed query`).toHaveValue(query)
  await checkpointPage(page, `${prefix}-query`)

  const wanted = scope.locator(`.pd-inputref-item-${documentId}`)
  await expect(wanted, `the wanted result for ${what}`).toBeVisible({ timeout: LOADING_TIMEOUT })
  // Every result also links to the document it stands for, so the user can read it before picking it.
  await expect(scope.locator(".pd-inputref-link-result").first(), `the link of the first result for ${what}`).toBeVisible()
  await checkpointPage(page, `${prefix}-results`)

  await wanted.click()
  // The picked reference replaces the search input with the document's label and a clear button.
  await expect(scope.locator(".pd-inputref-value").first(), `the picked reference for ${what}`).toBeVisible()
  await expect(scope.locator(".pd-inputref-button-clear").first(), `the clear button after picking ${what}`).toBeVisible()
  await expectNothingLoading(page)
  await checkpointPage(page, `${prefix}-picked`)
}

// Opens the precision selector of a time input and leaves it open, so that which precision is
// selected can be read off the entries themselves. The selected entry is the one carrying
// aria-selected, which is how the precision is asserted without writing down a label: every label
// the selector shows is translated and differs between the three languages the site is served in.
async function openPrecision(scope: Locator, what: string): Promise<void> {
  const selector = scope.locator(".pd-inputtime-select-precision").first()
  await expect(selector, `the precision selector of ${what}`).toBeVisible()
  const entries = scope.locator(".pd-inputtime-item-precision").first()
  const opened = async (): Promise<boolean> => await entries.isVisible().catch(() => false)

  // The selector is pressed until it opens rather than once. Pressing it takes focus into it, which
  // scrolls it out from under the navbar, and a press which the page moves under lands as a press
  // outside the list the selector has just opened, which closes it again. Bringing the selector into
  // view first makes that unlikely and pressing it again makes it harmless.
  await expect
    .poll(
      async () => {
        if (await opened()) {
          return true
        }
        await selector.scrollIntoViewIfNeeded().catch(() => null)
        await selector.click().catch(() => null)
        return await opened()
      },
      { message: `the precision selector of ${what} opens`, timeout: LOADING_TIMEOUT },
    )
    .toBe(true)
}

// Closes the precision selector of a time input again and waits until its entries are gone, so that
// a screenshot taken afterwards is of the input rather than of a list which is on its way out. The
// selector is pressed rather than escaped, because pressing it closes it whatever holds focus.
async function closePrecision(scope: Locator, what: string): Promise<void> {
  const selector = scope.locator(".pd-inputtime-select-precision").first()
  const entries = scope.locator(".pd-inputtime-item-precision").first()
  const closed = async (): Promise<boolean> => !(await entries.isVisible().catch(() => true))

  await expect
    .poll(
      async () => {
        if (await closed()) {
          return true
        }
        await selector.click().catch(() => null)
        return await closed()
      },
      { message: `the precision selector of ${what} closes`, timeout: LOADING_TIMEOUT },
    )
    .toBe(true)
}

test.describe("PeerDB Claim Input Flows", () => {
  test("Test the string input", async ({ context }) => {
    const page = await context.newPage()

    await signIn(page, [SPECIES_ROLE])
    await createDocument(page, SPECIES_CLASS, `${NAME_PREFIX} String`, "claiminputs-string", { emptyForm: true })

    await startEdit(page)
    await hideDuplicates(page)
    await settleFormFocus(page)
    await checkpointPage(page, "claiminputs-string-edit-form")

    // A string input keeps the value exactly as it was typed. The alternative name is a repeated
    // field, so filling its only slot grows an empty one below it, which is how a repeated field is
    // given a second value: there is no button which adds one.
    await expect(fieldSlots(page, PROPERTY_IDS.ALTERNATIVE_NAME), "the alternative name starts with a single empty slot").toHaveCount(1)
    await fillSlot(page, PROPERTY_IDS.ALTERNATIVE_NAME, 0, ".pd-inputstring", "Rim-waiters", 2, "the first alternative name")
    await checkpointBlock(page, field(page, PROPERTY_IDS.ALTERNATIVE_NAME), "claiminputs-string-alternativename-first")
    await fillSlot(page, PROPERTY_IDS.ALTERNATIVE_NAME, 1, ".pd-inputstring", "Kesh-anaru people", 3, "the second alternative name")
    await checkpointBlock(page, field(page, PROPERTY_IDS.ALTERNATIVE_NAME), "claiminputs-string-alternativename-second")

    // The endonym holds a string too, and a value which carries sub-claims renders their inputs
    // below itself as soon as it has one, so the gloss of the endonym is a string input of its own.
    const endonym = slotInput(page, PROPERTY_IDS.ENDONYM, 0, ".pd-inputstring").first()
    await expect(endonym, "the endonym input").toBeVisible()
    await endonym.fill("kesh-anaru")
    await endonym.blur()
    await expect(endonym, "the endonym input holds the entered text").toHaveValue("kesh-anaru")
    const gloss = subField(page, PROPERTY_IDS.ENDONYM, PROPERTY_IDS.GLOSS).locator(".pd-inputstring").first()
    await expect(gloss, "the gloss, which is rendered once the endonym it belongs to has a value").toBeVisible({ timeout: LOADING_TIMEOUT })
    await gloss.fill("those who wait at the rim")
    await gloss.blur()
    await expect(gloss, "the gloss holds the entered text").toHaveValue("those who wait at the rim")
    await checkpointBlock(page, field(page, PROPERTY_IDS.ENDONYM), "claiminputs-string-endonym")

    await saveEdit(page)
    await expect(fieldValues(page, PROPERTY_IDS.ALTERNATIVE_NAME), "both alternative names are shown on the saved document").toHaveCount(2)
    await expect(fieldValues(page, PROPERTY_IDS.ENDONYM).first(), "the endonym shown on the saved document").toHaveText("kesh-anaru")
    await expect(fieldValues(page, PROPERTY_IDS.GLOSS).first(), "the gloss shown under the endonym").toHaveText("those who wait at the rim")
    await checkpointPage(page, "claiminputs-string-saved-document", volatile(page))

    // What was typed has to come back when the document is opened for editing again.
    await startEdit(page)
    await hideDuplicates(page)
    await settleFormFocus(page)
    await expect(slotInput(page, PROPERTY_IDS.ALTERNATIVE_NAME, 0, ".pd-inputstring").first(), "the saved first alternative name").toHaveValue("Rim-waiters")
    await expect(slotInput(page, PROPERTY_IDS.ALTERNATIVE_NAME, 1, ".pd-inputstring").first(), "the saved second alternative name").toHaveValue("Kesh-anaru people")
    await expect(slotInput(page, PROPERTY_IDS.ENDONYM, 0, ".pd-inputstring").first(), "the saved endonym").toHaveValue("kesh-anaru")
    await expect(subField(page, PROPERTY_IDS.ENDONYM, PROPERTY_IDS.GLOSS).locator(".pd-inputstring").first(), "the saved gloss").toHaveValue("those who wait at the rim")
    await checkpointPage(page, "claiminputs-string-reopened-form")

    console.log("Successfully edited 4 string claims (two values of a repeated field, an endonym and the gloss under it) on a newly created species.")
  })

  test("Test the identifier input", async ({ context }) => {
    const page = await context.newPage()

    await signIn(page, [SPECIES_ROLE])
    await createDocument(page, SPECIES_CLASS, `${NAME_PREFIX} Identifier`, "claiminputs-identifier")

    await startEdit(page)
    await hideDuplicates(page)
    await settleFormFocus(page)

    // An identifier input trims what was typed, and does so when the input is left rather than while
    // it is being typed into.
    const taxonCode = fieldInput(page, PROPERTY_IDS.TAXON_CODE, ".pd-inputidentifier")
    await expect(taxonCode, "the taxon code input").toBeVisible()
    await taxonCode.fill("  TX-9101-c  ")
    await expect(taxonCode, "the taxon code input holds the untrimmed text while it is focused").toHaveValue("  TX-9101-c  ")
    await checkpointBlock(page, field(page, PROPERTY_IDS.TAXON_CODE), "claiminputs-identifier-typed")
    await taxonCode.blur()
    await expect(taxonCode, "the taxon code input holds the trimmed identifier").toHaveValue("TX-9101-c")
    await checkpointBlock(page, field(page, PROPERTY_IDS.TAXON_CODE), "claiminputs-identifier-trimmed")

    await saveEdit(page)
    // The property declares a template for the register the code belongs to, so the saved identifier
    // is shown as a link into that register rather than as plain text. Only the register the link
    // leads into is asserted, and not the whole address: how the identifier is written into the
    // template is the template's business and is asserted where templates are tested.
    const savedCode = fieldValues(page, PROPERTY_IDS.TAXON_CODE).locator(".pd-claimvalueid")
    await expect(savedCode, "the identifier shown on the saved document").toHaveText("TX-9101-c")
    await expect(savedCode, "the identifier links into the register its property names").toHaveAttribute("href", /^https:\/\/registry\.ccx\.example\/taxon\//)
    await checkpointPage(page, "claiminputs-identifier-saved-document", volatile(page))

    await startEdit(page)
    await hideDuplicates(page)
    await settleFormFocus(page)
    await expect(fieldInput(page, PROPERTY_IDS.TAXON_CODE, ".pd-inputidentifier"), "the saved taxon code").toHaveValue("TX-9101-c")
    await checkpointPage(page, "claiminputs-identifier-reopened-form")

    console.log("Successfully edited 1 identifier claim, trimmed from the 4 spaces it was typed with, on a newly created species.")
  })

  test("Test the link input", async ({ context }) => {
    const page = await context.newPage()

    await signIn(page, [RESEARCHER_ROLE])
    await createDocument(page, RESEARCHER_CLASS, `${NAME_PREFIX} Link`, "claiminputs-link", { emptyForm: true })

    await startEdit(page)
    await hideDuplicates(page)
    await settleFormFocus(page)

    // A link input normalizes the URL when the input is left: the host is lower cased while the path
    // is kept as it was typed.
    const website = fieldInput(page, PROPERTY_IDS.WEBSITE, ".pd-inputlink-input")
    await expect(website, "the website input").toBeVisible()
    await website.fill("https://Anchor.CCX.example/people/Claim-Inputs")
    await expect(website, "the website input holds the URL as typed while it is focused").toHaveValue("https://Anchor.CCX.example/people/Claim-Inputs")
    await checkpointBlock(page, field(page, PROPERTY_IDS.WEBSITE), "claiminputs-link-typed")
    await website.blur()
    await expect(website, "the website input holds the normalized URL").toHaveValue("https://anchor.ccx.example/people/Claim-Inputs")
    // Once the URL parses, the input offers a link which opens it.
    const openLink = field(page, PROPERTY_IDS.WEBSITE).locator(".pd-inputlink-link").first()
    await expect(openLink, "the link to the entered URL").toBeVisible()
    await expect(openLink, "the link leads to the entered URL").toHaveAttribute("href", "https://anchor.ccx.example/people/Claim-Inputs")
    await checkpointBlock(page, field(page, PROPERTY_IDS.WEBSITE), "claiminputs-link-normalized")

    await saveEdit(page)
    const savedLink = fieldValues(page, PROPERTY_IDS.WEBSITE).locator(".pd-claimvaluelink")
    await expect(savedLink, "the link shown on the saved document").toHaveText("https://anchor.ccx.example/people/Claim-Inputs")
    await expect(savedLink, "the link shown on the saved document leads to the entered URL").toHaveAttribute("href", "https://anchor.ccx.example/people/Claim-Inputs")
    await checkpointPage(page, "claiminputs-link-saved-document", volatile(page))

    await startEdit(page)
    await hideDuplicates(page)
    await settleFormFocus(page)
    await expect(fieldInput(page, PROPERTY_IDS.WEBSITE, ".pd-inputlink-input"), "the saved website").toHaveValue("https://anchor.ccx.example/people/Claim-Inputs")
    await checkpointPage(page, "claiminputs-link-reopened-form")

    console.log("Successfully edited 1 link claim, whose host was lower cased and whose path was kept, on a newly created researcher.")
  })

  test("Test the time input and its precision selector", async ({ context }) => {
    const page = await context.newPage()

    await signIn(page, [SPECIES_ROLE])
    await createDocument(page, SPECIES_CLASS, `${NAME_PREFIX} Time`, "claiminputs-time")

    await startEdit(page)
    await hideDuplicates(page)
    await settleFormFocus(page)

    // A time input infers its precision from how much of the date was typed, so a whole date is
    // known to the day.
    const contact = field(page, PROPERTY_IDS.FIRST_CONTACT)
    const contactTime = fieldInput(page, PROPERTY_IDS.FIRST_CONTACT, ".pd-inputtime-input-time")
    await expect(contactTime, "the first contact input").toBeVisible()
    await contactTime.fill("2270-03-14")
    await expect(contactTime, "the first contact input holds the entered date").toHaveValue("2270-03-14")
    await openPrecision(contact, "the first contact")
    await expect(contact.locator(".pd-inputtime-item-precision-d"), "the precision inferred from a whole date is days").toHaveAttribute("aria-selected", "true")
    await checkpointPage(page, "claiminputs-time-precision-open")

    // Choosing a coarser precision rewrites the date down to it.
    await contact.locator(".pd-inputtime-item-precision-y").first().click()
    await expect(contactTime, "the date is reduced to the chosen precision").toHaveValue("2270")
    await openPrecision(contact, "the first contact after choosing years")
    await expect(contact.locator(".pd-inputtime-item-precision-y"), "the precision after choosing years").toHaveAttribute("aria-selected", "true")
    await closePrecision(contact, "the first contact after choosing years")
    await checkpointBlock(page, contact, "claiminputs-time-precision-years")

    // A date typed down to the month gets the matching precision without the selector being touched,
    // and is written in the canonical form of that precision, which spells out the day as 00, when
    // the input is left.
    await contactTime.fill("2299-06")
    await expect(contactTime, "the first contact input holds the entered year and month").toHaveValue("2299-06")
    await contactTime.blur()
    await expect(contactTime, "the date is written in the canonical form of its precision").toHaveValue("2299-06-00")
    await openPrecision(contact, "the first contact after typing a year and a month")
    await expect(contact.locator(".pd-inputtime-item-precision-m"), "the precision inferred from a year and a month is months").toHaveAttribute("aria-selected", "true")
    await closePrecision(contact, "the first contact after typing a year and a month")
    await checkpointBlock(page, contact, "claiminputs-time-months")

    await saveEdit(page)
    await expect(fieldValues(page, PROPERTY_IDS.FIRST_CONTACT), "the time shown on the saved document").toHaveCount(1)
    await checkpointPage(page, "claiminputs-time-saved-document", volatile(page))

    await startEdit(page)
    await hideDuplicates(page)
    await settleFormFocus(page)
    await expect(fieldInput(page, PROPERTY_IDS.FIRST_CONTACT, ".pd-inputtime-input-time"), "the saved first contact").toHaveValue("2299-06-00")
    await openPrecision(field(page, PROPERTY_IDS.FIRST_CONTACT), "the saved first contact")
    await expect(field(page, PROPERTY_IDS.FIRST_CONTACT).locator(".pd-inputtime-item-precision-m"), "the saved precision").toHaveAttribute("aria-selected", "true")
    await closePrecision(field(page, PROPERTY_IDS.FIRST_CONTACT), "the saved first contact")
    await checkpointPage(page, "claiminputs-time-reopened-form")

    console.log("Successfully edited 1 time claim through 3 precisions (days, years and months) on a newly created species.")
  })

  test("Test the amount input and its unit", async ({ context }) => {
    const page = await context.newPage()

    await signIn(page, [SPECIES_ROLE])
    await createDocument(page, SPECIES_CLASS, `${NAME_PREFIX} Amount`, "claiminputs-amount")

    await startEdit(page)
    await hideDuplicates(page)
    await settleFormFocus(page)

    // An amount is a number together with the precision it is known to, and the precision follows
    // the number of decimals typed for as long as the precision itself is left alone.
    const height = field(page, PROPERTY_IDS.TYPICAL_HEIGHT)
    const amount = fieldInput(page, PROPERTY_IDS.TYPICAL_HEIGHT, ".pd-inputamount-input-amount")
    const precision = fieldInput(page, PROPERTY_IDS.TYPICAL_HEIGHT, ".pd-inputamount-input-precision")
    await expect(amount, "the typical height input").toBeVisible()
    await amount.fill("1.75")
    await expect(amount, "the typical height input holds the entered amount").toHaveValue("1.75")
    await expect(precision, "the precision inferred from two decimals").toHaveValue("0.01")
    await checkpointBlock(page, height, "claiminputs-amount-typed")

    // Entering a precision by hand makes the precision the side which leads, and the amount is
    // rounded to it.
    await precision.fill("1")
    await precision.blur()
    await expect(precision, "the precision input holds the entered precision").toHaveValue("1")
    await expect(amount, "the amount is rounded to the entered precision").toHaveValue("2")
    await checkpointBlock(page, height, "claiminputs-amount-rounded")

    // Typing an amount again hands the lead back to the amount, so its decimals set the precision
    // once more.
    await amount.fill("1.75")
    await amount.blur()
    await expect(amount, "the amount typed after the precision").toHaveValue("1.75")
    await expect(precision, "the precision inferred again from the amount").toHaveValue("0.01")

    // The unit is a reference sub-claim of the amount, and like every sub-field it is rendered only
    // once the value it belongs to has been entered.
    const unit = subField(page, PROPERTY_IDS.TYPICAL_HEIGHT, PROPERTY_IDS.IN_UNIT)
    await expect(unit.locator(".pd-inputref-input").first(), "the unit of the typical height").toBeVisible({ timeout: LOADING_TIMEOUT })
    await pickReferenceShown(page, unit, METRE_QUERY, METRE_ID, "claiminputs-amount-unit", "the unit of the typical height")
    await expect(unit.locator(".pd-inputref-link-document").first(), "the picked unit links to the unit document").toHaveAttribute("href", new RegExp(`/d/${METRE_ID}`))

    await saveEdit(page)
    await expect(fieldValues(page, PROPERTY_IDS.TYPICAL_HEIGHT).locator(".pd-claimvalueamount"), "the amount shown on the saved document").toHaveText("1.75")
    await checkpointPage(page, "claiminputs-amount-saved-document", volatile(page))

    await startEdit(page)
    await hideDuplicates(page)
    await settleFormFocus(page)
    await expect(fieldInput(page, PROPERTY_IDS.TYPICAL_HEIGHT, ".pd-inputamount-input-amount"), "the saved amount").toHaveValue("1.75")
    await expect(fieldInput(page, PROPERTY_IDS.TYPICAL_HEIGHT, ".pd-inputamount-input-precision"), "the saved precision").toHaveValue("0.01")
    await expect(subField(page, PROPERTY_IDS.TYPICAL_HEIGHT, PROPERTY_IDS.IN_UNIT).locator(".pd-inputref-link-document").first(), "the saved unit").toHaveAttribute(
      "href",
      new RegExp(`/d/${METRE_ID}`),
    )
    await checkpointPage(page, "claiminputs-amount-reopened-form")

    console.log("Successfully edited 1 amount claim through 3 precisions of its own and gave it a unit, on a newly created species.")
  })

  test("Test the amount interval input", async ({ context }) => {
    const page = await context.newPage()

    await signIn(page, [SPECIES_ROLE])
    await createDocument(page, SPECIES_CLASS, `${NAME_PREFIX} Amount Interval`, "claiminputs-amountinterval")

    await startEdit(page)
    await hideDuplicates(page)
    await settleFormFocus(page)

    // An amount interval is two amount inputs, one per bound, under one property and one unit.
    const lifespan = field(page, PROPERTY_IDS.LIFESPAN)
    await expect(lifespan.locator(".pd-fieldsformrow-amountinterval"), "the lifespan is an amount interval").toBeVisible()
    const from = lifespan.locator(".pd-fieldsformrow-field-from").first()
    const to = lifespan.locator(".pd-fieldsformrow-field-to").first()

    const fromAmount = from.locator(".pd-inputamount-input-amount").first()
    await expect(fromAmount, "the from bound of the lifespan").toBeVisible()
    await fromAmount.fill("55")
    await expect(fromAmount, "the from bound holds the entered amount").toHaveValue("55")
    await expect(from.locator(".pd-inputamount-input-precision").first(), "the precision inferred for a whole number").toHaveValue("1")
    await checkpointBlock(page, lifespan, "claiminputs-amountinterval-from")

    const toAmount = to.locator(".pd-inputamount-input-amount").first()
    await expect(toAmount, "the to bound of the lifespan").toBeVisible()
    await toAmount.fill("110")
    await toAmount.blur()
    await expect(toAmount, "the to bound holds the entered amount").toHaveValue("110")
    await checkpointBlock(page, lifespan, "claiminputs-amountinterval-to")

    // The unit belongs to the interval as a whole and not to either bound, so there is one unit
    // sub-field for the two of them.
    const unit = subField(page, PROPERTY_IDS.LIFESPAN, PROPERTY_IDS.IN_UNIT)
    await expect(unit.locator(".pd-inputref-input").first(), "the unit of the lifespan").toBeVisible({ timeout: LOADING_TIMEOUT })
    await pickReference(page, unit, EARTH_YEAR_QUERY, EARTH_YEAR_ID, "claiminputs-amountinterval-unit")
    await expect(unit.locator(".pd-inputref-link-document").first(), "the picked unit links to the unit document").toHaveAttribute(
      "href",
      new RegExp(`/d/${EARTH_YEAR_ID}`),
    )
    await checkpointBlock(page, lifespan, "claiminputs-amountinterval-unit-picked")

    await saveEdit(page)
    await expect(fieldValues(page, PROPERTY_IDS.LIFESPAN).locator(".pd-claimvalueamountinterval-from"), "the from bound on the saved document").toHaveText("55")
    await expect(fieldValues(page, PROPERTY_IDS.LIFESPAN).locator(".pd-claimvalueamountinterval-to"), "the to bound on the saved document").toHaveText("110")
    await checkpointPage(page, "claiminputs-amountinterval-saved-document", volatile(page))

    await startEdit(page)
    await hideDuplicates(page)
    await settleFormFocus(page)
    const savedLifespan = field(page, PROPERTY_IDS.LIFESPAN)
    await expect(savedLifespan.locator(".pd-fieldsformrow-field-from .pd-inputamount-input-amount").first(), "the saved from bound").toHaveValue("55")
    await expect(savedLifespan.locator(".pd-fieldsformrow-field-to .pd-inputamount-input-amount").first(), "the saved to bound").toHaveValue("110")
    await checkpointPage(page, "claiminputs-amountinterval-reopened-form")

    console.log("Successfully edited 1 amount interval claim, 2 bounds and 1 unit, on a newly created species.")
  })

  test("Test the time interval input and the none and unknown markers of its bounds", async ({ context }) => {
    const page = await context.newPage()

    await signIn(page, [RESEARCHER_ROLE])
    await createDocument(page, RESEARCHER_CLASS, `${NAME_PREFIX} Time Interval`, "claiminputs-timeinterval")

    await startEdit(page)
    await hideDuplicates(page)
    await settleFormFocus(page)

    // A time interval is two time inputs, and each bound may be a date, or a statement that there is
    // no such date, or that the date is not known.
    const period = field(page, PROPERTY_IDS.ACTIVE_PERIOD)
    await expect(period.locator(".pd-fieldsformrow-timeinterval"), "the period of activity is a time interval").toBeVisible()
    const from = period.locator(".pd-fieldsformrow-field-from").first()
    const to = period.locator(".pd-fieldsformrow-field-to").first()

    const fromTime = from.locator(".pd-inputtime-input-time").first()
    await expect(fromTime, "the from bound of the period of activity").toBeVisible()
    await fromTime.fill("2258")
    await expect(fromTime, "the from bound holds the entered year").toHaveValue("2258")
    await openPrecision(from, "the from bound")
    await expect(from.locator(".pd-inputtime-item-precision-y"), "the precision inferred from a bare year").toHaveAttribute("aria-selected", "true")
    await closePrecision(from, "the from bound")
    await checkpointBlock(page, period, "claiminputs-timeinterval-from")

    // The two markers of a bound are mutually exclusive, so marking the bound as none has to clear
    // unknown and the other way around.
    const toUnknown = to.locator(".pd-inputmissing-checkbox-unknown").first()
    const toNone = to.locator(".pd-inputmissing-checkbox-none").first()
    await expect(toUnknown, "the unknown marker of the to bound").toBeVisible()
    await expect(toNone, "the none marker of the to bound").toBeVisible()
    await toUnknown.check()
    await expect(toUnknown, "the to bound is marked as unknown").toBeChecked()
    await expect(toNone, "the none marker stays unset while unknown is set").not.toBeChecked()
    await checkpointBlock(page, period, "claiminputs-timeinterval-to-unknown")

    await toNone.check()
    await expect(toNone, "the to bound is marked as none").toBeChecked()
    await expect(toUnknown, "the unknown marker is cleared by the none marker").not.toBeChecked()
    await checkpointBlock(page, period, "claiminputs-timeinterval-to-none")

    await toUnknown.check()
    await expect(toUnknown, "the to bound is marked as unknown again").toBeChecked()
    await expect(toNone, "the none marker is cleared by the unknown marker").not.toBeChecked()
    await toNone.check()
    await expect(toNone, "the to bound is left marked as none").toBeChecked()

    // The from bound carries the same two markers, which have to stay untouched by what was done to
    // the other bound.
    await expect(from.locator(".pd-inputmissing-checkbox-unknown").first(), "the unknown marker of the from bound").not.toBeChecked()
    await expect(from.locator(".pd-inputmissing-checkbox-none").first(), "the none marker of the from bound").not.toBeChecked()
    await checkpointPage(page, "claiminputs-timeinterval-edit-form")

    await saveEdit(page)
    await expect(fieldValues(page, PROPERTY_IDS.ACTIVE_PERIOD).locator(".pd-claimvaluetimeinterval-from"), "the from bound on the saved document").toBeVisible()
    await expect(fieldValues(page, PROPERTY_IDS.ACTIVE_PERIOD).locator(".pd-claimvaluetimeinterval-to"), "the to bound on the saved document").toBeVisible()
    await checkpointPage(page, "claiminputs-timeinterval-saved-document", volatile(page))

    await startEdit(page)
    await hideDuplicates(page)
    await settleFormFocus(page)
    const savedPeriod = field(page, PROPERTY_IDS.ACTIVE_PERIOD)
    await expect(savedPeriod.locator(".pd-fieldsformrow-field-from .pd-inputtime-input-time").first(), "the saved from bound").toHaveValue("2258")
    await expect(savedPeriod.locator(".pd-fieldsformrow-field-to .pd-inputmissing-checkbox-none").first(), "the saved none marker of the to bound").toBeChecked()
    await expect(
      savedPeriod.locator(".pd-fieldsformrow-field-to .pd-inputmissing-checkbox-unknown").first(),
      "the saved unknown marker of the to bound",
    ).not.toBeChecked()
    await checkpointPage(page, "claiminputs-timeinterval-reopened-form")

    console.log("Successfully edited 1 time interval claim whose from bound is a year and whose to bound was marked 4 times before it was left as none.")
  })

  test("Test the reference input", async ({ context }) => {
    // Creating the document, driving the input through typing, searching and picking, and saving is more
    // steps than one test's default budget covers while the rest of the suite is running.
    test.slow()

    const page = await context.newPage()

    await signIn(page, [SPECIES_ROLE])
    await createDocument(page, SPECIES_CLASS, `${NAME_PREFIX} Reference`, "claiminputs-ref")

    await startEdit(page)
    await hideDuplicates(page)
    await settleFormFocus(page)

    // The homeworld may only reference a planet or a moon, so the results are the worlds matching the
    // query and never any of the species this suite creates.
    const homeworld = field(page, PROPERTY_IDS.HAS_HOMEWORLD)
    await pickReferenceShown(page, homeworld, HOMEWORLD_QUERY, HOMEWORLD_ID, "claiminputs-ref-homeworld", "the homeworld")
    await expect(homeworld.locator(".pd-inputref-link-document").first(), "the picked homeworld links to the world it stands for").toHaveAttribute(
      "href",
      new RegExp(`/d/${HOMEWORLD_ID}`),
    )

    // Clearing drops the selection and brings back the search input, so a different document can be
    // picked in its place.
    const clearButton = homeworld.locator(".pd-inputref-button-clear").first()
    await expect(clearButton, "the clear button of the homeworld").toBeVisible()
    await clearButton.click()
    await expect(homeworld.locator(".pd-inputref-value"), "the selection after clearing").toHaveCount(0)
    await expect(homeworld.locator(".pd-inputref-input").first(), "the search input after clearing").toBeVisible()
    await checkpointPage(page, "claiminputs-ref-homeworld-cleared")

    await pickReferenceShown(page, homeworld, HOMEWORLD_QUERY, HOMEWORLD_ID, "claiminputs-ref-homeworld-again", "the homeworld picked again")

    await saveEdit(page)
    await expect(fieldValues(page, PROPERTY_IDS.HAS_HOMEWORLD).locator(`a[href*="/d/${HOMEWORLD_ID}"]`), "the reference shown on the saved document").toBeVisible()
    await checkpointPage(page, "claiminputs-ref-saved-document", volatile(page))

    await startEdit(page)
    await hideDuplicates(page)
    await settleFormFocus(page)
    await expect(field(page, PROPERTY_IDS.HAS_HOMEWORLD).locator(".pd-inputref-link-document").first(), "the saved homeworld").toHaveAttribute(
      "href",
      new RegExp(`/d/${HOMEWORLD_ID}`),
    )
    await checkpointPage(page, "claiminputs-ref-reopened-form")

    console.log("Successfully picked, cleared and picked again 1 reference claim on a newly created species.")
  })

  test("Test the HTML input and its toolbar", async ({ context }) => {
    const page = await context.newPage()

    await signIn(page, [SPECIES_ROLE])
    await createDocument(page, SPECIES_CLASS, `${NAME_PREFIX} HTML`, "claiminputs-html")

    await startEdit(page)
    await hideDuplicates(page)
    await settleFormFocus(page)

    // The notes are a repeated field, so a second slot with an editor and a toolbar of its own grows
    // as soon as the first one holds something. Everything below is therefore addressed inside the
    // first slot rather than inside the whole field.
    const notes = fieldSlots(page, PROPERTY_IDS.NOTES).first()
    const editor = notes.locator(".pd-inputhtml-editor")
    await expect(editor, "the notes editor").toBeVisible()
    await editor.click()
    await page.keyboard.type("The first paragraph of the note.")
    expect(await editor.innerHTML(), "the typed text is a paragraph").toContain("<p>The first paragraph of the note.</p>")
    await checkpointBlock(page, notes, "claiminputs-html-paragraph")

    // The block level buttons turn the block the cursor is in into a heading and into a list. They
    // are used before the inline marks below so that no mark can leak into the text typed afterwards.
    await page.keyboard.press("Enter")
    await page.keyboard.type("Section heading")
    await notes.locator(".pd-inputhtml-button-heading2").click()
    expect(await editor.innerHTML(), "the second block is a heading").toContain("<h2>Section heading</h2>")
    await checkpointBlock(page, notes, "claiminputs-html-heading")

    await page.keyboard.press("Enter")
    await page.keyboard.type("First item")
    await notes.locator(".pd-inputhtml-button-bulletlist").click()
    expect(await editor.innerHTML(), "the third block is a bullet list").toContain("<ul><li><p>First item</p></li></ul>")
    await checkpointBlock(page, notes, "claiminputs-html-bulletlist")

    // A link is made in three steps: the button opens a form at the bottom of the editor, the URL is
    // typed into it, and confirming wraps the selected text into the link.
    await page.keyboard.press("Enter")
    await page.keyboard.type("Source")
    await page.keyboard.press("Shift+Home")
    const linkButton = notes.locator(".pd-inputhtml-button-link")
    await expect(linkButton, "the link button").toBeEnabled()
    await linkButton.click()
    const linkForm = notes.locator(".pd-inputhtml-form-link")
    await expect(linkForm, "the link form").toBeVisible()
    const linkInput = linkForm.locator(".pd-inputhtml-input-link .pd-inputlink-input")
    await expect(linkInput, "the URL input of the link form").toBeVisible()
    await linkInput.fill("https://anchor.ccx.example/notes/claim-inputs")
    await expect(linkInput, "the URL input holds the entered URL").toHaveValue("https://anchor.ccx.example/notes/claim-inputs")
    await checkpointBlock(page, notes, "claiminputs-html-link-form")

    const confirmButton = linkForm.locator(".pd-inputhtml-button-confirm")
    await expect(confirmButton, "the confirm button of the link form").toBeEnabled()
    await confirmButton.click()
    expect(await editor.innerHTML(), "the selected text became a link").toContain('<a href="https://anchor.ccx.example/notes/claim-inputs"')
    await checkpointBlock(page, notes, "claiminputs-html-link-inserted")

    // The inline marks are applied to the first paragraph, selected from its start to its end.
    await editor.click()
    await page.keyboard.press("Control+Home")
    await page.keyboard.press("Shift+End")
    await notes.locator(".pd-inputhtml-button-bold").click()
    expect(await editor.innerHTML(), "the first paragraph is bold").toContain("<p><b>The first paragraph of the note.</b></p>")
    await expect(notes.locator(".pd-inputhtml-button-bold"), "the bold button shows the mark is on").toHaveAttribute("aria-pressed", "true")
    await notes.locator(".pd-inputhtml-button-italic").click()
    expect(await editor.innerHTML(), "the first paragraph is bold and italic").toContain("<p><b><i>The first paragraph of the note.</i></b></p>")
    await expect(notes.locator(".pd-inputhtml-button-italic"), "the italic button shows the mark is on").toHaveAttribute("aria-pressed", "true")
    await checkpointBlock(page, notes, "claiminputs-html-marks")

    await saveEdit(page)
    const savedHtml = await fieldValues(page, PROPERTY_IDS.NOTES).locator(".pd-claimvaluehtml").innerHTML()
    expect(savedHtml, "the saved note keeps the marks").toContain("<p><b><i>The first paragraph of the note.</i></b></p>")
    expect(savedHtml, "the saved note keeps the heading").toContain("<h2>Section heading</h2>")
    expect(savedHtml, "the saved note keeps the list").toContain("<li><p>First item</p></li>")
    expect(savedHtml, "the saved note keeps the link").toContain('href="https://anchor.ccx.example/notes/claim-inputs"')
    await checkpointPage(page, "claiminputs-html-saved-document", volatile(page))

    console.log("Successfully edited 1 HTML claim of 4 blocks with the heading, list, link, bold and italic buttons of its toolbar, on a newly created species.")
  })

  test("Test the reference select rendered as radio buttons", async ({ context }) => {
    const page = await context.newPage()

    await signIn(page, [SPECIES_ROLE])
    await createDocument(page, SPECIES_CLASS, `${NAME_PREFIX} Radio Select`, "claiminputs-radioselect")

    await startEdit(page)
    await hideDuplicates(page)
    await settleFormFocus(page)

    // A reference field whose candidate documents are few enough to be listed at once is rendered as
    // a list of all of them instead of as a search input, and a field which holds at most one value
    // gets radio buttons. The mode of individuality is such a field: it holds one value out of the
    // handful of modes the vocabulary declares.
    const individuality = field(page, PROPERTY_IDS.HAS_INDIVIDUALITY_MODE)
    await expect(individuality.locator(".pd-claimrefselect"), "the mode of individuality is a reference select").toBeVisible()
    await expect(individuality.locator(".pd-claimrefselect-radio").first(), "the entries of the mode of individuality are radio buttons").toBeVisible()
    await expect(individuality.locator(".pd-claimrefselect-checkbox"), "the mode of individuality offers no checkbox").toHaveCount(0)
    // Every entry also links to the document it stands for, so the user can read it before picking it.
    await expect(selectItem(page, PROPERTY_IDS.HAS_INDIVIDUALITY_MODE, BOUNDED_ID).locator(".pd-claimrefselect-link"), "the link of an entry").toBeVisible()
    await checkpointBlock(page, individuality, "claiminputs-radioselect-empty")

    const bounded = selectItem(page, PROPERTY_IDS.HAS_INDIVIDUALITY_MODE, BOUNDED_ID).locator(".pd-claimrefselect-radio")
    const fissioning = selectItem(page, PROPERTY_IDS.HAS_INDIVIDUALITY_MODE, FISSIONING_ID).locator(".pd-claimrefselect-radio")
    // A selection shows only once the change it made has been committed, so what says a click landed
    // is the entry reporting itself as selected rather than the click returning.
    await bounded.click()
    await expect(bounded, "the first entry is selected").toBeChecked({ timeout: LOADING_TIMEOUT })
    await checkpointBlock(page, individuality, "claiminputs-radioselect-first")

    // Selecting another entry of a single valued field replaces the selection.
    await fissioning.click()
    await expect(fissioning, "the second entry is selected").toBeChecked({ timeout: LOADING_TIMEOUT })
    await expect(bounded, "the first entry is unselected by the second").not.toBeChecked()
    await checkpointBlock(page, individuality, "claiminputs-radioselect-second")

    // Clicking the selected entry again clears the selection, which is how a radio button of a field
    // which may also hold nothing is unselected.
    await fissioning.click()
    await expect(fissioning, "the selected entry is cleared by clicking it again").not.toBeChecked({ timeout: LOADING_TIMEOUT })
    await checkpointBlock(page, individuality, "claiminputs-radioselect-cleared")

    await bounded.click()
    await expect(bounded, "the first entry is selected again").toBeChecked({ timeout: LOADING_TIMEOUT })

    await saveEdit(page)
    await expect(
      fieldValues(page, PROPERTY_IDS.HAS_INDIVIDUALITY_MODE).locator(`a[href*="/d/${BOUNDED_ID}"]`),
      "the selected mode of individuality on the saved document",
    ).toBeVisible()
    await checkpointPage(page, "claiminputs-radioselect-saved-document", volatile(page))

    await startEdit(page)
    await hideDuplicates(page)
    await settleFormFocus(page)
    await expect(selectItem(page, PROPERTY_IDS.HAS_INDIVIDUALITY_MODE, BOUNDED_ID).locator(".pd-claimrefselect-radio"), "the saved selection").toBeChecked({
      timeout: LOADING_TIMEOUT,
    })
    await expect(
      selectItem(page, PROPERTY_IDS.HAS_INDIVIDUALITY_MODE, FISSIONING_ID).locator(".pd-claimrefselect-radio"),
      "the entry which was cleared",
    ).not.toBeChecked()
    await checkpointPage(page, "claiminputs-radioselect-reopened-form")

    console.log(
      "Successfully selected, replaced, cleared and selected again 1 reference claim through the radio buttons of a reference select, on a newly created species.",
    )
  })

  test("Test the reference select rendered as checkboxes", async ({ context }) => {
    const page = await context.newPage()

    await signIn(page, [SPECIES_ROLE])
    await createDocument(page, SPECIES_CLASS, `${NAME_PREFIX} Checkbox Select`, "claiminputs-checkboxselect")

    await startEdit(page)
    await hideDuplicates(page)
    await settleFormFocus(page)

    // A reference field which is listed at once and may hold several values gets checkboxes instead
    // of radio buttons. The subsistence mode is such a field: a species may feed itself in more than
    // one way.
    const subsistence = field(page, PROPERTY_IDS.HAS_SUBSISTENCE_MODE)
    await expect(subsistence.locator(".pd-claimrefselect"), "the subsistence mode is a reference select").toBeVisible()
    await expect(subsistence.locator(".pd-claimrefselect-checkbox").first(), "the entries of the subsistence mode are checkboxes").toBeVisible()
    await expect(subsistence.locator(".pd-claimrefselect-radio"), "the subsistence mode offers no radio button").toHaveCount(0)
    await checkpointBlock(page, subsistence, "claiminputs-checkboxselect-empty")

    const sealed = selectItem(page, PROPERTY_IDS.HAS_SUBSISTENCE_MODE, SEALED_CULTIVATION_ID).locator(".pd-claimrefselect-checkbox")
    const trade = selectItem(page, PROPERTY_IDS.HAS_SUBSISTENCE_MODE, TRADE_DEPENDENCY_ID).locator(".pd-claimrefselect-checkbox")
    await sealed.click()
    await expect(sealed, "the first entry is selected").toBeChecked({ timeout: LOADING_TIMEOUT })
    await checkpointBlock(page, subsistence, "claiminputs-checkboxselect-first")

    // Selecting another entry of a repeated field adds to the selection instead of replacing it.
    await trade.click()
    await expect(trade, "the second entry is selected").toBeChecked({ timeout: LOADING_TIMEOUT })
    await expect(sealed, "the first entry stays selected next to the second").toBeChecked()
    await checkpointBlock(page, subsistence, "claiminputs-checkboxselect-second")

    // Clicking a selected checkbox drops that value alone.
    await trade.click()
    await expect(trade, "the second entry is unselected by clicking it again").not.toBeChecked({ timeout: LOADING_TIMEOUT })
    await expect(sealed, "the first entry survives the second being unselected").toBeChecked()
    await checkpointBlock(page, subsistence, "claiminputs-checkboxselect-unselected")

    await trade.click()
    await expect(trade, "the second entry is selected again").toBeChecked({ timeout: LOADING_TIMEOUT })

    await saveEdit(page)
    await expect(fieldValues(page, PROPERTY_IDS.HAS_SUBSISTENCE_MODE), "both selected values are shown on the saved document").toHaveCount(2)
    await expect(
      fieldValues(page, PROPERTY_IDS.HAS_SUBSISTENCE_MODE).locator(`a[href*="/d/${SEALED_CULTIVATION_ID}"]`),
      "the first selected value on the saved document",
    ).toBeVisible()
    await expect(
      fieldValues(page, PROPERTY_IDS.HAS_SUBSISTENCE_MODE).locator(`a[href*="/d/${TRADE_DEPENDENCY_ID}"]`),
      "the second selected value on the saved document",
    ).toBeVisible()
    await checkpointPage(page, "claiminputs-checkboxselect-saved-document", volatile(page))

    await startEdit(page)
    await hideDuplicates(page)
    await settleFormFocus(page)
    await expect(
      selectItem(page, PROPERTY_IDS.HAS_SUBSISTENCE_MODE, SEALED_CULTIVATION_ID).locator(".pd-claimrefselect-checkbox"),
      "the first saved selection",
    ).toBeChecked({ timeout: LOADING_TIMEOUT })
    await expect(
      selectItem(page, PROPERTY_IDS.HAS_SUBSISTENCE_MODE, TRADE_DEPENDENCY_ID).locator(".pd-claimrefselect-checkbox"),
      "the second saved selection",
    ).toBeChecked({ timeout: LOADING_TIMEOUT })
    await checkpointPage(page, "claiminputs-checkboxselect-reopened-form")

    console.log(
      "Successfully selected 2 reference claims, unselected 1 of them and selected it again through the checkboxes of a reference select, on a newly created species.",
    )
  })
})
