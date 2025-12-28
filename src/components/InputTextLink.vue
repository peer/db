<!--
This component uses @tailwindcss/forms style for input text field and applies it to
a link. Then we add our own site style InputText.vue on top to make the link
look the same as InputText.vue.
-->

<script setup lang="ts">
import type { RouteLocationRaw } from "vue-router"

withDefaults(
  defineProps<{
    to: RouteLocationRaw
    replace?: boolean
    disabled?: boolean
    afterClick?: () => void | Promise<void>
  }>(),
  {
    replace: false,
    disabled: false,
    afterClick: undefined,
  },
)
</script>

<template>
  <RouterLink
    :to="to"
    :replace="replace"
    :disabled="disabled"
    :after-click="afterClick"
    class="appearance-none border-gray-500 px-3 py-2 text-base focus:border-blue-600"
    :class="{
      'text-left outline-none': true, // Override default @tailwindcss/forms style.
      'rounded-sm border-0 shadow-sm ring-2 ring-neutral-300 focus:ring-2': true, // InputText.vue style.
      'cursor-not-allowed': disabled, // InputText.vue readonly style.
      'bg-gray-100 text-gray-800 hover:ring-neutral-300 focus:border-primary-300 focus:ring-primary-300': disabled, // InputText.vue readonly style.
      'bg-white hover:ring-neutral-400 focus:ring-primary-500': !disabled, // InputText.vue non-readonly style.
    }"
  >
    <slot />
  </RouterLink>
</template>
