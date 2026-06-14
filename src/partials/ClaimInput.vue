<!--
ClaimInput edits a single Claim (or null for an empty slot waiting for the
user to start typing). It owns the local FieldEntryValue mirror used by
FieldsFormRow, runs the focusout commit path that turns user edits into
Add / Set / Remove changes through inject(saveChange), and recursively
renders one ClaimCardinality per sub-field.

For HAS the value input is omitted; the slot's emptiness is driven entirely
by whether the sub-claims are empty. ensureClaimId lazily issues the empty
HAS AddClaimChange on the first sub-claim add.

initialClaim is the pre-session baseline used to seed the revert/checkpoint
machinery. It is watched so a parent that re-anchors (mid-session reload,
remount with a different doc/version) re-syncs the checkpoint without
having to remount this component.
-->

<script setup lang="ts">
import type { DeepReadonly, ShallowUnwrapRef } from "vue"

import type { Claim, ClaimTypes } from "@/document"
import type { FieldData, FieldEntryValue } from "@/fields"
import type { ValidatedInput } from "@/types"

import { computed, inject, onBeforeUnmount, onMounted, ref, useTemplateRef, watch } from "vue"

import CheckBox from "@/components/CheckBox.vue"
import { VT_HAS, VT_NONE, VT_UNKNOWN } from "@/core"
import { AddClaimChange, claimPatchFrom, getClaimsOfTypeWithConfidence, RemoveClaimChange, SetClaimChange } from "@/document"
import {
  emptyFieldEntryValue,
  equalFieldEntryValue,
  fieldKey,
  fieldLabelCellKey,
  getClaimValues,
  getNextChangeNumberKey,
  makePatchForField,
  registerForFlushKey,
  saveChangeKey,
  unregisterForFlushKey,
  valueTypeToClaimType,
} from "@/fields"
import ClaimCardinality from "@/partials/ClaimCardinality.vue"
import DocumentRefInline from "@/partials/DocumentRefInline.vue"
import FieldsFormRow from "@/partials/FieldsFormRow.vue"
import InputErrors from "@/partials/InputErrors.vue"
import { allErrors, useRegisterForValidation, useValidationRegistry } from "@/validation"
import { Identifier } from "@tozd/identifier"

const props = withDefaults(
  defineProps<{
    modelValue: DeepReadonly<Claim> | null
    initialClaim: DeepReadonly<Claim> | null
    field: DeepReadonly<FieldData>
    // parentClaimId is a callback that returns the parent's claim id
    // (creating it lazily if the parent is a HAS slot whose claim has
    // not been committed yet). Undefined for top-level claims.
    parentClaimId?: () => Promise<string>
    invalid?: boolean
    // Drives the inner input's own "Required value." text via its
    // validator. Set by ClaimCardinality on slots that the field's
    // missing-min check has flagged. Toggling this re-runs FieldsFormRow's
    // inner validators (see the watch in FieldsFormRow), which surfaces
    // / clears the message as the slot's required state changes.
    required?: boolean
    session: string
    base: readonly string[]
  }>(),
  {
    parentClaimId: undefined,
    invalid: false,
    required: false,
  },
)

const emit = defineEmits<{
  "update:modelValue": [Claim | null]
}>()

let fallbackNum = 1
const getNextChangeNumber = inject(getNextChangeNumberKey, () => fallbackNum++)
const saveChange = inject(saveChangeKey, () => Promise.resolve())
const registerForFlush = inject(registerForFlushKey, () => {})
const unregisterForFlush = inject(unregisterForFlushKey, () => {})
// Lets the slot's focusout detect focus moving to a control inside the
// field's label cell (i.e. the field-level Revert button). When that's
// the case we skip the per-slot commit so it does not race the Revert
// click. Null when ClaimInput is mounted outside FieldsFormField (e.g.
// in tests or as a recursive sub-slot - recursive sub-slots' Revert
// button still lives on the topmost FieldsFormField anyway).
const getFieldLabelCell = inject(fieldLabelCellKey, () => null)

const isHas = computed(() => props.field.valueType === VT_HAS)
const isPresenceOnly = computed(() => isHas.value || props.field.valueType === VT_NONE || props.field.valueType === VT_UNKNOWN)

// Local raw-value state. Hydrated from modelValue at setup, then owned by
// the user; subsequent modelValue prop changes (echoes of our own commits,
// or rare external doc-sync updates) update local only when we are not
// dirty so a mid-typing external sync does not clobber the user's work.
const local = ref<FieldEntryValue>(props.modelValue ? getClaimValues(props.modelValue) : emptyFieldEntryValue())

