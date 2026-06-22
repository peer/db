<!--
A wrapper that styles its rendered element to look like a button.

We do not use :read-only or :disabled pseudo classes to style the component because
we want component to retain how it visually looks even if DOM element's read-only or
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
import type { Component } from "vue"

withDefaults(
  defineProps<{
    as: string | Component
    inactive?: boolean
    primary?: boolean
    active?: boolean
    invalid?: boolean
    // focusWithin opts the component into picking up the focused ring when a
    // descendant is focused, not just when the rendered element itself is
    // focused.
    focusWithin?: boolean
  }>(),
  {
    inactive: false,
    primary: false,
    active: false,
    invalid: false,
    focusWithin: false,
  },
)

defineOptions({
  inheritAttrs: false,
})
</script>

<template>
  <component
    :is="as"
    v-tw-merge
    v-bind="$attrs"
    class="pd-buttonstyled relative rounded-sm px-6 py-2.5 text-center leading-tight font-medium whitespace-nowrap uppercase shadow-sm outline-none select-none focus:ring-2 focus:ring-offset-1 navbar:min-w-[calc(--spacing(5)_+_--spacing(3.5)*2)] navbar:overflow-hidden navbar:px-0"
    :class="{
      'cursor-not-allowed': inactive,
      'bg-primary-300 text-gray-100': primary && inactive,
      'bg-primary-600 text-white hover:bg-primary-700 focus:ring-primary-500 active:bg-primary-500': primary && !inactive && !active && !invalid,
      'bg-primary-500 text-white focus:ring-primary-500': primary && !inactive && active && !invalid,
      'bg-error-600 text-white hover:bg-error-700 focus:ring-error-500 active:bg-error-500': primary && !inactive && !active && invalid,
      'bg-error-500 text-white focus:ring-error-500': primary && !inactive && active && invalid,
      'bg-gray-100 text-gray-800 shadow-none inset-ring-2 inset-ring-neutral-300': !primary && inactive,
      'bg-white text-primary-600 inset-ring-2 inset-ring-primary-600 hover:bg-primary-50 hover:text-primary-700 hover:inset-ring-primary-700 focus:ring-primary-500 active:bg-primary-100 active:text-primary-500 active:inset-ring-primary-500':
        !primary && !inactive && !active && !invalid,
      'bg-primary-100 text-primary-500 inset-ring-2 inset-ring-primary-500 focus:ring-primary-500': !primary && !inactive && active && !invalid,
      'bg-error-50 text-error-600 inset-ring-2 inset-ring-error-600 hover:bg-error-100 hover:text-error-700 hover:inset-ring-error-700 focus:ring-error-500 active:bg-error-200 active:text-error-500 active:inset-ring-error-500':
        !primary && !inactive && !active && invalid,
      'bg-error-200 text-error-500 inset-ring-2 inset-ring-error-500 focus:ring-error-500': !primary && !inactive && active && invalid,
      'focus-within:ring-2 focus-within:ring-offset-1': focusWithin,
      'focus-within:ring-primary-500': focusWithin && !inactive && !invalid,
      'focus-within:ring-error-500': focusWithin && !inactive && invalid,
    }"
  >
    <!--
      The label is wrapped so that inside the navbar (navbar: variant) it becomes a block whose
      trailing edge is faded out when the label does not fit. Its px-3.5 hosts the fade, so it
      stays invisible until the label overflows. The button's navbar floor (min-width) is derived
      from that same px-3.5 plus a size-5 icon, so a fully shrunk button still shows an icon's worth
      of width. Everywhere else the wrapper is display: contents and thus layout neutral, so button
      content (including flex layouts) behaves like there is no wrapper.
    -->
    <span class="pd-buttonstyled-label contents navbar:block navbar:[mask-image:linear-gradient(to_right,black_calc(100%_-_--spacing(3.5)),transparent)] navbar:px-3.5"
      ><slot
    /></span>
  </component>
</template>
