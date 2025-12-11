<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { IdentifierClaim } from "@/document"

defineProps<{
  claim: IdentifierClaim | DeepReadonly<IdentifierClaim> | null
}>()
</script>

<template>
  <template v-if="claim">{{ claim.value }}</template>
</template>
