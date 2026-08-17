import { LANGUAGES, ROLES } from "../peerdb_utils"
import {
  checkpoint,
  checkpointElement,
  expect,
  expectOfferedRoles,
  goHome,
  mockUsername,
  openUserMenu,
  PEERDB_URL,
  signIn,
  signOut,
  switchLanguage,
  test,
} from "../utils"

// The roles the sign-in tests below hold. One role, several roles and none at all are the three shapes a
// sign-in takes, and the name the mock gives the user follows from them.
const ONE_ROLE = ["curator"] as const
const SEVERAL_ROLES = ["author", "researcher"] as const

test.describe("PeerDB Mock Sign-In Flows", () => {
  test("Test the mock sign-in page offers every role the site declares", async ({ context }) => {
    const page = await context.newPage()

    await goHome(page)
    // A site without an issuer signs in through the mock, which asks which roles to sign in with instead
    // of taking the browser to an issuer.
    await page.locator("#navbar-button-signin").click()
    await expect(page).toHaveURL(/\/auth\/mock\?state=/)

    const mockSignIn = page.locator(".pd-authmocksignin")
    await expect(mockSignIn, "the mock sign-in page").toBeVisible()
    await expect(page.locator("#authmocksignin-title"), "title of the sign-in page").toBeVisible()
    await expect(page.locator(".pd-authmocksignin-text-description"), "description of the sign-in page").not.toHaveText(/^\s*$/)
    await expect(page.locator(".pd-authmocksignin-form"), "sign-in form").toBeVisible()
    await expect(page.locator(".pd-authmocksignin-list-roles"), "list of roles").toBeVisible()
    await expect(page.locator("#authmocksignin-empty-roles"), "the site declares roles, so the empty notice is not shown").toHaveCount(0)
    await expect(page.locator("#authmocksignin-button-signin"), "sign in button").toBeVisible()

    // The page lists exactly the roles the site is configured with, one checkbox each, and each of them
    // carries the role in its own CSS class.
    await expectOfferedRoles(page, ROLES)
    for (const role of ROLES) {
      await expect(page.locator(`.pd-authmocksignin-checkbox-role-${role}`), `checkbox of the ${role} role`).toBeVisible()
      await expect(page.locator(`.pd-authmocksignin-checkbox-role-${role}`), `checkbox of the ${role} role starts unticked`).not.toBeChecked()
    }

    await checkpoint(page, "auth-mock-sign-in")

    console.log(`Successfully verified the mock sign-in page offers the ${ROLES.length} roles the site declares.`)
  })

  test("Test signing in with one role", async ({ context }) => {
    const page = await context.newPage()

    await signIn(page, ONE_ROLE)

    // Signing in lands back where it started, as the user the chosen roles make: the menu is named by
    // their username and holds the roles and the subject spelling them out.
    await expect(page).toHaveURL(PEERDB_URL + "/")
    await expect(page.locator(".pd-navbarmenu-button")).toHaveText(mockUsername(ONE_ROLE))
    await openUserMenu(page)
    await expect(page.locator(".pd-navbaruser-roles")).toHaveText(ONE_ROLE[0])
    await expect(page.locator(".pd-navbaruser-id span").first()).toHaveText(new RegExp(`^mock-user-${ONE_ROLE[0]}@`))
    await checkpointElement(page, page.locator(".pd-navbarmenu-panel"), "auth-signed-in-one-role")

    // The role is what the session holds, so what it grants is offered: creating documents here.
    await expect(page.locator(".pd-createbutton"), "the create button a role which may create earns").toBeVisible()

    console.log(`Successfully signed in through the mock sign-in page as a user holding the ${ONE_ROLE[0]} role.`)
  })

  test("Test signing in with several roles", async ({ context }) => {
    const page = await context.newPage()

    await signIn(page, SEVERAL_ROLES)

    // The mock names the user after every role it was given, in alphabetical order, and the subject is
    // built the same way, so holding two roles is one identity and not two.
    await expect(page.locator(".pd-navbarmenu-button")).toHaveText(mockUsername(SEVERAL_ROLES))
    await openUserMenu(page)
    const roles = page.locator(".pd-navbaruser-roles")
    for (const role of SEVERAL_ROLES) {
      await expect(roles, `the menu names the ${role} role`).toContainText(role)
    }
    await expect(page.locator(".pd-navbaruser-id span").first()).toHaveText(new RegExp(`^mock-user-${[...SEVERAL_ROLES].sort().join("-")}@`))
    await checkpointElement(page, page.locator(".pd-navbarmenu-panel"), "auth-signed-in-several-roles")

    console.log(`Successfully signed in holding ${SEVERAL_ROLES.join(" and ")}.`)
  })

  test("Test signing in with no role at all", async ({ context }) => {
    const page = await context.newPage()

    await signIn(page, [])

    // Choosing nothing signs in a user of the site who holds no role, whose subject names none either, and
    // who is therefore offered nothing beyond what a visitor is offered.
    await expect(page.locator(".pd-navbarmenu-button")).toHaveText("mock")
    await openUserMenu(page)
    await expect(page.locator(".pd-navbaruser-roles"), "a user holding no role has no roles to list").toHaveCount(0)
    await expect(page.locator(".pd-navbaruser-id span").first()).toHaveText(/^mock-user@/)
    await expect(page.locator(".pd-createbutton"), "signing in without a role grants no creating").toHaveCount(0)
    await checkpointElement(page, page.locator(".pd-navbarmenu-panel"), "auth-signed-in-no-role")

    console.log("Successfully signed in through the mock sign-in page as a user holding no role.")
  })

  test("Test signing out", async ({ context }) => {
    const page = await context.newPage()

    await signIn(page, ONE_ROLE)
    await expect(page.locator(".pd-navbarmenu-button")).toBeVisible()

    await signOut(page)

    // Signing out takes the user's menu away and brings the sign in button back, and with it goes
    // everything the role granted.
    await expect(page.locator(".pd-navbarmenu-button"), "the menu of the signed-in user is gone").toHaveCount(0)
    await expect(page.locator("#navbar-button-signin"), "the sign in button is back").toBeVisible()
    await expect(page.locator(".pd-createbutton"), "creating is no longer offered").toHaveCount(0)
    await checkpoint(page, "auth-signed-out")

    console.log("Successfully signed out again.")
  })

  test("Test the mock sign-in page asked for without a sign-in in progress", async ({ context }) => {
    const page = await context.newPage()

    // The page carries the state of the sign-in it belongs to. Asked for on its own there is none, so it
    // says so instead of offering a form which could not go anywhere.
    await page.goto(`${PEERDB_URL}/auth/mock`)
    await expect(page.locator(".pd-authmocksignin"), "the mock sign-in page").toBeVisible()
    await expect(page.locator("#authmocksignin-error-nostate"), "the page says there is no sign-in to complete").toBeVisible()
    await expect(page.locator(".pd-authmocksignin-form"), "no form is offered without a sign-in to complete").toHaveCount(0)
    await checkpoint(page, "auth-mock-sign-in-no-state")

    console.log("Successfully verified the mock sign-in page asked for without a sign-in in progress.")
  })

  for (const language of LANGUAGES) {
    test(`Test the mock sign-in page in ${language}`, async ({ context }) => {
      const page = await context.newPage()

      await goHome(page)
      await switchLanguage(page, language)
      await page.locator("#navbar-button-signin").click()
      await expect(page.locator(".pd-authmocksignin")).toBeVisible()

      // The page is part of the interface, so it is translated, while the roles it lists are the names the
      // site declares them under and are the same in every language.
      await expect(page.locator("#authmocksignin-title"), "title of the sign-in page").not.toHaveText(/^\s*$/)
      await expectOfferedRoles(page, ROLES)
      await checkpoint(page, `auth-mock-sign-in-${language}`)

      console.log(`Successfully verified the mock sign-in page in ${language}.`)
    })
  }
})
