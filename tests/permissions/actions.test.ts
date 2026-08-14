import type { Page } from "@playwright/test"

import type { Role } from "../peerdb_utils"

import { CLASS_IDS, CORE_CLASS_IDS, documentIdOf, RESTRICTED_CLASS, ROLE_CREATES, ROLES } from "../peerdb_utils"
import {
  checkpoint,
  checkpointElement,
  clearRefusedRequestErrors,
  expect,
  expectNothingLoading,
  fetchFromPage,
  goHome,
  LOADING_TIMEOUT,
  openDocument,
  openDocumentTab,
  PEERDB_URL,
  settleDocument,
  signIn,
  test,
} from "../utils"

// The documents the buttons are asserted on. The planet is of a public class which every identity reads
// and which no document-level permission claim touches, so what it offers follows from the roles alone.
// The observation is public to read as well and carries permission claims of its own, granting updating to
// the researcher's subject and deleting to the curator's, which is how a document hands out an action no
// role grant of the site gives (see section 5.4 of the test data). The plain observation carries no such
// claims and stands next to it.
const PUBLIC_DOCUMENT = await documentIdOf("PLANET", "G1_HOLLIS_III")
const PERMISSIONED_OBSERVATION = await documentIdOf("OBSERVATION", "OBSA_BED_LEFT_TO_FAIL")
const PLAIN_OBSERVATION = await documentIdOf("OBSERVATION", "OBSA_BENDER_PIECE_HANDED_BACK")
// An interview the ethics role reaches through its own grant of the restricted class, which is the one
// class it may decide access to.
const INTERVIEW = await documentIdOf(RESTRICTED_CLASS, "INTA_LONG_DEBT_FORTY_DAY_ACCOUNT")

// How many classes of the site can be started a new document of at all: every class which is not abstract
// and declares at least one field to fill in. It is what the role holding everything is offered, which is
// more than the working roles' lists put together, because the schema's own classes are creatable too.
const ALL_CREATABLE_CLASSES = 43

// What the server answers a caller who asks for a page they hold no action for.
const FORBIDDEN = 403

// The classes the curator role may start. The site grants it the core unit class next to its own, which is
// a class of the schema rather than of the test data, so it is added to the table the test data helpers
// carry (ROLE_CREATES only names classes the test data declares).
const CURATOR_CREATES = [...ROLE_CREATES.curator!.map((entityClass) => CLASS_IDS[entityClass]), CORE_CLASS_IDS.UNIT]

// One identity, with everything the site offers it. The roles are the ones to sign in with through the
// mock authenticator, or null for a visitor who does not sign in at all.
interface Identity {
  // What names the identity in a test title and in what a test reports.
  what: string
  // The suffix which tells this identity's screenshots apart from the others'.
  slug: string
  // The roles to sign in with, or null for not signing in.
  roles: ReadonlyArray<Role> | null
  // The identifiers of the classes the identity may start a new document of, or null when it may start any
  // of them. An empty list is an identity which may create nothing, which is offered no create button and
  // is refused the create page outright.
  creates: ReadonlyArray<string> | null
  // Whether the document view offers editing and deleting a document of a public class.
  edits: boolean
  deletes: boolean
  // Whether the permissions tab of a document of a public class offers changing what is granted.
  updatesPermissions: boolean
}

const IDENTITIES: ReadonlyArray<Identity> = [
  { what: "a visitor who is not signed in", slug: "visitor", roles: null, creates: [], edits: false, deletes: false, updatesPermissions: false },
  { what: "a user signed in with no role", slug: "norole", roles: [], creates: [], edits: false, deletes: false, updatesPermissions: false },
  // Bulk reading is about enumerating the store, so it offers nothing on a document at all.
  { what: "the bulk role", slug: "bulk", roles: ["bulk"], creates: [], edits: false, deletes: false, updatesPermissions: false },
  {
    what: "the surveyor role",
    slug: "surveyor",
    roles: ["surveyor"],
    creates: ROLE_CREATES.surveyor!.map((entityClass) => CLASS_IDS[entityClass]),
    edits: true,
    deletes: false,
    updatesPermissions: false,
  },
  {
    what: "the researcher role",
    slug: "researcher",
    roles: ["researcher"],
    creates: ROLE_CREATES.researcher!.map((entityClass) => CLASS_IDS[entityClass]),
    edits: true,
    deletes: false,
    updatesPermissions: false,
  },
  {
    what: "the author role",
    slug: "author",
    roles: ["author"],
    creates: ROLE_CREATES.author!.map((entityClass) => CLASS_IDS[entityClass]),
    edits: true,
    deletes: false,
    updatesPermissions: false,
  },
  { what: "the curator role", slug: "curator", roles: ["curator"], creates: CURATOR_CREATES, edits: true, deletes: false, updatesPermissions: false },
  // The ethics committee is granted updating on the restricted class and on the protocols alone, so on a
  // document of any other class it is offered nothing, however much of it it may read.
  {
    what: "the ethics role",
    slug: "ethics",
    roles: ["ethics"],
    creates: ROLE_CREATES.ethics!.map((entityClass) => CLASS_IDS[entityClass]),
    edits: false,
    deletes: false,
    updatesPermissions: false,
  },
  { what: "the admin role", slug: "admin", roles: ["admin"], creates: null, edits: true, deletes: true, updatesPermissions: true },
]

