import type { Page } from "@playwright/test"

import { unzipSync } from "fflate"
import { readFileSync } from "node:fs"
import { fileURLToPath } from "node:url"

import { documentIdOf } from "../peerdb_utils"
import {
  checkpoint,
  checkpointElement,
  downloadOverlay,
  expect,
  expectResults,
  holdFileRequests,
  LOADING_TIMEOUT,
  PEERDB_URL,
  requestGate,
  settleFilters,
  signIn,
  test,
  volatile,
} from "../utils"

// The role which is granted bulk reading of files, which is what the download buttons are offered to. It is
// the only role of the site other than the administrator which holds that action.
const BULK_ROLE = "bulk"

// The documents the download search is scoped to. Between them they carry every kind of attachment the test
// data has: an image, a record, a paper, a report and a recording. The artifact carries two of them, so the
// results hold more attachments than documents.
const ATTACHED_DOCUMENTS = [
  await documentIdOf("ARTIFACT", "G1_SALT_TABLET"),
  await documentIdOf("PUBLICATION", "PUB_COMPARATIVE_FERMENT"),
  await documentIdOf("EXPEDITION", "EXP_GRID_44_THIRD"),
  await documentIdOf("RESEARCHER", "RES_HALVORSEN"),
  await documentIdOf("NARRATIVE", "G2_UNCLOSED_LAMENT"),
  await documentIdOf("ORGANISM", "G3_SWEEP_PURPLE"),
]

// The attachments those documents link to, by their path inside the test data files directory. The download
// names each file after what the storage advertises for it, which is the name the file is stored under, so
// the archive is checked against these files on disk, content and all.
const ATTACHMENTS = [
  "artifacts/G1_SALT_TABLET.jpg",
  "audio/narrative-G2_UNCLOSED_LAMENT.mp3",
  "organisms/G3_SWEEP_PURPLE.jpg",
  "papers/PUB_COMPARATIVE_FERMENT.pdf",
  "records/G1_SALT_TABLET.txt",
  "reports/EXP_GRID_44_THIRD.pdf",
  "researchers/RES_HALVORSEN.jpg",
]

// Where those files live, resolved from this file rather than from the working directory, so the test does
// not depend on where the suite was started.
const FILES_DIRECTORY = fileURLToPath(new URL("../../testdata/files/", import.meta.url))

// The filename the zip download suggests, which is what the browser then saves it under (the default of
// startZipDownload in src/download.ts).
const ZIP_FILENAME = "download.zip"

// How long the archive is given to reach the browser. The wait for it is armed before the button is pressed,
// because the event is missed otherwise, so it also has to cover everything the test does while the download
// is held back on purpose: two checkpoints, each of which takes a screenshot and runs an accessibility scan.
const DOWNLOAD_TIMEOUT = 3 * LOADING_TIMEOUT

// Takes away the save picker, which is a dialog of the browser itself and cannot be driven from a test.
// Without it the zip download falls back to handing the finished archive to the browser as a download, which
// is what the test then waits for. The directory picker is left alone: the browser has one, which is what
// makes the second download button appear, and this suite asserts what that button looks like rather than
// pressing it, because writing into a directory the picker returned cannot be driven either.
async function useDownloadFallback(page: Page): Promise<void> {
  await page.addInitScript(() => {
    Reflect.deleteProperty(window, "showSaveFilePicker")
  })
}

// Opens a search scoped to the documents which have an attachment. The download buttons act on the results of
// the search, so this is the search which has every attachment of the set to download.
async function openAttachmentSearch(page: Page): Promise<void> {
  await page.goto(`${PEERDB_URL}/s?${ATTACHED_DOCUMENTS.map((id) => `id=${id}`).join("&")}`)
  await expectResults(page)
  await expect(page.locator(".pd-searchresult"), "the search is scoped to the documents which have an attachment").toHaveCount(ATTACHED_DOCUMENTS.length)
}

// The two download buttons of the results header.
function downloadZipButton(page: Page) {
  return page.locator(".pd-searchresultsheader-button-downloadzip")
}

