<!--
We do not use :read-only or :disabled pseudo classes to style the component because
we want component to retain how it visually looks even if DOM element is read-only or
disabled attributes are set, unless they are set through component's props.
This is used during transitions/animations to disable the component by directly setting
its DOM attributes without flickering how the component looks.
-->

<script setup lang="ts">
import ProgressBar from "@/components/ProgressBar.vue"

withDefaults(
  defineProps<{
    progress?: number
    disabled?: boolean
    primary?: boolean
  }>(),
  {
    progress: 0,
    disabled: false,
    primary: false,
  },
)
</script>

<template>
  <button
    :disabled="progress > 0 || disabled"
    class="relative select-none rounded font-medium uppercase leading-tight shadow outline-none focus:ring-2 focus:ring-offset-1 text-center"
    :class="{
      'cursor-not-allowed': progress > 0 || disabled,
      'px-6 py-2.5 ': primary,
      'px-[calc(1.5rem_-_2px)] py-[calc(0.625rem_-_2px)]': !primary,
      'bg-primary-300 text-gray-100': primary && (progress > 0 || disabled),
      'bg-primary-600 text-white hover:bg-primary-700 focus:ring-primary-500 active:bg-primary-500': primary && progress === 0 && !disabled,
      'border-neutral-300 border-2 bg-gray-100 text-gray-800 shadow-none': !primary && (progress > 0 || disabled),
      'border-primary-600 border-2 text-primary-600 hover:bg-primary-50 hover:border-primary-700 hover:text-primary-700 focus:ring-primary-500 active:border-primary-500 active:text-primary-500 active:bg-primary-100':
        !primary && progress === 0 && !disabled,
    }"
  >
    <slot />
    <ProgressBar :progress="progress" class="absolute inset-x-0 bottom-0 rounded-b" />
  </button>
</template>
