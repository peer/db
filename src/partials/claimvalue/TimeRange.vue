<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { TimeRangeClaim } from "@/document"

defineProps<{
  claim: TimeRangeClaim | DeepReadonly<TimeRangeClaim> | null
}>()
</script>

<template>
  <template v-if="claim">{{ claim.lower }}-{{ claim.upper }}</template>
</template>
