import type { Locator, Page } from "@playwright/test"

import { CLASS_IDS, documentIdOf, PROPERTY_IDS } from "../peerdb_utils"
import {
  checkpoint,
  checkpointFormAt,
  createShortcutButton,
  expect,
  expectResults,
  field,
  fillSlot,
  goHome,
  hideDuplicates,
  loadAllResults,
  LOADING_TIMEOUT,
  openDocument,
  openDocumentTab,
  PEERDB_URL,
  pressCreateShortcut,
  propertyValues,
  resultCount,
  resultIds,
  saveEdit,
  settle,
  settleEdit,
  shortcutCount,
  signIn,
  test,
  volatile,
} from "../utils"

// The prefix every document this file creates is named with, so that the documents of one test file never
// collide with another's and a stray document says which file made them.
const PREFIX = "E2E Create Shortcut"

const PLANET_NAME = `${PREFIX} Planet`
const OBSERVATION_NAME = `${PREFIX} Observation`

// The documents the shortcuts are driven from, and the documents which already point at them. Each is
// named by the class mnemonic and the key which say which document of the test data it is, so the tests
// pick the same documents on every run without depending on labels, which differ between the languages.
const KEPHRA = await documentIdOf("STAR_SYSTEM", "G1_KEPHRA")
const KEPHRA_PLANETS = [await documentIdOf("PLANET", "G1_KEPHRA_TWO"), await documentIdOf("PLANET", "G1_KEPHRA_FOUR")]
const GRID_44_THIRD = await documentIdOf("EXPEDITION", "EXP_GRID_44_THIRD")
const GRID_44_THIRD_OBSERVATIONS = [await documentIdOf("OBSERVATION", "OBSA_DUSK_SIGN_REFUSAL"), await documentIdOf("OBSERVATION", "OBSA_NIGHT_WARREN_BINS")]

// How long the search index is given to catch up with a document these tests have just saved. The index is
// written to after a save has returned, so a count fetched right after a save can still be the old one, and
// the tests of this suite write next to each other, which makes the catching up take longer than the wait a
// view has to be given.
const INDEX_CATCHUP_TIMEOUT = 120000

// The whole sidebar row of that same shortcut, which holds the search link and its count next to the "+".
// The row is addressed by what its search link does rather than by the "+" next to it, so that it is found
// for a caller who may not create and is offered no "+" at all.
function shortcutRow(page: Page, classId: string, propertyId: string, selfId: string): Locator {
  return page.locator(`.pd-documentget-link-shortcut:has(.pd-searchshortcutlink-link[href*="${propertyId}=${selfId}"][href*="${PROPERTY_IDS.INSTANCE_OF}=${classId}"])`)
}

// Asserts that the document being edited holds exactly one claim pointing the given property at the given
// document, read off the claims of the session rather than off the form, so it says what will be saved.
async function expectSavedReference(page: Page, propertyId: string, targetId: string, what: string): Promise<void> {
  await expect(propertyValues(page, propertyId).locator(`.pd-claimvalueref[data-url="/api/d/${targetId}"]`), what).toHaveCount(1)
}

