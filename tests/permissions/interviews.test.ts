import type { Page } from "@playwright/test"

import { documentIdOf, RESTRICTED_CLASS } from "../peerdb_utils"
import {
  checkpoint,
  checkpointElement,
  clearRefusedRequestErrors,
  expect,
  expectNothingLoading,
  fetchFromPage,
  goHome,
  mockUsername,
  openDocument,
  openDocumentTab,
  PEERDB_URL,
  settleDocument,
  signIn,
  signOut,
  test,
} from "../utils"

// The interviews of the test data are the documents which demonstrate document-level permissions: the
// site keeps their class out of the public read scope, so an interview is reachable only through a role
// which is granted the class outright or through the permission claims the interview carries itself. The
// claims name mock subjects, and the mock authenticator builds a subject out of the roles it is signed in
// with, so each of the subjects below is a sign-in a test can actually make: mock-user@localhost is a
// sign-in holding no role at all, and mock-user-<role>@localhost is a sign-in holding that one role.
//
// Which interview names whom is section 5.4 of the test data. The five below are one per shape the claims
// take.
//
//   - INTA_LONG_DEBT_FORTY_DAY_ACCOUNT grants reading and reading history to mock-user@localhost, so it is
//     reached by signing in and choosing no role.
//   - INTA_ASELUNE_FOUR_TABLES grants the same two to mock-user-researcher@localhost.
//   - INTA_FOURTH_COUNT_RUNNING_ACCOUNT grants mock-user-curator@localhost the four actions an owner
//     holds short of deleting, and carries a pending request for reading from mock-user@localhost.
//   - INTA_MAKKUNE_OPEN_POSITION names mock-user@localhost and mock-user-curator@localhost on the same two
//     claims, so two different sign-ins reach one document.
//   - INTB_FOSTER_CHAIN grants reading to mock-user@localhost and carries that same user's own pending
//     request, which is the one case where the request can be withdrawn by whoever is looking at it.
const USER_INTERVIEW = await documentIdOf(RESTRICTED_CLASS, "INTA_LONG_DEBT_FORTY_DAY_ACCOUNT")
const RESEARCHER_INTERVIEW = await documentIdOf(RESTRICTED_CLASS, "INTA_ASELUNE_FOUR_TABLES")
const CURATOR_INTERVIEW = await documentIdOf(RESTRICTED_CLASS, "INTA_FOURTH_COUNT_RUNNING_ACCOUNT")
const SHARED_INTERVIEW = await documentIdOf(RESTRICTED_CLASS, "INTA_MAKKUNE_OPEN_POSITION")
const REQUESTED_INTERVIEW = await documentIdOf(RESTRICTED_CLASS, "INTB_FOSTER_CHAIN")

// Every interview named above, so that a test about who is refused all of them says so about all of them.
const EVERY_INTERVIEW = [USER_INTERVIEW, RESEARCHER_INTERVIEW, CURATOR_INTERVIEW, SHARED_INTERVIEW, REQUESTED_INTERVIEW]

// A document of a class the site does list as public, which stands next to the interviews: an identity
// which is refused every interview and still served this one is being filtered rather than turned away.
const PUBLIC_DOCUMENT = await documentIdOf("PLANET", "G1_HOLLIS_III")

// What the server answers a caller who may not read what they asked for. The address of the document is
// answered with it as well, so the application is never even served for a document which is refused.
const FORBIDDEN = 403

// Asserts that the document is refused, both as the address a browser is pointed at and as the request the
// application itself makes, which is what the two layers of the read check come to.
async function expectRefused(page: Page, id: string, what: string): Promise<void> {
  const opened = await page.goto(`${PEERDB_URL}/d/${id}`)
  expect(opened?.status(), `the page of ${what}`).toBe(FORBIDDEN)

  const response = await fetchFromPage(page, `/api/d/${id}`)
  expect(response.status, `the API answer for ${what}`).toBe(FORBIDDEN)
  clearRefusedRequestErrors(page)
}

