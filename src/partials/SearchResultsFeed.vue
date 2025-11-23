<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type {
  AmountFilterState,
  ClientSearchState,
  FiltersState,
  IndexFilterState,
  RelFilterState,
  SearchResult as SearchResultType,
  SizeFilterState,
  StringFilterState,
  TimeFilterState,
  SearchViewType,
  FilterStateChange,
  AmountUnit,
} from "@/types"

import { useRoute, useRouter } from "vue-router"
import { computed, onBeforeUnmount, onMounted, ref, toRef, watch } from "vue"

import Button from "@/components/Button.vue"
import SearchResult from "@/partials/SearchResult.vue"
import SearchResultsHeader from "@/partials/SearchResultsHeader.vue"
import RelFiltersResult from "@/partials/RelFiltersResult.vue"
import IndexFiltersResult from "@/partials/IndexFiltersResult.vue"
import TimeFiltersResult from "@/partials/TimeFiltersResult.vue"
import StringFiltersResult from "@/partials/StringFiltersResult.vue"
import SizeFiltersResult from "@/partials/SizeFiltersResult.vue"
import AmountFiltersResult from "@/partials/AmountFiltersResult.vue"
import { useVisibilityTracking } from "@/visibility"
import { encodeQuery, useLimitResults } from "@/utils.ts"
import { SEARCH_INITIAL_LIMIT, SEARCH_INCREASE, FILTERS_INCREASE, FILTERS_INITIAL_LIMIT, useFilters, activeSearchState } from "@/search.ts"
import { injectProgress } from "@/progress.ts"

const props = defineProps<{
  searchView: SearchViewType

  // Search props.
  s: string
  searchResults: DeepReadonly<SearchResultType[]>
  searchTotal: number | null
  searchMoreThanTotal: boolean
  searchState: DeepReadonly<ClientSearchState | null>
  searchProgress: number

  // Filter props.
  filtersEnabled: boolean
  filtersState: FiltersState
  updateFiltersProgress: number
}>()

const $emit = defineEmits<{
  onFilterChange: [change: FilterStateChange]
  "update:searchView": [value: SearchViewType]
}>()

const router = useRouter()
const route = useRoute()

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

const filtersProgress = injectProgress()
const {
  results: filtersResults,
  total: filtersTotal,
  error: filtersError,
  url: filtersURL,
} = useFilters(
  activeSearchState(
    toRef(() => props.searchState),
    toRef(() => props.s),
  ),
  filtersEl,
  filtersProgress,
)

const {
  limitedResults: limitedFiltersResults,
  hasMore: filtersHasMore,
  loadMore: filtersLoadMore,
} = useLimitResults(filtersResults, FILTERS_INITIAL_LIMIT, FILTERS_INCREASE)

const { track, visibles } = useVisibilityTracking()

onMounted(() => {
  window.addEventListener("scroll", onScroll, { passive: true })
})

onBeforeUnmount(() => {
  abortController.abort()

  window.removeEventListener("scroll", onScroll)
})

const abortController = new AbortController()

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

const idToIndex = computed(() => {
  const map = new Map<string, number>()
  for (const [i, result] of props.searchResults.entries()) {
    map.set(result.id, i)
  }
  return map
})

const initialRouteName = route.name
watch(
  () => {
    const sorted = Array.from(visibles)
    sorted.sort((a, b) => (idToIndex.value.get(a) ?? Infinity) - (idToIndex.value.get(b) ?? Infinity))
    return sorted[0]
  },
  async (topId, oldTopId, onCleanup) => {
    // Watch can continue to run for some time after the route changes.
    if (initialRouteName !== route.name) {
      return
    }
    // Initial data has not yet been loaded, so we wait.
    if (!topId && props.searchTotal === null) {
      return
    }
    await router.replace({
      name: route.name as string,
      params: route.params,
      // We do not want to set an empty "at" query parameter.
      query: encodeQuery({ ...route.query, at: topId || undefined }),
      hash: route.hash,
    })
  },
  {
    immediate: true,
  },
)

