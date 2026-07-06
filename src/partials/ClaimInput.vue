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
import type { FieldsFormFlush, InputColumn, SaveChangeResult, SaveChangeSpec, ValidatedInput } from "@/types"

import { computed, inject, nextTick, onBeforeUnmount, onMounted, ref, shallowRef, toRaw, useId, useTemplateRef, watch } from "vue"

import CheckBox from "@/components/CheckBox.vue"
import { VT_HAS, VT_NONE, VT_UNKNOWN } from "@/core"
import { claimPatchFrom, claimTypeName, getClaimsOfTypeWithConfidence } from "@/document"
import {
  ChangeDroppedError,
  emptyFieldEntryValue,
  equalFieldEntryValue,
  fieldKey,
  fieldLabelCellKey,
  getClaimValues,
  getCommittedClaimKey,
  isSimpleField,
  makeDefaultPatchForField,
  makePatchForField,
  registerForFlushKey,
  registerRemoteConflictKey,
  saveChangeKey,
  unregisterForFlushKey,
  unregisterRemoteConflictKey,
  valueTypeToClaimType,
} from "@/fields"
import ClaimCardinality from "@/partials/ClaimCardinality.vue"
import FieldsFormRow from "@/partials/FieldsFormRow.vue"
import { allErrors, useRegisterForValidation, useValidationRegistry } from "@/validation"

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
    // Marks the slot as one of the field's min-cardinality "designated" slots:
    // it shows the "required" badge and lets the inner input surface its own
    // "Required value." (via its validator) when left empty and blurred. Set by
    // ClaimCardinality; the designation shifts live as slots fill/empty. The
    // text only appears on the input's own blur (or Save), never on load - see
    // the clear-only watch in FieldsFormRow.
    required?: boolean
    // Whether this is the first slot of its field. Only the first slot of a default value field
    // offers the "fill sub-fields with no value" affordance (which creates the none/unknown form).
    // Subsequent slots are value-first - their sub-fields appear only once a value is committed -
    // so a trailing placeholder cannot become a second default-form entry.
    isFirst?: boolean
    // Set by an enclosing slot whose own change is still being committed: this whole slot
    // renders read-only until the ancestor's committed state settles.
    readonly?: boolean
    // Id of this field's label element, threaded down to the value input's
    // FieldsFormRow so a bare single-column input is named via labelledby.
    labelId?: string
    // Suppress the value input's own labels row. Set by the enclosing cardinality
    // on a repeated field's slots: it hoists the shared column labels above all of
    // them (see FieldsFormRow for how the interval bounds keep their labels).
    hideLabels?: boolean
  }>(),
  {
    parentClaimId: undefined,
    invalid: false,
    required: false,
    isFirst: false,
    readonly: false,
    labelId: undefined,
    hideLabels: false,
  },
)

const emit = defineEmits<{
  "update:modelValue": [Claim | null]
  // A user-driven operation removed the slot's claim while focus was inside the slot.
  // The control focus was on unmounts with the filled state (e.g. InputFile's Clear
  // button), and when the slot itself is spliced the replacement is a fresh instance,
  // so a component's own focus restoration cannot reach it - the cardinality restores
  // focus onto the remaining or replacement input instead.
  cleared: []
}>()

const saveChange = inject(saveChangeKey, (spec: SaveChangeSpec) => Promise.resolve({ id: "id" in spec ? spec.id : "" }))
const getCommittedClaim = inject(getCommittedClaimKey, () => null)
const registerForFlush = inject(registerForFlushKey, () => {})
const unregisterForFlush = inject(unregisterForFlushKey, () => {})
const registerRemoteConflict = inject(registerRemoteConflictKey, () => {})
const unregisterRemoteConflict = inject(unregisterRemoteConflictKey, () => {})
// Lets the slot's focusout detect focus moving to a control inside the
// field's label cell (i.e. the field-level Revert button). When that's
// the case we skip the per-slot commit so it does not race the Revert
// click. Null when ClaimInput is mounted outside FieldsFormField (e.g.
// in tests or as a recursive sub-slot - recursive sub-slots' Revert
// button still lives on the topmost FieldsFormField anyway).
const getFieldLabelCell = inject(fieldLabelCellKey, () => null)

const isHas = computed(() => props.field.valueType === VT_HAS)
const isPresenceOnly = computed(() => isHas.value || props.field.valueType === VT_NONE || props.field.valueType === VT_UNKNOWN)

// Local raw-value state, owned by the user once mounted. It is seeded from the session
// BASELINE (initialClaim) for the first render and switched to the claim's loaded values
// in onMounted, before paint: the inner inputs checkpoint themselves at their own setup
// against the then-current model, so seeding with the baseline anchors their checkpoints
// (and thereby the per-input changed badges) to the session baseline. Seeding with the
// loaded values instead would make a mid-session reload look pristine to them even
// though the values differ from the session's starting point.
const local = ref<FieldEntryValue>(props.initialClaim ? getClaimValues(props.initialClaim) : emptyFieldEntryValue())
// True while local still holds the baseline seed. localIsEmpty follows the claim during
// that window: the baseline values must not make a loaded slot look empty, or the
// cardinality's compaction (running as other slots register mid-mount) drops it.
const localSeeded = ref(true)
onMounted(() => {
  const loaded = props.modelValue ? getClaimValues(props.modelValue) : emptyFieldEntryValue()
  if (!equalFieldEntryValue(local.value, loaded)) {
    local.value = loaded
  }
  localSeeded.value = false
})

