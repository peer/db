import type { Locator, Page } from "@playwright/test"

import { mkdtempSync, writeFileSync } from "node:fs"
import { tmpdir } from "node:os"
import { basename, join } from "node:path"

import { documentIdOf, notesSlot, PROPERTY_IDS, startCreate } from "../peerdb_utils"
import {
  checkpoint,
  checkpointElement,
  clearConsoleErrors,
  discardEdit,
  expect,
  expectAttachmentServed,
  field,
  hideDuplicates,
  holdUploads,
  LOADING_TIMEOUT,
  openDocument,
  openDocumentTab,
  pickReference,
  saveEdit,
  settleFormFocus,
  signIn,
  startEdit,
  test,
  volatile,
  volatileFileLinks,
} from "../utils"

// Every document these tests create is named with this prefix, so that the documents of this file never
// collide with the ones another test file creates and a document left in the data set says which file
// made it.
const NAME_PREFIX = "Attachments"

// The class every upload but the last one is made on. A species carries both an image, which is a file
// field, and notes, which is a rich text field with an attach button, so both ways of attaching a file
// sit on one form, and it needs nothing but a name in order to be saved. The role which may create one
// is the researcher (ROLE_CREATES in peerdb_utils says which role opens which class).
const SPECIES_CLASS = "SPECIES"
const SPECIES_ROLE = "researcher"

// The class the last test drives, together with the role which may create one and the role which may
// change one without being allowed to upload anything. An interview carries a recording, which is a file
// field, and the ethics role is granted updating interviews but not creating files, so it is the one
// identity of this site which reaches a form with a file field and may not put a file into it.
const INTERVIEW_CLASS = "INTERVIEW"
const INTERVIEW_ROLE = "researcher"
const NO_UPLOAD_ROLE = "ethics"

// The two documents the required reference fields of a new interview are pointed at, each by its
// identifier, so that the same document is picked out of the ranked results on every run.
const INTERVIEWEE_ID = await documentIdOf("INDIVIDUAL", "G1_ASELUNE")
const INTERVIEWEE_QUERY = "Aselune"
const INTERVIEWER_ID = await documentIdOf("RESEARCHER", "RES_HALVORSEN")
const INTERVIEWER_QUERY = "Halvorsen"

const DISCARD_UPLOAD_URL = "**/api/f/discardUpload/*"
const END_UPLOAD_URL = "**/api/f/endUpload/*"

// A hash which matches no file, which is what an upload whose bytes were damaged on the way ends up
// reporting: the client hashes what it read from disk and the server hashes what it assembled, so the
// two differ. It is written over the hash the client computed rather than over the bytes of a chunk
// because the two produce the same mismatch and this way the payload stays readable.
const WRONG_HASH = "0".repeat(64)

// The directory the files the uploads are made with are written into. They are written rather than taken
// out of the test data, so that nothing the rest of the suite compares against is handed to the browser,
// and they are small enough to go up in a single chunk.
const FIXTURES = mkdtempSync(join(tmpdir(), "peerdb-attachments-"))

// Writes one of those files and returns the path it was written to, which is what is uploaded.
function fixture(filename: string, content: string): string {
  const path = join(FIXTURES, filename)
  writeFileSync(path, content)
  return path
}

// The two files the uploads are made with. There are two of them because a test which replaces an
// attachment has to tell the file which went up second from the one which went up first, and they are of
// two kinds so that the server has to serve two different media types back.
const FIRST_FILE = fixture(
  "attachments-field-note.txt",
  "Field note, station 4.\nThe tide table was read back twice and agreed with on the second reading.\nCollected on the flats.\n",
)
const SECOND_FILE = fixture("attachments-tide-table.csv", "turn,height,read_by\n1,0.4,warden\n2,1.1,warden\n3,0.9,informant\n")

// The block of the form which holds the image of a species, which is the file field these tests drive.
function imageField(page: Page): Locator {
  return field(page, PROPERTY_IDS.IMAGE)
}

