<script setup lang="ts">
defineProps({
  progress: {
    type: Boolean,
    default: false,
  },
})
</script>

<template>
  <button
    :readonly="progress"
    class="mx-4 rounded bg-primary-600 px-6 py-2.5 font-medium uppercase leading-tight text-white shadow-sm outline-none hover:bg-primary-700 focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 active:bg-primary-500"
    :class="progress ? 'cursor-default bg-primary-300 text-gray-100 hover:bg-primary-300 focus:ring-primary-300 active:bg-primary-300' : ''"
  >
    <slot></slot>
  </button>
</template>
