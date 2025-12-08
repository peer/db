<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { TimeClaim } from "@/document"

defineProps<{
  claim: TimeClaim | DeepReadonly<TimeClaim> | null
}>()
</script>

<template>
  <template v-if="claim">{{ claim.timestamp }}</template>
</template>
