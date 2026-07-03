<!--
ClaimCardinality renders one ClaimInput per slot for a given field. Slots
are local state (stable keys), reconciled with props.modelValue (which is
the doc's current claims for this field) on prop change and updated
optimistically on per-slot @update:modelValue.

Auto-grow / auto-shrink keeps exactly one trailing-empty slot when under
maxCardinality. A slot's emptiness is provided by the wrapped ClaimInput
(its isEmpty includes both its own local raw-value emptiness and the
emptiness of every sub-ClaimCardinality below it, so a HAS slot whose
sub-claims are dirty does not get auto-shrunk).

initialClaims is the pre-session baseline used for revert and the
"Changed" flag. The field-level Revert button on FieldsFormField calls
revert() exposed here.
-->

<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { Claim } from "@/document"
import type { FieldData } from "@/fields"
import type { InputColumn, ValidatedInput } from "@/types"

import { computed, onBeforeUnmount, provide, ref, shallowReactive, useTemplateRef, watch } from "vue"
import { useI18n } from "vue-i18n"

import { AddClaimChange, claimPatchFrom, claimTypeName, RemoveClaimChange } from "@/document"
import { getClaimValues, makePatchForField, valueTypeToClaimType } from "@/fields"
import ClaimInput from "@/partials/ClaimInput.vue"
import DocumentRefInline from "@/partials/DocumentRefInline.vue"
import InputBadges from "@/partials/InputBadges.vue"
import { useLocked } from "@/progress"
import { allErrors, useRegisterForValidation, useValidationRegistry } from "@/validation"
import { ArrowPathSingleCounterclockwiseIcon } from "@sidekickicons/vue/20/solid"
import { Identifier } from "@tozd/identifier"

import { inject as injectFn } from "vue"

import { fieldLabelCellKey, getNextChangeNumberKey, saveChangeKey } from "@/fields"

const props = withDefaults(
  defineProps<{
    modelValue: DeepReadonly<readonly Claim[]>
    initialClaims: DeepReadonly<readonly Claim[]>
    field: DeepReadonly<FieldData>
    parentClaimId?: () => Promise<string>
    invalid?: boolean
    session: string
    base: readonly string[]
    // Id of the (sub)field's label element, provided down so a bare value input
    // is named via InputField's labelledby.
    labelId?: string
    // When true (sub-fields), render the field's label + whole-field badge as a
    // header above the slots. Top-level fields render no header (their label
    // lives in FieldsFormField's left cell).
    showHeader?: boolean
  }>(),
  {
    parentClaimId: undefined,
    invalid: false,
    labelId: undefined,
    showHeader: false,
  },
)

// When this cardinality renders its own header (sub-fields), the header holds
// the Revert button, so it acts as the "label cell" for the slots below: their
// ClaimInputs skip the commit when focus moves to it (avoiding a commit/revert
// race). Top-level cardinalities pass the FieldsFormField label cell through.
const parentLabelCell = injectFn(fieldLabelCellKey, () => null)
const headerRef = useTemplateRef<HTMLElement>("headerRef")
provide(fieldLabelCellKey, () => (props.showHeader ? headerRef.value : parentLabelCell()))

// A field is repeated when it can hold more than one value; repeated slots are
// numbered (1., 2., ...) so repetition reads differently from sub-field nesting.
const isRepeated = computed<boolean>(() => props.field.maxCardinality > 1)

// Repeated entries spread further apart (gap-8) when each entry is non-simple -
// the field has sub-fields, so every slot renders a value plus sub-field blocks.
// A plain repeated value uses the tighter gap-4.
const entryGapClass = computed<string>(() => (props.field.subFields.length > 0 ? "gap-y-8" : "gap-y-4"))

// Whether the value input renders its own label row, read from the columns the
// slot's input reports (a labeled column means a label row: amount/precision,
// time/precision, interval bounds). For those the count cell reserves a matching
// empty grid row (one line height, plus the label's mb-1 gap) above the count,
// so the count lines up with the input rather than the labels. All slots of a
// field share the same input type, so the first mounted slot is representative.
const hasLabelRow = computed<boolean>(() => {
  for (const input of slotInputs.values()) {
    const cols = input.columns as unknown as InputColumn[] | undefined
    return (cols ?? []).some((col) => col.label !== "")
  }
  return false
})

// A repeated field whose value input has no labels of its own (e.g. a repeated
// string) shows the per-entry changed/revert as a small icon under each count,
// rather than a single whole-field changed/revert on the field's label. Fields
// whose inputs have labels keep the per-input badge next to those labels.
const perEntryRevert = computed<boolean>(() => isRepeated.value && !hasLabelRow.value)

const { t } = useI18n({ useScope: "global" })

let fallbackNum = 1
const getNextChangeNumber = injectFn(getNextChangeNumberKey, () => fallbackNum++)
const saveChange = injectFn(saveChangeKey, () => Promise.resolve())

const locked = useLocked()

// Slots: the user-facing list of editable rows. Each has a stable key
// so Vue's v-for can re-anchor across claim renames; claim is the
// committed claim (or null for an as-yet-unfilled trailing slot).
// baseline is the session-start claim this slot represents (or null for
// a session-added / trailing-empty slot). It is set at mount from
// initialClaims and again whenever revertField resurrects a previously
// removed baseline. Tracking it explicitly is important: after a
// resurrect the slot's claim has a brand-new content-addressed id (a
// re-Add cannot reuse the original id), so a naive lookup by claim.id
// would miss the baseline and mis-classify the slot as session-added on
// the next revert click - which would immediately remove what we just
// added back.
type Slot = { key: string; claim: DeepReadonly<Claim> | null; baseline: DeepReadonly<Claim> | null }

let slotKeyCounter = 0
function nextSlotKey(): string {
  return `slot-${slotKeyCounter++}`
}

const slots = ref<Slot[]>([])

// slotsCheckpoint is the baseline used by revert + the "Changed" diff.
// Seeded from initialClaims; watched so a re-anchored baseline (mid-
// session reload, remount with a different doc/version) updates without
// requiring the component to unmount.
const slotsCheckpoint = ref<readonly DeepReadonly<Claim>[]>(props.initialClaims)

// reanchorSlotBaselines re-resolves slot.baseline against the current
// slotsCheckpoint so a re-anchor (props.initialClaims change, or
// checkpoint() after Save) makes existing slots correctly reflect what
// they represent against the new baseline.
function reanchorSlotBaselines(): void {
  const baselineById = new Map<string, DeepReadonly<Claim>>()
  for (const b of slotsCheckpoint.value) baselineById.set(b.id, b)
  for (const slot of slots.value) {
    slot.baseline = slot.claim ? (baselineById.get(slot.claim.id) ?? null) : null
  }
}

watch(
  () => props.initialClaims,
  (v) => {
    slotsCheckpoint.value = v
    reanchorSlotBaselines()
  },
  { flush: "sync" },
)

// rootRef is the DOM identity for self-registration into the surrounding
// validation registry.
const rootRef = useTemplateRef<HTMLDivElement>("rootRef")

// Sub-registry: each ClaimInput's ValidatedInput registers here. We
// expose ourselves to the outer registry as one ValidatedInput by
// self-registering after building the composite below.
let forwardInteraction: (() => void) | null = null
const {
  validateAll: validateChildAll,
  resetAll: resetChildAll,
  checkpointAll: checkpointChildAll,
  anyDirty: anyChildDirty,
  allEmpty: allChildEmpty,
  inputs: childInputs,
  firstInputEl: firstChildInputEl,
} = useValidationRegistry(() => {
  reconcileSlots()
  forwardInteraction?.()
})

// slotInputs maps a slot key to the ClaimInput's exposed object (we use it
// to read each slot's isEmpty and to call its revert()). Set via the
// :ref function in the template. shallowReactive so add/delete trigger
// reactivity but the values (which include refs from defineExpose) are
// not deeply wrapped.
const slotInputs = shallowReactive(new Map<string, ExposedClaimInput>())

// ClaimInput's defineExpose overrides ValidatedInput.revert with the
// async version so we can await per-slot reverts during a field-level
// revert. Reflect that here so the type at the call site matches.
type ExposedClaimInput = Omit<ValidatedInput, "revert"> & {
  revert: () => Promise<void>
  ensureClaimId: () => Promise<string>
  hasValue: boolean
}

function setSlotRef(key: string, el: unknown): void {
  if (el == null) {
    slotInputs.delete(key)
    return
  }
  slotInputs.set(key, el as ExposedClaimInput)
}

// Per-slot dirty + revert, used to drive the per-entry revert icon under the
// count (see perEntryRevert). isDirty is exposed as a Ref by ClaimInput but the
// parent-side proxy unwraps it, so we read it as a plain boolean.
function slotDirty(key: string): boolean {
  return (slotInputs.get(key)?.isDirty as unknown as boolean) === true
}

function revertSlot(key: string): void {
  void slotInputs.get(key)?.revert()
}

// Per-slot emptiness reading. Always defer to the ClaimInput's exposed
// isEmpty: it covers both the committed claim AND the user's local
// uncommitted edit (and any sub-claim activity). Using slot.claim alone
// would mis-classify a slot whose claim is still set but whose user
// just cleared the value - and that mis-classification is exactly what
// breaks auto-shrink for "user emptied a field, blurred" flows.
function slotIsEmpty(slot: Slot): boolean {
  const input = slotInputs.get(slot.key)
  if (input) {
    return (input.isEmpty as unknown as boolean) === true
  }
  // No input registered yet (first render): use the committed claim.
  return slot.claim === null
}

// slotHasValue reports whether a slot has a base value, used to decide whether to offer a new
// trailing slot. A slot whose only content is sub-claims (a default field's none/unknown form,
// e.g. a studio with notes but no location) has no base value and does not grow a placeholder.
function slotHasValue(slot: Slot): boolean {
  const input = slotInputs.get(slot.key)
  if (input) {
    return input.hasValue
  }
  // No input registered yet (first render): a committed claim of the field's value type has a
  // value; a default (none/unknown) claim, or no claim, does not.
  return slot.claim !== null && claimTypeName(slot.claim) === valueTypeToClaimType(props.field.valueType)
}

// reconcileSlots maintains exactly one trailing empty slot under
// maxCardinality, growing when the last slot becomes non-empty and
// shrinking when an empty slot ends up trailing.
//
// We deliberately do NOT sync from props.modelValue here. Slots are the
// local source of truth after mount; modelValue is only the doc's view
// (which lags by a poll cycle). Reading from it during the optimistic
// window after a per-slot emit causes duplicate slots (we already set
// slot.claim, but modelValue still has the old version under the same
// id - or worse, after a remove, the stale claim looks "new" and the
// reconcile pushes a fresh slot for it).
function reconcileSlots(): void {
  const max = props.field.maxCardinality === Infinity ? Number.MAX_SAFE_INTEGER : props.field.maxCardinality
  // lastContentIdx: last slot with any content (a value or sub-claims) - these are kept.
  // lastValueIdx: last slot with a base value - a trailing empty slot is offered only after one of
  // these. A slot whose only content is sub-claims (a default field's notes-only entry) is kept
  // but does not grow a further placeholder.
  let lastContentIdx = -1
  let lastValueIdx = -1
  for (let i = slots.value.length - 1; i >= 0; i--) {
    if (lastContentIdx === -1 && !slotIsEmpty(slots.value[i])) {
      lastContentIdx = i
    }
    if (lastValueIdx === -1 && slotHasValue(slots.value[i])) {
      lastValueIdx = i
    }
    if (lastContentIdx !== -1 && lastValueIdx !== -1) {
      break
    }
  }
  let desired = Math.max(lastContentIdx + 1, lastValueIdx + 2) // keep content slots; one trailing empty after the last value
  if (desired > max) desired = max
  if (desired < 1) desired = Math.min(1, max)

  // Shrink: drop empty trailing slots beyond one. Skip the active-focus
  // slot so we never yank the user out of the row they are currently in.
  while (slots.value.length > desired) {
    const last = slots.value[slots.value.length - 1]
    if (!slotIsEmpty(last)) break // safety: don't drop a filled tail
    const lastEl = slotInputs.get(last.key)?.mainEl?.()
    const focused = typeof document !== "undefined" ? document.activeElement : null
    if (focused && lastEl?.contains(focused)) break // keep focused trailing
    slots.value.pop()
  }

  // Grow: append empty trailing if we're under desired.
  while (slots.value.length < desired) {
    slots.value.push({ key: nextSlotKey(), claim: null, baseline: null })
  }
}

// Initial population: do it eagerly on setup so the first render already
// has the right slots. Each slot's baseline is resolved by id from
// slotsCheckpoint; modelValue claims that aren't in the baseline are
// session-added (e.g. a mid-session reload picking up changes already
// committed during the session).
{
  const baselineById = new Map<string, DeepReadonly<Claim>>()
  for (const b of slotsCheckpoint.value) baselineById.set(b.id, b)
  for (const claim of props.modelValue) {
    slots.value.push({ key: nextSlotKey(), claim, baseline: baselineById.get(claim.id) ?? null })
  }
}
reconcileSlots()

// Re-reconcile whenever any slot's emptiness flips. We watch the
// computed array of per-slot isEmpty values so a flip in any one
// triggers reconcileSlots without us having to set up per-slot watchers.
const slotsIsEmptyVector = computed(() => slots.value.map((s) => slotIsEmpty(s)))
watch(slotsIsEmptyVector, () => reconcileSlots(), { flush: "post" })

// updateSlotClaim handles the per-slot @update:modelValue from
// ClaimInput. The new value is either a fresh claim (Add path), an
// updated claim (Set path with possibly the same id), or null (Remove
// path).
//
// When the slot transitions from a committed claim to null, we drop the
// slot entirely - that matches the original FieldsFormField behaviour
// (removeRow on commit-empty) and lets the user clear a field by blanking
// it instead of leaving an empty-but-mounted row behind.
function updateSlotClaim(slotKey: string, claim: DeepReadonly<Claim> | null): void {
  const idx = slots.value.findIndex((s) => s.key === slotKey)
  if (idx < 0) return
  const slot = slots.value[idx]
  if (slot.claim !== null && claim === null) {
    slots.value.splice(idx, 1)
    return
  }
  slot.claim = claim
}

// initialClaimForSlot returns the slot's baseline claim. This is set on
// the slot itself (at mount or revert-resurrect), so a resurrected slot
// whose claim has a fresh id still correctly points back at the original
// baseline.
function initialClaimForSlot(slot: Slot): DeepReadonly<Claim> | null {
  return slot.baseline
}

// Required-violation computation: if fewer than minCardinality slots
// are non-empty, mark the first N empty slots invalid. Suppressed when
// locked (the surrounding form is in a noop state).
const missing = computed<{ flags: boolean[]; ourErrors: { code: string; el?: HTMLElement }[] }>(() => {
  if (locked.value) return { flags: [], ourErrors: [] }
  const min = props.field.minCardinality
  let nonEmptyCount = 0
  for (const slot of slots.value) {
    if (!slotIsEmpty(slot)) nonEmptyCount++
  }
  let need = min - nonEmptyCount
  const flags: boolean[] = []
  const ourErrors: { code: string; el?: HTMLElement }[] = []
  for (const slot of slots.value) {
    if (need <= 0 || !slotIsEmpty(slot)) {
      flags.push(false)
      continue
    }
    flags.push(true)
    ourErrors.push({ code: "required", el: slotInputs.get(slot.key)?.inputEl?.() ?? undefined })
    need--
  }
  return { flags, ourErrors }
})

// Gate that controls when missing-required is surfaced. Off at mount so
// a freshly-opened form does not yell "Required value." before the user
// has interacted with the field. Flipped on when the user blurs out of
// the cardinality (focus moves to something not inside us) AND there is
// an actual violation, or when validateAll runs (Save). Auto-clears the
// moment the field actually satisfies its min cardinality, so a
// subsequent empty-while-typing does not flash red mid-edit.
//
// We gate the auto-clear on minSatisfied rather than on
// missing.ourErrors.length === 0 because the latter is also "true" in
// the transient empty-slots window after a Remove (slots is briefly
// [] before reconcileSlots grows a fresh trailing) - which would
// inadvertently reset triggered and leave the new slot non-red even
// though min is still unsatisfied.
const triggered = ref(false)
const minSatisfied = computed<boolean>(() => {
  const min = props.field.minCardinality
  if (min <= 0) return true
  let nonEmptyCount = 0
  for (const slot of slots.value) {
    if (!slotIsEmpty(slot)) nonEmptyCount++
  }
  return nonEmptyCount >= min
})
watch(minSatisfied, (satisfied) => {
  if (satisfied) triggered.value = false
})

function onFocusOut(event: FocusEvent): void {
  const next = event.relatedTarget as Node | null
  if (next && rootRef.value?.contains(next)) return
  if (missing.value.ourErrors.length > 0) {
    triggered.value = true
  }
}

// slotsDirtyByDiff: the set of baselines represented by current slots
// differs from slotsCheckpoint. We key by slot.baseline (not slot.claim.id)
// so a resurrected slot - whose claim has a fresh id after the re-Add -
// is still recognised as representing the original baseline rather than
// looking session-added.
const slotsDirtyByDiff = computed<boolean>(() => {
  const baselineIds = new Set(slotsCheckpoint.value.map((c) => c.id))
  const representedIds = new Set<string>()
  for (const slot of slots.value) {
    if (!slot.claim) continue
    if (slot.baseline === null) return true // session-added
    representedIds.add(slot.baseline.id)
  }
  for (const id of baselineIds) {
    if (!representedIds.has(id)) return true // baseline removed
  }
  return false
})

// Whole-field dirty: a baseline diff (slot added/removed) or any child slot
// dirty. Drives the header's changed/revert badge (sub-fields) and is exposed
// upward (so FieldsFormField's left-cell badge sees it for top-level fields).
const isDirty = computed<boolean>(() => slotsDirtyByDiff.value || anyChildDirty.value)

// Header Revert (sub-fields): revert the whole (sub)field. Discards the Promise
// since the badge's event handler is synchronous.
function onHeaderRevert(): void {
  void revertField()
}

// Composite ValidatedInput exposed upward.
// As with ClaimInput, validatedInput.revert wraps the async revertField
// in a void-discarding thunk so the framework's revertAll cascade (which
// is synchronous) doesn't choke on the returned Promise. defineExpose
// overrides with the async version for direct callers.
const validatedInput: ValidatedInput = {
  validate: async (signal) => {
    await validateChildAll(signal)
    if (missing.value.ourErrors.length > 0) {
      triggered.value = true
    }
  },
  reset: () => {
    resetChildAll()
    // Rebuild slots from current modelValue + one trailing empty.
    slots.value = []
    const baselineById = new Map<string, DeepReadonly<Claim>>()
    for (const b of slotsCheckpoint.value) baselineById.set(b.id, b)
    for (const claim of props.modelValue) {
      slots.value.push({ key: nextSlotKey(), claim, baseline: baselineById.get(claim.id) ?? null })
    }
    reconcileSlots()
    triggered.value = false
  },
  revert: () => {
    void revertField()
  },
  // Focus target is the first slot's focusable control, or null until a slot
  // has mounted (focus helpers then skip past instead of landing on the
  // non-focusable wrapper). Identity (mainEl) is the cardinality wrapper
  // spanning all slots, which the outer registry's containment check needs.
  inputEl: firstChildInputEl,
  mainEl: () => rootRef.value,
  isDirty,
  isEmpty: allChildEmpty,
  errors: computed(() => {
    const ourErrors = triggered.value ? missing.value.ourErrors : []
    return [...ourErrors, ...allErrors(childInputs).value]
  }),
  checkpoint: () => {
    // Move the baseline forward to the current claims (mirroring what
    // <DocumentEdit> does after Save). Cascade child checkpoints so each
    // <ClaimInput> re-anchors too, and re-resolve every slot.baseline so
    // session-added rows that just got saved now count as "represents
    // baseline".
    slotsCheckpoint.value = props.modelValue
    reanchorSlotBaselines()
    checkpointChildAll()
  },
}

const { onInteraction: notifyOuter } = useRegisterForValidation(validatedInput)
forwardInteraction = notifyOuter

defineExpose({
  ...validatedInput,
  // Override with the async revertField so FieldsFormField's Revert
  // button can await it.
  revert: revertField,
  // So FieldsFormField's left-cell badge can drop its changed/revert for a
  // repeated label-less field (the revert then lives per entry, under the count).
  perEntryRevert,
})

// revertField runs the field-level Revert: re-add removed baseline
// claims, remove session-added claims, then cascade revert into each
// surviving slot's ClaimInput. We classify slots by slot.baseline, not
// by claim id, so a slot resurrected on a previous revert click (its
// claim has a fresh content-addressed id) is still correctly recognised
// as representing its original baseline.
async function revertField(): Promise<void> {
  // Snapshot which baselines are already represented before any of our
  // mutations. The new (resurrected) slots we push below get
  // baseline:set so the next-click revert correctly classifies them.
  const representedBaselineIds = new Set<string>()
  for (const slot of slots.value) {
    if (slot.claim && slot.baseline) representedBaselineIds.add(slot.baseline.id)
  }

  // 1) Re-add baseline claims that no current slot represents. Resolve the (possibly lazily-
  // created) parent FIRST so it gets the lower change number and is posted before this claim;
  // the server requires change numbers to arrive in sequence.
  for (const baseline of slotsCheckpoint.value) {
    if (representedBaselineIds.has(baseline.id)) continue
    const under = props.parentClaimId ? await props.parentClaimId() : undefined
    const num = getNextChangeNumber()
    const changeBase = [...props.base, "SESSION", props.session, String(num)]
    const newId = (await Identifier.from(...changeBase)).toString()
    const values = getClaimValues(baseline)
    const patch = makePatchForField(props.field, values)
    const addChange = new AddClaimChange({ id: newId, base: changeBase, patch })
    if (under !== undefined) {
      addChange.under = under
    }
    await saveChange(addChange, num)
    const newClaim = claimPatchFrom(patch).New(newId)
    slots.value.push({ key: nextSlotKey(), claim: newClaim, baseline })
  }

  // 2) Remove session-added claims (slots whose baseline is null).
  // Iterate backwards so splicing while iterating doesn't skip entries.
  for (let i = slots.value.length - 1; i >= 0; i--) {
    const slot = slots.value[i]
    if (!slot.claim) continue
    if (slot.baseline !== null) continue
    const num = getNextChangeNumber()
    await saveChange(new RemoveClaimChange({ id: slot.claim.id }), num)
    slots.value.splice(i, 1)
  }

  // 3) Cascade revert into surviving slots. Each ClaimInput's revertField
  // sees its baseline (via initialClaimForSlot -> slot.baseline) and
  // computes the Add / Set / no-op accordingly. For a slot whose values
  // already match its baseline (resurrected slot, or untouched
  // original), this is a no-op.
  for (const slot of slots.value) {
    if (!slot.claim) continue
    const input = slotInputs.get(slot.key)
    if (!input) continue
    await input.revert()
  }

  // 4) Cleanup: drop leftover empty (claim-less) slots, then let
  // reconcileSlots grow exactly one trailing empty. The empties cleaned
  // up here include the trailing-empty that the cardinality auto-grew
  // earlier (e.g. after the user cleared a claim and updateSlotClaim
  // spliced its row), and any extra empties the per-await reconcileSlots
  // re-runs may have inserted in between our resurrected rows. Doing
  // this AT THE END keeps the awaits in step 1 from triggering a
  // mid-revertField reconcile that would otherwise create stranded
  // empties between filled slots.
  slots.value = slots.value.filter((s) => s.claim !== null)
  reconcileSlots()
}

onBeforeUnmount(() => {
  slotInputs.clear()
})
</script>

<template>
  <!--
    Renders one ClaimInput per slot. Each slot (a repeated entry, or the single
    entry of a non-repeated field) is its own "group" with a gray left rail, so
    repeated entries read as separate blocks and nesting shows via the rails'
    indentation. Repeated fields number each slot in a min-content count column.

    The sub-field header (the field label + whole-field badge, shown only via
    showHeader) sits left-aligned above the slots with mb-4; a transparent
    rail-width border keeps it aligned with the slots' content (the count), while
    only the slots carry the visible rail. Top-level fields render no header
    (their label is in FieldsFormField's left cell).
  -->
  <div ref="rootRef" class="flex min-w-0 grow flex-col" @focusout="onFocusOut">
    <div v-if="showHeader" ref="headerRef" class="mb-4 flex flex-row flex-wrap items-center gap-1 border-l-4 border-transparent pl-4">
      <span :id="labelId" class="leading-none font-medium text-gray-700"><DocumentRefInline :id="field.propertyId" :link="false" /></span>
      <InputBadges :required="field.minCardinality > 0" :multiple="field.maxCardinality > 1" :changed="isDirty" :revertable="!perEntryRevert" @revert="onHeaderRevert" />
    </div>
    <div v-if="isRepeated" class="flex flex-col" :class="entryGapClass">
      <div v-for="(slot, idx) in slots" :key="slot.key" class="grid grid-cols-[min-content_auto] items-start gap-x-4 border-l-4 border-gray-200 pl-4">
        <!--
          When the value input has a label row, the count cell reserves a matching
          empty grid row (one line height) above the count and places the count in
          the second row, so it lines up with the input. When the input has no
          labels, the count sits at the top and a per-entry revert icon sits under
          it. The button is a square the same height as the "changed" badge,
          rendered unconditionally (just visibility:hidden when the entry is
          unchanged) so it always reserves the count column's width and the input
          does not shift when it appears. The mousedown is prevented so clicking it
          does not blur the value input first (which would commit before revert).
        -->
        <div :class="hasLabelRow ? 'grid grid-rows-[1lh_auto] gap-y-1' : 'flex flex-col items-start gap-y-1'">
          <div class="pt-0.5 leading-none font-medium text-gray-700 italic" :class="{ 'row-start-2': hasLabelRow }">{{ idx + 1 }}.</div>
          <button
            v-if="perEntryRevert"
            type="button"
            :title="t('common.buttons.revert')"
            class="flex items-center justify-center rounded-xs bg-primary-300 p-0.5 text-gray-100 shadow-xs outline-none hover:cursor-pointer hover:bg-primary-400 focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 active:bg-primary-500"
            :class="{ invisible: !slotDirty(slot.key) }"
            @mousedown.prevent
            @click="revertSlot(slot.key)"
          >
            <ArrowPathSingleCounterclockwiseIcon class="size-3" aria-hidden="true" />
          </button>
        </div>
        <ClaimInput
          :ref="(el) => setSlotRef(slot.key, el)"
          :model-value="slot.claim"
          :initial-claim="initialClaimForSlot(slot)"
          :field="field"
          :parent-claim-id="parentClaimId"
          :invalid="invalid || (triggered && missing.flags[idx]) || false"
          :required="triggered && missing.flags[idx]"
          :is-first="idx === 0"
          :session="session"
          :base="base"
          :label-id="labelId"
          @update:model-value="(claim) => updateSlotClaim(slot.key, claim)"
        />
      </div>
    </div>
    <template v-else>
      <div v-for="(slot, idx) in slots" :key="slot.key" class="border-l-4 border-gray-200 pl-4">
        <ClaimInput
          :ref="(el) => setSlotRef(slot.key, el)"
          :model-value="slot.claim"
          :initial-claim="initialClaimForSlot(slot)"
          :field="field"
          :parent-claim-id="parentClaimId"
          :invalid="invalid || (triggered && missing.flags[idx]) || false"
          :required="triggered && missing.flags[idx]"
          :is-first="idx === 0"
          :session="session"
          :base="base"
          :label-id="labelId"
          @update:model-value="(claim) => updateSlotClaim(slot.key, claim)"
        />
      </div>
    </template>
  </div>
</template>
