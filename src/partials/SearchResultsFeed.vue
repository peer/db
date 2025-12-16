<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type {
  AmountFilterState,
  ClientSearchSession,
  FiltersState,
  RelFilterState,
  Result as SearchResultType,
  StringFilterState,
  TimeFilterState,
  SearchViewType,
  FilterStateChange,
  AmountUnit,
} from "@/types"

import { computed, onBeforeUnmount, ref, toRef } from "vue"
import { FunnelIcon } from "@heroicons/vue/20/solid"

import Button from "@/components/Button.vue"
import SearchResult from "@/partials/SearchResult.vue"
import SearchResultsHeader from "@/partials/SearchResultsHeader.vue"
import RelFiltersResult from "@/partials/RelFiltersResult.vue"
import TimeFiltersResult from "@/partials/TimeFiltersResult.vue"
import StringFiltersResult from "@/partials/StringFiltersResult.vue"
import AmountFiltersResult from "@/partials/AmountFiltersResult.vue"
import { useVisibilityTracking } from "@/visibility"
import { useLimitResults, useOnScrollOrResize } from "@/utils.ts"
import { useFilters, FILTERS_INITIAL_LIMIT, FILTERS_INCREASE, useLocationAt } from "@/search.ts"
import { injectProgress } from "@/progress.ts"
import Footer from "@/partials/Footer.vue"

const props = defineProps<{
  searchView: SearchViewType

  // Search props.
  searchResults: DeepReadonly<SearchResultType[]>
  searchTotal: number | null
  searchMoreThanTotal: boolean
  searchSession: DeepReadonly<ClientSearchSession | null>
  searchProgress: number

  // Filter props.
  filtersState: FiltersState
  updateFiltersProgress: number
}>()

const $emit = defineEmits<{
  filterChange: [change: FilterStateChange]
  "update:searchView": [value: SearchViewType]
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

const filtersEl = ref(null)
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

const searchMoreButton = ref()
const filtersMoreButton = ref()
const supportPageOffset = window.pageYOffset !== undefined

const searchViewValue = computed({
  get() {
    return props.searchView
  },
  set(value) {
    $emit("update:searchView", value)
  },
})

useLocationAt(
  toRef(() => props.searchResults),
  toRef(() => props.searchTotal),
  visibles,
)

const content = ref(null)

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
        searchMoreButton.value.$el.click()
      }
      if (filtersMoreButton.value) {
        filtersMoreButton.value.$el.click()
      }
    }
  }
}

function onRelFiltersStateUpdate(id: string, value: RelFilterState) {
  $emit("filterChange", { type: "rel", id, value })
}

function onAmountFiltersStateUpdate(id: string, unit: AmountUnit, value: AmountFilterState) {
  $emit("filterChange", { type: "amount", id, unit, value })
}

function onTimeFiltersStateUpdate(id: string, value: TimeFilterState) {
  $emit("filterChange", { type: "time", id, value })
}

function onStringFiltersStateUpdate(id: string, value: StringFilterState) {
  $emit("filterChange", { type: "string", id, value })
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
    <Button primary class="!px-3.5 sm:hidden" type="button" @click="onFilters">
      <FunnelIcon class="h-5 w-5" alt="Filters" />
    </Button>
  </Teleport>

  <div ref="content" class="flex w-full gap-x-1 sm:gap-x-4 p-1 sm:p-4">
    <!-- Search results column -->
    <div class="flex-auto basis-3/4 flex-col gap-y-1 sm:flex sm:gap-y-4" :class="filtersEnabled ? 'hidden' : 'flex'">
      <SearchResultsHeader
        v-model:search-view="searchViewValue"
        :search-session="searchSession"
        :search-total="searchTotal"
        :search-more-than-total="searchMoreThanTotal"
      />

      <template v-if="searchSession !== null && searchTotal !== null && searchTotal > 0">
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

        <Button v-if="searchHasMore" ref="searchMoreButton" :progress="searchProgress" primary class="w-1/4 min-w-fit self-center" @click="searchLoadMore"
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

      <div v-else-if="searchSession === null || searchTotal === null || filtersTotal === null" class="my-1 sm:my-4">
        <div class="text-center text-sm">Determining filters...</div>
      </div>

      <div v-else-if="filtersTotal === 0" class="my-1 sm:my-4">
        <div class="text-center text-sm">No filters available.</div>
      </div>

      <template v-else-if="filtersTotal > 0">
        <div class="text-center text-sm">{{ filtersTotal }} filters available.</div>

        <template v-for="filter in limitedFiltersResults" :key="filter.id">
          <RelFiltersResult
            v-if="filter.type === 'rel'"
            :search-session-id="searchSession.id"
            :search-total="searchTotal"
            :result="filter"
            :state="filtersState.rel[filter.id] ?? []"
            :update-progress="updateFiltersProgress"
            @update:state="onRelFiltersStateUpdate(filter.id, $event)"
          />

          <AmountFiltersResult
            v-if="filter.type === 'amount'"
            :search-session-id="searchSession.id"
            :search-total="searchTotal"
            :result="filter"
            :state="filtersState.amount[`${filter.id}/${filter.unit}`] ?? null"
            :update-progress="updateFiltersProgress"
            @update:state="onAmountFiltersStateUpdate(filter.id, filter.unit, $event)"
          />

          <TimeFiltersResult
            v-if="filter.type === 'time'"
            :search-session-id="searchSession.id"
            :search-total="searchTotal"
            :result="filter"
            :state="filtersState.time[filter.id] ?? null"
            :update-progress="updateFiltersProgress"
            @update:state="onTimeFiltersStateUpdate(filter.id, $event)"
          />

          <StringFiltersResult
            v-if="filter.type === 'string'"
            :search-session-id="searchSession.id"
            :search-total="searchTotal"
            :result="filter"
            :state="filtersState.str[filter.id] ?? []"
            :update-progress="updateFiltersProgress"
            @update:state="onStringFiltersStateUpdate(filter.id, $event)"
          />
        </template>

        <Button v-if="filtersHasMore" ref="filtersMoreButton" :progress="filtersProgress" primary class="w-1/2 min-w-fit self-center" @click="filtersLoadMore"
          >More filters</Button
        >

        <div v-else-if="filtersTotal > limitedFiltersResults.length" class="text-center text-sm">
          {{ filtersTotal - limitedFiltersResults.length }} filters not shown.
        </div>
      </template>
    </div>
  </div>

  <Teleport v-if="(searchTotal !== null && searchTotal > 0 && !searchHasMore) || searchTotal === 0" to="footer">
    <Footer class="border-t border-slate-50 bg-slate-200 shadow" />
  </Teleport>
</template>
