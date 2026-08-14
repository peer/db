import type { Locator, Page } from "@playwright/test"

import en from "@/locales/en.json" with { type: "json" }
import pt from "@/locales/pt.json" with { type: "json" }
import sl from "@/locales/sl.json" with { type: "json" }
import { CLASS_IDS, LANGUAGES, PROPERTY_IDS, startCreate } from "../peerdb_utils"
import {
  checkpoint,
  checkpointElement,
  clearRefusedRequestErrors,
  discardCreate,
  discardEdit,
  documentId,
  editingDocumentId,
  expect,
  fetchFromPage,
  fillSlot,
  goHome,
  hideDuplicates,
  LOADING_TIMEOUT,
  openDocument,
  openUserMenu,
  PEERDB_URL,
  saveEdit,
  settle,
  settleDocument,
  settleEdit,
  signIn,
  slotInput,
  startEdit,
  switchLanguage,
  test,
  volatile,
} from "../utils"

// The class every session this file opens is about. A biome is one of the controlled vocabularies of the
// test data, so a document of it is created by filling in a single name, and creating one never touches a
// document which already exists.
const VOCABULARY_CLASS = "BIOME"

// The roles each test here signs in with. The sessions view lists the sessions of the user who is signed in,
// and the mock authenticator gives every set of roles a user of its own, so a set which no other test file
// signs in with keeps the list under the control of this file alone. The tests of this file run next to each
// other as well, and each of them opens a session, so each signs in as a user of its own too: sharing one
// would have every test see the sessions the others are in the middle of. Curator is the role which may
// create a vocabulary entry (ROLE_CREATES in peerdb_utils); every other role in a set is there only to tell
// one user from another and grants nothing these tests use.
const SESSION_ROLES = {
  none: ["bulk", "curator"],
  creating: ["author", "curator"],
  editing: ["curator", "surveyor"],
  discarding: ["curator", "ethics"],
  ended: ["author", "bulk", "curator"],
  saved: ["bulk", "curator", "surveyor"],
} as const

// Every document this file creates is named beginning with this, so that the documents of one test file
// never collide with another's and a document left behind says which file made it.
const PREFIX = "EditSession"

// The interface messages the tests read back, taken from the application's own translations, so that a label
// which differs between the three languages the site is served in is not repeated here.
const LOCALES = { en, sl, pt }

// The items of the sessions view, one per session the signed-in user can still continue.
function sessionItems(page: Page): Locator {
  return page.locator(".pd-documentsessions-item")
}

// The item of the sessions view which is for the given session, addressed by the address its links carry, so
// that a test finds its own session among whatever else the user has open.
function sessionItem(page: Page, sessionPath: string): Locator {
  return sessionItems(page).filter({ has: page.locator(`.pd-documentsessions-button-open[href="${sessionPath}"]`) })
}

