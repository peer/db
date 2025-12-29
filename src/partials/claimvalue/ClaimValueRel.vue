<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { RelationClaim } from "@/document"

import DocumentRefInline from "@/partials/DocumentRefInline.vue"

withDefaults(
  defineProps<{
    claim: RelationClaim | DeepReadonly<RelationClaim> | null
    loadingWidth?: "auto" | "fixed"
  }>(),
  {
    loadingWidth: "fixed",
  },
)
</script>

<template>
  <DocumentRefInline :id="claim?.to.id || null" :loading-width="loadingWidth" />
</template>
