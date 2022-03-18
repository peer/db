<script setup lang="ts">
defineProps({
  progress: {
    type: Number,
    default: 0,
  },
})
</script>

<template>
  <div v-if="progress > 0" class="h-1 w-full overflow-hidden">
    <div :style="{ width: (1 / progress) * 100 + '%' }" class="relative h-full overflow-hidden">
      <div class="progress-bar-long absolute inset-0 bg-secondary-300" style="right: 100%"></div>
      <div class="progress-bar-short absolute inset-0 bg-secondary-300" style="right: 100%"></div>
    </div>
  </div>
</template>
