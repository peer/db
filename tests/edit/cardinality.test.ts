import type { Locator, Page } from "@playwright/test"

import type { EntityClass } from "../peerdb_utils"

import { PROPERTY_IDS, roleWhichCreates, startCreate } from "../peerdb_utils"
import {
  checkpoint,
  checkpointElement,
  expect,
  fetchFromPage,
  field,
  fieldErrors,
  fieldInput,
  fieldSlots,
  hideDuplicates,
  LOADING_TIMEOUT,
  pressSave,
  saveEdit,
  settleEdit,
  signIn,
  slotValue,
  startEdit,
  switchLanguage,
  test,
  volatileSelect,
} from "../utils"

// Every test here works on an artifact it creates itself, because that class carries one field of each
// shape the slot machinery has to deal with:
//
//   - the name, which is required and may be stated more than once (cardinality 1.. in
//     internal/xeno/fields.go),
//   - the material, which is optional and repeated (0..), so it is the field slots grow and shrink on,
//   - the accession code, which holds at most one value (0..1) and therefore never grows a slot,
//   - the dimension, an optional entry with a required sub-field (the axis the measurement was taken
//     along), which is what says that a required sub-field only applies to an entry somebody started.
const ARTIFACT_CLASS: EntityClass = "ARTIFACT"

// The names of the documents these tests create all begin with this, so that a document made here is
// told apart from the ones another test file makes, and a leftover says which file made it.
const NAME_PREFIX = "PeerDB Edit Cardinality"

// The values the tests type. Everything in the catalogue is invented, and so are these.
const FIRST_MATERIAL = "braided fibre"
const SECOND_MATERIAL = "salt glaze"
const THIRD_MATERIAL = "hammered plate"
const CHANGED_MATERIAL = "beaten foil"
const ACCESSION_CODE = "PEERDB-CARDINALITY-1"
const MEASUREMENT = "5"
const AXIS = "length"

// What the form says about a field which is required and holds nothing.
const REQUIRED_MESSAGE = "Required value."

const ROLE = roleWhichCreates(ARTIFACT_CLASS)

// The label cell of a field, which is where the badges saying what the field is (required, repeated,
// changed) live, and where the button reverting the whole field sits.
function fieldLabel(page: Page, propertyId: string): Locator {
  return field(page, propertyId).locator(".pd-fieldsformfield-cell-label")
}

// The button which takes a field back to what it held when the session was opened. It is always
// rendered and only shown once the field has something to revert, so a test asserts on whether it is
// visible rather than on whether it is there.
function fieldRevert(page: Page, propertyId: string): Locator {
  return fieldLabel(page, propertyId).locator(".pd-inputbadges-button-revert")
}

// The block of one sub-field inside one slot of a repeated field, which is where the values which hang
// off that slot's own value are edited.
function subField(slot: Locator, propertyId: string): Locator {
  return slot.locator(`.pd-claimcardinality-${propertyId}`)
}

// The values every slot of a repeated field currently holds, in the order the form shows them, which is
// what a test about slots growing, being compacted and being restored compares.
async function slotValues(page: Page, propertyId: string, input: string): Promise<Array<string>> {
  return await field(page, propertyId)
    .locator(`.pd-claiminput-${propertyId} > .pd-claiminput-value`)
    .locator(input)
    .evaluateAll((inputs) => inputs.map((element) => (element as HTMLInputElement).value))
}

// Fills one slot's own value and waits until the form has settled on the number of slots the field is
// expected to show afterwards. Filling the trailing empty slot of a repeated field grows it by a fresh
// empty one, while overwriting a slot which already holds a value leaves the count alone, and emptying
// one takes it away.
//
// What was typed is not read back from the input it was typed into: emptying a slot takes that input
// away with it, so the values are read from the field as a whole (see slotValues) once the field has
// settled on how many slots it shows.
async function fillSlotValue(page: Page, propertyId: string, slot: number, input: string, value: string, slots: number, what: string): Promise<void> {
  const filled = slotValue(page, propertyId, slot, input)
  await expect(filled, what).toBeVisible({ timeout: LOADING_TIMEOUT })
  await filled.fill(value)
  await filled.blur()
  await expect(fieldSlots(page, propertyId), `slots of ${what} after typing`).toHaveCount(slots, { timeout: LOADING_TIMEOUT })
}

