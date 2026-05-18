<!--
We do not use :read-only or :disabled pseudo classes to style the component because
we want component to retain how it visually looks even if DOM element's read-only or
disabled attributes are set, unless they are set through component's props.
This is used during transitions/animations to disable the component by directly setting
its DOM attributes without flickering how the component looks.
-->

<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { ClientSearchSession, SelectButtonOption, ViewType } from "@/types"

import { ArchiveBoxArrowDownIcon, ArrowDownTrayIcon, Bars4Icon, TableCellsIcon } from "@heroicons/vue/24/outline"
import { useI18n } from "vue-i18n"

import SelectButton from "@/components/SelectButton.vue"
import siteContext from "@/context"

const props = defineProps<{
  searchSession: DeepReadonly<ClientSearchSession>
  searchTotal: number | null
  searchMoreThanTotal: boolean
  isDownloading: boolean
}>()

const $emit = defineEmits<{
  viewChange: [value: ViewType]
  downloadZip: []
  downloadFiles: []
}>()

const { t } = useI18n({ useScope: "global" })

const directoryPickerSupported = "showDirectoryPicker" in window

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
  for (const values of Object.values(props.searchSession.filters.ref)) {
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
  return n
}
</script>

<template>
  <div class="pd-searchresultsheader flex flex-row gap-x-1 sm:gap-x-4">
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
        {{ t("partials.SearchResultsHeader.searchingNoQueryFiltersInProgress", { count: countFilters() }) }}
      </div>
      <div v-else-if="searchTotal !== null && !searchSession.query">
        {{ t("partials.SearchResultsHeader.searchingNoQueryFilters", { count: countFilters() }) }}
      </div>
      <template v-if="searchTotal !== null">
        <div v-if="searchTotal === 0">{{ t("partials.SearchResultsHeader.noResults") }}</div>
        <div v-else-if="searchMoreThanTotal">{{ t("partials.SearchResultsHeader.resultsFoundMoreThan", { count: searchTotal }) }}</div>
        <div v-else>{{ t("partials.SearchResultsHeader.resultsFound", { count: searchTotal }) }}</div>
      </template>
    </div>

    <SelectButton
      v-if="siteContext.features.searchResultsTable"
      :model-value="searchSession.view"
      :options="selectButtonOptions"
      class="shrink-0"
      @update:model-value="(v) => $emit('viewChange', v)"
    />

    <div v-if="siteContext.features.downloadButtons" class="flex shrink-0 items-center gap-1 rounded-sm bg-slate-200 px-1 py-1">
      <button
        class="h-full rounded-sm px-2 py-0.5 outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-1"
        :class="{
          'cursor-not-allowed text-gray-500': isDownloading, // Disabled style.
          'hover:bg-slate-100': !isDownloading, // Enabled style.
        }"
        :disabled="isDownloading"
        :title="t('partials.SearchResultsHeader.downloadZip')"
        @click.prevent="$emit('downloadZip')"
      >
        <ArchiveBoxArrowDownIcon class="size-6" :alt="t('partials.SearchResultsHeader.downloadZip')" />
      </button>
      <button
        v-if="directoryPickerSupported"
        class="h-full rounded-sm px-2 py-0.5 outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-1"
        :class="{
          'cursor-not-allowed text-gray-500': isDownloading, // Disabled style.
          'hover:bg-slate-100': !isDownloading, // Enabled style.
        }"
        :disabled="isDownloading"
        :title="t('partials.SearchResultsHeader.downloadFiles')"
        @click.prevent="$emit('downloadFiles')"
      >
        <ArrowDownTrayIcon class="size-6" :alt="t('partials.SearchResultsHeader.downloadFiles')" />
      </button>
    </div>
  </div>
</template>
