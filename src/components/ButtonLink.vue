<script setup lang="ts">
import { computed } from "vue"
import { useLink } from "vue-router"

const props = defineProps({
  to: {
    type: [String, Object],
    required: true,
  },
  replace: {
    type: Boolean,
    default: false,
  },
  disabled: {
    type: Boolean,
    default: false,
  },
})

// We use fake "/" when disabled. The link is not really active then, so that is OK.
// We have to make both be computed to retain reactivity.
const { navigate, href } = useLink({
  to: computed(() => (props.disabled ? "/" : props.to)),
  replace: computed(() => props.replace),
})
</script>

<template>
  <div
    v-if="disabled"
    class="cursor-not-allowed select-none rounded bg-primary-300 px-6 py-2.5 font-medium uppercase leading-tight text-gray-100 shadow outline-none hover:bg-primary-300 focus:ring-2 focus:ring-primary-300 focus:ring-offset-1 active:bg-primary-300"
  >
    <slot />
  </div>
  <a
    v-else
    :href="href"
    class="select-none rounded bg-primary-600 px-6 py-2.5 font-medium uppercase leading-tight text-white shadow outline-none hover:bg-primary-700 focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 active:bg-primary-500"
    @click="navigate"
  >
    <slot />
  </a>
</template>