function onScroll() {
  if (abortController.signal.aborted) {
    return
  }
  if (!searchMoreButton.value && !filtersMoreButton.value) {
    return
  }

  const viewportHeight = document.documentElement.clientHeight || document.body.clientHeight
  const scrollHeight = Math.max(
    document.body.scrollHeight,
    document.documentElement.scrollHeight,
    document.body.offsetHeight,
    document.documentElement.offsetHeight,
    document.body.clientHeight,
    document.documentElement.clientHeight,
  )
  const currentScrollPosition = supportPageOffset ? window.pageYOffset : document.documentElement.scrollTop

  if (currentScrollPosition > scrollHeight - 2 * viewportHeight) {
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

function onRelFiltersStateUpdate(id: string, value: RelFilterState) {
  $emit("onFilterChange", { type: "rel", id, value })
}

function onAmountFiltersStateUpdate(id: string, unit: AmountUnit, value: AmountFilterState) {
  $emit("onFilterChange", { type: "amount", id, unit, value })
}

function onTimeFiltersStateUpdate(id: string, value: TimeFilterState) {
  $emit("onFilterChange", { type: "time", id, value })
}

function onStringFiltersStateUpdate(id: string, value: StringFilterState) {
  $emit("onFilterChange", { type: "string", id, value })
}

function onIndexFiltersStateUpdate(value: IndexFilterState) {
  $emit("onFilterChange", { type: "index", value })
}

function onSizeFiltersStateUpdate(value: SizeFilterState) {
  $emit("onFilterChange", { type: "size", value })
}
</script>

<template>
  <div class="flex w-full gap-x-1 sm:gap-x-4">
    <!-- Search results column -->
    <div class="flex-auto basis-3/4 flex-col gap-y-1 sm:flex sm:gap-y-4" :class="filtersEnabled ? 'hidden' : 'flex'">
      <SearchResultsHeader v-model:search-view="searchViewValue" :search-state="searchState" :search-total="searchTotal" :search-more-than-total="searchMoreThanTotal" />

      <template v-if="searchTotal !== null && searchTotal > 0">
        <template v-for="(result, i) in limitedSearchResults" :key="result.id">
          <div v-if="i > 0 && i % 10 === 0" class="my-1 sm:my-4">
            <div v-if="searchResults.length < searchTotal" class="text-center text-sm">{{ i }} of {{ searchResults.length }} shown results.</div>
            <div v-else-if="searchResults.length == searchTotal" class="text-center text-sm">{{ i }} of {{ searchResults.length }} results.</div>
            <div class="relative h-2 w-full bg-slate-200">
              <div class="absolute inset-y-0 bg-secondary-400" style="left: 0" :style="{ width: (i / searchResults.length) * 100 + '%' }" />
            </div>
          </div>
          <SearchResult :ref="track(result.id) as any" :s="s" :result="result" />
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

      <div v-else-if="searchTotal === null || filtersTotal === null" class="my-1 sm:my-4">
        <div class="text-center text-sm">Determining filters...</div>
      </div>

      <div v-else-if="filtersTotal === 0" class="my-1 sm:my-4">
        <div class="text-center text-sm">No filters available.</div>
      </div>

      <template v-else-if="filtersTotal > 0">
        <div class="text-center text-sm">{{ filtersTotal }} filters available.</div>

        <template v-for="result in limitedFiltersResults" :key="'id' in result ? result.id : result.type">
          <RelFiltersResult
            v-if="result.type === 'rel'"
            :s="s"
            :search-total="searchTotal"
            :result="result"
            :state="filtersState.rel[result.id] ?? []"
            :update-progress="updateFiltersProgress"
            @update:state="onRelFiltersStateUpdate(result.id, $event)"
          />

          <AmountFiltersResult
            v-if="result.type === 'amount'"
            :s="s"
            :search-total="searchTotal"
            :result="result"
            :state="filtersState.amount[`${result.id}/${result.unit}`] ?? null"
            :update-progress="updateFiltersProgress"
            @update:state="onAmountFiltersStateUpdate(result.id, result.unit, $event)"
          />

          <TimeFiltersResult
            v-if="result.type === 'time'"
            :s="s"
            :search-total="searchTotal"
            :result="result"
            :state="filtersState.time[result.id] ?? null"
            :update-progress="updateFiltersProgress"
            @update:state="onTimeFiltersStateUpdate(result.id, $event)"
          />

          <StringFiltersResult
            v-if="result.type === 'string'"
            :s="s"
            :search-total="searchTotal"
            :result="result"
            :state="filtersState.str[result.id] ?? []"
            :update-progress="updateFiltersProgress"
            @update:state="onStringFiltersStateUpdate(result.id, $event)"
          />

          <IndexFiltersResult
            v-if="result.type === 'index'"
            :s="s"
            :search-total="searchTotal"
            :result="result"
            :state="filtersState.index"
            :update-progress="updateFiltersProgress"
            @update:state="onIndexFiltersStateUpdate($event)"
          />

          <SizeFiltersResult
            v-if="result.type === 'size'"
            :s="s"
            :search-total="searchTotal"
            :result="result"
            :state="filtersState.size"
            :update-progress="updateFiltersProgress"
            @update:state="onSizeFiltersStateUpdate($event)"
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
</template>