// checkpointClaim is the revert target. Seeded from initialClaim; watched
// so a parent re-anchor moves it without remount. Updated by checkpoint()
// to the current modelValue (called from DocumentEdit's checkpointAll).
const checkpointClaim = ref<DeepReadonly<Claim> | null>(props.initialClaim)
const checkpointEntry = ref<FieldEntryValue>(props.initialClaim ? getClaimValues(props.initialClaim) : emptyFieldEntryValue())

watch(
  () => props.initialClaim,
  (v) => {
    checkpointClaim.value = v
    checkpointEntry.value = v ? getClaimValues(v) : emptyFieldEntryValue()
  },
  { flush: "sync" },
)

// Sub-claim extraction: for each sub-field, pull the matching claims out
// of the current modelValue (and initialClaim) so the sub-ClaimCardinality
// has the right slice.
function extractSubClaims(claim: DeepReadonly<Claim> | null, subField: DeepReadonly<FieldData>): readonly DeepReadonly<Claim>[] {
  if (!claim || !claim.sub) return []
  const t = valueTypeToClaimType(subField.valueType)
  return getClaimsOfTypeWithConfidence(claim.sub, t, subField.propertyId)
}

// Whether the sub-ClaimCardinality should be rendered for this slot.
// For HAS with sub-fields we always render (the user only edits HAS
// *through* its sub-claims, which lazy-create the HAS itself). For other
// types we only render after the parent claim has been committed - a
// sub-claim requires a parent id, and ensureClaimId for non-HAS just
// returns the existing id rather than creating one.
const showSubFields = computed(() => {
  if (props.field.subFields.length === 0) return false
  if (isHas.value) return true
  return props.modelValue !== null
})

// Whether to render the presence-toggle checkbox. NONE / UNKNOWN never
// have a value of their own, and HAS without sub-fields also degenerates
// to a presence-only toggle (no sub-form is shown). HAS *with* sub-fields
// is checkbox-less: the user creates the HAS claim implicitly by adding
// the first sub-claim (lazy via ensureClaimId).
const showCheckbox = computed(() => {
  if (props.field.valueType === VT_NONE || props.field.valueType === VT_UNKNOWN) return true
  if (props.field.valueType === VT_HAS && props.field.subFields.length === 0) return true
  return false
})

// hasAnySubClaims is true when any sub-field has at least one claim
// attached to this claim's sub. Used by isEmpty (and the commit logic's
// "don't auto-remove a parent that still has sub-claims" branch).
const hasAnySubClaims = computed(() => {
  if (!props.modelValue?.sub) return false
  for (const subField of props.field.subFields) {
    if (extractSubClaims(props.modelValue, subField).length > 0) return true
  }
  return false
})

// localIsEmpty: every relevant local raw field is at its default. For HAS
// (and NONE/UNKNOWN) presence-only types the local raw values are always
// at defaults, so the slot's emptiness is determined by sub-claims alone.
const localIsEmpty = computed(() => {
  if (isPresenceOnly.value) return true
  return equalFieldEntryValue(local.value, emptyFieldEntryValue())
})

// rootRef is the DOM identity used by the sub-validation registry's
// self-registration so the outer registry (ClaimCardinality) can find us.
const rootRef = useTemplateRef<HTMLElement>("rootRef")

// formRowRef points at the FieldsFormRow ValidatedInput. We call its
// validate directly (rather than the broader validateChildAll on the
// sub-registry) when committing this slot's value, so a sub-cardinality
// "Required value." from an empty required sub-field does NOT block the
// parent commit. Sub-cardinality validation is reserved for Save time
// (via the outer revertAll/validateAll cascade).
const formRowRef = useTemplateRef<ShallowUnwrapRef<ValidatedInput>>("formRowRef")

// Sub-registry: FieldsFormRow and each sub-ClaimCardinality register here.
// Their dirty/empty bubbles up via anyChildDirty / allChildEmpty into the
// composite ValidatedInput exposed below.
let forwardInteraction: (() => void) | null = null
const {
  validateAll: validateChildAll,
  resetAll: resetChildAll,
  revertAll: revertChildAll,
  checkpointAll: checkpointChildAll,
  anyDirty: anyChildDirty,
  allEmpty: allChildEmpty,
  inputs: childInputs,
  firstEl,
} = useValidationRegistry(() => {
  forwardInteraction?.()
})

