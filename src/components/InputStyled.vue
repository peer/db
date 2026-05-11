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
  }>(),
  {
    inactive: false,
    invalid: false,
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
      // py-2/px-3, text-base). Then text-left overrides <button>'s default
      // center alignment; outline-none neutralizes native focus outlines on
      // <button>/<a> (we draw our own via focus:ring-*). Tailwind's preflight
      // already inherits color and text-decoration on <a>, so no extra resets needed.
      'appearance-none px-3 py-2 text-left text-base outline-none': true,
      'cursor-not-allowed': inactive,
      'bg-gray-100': !invalid && inactive,
      'bg-white': !invalid && !inactive,
      'bg-error-50': invalid,
      'text-gray-800': inactive,
      'hover:ring-neutral-300 focus:ring-primary-300': inactive,
      'hover:ring-neutral-400 focus:ring-primary-500': !inactive,
    }"
  >
    <slot />
  </component>
</template>
