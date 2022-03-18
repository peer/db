<script setup lang="ts">
defineProps({
  progress: {
    type: Boolean,
    default: false,
  },
})
</script>

<template>
  <input
    :readonly="progress"
    type="text"
    class="mx-4 w-full max-w-2xl rounded border-0 bg-white shadow-sm ring-2 ring-neutral-300 read-only:bg-gray-100 read-only:text-gray-800 hover:ring-neutral-400 read-only:hover:ring-neutral-300 focus:ring-2 focus:ring-primary-500 read-only:focus:border-primary-300 read-only:focus:ring-2 read-only:focus:ring-primary-300 sm:w-4/5 md:w-2/3 lg:w-1/2"
  />
</template>
