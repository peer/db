import type { Locator, Page } from "@playwright/test"

import { createNamed, documentIdOf, PROPERTY_IDS, startCreate } from "../peerdb_utils"
import { checkpointElement, discardEdit, expect, field, goHome, hideDuplicates, LOADING_TIMEOUT, openDocument, settleEdit, signIn, startEdit, test } from "../utils"

// The prefix every document this file creates is named with, so that the documents of one test file never
// collide with another's and a stray document says which file made it.
const PREFIX = "E2E Create Duplicates"

// The name the documents of the test which looks for a document it made itself are created with. It is one
// name for every run: a run which saves it makes the next run find what the run before it saved, which is
// exactly what that test is about.
const BIOME_NAME = `${PREFIX} Biome`

// The documents the panel is provoked into finding, together with the name and the code they carry. A
// document is named by the class mnemonic and the key which say which document of the test data it is, so
// the tests find the same document on every run, while the name and the code have to be written out: they
// are what a person types into a form when they are about to record something which is already there.
//
// Both documents are only read, never edited, so a test running next to this one still sees them.
const KEPHRA = await documentIdOf("STAR_SYSTEM", "G1_KEPHRA")
const KEPHRA_NAME = "Kephra"
const KEPHRA_CODE = "MW-1104-KEP"
const SURVEY_GRID_44_B = await documentIdOf("PLANET", "G1_SURVEY_GRID_44_B")
const SURVEY_GRID_44_B_NAME = "Survey Grid 44-b"
const SURVEY_GRID_44_B_CODE = "SG-44/b"

// A name and a code no document carries. The name is matched with every one of its words required and with
// room for a misspelling, so a name whose words are words of no language cannot be matched however it is
// spelled, and the code is matched as a whole, so one which is nothing but noise matches nothing.
const UNMATCHED_NAME = "Zzzqqqxyzzy Vurblenak"
const UNMATCHED_CODE = "ZZQ-0000-XYZ"

// The panel of potential duplicates of the document being created.
function duplicates(page: Page): Locator {
  return page.locator(".pd-documentduplicates")
}

// The entry of the panel which stands for one document, addressed by the document it links to.
function duplicateOf(page: Page, id: string): Locator {
  return duplicates(page).locator(`.pd-documentduplicates-item:has(.pd-documentduplicates-link[href$="/d/${id}"])`)
}

// Types into the first slot of a field, commits it, and waits until the panel has searched again with what
// the form holds now. The panel searches whenever the focus leaves the form, so without waiting for that
// search to answer an assertion would be about what the form said before this value was typed.
async function typeAndSearch(page: Page, propertyId: string, input: string, value: string, what: string): Promise<void> {
  const target = field(page, propertyId).locator(input).first()
  await expect(target, what).toBeVisible({ timeout: LOADING_TIMEOUT })
  const searched = page.waitForResponse((response) => response.url().includes("/api/d/findDuplicates"), { timeout: LOADING_TIMEOUT })
  await target.fill(value)
  await target.blur()
  await expect(target, `${what} after typing`).toHaveValue(value)
  await searched
  await settleEdit(page)
}

