<script setup lang="ts">
import type { ComponentPublicInstance, DeepReadonly } from "vue"

import type { D } from "@/document"
import type { Filter, Result, SearchSession, ViewType } from "@/types"

import { FunnelIcon } from "@heroicons/vue/20/solid"
import { onBeforeUnmount, ref, toRef, useTemplateRef } from "vue"
import { useI18n } from "vue-i18n"

import Button from "@/components/Button.vue"
import WithDocument from "@/components/WithDocument.vue"
import DisplayLabel from "@/partials/DisplayLabel.vue"
import FiltersResult from "@/partials/FiltersResult.vue"
import Footer from "@/partials/Footer.vue"
import SearchResult from "@/partials/SearchResult.vue"
import SearchResultsHeader from "@/partials/SearchResultsHeader.vue"
import { useBusy } from "@/progress"
import { FILTERS_INCREASE, FILTERS_INITIAL_LIMIT, useFilters, useLocationAt } from "@/search"
import { loadingWidth, useLimitResults, useOnScrollOrResize } from "@/utils"
import { useVisibilityTracking } from "@/visibility"

const props = defineProps<{
  // Search props.
  searchResults: DeepReadonly<Result[]>
  searchTotal: number | null
  searchMoreThanTotal: boolean
  searchSession: DeepReadonly<SearchSession>
  isDownloading: boolean

  // Filter props.
  filters: Filter[]
}>()

const $emit = defineEmits<{
  filterUpdate: [filterId: string, filter: Filter]
  viewChange: [value: ViewType]
  downloadZip: []
  downloadFiles: []
  reverseClear: []
}>()

const { t } = useI18n({ useScope: "global" })

const SEARCH_INITIAL_LIMIT = 50
const SEARCH_INCREASE = 50

const {
  limitedResults: limitedSearchResults,
  hasMore: searchHasMore,
  loadMore: searchLoadMore,
} = useLimitResults(
  toRef(() => props.searchResults),
  SEARCH_INITIAL_LIMIT,
  SEARCH_INCREASE,
)

const filtersEl = useTemplateRef<HTMLElement>("filtersEl")
const filtersEnabled = ref(false)

// Data loading and controls for data loading.
const busy = useBusy()
const {
  results: filtersResults,
  total: filtersTotal,
  error: filtersError,
  url: filtersURL,
} = useFilters(
  toRef(() => props.searchSession),
  filtersEl,
  busy,
)

const {
  limitedResults: limitedFiltersResults,
  hasMore: filtersHasMore,
  loadMore: filtersLoadMore,
} = useLimitResults(filtersResults, FILTERS_INITIAL_LIMIT, FILTERS_INCREASE)

const { track, visibles } = useVisibilityTracking()

const abortController = new AbortController()

onBeforeUnmount(() => {
  abortController.abort()
})

const searchMoreButton = useTemplateRef<ComponentPublicInstance>("searchMoreButton")
const filtersMoreButton = useTemplateRef<ComponentPublicInstance>("filtersMoreButton")
const supportPageOffset = window.pageYOffset !== undefined

useLocationAt(
  toRef(() => props.searchResults),
  toRef(() => props.searchTotal),
  visibles,
)

const content = useTemplateRef<HTMLElement>("content")

useOnScrollOrResize(content, onScrollOrResize)

function onScrollOrResize() {
  if (abortController.signal.aborted) {
    return
  }

  if (searchMoreButton.value || filtersMoreButton.value) {
    const viewportHeight = document.documentElement.clientHeight || document.body.clientHeight
    const scrollHeight = Math.max(
      document.body.scrollHeight,
      document.documentElement.scrollHeight,
      document.body.offsetHeight,
      document.documentElement.offsetHeight,
      document.body.clientHeight,
      document.documentElement.clientHeight,
    )
    const currentScrollYPosition = supportPageOffset ? window.pageYOffset : document.documentElement.scrollTop

    if (currentScrollYPosition > scrollHeight - 2 * viewportHeight) {
      // We load more by clicking the button so that we have one place to disable loading more (by disabling the button).
      // This assures that UX is consistent and that user cannot load more through any interaction (click or scroll).
      if (searchMoreButton.value) {
        ;(searchMoreButton.value.$el as HTMLButtonElement).click()
      }
      if (filtersMoreButton.value) {
        ;(filtersMoreButton.value.$el as HTMLButtonElement).click()
      }
    }
  }
}

