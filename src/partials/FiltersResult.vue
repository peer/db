<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { AmountFilterEntry, Filter, FilterResult, RefFilterEntry, SearchSession, TimeFilterEntry } from "@/types"

import { onBeforeUnmount } from "vue"

import AmountFiltersResult from "@/partials/AmountFiltersResult.vue"
import RefFiltersResult from "@/partials/RefFiltersResult.vue"
import TimeFiltersResult from "@/partials/TimeFiltersResult.vue"

const props = defineProps<{
  result: FilterResult
  searchSession: DeepReadonly<SearchSession>
  searchTotal: number
  filters: Filter[]
}>()

const $emit = defineEmits<{
  filterUpdate: [filterId: string, filter: Filter]
}>()

// We have to explicitly pass attributes because we use multiple root nodes.
defineOptions({
  inheritAttrs: false,
})

const abortController = new AbortController()

onBeforeUnmount(() => {
  abortController.abort()
})

// Find the active filter by filterId. Returns undefined for inactive filters.
function findRefFilter(result: FilterResult): RefFilterEntry | undefined {
  if (!result.filterId) {
    return undefined
  }
  return props.filters.find((f): f is RefFilterEntry => "ref" in f && f.id === result.filterId)
}

function findAmountFilter(result: FilterResult): AmountFilterEntry | undefined {
  if (!result.filterId) {
    return undefined
  }
  return props.filters.find((f): f is AmountFilterEntry => "amount" in f && f.id === result.filterId)
}

function findTimeFilter(result: FilterResult): TimeFilterEntry | undefined {
  if (!result.filterId) {
    return undefined
  }
  return props.filters.find((f): f is TimeFilterEntry => "time" in f && f.id === result.filterId)
}

function onFilterUpdate(filterId: string, filter: Filter) {
  if (abortController.signal.aborted) {
    return
  }

  $emit("filterUpdate", filterId, filter)
}
</script>

<template>
  <RefFiltersResult
    v-if="result.type === 'ref'"
    class="pd-filterresult"
    :search-session="searchSession"
    :search-total="searchTotal"
    :result="result"
    :filter="findRefFilter(result)"
    v-bind="$attrs"
    @filter-update="onFilterUpdate"
  />

  <AmountFiltersResult
    v-if="result.type === 'amount'"
    class="pd-filterresult"
    :search-session="searchSession"
    :search-total="searchTotal"
    :result="result"
    :filter="findAmountFilter(result)"
    v-bind="$attrs"
    @filter-update="onFilterUpdate"
  />

  <TimeFiltersResult
    v-if="result.type === 'time'"
    class="pd-filterresult"
    :search-session="searchSession"
    :search-total="searchTotal"
    :result="result"
    :filter="findTimeFilter(result)"
    v-bind="$attrs"
    @filter-update="onFilterUpdate"
  />
</template>
