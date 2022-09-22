<script setup lang="ts">
import ProgressBar from "@/components/ProgressBar.vue"

defineProps({
  progress: {
    type: Number,
    default: 0,
  },
  disabled: {
    type: Boolean,
    default: false,
  },
})
</script>

<template>
  <button
    :disabled="progress > 0 || disabled"
    class="relative select-none rounded px-6 py-2.5 font-medium uppercase leading-tight shadow outline-none focus:ring-2 focus:ring-offset-1"
    :class="
      progress > 0 || disabled
        ? 'cursor-not-allowed bg-primary-300 text-gray-100 hover:bg-primary-300 focus:ring-primary-300 active:bg-primary-300'
        : 'bg-primary-600 text-white hover:bg-primary-700 focus:ring-primary-500 active:bg-primary-500'
    "
  >
    <slot></slot>
    <ProgressBar :progress="progress" class="absolute inset-x-0 bottom-0 rounded-b" />
  </button>
</template>
