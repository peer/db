<!--
FieldsFormField renders one InputCardinality per field, with all rows
(existing claims + freshly added new entries) managed as a single list.

Per-row identity is tracked via inputToClaimId, a Map keyed by the row's
self-registered ValidatedInput (FieldsFormRow exposes one per row). On
mount, syncFromDoc populates a seed queue from the doc's existing claims;
each new input registered by InputCardinality is drained one entry off
the queue, its values are seeded via the useRepeatedInputs' setFor, and
its claim ID is recorded. Newly added rows that the user types into get
their claim IDs assigned at flush time.

Has/None/Unknown are presence-only and skip InputCardinality entirely:
existing claims are listed as static rows with a Remove button, and a
simple "+" button issues an AddClaimChange directly.
-->

<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { ClaimTypes, TimePrecision } from "@/document"
import type { ExistingClaimValue, FieldData, FieldEntryValue, FieldsFormSaveChange, FlushFn } from "@/fields"
import type { ValidatedInput } from "@/types"

import CheckBox from "@/components/CheckBox.vue"
import { VT_HAS, VT_NONE, VT_UNKNOWN } from "@/core"
import { ClaimTypes as ClaimTypesClass } from "@/document"
import { AddClaimChange, RemoveClaimChange, SetClaimChange } from "@/document/patch"
import {
  emptyFieldEntryValue,
  equalFieldEntryValue,
  getExistingClaimValues,
  getNextChangeNumberKey,
  makePatchForField,
  registerForFlushKey,
  saveChangeKey,
  unregisterForFlushKey,
} from "@/fields"
import DocumentRefInline from "@/partials/DocumentRefInline.vue"
import FieldsForm from "@/partials/FieldsForm.vue"
import FieldsFormRow from "@/partials/FieldsFormRow.vue"
import InputBadges from "@/partials/InputBadges.vue"
import InputCardinality from "@/partials/InputCardinality.vue"
import { useRepeatedInput } from "@/useRepeatedInput"
import { useRegisterForValidation } from "@/validation"
import { Identifier } from "@tozd/identifier"
import { computed, inject, nextTick, onBeforeUnmount, onMounted, reactive, useTemplateRef, watch } from "vue"

const props = defineProps<{
  field: DeepReadonly<FieldData>
  claims: DeepReadonly<ClaimTypes>
  // Pre-session claims for this property's parent doc. Used as the
  // baseline for the "changed" badge (a property is "changed" iff its
  // current claim set differs from this baseline OR there is an
  // uncommitted in-input edit) and for Revert (we post the diff
  // back to restore the baseline).
  initialClaims: DeepReadonly<ClaimTypes>
  base: DeepReadonly<string[]>
  session: string
  parentClaimId?: string
}>()

let fallbackNum = 1
const getNextChangeNumber = inject(getNextChangeNumberKey, () => fallbackNum++)
const saveChange = inject(saveChangeKey, () => Promise.resolve())
const registerForFlush = inject(registerForFlushKey, () => {})
const unregisterForFlush = inject(unregisterForFlushKey, () => {})

function isToggleField(): boolean {
  return props.field.valueType === VT_HAS || props.field.valueType === VT_NONE || props.field.valueType === VT_UNKNOWN
}

// One useRepeatedInput per FieldEntryValue field. Each owns a Map<input, T>
// keyed on the row's FieldsFormRow ValidatedInput. modelFor(input) yields
// the v-model binding to spread onto FieldsFormRow; valueFor/rawFor read
// back the stored value at flush time; setFor seeds initial values.
const valueRI = useRepeatedInput("value", { default: "" })
const valueToRI = useRepeatedInput("valueTo", { default: "" })
const amountPrecisionRI = useRepeatedInput("amountPrecision", { default: "" })
const amountPrecisionToRI = useRepeatedInput("amountPrecisionTo", { default: "" })
const timePrecisionRI = useRepeatedInput("timePrecision", { default: "y" as TimePrecision })
const timePrecisionToRI = useRepeatedInput("timePrecisionTo", { default: "y" as TimePrecision })
const fromUnknownRI = useRepeatedInput("fromUnknown", { default: false })
const fromNoneRI = useRepeatedInput("fromNone", { default: false })
const toUnknownRI = useRepeatedInput("toUnknown", { default: false })
const toNoneRI = useRepeatedInput("toNone", { default: false })