// One sub-field of the image, which is where the name and the caption of an uploaded file are edited.
// A sub-field is rendered only once the value it belongs to has been uploaded, and it carries the
// identifier of its own property.
function imageSubField(page: Page, subPropertyId: string): Locator {
  return imageField(page).locator(`.pd-claimcardinality-${subPropertyId}`).first()
}

// Signs in and starts creating a species, which is where every test but the last one starts. The name is
// filled right away because it is the only required field, so the document can be saved at the end of
// every test, and it is committed by blurring the input, the same way a user leaving the field does.
async function createSpecies(page: Page, name: string): Promise<void> {
  await signIn(page, [SPECIES_ROLE])
  await startCreate(page, SPECIES_CLASS)
  await hideDuplicates(page)
  await settleFormFocus(page)

  const nameInput = field(page, PROPERTY_IDS.NAME).locator(".pd-inputstring").first()
  await expect(nameInput, "name input of the new species").toBeVisible({ timeout: LOADING_TIMEOUT })
  await nameInput.fill(`${NAME_PREFIX} ${name}`)
  await nameInput.blur()
  await expect(nameInput, "name input of the new species after filling it").toHaveValue(`${NAME_PREFIX} ${name}`)
}

// Uploads a file into the image field and waits until the field holds it, without looking at the states
// the upload passes through. Used by the tests which are about what happens to a file which is already
// uploaded rather than about the upload itself.
async function uploadImage(page: Page, path: string): Promise<Locator> {
  const value = imageField(page).locator(".pd-inputfile-value")
  await expect(imageField(page).locator(".pd-inputfile-button-browse"), "browse button of the empty image field").toBeVisible()
  await imageField(page).locator(".pd-inputfile-input").setInputFiles(path)
  await expect(value, "uploaded image entry").toBeVisible({ timeout: LOADING_TIMEOUT })
  return value
}

// The link of a stored file is an ordinary anchor and not a route of the application, because a file is
// served by the server rather than rendered by it, and it carries the class the file icon is drawn from.
async function expectFileLinkClasses(link: Locator, what: string): Promise<void> {
  await expect(link, `class of the file link of ${what}`).toHaveClass(/pd-link-file/)
  await expect(link, `the file link of ${what} is an internal address`).toHaveClass(/pd-link-internal/)
  await expect(link, `the file link of ${what} is not routed by the application`).toHaveClass(/pd-link-internal-noview/)
}

