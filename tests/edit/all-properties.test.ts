import type { Locator, Page } from "@playwright/test"

import type { DocumentClass, Role } from "../peerdb_utils"

import { copyFileSync, mkdtempSync, statSync } from "node:fs"
import { tmpdir } from "node:os"
import { dirname, join } from "node:path"
import { fileURLToPath } from "node:url"

import { CLASS_IDS, createNamed, PROPERTY_IDS, ROLE_CREATES } from "../peerdb_utils"
import {
  checkpoint,
  checkpointElement,
  expect,
  expectNothingLoading,
  fetchFromPage,
  hideDuplicates,
  LOADING_TIMEOUT,
  openDocumentTab,
  pickReference,
  propertyRow,
  saveEdit,
  settle,
  signIn,
  startEdit,
  test,
} from "../utils"

// The class every document of this file is created as, and the role which may start one (ROLE_CREATES in
// peerdb_utils says which). None of the properties the claims below are stated on is a field of this class,
// which is what the claim editor of the "all properties" tab is for: it is the only way to state a property
// the class declares no field for.
const SUBJECT_CLASS: DocumentClass = "ARTIFACT"
const CREATOR_ROLE: Role = "curator"

// What every document this file creates is named by, so that its documents never collide with the ones
// another test file makes and a document left behind says which file made it.
const NAME_PREFIX = "AllProps"

// The values the claims are stated with. They are written here rather than at the point of use so that what
// is typed into the editor and what the saved document is expected to show are the same string.
const ID_VALUE = "XA-1987-ALLPROPS"
const STRING_VALUE = "AllProps gloss of the name"
const EDITED_STRING_VALUE = "AllProps gloss after the edit"
const HTML_VALUE = "AllProps source note"
const AMOUNT_VALUE = "12"
const AMOUNT_PRECISION = "1"
const AMOUNT_FROM = "3"
const AMOUNT_TO = "9"
const TIME_VALUE = "2317-04-05"
const TIME_FROM = "2301-01-01"
const TIME_TO = "2317-12-31"
const LINK_VALUE = "https://registry.ccx.example/entry/allprops"

// The register an identifier of the property the identifier claim is stated on links into, which is the
// start of the link template that property declares (CATALOGUE_CODE in the test data schema). Only the
// register is asserted here, because what the template makes of the identifier itself is what the file
// looking at the claim values of the test data is about, while this one is about the editor which made the
// claim.
const ID_LINK = /^https:\/\/registry\.ccx\.example\/entry\//

// The document a reference claim is made to. A reference is picked by searching for it, and the mnemonic a
// class is declared under is the same query in every interface language, unlike the class's label, so the
// class document of the class being created is what this file points at.
const REF_TARGET_CLASS: DocumentClass = SUBJECT_CLASS

// The attachment the file tab uploads, named by its path inside the test data files directory, and the
// directory itself, derived from this file's own location rather than from the working directory, which the
// test process does not control.
const UPLOAD_FILE = "records/G4_AR_FIRST_HEAT_BOWL.txt"
const FILES_DIR = join(dirname(fileURLToPath(import.meta.url)), "..", "..", "testdata", "files")

// The mnemonics of the properties of the test data schema, which is how a property is named here: a mnemonic
// is both the key the property's identifier is looked up under and the query the property is searched for by
// in the editor's property field, and it is the same in every interface language.
type PropertyMnemonic = keyof typeof PROPERTY_IDS

// Copies the attachment into a temporary directory of its own and returns the path of the copy, which is what
// gets uploaded: the repository's own copy is what other tests compare against, so it is never handed to the
// browser.
function tempCopy(path: string): string {
  const segments = path.split("/")
  const copy = join(mkdtempSync(join(tmpdir(), "peerdb-allprops-")), segments[segments.length - 1])
  copyFileSync(join(FILES_DIR, ...segments), copy)
  return copy
}

// One claim type the editor offers, with everything driving its panel and checking what it saved takes.
interface ClaimTypeCase {
  // The suffix of the tab and of the panel of this claim type, which is the claim type lowercased.
  tab: string
  // The property the claim is stated on. Every case uses one of its own, so a document ends up carrying
  // exactly one claim of the property its test is about.
  property: PropertyMnemonic
  // The claim type the saved document holds the claim under. The file tab uploads a file and states the
  // address it was stored under, so it is the one case whose claim is stored as something else than the tab.
  stored: string
  // Fills the value fields of the panel. The types which state a property and no value at all have none.
  fill?: (page: Page, panel: Locator) => Promise<void>
  // Asserts what the row of the "all properties" tab renders for the claim once it is saved.
  shows: (page: Page, row: Locator) => Promise<void>
  // What the checkpoints of this claim type have to mask: the elements showing something the run made up
  // rather than something the test typed, which would otherwise make a screenshot differ between runs.
  volatile?: (page: Page) => Array<Locator>
}