// Per-row identity. Inputs not in this map are "new" rows (no claim ID
// yet); they get assigned in flush. Claim IDs without an input record
// represent an externally-added (or in-flight-just-committed) claim that
// has not been associated with any row in this component yet.
//
// Plain Map - not reactive on purpose. A reactive(Map) wraps its keys in a
// reactive proxy on iteration, which means a key looked up via has(input)
// where input is the raw ValidatedInput would never match the wrapped key
// yielded by keys()/entries(). This map is only read from script (flush,
// syncFromDoc), never templates, so it does not need reactivity.
const inputToClaimId = new Map<ValidatedInput, string>()

// seedQueue holds existing claims whose values still need to be seeded
// onto a yet-to-register InputCardinality row. Drained FIFO as the
// inputs-watcher sees each fresh input.
const seedQueue: ExistingClaimValue[] = []

// existingByClaimId caches the doc's current view of this field's claims,
// used both to drive the initial seed queue and to detect new external
// additions/removals between polls.
const existingByClaimId = reactive(new Map<string, ExistingClaimValue>())

// Presence-only types are managed outside InputCardinality: one row per
// existing toggle claim plus an "Add" button. The map mirrors
// existingByClaimId for these types so the template can iterate them.
const toggleEntries = computed<ExistingClaimValue[]>(() => (isToggleField() ? [...existingByClaimId.values()] : []))

const cardinalityRef = useTemplateRef<{
  reset: () => void
  revert: () => void
  // isDirty is exposed as a Ref<boolean> by defineExpose but the parent-side
  // proxy unwraps it on access, so we read it as a plain boolean here.
  isDirty: boolean
  inputs: ReadonlyArray<ValidatedInput>
  removeRow: (input: ValidatedInput) => void
}>("cardinalityRef")

// labelCellRef points to the field's <th> (or <td> for toggle fields).
// onRowFocusOut consults it to suppress the on-blur commit when focus
// moves to a control inside the label cell - specifically the revert
// button, which the InputBadges renders there.
const labelCellRef = useTemplateRef<HTMLElement>("labelCellRef")

// initialEntries / currentEntries reflect this property's claim set as
// it was at the session's start (initialEntries) vs as it is right now
// (currentEntries). currentEntries reads from existingByClaimId rather
// than props.claims directly because commitRow / onToggleChange /
// revertField mirror their writes into existingByClaimId before the
// next polling cycle has a chance to sync the doc - so a value posted
// via a live commit immediately reflects in the badge and Revert diff,
// not only after the GET-changes timer fires.
const initialEntries = computed<ExistingClaimValue[]>(() => getExistingClaimValues(props.initialClaims, props.field as FieldData))
const currentEntries = computed<ExistingClaimValue[]>(() => [...existingByClaimId.values()])

// sessionChanged is true when this property's claim set differs from
// the session's pre-session baseline. The diff is a multiset over
// FieldEntryValue contents, NOT claim IDs: Revert may have re-created
// a session-removed claim with a fresh content-addressed ID, but the
// VALUES match the baseline so the field is no longer "changed".
const sessionChanged = computed<boolean>(() => {
  const a = initialEntries.value
  const b = currentEntries.value
  if (a.length !== b.length) return true
  const consumed = new Set<number>()
  for (const ea of a) {
    let matched = -1
    for (let i = 0; i < b.length; i++) {
      if (consumed.has(i)) continue
      if (equalFieldEntryValue(ea, b[i])) {
        matched = i
        break
      }
    }
    if (matched < 0) return true
    consumed.add(matched)
  }
  return false
})

