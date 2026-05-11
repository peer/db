<!--
We paint the non-primary variant's outline with an inset ring (box-shadow) instead of
a CSS border so that both variants share the same padding and outer dimensions. A real
border participates in layout, and at fractional device pixel ratios (e.g. 125% display
scaling) the browser snaps border-width down to the nearest device pixel - rendering
2px as 1.6 CSS px and making the non-primary button shorter than the primary one. An
inset ring is paint-only, so the outer box stays identical regardless of DPR.
-->

<script setup lang="ts">
import type { RouteLocationRaw } from "vue-router"

import { toRef } from "vue"
import { useLink } from "vue-router"

const props = withDefaults(
  defineProps<{
    to: RouteLocationRaw
    replace?: boolean
    disabled?: boolean
    primary?: boolean
  }>(),
  {
    replace: false,
    disabled: false,
    primary: false,
  },
)

// We use fake "/" when disabled. The link is not really active then, so that is OK.
// We have to make both be computed to retain reactivity.
//
// eslint-disable-next-line @typescript-eslint/unbound-method
const { navigate, href } = useLink({
  to: toRef(() => (props.disabled ? "/" : props.to)),
  replace: toRef(() => props.replace),
})
</script>

<template>
  <div
    v-if="disabled"
    class="pd-buttonlink relative rounded-sm px-6 py-2.5 text-center leading-tight font-medium uppercase shadow-sm outline-none select-none focus:ring-2 focus:ring-offset-1"
    :class="{
      'cursor-not-allowed': disabled,
      'bg-primary-300 text-gray-100': primary && disabled,
      'bg-primary-600 text-white hover:bg-primary-700 focus:ring-primary-500 active:bg-primary-500': primary && !disabled,
      'bg-gray-100 text-gray-800 shadow-none inset-ring-2 inset-ring-neutral-300': !primary && disabled,
      'text-primary-600 inset-ring-2 inset-ring-primary-600 hover:bg-primary-50 hover:text-primary-700 hover:inset-ring-primary-700 focus:ring-primary-500 active:bg-primary-100 active:text-primary-500 active:inset-ring-primary-500':
        !primary && !disabled,
    }"
  >
    <slot />
  </div>
  <a
    v-else
    :href="href"
    class="pd-buttonlink relative rounded-sm px-6 py-2.5 text-center leading-tight font-medium uppercase shadow-sm outline-none select-none focus:ring-2 focus:ring-offset-1"
    :class="{
      'cursor-not-allowed': disabled,
      'bg-primary-300 text-gray-100': primary && disabled,
      'bg-primary-600 text-white hover:bg-primary-700 focus:ring-primary-500 active:bg-primary-500': primary && !disabled,
      'bg-gray-100 text-gray-800 shadow-none inset-ring-2 inset-ring-neutral-300': !primary && disabled,
      'text-primary-600 inset-ring-2 inset-ring-primary-600 hover:bg-primary-50 hover:text-primary-700 hover:inset-ring-primary-700 focus:ring-primary-500 active:bg-primary-100 active:text-primary-500 active:inset-ring-primary-500':
        !primary && !disabled,
    }"
    @click="navigate"
  >
    <slot />
  </a>
</template>