// Types a value into one of the value inputs of the claim editor and commits it the way a user leaving the
// field does, so that the claim form holds it before the claim is added.
async function fillValue(input: Locator, value: string, what: string): Promise<void> {
  await expect(input, what).toBeVisible({ timeout: LOADING_TIMEOUT })
  await input.fill(value)
  await input.blur()
  await expect(input, `${what} after typing`).toHaveValue(value)
}

// Every claim type the editor offers, in the order it offers them. A type added to the editor without a case
// here fails the test which counts the tabs, so the list cannot fall behind the editor.
const CLAIM_TYPE_CASES: ReadonlyArray<ClaimTypeCase> = [
  {
    tab: "id",
    property: "CATALOGUE_CODE",
    stored: "id",
    fill: async (page, panel) => {
      await fillValue(panel.locator(".pd-inputidentifier"), ID_VALUE, "the identifier value")
    },
    shows: async (page, row) => {
      const value = row.locator(".pd-claimvalueid")
      await expect(value, "the identifier value").toHaveText(ID_VALUE)
      // The property declares a link template, so the identifier renders as a link into the register that
      // template names rather than as bare text.
      await expect(value, "the register the identifier value links into").toHaveAttribute("href", ID_LINK)
    },
  },
  {
    tab: "string",
    property: "GLOSS",
    stored: "string",
    fill: async (page, panel) => {
      await fillValue(panel.locator(".pd-inputstring"), STRING_VALUE, "the string value")
    },
    shows: async (page, row) => {
      await expect(row.locator(".pd-claimvaluestring"), "the string value").toHaveText(STRING_VALUE)
    },
  },
  {
    tab: "html",
    property: "SOURCE",
    stored: "html",
    fill: async (page, panel) => {
      // The rich text editor holds its value in the element ProseMirror makes editable, which is what is
      // typed into: the mount point around it is not focusable.
      const editor = panel.locator('.pd-inputhtml-editor [contenteditable="true"]').first()
      await expect(editor, "the rich text editor").toBeVisible({ timeout: LOADING_TIMEOUT })
      await editor.click()
      await page.keyboard.type(HTML_VALUE)
      // A key press is handled by the editor rather than by the input itself, so what it did is not in the
      // document the moment the press returns.
      await expect.poll(async () => await editor.innerHTML(), { message: "the typed text becomes a paragraph" }).toContain(`<p>${HTML_VALUE}</p>`)
      await editor.blur()
    },
    shows: async (page, row) => {
      const value = row.locator(".pd-claimvaluehtml")
      await expect(value, "the HTML value").toHaveText(HTML_VALUE)
      expect(await value.innerHTML(), "the markup of the HTML value").toContain(`<p>${HTML_VALUE}</p>`)
    },
  },
  {
    tab: "amount",
    property: "STAFF_COUNT",
    stored: "amount",
    fill: async (page, panel) => {
      await fillValue(panel.locator(".pd-inputamount-input-amount"), AMOUNT_VALUE, "the amount value")
      await fillValue(panel.locator(".pd-inputamount-input-precision"), AMOUNT_PRECISION, "the precision of the amount value")
    },
    shows: async (page, row) => {
      await expect(row.locator(".pd-claimvalueamount"), "the amount value").toHaveText(AMOUNT_VALUE)
    },
  },
  {
    tab: "amountinterval",
    property: "MEMBER_COUNT",
    stored: "amountInterval",
    fill: async (page, panel) => {
      const from = panel.locator(".pd-documentedit-field-from")
      const to = panel.locator(".pd-documentedit-field-to")
      await fillValue(from.locator(".pd-inputamount-input-amount"), AMOUNT_FROM, "the lower bound of the amount interval")
      await fillValue(from.locator(".pd-inputamount-input-precision"), AMOUNT_PRECISION, "the precision of the lower bound")
      await fillValue(to.locator(".pd-inputamount-input-amount"), AMOUNT_TO, "the upper bound of the amount interval")
      await fillValue(to.locator(".pd-inputamount-input-precision"), AMOUNT_PRECISION, "the precision of the upper bound")
    },
    shows: async (page, row) => {
      await expect(row.locator(".pd-claimvalueamountinterval-from"), "the lower bound of the amount interval").toHaveText(AMOUNT_FROM)
      await expect(row.locator(".pd-claimvalueamountinterval-to"), "the upper bound of the amount interval").toHaveText(AMOUNT_TO)
    },
  },
  {
    tab: "time",
    property: "FIRST_DOCUMENTED",
    stored: "time",
    fill: async (page, panel) => {
      // The precision is inferred from how much of the timestamp was typed, so a date leaves the field at
      // day precision without the precision list being opened.
      await fillValue(panel.locator(".pd-inputtime-input-time"), TIME_VALUE, "the time value")
    },
    shows: async (page, row) => {
      await expect(row.locator(".pd-claimvaluetime"), "the time value").toContainText(TIME_VALUE)
    },
  },
  {
    tab: "timeinterval",
    property: "PERIOD",
    stored: "timeInterval",
    fill: async (page, panel) => {
      await fillValue(panel.locator(".pd-documentedit-field-from .pd-inputtime-input-time"), TIME_FROM, "the start of the time interval")
      await fillValue(panel.locator(".pd-documentedit-field-to .pd-inputtime-input-time"), TIME_TO, "the end of the time interval")
    },
    shows: async (page, row) => {
      await expect(row.locator(".pd-claimvaluetimeinterval-from"), "the start of the time interval").toContainText(TIME_FROM)
      await expect(row.locator(".pd-claimvaluetimeinterval-to"), "the end of the time interval").toContainText(TIME_TO)
    },
  },
  {
    tab: "link",
    property: "WEBSITE",
    stored: "link",
    fill: async (page, panel) => {
      await fillValue(panel.locator(".pd-inputlink-input"), LINK_VALUE, "the link value")
    },
    shows: async (page, row) => {
      const value = row.locator(".pd-claimvaluelink")
      await expect(value, "the address the link value points at").toHaveAttribute("href", LINK_VALUE)
      await expect(value, "the link value is external").toHaveClass(/pd-link-external/)
    },
  },
  {
    tab: "file",
    property: "HAS_REPORT",
    stored: "link",
    fill: async (page, panel) => {
      // The hidden native input behind the browse button is what the file is handed to, because a file
      // picker opened by a click cannot be answered from the page.
      await expect(panel.locator(".pd-inputfile-button-browse"), "the browse button of the empty file field").toBeVisible()
      await panel.locator(".pd-inputfile-input").setInputFiles(tempCopy(UPLOAD_FILE))
      await expect(panel.locator(".pd-inputfile-value"), "the uploaded file").toBeVisible({ timeout: LOADING_TIMEOUT })
      await expect(panel.locator(".pd-inputfile-error"), "an error while uploading the file").toHaveCount(0)
    },
    shows: async (page, row) => {
      // An uploaded file is stated as a link to the address the file was stored under, so it renders as a
      // link like any other, marked as a link to a file of this instance.
      const value = row.locator(".pd-claimvaluelink")
      await expect(value, "the file value is a link to a file").toHaveClass(/pd-link-file/)
      const href = await value.getAttribute("href")
      expect(href, "the address the file value points at").toMatch(/^\/f\/[0-9A-Za-z]+$/)
      // The file is served whole from the address the claim points at, so the upload is reachable from the
      // saved document and not only referenced by it.
      const response = await fetchFromPage(page, href!)
      expect(response.status, "the status the uploaded file is served with").toBe(200)
      expect(response.length, "the number of bytes of the uploaded file").toBe(statSync(join(FILES_DIR, ...UPLOAD_FILE.split("/"))).size)
    },
    // A file is stored under a fresh identifier every time one is uploaded, and the address it is served
    // from is what the form and the saved claim both show, so the two boxes holding it are masked. The
    // boxes are masked rather than the addresses themselves, because a box is as wide as the layout makes
    // it while a link is only as wide as the address it holds, so a mask over the link would move with the
    // address it is there to hide.
    volatile: (page) => [page.locator(".pd-inputfile-value"), page.locator(`.pd-propertiesview-row-${PROPERTY_IDS.HAS_REPORT} .pd-propertiesview-value`)],
  },
  {
    tab: "ref",
    property: "ABOUT",
    stored: "ref",
    fill: async (page, panel) => {
      await pickReference(page, panel.locator(".pd-documentedit-field-value"), REF_TARGET_CLASS, CLASS_IDS[REF_TARGET_CLASS], "the referenced document")
    },
    shows: async (page, row) => {
      const value = row.locator(".pd-claimvalueref")
      await expect(value, "the referenced document").toBeVisible()
      await expect(value, "the document the reference value leads to").toHaveAttribute("href", `/d/${CLASS_IDS[REF_TARGET_CLASS]}`)
    },
  },
  {
    tab: "has",
    property: "HAS_NOTATION_SYSTEM",
    stored: "has",
    shows: async (page, row) => {
      // A claim which states a property and no value renders as its property alone: the row carries the
      // label and no value cell at all, which is what tells it apart from none and unknown, both of which
      // render a value saying so.
      await expect(row.locator(".pd-propertiesview-label"), "the label of the property stated without a value").toBeVisible()
      await expect(row.locator(".pd-propertiesview-value"), "a value next to the property stated without one").toHaveCount(0)
    },
  },
  {
    tab: "none",
    property: "PERIODICITY",
    stored: "none",
    shows: async (page, row) => {
      await expect(row.locator(".pd-claimvaluenone"), "the value saying there is none").toBeVisible()
    },
  },
  {
    tab: "unknown",
    property: "FIRST_CONTACT",
    stored: "unknown",
    shows: async (page, row) => {
      await expect(row.locator(".pd-claimvalueunknown"), "the value saying it is unknown").toBeVisible()
    },
  },
]