// Asserts that the save was refused and left the editing session open, so what the form complains about
// is reported on the form rather than stored.
async function expectSaveRefused(page: Page): Promise<void> {
  await expect(page.locator(".pd-documentedit"), "the editing session stays open").toBeVisible()
  await expect(page.locator(".pd-fieldsform"), "the form of the editing session stays open").toBeVisible()
  await expect(page.locator("#documentedit-error-session"), "the whole-form error of a refused save").toHaveCount(0)
  await expect(page.locator(".pd-documentget"), "the document view a save which went through would lead to").toHaveCount(0)
}

// Starts creating an artifact. The panel of potential duplicates is hidden for as long as the form is
// open: it searches the index for documents which resemble the one being created, so from the second run
// on it lists what an earlier run saved, which would move everything below it in a screenshot.
async function startArtifact(page: Page): Promise<void> {
  await startCreate(page, ARTIFACT_CLASS)
  await hideDuplicates(page)
}

// Fills in the name of the document being created, which is what every class of the catalogue needs
// before it can be saved at all.
async function fillName(page: Page, name: string): Promise<void> {
  const nameInput = fieldInput(page, PROPERTY_IDS.NAME, ".pd-inputstring")
  await expect(nameInput, "name input of the new artifact").toBeVisible({ timeout: LOADING_TIMEOUT })
  await nameInput.fill(name)
  await nameInput.blur()
  await expect(nameInput, "name input holds the entered name").toHaveValue(name)
}

// One kind of claim of a document as the server stores it, which is how a test tells what the slots on
// the form actually wrote.
async function storedClaims(page: Page, id: string, kind: string): Promise<Array<Record<string, unknown>>> {
  const response = await fetchFromPage(page, `/api/d/${id}`)
  expect(response.status, `status of the stored document ${id}`).toBe(200)
  const document = JSON.parse(response.body) as { claims: Record<string, Array<Record<string, unknown>> | undefined> }
  return document.claims[kind] ?? []
}

// The values a document states for one string property, in the order they are stored in.
async function storedStrings(page: Page, id: string, propertyId: string): Promise<Array<string>> {
  return (await storedClaims(page, id, "string")).filter((claim) => (claim.prop as { id: string } | undefined)?.id === propertyId).map((claim) => claim.string as string)
}

// The strings stated as sub-claims of one claim, which is where the values of a sub-field are stored.
function subStrings(claim: Record<string, unknown>, propertyId: string): Array<string> {
  const sub = (claim.sub ?? {}) as { string?: Array<{ prop?: { id: string }; string: string }> }
  return (sub.string ?? []).filter((subClaim) => subClaim.prop?.id === propertyId).map((subClaim) => subClaim.string)
}