// Whether the field is "changed" - either has session-level diff from
// the baseline or has an uncommitted in-input edit. For toggle fields
// the cardinality has no rows, so we drop that side; for regular
// fields the cardinality's anyDirty captures in-input edits that have
// not yet been blur-committed.
const fieldChanged = computed<boolean>(() => {
  if (isToggleField()) return sessionChanged.value
  return sessionChanged.value || (cardinalityRef.value?.isDirty ?? false)
})

function isRequired(): boolean {
  return props.field.minCardinality > 0
}

function isMultiple(): boolean {
  return props.field.maxCardinality > 1
}

// revertField restores this property to its pre-session baseline:
//   - Claims removed during the session are re-added (new claim IDs,
//     same values as the baseline - identifier preservation across an
//     "undo a remove" round trip is not a goal).
//   - Claims added during the session are removed.
//   - Claims modified during the session are Set back to baseline.
//   - In-progress (uncommitted) input edits are also reverted to
//     their last-committed checkpoint (which, after the diff above
//     lands, is the baseline value).
//
// We mirror every diff into local state (existingByClaimId,
// inputToClaimId, seedQueue, cardinality rows) too. That lets the
// form match the just-posted state immediately, without waiting for
// the next polling cycle to sync the doc; otherwise the user sees
// their typed value linger in the input after they have clicked
// Revert and only catches up once the GET-changes timer fires.
async function revertField() {
  // Snap in-progress inputs back to their checkpoints first, so that
  // any subsequent reactive sync from the (about-to-be-mutated) doc
  // does not race against still-dirty inputs.
  if (!isToggleField()) {
    cardinalityRef.value?.revert()
  }

  const initial = initialEntries.value
  const current = currentEntries.value
  const initialById = new Map(initial.map((e) => [e.claimId, e]))
  const currentById = new Map(current.map((e) => [e.claimId, e]))

  // 1. Re-add claims that the session removed. The new claim IDs are
  //    derived from the change number's Base (content-addressed), so
  //    we cannot reuse the original ID, just the value.
  for (const ev of initial) {
    if (currentById.has(ev.claimId)) continue
    const num = getNextChangeNumber()
    const changeBase = [...props.base, "SESSION", props.session, String(num)]
    const newId = (await Identifier.from(...changeBase)).toString()
    const patch = makePatchForField(props.field as FieldData, ev)
    const addChange = new AddClaimChange({ id: newId, base: changeBase, patch })
    if (props.parentClaimId) addChange.under = props.parentClaimId
    await saveChange(addChange, num)
    // Mirror: record the resurrected claim locally so an existing or
    // newly-added cardinality row can pick it up via seedQueue.
    const newEntry: ExistingClaimValue = { ...ev, claimId: newId }
    existingByClaimId.set(newId, newEntry)
    seedQueue.push(newEntry)
  }

  // 2. Remove claims the session added.
  for (const ev of current) {
    if (initialById.has(ev.claimId)) continue
    const num = getNextChangeNumber()
    await saveChange(new RemoveClaimChange({ id: ev.claimId }), num)
    // Mirror: drop the local claim record and the row that owns it
    // (for toggles there are no rows, only existingByClaimId drives
    // the checkbox state).
    existingByClaimId.delete(ev.claimId)
    if (!isToggleField()) {
      for (const [input, cid] of inputToClaimId.entries()) {
        if (cid !== ev.claimId) continue
        inputToClaimId.delete(input)
        cardinalityRef.value?.removeRow(input)
      }
    }
  }

  // 3. Set claims that were modified but kept their ID back to baseline.
  for (const cur of current) {
    const init = initialById.get(cur.claimId)
    if (!init) continue
    if (equalFieldEntryValue(init, cur)) continue
    const patch = makePatchForField(props.field as FieldData, init)
    const num = getNextChangeNumber()
    await saveChange(new SetClaimChange({ id: cur.claimId, patch }), num)
    // Mirror: rewrite the local snapshot and re-seed the input row
    // tracking this claim so the visible value matches the baseline.
    const newEntry: ExistingClaimValue = { ...init, claimId: cur.claimId }
    existingByClaimId.set(cur.claimId, newEntry)
    if (!isToggleField()) {
      for (const [input, cid] of inputToClaimId.entries()) {
        if (cid !== cur.claimId) continue
        seedRow(input, newEntry)
      }
    }
  }
}

