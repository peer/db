<!--
We do not use :read-only or :disabled pseudo classes to style the component because
we want component to retain how it visually looks even if DOM element's read-only or
disabled attributes are set, unless they are set through component's props.
This is used during transitions/animations to disable the component by directly setting
its DOM attributes without flickering how the component looks.
-->

<script setup lang="ts">
import { computed } from "vue"

const props = withDefaults(
  defineProps<{
    progress?: number
    disabled?: boolean
    modelValue?: string
  }>(),
  {
    progress: 0,
    disabled: false,
    modelValue: "",
  },
)

const $emit = defineEmits<{
  "update:modelValue": [value: string]
}>()

const v = computed({
  get() {
    return props.modelValue
  },
  set(value: string) {
    $emit("update:modelValue", value)
  },
})
</script>

<template>
  <input
    v-model="v"
    :disabled="progress > 0 || disabled"
    type="radio"
    :class="{
      'cursor-not-allowed bg-gray-100 text-primary-300': progress > 0 || disabled,
      'cursor-pointer text-primary-600 focus:ring-primary-500': progress === 0 && !disabled,
    }"
  />
</template>