test.describe("PeerDB Edit Cardinality Flows", () => {
  test("Test a repeated field grows a trailing empty slot as soon as its last one is filled", async ({ context }) => {
    const page = await context.newPage()

    await signIn(page, [ROLE])
    await startArtifact(page)
    await fillName(page, `${NAME_PREFIX} Growing Artifact`)

    // A field which may be stated more than once says so, and starts with the single empty slot the
    // first value goes into.
    const materialField = field(page, PROPERTY_IDS.MATERIAL)
    await expect(fieldLabel(page, PROPERTY_IDS.MATERIAL).locator(".pd-inputbadges-badge-multiple"), "the material field says it may hold several values").toBeVisible()
    await expect(fieldSlots(page, PROPERTY_IDS.MATERIAL), "the empty material field offers a single slot").toHaveCount(1)
    await checkpointElement(page, materialField, "edit-cardinality-material-empty", volatileSelect(page))

    // Every filled slot is followed by an empty one, so there is always somewhere to put the next value
    // and never a button to press for it.
    await fillSlotValue(page, PROPERTY_IDS.MATERIAL, 0, ".pd-inputstring", FIRST_MATERIAL, 2, "the first material")
    expect(await slotValues(page, PROPERTY_IDS.MATERIAL, ".pd-inputstring"), "the material field after the first value").toEqual([FIRST_MATERIAL, ""])
    await fillSlotValue(page, PROPERTY_IDS.MATERIAL, 1, ".pd-inputstring", SECOND_MATERIAL, 3, "the second material")
    expect(await slotValues(page, PROPERTY_IDS.MATERIAL, ".pd-inputstring"), "the material field after the second value").toEqual([FIRST_MATERIAL, SECOND_MATERIAL, ""])

    // The slots are numbered as they are shown, so the trailing empty one is counted too.
    expect(await materialField.locator(".pd-claimcardinality-count").allTextContents(), "the numbering of the material slots").toEqual(["1.", "2.", "3."])
    await checkpointElement(page, materialField, "edit-cardinality-material-grown", volatileSelect(page))

    // A field which holds at most one value never grows: filling it leaves it with the slot it started
    // with, and it does not say it may hold several values either.
    const codeField = field(page, PROPERTY_IDS.ACCESSION_CODE)
    await expect(
      fieldLabel(page, PROPERTY_IDS.ACCESSION_CODE).locator(".pd-inputbadges-badge-multiple"),
      "the accession code says nothing about several values",
    ).toHaveCount(0)
    await expect(fieldSlots(page, PROPERTY_IDS.ACCESSION_CODE), "the empty accession code offers a single slot").toHaveCount(1)
    await fillSlotValue(page, PROPERTY_IDS.ACCESSION_CODE, 0, ".pd-inputidentifier", ACCESSION_CODE, 1, "the accession code")
    await checkpointElement(page, codeField, "edit-cardinality-code-filled", volatileSelect(page))
    await checkpoint(page, "edit-cardinality-growing-form", { mask: volatileSelect(page) })

    // The trailing empty slot is somewhere to type and nothing else: it reaches the saved document as no
    // value at all.
    const id = await saveEdit(page)
    expect(await storedStrings(page, id, PROPERTY_IDS.MATERIAL), "the materials of the saved artifact").toEqual([FIRST_MATERIAL, SECOND_MATERIAL])

    console.log(`Successfully filled 2 slots of a repeated field on document ${id}, each growing the trailing empty slot which the save then stored nothing of.`)
  })

  test("Test a required field says so and blocks the save until it holds a value", async ({ context }) => {
    const page = await context.newPage()

    const name = `${NAME_PREFIX} Required Artifact`

    await signIn(page, [ROLE])
    // What the form says about a field which holds nothing is asserted below, so the language it says it
    // in is pinned rather than left to whatever the browser asks for.
    await switchLanguage(page, "en")
    await startArtifact(page)

    // A required field says so before anything is typed, and a field which is not required says nothing.
    await expect(fieldLabel(page, PROPERTY_IDS.NAME).locator(".pd-inputbadges-badge-required"), "the name field says it is required").toBeVisible()
    await expect(fieldLabel(page, PROPERTY_IDS.MATERIAL).locator(".pd-inputbadges-badge-required"), "the material field says nothing about being required").toHaveCount(0)
    await expect(fieldErrors(page, PROPERTY_IDS.NAME), "complaint about the empty name before the save").toHaveCount(0)
    await checkpointElement(page, field(page, PROPERTY_IDS.NAME), "edit-cardinality-required-empty", volatileSelect(page))

    await pressSave(page)

    // The save is refused, the field says what is missing, and the caret goes to the input which is
    // missing it, so the user does not have to look for it.
    await expectSaveRefused(page)
    const nameInput = fieldInput(page, PROPERTY_IDS.NAME, ".pd-inputstring")
    await expect(fieldErrors(page, PROPERTY_IDS.NAME), "complaint about the empty name after the refused save").toHaveText(REQUIRED_MESSAGE)
    await expect(nameInput, "name input is marked as invalid").toHaveAttribute("aria-invalid", "true")
    await expect(nameInput, "name input takes the focus").toBeFocused()
    await checkpointElement(page, field(page, PROPERTY_IDS.NAME), "edit-cardinality-required-refused", volatileSelect(page))
    await checkpoint(page, "edit-cardinality-required-form", { mask: volatileSelect(page) })

    // Typing a value is what the field asks for, and the same save then goes through.
    await fillName(page, name)
    await expect(fieldErrors(page, PROPERTY_IDS.NAME), "complaint about the name once it holds a value").toHaveCount(0)
    await expect(nameInput, "name input is no longer marked as invalid").not.toHaveAttribute("aria-invalid", "true")

    const id = await saveEdit(page)
    expect(await storedStrings(page, id, PROPERTY_IDS.NAME), "the name of the saved artifact").toEqual([name])

    console.log(`Successfully had 1 save refused over a required field which held nothing, and saved it as document ${id} once the field held a value.`)
  })

  test("Test an emptied slot in the middle of a repeated field is compacted away", async ({ context }) => {
    const page = await context.newPage()

    await signIn(page, [ROLE])
    await startArtifact(page)
    await fillName(page, `${NAME_PREFIX} Compacting Artifact`)

    await fillSlotValue(page, PROPERTY_IDS.MATERIAL, 0, ".pd-inputstring", FIRST_MATERIAL, 2, "the first material")
    await fillSlotValue(page, PROPERTY_IDS.MATERIAL, 1, ".pd-inputstring", SECOND_MATERIAL, 3, "the second material")
    await fillSlotValue(page, PROPERTY_IDS.MATERIAL, 2, ".pd-inputstring", THIRD_MATERIAL, 4, "the third material")
    expect(await slotValues(page, PROPERTY_IDS.MATERIAL, ".pd-inputstring"), "the material field before anything is emptied").toEqual([
      FIRST_MATERIAL,
      SECOND_MATERIAL,
      THIRD_MATERIAL,
      "",
    ])

    // Emptying a slot in the middle takes the slot away rather than leaving a hole where it was: the
    // values below it move up and the trailing empty slot stays the only empty one.
    const materialField = field(page, PROPERTY_IDS.MATERIAL)
    await fillSlotValue(page, PROPERTY_IDS.MATERIAL, 1, ".pd-inputstring", "", 3, "the second material after it was emptied")
    expect(await slotValues(page, PROPERTY_IDS.MATERIAL, ".pd-inputstring"), "the material field after the middle slot was emptied").toEqual([
      FIRST_MATERIAL,
      THIRD_MATERIAL,
      "",
    ])
    expect(await materialField.locator(".pd-claimcardinality-count").allTextContents(), "the numbering of the material slots after the compaction").toEqual([
      "1.",
      "2.",
      "3.",
    ])
    await expect(fieldErrors(page, PROPERTY_IDS.MATERIAL), "complaints about the compacted field").toHaveCount(0)
    await checkpointElement(page, materialField, "edit-cardinality-material-compacted", volatileSelect(page))

    // What the compaction did to the form is what the save writes: the emptied value is gone and the two
    // which were kept are stored in the order the form shows them.
    const id = await saveEdit(page)
    expect(await storedStrings(page, id, PROPERTY_IDS.MATERIAL), "the materials of the saved artifact").toEqual([FIRST_MATERIAL, THIRD_MATERIAL])

    console.log(`Successfully emptied 1 of the 3 slots of a repeated field on document ${id} and had the remaining 2 compacted into a field with a single empty slot.`)
  })

  test("Test a required sub-field says nothing until its entry holds a value", async ({ context }) => {
    // The document is created, saved once with the entry untouched, refused once and saved once more,
    // which is more than a test of the default length has time for.
    test.slow()

    const page = await context.newPage()

    await signIn(page, [ROLE])
    await switchLanguage(page, "en")
    await startArtifact(page)
    await fillName(page, `${NAME_PREFIX} Sub-field Artifact`)

    // A measurement is only meaningful along an axis, so the axis is required. The measurement itself is
    // not, and an entry nobody started is not half-filled in: its required sub-field says nothing and
    // does not stand in the way of the save.
    const dimensionField = field(page, PROPERTY_IDS.DIMENSION)
    const untouched = fieldSlots(page, PROPERTY_IDS.DIMENSION).nth(0)
    await expect(subField(untouched, PROPERTY_IDS.AXIS), "the axis of an entry nobody started").toHaveCount(0)
    await expect(fieldErrors(page, PROPERTY_IDS.DIMENSION), "complaints about an entry nobody started").toHaveCount(0)
    await checkpointElement(page, dimensionField, "edit-cardinality-dimension-untouched", volatileSelect(page))

    const id = await saveEdit(page)
    await expect(page.locator("#documentget-title"), "the artifact saved with the entry untouched").toBeVisible()

    // Typing a measurement is what starts the entry, and the axis is required from then on.
    await startEdit(page)
    await hideDuplicates(page)
    await fillSlotValue(page, PROPERTY_IDS.DIMENSION, 0, ".pd-inputamount-input-amount", MEASUREMENT, 2, "the measurement")
    const started = fieldSlots(page, PROPERTY_IDS.DIMENSION).nth(0)
    const axis = subField(started, PROPERTY_IDS.AXIS)
    await expect(axis, "the axis of the entry which was started").toBeVisible({ timeout: LOADING_TIMEOUT })
    await expect(axis.locator(".pd-inputbadges-badge-required"), "the axis says it is required once the entry holds a measurement").toBeVisible()
    await checkpointElement(page, field(page, PROPERTY_IDS.DIMENSION), "edit-cardinality-dimension-started", volatileSelect(page))

    await pressSave(page)
    await expectSaveRefused(page)
    const axisInput = axis.locator(".pd-inputstring").first()
    await expect(fieldErrors(page, PROPERTY_IDS.DIMENSION), "complaint about the axis which was left empty").toHaveText(REQUIRED_MESSAGE)
    await expect(axisInput, "the axis input takes the focus").toBeFocused()
    await checkpointElement(page, field(page, PROPERTY_IDS.DIMENSION), "edit-cardinality-dimension-refused", volatileSelect(page))

    await axisInput.fill(AXIS)
    await axisInput.blur()
    await settleEdit(page)
    await expect(fieldErrors(page, PROPERTY_IDS.DIMENSION), "complaints once the axis holds a value").toHaveCount(0)

    await saveEdit(page)
    // The measurement is stored as one claim and the axis as a claim hanging off it, which is what makes
    // the axis a sub-field of the entry rather than a field of the document.
    const measurements = (await storedClaims(page, id, "amount")).filter((claim) => (claim.prop as { id: string } | undefined)?.id === PROPERTY_IDS.DIMENSION)
    expect(measurements.length, "the measurements of the saved artifact").toBe(1)
    expect(measurements[0].amount, "the measurement stored for the entry which was started").toBe(MEASUREMENT)
    expect(subStrings(measurements[0], PROPERTY_IDS.AXIS), "the axis stored under the measurement").toEqual([AXIS])
    await checkpoint(page, "edit-cardinality-dimension-saved", { mask: volatileSelect(page) })

    console.log(`Successfully saved document ${id} with an untouched entry whose sub-field is required, and had 1 save refused once the entry was started.`)
  })

  test("Test the revert button takes a field back to what it held on the first click", async ({ context }) => {
    // The document is created and then driven through three reverts and a save, which is more than a
    // test of the default length has time for.
    test.slow()

    const page = await context.newPage()

    await signIn(page, [ROLE])
    await startArtifact(page)
    await fillName(page, `${NAME_PREFIX} Revert Artifact`)
    await fillSlotValue(page, PROPERTY_IDS.MATERIAL, 0, ".pd-inputstring", FIRST_MATERIAL, 2, "the first material")
    await fillSlotValue(page, PROPERTY_IDS.MATERIAL, 1, ".pd-inputstring", SECOND_MATERIAL, 3, "the second material")
    const id = await saveEdit(page)

    // What the field holds when the session is opened is what a revert goes back to, so the reverting is
    // driven in a session on the saved document rather than in the one which created it.
    await startEdit(page)
    await hideDuplicates(page)
    const materialField = field(page, PROPERTY_IDS.MATERIAL)
    const revert = fieldRevert(page, PROPERTY_IDS.MATERIAL)
    await expect(revert, "the revert button of a field nobody has touched").toBeHidden()
    expect(await slotValues(page, PROPERTY_IDS.MATERIAL, ".pd-inputstring"), "the material field as the session opens it").toEqual([FIRST_MATERIAL, SECOND_MATERIAL, ""])

    // A field which was changed says so, and one click on the button next to the label is what it takes
    // to get the value back: the click must not be swallowed by the slot committing what was typed.
    await fillSlotValue(page, PROPERTY_IDS.MATERIAL, 0, ".pd-inputstring", CHANGED_MATERIAL, 3, "the first material after it was changed")
    await expect(revert, "the revert button of a field which was changed").toBeVisible()
    await checkpointElement(page, materialField, "edit-cardinality-revert-changed", volatileSelect(page))
    await revert.click()
    await expect
      .poll(async () => await slotValues(page, PROPERTY_IDS.MATERIAL, ".pd-inputstring"), { message: "the material field after one click on revert" })
      .toEqual([FIRST_MATERIAL, SECOND_MATERIAL, ""])
    await expect(revert, "the revert button once the field is back to what it held").toBeHidden()
    await checkpointElement(page, materialField, "edit-cardinality-revert-restored", volatileSelect(page))

    // Reverting a value which was removed brings the value back as a claim of its own, and the field
    // then counts as untouched again: a second click has nothing left to undo, and the restored value
    // must not be what it undoes.
    await fillSlotValue(page, PROPERTY_IDS.MATERIAL, 0, ".pd-inputstring", "", 2, "the first material after it was removed")
    expect(await slotValues(page, PROPERTY_IDS.MATERIAL, ".pd-inputstring"), "the material field with one value removed").toEqual([SECOND_MATERIAL, ""])
    await expect(revert, "the revert button of a field a value was removed from").toBeVisible()
    await revert.click()
    await expect
      .poll(async () => [...(await slotValues(page, PROPERTY_IDS.MATERIAL, ".pd-inputstring"))].sort(), { message: "the material field after the removal was reverted" })
      .toEqual(["", FIRST_MATERIAL, SECOND_MATERIAL].sort())
    await expect(revert, "the revert button once the removed value is back").toBeHidden()
    await checkpointElement(page, materialField, "edit-cardinality-revert-resurrected", volatileSelect(page))

    // The same for one entry of the field: every slot carries a revert of its own, which takes back what
    // was done to that slot alone.
    const secondSlot = fieldSlots(page, PROPERTY_IDS.MATERIAL).nth(1)
    const beforeEntryChange = await slotValues(page, PROPERTY_IDS.MATERIAL, ".pd-inputstring")
    await fillSlotValue(page, PROPERTY_IDS.MATERIAL, 1, ".pd-inputstring", CHANGED_MATERIAL, 3, "the second material after it was changed")
    const entryRevert = secondSlot.locator(".pd-claimcardinality-button-revert")
    await expect(entryRevert, "the revert button of the entry which was changed").toBeVisible()
    await entryRevert.click()
    await expect
      .poll(async () => await slotValues(page, PROPERTY_IDS.MATERIAL, ".pd-inputstring"), { message: "the material field after the entry was reverted" })
      .toEqual(beforeEntryChange)
    await expect(revert, "the revert button of the field once the entry is back").toBeHidden()

    // Nothing of all that reaches the document: the session is saved with the field as it was found. The
    // name is changed first, because a session which was reverted to what it started from has nothing to
    // save and the save button stays disabled until something in it does differ.
    await fillName(page, `${NAME_PREFIX} Revert Artifact Saved`)
    await saveEdit(page)
    expect([...(await storedStrings(page, id, PROPERTY_IDS.MATERIAL))].sort(), "the materials of the saved artifact").toEqual([FIRST_MATERIAL, SECOND_MATERIAL].sort())

    console.log(`Successfully reverted 3 changes to a repeated field of document ${id}, each on the first click, leaving the field with the 2 values it was opened with.`)
  })
})
