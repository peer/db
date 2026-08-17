import type { Locator, Page } from "@playwright/test"

import type { EntityClass } from "../peerdb_utils"

import { documentIdOf, PROPERTY_IDS, roleWhichCreates, startCreate } from "../peerdb_utils"
import {
  changePosted,
  checkpoint,
  checkpointElement,
  expect,
  expectNothingPending,
  field,
  fieldErrors,
  fieldInput,
  fieldSlots,
  hideDuplicates,
  LOADING_TIMEOUT,
  pickReference,
  pressSave,
  saveEdit,
  settleEdit,
  signIn,
  slotValue,
  storedDocument,
  subField,
  test,
  volatileSelect,
} from "../utils"

// The first two tests work on an artifact they create themselves, because that class carries one field
// of each kind the duplicate check treats differently:
//
//   - the endonym, a repeated field of strings which internal/xeno/fields.go tags duplicate:"top". Such
//     a field compares the values alone, so what hangs off a value does not tell two of them apart.
//   - the dimension, a repeated field of amounts whose entries carry sub-fields of their own (the axis
//     the measurement was taken along, and the unit) and which carries no duplicate tag, so the form
//     compares whole claims, sub-claims included.
//
// What the two together say is that the tag is what decides how the claims are compared: the same
// endonym twice is refused however it is glossed, while the same measurement twice is saved as soon as
// the two are along different axes.
const ARTIFACT_CLASS: EntityClass = "ARTIFACT"

// The last two tests work on an expedition, which carries the two shapes a reference field takes: a
// combobox when there are more candidates than the form will list (the team members, drawn from every
// researcher), and a list of options when they all fit (the organisers, drawn from the institutes, and
// the ethics protocol). Only the first shape can be told to hold a value it already holds, which is why
// the duplicate check never runs on the second one.
const EXPEDITION_CLASS: EntityClass = "EXPEDITION"

// The names of the documents these tests create all begin with this, so that a document made here is
// told apart from the ones another test file makes, and a leftover says which file made it.
const NAME_PREFIX = "PeerDB Edit Duplicates"

// The values the tests type. Everything in the catalogue is invented, and so are these.
const ENDONYM = "kel-marun"
const OTHER_ENDONYM = "kel-marun-vel"
const GLOSS = "the salt which is carried"
const MEASUREMENT = "5"
const AXIS = "length"
const OTHER_AXIS = "width"

// What the form says about a value the field already holds. It is shown on every slot which says the
// same thing as another one, so both of a repeated pair carry it: which of them the editor meant to keep
// is not for the form to guess.
const DUPLICATE_MESSAGE = "Duplicate value."

// The documents the reference fields are pointed at, named by what they are in the test data rather than
// by the opaque identifier the two hash to.
const BONETTI_ID = await documentIdOf("RESEARCHER", "RES_BONETTI")
const ARAI_ID = await documentIdOf("RESEARCHER", "RES_ARAI")
const EVENING_ID = await documentIdOf("INSTITUTE", "INST_EVENING")
const CANOPY_ID = await documentIdOf("INSTITUTE", "INST_CANOPY")
const BELLWETHER_ID = await documentIdOf("PLANET", "G1_BELLWETHER")

// The queries which find those documents in a reference input. A researcher is labelled by both names
// and a planet by its own name and the system it is in, so a family name and a planet name are enough.
const BONETTI_QUERY = "Bonetti"
const ARAI_QUERY = "Arai"
const BELLWETHER_QUERY = "Bellwether"

const ARTIFACT_ROLE = roleWhichCreates(ARTIFACT_CLASS)
const EXPEDITION_ROLE = roleWhichCreates(EXPEDITION_CLASS)

