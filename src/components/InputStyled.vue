<!--
A wrapper that styles its rendered element to look like text input element.

We do not use :read-only or :disabled pseudo classes to style the component because
we want component to retain how it visually looks even if DOM element's read-only or
disabled attributes are set, unless they are set through component's props.
This is used during transitions/animations to disable the component by directly setting
its DOM attributes without flickering how the component looks.
-->

<script setup lang="ts">
import type { Component } from "vue"

withDefaults(
  defineProps<{
    as: string | Component
    inactive?: boolean
    invalid?: boolean
    // focusWithin opts the component into picking up the focused look when a
    // descendant is focused, not just when the rendered element itself is
    // focused.
    focusWithin?: boolean
  }>(),
  {
    inactive: false,
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
    class="pd-inputstyled rounded-sm border-none shadow-sm ring-2 ring-neutral-300 focus:ring-2"
    :class="{
      // We have these classes here set with 'true' to make it clear what is needed
      // to style non-input elements to look like input elements.
      // They are redundant for input elements but we set them always for simplicity.
      // This includes what @tailwindcss/forms applies to inputs (appearance-none,
      // py-2/px-3, text-base). Then text-left overrides button's default
      // center alignment; outline-none neutralizes native focus outlines on
      // button/anchor (we draw our own via focus:ring-*). Tailwind's preflight
      // already inherits color and text-decoration on anchors, so no extra resets needed.
      'appearance-none px-3 py-2 text-left text-base outline-none': true,
      'cursor-not-allowed': inactive,
      // inactive wins over invalid for the background: when the user
      // cannot act on the input, red is misleading - we want the
      // disabled look. Active inputs still surface invalid as red.
      'bg-gray-100': inactive,
      'bg-white': !invalid && !inactive,
      'bg-error-50': invalid && !inactive,
      'text-gray-800': inactive,
      // Default (focusWithin off): the rendered element is itself the
      // focus target, so a single focus: variant is enough; focus wins
      // over hover at equal CSS specificity because Tailwind orders
      // focus rules after hover.
      'hover:ring-neutral-300 focus:ring-primary-300': inactive && !focusWithin,
      'hover:ring-neutral-400 focus:ring-primary-500': !inactive && !focusWithin,
      // focusWithin on: also paint the focused look when a descendant is
      // focused. Hover is gated behind not-focus-within: because focus-within
      // does not reliably win over hover at equal specificity in Tailwind's
      // output. Without the gate, hovering a focused composite control
      // would paint the hover ring color over the focused ring.
      'focus-within:ring-2 focus-within:ring-primary-300 not-focus-within:hover:ring-neutral-300 focus:ring-primary-300': inactive && focusWithin,
      'focus-within:ring-2 focus-within:ring-primary-500 not-focus-within:hover:ring-neutral-400 focus:ring-primary-500': !inactive && focusWithin,
    }"
  >
    <slot />
  </component>
</template>
