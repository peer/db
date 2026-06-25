<script setup lang="ts">
import { computed } from "vue"

import { amountRangeDisplay } from "@/utils"

const props = defineProps<{
  // Range bounds.
  from: number
  to: number
}>()

// Both edges are rounded to a decimal precision derived from the span between them; the title keeps the raw value.
const range = computed(() => amountRangeDisplay(props.from, props.to))
</script>

<template>
  <span :title="String(from)">{{ range.from }}</span>
  –
  <span :title="String(to)">{{ range.to }}</span>
</template>
