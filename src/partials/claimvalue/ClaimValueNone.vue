<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { NoValueClaim } from "@/document"

defineProps<{
  claim: NoValueClaim | DeepReadonly<NoValueClaim> | null
}>()
</script>

<template>
  <template v-if="claim"><i>none</i></template>
</template>
