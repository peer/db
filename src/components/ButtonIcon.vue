<!--
We do not use :read-only or :disabled pseudo classes to style the component because
we want component to retain how it visually looks even if DOM element is read-only or
disabled attributes are set, unless they are set through component's props.
This is used during transitions/animations to disable the component by directly setting
its DOM attributes without flickering how the component looks.
-->

<script setup lang="ts">
withDefaults(
  defineProps<{
    progress?: number
    disabled?: boolean
    active?: boolean
  }>(),
  {
    progress: 0,
    disabled: false,
    active: false,
  },
)
</script>

<template>
  <button
    :disabled="progress > 0 || disabled"
    class="relative rounded p-0.5 text-center outline-none select-none focus-visible:ring-2 focus-visible:ring-offset-1"
    :class="{
      'cursor-not-allowed bg-primary-300 text-gray-100': progress > 0 || disabled,
      'bg-primary-600 text-white hover:bg-primary-700 focus:ring-primary-500 active:bg-primary-500': progress === 0 && !disabled && active,
      'bg-none text-primary-600 hover:bg-neutral-200 hover:text-primary-700 active:bg-neutral-300 active:text-primary-500': progress === 0 && !disabled && !active,
    }"
  >
    <slot />
  </button>
</template>
