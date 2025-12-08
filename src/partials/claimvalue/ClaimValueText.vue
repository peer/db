<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { TextClaim } from "@/document"

defineProps<{
  claim: TextClaim | DeepReadonly<TextClaim> | null
}>()
</script>

<template>
  <div v-if="claim" class="prose prose-slate max-w-none" v-html="claim.html?.en"></div>
</template>
