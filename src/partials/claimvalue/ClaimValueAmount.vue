<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { AmountClaim } from "@/document"

import { formatValue } from "@/utils"

defineProps<{
  claim: AmountClaim | DeepReadonly<AmountClaim> | null
}>()
</script>

<template>
  <template v-if="claim">{{ formatValue(claim.amount, claim.unit) }}</template>
</template>
