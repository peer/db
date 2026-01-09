<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { AmountRangeClaim } from "@/document"

import { formatRange } from "@/utils";

defineProps<{
  claim: AmountRangeClaim | DeepReadonly<AmountRangeClaim> | null
}>()
</script>

<template>
  <template v-if="claim"
    >{{ formatRange(claim.lower, claim.upper, claim.unit) }}</template
  >
</template>
