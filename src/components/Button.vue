<!--
We do not use :read-only or :disabled pseudo classes to style the component because
we want component to retain how it visually looks even if DOM element is read-only or
disabled attributes are set, unless they are set through component's props.
This is used during transitions/animations to disable the component by directly setting
its DOM attributes without flickering how the component looks.

We paint the non-primary variant's outline with an inset ring (box-shadow) instead of
a CSS border so that both variants share the same padding and outer dimensions. A real
border participates in layout, and at fractional device pixel ratios (e.g. 125% display
scaling) the browser snaps border-width down to the nearest device pixel - rendering
2px as 1.6 CSS px and making the non-primary button shorter than the primary one. An
inset ring is paint-only, so the outer box stays identical regardless of DPR.
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
    v-tw-merge
    :disabled="progress > 0 || disabled"
    class="pd-button relative rounded-sm px-6 py-2.5 text-center leading-tight font-medium uppercase shadow-sm outline-none select-none focus:ring-2 focus:ring-offset-1"
    :class="{
      'cursor-not-allowed': progress > 0 || disabled,
      'bg-primary-300 text-gray-100': primary && (progress > 0 || disabled),
      'bg-primary-600 text-white hover:bg-primary-700 focus:ring-primary-500 active:bg-primary-500': primary && progress === 0 && !disabled,
      'bg-gray-100 text-gray-800 shadow-none inset-ring-2 inset-ring-neutral-300': !primary && (progress > 0 || disabled),
      'text-primary-600 inset-ring-2 inset-ring-primary-600 hover:bg-primary-50 hover:text-primary-700 hover:inset-ring-primary-700 focus:ring-primary-500 active:bg-primary-100 active:text-primary-500 active:inset-ring-primary-500':
        !primary && progress === 0 && !disabled,
    }"
  >
    <slot />
    <ProgressBar :progress="progress" class="absolute inset-x-0 bottom-0 rounded-b" />
  </button>
</template>