function downloadFilesButton(page: Page) {
  return page.locator(".pd-searchresultsheader-button-downloadfiles")
}

// Waits until the download has collected everything it downloads and is about to start, which is the state
// the held-back metadata requests keep it in.
async function expectPreparing(page: Page): Promise<void> {
  await expect(downloadOverlay(page), "the download downloadOverlay").toBeVisible()
  await expect(page.locator(".pd-downloadoverlay-text-preparing"), "the download says it is collecting the files").toBeVisible()
  await expect(page.locator(".pd-downloadoverlay-progress"), "the download shows how far it has come").toBeVisible()
}

// Waits until the download is at its last attachment, which is the state the held-back content request keeps
// it in, and where it names the attachment it is on.
async function expectDownloadingLast(page: Page): Promise<void> {
  await expect(downloadOverlay(page), "the download downloadOverlay").toBeVisible()
  await expect(page.locator(".pd-downloadoverlay-text-downloading"), "the download says it is downloading").toBeVisible()
  await expect(page.locator(".pd-downloadoverlay-text-file"), "the download names the file it is on").toBeVisible()
  await expect(page.locator(".pd-downloadoverlay-error"), "the download reports no error").toHaveCount(0)
}

test.describe("PeerDB Search Download Flows", () => {
  test("Test the download buttons are offered only to a caller who may bulk read files", async ({ context }) => {
    const page = await context.newPage()

    // Bulk downloading the attachments of a whole result set is enumeration, so it is offered only to a
    // caller who was granted it, and a visitor is not one.
    await openAttachmentSearch(page)
    await expect(downloadZipButton(page), "a visitor is not offered the archive download").toHaveCount(0)
    await expect(downloadFilesButton(page), "a visitor is not offered the directory download").toHaveCount(0)
    await expect(page.locator(".pd-searchresultsheader-group-download"), "a visitor is offered no download group at all").toHaveCount(0)
    await checkpointElement(page, page.locator(".pd-searchresultsheader-toolbar"), "download-toolbar-visitor")

    await signIn(page, [BULK_ROLE])
    await openAttachmentSearch(page)
    await expect(downloadZipButton(page), "the archive download is offered to a caller who may bulk read files").toBeVisible()
    await expect(downloadFilesButton(page), "the directory download is offered to a caller who may bulk read files").toBeVisible()
    await checkpointElement(page, page.locator(".pd-searchresultsheader-toolbar"), "download-toolbar-bulk")

    console.log(`Successfully verified that the two download buttons are offered to the ${BULK_ROLE} role and to nobody else.`)
  })

  test("Test downloading the attachments of the results as a zip archive", async ({ context }) => {
    const page = await context.newPage()

    await useDownloadFallback(page)
    await signIn(page, [BULK_ROLE])
    await openAttachmentSearch(page)
    await settleFilters(page)
    await checkpoint(page, "download-zip-search-results", { mask: volatile(page) })

    const metadata = requestGate()
    const lastContent = requestGate()
    await holdFileRequests(page, ATTACHMENTS.length, metadata, lastContent)

    // The archive is handed to the browser as a download once it is assembled, so the download is waited for
    // from the moment the button is pressed.
    const download = page.waitForEvent("download", { timeout: DOWNLOAD_TIMEOUT })
    await expect(downloadZipButton(page), "the archive download button").toBeVisible()
    await downloadZipButton(page).click()

    await expectPreparing(page)
    await checkpoint(page, "download-zip-downloadOverlay-preparing", { mask: volatile(page), fullPage: false })

    metadata.open()
    await expectDownloadingLast(page)
    await checkpoint(page, "download-zip-downloadOverlay-downloading", { mask: volatile(page), fullPage: false })

    lastContent.open()
    const zip = await download
    expect(zip.suggestedFilename(), "the archive is offered under the name the download gives it").toBe(ZIP_FILENAME)

    // The archive has to hold every attachment of the results, each of them under the name the storage
    // advertises for it and with the bytes of the file it was uploaded from.
    const path = await zip.path()
    const entries = unzipSync(new Uint8Array(readFileSync(path)))
    const expected = ATTACHMENTS.map((attachment) => attachment.slice(attachment.lastIndexOf("/") + 1))
    expect(Object.keys(entries).sort(), "the archive holds every attachment of the results").toEqual([...expected].sort())
    for (const attachment of ATTACHMENTS) {
      const name = attachment.slice(attachment.lastIndexOf("/") + 1)
      const stored = readFileSync(FILES_DIRECTORY + attachment)
      expect(Buffer.from(entries[name]).equals(stored), `the archived ${name} is the file it was uploaded from`).toBe(true)
    }

    // The downloadOverlay closes itself once the download is over, leaving the results as they were.
    await expect(downloadOverlay(page), "the downloadOverlay closes itself once the download is over").toBeHidden()
    await expect(downloadZipButton(page), "the archive download is offered again").toBeEnabled()
    await settleFilters(page)
    await checkpoint(page, "download-zip-search-results-after", { mask: volatile(page) })

    console.log(`Successfully downloaded the ${ATTACHMENTS.length} attachments of ${ATTACHED_DOCUMENTS.length} documents as a zip archive.`)
  })

  test("Test the directory download button", async ({ context }) => {
    const page = await context.newPage()

    await useDownloadFallback(page)
    await signIn(page, [BULK_ROLE])
    await openAttachmentSearch(page)
    await expect(downloadFilesButton(page), "the directory download is offered").toBeVisible()
    await expect(downloadFilesButton(page), "the directory download is offered while nothing is downloading").toBeEnabled()

    // The download writes each file into a directory the user picks, so it is offered only where the browser
    // has a directory picker to pick it with. Taking the picker away is what a browser without one looks
    // like, and the archive download, which needs no directory, stays.
    const withoutPicker = await context.newPage()
    await withoutPicker.addInitScript(() => {
      Reflect.deleteProperty(window, "showDirectoryPicker")
    })
    await openAttachmentSearch(withoutPicker)
    await expect(downloadZipButton(withoutPicker), "the archive download is offered without a directory picker").toBeVisible()
    await expect(downloadFilesButton(withoutPicker), "the directory download is not offered without a directory picker").toHaveCount(0)
    await checkpointElement(withoutPicker, withoutPicker.locator(".pd-searchresultsheader-toolbar"), "download-toolbar-without-directory-picker")
    await withoutPicker.close()

    // Both buttons act on the same results and only one download runs at a time, so a download started by
    // either of them disables both for as long as it lasts. The download is held at its first step so that
    // the disabled state can be asserted and screenshotted while it is in effect. The directory download
    // itself is not driven: picking a directory is a dialog of the browser which a test cannot answer.
    const metadata = requestGate()
    const lastContent = requestGate()
    await holdFileRequests(page, ATTACHMENTS.length, metadata, lastContent)

    const download = page.waitForEvent("download", { timeout: DOWNLOAD_TIMEOUT })
    await downloadZipButton(page).click()
    await expectPreparing(page)
    await expect(downloadFilesButton(page), "the directory download is refused while a download runs").toBeDisabled()
    await expect(downloadZipButton(page), "the archive download is refused while a download runs").toBeDisabled()
    await checkpointElement(page, page.locator(".pd-searchresultsheader-toolbar"), "download-toolbar-downloading")

    metadata.open()
    lastContent.open()
    await download
    await expect(downloadOverlay(page), "the downloadOverlay closes itself once the download is over").toBeHidden()
    await expect(downloadFilesButton(page), "the directory download is offered again once the download is over").toBeEnabled()
    await expect(downloadZipButton(page), "the archive download is offered again once the download is over").toBeEnabled()
    await checkpointElement(page, page.locator(".pd-searchresultsheader-toolbar"), "download-toolbar-after-download")

    console.log("Successfully verified the directory download button: offered with a directory picker, hidden without one, disabled while a download runs.")
  })
})
