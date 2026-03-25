<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { AmountIntervalClaim } from "@/document"

defineProps<{
  claim: AmountIntervalClaim | DeepReadonly<AmountIntervalClaim> | null
}>()
</script>

<template>
  <!-- TODO: Improve. -->
  <template v-if="claim">{{ claim.from ?? "?" }}–{{ claim.to ?? "?" }}</template>
</template>
