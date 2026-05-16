<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { HTMLClaim } from "@/document"

import { computed } from "vue"

import { useInternalLinksClick, useTransformedHtml } from "@/internal-links"

const props = defineProps<{
  claim: HTMLClaim | DeepReadonly<HTMLClaim> | null
}>()

const html = computed(() => props.claim?.html ?? "")
const transformedHtml = useTransformedHtml(html)
const onClick = useInternalLinksClick()
</script>

<template>
  <!-- eslint-disable-next-line vue/no-v-html -->
  <div v-if="claim" class="prose max-w-none prose-gray" @click="onClick" v-html="transformedHtml"></div>
</template>
