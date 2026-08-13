import type { Locator, Page } from "@playwright/test"

import type { EntityClass, Role } from "../peerdb_utils"

import { PROPERTY_IDS, ROLE_CREATES, ROLES, startCreate } from "../peerdb_utils"
import {
  checkpoint,
  checkpointElement,
  documentId,
  expect,
  fetchFromPage,
  field,
  fieldInput,
  hideDuplicates,
  LOADING_TIMEOUT,
  saveEdit,
  settleEdit,
  signIn,
  startEdit,
  switchLanguage,
  test,
  volatile,
} from "../utils"

// Every test here works on a researcher it creates itself. That class carries one field of each kind
// whose invalid values are driven here: a link (the website), a time (the year of birth), a reference
// (what the researcher specialises in) and a time interval (the period of activity). The year of birth
// comes before the website in the form (order 8 against order 9 in fields.go), which is what the test
// of which field a refused save focuses rests on.
const RESEARCHER_CLASS: EntityClass = "RESEARCHER"

// The names of the documents these tests create all begin with this, so that a document made here is
// told apart from the ones another test file makes, and a leftover says which file made it.
const NAME_PREFIX = "PeerDB Edit Validation"

// A link which cannot be parsed as a URL: it carries no scheme, it does not begin with a slash, and the
// spaces in it leave nothing for the URL parser to make sense of.
const INVALID_URL = "not a url"

// A date which cannot exist: there is no thirteenth month and no month has a forty fifth day. The time
// input checks the month first, so the month is what it complains about.
const INVALID_DATE = "1975-13-45"
const INVALID_DATE_MESSAGE = "Months need to be between 0-12."

// What the form says about a value it could not make sense of and has nothing more precise to say about,
// and about a reference whose query was typed but never resolved into a document.
const INVALID_VALUE_MESSAGE = "Invalid value."
const UNFINISHED_VALUE_MESSAGE = "Unfinished value."

// The role a test signs in with: the one the site grants creating the worked-on class to. It is looked
// up rather than written out so that the tests follow the same table the site is configured by.
function roleWhichCreates(entityClass: EntityClass): Role {
  const role = ROLES.find((r) => ROLE_CREATES[r]?.includes(entityClass))
  if (role === undefined) {
    throw new Error(`no role may create ${entityClass}`)
  }
  return role
}

const ROLE = roleWhichCreates(RESEARCHER_CLASS)

// The parts of a view which do not look the same on every run, and which every checkpoint therefore
// masks. Next to the times the shared helper masks, a reference field whose candidates all fit is
// rendered as a list of options which comes from a search of the index, and the order in which a search
// returns the documents a filter alone matches is not stable while the suite is writing documents, so
// such a list is masked rather than compared.
function unstable(page: Page): Array<Locator> {
  return [...volatile(page), page.locator(".pd-claimrefselect-list")]
}

// The complaint the form shows on the given field. Every value slot renders the error of the input
// inside it, so a field which nothing is wrong with matches no element at all.
function fieldError(page: Page, propertyId: string): Locator {
  return field(page, propertyId).locator(".pd-inputfield-error")
}

// One bound of the interval field, which renders its two bounds as rows of its own.
function intervalBound(page: Page, propertyId: string, bound: "from" | "to"): Locator {
  return field(page, propertyId).locator(`.pd-fieldsformrow-field-${bound}`)
}

// The document as the server stores it, fetched from the API as text. An editing session keeps its
// changes to itself until the save goes through, so this is how a test tells what a refused save left
// behind. The body is searched as text rather than dug through as a claim tree, because that would make
// the test know the shape of every kind of claim it looks at.
async function storedDocument(page: Page, id: string): Promise<string> {
  const response = await fetchFromPage(page, `/api/d/${id}`)
  expect(response.status, `status of the stored document ${id}`).toBe(200)
  return response.body
}