// currentClaim mirrors props.modelValue but is also updated synchronously at every
// commit. Props round-trip through the parent's re-render, so two operations in quick
// succession (e.g. Save's flush while the focusout commit is still posting) would
// otherwise both read the pre-commit claim and post the same change twice.
//
// A prop value different from currentClaim is an EXTERNAL write to the slot (our own
// commits come back as the reference we emitted - compared raw, since the slot state
// wraps it in a reactive proxy): a remote add filling this placeholder slot. The slot
// takes it over completely, like resyncCommitted does for notified changes - local raw
// values rehydrate and the user is defocused.
// TODO: Implement better conflict handling.
const currentClaim = shallowRef<DeepReadonly<Claim> | null>(props.modelValue)
watch(
  () => props.modelValue,
  (v) => {
    if (toRaw(v) === toRaw(currentClaim.value)) {
      return
    }
    currentClaim.value = v
    local.value = v && !isPresenceOnly.value ? getClaimValues(v) : emptyFieldEntryValue()
    blurIfInside()
  },
  { flush: "sync" },
)

// blurIfInside defocuses the user when their focus is inside this slot. Called after the
// slot's state has been replaced by an external change, so the blur-triggered commit
// observes local matching the committed claim and does nothing.
// TODO: No need once we have better conflict handling.
function blurIfInside(): void {
  const focused = document.activeElement
  if (focused instanceof HTMLElement && rootRef.value?.contains(focused)) {
    focused.blur()
  }
}

// setClaim publishes a new committed claim state both locally (synchronously) and to
// the parent slot. Local raw values mirror the committed claim, so after every commit
// the input shows the canonical committed form - including parts the patch defaulted
// (e.g. an empty interval bound committing as none) or canonicalized. The slot is
// read-only while its change is in flight, so no user edit can be clobbered here.
function setClaim(claim: DeepReadonly<Claim> | null): void {
  currentClaim.value = claim
  local.value = claim && !isPresenceOnly.value ? getClaimValues(claim) : emptyFieldEntryValue()
  emit("update:modelValue", claim as Claim | null)
}

// checkpointClaim is the revert target. Seeded from initialClaim; watched
// so a parent re-anchor moves it without remount. Updated by checkpoint()
// to the current claim (called from DocumentEdit's checkpointAll).
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
// of the current claim (and initialClaim) so the sub-ClaimCardinality
// has the right slice.
function extractSubClaims(claim: DeepReadonly<Claim> | null, subField: DeepReadonly<FieldData>): readonly DeepReadonly<Claim>[] {
  if (!claim || !claim.sub) return []
  const t = valueTypeToClaimType(subField.valueType)
  return getClaimsOfTypeWithConfidence(claim.sub, t, subField.propertyId)
}

// currentSubClaims returns the current claim's sub-claims for a sub-field, read from the
// COMMITTED doc state of the claim when the doc holds it. The local claim object is only
// a snapshot from creation/last set: sub-claims committed through the sub-cardinalities
// are never attached to it, and removed ones never leave it. Reading it directly would
// show a lazily-created base as sub-less even after sub-claims committed (a remount would
// then seed empty sub-forms over them, and typing would duplicate the sub-claims under
// the same base), and would keep removed sub-claims forever on a loaded claim (blocking
// the empty-base cleanup).
function currentSubClaims(subField: DeepReadonly<FieldData>): readonly DeepReadonly<Claim>[] {
  const committed = currentClaim.value
  if (!committed) return []
  return extractSubClaims(getCommittedClaim(committed.id) ?? committed, subField)
}

// Whether the sub-ClaimCardinality should be rendered for this slot.
// For HAS with sub-fields we always render (the user only edits HAS
// *through* its sub-claims, which lazy-create the HAS itself). A value field
// with a default behaves the same: ensureClaimId lazily creates the base as a
// none/unknown claim so sub-claims can be added before a value is entered
// (e.g. notes on a studio whose location is still unknown). For other types we
// only render after the parent claim has been committed - a sub-claim requires
// a parent id, and ensureClaimId for those just returns the existing id.
const showSubFields = computed(() => {
  if (props.field.subFields.length === 0) return false
  if (isHas.value) return true
  // Only the first slot of a default field shows sub-fields before a value exists (so the single
  // primary entry can be the none/unknown form). Other slots are value-first.
  if (props.field.default && props.isFirst) return true
  // Show as soon as the slot has a value locally (on dirty), before the claim is committed on blur,
  // mirroring how the cardinality grows a new trailing slot on dirty. A sub-claim added before the
  // commit lazily creates the parent claim via ensureClaimId. Stay shown while a committed claim
  // exists even if the value is momentarily cleared mid-edit.
  return hasValue.value || currentClaim.value !== null
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

// hasAnySubClaims is true when any sub-field has at least one committed claim attached
// to this claim's sub. Used by the commit/cleanup logic's "don't auto-remove a parent
// that still has sub-claims" branches. A function, not a computed: callers read it at
// operation time and the committed doc it consults is not reactive.
function hasAnySubClaims(): boolean {
  for (const subField of props.field.subFields) {
    if (currentSubClaims(subField).length > 0) return true
  }
  return false
}

// localIsEmpty: every relevant local raw field is at its default. For HAS
// (and NONE/UNKNOWN) presence-only types the local raw values are always
// at defaults, so the slot's emptiness is determined by sub-claims alone.
const localIsEmpty = computed(() => {
  if (isPresenceOnly.value) return true
  if (localSeeded.value) {
    return currentClaim.value === null
  }
  return equalFieldEntryValue(local.value, emptyFieldEntryValue())
})

// rootRef is the DOM identity used by the sub-validation registry's
// self-registration so the outer registry (ClaimCardinality) can find us.
const rootRef = useTemplateRef<HTMLElement>("rootRef")

// Id on the presence-toggle CheckBox (NONE / UNKNOWN / HAS-without-sub-fields).
// Those slots have no registered child input, so inputEl resolves the checkbox
// itself as the focus target, found by id (CheckBox renders a leading template
// comment, so its $el is a comment node, not an element to querySelector).
const checkboxId = useId()
function checkboxInputEl(): HTMLElement | null {
  return document.getElementById(checkboxId)
}

// formRowRef points at the FieldsFormRow ValidatedInput. We call its
// validate directly (rather than the broader validateChildAll on
// the sub-registry) when committing this slot's value, so a sub-cardinality
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
  firstInputEl,
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
  const hasCurrent = currentClaim.value !== null
  const hasBaseline = checkpointClaim.value !== null
  return hasCurrent !== hasBaseline
})

