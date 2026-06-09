<script setup lang="ts">
import type { Filter } from "@/types"
import type { DeepReadonly } from "vue"

import { computed } from "vue"

import DocumentRefInline from "@/partials/DocumentRefInline.vue"

const props = defineProps<{
  filter: DeepReadonly<Filter>
}>()

// Only reference prefilters carry "to" values to list (search shortcuts produce ref prefilters).
// Other filter kinds show just their property path.
const values = computed(() => ("ref" in props.filter && props.filter.ref.to ? props.filter.ref.to : []))
</script>

<template>
  <span class="pd-prefilter-label">
    <template v-for="(prop, i) in filter.prop" :key="prop">
      <template v-if="i > 0">{{ " > " }}</template>
      <DocumentRefInline :id="prop" />
    </template>
    <template v-if="values.length > 0">
      {{ ": " }}
      <template v-for="(value, i) in values" :key="value.id">
        <template v-if="i > 0">{{ ", " }}</template>
        <DocumentRefInline :id="value.id" />
      </template>
    </template>
  </span>
</template>
