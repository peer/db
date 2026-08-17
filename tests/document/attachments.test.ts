import type { Locator, Page } from "@playwright/test"

import { statSync } from "node:fs"
import { dirname, join } from "node:path"
import { fileURLToPath } from "node:url"

import { documentIdOf, filePathOf, PROPERTY_IDS } from "../peerdb_utils"
import { checkpoint, checkpointElement, expect, fetchFromPage, openDocument, openDocumentTab, propertyValues, settle, test, volatile } from "../utils"

// The documents whose attachments are looked at, addressed by their document identifier so that the same
// document is opened on every run. The artifact carries both an image and a record of its accession, the
// communication system carries a recording with everything a recording can be annotated with, and the
// publication carries the paper itself.
const ARTIFACT_ID = await documentIdOf("ARTIFACT", "G1_SALT_TABLET")
const COMMUNICATION_SYSTEM_ID = await documentIdOf("COMMUNICATION_SYSTEM", "G4_COM_TERRACE_REGISTER")
const PUBLICATION_ID = await documentIdOf("PUBLICATION", "PUB_COMPARATIVE_FERMENT")

// The attachments themselves, named by their path inside the test data files directory. A file is stored under
// an identifier derived from that path, so the address it is served from follows from the path alone and a test
// does not have to read it out of the document to know where the link has to point.
const ARTIFACT_IMAGE = "artifacts/G1_SALT_TABLET.jpg"
const ARTIFACT_RECORD = "records/G1_SALT_TABLET.txt"
const SYSTEM_RECORDING = "audio/system-G4_COM_TERRACE_REGISTER.mp3"
const PUBLICATION_PAPER = "papers/PUB_COMPARATIVE_FERMENT.pdf"

// The directory the attachments were populated from, so that what the server sends can be compared against the
// file it was populated with. It is derived from this file's own location rather than from the working
// directory, which the test process does not control.
const FILES_DIR = join(dirname(fileURLToPath(import.meta.url)), "..", "..", "testdata", "files")

// The name a file is linked under, which is the last segment of its path inside the test data files directory.
function fileNameOf(path: string): string {
  return path.split("/").pop()!
}

// The value cells of the sub-claims of one attachment which are for the given property, which is how the name,
// the caption, the source and the duration recorded next to a file are addressed. Sub-claims render as a table
// of their own in a row below the claim they hang off, and that row carries the property of the claim they hang
// off in its class name. Defined here rather than in the shared helpers because this file and the claim values
// one are the only ones which reach into a claim's sub-claims by the property of its parent.
function attachmentDetail(page: Page, attachmentPropertyId: string, propertyId: string): Locator {
  return page.locator(
    `.pd-documentget-panel-allproperties .pd-propertiesview-row-sub-${attachmentPropertyId} .pd-propertiesview-row-${propertyId} .pd-propertiesview-value`,
  )
}

// The table holding everything recorded about one attachment. A row of the properties table is laid out as
// contents rather than as a box of its own, so the table inside it is what a screenshot of the sub-claims can
// be taken of.
function attachmentDetails(page: Page, attachmentPropertyId: string): Locator {
  return page.locator(`.pd-documentget-panel-allproperties .pd-propertiesview-row-sub-${attachmentPropertyId} .pd-propertiesview`).first()
}

// Opens a document on its "all properties" tab, which is where a file claim is shown together with everything
// recorded about the file next to it.
async function openAllProperties(page: Page, id: string, name: string): Promise<void> {
  await openDocument(page, id)
  await settle(page)
  await openDocumentTab(page, "allproperties")
  await settle(page)
  await expect(page.locator(".pd-documentget-panel-allproperties .pd-propertiesview-row").first(), `rows of the all properties tab of ${name}`).toBeVisible()
  await checkpoint(page, name, { mask: volatile(page) })
}

// Asserts that the link of an attachment points at the file the given path was populated from and shows the
// name of that file rather than the address it is served from, and returns the link.
async function expectAttachmentLink(page: Page, propertyId: string, path: string, what: string): Promise<Locator> {
  const values = propertyValues(page, propertyId)
  await expect(values, `${what} claims`).toHaveCount(1)
  const link = values.locator(".pd-claimvaluelink")
  await expect(link, `the link of ${what}`).toBeVisible()
  await expect(link, `the link of ${what} points at the stored file`).toHaveAttribute("href", await filePathOf(path))
  await expect(link, `the link of ${what} shows the name of the file`).toHaveText(fileNameOf(path))
  // A file of this instance is marked as a file rather than as a page, which is what the icon next to it is
  // drawn from, and it does not route inside the application because a file has no view of its own.
  await expect(link, `the link of ${what} is marked as a file`).toHaveClass(/pd-link-file/)
  return link
}

