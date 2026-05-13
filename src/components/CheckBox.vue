<!--
We do not use :read-only or :disabled pseudo classes to style the component because
we want component to retain how it visually looks even if DOM element's read-only or
disabled attributes are set, unless they are set through component's props.
This is used during transitions/animations to disable the component by directly setting
its DOM attributes without flickering how the component looks.
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
</script>

<template>
  <!-- We wrap input in div to align check box correctly vertically inside the grid. -->
  <div>
    <input
      v-model="model"
      v-tw-merge
      v-bind="$attrs"
      :disabled="inactive()"
      type="checkbox"
      class="pd-checkbox -mt-0.5 rounded-sm align-middle"
      :class="{
        'cursor-not-allowed bg-gray-400 text-primary-300': inactive(),
        'cursor-pointer text-primary-600 focus:ring-primary-500': !inactive(),
      }"
    />
  </div>
</template>