// Puts the browser in the state the identity describes.
async function become(page: Page, identity: Identity): Promise<void> {
  if (identity.roles === null) {
    await goHome(page)
  } else {
    await signIn(page, identity.roles)
  }
}

// The identifiers of the classes the create page offers to start a document of, read out of the CSS class
// every class button carries. A class which is a subclass of more than one class is listed once under each
// of them, so the identifiers are reported as a set.
async function offeredClasses(page: Page): Promise<Array<string>> {
  const prefix = "pd-classtreelabel-button-"
  const ids = await page
    .locator(".pd-classtreelabel-button")
    .evaluateAll(
      (buttons, cssPrefix) =>
        buttons.flatMap((button) => [...button.classList].filter((name) => name.startsWith(cssPrefix)).map((name) => name.slice(cssPrefix.length))),
      prefix,
    )
  return [...new Set(ids)].sort()
}

// The identifiers of the classes the create page would offer, asked of the endpoint the page builds itself
// from. It is used for a role which may start exactly one class: opening the create page then skips the
// picker and starts creating that class straight away, which a test which changes nothing must not do.
async function creatableClasses(page: Page): Promise<Array<string>> {
  const response = await fetchFromPage(page, "/api/d/createOptions")
  expect(response.status, "the create options").toBe(200)
  const options = JSON.parse(response.body) as { classes: Array<{ id: string; creatable?: boolean }> }
  return options.classes
    .filter((entry) => entry.creatable)
    .map((entry) => entry.id)
    .sort()
}

