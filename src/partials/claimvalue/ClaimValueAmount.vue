<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { AmountClaim } from "@/document"

defineProps<{
  claim: AmountClaim | DeepReadonly<AmountClaim> | null
}>()
</script>

<template>
  <template v-if="claim"
    >{{ claim.amount }}<template v-if="claim.unit !== '1'"> {{ claim.unit }}</template></template
  >
</template>
