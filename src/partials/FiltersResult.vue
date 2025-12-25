<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type {
  AmountFilterState,
  AmountUnit,
  ClientSearchSession,
  FilterResult,
  FiltersState,
  FilterStateChange,
  RelFilterState,
  StringFilterState,
  TimeFilterState,
} from "@/types"

import AmountFiltersResult from "@/partials/AmountFiltersResult.vue"
import RelFiltersResult from "@/partials/RelFiltersResult.vue"
import StringFiltersResult from "@/partials/StringFiltersResult.vue"
import TimeFiltersResult from "@/partials/TimeFiltersResult.vue"

defineProps<{
  filter: FilterResult
  searchSession: DeepReadonly<ClientSearchSession>
  searchTotal: number
  updateSearchSessionProgress: number
  filtersState: FiltersState
}>()

const $emit = defineEmits<{
  filterChange: [change: FilterStateChange]
}>()

const abortController = new AbortController()

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
    v-if="filter.type === 'rel'"
    :search-session="searchSession"
    :search-total="searchTotal"
    :result="filter"
    :state="filtersState.rel[filter.id] ?? []"
    :update-progress="updateSearchSessionProgress"
    @update:state="onRelFiltersStateUpdate(filter.id, $event)"
  />

  <AmountFiltersResult
    v-if="filter.type === 'amount'"
    :search-session="searchSession"
    :search-total="searchTotal"
    :result="filter"
    :state="filtersState.amount[`${filter.id}/${filter.unit}`] ?? null"
    :update-progress="updateSearchSessionProgress"
    @update:state="onAmountFiltersStateUpdate(filter.id, filter.unit, $event)"
  />

  <TimeFiltersResult
    v-if="filter.type === 'time'"
    :search-session="searchSession"
    :search-total="searchTotal"
    :result="filter"
    :state="filtersState.time[filter.id] ?? null"
    :update-progress="updateSearchSessionProgress"
    @update:state="onTimeFiltersStateUpdate(filter.id, $event)"
  />

  <StringFiltersResult
    v-if="filter.type === 'string'"
    :search-session="searchSession"
    :search-total="searchTotal"
    :result="filter"
    :state="filtersState.str[filter.id] ?? []"
    :update-progress="updateSearchSessionProgress"
    @update:state="onStringFiltersStateUpdate(filter.id, $event)"
  />
</template>