// Fills one slot's own value and waits until the form has settled on the number of slots the field is
// expected to show afterwards, and until the change has reached the session. Filling the trailing empty
// slot of a repeated field grows it by a fresh empty one, while overwriting a slot which already holds a
// value leaves the count alone.
//
// The post is what the save below acts on: the duplicate check compares the claims the session holds, so a
// value which has been typed but not posted is not yet a claim and is not a duplicate of anything. A save
// which goes ahead of the post is therefore a save of a document with nothing repeated in it, and it goes
// through instead of being refused.
async function fillSlotValue(page: Page, propertyId: string, slot: number, input: string, value: string, slots: number, what: string): Promise<void> {
  const filled = slotValue(page, propertyId, slot, input)
  await expect(filled, what).toBeVisible({ timeout: LOADING_TIMEOUT })
  const posted = changePosted(page)
  await filled.fill(value)
  await filled.blur()
  await expect(filled, `${what} after typing`).toHaveValue(value)
  await expect(fieldSlots(page, propertyId), `slots of ${what} after typing`).toHaveCount(slots, { timeout: LOADING_TIMEOUT })
  await posted
  await expectNothingPending(page)
}

// Fills a text-like input and commits the slot by taking the focus out of it. A slot posts its change
// when the focus leaves it, so without the blur the value would still be uncommitted, and the post is
// waited for the same way and for the same reason as in fillSlotValue.
async function fillAndCommit(page: Page, input: Locator, value: string, what: string): Promise<void> {
  await expect(input, what).toBeVisible({ timeout: LOADING_TIMEOUT })
  const posted = changePosted(page)
  await input.fill(value)
  await input.blur()
  await expect(input, `${what} after typing`).toHaveValue(value)
  await posted
  await settleEdit(page)
}

// Asserts that the save was refused and left the editing session open, so the repeated value is reported
// on the form rather than stored.
//
// What the refusal is waited for on is the complaint it puts on the form. A save which is going through
// leaves the form standing until it has written the document, so asserting the form alone would pass
// while the save is still in flight and fail several assertions later, on a page which has meanwhile
// moved to the document view. Every refusal these tests provoke is a repeated value, which is reported
// on the slots holding it.
async function expectSaveRefused(page: Page): Promise<void> {
  await expect(page.locator(".pd-inputfield-error").first(), "the complaint a refused save puts on the form").toBeVisible({ timeout: LOADING_TIMEOUT })
  await expect(page.locator(".pd-documentedit"), "the editing session stays open").toBeVisible()
  await expect(page.locator(".pd-fieldsform"), "the form of the editing session stays open").toBeVisible()
  await expect(page.locator("#documentedit-error-session"), "the whole-form error of a refused save").toHaveCount(0)
  await expect(page.locator(".pd-documentget"), "the document view a save which went through would lead to").toHaveCount(0)
}

// Starts creating a document of the given class with its name filled in, which is what every class of
// the catalogue needs before it can be saved at all. The panel of potential duplicates is hidden for as
// long as the form is open: it searches the index for documents which resemble the one being created, so
// from the second run on it lists what an earlier run saved, which would move everything below it in a
// screenshot.
async function startNamed(page: Page, entityClass: EntityClass, name: string): Promise<void> {
  await startCreate(page, entityClass)
  await hideDuplicates(page)

  const nameInput = fieldInput(page, PROPERTY_IDS.NAME, ".pd-inputstring")
  await expect(nameInput, `name input of the new ${entityClass}`).toBeVisible({ timeout: LOADING_TIMEOUT })
  const posted = changePosted(page)
  await nameInput.fill(name)
  await nameInput.blur()
  await expect(nameInput, `name input of the new ${entityClass} holds the entered name`).toHaveValue(name)
  // The name has to reach the session like every other value, or the save which ends these tests writes a
  // document without one, which the class requires.
  await posted
  await expectNothingPending(page)
}

