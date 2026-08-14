<!--
We do not use :read-only or :disabled pseudo classes to style the component because
we want component to retain how it visually looks even if DOM element's read-only or
disabled attributes are set, unless they are set through component's props.
This is used during transitions/animations to disable the component by directly setting
its DOM attributes without flickering how the component looks.
-->

<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { SearchSession, SelectButtonOption, ViewType } from "@/types"

import { AdjustmentsHorizontalIcon, ArchiveBoxArrowDownIcon, ArrowDownTrayIcon, Bars4Icon, PrinterIcon, TableCellsIcon } from "@heroicons/vue/24/outline"
import { useI18n } from "vue-i18n"

import { hasFilePermission } from "@/auth"
import { ACTION_READ_BULK } from "@/core"
import SelectButton from "@/components/SelectButton.vue"
import siteContext from "@/context"
import { useInactivated, useLocked } from "@/progress"

const props = withDefaults(
  defineProps<{
    searchSession: DeepReadonly<SearchSession>
    searchTotal: number | null
    searchMoreThanTotal: boolean
    // sortable shows the sort & grouping button (the feed toolbar; the table does not use it).
    sortable?: boolean
    // printable shows the print-view button (the feed toolbar; the table does not use it).
    printable?: boolean
  }>(),
  {
    sortable: false,
    printable: false,
  },
)

// Starting a download while the view is working would act on a result set which is being replaced, and a
// download of its own locks the view (see SearchGet), so the buttons which start one follow the lock rather
// than a state of their own.
const locked = useLocked()
const inactivated = useInactivated()

