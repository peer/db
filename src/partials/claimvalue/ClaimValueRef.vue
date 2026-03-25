<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { ReferenceClaim } from "@/document"

import DocumentRefInline from "@/partials/DocumentRefInline.vue"

defineProps<{
  claim: ReferenceClaim | DeepReadonly<ReferenceClaim> | null
}>()
</script>

<template>
  <DocumentRefInline :id="claim?.to.id || null" />
</template>