// Whether this slot's local raw values differ from the checkpoint values.
const localDirty = computed(() => !equalFieldEntryValue(local.value, checkpointEntry.value))

// Whether the slot's claim presence (committed vs not) differs from the
// checkpoint's. This catches session-added (no baseline + committed
// claim) and session-removed (had baseline + now nothing). We compare
// by HAS-A-CLAIM rather than by claim id because a resurrected slot has
// a fresh content-addressed id (a re-Add cannot reuse the original id),
// yet conceptually it represents the same baseline claim - comparing
// by id would falsely flag it as dirty and leave "Changed" lit after
// the user reverts.
const identityDirty = computed(() => {
  const hasCurrent = props.modelValue !== null
  const hasBaseline = checkpointClaim.value !== null
  return hasCurrent !== hasBaseline
})

// isEmpty: presence-only slots are empty iff there are no sub-claims; non-
// HAS slots also consider local raw emptiness.
const isEmpty = computed<boolean>(() => {
  if (isPresenceOnly.value) {
    // No own value; emptiness is sub-claim emptiness AND no committed claim.
    if (props.modelValue !== null) return false
    return allChildEmpty.value
  }
  if (!localIsEmpty.value) return false
  if (props.field.subFields.length === 0) return true
  // We treat any non-empty sub-claim as "this slot has content".
  return allChildEmpty.value
})

// Compose the ValidatedInput exposed to the outer registry.
// The framework's revertAll() cascade is fire-and-forget (revertAll is
// synchronous), so we wrap the async revertField in a void-discarding
// thunk here. The async version is exposed via defineExpose below for
// direct callers (FieldsFormField's revert button).
const validatedInput: ValidatedInput = {
  validate: validateChildAll,
  reset: () => {
    local.value = emptyFieldEntryValue()
    resetChildAll()
  },
  revert: () => {
    void revertField()
  },
  el: () => rootRef.value ?? firstEl(),
  isDirty: computed(() => localDirty.value || identityDirty.value || anyChildDirty.value),
  isEmpty,
  errors: allErrors(childInputs),
  checkpoint: () => {
    checkpointClaim.value = props.modelValue
    checkpointEntry.value = props.modelValue ? getClaimValues(props.modelValue) : emptyFieldEntryValue()
    checkpointChildAll()
  },
}

const { onInteraction: notifyOuter } = useRegisterForValidation(validatedInput)
forwardInteraction = notifyOuter

// ensureClaimId returns this claim's id, lazily creating an empty HAS
// claim if needed. Sub-ClaimCardinality passes this down to its slots so
// their AddClaimChange knows what to set under to.
async function ensureClaimId(): Promise<string> {
  if (props.modelValue !== null) {
    return props.modelValue.id
  }
  if (!isHas.value) {
    throw new Error("ensureClaimId called with no committed claim on a non-HAS slot")
  }
  // Lazy-create the HAS claim with an empty body.
  const num = getNextChangeNumber()
  const changeBase = [...props.base, "SESSION", props.session, String(num)]
  const newId = (await Identifier.from(...changeBase)).toString()
  const patch = makePatchForField(props.field, emptyFieldEntryValue())
  const addChange = new AddClaimChange({ id: newId, base: changeBase, patch })
  if (props.parentClaimId) {
    addChange.under = await props.parentClaimId()
  }
  await saveChange(addChange, num)
  const newClaim = claimPatchFrom(patch).New(newId)
  emit("update:modelValue", newClaim)
  return newId
}