test.describe("PeerDB Attachment Flows", () => {
  test("Test uploading a file into the image field", async ({ context }) => {
    const page = await context.newPage()

    await createSpecies(page, "Image Upload")

    const image = imageField(page)
    const browseButton = image.locator(".pd-inputfile-button-browse")
    await expect(browseButton, "browse button of the empty image field").toBeVisible()
    await expect(image.locator(".pd-fieldsformrow-file"), "entries of the empty image field").toHaveCount(1)
    await checkpointElement(page, image, "attachments-image-field-empty")

    // The upload is held so that the state which exists only while it runs can be looked at.
    const release = await holdUploads(page)

    // Clicking the control has to be what opens the file picker, so the test waits for the browser to ask
    // for a file instead of setting the files of the hidden input behind the control's back.
    const [chooser] = await Promise.all([page.waitForEvent("filechooser"), browseButton.click()])
    expect(chooser.isMultiple(), "the image file picker takes a single file").toBe(false)
    await chooser.setFiles(FIRST_FILE)

    const cancelButton = image.locator(".pd-inputfile-button-cancel")
    await expect(cancelButton, "cancel button while the image uploads").toBeVisible()
    await expect(image.locator(".pd-progressbar"), "progress bar while the image uploads").toBeVisible()
    await checkpointElement(page, image, "attachments-image-upload-in-progress")

    await release()

    const value = image.locator(".pd-inputfile-value")
    await expect(value, "uploaded image entry").toBeVisible({ timeout: LOADING_TIMEOUT })
    await expect(cancelButton, "cancel button after the image uploaded").toHaveCount(0)
    await expect(image.locator(".pd-inputfile-error"), "error after the image uploaded").toHaveCount(0)
    // The field may hold several images, so taking one grows a second, empty entry below it.
    await expect(image.locator(".pd-fieldsformrow-file"), "entries of the image field after the upload").toHaveCount(2)
    await checkpointElement(page, image, "attachments-image-uploaded", volatileFileLinks(page))

    // The uploaded entry has to point at the stored file, which is the value the field ends up holding.
    await expect(value.locator(".pd-claimvaluelink"), "link of the uploaded image entry").toHaveAttribute("href", /^\/f\/[0-9A-Za-z]+$/)

    await saveEdit(page)
    await checkpoint(page, "attachments-image-saved-document", { mask: [...volatile(page), ...volatileFileLinks(page)] })

    const savedLink = page.locator(`.pd-documentget-panel-properties .pd-fieldsview-row-${PROPERTY_IDS.IMAGE} .pd-claimvaluelink`).first()
    await expectFileLinkClasses(savedLink, "the saved species")
    await expectAttachmentServed(page, savedLink, FIRST_FILE, "the saved species")

    console.log("Successfully uploaded 1 file into the image field of a new species and reached it from the saved document.")
  })

  test("Test the name and the caption of an uploaded file", async ({ context }) => {
    const page = await context.newPage()

    await createSpecies(page, "Image Sub Fields")

    const image = imageField(page)
    // An empty file field is only a control to pick a file with: what is said about a file (what it is
    // called, what it shows, where it came from) is asked for once there is a file to say it about.
    await expect(imageSubField(page, PROPERTY_IDS.NAME), "name sub-field of the empty image field").toHaveCount(0)
    await expect(imageSubField(page, PROPERTY_IDS.CAPTION), "caption sub-field of the empty image field").toHaveCount(0)

    await uploadImage(page, FIRST_FILE)

    const nameInput = imageSubField(page, PROPERTY_IDS.NAME).locator(".pd-inputstring").first()
    const captionInput = imageSubField(page, PROPERTY_IDS.CAPTION).locator(".pd-inputstring").first()
    await expect(nameInput, "name sub-field of the uploaded image").toBeVisible()
    await expect(captionInput, "caption sub-field of the uploaded image").toBeVisible()
    // The image also carries where it came from, which is written as rich text rather than as a string.
    await expect(imageSubField(page, PROPERTY_IDS.SOURCE).locator(".pd-inputhtml-editor"), "source sub-field of the uploaded image").toBeVisible()
    await checkpointElement(page, image, "attachments-image-subfields", volatileFileLinks(page))

    const name = basename(FIRST_FILE)
    const caption = "The tide table as it was read back on the flats."
    await nameInput.fill(name)
    await nameInput.blur()
    await captionInput.fill(caption)
    await captionInput.blur()
    await expect(nameInput, "name sub-field after filling it").toHaveValue(name)
    await expect(captionInput, "caption sub-field after filling it").toHaveValue(caption)
    await checkpointElement(page, image, "attachments-image-subfields-filled", volatileFileLinks(page))

    await saveEdit(page)

    // The name of a file stands in for the address it is served under, so the saved link is named by it
    // rather than by the path, which is what makes the file readable to somebody who did not upload it.
    const savedLink = page.locator(`.pd-documentget-panel-properties .pd-fieldsview-row-${PROPERTY_IDS.IMAGE} .pd-claimvaluelink`).first()
    await expect(savedLink, "the saved image link is named by the file").toHaveText(name)
    await checkpoint(page, "attachments-image-subfields-saved-document", { mask: volatile(page) })

    // The caption is a claim of its own hanging off the file, which the tab listing every claim shows.
    await openDocumentTab(page, "allproperties")
    const captionValue = page.locator(`.pd-documentget-panel-allproperties .pd-propertiesview-row-${PROPERTY_IDS.CAPTION} .pd-propertiesview-value`)
    await expect(captionValue, "the saved caption").toHaveText(caption)
    await checkpoint(page, "attachments-image-subfields-saved-claims", { mask: volatile(page) })

    console.log("Successfully named and captioned 1 uploaded file and read both back from the saved species.")
  })

  test("Test clearing an uploaded file before saving", async ({ context }) => {
    const page = await context.newPage()

    await createSpecies(page, "Image Clear")

    const image = imageField(page)
    const value = await uploadImage(page, FIRST_FILE)
    await checkpointElement(page, image, "attachments-image-clear-uploaded", volatileFileLinks(page))

    // Clearing has to take the field back to its empty state, ready to take another file.
    const clearButton = image.locator(".pd-inputfile-button-clear")
    await expect(clearButton, "clear button of the uploaded image").toBeVisible()
    await clearButton.click()

    await expect(value, "image entry after clearing it").toHaveCount(0)
    // The field grew a second, empty entry when the first one took a file, and clearing the first one
    // takes that spare entry away again, so the field is back to the single empty entry it started with.
    await expect(image.locator(".pd-fieldsformrow-file"), "entries of the image field after clearing it").toHaveCount(1)
    await expect(image.locator(".pd-inputfile-button-browse"), "browse button after clearing the image").toBeVisible()
    await expect(imageSubField(page, PROPERTY_IDS.CAPTION), "caption sub-field after clearing the image").toHaveCount(0)
    await checkpointElement(page, image, "attachments-image-clear-cleared")

    await saveEdit(page)
    await checkpoint(page, "attachments-image-clear-saved-document", { mask: volatile(page) })

    // The cleared file must not survive into the saved document.
    await expect(page.locator(".pd-documentget-panel-properties .pd-claimvaluelink"), "file links of the saved species").toHaveCount(0)

    console.log("Successfully cleared 1 uploaded file and verified the saved species carries no file.")
  })

  test("Test cancelling an upload and uploading the same file again", async ({ context }) => {
    const page = await context.newPage()

    await createSpecies(page, "Image Cancel")

    const image = imageField(page)
    const release = await holdUploads(page)
    await expect(image.locator(".pd-inputfile-button-browse"), "browse button of the empty image field").toBeVisible()
    // The retry below picks the very same file the cancelled upload was made with, which is what a user
    // reaches for first after stopping an upload. The field empties its hidden input as soon as it has
    // taken the file out of it, so that this second pick is a change of the input and reaches the field
    // at all.
    await image.locator(".pd-inputfile-input").setInputFiles(SECOND_FILE)

    const cancelButton = image.locator(".pd-inputfile-button-cancel")
    await expect(cancelButton, "cancel button while the image uploads").toBeVisible()
    await checkpointElement(page, image, "attachments-image-cancel-in-progress")

    // A cancelled upload has to release the session it opened on the server rather than leaving it
    // behind, so the request which discards it is waited for.
    const discarded = page.waitForRequest(DISCARD_UPLOAD_URL, { timeout: LOADING_TIMEOUT })
    await cancelButton.click()
    await release()
    await discarded

    // A cancelled upload leaves the field empty and reports nothing: the user stopped it on purpose.
    await expect(cancelButton, "cancel button after cancelling the upload").toHaveCount(0)
    await expect(image.locator(".pd-inputfile-value"), "image entry after cancelling the upload").toHaveCount(0)
    await expect(image.locator(".pd-inputfile-error"), "error after cancelling the upload").toHaveCount(0)
    await expect(image.locator(".pd-inputfile-button-browse"), "browse button after cancelling the upload").toBeEnabled()
    await checkpointElement(page, image, "attachments-image-cancel-cancelled")

    // The field has to be usable again afterwards, so the same file goes up once more, this time fully.
    await uploadImage(page, SECOND_FILE)
    await checkpointElement(page, image, "attachments-image-cancel-retried", volatileFileLinks(page))

    await saveEdit(page)
    await checkpoint(page, "attachments-image-cancel-saved-document", { mask: [...volatile(page), ...volatileFileLinks(page)] })

    const savedLink = page.locator(`.pd-documentget-panel-properties .pd-fieldsview-row-${PROPERTY_IDS.IMAGE} .pd-claimvaluelink`).first()
    await expectAttachmentServed(page, savedLink, SECOND_FILE, "the retried species")

    console.log("Successfully cancelled 1 upload, released its session, uploaded the same file again and reached it from the saved document.")
  })

  test("Test an upload whose contents do not match what was uploaded", async ({ context }) => {
    const page = await context.newPage()

    await createSpecies(page, "Image Damaged")

    // The client hashes the file as it reads it and the server hashes what it assembled out of the
    // chunks, so a file which was damaged on the way is caught by the two hashes differing. Writing a
    // hash which matches nothing over the one the client computed is the same mismatch, and it is what
    // this test makes the server refuse the upload with.
    await page.route(END_UPLOAD_URL, async (route) => {
      await route.continue({ postData: JSON.stringify({ hash: WRONG_HASH }) })
    })

    const image = imageField(page)
    await expect(image.locator(".pd-inputfile-button-browse"), "browse button of the empty image field").toBeVisible()
    await image.locator(".pd-inputfile-input").setInputFiles(FIRST_FILE)

    // A refused upload has to say so rather than stopping at a progress bar which never finishes.
    const error = image.locator(".pd-inputfile-error")
    await expect(error, "error after the upload was refused").toBeVisible({ timeout: LOADING_TIMEOUT })
    await expect(image.locator(".pd-inputfile-value"), "image entry after the upload was refused").toHaveCount(0)
    await expect(image.locator(".pd-inputfile-button-cancel"), "cancel button after the upload was refused").toHaveCount(0)
    await expect(image.locator(".pd-inputfile-button-browse"), "browse button after the upload was refused").toBeEnabled()
    // The application logs the upload it could not finish, which is expected here and would otherwise
    // fail the checkpoint below, so it is cleared before anything is looked at.
    clearConsoleErrors(page)
    await checkpointElement(page, image, "attachments-image-hash-refused")

    // The field has to take a file again once the upload it refused is out of the way.
    await page.unrouteAll({ behavior: "ignoreErrors" })
    await uploadImage(page, FIRST_FILE)
    await expect(error, "error after the upload which was accepted").toHaveCount(0)
    await checkpointElement(page, image, "attachments-image-hash-retried", volatileFileLinks(page))

    await saveEdit(page)
    const savedLink = page.locator(`.pd-documentget-panel-properties .pd-fieldsview-row-${PROPERTY_IDS.IMAGE} .pd-claimvaluelink`).first()
    await expectAttachmentServed(page, savedLink, FIRST_FILE, "the species whose first upload was refused")

    console.log("Successfully had 1 upload refused for a hash which does not match its contents, and uploaded the same file again afterwards.")
  })

  test("Test attaching a file to the notes", async ({ context }) => {
    const page = await context.newPage()

    await createSpecies(page, "Notes Attachment")

    const notes = notesSlot(page)
    const editor = notes.locator(".pd-inputhtml-editor")
    await expect(editor, "notes editor").toBeVisible()
    await editor.click()
    await page.keyboard.type("Attached to these notes: ")
    await checkpointElement(page, notes, "attachments-notes-typed")

    const release = await holdUploads(page)

    // The attach button is what snapshots where the file link goes, so the file is picked through it.
    const attachButton = notes.locator(".pd-inputhtml-button-attachfile")
    await expect(attachButton, "attach file button of the notes editor").toBeVisible()
    const [chooser] = await Promise.all([page.waitForEvent("filechooser"), attachButton.click()])
    expect(chooser.isMultiple(), "the notes file picker takes more than one file").toBe(true)
    await chooser.setFiles(FIRST_FILE)

    await expect(notes.locator(".pd-inputhtml-text-upload"), "upload message of the notes editor").toContainText(basename(FIRST_FILE))
    await expect(notes.locator(".pd-inputhtml-button-cancelupload"), "cancel button while the attachment uploads").toBeVisible()
    await expect(notes.locator(".pd-inputhtml-loading"), "progress bar while the attachment uploads").toBeVisible()
    await checkpointElement(page, notes, "attachments-notes-in-progress")

    await release()

    // The finished upload is inserted into the text as a link named after the file.
    const link = editor.locator(".pd-link-file")
    await expect(link, "file link in the notes").toBeVisible({ timeout: LOADING_TIMEOUT })
    await expect(link, "name of the file link in the notes").toHaveText(basename(FIRST_FILE))
    await expect(notes.locator(".pd-inputhtml-error-upload"), "upload error of the notes editor").toHaveCount(0)
    await checkpointElement(page, notes, "attachments-notes-attached")

    await saveEdit(page)
    await checkpoint(page, "attachments-notes-saved-document", { mask: volatile(page) })

    const savedLink = page.locator(`.pd-documentget-panel-properties .pd-fieldsview-row-${PROPERTY_IDS.NOTES} .pd-claimvaluehtml .pd-link-file`).first()
    await expect(savedLink, "name of the file link of the saved species").toHaveText(basename(FIRST_FILE))
    await expectFileLinkClasses(savedLink, "the notes of the saved species")
    await expectAttachmentServed(page, savedLink, FIRST_FILE, "the notes of the saved species")

    console.log("Successfully attached 1 file to the notes of a new species and reached it from the saved document.")
  })

  test("Test replacing a file attached to the notes", async ({ context }) => {
    const page = await context.newPage()

    await createSpecies(page, "Notes Replace")

    const notes = notesSlot(page)
    const editor = notes.locator(".pd-inputhtml-editor")
    await expect(editor, "notes editor").toBeVisible()
    await editor.click()
    await page.keyboard.type("Replaced attachment: ")

    const attachButton = notes.locator(".pd-inputhtml-button-attachfile")
    await expect(attachButton, "attach file button of the notes editor").toBeVisible()
    const [chooser] = await Promise.all([page.waitForEvent("filechooser"), attachButton.click()])
    await chooser.setFiles(FIRST_FILE)

    const link = editor.locator(".pd-link-file")
    await expect(link, "file link in the notes").toBeVisible({ timeout: LOADING_TIMEOUT })
    await checkpointElement(page, notes, "attachments-notes-replace-attached")

    // Putting the cursor on the file link is what turns the bottom toolbar into the controls for that one
    // file, which is where replacing and removing it live.
    await link.click()
    const replaceButton = notes.locator(".pd-inputhtml-button-replacefile")
    await expect(replaceButton, "replace file button of the notes editor").toBeVisible()
    await expect(notes.locator(".pd-inputhtml-link-open"), "open link of the notes editor").toBeVisible()
    await expect(notes.locator(".pd-inputhtml-button-unlink"), "unlink button of the notes editor").toBeVisible()
    await expect(notes.locator(".pd-inputhtml-button-removefile"), "remove file button of the notes editor").toBeVisible()
    await checkpointElement(page, notes, "attachments-notes-replace-toolbar")

    const before = await link.getAttribute("href")
    const release = await holdUploads(page)
    const [replaceChooser] = await Promise.all([page.waitForEvent("filechooser"), replaceButton.click()])
    await replaceChooser.setFiles(SECOND_FILE)

    await expect(notes.locator(".pd-inputhtml-text-upload"), "upload message of the notes editor").toContainText(basename(SECOND_FILE))
    await expect(notes.locator(".pd-inputhtml-button-cancelupload"), "cancel button while the replacement uploads").toBeVisible()
    await checkpointElement(page, notes, "attachments-notes-replace-in-progress")

    await release()

    // Replacing rewrites the address the existing link points at, and leaves the text of the link alone.
    await expect(link, "file link in the notes after replacing the file").not.toHaveAttribute("href", before!, { timeout: LOADING_TIMEOUT })
    await expect(link, "address of the file link in the notes after replacing the file").toHaveAttribute("href", /^\/f\/[0-9A-Za-z]+$/)
    await expect(link, "name of the file link in the notes after replacing the file").toHaveText(basename(FIRST_FILE))
    await expect(notes.locator(".pd-inputhtml-error-upload"), "upload error of the notes editor").toHaveCount(0)
    await checkpointElement(page, notes, "attachments-notes-replace-replaced")

    await saveEdit(page)
    await checkpoint(page, "attachments-notes-replace-saved-document", { mask: volatile(page) })

    // The saved document has to serve the replacement, which is the file the second upload stored.
    const savedLink = page.locator(`.pd-documentget-panel-properties .pd-fieldsview-row-${PROPERTY_IDS.NOTES} .pd-claimvaluehtml .pd-link-file`).first()
    await expectAttachmentServed(page, savedLink, SECOND_FILE, "the notes of the saved species")

    console.log("Successfully replaced 1 file attached to the notes and reached the replacement from the saved document.")
  })

  test("Test cancelling an attachment upload and removing an attached file", async ({ context }) => {
    const page = await context.newPage()

    await createSpecies(page, "Notes Remove")

    const notes = notesSlot(page)
    const editor = notes.locator(".pd-inputhtml-editor")
    await expect(editor, "notes editor").toBeVisible()
    await editor.click()
    await page.keyboard.type("These notes keep their text: ")

    const attachButton = notes.locator(".pd-inputhtml-button-attachfile")
    await expect(attachButton, "attach file button of the notes editor").toBeVisible()

    const release = await holdUploads(page)
    const [cancelledChooser] = await Promise.all([page.waitForEvent("filechooser"), attachButton.click()])
    await cancelledChooser.setFiles(SECOND_FILE)

    const cancelButton = notes.locator(".pd-inputhtml-button-cancelupload")
    await expect(cancelButton, "cancel button while the attachment uploads").toBeVisible()
    await checkpointElement(page, notes, "attachments-notes-remove-in-progress")

    const discarded = page.waitForRequest(DISCARD_UPLOAD_URL, { timeout: LOADING_TIMEOUT })
    await cancelButton.click()
    await release()
    await discarded

    // A cancelled attachment leaves the text as it was, with nothing linked into it and nothing reported.
    await expect(editor.locator(".pd-link-file"), "file links in the notes after cancelling the upload").toHaveCount(0)
    await expect(notes.locator(".pd-inputhtml-error-upload"), "upload error after cancelling the upload").toHaveCount(0)
    await expect(editor, "text of the notes after cancelling the upload").toContainText("These notes keep their text:")
    await checkpointElement(page, notes, "attachments-notes-remove-cancelled")

    // The editor has to take an attachment again afterwards.
    await editor.click()
    const [chooser] = await Promise.all([page.waitForEvent("filechooser"), attachButton.click()])
    await chooser.setFiles(SECOND_FILE)

    const link = editor.locator(".pd-link-file")
    await expect(link, "file link in the notes").toBeVisible({ timeout: LOADING_TIMEOUT })
    await expect(link, "name of the file link in the notes").toHaveText(basename(SECOND_FILE))
    await checkpointElement(page, notes, "attachments-notes-remove-attached")

    // Removing takes the whole link out of the text, unlike unlinking, which would keep its name behind.
    await link.click()
    const removeButton = notes.locator(".pd-inputhtml-button-removefile")
    await expect(removeButton, "remove file button of the notes editor").toBeVisible()
    await removeButton.click()

    await expect(editor.locator(".pd-link-file"), "file links in the notes after removing the file").toHaveCount(0)
    await expect(editor, "text of the notes after removing the file").toContainText("These notes keep their text:")
    await checkpointElement(page, notes, "attachments-notes-remove-removed")

    await saveEdit(page)
    await checkpoint(page, "attachments-notes-remove-saved-document", { mask: volatile(page) })

    // The removed attachment must not come back through the saved document.
    await expect(page.locator(".pd-documentget-panel-properties .pd-link-file"), "file links of the saved species").toHaveCount(0)
    await expect(page.locator(`.pd-documentget-panel-properties .pd-fieldsview-row-${PROPERTY_IDS.NOTES}`), "notes of the saved species").toContainText(
      "These notes keep their text:",
    )

    console.log("Successfully cancelled 1 attachment upload, attached a file, removed it again and verified the saved species carries no file.")
  })

  test("Test what a caller who may not upload a file is shown", async ({ context }) => {
    const page = await context.newPage()

    // The interview is created rather than taken out of the test data, so that running the suite twice
    // leaves the data set in the same shape. It is created by the role which opens interviews, which is
    // also a role which may upload, so that the form is first seen the way somebody who may.
    await signIn(page, [INTERVIEW_ROLE])
    await startCreate(page, INTERVIEW_CLASS)
    await hideDuplicates(page)
    await settleFormFocus(page)

    const titleInput = field(page, PROPERTY_IDS.NAME).locator(".pd-inputstring").first()
    await expect(titleInput, "title input of the new interview").toBeVisible({ timeout: LOADING_TIMEOUT })
    await titleInput.fill(`${NAME_PREFIX} No Upload`)
    await titleInput.blur()

    // Who was spoken to and who spoke to them are both required, so an interview cannot be saved
    // without them.
    await pickReference(page, field(page, PROPERTY_IDS.HAS_INTERVIEWEE), INTERVIEWEE_QUERY, INTERVIEWEE_ID, "the interviewee of the new interview")
    await pickReference(page, field(page, PROPERTY_IDS.HAS_INTERVIEWER), INTERVIEWER_QUERY, INTERVIEWER_ID, "the interviewer of the new interview")

    const recording = field(page, PROPERTY_IDS.AUDIO)
    await expect(recording.locator(".pd-inputfile-button-browse"), "browse button offered to the role which may upload").toBeVisible()
    await expect(recording.locator(".pd-inputfile-text-nopermission"), "refusal shown to the role which may upload").toHaveCount(0)
    await checkpointElement(page, recording, "attachments-nopermission-allowed")

    const id = await saveEdit(page)

    // The ethics role is granted changing an interview but is granted nothing on files, so the same form
    // says so where the control to pick a file would be.
    await signIn(page, [NO_UPLOAD_ROLE])
    await openDocument(page, id)
    await startEdit(page)
    await hideDuplicates(page)
    await settleFormFocus(page)

    const refused = field(page, PROPERTY_IDS.AUDIO)
    await expect(refused.locator(".pd-inputfile-text-nopermission"), "refusal shown to the role which may not upload").toBeVisible()
    await expect(refused.locator(".pd-inputfile-button-browse"), "browse button offered to the role which may not upload").toHaveCount(0)
    await expect(refused.locator(".pd-inputfile-button-cancel"), "cancel button offered to the role which may not upload").toHaveCount(0)
    await checkpointElement(page, refused, "attachments-nopermission-refused")

    // The same refusal reaches the rich text editor, which offers no way to attach a file either: neither
    // the button which opens the picker nor the input it would pick into is rendered at all, while
    // everything which does not upload anything stays where it was.
    const notes = notesSlot(page)
    await expect(notes.locator(".pd-inputhtml-button-attachfile"), "attach file button offered to the role which may not upload").toHaveCount(0)
    await expect(notes.locator(".pd-inputhtml-input-file"), "file input offered to the role which may not upload").toHaveCount(0)
    await expect(notes.locator(".pd-inputhtml-button-link"), "link button offered to the role which may not upload").toBeVisible()
    await checkpointElement(page, notes, "attachments-nopermission-notes")

    // Nothing was changed, so the session is discarded rather than saved.
    await discardEdit(page)

    console.log("Successfully verified that the role which may not upload is shown the refusal on 1 file field and is offered no attach button on 1 editor.")
  })
})