// isEmpty: presence-only slots are empty iff there are no sub-claims; non-
// HAS slots also consider local raw emptiness.
const isEmpty = computed<boolean>(() => {
  if (isPresenceOnly.value) {
    // No own value; emptiness is sub-claim emptiness AND no committed claim.
    if (currentClaim.value !== null) return false
    return allChildEmpty.value
  }
  if (!localIsEmpty.value) return false
  if (props.field.subFields.length === 0) return true
  // We treat any non-empty sub-claim as "this slot has content".
  return allChildEmpty.value
})

// hasValue: whether this slot has a base value. Drives whether the cardinality offers a new
// trailing slot. For presence-only slots the presence/sub-claims are the value, so it mirrors
// non-emptiness. For value fields it is true only when the value input itself is non-empty: a
// value field with a default whose only content is sub-claims (e.g. a studio with notes but an
// unknown location) does NOT count as having a value, so it does not grow a new trailing slot.
const hasValue = computed<boolean>(() => {
  if (isPresenceOnly.value) return !isEmpty.value
  return !localIsEmpty.value
})

// Number of this slot's changes queued or in flight. While non-zero the whole slot
// (value input, checkbox, and sub-fields) is read-only - grayed and non-interactive,
// but selectable - so no further local edits pile up on a claim whose committed state
// is not settled yet.
const pendingCount = ref(0)
const slotReadonly = computed<boolean>(() => props.readonly || pendingCount.value > 0)

// Slot operations (commit, revert, slot cleanup, checkbox toggle, lazy parent create)
// are serialized so a second trigger (e.g. Save's flush while the focusout commit is
// still posting) observes the state left behind by the first instead of racing it.
let operationChain: Promise<unknown> = Promise.resolve()
function runSerialized<T>(fn: () => Promise<T>): Promise<T> {
  const run = operationChain.then(fn)
  operationChain = run.catch(() => undefined)
  return run
}

// resyncCommitted replaces the slot's state (claim and local raw values) with the given
// committed claim. Used when a change is dropped or a remote change wins over local work.
// When the user is focused inside the slot they are defocused: the state under them has
// just been replaced, so their in-progress interaction no longer applies.
function resyncCommitted(claim: DeepReadonly<Claim> | null): void {
  setClaim(claim)
  blurIfInside()
}

// submitChange queues one change and tracks it as pending for this slot. Returns null
// when the change was dropped (it lost its change number to a concurrent change and does
// not apply anymore, see ChangeDroppedError); the slot has then already been resynced to
// the committed state and the caller should stop its flow.
async function submitChange(spec: SaveChangeSpec): Promise<SaveChangeResult | null> {
  pendingCount.value += 1
  try {
    return await saveChange(spec)
  } catch (err) {
    if (err instanceof ChangeDroppedError) {
      resyncCommitted(spec.type === "add" ? null : getCommittedClaim(spec.id))
      return null
    }
    throw err
  } finally {
    pendingCount.value -= 1
  }
}

// hasUncommittedLocal reports whether local raw values differ from the committed claim's
// values (typed but not committed yet). Presence-only slots have no local raw values.
function hasUncommittedLocal(): boolean {
  if (isPresenceOnly.value) return false
  const committedValues = currentClaim.value ? getClaimValues(currentClaim.value) : emptyFieldEntryValue()
  return !equalFieldEntryValue(local.value, committedValues)
}

// A remote change touched claims we may hold local state for. Server wins: the slot
// resyncs to the committed state, discarding uncommitted local edits if any. By
// notification time the subscription has applied all committed ops for the touched claims, so
// the committed lookup is current and the own-echo lag that keeps slots from syncing
// off the doc in general does not apply here. Slots with a queued change are left
// alone: that change either overrides the remote one or is dropped by the queue's
// conflict handling, which resyncs through submitChange.
function onRemoteConflict(claimIds: ReadonlySet<string>): void {
  const committed = currentClaim.value
  if (!committed || !claimIds.has(committed.id)) {
    return
  }
  if (pendingCount.value > 0) {
    return
  }
  resyncCommitted(getCommittedClaim(committed.id))
}
onMounted(() => registerRemoteConflict(onRemoteConflict))
onBeforeUnmount(() => unregisterRemoteConflict(onRemoteConflict))

