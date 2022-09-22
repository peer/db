<script setup lang="ts">
import type { FilterState, FiltersState, ClientQuery } from "@/types"

import { ref, computed, watch, onMounted, onBeforeUnmount } from "vue"
import { useRoute, useRouter } from "vue-router"
import SearchResult from "@/components/SearchResult.vue"
import RelFiltersResult from "@/components/RelFiltersResult.vue"
import AmountFiltersResult from "@/components/AmountFiltersResult.vue"
import NavBar from "@/components/NavBar.vue"
import Footer from "@/components/Footer.vue"
import Button from "@/components/Button.vue"
import NavBarSearch from "@/components/NavBarSearch.vue"
import { useSearch, useFilters, postFilters } from "@/search"
import { useVisibilityTracking } from "@/visibility"
import { globalProgress } from "@/api"

const router = useRouter()
const route = useRoute()

const searchProgress = ref(0)
const {
  docs: searchDocs,
  total: searchTotal,
  results: searchResults,
  filters: searchFilters,
  moreThanTotal: searchMoreThanTotal,
  hasMore: searchHasMore,
  loadMore: searchLoadMore,
} = useSearch(searchProgress, async (query) => {
  await router.replace({
    name: "DocumentSearch",
    // Maybe route.query has "at" parameter which we want to keep.
    query: { ...route.query, ...query },
  })
})

const filtersProgress = ref(0)
const { docs: filtersDocs, total: filtersTotal, hasMore: filtersHasMore, loadMore: filtersLoadMore } = useFilters(filtersProgress)

