<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { HTMLClaim } from "@/document"

import { useInternalLinksClick } from "@/internal-links"

defineProps<{
  claim: HTMLClaim | DeepReadonly<HTMLClaim> | null
}>()

const onClick = useInternalLinksClick()
</script>

<template>
  <!-- eslint-disable-next-line vue/no-v-html -->
  <div v-if="claim" class="prose max-w-none prose-slate" @click="onClick" v-html="claim.html"></div>
</template>
