<script setup lang="ts">
import type { InputColumn, ValidatedInput, ValidationError } from "@/types"

import { computed, onBeforeUnmount, ref, useId, useTemplateRef, watch } from "vue"
import { useI18n } from "vue-i18n"

import CheckBox from "@/components/CheckBox.vue"
import WithInactive from "@/components/WithInactive.vue"
import { getParentInactive, useInactive } from "@/progress"
import { anySignal } from "@/utils"
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
  // Disables the missing-state checkboxes (the wrapped input receives its own readonly
  // binding from the enclosing FieldsFormRow).
  readonly?: boolean
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
// is checked - the wrapped input is inactive then and its (now stale)
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

// useInactive establishes an inactive boundary for the slotted input
// (parent + own count, the latter rising while a missing-state checkbox
// is checked). The mark is a state of the field rather than an operation
// running, so it goes through the inactive channel and not the lock one:
// the input stays inactive for as long as the mark is on it.
const inactive = useInactive()

// We re-provide that bare parent count via WithInactive around the checkbox
// column to keep the checkboxes interactive regardless of our own count,
// which is what leaves a way to take the mark off again.
const parentInactive = getParentInactive()
function getParentInactiveRef() {
  return parentInactive
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

// Toggle the own inactive counter on transitions to/from a checked state.
watch(
  missingSet,
  (marked, wasMarked) => {
    if (marked && !wasMarked) inactive.value += 1
    else if (!marked && wasMarked) inactive.value -= 1
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
  inputs: childInputs,
} = useValidationRegistry(() => {
  clearShowRequired()
  forwardInteraction?.()
})

// Ids on the checkboxes: the first one is the focus target exposed through columns, and
// both are focused explicitly on label click (see the labels in the template).
const unknownCheckboxId = useId()
const noneCheckboxId = useId()

// Restores the focus which the labels' prevented mousedown suppressed (the prevent also
// covers direct checkbox clicks, whose mousedown bubbles through the label). With the
// checkbox focused after a toggle, the widget holds focus, so clicking outside later
// blurs the slot and commits it; without this, focus would sit outside the slot and a
// deselected-and-empty bound would never resolve. A checkbox disabled by an immediate
// commit refuses focus, which is harmless - nothing was blurred either.
function focusCheckbox(id: string): void {
  document.getElementById(id)?.focus()
}

// Every wrapped input's columns (labels and focus targets forwarded as-is), in
// registration order, followed by a trailing unlabeled column for the
// none/unknown checkboxes. A wrapped input that declares no columns contributes
// a single column focusing its own control; with no wrapped input at all (e.g.
// before it mounts) we still render one value column so the grid stays stable.
const columns = computed<InputColumn[]>(() => {
  const wrapped = Array.from(childInputs).flatMap((input) => input.columns?.value ?? [{ label: "", el: () => input.inputEl() ?? null }])
  return [...(wrapped.length ? wrapped : [{ label: "", el: firstChildEl }]), { label: "", el: () => document.getElementById(unknownCheckboxId) }]
})

// Every wrapped input's hint lines, in registration order.
const hints = computed<string[]>(() => Array.from(childInputs).flatMap((input) => input.hints?.value ?? []))

// The contents root spanning the wrapped input plus the checkbox column, used
// as mainEl and by onFocusOut.
const rootRef = useTemplateRef<HTMLDivElement>("rootRef")

// Checkpoints for our own dirty / checkpoint machinery. The wrapped
// input keeps its own checkpoint through the sub-registry.
const unknownCheckpoint = ref<boolean>(unknown.value)
const noneCheckpoint = ref<boolean>(none.value)

// onBeforeUnmount, not onUnmounted: Vue runs a parent's beforeUnmount before its children's, while
// onUnmounted hooks run children-first. The wrapped input's useValidation aborts its own controller in
// its own onBeforeUnmount, so aborting here is what makes this signal already aborted while the wrapped
// input tears down, whatever its validation then does with the abort.
const abortController = new AbortController()
onBeforeUnmount(() => {
  abortController.abort()
})

const validatedInput: ValidatedInput = {
  validate: async (signal, options) => {
    // When a missing-state checkbox is checked the wrapped input is
    // inactive and its value is intentionally "missing" - skip its
    // validation entirely.
    if (missingSet.value) {
      ownErrors.value = []
      clearShowRequired()
      return
    }
    // The caller's signal is combined with ours, never replaced by it: the caller aborts a superseded
    // run, we abort on teardown, and both must cancel the wrapped input's validation.
    const effective = signal ? anySignal(signal, abortController.signal) : abortController.signal
    await validateChildAll(effective, options)
    // A superseded or torn-down validation run must not paint the required state.
    if (effective.aborted) {
      return
    }
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
  // inputEl is the wrapped input's first focusable control; mainEl is the
  // contents root spanning the wrapped input and the checkbox column.
  inputEl: firstChildEl,
  mainEl: () => rootRef.value,
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
  columns,
  hints,
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
async function onFocusOut(event: FocusEvent) {
  const next = event.relatedTarget as Node | null
  if (next && rootRef.value?.contains(next)) return
  await validatedInput.validate()
}
</script>

<template>
  <!--
    display:contents so the wrapped input's columns and our checkbox column
    become direct grid items of the enclosing component.
  -->
  <div ref="rootRef" class="pd-inputmissing contents" @focusout="onFocusOut">
    <slot v-bind="$attrs" :invalid="invalid || showRequired" @errors="(v: ValidationError[]) => (innerErrors = v)" />
    <!--
      The labels prevent mousedown so clicking them does not blur the previously focused
      element first: a label is not focusable, so that blur would report a null
      relatedTarget and the enclosing slot could not tell focus is staying inside - it
      would commit mid-interaction, and the pending read-only flash would then swallow
      the label's forwarded click. The click itself still toggles the checkbox.
    -->
    <WithInactive :inactive="getParentInactiveRef">
      <!--
        items-start keeps each label sized to its own content, so the clickable area
        does not extend past the text (the column is as wide as the widest label).
      -->
      <div class="pd-inputmissing-group flex flex-col items-start">
        <label class="pd-inputmissing-label-unknown flex cursor-pointer items-center gap-1 leading-5" @mousedown.prevent @click="focusCheckbox(unknownCheckboxId)"
          ><CheckBox
            :id="unknownCheckboxId"
            v-model="isUnknown"
            :disabled="readonly"
            :invalid="invalid || showRequired"
            class="pd-inputmissing-checkbox-unknown"
          /><span>{{ t("common.values.unknown") }}</span></label
        >
        <label class="pd-inputmissing-label-none flex cursor-pointer items-center gap-1 leading-5" @mousedown.prevent @click="focusCheckbox(noneCheckboxId)"
          ><CheckBox :id="noneCheckboxId" v-model="isNone" :disabled="readonly" :invalid="invalid || showRequired" class="pd-inputmissing-checkbox-none" /><span>{{
            t("common.values.none")
          }}</span></label
        >
      </div>
    </WithInactive>
  </div>
</template>
