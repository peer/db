<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { StringClaim } from "@/document"

defineProps<{
  claim: StringClaim | DeepReadonly<StringClaim> | null
}>()
</script>

<template>
  <template v-if="claim">{{ claim.string }}</template>
</template>
