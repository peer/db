<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { HasClaim } from "@/document"

defineProps<{
  claim: HasClaim | DeepReadonly<HasClaim> | null
}>()
</script>

<template>
  <span v-if="claim"></span>
</template>
