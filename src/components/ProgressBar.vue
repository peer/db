<script setup lang="ts">
import { computed, ref, watch } from "vue"

const props = withDefaults(
  defineProps<{
    progress: number
    total?: number | null
  }>(),
  {
    total: null,
  },
)

// total === null/undefined selects indeterminate mode.
// total === number selects determinate mode where the fill width is progress / total.
const determinate = computed(() => props.total != null)

const percent = computed(() => {
  if (!determinate.value || !props.total) {
    return 0
  }
  return Math.min(100, (props.progress / props.total) * 100)
})

// Skip the width transition when the determinate bar resets to 0% (e.g.
// between phases). Without this, the bar visibly animates back to the
// start; with it, the reset snaps back.
const skipTransition = ref(false)
watch(
  percent,
  (v, prev) => {
    if (v === 0 && prev !== undefined && prev > 0) {
      skipTransition.value = true
    } else {
      skipTransition.value = false
    }
  },
  {
    flush: "pre",
  },
)
</script>

<template>
  <!-- Determinate: rendered whenever total is provided. Indeterminate: rendered only while progress > 0. -->
  <div v-if="determinate || progress > 0" v-tw-merge class="pd-progressbar relative h-1 w-full overflow-hidden">
    <div
      v-if="determinate"
      class="absolute inset-y-0 left-0 bg-secondary-400 motion-safe:transition-all motion-safe:duration-300"
      :style="{ width: percent + '%', transition: skipTransition ? 'none' : undefined }"
    />
    <template v-else>
      <div class="pd-progressbar-long absolute inset-0 bg-secondary-400 motion-safe:right-full"></div>
      <div class="pd-progressbar-short absolute inset-0 bg-secondary-400 motion-safe:right-full"></div>
    </template>
  </div>
</template>