const $emit = defineEmits<{
  viewChange: [value: ViewType]
  downloadZip: []
  downloadFiles: []
  sortOpen: []
  printOpen: []
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

function countFilters(): number {
  if (!props.searchSession.filters) {
    return 0
  }

  return props.searchSession.filters.length
}
</script>

<template>
  <div class="pd-searchresultsheader flex items-start gap-y-1 max-sm:flex-col sm:gap-x-4 sm:gap-y-0">
    <!--
      The status box wraps rather than stacking on a fixed breakpoint: its description and result count sit side
      by side (spread apart) while both fit, and the count drops onto its own line under the description once
      there is no longer room, instead of each shrinking and wrapping internally.
    -->
    <div
      class="pd-searchresultsheader-status flex w-full flex-wrap items-start justify-between gap-x-4 gap-y-0.5 rounded-sm bg-slate-200 px-2 py-1 sm:px-4 sm:py-2 print:bg-transparent print:px-1 print:py-0"
    >
      <div v-if="searchTotal === null && searchSession.query" class="pd-searchresultsheader-text-query">
        <i18n-t keypath="partials.SearchResultsHeader.searchingQueryFiltersInProgress" :plural="countFilters()" scope="global">
          <template #query>
            <i>{{ searchSession.query }}</i>
          </template>
        </i18n-t>
      </div>
      <div v-else-if="searchTotal !== null && searchSession.query" class="pd-searchresultsheader-text-query">
        <i18n-t keypath="partials.SearchResultsHeader.searchingQueryFilters" :plural="countFilters()" scope="global">
          <template #query>
            <i>{{ searchSession.query }}</i>
          </template>
        </i18n-t>
      </div>
      <div v-if="searchTotal === null && !searchSession.query" class="pd-searchresultsheader-text-query">
        {{ t("partials.SearchResultsHeader.searchingNoQueryFiltersInProgress", { count: countFilters() }) }}
      </div>
      <div v-else-if="searchTotal !== null && !searchSession.query" class="pd-searchresultsheader-text-query">
        {{ t("partials.SearchResultsHeader.searchingNoQueryFilters", { count: countFilters() }) }}
      </div>
      <template v-if="searchTotal !== null">
        <div v-if="searchTotal === 0" class="pd-searchresultsheader-count-results">{{ t("partials.SearchResultsHeader.noResults") }}</div>
        <div v-else-if="searchMoreThanTotal" class="pd-searchresultsheader-count-results">{{
          t("partials.SearchResultsHeader.resultsFoundMoreThan", { count: searchTotal })
        }}</div>
        <div v-else class="pd-searchresultsheader-count-results">{{ t("partials.SearchResultsHeader.resultsFound", { count: searchTotal }) }}</div>
      </template>
    </div>

    <!--
      Below sm the toolbar sits on its own row under the status box, so its buttons form a horizontal group here.
      From sm up this wrapper is display: contents, so the button groups are direct children of the header row again.
    -->
    <div class="pd-searchresultsheader-toolbar flex flex-row gap-x-1 sm:contents">
      <div
        v-if="sortable && !siteContext.features.disableSearchSort"
        class="pd-searchresultsheader-group-sort pd-print-hidden flex shrink-0 items-center rounded-sm bg-slate-200 px-1 py-1"
      >
        <button
          class="pd-searchresultsheader-button pd-searchresultsheader-button-sort h-full rounded-sm px-2 py-0.5 outline-none hover:bg-slate-100 focus:ring-2 focus:ring-primary-500 focus:ring-offset-1"
          type="button"
          :title="t('partials.SearchResultsHeader.sort')"
          @click.prevent="$emit('sortOpen')"
        >
          <AdjustmentsHorizontalIcon class="size-6" :alt="t('partials.SearchResultsHeader.sort')" />
        </button>
      </div>

      <SelectButton
        v-if="siteContext.features.searchResultsTable"
        :model-value="searchSession.view"
        :options="selectButtonOptions"
        class="pd-searchresultsheader-select-view pd-print-hidden shrink-0"
        @update:model-value="(v) => $emit('viewChange', v)"
      />

      <div
        v-if="printable && !siteContext.features.disablePrintView"
        class="pd-searchresultsheader-group-print pd-print-hidden flex shrink-0 items-center rounded-sm bg-slate-200 px-1 py-1"
      >
        <button
          class="pd-searchresultsheader-button pd-searchresultsheader-button-print h-full rounded-sm px-2 py-0.5 outline-none hover:bg-slate-100 focus:ring-2 focus:ring-primary-500 focus:ring-offset-1"
          type="button"
          :title="t('partials.SearchResultsHeader.print')"
          @click.prevent="$emit('printOpen')"
        >
          <PrinterIcon class="size-6" :alt="t('partials.SearchResultsHeader.print')" />
        </button>
      </div>

      <div
        v-if="siteContext.features.downloadButtons && hasFilePermission(ACTION_READ_BULK)"
        class="pd-searchresultsheader-group-download pd-print-hidden flex shrink-0 items-center gap-1 rounded-sm bg-slate-200 px-1 py-1"
      >
        <button
          class="pd-searchresultsheader-button pd-searchresultsheader-button-downloadzip h-full rounded-sm px-2 py-0.5 outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-1"
          :class="{
            'pd-locked': locked,
            'pd-inactive': inactivated,
            'cursor-not-allowed text-gray-500': locked, // Disabled style.
            'hover:bg-slate-100': !locked, // Enabled style.
          }"
          :disabled="locked"
          :title="t('partials.SearchResultsHeader.downloadZip')"
          @click.prevent="$emit('downloadZip')"
        >
          <ArchiveBoxArrowDownIcon class="size-6" :alt="t('partials.SearchResultsHeader.downloadZip')" />
        </button>
        <button
          v-if="directoryPickerSupported"
          class="pd-searchresultsheader-button pd-searchresultsheader-button-downloadfiles h-full rounded-sm px-2 py-0.5 outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-1"
          :class="{
            'pd-locked': locked,
            'pd-inactive': inactivated,
            'cursor-not-allowed text-gray-500': locked, // Disabled style.
            'hover:bg-slate-100': !locked, // Enabled style.
          }"
          :disabled="locked"
          :title="t('partials.SearchResultsHeader.downloadFiles')"
          @click.prevent="$emit('downloadFiles')"
        >
          <ArrowDownTrayIcon class="size-6" :alt="t('partials.SearchResultsHeader.downloadFiles')" />
        </button>
      </div>
    </div>
  </div>
</template>
