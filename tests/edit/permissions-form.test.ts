import type { Locator, Page } from "@playwright/test"

import type { Role } from "../peerdb_utils"

import { coreDocumentIdOf, createNamed, documentIdOf, PROPERTY_IDS, RESTRICTED_CLASS, startCreate } from "../peerdb_utils"
import {
  checkpoint,
  checkpointElement,
  clearRefusedRequestErrors,
  discardEdit,
  expect,
  expectNothingLoading,
  fetchFromPage,
  field,
  fillSlot,
  goHome,
  hideDuplicates,
  LOADING_TIMEOUT,
  mockUsername,
  openDocument,
  openDocumentTab,
  openUserMenu,
  PEERDB_URL,
  pickReference,
  saveEdit,
  settle,
  settleDocument,
  signIn,
  signOut,
  test,
} from "../utils"

// The class the tests which need a document anybody may read create in. A galaxy is the smallest class of
// the site: a single name makes a saveable document, so a test about who may do what with it does not have
// to fill a form first.
const GALAXY = "GALAXY"

// The roles the tests sign in as. Managing what a document grants is held by the role which holds it
// everywhere, and by the ethics committee on the class the site keeps out of the public read scope, which
// is the one class it may decide access to (roles in config.yml). Interviews of that class are opened by
// the research role, which is therefore the one which creates the documents the ethics tests work on.
const MANAGING_ROLE: Role = "admin"
const ETHICS_ROLE: Role = "ethics"
const RESEARCH_ROLE: Role = "researcher"

// The role which asks for access. It is granted nothing at all on the restricted class and is named by no
// document of the test data, so what it can do with an interview is exactly what a test grants it.
const ASKING_ROLE: Role = "surveyor"

// The names the tests give the documents they create. They all begin with the same invented prefix, which
// no document of the test data and no other test file carries, and each test uses a name of its own.
const LISTING_NAME = "PDEPERM Listed Galaxy"
const GRANTING_NAME = "PDEPERM Granted Galaxy"
const INTERVIEW_NAME = "PDEPERM Granted Interview"
const APPROVING_NAME = "PDEPERM Approved Interview"
const DENYING_NAME = "PDEPERM Denied Interview"

// The documents an interview created by a test refers to, which are two documents of the test data: an
// interview records who was interviewed and by whom, and both fields have to be filled in for the record to
// say anything. They are only ever referred to, never changed.
const INTERVIEWEE = await documentIdOf("INDIVIDUAL", "G1_ASELUNE")
const INTERVIEWER = await documentIdOf("RESEARCHER", "RES_ARAI")

// The queries which offer those two documents in the reference field they are picked in. Each is a word out
// of the document's own name, so the search which the field runs finds it.
const INTERVIEWEE_QUERY = "Aselune"
const INTERVIEWER_QUERY = "Arai"

// The permission actions a document's own claims can grant, in the order the permissions tab lists them.
// Creating is missing because a document's claims never grant it, and so is bulk reading, which is not
// about a particular document.
const ACTION_READ = await coreDocumentIdOf("PERMISSION_ACTIONS", "ACTION_READ")
const ACTION_READ_HISTORIC = await coreDocumentIdOf("PERMISSION_ACTIONS", "ACTION_READ_HISTORIC")
const ACTION_UPDATE = await coreDocumentIdOf("PERMISSION_ACTIONS", "ACTION_UPDATE")
const ACTION_UPDATE_PERMISSIONS = await coreDocumentIdOf("PERMISSION_ACTIONS", "ACTION_UPDATE_PERMISSIONS")
const ACTION_DELETE = await coreDocumentIdOf("PERMISSION_ACTIONS", "ACTION_DELETE")
const GRANTABLE_ACTIONS = [ACTION_READ, ACTION_READ_HISTORIC, ACTION_UPDATE, ACTION_UPDATE_PERMISSIONS, ACTION_DELETE]

// The actions the site grants everybody on a document of a public class (the empty role in config.yml), so
// a document of such a class has nothing to add to them for anybody, and the ones it can still hand out.
const PUBLIC_ACTIONS = [ACTION_READ, ACTION_READ_HISTORIC]
const PRIVATE_ACTIONS = [ACTION_UPDATE, ACTION_UPDATE_PERMISSIONS, ACTION_DELETE]