// Opens the "all properties" tab of the edit view, which is where the claim editor lives, and waits until
// the claims already on the document have resolved.
async function openClaimEditor(page: Page): Promise<void> {
  const tab = page.locator(".pd-documentedit-tab-allproperties")
  await expect(tab, "the all properties tab of the edit view").toBeVisible()
  await tab.click()
  await expect(page.locator(".pd-documentedit-panel-allproperties"), "the all properties panel of the edit view").toBeVisible()
  await expect(page.locator("#documentedit-form-claim"), "the claim form").toBeVisible()
  await expectNothingLoading(page)
}

// Selects one of the claim type tabs of the claim editor and returns the panel it opened, which is what the
// property and the value of the claim are filled in.
async function selectClaimType(page: Page, type: string): Promise<Locator> {
  const tab = page.locator(`.pd-documentedit-tab-claimtype-${type}`)
  await expect(tab, `the ${type} claim type tab`).toBeVisible()
  await tab.click()
  const panel = page.locator(`.pd-documentedit-panel-claimtype-${type}`)
  await expect(panel, `the ${type} claim type panel`).toBeVisible()
  return panel
}

// Picks the property the claim is stated on, by searching for the mnemonic the property is declared under.
async function pickProperty(page: Page, panel: Locator, property: PropertyMnemonic): Promise<void> {
  await pickReference(page, panel.locator(".pd-documentedit-field-property"), property, PROPERTY_IDS[property], `the ${property} property`)
}

