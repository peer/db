import type { Locator, Page } from "@playwright/test"

import en from "@/locales/en.json" with { type: "json" }
import pt from "@/locales/pt.json" with { type: "json" }
import sl from "@/locales/sl.json" with { type: "json" }
import { createNamed, LANGUAGES, PROPERTY_IDS, startCreate } from "../peerdb_utils"
import {
  checkpoint,
  checkpointElement,
  checkpointFields,
  clearRefusedRequestErrors,
  discardCreate,
  discardEdit,
  documentId,
  documentValues,
  editingDocumentId,
  expect,
  fetchFromPage,
  fieldSlots,
  fillSlot,
  hideDuplicates,
  LOADING_TIMEOUT,
  openDocument,
  PEERDB_URL,
  saveEdit,
  settleDocument,
  settleEdit,
  signIn,
  slotInput,
  slotRevert,
  startEdit,
  switchLanguage,
  test,
  volatile,
} from "../utils"

// The class every test here creates its own document in. A biome is one of the controlled vocabularies of
// the test data, and a vocabulary entry holds a repeated name, a repeated description and a repeated code.
// That is enough to change a value, to add a second value to a field which already has one and to remove a
// value again, without ever touching a document which already exists and which the other test areas
// screenshot. The vocabulary already holds sixteen entries, well over the ten at which a reference field
// switches from a list of checkboxes to a combobox, so the entries this file adds cannot change how another
// class's form is rendered.
const VOCABULARY_CLASS = "BIOME"

// The role the site grants creating a vocabulary entry to (ROLE_CREATES in peerdb_utils).
const CREATING_ROLE = "curator"

// Every document this file creates is named beginning with this, so that the documents of one test file
// never collide with another's and a document left behind says which file made it.
const PREFIX = "EditDoc"

// The interface messages the tests read back, taken from the application's own translations, so that a label
// which differs between the three languages the site is served in is not repeated here.
const LOCALES = { en, sl, pt }

// The field-level "changed" badge, which doubles as the revert button for the whole field. The badge is
// rendered even when the field is unchanged (it only reserves its width then), so it is asserted on with
// toBeVisible/toBeHidden rather than by counting. A field with sub-fields renders a badge per sub-field too,
// and the field's own badge sits in its label cell, which comes first in the document.
function fieldRevert(page: Page, propertyId: string): Locator {
  return page.locator(`.pd-fieldsformfield-${propertyId} .pd-inputbadges-button-revert`).first()
}

// Types into the rich text editor of a field. Focus is moved off the editor by pressing the document's
// title, which is not focusable, so that the editor commits what was typed and so that the screenshot
// afterwards is of the settled editor rather than of the focused one.
async function fillHTMLSlot(page: Page, propertyId: string, slot: number, value: string, slots: number, what: string): Promise<void> {
  const editor = fieldSlots(page, propertyId).nth(slot).locator(".pd-inputhtml-editor")
  await expect(editor, what).toBeVisible()
  await editor.click()
  await page.keyboard.type(value)
  await expect(editor, `${what} after typing`).toHaveText(value)
  const title = page.locator("#documentedit-title")
  await expect(title, "title of the document being edited").toBeVisible()
  await title.click()
  await expect(fieldSlots(page, propertyId), `slots of ${what} after typing`).toHaveCount(slots)
}

