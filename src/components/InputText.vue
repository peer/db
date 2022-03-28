<script setup lang="ts">
defineProps({
  progress: {
    type: Number,
    default: 0,
  },
})
</script>

<template>
  <input
    :readonly="progress > 0"
    type="text"
    class="rounded border-0 bg-white shadow-sm ring-2 ring-neutral-300 read-only:bg-gray-100 read-only:text-gray-800 hover:ring-neutral-400 read-only:hover:ring-neutral-300 focus:ring-2 focus:ring-primary-500 read-only:focus:border-primary-300 read-only:focus:ring-2 read-only:focus:ring-primary-300"
    :class="progress > 0 ? 'cursor-not-allowed' : ''"
  />
</template>
