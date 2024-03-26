<!--
We do not use :read-only or :disabled pseudo classes to style the component because
we want component to retain how it visually looks even if DOM element's read-only or
disabled attributes are set, unless they are set through component's props.
This is used during transitions/animations to disable the component by directly setting
its DOM attributes without flickering how the component looks.
-->

<script setup lang="ts" generic="T">
import { computed } from "vue"

const props = withDefaults(
  defineProps<{
    progress?: number
    disabled?: boolean
    modelValue?: T
  }>(),
  {
    progress: 0,
    disabled: false,
    modelValue: undefined,
  },
)

const $emit = defineEmits<{
  "update:modelValue": [value: T]
}>()

const v = computed({
  get() {
    // We use ! operator here to satisfy type constraints and assert that modelValue cannot be undefined,
    // but in fact modelValue can be undefined, but that is handled correctly by Vue's v-model on <input>.
    return props.modelValue!
  },
  set(value: T) {
    $emit("update:modelValue", value)
  },
})
</script>

<template>
  <input
    v-model="v"
    :disabled="progress > 0 || disabled"
    type="checkbox"
    class="rounded"
    :class="{
      'cursor-not-allowed bg-gray-100 text-primary-300': progress > 0 || disabled,
      'cursor-pointer text-primary-600 focus:ring-primary-500': progress === 0 && !disabled,
    }"
  />
</template>