function seedRow(input: ValidatedInput, ev: ExistingClaimValue) {
  valueRI.setFor(input, ev.value)
  valueToRI.setFor(input, ev.valueTo)
  amountPrecisionRI.setFor(input, ev.amountPrecision)
  amountPrecisionToRI.setFor(input, ev.amountPrecisionTo)
  timePrecisionRI.setFor(input, ev.timePrecision)
  timePrecisionToRI.setFor(input, ev.timePrecisionTo)
  fromUnknownRI.setFor(input, ev.fromUnknown)
  fromNoneRI.setFor(input, ev.fromNone)
  toUnknownRI.setFor(input, ev.toUnknown)
  toNoneRI.setFor(input, ev.toNone)
  // After Vue propagates the seeded values down to the inner inputs,
  // re-checkpoint the row so isDirty starts at false (the input wraps
  // useValidation, which captured its checkpoint at the initial default).
  // Without this every seeded row would appear dirty and a Save would
  // generate redundant Sets for unchanged claims.
  void nextTick(() => input.checkpoint())
}

// rowBinding aggregates the v-model bindings for one InputCardinality row.
// Vue compiles multiple v-bind="..." onto a single element fine, but
// eslint-plugin-vue's template parser flags them as duplicate-attribute
// errors. Building one merged object lets the template stay with a single
// v-bind spread.
function rowBinding(input: ValidatedInput | null | undefined): Record<string, unknown> {
  return {
    ...valueRI.modelFor(input),
    ...valueToRI.modelFor(input),
    ...amountPrecisionRI.modelFor(input),
    ...amountPrecisionToRI.modelFor(input),
    ...timePrecisionRI.modelFor(input),
    ...timePrecisionToRI.modelFor(input),
    ...fromUnknownRI.modelFor(input),
    ...fromNoneRI.modelFor(input),
    ...toUnknownRI.modelFor(input),
    ...toNoneRI.modelFor(input),
  }
}

function readRowValue(input: ValidatedInput): FieldEntryValue {
  return {
    value: valueRI.rawFor(input),
    valueTo: valueToRI.rawFor(input),
    amountPrecision: amountPrecisionRI.rawFor(input),
    amountPrecisionTo: amountPrecisionToRI.rawFor(input),
    timePrecision: timePrecisionRI.rawFor(input),
    timePrecisionTo: timePrecisionToRI.rawFor(input),
    fromUnknown: fromUnknownRI.rawFor(input),
    fromNone: fromNoneRI.rawFor(input),
    toUnknown: toUnknownRI.rawFor(input),
    toNone: toNoneRI.rawFor(input),
  }
}

// syncFromDoc reconciles existingByClaimId with the doc and adds any
// not-yet-associated claims to the seed queue so freshly-registered rows
// can pick them up. We do NOT clear inputToClaimId entries whose claim
// has disappeared from the doc - the row's local typed value still belongs
// to the user, and Save's flush would emit the appropriate change anyway.
function syncFromDoc() {
  if (isToggleField()) {
    existingByClaimId.clear()
    for (const ev of getExistingClaimValues(props.claims, props.field as FieldData)) {
      existingByClaimId.set(ev.claimId, ev)
    }
    return
  }
  const existing = getExistingClaimValues(props.claims, props.field as FieldData)
  existingByClaimId.clear()
  for (const ev of existing) {
    existingByClaimId.set(ev.claimId, ev)
  }
  const assigned = new Set(inputToClaimId.values())
  const queued = new Set(seedQueue.map((ev) => ev.claimId))
  for (const ev of existing) {
    if (!assigned.has(ev.claimId) && !queued.has(ev.claimId)) {
      seedQueue.push(ev)
    }
  }
}

watch(() => props.claims, syncFromDoc, { deep: true, immediate: true })