// Forward the value input's reported columns and hints (empty for presence-only
// slots with no value input) so the enclosing cardinality can read whether the
// input renders labels, and render the hoisted label/hint rows of a repeated
// field, without hardcoding any of it.
const columns = computed<InputColumn[]>(() => formRowRef.value?.columns ?? [])
const hints = computed<string[]>(() => formRowRef.value?.hints ?? [])

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
  // Focus target: the slot's value input (or its first sub-field), and the
  // presence checkbox for NONE / UNKNOWN / HAS-without-sub-fields slots,
  // whose checkbox is not a registered validation input.
  inputEl: () => (showCheckbox.value ? checkboxInputEl() : firstInputEl()),
  // Identity is the slot wrapper, not the inner control: ClaimCardinality
  // tests whether mainEl contains document.activeElement to decide whether
  // the trailing slot the user is editing may be shrunk.
  mainEl: () => rootRef.value,
  isDirty: computed(() => localDirty.value || identityDirty.value || anyChildDirty.value),
  isEmpty,
  errors: allErrors(childInputs),
  columns,
  hints,
  checkpoint: () => {
    checkpointClaim.value = currentClaim.value
    checkpointEntry.value = currentClaim.value ? getClaimValues(currentClaim.value) : emptyFieldEntryValue()
    checkpointChildAll()
  },
}

const { onInteraction: notifyOuter } = useRegisterForValidation(validatedInput)
forwardInteraction = notifyOuter

// ensureClaimId returns this claim's id, lazily creating an empty base claim
// if needed. Sub-ClaimCardinality passes this down to its slots so their
// AddClaimChange knows what to set under to. The lazily-created base is a HAS
// claim for HAS fields, or the none/unknown default form for a value field
// with a default (so sub-claims can be attached before a value is entered).
// For other value fields the slot's own typed-but-uncommitted value is
// committed instead: a sub-claim interaction may deliberately keep focus in
// the value input (preventing the blur commit, see ClaimRefSelect), yet its
// add needs the parent claim to exist. Serialized with the slot's other
// operations so a concurrent commit cannot create the base claim twice.
function ensureClaimId(): Promise<string> {
  return runSerialized(doEnsureClaimId)
}
async function doEnsureClaimId(): Promise<string> {
  if (currentClaim.value !== null) {
    return currentClaim.value.id
  }
  let patch: object
  if (isHas.value) {
    patch = makePatchForField(props.field, emptyFieldEntryValue())
  } else if (props.field.default) {
    patch = makeDefaultPatchForField(props.field)
  } else {
    await doCommit(false)
    // The cast defeats the narrowing from the null check above: doCommit sets
    // currentClaim when it commits the value.
    const committed = currentClaim.value as DeepReadonly<Claim> | null
    if (committed !== null) {
      return committed.id
    }
    throw new Error("ensureClaimId called with no committable value on a non-HAS slot without a default")
  }
  const newClaim = await addClaimWithParent(patch)
  if (!newClaim) {
    throw new Error("lazily created base claim was dropped")
  }
  setClaim(newClaim)
  return newClaim.GetID()
}

// cleanupEmptyBase removes this slot's claim when it is a lazily-created base left
// holding nothing: a HAS claim (of a field with sub-fields) or a default (none/unknown)
// form whose sub-claims are all gone and whose value side is empty. Sub-cardinalities
// call it when a revert empties them - the base came into existence implicitly with the
// first sub-claim (ensureClaimId), so its removal mirrors that; otherwise an invisible
// empty claim would keep the field flagged as changed with no control left to remove it.
function cleanupEmptyBase(): Promise<void> {
  return runSerialized(async () => {
    const committed = currentClaim.value
    if (!committed) {
      return
    }
    if (!allChildEmpty.value || !localIsEmpty.value) {
      return
    }
    // Committed sub-claims mean removes may still be in flight (see cleanupResidue);
    // each landing removal re-triggers this through updateSlotClaim, so the last one
    // gets to remove the base.
    if (hasAnySubClaims()) {
      return
    }
    const isLazyBase = (isHas.value && props.field.subFields.length > 0) || (props.field.default !== undefined && claimTypeName(committed) === props.field.default)
    if (!isLazyBase) {
      return
    }
    if (!(await submitChange({ type: "remove", id: committed.id }))) {
      return
    }
    setClaim(null)
  })
}

// addClaimWithParent commits an AddClaimChange for the given patch and returns the new
// claim (with the id assigned by the change queue), or null when the add was dropped. It
// resolves the (possibly lazily-created) parent FIRST so the parent is committed before
// this claim is queued: a sub-claim's under has to reference a committed claim id.
async function addClaimWithParent(patch: object): Promise<Claim | null> {
  const under = props.parentClaimId ? await props.parentClaimId() : undefined
  const result = await submitChange(under === undefined ? { type: "add", patch } : { type: "add", patch, under })
  if (!result) {
    return null
  }
  return claimPatchFrom(patch).New(result.id)
}