// The rows of the "all properties" tab of the edit view which are for the given property. The editable table
// gives every claim a row of its own, and the shared helper for the same table is scoped to the document
// view, so the edit view needs one of its own.
function editorRow(page: Page, propertyId: string): Locator {
  return page.locator(`.pd-documentedit-panel-allproperties .pd-propertiesview-row-${propertyId}`)
}

// Submits the claim form and waits until the document being edited carries the claim it describes. The form
// empties itself once the claim is in, which is what says the submit went through rather than being refused.
async function addClaim(page: Page, propertyId: string, rows: number, what: string): Promise<void> {
  const add = page.locator("#documentedit-button-addclaim")
  await expect(add, `the add button for ${what}`).toBeEnabled({ timeout: LOADING_TIMEOUT })
  await add.click()
  await expect(editorRow(page, propertyId), `the rows of ${what} in the editor`).toHaveCount(rows, { timeout: LOADING_TIMEOUT })
  await expect(page.locator("#documentedit-error-claim"), `an error while adding ${what}`).toHaveCount(0)
}

// Asserts that the saved document holds exactly one claim of the given property, under the claim type the
// editor was asked for. Several claim types render into the same element (an uploaded file and a web address
// are both links) and one of them renders no value at all, so what the page shows does not say them apart on
// its own.
async function expectStoredClaim(page: Page, id: string, stored: string, propertyId: string, what: string): Promise<void> {
  const response = await fetchFromPage(page, `/api/d/${id}`)
  expect(response.status, `the status the document holding ${what} is served with`).toBe(200)
  const doc = JSON.parse(response.body) as { claims: Record<string, Array<{ prop: { id: string } }>> }
  const claims = (doc.claims[stored] ?? []).filter((claim) => claim.prop.id === propertyId)
  expect(claims.length, `the ${stored} claims of ${what} the saved document holds`).toBe(1)
}

