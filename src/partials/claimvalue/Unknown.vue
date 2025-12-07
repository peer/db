<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { UnknownValueClaim } from "@/document"

defineProps<{
  claim: UnknownValueClaim | DeepReadonly<UnknownValueClaim> | null
}>()
</script>

<template>
  <template v-if="claim">unknown</template>
</template>
