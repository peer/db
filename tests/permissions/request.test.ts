import type { Locator, Page } from "@playwright/test"

import { coreDocumentIdOf, documentIdOf, LANGUAGES, RESTRICTED_CLASS } from "../peerdb_utils"
import {
  checkpoint,
  checkpointElement,
  clearRefusedRequestErrors,
  expect,
  expectNothingLoading,
  goHome,
  LOADING_TIMEOUT,
  PEERDB_URL,
  signIn,
  switchLanguage,
  test,
} from "../utils"

// The documents the request page is opened for. The planet is of a public class, so a caller who reaches
// the page can read it too and is shown what they are asking about. The interview is of the class the site
// keeps out of the public read scope, so the same page is opened for a document the caller generally
// cannot read, which is what the page exists for.
const PUBLIC_DOCUMENT = await documentIdOf("PLANET", "G1_HOLLIS_III")
const INTERVIEW = await documentIdOf(RESTRICTED_CLASS, "INTA_ASELUNE_FOUR_TABLES")
// The interview naming the user who signs in holding no role, which they read already, so the page has
// fewer actions left to offer them.
const READABLE_INTERVIEW = await documentIdOf(RESTRICTED_CLASS, "INTA_LONG_DEBT_FORTY_DAY_ACCOUNT")
// An identifier derived the way a document's is but from a key the test data does not carry, so it is a
// well formed address of a document which is not there.
const MISSING_DOCUMENT = await documentIdOf(RESTRICTED_CLASS, "NO_SUCH_INTERVIEW")

// The permission actions a document's own claims can grant, which are the ones the page offers to ask for.
// Creating is missing because a document's claims never grant it, and so is bulk reading, which is not
// about a particular document.
const REQUESTABLE_ACTIONS = [
  await coreDocumentIdOf("PERMISSION_ACTIONS", "ACTION_READ"),
  await coreDocumentIdOf("PERMISSION_ACTIONS", "ACTION_READ_HISTORIC"),
  await coreDocumentIdOf("PERMISSION_ACTIONS", "ACTION_UPDATE"),
  await coreDocumentIdOf("PERMISSION_ACTIONS", "ACTION_UPDATE_PERMISSIONS"),
  await coreDocumentIdOf("PERMISSION_ACTIONS", "ACTION_DELETE"),
]
const ACTION_READ = REQUESTABLE_ACTIONS[0]
const ACTION_READ_HISTORIC = REQUESTABLE_ACTIONS[1]
const ACTION_UPDATE = REQUESTABLE_ACTIONS[2]
const ACTION_UPDATE_PERMISSIONS = REQUESTABLE_ACTIONS[3]
const ACTION_DELETE = REQUESTABLE_ACTIONS[4]

// The checkbox of one action on the request form, addressed by the action it is for.
function actionCheckbox(page: Page, action: string): Locator {
  return page.locator(`.pd-permissionactionsinput-checkbox-${action}`)
}

