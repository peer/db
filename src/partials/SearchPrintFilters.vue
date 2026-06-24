<script setup lang="ts">
import type { Filter } from "@/types"
import type { DeepReadonly } from "vue"

import { useI18n } from "vue-i18n"

import DocumentRefInline from "@/partials/DocumentRefInline.vue"
import FilterPropLabel from "@/partials/FilterPropLabel.vue"
import RefFilterValues from "@/partials/RefFilterValues.vue"
import TimeRange from "@/partials/TimeRange.vue"
import { listFormatParts } from "@/utils"

// The session's active filters, rendered as a plain readable list for the print layout.
defineProps<{
  filters: DeepReadonly<Filter[]>
}>()

const { locale, t } = useI18n({ useScope: "global" })

// A has filter's properties interleaved with the locale's list separators (via Intl.ListFormat): each entry
// is either a separator to print or a property id to render. The properties are OR-ed by the filter, so they
// are listed as a disjunction (in English "a, b, or c").
function hasValueParts(values: readonly { id: string }[]): Array<{ separator: string } | { id: string }> {
  return listFormatParts(locale.value, values.length, "disjunction").map((part) => (part.type === "literal" ? { separator: part.value } : { id: values[part.index].id }))
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
          <template #label>
            <i18n-t v-if="filter.amount.unit" keypath="common.labelWithUnit" scope="global">
              <template #label><FilterPropLabel :prop-ids="filter.prop" :link="false" /></template>
              <template #unit><DocumentRefInline :id="filter.amount.unit" :link="false" /></template>
            </i18n-t>
            <FilterPropLabel v-else :prop-ids="filter.prop" :link="false" />
          </template>
          <!-- A filter that is neither missing nor exists is a range, so gte and lte are both set. The gte check narrows the type past the empty clear payload. -->
          <template #values>
            <i v-if="filter.amount.missing">{{ t("common.values.missing") }}</i>
            <i v-else-if="filter.amount.exists">{{ t("common.values.exists") }}</i>
            <template v-else-if="filter.amount.gte != null">{{ filter.amount.gte }} – {{ filter.amount.lte }}</template>
          </template>
        </i18n-t>
        <i18n-t v-else-if="'time' in filter" keypath="common.labelWithValues" scope="global">
          <template #label><FilterPropLabel :prop-ids="filter.prop" :link="false" /></template>
          <template #values>
            <i v-if="filter.time.missing">{{ t("common.values.missing") }}</i>
            <i v-else-if="filter.time.exists">{{ t("common.values.exists") }}</i>
            <TimeRange v-else-if="filter.time.gte != null" :from="filter.time.gte" :to="filter.time.lte" />
          </template>
        </i18n-t>
        <i18n-t v-else-if="'has' in filter && filter.has.props && filter.has.props.length > 0" keypath="common.labelWithValues" scope="global">
          <template #label><FilterPropLabel :prop-ids="filter.prop" :link="false" /></template>
          <template #values>
            <template v-for="(part, i) in hasValueParts(filter.has.props)" :key="i">
              <template v-if="'separator' in part">{{ part.separator }}</template>
              <DocumentRefInline v-else :id="part.id" :link="false" />
            </template>
          </template>
        </i18n-t>
        <FilterPropLabel v-else :prop-ids="filter.prop" :link="false" />
      </li>
    </ul>
  </div>
</template>
