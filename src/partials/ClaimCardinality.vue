<!--
ClaimCardinality renders one ClaimInput per slot for a given field. Slots
are local state (stable keys), reconciled with props.modelValue (which is
the doc's current claims for this field) on prop change and updated
optimistically on per-slot @update:modelValue.

Auto-grow / auto-shrink keeps one trailing-empty slot when under
maxCardinality, and never fewer than minCardinality slots so every
designated (required) slot is visible. A slot's emptiness is provided by the wrapped ClaimInput
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
import type { InputColumn, Result, SaveChangeResult, SaveChangeSpec, ValidatedInput } from "@/types"

import { computed, nextTick, onBeforeMount, onBeforeUnmount, onMounted, provide, ref, shallowReactive, shallowRef, useTemplateRef, watch } from "vue"
import { useI18n } from "vue-i18n"
import { useRouter } from "vue-router"

import { postJSON } from "@/api"
import { claimPatchFrom, claimTypeName } from "@/document"
import { getClaimValues, getFieldInstructions, makePatchForField, valueTypeToClaimType } from "@/fields"
import ClaimInput from "@/partials/ClaimInput.vue"
import ClaimRefSelect from "@/partials/ClaimRefSelect.vue"
import DocumentRefInline from "@/partials/DocumentRefInline.vue"
import InputBadges from "@/partials/InputBadges.vue"
import { useInternalLinksClick, useTransformedHtml } from "@/internal-links"
import { useLocked, useProgress } from "@/progress"
import { shortcutToFilters } from "@/shortcut"
import { escapeHtml } from "@/utils"
import { allErrors, useRegisterForValidation, useValidationRegistry } from "@/validation"
import { ArrowPathSingleCounterclockwiseIcon } from "@sidekickicons/vue/20/solid"

import { inject as injectFn } from "vue"

import { ChangeDroppedError, fieldLabelCellKey, registerRemoteAddsKey, saveChangeKey, unregisterRemoteAddsKey } from "@/fields"

const props = withDefaults(
  defineProps<{
    modelValue: DeepReadonly<readonly Claim[]>
    initialClaims: DeepReadonly<readonly Claim[]>
    field: DeepReadonly<FieldData>
    parentClaimId?: () => Promise<string>
    // parentCleanup asks the enclosing slot to remove its lazily-created base claim
    // (an empty HAS or a default none/unknown form) once this sub-field's revert has
    // emptied it. The base came into existence implicitly with the first sub-claim, so
    // its removal mirrors that; otherwise an invisible empty claim would keep the field
    // flagged as changed with no control left to remove it. Undefined for top-level
    // fields.
    parentCleanup?: () => Promise<void>
    invalid?: boolean
    // Set by an enclosing slot whose own change is still being committed: all slots of
    // this (sub)field render read-only until the ancestor's committed state settles.
    readonly?: boolean
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
    parentCleanup: undefined,
    invalid: false,
    readonly: false,
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

// Repeated entries spread further apart (mt-8 on every entry but the first) when
// each entry is non-simple - the field has sub-fields, so every slot renders a
// value plus sub-field blocks. A plain repeated value uses the tighter mt-4. A
// margin instead of a container gap because the entries are rows of the shared
// repeated-layout grid, whose other rows (labels, hints) use tighter spacing.
const entryGapClass = computed<string>(() => (props.field.subFields.length > 0 ? "mt-8" : "mt-4"))

// The columns and hints of the slots' value input, read from the first mounted
// slot (all slots of a field share the same input type). Empty until one mounts.
const slotColumns = computed<InputColumn[]>(() => {
  for (const input of slotInputs.values()) {
    return (input.columns as unknown as InputColumn[] | undefined) ?? []
  }
  return []
})
const slotHints = computed<string[]>(() => {
  for (const input of slotInputs.values()) {
    return (input.hints as unknown as string[] | undefined) ?? []
  }
  return []
})

// Whether the value input renders a label row (a labeled column means one:
// amount/precision, time/precision, interval bounds).
const hasLabelRow = computed<boolean>(() => slotColumns.value.some((col) => col.label !== ""))

// Whether the field's value input is an interval (a from/to pair of InputFields).
// Interval entries keep their own label rows - the per-bound changed/revert badges
// live there - so a repeated interval hoists only its hint, not its labels.
const isInterval = computed<boolean>(() => {
  const claimType = valueTypeToClaimType(props.field.valueType)
  return claimType === "amountInterval" || claimType === "timeInterval"
})

// Grid template of the hoisted label row of a repeated field, mirroring
// InputField's grid so the labels align with the entries' input columns (same
// container width, same template).
const labelsGridTemplateColumns = computed<string>(() =>
  [`minmax(0,${slotColumns.value[0]?.width ?? "1fr"})`, ...Array(Math.max(0, slotColumns.value.length - 1)).fill("auto")].join(" "),
)

// A press on a hoisted column label focuses that column's control in the first
// entry, like InputField's own labels do (mousedown-prevented in the template so
// the currently focused control is not blurred to the body first).
function onColumnLabelMousedown(col: InputColumn): void {
  col.el()?.focus()
}

// A repeated field shows a per-entry changed/revert as a small icon under each
// count, on top of the whole-field changed/revert on the field's label: the
// label-level revert reverts every entry, the count-level one only its entry.
const perEntryRevert = computed<boolean>(() => isRepeated.value)

// Select mode: a reference field without sub-fields and without a default shows
// ALL candidate documents as deselectable radio buttons (single value) or
// checkboxes (repeated), managed by a single ClaimRefSelect instead of the slots,
// when the field's filtered candidate set is small enough for the user to see
// every choice at once. The check runs once on mount: an empty-query search
// constrained by the field's filter returning at most SELECT_MODE_MAX results.
// The endpoint returns the first page of far more than SELECT_MODE_MAX results,
// so such a short response is the complete candidate set. Sub-field-bearing and
// default-form fields keep the combobox slots: their entries carry more than the
// reference itself.
const SELECT_MODE_MAX = 10

const selectEligible = computed<boolean>(
  () => valueTypeToClaimType(props.field.valueType) === "ref" && props.field.subFields.length === 0 && props.field.default === undefined,
)

// The candidate documents when in select mode, null otherwise (or while the check
// is still in flight, see modeResolved).
const refOptions = shallowRef<Result[] | null>(null)

// Gates rendering of eligible ref fields until the select-mode check resolves, so
// the field does not flash the combobox slots before switching to the list.
const modeResolved = ref(!selectEligible.value)

const router = useRouter()
const progress = useProgress()
const modeAbortController = new AbortController()
onBeforeUnmount(() => modeAbortController.abort())

onBeforeMount(async () => {
  if (!selectEligible.value) {
    return
  }
  progress.value += 1
  try {
    // shortcutToFilters throws on filters referencing "self" (no self document is
    // available here) and on parse errors; such fields keep the combobox slots.
    const filters = props.field.values ? await shortcutToFilters(props.field.values) : null
    const results = await postJSON<Result[]>(
      router.apiResolve({ name: "SearchJustResults" }).href,
      { query: "", ...(filters ?? {}) },
      modeAbortController.signal,
      progress,
    )
    if (modeAbortController.signal.aborted) {
      return
    }
    if (results.length <= SELECT_MODE_MAX) {
      refOptions.value = results
    }
  } catch (err) {
    if (modeAbortController.signal.aborted) {
      return
    }
    console.error("ClaimCardinality.selectMode", err)
  } finally {
    modeResolved.value = true
    progress.value -= 1
  }
})

// The single ClaimRefSelect of select mode; its defineExpose overrides
// ValidatedInput.revert with the async version so revertField can await it.
const claimRefSelectRef = useTemplateRef<{ revert: () => Promise<void> }>("claimRefSelectRef")

const { t, locale } = useI18n({ useScope: "global" })

// The field's instructions in the current language: longer form guidance (HTML
// paragraphs) from the field's configuration, shown once at the bottom of the
// whole field, after the value inputs' hints.
const instructions = computed(() => getFieldInstructions(props.field, locale.value))

// The hints and instructions combined into one HTML fragment: each hint becomes its
// own (escaped) paragraph, followed by the instructions' HTML. All paragraphs end up
// direct children of the single prose element rendering this, so the gap between any
// two paragraphs (and the zeroed first/last margins) is uniform. The instructions go
// through the internal-links transformation like any HTML claim (see ClaimValueHtml).
const hintsAndInstructionsHtml = useTransformedHtml(
  computed(() => slotHints.value.map((hint) => `<p>${escapeHtml(hint)}</p>`).join("") + instructions.value.map((instruction) => instruction.html).join("")),
)
const onInternalLinksClick = useInternalLinksClick()

// Classes of the prose element rendering the combined block, shared by its two
// render sites (a subgrid row of the repeated layout, and below the single-value
// and select layouts). The prose body colour is lightened like hints were and
// italics are forced.
const hintsAndInstructionsClasses = "prose prose-sm max-w-none min-w-0 italic prose-gray [--tw-prose-body:var(--color-neutral-500)]"

const saveChange = injectFn(saveChangeKey, (spec: SaveChangeSpec) => Promise.resolve({ id: "id" in spec ? spec.id : "" }))
const registerRemoteAdds = injectFn(registerRemoteAddsKey, () => {})
const unregisterRemoteAdds = injectFn(unregisterRemoteAddsKey, () => {})

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

// Restores focus after a user-driven operation removed a slot's claim while focus was
// inside the slot (see the cleared emit on ClaimInput): the control focus was on
// unmounts with the filled state, and a spliced slot is replaced by a fresh trailing
// instance which the component's own focus restoration cannot reach. Focus the same
// slot's input when the slot was kept (min-cardinality), else the last slot (the
// trailing empty replacement). Two ticks: the splice triggers the grow of the trailing
// slot in a post-flush watcher, and the new input mounts one render later.
function onSlotCleared(key: string): void {
  void (async () => {
    await nextTick()
    await nextTick()
    const input = slotInputs.get(key) ?? slotInputs.get(slots.value[slots.value.length - 1]?.key ?? "")
    input?.inputEl()?.focus()
  })()
}

function revertSlot(key: string): void {
  void (async () => {
    await slotInputs.get(key)?.revert()
    await cleanupParentIfEmpty()
  })()
}

// cleanupParentIfEmpty runs the enclosing slot's base cleanup once a revert has left
// this whole (sub)field without claims.
async function cleanupParentIfEmpty(): Promise<void> {
  if (!props.parentCleanup) {
    return
  }
  if (slots.value.some((slot) => slot.claim !== null)) {
    return
  }
  await props.parentCleanup()
}

// Clicking the sub-field header label focuses the field's first input, like a
// <label for>. mousedown + preventDefault (the @mousedown.prevent in the
// template) sends focus straight there instead of blurring to the body first.
function onLabelMousedown(): void {
  firstChildInputEl()?.focus()
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
  const min = props.field.minCardinality

  // Compact: drop empty slots that are neither needed nor being edited, so a
  // stray empty in the MIDDLE - one the user typed past, or a designated slot a
  // later value has since made redundant - does not linger between filled rows.
  // We keep every slot with content (a value or sub-claims), the currently focused
  // slot (never yank the user out of a row), the first (min - filled) empty slots
  // (designated to satisfy the min cardinality), and one trailing empty placeholder.
  // Everything else empty is dropped.
  const focused = typeof document !== "undefined" ? document.activeElement : null
  const filledCount = slots.value.reduce((count, slot) => count + (slotIsEmpty(slot) ? 0 : 1), 0)
  let needEmpty = Math.max(0, min - filledCount)
  let lastEmptyIdx = -1
  for (let i = slots.value.length - 1; i >= 0; i--) {
    if (slotIsEmpty(slots.value[i])) {
      lastEmptyIdx = i
      break
    }
  }
  const kept = slots.value.filter((slot, i) => {
    if (!slotIsEmpty(slot)) return true
    // A slot whose claim is still committed is never compacted away even when its local
    // state is empty (the user just cleared it and the removal commit is in flight):
    // dropping it would unmount the input mid-commit - Vue silently swallows emits from
    // unmounted components, losing the cleared report - and would desync the display
    // from the committed claim if the removal gets dropped. Such a slot leaves through
    // updateSlotClaim once its claim is actually removed.
    if (slot.claim !== null) return true
    const el = slotInputs.get(slot.key)?.mainEl?.()
    if (focused && el?.contains(focused)) return true
    if (needEmpty > 0) {
      needEmpty--
      return true
    }
    return i === lastEmptyIdx
  })
  if (kept.length !== slots.value.length) {
    slots.value = kept
  }

  // Grow: append a trailing empty after the last value (lastValueIdx), keep any
  // sub-claim-only content slot (lastContentIdx), and top up to at least
  // minCardinality slots so every designated (required) slot is visible up front
  // (a fresh 3..6 field shows three empty required inputs), capped at max.
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
  let desired = Math.max(lastContentIdx + 1, lastValueIdx + 2, min)
  if (desired > max) desired = max
  if (desired < 1) desired = Math.min(1, max)
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
// When the slot transitions from a committed claim to null, we normally drop the
// slot entirely - that matches the original FieldsFormField behaviour (removeRow
// on commit-empty) and lets the user clear a field by blanking it instead of
// leaving an empty-but-mounted row behind. The exception is a slot the field
// still needs to reach its min cardinality: dropping it would replace it with a
// fresh trailing empty that has lost the slot's "Required value." and touched
// state, so the requirement would silently stop showing. There we keep the same
// slot (now empty), so its input holds the required error surfaced on the
// clearing blur and stays flagged until refilled.
function updateSlotClaim(slotKey: string, claim: DeepReadonly<Claim> | null): void {
  const idx = slots.value.findIndex((s) => s.key === slotKey)
  if (idx < 0) return
  const slot = slots.value[idx]
  if (slot.claim !== null && claim === null) {
    const othersNonEmpty = slots.value.reduce((count, other, otherIdx) => count + (otherIdx !== idx && !slotIsEmpty(other) ? 1 : 0), 0)
    if (othersNonEmpty < props.field.minCardinality) {
      slot.claim = null
    } else {
      slots.value.splice(idx, 1)
    }
    // The removal may have left this whole (sub)field claim-less: ask the enclosing
    // slot to remove its lazily-created base claim. Triggered here, after the removal
    // committed, rather than from the enclosing slot's focusout cleanup, which runs off
    // the same focusout burst as the sub-claim's remove commit and so can still observe
    // the pre-remove state and bail.
    void cleanupParentIfEmpty()
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

// Remote changes may have added claims of this field which no slot represents yet
// (committed by another editor). Push slots for them; content updates and removals of
// represented claims are handled by each slot's own ClaimInput. Runs after the render
// flush (see loadChanges in DocumentEdit), so props.modelValue already reflects the
// committed doc, including a resynced parent claim for sub-field cardinalities. Own adds
// can never look remote here: their slots are set at commit time under the same final id
// the doc echoes.
function onRemoteAdds(claimIds: ReadonlySet<string>): boolean {
  // In select mode the ClaimRefSelect adopts remote changes itself, through its
  // modelValue watcher.
  if (refOptions.value !== null) {
    return false
  }
  const represented = new Set<string>()
  for (const slot of slots.value) {
    if (slot.claim) {
      represented.add(slot.claim.id)
    }
  }
  const baselineById = new Map<string, DeepReadonly<Claim>>()
  for (const b of slotsCheckpoint.value) {
    baselineById.set(b.id, b)
  }
  let added = false
  for (const claim of props.modelValue) {
    if (!claimIds.has(claim.id) || represented.has(claim.id)) {
      continue
    }
    // Fill an existing empty placeholder slot when there is one - locally an add fills
    // the slot the user typed into, so a remote add reuses the placeholder the same way.
    // Pushing next to it instead would leave both mounted (the compaction always keeps
    // one empty), rendering a second input inside the field (visible e.g. on 0..1
    // fields).
    const empty = slots.value.find((slot) => slot.claim === null && slotIsEmpty(slot))
    if (empty) {
      empty.claim = claim
      empty.baseline = baselineById.get(claim.id) ?? null
    } else {
      slots.value.push({ key: nextSlotKey(), claim, baseline: baselineById.get(claim.id) ?? null })
    }
    added = true
  }
  if (added) {
    reconcileSlots()
  }
  return added
}
onMounted(() => registerRemoteAdds(onRemoteAdds))
onBeforeUnmount(() => unregisterRemoteAdds(onRemoteAdds))

// Designated slots: the min slots that satisfy, or are still needed to satisfy,
// the field's min cardinality. We pick min slots, preferring the filled ones
// (they contribute) and topping up with the earliest empty ones (still needed).
// A designated slot passes required=true to its input, which (a) shows the
// "required" badge and (b) lets the input surface its own "Required value." when
// the user leaves it empty - there is no field-level trigger, each empty slot
// reds on its own blur. The designation shifts live: filling a non-designated
// slot while a designated one is empty moves the designation onto the filled
// slot and off the empty one. Empty for non-required fields (min <= 0) and while
// locked (the surrounding form is in a noop state).
const designated = computed<boolean[]>(() => {
  if (locked.value) return slots.value.map(() => false)
  const min = props.field.minCardinality
  if (min <= 0) return slots.value.map(() => false)
  let nonEmptyCount = 0
  for (const slot of slots.value) {
    if (!slotIsEmpty(slot)) nonEmptyCount++
  }
  const needEmpty = Math.max(0, min - nonEmptyCount)
  let filledSeen = 0
  let emptySeen = 0
  return slots.value.map((slot) => {
    if (!slotIsEmpty(slot)) {
      filledSeen++
      return filledSeen <= min
    }
    emptySeen++
    return emptySeen <= needEmpty
  })
})

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
// In select mode there are no slots and the ClaimRefSelect tracks its own
// baseline diff, so only the registry's dirty counts (the slot diff would
// falsely flag every baseline claim as removed).
const isDirty = computed<boolean>(() => (refOptions.value !== null ? anyChildDirty.value : slotsDirtyByDiff.value || anyChildDirty.value))

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
  // On Save the cascade validates every child input; each empty designated slot
  // then surfaces its own "Required value." (its required prop is already true),
  // so the min-cardinality violation reaches the aggregate without a field-level
  // error of our own.
  validate: async (signal, options) => {
    await validateChildAll(signal, options)
  },
  reset: () => {
    resetChildAll()
    if (refOptions.value !== null) {
      return
    }
    // Rebuild slots from current modelValue + one trailing empty.
    slots.value = []
    const baselineById = new Map<string, DeepReadonly<Claim>>()
    for (const b of slotsCheckpoint.value) baselineById.set(b.id, b)
    for (const claim of props.modelValue) {
      slots.value.push({ key: nextSlotKey(), claim, baseline: baselineById.get(claim.id) ?? null })
    }
    reconcileSlots()
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
  errors: allErrors(childInputs),
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
})

// revertField runs the field-level Revert: re-add removed baseline
// claims, then revert every claim-holding slot through its own input
// (which removes session-added claims, sets diverged values back, and
// cascades into sub-claims). We classify slots by slot.baseline, not
// by claim id, so a slot resurrected on a previous revert click (its
// claim has a fresh content-addressed id) is still correctly recognised
// as representing its original baseline.
async function revertField(): Promise<void> {
  // In select mode the single ClaimRefSelect owns all of the field's claims and
  // reconciles them back to its checkpoint itself.
  if (refOptions.value !== null) {
    await claimRefSelectRef.value?.revert()
    return
  }

  // Snapshot which baselines are already represented before any of our
  // mutations. The new (resurrected) slots we push below get
  // baseline:set so the next-click revert correctly classifies them.
  const representedBaselineIds = new Set<string>()
  for (const slot of slots.value) {
    if (slot.claim && slot.baseline) representedBaselineIds.add(slot.baseline.id)
  }

  // 1) Re-add baseline claims that no current slot represents. Resolve the (possibly
  // lazily-created) parent FIRST so it is committed before this claim is queued: the
  // add's under has to reference a committed claim id. A re-add which gets dropped
  // (e.g. its parent claim was removed concurrently) is skipped.
  for (const baseline of slotsCheckpoint.value) {
    if (representedBaselineIds.has(baseline.id)) continue
    const under = props.parentClaimId ? await props.parentClaimId() : undefined
    const values = getClaimValues(baseline)
    const patch = makePatchForField(props.field, values)
    let result: SaveChangeResult
    try {
      result = await saveChange(under === undefined ? { type: "add", patch } : { type: "add", patch, under })
    } catch (err) {
      if (err instanceof ChangeDroppedError) {
        continue
      }
      throw err
    }
    const newClaim = claimPatchFrom(patch).New(result.id)
    slots.value.push({ key: nextSlotKey(), claim: newClaim, baseline })
  }

  // 2) Revert every claim-holding slot through its own input. Each ClaimInput's
  // revertField sees its baseline (via initialClaimForSlot -> slot.baseline) and
  // computes the Remove (session-added slot) / Set (diverged values) / no-op
  // accordingly, serialized with the slot's other operations so an in-flight commit
  // cannot race it. Iterate over a snapshot: a removed slot's update splices
  // slots.value while we go through it.
  for (const slot of [...slots.value]) {
    if (!slot.claim) continue
    const input = slotInputs.get(slot.key)
    if (!input) continue
    await input.revert()
  }

  // 3) Cleanup: drop leftover empty (claim-less) slots, then let
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

  // 4) When the revert left this whole (sub)field without claims, ask the enclosing
  // slot to remove its lazily-created base claim too.
  await cleanupParentIfEmpty()
}

onBeforeUnmount(() => {
  slotInputs.clear()
})
</script>

<template>
  <!--
    Renders one ClaimInput per slot. Each slot (a repeated entry, or the single
    entry of a non-repeated field) is its own "group" with a left rail, so
    repeated entries read as separate blocks and nesting shows via the rails'
    indentation. Repeated fields number each slot in a min-content count column.

    The rail mirrors the input's ring, resolving by priority: error (red) when it
    contains an invalid input, else primary-500 (blue) when the slot or anything
    inside it is focused, else primary-300 (the revert button's colour) when the
    slot is changed, else neutral. The invalid/focus overrides are CSS variants
    with higher specificity than the changed/neutral base, so invalid > focus >
    changed. focus-within / has(aria-invalid) / the aggregated dirty all bubble
    up, so the colour forms a path down the nested rails to the field.

    The sub-field header (the field label + whole-field badge, shown only via
    showHeader) sits left-aligned above the slots with mb-4 and the same pl-4 as
    the slots, so it aligns with their content (the count); the rail bar is
    absolutely positioned and does not shift that. Top-level fields render no
    header (their label is in FieldsFormField's left cell).
  -->
  <div ref="rootRef" class="flex min-w-0 grow flex-col">
    <div v-if="showHeader" ref="headerRef" class="mb-4 flex flex-row flex-wrap items-center gap-1 pl-4">
      <span :id="labelId" class="cursor-pointer leading-none font-medium text-gray-700" @mousedown.prevent="onLabelMousedown"
        ><DocumentRefInline :id="field.propertyId" :link="false"
      /></span>
      <InputBadges :required="field.minCardinality > 0" :multiple="field.maxCardinality > 1" :changed="isDirty" @revert="onHeaderRevert" />
    </div>
    <!--
      Select mode: all of the field's claims are managed by one ClaimRefSelect (no
      slots and no per-entry counts; the field-level badge and revert cover the
      whole selection). The rail wrapper matches the slots' one: dirty comes from
      the registry (the ClaimRefSelect is its only registered input here) and the
      invalid/focus overrides bubble up via CSS.
    -->
    <div
      v-if="refOptions !== null"
      class="relative pl-4 before:absolute before:inset-y-0 before:left-0 before:w-1 before:rounded-sm before:content-[''] not-has-[[aria-invalid=true]]:focus-within:before:bg-primary-500 has-[[aria-invalid=true]]:before:bg-error-600"
      :class="anyChildDirty ? 'before:bg-primary-300' : 'before:bg-neutral-300'"
    >
      <ClaimRefSelect
        ref="claimRefSelectRef"
        :model-value="modelValue"
        :initial-claims="initialClaims"
        :field="field"
        :options="refOptions"
        :multiple="isRepeated"
        :parent-claim-id="parentClaimId"
        :parent-cleanup="parentCleanup"
        :invalid="invalid"
        :readonly="readonly"
        :label-id="labelId"
      />
    </div>
    <!--
      Repeated layout: one shared grid whose first (min-content) column holds the
      entries' counts with their per-entry revert buttons; the label row, every
      entry, and the hint/instruction row are subgrid rows spanning both columns,
      so the count column is a single track sized by its widest cell and the
      second column stays aligned across all rows even when counts grow to
      multiple digits. Every row carries the same pl-4 (the rails' content
      offset), keeping the tracks' edge insets uniform.
    -->
    <div v-else-if="modeResolved && isRepeated" class="grid grid-cols-[min-content_auto] gap-x-4">
      <!--
        Hoisted label row of a repeated field whose input has labeled columns
        (amount/precision, time/precision): shown once above all entries, outside
        their rails, with an empty cell standing in for the count column. The label
        grid uses the same template as the entries' InputField grids, so the labels
        align with the input columns below. Interval entries keep their own label
        rows instead (the per-bound revert badges live there). A press on a label
        focuses the first entry's control in that column, like InputField's own
        labels. The mb-1 matches the label-to-control spacing inside InputField,
        tighter than the entry gap.
      -->
      <div v-if="!isInterval && hasLabelRow" class="col-span-2 mb-1 grid grid-cols-subgrid items-start pl-4">
        <div></div>
        <div class="grid items-start justify-start gap-x-4" :style="{ gridTemplateColumns: labelsGridTemplateColumns }">
          <span v-for="(col, i) in slotColumns" :key="i" class="cursor-pointer leading-none" @mousedown.prevent="onColumnLabelMousedown(col)">{{ col.label }}</span>
        </div>
      </div>
      <template v-for="(slot, idx) in slots" :key="slot.key">
        <div
          class="relative col-span-2 grid grid-cols-subgrid items-start pl-4 before:absolute before:inset-y-0 before:left-0 before:w-1 before:rounded-sm before:content-[''] not-has-[[aria-invalid=true]]:focus-within:before:bg-primary-500 has-[[aria-invalid=true]]:before:bg-error-600"
          :class="[slotDirty(slot.key) ? 'before:bg-primary-300' : 'before:bg-neutral-300', idx > 0 ? entryGapClass : '']"
        >
          <!--
          The count sits at the top, aligned with the entry's first line (the input
          row, or the label row of an interval entry), with the per-entry revert
          icon under it. The button is a square the same height as the "changed"
          badge, rendered unconditionally (just visibility:hidden when the entry is
          unchanged) so it always reserves the count column's width and the input
          does not shift when it appears. The mousedown is prevented so clicking it
          does not blur the value input first (which would commit before revert).
        -->
          <div class="flex flex-col items-start gap-y-1">
            <div class="pt-0.5 leading-none font-medium text-gray-700">{{ idx + 1 }}.</div>
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
            :invalid="invalid"
            :required="designated[idx]"
            :is-first="idx === 0"
            :readonly="readonly"
            :label-id="labelId"
            :hide-labels="!isInterval"
            @update:model-value="(claim) => updateSlotClaim(slot.key, claim)"
            @cleared="onSlotCleared(slot.key)"
          />
        </div>
      </template>
      <!-- The hints/instructions block (see below), as a subgrid row of the repeated layout. -->
      <div v-if="slotHints.length > 0 || instructions.length > 0" class="col-span-2 mt-1 grid grid-cols-subgrid items-start pl-4">
        <div></div>
        <!-- eslint-disable vue/no-v-html -->
        <div :class="hintsAndInstructionsClasses" @click="onInternalLinksClick" v-html="hintsAndInstructionsHtml"></div>
        <!-- eslint-enable vue/no-v-html -->
      </div>
    </div>
    <template v-else-if="modeResolved">
      <div
        v-for="(slot, idx) in slots"
        :key="slot.key"
        class="relative pl-4 before:absolute before:inset-y-0 before:left-0 before:w-1 before:rounded-sm before:content-[''] not-has-[[aria-invalid=true]]:focus-within:before:bg-primary-500 has-[[aria-invalid=true]]:before:bg-error-600"
        :class="slotDirty(slot.key) ? 'before:bg-primary-300' : 'before:bg-neutral-300'"
      >
        <ClaimInput
          :ref="(el) => setSlotRef(slot.key, el)"
          :model-value="slot.claim"
          :initial-claim="initialClaimForSlot(slot)"
          :field="field"
          :parent-claim-id="parentClaimId"
          :invalid="invalid"
          :required="designated[idx]"
          :is-first="idx === 0"
          :readonly="readonly"
          :label-id="labelId"
          @update:model-value="(claim) => updateSlotClaim(slot.key, claim)"
          @cleared="onSlotCleared(slot.key)"
        />
      </div>
    </template>
    <!--
      The field's hints and instructions, combined into one prose block shown once at
      the bottom of the whole field, outside the rails (the inputs' own hints are
      always suppressed by FieldsFormRow): each hint is its own paragraph, followed by
      the instructions' paragraphs, so the gap between any two paragraphs is uniform.
      This is the single-value and select layouts' render site; the repeated layout
      renders the same block as a subgrid row above, aligned with the entries' inputs.
      The mt-1 matches the control-to-hint spacing previously inside InputField.
    -->
    <div v-if="(slotHints.length > 0 || instructions.length > 0) && (refOptions !== null || !isRepeated)" class="mt-1 pl-4">
      <!-- eslint-disable vue/no-v-html -->
      <div :class="hintsAndInstructionsClasses" @click="onInternalLinksClick" v-html="hintsAndInstructionsHtml"></div>
      <!-- eslint-enable vue/no-v-html -->
    </div>
  </div>
</template>
