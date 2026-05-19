<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { AmountFilterState, AmountSearchResult, ClientSearchSession, FilterResult, FiltersState, FilterStateChange, RefFilterState, TimeFilterState } from "@/types"

import { onBeforeUnmount } from "vue"

import AmountFiltersResult from "@/partials/AmountFiltersResult.vue"
import RefFiltersResult from "@/partials/RefFiltersResult.vue"
import TimeFiltersResult from "@/partials/TimeFiltersResult.vue"

defineProps<{
  result: FilterResult
  searchSession: DeepReadonly<ClientSearchSession>
  searchTotal: number
  filtersState: FiltersState
}>()

const $emit = defineEmits<{
  filterChange: [change: FilterStateChange]
}>()

// We have to explicitly pass attributes because we use multiple root nodes.
defineOptions({
  inheritAttrs: false,
})

const abortController = new AbortController()

onBeforeUnmount(() => {
  abortController.abort()
})

function onRefFiltersStateUpdate(id: string, value: RefFilterState) {
  if (abortController.signal.aborted) {
    return
  }

  $emit("filterChange", { type: "ref", id, value })
}

function onAmountFiltersStateUpdate(id: string, unit: string | undefined, value: AmountFilterState) {
  if (abortController.signal.aborted) {
    return
  }

  $emit("filterChange", { type: "amount", id, unit, value })
}

function onTimeFiltersStateUpdate(id: string, value: TimeFilterState) {
  if (abortController.signal.aborted) {
    return
  }

  $emit("filterChange", { type: "time", id, value })
}

function amountFilterKey(id: string, unit?: string): string {
  if (unit) {
    return `${id}/${unit}`
  }
  return id
}
</script>

<template>
  <RefFiltersResult
    v-if="result.type === 'ref'"
    class="pd-filterresult"
    :search-session="searchSession"
    :search-total="searchTotal"
    :result="result"
    :state="filtersState.ref[result.id] ?? []"
    v-bind="$attrs"
    @update:state="(v) => onRefFiltersStateUpdate(result.id, v)"
  />

  <AmountFiltersResult
    v-if="result.type === 'amount'"
    class="pd-filterresult"
    :search-session="searchSession"
    :search-total="searchTotal"
    :result="result"
    :state="filtersState.amount[amountFilterKey(result.id, (result as AmountSearchResult).unit)] ?? null"
    v-bind="$attrs"
    @update:state="(v) => onAmountFiltersStateUpdate(result.id, (result as AmountSearchResult).unit, v)"
  />

  <TimeFiltersResult
    v-if="result.type === 'time'"
    class="pd-filterresult"
    :search-session="searchSession"
    :search-total="searchTotal"
    :result="result"
    :state="filtersState.time[result.id] ?? null"
    v-bind="$attrs"
    @update:state="(v) => onTimeFiltersStateUpdate(result.id, v)"
  />
</template>
