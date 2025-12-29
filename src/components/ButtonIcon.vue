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
  }>(),
  {
    progress: 0,
    disabled: false,
  },
)
</script>

<template>
  <button
    :disabled="progress > 0 || disabled"
    class="relative p-0.5 select-none rounded outline-none focus-visible:ring-2 focus-visible:ring-offset-1"
    :class="{
      'cursor-not-allowed': progress > 0 || disabled,
      'bg-neutral-300 text-neutral-400': progress > 0 || disabled,
      'bg-neutral-300 text-gray-800 hover:bg-neutral-200 focus:ring-primary-500 active:bg-neutral-400': progress === 0 && !disabled,
    }"
  >
    <slot />
  </button>
</template>
