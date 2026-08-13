<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { AmountClaim } from "@/document"

defineProps<{
  claim: AmountClaim | DeepReadonly<AmountClaim> | null
}>()
</script>

<template>
  <template v-if="claim"
    ><span class="pd-claimvalueamount">{{ claim.amount }}</span></template
  >
</template>
