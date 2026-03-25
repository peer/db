<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { TimeIntervalClaim } from "@/document"

defineProps<{
  claim: TimeIntervalClaim | DeepReadonly<TimeIntervalClaim> | null
}>()
</script>

<template>
  <template v-if="claim">{{ claim.from ?? "?" }}–{{ claim.to ?? "?" }}</template>
</template>