test.describe("PeerDB Edit Duplicate Value Flows", () => {
  test("Test a save refused because a field comparing top-level values holds one twice", async ({ context }) => {
    // The document is saved twice against a repeated value and once for real, which is more than a test
    // of the default length has time for.
    test.slow()

    const page = await context.newPage()

    await signIn(page, [ARTIFACT_ROLE])
    await startNamed(page, ARTIFACT_CLASS, `${NAME_PREFIX} Endonym Artifact`)

    const endonymField = field(page, PROPERTY_IDS.ENDONYM)
    await expect(fieldSlots(page, PROPERTY_IDS.ENDONYM), "the empty endonym field offers a single slot").toHaveCount(1)
    await fillSlotValue(page, PROPERTY_IDS.ENDONYM, 0, ".pd-inputstring", ENDONYM, 2, "the first endonym")
    await fillSlotValue(page, PROPERTY_IDS.ENDONYM, 1, ".pd-inputstring", ENDONYM, 3, "the second endonym")

    // Nothing is said about the repetition while it is being typed: the complaint belongs to the save,
    // because the next keystroke may be what makes the two values differ again.
    await expect(fieldErrors(page, PROPERTY_IDS.ENDONYM), "complaints before the save").toHaveCount(0)

    await pressSave(page)
    await expectSaveRefused(page)

    const errors = fieldErrors(page, PROPERTY_IDS.ENDONYM)
    await expect(errors, "the slots complained about").toHaveCount(2, { timeout: LOADING_TIMEOUT })
    await expect(errors.first(), "what the form says about the repeated value").toHaveText(DUPLICATE_MESSAGE)
    await expect(slotValue(page, PROPERTY_IDS.ENDONYM, 0, ".pd-inputstring"), "the first of the repeated slots is marked as invalid").toHaveAttribute(
      "aria-invalid",
      "true",
    )
    await expect(slotValue(page, PROPERTY_IDS.ENDONYM, 1, ".pd-inputstring"), "the second of the repeated slots is marked as invalid").toHaveAttribute(
      "aria-invalid",
      "true",
    )
    await checkpointElement(page, endonymField, "edit-duplicates-endonym-refused", { mask: volatileSelect(page) })
    await checkpoint(page, "edit-duplicates-endonym-form", { mask: volatileSelect(page) })

    // This is what tells a field comparing top-level values from one comparing whole claims: a gloss on
    // the second endonym makes the two claims differ in what hangs off them, and the field goes on
    // refusing them, because it compares nothing but the values. The complaint does clear while the
    // gloss is being typed, since the check runs on the save.
    const gloss = subField(fieldSlots(page, PROPERTY_IDS.ENDONYM).nth(1), PROPERTY_IDS.GLOSS).locator(".pd-inputstring").first()
    await fillAndCommit(page, gloss, GLOSS, "the gloss of the second endonym")
    await expect(fieldErrors(page, PROPERTY_IDS.ENDONYM), "complaints while the gloss is being typed").toHaveCount(0)
    await checkpointElement(page, endonymField, "edit-duplicates-endonym-glossed", { mask: volatileSelect(page) })

    await pressSave(page)
    await expectSaveRefused(page)
    await expect(fieldErrors(page, PROPERTY_IDS.ENDONYM), "the slots complained about once the claims differ but the values do not").toHaveCount(2, {
      timeout: LOADING_TIMEOUT,
    })

    // Making the values themselves differ is what the field asks for, and the save then goes through
    // with both of them, the gloss included.
    await fillSlotValue(page, PROPERTY_IDS.ENDONYM, 1, ".pd-inputstring", OTHER_ENDONYM, 3, "the second endonym after it was changed")
    await expect(fieldErrors(page, PROPERTY_IDS.ENDONYM), "complaints once the values differ").toHaveCount(0)

    const id = await saveEdit(page)
    const properties = page.locator(".pd-documentget-panel-properties")
    await expect(properties, "the first endonym of the saved artifact").toContainText(ENDONYM)
    await expect(properties, "the second endonym of the saved artifact").toContainText(OTHER_ENDONYM)
    await checkpoint(page, "edit-duplicates-endonym-saved", { mask: volatileSelect(page) })

    const saved = await storedDocument(page, id)
    expect(saved, "the endonym which was repeated is stored once").toContain(`"${ENDONYM}"`)
    expect(saved, "the endonym which was changed is stored as well").toContain(`"${OTHER_ENDONYM}"`)
    expect(saved, "the gloss which did not tell the values apart is stored on the value it belongs to").toContain(GLOSS)

    console.log(
      `Successfully had 2 saves of document ${id} refused for a value repeated in a field which compares top-level values, one of them with a gloss telling the claims apart.`,
    )
  })

  test("Test a field comparing whole claims refuses the same value twice and allows it with different sub-values", async ({ context }) => {
    test.slow()

    const page = await context.newPage()

    await signIn(page, [ARTIFACT_ROLE])
    await startNamed(page, ARTIFACT_CLASS, `${NAME_PREFIX} Dimension Artifact`)

    // A measurement is only meaningful along an axis, so the axis is a sub-field of the measurement, and
    // it is required as soon as the slot it belongs to holds anything at all.
    const dimensionField = field(page, PROPERTY_IDS.DIMENSION)
    await expect(fieldSlots(page, PROPERTY_IDS.DIMENSION), "the empty dimension field offers a single slot").toHaveCount(1)
    await fillSlotValue(page, PROPERTY_IDS.DIMENSION, 0, ".pd-inputamount-input-amount", MEASUREMENT, 2, "the first measurement")
    const firstAxis = subField(fieldSlots(page, PROPERTY_IDS.DIMENSION).nth(0), PROPERTY_IDS.AXIS).locator(".pd-inputstring").first()
    await fillAndCommit(page, firstAxis, AXIS, "the axis of the first measurement")

    await fillSlotValue(page, PROPERTY_IDS.DIMENSION, 1, ".pd-inputamount-input-amount", MEASUREMENT, 3, "the second measurement")
    const secondAxis = subField(fieldSlots(page, PROPERTY_IDS.DIMENSION).nth(1), PROPERTY_IDS.AXIS).locator(".pd-inputstring").first()
    await fillAndCommit(page, secondAxis, AXIS, "the axis of the second measurement")
    await expect(fieldErrors(page, PROPERTY_IDS.DIMENSION), "complaints before the save").toHaveCount(0)

    // Twice the same measurement along the same axis is the same claim however the claims are compared,
    // so this half of the test would hold for either kind of field.
    await pressSave(page)
    await expectSaveRefused(page)
    const errors = fieldErrors(page, PROPERTY_IDS.DIMENSION)
    await expect(errors, "the slots complained about").toHaveCount(2, { timeout: LOADING_TIMEOUT })
    await expect(errors.first(), "what the form says about the repeated measurement").toHaveText(DUPLICATE_MESSAGE)
    await checkpointElement(page, dimensionField, "edit-duplicates-dimension-refused", { mask: volatileSelect(page) })

    // This is the half which tells the two kinds of field apart: another axis on the second measurement
    // leaves both claims saying the same number while the claims themselves differ, and a field
    // comparing whole claims accepts that. The endonym field above refuses exactly this.
    await fillAndCommit(page, secondAxis, OTHER_AXIS, "the axis of the second measurement after it was changed")
    await expect(fieldErrors(page, PROPERTY_IDS.DIMENSION), "complaints once the claims differ").toHaveCount(0)
    await checkpointElement(page, dimensionField, "edit-duplicates-dimension-distinguished", { mask: volatileSelect(page) })

    const id = await saveEdit(page)
    await checkpoint(page, "edit-duplicates-dimension-saved", { mask: volatileSelect(page) })

    // Both measurements reach the saved document, which is what says they were not treated as one claim.
    const saved = JSON.parse(await storedDocument(page, id)) as { claims: { amount?: Array<Record<string, unknown>> } }
    const measurements = (saved.claims.amount ?? []).filter((claim) => (claim.prop as { id: string } | undefined)?.id === PROPERTY_IDS.DIMENSION)
    expect(measurements.length, "the measurements of the saved artifact").toBe(2)
    expect(
      measurements.map((claim) => claim.amount),
      "both measurements are stored with the number which was repeated",
    ).toEqual([MEASUREMENT, MEASUREMENT])
    const axes = measurements.map((claim) => (claim.sub as { string: Array<{ string: string }> }).string[0].string)
    expect([...axes].sort(), "the axes which tell the two measurements apart").toEqual([AXIS, OTHER_AXIS].sort())

    console.log(
      `Successfully had 1 save of document ${id} refused for a repeated measurement and saved the same measurement twice once the axes told the two claims apart.`,
    )
  })

  test("Test a reference field which cannot hold a value twice offers the value it holds as already used", async ({ context }) => {
    const page = await context.newPage()

    await signIn(page, [EXPEDITION_ROLE])
    await startNamed(page, EXPEDITION_CLASS, `${NAME_PREFIX} Team Expedition`)

    // The team members are drawn from every researcher of the catalogue, which is more than the form
    // will list at once, so the field is a combobox and a document is reached by searching for it.
    const teamField = field(page, PROPERTY_IDS.HAS_TEAM_MEMBER)
    await expect(teamField.locator(".pd-inputref-input"), "the team member field is a combobox").toHaveCount(1)
    await expect(fieldSlots(page, PROPERTY_IDS.HAS_TEAM_MEMBER), "the empty team member field offers a single slot").toHaveCount(1)

    const firstSlot = fieldSlots(page, PROPERTY_IDS.HAS_TEAM_MEMBER).nth(0)
    await pickReference(page, firstSlot, BONETTI_QUERY, BONETTI_ID, "the first team member")
    await expect(fieldSlots(page, PROPERTY_IDS.HAS_TEAM_MEMBER), "the filled team member field grew a slot").toHaveCount(2, { timeout: LOADING_TIMEOUT })

    // The same search in the next slot still finds the researcher who is already on the team, because
    // hiding a result would leave the user wondering where it went. It is offered as taken instead, and
    // it cannot be picked: doing so could only be reported as a duplicate once the save ran.
    const secondSlot = fieldSlots(page, PROPERTY_IDS.HAS_TEAM_MEMBER).nth(1)
    const secondInput = secondSlot.locator(".pd-inputref-input")
    await expect(secondInput, "reference input of the second team member").toBeVisible()
    await secondInput.fill(BONETTI_QUERY)
    const taken = secondSlot.locator(`.pd-inputref-item-${BONETTI_ID}`)
    await expect(taken, "the researcher who is already on the team is still offered").toBeVisible({ timeout: LOADING_TIMEOUT })
    await expect(taken.locator(".pd-inputref-text-alreadyused"), "the researcher who is already on the team is marked as taken").toBeVisible()
    await expect(taken, "the researcher who is already on the team cannot be picked again").toHaveAttribute("aria-disabled", "true")
    await checkpointElement(page, teamField, "edit-duplicates-team-alreadyused", { mask: volatileSelect(page) })

    // Anybody else is offered as usual, so it is the value the field holds which is taken and not the
    // search which stopped working.
    await secondInput.fill(ARAI_QUERY)
    const free = secondSlot.locator(`.pd-inputref-item-${ARAI_ID}`)
    await expect(free, "a researcher who is not on the team yet").toBeVisible({ timeout: LOADING_TIMEOUT })
    await expect(free.locator(".pd-inputref-text-alreadyused"), "a researcher who is not on the team carries no taken mark").toHaveCount(0)
    await expect(free, "a researcher who is not on the team can be picked").not.toHaveAttribute("aria-disabled", "true")
    await free.click()
    await expect(secondSlot.locator(".pd-inputref-value"), "the second team member").toBeVisible({ timeout: LOADING_TIMEOUT })
    await expect(fieldErrors(page, PROPERTY_IDS.HAS_TEAM_MEMBER), "complaints about the two team members").toHaveCount(0)
    await checkpointElement(page, teamField, "edit-duplicates-team-picked", { mask: volatileSelect(page) })

    // Nothing is saved: a create session materializes no document until it is saved, so the expedition
    // this test drove leaves nothing behind.
    console.log("Successfully verified that a reference field which cannot repeat a value offers the 1 value it holds as taken and lets a different one be picked.")
  })

  test("Test a reference field offering every candidate cannot hold a value twice at all", async ({ context }) => {
    test.slow()

    const page = await context.newPage()

    await signIn(page, [EXPEDITION_ROLE])
    await startNamed(page, EXPEDITION_CLASS, `${NAME_PREFIX} Organisers Expedition`)

    // A field whose candidates all fit is offered as a list of options rather than as a combobox: a
    // repeated one as checkboxes, a single-valued one as radios. Neither can be told to hold a value
    // twice, which is why the duplicate check never has to run on them.
    const organisersField = field(page, PROPERTY_IDS.ORGANISED_BY)
    await expect(organisersField.locator(".pd-claimrefselect-list"), "the organisers are offered as a list of options").toBeVisible({ timeout: LOADING_TIMEOUT })
    await expect(organisersField.locator(".pd-inputref-input"), "the organisers are not offered as a combobox").toHaveCount(0)
    const checkboxes = organisersField.locator(".pd-claimrefselect-checkbox")
    expect(await checkboxes.count(), "the organisers are offered as checkboxes, one per institute").toBeGreaterThan(1)

    const ethicsField = field(page, PROPERTY_IDS.UNDER_ETHICS_PROTOCOL)
    await expect(ethicsField.locator(".pd-claimrefselect-list"), "the ethics protocol is offered as a list of options").toBeVisible()
    expect(await ethicsField.locator(".pd-claimrefselect-radio").count(), "a field which holds one value is offered as radios").toBeGreaterThan(1)

    // Every candidate is listed exactly once, which is what makes picking the same one twice impossible.
    await expect(organisersField.locator(`.pd-claimrefselect-item-${EVENING_ID}`), "the institute is listed once").toHaveCount(1)
    await expect(organisersField.locator(`.pd-claimrefselect-item-${CANOPY_ID}`), "the other institute is listed once").toHaveCount(1)

    const evening = organisersField.locator(`.pd-claimrefselect-item-${EVENING_ID} .pd-claimrefselect-checkbox`)
    const canopy = organisersField.locator(`.pd-claimrefselect-item-${CANOPY_ID} .pd-claimrefselect-checkbox`)
    await evening.check()
    await settleEdit(page)
    await canopy.check()
    await settleEdit(page)
    await expect(evening, "the first organiser stays picked").toBeChecked()
    await expect(canopy, "the second organiser is picked").toBeChecked()
    // The whole form is checkpointed rather than the field which was just driven, because the rows of a
    // list of options are exactly what a checkpoint has to mask, so a screenshot of that field alone
    // would be a screenshot of the mask.
    await checkpoint(page, "edit-duplicates-organisers-form", { mask: volatileSelect(page) })

    // The destination is the one field an expedition cannot be saved without, and it is a combobox
    // because it may point at any world or region of the catalogue.
    const destinationSlot = fieldSlots(page, PROPERTY_IDS.HAS_DESTINATION).nth(0)
    await pickReference(page, destinationSlot, BELLWETHER_QUERY, BELLWETHER_ID, "the destination")

    // Two options of one field are two claims and never a repeated one, so nothing is complained about
    // and the save goes through.
    await expect(page.locator(".pd-inputfield-error"), "complaints anywhere on the form").toHaveCount(0)
    const id = await saveEdit(page)
    await checkpoint(page, "edit-duplicates-organisers-saved", { mask: volatileSelect(page) })

    const saved = JSON.parse(await storedDocument(page, id)) as { claims: { ref?: Array<Record<string, unknown>> } }
    const organisers = (saved.claims.ref ?? [])
      .filter((claim) => (claim.prop as { id: string } | undefined)?.id === PROPERTY_IDS.ORGANISED_BY)
      .map((claim) => (claim.to as { id: string }).id)
    expect([...organisers].sort(), "the organisers of the saved expedition").toEqual([EVENING_ID, CANOPY_ID].sort())

    console.log(
      `Successfully verified that the 2 options picked in a field offering every candidate reached document ${id} as 2 claims, with no duplicate complained about.`,
    )
  })
})