// The identifiers of the documents the panel lists, in the order it lists them, so a test can say which of
// them the panel put first without depending on what the rest of them are.
//
// The links are waited for before they are read: an entry renders its link once the document it stands for
// has arrived, and reading the elements does not wait for them the way an assertion does. Reading them too
// early hands back fewer identifiers than the panel has entries, or none at all, which the assertions
// around this can only report as the panel having found something else.
async function listed(page: Page): Promise<Array<string>> {
  const entries = duplicates(page).locator(".pd-documentduplicates-item")
  const links = duplicates(page).locator(".pd-documentduplicates-link")
  await expect(links, "the entries of the panel of potential duplicates link to their documents").toHaveCount(await entries.count(), { timeout: LOADING_TIMEOUT })
  return await links.evaluateAll((elements) => elements.map((link) => (link.getAttribute("href") || "").replace(/^.*\//, "")))
}

test.describe("PeerDB Create Duplicates Flows", () => {
  test("Test the panel of potential duplicates names the document the form resembles", async ({ context }) => {
    test.setTimeout(300_000)

    const page = await context.newPage()

    await signIn(page, ["surveyor"])
    await startCreate(page, "STAR_SYSTEM")

    // The panel is the only thing standing between an editor and a document recorded twice, so it has to
    // find the star system which already carries what is being typed, and to say which one it is rather
    // than only that there is one. The name and the code are both entered, because the panel weighs a
    // shared external identifier far above a shared name and this is what a person recording a star system
    // which is already there would type.
    await typeAndSearch(page, PROPERTY_IDS.NAME, ".pd-inputstring", KEPHRA_NAME, "the name of the star system being created")
    await typeAndSearch(page, PROPERTY_IDS.CATALOGUE_CODE, ".pd-inputidentifier", KEPHRA_CODE, "the catalogue code of the star system being created")

    await expect(duplicates(page), "the panel of potential duplicates").toBeVisible({ timeout: LOADING_TIMEOUT })
    await expect(duplicates(page).locator(".pd-documentduplicates-title"), "the panel says what it is a list of").not.toHaveText(/^\s*$/)
    const match = duplicateOf(page, KEPHRA)
    await expect(match, "the entry for the star system which already carries what was typed").toHaveCount(1, { timeout: LOADING_TIMEOUT })
    await expect(match, "the entry names the star system it leads to").not.toHaveText(/^\s*$/)

    // The panel is a ranking, and a document agreeing on both the name and the external identifier agrees
    // on more than anything else in the catalogue can, so it has to be at the top of it.
    expect((await listed(page))[0], "the document the panel puts first").toBe(KEPHRA)
    // The whole panel is not screenshotted: which documents follow the one which was provoked depends on
    // what the rest of the suite has recorded, while the entry which was provoked is this test's own.
    await checkpointElement(page, match, "create-duplicates-starsystem-match")

    // A form which no longer resembles that star system has to stop naming it: the panel is read as a
    // statement about what is on the form right now, not about what was on it a moment ago.
    await typeAndSearch(page, PROPERTY_IDS.NAME, ".pd-inputstring", UNMATCHED_NAME, "the name which resembles nothing")
    await typeAndSearch(page, PROPERTY_IDS.CATALOGUE_CODE, ".pd-inputidentifier", UNMATCHED_CODE, "the catalogue code which resembles nothing")
    await expect(duplicateOf(page, KEPHRA), "the entry for the star system after the form stopped resembling it").toHaveCount(0, { timeout: LOADING_TIMEOUT })

    // The session is left rather than saved. A document saved here would carry the name and the code which
    // are meant to resemble nothing, and the next run would then find it.
    await goHome(page)

    console.log("Successfully provoked the panel of potential duplicates into naming 1 document and into dropping it again.")
  })

  test("Test the panel of potential duplicates looks past the class being created", async ({ context }) => {
    test.setTimeout(300_000)

    const page = await context.newPage()

    await signIn(page, ["surveyor"])
    await startCreate(page, "SECTOR")

    // What is typed into the sector being created is the name and the catalogue code of a planet, which is
    // neither a sector nor anything a sector could be confused with by its class. The panel finds it
    // anyway, because it searches every document and weighs what the documents say rather than what they
    // are, which is what makes it able to catch the same thing recorded under two different classes.
    await typeAndSearch(page, PROPERTY_IDS.NAME, ".pd-inputstring", SURVEY_GRID_44_B_NAME, "the name of the sector being created")
    await typeAndSearch(page, PROPERTY_IDS.CATALOGUE_CODE, ".pd-inputidentifier", SURVEY_GRID_44_B_CODE, "the catalogue code of the sector being created")

    await expect(duplicates(page), "the panel of potential duplicates").toBeVisible({ timeout: LOADING_TIMEOUT })
    const match = duplicateOf(page, SURVEY_GRID_44_B)
    await expect(match, "the entry for the planet which already carries what was typed").toHaveCount(1, { timeout: LOADING_TIMEOUT })
    const found = await listed(page)
    expect(found[0], "the document the panel puts first").toBe(SURVEY_GRID_44_B)
    await checkpointElement(page, match, "create-duplicates-cross-class-match")

    await goHome(page)

    console.log(
      `Successfully verified that the panel of potential duplicates put a document of another class than the one being created first of the ${found.length} it listed.`,
    )
  })

  test("Test the panel of potential duplicates is offered while creating and not while editing", async ({ context }) => {
    test.setTimeout(300_000)

    const page = await context.newPage()

    await signIn(page, ["curator"])

    // The document the panel is then asked about is created here rather than taken from the test data, so
    // that this test writes only to documents of its own. The panel is hidden while it is created: it is
    // not what is being looked at yet, and from the second run on it lists what an earlier run saved.
    const id = await createNamed(page, "BIOME", BIOME_NAME)

    // Hiding the panel while the first entry was created is a style added to the page, and the interface
    // never loads the page again on its own, so the page is loaded again to bring the panel back.
    await goHome(page)

    // Creating a second entry under the name the first one carries is the case the panel exists for, and
    // the document it has to name is one this test made. An earlier run of this test saved a document under
    // the same name, so the panel can name that one instead, which is why what is asserted is that every
    // entry it lists is a document carrying this name rather than which one of them it is.
    await startCreate(page, "BIOME")
    await typeAndSearch(page, PROPERTY_IDS.NAME, ".pd-inputstring", BIOME_NAME, "the name of the entry being created")
    await expect(duplicates(page), "the panel of potential duplicates").toBeVisible({ timeout: LOADING_TIMEOUT })
    const entries = duplicates(page).locator(".pd-documentduplicates-item")
    const named = duplicates(page).locator(".pd-documentduplicates-item", { hasText: BIOME_NAME })
    await expect(named.first(), "the panel names an entry carrying the name which was typed").toBeVisible({ timeout: LOADING_TIMEOUT })
    // An entry which agrees on the name agrees on more than anything else the panel has to offer, so it is
    // what the panel puts at the top of its list.
    await expect(entries.first(), "the entry the panel puts first").toContainText(BIOME_NAME)
    const matched = await named.count()
    const found = await entries.count()

    // The second entry is left rather than saved, so that each run of this test adds one document and not
    // two.
    await goHome(page)

    // Editing a document which is already recorded is not a case the panel is for: the document is not
    // about to be recorded a second time, it is the one which is there. Nothing about the document changed
    // between the two, so the panel would have as much to say here as it had a moment ago.
    await openDocument(page, id)
    await startEdit(page)
    await expect(duplicates(page), "the panel of potential duplicates while editing a document which is already recorded").toHaveCount(0)
    const nameInput = field(page, PROPERTY_IDS.NAME).locator(".pd-inputstring").first()
    await expect(nameInput, "the name of the document being edited").toHaveValue(BIOME_NAME)
    await nameInput.click()
    await nameInput.blur()
    await settleEdit(page)
    await expect(duplicates(page), "the panel of potential duplicates after the focus left the form of a document being edited").toHaveCount(0)

    await discardEdit(page)

    console.log(
      `Successfully verified that the panel of potential duplicates is offered while creating, where ${matched} of the ${found} entries it listed carried the name which was typed, and never while editing (${id}).`,
    )
  })

  test("Test the panel of potential duplicates says nothing about a form nothing has been typed into", async ({ context }) => {
    test.setTimeout(300_000)

    const page = await context.newPage()

    await signIn(page, ["surveyor"])
    await startCreate(page, "STAR_SYSTEM")

    // A form nothing has been typed into resembles no document: it says only what class is being created,
    // and the panel is meant to hold its tongue about a bare class (a shared class alone is under the score
    // it takes to be reported, see minDuplicateScore in search/duplicates.go).
    //
    // This currently fails, and it is a defect rather than a test which expects too much. Every document
    // created through the interface is given the same five permission claims (read, read historic, update,
    // delete and update permissions, each scoped to the document itself), and the create session is given
    // them too before the form is shown. The duplicate search counts every reference claim of the document
    // being created, permission claims included, so those five alone are worth ten, which is over twice the
    // score it takes to be reported. Every document ever created through the interface therefore matches
    // every document being created, whatever either of them is about, and the panel fills up with documents
    // which have nothing to do with what is being recorded.
    const search = page.waitForResponse((response) => response.url().includes("/api/d/findDuplicates"), { timeout: LOADING_TIMEOUT })
    const nameInput = field(page, PROPERTY_IDS.NAME).locator(".pd-inputstring").first()
    await expect(nameInput, "the name of the star system being created").toBeVisible({ timeout: LOADING_TIMEOUT })
    await nameInput.click()
    await nameInput.blur()
    await search
    await settleEdit(page)

    const entries = await duplicates(page).locator(".pd-documentduplicates-item").allTextContents()
    expect(
      entries.map((entry) => entry.trim()),
      "the documents the panel lists for a form nothing has been typed into",
    ).toEqual([])

    await goHome(page)

    console.log(`Successfully verified that the panel of potential duplicates lists ${entries.length} documents for a form nothing has been typed into.`)
  })

  test("Test the form of a document being created can be read with the panel of potential duplicates hidden", async ({ context }) => {
    const page = await context.newPage()

    await signIn(page, ["surveyor"])
    await startCreate(page, "STAR_SYSTEM")

    // The panel sits under the form and grows as the index does, which is what every other test of this
    // suite hides before it screenshots a form being filled in. Hiding it has to leave the form itself
    // alone, which is what is checked here, so that the rest of the suite is hiding the panel and nothing
    // else.
    await typeAndSearch(page, PROPERTY_IDS.NAME, ".pd-inputstring", KEPHRA_NAME, "the name of the star system being created")
    await expect(duplicates(page), "the panel of potential duplicates before it is hidden").toBeVisible({ timeout: LOADING_TIMEOUT })
    const shown = await duplicates(page).locator(".pd-documentduplicates-item").count()

    await hideDuplicates(page)
    await expect(duplicates(page), "the panel of potential duplicates once it is hidden").toBeHidden()
    await expect(field(page, PROPERTY_IDS.NAME).locator(".pd-inputstring").first(), "the form once the panel is hidden").toHaveValue(KEPHRA_NAME)
    await expect(page.locator("#documentedit-button-save"), "the save button once the panel is hidden").toBeVisible()

    await goHome(page)

    console.log(`Successfully hid a panel of ${shown} potential duplicates of a document being created and left the form of it standing.`)
  })
})