// Watch InputCardinality's registered inputs in DOM order. Fresh inputs get
// the next queued existing value seeded into them (with claim ID recorded);
// rows the cardinality has removed (auto-shrink of trailing empties) drop
// out of inputToClaimId so the map does not retain stale identity.
watch(
  () => cardinalityRef.value?.inputs ?? [],
  (newInputs) => {
    for (const input of newInputs) {
      if (inputToClaimId.has(input)) continue
      const ev = seedQueue.shift()
      if (!ev) continue
      seedRow(input, ev)
      inputToClaimId.set(input, ev.claimId)
    }
    const present = new Set(newInputs)
    for (const input of inputToClaimId.keys()) {
      if (!present.has(input)) {
        inputToClaimId.delete(input)
      }
    }
  },
  { flush: "post" },
)

// Initial row count = max(1, existing count). InputCardinality auto-grows
// once seeded rows fill (the watcher above seeds them) so the user always
// sees one extra trailing empty row available for adding more.
const initialRowCount = computed(() => Math.max(1, existingByClaimId.size))

const cardMin = computed(() => props.field.minCardinality)
const cardMax = computed(() => (props.field.maxCardinality === Infinity ? null : props.field.maxCardinality))
const showCardinality = computed(() => !isToggleField())

// commitRow runs the Add / Set / Remove live-commit for one row.
//
// Invalid values are NOT posted. We await validate() first; if the
// row has any error, commitRow short-circuits. The row stays in the
// form, visually red, dirty, and uncommitted - the next blur / Save
// will re-attempt the commit once the user fixes the value. This is
// the only path that submits patches to the session, so a "htt"-style
// invalid IRI never reaches the backend.
//
// Empty existing rows (a claim that was present but the user cleared)
// are committed as RemoveClaimChange immediately. Per-property revert
// is the user's undo for accidental clears - we no longer defer the
// Remove to a final Save batch.
//
// Empty NEW rows (no claim ID, no value) are scratch space and are
// just dismissed from the cardinality without any server traffic.
async function commitRow(input: ValidatedInput) {
  if (!input.isDirty.value) return
  await input.validate()
  if (input.errors.value.length > 0) return
  const claimId = inputToClaimId.get(input)
  const empty = input.isEmpty.value
  if (claimId) {
    if (empty) {
      const num = getNextChangeNumber()
      await saveChange(new RemoveClaimChange({ id: claimId }), num)
      inputToClaimId.delete(input)
      existingByClaimId.delete(claimId)
      cardinalityRef.value?.removeRow(input)
      return
    }
    const value = readRowValue(input)
    const patch = makePatchForField(props.field as FieldData, value)
    const num = getNextChangeNumber()
    await saveChange(new SetClaimChange({ id: claimId, patch }), num)
    existingByClaimId.set(claimId, { ...value, claimId })
    input.checkpoint()
    return
  }
  if (empty) {
    cardinalityRef.value?.removeRow(input)
    return
  }
  const num = getNextChangeNumber()
  const changeBase = [...props.base, "SESSION", props.session, String(num)]
  const newId = (await Identifier.from(...changeBase)).toString()
  const value = readRowValue(input)
  const patch = makePatchForField(props.field as FieldData, value)
  const addChange = new AddClaimChange({ id: newId, base: changeBase, patch })
  if (props.parentClaimId) addChange.under = props.parentClaimId
  await saveChange(addChange, num)
  inputToClaimId.set(input, newId)
  existingByClaimId.set(newId, { ...value, claimId: newId })
  input.checkpoint()
}

async function onRowFocusOut(event: FocusEvent, input: ValidatedInput) {
  const target = event.currentTarget as Node | null
  const next = event.relatedTarget as Node | null
  // Focus moved to another element inside this row (e.g. for intervals,
  // tabbing from "from" to "to") - the row is not done being edited yet.
  if (target && next && target.contains(next)) return
  // Focus moved to a control inside this field's label cell (the revert
  // button is rendered there). The revert action - which posts the
  // reverse-diff itself - must not race with a stale-data commit, so
  // skip the commit when the user is on their way to revert. Mouse and
  // keyboard navigation both pass through relatedTarget, so this catches
  // tabbing to the revert button as well as clicking it.
  if (labelCellRef.value && next instanceof Node && labelCellRef.value.contains(next)) return
  await commitRow(input)
}