// Signs in as the given roles, leaving whoever was signed in before. The sign-in helper starts from the
// home page of a caller who is not signed in, because it presses the button which is there only then, so a
// test taking on one identity after another signs the previous one out first.
async function signInAgain(page: Page, roles: ReadonlyArray<string>): Promise<void> {
  await goHome(page)
  if ((await page.locator(".pd-navbarmenu-button").count()) > 0) {
    await signOut(page)
  }
  await signIn(page, roles)
}

// Asserts that the document is served and has rendered whole, which is what says a caller reaches it
// rather than merely not being turned away at the door.
async function expectServed(page: Page, id: string, what: string): Promise<void> {
  await openDocument(page, id)
  await settleDocument(page)
  await expect(page.locator("#documentget-title"), `title of ${what}`).not.toHaveText(/^\s*$/)
  expect(page.url(), `the browser stays on ${what}`).toContain(`/d/${id}`)
}

test.describe("PeerDB Document Permission Flows", () => {
  test("Test every interview is refused to a visitor who is not signed in", async ({ context }) => {
    const page = await context.newPage()

    await goHome(page)
    for (const id of EVERY_INTERVIEW) {
      await expectRefused(page, id, "an interview asked for by a visitor who is not signed in")
    }

    // The same visitor is served a document of a public class, so what refuses the interviews is the class
    // being left out of the public read scope and not the visitor being unable to read anything.
    await expectServed(page, PUBLIC_DOCUMENT, "a document of a public class")

    console.log(`Successfully verified that all ${EVERY_INTERVIEW.length} interviews are refused to a visitor who is not signed in.`)
  })

  test("Test every interview is refused to a role which is not granted the class", async ({ context }) => {
    const page = await context.newPage()

    // A surveyor may update anything they can read, which is every public class, and the interviews are
    // still refused: an action is held only together with what it requires, so updating an interview would
    // take reading it first, which no grant of this role gives. Its subject is named by no interview
    // either, so nothing reaches it from the document's side.
    await signIn(page, ["surveyor"])
    for (const id of EVERY_INTERVIEW) {
      await expectRefused(page, id, "an interview asked for by the surveyor role")
    }
    await expectServed(page, PUBLIC_DOCUMENT, "a document of a public class read by the surveyor role")

    console.log(`Successfully verified that all ${EVERY_INTERVIEW.length} interviews are refused to the surveyor role.`)
  })

  test("Test every interview is refused to the role which may only enumerate", async ({ context }) => {
    const page = await context.newPage()

    // Bulk reading is granted on everything, and it is still not reading: the listing endpoint is opened
    // by it, the documents themselves are not.
    await signIn(page, ["bulk"])
    for (const id of EVERY_INTERVIEW) {
      await expectRefused(page, id, "an interview asked for by the bulk role")
    }

    console.log(`Successfully verified that all ${EVERY_INTERVIEW.length} interviews are refused to the bulk role.`)
  })

  test("Test every interview is served to the ethics role", async ({ context }) => {
    const page = await context.newPage()

    // The ethics committee is the one role the site grants the class itself, so it reaches every interview
    // whatever the interviews say about who may read them.
    await signIn(page, ["ethics"])
    for (const id of EVERY_INTERVIEW) {
      await expectServed(page, id, "an interview read by the ethics role")
    }

    // The same role is granted updating the class, which is what the document offers it, while deleting is
    // held by nobody but the admin role.
    await openDocument(page, USER_INTERVIEW)
    await expect(page.locator("#documentget-button-edit"), "the ethics role is offered editing an interview").toBeVisible()
    await expect(page.locator("#documentget-button-delete"), "the ethics role is offered no deleting").toHaveCount(0)
    await checkpoint(page, "permissions-interviews-ethics")

    console.log(`Successfully verified that all ${EVERY_INTERVIEW.length} interviews are served to the ethics role.`)
  })

  test("Test an interview is served to the user its own claims name", async ({ context }) => {
    const page = await context.newPage()

    // Signing in without a role grants nothing through a role, so everything this identity reaches beyond
    // what a visitor reaches comes from the interviews naming its subject.
    await signIn(page, [])
    await expectServed(page, USER_INTERVIEW, "the interview naming the user holding no role")
    await checkpoint(page, "permissions-interviews-user")

    // The interviews naming somebody else stay refused to the same identity, which is what says a
    // permission claim reaches one document and not the class.
    await expectRefused(page, RESEARCHER_INTERVIEW, "the interview naming the researcher")
    await expectRefused(page, CURATOR_INTERVIEW, "the interview naming the curator")

    console.log("Successfully verified that the user holding no role reads the interview naming them and none of the interviews naming somebody else.")
  })

  test("Test an interview is served to the researcher its own claims name", async ({ context }) => {
    const page = await context.newPage()

    await signIn(page, ["researcher"])
    await expectServed(page, RESEARCHER_INTERVIEW, "the interview naming the researcher")
    await expectRefused(page, USER_INTERVIEW, "the interview naming the user holding no role")
    await expectRefused(page, CURATOR_INTERVIEW, "the interview naming the curator")

    console.log("Successfully verified that the researcher role reads the interview naming its subject and no other.")
  })

  test("Test an interview is served to the curator its own claims name", async ({ context }) => {
    const page = await context.newPage()

    await signIn(page, ["curator"])
    await expectServed(page, CURATOR_INTERVIEW, "the interview naming the curator")

    // This interview hands the curator updating and updating permissions on top of reading, which no role
    // of theirs gives on the class, so the document offers what its own claims say and nothing more.
    await expect(page.locator("#documentget-button-edit"), "editing an interview which grants the curator updating").toBeVisible()
    await expect(page.locator("#documentget-button-delete"), "the claims grant no deleting").toHaveCount(0)

    await expectRefused(page, RESEARCHER_INTERVIEW, "the interview naming the researcher")

    console.log("Successfully verified that the curator role reads and may edit the interview whose claims hand it those actions.")
  })

  test("Test an interview naming two users is served to both of them", async ({ context }) => {
    const page = await context.newPage()

    // One claim can name several users, and each of them reaches the document on their own.
    await signIn(page, [])
    await expectServed(page, SHARED_INTERVIEW, "the interview naming two users, read by the user holding no role")

    await signInAgain(page, ["curator"])
    await expectServed(page, SHARED_INTERVIEW, "the interview naming two users, read by the curator")

    // The researcher is named by neither of the two claims, so the same document is refused to them.
    await signInAgain(page, ["researcher"])
    await expectRefused(page, SHARED_INTERVIEW, "the interview naming two users, asked for by the researcher")

    console.log("Successfully verified that an interview naming two users is served to both of them and refused to a third.")
  })

  test("Test signing out takes back the interview the signed-in user reached", async ({ context }) => {
    const page = await context.newPage()

    await signIn(page, [])
    await expectServed(page, USER_INTERVIEW, "the interview naming the user holding no role")

    // Signing out from the page the document is on re-renders the application and drops what was fetched
    // under the identity which is gone, so the document has to be asked for again and is now refused. The
    // page says so where the document was, instead of going on showing what the user could read a moment
    // ago.
    await signOut(page)
    const denied = page.locator(".pd-documentget-error-accessdenied")
    await expect(denied, "the document says access was denied once the user signed out").toBeVisible()
    await expect(page.locator("#documentget-title"), "nothing of the document is left on the page").toBeHidden()
    expect(page.url(), "the browser stays on the document which is now refused").toContain(`/d/${USER_INTERVIEW}`)
    await expectNothingLoading(page)
    await checkpoint(page, "permissions-interviews-denied-after-signout")

    // A document of a public class is served to the same browser afterwards, so signing out took away the
    // one document the identity had and not the session's ability to read at all.
    await expectServed(page, PUBLIC_DOCUMENT, "a document of a public class after signing out")

    console.log("Successfully verified that an interview reached by a signed-in user is refused again as soon as they sign out.")
  })

  test("Test the permissions tab of an interview lists the users its claims name", async ({ context }) => {
    const page = await context.newPage()

    // The permissions tab renders the document's own permission claims, so it is where the grants which
    // decide the tests above are read back. The ethics role sees them without being named by them.
    await signIn(page, ["ethics"])
    await openDocument(page, CURATOR_INTERVIEW)
    await openDocumentTab(page, "permissions")
    await expectNothingLoading(page)

    const users = page.locator(".pd-permissionsview-item-user")
    await expect(users, "the interview grants access to exactly one user").toHaveCount(1)
    await expect(users.locator(".pd-permissionsview-label-user"), "the user the interview names").toHaveText(mockUsername(["curator"]))
    // The four actions the claims grant are rendered one badge each, and every badge carries the action it
    // is for, so what is granted is read without depending on the labels.
    await expect(users.locator(".pd-permissionsview-badge-action"), "one badge per granted action").toHaveCount(4)

    // The same tab lists the requests waiting for a decision, which this interview has one of, from the
    // user holding no role.
    const requests = page.locator(".pd-permissionsview-item-request")
    await expect(requests, "the interview carries one pending access request").toHaveCount(1)
    await expect(requests.locator(".pd-permissionsview-label-user"), "who the pending request is from").toHaveText(mockUsername([]))
    await expect(requests.locator(".pd-permissionsview-button-cancel"), "a request of somebody else cannot be withdrawn here").toHaveCount(0)

    // Deciding the requests and changing the grants both go through the edit page, which the tab offers
    // because the ethics role is granted updating permissions on this class.
    await expect(page.locator(".pd-permissionsview-button-edit"), "the ethics role is offered editing the permissions").toBeVisible()
    await expect(page.locator(".pd-permissionsview-button-manage"), "the ethics role is offered deciding the requests").toBeVisible()
    await expect(page.locator(".pd-permissionsview-button-request"), "a role which may decide requests is not offered making one").toHaveCount(0)
    await checkpointElement(page, page.locator(".pd-permissionsview"), "permissions-interviews-permissions-tab-ethics")

    console.log("Successfully verified that the permissions tab of an interview lists the one user it grants access to and the one request waiting for a decision.")
  })

  test("Test a user sees their own pending request on the interview they may read", async ({ context }) => {
    const page = await context.newPage()

    // This interview both grants the user reading and carries that user's own request for it, so the tab
    // shows them their request together with the way to withdraw it, which is the one change this page
    // makes on its own.
    await signIn(page, [])
    await openDocument(page, REQUESTED_INTERVIEW)
    await openDocumentTab(page, "permissions")
    await expectNothingLoading(page)

    const requests = page.locator(".pd-permissionsview-item-request")
    await expect(requests, "the interview carries one pending access request").toHaveCount(1)
    await expect(requests.locator(".pd-permissionsview-label-user"), "the request is the signed-in user's own").toHaveText(mockUsername([]))
    await expect(requests.locator(".pd-permissionsview-button-cancel"), "a request of one's own can be withdrawn").toBeVisible()

    // Reading is all the interview grants this user, so the tab offers asking for more instead of editing
    // what is granted.
    await expect(page.locator(".pd-permissionsview-button-edit"), "a user who may not update permissions is offered no editing").toHaveCount(0)
    await expect(page.locator(".pd-permissionsview-button-request"), "a user who may not update permissions is offered asking for access").toBeVisible()
    await checkpointElement(page, page.locator(".pd-permissionsview"), "permissions-interviews-permissions-tab-user")

    console.log("Successfully verified that a user is shown their own pending request on the interview they may read, with the way to withdraw it.")
  })
})
