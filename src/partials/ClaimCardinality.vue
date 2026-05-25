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
import type { ValidatedInput } from "@/types"

import { computed, onBeforeUnmount, ref, shallowReactive, useTemplateRef, watch } from "vue"

import { AddClaimChange, claimPatchFrom, RemoveClaimChange } from "@/document"
import { getClaimValues, makePatchForField } from "@/fields"
import ClaimInput from "@/partials/ClaimInput.vue"
import { useLocked } from "@/progress"
import { allErrors, useRegisterForValidation, useValidationRegistry } from "@/validation"
import { Identifier } from "@tozd/identifier"

import { inject as injectFn } from "vue"

import { getNextChangeNumberKey, saveChangeKey } from "@/fields"

const props = withDefaults(
  defineProps<{
    modelValue: DeepReadonly<readonly Claim[]>
    initialClaims: DeepReadonly<readonly Claim[]>
    field: DeepReadonly<FieldData>
    parentClaimId?: () => Promise<string>
    invalid?: boolean
    session: string
    base: readonly string[]
  }>(),
  {
    parentClaimId: undefined,
    invalid: false,
  },
)

let fallbackNum = 1
const getNextChangeNumber = injectFn(getNextChangeNumberKey, () => fallbackNum++)
const saveChange = injectFn(saveChangeKey, () => Promise.resolve())

const locked = useLocked()

// Slots: the user-facing list of editable rows. Each has a stable key
// so Vue's v-for can re-anchor across claim renames; claim is the
// committed claim (or null for an as-yet-unfilled trailing slot).
type Slot = { key: string; claim: DeepReadonly<Claim> | null }

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

