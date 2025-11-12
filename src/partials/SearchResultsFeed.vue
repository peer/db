<script setup lang="ts">
import Button from "@/components/Button.vue"
import SearchResult from "@/partials/SearchResult.vue"
import {
  AmountFilterState,
  AmountSearchResult,
  ClientSearchState,
  FiltersState,
  FilterState,
  IndexFilterState,
  IndexSearchResult,
  RelFilterState,
  RelSearchResult,
  SearchResult as SearchResultType,
  SearchResultFilterType,
  SizeFilterState,
  SizeSearchResult,
  StringFilterState,
  StringSearchResult,
  TimeFilterState,
  TimeSearchResult,
} from "@/types"
import { ComponentPublicInstance, computed, DeepReadonly, onBeforeUnmount, onMounted } from "vue"
import SearchResultsHeader, { SearchViewType } from "@/partials/SearchResultsHeader.vue"
import RelFiltersResult from "@/partials/RelFiltersResult.vue"
import IndexFiltersResult from "@/partials/IndexFiltersResult.vue"
import TimeFiltersResult from "@/partials/TimeFiltersResult.vue"
import StringFiltersResult from "@/partials/StringFiltersResult.vue"
import SizeFiltersResult from "@/partials/SizeFiltersResult.vue"
import AmountFiltersResult from "@/partials/AmountFiltersResult.vue"

export type SearchResultsFeedProps = {
  // search props
  limitedSearchResults: DeepReadonly<SearchResultType[]>
  searchResults: DeepReadonly<SearchResultType[]>
  searchTotal: number | null
  track: (id: string) => (el: Element | ComponentPublicInstance | null) => void
  s: string
  searchHasMore: boolean
  searchProgress: number
  searchMoreThanTotal: boolean
  searchStateError: string | null
  searchResultsError: string | null
  searchView: SearchViewType
  searchState: DeepReadonly<ClientSearchState | null>
  searchUrl: Readonly<string | null>
  searchEl: HTMLElement | null

  // filter props
  filtersEnabled: boolean
  filtersError: Readonly<string | null>
  filtersUrl: Readonly<string | null>
  filtersTotal: Readonly<number | null>
  limitedFiltersResults: ReadonlyArray<DeepReadonly<RelSearchResult | AmountSearchResult | TimeSearchResult | StringSearchResult | IndexSearchResult | SizeSearchResult>>
  filtersState: FiltersState
  updateFiltersProgress: number
  filtersProgress: number
  filtersHasMore: boolean
  filtersEl: HTMLElement | null
}

const props = defineProps<SearchResultsFeedProps>()

const $emit = defineEmits<{
  onFilterChange: [type: SearchResultFilterType, payload: { id?: string; unit?: string; value: FilterState }]
  onMoreResults: []
  onMoreFilters: []
  "update:searchView": [value: SearchViewType]
}>()

const abortController = new AbortController()

onBeforeUnmount(() => {
  abortController.abort()
})

const searchViewValue = computed({
  get() {
    return props.searchView
  },
  set(value) {
    $emit("update:searchView", value)
  },
})

const supportPageOffset = window.pageYOffset !== undefined

function onScroll() {
  if (abortController.signal.aborted) {
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
    $emit("onMoreResults")
    $emit("onMoreFilters")
  }
}

onMounted(() => {
  window.addEventListener("scroll", onScroll, { passive: true })
})

onBeforeUnmount(() => {
  window.removeEventListener("scroll", onScroll)
})

function onRelFiltersStateUpdate(id: string, value: RelFilterState) {
  $emit("onFilterChange", "rel", { id, value })
}

function onAmountFiltersStateUpdate(id: string, unit: string, value: AmountFilterState) {
  $emit("onFilterChange", "amount", { id, unit, value })
}

function onTimeFiltersStateUpdate(id: string, value: TimeFilterState) {
  $emit("onFilterChange", "time", { id, value })
}

function onStringFiltersStateUpdate(id: string, value: StringFilterState) {
  $emit("onFilterChange", "string", { id, value })
}

function onIndexFiltersStateUpdate(value: IndexFilterState) {
  $emit("onFilterChange", "index", { value })
}

function onSizeFiltersStateUpdate(value: SizeFilterState) {
  $emit("onFilterChange", "size", { value })
}
</script>