test.describe("PeerDB Permitted Action Flows", () => {
  test("Test the identities asserted on cover every role the site declares", async ({ context }) => {
    const page = await context.newPage()

    // What each role is offered is written out rather than derived, so a role added to the site without a
    // line of its own in the table would be tested by nothing.
    const covered = IDENTITIES.filter((identity) => identity.roles?.length === 1).map((identity) => identity.roles![0])
    expect([...covered].sort(), "the identities cover exactly the roles the site declares").toEqual([...ROLES].sort())

    // Every role the site's create grants name may create, and no other role may.
    const creating = IDENTITIES.filter((identity) => identity.roles?.length === 1 && identity.creates?.length !== 0).map((identity) => identity.roles![0])
    expect([...creating].sort(), "the roles which may create are the ones the create grants name").toEqual([...Object.keys(ROLE_CREATES), "admin"].sort())

    await goHome(page)
    console.log(`Successfully verified that the ${IDENTITIES.length} identities asserted on cover the ${ROLES.length} roles the site declares.`)
  })

  for (const identity of IDENTITIES) {
    test(`Test what ${identity.what} is offered to create`, async ({ context }) => {
      const page = await context.newPage()

      await become(page, identity)
      const createButton = page.locator(".pd-createbutton")

      if (identity.creates?.length === 0) {
        // Creating is granted per class, so a role which is named by no create grant is offered no button
        // at all, and the page behind it is refused by the server rather than shown empty.
        await expect(createButton, `${identity.what} is offered no create button`).toHaveCount(0)
        const opened = await page.goto(`${PEERDB_URL}/d/create`)
        expect(opened?.status(), `the create page asked for by ${identity.what}`).toBe(FORBIDDEN)

        console.log(`Successfully verified that ${identity.what} is offered no creating at all.`)
        return
      }

      await expect(createButton, `${identity.what} is offered a create button`).toBeVisible()

      // A role which may start exactly one class is not taken through the picker at all: the create page
      // starts that class straight away. What the page would list is therefore asked of the endpoint the
      // page builds itself from, so that nothing is created by looking.
      if (identity.creates !== null && identity.creates.length === 1) {
        expect(await creatableClasses(page), `the classes ${identity.what} may start`).toEqual([...identity.creates].sort())

        console.log(`Successfully verified that ${identity.what} may start exactly ${identity.creates.length} class.`)
        return
      }

      await createButton.click()
      await expect(page.locator(".pd-documentcreate"), "the create page").toBeVisible({ timeout: LOADING_TIMEOUT })
      await expect(page.locator("#documentcreate-title"), "title of the create page").toBeVisible({ timeout: LOADING_TIMEOUT })
      await expect(page.locator("#documentcreate-loading"), "the create page has loaded its classes").toHaveCount(0)
      await expectNothingLoading(page)
      const offered = await offeredClasses(page)

      if (identity.creates === null) {
        // The role holding everything is offered every class which can be started at all, which includes
        // the classes of the schema itself and the class the site keeps out of the public read scope.
        expect(offered, "the role holding everything is offered every class which can be started").toHaveLength(ALL_CREATABLE_CLASSES)
        expect(offered, "the role holding everything is offered the restricted class as well").toContain(CLASS_IDS[RESTRICTED_CLASS])
        for (const [role, classes] of Object.entries(ROLE_CREATES)) {
          for (const entityClass of classes) {
            expect(offered, `the role holding everything is offered what the ${role} role may start`).toContain(CLASS_IDS[entityClass])
          }
        }
      } else {
        // Anything else is offered exactly what the site's create grants name for its role, and the classes
        // above those in the tree are rendered as headings rather than as buttons, so nothing which cannot
        // be started is offered.
        expect(offered, `the classes ${identity.what} may start`).toEqual([...identity.creates].sort())
      }

      // The class tree is ordered by how many documents each class holds, and the suite creates documents,
      // so the order of its entries is not the same from one run to the next. What each identity is offered
      // is asserted above; the screenshot is of the page around the tree.
      await checkpoint(page, `permissions-actions-create-${identity.slug}`, { mask: [page.locator(".pd-classtreelist")] })

      console.log(`Successfully verified that ${identity.what} is offered ${offered.length} classes to start a document of.`)
    })
  }

  for (const identity of IDENTITIES) {
    test(`Test what ${identity.what} is offered on a document`, async ({ context }) => {
      const page = await context.newPage()

      await become(page, identity)
      await openDocument(page, PUBLIC_DOCUMENT)
      await settleDocument(page)

      // The two buttons of the document sidebar are shown for the actions the caller holds on the document
      // being looked at, so which of them are there says what the roles come to on a public class.
      const edit = page.locator("#documentget-button-edit")
      const remove = page.locator("#documentget-button-delete")
      if (identity.edits) {
        await expect(edit, `${identity.what} is offered editing`).toBeVisible()
      } else {
        await expect(edit, `${identity.what} is offered no editing`).toHaveCount(0)
      }
      if (identity.deletes) {
        await expect(remove, `${identity.what} is offered deleting`).toBeVisible()
      } else {
        await expect(remove, `${identity.what} is offered no deleting`).toHaveCount(0)
      }

      // The permissions tab is where changing what a document grants is offered from, so it is the third
      // thing a document offers on top of editing and deleting.
      await openDocumentTab(page, "permissions")
      await expectNothingLoading(page)

      // This document's access is decided by the roles alone, so it grants nobody anything of its own and
      // the tab says so.
      await expect(page.locator(".pd-permissionsview-empty-users"), "the document grants nobody anything of its own").toBeVisible()
      await expect(page.locator(".pd-permissionsview-section-requests"), "no access request is waiting on this document").toHaveCount(0)

      const editPermissions = page.locator(".pd-permissionsview-button-edit")
      const requestPermissions = page.locator(".pd-permissionsview-button-request")
      if (identity.updatesPermissions) {
        await expect(editPermissions, `${identity.what} is offered changing what is granted`).toBeVisible()
        await expect(requestPermissions, `${identity.what} has nothing to ask for`).toHaveCount(0)
      } else {
        await expect(editPermissions, `${identity.what} is offered no changing of what is granted`).toHaveCount(0)
        // Asking for access records who asked, so it is offered only to a caller who signed in.
        if (identity.roles === null) {
          await expect(requestPermissions, "a visitor who is not signed in is offered no asking for access").toHaveCount(0)
        } else {
          await expect(requestPermissions, `${identity.what} is offered asking for access`).toBeVisible()
        }
      }

      // The delete page is gated by the same action as the button leading to it, and the server refuses the
      // address itself rather than serving a page which could not go through.
      const opened = await page.goto(`${PEERDB_URL}/d/delete/${PUBLIC_DOCUMENT}`)
      expect(opened?.status(), `the delete page asked for by ${identity.what}`).toBe(identity.deletes ? 200 : FORBIDDEN)

      console.log(
        `Successfully verified that ${identity.what} is offered ${identity.edits ? "editing" : "no editing"}, ${identity.deletes ? "deleting" : "no deleting"} and ` +
          `${identity.updatesPermissions ? "changing" : "no changing"} of what a document of a public class grants.`,
      )
    })
  }

  test("Test a document's own claims offer deleting to a role which is granted none", async ({ context }) => {
    const page = await context.newPage()

    // The curator role holds no deleting anywhere, and one observation of the test data hands its subject
    // exactly that action, so the button follows the document rather than the role.
    await signIn(page, ["curator"])
    await openDocument(page, PERMISSIONED_OBSERVATION)
    await settleDocument(page)
    await expect(page.locator("#documentget-button-delete"), "deleting the observation whose claims grant it").toBeVisible()
    await checkpointElement(page, page.locator("#documentget-sidebar"), "permissions-actions-sidebar-claimed-delete")

    // The delete page is opened by the same claim, which is what says the grant reaches the server and not
    // only the buttons.
    const opened = await page.goto(`${PEERDB_URL}/d/delete/${PERMISSIONED_OBSERVATION}`)
    expect(opened?.status(), "the delete page of the observation whose claims grant deleting").toBe(200)
    await expect(page.locator("#documentdelete-title"), "title of the delete page").toBeVisible({ timeout: LOADING_TIMEOUT })
    await expect(page.locator("#documentdelete-text-confirm"), "the delete page asks for a confirmation").toBeVisible()
    await expect(page.locator("#documentdelete-button-delete"), "the delete page offers going through with it").toBeVisible()
    await expect(page.locator("#documentdelete-button-cancel"), "the delete page offers backing out").toBeVisible()
    await expect(page.locator("#documentdelete-text-notallowed"), "the delete page does not refuse a caller who holds the action").toHaveCount(0)
    await expectNothingLoading(page)
    await checkpoint(page, "permissions-actions-delete-page")

    // Another observation of the same class carries no such claim, so the same role is offered nothing on
    // it and the same page is refused.
    await openDocument(page, PLAIN_OBSERVATION)
    await settleDocument(page)
    await expect(page.locator("#documentget-button-delete"), "deleting an observation whose claims grant nothing").toHaveCount(0)
    const refused = await page.goto(`${PEERDB_URL}/d/delete/${PLAIN_OBSERVATION}`)
    expect(refused?.status(), "the delete page of an observation whose claims grant nothing").toBe(FORBIDDEN)

    console.log("Successfully verified that deleting is offered on the observation whose own claims grant it and on no other document of the same class.")
  })

  test("Test a visitor is offered nothing on the document whose claims grant a role something", async ({ context }) => {
    const page = await context.newPage()

    // The claims of that observation name subjects, and a visitor is nobody, so the document offers them
    // exactly what any other public document does.
    await goHome(page)
    await openDocument(page, PERMISSIONED_OBSERVATION)
    await settleDocument(page)
    await expect(page.locator("#documentget-button-edit"), "the edit button").toHaveCount(0)
    await expect(page.locator("#documentget-button-delete"), "the delete button").toHaveCount(0)
    await expect(page.locator(".pd-createbutton"), "the create button").toHaveCount(0)

    // Its permission claims are still part of the document, so the permissions tab lists them to whoever
    // may read it, which is everybody here.
    await openDocumentTab(page, "permissions")
    await expectNothingLoading(page)
    await expect(page.locator(".pd-permissionsview-item-user"), "the users the observation's claims name").toHaveCount(2)

    // Looking a user up is refused to a caller who is not signed in, so each of the two is named by the
    // subject the claim records instead of by their username. The browser logs the refused lookup whatever
    // the application then does with it, so those errors are cleared before the tab is captured.
    const named = page.locator(".pd-permissionsview-item-user .pd-identityinline-error")
    await expect(named, "the users are named by their subject to a caller who cannot look them up").toHaveCount(2)
    await expect(named.first(), "the subject a claim records names the user it grants to").not.toHaveText(/^\s*$/)
    clearRefusedRequestErrors(page)
    await checkpointElement(page, page.locator(".pd-permissionsview"), "permissions-actions-permissions-tab-observation")

    console.log("Successfully verified that a visitor is offered nothing on the observation whose claims grant a role deleting, while still reading those claims.")
  })

  test("Test the ethics role is offered changing permissions on the restricted class alone", async ({ context }) => {
    const page = await context.newPage()

    // Updating permissions is granted to this role on the restricted class only, so the same role is
    // offered it on an interview and offered asking for it on anything else.
    await signIn(page, ["ethics"])
    await openDocument(page, INTERVIEW)
    await openDocumentTab(page, "permissions")
    await expectNothingLoading(page)
    await expect(page.locator(".pd-permissionsview-button-edit"), "changing what the interview grants").toBeVisible()
    await expect(page.locator(".pd-permissionsview-button-request"), "a role which may change what is granted asks for nothing").toHaveCount(0)

    await openDocument(page, PUBLIC_DOCUMENT)
    await openDocumentTab(page, "permissions")
    await expectNothingLoading(page)
    await expect(page.locator(".pd-permissionsview-button-edit"), "changing what a document of a public class grants").toHaveCount(0)
    await expect(page.locator(".pd-permissionsview-button-request"), "the same role is left with asking for access").toBeVisible()

    console.log("Successfully verified that the ethics role may change what an interview grants and may not change what a document of any other class grants.")
  })
})