// Toggle state mirrors the doc directly - there is no local "intent"
// to defer because the only valid values are presence-or-absence of
// the claim and that's always valid. onToggleChange commits the
// Add/Remove immediately. The "changed" badge for toggle fields is
// driven by sessionChanged (current doc state vs initial baseline).
const toggleState = computed<boolean>(() => existingByClaimId.size > 0)

async function onToggleChange(checked: boolean | undefined) {
  const desired = !!checked
  const currentHas = existingByClaimId.size > 0
  if (desired === currentHas) return
  if (desired) {
    const num = getNextChangeNumber()
    const changeBase = [...props.base, "SESSION", props.session, String(num)]
    const claimId = (await Identifier.from(...changeBase)).toString()
    const empty = emptyFieldEntryValue()
    const patch = makePatchForField(props.field as FieldData, empty)
    const addChange = new AddClaimChange({ id: claimId, base: changeBase, patch })
    if (props.parentClaimId) addChange.under = props.parentClaimId
    await saveChange(addChange, num)
    existingByClaimId.set(claimId, { ...empty, claimId })
  } else {
    for (const claimId of [...existingByClaimId.keys()]) {
      const num = getNextChangeNumber()
      await saveChange(new RemoveClaimChange({ id: claimId }), num)
      existingByClaimId.delete(claimId)
    }
  }
}

// Register a small ValidatedInput representing this whole field with
// the enclosing FieldsForm registry so the form-level anyDirty (which
// powers DocumentEdit.canSave) reflects session-wide changes even
// when no per-row InputCardinality input is currently dirty. We bind
// isDirty to sessionChanged so a session that only contains a flipped
// toggle or a previously-committed live-commit still enables Save.
useRegisterForValidation({
  validate: async () => {},
  reset: () => {},
  revert: () => {},
  el: () => null,
  isDirty: sessionChanged,
  isEmpty: computed<boolean>(() => currentEntries.value.length === 0),
  errors: computed<[]>(() => []),
  checkpoint: () => {},
})

// flush is called from DocumentEdit.onSave to ensure no row is left
// uncommitted just because the user clicked Save without blurring out
// of the input first. commitRow already validates and posts changes
// directly to the session, so flush returns [] - the legacy
// "build changes, post later" contract collapses to a no-op here
// because the per-row live-commit path is now the only writer.
async function flush(): Promise<FieldsFormSaveChange[]> {
  if (isToggleField()) return []
  const inputs = cardinalityRef.value?.inputs ?? []
  for (const input of inputs) {
    await commitRow(input)
  }
  return []
}

const flushFn: FlushFn = flush
onMounted(() => registerForFlush(flushFn))
onBeforeUnmount(() => unregisterForFlush(flushFn))

function getSubClaims(claimId: string): DeepReadonly<ClaimTypes> {
  const claim = props.claims?.GetByID(claimId)
  return new ClaimTypesClass(claim?.sub ?? {})
}

// The initial baseline for sub-fields: the sub-claims of the
// corresponding parent claim AT SESSION START. For claims that were
// added during the session (claim ID not present in initialClaims),
// the baseline is an empty ClaimTypes - any sub-field a user adds
// underneath a session-added parent is "new" from the baseline's
// perspective.
function getInitialSubClaims(claimId: string): DeepReadonly<ClaimTypes> {
  const claim = props.initialClaims?.GetByID(claimId)
  return new ClaimTypesClass(claim?.sub ?? {})
}
</script>

