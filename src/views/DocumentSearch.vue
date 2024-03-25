<script setup lang="ts">
import type {
  RelFilterState,
  AmountFilterState,
  TimeFilterState,
  StringFilterState,
  IndexFilterState,
  SizeFilterState,
  FiltersState,
  RelSearchResult,
  AmountSearchResult,
  TimeSearchResult,
  StringSearchResult,
  IndexSearchResult,
  SizeSearchResult,
} from "@/types"

import { ref, computed, watch, onMounted, onBeforeUnmount, watchEffect } from "vue"
import { useRoute, useRouter } from "vue-router"
import SearchResult from "@/components/SearchResult.vue"
import RelFiltersResult from "@/components/RelFiltersResult.vue"
import AmountFiltersResult from "@/components/AmountFiltersResult.vue"
import TimeFiltersResult from "@/components/TimeFiltersResult.vue"
import StringFiltersResult from "@/components/StringFiltersResult.vue"
import IndexFiltersResult from "@/components/IndexFiltersResult.vue"
import SizeFiltersResult from "@/components/SizeFiltersResult.vue"
import NavBar from "@/components/NavBar.vue"
import Footer from "@/components/Footer.vue"
import Button from "@/components/Button.vue"
import NavBarSearch from "@/components/NavBarSearch.vue"
import { useSearch, useFilters, postFilters, SEARCH_INITIAL_LIMIT, SEARCH_INCREASE, FILTERS_INITIAL_LIMIT, FILTERS_INCREASE } from "@/search"
import { useVisibilityTracking } from "@/visibility"
import { clone, useLimitResults, encodeQuery } from "@/utils"
import { injectMainProgress, localProgress } from "@/progress"

const router = useRouter()
const route = useRoute()

const mainProgress = injectMainProgress()

const abortController = new AbortController()

onBeforeUnmount(() => {
  abortController.abort()
})

const searchEl = ref(null)

const searchProgress = localProgress(mainProgress)
const {
  results: searchResults,
  total: searchTotal,
  filters: searchFilters,
  moreThanTotal: searchMoreThanTotal,
  error: searchError,
  url: searchURL,
} = useSearch(searchEl, searchProgress, async (query) => {
  await router.replace({
    name: "DocumentSearch",
    // Maybe route.query has non-empty "at" parameter which we want to keep.
    query: encodeQuery({ at: route.query.at || undefined, ...query }),
  })
})

const { limitedResults: limitedSearchResults, hasMore: searchHasMore, loadMore: searchLoadMore } = useLimitResults(searchResults, SEARCH_INITIAL_LIMIT, SEARCH_INCREASE)

const filtersEl = ref(null)

const filtersProgress = localProgress(mainProgress)
const { results: filtersResults, total: filtersTotal, error: filtersError, url: filtersURL } = useFilters(filtersEl, filtersProgress)

const {
  limitedResults: limitedFiltersResults,
  hasMore: filtersHasMore,
  loadMore: filtersLoadMore,
} = useLimitResults(filtersResults, FILTERS_INITIAL_LIMIT, FILTERS_INCREASE)

const idToIndex = computed(() => {
  const map = new Map<string, number>()
  for (const [i, result] of searchResults.value.entries()) {
    map.set(result.id, i)
  }
  return map
})

const { track, visibles } = useVisibilityTracking()

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
    if (!topId && searchTotal.value === null) {
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

const searchMoreButton = ref()
const filtersMoreButton = ref()
const supportPageOffset = window.pageYOffset !== undefined

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
    if (searchMoreButton.value) {
      searchMoreButton.value.$el.click()
    }
    if (filtersMoreButton.value) {
      filtersMoreButton.value.$el.click()
    }
  }
}

onMounted(() => {
  window.addEventListener("scroll", onScroll, { passive: true })
})

onBeforeUnmount(() => {
  window.removeEventListener("scroll", onScroll)
})

const updateFiltersProgress = localProgress(mainProgress)
// A non-read-only version of filters state so that we can modify it as necessary.
const filtersState = ref<FiltersState>({ rel: {}, amount: {}, time: {}, str: {}, index: [], size: null })
// We keep it in sync with upstream version.
watchEffect((onCleanup) => {
  // We copy to make a read-only value mutable.
  filtersState.value = clone(searchFilters.value)
})

