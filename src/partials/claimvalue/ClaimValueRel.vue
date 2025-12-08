<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { RelationClaim } from "@/document"

import DocumentRefInline from "@/partials/DocumentRefInline.vue"

defineProps<{
  claim: RelationClaim | DeepReadonly<RelationClaim> | null
}>()
</script>

<template>
  <DocumentRefInline :id="claim?.to.id || null" />
</template>
