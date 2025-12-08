<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { AmountRangeClaim } from "@/document"

defineProps<{
  claim: AmountRangeClaim | DeepReadonly<AmountRangeClaim> | null
}>()
</script>

<template>
  <template v-if="claim"
    >{{ claim.lower }}–{{ claim.upper }}<template v-if="claim.unit !== '1'"> {{ claim.unit }}</template></template
  >
</template>