// commit runs Add / Set / Cast / Remove for the value side of this claim.
// Sub-claims are handled by the nested ClaimCardinality instances and do
// not go through this path.
//
// A value field with a default (none/unknown) can hold either a value claim
// (a location is known) or the default form carrying only sub-claims (the
// location is unknown but notes exist). Switching between the two changes the
// claim type, which a Set cannot do, so we use a Cast to change the type in
// place while preserving the claim id and its sub-claims.
function commit(): Promise<void> {
  // Whether focus is inside the slot has to be captured when the commit is REQUESTED: by
  // the time the serialized body runs, a cleared value's filled-state controls (e.g.
  // InputFile's Clear button) may already have unmounted in a render flush, dropping
  // focus to the body. A remove with focus inside reports cleared so the cardinality
  // restores focus (see the cleared emit).
  const hadFocus = rootRef.value?.contains(document.activeElement) ?? false
  return runSerialized(() => doCommit(hadFocus))
}
async function doCommit(hadFocus: boolean): Promise<void> {
  if (isPresenceOnly.value) return
  const committed = currentClaim.value
  const valueType = valueTypeToClaimType(props.field.valueType)

  // Empty path. Skip validation here on purpose: the user is clearing the input, and a
  // sub-cardinality that would normally fire "Required value." for an empty sub-field must NOT
  // block the remove/cast, otherwise the row gets stuck and stays red.
  if (localIsEmpty.value) {
    if (!committed) return
    if (props.field.default) {
      if (props.isFirst) {
        // First slot of a default field - the only entry allowed to be the default form. If
        // sub-claims remain (live state), demote a value claim to its default (none/unknown) form,
        // preserving them. If nothing remains, removal of the now-empty default claim is handled by
        // cleanupResidue once focus leaves the whole slot - doing it here would fire while focus is
        // still in the value input and the user may be mid-edit.
        if (!allChildEmpty.value && claimTypeName(committed) !== props.field.default) {
          const patch = makeDefaultPatchForField(props.field)
          if (!(await submitChange({ type: "cast", id: committed.id, patch }))) return
          const updated = claimPatchFrom(patch).New(committed.id)
          if (committed.sub) {
            updated.sub = committed.sub as unknown as ClaimTypes
          }
          setClaim(updated)
        }
        return
      }
      // Non-first slot of a default field: value-first, like a regular field. It must NOT demote to
      // the default form (only the first slot may be the default form), and a value claim cannot
      // hold an empty value, so keep it while sub-claims remain (live state), else remove it.
      if (allChildEmpty.value) {
        if (!(await submitChange({ type: "remove", id: committed.id }))) return
        // Reported BEFORE the removal is published: publishing splices the slot, and an
        // emit from a component queued for unmount is silently dropped by Vue.
        if (hadFocus) {
          emit("cleared")
        }
        setClaim(null)
      }
      return
    }
    // Regular (non-default) field: a value claim cannot hold an empty value, so keep it when
    // sub-claims remain, otherwise remove it.
    if (!hasAnySubClaims()) {
      if (!(await submitChange({ type: "remove", id: committed.id }))) return
      // Reported BEFORE the removal is published, see above.
      if (hadFocus) {
        emit("cleared")
      }
      setClaim(null)
    }
    return
  }

  // Non-empty path: Add / Set / Cast. Validate the row's inner inputs first so an invalid value
  // (e.g. "htt" for an IRI) stays in the form uncommitted. Sub-cardinality validation is
  // deliberately excluded - see formRowRef definition for why.
  if (formRowRef.value) {
    await formRowRef.value.validate()
    if (formRowRef.value.errors.length > 0) return
    // Refuse to Set/Add a value-less claim. We reach the non-empty path because
    // localIsEmpty counts precision, so a stray precision on an empty value looks
    // non-empty here; but the value input reporting empty means there is no value
    // to store, and a claim carrying only a precision is invalid. Bail so such a
    // claim can never reach the server, even if some path left a precision behind
    // (the input-level reset on value-clear already empties these rows normally).
    if (formRowRef.value.isEmpty) return
  }
  const patch = makePatchForField(props.field, local.value)
  if (committed) {
    if (claimTypeName(committed) !== valueType) {
      // The committed claim is the default (none/unknown) form and the user has now entered a
      // value. Promote it to the value type, preserving the sub-claims.
      if (!(await submitChange({ type: "cast", id: committed.id, patch }))) return
      const updated = claimPatchFrom(patch).New(committed.id)
      if (committed.sub) {
        updated.sub = committed.sub as unknown as ClaimTypes
      }
      setClaim(updated)
      return
    }
    // Update existing claim. Only post if values actually changed.
    if (equalFieldEntryValue(local.value, getClaimValues(committed))) {
      return
    }
    if (!(await submitChange({ type: "set", id: committed.id, patch }))) return
    // Reconstruct the claim with the new values so the parent sees the updated state
    // immediately, without waiting for the next doc sync.
    const updated = claimPatchFrom(patch).New(committed.id)
    if (committed.sub) {
      // Preserve sub-claims through the update. The DeepReadonly is a type-only
      // concern; at runtime ClaimTypes is the same object.
      updated.sub = committed.sub as unknown as ClaimTypes
    }
    setClaim(updated)
    return
  }
  // No claim yet. Add.
  const newClaim = await addClaimWithParent(patch)
  if (newClaim) {
    setClaim(newClaim)
  }
}