// Creates a document of the class this file drives and opens the claim editor on it. A test never edits a
// document of the test data, so that running the suite twice leaves the data set in the same shape.
async function createAndEdit(page: Page, name: string): Promise<string> {
  await signIn(page, [CREATOR_ROLE])
  const id = await createNamed(page, SUBJECT_CLASS, `${NAME_PREFIX} ${name}`)
  await startEdit(page)
  // An edit session shows no panel of potential duplicates, which only a create session has, but the style
  // rule for a screenshot of a form asks for it to be hidden wherever a form is captured.
  await hideDuplicates(page)
  await openClaimEditor(page)
  return id
}

// Adds one claim through the editor, from picking the claim type to the claim being part of the document.
async function stateClaim(page: Page, claimCase: ClaimTypeCase, rows: number): Promise<void> {
  const panel = await selectClaimType(page, claimCase.tab)
  await pickProperty(page, panel, claimCase.property)
  if (claimCase.fill) {
    await claimCase.fill(page, panel)
  }
  await addClaim(page, PROPERTY_IDS[claimCase.property], rows, `the ${claimCase.tab} claim`)
}

// Opens the "all properties" tab of the saved document, which is where the claim the editor added is read
// back from.
async function openSavedProperties(page: Page): Promise<void> {
  await openDocumentTab(page, "allproperties")
  await settle(page)
}

