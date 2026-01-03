<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type {
  AmountFilterState,
  AmountSearchResult,
  AmountUnit,
  ClientSearchSession,
  FilterResult,
  FiltersState,
  FilterStateChange,
  RelFilterState,
  StringFilterState,
  TimeFilterState,
} from "@/types"

import { onBeforeUnmount } from "vue"

import AmountFiltersResult from "@/partials/AmountFiltersResult.vue"
import RelFiltersResult from "@/partials/RelFiltersResult.vue"
import StringFiltersResult from "@/partials/StringFiltersResult.vue"
import TimeFiltersResult from "@/partials/TimeFiltersResult.vue"

defineProps<{
  result: FilterResult
  searchSession: DeepReadonly<ClientSearchSession>
  searchTotal: number
  updateSearchSessionProgress: number
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

function onRelFiltersStateUpdate(id: string, value: RelFilterState) {
  if (abortController.signal.aborted) {
    return
  }

  $emit("filterChange", { type: "rel", id, value })
}

function onAmountFiltersStateUpdate(id: string, unit: AmountUnit, value: AmountFilterState) {
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

function onStringFiltersStateUpdate(id: string, value: StringFilterState) {
  if (abortController.signal.aborted) {
    return
  }

  $emit("filterChange", { type: "string", id, value })
}
</script>

<template>
  <RelFiltersResult
    v-if="result.type === 'rel'"
    :search-session="searchSession"
    :search-total="searchTotal"
    :result="result"
    :state="filtersState.rel[result.id] ?? []"
    :update-progress="updateSearchSessionProgress"
    v-bind="$attrs"
    @update:state="(v) => onRelFiltersStateUpdate(result.id, v)"
  />

  <AmountFiltersResult
    v-if="result.type === 'amount'"
    :search-session="searchSession"
    :search-total="searchTotal"
    :result="result"
    :state="filtersState.amount[`${result.id}/${result.unit}`] ?? null"
    :update-progress="updateSearchSessionProgress"
    v-bind="$attrs"
    @update:state="(v) => onAmountFiltersStateUpdate(result.id, (result as AmountSearchResult).unit, v)"
  />

  <TimeFiltersResult
    v-if="result.type === 'time'"
    :search-session="searchSession"
    :search-total="searchTotal"
    :result="result"
    :state="filtersState.time[result.id] ?? null"
    :update-progress="updateSearchSessionProgress"
    v-bind="$attrs"
    @update:state="(v) => onTimeFiltersStateUpdate(result.id, v)"
  />

  <StringFiltersResult
    v-if="result.type === 'string'"
    :search-session="searchSession"
    :search-total="searchTotal"
    :result="result"
    :state="filtersState.str[result.id] ?? []"
    :update-progress="updateSearchSessionProgress"
    v-bind="$attrs"
    @update:state="(v) => onStringFiltersStateUpdate(result.id, v)"
  />
</template>
