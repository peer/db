<script setup lang="ts">
import type { Filter } from "@/types"
import type { DeepReadonly } from "vue"

import { useI18n } from "vue-i18n"

import DocumentRefInline from "@/partials/DocumentRefInline.vue"

// The session's active filters, rendered as a plain readable list for the print layout.
defineProps<{
  filters: DeepReadonly<Filter[]>
}>()

const { t } = useI18n({ useScope: "global" })

function formatTime(seconds: number): string {
  return new Date(seconds * 1000).toLocaleString()
}
</script>

<template>
  <div v-if="filters.length > 0" class="pd-searchprintfilters">
    <ul class="list-disc pl-6">
      <li v-for="filter in filters" :key="filter.id">
        <template v-for="(prop, i) in filter.prop" :key="prop">
          <template v-if="i > 0">{{ " > " }}</template>
          <DocumentRefInline :id="prop" :link="false" />
        </template>
        <template v-if="'ref' in filter">
          <template v-if="filter.ref.to && filter.ref.to.length > 0">
            {{ ": " }}
            <template v-for="(value, i) in filter.ref.to" :key="value.id">
              <template v-if="i > 0">{{ ", " }}</template>
              <DocumentRefInline :id="value.id" :link="false" />
            </template>
          </template>
          <template v-else-if="filter.ref.missing"
            >{{ ": " }}<i>{{ t("common.values.missing") }}</i></template
          >
        </template>
        <template v-else-if="'amount' in filter"
          >{{ ": " }}{{ filter.amount.gte ?? "" }} - {{ filter.amount.lte ?? "" }}
          <DocumentRefInline v-if="filter.amount.unit" :id="filter.amount.unit" :link="false" />
        </template>
        <template v-else-if="'time' in filter"
          >{{ ": " }}{{ filter.time.gte != null ? formatTime(filter.time.gte) : "" }} - {{ filter.time.lte != null ? formatTime(filter.time.lte) : "" }}</template
        >
        <template v-else-if="'has' in filter && filter.has.props && filter.has.props.length > 0">
          {{ ": " }}
          <template v-for="(value, i) in filter.has.props" :key="value.id">
            <template v-if="i > 0">{{ ", " }}</template>
            <DocumentRefInline :id="value.id" :link="false" />
          </template>
        </template>
      </li>
    </ul>
  </div>
</template>
