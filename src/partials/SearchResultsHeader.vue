<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { ClientSearchSession, SelectButtonOption, ViewType } from "@/types"

import { Bars4Icon, TableCellsIcon } from "@heroicons/vue/24/solid"
import { useI18n } from "vue-i18n"

import SelectButton from "@/components/SelectButton.vue"

const props = defineProps<{
  searchSession: DeepReadonly<ClientSearchSession>
  searchTotal: number | null
  searchMoreThanTotal: boolean
}>()

const $emit = defineEmits<{
  viewChange: [value: ViewType]
}>()

const { t } = useI18n()

const selectButtonOptions: SelectButtonOption<ViewType>[] = [
  {
    name: "feed",
    icon: {
      component: Bars4Icon,
      alt: t("common.icons.feed"),
    },
    value: "feed",
  },
  {
    name: "table",
    icon: {
      component: TableCellsIcon,
      alt: t("common.icons.table"),
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
    <div class="flex w-full flex-row items-center justify-between gap-x-1 rounded-sm bg-slate-200 px-2 py-1 sm:gap-x-4 sm:px-4 sm:py-2">
      <div v-if="searchTotal === null && searchSession.query">
        <i18n-t keypath="partials.SearchResultsHeader.searchingQueryFiltersInProgress" :plural="countFilters()" scope="global">
          <template #query>
            <i>{{ searchSession.query }}</i>
          </template>
        </i18n-t>
      </div>
      <div v-else-if="searchTotal !== null && searchSession.query">
        <i18n-t keypath="partials.SearchResultsHeader.searchingQueryFilters" :plural="countFilters()" scope="global">
          <template #query>
            <i>{{ searchSession.query }}</i>
          </template>
        </i18n-t>
      </div>
      <div v-if="searchTotal === null && !searchSession.query">
        {{ t("partials.SearchResultsHeader.searchingNoQueryFiltersInProgress", {count: countFilters()}) }}
      </div>
      <div v-else-if="searchTotal !== null && !searchSession.query">
        {{ t("partials.SearchResultsHeader.searchingNoQueryFilters", {count: countFilters()}) }}
      </div>
      <template v-if="searchTotal !== null">
        <div v-if="searchTotal === 0">{{ t("partials.SearchResultsHeader.noResults") }}</div>
        <div v-else-if="searchMoreThanTotal">{{ t("partials.SearchResultsHeader.resultsFoundMoreThan", { count: searchTotal }) }}</div>
        <div v-else>{{ t("partials.SearchResultsHeader.resultsFound", { count: searchTotal }) }}</div>
      </template>
    </div>

    <SelectButton :model-value="searchSession.view" :options="selectButtonOptions" class="shrink-0" @update:model-value="(v) => $emit('viewChange', v)" />
  </div>
</template>
