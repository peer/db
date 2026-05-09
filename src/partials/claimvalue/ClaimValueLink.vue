<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { LinkClaim } from "@/document"

import Link from "@/components/Link.vue"

defineProps<{
  claim: LinkClaim | DeepReadonly<LinkClaim> | null
}>()
</script>

<template>
  <Link v-if="claim" :iri="claim.iri" />
</template>