// commitRow runs Add / Set / Remove for the value side of this claim.
// Sub-claims are handled by the nested ClaimCardinality instances and do
// not go through this path.
async function commit(): Promise<void> {
  if (isPresenceOnly.value) return
  const currentClaim = props.modelValue
  // Empty path: either remove (no sub-claims) or no-op (sub-claims keep
  // the parent alive). Skip validation here on purpose. The user is
  // clearing the input, and a sub-cardinality that would normally fire a
  // "Required value." for an empty sub-field must NOT block the remove,
  // otherwise the row gets stuck and stays red.
  if (localIsEmpty.value) {
    if (currentClaim && !hasAnySubClaims.value) {
      // Remove the claim. Empty value and nothing under it.
      const num = getNextChangeNumber()
      await saveChange(new RemoveClaimChange({ id: currentClaim.id }), num)
      emit("update:modelValue", null)
    }
    // If the claim exists with sub-claims, keep it (user can revert).
    // If no claim and empty, nothing to do.
    return
  }
  // Non-empty path: Add or Set. Validate the row's inner inputs first so
  // an invalid value (e.g. "htt" for an IRI) stays in the form
  // uncommitted. Sub-cardinality validation is deliberately excluded -
  // see formRowRef definition for why.
  if (formRowRef.value) {
    await formRowRef.value.validate()
    if (formRowRef.value.errors.length > 0) return
  }
  // Local has values. Build the patch.
  const patch = makePatchForField(props.field, local.value)
  if (currentClaim) {
    // Update existing claim. Only post if values actually changed.
    if (equalFieldEntryValue(local.value, getClaimValues(currentClaim))) {
      // Nothing to do.
      return
    }
    const num = getNextChangeNumber()
    await saveChange(new SetClaimChange({ id: currentClaim.id, patch }), num)
    // Reconstruct the claim with the new values so the parent sees the
    // updated state immediately, without waiting for the next doc sync.
    const updated = claimPatchFrom(patch).New(currentClaim.id)
    if (currentClaim.sub) {
      // Preserve sub-claims through the optimistic update. The
      // DeepReadonly is a type-only concern; at runtime ClaimTypes is
      // the same object.
      updated.sub = currentClaim.sub as unknown as ClaimTypes
    }
    emit("update:modelValue", updated)
    return
  }
  // No claim yet. Add.
  const num = getNextChangeNumber()
  const changeBase = [...props.base, "SESSION", props.session, String(num)]
  const newId = (await Identifier.from(...changeBase)).toString()
  const addChange = new AddClaimChange({ id: newId, base: changeBase, patch })
  if (props.parentClaimId) {
    addChange.under = await props.parentClaimId()
  }
  await saveChange(addChange, num)
  const newClaim = claimPatchFrom(patch).New(newId)
  emit("update:modelValue", newClaim)
}

async function onFocusOut(event: FocusEvent): Promise<void> {
  const target = event.currentTarget as Node | null
  const next = event.relatedTarget as Node | null
  // Focus moved to another element inside this slot's root (e.g. between
  // "from" and "to" inputs in an interval): not yet done editing.
  if (target && next && target.contains(next)) return
  // Focus moved to a control inside the field's label cell - that's
  // the Revert button. The Revert action posts the reverse-diff itself
  // and must not race with a stale-data commit, so skip the commit. Mouse
  // and keyboard navigation both populate relatedTarget, so this catches
  // both tab-to and click-to the Revert button.
  const labelCell = getFieldLabelCell()
  if (labelCell && next instanceof Node && labelCell.contains(next)) return
  await commit()
}

// Checkbox-driven presence: NONE / UNKNOWN and HAS-without-sub-fields use
// a simple checkbox to add or remove the claim outright.
async function onCheckboxChange(checked: boolean | undefined): Promise<void> {
  const desired = !!checked
  const currentHas = props.modelValue !== null
  if (desired === currentHas) return
  if (desired) {
    // Add an empty presence claim.
    const num = getNextChangeNumber()
    const changeBase = [...props.base, "SESSION", props.session, String(num)]
    const newId = (await Identifier.from(...changeBase)).toString()
    const patch = makePatchForField(props.field, emptyFieldEntryValue())
    const addChange = new AddClaimChange({ id: newId, base: changeBase, patch })
    if (props.parentClaimId) {
      addChange.under = await props.parentClaimId()
    }
    await saveChange(addChange, num)
    const newClaim = claimPatchFrom(patch).New(newId)
    emit("update:modelValue", newClaim)
    return
  }
  // Remove the existing claim (sub-claims, if any, cascade with it on the backend).
  if (!props.modelValue) return
  const num = getNextChangeNumber()
  await saveChange(new RemoveClaimChange({ id: props.modelValue.id }), num)
  emit("update:modelValue", null)
}