test.describe("PeerDB Access Request Flows", () => {
  test("Test the request page shown to a visitor who is not signed in", async ({ context }) => {
    const page = await context.newPage()

    // Asking for access records who asked, so there is nothing a caller who did not sign in can do here
    // and the page says so instead of offering a form which could not be sent.
    await goHome(page)
    const opened = await page.goto(`${PEERDB_URL}/d/request/${PUBLIC_DOCUMENT}`)
    expect(opened?.status(), "the request page of a document of a public class").toBe(200)
    await expect(page.locator(".pd-documentrequest"), "the request page").toBeVisible({ timeout: LOADING_TIMEOUT })
    await expect(page.locator("#documentrequest-text-notsignedin"), "the page says the caller has to sign in first").toBeVisible()
    await expect(page.locator(".pd-documentrequest-form"), "no form is offered to a caller who did not sign in").toHaveCount(0)
    await expect(page.locator("#documentrequest-button-request"), "nothing to send").toHaveCount(0)
    await expectNothingLoading(page)
    await checkpoint(page, "permissions-request-visitor")

    console.log("Successfully verified that the request page offers a visitor who is not signed in no way to ask for access.")
  })

  test("Test the request page of a document a visitor may not read leaks nothing of it", async ({ context }) => {
    const page = await context.newPage()

    // The page is served for any document which exists, because whoever asks for access generally cannot
    // read the document, so its existence is all the address checks. What the page then shows of the
    // document is still gated: a caller who may not read it is shown none of it.
    await goHome(page)
    const opened = await page.goto(`${PEERDB_URL}/d/request/${INTERVIEW}`)
    expect(opened?.status(), "the request page of a document of the restricted class").toBe(200)
    await expect(page.locator(".pd-documentrequest"), "the request page").toBeVisible({ timeout: LOADING_TIMEOUT })
    await expect(page.locator("#documentrequest-text-notsignedin"), "the page says the caller has to sign in first").toBeVisible()
    await expect(page.locator(".pd-documentrequest-result"), "nothing of the document is shown").toHaveCount(0)
    await expectNothingLoading(page)

    console.log("Successfully verified that the request page of an interview shows a visitor who is not signed in nothing of it.")
  })

  test("Test the request page is refused for a document which is not there", async ({ context }) => {
    const page = await context.newPage()

    // The address is checked against the store, so a well formed identifier which stands for no document is
    // answered with the not found status rather than with a form asking for access to nothing.
    await goHome(page)
    const opened = await page.goto(`${PEERDB_URL}/d/request/${MISSING_DOCUMENT}`)
    expect(opened?.status(), "the request page of a document which is not there").toBe(404)

    console.log("Successfully verified that the request page is refused for a well formed identifier which stands for no document.")
  })

  test("Test the request page offers every action to a signed-in user who may not read the document", async ({ context }) => {
    const page = await context.newPage()

    // A surveyor is granted nothing on the restricted class and is named by no interview, so this is the
    // page's own case: nothing about the document is known, so every action a document's claims can grant
    // is worth asking for.
    await signIn(page, ["surveyor"])
    await page.goto(`${PEERDB_URL}/d/request/${INTERVIEW}`)
    await expect(page.locator(".pd-documentrequest-form"), "the request form").toBeVisible({ timeout: LOADING_TIMEOUT })
    await expect(page.locator("#documentrequest-title"), "title of the request page").toBeVisible()
    await expect(page.locator("#documentrequest-text-confirm"), "the page says what sending the form does").toBeVisible()
    await expect(page.locator("#documentrequest-text-requested"), "nothing has been asked for yet").toHaveCount(0)
    await expect(page.locator("#documentrequest-text-nothing"), "there is something left to ask for").toHaveCount(0)

    for (const action of REQUESTABLE_ACTIONS) {
      await expect(actionCheckbox(page, action), `the checkbox of the action ${action}`).toBeVisible()
      await expect(actionCheckbox(page, action), `the checkbox of the action ${action} starts unticked`).not.toBeChecked()
    }
    await expect(page.locator(".pd-permissionactionsinput-checkbox"), "one checkbox per action a document's claims can grant").toHaveCount(REQUESTABLE_ACTIONS.length)

    // Nothing of the document is shown and there is no way back to it either, because this caller may not
    // read it, which is the state the page is meant for.
    await expect(page.locator(".pd-documentrequest-result"), "nothing of the document is shown").toHaveCount(0)
    await expect(page.locator("#documentrequest-button-cancel"), "there is no document view to go back to").toHaveCount(0)

    // The note is optional, so nothing has been filled in and the button which sends the form is held.
    await expect(page.locator("#documentrequest-note"), "the note for whoever decides the request").toBeVisible()
    await expect(page.locator("#documentrequest-note-hint"), "what the note is for").toBeVisible()
    await expect(page.locator("#documentrequest-button-request"), "the form cannot be sent while nothing is filled in").toBeDisabled()

    // The application asked for the document and was refused, which is what left the page without it.
    clearRefusedRequestErrors(page)
    await checkpoint(page, "permissions-request-no-access")

    console.log(`Successfully verified that the request page offers all ${REQUESTABLE_ACTIONS.length} actions to a signed-in user who may not read the document.`)
  })

  test("Test the request page offers only the actions the caller does not hold already", async ({ context }) => {
    const page = await context.newPage()

    // This interview grants the user signing in without a role reading and reading history, so those two
    // are not worth asking for and the page drops them, leaving the three actions they do not hold.
    await signIn(page, [])
    await page.goto(`${PEERDB_URL}/d/request/${READABLE_INTERVIEW}`)
    await expect(page.locator(".pd-documentrequest-form"), "the request form").toBeVisible({ timeout: LOADING_TIMEOUT })

    await expect(actionCheckbox(page, ACTION_READ), "reading is held already, so it is not offered").toHaveCount(0)
    await expect(actionCheckbox(page, ACTION_READ_HISTORIC), "reading history is held already, so it is not offered").toHaveCount(0)
    await expect(actionCheckbox(page, ACTION_UPDATE), "updating is offered").toBeVisible()
    await expect(actionCheckbox(page, ACTION_UPDATE_PERMISSIONS), "updating permissions is offered").toBeVisible()
    await expect(actionCheckbox(page, ACTION_DELETE), "deleting is offered").toBeVisible()
    await expect(page.locator(".pd-permissionactionsinput-checkbox"), "one checkbox per action which is not held already").toHaveCount(3)

    // The caller reads this document, so the page shows it and offers going back to it.
    await expect(page.locator(".pd-documentrequest-result"), "what the request is about").toBeVisible()
    await expect(page.locator("#documentrequest-button-cancel"), "the way back to the document").toBeVisible()
    await expectNothingLoading(page)
    await checkpoint(page, "permissions-request-partial-access")

    console.log("Successfully verified that the request page offers a caller only the 3 actions they do not hold on the document already.")
  })

  test("Test choosing an action chooses everything it builds on", async ({ context }) => {
    const page = await context.newPage()

    // An action is meaningful only together with what it requires, and the form applies the same rule the
    // permission checks do, so ticking one action ticks its requirements as well: updating requires
    // reading, and updating permissions requires updating, which requires reading in turn.
    await signIn(page, ["surveyor"])
    await page.goto(`${PEERDB_URL}/d/request/${INTERVIEW}`)
    await expect(page.locator(".pd-documentrequest-form"), "the request form").toBeVisible({ timeout: LOADING_TIMEOUT })

    await actionCheckbox(page, ACTION_UPDATE).click()
    await expect(actionCheckbox(page, ACTION_UPDATE), "updating is chosen").toBeChecked()
    await expect(actionCheckbox(page, ACTION_READ), "updating chose reading with it").toBeChecked()
    await expect(actionCheckbox(page, ACTION_DELETE), "nothing which was not asked for is chosen").not.toBeChecked()

    await actionCheckbox(page, ACTION_UPDATE_PERMISSIONS).click()
    await expect(actionCheckbox(page, ACTION_UPDATE_PERMISSIONS), "updating permissions is chosen").toBeChecked()
    await expect(actionCheckbox(page, ACTION_UPDATE), "updating permissions kept updating with it").toBeChecked()
    await expect(actionCheckbox(page, ACTION_READ), "updating permissions kept reading with it").toBeChecked()

    // Something has been chosen now, so the form can be sent. It is left unsent: sending it writes the
    // request onto the document.
    await expect(page.locator("#documentrequest-button-request"), "the form can be sent once something is chosen").toBeEnabled()
    await checkpointElement(page, page.locator(".pd-documentrequest-input-permission"), "permissions-request-actions-chosen")

    // Unticking what the others build on takes them with it, which is the same rule read the other way.
    await actionCheckbox(page, ACTION_READ).click()
    await expect(actionCheckbox(page, ACTION_READ), "reading is no longer chosen").not.toBeChecked()
    await expect(actionCheckbox(page, ACTION_UPDATE), "updating went with the reading it builds on").not.toBeChecked()
    await expect(actionCheckbox(page, ACTION_UPDATE_PERMISSIONS), "updating permissions went with it too").not.toBeChecked()

    clearRefusedRequestErrors(page)
    console.log("Successfully verified that choosing an action on the request page chooses everything it builds on, and unchoosing one takes everything built on it.")
  })

  test("Test the request page has nothing to ask for when the caller holds everything", async ({ context }) => {
    const page = await context.newPage()

    // The role holding every action on everything has nothing left to ask for, so the page says so and
    // offers no way to send a request which would record nothing.
    await signIn(page, ["admin"])
    await page.goto(`${PEERDB_URL}/d/request/${PUBLIC_DOCUMENT}`)
    await expect(page.locator(".pd-documentrequest-form"), "the request form").toBeVisible({ timeout: LOADING_TIMEOUT })
    await expect(page.locator("#documentrequest-text-nothing"), "the page says there is nothing to ask for").toBeVisible()
    await expect(page.locator(".pd-permissionactionsinput"), "no actions are offered").toHaveCount(0)
    await expect(page.locator("#documentrequest-note"), "no note is asked for either").toHaveCount(0)
    await expect(page.locator("#documentrequest-button-request"), "there is nothing to send").toHaveCount(0)

    // The caller reads the document, so it is shown and the way back to it is offered, which is all the
    // page is left with.
    await expect(page.locator(".pd-documentrequest-result"), "what there would be to ask about").toBeVisible()
    await expect(page.locator("#documentrequest-button-cancel"), "the way back to the document").toBeVisible()
    await expectNothingLoading(page)
    await checkpoint(page, "permissions-request-nothing-to-request")

    console.log("Successfully verified that the request page has nothing to ask for when the caller holds every action on the document already.")
  })

  for (const language of LANGUAGES) {
    test(`Test the request page in ${language}`, async ({ context }) => {
      const page = await context.newPage()

      // The page is part of the interface, so everything on it is translated, while the document it is
      // about and the identifiers addressing it are the same in every language.
      await signIn(page, ["surveyor"])
      await switchLanguage(page, language)
      await page.goto(`${PEERDB_URL}/d/request/${INTERVIEW}`)
      await expect(page.locator(".pd-documentrequest-form"), "the request form").toBeVisible({ timeout: LOADING_TIMEOUT })
      await expect(page.locator("#documentrequest-title"), "title of the request page").not.toHaveText(/^\s*$/)
      await expect(page.locator("#documentrequest-text-confirm"), "what sending the form does").not.toHaveText(/^\s*$/)
      await expect(page.locator("#documentrequest-note-hint"), "what the note is for").not.toHaveText(/^\s*$/)
      await expect(page.locator(".pd-permissionactionsinput-checkbox"), "one checkbox per action which can be asked for").toHaveCount(REQUESTABLE_ACTIONS.length)

      clearRefusedRequestErrors(page)
      await checkpoint(page, `permissions-request-${language}`)

      console.log(`Successfully verified the request page in ${language}.`)
    })
  }
})