// One stored claim of a document, picked by the property it is stated on and by the kind of claim it is,
// so that a test about what a save wrote can look at the claim itself rather than at the text of the
// whole document.
async function storedClaim(page: Page, id: string, kind: string, propertyId: string): Promise<Record<string, unknown>> {
  const document = JSON.parse(await storedDocument(page, id)) as { claims: Record<string, Array<Record<string, unknown>>> }
  const claims = (document.claims[kind] ?? []).filter((claim) => (claim.prop as { id: string } | undefined)?.id === propertyId)
  expect(claims.length, `the stored document ${id} states one ${kind} claim on ${propertyId}`).toBe(1)
  return claims[0]
}

// Presses save on the open editing session without waiting for the session to end, which is what a test
// of a save which is refused needs. Focus is moved onto the discard button next to save first, the way
// saveEdit does it, so that the value typed last is offered to the session before the save runs.
async function pressSave(page: Page): Promise<void> {
  const discardButton = page.locator("#documentedit-button-discard")
  await expect(discardButton, "discard button").toBeVisible()
  await discardButton.focus()
  const saveButton = page.locator("#documentedit-button-save")
  await expect(saveButton, "save button").toBeEnabled()
  await saveButton.click()
}

// Asserts that the save was refused and left the editing session as it was: the browser is still in the
// session it was in before the save, the form is still there, and nothing went wrong with the session
// itself, because a field which does not validate is not a failure of the session.
async function expectSaveRefused(page: Page, editUrl: string): Promise<void> {
  await expect(page.locator(".pd-documentedit"), "the editing session stays open").toBeVisible()
  await expect(page.locator(".pd-fieldsform"), "the form of the editing session stays open").toBeVisible()
  await expect(page.locator("#documentedit-error-session"), "the whole-form error of a refused save").toHaveCount(0)
  await expect(page.locator(".pd-documentget"), "the document view a save which went through would lead to").toHaveCount(0)
  expect(page.url(), "the refused save leaves the browser in the editing session").toBe(editUrl)
}

// Creates a researcher with the given name, and with a website and a year of birth when they are given,
// and returns the identifier of the created document. An empty string leaves the field alone. A name is
// all a new researcher needs to be saveable, because the family name declares a default and is therefore
// not required. Every test creates the researcher it then breaks, so no test ever puts an invalid value
// into a document another test reads.
//
// Nothing is checkpointed while the document is being created: the create view also lists the documents
// which look like duplicates of the one being made, and from the second run on that list holds the
// researcher the previous run created, so a screenshot of it would differ between runs.
async function createResearcher(page: Page, name: string, website: string, born: string): Promise<string> {
  await startCreate(page, RESEARCHER_CLASS)
  await hideDuplicates(page)

  const nameInput = fieldInput(page, PROPERTY_IDS.NAME, ".pd-inputstring")
  await expect(nameInput, "name input of the new researcher").toBeVisible({ timeout: LOADING_TIMEOUT })
  await nameInput.fill(name)
  await nameInput.blur()
  await expect(nameInput, "name input holds the entered name").toHaveValue(name)

  if (born !== "") {
    const bornInput = fieldInput(page, PROPERTY_IDS.BORN, ".pd-inputtime-input-time")
    await expect(bornInput, "year of birth input of the new researcher").toBeVisible()
    await bornInput.fill(born)
    await bornInput.blur()
    await expect(bornInput, "year of birth input holds the entered date").toHaveValue(born)
  }

  if (website !== "") {
    const websiteInput = fieldInput(page, PROPERTY_IDS.WEBSITE, ".pd-inputlink-input")
    await expect(websiteInput, "website input of the new researcher").toBeVisible()
    await websiteInput.fill(website)
    await websiteInput.blur()
    await expect(websiteInput, "website input holds the entered URL").toHaveValue(website)
  }

  await saveEdit(page)
  await expect(page.locator("#documentget-title"), "title of the created researcher").toContainText(name, { timeout: LOADING_TIMEOUT })

  return documentId(page)
}

// Opens an editing session on the document the browser is on and reports the address of that session, so
// that a test can assert a refused save left the browser in it.
async function editDocument(page: Page): Promise<string> {
  await startEdit(page)
  await hideDuplicates(page)
  return page.url()
}

