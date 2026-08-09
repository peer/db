import { checkpoint, expect, PEERDB_URL, test } from "../utils"

test.describe("PeerDB Mock Sign-In", () => {
  test("Signing in with a chosen role", async ({ context }) => {
    const page = await context.newPage()

    await page.goto(PEERDB_URL)

    // A site without an issuer signs in through the mock, which asks which roles to sign in with instead
    // of taking the browser to an issuer.
    await page.locator("#navbar-button-signin").click()
    await expect(page).toHaveURL(/\/auth\/mock\?state=/)

    const mockSignIn = page.locator(".pd-authmocksignin")
    await expect(mockSignIn).toBeVisible()
    await checkpoint(page, "auth-mock-sign-in")
    await mockSignIn.getByRole("checkbox").first().check()
    await page.locator("#authmocksignin-button-signin").click()

    // Signing in lands back where it started, as the user the chosen roles make: the menu is named by
    // their username and holds the roles and the subject spelling them out.
    await expect(page).toHaveURL(PEERDB_URL + "/")
    const menuButton = page.locator(".pd-navbarmenu-button")
    await expect(menuButton).toHaveText("mock-admin")
    await menuButton.click()
    await expect(page.locator(".pd-navbaruser-roles")).toHaveText("admin")
    await expect(page.locator(".pd-navbaruser-id span")).toHaveText("mock-user-admin@peerdb-container")

    // The role is what the session holds, so what it grants is offered: creating documents here.
    await expect(page.getByRole("link", { name: "Create" })).toBeVisible()

    console.log("Successfully signed in through the mock sign-in page as a user holding the admin role.")
  })

  test("Signing in with no role at all", async ({ context }) => {
    const page = await context.newPage()

    await page.goto(PEERDB_URL)

    await page.locator("#navbar-button-signin").click()
    await expect(page).toHaveURL(/\/auth\/mock\?state=/)
    await expect(page.locator(".pd-authmocksignin")).toBeVisible()
    await checkpoint(page, "auth-mock-sign-in")

    // Choosing nothing signs in a user of the site who holds no role, whose subject names none either.
    await page.locator("#authmocksignin-button-signin").click()

    await expect(page).toHaveURL(PEERDB_URL + "/")
    const menuButton = page.locator(".pd-navbarmenu-button")
    await expect(menuButton).toHaveText("mock")
    await menuButton.click()
    await expect(page.locator(".pd-navbaruser-roles")).toHaveCount(0)
    await expect(page.locator(".pd-navbaruser-id span")).toHaveText("mock-user@peerdb-container")

    console.log("Successfully signed in through the mock sign-in page as a user holding no role.")
  })
})
