<script setup lang="ts">
import type { ComponentPublicInstance, DeepReadonly } from "vue"

import type { ClientSearchSession, FiltersState, FilterStateChange, Result, ViewType } from "@/types"

import { FunnelIcon } from "@heroicons/vue/20/solid"
import { onBeforeUnmount, ref, toRef, useTemplateRef } from "vue"

import Button from "@/components/Button.vue"
import FiltersResult from "@/partials/FiltersResult.vue"
import Footer from "@/partials/Footer.vue"
import SearchResult from "@/partials/SearchResult.vue"
import SearchResultsHeader from "@/partials/SearchResultsHeader.vue"
import { injectProgress } from "@/progress"
import { FILTERS_INCREASE, FILTERS_INITIAL_LIMIT, useFilters, useLocationAt } from "@/search"
import { useLimitResults, useOnScrollOrResize } from "@/utils"
import { useVisibilityTracking } from "@/visibility"

const props = defineProps<{
  // Search props.
  searchResults: DeepReadonly<Result[]>
  searchTotal: number | null
  searchMoreThanTotal: boolean
  searchSession: DeepReadonly<ClientSearchSession>
  searchProgress: number
  updateSearchSessionProgress: number

  // Filter props.
  filtersState: FiltersState
}>()

const $emit = defineEmits<{
  filterChange: [change: FilterStateChange]
  viewChange: [value: ViewType]
}>()

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

const filtersProgress = injectProgress()
const {
  results: filtersResults,
  total: filtersTotal,
  error: filtersError,
  url: filtersURL,
} = useFilters(
  toRef(() => props.searchSession),
  filtersEl,
  filtersProgress,
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
</script>

<template>
  <Teleport to="#navbarsearch-teleport-end">
    <Button primary class="px-3.5! sm:hidden" type="button" @click.prevent="onFilters">
      <FunnelIcon class="size-5" alt="Filters" />
    </Button>
  </Teleport>

  <div ref="content" class="flex w-full gap-x-1 p-1 sm:gap-x-4 sm:p-4">
    <!-- Search results column -->
    <div class="flex-auto basis-3/4 flex-col gap-y-1 sm:flex sm:gap-y-4" :class="filtersEnabled ? 'hidden' : 'flex'">
      <SearchResultsHeader
        :search-session="searchSession"
        :search-total="searchTotal"
        :search-more-than-total="searchMoreThanTotal"
        @view-change="(v) => $emit('viewChange', v)"
      />

      <template v-if="searchTotal !== null && searchTotal > 0">
        <template v-for="(result, i) in limitedSearchResults" :key="result.id">
          <div v-if="i > 0 && i % 10 === 0" class="my-1 sm:my-4">
            <div v-if="searchResults.length < searchTotal" class="text-center text-sm">{{ i }} of {{ searchResults.length }} shown results.</div>
            <div v-else-if="searchResults.length == searchTotal" class="text-center text-sm">{{ i }} of {{ searchResults.length }} results.</div>
            <div class="relative h-2 w-full bg-slate-200">
              <div class="absolute inset-y-0 bg-secondary-400" style="left: 0" :style="{ width: (i / searchResults.length) * 100 + '%' }" />
            </div>
          </div>
          <SearchResult :ref="track(result.id)" :search-session-id="searchSession.id" :result="result" />
        </template>

        <Button v-if="searchHasMore" ref="searchMoreButton" :progress="searchProgress" primary class="w-1/4 min-w-fit self-center" @click.prevent="searchLoadMore"
          >Load more</Button
        >

        <div v-else class="my-1 sm:my-4">
          <div v-if="searchMoreThanTotal" class="text-center text-sm">All of first {{ searchResults.length }} shown of more than {{ searchTotal }} results found.</div>
          <div v-else-if="searchResults.length < searchTotal" class="text-center text-sm">
            All of first {{ searchResults.length }} shown of {{ searchTotal }} results found.
          </div>
          <div v-else-if="searchResults.length === searchTotal" class="text-center text-sm">All of {{ searchResults.length }} results shown.</div>
          <div class="relative h-2 w-full bg-slate-200">
            <div class="absolute inset-y-0 bg-secondary-400" style="left: 0" :style="{ width: 100 + '%' }"></div>
          </div>
        </div>
      </template>
    </div>

    <!-- Filters column -->
    <div ref="filtersEl" class="flex-auto basis-1/4 flex-col gap-y-1 sm:flex sm:gap-y-4" :class="filtersEnabled ? 'flex' : 'hidden'" :data-url="filtersURL">
      <div v-if="filtersError" class="my-1 sm:my-4">
        <div class="text-center text-sm"><i class="text-error-600">loading data failed</i></div>
      </div>

      <div v-else-if="searchTotal === null || filtersTotal === null" class="my-1 sm:my-4">
        <div class="text-center text-sm">Determining filters...</div>
      </div>

      <div v-else-if="filtersTotal === 0" class="my-1 sm:my-4">
        <div class="text-center text-sm">No filters available.</div>
      </div>

      <template v-else-if="filtersTotal > 0">
        <div class="text-center text-sm">{{ filtersTotal }} filters available.</div>

        <template v-for="filter in limitedFiltersResults" :key="filter.id">
          <FiltersResult
            :result="filter"
            :search-session="searchSession"
            :search-total="searchTotal"
            :update-search-session-progress="updateSearchSessionProgress"
            :filters-state="filtersState"
            class="rounded-sm border border-gray-200 bg-white p-4 shadow-sm"
            @filter-change="(c) => $emit('filterChange', c)"
          />
        </template>

        <Button v-if="filtersHasMore" ref="filtersMoreButton" :progress="filtersProgress" primary class="w-1/2 min-w-fit self-center" @click.prevent="filtersLoadMore"
          >More filters</Button
        >

        <div v-else-if="filtersTotal > limitedFiltersResults.length" class="text-center text-sm">
          {{ filtersTotal - limitedFiltersResults.length }} filters not shown.
        </div>
      </template>
    </div>
  </div>

  <Teleport v-if="(searchTotal !== null && searchTotal > 0 && !searchHasMore) || searchTotal === 0" to="footer">
    <Footer class="border-t border-slate-50 bg-slate-200 shadow-sm" />
  </Teleport>
</template>
