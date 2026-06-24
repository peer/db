<script setup lang="ts">
import { computed } from "vue"

import TimeDisplay from "@/partials/TimeDisplay.vue"
import { timeRangeDisplay } from "@/utils"

const props = defineProps<{
  // Range bounds as float64 unix-second timestamps.
  from: number
  to: number
}>()

// Both edges share one precision derived from the span between the bounds, so they line up visually.
const range = computed(() => timeRangeDisplay(props.from, props.to))
</script>

<template>
  <TimeDisplay :timestamp="range.from" :precision="range.precision" />
  –
  <TimeDisplay :timestamp="range.to" :precision="range.precision" />
</template>