test.describe("PeerDB Edit Validation Flows", () => {
  test("Test a save refused because a link field holds a value which is not a URL", async ({ context }) => {
    const page = await context.newPage()

    const name = `${NAME_PREFIX} Link Researcher`
    const website = "https://example.com/peerdb-validation-link"
    const corrected = "https://example.com/peerdb-validation-link-corrected"

    await signIn(page, [ROLE])
    // What the form says about an invalid value is asserted below, so the language it says it in is
    // pinned rather than left to whatever the browser asks for.
    await switchLanguage(page, "en")

    const id = await createResearcher(page, name, website, "")
    expect(await storedDocument(page, id), "the created researcher carries the website it was created with").toContain(website)

    const editUrl = await editDocument(page)

    const websiteInput = fieldInput(page, PROPERTY_IDS.WEBSITE, ".pd-inputlink-input")
    await expect(websiteInput, "website input holds the URL of the created researcher").toHaveValue(website)
    await websiteInput.fill(INVALID_URL)
    await expect(websiteInput, "website input holds the invalid link").toHaveValue(INVALID_URL)
    // Nothing is said while the value is being typed: the input complains once it is left, or once a
    // save asks every input of the form whether it is happy with what it holds.
    await expect(fieldError(page, PROPERTY_IDS.WEBSITE), "complaint about the website before the save").toHaveCount(0)
    await checkpointElement(page, field(page, PROPERTY_IDS.WEBSITE), "edit-validation-link-typed", unstable(page))

    await pressSave(page)

    await expect(fieldError(page, PROPERTY_IDS.WEBSITE), "complaint about the website after the refused save").toHaveText(INVALID_VALUE_MESSAGE)
    await expect(websiteInput, "website input is marked as invalid").toHaveAttribute("aria-invalid", "true")
    // The save puts the caret back into the input it stumbled over, so the user does not have to look
    // for it (focusFirstInvalid in DocumentEdit.onSave).
    await expect(websiteInput, "website input takes the focus").toBeFocused()
    await expect(websiteInput, "the invalid link stays in the form").toHaveValue(INVALID_URL)
    await expectSaveRefused(page, editUrl)
    await checkpoint(page, "edit-validation-link-refused", { mask: unstable(page) })

    // A refused save flushes nothing, so the stored document is exactly what it was.
    const refused = await storedDocument(page, id)
    expect(refused, "the refused save leaves the stored website alone").toContain(website)
    expect(refused, "the refused save stores nothing of the invalid link").not.toContain(INVALID_URL)

    // Correcting the value has to release the block: the complaint goes away as soon as the value parses
    // and the same save then goes through.
    await websiteInput.fill(corrected)
    await websiteInput.blur()
    await expect(websiteInput, "website input holds the corrected link").toHaveValue(corrected)
    await expect(fieldError(page, PROPERTY_IDS.WEBSITE), "complaint about the corrected website").toHaveCount(0)
    await expect(websiteInput, "website input is no longer marked as invalid").not.toHaveAttribute("aria-invalid", "true")
    await checkpointElement(page, field(page, PROPERTY_IDS.WEBSITE), "edit-validation-link-corrected", unstable(page))

    await saveEdit(page)
    await expect(page.locator(".pd-claimvaluelink").first(), "link shown on the saved researcher").toHaveText(corrected)
    await checkpoint(page, "edit-validation-link-saved", { mask: unstable(page) })

    const saved = await storedDocument(page, id)
    expect(saved, "the corrected link is stored").toContain(corrected)
    expect(saved, "the invalid link never reaches the store").not.toContain(INVALID_URL)

    console.log(`Successfully had 1 save of document ${id} refused for an invalid link and saved it once the link was corrected.`)
  })

  test("Test a save refused because a time field holds a date which cannot exist", async ({ context }) => {
    const page = await context.newPage()

    const name = `${NAME_PREFIX} Time Researcher`
    const born = "1975-03-14"
    const corrected = "1980-06-02"

    await signIn(page, [ROLE])
    await switchLanguage(page, "en")

    const id = await createResearcher(page, name, "", born)
    expect(await storedDocument(page, id), "the created researcher carries the date of birth it was created with").toContain(born)

    const editUrl = await editDocument(page)

    const bornInput = fieldInput(page, PROPERTY_IDS.BORN, ".pd-inputtime-input-time")
    await expect(bornInput, "year of birth input holds the date of the created researcher").toHaveValue(born)
    await bornInput.fill(INVALID_DATE)
    await expect(bornInput, "year of birth input holds the impossible date").toHaveValue(INVALID_DATE)
    await expect(fieldError(page, PROPERTY_IDS.BORN), "complaint about the year of birth before the save").toHaveCount(0)
    await checkpointElement(page, field(page, PROPERTY_IDS.BORN), "edit-validation-time-typed", unstable(page))

    await pressSave(page)

    // A time says what exactly is wrong with it rather than only that it is invalid, and the month is
    // what it checks first, so the day being impossible too is not what it reports.
    await expect(fieldError(page, PROPERTY_IDS.BORN), "complaint about the year of birth after the refused save").toHaveText(INVALID_DATE_MESSAGE)
    await expect(bornInput, "year of birth input is marked as invalid").toHaveAttribute("aria-invalid", "true")
    await expect(bornInput, "year of birth input takes the focus").toBeFocused()
    await expect(bornInput, "the impossible date stays in the form").toHaveValue(INVALID_DATE)
    await expectSaveRefused(page, editUrl)
    await checkpoint(page, "edit-validation-time-refused", { mask: unstable(page) })

    const refused = await storedDocument(page, id)
    expect(refused, "the refused save leaves the stored date of birth alone").toContain(born)
    expect(refused, "the refused save stores nothing of the impossible date").not.toContain(INVALID_DATE)

    await bornInput.fill(corrected)
    await bornInput.blur()
    await expect(bornInput, "year of birth input holds the corrected date").toHaveValue(corrected)
    await expect(fieldError(page, PROPERTY_IDS.BORN), "complaint about the corrected year of birth").toHaveCount(0)
    // A date is read for how precise it is, and a full date is precise to the day, which is what the
    // precision next to the input says once the value parses again.
    await expect(fieldInput(page, PROPERTY_IDS.BORN, ".pd-inputtime-precision"), "precision of the corrected year of birth").toHaveText("days")
    await checkpointElement(page, field(page, PROPERTY_IDS.BORN), "edit-validation-time-corrected", unstable(page))

    await saveEdit(page)
    await expect(page.locator(".pd-claimvaluetime").first(), "time shown on the saved researcher").toContainText("1980")
    await checkpoint(page, "edit-validation-time-saved", { mask: unstable(page) })

    const saved = await storedDocument(page, id)
    expect(saved, "the corrected date of birth is stored").toContain(corrected)
    expect(saved, "the impossible date never reaches the store").not.toContain(INVALID_DATE)

    console.log(`Successfully had 1 save of document ${id} refused for an impossible date and saved it once the date was corrected.`)
  })

  test("Test which field a save refused over two invalid values focuses", async ({ context }) => {
    // The document is created, saved twice against invalid values and saved once for real, which is more
    // than a test of the default length has time for.
    test.slow()

    const page = await context.newPage()

    const name = `${NAME_PREFIX} Order Researcher`
    const born = "1962-08-09"
    const correctedBorn = "1963-09-10"
    const website = "https://example.com/peerdb-validation-order"
    const correctedWebsite = "https://example.com/peerdb-validation-order-corrected"

    await signIn(page, [ROLE])
    await switchLanguage(page, "en")

    const id = await createResearcher(page, name, website, born)

    const editUrl = await editDocument(page)

    // Both fields are broken before anything is saved. Typing into the website leaves the year of birth,
    // which is what makes the date complain before the save is even pressed.
    const bornInput = fieldInput(page, PROPERTY_IDS.BORN, ".pd-inputtime-input-time")
    const websiteInput = fieldInput(page, PROPERTY_IDS.WEBSITE, ".pd-inputlink-input")
    await expect(bornInput, "year of birth input holds the date of the created researcher").toHaveValue(born)
    await bornInput.fill(INVALID_DATE)
    await expect(websiteInput, "website input holds the URL of the created researcher").toHaveValue(website)
    await websiteInput.fill(INVALID_URL)
    await expect(bornInput, "year of birth input holds the impossible date").toHaveValue(INVALID_DATE)
    await expect(websiteInput, "website input holds the invalid link").toHaveValue(INVALID_URL)
    await expect(fieldError(page, PROPERTY_IDS.BORN), "complaint about the year of birth which was left").toHaveText(INVALID_DATE_MESSAGE)
    await expect(fieldError(page, PROPERTY_IDS.WEBSITE), "complaint about the website which still has the focus").toHaveCount(0)

    await pressSave(page)

    // Both fields say what is wrong with them, and the caret goes to the first of them in the form,
    // which is the year of birth: the class puts it above the website.
    await expect(fieldError(page, PROPERTY_IDS.BORN), "complaint about the year of birth").toHaveText(INVALID_DATE_MESSAGE)
    await expect(fieldError(page, PROPERTY_IDS.WEBSITE), "complaint about the website").toHaveText(INVALID_VALUE_MESSAGE)
    await expect(bornInput, "the first invalid field takes the focus").toBeFocused()
    await expectSaveRefused(page, editUrl)
    await checkpoint(page, "edit-validation-order-both-refused", { mask: unstable(page) })

    const refusedBoth = await storedDocument(page, id)
    expect(refusedBoth, "the refused save leaves the stored date of birth alone").toContain(born)
    expect(refusedBoth, "the refused save leaves the stored website alone").toContain(website)

    // Correcting the first of the two moves the block to the second: the save is refused again, and now
    // it is the website which takes the focus.
    await bornInput.fill(correctedBorn)
    await pressSave(page)

    await expect(fieldError(page, PROPERTY_IDS.BORN), "complaint about the corrected year of birth").toHaveCount(0)
    await expect(fieldError(page, PROPERTY_IDS.WEBSITE), "complaint about the website which is still invalid").toHaveText(INVALID_VALUE_MESSAGE)
    await expect(websiteInput, "the field which is still invalid takes the focus").toBeFocused()
    await expectSaveRefused(page, editUrl)
    await checkpointElement(page, field(page, PROPERTY_IDS.WEBSITE), "edit-validation-order-second-refused", unstable(page))

    // The corrected date of birth was committed into the session by the save which was refused over the
    // website, but a refused save flushes nothing, so the stored document still holds the old date.
    expect(await storedDocument(page, id), "the second refused save stores nothing either").toContain(born)

    await websiteInput.fill(correctedWebsite)
    await websiteInput.blur()
    await expect(fieldError(page, PROPERTY_IDS.WEBSITE), "complaint about the corrected website").toHaveCount(0)

    await saveEdit(page)
    await expect(page.locator(".pd-claimvaluelink").first(), "link shown on the saved researcher").toHaveText(correctedWebsite)
    await checkpoint(page, "edit-validation-order-saved", { mask: unstable(page) })

    const saved = await storedDocument(page, id)
    expect(saved, "the corrected date of birth is stored").toContain(correctedBorn)
    expect(saved, "the corrected website is stored").toContain(correctedWebsite)
    expect(saved, "the impossible date never reaches the store").not.toContain(INVALID_DATE)
    expect(saved, "the invalid link never reaches the store").not.toContain(INVALID_URL)

    console.log(`Successfully had 2 saves of document ${id} refused, each focusing the first of its invalid fields, and saved it once both were corrected.`)
  })

  test("Test a save refused because a reference field holds a query which was never picked", async ({ context }) => {
    const page = await context.newPage()

    const name = `${NAME_PREFIX} Reference Researcher`
    const website = "https://example.com/peerdb-validation-reference"
    // A query which matches a research method of the test data, so that the input has something to offer
    // and the test is about the user not picking rather than about there being nothing to pick.
    const query = "photog"

    await signIn(page, [ROLE])
    await switchLanguage(page, "en")

    const id = await createResearcher(page, name, "", "")
    const editUrl = await editDocument(page)

    // Another field is filled in first, because a query typed into a reference input is no change of the
    // document until a document is picked, and a session with nothing in it has nothing to save. What the
    // save then does with this change is the other half of the test: it is refused over the reference and
    // stores neither of the two.
    const websiteInput = fieldInput(page, PROPERTY_IDS.WEBSITE, ".pd-inputlink-input")
    await expect(websiteInput, "website input of the researcher being edited").toBeVisible()
    await websiteInput.fill(website)
    await websiteInput.blur()
    await expect(websiteInput, "website input holds the entered URL").toHaveValue(website)

    // The field is offered as a combobox rather than as a list of options because the class allows more
    // methods than the form is willing to list at once, which is what makes a query typed into it
    // possible in the first place.
    const specialisation = field(page, PROPERTY_IDS.SPECIALISES_IN)
    const referenceInput = specialisation.locator(".pd-inputref-input").first()
    await expect(referenceInput, "reference input of the specialisation field").toBeVisible()
    await referenceInput.fill(query)
    await expect(specialisation.locator(".pd-inputref-item").first(), "the results the typed query offers").toBeVisible({ timeout: LOADING_TIMEOUT })
    // Leaving the input without picking anything says nothing yet: what was typed may still become a
    // reference, so the complaint waits for the save.
    await referenceInput.blur()
    await expect(fieldError(page, PROPERTY_IDS.SPECIALISES_IN), "complaint about the unfinished reference before the save").toHaveCount(0)

    await pressSave(page)

    await expect(fieldError(page, PROPERTY_IDS.SPECIALISES_IN), "complaint about the reference which was never picked").toHaveText(UNFINISHED_VALUE_MESSAGE)
    await expect(referenceInput, "reference input is marked as invalid").toHaveAttribute("aria-invalid", "true")
    await expect(referenceInput, "reference input takes the focus").toBeFocused()
    await expect(referenceInput, "the typed query stays in the form").toHaveValue(query)
    await expectSaveRefused(page, editUrl)
    await checkpointElement(page, specialisation, "edit-validation-reference-refused", unstable(page))

    // The refused save flushes nothing at all, the change which is not complained about included.
    expect(await storedDocument(page, id), "the refused save stores neither the reference nor the website next to it").not.toContain(website)

    // Editing the query is enough to clear the complaint: what is typed now may resolve into a document,
    // so the form stops saying it will not.
    await referenceInput.fill(`${query}r`)
    await expect(fieldError(page, PROPERTY_IDS.SPECIALISES_IN), "complaint about the reference once the query is edited").toHaveCount(0)
    await expect(referenceInput, "reference input is no longer marked as invalid").not.toHaveAttribute("aria-invalid", "true")

    // Picking one of the offered documents is what the field asks for, and the save then goes through.
    const picked = specialisation.locator(".pd-inputref-item").first()
    await expect(picked, "the results the edited query offers").toBeVisible({ timeout: LOADING_TIMEOUT })
    await picked.click()
    await expect(specialisation.locator(".pd-inputref-value").first(), "the picked reference").toBeVisible()
    await checkpointElement(page, specialisation, "edit-validation-reference-picked", unstable(page))

    await saveEdit(page)
    await expect(page.locator(".pd-claimvalueref").first(), "reference shown on the saved researcher").toBeVisible()
    const saved = await storedDocument(page, id)
    expect(saved, "the saved researcher states what it specialises in").toContain(PROPERTY_IDS.SPECIALISES_IN)
    expect(saved, "the change which was held back by the refused save is stored with it").toContain(website)
    expect(saved, "nothing of the typed query is stored as a value of its own").not.toContain(`"${query}"`)

    console.log(`Successfully had 1 save of document ${id} refused for a reference query which was never picked, and saved it once a document was picked.`)
  })

  test("Test an interval bound which is given a value keeps no mark saying it is missing", async ({ context }) => {
    // The document is created and then saved three more times, once per state the interval goes through,
    // which is more than a test of the default length has time for.
    test.slow()

    const page = await context.newPage()

    const name = `${NAME_PREFIX} Interval Researcher`
    const from = "1990-03-04"
    const to = "2010-05-06"
    const laterFrom = "1985-01-02"

    await signIn(page, [ROLE])
    await switchLanguage(page, "en")

    const id = await createResearcher(page, name, "", "")
    await editDocument(page)

    const period = field(page, PROPERTY_IDS.ACTIVE_PERIOD)
    const fromBound = intervalBound(page, PROPERTY_IDS.ACTIVE_PERIOD, "from")
    const toBound = intervalBound(page, PROPERTY_IDS.ACTIVE_PERIOD, "to")
    const fromInput = fromBound.locator(".pd-inputtime-input-time")
    const toInput = toBound.locator(".pd-inputtime-input-time")

    // An interval is one claim about two bounds, so a bound which is not given a value has to say what it
    // is instead: giving the lower bound a value marks the upper one as unknown by itself.
    await expect(fromInput, "the lower bound of the untouched period").toHaveValue("")
    await fromInput.fill(from)
    await fromInput.blur()
    await expect(toBound.locator(".pd-inputmissing-checkbox-unknown"), "the upper bound is marked as unknown once the lower one has a value").toBeChecked({
      timeout: LOADING_TIMEOUT,
    })
    await expect(toInput, "the upper bound cannot be typed into while it is marked as unknown").toHaveAttribute("readonly", "")
    await checkpointElement(page, period, "edit-validation-interval-half", unstable(page))

    // Taking the mark off is what makes room for a value, and the two of them must not end up on the
    // claim together.
    await toBound.locator(".pd-inputmissing-checkbox-unknown").uncheck()
    await expect(toInput, "the upper bound can be typed into once the mark is off").not.toHaveAttribute("readonly", "")
    await toInput.fill(to)
    await toInput.blur()
    await expect(toInput, "the upper bound holds the entered date").toHaveValue(to)
    await checkpointElement(page, period, "edit-validation-interval-both", unstable(page))

    await saveEdit(page)
    const both = await storedClaim(page, id, "timeInterval", PROPERTY_IDS.ACTIVE_PERIOD)
    expect(both.from, "the lower bound of the stored period").toBe(from)
    expect(both.to, "the upper bound of the stored period").toBe(to)
    expect("toIsUnknown" in both, "the stored period keeps no unknown mark on the bound which was given a value").toBe(false)
    await checkpoint(page, "edit-validation-interval-saved", { mask: unstable(page) })

    // The other way round: a bound which is marked as having no value at all stores the mark and no value.
    await editDocument(page)
    await intervalBound(page, PROPERTY_IDS.ACTIVE_PERIOD, "from").locator(".pd-inputmissing-checkbox-none").check()
    await expect(intervalBound(page, PROPERTY_IDS.ACTIVE_PERIOD, "from").locator(".pd-inputtime-input-time"), "the marked lower bound holds no value").toHaveValue("")
    await saveEdit(page)
    const marked = await storedClaim(page, id, "timeInterval", PROPERTY_IDS.ACTIVE_PERIOD)
    expect(marked.fromIsNone, "the stored period says its lower bound has no value").toBe(true)
    expect("from" in marked, "the stored period keeps no value on the bound which was marked").toBe(false)
    expect(marked.to, "the upper bound of the stored period is untouched by the mark on the lower one").toBe(to)

    // Giving that bound a value again has to take the mark off the stored claim, and not leave the claim
    // saying both that the bound has no value and what the value is.
    await editDocument(page)
    const markedFrom = intervalBound(page, PROPERTY_IDS.ACTIVE_PERIOD, "from")
    await expect(markedFrom.locator(".pd-inputmissing-checkbox-none"), "the mark of the lower bound is read back into the form").toBeChecked()
    await markedFrom.locator(".pd-inputmissing-checkbox-none").uncheck()
    await markedFrom.locator(".pd-inputtime-input-time").fill(laterFrom)
    await markedFrom.locator(".pd-inputtime-input-time").blur()
    await settleEdit(page)
    await checkpointElement(page, field(page, PROPERTY_IDS.ACTIVE_PERIOD), "edit-validation-interval-unmarked", unstable(page))
    await saveEdit(page)

    const unmarked = await storedClaim(page, id, "timeInterval", PROPERTY_IDS.ACTIVE_PERIOD)
    expect(unmarked.from, "the lower bound of the stored period after it was given a value again").toBe(laterFrom)
    expect("fromIsNone" in unmarked, "the stored period keeps no mark on the bound which was given a value again").toBe(false)
    expect(unmarked.to, "the upper bound of the stored period is still what it was").toBe(to)

    console.log(`Successfully drove the period of document ${id} through 3 saves, each storing either a value or a mark on a bound and never both.`)
  })
})