// revertField restores this slot to its session-start state via the
// validation registry's checkpoint, then issues the appropriate
// Add / Set / Remove to bring the backend in line.
async function revertField(): Promise<void> {
  const current = props.modelValue
  const baseline = checkpointClaim.value
  const baselineValue = checkpointEntry.value
  // Restore local raw state from checkpoint first.
  local.value = { ...baselineValue }
  // Now reconcile claim-level state.
  if (baseline === null && current !== null) {
    // Slot was added during the session -> remove.
    const num = getNextChangeNumber()
    await saveChange(new RemoveClaimChange({ id: current.id }), num)
    emit("update:modelValue", null)
  } else if (baseline !== null && current === null) {
    // Slot was removed during the session -> resurrect with the baseline values.
    const num = getNextChangeNumber()
    const changeBase = [...props.base, "SESSION", props.session, String(num)]
    const newId = (await Identifier.from(...changeBase)).toString()
    const patch = makePatchForField(props.field, baselineValue)
    const addChange = new AddClaimChange({ id: newId, base: changeBase, patch })
    if (props.parentClaimId) {
      addChange.under = await props.parentClaimId()
    }
    await saveChange(addChange, num)
    const newClaim = claimPatchFrom(patch).New(newId)
    emit("update:modelValue", newClaim)
  } else if (baseline !== null && current !== null) {
    // Both non-null: if local values diverged from baseline, Set back.
    const currentValues = getClaimValues(current)
    if (!equalFieldEntryValue(currentValues, baselineValue)) {
      const patch = makePatchForField(props.field, baselineValue)
      const num = getNextChangeNumber()
      await saveChange(new SetClaimChange({ id: current.id, patch }), num)
      const updated = claimPatchFrom(patch).New(current.id)
      if (current.sub) {
        updated.sub = current.sub as unknown as ClaimTypes
      }
      emit("update:modelValue", updated)
    }
  }
  // Cascade revert into sub-claims (ClaimCardinality children).
  revertChildAll()
}

// Flush: cover the case where Save fires while the user is still in the
// input. commit() short-circuits on validation errors so an invalid value
// will not be posted.
async function flush(): Promise<[]> {
  await commit()
  return []
}

onMounted(() => registerForFlush(flush))
onBeforeUnmount(() => unregisterForFlush(flush))

// Pass-through callback: sub-ClaimCardinality receives this as its
// parent-claim-id; it forwards to each sub-ClaimInput, which calls it at
// the moment of Add to know what to set under to.
const ensureClaimIdCallback: () => Promise<string> = ensureClaimId

defineExpose({
  ...validatedInput,
  // Override the sync wrapper with the actual async function so direct
  // callers (e.g. ClaimCardinality.revertField) can await it.
  revert: revertField,
  ensureClaimId: ensureClaimIdCallback,
})
</script>

<template>
  <div ref="rootRef" class="flex min-w-0 grow flex-col gap-y-2">
    <!--
      Value input. Skipped for presence-only types (HAS / NONE / UNKNOWN);
      for those, see the checkbox / sub-form blocks below.

      required flows from ClaimCardinality (true only on slots that the
      missing-min check has flagged AND only once triggered is on).
      The inner input's validator then drives the "Required value." text;
      FieldsFormRow watches the prop and re-validates so toggling it
      surfaces/clears the message immediately rather than on the next
      blur.
    -->
    <InputErrors v-if="!isPresenceOnly" v-slot="errorProps" class="min-w-0 flex-auto grow">
      <div v-bind="errorProps" class="flex min-w-0 grow flex-col" @focusout="onFocusOut">
        <FieldsFormRow ref="formRowRef" v-model:entry="local" :field="field" :required="required" :invalid="invalid" />
      </div>
    </InputErrors>

    <!--
      Presence-toggle checkbox for NONE, UNKNOWN, and HAS-without-sub-fields.
      HAS *with* sub-fields skips the checkbox entirely and relies on the
      sub-form to drive presence (lazy create via ensureClaimId).
    -->
    <CheckBox v-if="showCheckbox" :model-value="modelValue !== null" @update:model-value="onCheckboxChange" />

    <!--
      Sub-fields: one ClaimCardinality per sub-field, each with its property
      label rendered above the input (unlike top-level fields, whose label
      sits in FieldsFormField's left grid column). Hidden for non-HAS slots
      that don't have a committed claim yet (the parent must exist before a
      sub-claim can sit under it). For HAS the sub-form is always shown;
      ensureClaimId lazily creates the parent on the first sub add.
    -->
    <template v-if="showSubFields">
      <div v-for="subField in field.subFields" :key="fieldKey(subField)" class="flex flex-col gap-y-1 pl-4">
        <DocumentRefInline :id="subField.propertyId" :link="false" class="leading-none font-medium text-gray-700" />
        <ClaimCardinality
          :model-value="extractSubClaims(modelValue, subField)"
          :initial-claims="extractSubClaims(initialClaim, subField)"
          :field="subField"
          :parent-claim-id="ensureClaimIdCallback"
          :session="session"
          :base="base"
        />
      </div>
    </template>
  </div>
</template>