test.describe("PeerDB All Properties Editor Flows", () => {
  test("Test the claim editor of the all properties tab offers every claim type", async ({ context }) => {
    const page = await context.newPage()

    // Creating is granted per class, so the role signed in with below is the one the site lets start a
    // document of the class this file drives.
    expect(ROLE_CREATES[CREATOR_ROLE], "the role the documents are created with may create the class").toContain(SUBJECT_CLASS)

    await createAndEdit(page, "claim editor tabs")

    // Every claim type the editor offers has a case in this file, so a type added to the editor without one
    // is caught here rather than going untested.
    const tabs = page.locator(".pd-documentedit-tab-claimtype")
    await expect(tabs, "the claim type tabs of the editor").toHaveCount(CLAIM_TYPE_CASES.length)
    for (const claimCase of CLAIM_TYPE_CASES) {
      await expect(page.locator(`.pd-documentedit-tab-claimtype-${claimCase.tab}`), `the ${claimCase.tab} claim type tab`).toHaveCount(1)
    }

    // A claim which is being added is of no type yet, so every type is open to be picked and the editor
    // starts on the first of them.
    await expect(page.locator(".pd-documentedit-tab-claimtype:disabled"), "the claim type tabs which are locked while a claim is added").toHaveCount(0)
    await expect(page.locator(`.pd-documentedit-panel-claimtype-${CLAIM_TYPE_CASES[0].tab}`), "the panel the editor opens on").toBeVisible()

    // The form of a claim which has nothing filled in yet has nothing to add and nothing to cancel.
    await expect(page.locator("#documentedit-title-claim"), "the heading of the claim form").toBeVisible()
    await expect(page.locator("#documentedit-button-addclaim"), "the add button of an empty claim form").toBeDisabled()
    await expect(page.locator("#documentedit-button-cancelclaim"), "the cancel button of an empty claim form").toBeDisabled()

    // The document is shown next to the form, so what the editor adds to lands right above what adds to it.
    await expect(editorRow(page, PROPERTY_IDS.INSTANCE_OF), "the class the document is an instance of").toHaveCount(1)
    await expect(editorRow(page, PROPERTY_IDS.NAME), "the name of the document").toHaveCount(1)

    await checkpoint(page, "allpropsedit-editor")

    console.log(`Successfully verified that the claim editor of the all properties tab offers all ${CLAIM_TYPE_CASES.length} claim types, none of them locked.`)
  })

  for (const claimCase of CLAIM_TYPE_CASES) {
    test(`Test adding a claim of the ${claimCase.tab} type through the all properties editor`, async ({ context }) => {
      const page = await context.newPage()
      const propertyId = PROPERTY_IDS[claimCase.property]

      const id = await createAndEdit(page, `${claimCase.tab} claim`)

      // The property is one the class declares no field for, so the claim editor is the only way to state
      // it: the class tab of the form has no row for it.
      await expect(page.locator(`.pd-fieldsformfield-${propertyId}`), `a field of the form for the ${claimCase.property} property`).toHaveCount(0)

      const panel = await selectClaimType(page, claimCase.tab)
      await pickProperty(page, panel, claimCase.property)
      if (claimCase.fill) {
        await claimCase.fill(page, panel)
      }
      const masks = claimCase.volatile ? claimCase.volatile(page) : []
      await checkpointElement(page, page.locator("#documentedit-form-claim"), `allpropsedit-form-${claimCase.tab}`, masks)

      await addClaim(page, propertyId, 1, `the ${claimCase.tab} claim`)
      // Adding a claim empties the form for the next one, so there is nothing left to add.
      await expect(page.locator("#documentedit-button-addclaim"), "the add button after the claim was added").toBeDisabled()

      const savedId = await saveEdit(page)
      expect(savedId, "the document the claim was added to").toBe(id)

      await openSavedProperties(page)
      const row = propertyRow(page, propertyId)
      await expect(row, `the row of the ${claimCase.tab} claim on the saved document`).toHaveCount(1)
      await claimCase.shows(page, row)
      await expectStoredClaim(page, id, claimCase.stored, propertyId, `the ${claimCase.tab} claim`)

      await checkpointElement(page, page.locator(".pd-documentget-panel-allproperties .pd-propertiesview").first(), `allpropsedit-saved-${claimCase.tab}`, masks)

      console.log(
        `Successfully verified that a claim of the ${claimCase.tab} type added through the all properties editor is stored as 1 ${claimCase.stored} claim and renders as that type.`,
      )
    })
  }

  test("Test editing a claim through the all properties editor", async ({ context }) => {
    const page = await context.newPage()
    // A string claim is edited because its value is one field, so what the form was populated with and what
    // was typed over it are both a single value to compare.
    const edited = CLAIM_TYPE_CASES.find((claimCase) => claimCase.tab === "string")!
    const propertyId = PROPERTY_IDS[edited.property]

    const id = await createAndEdit(page, "edited claim")
    await stateClaim(page, edited, 1)
    await saveEdit(page)

    // The claim is edited on a document which already carries it, and not in the session which added it, so
    // the editor is driven the way a reader who comes back to a document drives it.
    await openSavedProperties(page)
    await expect(propertyRow(page, propertyId).locator(".pd-claimvaluestring"), "the value the claim was saved with").toHaveText(STRING_VALUE)

    await startEdit(page)
    await hideDuplicates(page)
    await openClaimEditor(page)
    const editButton = editorRow(page, propertyId).locator(".pd-propertiesview-button-edit")
    await expect(editButton, "the edit button of the claim").toBeVisible()
    await editButton.click()

    // Editing a claim populates the form with it and locks the claim type: a claim keeps the type it was
    // made with, so only the tab of that type stays open.
    const panel = page.locator(`.pd-documentedit-panel-claimtype-${edited.tab}`)
    await expect(panel, `the ${edited.tab} claim type panel the claim opened`).toBeVisible()
    await expect(page.locator(".pd-documentedit-tab-claimtype:disabled"), "the claim type tabs which are locked while a claim is edited").toHaveCount(
      CLAIM_TYPE_CASES.length - 1,
    )
    await expect(panel.locator(".pd-inputstring"), "the value the form was populated with").toHaveValue(STRING_VALUE)
    // The property is populated as well. Populating the form moves focus into its first input, which is the
    // property, and a reference input which has focus shows its search box instead of the document it holds,
    // so what says the property is there is the button offering to clear it.
    const property = panel.locator(".pd-documentedit-field-property")
    await expect(property.locator(".pd-inputref-button-clear"), "the property the form was populated with").toBeVisible()
    // The claim being edited is the one the form is about, so its own edit button is out of the way.
    await expect(editButton, "the edit button of the claim being edited").toBeDisabled()

    await fillValue(panel.locator(".pd-inputstring"), EDITED_STRING_VALUE, "the value of the claim being edited")
    // With focus moved on to the value, the property field shows the property it holds again.
    await expect(property.locator(".pd-inputref-value"), "the property the claim being edited is stated on").toBeVisible()
    await checkpointElement(page, page.locator("#documentedit-form-claim"), "allpropsedit-editing-form")

    const update = page.locator("#documentedit-button-addclaim")
    await expect(update, "the update button of the claim being edited").toBeEnabled()
    await update.click()

    // Updating replaces the claim instead of adding one, so the property still has a single row and it
    // carries what was typed over the old value.
    await expect(editorRow(page, propertyId), "the rows of the property after the update").toHaveCount(1)
    await expect(editorRow(page, propertyId).locator(".pd-claimvaluestring"), "the value of the claim after the update").toHaveText(EDITED_STRING_VALUE)
    await expect(page.locator("#documentedit-error-claim"), "an error while updating the claim").toHaveCount(0)

    await saveEdit(page)
    await openSavedProperties(page)
    await expect(propertyRow(page, propertyId), "the rows of the property on the saved document").toHaveCount(1)
    await expect(propertyRow(page, propertyId).locator(".pd-claimvaluestring"), "the value of the saved claim").toHaveText(EDITED_STRING_VALUE)
    await expectStoredClaim(page, id, edited.stored, propertyId, "the edited claim")
    await checkpointElement(page, page.locator(".pd-documentget-panel-allproperties .pd-propertiesview").first(), "allpropsedit-edited")

    const rows = await propertyRow(page, propertyId).count()
    console.log(`Successfully verified that editing a claim through the all properties editor replaced its value and left the property with ${rows} claim.`)
  })

  test("Test removing a claim through the all properties editor", async ({ context }) => {
    const page = await context.newPage()
    // Two claims of two properties are added, so that removing one says which one went: a removal which
    // took the wrong claim, or all of them, is a different picture than the one asserted below.
    const removed = CLAIM_TYPE_CASES.find((claimCase) => claimCase.tab === "string")!
    const kept = CLAIM_TYPE_CASES.find((claimCase) => claimCase.tab === "amount")!
    const removedId = PROPERTY_IDS[removed.property]
    const keptId = PROPERTY_IDS[kept.property]

    const id = await createAndEdit(page, "removed claim")
    await stateClaim(page, removed, 1)
    await stateClaim(page, kept, 1)
    await saveEdit(page)

    await openSavedProperties(page)
    await expect(propertyRow(page, removedId), "the row of the claim to be removed").toHaveCount(1)
    await expect(propertyRow(page, keptId), "the row of the claim which stays").toHaveCount(1)

    await startEdit(page)
    await hideDuplicates(page)
    await openClaimEditor(page)
    const removeButton = editorRow(page, removedId).locator(".pd-propertiesview-button-remove")
    await expect(removeButton, "the remove button of the claim").toBeVisible()
    await removeButton.click()

    // The claim goes as soon as it is removed, before anything is saved, and nothing else goes with it.
    await expect(editorRow(page, removedId), "the rows of the removed claim in the editor").toHaveCount(0, { timeout: LOADING_TIMEOUT })
    await expect(editorRow(page, keptId), "the rows of the claim which stays").toHaveCount(1)
    await expect(editorRow(page, PROPERTY_IDS.NAME), "the name of the document after the removal").toHaveCount(1)
    await checkpointElement(page, page.locator(".pd-documentedit-panel-allproperties .pd-propertiesview").first(), "allpropsedit-removed-editor")

    await saveEdit(page)
    await openSavedProperties(page)
    await expect(propertyRow(page, removedId), "the rows of the removed claim on the saved document").toHaveCount(0)
    await expect(propertyRow(page, keptId), "the rows of the claim which stays on the saved document").toHaveCount(1)
    await expect(propertyRow(page, keptId).locator(".pd-claimvalueamount"), "the value of the claim which stays").toHaveText(AMOUNT_VALUE)
    await expectStoredClaim(page, id, kept.stored, keptId, "the claim which stays")
    await checkpointElement(page, page.locator(".pd-documentget-panel-allproperties .pd-propertiesview").first(), "allpropsedit-removed")

    const rows = await page.locator(".pd-documentget-panel-allproperties .pd-propertiesview-row").count()
    console.log(
      `Successfully verified that removing a claim through the all properties editor took 1 of the 2 claims added, leaving the saved document with ${rows} claims.`,
    )
  })
})
