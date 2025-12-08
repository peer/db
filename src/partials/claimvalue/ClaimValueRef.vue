<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { ReferenceClaim } from "@/document"

defineProps<{
  claim: ReferenceClaim | DeepReadonly<ReferenceClaim> | null
}>()
</script>

<template>
  <a v-if="claim" :href="claim.iri" class="link break-all">{{ claim.iri }}</a>
</template>