// The path of an editing session, which is what the sessions view links to and what a test compares against
// to tell one session from another.
function sessionPath(page: Page): string {
  const match = /(\/d\/edit\/[0-9A-Za-z]+\/[0-9A-Za-z]+)(?:[?#]|$)/.exec(page.url())
  expect(match, `the browser is on an editing session: ${page.url()}`).not.toBeNull()
  return match![1]
}

// Opens the sessions view and waits for the list of sessions to be there, whether it holds anything or not.
// The view is reached through the menu of the signed-in user, which is where the site offers it.
async function openSessions(page: Page): Promise<void> {
  await goHome(page)
  await openUserMenu(page)
  const link = page.locator(".pd-navbaruser-sessions")
  await expect(link, "the link to the sessions view in the user's menu").toBeVisible()
  await link.click()
  await expect(page.locator(".pd-documentsessions"), "the sessions view").toBeVisible({ timeout: LOADING_TIMEOUT })
  await expect(page.locator("#documentsessions-loading"), "the sessions view while it is loading").toHaveCount(0, { timeout: LOADING_TIMEOUT })
  await settle(page)
}

// Opens a create session of the vocabulary class and gives the document being created a name, so that the
// session is told apart from any other by what it is about.
async function startNamedCreate(page: Page, name: string): Promise<void> {
  await startCreate(page, VOCABULARY_CLASS)
  await hideDuplicates(page)
  await fillSlot(page, PROPERTY_IDS.NAME, 0, ".pd-inputstring", name, 2, "name of the document being created")
  await expect(page.locator("#documentedit-title"), "title of the document being created").toHaveText(name)
}

// The two lines of a session item which say when it was begun and when it was last changed. They are the
// only part of the view which follows the clock, so they are masked out of every screenshot of it.
function sessionTimes(page: Page): Array<Locator> {
  return [page.locator(".pd-documentsessions-text-started"), page.locator(".pd-documentsessions-text-lastchange")]
}

test.describe("PeerDB Editing Session Flows", () => {
  test("Test the sessions view says so when the user has no session to continue", async ({ context }) => {
    // Every session this file opens is ended by the test which opened it, so the user starts with none. A
    // run which was interrupted can leave one behind, which this test closes before asserting the empty
    // view, so that the file always starts from the state it expects.
    test.slow()

    const page = await context.newPage()

    await signIn(page, SESSION_ROLES.none)
    await openSessions(page)

    let left = await sessionItems(page).count()
    for (let i = 0; i < left; i++) {
      const open = sessionItems(page).first().locator(".pd-documentsessions-button-open")
      await expect(open, "the button which continues a session left behind").toBeVisible()
      await open.click()
      await settleEdit(page)
      const discardButton = page.locator("#documentedit-button-discard")
      await expect(discardButton, "discard button of a session left behind").toBeVisible()
      await discardButton.focus()
      await discardButton.click()
      // A discarded session goes back to the document it was editing or, when it was creating one, to the
      // class tree, so what is waited for is that the session is no longer open.
      await expect.poll(() => page.url().includes("/d/edit/"), { message: "the session left behind was discarded", timeout: LOADING_TIMEOUT }).toBe(false)
      await openSessions(page)
    }
    left = await sessionItems(page).count()

    // With nothing to continue the view says so instead of showing an empty list.
    await expect(sessionItems(page), "sessions of a user who has none").toHaveCount(0)
    await expect(page.locator("#documentsessions-empty"), "the message of a sessions view with nothing in it").toBeVisible()
    await expect(page.locator("#documentsessions-title"), "title of the sessions view").toBeVisible()
    await checkpoint(page, "editsessions-view-empty")

    console.log(`Successfully emptied the sessions view of the user, which listed ${left} sessions to continue afterwards.`)
  })

  test("Test the sessions view lists a session which is creating a document and continues it", async ({ context }) => {
    const page = await context.newPage()

    const name = `${PREFIX} Listed Biome`

    await signIn(page, SESSION_ROLES.creating)

    await openSessions(page)
    const before = await sessionItems(page).count()

    await startNamedCreate(page, name)
    const path = sessionPath(page)
    const id = editingDocumentId(page)

    // The session is listed as soon as it exists, and it is named by what has been put into it rather than
    // by what is stored of it, which for a document being created is nothing at all.
    await openSessions(page)
    await expect(sessionItems(page), "sessions after one was opened").toHaveCount(before + 1)
    const item = sessionItem(page, path)
    await expect(item, "the item of the session which was opened").toHaveCount(1)
    await expect(item.locator(".pd-documentsessions-link-document"), "the document the session is about").toHaveText(name)
    await expect(item.locator(".pd-documentsessions-tags").locator("li").first(), "the tag saying what the session does").toHaveText(
      LOCALES.en.views.DocumentSessions.create,
    )
    await expect(item.locator(".pd-documentsessions-text-started"), "the line saying when the session was begun").toBeVisible()
    await expect(item.locator(".pd-documentsessions-text-lastchange"), "the line saying when the session last changed").toBeVisible()
    await checkpointElement(page, item, "editsessions-item-create", sessionTimes(page))

    // Continuing the session goes back to the form with everything which was put into it.
    await item.locator(".pd-documentsessions-button-open").click()
    await settleEdit(page)
    expect(sessionPath(page), "the session which was continued").toBe(path)
    await expect(slotInput(page, PROPERTY_IDS.NAME, 0, ".pd-inputstring"), "name of the continued session").toHaveValue(name)
    await expect(page.locator("#documentedit-title"), "title of the continued session").toHaveText(name)

    // Discarding it takes it off the list, so what is listed is what can still be continued.
    await discardCreate(page)
    await openSessions(page)
    await expect(sessionItems(page), "sessions after the one which was opened was discarded").toHaveCount(before)
    await expect(sessionItem(page, path), "the item of the discarded session").toHaveCount(0)

    // The document the discarded session would have saved into was never stored.
    const missing = await fetchFromPage(page, `/api/d/${id}`)
    expect(missing.status, "the document of the discarded create session").toBe(404)
    clearRefusedRequestErrors(page)

    console.log(`Successfully listed, continued and discarded a create session, which took the sessions view from ${before} items to ${before + 1} and back.`)
  })

  test("Test the sessions view lists a session which is editing a document and continues it", async ({ context }) => {
    const page = await context.newPage()

    const name = `${PREFIX} Edited Biome`
    const code = "EDITSESSION-EDITED"

    await signIn(page, SESSION_ROLES.editing)

    // The document is created first so that the session below edits a document this test owns.
    await startNamedCreate(page, name)
    const id = await saveEdit(page)

    await openSessions(page)
    const before = await sessionItems(page).count()

    await openDocument(page, id)
    await settleDocument(page)
    await startEdit(page)
    const path = sessionPath(page)
    await fillSlot(page, PROPERTY_IDS.CODE, 0, ".pd-inputidentifier", code, 2, "code of the document being edited")

    await openSessions(page)
    await expect(sessionItems(page), "sessions after an edit session was opened").toHaveCount(before + 1)
    const item = sessionItem(page, path)
    await expect(item, "the item of the edit session").toHaveCount(1)
    await expect(item.locator(".pd-documentsessions-link-document"), "the document the edit session is about").toHaveText(name)
    await expect(item.locator(".pd-documentsessions-tags").locator("li").first(), "the tag saying what the session does").toHaveText(
      LOCALES.en.views.DocumentSessions.edit,
    )
    await checkpointElement(page, item, "editsessions-item-edit", sessionTimes(page))

    // The title of the item is a link to the session as much as the button next to it is.
    await item.locator(".pd-documentsessions-link-document").click()
    await settleEdit(page)
    expect(sessionPath(page), "the session which was continued").toBe(path)
    // A change which was made before the session was left is still in it, because a session holds the
    // changes rather than the form which was open when they were made.
    await expect(slotInput(page, PROPERTY_IDS.CODE, 0, ".pd-inputidentifier"), "code of the continued session").toHaveValue(code)
    await expect(page.locator("#documentedit-button-save"), "save button of the continued session").toBeEnabled()
    await checkpointElement(page, page.locator(".pd-fieldsform"), "editsessions-form-continued")

    // Discarding an edit session goes back to the document it was editing, which is left as it was.
    await discardEdit(page)
    expect(documentId(page), "the document a discarded edit session goes back to").toBe(id)
    await expect(page.locator("#documentget-title"), "title of the document the discarded session was editing").toHaveText(name)
    await expect(page.locator(".pd-documentget-panel-properties .pd-fieldsview-value"), "values of the document the discarded session was editing").toHaveText([name])
    await checkpoint(page, "editsessions-document-after-discard", { mask: volatile(page) })

    await openSessions(page)
    await expect(sessionItems(page), "sessions after the edit session was discarded").toHaveCount(before)

    console.log(`Successfully listed, continued and discarded an edit session of document ${id}, which left its 1 value unchanged.`)
  })

  test("Test discarding a session which is creating a document goes back to the create page", async ({ context }) => {
    const page = await context.newPage()

    const name = `${PREFIX} Discarded Biome`

    await signIn(page, SESSION_ROLES.discarding)

    await startNamedCreate(page, name)
    const id = editingDocumentId(page)
    await checkpointElement(page, page.locator(".pd-documentedit-actions"), "editsessions-create-actions")

    // A document which is being created has no document view to go back to, so discarding goes back to the
    // page the creation was started from, ready for another one.
    await discardCreate(page)
    expect(new URL(page.url()).pathname, "the address a discarded create session goes back to").toBe("/d/create")
    await expect(page.locator(`.pd-classtreelabel-button-${CLASS_IDS[VOCABULARY_CLASS]}`).first(), "the class of the discarded document is offered again").toBeVisible({
      timeout: LOADING_TIMEOUT,
    })
    await expect(page.locator("#documentedit-error-load"), "an error in place of the create page").toHaveCount(0)
    await settle(page)
    // The classes are ranked by a search whose scores follow the term statistics of the whole index, which
    // every document the suite writes changes, so the tree is masked. The capture is of the viewport and not
    // of the whole page, because the tree also changes height as its labels wrap differently, which moves
    // everything below it and makes a full page differ however well the tree itself is masked.
    await checkpoint(page, "editsessions-create-page-after-discard", { fullPage: false, mask: [page.locator(".pd-classtreelist")] })

    // Nothing was stored of the document the discarded session was about.
    const missing = await fetchFromPage(page, `/api/d/${id}`)
    expect(missing.status, "the document of the discarded create session").toBe(404)
    clearRefusedRequestErrors(page)

    console.log(`Successfully discarded a create session of document ${id} and landed back on the create page.`)
  })

  test("Test a session which has ended says so instead of showing a form", async ({ context }) => {
    const page = await context.newPage()

    const name = `${PREFIX} Ended Biome`

    await signIn(page, SESSION_ROLES.ended)

    await startNamedCreate(page, name)
    const id = await saveEdit(page)

    await startEdit(page)
    const path = sessionPath(page)

    // The sessions view is loaded while the session is still open, so its list holds a session which the
    // step below ends. This is how a user reaches a session which is no longer there: through a list which
    // was made before it ended. The second page shares the browser's cookies, so it is the same user.
    const listing = await context.newPage()
    await openSessions(listing)
    const item = sessionItem(listing, path)
    await expect(item, "the item of the session which is about to end").toHaveCount(1)

    await discardEdit(page)
    expect(documentId(page), "the document the discarded session goes back to").toBe(id)

    // Opening the session which ended shows what happened to it, with a link to the document it was about,
    // and no form to go on editing in.
    await item.locator(".pd-documentsessions-button-open").click()
    const ended = listing.locator("#documentedit-text-sessionended")
    await expect(ended, "the message saying the session has ended").toBeVisible({ timeout: LOADING_TIMEOUT })
    await expect(ended, "the document named by the message").toContainText(name)
    await expect(listing.locator(".pd-fieldsform"), "a form of a session which has ended").toHaveCount(0)
    await expect(listing.locator("#documentedit-button-save"), "the save button of a session which has ended").toHaveCount(0)
    await expect(listing.locator("#documentedit-button-discard"), "the discard button of a session which has ended").toHaveCount(0)
    await expect(listing.locator("#documentedit-error-load"), "an error in place of the message").toHaveCount(0)
    await settle(listing)
    await checkpoint(listing, "editsessions-session-ended")

    // The document the session was about is a link, so the message is also the way back to it.
    const link = ended.locator("a")
    await expect(link, "the link of the message saying the session has ended").toHaveAttribute("href", `/d/${id}`)
    await link.click()
    await settleDocument(listing)
    expect(documentId(listing), "the document the message links to").toBe(id)
    await listing.close()

    console.log(`Successfully opened the ended session of document ${id} and was told it has ended instead of being given a form.`)
  })

  test("Test the address of a session which was saved leads to the document it saved", async ({ context }) => {
    const page = await context.newPage()

    const name = `${PREFIX} Saved Biome`
    const code = "EDITSESSION-SAVED"

    await signIn(page, SESSION_ROLES.saved)

    await startNamedCreate(page, name)
    const path = sessionPath(page)
    const id = await saveEdit(page)
    expect(new URL(page.url()).pathname, "the address a saved create session goes to").toBe(`/d/${id}`)

    // A session which was saved is committed in the background, after which its address is answered with the
    // document it saved rather than with a form. The commit is the last step of saving and is not waited for
    // by the browser, so the address is asked for until it leads there.
    await expect
      .poll(
        async () => {
          await page.goto(`${PEERDB_URL}${path}`)
          return new URL(page.url()).pathname
        },
        { message: "the address of the saved session leads to the document it saved", timeout: LOADING_TIMEOUT },
      )
      .toBe(`/d/${id}`)
    await settleDocument(page)
    await expect(page.locator("#documentget-title"), "title of the document the saved session leads to").toHaveText(name)

    // The same holds for a session which edited a document which already existed.
    await startEdit(page)
    const editPath = sessionPath(page)
    await fillSlot(page, PROPERTY_IDS.CODE, 0, ".pd-inputidentifier", code, 2, "code of the document being edited")
    await saveEdit(page)
    await expect
      .poll(
        async () => {
          await page.goto(`${PEERDB_URL}${editPath}`)
          return new URL(page.url()).pathname
        },
        { message: "the address of the saved edit session leads to the document it saved", timeout: LOADING_TIMEOUT },
      )
      .toBe(`/d/${id}`)
    await settleDocument(page)
    await expect(page.locator(".pd-documentget-panel-properties .pd-fieldsview-value"), "values of the document both sessions saved into").toHaveText([name, code])
    await checkpoint(page, "editsessions-document-after-saved-sessions", { mask: volatile(page) })

    console.log(`Successfully followed the addresses of 2 saved sessions of document ${id} to the document they saved.`)
  })

  test("Test the sessions view of a visitor who is not signed in", async ({ context }) => {
    const page = await context.newPage()

    // The view is about the sessions of whoever is signed in, so without a user it offers nothing and says
    // why. It is checked in every language the site is served in, because the whole view is that one line.
    for (const language of LANGUAGES) {
      await goHome(page)
      await switchLanguage(page, language)
      await page.goto(`${PEERDB_URL}/d/session`)
      await expect(page.locator(".pd-documentsessions"), `the sessions view in ${language}`).toBeVisible({ timeout: LOADING_TIMEOUT })
      const notSignedIn = page.locator("#documentsessions-text-notsignedin")
      await expect(notSignedIn, `the message for a visitor who is not signed in in ${language}`).toHaveText(LOCALES[language].views.DocumentSessions.notSignedIn)
      await expect(page.locator(".pd-documentsessions-list"), `the list of sessions shown to a visitor who is not signed in in ${language}`).toHaveCount(0)
      await expect(page.locator("#documentsessions-title"), `the title of the sessions view of a visitor who is not signed in in ${language}`).toHaveCount(0)
      await settle(page)
      await checkpoint(page, `editsessions-view-notsignedin-${language}`)
    }

    console.log(`Successfully checked the sessions view of a visitor who is not signed in in ${LANGUAGES.length} languages.`)
  })
})
