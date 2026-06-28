<script setup lang="ts">
import type { ValidatedInput, ValidationError } from "@/types"

import { computed, ref, useTemplateRef, watch } from "vue"
import { useI18n } from "vue-i18n"

import CheckBox from "@/components/CheckBox.vue"
import WithLock from "@/components/WithLock.vue"
import { getParentLock, useLock } from "@/progress"
import { useRegisterForValidation, useValidationRegistry } from "@/validation"

const props = defineProps<{
  // We do NOT forward required to the wrapped input - we own the required
  // check ourselves because a missing-state checkbox already satisfies
  // "field has a value". When our own validate() detects required-but-empty
  // (neither checkbox checked, wrapped input also empty), showRequired
  // flips on and we route :invalid=true through to the wrapped input and
  // to the checkboxes for visual feedback; any subsequent interaction
  // clears it.
  required?: boolean
  // Presentational override.
  invalid?: boolean
}>()

// Two independent v-models, one per checkbox. They are kept mutually
// exclusive internally (checking one unchecks the other). The wrapped
// input owns its own v-model; calling code is expected to check these
// two models first when emitting a value - if either is true, the
// wrapped input's value should be ignored.
const unknown = defineModel<boolean>("unknown", { default: false })
const none = defineModel<boolean>("none", { default: false })

// True when either missing-state checkbox is checked.
const missingSet = computed<boolean>(() => unknown.value || none.value)

const ownErrors = ref<ValidationError[]>([])
const innerErrors = ref<ValidationError[]>([])

// Single source of truth for "what errors does this input currently
// surface": our own (e.g. the required-but-empty error we produce in
// validate) plus whatever the wrapped input emits through the slot's
// @errors binding. Short-circuited while a missing-state checkbox
// is checked - the wrapped input is locked then and its (now stale)
// errors do not represent the field's state.
const errors = computed<ValidationError[]>(() => {
  if (missingSet.value) {
    return ownErrors.value
  }
  return [...ownErrors.value, ...innerErrors.value].map((error) => (error.el ? error : { ...error, el: firstChildEl() ?? undefined }))
})

const emit = defineEmits<{ errors: [ValidationError[]] }>()
watch(errors, (v) => emit("errors", v), { flush: "sync" })

defineOptions({
  inheritAttrs: false,
})

const { t } = useI18n({ useScope: "global" })

// useLock establishes a lock boundary for the slotted input
// (parentLock + own count, the latter rising while a missing-state
// checkbox is checked).
const lock = useLock()

// We re-provide that bare parentLock via WithLock around the checkbox
// column to keep the checkboxes interactive regardless of our own count.
const parentLock = getParentLock()
function getParentLockRef() {
  return parentLock
}

// Transient "show the required visual" flag. Turned on by validate() when
// the field is required-but-empty (and no missing-state checkbox is
// checked); turned back off on the first interaction (typing in the
// wrapped input or toggling a checkbox) so the red state does not linger
// once the user is acting on it.
const showRequired = ref(false)

function clearShowRequired(): void {
  showRequired.value = false
  ownErrors.value = []
}

// Mutual-exclusion bindings used by the two checkboxes. Checking one
// flips the other off; the underlying defineModels stay independent so
// the parent can observe each one with its own v-model.
const isUnknown = computed<boolean>({
  get: () => unknown.value,
  set: (v) => {
    unknown.value = v
    if (v) none.value = false
    clearShowRequired()
  },
})

const isNone = computed<boolean>({
  get: () => none.value,
  set: (v) => {
    none.value = v
    if (v) unknown.value = false
    clearShowRequired()
  },
})

// Toggle the own lock counter on transitions to/from a checked state.
watch(
  missingSet,
  (locked, wasLocked) => {
    if (locked && !wasLocked) lock.value += 1
    else if (!locked && wasLocked) lock.value -= 1
  },
  { immediate: true, flush: "sync" },
)