// How many slots the name field of the create form shows once its only slot is filled: a name may be stated
// more than once, so filling the slot grows a fresh empty one below it.
const NAME_SLOTS = 2

// What the server answers a caller who asks for something they hold no action for.
const FORBIDDEN = 403

// How long a change which is committed after the request which made it is given to land. Asking for access
// is recorded in an edit session of its own which the server commits asynchronously, so the document carries
// the request some time after the page which sent it has already said so.
const COMMIT_TIMEOUT = 30000

// Puts the browser in the state the given roles describe, from whatever state it is in: a page which is
// already signed in has to be signed out first, because the button which signs in is the one the signed-in
// user's own menu replaced. Passing no role at all signs in as the user who holds none.
//
// Whether somebody is signed in is read on the home page and not on whatever page the browser is on: a page
// which has only just been navigated to has not mounted the application yet, and its navbar is therefore
// empty however the visit is authenticated.
async function become(page: Page, roles: ReadonlyArray<Role>): Promise<void> {
  await goHome(page)
  if ((await page.locator(".pd-navbarmenu-button").count()) > 0) {
    await signOut(page)
  }
  await signIn(page, roles)
}

// Signs in with the given roles and reports the subject the site knows that user by, which is what names
// them in a permission claim and what the permissions tab is given to grant access to somebody. The subject
// is read out of the signed-in user's own menu rather than derived, so a test never has to spell out how the
// authenticator builds it.
async function subjectOf(page: Page, roles: ReadonlyArray<Role>): Promise<string> {
  await become(page, roles)
  await openUserMenu(page)
  const shown = page.locator(".pd-navbaruser-text-id")
  await expect(shown, "the subject of the signed-in user").toBeVisible()
  const subject = ((await shown.textContent()) || "").trim()
  expect(subject, "the subject of the signed-in user").not.toBe("")
  return subject
}

// The entry of the permissions form for one user, picked out by the name the site knows them by, so that a
// test never depends on the order the entries are listed in.
function userEntry(page: Page, username: string): Locator {
  return page.locator(`.pd-permissionsform-item-user:has(.pd-permissionsform-label-user:text-is("${username}"))`)
}

// The checkbox of one action inside the given block of the permissions form: an entry of a user with access,
// or the entry naming somebody who has none yet.
function actionCheckbox(scope: Locator, action: string): Locator {
  return scope.locator(`.pd-permissionsform-checkbox-action-${action}`)
}

// The block of the permissions form where a user who has not asked for access is named.
function addUserSection(page: Page): Locator {
  return page.locator(".pd-permissionsform-section-adduser")
}

// The badge the permissions tab of the document view renders for one action of one user.
function actionBadge(scope: Locator, action: string): Locator {
  return scope.locator(`.pd-permissionsview-badge-action-${action}`)
}

// The entry of the permissions tab of the document view for one user.
function viewEntry(page: Page, username: string): Locator {
  return page.locator(`.pd-permissionsview-item-user:has(.pd-permissionsview-label-user:text-is("${username}"))`)
}

// Opens the permissions tab of the document view and waits until it has rendered what the document grants.
async function openPermissions(page: Page, id: string): Promise<void> {
  await openDocument(page, id)
  await openDocumentTab(page, "permissions")
  await settle(page)
  await expect(page.locator(".pd-permissionsview"), "permissions tab of the document").toBeVisible()
}

// Opens the permissions tab of the edit view through the button the document view offers for it, which is
// what a user holding the permissions action reaches it by: it begins an edit session on the document and
// opens it on that tab.
async function openPermissionsForm(page: Page, id: string, button: "edit" | "manage"): Promise<void> {
  await openPermissions(page, id)
  const open = page.locator(`.pd-permissionsview-button-${button}`)
  await expect(open, `the button which opens the permissions form of the document`).toBeVisible()
  await open.click()
  await expect(page.locator(".pd-permissionsform"), "permissions form of the edit view").toBeVisible({ timeout: LOADING_TIMEOUT })
  await expect(page.locator(".pd-documentedit-panel-permissions"), "permissions tab of the edit view").toBeVisible()
  await expectNothingLoading(page)
  await expect(page.locator(".pd-permissionsform-loading"), "the permissions form knows every user it lists").toHaveCount(0, { timeout: LOADING_TIMEOUT })
  expect(new URL(page.url()).pathname, "the address of the edit session").toContain(`/d/edit/${id}/`)
  expect(new URL(page.url()).searchParams.get("tab"), "the tab the edit session was opened on").toBe("permissions")
}