// Creates a vocabulary entry with the given names, code and optional description, taking a checkpoint after
// every field it touches, and returns the identifier of the created document. Every test starts by creating
// the document it then edits, so it never changes a document which the test data already contains.
async function createBiome(page: Page, prefix: string, names: Array<string>, code: string, description?: string): Promise<string> {
  await startCreate(page, VOCABULARY_CLASS)
  await hideDuplicates(page)
  await expect(page.locator("#documentedit-button-discard"), "discard button of the create form").toBeVisible()
  await checkpointFields(page, `${prefix}-create-form-empty`)

  for (const [i, name] of names.entries()) {
    // Every name goes into the field's trailing empty slot, so the field grows by one with each of them.
    await fillSlot(page, PROPERTY_IDS.NAME, i, ".pd-inputstring", name, i + 2, `name ${i + 1} of the new document`)
    // The title of a document is built from its first name, so filling the first name names the document
    // being created.
    await expect(page.locator("#documentedit-title"), "title of the document being created").toHaveText(names[0])
    await expect(fieldRevert(page, PROPERTY_IDS.NAME), "changed badge of the name field").toBeVisible()
    await checkpointFields(page, `${prefix}-create-name-${i + 1}`)
  }

  await fillSlot(page, PROPERTY_IDS.CODE, 0, ".pd-inputidentifier", code, 2, "code of the new document")
  await expect(fieldRevert(page, PROPERTY_IDS.CODE), "changed badge of the code field").toBeVisible()
  await checkpointFields(page, `${prefix}-create-code`)

  if (description !== undefined) {
    await fillHTMLSlot(page, PROPERTY_IDS.DESCRIPTION, 0, description, 2, "description of the new document")
    await expect(fieldRevert(page, PROPERTY_IDS.DESCRIPTION), "changed badge of the description field").toBeVisible()
    await checkpointFields(page, `${prefix}-create-description`)
  }

  await saveEdit(page)
  await checkpoint(page, `${prefix}-created-document`, { mask: volatile(page) })

  return documentId(page)
}