// Sub-registry: the wrapped input registers here instead of bubbling up
// to the ancestor form. We proxy its inputs upward as a single
// ValidatedInput that combines its dirty/validate state with our own
// missing-state transitions. Any interaction inside the wrapped input
// also clears our transient required-visual flag.
let forwardInteraction: (() => void) | null = null
const {
  validateAll: validateChildAll,
  resetAll: resetChildAll,
  revertAll: revertChildAll,
  firstInputEl: firstChildEl,
  anyDirty: anyChildDirty,
  allEmpty: allChildEmpty,
  checkpointAll: checkpointChildAll,
} = useValidationRegistry(() => {
  clearShowRequired()
  forwardInteraction?.()
})

// Checkpoints for our own dirty / checkpoint machinery. The wrapped
// input keeps its own checkpoint through the sub-registry.
const unknownCheckpoint = ref<boolean>(unknown.value)
const noneCheckpoint = ref<boolean>(none.value)

const validatedInput: ValidatedInput = {
  validate: async (signal) => {
    // When a missing-state checkbox is checked the wrapped input is
    // locked and its value is intentionally "missing" - skip its
    // validation entirely.
    if (missingSet.value) {
      ownErrors.value = []
      clearShowRequired()
      return
    }
    await validateChildAll(signal)
    if (props.required && allChildEmpty.value) {
      showRequired.value = true
      // TODO: Use standard codes.
      ownErrors.value = [{ code: "required" }]
      return
    }
    ownErrors.value = []
    clearShowRequired()
  },
  reset: () => {
    resetChildAll()
    unknown.value = false
    none.value = false
    clearShowRequired()
  },
  revert: () => {
    revertChildAll()
    unknown.value = unknownCheckpoint.value
    none.value = noneCheckpoint.value
    clearShowRequired()
  },
  inputEl: firstChildEl,
  mainEl: firstChildEl,
  isDirty: computed<boolean>(() => {
    if (unknown.value !== unknownCheckpoint.value || none.value !== noneCheckpoint.value) return true
    return anyChildDirty.value
  }),
  isEmpty: computed<boolean>(() => {
    // "Empty" for InputMissing means there is no value at all: neither
    // missing-state checkbox is checked and the wrapped input has no value
    // either.
    if (missingSet.value) return false
    return allChildEmpty.value
  }),
  errors,
  checkpoint: () => {
    unknownCheckpoint.value = unknown.value
    noneCheckpoint.value = none.value
    checkpointChildAll()
  },
}

const { onInteraction: notifyOuter } = useRegisterForValidation(validatedInput)
forwardInteraction = notifyOuter

defineExpose(validatedInput)

// Trigger validation when focus leaves the entire InputMissing (the
// wrapped input plus the two checkboxes). focusout bubbles, so a single
// handler on the root catches all internal blur events. If the new focus
// target is still inside us, this is just internal navigation and we
// skip. A null relatedTarget (focus moved to body or a non-focusable
// element) is treated as leaving.
const rootRef = useTemplateRef<HTMLDivElement>("rootRef")
async function onFocusOut(event: FocusEvent) {
  const next = event.relatedTarget as Node | null
  if (next && rootRef.value?.contains(next)) return
  await validatedInput.validate()
}
</script>

<template>
  <!-- TODO: Change items-center to items-start once we have dynamic column building. -->
  <div ref="rootRef" class="flex flex-row items-center gap-x-4" @focusout="onFocusOut">
    <div class="flex min-w-0 grow flex-row">
      <slot v-bind="$attrs" :invalid="invalid || showRequired" @errors="(v: ValidationError[]) => (innerErrors = v)" />
    </div>
    <WithLock :lock="getParentLockRef">
      <div class="flex flex-col">
        <label class="flex items-center gap-1 leading-5"
          ><CheckBox v-model="isUnknown" :invalid="invalid || showRequired" /><span>{{ t("common.values.unknown") }}</span></label
        >
        <label class="flex items-center gap-1 leading-5"
          ><CheckBox v-model="isNone" :invalid="invalid || showRequired" /><span>{{ t("common.values.none") }}</span></label
        >
      </div>
    </WithLock>
  </div>
</template>