// Asserts that the file the given path was populated from is served under the name it was populated with, with
// the media type it holds and in full, and returns how many bytes came back.
async function expectFileServed(page: Page, path: string, mediaType: RegExp, what: string): Promise<number> {
  const response = await fetchFromPage(page, await filePathOf(path))
  expect(response.status, `the status the server answers for ${what} with`).toBe(200)
  expect(response.headers["content-type"], `the media type ${what} is served as`).toMatch(mediaType)
  // The name is carried in the header which says how to present the file, so a download of it lands under the
  // name it was stored with rather than under the identifier it is addressed by.
  expect(response.headers["content-disposition"], `the name ${what} is served under`).toContain(fileNameOf(path))
  // What came back is compared against the file on disk, so a truncated or a padded answer is caught rather
  // than only an empty one.
  expect(response.length, `the number of bytes of ${what}`).toBe(statSync(join(FILES_DIR, path)).size)
  return response.length
}

test.describe("PeerDB Document Attachments Flows", () => {
  test("Test the image and the record attached to an artifact", async ({ context }) => {
    const page = await context.newPage()

    await openAllProperties(page, ARTIFACT_ID, "attachments-artifact-allproperties")

    // The image of the artifact: a file claim whose address is derived from the path the file was populated
    // from, shown under the name of the file.
    const imageLink = await expectAttachmentLink(page, PROPERTY_IDS.IMAGE, ARTIFACT_IMAGE, "the image of the artifact")
    await expect(imageLink, "the link of the image of the artifact does not show the address it is served from").not.toHaveText(/\/f\//)
    await checkpointElement(page, propertyValues(page, PROPERTY_IDS.IMAGE), "attachments-artifact-image")

    // What is recorded about the image hangs off the image claim as sub-claims: the name the file was stored
    // under, the caption to show it with and where it came from. The caption and the source are the two which
    // are about the picture rather than about the file.
    await expect(attachmentDetail(page, PROPERTY_IDS.IMAGE, PROPERTY_IDS.NAME), "the name recorded for the image").toHaveText(fileNameOf(ARTIFACT_IMAGE))
    const imageCaption = attachmentDetail(page, PROPERTY_IDS.IMAGE, PROPERTY_IDS.CAPTION)
    await expect(imageCaption, "the caption recorded for the image").toHaveCount(1)
    await expect(imageCaption, "the caption recorded for the image says something").not.toHaveText(/^\s*$/)
    const imageSource = attachmentDetail(page, PROPERTY_IDS.IMAGE, PROPERTY_IDS.SOURCE)
    await expect(imageSource, "the source recorded for the image").toHaveCount(1)
    await expect(imageSource, "the source recorded for the image says something").not.toHaveText(/^\s*$/)
    // The source is recorded as text and rendered as markup, because it may name a register and link to it.
    await expect(imageSource.locator(".pd-claimvaluehtml"), "the source recorded for the image is rendered as HTML").toHaveCount(1)
    await checkpointElement(page, attachmentDetails(page, PROPERTY_IDS.IMAGE), "attachments-artifact-image-details")

    // The record of the accession of the artifact: a file of a different kind, on the same document, annotated
    // with a name and a caption but with no source, because the register wrote it itself.
    await expectAttachmentLink(page, PROPERTY_IDS.ATTACHED_DOCUMENT, ARTIFACT_RECORD, "the record attached to the artifact")
    await expect(attachmentDetail(page, PROPERTY_IDS.ATTACHED_DOCUMENT, PROPERTY_IDS.NAME), "the name recorded for the record").toHaveText(fileNameOf(ARTIFACT_RECORD))
    await expect(attachmentDetail(page, PROPERTY_IDS.ATTACHED_DOCUMENT, PROPERTY_IDS.CAPTION), "the caption recorded for the record").toHaveCount(1)
    await expect(attachmentDetail(page, PROPERTY_IDS.ATTACHED_DOCUMENT, PROPERTY_IDS.SOURCE), "the record carries no source").toHaveCount(0)
    await checkpointElement(page, propertyValues(page, PROPERTY_IDS.ATTACHED_DOCUMENT), "attachments-artifact-record")

    // Both files are served by the instance under the name they were stored with, as what they are and whole.
    const imageBytes = await expectFileServed(page, ARTIFACT_IMAGE, /^image\/jpeg$/, "the image of the artifact")
    const recordBytes = await expectFileServed(page, ARTIFACT_RECORD, /^text\/plain/, "the record attached to the artifact")

    console.log(
      `Successfully verified the two attachments of an artifact: an image of ${imageBytes} bytes and a record of ${recordBytes} bytes, each linked under its own name.`,
    )
  })

  test("Test the recording attached to a communication system", async ({ context }) => {
    const page = await context.newPage()

    await openAllProperties(page, COMMUNICATION_SYSTEM_ID, "attachments-system-allproperties")

    // The recording of the system: a file claim like the image and the record, so a reader reaches audio the
    // same way as anything else which was attached.
    await expectAttachmentLink(page, PROPERTY_IDS.AUDIO, SYSTEM_RECORDING, "the recording of the communication system")
    await checkpointElement(page, propertyValues(page, PROPERTY_IDS.AUDIO), "attachments-system-recording")

    // A recording carries more than a picture does: besides the name and the caption it records how long it
    // runs and a note about how it was made.
    await expect(attachmentDetail(page, PROPERTY_IDS.AUDIO, PROPERTY_IDS.NAME), "the name recorded for the recording").toHaveText(fileNameOf(SYSTEM_RECORDING))
    await expect(attachmentDetail(page, PROPERTY_IDS.AUDIO, PROPERTY_IDS.CAPTION), "the caption recorded for the recording").toHaveCount(1)
    await expect(attachmentDetail(page, PROPERTY_IDS.AUDIO, PROPERTY_IDS.CAPTION), "the caption recorded for the recording says something").not.toHaveText(/^\s*$/)
    const duration = attachmentDetail(page, PROPERTY_IDS.AUDIO, PROPERTY_IDS.DURATION)
    await expect(duration, "the duration recorded for the recording").toHaveCount(1)
    await expect(duration.locator(".pd-claimvalueamount"), "the duration recorded for the recording is a number").toHaveText(/^\d+(\.\d+)?$/)
    // The duration is an amount and carries the unit it was measured in as a sub-claim of its own, so the
    // annotations of a file nest as deeply as any other claim does.
    await expect(
      page.locator(`.pd-documentget-panel-allproperties .pd-propertiesview-row-sub-${PROPERTY_IDS.DURATION} .pd-propertiesview-row-${PROPERTY_IDS.IN_UNIT}`),
      "the unit the duration was measured in",
    ).toHaveCount(1)
    await expect(attachmentDetail(page, PROPERTY_IDS.AUDIO, PROPERTY_IDS.NOTES), "the note recorded for the recording").toHaveCount(1)
    await checkpointElement(page, attachmentDetails(page, PROPERTY_IDS.AUDIO), "attachments-system-recording-details")

    const bytes = await expectFileServed(page, SYSTEM_RECORDING, /^audio\/mpeg$/, "the recording of the communication system")

    console.log(
      `Successfully verified the recording attached to a communication system: ${bytes} bytes of audio, linked under its own name and annotated with its duration.`,
    )
  })

  test("Test every file linked from a document is served under the name its link shows", async ({ context }) => {
    const page = await context.newPage()

    await openAllProperties(page, PUBLICATION_ID, "attachments-publication-allproperties")

    // The paper itself is attached to the publication, which is the third kind of file the test data carries.
    await expectAttachmentLink(page, PROPERTY_IDS.ATTACHED_DOCUMENT, PUBLICATION_PAPER, "the paper attached to the publication")
    await expect(attachmentDetail(page, PROPERTY_IDS.ATTACHED_DOCUMENT, PROPERTY_IDS.CAPTION), "the caption recorded for the paper").toHaveCount(1)
    await checkpointElement(page, propertyValues(page, PROPERTY_IDS.ATTACHED_DOCUMENT), "attachments-publication-paper")
    const bytes = await expectFileServed(page, PUBLICATION_PAPER, /^application\/pdf$/, "the paper attached to the publication")

    // Whatever else the document links to, every link to a file of this instance has to lead to a file which is
    // there and which is served under the name the link shows, so no attachment is left pointing at nothing.
    const fileLinks = page.locator(".pd-documentget-panel-allproperties .pd-claimvaluelink.pd-link-file")
    const count = await fileLinks.count()
    expect(count, "file links of the publication").toBeGreaterThan(0)
    for (let i = 0; i < count; i++) {
      const link = fileLinks.nth(i)
      const href = (await link.getAttribute("href"))!
      const name = (await link.textContent())!.trim()
      expect(href, `file link ${i} is addressed by an identifier`).toMatch(/^\/f\/[0-9A-Za-z]+$/)
      const response = await fetchFromPage(page, href)
      expect(response.status, `the status the server answers for file link ${i} with`).toBe(200)
      expect(response.length, `the number of bytes of file link ${i}`).toBeGreaterThan(0)
      expect(response.headers["content-disposition"], `the name file link ${i} is served under`).toContain(name)
    }

    console.log(
      `Successfully verified that all ${count} files linked from a publication are served under the names their links show, the paper of ${bytes} bytes among them.`,
    )
  })
})
