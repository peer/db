import type { Locator, Page } from "@playwright/test"

import type { EntityClass, Role } from "../peerdb_utils"

import { Identifier } from "@tozd/identifier"

import { CLASS_IDS, CORE_CLASS_IDS, createNamed, documentIdOf, LANGUAGES, PROPERTY_IDS, ROLE_CREATES, startCreate } from "../peerdb_utils"
import {
  checkpoint,
  checkpointElement,
  checkpointFormAt,
  expect,
  fetchFromPage,
  field,
  fieldSlots,
  fieldsPanel,
  fillHtmlField,
  fillSlot,
  goHome,
  hideDuplicates,
  LOADING_TIMEOUT,
  offeredClasses,
  PEERDB_URL,
  pickReference,
  saveEdit,
  settle,
  settleEdit,
  signIn,
  signOut,
  slotInput,
  switchLanguage,
  test,
  volatile,
} from "../utils"

// The prefix every document this file creates is named with, so that the documents of one test file
// never collide with another's and a stray document says which file made it.
const PREFIX = "E2E Create Doc"

// What each test writes into the document it creates. The strings are deliberately unlike anything in
// the test data, so a reference search or a duplicate search run by these tests never offers a document
// of the test data instead of the one which is meant.
const BIOME_NAME = `${PREFIX} Biome`
const BIOME_CODE = "e2e-create-doc-biome"
const BIOME_DESCRIPTION = "A biome written down by the create test."
const STAR_SYSTEM_NAME = `${PREFIX} Star System`
const STAR_SYSTEM_CODE = "E2E-CD-1104"
const STAR_SYSTEM_SPECTRAL_CLASS = "K2V"
const STAR_SYSTEM_STARS = "2"
const STAR_SYSTEM_DISTANCE = "12.5"
const STAR_SYSTEM_SURVEYED = "2301-04-05"
const PLANET_NAME = `${PREFIX} Planet`
const SITE_TYPE_FIRST_NAME = `${PREFIX} Site Type One`
const SITE_TYPE_SECOND_NAME = `${PREFIX} Site Type Two`

// The documents the reference fields of the created documents are pointed at. Each is named by the
// class mnemonic and the key which say which document of the test data it is, so the test picks the
// same document on every run without depending on its label, which differs between the languages.
const KEPHRA_MARCH = await documentIdOf("SECTOR", "G1_KEPHRA_MARCH")
const KEPHRA = await documentIdOf("STAR_SYSTEM", "G1_KEPHRA")
const TIDAL_OCEAN = await documentIdOf("PLANET_TYPE", "TIDAL_OCEAN")
const EYEBALL = await documentIdOf("PLANET_TYPE", "EYEBALL")

// The classes a role is offered on top of the ones the shared table lists. Creating is granted per class
// (roles in config.yml) and the curator is granted one class of the core schema, the units everything is
// measured in, besides the test data classes. The shared table carries the test data schema only, so the
// core class is named here instead.
const EXTRA_ROLE_CREATES: Partial<Record<Role, ReadonlyArray<string>>> = {
  curator: [CORE_CLASS_IDS.UNIT],
}

// The roles which may create more than one class, so the create page has a tree to offer them.
const MULTI_CLASS_ROLES: ReadonlyArray<Role> = ["surveyor", "researcher", "curator"]

// The roles which may create exactly one class, which is the case the create page answers by opening the
// editor for that class instead of offering a picker with a single button on it.
const SINGLE_CLASS_ROLES: ReadonlyArray<Role> = ["author", "ethics"]

// The class each of those roles may create, which is what the editor it lands in has to be for.
const SINGLE_CLASS_OF_ROLE: Record<string, EntityClass> = {
  author: "PUBLICATION",
  ethics: "ETHICS_PROTOCOL",
}

// The class buttons and headings the create page is expected to show for a role: the classes the role may
// create, together with whatever else the site grants it beyond the test data schema.
function expectedClasses(role: Role): Array<string> {
  const classes = (ROLE_CREATES[role] ?? []).map((entityClass: EntityClass) => CLASS_IDS[entityClass])
  return [...new Set([...classes, ...(EXTRA_ROLE_CREATES[role] ?? [])])].sort()
}

