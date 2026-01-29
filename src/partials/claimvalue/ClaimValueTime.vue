<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { TimeClaim } from "@/document"

import TimeDisplay from "@/components/TimeDisplay.vue"

defineProps<{
  claim: TimeClaim | DeepReadonly<TimeClaim> | null
}>()
</script>

<template>
  <TimeDisplay v-if="claim" :timestamp="claim.time" :precision="claim.precision" />
</template>