// onSlotFocusOut runs when focus leaves the whole slot (the value input and all
// sub-fields): it commits the value input's local state and then cleans up residue
// entries. Moving from the value input into the slot's own sub-fields deliberately
// does NOT commit: the commit's pending phase would flash the slot read-only and a
// disabled control ejects the focus the user just moved onto it (the first Tab into
// a sub-field checkbox would visibly do nothing); a sub-claim interaction needing
// the parent claim flushes the value itself, see ensureClaimId.
async function onSlotFocusOut(event: FocusEvent): Promise<void> {
  const next = event.relatedTarget as Node | null
  // Focus moved to another element inside this slot's root (e.g. between the
  // "from" and "to" inputs of an interval, or into a sub-field): not yet done
  // editing this entry.
  if (rootRef.value && next instanceof Node && rootRef.value.contains(next)) return
  // Focus moved to a control inside the field's label cell - that's
  // the Revert button. The Revert action posts the reverse-diff itself
  // and must not race with a stale-data commit, so skip the commit. Mouse
  // and keyboard navigation both populate relatedTarget, so this catches
  // both tab-to and click-to the Revert button.
  const labelCell = getFieldLabelCell()
  if (labelCell && next instanceof Node && labelCell.contains(next)) return
  // A null relatedTarget is ambiguous: focus may really have left (moved to the body or
  // a non-focusable element), but Chrome also dispatches such a focusout when a focused
  // element inside the slot UNMOUNTS mid-interaction (e.g. InputRef's Clear button
  // unmounting with the chip, before the input restores focus into the now-empty search
  // input). Wait a tick for any such programmatic focus restore to land and only commit
  // when focus actually settled outside the slot; committing on the unmount blur would
  // remove the just-cleared claim (and splice the slot) under the user mid-edit.
  if (!next) {
    await nextTick()
    if (unmounting) return
    const active = document.activeElement
    if (active && active !== document.body && rootRef.value?.contains(active)) return
  }
  await commit()
  await cleanupResidue()
}

// Set while the component tears down, so the deferred focusout check above does not
// commit from a slot that got unmounted during its tick.
let unmounting = false
onBeforeUnmount(() => {
  unmounting = true
})

// A missing-state checkbox (unknown/none of an interval bound) was toggled. CHECKING a
// state is a complete decision, so it commits immediately. Deferring it to blur would
// leave local diverging from the claim, and a later unrelated focus change would post a
// surprise set: the slot flashes read-only mid-gesture and a now-disabled checkbox
// drops focus and eats the click the user is in the middle of. UNCHECKING is different:
// it is the start of providing a value for the bound, so it stays uncommitted - an
// immediate commit would just snap the empty bound back to its default missing state
// and lock the input away from the user. The blur commit resolves a bound left empty.
async function onMissingChange(side: "from" | "to"): Promise<void> {
  // While no claim is committed yet, a toggle stays uncommitted like typing does: an
  // immediate commit would have to materialize the whole interval and would default the
  // other, untouched bound mid-editing (a surprising "unknown" appearing on To right
  // after the first click on From). The commit happens once focus leaves the whole
  // widget (both bounds with their precisions and checkboxes), like for typed values.
  if (currentClaim.value === null) {
    return
  }
  const l = local.value
  const boundSet = side === "from" ? l.fromUnknown || l.fromNone : l.toUnknown || l.toNone
  if (!boundSet) {
    return
  }
  // The commit resolves the WHOLE interval, so it is immediate only when the other
  // bound is resolved too (a value or a flag). A deselected-and-empty other bound is a
  // transient state - the user is preparing to type its value - and committing now
  // would snap its default back on.
  const otherResolved = side === "from" ? l.toUnknown || l.toNone || !!l.valueTo : l.fromUnknown || l.fromNone || !!l.value
  if (!otherResolved) {
    return
  }
  await commit()
}

// The value input made a change which is a complete decision on its own (a finished
// file upload, a cleared file). There is no natural blur after it (the file dialog and
// the async upload leave focus where it was), so it commits immediately.
async function onCompleteChange(): Promise<void> {
  await commit()
}

// cleanupResidue removes the slot's claim on slot-leave when it is meaningless residue: an
// entry with no value and no sub-claims, for the entry kinds whose emptiness commit() does
// not handle (commit() only sees the value side and never touches presence-only claims).
async function cleanupResidue(): Promise<void> {
  // Two entry kinds defer their empty-removal to slot-leave: the FIRST slot of a default field
  // (non-first slots remove an empty entry directly in commit(), like regular fields, so the
  // cleanup must not also fire there - it would be a double remove), and a HAS base of a field
  // with sub-fields (commit() never touches presence-only claims, and with sub-fields there is
  // no checkbox to remove it; such an empty base is always residue of ensureClaimId, see
  // cleanupEmptyBase).
  const defersToSlotLeave = (props.field.default !== undefined && props.isFirst) || (isHas.value && props.field.subFields.length > 0)
  if (!defersToSlotLeave) return
  await runSerialized(async () => {
    const committed = currentClaim.value
    if (!committed) return
    if (!isEmpty.value) return // still has a value or mid-edit (uncommitted) sub-fields
    // Claim-based check on top of the local-state isEmpty: a sub-claim's remove commit
    // triggered by this same focusout burst may still be in flight (its local state is
    // already empty), and removing the base claim first would cascade the sub-claim
    // away on the backend and wedge the session replay on the sub-claim's own remove.
    // Bail while sub-claims remain committed; once their removal lands, the
    // sub-cardinality's updateSlotClaim runs cleanupParentIfEmpty, which removes the
    // then-empty base.
    if (hasAnySubClaims()) return
    if (!(await submitChange({ type: "remove", id: committed.id }))) return
    setClaim(null)
  })
}

