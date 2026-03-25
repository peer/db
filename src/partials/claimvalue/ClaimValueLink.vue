<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { LinkClaim } from "@/document"

defineProps<{
  claim: LinkClaim | DeepReadonly<LinkClaim> | null
}>()
</script>

<template>
  <a v-if="claim" :href="claim.iri" class="link break-all">{{ claim.iri }}</a>
</template>
