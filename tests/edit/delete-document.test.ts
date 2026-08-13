import type { Page } from "@playwright/test"

import type { Role } from "../peerdb_utils"

import { createNamed } from "../peerdb_utils"
import {
  checkpoint,
  checkpointElement,
  clearRefusedRequestErrors,
  expect,
  expectNothingLoading,
  goHome,
  LOADING_TIMEOUT,
  openDocument,
  PEERDB_URL,
  resultIds,
  settle,
  settleDocument,
  settleFilters,
  settleSearch,
  signIn,
  signOut,
  test,
  volatile,
} from "../utils"

// The class every test here creates the document it works on. A galaxy is the smallest class of the site:
// a single name makes a saveable document, so a test about deleting does not have to fill a form first.
// Deleting is the one action which takes a document away for good, so every test creates the document it
// deletes and never touches a document of the test data or of another test file.
const GALAXY = "GALAXY"

// The role which is granted deleting on everything (roles in config.yml), and which may also start a
// document of any class, so it is the one identity which can both create and delete what these tests need.
const DELETING_ROLE: Role = "admin"

// A role which may create a galaxy and update every document of the site but is granted no deleting
// anywhere, which is what a document deleting is asserted not to offer it.
const SURVEYING_ROLE: Role = "surveyor"

// The names the tests give the documents they create. They all begin with the same invented prefix, which
// no document of the test data and no other test file carries, so a search for one of them matches the
// document the test created and nothing else. Each test uses a name of its own, so the documents of the
// tests which run next to each other never turn up in the search a deleting test runs.
const CANCEL_NAME = "PDEDELETE Preklicoslovje"
const DELETE_NAME = "PDEDELETE Zbrisoslovje"
const ROLES_NAME = "PDEDELETE Varovanoslovje"
const CREATOR_NAME = "PDEDELETE Izmerjenoslovje"

// The word the deleting test searches for. It is the invented half of the name of the document that test
// creates: a full text query is matched word by word, so searching for the whole name would find every
// document carrying any of its words, the galaxies of the test data included, while this word is carried by
// the created document alone.
const DELETE_QUERY = "Zbrisoslovje"

// How long the index is given to catch up with a write. Indexing runs after the write has been committed,
// so a document becomes searchable, and stops being searchable, some time after the view which wrote it
// has already moved on.
const INDEXING_TIMEOUT = 60000

// What the server answers a caller who asks for a page they hold no action for.
const FORBIDDEN = 403

// One identity a document is looked at as, and whether it is offered deleting it. The roles are the ones
// to sign in with through the mock authenticator, or null for a visitor who does not sign in at all.
interface Identity {
  what: string
  slug: string
  roles: ReadonlyArray<Role> | null
  deletes: boolean
}

// The identities the document created by the deleting role is looked at as, in the order one page walks
// through them. Deleting is granted to the one role which holds it and to nobody else, whatever else they
// may do with the document, so the ones which may change it are here next to the ones which may not.
const IDENTITIES: ReadonlyArray<Identity> = [
  { what: "the role which is granted deleting", slug: "admin", roles: [DELETING_ROLE], deletes: true },
  { what: "a visitor who is not signed in", slug: "visitor", roles: null, deletes: false },
  { what: "a user signed in with no role", slug: "norole", roles: [], deletes: false },
  { what: "a role which may create and update but not delete", slug: "surveyor", roles: [SURVEYING_ROLE], deletes: false },
]

// Puts the browser in the state the identity describes, from whatever state it is in: a page which is
// already signed in has to be signed out first, because the button which signs in is the one the signed-in
// user's own menu replaced.
//
// Whether somebody is signed in is read on the home page and not on whatever page the browser is on. A page
// which has only just been navigated to has not mounted the application yet, and its navbar is therefore
// empty however the visit is authenticated, so reading it there would take a signed-in user for a signed-out
// one and leave the previous user signed in for the rest of the test. The search box of the home page is
// rendered by the application, so waiting for it is waiting for a navbar which says what it knows.
async function become(page: Page, identity: Identity): Promise<void> {
  await goHome(page)
  if ((await page.locator(".pd-navbarmenu-button").count()) > 0) {
    await signOut(page)
  }
  if (identity.roles !== null) {
    await signIn(page, identity.roles)
  }
}

