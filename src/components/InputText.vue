<!--
We do not use :read-only or :disabled pseudo classes to style the component because
we want component to retain how it visually looks even if DOM element's read-only or
disabled attributes are set, unless they are set through component's props.
This is used during transitions/animations to disable the component by directly setting
its DOM attributes without flickering how the component looks.
-->

<script setup lang="ts">
withDefaults(
  defineProps<{
    progress?: number
    readonly?: boolean
    type?: string
    invalid?: boolean
  }>(),
  {
    progress: 0,
    readonly: false,
    type: "text",
    invalid: false,
  },
)

const model = defineModel<string>({ default: "" })
</script>

<template>
  <input
    v-model="model"
    :readonly="progress > 0 || readonly"
    :type="type"
    class="rounded-sm border-none shadow-sm ring-2 ring-neutral-300 focus:ring-2"
    :class="{
      'cursor-not-allowed': progress > 0 || readonly,
      'bg-gray-100': !invalid && (progress > 0 || readonly),
      'bg-white': !invalid && progress === 0 && !readonly,
      'bg-error-50': invalid,
      'text-gray-800': progress > 0 || readonly,
      'hover:ring-neutral-300 focus:ring-primary-300': progress > 0 || readonly,
      'hover:ring-neutral-400 focus:ring-primary-500': progress === 0 && !readonly,
    }"
  />
</template>
