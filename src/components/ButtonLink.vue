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
const { navigate, href } = useLink({
  to: toRef(() => (props.disabled ? "/" : props.to)),
  replace: toRef(() => props.replace),
})
</script>

<template>
  <div
    v-if="disabled"
    class="relative select-none rounded font-medium uppercase leading-tight shadow outline-none focus:ring-2 focus:ring-offset-1 text-center"
    :class="{
      'cursor-not-allowed': disabled,
      'px-6 py-2.5 ': primary,
      'px-[calc(1.5rem_-_2px)] py-[calc(0.625rem_-_2px)]': !primary,
      'bg-primary-300 text-gray-100': primary && disabled,
      'bg-primary-600 text-white hover:bg-primary-700 focus:ring-primary-500 active:bg-primary-500': primary && !disabled,
      'border-neutral-300 border-2 bg-gray-100 text-gray-800 shadow-none': !primary && disabled,
      'border-primary-600 border-2 text-primary-600 hover:bg-primary-50 hover:border-primary-700 hover:text-primary-700 focus:ring-primary-500 active:border-primary-500 active:text-primary-500 active:bg-primary-100':
        !primary && !disabled,
    }"
  >
    <slot />
  </div>
  <a
    v-else
    :href="href"
    class="relative select-none rounded font-medium uppercase leading-tight shadow outline-none focus:ring-2 focus:ring-offset-1 text-center"
    :class="{
      'cursor-not-allowed': disabled,
      'px-6 py-2.5 ': primary,
      'px-[calc(1.5rem_-_2px)] py-[calc(0.625rem_-_2px)]': !primary,
      'bg-primary-300 text-gray-100': primary && disabled,
      'bg-primary-600 text-white hover:bg-primary-700 focus:ring-primary-500 active:bg-primary-500': primary && !disabled,
      'border-neutral-300 border-2 bg-gray-100 text-gray-800 shadow-none': !primary && disabled,
      'border-primary-600 border-2 text-primary-600 hover:bg-primary-50 hover:border-primary-700 hover:text-primary-700 focus:ring-primary-500 active:border-primary-500 active:text-primary-500 active:bg-primary-100':
        !primary && !disabled,
    }"
    @click="navigate"
  >
    <slot />
  </a>
</template>
