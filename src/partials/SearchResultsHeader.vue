<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { ClientSearchSession, SearchViewType, SelectButtonOption } from "@/types"

import { computed } from "vue"
import { Bars4Icon, TableCellsIcon } from "@heroicons/vue/20/solid"

import SelectButton from "@/components/SelectButton.vue"

const props = defineProps<{
  searchSession: DeepReadonly<ClientSearchSession | null>
  searchTotal: number | null
  searchMoreThanTotal: boolean
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

// TODO: Use a computed property instead of computing countFilters multiple times.

function countFilters(): number {
  if (!props.searchSession) {
    return 0
  }
  if (!props.searchSession.filters) {
    return 0
  }

  let n = 0
  for (const values of Object.values(props.searchSession.filters.rel)) {
    n += values.length
  }
  for (const value of Object.values(props.searchSession.filters.amount)) {
    if (value) {
      n++
    }
  }
  for (const value of Object.values(props.searchSession.filters.time)) {
    if (value) {
      n++
    }
  }
  for (const values of Object.values(props.searchSession.filters.str)) {
    n += values.length
  }
  return n
}
</script>

<template>
  <div class="flex flex-row gap-x-1 sm:gap-x-4">
    <div class="bg-slate-200 px-2 sm:px-4 py-1 sm:py-2 rounded flex flex-row justify-between items-center w-full gap-x-1 sm:gap-x-4">
      <div v-if="searchSession === null">Loading...</div>
      <div v-else-if="searchSession.query && countFilters() === 1">
        Searching query <i>{{ searchSession.query }}</i> and 1 active filter<template v-if="searchTotal === null">...</template><template v-else>.</template>
      </div>
      <div v-else-if="searchSession.query">
        Searching query <i>{{ searchSession.query }}</i> and {{ countFilters() }} active filters<template v-if="searchTotal === null">...</template
        ><template v-else>.</template>
      </div>
      <div v-else-if="countFilters() === 1">
        Searching without query and with 1 active filter<template v-if="searchTotal === null">...</template><template v-else>.</template>
      </div>
      <div v-else>
        Searching without query and with {{ countFilters() }} active filters<template v-if="searchTotal === null">...</template><template v-else>.</template>
      </div>
      <template v-if="searchTotal !== null">
        <div v-if="searchTotal === 0">No results found.</div>
        <div v-else-if="searchMoreThanTotal">More than {{ searchTotal }} results found.</div>
        <div v-else>{{ searchTotal }} results found.</div>
      </template>
    </div>

    <SelectButton v-model="selectButtonValue" :options="selectButtonOptions" class="flex-shrink-0" />
  </div>
</template>
