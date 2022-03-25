<script setup lang="ts">
import type { PeerDBDocument } from "@/types"

defineProps<{
  doc: PeerDBDocument
}>()
</script>

<template>
  <div>a {{ doc._id }} {{ doc }}</div>
</template>