test.describe("PeerDB Create Shortcut Flows", () => {
  test("Test the planets create shortcut of a star system", async ({ context }) => {
    // The test creates a document and then waits for the search index to catch up with it, which is more
    // than the default budget of a test allows for.
    test.setTimeout(600_000)

    const page = await context.newPage()

    await signIn(page, ["surveyor"])
    await openDocument(page, KEPHRA)
    await settle(page)

    // A star system declares one shortcut, and it is a shortcut of both kinds at once: it searches for the
    // planets of this system and it offers creating one of them.
    const sidebar = page.locator(".pd-documentget-list-shortcuts")
    await expect(sidebar, "the sidebar of the star system").toBeVisible()
    await expect(page.locator(".pd-documentget-link-shortcut"), "the shortcuts a star system declares").toHaveCount(1)
    const planets = shortcutRow(page, CLASS_IDS.PLANET, PROPERTY_IDS.CONTAINED_IN, KEPHRA)
    await expect(planets, "the planets shortcut of the star system").toHaveCount(1)
    await expect(createShortcutButton(page, CLASS_IDS.PLANET, PROPERTY_IDS.CONTAINED_IN, KEPHRA), "the create button of the planets shortcut").toBeVisible()
    // Every document also offers the search of what points at it, which declares no creating at all.
    await expect(page.locator("#documentget-button-referencedby"), "the referenced by shortcut").toBeVisible()
    await expect(page.locator("#documentget-button-referencedby .pd-searchshortcutlink-button-create"), "the referenced by shortcut offers no creating").toHaveCount(0)

    const before = await shortcutCount(planets)

    // The search half of the shortcut has to find what it counts: the planets of the test data which are in
    // this system are among the results, and there are at least as many results as the count promised, which
    // was read before the search ran and can only have grown since. Only what the test data holds is named,
    // because the tests of this suite add planets to this system as they run.
    await planets.locator(".pd-searchshortcutlink-link").click()
    await expectResults(page)
    expect(await resultCount(page), "how many documents the planets shortcut finds against what it counted").toBeGreaterThanOrEqual(before)
    await loadAllResults(page)
    const found = await resultIds(page)
    for (const planet of KEPHRA_PLANETS) {
      expect(found, `the planets shortcut finds the planet ${planet} of the test data`).toContain(planet)
    }

    await openDocument(page, KEPHRA)
    await settle(page)
    await pressCreateShortcut(page, createShortcutButton(page, CLASS_IDS.PLANET, PROPERTY_IDS.CONTAINED_IN, KEPHRA))

    // The field the shortcut names is filled in before the editor is handed over, and it is filled in as a
    // change of the session rather than as something already recorded, so the field says it was changed.
    const containedIn = field(page, PROPERTY_IDS.CONTAINED_IN)
    await expect(containedIn.locator(".pd-inputref-value").first(), "the star system field holds a picked document").toBeVisible()
    await expect(
      containedIn.locator(`.pd-inputref-link-document[href$="/d/${KEPHRA}"]`),
      "the star system field holds the star system the shortcut was pressed on",
    ).toBeVisible()
    await expect(containedIn.locator(".pd-inputbadges-button-revert"), "the field the shortcut filled in can be taken back").toBeVisible()

    // The star system is the only field a planet requires besides its name, so a planet reached this way
    // needs nothing but a name.
    await fillSlot(page, PROPERTY_IDS.NAME, 0, ".pd-inputstring", PLANET_NAME, 2, "the name of the new planet")
    await checkpointFormAt(page, page.locator("#section-identification"), "create-shortcuts-planet-editor")

    const planetId = await saveEdit(page)
    expect(planetId, "the planet was saved under an identifier of its own").not.toBe(KEPHRA)
    await checkpoint(page, "create-shortcuts-planet-document", { mask: volatile(page) })

    // What the shortcut promised has to be what was saved: a planet, in the star system it was pressed on.
    await openDocumentTab(page, "allproperties")
    await expectSavedReference(page, PROPERTY_IDS.INSTANCE_OF, CLASS_IDS.PLANET, "the created document is a planet")
    await expectSavedReference(page, PROPERTY_IDS.CONTAINED_IN, KEPHRA, "the created planet is in the star system the shortcut was pressed on")

    // The count the shortcut shows comes from a search, which is answered from the index, so the star system
    // is opened again until the index has the planet. The count is only asserted to have grown: the tests of
    // this suite write next to each other and the index catches up with all of them at its own pace, so by
    // how much it grew is not this test's to say. What is this test's to say is that the search the shortcut
    // runs finds the planet which was created through it, which is what is asserted next.
    await expect
      .poll(
        async () => {
          await openDocument(page, KEPHRA)
          return await shortcutCount(planets)
        },
        { message: "the planets shortcut of the star system counts the new planet", timeout: INDEX_CATCHUP_TIMEOUT },
      )
      .toBeGreaterThan(before)
    const after = await shortcutCount(planets)

    await planets.locator(".pd-searchshortcutlink-link").click()
    await expectResults(page)
    await loadAllResults(page)
    expect(await resultIds(page), "the planets shortcut finds the planet which was created through it").toContain(planetId)

    console.log(`Successfully created a planet through the create shortcut of a star system, which took the shortcut's count from ${before} to ${after}.`)
  })

  test("Test the observations create shortcut of an expedition", async ({ context }) => {
    test.setTimeout(600_000)

    const page = await context.newPage()

    await signIn(page, ["researcher"])
    await openDocument(page, GRID_44_THIRD)
    await settle(page)

    const observations = shortcutRow(page, CLASS_IDS.OBSERVATION, PROPERTY_IDS.PART_OF_EXPEDITION, GRID_44_THIRD)
    await expect(observations, "the observations shortcut of the expedition").toHaveCount(1)
    const before = await shortcutCount(observations)

    await observations.locator(".pd-searchshortcutlink-link").click()
    await expectResults(page)
    expect(await resultCount(page), "how many documents the observations shortcut finds against what it counted").toBeGreaterThanOrEqual(before)
    await loadAllResults(page)
    const found = await resultIds(page)
    for (const observation of GRID_44_THIRD_OBSERVATIONS) {
      expect(found, `the observations shortcut finds the observation ${observation} of the test data`).toContain(observation)
    }

    await openDocument(page, GRID_44_THIRD)
    await settle(page)
    await pressCreateShortcut(page, createShortcutButton(page, CLASS_IDS.OBSERVATION, PROPERTY_IDS.PART_OF_EXPEDITION, GRID_44_THIRD))

    // This shortcut fills in a field the class does not require, so the prefill has to land on that field
    // and the field the class does require has to be left empty for the user to fill in.
    const expedition = field(page, PROPERTY_IDS.PART_OF_EXPEDITION)
    await expect(
      expedition.locator(`.pd-inputref-link-document[href$="/d/${GRID_44_THIRD}"]`),
      "the expedition field holds the expedition the shortcut was pressed on",
    ).toBeVisible()
    await expect(field(page, PROPERTY_IDS.NAME).locator(".pd-inputbadges-badge-required"), "the name of an observation is required").toBeVisible()
    await expect(field(page, PROPERTY_IDS.NAME).locator(".pd-inputstring").first(), "the name of the new observation starts out empty").toHaveValue("")

    await fillSlot(page, PROPERTY_IDS.NAME, 0, ".pd-inputstring", OBSERVATION_NAME, 2, "the name of the new observation")
    await checkpoint(page, "create-shortcuts-observation-editor")

    const observationId = await saveEdit(page)
    expect(observationId, "the observation was saved under an identifier of its own").not.toBe(GRID_44_THIRD)
    await checkpoint(page, "create-shortcuts-observation-document", { mask: volatile(page) })

    await openDocumentTab(page, "allproperties")
    await expectSavedReference(page, PROPERTY_IDS.INSTANCE_OF, CLASS_IDS.OBSERVATION, "the created document is an observation")
    await expectSavedReference(page, PROPERTY_IDS.PART_OF_EXPEDITION, GRID_44_THIRD, "the created observation is part of the expedition the shortcut was pressed on")

    await expect
      .poll(
        async () => {
          await openDocument(page, GRID_44_THIRD)
          return await shortcutCount(observations)
        },
        { message: "the observations shortcut of the expedition counts the new observation", timeout: INDEX_CATCHUP_TIMEOUT },
      )
      .toBeGreaterThan(before)
    const after = await shortcutCount(observations)

    await observations.locator(".pd-searchshortcutlink-link").click()
    await expectResults(page)
    await loadAllResults(page)
    expect(await resultIds(page), "the observations shortcut finds the observation which was created through it").toContain(observationId)

    console.log(`Successfully created an observation through the create shortcut of an expedition, which took the shortcut's count from ${before} to ${after}.`)
  })

  test("Test a create shortcut skips the class picker and going back creates nothing", async ({ context }) => {
    test.setTimeout(600_000)

    const page = await context.newPage()

    // Every request which starts a document is counted, because what this test is about is a second document
    // being created without the user asking for one, which nothing on the page would show.
    let created = 0
    page.on("request", (request) => {
      if (request.method() === "POST" && /\/api\/d\/create(\?|$)/.test(request.url())) {
        created += 1
      }
    })

    await signIn(page, ["surveyor"])
    await openDocument(page, KEPHRA)
    await settle(page)

    await pressCreateShortcut(page, createShortcutButton(page, CLASS_IDS.PLANET, PROPERTY_IDS.CONTAINED_IN, KEPHRA))
    expect(created, "how many documents the press of the create shortcut started").toBe(1)

    // A shortcut which creates a single class replaces the create view with the editor instead of pushing
    // it, so going back from the editor lands on the document the shortcut was pressed on. Without that the
    // create view would come back, see a single class again, and start a second document nobody asked for.
    await page.goBack()
    await expect(page, "going back from the editor lands on the star system").toHaveURL(new RegExp(`/d/${KEPHRA}$`))
    await expect(page.locator("#documentget-title"), "the star system the shortcut was pressed on is shown again").toBeVisible()
    await expect(page.locator(".pd-documentcreate"), "the create view after going back").toHaveCount(0)
    await settle(page)
    expect(created, "how many documents were started once the editor was left again").toBe(1)

    // The other half of the same rule: a class which has subclasses to choose between cannot be skipped to,
    // so the picker is shown and nothing is created until a class is pressed. The world class of the test
    // data gathers exactly the two classes a document can be created for.
    await page.goto(`${PEERDB_URL}/d/create?limit=${CLASS_IDS.WORLD}`)
    await expect(page.locator("#documentcreate-title"), "the title of the create view").toBeVisible({ timeout: LOADING_TIMEOUT })
    await settle(page)
    await expect(page.locator(".pd-classtreelist").first(), "the class picker").toBeVisible()
    for (const entityClass of ["PLANET", "MOON"] as const) {
      await expect(page.locator(`.pd-classtreelabel-button-${CLASS_IDS[entityClass]}`), `the ${entityClass} button of the picker`).toHaveCount(1)
    }
    await expect(page.locator(".pd-classtreelabel-button"), "how many classes the picker offers").toHaveCount(2)
    await expect(page.locator(".pd-documentedit"), "the editor is not opened without a class being chosen").toHaveCount(0)
    expect(created, "how many documents the picker started on its own").toBe(1)
    await checkpoint(page, "create-shortcuts-picker-classes")

    // A limit which resolves to a single class is the case the shortcut takes: the editor opens straight
    // away, which is the same behaviour reached without a shortcut being involved at all.
    await page.goto(`${PEERDB_URL}/d/create?limit=${CLASS_IDS.MOON}`)
    await settleEdit(page)
    await hideDuplicates(page)
    await expect(page, "a limit of a single class opens an editing session").toHaveURL(/\/d\/edit\/[0-9A-Za-z]+\/[0-9A-Za-z]+/)
    expect(created, "how many documents were started in this test").toBe(2)

    // Both sessions are left rather than saved: what they were opened for is that they were opened at all.
    await goHome(page)

    console.log(
      "Successfully verified that a create shortcut of a single class skips the picker, that going back creates nothing, and that 2 classes bring the picker back.",
    )
  })

  test("Test the create half of a shortcut is offered only to a caller who may create", async ({ context }) => {
    const page = await context.newPage()

    // A visitor who is not signed in may read the star system and follow the search its shortcut runs, but
    // creating a planet is not theirs to do, so the "+" which leads to the create view is not offered.
    await openDocument(page, KEPHRA)
    await settle(page)
    const planets = shortcutRow(page, CLASS_IDS.PLANET, PROPERTY_IDS.CONTAINED_IN, KEPHRA)
    await expect(planets, "the planets shortcut a visitor is shown").toHaveCount(1)
    await expect(planets.locator(".pd-searchshortcutlink-link"), "the search half of the shortcut a visitor is shown").toBeVisible()
    await expect(page.locator(".pd-searchshortcutlink-button-create"), "the create half of a shortcut for a visitor who may not create").toHaveCount(0)
    const offered = await page.locator(".pd-searchshortcutlink-link").count()

    // The same document, read by someone who may create planets, offers the create half of the same
    // shortcut, so what differs is the caller and not the document.
    await signIn(page, ["surveyor"])
    await openDocument(page, KEPHRA)
    await settle(page)
    await expect(
      createShortcutButton(page, CLASS_IDS.PLANET, PROPERTY_IDS.CONTAINED_IN, KEPHRA),
      "the create half of the shortcut for a caller who may create planets",
    ).toBeVisible()

    console.log(
      `Successfully verified that all ${offered} shortcuts a visitor is shown offer searching and none of them offers creating, while a caller who may create is offered both.`,
    )
  })
})
