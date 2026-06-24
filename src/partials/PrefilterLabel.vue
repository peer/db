<script setup lang="ts">
import type { Filter } from "@/types"
import type { DeepReadonly } from "vue"

import FilterPropLabel from "@/partials/FilterPropLabel.vue"
import RefFilterValues from "@/partials/RefFilterValues.vue"

defineProps<{
  filter: DeepReadonly<Filter>
}>()
</script>

<template>
  <span class="pd-prefilter-label">
    <RefFilterValues v-if="'ref' in filter" :ref-filter="filter.ref">
      <FilterPropLabel :prop-ids="filter.prop" />
    </RefFilterValues>
    <FilterPropLabel v-else :prop-ids="filter.prop" />
  </span>
</template>
