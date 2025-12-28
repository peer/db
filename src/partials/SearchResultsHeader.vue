<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { ClientSearchSession, SelectButtonOption, ViewType } from "@/types"

import { Bars4Icon, TableCellsIcon } from "@heroicons/vue/20/solid"

import SelectButton from "@/components/SelectButton.vue"

const props = defineProps<{
  searchSession: DeepReadonly<ClientSearchSession>
  searchTotal: number | null
  searchMoreThanTotal: boolean
}>()

const $emit = defineEmits<{
  viewChange: [value: ViewType]
}>()

const selectButtonOptions: SelectButtonOption<ViewType>[] = [
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
    <div class="flex w-full flex-row items-center justify-between gap-x-1 rounded bg-slate-200 px-2 py-1 sm:gap-x-4 sm:px-4 sm:py-2">
      <div v-if="searchSession.query && countFilters() === 1">
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

    <SelectButton :model-value="searchSession.view" :options="selectButtonOptions" class="shrink-0" @update:model-value="(v) => $emit('viewChange', v)" />
  </div>
</template>
