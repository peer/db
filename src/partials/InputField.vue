<script setup lang="ts">
import type { ComponentPublicInstance, ShallowUnwrapRef } from "vue"

import type { InputColumn, ValidatedInput } from "@/types"

import { computed, shallowRef, useId } from "vue"
import { useI18n } from "vue-i18n"

import InputBadges from "@/partials/InputBadges.vue"
import { pickErrorMessage } from "@/validation"

const props = defineProps<{
  required?: boolean
  // Presentational override.
  invalid?: boolean
  // When set, overrides the first label coming from the wrapped input.
  label?: string
  // Id of an external label element that names this input. Used for
  // aria-labelledby when the input renders no labels of its own (a bare
  // single-column input whose label lives elsewhere, e.g. FieldsForm's left
  // cell or a sub-field's label-above).
  labelledby?: string
  // Suppress the whole-input changed/revert + required badge on the first label.
  hideBadge?: boolean
  // When set, the badge's revert invokes this instead of restoring the wrapped input's
  // own checkpoints. ClaimInput passes its per-bound revert through FieldsFormRow for
  // the interval bounds, so the badge behaves like the field-level revert - posting
  // the reverting changes right away - rather than a local-only restore which would
  // stay uncommitted until the next blur and leave the claim-level changed badges
  // standing.
  revert?: () => void
}>()

const { t } = useI18n({ useScope: "global" })

const input = shallowRef<ShallowUnwrapRef<ValidatedInput> | null>(null)

// The parameter is typed against Vue's VNodeRef signature so the function
// can be spread via v-bind="inputProps" onto any component without TS
// narrowing complaints. At runtime the consumer's input is a validated
// component instance whose defineExpose makes its ValidatedInput shape
// available with refs auto-unwrapped (ShallowUnwrapRef).
function setInputRef(i: Element | ComponentPublicInstance | null) {
  input.value = i as ShallowUnwrapRef<ValidatedInput> | null
}

// The columns the wrapped input declares (label + focusable el, one per grid
// column). An input that does not declare any is treated as a single unlabeled
// column.
const columns = computed<InputColumn[]>(() => input.value?.columns ?? [{ label: "", el: () => input.value?.inputEl() ?? null }])

// The first column's label is overridden by the label prop when it is set.
const displayColumns = computed<InputColumn[]>(() => columns.value.map((col, i) => (i === 0 && props.label !== undefined ? { ...col, label: props.label } : col)))

const columnCount = computed<number>(() => displayColumns.value.length)

// The label row (and the whole-input badge) is shown only when at least one
// column has a label. A bare input (no labels) renders just its control and
// errors; its accessible name comes from the labelledby prop instead.
const showLabels = computed<boolean>(() => displayColumns.value.some((col) => col.label !== ""))

// The first column grows to fill the available width; the remaining columns
// (e.g. a precision input, or InputMissing's checkbox column) size to content.
const gridTemplateColumns = computed<string>(() => ["minmax(0,1fr)", ...Array(Math.max(0, columnCount.value - 1)).fill("auto")].join(" "))

const errorId = useId()

// Names the fieldset group via aria-labelledby. Points at the first label's text only,
// so the badges next to it do not leak into the group's accessible name.
const labelId = useId()

const errorMessage = computed<string | null>(() => pickErrorMessage(input.value?.errors ?? [], t))

// The wrapped input's hint lines, shown only when there is no error to show.
const hints = computed<string[]>(() => input.value?.hints ?? [])

// Simulates the click-to-focus behavior of a <label for=...>: a press on a
// column's label text focuses that column's own control. We act on mousedown
// and preventDefault rather than on click so the currently focused control is
// not first blurred to the body (the label is not focusable, so the default
// mousedown would move focus to <body> before our focus() ran). Focusing
// directly means a composite widget sees an internal focus move (relatedTarget
// still inside it) and does not fire its leave-validation, matching how
// tabbing between columns never trips the "required" check.
function onLabelMousedown(event: MouseEvent, col: InputColumn): void {
  const target = event.target as HTMLElement | null
  // We replicate HTML's "interactive content" exception so a press on or
  // inside a focusable descendant (e.g. the InputBadges' revert button) keeps
  // its own behavior instead of also moving focus into the input.
  if (target?.closest("a[href], button, input, select, textarea, details, [tabindex]:not([tabindex='-1'])")) return
  event.preventDefault()
  col.el()?.focus()
}

// Revert the input's pending edit and then return focus to the input. The revert prop,
// when set, takes over from the local checkpoint restore - see its comment.
function onRevert(): void {
  if (props.revert) {
    props.revert()
  } else {
    input.value?.revert()
  }
  input.value?.inputEl()?.focus()
}
</script>

<template>
  <!--
    The wrapped input renders one top-level element per grid column (signaled by
    its labels). This fieldset is the grid: row 1 holds the labels, row 2 the
    controls, and the last row the error message (or, when there is none, the
    hint). items-start keeps every column aligned at the top regardless of how
    tall any single column's control is.
  -->
  <fieldset v-tw-merge class="grid items-start gap-x-4" :style="{ gridTemplateColumns }" :aria-labelledby="showLabels ? labelId : labelledby || undefined">
    <template v-if="showLabels">
      <div v-for="(col, i) in displayColumns" :key="i" class="mb-1 flex flex-row flex-wrap items-center gap-1">
        <span v-if="col.label" :id="i === 0 ? labelId : undefined" class="cursor-pointer leading-none" @mousedown="onLabelMousedown($event, col)">{{ col.label }}</span>
        <InputBadges v-if="i === 0 && !hideBadge" :required="required" :changed="input?.isDirty ?? false" @revert="onRevert" />
      </div>
    </template>
    <!--
      Single column: a one-column grid so the control stretches to fill the
      width. Multiple columns: display:contents so the input's top-level
      elements become direct grid items of this fieldset, each in its column.
    -->
    <div :class="columnCount > 1 ? 'contents' : 'grid grid-cols-[minmax(0,1fr)]'">
      <slot :ref="setInputRef" name="input" :required="required" :invalid="invalid" :aria-describedby="errorMessage ? errorId : undefined" />
    </div>
    <p v-if="errorMessage" :id="errorId" class="col-span-full mt-1 text-sm text-error-600">{{ errorMessage }}</p>
    <template v-else>
      <p v-for="(hint, i) in hints" :key="i" class="col-span-full mt-1 text-sm text-neutral-500 italic">{{ hint }}</p>
    </template>
  </fieldset>
</template>