// Checkbox-driven presence: NONE / UNKNOWN and HAS-without-sub-fields use
// a simple checkbox to add or remove the claim outright.
async function onCheckboxChange(checked: boolean | undefined): Promise<void> {
  const desired = !!checked
  // Captured at request time, like in commit above.
  const checkboxHadFocus = rootRef.value?.contains(document.activeElement) ?? false
  await runSerialized(async () => {
    const committed = currentClaim.value
    if (desired === (committed !== null)) return
    if (desired) {
      // Add an empty presence claim.
      const newClaim = await addClaimWithParent(makePatchForField(props.field, emptyFieldEntryValue()))
      if (newClaim) {
        setClaim(newClaim)
      }
      return
    }
    // Remove the existing claim (sub-claims, if any, cascade with it on the backend).
    if (!committed) return
    if (!(await submitChange({ type: "remove", id: committed.id }))) return
    // Reported BEFORE the removal is published, see doCommit.
    if (checkboxHadFocus) {
      emit("cleared")
    }
    setClaim(null)
  })
}

// revertEntryCallback backs the interval bounds' changed badges (through FieldsFormRow
// into InputField): each bound reverts independently and posts the reverting changes
// right away (see revertBound). They are the only per-input badges; whole-slot reverts
// come from the levels above (field label, cardinality count, or sub-field header).
function revertEntryCallback(side: "from" | "to"): void {
  void revertBound(side)
}

// revertField restores this slot to its session-start state via the
// validation registry's checkpoint, then issues the appropriate
// Add / Set / Remove to bring the backend in line.
function revertField(): Promise<void> {
  // Captured at request time, like in commit above.
  const hadFocus = rootRef.value?.contains(document.activeElement) ?? false
  return runSerialized(() => doRevertField(hadFocus))
}
async function doRevertField(hadFocus: boolean): Promise<void> {
  const committed = currentClaim.value
  const baseline = checkpointClaim.value
  const baselineValue = checkpointEntry.value
  // Restore local raw state from checkpoint first.
  local.value = { ...baselineValue }
  // Now reconcile claim-level state.
  if (baseline === null && committed !== null) {
    // Slot was added during the session -> remove. The removal cascades to the claim's
    // sub-claims, so the sub-cardinalities must NOT revert-with-changes - their removes
    // would target already-removed claims and break the session - they just drop their
    // local slots, after the cleared claim has propagated into their modelValue.
    if (await submitChange({ type: "remove", id: committed.id })) {
      // Reported BEFORE the removal is published, see doCommit.
      if (hadFocus) {
        emit("cleared")
      }
      setClaim(null)
      await nextTick()
      resetChildAll()
    }
    return
  }
  if (baseline !== null && committed === null) {
    // Slot was removed during the session -> resurrect with the baseline values.
    const newClaim = await addClaimWithParent(makePatchForField(props.field, baselineValue))
    if (newClaim) {
      setClaim(newClaim)
    }
  } else if (baseline !== null && committed !== null) {
    // Both non-null: if local values diverged from baseline, Set back.
    const committedValues = getClaimValues(committed)
    if (!equalFieldEntryValue(committedValues, baselineValue)) {
      const patch = makePatchForField(props.field, baselineValue)
      if (await submitChange({ type: "set", id: committed.id, patch })) {
        const updated = claimPatchFrom(patch).New(committed.id)
        if (committed.sub) {
          updated.sub = committed.sub as unknown as ClaimTypes
        }
        setClaim(updated)
      }
    }
  }
  // Cascade revert into sub-claims (ClaimCardinality children).
  revertChildAll()
}

// revertBound restores a single interval bound's local raw state to the checkpoint,
// leaving the other bound as the user has it, then commits the resulting interval like a
// blur would, so the revert is posted right away, like the field-level revert. The
// commit is skipped while the other bound is unresolved (no value and no missing flag):
// committing then would default the unresolved bound to unknown mid-edit (see
// onMissingChange), so the resolution is left to the natural blur commit.
function revertBound(side: "from" | "to"): Promise<void> {
  // Captured at request time, like in commit above.
  const hadFocus = rootRef.value?.contains(document.activeElement) ?? false
  return runSerialized(() => doRevertBound(side, hadFocus))
}
async function doRevertBound(side: "from" | "to", hadFocus: boolean): Promise<void> {
  const baselineValue = checkpointEntry.value
  const restored = { ...local.value }
  if (side === "from") {
    restored.value = baselineValue.value
    restored.amountPrecision = baselineValue.amountPrecision
    restored.timePrecision = baselineValue.timePrecision
    restored.fromUnknown = baselineValue.fromUnknown
    restored.fromNone = baselineValue.fromNone
  } else {
    restored.valueTo = baselineValue.valueTo
    restored.amountPrecisionTo = baselineValue.amountPrecisionTo
    restored.timePrecisionTo = baselineValue.timePrecisionTo
    restored.toUnknown = baselineValue.toUnknown
    restored.toNone = baselineValue.toNone
  }
  local.value = restored
  // Let the restored values propagate into the inner inputs before committing, so the
  // bound's changed badge clears and the commit's row validation observes them.
  await nextTick()
  const empty = equalFieldEntryValue(restored, emptyFieldEntryValue())
  const fromResolved = !!restored.value || restored.fromUnknown || restored.fromNone
  const toResolved = !!restored.valueTo || restored.toUnknown || restored.toNone
  if (empty || (fromResolved && toResolved)) {
    await doCommit(hadFocus)
  }
}