// Creates an interview of the restricted class, which is the class the ethics committee decides access to.
// An interview records who was interviewed and by whom, so both reference fields are filled in next to the
// name, each with a document of the test data.
async function createInterview(page: Page, name: string): Promise<string> {
  await startCreate(page, RESTRICTED_CLASS)
  await hideDuplicates(page)
  await fillSlot(page, PROPERTY_IDS.NAME, 0, ".pd-inputstring", name, NAME_SLOTS, "name of the new interview")
  await pickReference(page, field(page, PROPERTY_IDS.HAS_INTERVIEWEE), INTERVIEWEE_QUERY, INTERVIEWEE, "the interviewee of the new interview")
  await pickReference(page, field(page, PROPERTY_IDS.HAS_INTERVIEWER), INTERVIEWER_QUERY, INTERVIEWER, "the interviewer of the new interview")
  const id = await saveEdit(page)
  await expect(page.locator("#documentget-title"), "title of the created interview").toHaveText(name, { timeout: LOADING_TIMEOUT })
  return id
}

// Deletes a document a test created, through the confirmation page the interface offers, so that a second
// run of the suite starts from the same data set as the first. The role which is granted deleting everywhere
// is the one which does it, whoever created the document.
async function deleteDocument(page: Page, id: string): Promise<void> {
  await become(page, [MANAGING_ROLE])
  const opened = await page.goto(`${PEERDB_URL}/d/delete/${id}`)
  expect(opened?.status(), "the delete page of the document to clean up").toBe(200)
  await expect(page.locator("#documentdelete-button-delete"), "delete button of the confirmation").toBeVisible({ timeout: LOADING_TIMEOUT })
  await page.locator("#documentdelete-button-delete").click()
  await expect(page.locator(".pd-home"), "home page after deleting").toBeVisible({ timeout: LOADING_TIMEOUT })
}

// The status the server answers a request for the given path with, asked for from inside the page so that it
// carries the session of the browser making it.
async function statusOf(page: Page, path: string): Promise<number> {
  const response = await fetchFromPage(page, path)
  clearRefusedRequestErrors(page)
  return response.status
}