// Presses the delete action of the document currently shown and waits for the confirmation view, card
// included. The delete action lives in the sidebar of the document view, which at the viewport the tests
// run at is always laid out next to the card.
async function openDeleteConfirmation(page: Page): Promise<void> {
  const deleteButton = page.locator("#documentget-button-delete")
  await expect(deleteButton, "delete action of the document").toBeVisible()
  await deleteButton.click()
  await expect(page.locator(".pd-documentdelete"), "delete confirmation view").toBeVisible({ timeout: LOADING_TIMEOUT })
  await expect(page.locator("#documentdelete-button-delete"), "delete button of the confirmation").toBeVisible({ timeout: LOADING_TIMEOUT })
  await settle(page)
}

// Goes through with the deletion of the document the confirmation is open for and waits for the home page
// it lands on. There is no document to go back to, so confirming leaves the address of the document behind
// altogether.
async function confirmDeletion(page: Page): Promise<void> {
  await page.locator("#documentdelete-button-delete").click()
  await expect(page.locator(".pd-home"), "home page after deleting").toBeVisible({ timeout: LOADING_TIMEOUT })
  expect(new URL(page.url()).pathname, "the address after deleting").toBe("/")
}

// Deletes a document by its address, which is what a test does with the document it created once it is done
// with it, so that a second run of the suite starts from the same data set as the first. It is done through
// the confirmation page the interface offers, so the clean-up is also one more run of the flow under test.
async function deleteDocument(page: Page, id: string): Promise<void> {
  const opened = await page.goto(`${PEERDB_URL}/d/delete/${id}`)
  expect(opened?.status(), "the delete page of the document to clean up").toBe(200)
  await expect(page.locator("#documentdelete-button-delete"), "delete button of the confirmation").toBeVisible({ timeout: LOADING_TIMEOUT })
  await confirmDeletion(page)
}

// The identifiers a full text search for the given query finds. Every attempt starts a fresh search
// session, because a session which has already run keeps the result set it found when it ran.
async function searchIds(page: Page, query: string): Promise<Array<string>> {
  await page.goto(`${PEERDB_URL}/s?q=${encodeURIComponent(query)}`)
  await settleSearch(page)
  return await resultIds(page)
}

// Waits until a search for the given query does or does not find the given document, and leaves the browser
// on the results of the search which decided it. What is waited for is the document the test created and not
// the number of results: other tests write to the same index while this one runs.
async function expectSearchFinds(page: Page, query: string, id: string, found: boolean, what: string): Promise<void> {
  await expect.poll(async () => (await searchIds(page, query)).includes(id), { message: what, timeout: INDEXING_TIMEOUT, intervals: [1000] }).toBe(found)
}

