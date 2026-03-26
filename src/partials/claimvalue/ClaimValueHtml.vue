<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { HTMLClaim } from "@/document"

defineProps<{
  claim: HTMLClaim | DeepReadonly<HTMLClaim> | null
}>()
</script>

<template>
  <!-- eslint-disable-next-line vue/no-v-html -->
  <div v-if="claim" class="prose max-w-none prose-slate" v-html="claim.html"></div>
</template>