watch(
  () => props.initialClaims,
  (v) => {
    slotsCheckpoint.value = v
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
  firstEl: firstChildEl,
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
}

function setSlotRef(key: string, el: unknown): void {
  if (el == null) {
    slotInputs.delete(key)
    return
  }
  slotInputs.set(key, el as ExposedClaimInput)
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
  // Find the last "filled" slot index (claim present OR ClaimInput
  // reports non-empty local state).
  let lastFilledIdx = -1
  for (let i = slots.value.length - 1; i >= 0; i--) {
    if (!slotIsEmpty(slots.value[i])) {
      lastFilledIdx = i
      break
    }
  }
  let desired = lastFilledIdx + 2 // filled slots + one trailing empty
  if (desired > max) desired = max
  if (desired < 1) desired = Math.min(1, max)

  // Shrink: drop empty trailing slots beyond one. Skip the active-focus
  // slot so we never yank the user out of the row they are currently in.
  while (slots.value.length > desired) {
    const last = slots.value[slots.value.length - 1]
    if (!slotIsEmpty(last)) break // safety: don't drop a filled tail
    const lastEl = slotInputs.get(last.key)?.el?.()
    const focused = typeof document !== "undefined" ? document.activeElement : null
    if (focused && lastEl?.contains(focused)) break // keep focused trailing
    slots.value.pop()
  }

  // Grow: append empty trailing if we're under desired.
  while (slots.value.length < desired) {
    slots.value.push({ key: nextSlotKey(), claim: null })
  }
}

// Initial population: do it eagerly on setup so the first render already
// has the right slots.
for (const claim of props.modelValue) {
  slots.value.push({ key: nextSlotKey(), claim })
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

// initialClaimForSlot finds the baseline claim that corresponds to a
// given slot. Matches by current slot.claim.id against initialClaims;
// new (session-added) slots see null.
function initialClaimForSlot(slot: Slot): DeepReadonly<Claim> | null {
  if (!slot.claim) return null
  for (const baseline of slotsCheckpoint.value) {
    if (baseline.id === slot.claim.id) return baseline
  }
  return null
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
    ourErrors.push({ code: "required", el: slotInputs.get(slot.key)?.el?.() ?? undefined })
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

// slotsDirtyByDiff: the multiset of claim ids in current slots differs
// from the baseline. This catches session-level adds and removes.
const slotsDirtyByDiff = computed<boolean>(() => {
  const baselineIds = new Set(slotsCheckpoint.value.map((c) => c.id))
  const currentIds = new Set<string>()
  for (const slot of slots.value) {
    if (slot.claim) currentIds.add(slot.claim.id)
  }
  if (currentIds.size !== baselineIds.size) return true
  for (const id of currentIds) {
    if (!baselineIds.has(id)) return true
  }
  return false
})

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
    for (const claim of props.modelValue) {
      slots.value.push({ key: nextSlotKey(), claim })
    }
    reconcileSlots()
    triggered.value = false
  },
  revert: () => {
    void revertField()
  },
  el: () => rootRef.value ?? firstChildEl(),
  isDirty: computed(() => slotsDirtyByDiff.value || anyChildDirty.value),
  isEmpty: allChildEmpty,
  errors: computed(() => {
    const ourErrors = triggered.value ? missing.value.ourErrors : []
    return [...ourErrors, ...allErrors(childInputs).value]
  }),
  checkpoint: () => {
    // Move the baseline forward to the current claims (mirroring what
    // <DocumentEdit> does after Save). Cascade child checkpoints so each
    // <ClaimInput> re-anchors too.
    slotsCheckpoint.value = props.modelValue
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
// claims, remove session-added claims, then cascade revert into each
// surviving slot's ClaimInput.
async function revertField(): Promise<void> {
  const currentById = new Map<string, DeepReadonly<Claim>>()
  for (const slot of slots.value) {
    if (slot.claim) currentById.set(slot.claim.id, slot.claim)
  }
  const baselineById = new Map<string, DeepReadonly<Claim>>()
  for (const baseline of slotsCheckpoint.value) {
    baselineById.set(baseline.id, baseline)
  }

  // 1) Re-add claims that were removed during the session.
  for (const baseline of slotsCheckpoint.value) {
    if (currentById.has(baseline.id)) continue
    const num = getNextChangeNumber()
    const changeBase = [...props.base, "SESSION", props.session, String(num)]
    const newId = (await Identifier.from(...changeBase)).toString()
    const values = getClaimValues(baseline)
    const patch = makePatchForField(props.field as FieldData, values)
    const addChange = new AddClaimChange({ id: newId, base: changeBase, patch })
    if (props.parentClaimId) {
      addChange.under = await props.parentClaimId()
    }
    await saveChange(addChange, num)
    const newClaim = claimPatchFrom(patch).New(newId)
    slots.value.push({ key: nextSlotKey(), claim: newClaim })
  }

  // 2) Remove session-added claims.
  for (const [id, current] of currentById) {
    if (baselineById.has(id)) continue
    const num = getNextChangeNumber()
    await saveChange(new RemoveClaimChange({ id: current.id }), num)
    const idx = slots.value.findIndex((s) => s.claim?.id === id)
    if (idx >= 0) {
      slots.value.splice(idx, 1)
    }
  }

  // 3) Cascade revert into surviving slots. ClaimInput exposes the async
  // revert via defineExpose so we await each one. The cascade fans out
  // serially here for simplicity (parallel awaits could race on
  // saveChange's change-number sequencing).
  for (const slot of slots.value) {
    if (!slot.claim) continue
    const input = slotInputs.get(slot.key)
    if (!input) continue
    await input.revert()
  }

  // 4) Make sure we still have a trailing empty slot.
  reconcileSlots()
}

onBeforeUnmount(() => {
  slotInputs.clear()
})
</script>

<template>
  <div ref="rootRef" class="flex min-w-0 grow flex-col gap-y-2" @focusout="onFocusOut">
    <ClaimInput
      v-for="(slot, idx) in slots"
      :key="slot.key"
      :ref="(el) => setSlotRef(slot.key, el)"
      :model-value="slot.claim"
      :initial-claim="initialClaimForSlot(slot)"
      :field="field"
      :parent-claim-id="parentClaimId"
      :invalid="invalid || (triggered && missing.flags[idx]) || false"
      :required="triggered && missing.flags[idx]"
      :session="session"
      :base="base"
      @update:model-value="(claim) => updateSlotClaim(slot.key, claim)"
    />
  </div>
</template>