async function onRelFiltersStateUpdate(id: string, s: RelFilterState) {
  if (abortController.signal.aborted) {
    return
  }

  updateFiltersProgress.value += 1
  try {
    const updatedState = { ...filtersState.value }
    updatedState.rel = { ...updatedState.rel }
    updatedState.rel[id] = s
    await postFilters(router, route, updatedState, abortController.signal, updateFiltersProgress)
  } catch (err) {
    if (abortController.signal.aborted) {
      return
    }
    // TODO: Show notification with error.
    console.error("onRelFiltersStateUpdate", err)
  } finally {
    updateFiltersProgress.value -= 1
  }
}

async function onAmountFiltersStateUpdate(id: string, unit: string, s: AmountFilterState) {
  if (abortController.signal.aborted) {
    return
  }

  updateFiltersProgress.value += 1
  try {
    const updatedState = { ...filtersState.value }
    updatedState.amount = { ...updatedState.amount }
    updatedState.amount[`${id}/${unit}`] = s
    await postFilters(router, route, updatedState, abortController.signal, updateFiltersProgress)
  } catch (err) {
    if (abortController.signal.aborted) {
      return
    }
    // TODO: Show notification with error.
    console.error("onAmountFiltersStateUpdate", err)
  } finally {
    updateFiltersProgress.value -= 1
  }
}

async function onTimeFiltersStateUpdate(id: string, s: TimeFilterState) {
  if (abortController.signal.aborted) {
    return
  }

  updateFiltersProgress.value += 1
  try {
    const updatedState = { ...filtersState.value }
    updatedState.time = { ...updatedState.time }
    updatedState.time[id] = s
    await postFilters(router, route, updatedState, abortController.signal, updateFiltersProgress)
  } catch (err) {
    if (abortController.signal.aborted) {
      return
    }
    // TODO: Show notification with error.
    console.error("onTimeFiltersStateUpdate", err)
  } finally {
    updateFiltersProgress.value -= 1
  }
}

async function onStringFiltersStateUpdate(id: string, s: StringFilterState) {
  if (abortController.signal.aborted) {
    return
  }

  updateFiltersProgress.value += 1
  try {
    const updatedState = { ...filtersState.value }
    updatedState.str = { ...updatedState.str }
    updatedState.str[id] = s
    await postFilters(router, route, updatedState, abortController.signal, updateFiltersProgress)
  } catch (err) {
    if (abortController.signal.aborted) {
      return
    }
    // TODO: Show notification with error.
    console.error("onStringFiltersStateUpdate", err)
  } finally {
    updateFiltersProgress.value -= 1
  }
}

async function onIndexFiltersStateUpdate(s: IndexFilterState) {
  if (abortController.signal.aborted) {
    return
  }

  updateFiltersProgress.value += 1
  try {
    const updatedState = { ...filtersState.value }
    updatedState.index = s
    await postFilters(router, route, updatedState, abortController.signal, updateFiltersProgress)
  } catch (err) {
    if (abortController.signal.aborted) {
      return
    }
    // TODO: Show notification with error.
    console.error("onIndexFiltersStateUpdate", err)
  } finally {
    updateFiltersProgress.value -= 1
  }
}

async function onSizeFiltersStateUpdate(s: SizeFilterState) {
  if (abortController.signal.aborted) {
    return
  }

  updateFiltersProgress.value += 1
  try {
    const updatedState = { ...filtersState.value }
    updatedState.size = s
    await postFilters(router, route, updatedState, abortController.signal, updateFiltersProgress)
  } catch (err) {
    if (abortController.signal.aborted) {
      return
    }
    // TODO: Show notification with error.
    console.error("onSizeFiltersStateUpdate", err)
  } finally {
    updateFiltersProgress.value -= 1
  }
}

const filtersEnabled = ref(false)
</script>

