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
    readonly?: boolean
    modelValue?: string
    type?: string
    invalid?: boolean
  }>(),
  {
    progress: 0,
    readonly: false,
    modelValue: "",
    type: "text",
    invalid: false,
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
    :readonly="progress > 0 || readonly"
    :type="type"
    class="rounded border-0 shadow ring-2 ring-neutral-300 focus:ring-2"
    :class="{
      'cursor-not-allowed': progress > 0 || readonly,
      'bg-gray-100 text-gray-800 hover:ring-neutral-300 focus:border-primary-300 focus:ring-primary-300': progress > 0 || readonly,
      'bg-white hover:ring-neutral-400 focus:ring-primary-500': progress === 0 && !readonly,
      'bg-error-50': invalid,
    }"
  />
</template>
