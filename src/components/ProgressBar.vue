<script setup lang="ts">
defineProps({
  progress: {
    type: Number,
    required: true,
  },
})
</script>

<template>
  <div v-if="progress > 0" class="h-1 w-full overflow-hidden">
    <div class="progress-bar-long absolute inset-0 bg-secondary-400 opacity-60" style="left: 0%; right: 100%"></div>
    <div class="progress-bar-short absolute inset-0 bg-secondary-400 opacity-60" style="left: 0%; right: 100%"></div>
  </div>
</template>