test.describe("PeerDB Document Editing Flows", () => {
  test("Test changing, adding and removing values of a document and saving them", async ({ context }) => {
    // A screenshot is taken of every field which is filled and of both sides of every change, which is more
    // than a test of the default length has time for.
    test.slow()

    const page = await context.newPage()

    const names = [`${PREFIX} Biome Alpha`, `${PREFIX} Biome Beta`]
    const description = "Biome created by the document editing test."
    const code = "EDITDOC-ALPHA"
    const renamed = `${PREFIX} Biome Alpha Renamed`
    const addedCode = "EDITDOC-ALPHA-TWO"

    await signIn(page, [CREATING_ROLE])

    // The document is created first so that the edit below changes a document this test owns.
    const id = await createBiome(page, "editdoc-edit", names, code, description)

    // Reopening the created document by its identifier proves it was really stored, and it is the way a user
    // comes back to a document before editing it.
    await openDocument(page, id)
    await settleDocument(page)

    await startEdit(page)
    // Nothing has been changed yet, so the session has nothing to save and no field is marked changed.
    await expect(page.locator("#documentedit-button-save"), "save button of an untouched edit").toBeDisabled()
    await expect(fieldRevert(page, PROPERTY_IDS.NAME), "changed badge of the name field of an untouched edit").toBeHidden()
    await expect(fieldRevert(page, PROPERTY_IDS.CODE), "changed badge of the code field of an untouched edit").toBeHidden()

    // Change an existing value: the first of the two names.
    await expect(slotInput(page, PROPERTY_IDS.NAME, 0, ".pd-inputstring"), "first name before the change").toHaveValue(names[0])
    await checkpoint(page, "editdoc-edit-name-before-change")
    await fillSlot(page, PROPERTY_IDS.NAME, 0, ".pd-inputstring", renamed, 3, "first name")
    await expect(page.locator("#documentedit-title"), "title after the name changed").toHaveText(renamed)
    await expect(fieldRevert(page, PROPERTY_IDS.NAME), "changed badge of the name field after the change").toBeVisible()
    await checkpoint(page, "editdoc-edit-name-after-change")

    // Add a value to a field which already has one: a second code next to the first.
    await expect(fieldSlots(page, PROPERTY_IDS.CODE), "slots of the code field before the addition").toHaveCount(2)
    await checkpoint(page, "editdoc-edit-code-before-add")
    await fillSlot(page, PROPERTY_IDS.CODE, 1, ".pd-inputidentifier", addedCode, 3, "second code")
    await expect(slotInput(page, PROPERTY_IDS.CODE, 0, ".pd-inputidentifier"), "first code after the addition").toHaveValue(code)
    await expect(fieldRevert(page, PROPERTY_IDS.CODE), "changed badge of the code field after the addition").toBeVisible()
    await checkpoint(page, "editdoc-edit-code-after-add")

    // Remove a value: the second name, which leaves the required first one in place. A value is removed by
    // emptying its slot, after which the form shrinks back to one trailing empty slot.
    await expect(slotInput(page, PROPERTY_IDS.NAME, 1, ".pd-inputstring"), "second name before the removal").toHaveValue(names[1])
    await expect(fieldSlots(page, PROPERTY_IDS.NAME), "slots of the name field before the removal").toHaveCount(3)
    await checkpoint(page, "editdoc-edit-name-before-remove")
    const secondName = slotInput(page, PROPERTY_IDS.NAME, 1, ".pd-inputstring")
    await secondName.fill("")
    await secondName.blur()
    await expect(fieldSlots(page, PROPERTY_IDS.NAME), "slots of the name field after the removal").toHaveCount(2)
    await expect(slotInput(page, PROPERTY_IDS.NAME, 1, ".pd-inputstring"), "trailing name slot after the removal").toHaveValue("")
    await checkpoint(page, "editdoc-edit-name-after-remove")

    await saveEdit(page)

    // The saved document has to show every change: the renamed first name, both codes, and no trace of the
    // removed second name.
    await expect(page.locator("#documentget-title"), "title of the saved document").toHaveText(renamed)
    await expect(documentValues(page), "values of the saved document").toHaveText([renamed, description, code, addedCode])
    await checkpoint(page, "editdoc-edit-document-after", { mask: volatile(page) })

    // A form opened again on the saved document starts from what was saved and has nothing to save itself,
    // which is what says the changes were committed rather than only rendered.
    await startEdit(page)
    await expect(page.locator("#documentedit-button-save"), "save button of the reopened form").toBeDisabled()
    await expect(fieldSlots(page, PROPERTY_IDS.NAME), "slots of the name field of the reopened form").toHaveCount(2)
    await expect(slotInput(page, PROPERTY_IDS.NAME, 0, ".pd-inputstring"), "name of the reopened form").toHaveValue(renamed)
    await expect(fieldSlots(page, PROPERTY_IDS.CODE), "slots of the code field of the reopened form").toHaveCount(3)
    await expect(slotInput(page, PROPERTY_IDS.CODE, 0, ".pd-inputidentifier"), "first code of the reopened form").toHaveValue(code)
    await expect(slotInput(page, PROPERTY_IDS.CODE, 1, ".pd-inputidentifier"), "second code of the reopened form").toHaveValue(addedCode)
    await checkpointFields(page, "editdoc-edit-form-reopened")
    await discardEdit(page)

    console.log(`Successfully changed, added and removed values of document ${id}, saved them and read back its 4 values.`)
  })

  test("Test the changed indicator and the revert control of a field", async ({ context }) => {
    const page = await context.newPage()

    const name = `${PREFIX} Revert Biome`
    const code = "EDITDOC-REVERT"
    const changedCode = "EDITDOC-REVERT-CHANGED"

    await signIn(page, [CREATING_ROLE])

    const id = await createBiome(page, "editdoc-revert", [name], code)

    await openDocument(page, id)
    await settleDocument(page)
    await startEdit(page)

    // An untouched session marks nothing as changed and has nothing to save, so both the field's badge and
    // the badge of its only entry are still hidden.
    const badge = fieldRevert(page, PROPERTY_IDS.CODE)
    const entryBadge = slotRevert(page, PROPERTY_IDS.CODE, 0)
    const save = page.locator("#documentedit-button-save")
    await expect(save, "save button of an untouched edit").toBeDisabled()
    await expect(badge, "changed badge of an untouched code field").toBeHidden()
    await expect(entryBadge, "changed badge of an untouched code entry").toBeHidden()
    await checkpoint(page, "editdoc-revert-edit-initial")

    await fillSlot(page, PROPERTY_IDS.CODE, 0, ".pd-inputidentifier", changedCode, 2, "code being changed")
    // Changing the value marks the field and the entry it is in as changed, and gives the session something
    // to save.
    await expect(badge, "changed badge of the changed code field").toBeVisible()
    await expect(entryBadge, "changed badge of the changed code entry").toBeVisible()
    await expect(save, "save button after the change").toBeEnabled()
    await checkpoint(page, "editdoc-revert-code-changed")

    await badge.click()

    // Reverting puts the value the session started from back and takes the change away with it, so the
    // session again has nothing to save.
    await expect(slotInput(page, PROPERTY_IDS.CODE, 0, ".pd-inputidentifier"), "code after the revert").toHaveValue(code)
    await expect(badge, "changed badge of the reverted code field").toBeHidden()
    await expect(entryBadge, "changed badge of the reverted code entry").toBeHidden()
    await expect(save, "save button after the revert").toBeDisabled()
    await checkpoint(page, "editdoc-revert-code-reverted")

    await discardEdit(page)
    await expect(documentValues(page), "values of the document after the reverted edit").toHaveText([name, code])
    await checkpoint(page, "editdoc-revert-document-after", { mask: volatile(page) })

    console.log(`Successfully used the changed indicator and the revert control on document ${id}, whose 2 values came back unchanged.`)
  })

  test("Test the primary button says creating in a create session and updating in an edit one", async ({ context }) => {
    // The button is read back in each of the three languages the site is served in, and each of them opens a
    // create session and an edit session, which is more than a test of the default length has time for.
    test.slow()

    const page = await context.newPage()

    const name = `${PREFIX} Buttons Biome`

    await signIn(page, [CREATING_ROLE])

    // The document the edit sessions below are opened on, created once and edited in every language.
    const id = await createNamed(page, VOCABULARY_CLASS, name)

    const save = page.locator("#documentedit-button-save")
    for (const language of LANGUAGES) {
      await switchLanguage(page, language)

      // A session which is creating a document offers to create it.
      await startCreate(page, VOCABULARY_CLASS)
      await hideDuplicates(page)
      await expect(save, `primary button of a create session in ${language}`).toHaveText(LOCALES[language].common.buttons.create)
      await checkpointElement(page, page.locator(".pd-documentedit-actions"), `editdoc-actions-create-${language}`)
      await discardCreate(page)

      // A session which is editing a document which exists offers to update it instead, which is the same
      // button, in the same place, saying what the session will do.
      await openDocument(page, id)
      await settleDocument(page)
      await startEdit(page)
      await expect(save, `primary button of an edit session in ${language}`).toHaveText(LOCALES[language].common.buttons.update)
      await checkpointElement(page, page.locator(".pd-documentedit-actions"), `editdoc-actions-update-${language}`)
      await discardEdit(page)
    }

    console.log(`Successfully read the primary button of a create session and of an edit session of document ${id} in ${LANGUAGES.length} languages.`)
  })

  test("Test a document being created is not stored until the session is saved", async ({ context }) => {
    const page = await context.newPage()

    const name = `${PREFIX} Unsaved Biome`

    await signIn(page, [CREATING_ROLE])

    await startCreate(page, VOCABULARY_CLASS)
    await hideDuplicates(page)
    // The identifier the document will be saved under is allocated when the session is opened, so it is in
    // the address of the form long before there is a document to go with it.
    const id = editingDocumentId(page)

    const empty = await fetchFromPage(page, `/api/d/${id}`)
    expect(empty.status, "the document of a create session in which nothing was filled in").toBe(404)
    // Asking for a document which is not there is answered with a status the browser reports as a failed
    // request, which the checkpoint below would otherwise fail on.
    clearRefusedRequestErrors(page)

    await fillSlot(page, PROPERTY_IDS.NAME, 0, ".pd-inputstring", name, 2, "name of the document being created")
    const filled = await fetchFromPage(page, `/api/d/${id}`)
    expect(filled.status, "the document of a create session after a value was filled in").toBe(404)
    clearRefusedRequestErrors(page)

    // A visitor who asks for the address of the document being created is answered that there is nothing
    // there: the session holds the value, the store does not.
    const visitor = await context.newPage()
    const refused = await visitor.goto(`${PEERDB_URL}/d/${id}`)
    expect(refused?.status(), "the address of the document being created").toBe(404)
    await visitor.close()
    clearRefusedRequestErrors(page)

    await checkpointFields(page, "editdoc-unsaved-create-form")

    // Saving materializes the document under the identifier the session was opened with.
    const saved = await saveEdit(page)
    expect(saved, "the identifier the document was saved under").toBe(id)
    const stored = await fetchFromPage(page, `/api/d/${id}`)
    expect(stored.status, "the document after the create session was saved").toBe(200)
    await expect(page.locator("#documentget-title"), "title of the saved document").toHaveText(name)
    await expect(documentValues(page), "values of the saved document").toHaveText([name])
    await checkpoint(page, "editdoc-unsaved-document-after-save", { mask: volatile(page) })

    console.log(`Successfully checked that document ${id} was answered with 404 twice while it was being created and with 200 once it was saved.`)
  })

  test("Test a value typed right before saving reaches the saved document", async ({ context }) => {
    const page = await context.newPage()

    const name = `${PREFIX} Drained Biome`
    const code = "EDITDOC-DRAINED"

    await signIn(page, [CREATING_ROLE])

    await startCreate(page, VOCABULARY_CLASS)
    await hideDuplicates(page)
    await fillSlot(page, PROPERTY_IDS.NAME, 0, ".pd-inputstring", name, 2, "name of the document being created")

    // The code is typed and left in the input, the way a user who presses Save without leaving the field
    // does it. Pressing Save has to commit what the slot holds and wait for it to reach the server before it
    // ends the session, so the value is in the saved document even though it was never blurred.
    const codeInput = slotInput(page, PROPERTY_IDS.CODE, 0, ".pd-inputidentifier")
    await expect(codeInput, "code of the document being created").toBeVisible()
    await codeInput.click()
    await page.keyboard.type(code)
    await expect(codeInput, "code after typing").toHaveValue(code)
    await checkpointFields(page, "editdoc-drain-form-typed")

    const save = page.locator("#documentedit-button-save")
    await expect(save, "save button while a value is still being typed").toBeEnabled()
    await save.click()
    await settleDocument(page)
    const id = documentId(page)

    await expect(page.locator("#documentget-title"), "title of the saved document").toHaveText(name)
    await expect(documentValues(page), "values of the saved document").toHaveText([name, code])
    await checkpoint(page, "editdoc-drain-document-after-save", { mask: volatile(page) })

    console.log(`Successfully saved document ${id} straight from the input and found both of its 2 values in it.`)
  })

  test("Test leaving the page with a value which was not committed warns first", async ({ context }) => {
    const page = await context.newPage()

    const name = `${PREFIX} Warned Biome`

    await signIn(page, [CREATING_ROLE])

    const id = await createNamed(page, VOCABULARY_CLASS, name)
    await startEdit(page)

    const nameInput = slotInput(page, PROPERTY_IDS.NAME, 0, ".pd-inputstring")
    await expect(nameInput, "name of the document being edited").toHaveValue(name)
    await nameInput.click()
    await page.keyboard.type(" Changed")
    await expect(nameInput, "name after typing").toHaveValue(`${name} Changed`)

    // The warning is the browser's own dialog rather than anything the page renders, so it is answered by
    // the test. Dismissing it means staying, which is what a user who does not want to lose what they typed
    // chooses, so the form is still there afterwards.
    let dialogs = 0
    page.on("dialog", async (dialog) => {
      dialogs += 1
      expect(dialog.type(), "the dialog shown while leaving a form with an uncommitted value").toBe("beforeunload")
      await dialog.dismiss()
    })
    await page.close({ runBeforeUnload: true })
    await expect.poll(() => dialogs, { message: "the form warns before the page is left with an uncommitted value" }).toBe(1)
    expect(page.isClosed(), "the page after the warning was dismissed").toBe(false)
    await settleEdit(page)

    // Discarding what was typed leaves the document as it was.
    await discardEdit(page)
    await expect(page.locator("#documentget-title"), "title of the document after the edit was discarded").toHaveText(name)
    await expect(documentValues(page), "values of the document after the edit was discarded").toHaveText([name])

    // A document view has nothing which was not committed, so leaving it warns about nothing and the page
    // goes away.
    await page.close({ runBeforeUnload: true })
    await expect.poll(() => page.isClosed(), { message: "the document view is left without a warning" }).toBe(true)
    expect(dialogs, "warnings shown in the whole test").toBe(1)

    console.log(`Successfully got 1 warning while leaving the form of document ${id} and none while leaving its document view.`)
  })

  test("Test two views of one editing session share what each of them changes", async ({ context }) => {
    const page = await context.newPage()

    const names = [`${PREFIX} Shared Biome`, `${PREFIX} Shared Biome Second`]
    const code = "EDITDOC-SHARED"

    await signIn(page, [CREATING_ROLE])

    await startCreate(page, VOCABULARY_CLASS)
    await hideDuplicates(page)
    const session = page.url()
    await fillSlot(page, PROPERTY_IDS.NAME, 0, ".pd-inputstring", names[0], 2, "name of the document being created")

    // A session is a log of changes rather than a form, so opening its address again gives another view of
    // the same work, which starts from everything the session has been told so far.
    const other = await context.newPage()
    await other.goto(session)
    await settleEdit(other)
    await hideDuplicates(other)
    await expect(slotInput(other, PROPERTY_IDS.NAME, 0, ".pd-inputstring"), "name in the second view of the session").toHaveValue(names[0])
    await checkpointFields(other, "editdoc-shared-second-view")

    // What the second view changes reaches the first one, which watches the session for changes it did not
    // make itself.
    await fillSlot(other, PROPERTY_IDS.CODE, 0, ".pd-inputidentifier", code, 2, "code filled in by the second view")
    await expect(slotInput(page, PROPERTY_IDS.CODE, 0, ".pd-inputidentifier"), "code in the first view").toHaveValue(code, { timeout: LOADING_TIMEOUT })
    await expect(fieldSlots(page, PROPERTY_IDS.CODE), "slots of the code field of the first view").toHaveCount(2)
    await checkpointFields(page, "editdoc-shared-first-view")

    // The second view is closed before the session is ended, because a view whose session ends elsewhere
    // goes on asking that session for changes it no longer has.
    await other.close()

    await fillSlot(page, PROPERTY_IDS.NAME, 1, ".pd-inputstring", names[1], 3, "second name of the document being created")
    const id = await saveEdit(page)

    // Every change of both views is in the saved document, and none of them is in it twice.
    await expect(documentValues(page), "values of the document both views wrote into").toHaveText([names[0], names[1], code])
    await checkpoint(page, "editdoc-shared-document-after-save", { mask: volatile(page) })

    console.log(`Successfully saved document ${id} with the 3 values two views of one session wrote into it.`)
  })

  test("Test the tab of the edit view is kept in the address", async ({ context }) => {
    const page = await context.newPage()

    const name = `${PREFIX} Tabs Biome`

    await signIn(page, [CREATING_ROLE])

    const id = await createNamed(page, VOCABULARY_CLASS, name)
    await startEdit(page)

    // The class tab is the tab the edit view opens on, and being the default it names itself in no address.
    const fieldsTab = page.locator(".pd-documentedit-tab-fields")
    const allPropertiesTab = page.locator(".pd-documentedit-tab-allproperties")
    await expect(fieldsTab, "class tab of the edit view").toHaveAttribute("aria-selected", "true")
    expect(new URL(page.url()).searchParams.get("tab"), "the address of the tab the edit view opens on").toBeNull()

    // Every other tab writes its own name into the address, so a tab can be linked to.
    await allPropertiesTab.click()
    await expect(page.locator(".pd-documentedit-panel-allproperties"), "all properties panel").toBeVisible()
    await expect.poll(() => new URL(page.url()).searchParams.get("tab"), { message: "the address of the all properties tab" }).toBe("properties")

    // Selecting a tab is a step of its own in the browser's history, so the back button returns to the tab
    // which was selected before it.
    await page.goBack()
    await expect(fieldsTab, "class tab after the back button").toHaveAttribute("aria-selected", "true")
    await expect(page.locator(".pd-documentedit-panel-fields"), "fields panel after the back button").toBeVisible()
    await settleEdit(page)
    await expect(slotInput(page, PROPERTY_IDS.NAME, 0, ".pd-inputstring"), "name after the back button").toHaveValue(name)

    // The address of a tab is enough to open the form on it, without going through the tab strip.
    await page.goto(`${page.url().split("?")[0]}?tab=properties`)
    await expect(page.locator(".pd-documentedit-panel-allproperties"), "all properties panel of the linked tab").toBeVisible({ timeout: LOADING_TIMEOUT })
    await expect(allPropertiesTab, "all properties tab of the linked address").toHaveAttribute("aria-selected", "true")
    await expect(page.locator("#documentedit-form-claim"), "the form to add a claim of the linked tab").toBeVisible()

    // An address naming a tab which the edit view does not have falls back to the tab it opens on.
    await page.goto(`${page.url().split("?")[0]}?tab=nosuchtab`)
    await settleEdit(page)
    await expect(fieldsTab, "class tab of an address naming a tab which is not there").toHaveAttribute("aria-selected", "true")

    await discardEdit(page)

    console.log(`Successfully followed the tab of the edit view of document ${id} through the address, the back button and a link.`)
  })
})
