<script setup lang="ts">
defineProps<{
  progress: number
}>()
</script>

<template>
  <div v-if="progress > 0" class="pd-progressbar h-1 w-full overflow-hidden">
    <div class="pd-progressbar-long absolute inset-0 bg-secondary-400 motion-safe:right-full"></div>
    <div class="pd-progressbar-short absolute inset-0 bg-secondary-400 motion-safe:right-full"></div>
  </div>
</template>
