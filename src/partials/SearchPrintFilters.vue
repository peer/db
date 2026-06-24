<script setup lang="ts">
import type { Filter } from "@/types"
import type { DeepReadonly } from "vue"

import DocumentRefInline from "@/partials/DocumentRefInline.vue"
import FilterPropLabel from "@/partials/FilterPropLabel.vue"
import RefFilterValues from "@/partials/RefFilterValues.vue"

// The session's active filters, rendered as a plain readable list for the print layout.
defineProps<{
  filters: DeepReadonly<Filter[]>
}>()

function formatTime(seconds: number): string {
  return new Date(seconds * 1000).toLocaleString()
}
</script>

<template>
  <div v-if="filters.length > 0" class="pd-searchprintfilters">
    <ul class="list-disc pl-6">
      <li v-for="filter in filters" :key="filter.id">
        <RefFilterValues v-if="'ref' in filter" :ref-filter="filter.ref" :link="false">
          <FilterPropLabel :prop-ids="filter.prop" :link="false" />
        </RefFilterValues>
        <i18n-t v-else-if="'amount' in filter" keypath="common.labelWithValues" scope="global">
          <template #label><FilterPropLabel :prop-ids="filter.prop" :link="false" /></template>
          <template #values
            >{{ filter.amount.gte ?? "" }} - {{ filter.amount.lte ?? "" }}
            <DocumentRefInline v-if="filter.amount.unit" :id="filter.amount.unit" :link="false" /></template
          >
        </i18n-t>
        <i18n-t v-else-if="'time' in filter" keypath="common.labelWithValues" scope="global">
          <template #label><FilterPropLabel :prop-ids="filter.prop" :link="false" /></template>
          <template #values>{{ filter.time.gte != null ? formatTime(filter.time.gte) : "" }} - {{ filter.time.lte != null ? formatTime(filter.time.lte) : "" }}</template>
        </i18n-t>
        <i18n-t v-else-if="'has' in filter && filter.has.props && filter.has.props.length > 0" keypath="common.labelWithValues" scope="global">
          <template #label><FilterPropLabel :prop-ids="filter.prop" :link="false" /></template>
          <template #values>
            <template v-for="(value, i) in filter.has.props" :key="value.id">
              <template v-if="i > 0">{{ ", " }}</template>
              <DocumentRefInline :id="value.id" :link="false" />
            </template>
          </template>
        </i18n-t>
        <FilterPropLabel v-else :prop-ids="filter.prop" :link="false" />
      </li>
    </ul>
  </div>
</template>
