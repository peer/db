import type { Locator, Page } from "@playwright/test"

import type { EntityClass } from "../peerdb_utils"

import { PROPERTY_IDS, roleWhichCreates, startCreate } from "../peerdb_utils"
import { changePosted, expect, field, fieldErrors, fieldInput, hideDuplicates, LOADING_TIMEOUT, pressSave, settleEdit, signIn, test } from "../utils"

// The class these tests create. A sector is named (required, a string) and is contained in a galaxy
// (required, a reference), and the catalogue of four galaxies is short enough for the form to offer
// them as a list of options rather than as a combobox. That list is what makes the class the one to
// test the focus with: an option is a radio button, and a radio button which the form has locked is
// disabled rather than read-only, so it cannot take focus while the save is running (RadioButton.vue).
const SECTOR_CLASS: EntityClass = "SECTOR"

// The names of the documents these tests create all begin with this, so that a document made here is
// told apart from the ones another test file makes, and a leftover says which file made it.
const NAME_PREFIX = "PeerDB Edit Save"

// What the form says about a field which is required and holds nothing.
const REQUIRED_MESSAGE = "Required value."

const ROLE = roleWhichCreates(SECTOR_CLASS)

// The requests the form makes to write a change into the editing session. The save waits for every one
// of them to settle before it ends the session, which is what holding them parks the save in.
const SAVE_CHANGE = /\/api\/d\/saveChange\//

// The class an input carries while the form is locked (applied from the lock counter the save raises).
// The lock is what the inputs read to go read-only, so it says of the form what the save does to it
// rather than what any single input decided for itself.
const LOCKED = /pd-locked/

// Holds every change the form writes into the session until the returned function is called. The save
// then cannot get past the point where it waits for them, which is what makes the form readable while
// the save is running instead of racing it.
//
// Releasing leaves the route in place rather than taking it off, for the same reason holdFacetValues in
// the reloading tests does: a held request is resumed by the handler it is parked in, and taking the
// route off resumes it too, which would continue a request which has already been continued.
async function holdSaveChanges(page: Page): Promise<() => void> {
  let release: (() => void) | null = null
  const held = new Promise<void>((resolve) => {
    release = resolve
  })
  await page.route(SAVE_CHANGE, async (route) => {
    await held
    // The page may be gone by the time a held request is resumed, which is not a failure of what is tested.
    await route.continue().catch(() => null)
  })
  return () => release!()
}

// Starts creating a sector with its name filled in, which every class of the catalogue needs before it
// can be saved at all. The panel of potential duplicates is hidden for as long as the form is open: it
// lists what earlier runs saved, which would move everything below it in a screenshot.
async function startSector(page: Page, name: string): Promise<void> {
  await startCreate(page, SECTOR_CLASS)
  await hideDuplicates(page)

  const nameInput = fieldInput(page, PROPERTY_IDS.NAME, ".pd-inputstring")
  await expect(nameInput, "the name input of the new sector").toBeVisible({ timeout: LOADING_TIMEOUT })
  const posted = changePosted(page)
  await nameInput.fill(name)
  await nameInput.blur()
  await expect(nameInput, "the name input holds the entered name").toHaveValue(name)
  await posted
  await settleEdit(page)
}

// The options the galaxy field offers, one radio button each.
function galaxyOptions(page: Page): Locator {
  return field(page, PROPERTY_IDS.CONTAINED_IN).locator(".pd-claimrefselect-radio")
}

test.describe("PeerDB Edit Save Flows", () => {
  test("Test the form is locked for as long as the save runs", async ({ context }) => {
    const page = await context.newPage()

    await signIn(page, [ROLE])
    await startSector(page, `${NAME_PREFIX} Locked Sector`)

    // The galaxy is picked so that the save has nothing to complain about and runs to its end.
    const galaxy = galaxyOptions(page).first()
    await expect(galaxy, "the first galaxy the field offers").toBeVisible({ timeout: LOADING_TIMEOUT })
    const galaxyPosted = changePosted(page)
    await galaxy.click()
    await galaxyPosted
    await settleEdit(page)

    // A value is typed and left uncommitted, so that pressing save commits it and the save has a change
    // of its own to wait for. Every change from here on is held, which parks the save in that wait.
    const codeInput = fieldInput(page, PROPERTY_IDS.CATALOGUE_CODE, ".pd-inputidentifier")
    await expect(codeInput, "the catalogue code input").toBeVisible()
    await codeInput.fill("SEC-LOCKED-1")
    const release = await holdSaveChanges(page)

    await pressSave(page)

    // The form is locked while the save is waiting: it has validated what it was given and is writing,
    // and a value typed from here on would reach the session after the checks it was meant to pass.
    const nameInput = fieldInput(page, PROPERTY_IDS.NAME, ".pd-inputstring")
    await expect(nameInput, "the name input while the save is running").toHaveClass(LOCKED, { timeout: LOADING_TIMEOUT })
    await expect(nameInput, "the name input takes no typing while the save is running").toHaveAttribute("readonly", "")
    await expect(galaxyOptions(page).first(), "the galaxy options take no clicking while the save is running").toBeDisabled()
    await expect(page.locator("#documentedit-button-save"), "the save button while the save is running").toBeDisabled()
    // The session cannot have been ended yet: the save waits for the held change before it ends it.
    await expect(page.locator(".pd-documentget"), "the document view a finished save leads to").toHaveCount(0)

    release()

    // Once the change is let through the save runs to its end and the document is there to be read.
    await expect(page.locator(".pd-documentget"), "the document the save wrote").toBeVisible({ timeout: LOADING_TIMEOUT })

    console.log("Successfully verified that the form is locked while the save is writing and unlocked once it is done.")
  })

  test("Test a refused save focuses a field whose input is locked away while the save runs", async ({ context }) => {
    const page = await context.newPage()

    await signIn(page, [ROLE])
    await startSector(page, `${NAME_PREFIX} Refused Sector`)

    // Nothing is picked for the galaxy, so the save is refused over it. The name is filled, so it is the
    // only field the form has anything to say about and the only one the focus can land on.
    await expect(fieldErrors(page, PROPERTY_IDS.CONTAINED_IN), "complaints about the galaxy before the save").toHaveCount(0)

    await pressSave(page)

    await expect(fieldErrors(page, PROPERTY_IDS.CONTAINED_IN), "what the form says about the galaxy it was not given").toHaveText(REQUIRED_MESSAGE, {
      timeout: LOADING_TIMEOUT,
    })
    await expect(page.locator(".pd-fieldsform"), "the form of the editing session stays open").toBeVisible()
    await expect(page.locator(".pd-documentget"), "the document view a finished save leads to").toHaveCount(0)

    // The refused save takes the focus to what it complains about, which for this field is an option of a
    // list rather than a text box. An option is disabled for as long as the form is locked, so the focus
    // lands only because the save lets the lock go before it focuses (see onSave in DocumentEdit.vue).
    const galaxy = galaxyOptions(page).first()
    await expect(galaxy, "the galaxy options are usable again once the save is over").toBeEnabled()
    await expect(galaxy, "the first galaxy option takes the focus").toBeFocused()

    console.log("Successfully verified that a save refused over a field of options focuses the option it complains about.")
  })
})