test.describe("PeerDB Delete Document Flows", () => {
  test("Test cancelling the deletion of a document", async ({ context }) => {
    const page = await context.newPage()

    await signIn(page, [DELETING_ROLE])
    const id = await createNamed(page, GALAXY, CANCEL_NAME)
    await settleDocument(page)
    // What the document shows before the deletion is asked for, so that what it shows after the deletion
    // was cancelled can be compared against it.
    const before = await page.locator(".pd-documentget-card").innerText()
    await checkpoint(page, "editdelete-cancel-document-before", { mask: volatile(page) })

    await openDeleteConfirmation(page)
    expect(new URL(page.url()).pathname, "the address of the confirmation").toBe(`/d/delete/${id}`)
    // The confirmation asks about a named document rather than about "the document" alone: it renders the
    // card of the document it is about, with its name and a link to it, so what is about to be deleted can
    // be checked before it is.
    await expect(page.locator("#documentdelete-title"), "title of the confirmation").not.toHaveText(/^\s*$/)
    await expect(page.locator("#documentdelete-text-confirm"), "question the confirmation asks").not.toHaveText(/^\s*$/)
    const card = page.locator(".pd-documentdelete-result")
    await expect(card, "card of the document to be deleted").toBeVisible()
    await expect(card.locator(".pd-searchresult-title"), "name of the document to be deleted").toHaveText(CANCEL_NAME)
    await expect(card.locator(".pd-searchresult-link-title"), "link to the document to be deleted").toHaveAttribute("href", new RegExp(`/d/${id}(\\?|$)`))
    await expect(page.locator("#documentdelete-button-cancel"), "cancel button of the confirmation").toBeVisible()
    await expect(page.locator("#documentdelete-button-delete"), "delete button of the confirmation").toBeVisible()
    await expect(page.locator("#documentdelete-text-notallowed"), "the confirmation refuses nobody who holds the action").toHaveCount(0)
    await checkpoint(page, "editdelete-cancel-confirmation", { mask: volatile(page) })

    // Cancelling goes back to the document and leaves it alone.
    await page.locator("#documentdelete-button-cancel").click()
    await settleDocument(page)
    expect(new URL(page.url()).pathname, "the address after cancelling").toBe(`/d/${id}`)
    await expect(page.locator("#documentget-title"), "title of the document after cancelling").toHaveText(CANCEL_NAME)
    expect(await page.locator(".pd-documentget-card").innerText(), "the document is the same after cancelling").toBe(before)
    await checkpoint(page, "editdelete-cancel-document-after", { mask: volatile(page) })

    // Cancelling leaves the document behind on the server too, and not only in the view which was already
    // open, so its address still serves the document when it is loaded afresh.
    const response = await page.goto(`${PEERDB_URL}/d/${id}`)
    expect(response?.status(), "the status of the address of the document after cancelling").toBe(200)
    await settleDocument(page)
    await expect(page.locator("#documentget-title"), "title of the reloaded document").toHaveText(CANCEL_NAME)

    // The document has served its purpose, so it is taken away again: a run of the suite leaves the data
    // set the way it found it.
    await deleteDocument(page, id)
    const gone = await page.goto(`${PEERDB_URL}/d/${id}`)
    expect(gone?.status(), "the status of the address of the document after the clean-up").toBe(404)
    clearRefusedRequestErrors(page)

    console.log("Successfully cancelled the deletion of 1 document, found it unchanged and then deleted it after all.")
  })

  test("Test deleting a document", async ({ context }) => {
    // The document is created, waited for in the index, deleted and then waited for again, and each of the
    // two waits is a search repeated until the index has caught up, which is more than a test of the
    // default length has time for.
    test.slow()

    const page = await context.newPage()

    await signIn(page, [DELETING_ROLE])
    const id = await createNamed(page, GALAXY, DELETE_NAME)

    // The document is searchable before it is deleted, so that the search run after the deletion shows that
    // deleting took it out of the index and not merely that it was never in it.
    //
    // What is checkpointed is the card of the created document rather than the whole result page: the page
    // also carries the count of the results and the facets they add up to, and tests running next to this
    // one write documents which the same search can find.
    await expectSearchFinds(page, DELETE_QUERY, id, true, "the created galaxy is searchable before it is deleted")
    await checkpointElement(page, page.locator(`[id="result-${id}"]`), "editdelete-search-before-result")

    await openDocument(page, id)
    await settleDocument(page)
    await openDeleteConfirmation(page)
    await checkpoint(page, "editdelete-confirmation", { mask: volatile(page) })

    await confirmDeletion(page)
    await expect(page.locator("#home-input-search"), "search box of the home page after deleting").toBeVisible()
    await settle(page)
    await checkpoint(page, "editdelete-home")

    // Going back to the address of the deleted document inside the application asks the API for it again.
    // The document is gone rather than hidden, so the fetch fails outright and the document view renders its
    // error in place of the document, and not the access denied wording it uses for a document which exists
    // but may not be read.
    await page.goBack()
    await expect(page.locator(".pd-documentdelete"), "the confirmation the browser goes back to").toBeVisible()
    await page.goBack()
    await expect(page.locator(".pd-documentget"), "the document view the browser goes back to").toBeVisible()
    const error = page.locator(".pd-documentget-error")
    await expect(error, "error of the document view of the deleted document").toBeVisible({ timeout: LOADING_TIMEOUT })
    await expect(error, "what the document view says about the deleted document").not.toHaveText(/^\s*$/)
    await expect(page.locator(".pd-documentget-error-accessdenied"), "the access denied wording of the error").toHaveCount(0)
    await expect(page.locator("#documentget-title"), "the deleted document is headed by no title").toBeHidden()
    clearRefusedRequestErrors(page)
    await checkpoint(page, "editdelete-gone-in-application")

    // Loading the address afresh does not even reach the application: the handler which serves the document
    // page checks that the document exists and answers with a plain not found page instead.
    const response = await page.goto(`${PEERDB_URL}/d/${id}`)
    expect(response?.status(), "the status of the address of the deleted document").toBe(404)
    await expect(page.locator(".pd-documentget"), "the document view at the address of the deleted document").toHaveCount(0)
    clearRefusedRequestErrors(page)

    // The confirmation page of a document which is gone is refused as well, so a deletion cannot be
    // confirmed twice.
    const confirmation = await page.goto(`${PEERDB_URL}/d/delete/${id}`)
    expect(confirmation?.status(), "the status of the confirmation of the deleted document").toBe(404)
    clearRefusedRequestErrors(page)

    // The index catches up with the deletion after the fact, so the search is retried until it does. What is
    // asserted is that the deleted document is no longer among the results, and not that the search finds
    // nothing at all: other tests are writing to the same index while this one runs.
    await expectSearchFinds(page, DELETE_QUERY, id, false, "the deleted galaxy is no longer searchable")
    await expect(page.locator(`[id="result-${id}"]`), "the card of the deleted galaxy").toHaveCount(0)
    await settleFilters(page)
    await checkpoint(page, "editdelete-search-after", { mask: volatile(page) })

    console.log("Successfully deleted 1 document, found its address gone and found that a search no longer returns it.")
  })

  test("Test which identities a document offers deleting to", async ({ context }) => {
    test.slow()

    const page = await context.newPage()

    // The document is created by the role which is granted deleting, so what the other identities are
    // offered on it follows from what the site grants them and not from having created it themselves: a
    // creator is granted every action on what they create, which the test below is about.
    await signIn(page, [DELETING_ROLE])
    const id = await createNamed(page, GALAXY, ROLES_NAME)

    for (const identity of IDENTITIES) {
      await become(page, identity)
      await openDocument(page, id)
      await settleDocument(page)

      const deleteButton = page.locator("#documentget-button-delete")
      if (identity.deletes) {
        await expect(deleteButton, `${identity.what} is offered deleting`).toBeVisible()
      } else {
        await expect(deleteButton, `${identity.what} is offered no deleting`).toHaveCount(0)
      }
      await checkpointElement(page, page.locator(".pd-documentget-card"), `editdelete-offered-${identity.slug}`, volatile(page))

      // The confirmation page is gated by the same action as the button leading to it, and the server
      // refuses the address itself rather than serving a page which could not go through, so the wording
      // the page carries for a caller who may not delete is never reached by asking for the address.
      const opened = await page.goto(`${PEERDB_URL}/d/delete/${id}`)
      expect(opened?.status(), `the confirmation page asked for by ${identity.what}`).toBe(identity.deletes ? 200 : FORBIDDEN)
      clearRefusedRequestErrors(page)
    }

    // The document has served its purpose, and the identity which is offered deleting is the one which takes
    // it away again.
    await become(page, IDENTITIES[0])
    await deleteDocument(page, id)

    console.log(`Successfully verified which of ${IDENTITIES.length} identities a document offers deleting to, and deleted it with the one which is offered it.`)
  })

  test("Test a document offers deleting to the user who created it", async ({ context }) => {
    test.slow()

    const page = await context.newPage()

    // Creating a document grants its creator every action on it, deleting included, so a role which is
    // granted no deleting anywhere is still offered it on what it made itself. That grant lives on the
    // document as a permission claim, which the permissions tab lists next to the actions it hands out.
    await signIn(page, [SURVEYING_ROLE])
    const id = await createNamed(page, GALAXY, CREATOR_NAME)
    await settleDocument(page)
    await expect(page.locator("#documentget-button-edit"), "editing offered to the creator").toBeVisible()
    await expect(page.locator("#documentget-button-delete"), "deleting offered to the creator").toBeVisible()
    await checkpointElement(page, page.locator("#documentget-sidebar"), "editdelete-creator-sidebar")

    await page.locator(".pd-documentget-tab-permissions").click()
    await expect(page.locator(".pd-documentget-panel-permissions"), "permissions tab of the created document").toBeVisible()
    await expectNothingLoading(page)
    await expect(page.locator(".pd-permissionsview-item-user"), "the users the created document grants access to").toHaveCount(1)
    await expect(page.locator(".pd-permissionsview-empty-users"), "the created document grants somebody something").toHaveCount(0)
    await checkpointElement(page, page.locator(".pd-permissionsview"), "editdelete-creator-permissions")

    // The grant reaches the server and not only the buttons, so the confirmation page opens for the creator
    // and the deletion goes through, which is what a role holding no deleting of its own could not do to any
    // other document.
    await openDocument(page, id)
    await settleDocument(page)
    await openDeleteConfirmation(page)
    await confirmDeletion(page)
    const gone = await page.goto(`${PEERDB_URL}/d/${id}`)
    expect(gone?.status(), "the status of the address of the document the creator deleted").toBe(404)
    clearRefusedRequestErrors(page)

    console.log(`Successfully verified that the creator of 1 document is offered deleting it although their role is granted none, and deleted it as them.`)
  })
})