// Flush: cover the case where Save fires while the user is still in the
// input. commit() short-circuits on validation errors so an invalid value
// will not be posted. hasUncommitted lets DocumentEdit warn before the tab
// closes with local edits which have not produced their change yet.
const flushInstance: FieldsFormFlush = {
  flush: () => commit(),
  hasUncommitted: hasUncommittedLocal,
}

onMounted(() => registerForFlush(flushInstance))
onBeforeUnmount(() => unregisterForFlush(flushInstance))

// Pass-through callback: sub-ClaimCardinality receives this as its
// parent-claim-id; it forwards to each sub-ClaimInput, which calls it at
// the moment of Add to know what to set under to.
const ensureClaimIdCallback: () => Promise<string> = ensureClaimId

// Sub-field label ids (so a sub-field's bare value input is named via
// labelledby). Each sub-field's ClaimCardinality renders its own label + whole-
// sub-field badge (header) and is passed this id for the label element.
const subBaseId = useId()
function subFieldLabelId(subField: DeepReadonly<FieldData>): string {
  return `${subBaseId}-${fieldKey(subField)}`
}

// The sub-field group spaces its members like any field group: gap-8 once any
// sub-field is non-simple (repeats or has its own sub-fields), else gap-4.
const subFieldGapClass = computed<string>(() => (props.field.subFields.some((subField) => !isSimpleField(subField)) ? "gap-y-8" : "gap-y-4"))

defineExpose({
  ...validatedInput,
  // Override the sync wrapper with the actual async function so direct
  // callers (e.g. ClaimCardinality.revertField) can await it.
  revert: revertField,
  ensureClaimId: ensureClaimIdCallback,
  hasValue,
})
</script>

<template>
  <div ref="rootRef" class="flex min-w-0 grow flex-col gap-y-4" @focusout="onSlotFocusOut">
    <!--
      Value input. Skipped for presence-only types (HAS / NONE / UNKNOWN);
      for those, see the checkbox / sub-form blocks below.

      required flows from ClaimCardinality (true only on slots that the
      missing-min check has flagged AND only once triggered is on).
      The inner input's validator then drives the "Required value." text;
      FieldsFormRow watches the prop and re-validates so toggling it
      surfaces/clears the message immediately rather than on the next
      blur.

      readonly is on while this slot's changes are queued or in flight (or an
      ancestor slot's are), so no further edits pile up before the claim's
      committed state settles.
    -->
    <div v-if="!isPresenceOnly" class="flex min-w-0 grow flex-col">
      <FieldsFormRow
        ref="formRowRef"
        v-model:entry="local"
        :field="field"
        :required="required"
        :invalid="invalid"
        :readonly="slotReadonly"
        :revert="revertEntryCallback"
        :label-id="labelId"
        :hide-labels="hideLabels"
        @missing-change="onMissingChange"
        @complete-change="onCompleteChange"
      />
    </div>

    <!--
      Presence-toggle checkbox for NONE, UNKNOWN, and HAS-without-sub-fields.
      HAS *with* sub-fields skips the checkbox entirely and relies on the
      sub-form to drive presence (lazy create via ensureClaimId).
    -->
    <CheckBox v-if="showCheckbox" :id="checkboxId" :model-value="currentClaim !== null" :disabled="slotReadonly" @update:model-value="onCheckboxChange" />

    <!--
      Sub-fields: one ClaimCardinality per sub-field, each with its property
      label rendered above the input (unlike top-level fields, whose label
      sits in FieldsFormField's left grid column). Hidden for non-HAS slots
      that don't have a committed claim yet (the parent must exist before a
      sub-claim can sit under it). For HAS the sub-form is always shown;
      ensureClaimId lazily creates the parent on the first sub add.
    -->
    <div v-if="showSubFields" class="flex flex-col" :class="subFieldGapClass">
      <!--
        Each sub-field's ClaimCardinality renders its own header (property label +
        whole-sub-field badge) above its slots, in the input column
        so the label lines up with the input rather than the repeat count.
      -->
      <ClaimCardinality
        v-for="subField in field.subFields"
        :key="fieldKey(subField)"
        :show-header="true"
        :model-value="currentSubClaims(subField)"
        :initial-claims="extractSubClaims(initialClaim, subField)"
        :field="subField"
        :parent-claim-id="ensureClaimIdCallback"
        :parent-cleanup="cleanupEmptyBase"
        :readonly="slotReadonly"
        :label-id="subFieldLabelId(subField)"
      />
    </div>
  </div>
</template>