// The list item of the class tree which is headed by the given class, which is how a class a document
// cannot be created for is rendered: a heading gathering the classes below it rather than a button.
function classGroup(page: Page, classId: string): Locator {
  return page.locator(`.pd-classtreelist-item:has(> .pd-classtreelabel-title[data-url="/api/d/${classId}"])`)
}

// Reads the document the server holds under the given identifier, so that a test can assert on what was
// stored rather than only on what the view renders. The request is made from inside the page, which is
// what makes it carry the session of the view next to it.
async function storedDocument(page: Page, id: string): Promise<{ id: string; base: Array<string> }> {
  const response = await fetchFromPage(page, `/api/d/${id}`)
  expect(response.status, `the document ${id} is served`).toBe(200)
  return JSON.parse(response.body) as { id: string; base: Array<string> }
}

test.describe("PeerDB Document Create Flows", () => {
  test("Test the class tree the create page offers", async ({ context }) => {
    const page = await context.newPage()

    await signIn(page, ["surveyor"])

    // The tree is checked in every language the site is served in: the classes are named in all three, so
    // a class whose name is missing in one of them would leave a button with nothing on it, and the shape
    // of the tree has to be the same whatever the interface language is.
    for (const language of LANGUAGES) {
      await switchLanguage(page, language)
      await page.goto(`${PEERDB_URL}/d/create`)
      await expect(page.locator(".pd-documentcreate"), `the create page in ${language}`).toBeVisible({ timeout: LOADING_TIMEOUT })
      await expect(page.locator("#documentcreate-title"), `the title of the create page in ${language}`).toBeVisible({ timeout: LOADING_TIMEOUT })
      // A class whose document has not resolved yet renders a placeholder instead of its button, so the
      // offer is read only once every class the page lists has arrived.
      await settle(page)

      // A class a document cannot be created for is a heading which gathers the classes below it, and the
      // two the surveyor is offered are nested one inside the other, which is what makes the offer a tree
      // rather than a list.
      const place = classGroup(page, CLASS_IDS.PLACE)
      await expect(place, `the place heading in ${language}`).toHaveCount(1)
      await expect(place.locator(".pd-classtreelabel-title").first(), `the place heading in ${language} is named`).not.toHaveText(/^\s*$/)
      const world = classGroup(page, CLASS_IDS.WORLD)
      await expect(world, `the world heading in ${language}`).toHaveCount(1)
      await expect(
        place.locator(`.pd-classtreelabel-title[data-url="/api/d/${CLASS_IDS.WORLD}"]`),
        `the world heading sits under the place heading in ${language}`,
      ).toHaveCount(1)

      // The two classes of a world are the leaves under that heading, and they are buttons, because a
      // document can be created for them.
      for (const entityClass of ["PLANET", "MOON"] as const) {
        await expect(
          world.locator(`> .pd-classtreelist > .pd-classtreelist-item > .pd-classtreelabel-button-${CLASS_IDS[entityClass]}`),
          `the ${entityClass} button under the world heading in ${language}`,
        ).toHaveCount(1)
      }

      // Every class the role may create is offered, and every button is named. A button with no label
      // would be a class whose name is missing in this language.
      const offered = await offeredClasses(page)
      expect(offered, `the classes the create page offers in ${language}`).toEqual(expectedClasses("surveyor"))
      for (const label of await page.locator(".pd-classtreelabel-button").allTextContents()) {
        expect(label.trim(), `the label of a class button in ${language}`).not.toBe("")
      }

      // Only the world heading and what it gathers is screenshotted, and not the whole tree. The tree puts
      // the classes holding the most documents first, and this suite creates documents as it runs, so a
      // screenshot of the whole tree would eventually be of the same classes in another order. Under the
      // world heading there are two classes with a wide gap between how many documents they hold, so their
      // order is not something a run can move.
      await checkpointElement(page, classGroup(page, CLASS_IDS.WORLD), `create-doc-classtree-world-${language}`)
    }

    console.log(`Successfully checked the class tree of the create page in ${LANGUAGES.length} languages, each offering ${expectedClasses("surveyor").length} classes.`)
  })

  test("Test the class tree offers only the classes the signed-in role may create", async ({ context }) => {
    test.setTimeout(300_000)

    const page = await context.newPage()

    // Creating is the action the working roles differ on, so the offer of the create page is what says the
    // site is granting creating per class rather than per user. Each role is checked on its own, because a
    // user holding two of them is offered the union of the two.
    for (const role of MULTI_CLASS_ROLES) {
      await signIn(page, [role])
      await page.goto(`${PEERDB_URL}/d/create`)
      await expect(page.locator(".pd-documentcreate"), `the create page for the ${role} role`).toBeVisible({ timeout: LOADING_TIMEOUT })
      await expect(page.locator("#documentcreate-title"), `the title of the create page for the ${role} role`).toBeVisible({ timeout: LOADING_TIMEOUT })
      await settle(page)

      // What is asserted is the set of classes and not a screenshot of the tree: the tree puts the classes
      // holding the most documents first, and this suite creates documents as it runs, so the same classes
      // come in another order once enough has been recorded.
      const offered = await offeredClasses(page)
      expect(offered, `the classes the ${role} role is offered`).toEqual(expectedClasses(role))

      // The create page is left before the session ends. A visitor who is not signed in may not ask for
      // the classes to create, so signing out while the page is open would have it ask and be refused,
      // which the browser reports as a failed request and the next checkpoint would then fail on.
      await goHome(page)
      await signOut(page)
    }

    console.log(`Successfully checked what the create page offers each of the ${MULTI_CLASS_ROLES.length} roles which may create more than one class.`)
  })

  test("Test a role which may create a single class lands in the editor without a picker", async ({ context }) => {
    test.setTimeout(300_000)

    const page = await context.newPage()

    // A picker with a single button on it is a step which asks nothing, so the create page creates that
    // class itself and replaces itself with the editor. What says the picker was skipped rather than
    // passed through quickly is that neither the picker nor the title of the create page is ever rendered.
    for (const role of SINGLE_CLASS_ROLES) {
      await signIn(page, [role])
      await page.goto(`${PEERDB_URL}/d/create`)
      await settleEdit(page)
      await hideDuplicates(page)
      await expect(page, `the create page for the ${role} role opens an editing session`).toHaveURL(/\/d\/edit\/[0-9A-Za-z]+\/[0-9A-Za-z]+/)
      await expect(page.locator(".pd-classtreelist"), `the class picker for the ${role} role`).toHaveCount(0)
      await expect(page.locator("#documentcreate-title"), `the title of the create page for the ${role} role`).toHaveCount(0)

      // The session which was opened has to be for the one class the role may create, which is read off the
      // claims of the document being edited rather than off the form, so it says what was created and not
      // only what is being shown.
      const entityClass = SINGLE_CLASS_OF_ROLE[role]
      const allProperties = page.locator(".pd-documentedit-tab-allproperties")
      await expect(allProperties, `the claims tab of the session of the ${role} role`).toBeVisible()
      await allProperties.click()
      await expect(
        page.locator(
          `.pd-documentedit-panel-allproperties .pd-propertiesview-row-${PROPERTY_IDS.INSTANCE_OF} .pd-claimvalueref[data-url="/api/d/${CLASS_IDS[entityClass]}"]`,
        ),
        `the session of the ${role} role is for a ${entityClass}`,
      ).toHaveCount(1)
      await checkpoint(page, `create-doc-single-class-${role}`, { mask: volatile(page) })

      // The session is left rather than saved or discarded: nothing was entered, so there is nothing to
      // keep, and discarding a create session goes back to the create page, which for these roles would
      // only open another session.
      await goHome(page)
      await signOut(page)
    }

    console.log(`Successfully verified that each of the ${SINGLE_CLASS_ROLES.length} roles which may create a single class lands straight in the editor.`)
  })

  test("Test the class button which was pressed is the one which shows it is working", async ({ context }) => {
    const page = await context.newPage()

    await signIn(page, ["surveyor"])

    // Creating locks every class button at once, so without something on the button which was pressed the
    // user would not be told which class is being created. The creation is held up on purpose, because
    // otherwise it is over before what it looks like can be read, and it is held up for longer than the
    // screenshot below takes, so that the page is still in that state while it is captured.
    await page.route("**/api/d/create*", async (route) => {
      await new Promise((resolve) => setTimeout(resolve, 20000))
      await route.continue()
    })

    await page.goto(`${PEERDB_URL}/d/create`)
    const pressed = page.locator(`.pd-classtreelabel-button-${CLASS_IDS.REGION}`)
    await expect(pressed, "the class button to press").toBeVisible({ timeout: LOADING_TIMEOUT })
    await pressed.click()

    const offered = await page.locator(".pd-classtreelabel-button").count()
    await expect(pressed.locator(".pd-button-loading"), "the button which was pressed shows it is working").toBeVisible()
    await expect(page.locator(".pd-classtreelist .pd-button-loading"), "how many class buttons show they are working").toHaveCount(1)
    // The button is screenshotted rather than the page it sits on, because the tree around it puts the
    // classes holding the most documents first and this suite creates documents as it runs.
    await checkpointElement(page, pressed, "create-doc-classtree-pressed")

    await settleEdit(page)
    await page.unroute("**/api/d/create*")

    // The session is left rather than saved: this test is about the button, and a document saved here
    // would be a document with nothing in it.
    await goHome(page)

    console.log(`Successfully verified that 1 of the ${offered} class buttons of the create page shows that a document is being created, the one which was pressed.`)
  })

  test("Test creating a vocabulary entry with a name, a description and a code", async ({ context }) => {
    test.setTimeout(300_000)

    const page = await context.newPage()

    await signIn(page, ["curator"])
    await startCreate(page, "BIOME")
    await hideDuplicates(page)

    // A vocabulary entry is the shortest class in the catalogue: a name which is required, and a
    // description and a code which are not.
    const nameField = field(page, PROPERTY_IDS.NAME)
    await expect(nameField.locator(".pd-inputbadges-badge-required"), "the name of a vocabulary entry is required").toBeVisible()
    await expect(field(page, PROPERTY_IDS.DESCRIPTION), "the description field").toBeVisible()
    await expect(field(page, PROPERTY_IDS.CODE), "the code field").toBeVisible()
    await expect(fieldSlots(page, PROPERTY_IDS.NAME), "the empty name field offers a single slot").toHaveCount(1)
    await checkpoint(page, "create-doc-biome-form-empty")

    // Committing the first value of a repeated field grows it by a trailing empty slot, which is how a
    // repeated field is added to.
    await fillSlot(page, PROPERTY_IDS.NAME, 0, ".pd-inputstring", BIOME_NAME, 2, "the name of the new biome")
    await fillSlot(page, PROPERTY_IDS.CODE, 0, ".pd-inputidentifier", BIOME_CODE, 2, "the code of the new biome")
    await fillHtmlField(page, PROPERTY_IDS.DESCRIPTION, BIOME_DESCRIPTION, "the description of the new biome")
    await checkpoint(page, "create-doc-biome-form-filled")

    const id = await saveEdit(page)
    await checkpoint(page, "create-doc-biome-created", { mask: volatile(page) })

    await expect(page.locator("#documentget-title"), "the title of the created biome").toHaveText(BIOME_NAME)
    await expect(fieldsPanel(page), "the name which was entered").toContainText(BIOME_NAME)
    await expect(fieldsPanel(page), "the code which was entered").toContainText(BIOME_CODE)
    await expect(fieldsPanel(page), "the description which was entered").toContainText(BIOME_DESCRIPTION)

    console.log(`Successfully created a vocabulary entry with 3 fields filled in, saved as ${id}.`)
  })

  test("Test creating a star system with a reference, an identifier, amounts and a time", async ({ context }) => {
    test.setTimeout(600_000)

    const page = await context.newPage()

    await signIn(page, ["surveyor"])
    await startCreate(page, "STAR_SYSTEM")
    await hideDuplicates(page)

    // A star system is the class which carries one of each of the value shapes the catalogue is built
    // from, so creating one drives them all: a required name and a required reference, an identifier, two
    // amounts and a time.
    await expect(field(page, PROPERTY_IDS.NAME).locator(".pd-inputbadges-badge-required"), "the name of a star system is required").toBeVisible()
    await expect(field(page, PROPERTY_IDS.CONTAINED_IN).locator(".pd-inputbadges-badge-required"), "the sector a star system is in is required").toBeVisible()
    await checkpoint(page, "create-doc-starsystem-form-empty")

    await fillSlot(page, PROPERTY_IDS.NAME, 0, ".pd-inputstring", STAR_SYSTEM_NAME, 2, "the name of the new star system")

    // The sector is picked out of a search rather than off a list: there are more sectors than the form
    // shows as a list of options, so the field is a search box.
    await pickReference(page, field(page, PROPERTY_IDS.CONTAINED_IN), "Kephra March", KEPHRA_MARCH, "the sector of the new star system")
    await expect(
      field(page, PROPERTY_IDS.CONTAINED_IN).locator(`.pd-inputref-link-document[href$="/d/${KEPHRA_MARCH}"]`),
      "the sector field holds the sector which was picked",
    ).toBeVisible()

    await fillSlot(page, PROPERTY_IDS.CATALOGUE_CODE, 0, ".pd-inputidentifier", STAR_SYSTEM_CODE, 1, "the catalogue code of the new star system")
    await fillSlot(page, PROPERTY_IDS.SPECTRAL_CLASS, 0, ".pd-inputstring", STAR_SYSTEM_SPECTRAL_CLASS, 1, "the spectral class of the new star system")

    // An amount carries the precision it was measured to, which follows the shape of what was typed, so a
    // whole number is precise to one and a number with a decimal to a tenth.
    const stars = slotInput(page, PROPERTY_IDS.STAR_COUNT, 0, ".pd-inputamount-input-amount")
    await expect(stars, "the star count input").toBeVisible()
    await stars.fill(STAR_SYSTEM_STARS)
    await stars.blur()
    await expect(slotInput(page, PROPERTY_IDS.STAR_COUNT, 0, ".pd-inputamount-input-precision"), "the precision of a whole star count").toHaveValue("1")

    const distance = slotInput(page, PROPERTY_IDS.DISTANCE_FROM_SOL, 0, ".pd-inputamount-input-amount")
    await expect(distance, "the distance input").toBeVisible()
    await distance.fill(STAR_SYSTEM_DISTANCE)
    await distance.blur()
    await expect(slotInput(page, PROPERTY_IDS.DISTANCE_FROM_SOL, 0, ".pd-inputamount-input-precision"), "the precision of a distance with a decimal").toHaveValue("0.1")
    await settleEdit(page)
    await checkpoint(page, "create-doc-starsystem-amounts")

    // A time carries a precision too, which starts out at what was typed and can then be chosen by hand
    // from the list next to the input.
    const surveyed = slotInput(page, PROPERTY_IDS.FIRST_SURVEYED, 0, ".pd-inputtime-input-time")
    await expect(surveyed, "the first surveyed input").toBeVisible()
    await surveyed.fill(STAR_SYSTEM_SURVEYED)
    await surveyed.blur()
    await settleEdit(page)
    const precision = field(page, PROPERTY_IDS.FIRST_SURVEYED).locator(".pd-inputtime-precision")
    await expect(precision, "the precision of a full date").toHaveText("days")

    const precisionButton = field(page, PROPERTY_IDS.FIRST_SURVEYED).locator(".pd-inputtime-select-precision")
    await precisionButton.click()
    await expect(field(page, PROPERTY_IDS.FIRST_SURVEYED).locator(".pd-inputtime-list-precision"), "the list of precisions").toBeVisible()
    await checkpoint(page, "create-doc-starsystem-precision-open")
    const years = field(page, PROPERTY_IDS.FIRST_SURVEYED).locator(".pd-inputtime-item-precision-y")
    await expect(years, "the year precision option").toBeVisible()
    await years.click()
    await expect(precision, "the precision which was chosen by hand").toHaveText("years")
    await settleEdit(page)
    await checkpoint(page, "create-doc-starsystem-form-filled")

    const id = await saveEdit(page)
    await checkpoint(page, "create-doc-starsystem-created", { mask: volatile(page) })

    await expect(page.locator("#documentget-title"), "the title of the created star system").toContainText(STAR_SYSTEM_NAME)
    await expect(fieldsPanel(page).locator(`a[href="/d/${KEPHRA_MARCH}"]`), "the created star system is in the sector which was picked").toBeVisible()
    await expect(fieldsPanel(page), "the catalogue code which was entered").toContainText(STAR_SYSTEM_CODE)
    await expect(fieldsPanel(page), "the spectral class which was entered").toContainText(STAR_SYSTEM_SPECTRAL_CLASS)
    await expect(fieldsPanel(page), "the distance which was entered").toContainText(STAR_SYSTEM_DISTANCE)
    await expect(fieldsPanel(page), "the year the star system was first surveyed").toContainText("2301")

    console.log(`Successfully created a star system with a reference, an identifier, 2 amounts and a time, saved as ${id}.`)
  })

  test("Test creating a planet with sections, a chosen option and two absences", async ({ context }) => {
    test.setTimeout(600_000)

    const page = await context.newPage()

    await signIn(page, ["surveyor"])
    await startCreate(page, "PLANET")
    await hideDuplicates(page)

    // A world is the one class whose fields are grouped into sections, so its form is four blocks with a
    // heading each rather than one list.
    for (const section of ["identification", "physical", "environment", "survey"]) {
      await expect(page.locator(`#section-${section}`), `the ${section} section of the planet form`).toBeVisible()
    }
    // A form long enough to be read in sections is also long enough for a table of contents, which is the
    // one thing on the page which is not part of the form itself.
    await expect(page.locator(".pd-tableofcontents-item"), "the table of contents of the planet form").toHaveCount(4)
    await checkpointFormAt(page, page.locator("#section-identification"), "create-doc-planet-form-empty")

    await fillSlot(page, PROPERTY_IDS.NAME, 0, ".pd-inputstring", PLANET_NAME, 2, "the name of the new planet")
    await pickReference(page, field(page, PROPERTY_IDS.CONTAINED_IN), "Kephra", KEPHRA, "the star system of the new planet")

    // A reference field which holds at most one value and whose candidates all fit into one list is
    // rendered as a list of options instead of a search box, one row per candidate.
    const planetType = field(page, PROPERTY_IDS.HAS_PLANET_TYPE)
    await expect(planetType.locator(".pd-claimrefselect-list"), "the list of world types").toBeVisible({ timeout: LOADING_TIMEOUT })
    await expect(planetType.locator(".pd-claimrefselect-checkbox"), "a field which holds at most one value offers no checkbox").toHaveCount(0)
    const tidalOcean = planetType.locator(`.pd-claimrefselect-item-${TIDAL_OCEAN} .pd-claimrefselect-radio`)
    await expect(tidalOcean, "the option of the world type which is chosen").toBeVisible()
    await tidalOcean.click()
    await expect(tidalOcean, "the world type which was chosen").toBeChecked()
    await expect(planetType.locator(`.pd-claimrefselect-item-${EYEBALL} .pd-claimrefselect-radio`), "the world type which was not chosen").not.toBeChecked()
    await settleEdit(page)
    await checkpointFormAt(page, planetType, "create-doc-planet-type")

    // A field which only says that something is so renders a single checkbox and holds no value at all.
    const ringSystem = field(page, PROPERTY_IDS.HAS_RING_SYSTEM).locator(".pd-claiminput-checkbox").first()
    await expect(ringSystem, "the ring system checkbox").toBeVisible()
    await ringSystem.click()
    await expect(ringSystem, "the planet is marked as having a ring system").toBeChecked()

    // The biosphere is the field the catalogue states three ways: a description of what lives there, the
    // statement that nothing does, and the statement that nobody knows. The three are separate blocks on
    // the same property, and the last of them is the one which says it is not known.
    const biosphere = field(page, PROPERTY_IDS.BIOSPHERE)
    await expect(biosphere, "the biosphere is offered three ways").toHaveCount(3)
    const biosphereUnknown = biosphere.nth(2).locator(".pd-claiminput-checkbox").first()
    await expect(biosphereUnknown, "the checkbox saying the biosphere is not known").toBeVisible()
    await biosphereUnknown.click()
    await expect(biosphereUnknown, "the biosphere is marked as not known").toBeChecked()
    await settleEdit(page)
    await checkpointFormAt(page, biosphere.first(), "create-doc-planet-form-filled")

    const id = await saveEdit(page)
    await checkpoint(page, "create-doc-planet-created", { mask: volatile(page) })

    await expect(page.locator("#documentget-title"), "the title of the created planet").toContainText(PLANET_NAME)
    await expect(fieldsPanel(page).locator(`a[href="/d/${KEPHRA}"]`), "the created planet is in the star system which was picked").toBeVisible()
    await expect(fieldsPanel(page).locator(`a[href="/d/${TIDAL_OCEAN}"]`), "the created planet carries the world type which was chosen").toBeVisible()
    await expect(fieldsPanel(page).locator(".pd-claimvalueunknown"), "the created planet states that its biosphere is not known").toHaveCount(1)
    // The document view groups the fields into the same sections the form did, except that a section with
    // nothing in it is left out: nothing was entered in the survey section, so three of the four are shown.
    const shown = ["identification", "physical", "environment"]
    for (const section of shown) {
      await expect(page.locator(`.pd-fieldsview-header-section-${section}`), `the ${section} section of the created planet`).toBeVisible()
    }
    await expect(page.locator(".pd-fieldsview-header-section-survey"), "the section of the created planet which holds nothing").toHaveCount(0)

    console.log(
      `Successfully created a planet with a chosen option, a stated property and an unknown value, shown in ${shown.length} of the 4 sections its class declares, saved as ${id}.`,
    )
  })

  test("Test the identifier of a created document is derived from its base", async ({ context }) => {
    test.setTimeout(300_000)

    const page = await context.newPage()

    await signIn(page, ["curator"])

    // Two documents are created rather than one, so that the identifiers can be shown to differ. Nothing
    // but the name is filled in, because what is asserted is about the identifier and not about the
    // fields.
    const first = await createNamed(page, "SITE_TYPE", SITE_TYPE_FIRST_NAME)
    const second = await createNamed(page, "SITE_TYPE", SITE_TYPE_SECOND_NAME)
    expect(first, "two documents created one after the other are two documents").not.toBe(second)

    for (const [id, name] of [
      [first, SITE_TYPE_FIRST_NAME],
      [second, SITE_TYPE_SECOND_NAME],
    ] as const) {
      const stored = await storedDocument(page, id)
      expect(stored.id, `the document served under ${name} is the one which was asked for`).toBe(id)

      // The identifier is not a number handed out by the store: it is what the document's base hashes to,
      // so a document with the same base anywhere is the same document. What the base is made of is the
      // site the document belongs to, the kind of thing it is, and what tells it apart from its siblings.
      expect(stored.base.length, `the base of ${name}`).toBe(3)
      expect(stored.base[0], `the site the base of ${name} starts with`).toBe(new URL(PEERDB_URL).hostname)
      expect(stored.base[1], `the kind of thing the base of ${name} names`).toBe("DOCUMENT")
      expect(stored.base[2], `the part of the base of ${name} which tells it apart`).not.toBe("")
      expect((await Identifier.from(...stored.base)).toString(), `the identifier of ${name} is what its base hashes to`).toBe(id)
      // The identifier is derived from the whole base and not simply the last part of it repeated.
      expect(id, `the identifier of ${name} is not the last part of its base`).not.toBe(stored.base[2])
    }

    console.log(`Successfully verified that the identifiers of 2 created documents are what their bases hash to (${first}, ${second}).`)
  })
})