test.describe("PeerDB Permissions Form Flows", () => {
  test("Test the permissions tab of the edit view lists what the document grants", async ({ context }) => {
    test.slow()

    const page = await context.newPage()

    // Creating a document grants its creator every action on it, so a freshly created document already has
    // one user with access, which is what the tab of a document nobody else was given anything on shows.
    await become(page, [MANAGING_ROLE])
    const id = await createNamed(page, GALAXY, LISTING_NAME)
    const creator = mockUsername([MANAGING_ROLE])

    await openPermissions(page, id)
    await expect(page.locator(".pd-permissionsview-item-user"), "the users the created document grants access to").toHaveCount(1)
    await expect(viewEntry(page, creator), "the creator of the document is the user it grants access to").toBeVisible()
    for (const action of GRANTABLE_ACTIONS) {
      await expect(actionBadge(viewEntry(page, creator), action), `the badge of the action ${action} of the creator`).toBeVisible()
    }
    await expect(page.locator(".pd-permissionsview-section-requests"), "no access request is waiting on the created document").toHaveCount(0)
    await checkpointElement(page, page.locator(".pd-permissionsview"), "editpermissions-view-created")

    // The tab of the document view only reads what the document grants: changing it is the tab of the edit
    // view, which the button under the list opens by beginning an edit session on the document.
    await openPermissionsForm(page, id, "edit")
    await expect(page.locator(".pd-permissionsform-section-users"), "the users section of the form").toBeVisible()
    await expect(page.locator(".pd-permissionsform-section-requests"), "the requests section of the form").toBeVisible()
    await expect(page.locator(".pd-permissionsform-empty-requests"), "the form says no access request is waiting").toBeVisible()
    await expect(page.locator(".pd-permissionsform-empty-users"), "the form does not say the document grants nobody anything").toHaveCount(0)
    await expect(page.locator(".pd-permissionsform-item-user"), "the users the form lists").toHaveCount(1)
    await expect(userEntry(page, creator), "the entry of the creator").toBeVisible()

    // Every action the creator was granted is checked and can be unchecked, which is how access is taken
    // away, and the entry offers taking all of it away at once.
    for (const action of GRANTABLE_ACTIONS) {
      const checkbox = actionCheckbox(userEntry(page, creator), action)
      await expect(checkbox, `the checkbox of the action ${action} of the creator`).toBeChecked()
      await expect(checkbox, `the checkbox of the action ${action} of the creator can be unchecked`).toBeEnabled()
    }
    await expect(userEntry(page, creator).locator(".pd-permissionsform-button-remove"), "the button which takes all access away from the creator").toBeVisible()

    // With nobody named in the entry which grants access to a user who has not asked for it, there is
    // nobody to grant anything to, so its checkboxes show what could be granted and none of them can be
    // ticked.
    const adding = addUserSection(page)
    await expect(adding, "the entry which names a user to grant access to").toBeVisible()
    await expect(adding.locator(".pd-inputidentity-input"), "the input which names the user").toBeVisible()
    await expect(adding.locator(".pd-permissionsform-text-adduserhint"), "what naming a user there does").toBeVisible()
    await expect(adding.locator(".pd-permissionsform-checkbox-action"), "one checkbox per action which could be granted").toHaveCount(GRANTABLE_ACTIONS.length)
    for (const action of GRANTABLE_ACTIONS) {
      await expect(actionCheckbox(adding, action), `the checkbox of the action ${action} with nobody named`).toBeDisabled()
      await expect(actionCheckbox(adding, action), `the checkbox of the action ${action} with nobody named is unticked`).not.toBeChecked()
    }
    await checkpointElement(page, page.locator(".pd-permissionsform"), "editpermissions-form-created")
    await checkpoint(page, "editpermissions-form-page")

    // Nothing was changed, so the session is dropped rather than committed, and the document is left as the
    // creation made it.
    await discardEdit(page)
    await openPermissions(page, id)
    await expect(page.locator(".pd-permissionsview-item-user"), "the users the document grants access to after the session was discarded").toHaveCount(1)

    await deleteDocument(page, id)

    console.log(
      `Successfully verified that the permissions tab of the edit view lists 1 user with the ${GRANTABLE_ACTIONS.length} actions creating the document granted them.`,
    )
  })

  test("Test granting an action to a user and taking it back", async ({ context }) => {
    test.slow()

    const page = await context.newPage()

    // The user which is granted something holds no role at all, so what they can do with a document of a
    // public class is what the site grants everybody, and anything else has to come from the document.
    const subject = await subjectOf(page, [])
    const username = mockUsername([])

    await become(page, [MANAGING_ROLE])
    const id = await createNamed(page, GALAXY, GRANTING_NAME)

    await become(page, [])
    await openDocument(page, id)
    await settleDocument(page)
    await expect(page.locator("#documentget-button-delete"), "deleting offered to the user before anything is granted").toHaveCount(0)
    expect(await statusOf(page, `/d/delete/${id}`), "the delete page before anything is granted").toBe(FORBIDDEN)

    await become(page, [MANAGING_ROLE])
    await openPermissionsForm(page, id, "edit")
    const adding = addUserSection(page)
    await adding.locator(".pd-inputidentity-input").fill(subject)
    await adding.locator(".pd-inputidentity-input").blur()
    // The named user is looked up, and a subject the site can describe is shown by the name it knows them
    // by, in place of the input the subject was typed into.
    await expect(adding.locator(".pd-inputidentity-value"), "the user named in the entry").toBeVisible({ timeout: LOADING_TIMEOUT })
    await expect(adding.locator(".pd-permissionsform-error-user"), "naming the user ran into nothing").toHaveCount(0)
    await expect(page.locator(".pd-permissionsform-loading"), "the form knows the named user").toHaveCount(0, { timeout: LOADING_TIMEOUT })

    // An action the user's roles already cover is not offered at all, because granting it on the document
    // would add nothing to what they can do, so a document of a class everybody may read offers only the
    // actions which are not public.
    for (const action of PUBLIC_ACTIONS) {
      await expect(actionCheckbox(adding, action), `the action ${action}, which the site grants everybody, is not offered`).toHaveCount(0)
    }
    for (const action of PRIVATE_ACTIONS) {
      await expect(actionCheckbox(adding, action), `the action ${action} is offered for the named user`).toBeEnabled()
      await expect(actionCheckbox(adding, action), `the action ${action} is not granted to the named user yet`).not.toBeChecked()
    }
    await checkpointElement(page, adding, "editpermissions-adduser-named")

    // Granting an action makes the named user one of the users with access, listed above with what they were
    // granted, and clears the name, which is what says the grant landed.
    await actionCheckbox(adding, ACTION_DELETE).click()
    await expect(page.locator(".pd-permissionsform-item-user"), "the users the form lists once the named user was granted an action").toHaveCount(2, {
      timeout: LOADING_TIMEOUT,
    })
    const entry = userEntry(page, username)
    await expect(entry, "the entry of the user which was granted an action").toBeVisible()
    await expect(actionCheckbox(entry, ACTION_DELETE), "the action which was granted").toBeChecked()
    await expect(adding.locator(".pd-inputidentity-value"), "the name is cleared once the user has access").toHaveCount(0)
    await checkpointElement(page, entry, "editpermissions-granted-entry")

    // The grant is part of the document, so it is committed the way any other change is, and the tab of the
    // document view then lists it.
    await saveEdit(page)
    await openPermissions(page, id)
    await expect(page.locator(".pd-permissionsview-item-user"), "the users the document grants access to after the grant").toHaveCount(2)
    await expect(actionBadge(viewEntry(page, username), ACTION_DELETE), "the badge of the granted action").toBeVisible()
    await checkpointElement(page, page.locator(".pd-permissionsview"), "editpermissions-view-granted")

    // What the document grants reaches both the buttons the user is offered and the server behind them.
    await become(page, [])
    await openDocument(page, id)
    await settleDocument(page)
    await expect(page.locator("#documentget-button-delete"), "deleting offered to the user the document grants it").toBeVisible()
    expect(await statusOf(page, `/d/delete/${id}`), "the delete page of the user the document grants deleting").toBe(200)

    // Taking the access away takes the whole entry with it, and with it everything the document granted that
    // user.
    await become(page, [MANAGING_ROLE])
    await openPermissionsForm(page, id, "edit")
    await expect(userEntry(page, username), "the entry of the user whose access is taken away").toBeVisible()
    await userEntry(page, username).locator(".pd-permissionsform-button-remove").click()
    await expect(page.locator(".pd-permissionsform-item-user"), "the users the form lists once the access was taken away").toHaveCount(1, { timeout: LOADING_TIMEOUT })
    await expect(userEntry(page, username), "the entry of the user whose access was taken away").toHaveCount(0)
    await saveEdit(page)

    await openPermissions(page, id)
    await expect(page.locator(".pd-permissionsview-item-user"), "the users the document grants access to after the access was taken away").toHaveCount(1)
    await expect(viewEntry(page, username), "the user whose access was taken away").toHaveCount(0)

    await become(page, [])
    await openDocument(page, id)
    await settleDocument(page)
    await expect(page.locator("#documentget-button-delete"), "deleting offered to the user after the access was taken away").toHaveCount(0)
    expect(await statusOf(page, `/d/delete/${id}`), "the delete page after the access was taken away").toBe(FORBIDDEN)

    await deleteDocument(page, id)

    console.log(`Successfully granted 1 action to a user on a created document, found it offered to them, took it back and found it gone again.`)
  })

  test("Test the ethics role grants reading a document its class keeps closed", async ({ context }) => {
    test.slow()

    const page = await context.newPage()

    // A user holding no role is granted nothing at all on the restricted class, so an interview is refused
    // them outright, which is the state the grant below changes.
    const subject = await subjectOf(page, [])
    const username = mockUsername([])

    await become(page, [RESEARCH_ROLE])
    const id = await createInterview(page, INTERVIEW_NAME)

    await become(page, [])
    expect(await statusOf(page, `/api/d/${id}`), "the interview before anything is granted").toBe(FORBIDDEN)
    expect(await statusOf(page, `/api/d/history/${id}`), "the history of the interview before anything is granted").toBe(FORBIDDEN)

    // The ethics committee decides who reads a closed record, which is the one class it may change what is
    // granted on.
    await become(page, [ETHICS_ROLE])
    await openPermissionsForm(page, id, "edit")
    const adding = addUserSection(page)
    await adding.locator(".pd-inputidentity-input").fill(subject)
    await adding.locator(".pd-inputidentity-input").blur()
    await expect(adding.locator(".pd-inputidentity-value"), "the user named in the entry").toBeVisible({ timeout: LOADING_TIMEOUT })
    await expect(page.locator(".pd-permissionsform-loading"), "the form knows the named user").toHaveCount(0, { timeout: LOADING_TIMEOUT })

    // The user holds none of the actions on this document, so every one of them is offered, but only the
    // ones the committee holds itself can be handed out: the server refuses a change granting an action the
    // caller does not hold, and deleting is not one of theirs.
    await expect(adding.locator(".pd-permissionsform-checkbox-action"), "one checkbox per action which could be granted").toHaveCount(GRANTABLE_ACTIONS.length)
    for (const action of [ACTION_READ, ACTION_READ_HISTORIC, ACTION_UPDATE, ACTION_UPDATE_PERMISSIONS]) {
      await expect(actionCheckbox(adding, action), `the action ${action} can be granted by the ethics role`).toBeEnabled()
    }
    await expect(actionCheckbox(adding, ACTION_DELETE), "deleting cannot be granted by a role which does not hold it").toBeDisabled()
    await expect(adding.locator(".pd-permissionsform-text-cannotgrant"), "the entry says why an action cannot be granted").toBeVisible()
    await checkpointElement(page, adding, "editpermissions-adduser-interview")

    await actionCheckbox(adding, ACTION_READ).click()
    const entry = userEntry(page, username)
    await expect(entry, "the entry of the user which was granted reading").toBeVisible({ timeout: LOADING_TIMEOUT })
    await expect(actionCheckbox(entry, ACTION_READ), "reading is granted to the user").toBeChecked()
    await expect(actionCheckbox(entry, ACTION_READ_HISTORIC), "reading history is not granted along with reading").not.toBeChecked()
    await saveEdit(page)

    await openPermissions(page, id)
    await expect(actionBadge(viewEntry(page, username), ACTION_READ), "the badge of the granted action").toBeVisible()
    await checkpointElement(page, page.locator(".pd-permissionsview"), "editpermissions-view-interview")

    // What the whole history of the interview is, read by the role which is granted reading it, so that what
    // the granted user is answered can be compared against it.
    const whole = await fetchFromPage(page, `/api/d/history/${id}`)
    expect(whole.status, "the history of the interview read by the ethics role").toBe(200)
    const versions = (JSON.parse(whole.body) as Array<{ version: string }>).length
    expect(versions, "versions of an interview which was created and then granted to somebody").toBeGreaterThan(1)

    // The user reads the interview now, and the history it offers them is only the versions they may read:
    // reading history was not granted, so the versions written before they were named are not theirs, while
    // the version which names them is.
    await become(page, [])
    await openDocument(page, id)
    await settleDocument(page)
    await expect(page.locator("#documentget-title"), "title of the interview the user was granted reading").toHaveText(INTERVIEW_NAME)
    await openDocumentTab(page, "history")
    await settle(page)
    await expect(page.locator(".pd-documenthistory-error"), "the history of the granted user loaded without an error").toHaveCount(0)
    await expect(page.locator(".pd-documenthistory-item"), "the versions of the interview the granted user may read").toHaveCount(1)
    const partial = await fetchFromPage(page, `/api/d/history/${id}`)
    expect(partial.status, "the history of the interview read by the granted user").toBe(200)
    expect((JSON.parse(partial.body) as Array<{ version: string }>).length, "versions the granted user is answered with").toBe(1)

    // Taking the grant back closes the document again, which is what says the access came from the claim the
    // committee wrote and from nothing else.
    await become(page, [ETHICS_ROLE])
    await openPermissionsForm(page, id, "edit")
    await userEntry(page, username).locator(".pd-permissionsform-button-remove").click()
    await expect(userEntry(page, username), "the entry of the user whose access was taken away").toHaveCount(0, { timeout: LOADING_TIMEOUT })
    await saveEdit(page)

    await become(page, [])
    expect(await statusOf(page, `/api/d/${id}`), "the interview after the access was taken away").toBe(FORBIDDEN)

    await deleteDocument(page, id)

    console.log(`Successfully granted reading 1 interview to a user holding no role, found ${versions} versions of it readable to the ethics role and 1 to them.`)
  })

  test("Test approving an access request somebody left on a document", async ({ context }) => {
    test.slow()

    const page = await context.newPage()

    const username = mockUsername([ASKING_ROLE])

    await become(page, [RESEARCH_ROLE])
    const id = await createInterview(page, APPROVING_NAME)

    // A caller who may not read a document asks for access on the page made for it, which records the
    // request on the document itself, so whoever decides access finds it there.
    await become(page, [ASKING_ROLE])
    expect(await statusOf(page, `/api/d/${id}`), "the interview before access is asked for").toBe(FORBIDDEN)
    await page.goto(`${PEERDB_URL}/d/request/${id}`)
    await expect(page.locator(".pd-documentrequest-form"), "the request form").toBeVisible({ timeout: LOADING_TIMEOUT })
    await page.locator(`.pd-permissionactionsinput-checkbox-${ACTION_READ}`).click()
    await expect(page.locator("#documentrequest-button-request"), "the form can be sent once something is chosen").toBeEnabled()
    await page.locator("#documentrequest-button-request").click()
    await expect(page.locator("#documentrequest-text-requested"), "the page says the request was sent").toBeVisible({ timeout: LOADING_TIMEOUT })
    clearRefusedRequestErrors(page)

    // The request is recorded in an edit session of its own, which the server commits after answering, so
    // the document carries it a moment later.
    await become(page, [ETHICS_ROLE])
    await expect
      .poll(
        async () => {
          await openPermissions(page, id)
          return await page.locator(".pd-permissionsview-item-request").count()
        },
        { message: "the request reaches the document the access was asked for on", timeout: COMMIT_TIMEOUT },
      )
      .toBe(1)
    await expect(page.locator(".pd-permissionsview-section-requests"), "the requests section of the document view").toBeVisible()
    await expect(page.locator(".pd-permissionsview-text-action"), "which action the request asks for").not.toHaveText(/^\s*$/)
    await expect(page.locator(".pd-permissionsview-button-manage"), "the button which opens the requests to decide them").toBeVisible()
    await checkpointElement(page, page.locator(".pd-permissionsview"), "editpermissions-view-requested")

    // Deciding a request happens on the permissions tab of the edit view, which the button under the
    // requests opens, and approving one grants what was asked for and takes the request away with it.
    await openPermissionsForm(page, id, "manage")
    const request = page.locator(".pd-permissionsform-item-request")
    await expect(request, "the request the form lists").toHaveCount(1)
    await expect(request.locator(".pd-permissionsform-label-user"), "the user who asked for access").toHaveText(username)
    await expect(request.locator(".pd-permissionsform-text-action"), "the action the request asks for").not.toHaveText(/^\s*$/)
    await expect(request.locator(".pd-permissionsform-button-deny"), "the button which denies the request").toBeVisible()
    await checkpointElement(page, request, "editpermissions-request-entry")

    const approve = request.locator(".pd-permissionsform-button-approve")
    await expect(approve, "the button which approves the request").toBeEnabled()
    await approve.click()
    await expect(page.locator(".pd-permissionsform-item-request"), "the requests left once this one was approved").toHaveCount(0, { timeout: LOADING_TIMEOUT })
    await expect(page.locator(".pd-permissionsform-empty-requests"), "the form says no request is left").toBeVisible()
    const entry = userEntry(page, username)
    await expect(entry, "the user who asked for access is now one with access").toBeVisible()
    await expect(actionCheckbox(entry, ACTION_READ), "the action which was approved").toBeChecked()
    await saveEdit(page)

    await openPermissions(page, id)
    await expect(actionBadge(viewEntry(page, username), ACTION_READ), "the badge of the approved action").toBeVisible()
    await expect(page.locator(".pd-permissionsview-section-requests"), "no request is left on the document").toHaveCount(0)

    // The approval is what the requester was asking for, so the document opens for them now.
    await become(page, [ASKING_ROLE])
    await openDocument(page, id)
    await settleDocument(page)
    await expect(page.locator("#documentget-title"), "title of the interview the request was approved for").toHaveText(APPROVING_NAME)

    await deleteDocument(page, id)

    console.log("Successfully asked for reading 1 interview as a role which is granted none of it, approved the request as the ethics role and read it afterwards.")
  })

  test("Test denying an access request somebody left on a document", async ({ context }) => {
    test.slow()

    const page = await context.newPage()

    const username = mockUsername([ASKING_ROLE])

    await become(page, [RESEARCH_ROLE])
    const id = await createInterview(page, DENYING_NAME)

    await become(page, [ASKING_ROLE])
    await page.goto(`${PEERDB_URL}/d/request/${id}`)
    await expect(page.locator(".pd-documentrequest-form"), "the request form").toBeVisible({ timeout: LOADING_TIMEOUT })
    await page.locator(`.pd-permissionactionsinput-checkbox-${ACTION_READ}`).click()
    await page.locator("#documentrequest-button-request").click()
    await expect(page.locator("#documentrequest-text-requested"), "the page says the request was sent").toBeVisible({ timeout: LOADING_TIMEOUT })
    clearRefusedRequestErrors(page)

    await become(page, [ETHICS_ROLE])
    await expect
      .poll(
        async () => {
          await openPermissions(page, id)
          return await page.locator(".pd-permissionsview-item-request").count()
        },
        { message: "the request reaches the document the access was asked for on", timeout: COMMIT_TIMEOUT },
      )
      .toBe(1)

    // Denying a request only removes it: it grants nothing, so the user is not one of the users with access
    // afterwards and the document is closed to them as it was before they asked.
    await openPermissionsForm(page, id, "manage")
    await expect(page.locator(".pd-permissionsform-item-request"), "the request the form lists").toHaveCount(1)
    const users = await page.locator(".pd-permissionsform-item-user").count()
    await page.locator(".pd-permissionsform-item-request .pd-permissionsform-button-deny").click()
    await expect(page.locator(".pd-permissionsform-item-request"), "the requests left once this one was denied").toHaveCount(0, { timeout: LOADING_TIMEOUT })
    await expect(page.locator(".pd-permissionsform-item-user"), "denying a request grants nobody anything").toHaveCount(users)
    await expect(userEntry(page, username), "the user whose request was denied").toHaveCount(0)
    await checkpointElement(page, page.locator(".pd-permissionsform"), "editpermissions-form-denied")
    await saveEdit(page)

    await openPermissions(page, id)
    await expect(page.locator(".pd-permissionsview-section-requests"), "no request is left on the document").toHaveCount(0)
    await expect(viewEntry(page, username), "the user whose request was denied is granted nothing").toHaveCount(0)

    await become(page, [ASKING_ROLE])
    expect(await statusOf(page, `/api/d/${id}`), "the interview after the request was denied").toBe(FORBIDDEN)

    // What was denied can be asked for again, so a denial is not the end of it: the page offers the action
    // once more rather than saying there is nothing left to ask for.
    await page.goto(`${PEERDB_URL}/d/request/${id}`)
    await expect(page.locator(".pd-documentrequest-form"), "the request form after the denial").toBeVisible({ timeout: LOADING_TIMEOUT })
    await expect(page.locator(`.pd-permissionactionsinput-checkbox-${ACTION_READ}`), "reading can be asked for again").toBeVisible()
    await expect(page.locator("#documentrequest-text-requested"), "nothing is waiting to be decided any more").toHaveCount(0)
    clearRefusedRequestErrors(page)

    await deleteDocument(page, id)

    console.log("Successfully asked for reading 1 interview, denied the request as the ethics role and found the document closed and the action askable again.")
  })
})