<template>
  <!--
    The field group is a semantic <tbody> laid out as a 2-column CSS grid
    (1/5 label, rest content). <tr>s use display: contents (Tailwind
    "contents") so their <th>/<td> children participate directly in the
    grid - the property's row markup is preserved for screen readers
    while the visual layout is grid-based.
  -->
  <tbody class="grid grid-cols-[20%_1fr] items-start gap-y-1 px-2">
    <!-- Non-toggle: a single InputCardinality manages every row (existing + new).
         The label cell uses a flex column for property name + InputBadges
         (same shape as InputHTML's bottom toolbar label column): the badges
         live on their own line so a long property name does not push them
         off the side of the 1/5 column. -->
    <template v-if="showCardinality">
      <tr class="contents">
        <th ref="labelCellRef" scope="row" class="text-left font-medium text-gray-700">
          <div class="flex flex-col items-start gap-1">
            <DocumentRefInline :id="field.propertyId" :link="false" class="pt-0.5 leading-none" />
            <div class="flex flex-row flex-wrap gap-1">
              <InputBadges :required="isRequired()" :multiple="isMultiple()" :changed="fieldChanged" @revert="revertField" />
            </div>
          </div>
        </th>
        <td>
          <InputCardinality ref="cardinalityRef" :min="cardMin" :max="cardMax" :required="isRequired()" :initial="initialRowCount">
            <template #default="{ input, invalid }">
              <!--
                flex-col so InputErrors's [input, <p>] fragment stacks vertically:
                input on top, error message below. With flex-row the <p>
                rendered to the right of the input instead.
              -->
              <div class="flex min-w-0 grow flex-col" @focusout="(e: FocusEvent) => onRowFocusOut(e, input)">
                <FieldsFormRow :field="field" v-bind="rowBinding(input)" :required="false" :invalid="invalid" />
              </div>
            </template>
          </InputCardinality>
        </td>
      </tr>
      <!-- One sub-fields recursion per existing claim - nested FieldsForm reads
           that claim's sub-claims. We do NOT recurse for not-yet-persisted rows;
           the user adds the parent claim first, then sub-fields appear after the
           doc syncs. -->
      <template v-if="field.subFields.length > 0">
        <tr v-for="ev in existingByClaimId.values()" :key="ev.claimId + '-sub'" class="contents">
          <td></td>
          <td>
            <FieldsForm
              :fields-data="{ sections: [], fields: field.subFields }"
              :claims="getSubClaims(ev.claimId)"
              :initial-claims="getInitialSubClaims(ev.claimId)"
              :base="base"
              :session="session"
              :parent-claim-id="ev.claimId"
            />
          </td>
        </tr>
      </template>
    </template>

    <!-- Toggle: presence-only HAS/NONE/UNKNOWN - the field IS one checkbox.
         Label cell looks like every other field's (property name + badges);
         the right cell holds a sole checkbox. Sub-claims of an existing
         toggle claim render in a follow-up row beneath. There is no
         "Add another" affordance: the field is binary. -->
    <template v-else>
      <tr class="contents">
        <th scope="row" class="text-left font-medium text-gray-700">
          <div class="flex flex-col items-start gap-1">
            <DocumentRefInline :id="field.propertyId" :link="false" class="pt-0.5 leading-none" />
            <div class="flex flex-row flex-wrap gap-1">
              <InputBadges :required="isRequired()" :multiple="isMultiple()" :changed="fieldChanged" @revert="revertField" />
            </div>
          </div>
        </th>
        <td>
          <CheckBox :model-value="toggleState" @update:model-value="onToggleChange" />
        </td>
      </tr>
      <!-- For HAS with sub-fields: when the toggle is on, the user can edit
           the sub-claims attached to the (single) existing toggle claim. -->
      <template v-if="field.subFields.length > 0">
        <tr v-for="ev in toggleEntries" :key="ev.claimId + '-sub'" class="contents">
          <td></td>
          <td>
            <FieldsForm
              :fields-data="{ sections: [], fields: field.subFields }"
              :claims="getSubClaims(ev.claimId)"
              :initial-claims="getInitialSubClaims(ev.claimId)"
              :base="base"
              :session="session"
              :parent-claim-id="ev.claimId"
            />
          </td>
        </tr>
      </template>
    </template>
  </tbody>
</template>
