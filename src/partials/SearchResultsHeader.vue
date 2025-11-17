<script setup lang="ts">
import type { ClientSearchState, SearchViewType, SelectButtonOption } from "@/types"

import { computed, DeepReadonly } from "vue"
import SelectButton from "@/components/SelectButton.vue"
import { Bars4Icon, TableCellsIcon } from "@heroicons/vue/20/solid"

const props = defineProps<{
  state: DeepReadonly<ClientSearchState | null>
  total: number | null
  results: number
  moreThanTotal: boolean
  searchView: SearchViewType
}>()

const $emit = defineEmits<{
  "update:searchView": [value: SearchViewType]
}>()

const selectButtonValue = computed({
  get() {
    return props.searchView
  },
  set(newValue) {
    $emit("update:searchView", newValue)
  },
})

const selectButtonOptions: SelectButtonOption<SearchViewType>[] = [
  {
    name: "feed",
    icon: {
      component: Bars4Icon,
      alt: "Feed",
    },
    value: "feed",
  },
  {
    name: "table",
    icon: {
      component: TableCellsIcon,
      alt: "Table",
    },
    value: "table",
  },
]

function countFilters(): number {
  if (!props.state) {
    return 0
  }
  if (!props.state.filters) {
    return 0
  }

  let n = 0
  for (const values of Object.values(props.state.filters.rel)) {
    n += values.length
  }
  for (const value of Object.values(props.state.filters.amount)) {
    if (value) {
      n++
    }
  }
  for (const value of Object.values(props.state.filters.time)) {
    if (value) {
      n++
    }
  }
  for (const values of Object.values(props.state.filters.str)) {
    n += values.length
  }
  if (props.state.filters.index) {
    n += props.state.filters.index.length
  }
  if (props.state.filters.size) {
    n++
  }
  return n
}
</script>

<template>
  <div class="flex gap-4 items-center">
    <div class="bg-slate-200 px-4 py-2 rounded flex flex-row justify-between w-full">
      <div v-if="state === null">Loading...</div>
      <div v-else-if="state.promptError">Error interpreting your prompt.</div>
      <div v-else-if="state.p && !state.promptDone">Interpreting your prompt...</div>
      <div v-else-if="state.q && countFilters() === 1">
        Searching query <i>{{ state.q }}</i> and 1 active filter
        <template v-if="total === null">...</template>
        <template v-else>.</template>
      </div>
      <div v-else-if="state.q">
        Searching query <i>{{ state.q }}</i> and {{ countFilters() }} active filters
        <template v-if="total === null">...</template>
        <template v-else>.</template>
      </div>
      <div v-else-if="countFilters() === 1">
        Searching without query and with 1 active filter
        <template v-if="total === null">...</template>
        <template v-else>.</template>
      </div>
      <div v-else>
        Searching without query and with {{ countFilters() }} active filters
        <template v-if="total === null">...</template>
        <template v-else>.</template>
      </div>
      <template v-if="total !== null">
        <div v-if="total === 0">No results found.</div>
        <div v-else>{{ total }} results found.</div>
      </template>
    </div>

    <SelectButton v-model="selectButtonValue" :options="selectButtonOptions" class="flex-shrink-0" />
  </div>
</template>