const idToIndex = computed(() => {
  const map = new Map<string, number>()
  for (const [i, doc] of searchDocs.value.entries()) {
    map.set(doc._id, i)
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
    if (!topId && searchTotal === null) {
      return
    }
    // We set "s", "at", and "q" here to undefined so that we control their order in the query string.
    const query: ClientQuery = { s: undefined, at: undefined, q: undefined, ...route.query }
    if (!topId) {
      delete query.at
    } else {
      query.at = topId
    }
    await router.replace({
      name: route.name as string,
      params: route.params,
      query: query,
      hash: route.hash,
    })
  },
  { immediate: true },
)

const searchMoreButton = ref()
const filtersMoreButton = ref()
const supportPageOffset = window.pageYOffset !== undefined

function onScroll() {
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

const updateFiltersProgress = ref(0)
// TODO: Is it OK that we modify this computed reference later on (when initializing arrays for new properties).
const filtersState = computed(() => Object.assign({}, searchFilters.value as FiltersState))

async function onFiltersStateUpdate(id: string, s: FilterState) {
  updateFiltersProgress.value += 1
  try {
    const updatedState = Object.assign({}, filtersState.value)
    updatedState[id] = s
    await postFilters(router, route, updatedState, updateFiltersProgress)
  } finally {
    updateFiltersProgress.value -= 1
  }
}
</script>

<template>
  <Teleport to="header">
    <NavBar :progress="globalProgress">
      <NavBarSearch />
    </NavBar>
  </Teleport>
  <div class="mt-12 flex w-full gap-x-1 border-t border-transparent p-1 sm:mt-[4.5rem] sm:gap-x-4 sm:p-4">
    <div class="flex flex-auto basis-3/4 flex-col gap-y-1 sm:gap-y-4">
      <div v-if="searchTotal === 0">
        <div class="my-1 sm:my-4">
          <div class="text-center text-sm">No results found.</div>
        </div>
      </div>
      <template v-else-if="searchTotal !== null && searchTotal > 0">
        <template v-for="(doc, i) in searchDocs" :key="doc._id">
          <div v-if="i === 0 && searchMoreThanTotal" class="my-1 sm:my-4">
            <div class="text-center text-sm">Showing first {{ searchResults.length }} of more than {{ searchTotal }} results found.</div>
            <div class="h-1 w-full bg-slate-200"></div>
          </div>
          <div v-if="i === 0 && searchResults.length < searchTotal && !searchMoreThanTotal" class="my-1 sm:my-4">
            <div class="text-center text-sm">Showing first {{ searchResults.length }} of {{ searchTotal }} results found.</div>
            <div class="h-1 w-full bg-slate-200"></div>
          </div>
          <div v-if="i === 0 && searchResults.length == searchTotal && !searchMoreThanTotal" class="my-1 sm:my-4">
            <div class="text-center text-sm">Found {{ searchTotal }} results.</div>
            <div class="h-1 w-full bg-slate-200"></div>
          </div>
          <div v-else-if="i > 0 && i % 10 === 0" class="my-1 sm:my-4">
            <div v-if="searchResults.length < searchTotal" class="text-center text-sm">{{ i }} of {{ searchResults.length }} shown results.</div>
            <div v-else-if="searchResults.length == searchTotal" class="text-center text-sm">{{ i }} of {{ searchResults.length }} results.</div>
            <div class="relative h-2 w-full bg-slate-200">
              <div class="absolute inset-y-0 bg-secondary-400" style="left: 0" :style="{ width: (i / searchResults.length) * 100 + '%' }"></div>
            </div>
          </div>
          <SearchResult :ref="(track(doc._id) as any)" :doc="doc" />
        </template>
        <Button v-if="searchHasMore" ref="searchMoreButton" :progress="searchProgress" class="w-1/4 min-w-fit self-center" @click="searchLoadMore">Load more</Button>
        <div v-else class="my-1 sm:my-4">
          <div v-if="searchMoreThanTotal" class="text-center text-sm">All of first {{ searchResults.length }} shown of more than {{ searchTotal }} results found.</div>
          <div v-else-if="searchResults.length < searchTotal && !searchMoreThanTotal" class="text-center text-sm">
            All of first {{ searchResults.length }} shown of {{ searchTotal }} results found.
          </div>
          <div v-else-if="searchResults.length == searchTotal && !searchMoreThanTotal" class="text-center text-sm">All of {{ searchResults.length }} results shown.</div>
          <div class="relative h-2 w-full bg-slate-200">
            <div class="absolute inset-y-0 bg-secondary-400" style="left: 0" :style="{ width: 100 + '%' }"></div>
          </div>
        </div>
      </template>
    </div>
    <div class="flex flex-auto basis-1/4 flex-col gap-y-1 sm:gap-y-4">
      <div v-if="filtersTotal === 0">
        <div class="my-1 sm:my-4">
          <div class="text-center text-sm">No filters available.</div>
        </div>
      </div>
      <template v-else-if="searchTotal !== null && filtersTotal !== null && filtersTotal > 0">
        <div class="text-center text-sm">{{ filtersTotal }} filters available.</div>
        <template v-for="doc in filtersDocs" :key="doc._id">
          <RelFiltersResult
            v-if="doc._type === 'rel'"
            :search-total="searchTotal"
            :property="doc"
            :state="filtersState[doc._id] || (filtersState[doc._id] = [])"
            :update-progress="updateFiltersProgress"
            @update:state="onFiltersStateUpdate(doc._id, $event)"
          />
          <AmountFiltersResult
            v-if="doc._type === 'amount'"
            :search-total="searchTotal"
            :property="doc"
            :state="filtersState[doc._id] || (filtersState[doc._id] = [])"
            :update-progress="updateFiltersProgress"
            @update:state="onFiltersStateUpdate(doc._id, $event)"
          />
        </template>
        <Button v-if="filtersHasMore" ref="filtersMoreButton" :progress="filtersProgress" class="w-1/2 min-w-fit self-center" @click="filtersLoadMore"
          >More filters</Button
        >
        <div v-else-if="filtersTotal > filtersDocs.length" class="text-center text-sm">{{ filtersTotal - filtersDocs.length }} filters not shown.</div>
      </template>
    </div>
  </div>
  <Teleport v-if="(searchTotal !== null && searchTotal > 0 && !searchHasMore) || searchTotal === 0" to="footer">
    <Footer class="border-t border-slate-50 bg-slate-200 shadow" />
  </Teleport>
</template>