function onFilters() {
  if (abortController.signal.aborted) {
    return
  }

  filtersEnabled.value = !filtersEnabled.value
}

function onSkipTo(targetId: string) {
  document.getElementById(targetId)?.focus()
}

const WithDocumentD = WithDocument<D>
</script>

<template>
  <Teleport to="#navbarsearch-teleport-end">
    <Button primary class="px-3.5 sm:hidden" type="button" @click.prevent="onFilters">
      <FunnelIcon class="size-5" :alt="t('common.buttons.filters')" />
    </Button>
  </Teleport>

  <div ref="content" class="pd-searchresultsfeed relative flex w-full gap-x-1 p-1 sm:gap-x-4 sm:p-4">
    <a
      href="#search-filters"
      class="sr-only focus:not-sr-only focus:absolute focus:top-1 focus:left-1 focus:z-50 focus:rounded-sm focus:bg-primary-600 focus:px-4 focus:py-2 focus:font-medium focus:text-white focus:shadow-lg focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 focus:outline-none sm:focus:top-4 sm:focus:left-4"
      @click.prevent="onSkipTo('search-filters')"
      >{{ t("partials.SearchResultsFeed.skipToFilters") }}</a
    >
    <a
      href="#search-results"
      class="sr-only focus:not-sr-only focus:absolute focus:top-1 focus:left-1 focus:z-50 focus:rounded-sm focus:bg-primary-600 focus:px-4 focus:py-2 focus:font-medium focus:text-white focus:shadow-lg focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 focus:outline-none sm:focus:top-4 sm:focus:left-4"
      @click.prevent="onSkipTo('search-results')"
      >{{ t("partials.SearchResultsFeed.skipToResults") }}</a
    >
    <!-- Search results column -->
    <div
      id="search-results"
      tabindex="-1"
      class="flex-auto basis-3/4 flex-col gap-y-1 rounded-sm focus-visible:ring-2 focus-visible:ring-primary-500 focus-visible:ring-offset-1 focus-visible:outline-none sm:flex sm:gap-y-4"
      :class="filtersEnabled ? 'hidden' : 'flex'"
    >
      <SearchResultsHeader
        :search-session="searchSession"
        :search-total="searchTotal"
        :search-more-than-total="searchMoreThanTotal"
        :is-downloading="isDownloading"
        @view-change="(v) => $emit('viewChange', v)"
        @download-zip="$emit('downloadZip')"
        @download-files="$emit('downloadFiles')"
      />

      <template v-if="searchTotal !== null && searchTotal > 0">
        <template v-for="(result, i) in limitedSearchResults" :key="result.id">
          <div v-if="i > 0 && i % 10 === 0" class="pd-pager my-1 sm:my-4">
            <div v-if="searchResults.length < searchTotal" class="pd-count text-center text-sm">{{
              t("partials.SearchResultsFeed.shownResultsOnly", { i, count: searchResults.length })
            }}</div>
            <div v-else-if="searchResults.length == searchTotal" class="pd-count text-center text-sm">{{
              t("partials.SearchResultsFeed.shownResults", { i, count: searchResults.length })
            }}</div>
            <!-- We do not use ProgressBar here because we plan to make this an interactive bar on which you can click to move to that location. -->
            <div class="pd-track relative h-2 w-full bg-slate-200">
              <div class="pd-thumb absolute inset-y-0 left-0 bg-secondary-400" :style="{ width: (i / searchResults.length) * 100 + '%' }" />
            </div>
          </div>
          <SearchResult :ref="track(result.id)" :search-session-id="searchSession.id" :result="result" />
        </template>

        <Button
          v-if="searchHasMore"
          id="searchresultsfeed-button-loadmore"
          ref="searchMoreButton"
          primary
          class="w-1/4 min-w-fit self-center"
          @click.prevent="searchLoadMore"
          >{{ t("common.buttons.loadMore") }}</Button
        >

        <div v-else class="my-1 sm:my-4">
          <!-- Here we assume that MaxResultsCount is always set to a smaller value than what TrackTotalHits is set to. -->
          <div v-if="searchMoreThanTotal" class="text-center text-sm">{{
            t("common.status.allResultsMoreThan", { first: searchResults.length, count: searchTotal })
          }}</div>
          <div v-else-if="searchResults.length < searchTotal" class="text-center text-sm">{{
            t("common.status.allResultsOnly", { first: searchResults.length, count: searchTotal })
          }}</div>
          <div v-else-if="searchResults.length === searchTotal" class="text-center text-sm">{{ t("common.status.allResults", { count: searchResults.length }) }}</div>
          <div class="relative h-2 w-full bg-slate-200">
            <div class="absolute inset-y-0 left-0 bg-secondary-400" :style="{ width: 100 + '%' }"></div>
          </div>
        </div>
      </template>
    </div>

    <!-- Filters column -->
    <div
      id="search-filters"
      ref="filtersEl"
      tabindex="-1"
      class="flex-auto basis-1/4 flex-col gap-y-1 rounded-sm focus-visible:ring-2 focus-visible:ring-primary-500 focus-visible:ring-offset-1 focus-visible:outline-none sm:flex sm:gap-y-4"
      :class="filtersEnabled ? 'flex' : 'hidden'"
      :data-url="filtersURL"
    >
      <div v-if="filtersError" class="pd-searchresultsfeed-filters-error-wrapper my-1 sm:my-4">
        <div class="text-center text-sm"
          ><i class="pd-searchresultsfeed-filters-error text-error-600">{{ t("common.status.loadingDataFailed") }}</i></div
        >
      </div>

      <div v-else-if="searchTotal === null || filtersTotal === null" class="my-1 sm:my-4">
        <div class="text-center text-sm">{{ t("partials.SearchResultsFeed.determiningFilters") }}</div>
      </div>

      <div v-else-if="filtersTotal === 0" class="my-1 sm:my-4">
        <div class="text-center text-sm">{{ t("partials.SearchResultsFeed.noFilters") }}</div>
      </div>

      <template v-else-if="filtersTotal > 0 || searchSession.reverse">
        <div v-if="searchSession.reverse" class="text-center text-sm">
          <Button
            type="button"
            class="float-right ml-2 px-2.5 py-1"
            :title="t('partials.SearchResultsFeed.clearReferencing')"
            :aria-label="t('partials.SearchResultsFeed.clearReferencing')"
            @click.prevent="$emit('reverseClear')"
            >{{ t("common.buttons.clear") }}</Button
          >
          <i18n-t keypath="partials.SearchResultsFeed.resultsReferencing" scope="global">
            <template #label>
              <RouterLink :to="{ name: 'DocumentGet', params: { id: searchSession.reverse } }" class="link">
                <WithDocumentD :id="searchSession.reverse" name="DocumentGet">
                  <template #default="{ doc }">
                    <DisplayLabel :doc="doc" />
                  </template>
                  <template #loading>
                    <span
                      class="pd-withdocument-loading inline-block h-2 rounded-sm bg-slate-200 motion-safe:animate-pulse"
                      :class="[loadingWidth(searchSession.reverse!)]"
                    />
                  </template>
                </WithDocumentD>
              </RouterLink>
            </template>
          </i18n-t>
        </div>
        <div class="text-center text-sm">{{ t("partials.SearchResultsFeed.filtersAvailable", { count: filtersTotal }) }}</div>

        <template v-for="filter in limitedFiltersResults" :key="filter.filterId ?? `${filter.props?.join('/') ?? ''}/${'unit' in filter ? (filter.unit ?? '') : ''}`">
          <FiltersResult
            :result="filter"
            :search-session="searchSession"
            :filters="filters"
            class="rounded-sm border border-gray-200 bg-white p-4 shadow-sm"
            @filter-update="(filterId, filter) => $emit('filterUpdate', filterId, filter)"
          />
        </template>

        <Button v-if="filtersHasMore" ref="filtersMoreButton" primary class="w-1/2 min-w-fit self-center" @click.prevent="filtersLoadMore">{{
          t("partials.SearchResultsFeed.moreFilters")
        }}</Button>

        <div v-else-if="filtersTotal > limitedFiltersResults.length" class="text-center text-sm">{{
          t("partials.SearchResultsFeed.filtersNotShown", { count: filtersTotal - limitedFiltersResults.length })
        }}</div>
      </template>
    </div>
  </div>

  <Teleport v-if="(searchTotal !== null && searchTotal > 0 && !searchHasMore) || searchTotal === 0" to="footer">
    <Footer class="border-t border-slate-50 bg-slate-200 shadow-sm" />
  </Teleport>
</template>
