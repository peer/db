<!--
We do not use :read-only or :disabled pseudo classes to style the component because
we want component to retain how it visually looks even if DOM element's read-only or
disabled attributes are set, unless they are set through component's props.
This is used during transitions/animations to disable the component by directly setting
its DOM attributes without flickering how the component looks.

Clicking on the already-selected radio clears the selection.
-->

<script setup lang="ts" generic="T">
import { useLocked } from "@/progress"

const props = withDefaults(
  defineProps<{
    disabled?: boolean
  }>(),
  {
    disabled: false,
  },
)

const model = defineModel<T>()

const locked = useLocked()
const inactive = () => locked.value || props.disabled

// We want all fallthrough attributes to be passed to the input element.
defineOptions({
  inheritAttrs: false,
})

// At click handler time, vModelRadio's change listener has not yet run, so
// model.value still reflects the pre-click state. If it equals the clicked
// radio's value, the user clicked the already-selected radio - clear the
// model instead of re-selecting.
//
// We read el._value before el.value to support non-string values: Vue's
// patchProp stringifies the DOM value attribute (el.value = String(value))
// but stashes the original on el._value, and vModelRadio's getValue() reads
// _value first for the same reason.
function maybeDeselect(el: HTMLInputElement & { _value?: unknown }): boolean {
  const value = "_value" in el ? el._value : el.value
  if (model.value === value) {
    model.value = undefined
    return true
  }
  return false
}

function onClick(event: MouseEvent) {
  maybeDeselect(event.target as HTMLInputElement & { _value?: unknown })
}

// Chrome does not fire a click event for Space on an already-selected radio
// (Firefox does), so the @click handler alone is not enough for keyboard
// deselect. We mirror the deselect logic on keydown for Space; for an
// unselected radio Space falls through to native selection.
function onKeyDown(event: KeyboardEvent) {
  if (event.key !== " ") return
  if (maybeDeselect(event.target as HTMLInputElement & { _value?: unknown })) {
    event.preventDefault()
  }
}
</script>

<template>
  <!-- We wrap input in div to align radio button correctly vertically inside the grid. -->
  <div>
    <input
      v-model="model"
      v-tw-merge
      v-bind="$attrs"
      :disabled="inactive()"
      type="radio"
      class="pd-radiobutton -mt-0.5 align-middle"
      :class="{
        'cursor-not-allowed bg-gray-400 text-primary-300': inactive(),
        'cursor-pointer text-primary-600 focus:ring-primary-500': !inactive(),
      }"
      @click="onClick"
      @keydown="onKeyDown"
    />
  </div>
</template>