<template>
  <Teleport to="header">
    <NavBar :progress="mainProgress">
      <NavBarSearch v-model:filtersEnabled="filtersEnabled" />
    </NavBar>
  </Teleport>
  <div class="mt-12 flex w-full gap-x-1 border-t border-transparent p-1 sm:mt-[4.5rem] sm:gap-x-4 sm:p-4">
    <div ref="searchEl" class="flex-auto basis-3/4 flex-col gap-y-1 sm:flex sm:gap-y-4" :class="filtersEnabled ? 'hidden' : 'flex'" :data-url="searchURL">
      <div v-if="searchError" class="my-1 sm:my-4">
        <div class="text-center text-sm"><i class="text-error-600">loading data failed</i></div>
      </div>
      <div v-else-if="searchTotal === null" class="my-1 sm:my-4">
        <div class="text-center text-sm">Searching...</div>
      </div>
      <div v-else-if="searchTotal === 0" class="my-1 sm:my-4">
        <div class="text-center text-sm">No results found.</div>
      </div>
      <template v-else-if="searchTotal > 0">
        <template v-for="(result, i) in limitedSearchResults" :key="result.id">
          <div v-if="i === 0 && searchMoreThanTotal" class="my-1 sm:my-4">
            <div class="text-center text-sm">Showing first {{ searchResults.length }} of more than {{ searchTotal }} results found.</div>
            <div class="h-2 w-full bg-slate-200"></div>
          </div>
          <div v-if="i === 0 && searchResults.length < searchTotal && !searchMoreThanTotal" class="my-1 sm:my-4">
            <div class="text-center text-sm">Showing first {{ searchResults.length }} of {{ searchTotal }} results found.</div>
            <div class="h-2 w-full bg-slate-200"></div>
          </div>
          <div v-if="i === 0 && searchResults.length == searchTotal && !searchMoreThanTotal" class="my-1 sm:my-4">
            <div class="text-center text-sm">Found {{ searchTotal }} results.</div>
            <div class="h-2 w-full bg-slate-200"></div>
          </div>
          <div v-else-if="i > 0 && i % 10 === 0" class="my-1 sm:my-4">
            <div v-if="searchResults.length < searchTotal" class="text-center text-sm">{{ i }} of {{ searchResults.length }} shown results.</div>
            <div v-else-if="searchResults.length == searchTotal" class="text-center text-sm">{{ i }} of {{ searchResults.length }} results.</div>
            <div class="relative h-2 w-full bg-slate-200">
              <div class="absolute inset-y-0 bg-secondary-400" style="left: 0" :style="{ width: (i / searchResults.length) * 100 + '%' }"></div>
            </div>
          </div>
          <SearchResult :ref="track(result.id) as any" :result="result" />
        </template>
        <Button v-if="searchHasMore" ref="searchMoreButton" :progress="searchProgress" primary class="w-1/4 min-w-fit self-center" @click="searchLoadMore"
          >Load more</Button
        >
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
    <div ref="filtersEl" class="flex-auto basis-1/4 flex-col gap-y-1 sm:flex sm:gap-y-4" :class="filtersEnabled ? 'flex' : 'hidden'" :data-url="filtersURL">
      <div v-if="searchError || filtersError" class="my-1 sm:my-4">
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
        <template v-for="result in limitedFiltersResults" :key="result.id">
          <RelFiltersResult
            v-if="result.type === 'rel'"
            :search-total="searchTotal"
            :result="result as RelSearchResult"
            :state="filtersState.rel[result.id] || (filtersState.rel[result.id] = [])"
            :update-progress="updateFiltersProgress"
            @update:state="onRelFiltersStateUpdate(result.id, $event)"
          />
          <AmountFiltersResult
            v-if="result.type === 'amount'"
            :search-total="searchTotal"
            :result="result as AmountSearchResult"
            :state="filtersState.amount[`${result.id}/${result.unit}`] || (filtersState.amount[`${result.id}/${result.unit}`] = null)"
            :update-progress="updateFiltersProgress"
            @update:state="onAmountFiltersStateUpdate(result.id, result.unit, $event)"
          />
          <TimeFiltersResult
            v-if="result.type === 'time'"
            :search-total="searchTotal"
            :result="result as TimeSearchResult"
            :state="filtersState.time[result.id] || (filtersState.time[result.id] = null)"
            :update-progress="updateFiltersProgress"
            @update:state="onTimeFiltersStateUpdate(result.id, $event)"
          />
          <StringFiltersResult
            v-if="result.type === 'string'"
            :search-total="searchTotal"
            :result="result as StringSearchResult"
            :state="filtersState.str[result.id] || (filtersState.str[result.id] = [])"
            :update-progress="updateFiltersProgress"
            @update:state="onStringFiltersStateUpdate(result.id, $event)"
          />
          <IndexFiltersResult
            v-if="result.type === 'index'"
            :search-total="searchTotal"
            :result="result as IndexSearchResult"
            :state="filtersState.index"
            :update-progress="updateFiltersProgress"
            @update:state="onIndexFiltersStateUpdate($event)"
          />
          <SizeFiltersResult
            v-if="result.type === 'size'"
            :search-total="searchTotal"
            :result="result as SizeSearchResult"
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
  <Teleport v-if="(searchTotal !== null && searchTotal > 0 && !searchHasMore) || searchTotal === 0" to="footer">
    <Footer class="border-t border-slate-50 bg-slate-200 shadow" />
  </Teleport>
</template>