<template>
  <div class="flex w-full gap-x-1 border-t border-transparent sm:gap-x-4">
    <!-- Search results column -->
    <div ref="searchEl" class="flex-auto basis-3/4 flex-col gap-y-1 sm:flex sm:gap-y-4" :class="filtersEnabled ? 'hidden' : 'flex'" :data-url="searchUrl">
      <div v-if="searchStateError || searchResultsError" class="my-1 sm:my-4">
        <div class="text-center text-sm">
          <i class="text-error-600">loading data failed</i>
        </div>
      </div>

      <SearchResultsHeader
        v-else
        v-model:view="searchViewValue"
        :state="searchState"
        :total="searchTotal"
        :results="searchResults.length"
        :more-than-total="searchMoreThanTotal"
      />

      <template v-if="!searchStateError && !searchResultsError && searchTotal !== null && searchTotal > 0">
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

        <Button v-if="searchHasMore" ref="searchMoreButton" :progress="searchProgress" primary class="w-1/4 min-w-fit self-center" @click="$emit('onMoreResults')">
          Load more
        </Button>

        <div v-else class="my-1 sm:my-4">
          <div v-if="searchMoreThanTotal" class="text-center text-sm">All of first {{ searchResults.length }} shown of more than {{ searchTotal }} results found.</div>
          <div v-else-if="searchResults.length < searchTotal && !searchMoreThanTotal" class="text-center text-sm">
            All of first {{ searchResults.length }} shown of {{ searchTotal }} results found.
          </div>
          <div v-else-if="searchResults.length === searchTotal && !searchMoreThanTotal" class="text-center text-sm">All of {{ searchResults.length }} results shown.</div>
          <div class="relative h-2 w-full bg-slate-200">
            <div class="absolute inset-y-0 bg-secondary-400" style="left: 0" :style="{ width: 100 + '%' }"></div>
          </div>
        </div>
      </template>
    </div>

    <!-- Filters column -->
    <div ref="filtersEl" class="flex-auto basis-1/4 flex-col gap-y-1 sm:flex sm:gap-y-4" :class="filtersEnabled ? 'flex' : 'hidden'" :data-url="filtersUrl">
      <div v-if="searchStateError || searchResultsError || filtersError" class="my-1 sm:my-4">
        <div class="text-center text-sm">
          <i class="text-error-600">loading data failed</i>
        </div>
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
            :result="result as RelSearchResult"
            :state="filtersState.rel[result.id] ?? []"
            :update-progress="updateFiltersProgress"
            @update:state="onRelFiltersStateUpdate(result.id, $event)"
          />

          <AmountFiltersResult
            v-if="result.type === 'amount'"
            :s="s"
            :search-total="searchTotal"
            :result="result as AmountSearchResult"
            :state="filtersState.amount[`${result.id}/${result.unit}`] ?? null"
            :update-progress="updateFiltersProgress"
            @update:state="onAmountFiltersStateUpdate(result.id, result.unit, $event)"
          />

          <TimeFiltersResult
            v-if="result.type === 'time'"
            :s="s"
            :search-total="searchTotal"
            :result="result as TimeSearchResult"
            :state="filtersState.time[result.id] ?? null"
            :update-progress="updateFiltersProgress"
            @update:state="onTimeFiltersStateUpdate(result.id, $event)"
          />

          <StringFiltersResult
            v-if="result.type === 'string'"
            :s="s"
            :search-total="searchTotal"
            :result="result as StringSearchResult"
            :state="filtersState.str[result.id] ?? []"
            :update-progress="updateFiltersProgress"
            @update:state="onStringFiltersStateUpdate(result.id, $event)"
          />

          <IndexFiltersResult
            v-if="result.type === 'index'"
            :s="s"
            :search-total="searchTotal"
            :result="result as IndexSearchResult"
            :state="filtersState.index"
            :update-progress="updateFiltersProgress"
            @update:state="onIndexFiltersStateUpdate($event)"
          />

          <SizeFiltersResult
            v-if="result.type === 'size'"
            :s="s"
            :search-total="searchTotal"
            :result="result as SizeSearchResult"
            :state="filtersState.size"
            :update-progress="updateFiltersProgress"
            @update:state="onSizeFiltersStateUpdate($event)"
          />
        </template>

        <Button v-if="filtersHasMore" :progress="filtersProgress" primary class="w-1/2 min-w-fit self-center" @click="$emit('onMoreFilters')"> More filters </Button>

        <div v-else-if="filtersTotal > limitedFiltersResults.length" class="text-center text-sm">
          {{ filtersTotal - limitedFiltersResults.length }} filters not shown.
        </div>
      </template>
    </div>
  </div>
</template>
