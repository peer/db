<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { ClientSearchState, SearchResult as SearchResultType, SearchViewType } from "@/types"
import type { PeerDBDocument } from "@/document.ts"

import { computed, toRef, ref, onMounted, onBeforeUnmount } from "vue"

import Footer from "@/partials/Footer.vue"
import SearchResultsHeader from "@/partials/SearchResultsHeader.vue"
import ClaimValue from "@/partials/ClaimValue.vue"
import WithDocument from "@/components/WithDocument.vue"
import Button from "@/components/Button.vue"
import { encodeQuery, getBestClaimOfType, useLimitResults } from "@/utils.ts"
import { activeSearchState, FILTERS_INCREASE, FILTERS_INITIAL_LIMIT, useFilters, useLocationAt } from "@/search.ts"
import { injectProgress } from "@/progress.ts"
import { useVisibilityTracking } from "@/visibility.ts"
import DocumentRefInline from "@/partials/DocumentRefInline.vue"

const props = defineProps<{
  searchView: SearchViewType

  // Search props.
  s: string
  searchResults: DeepReadonly<SearchResultType[]>
  searchTotal: number | null
  searchMoreThanTotal: boolean
  searchState: DeepReadonly<ClientSearchState | null>
  searchProgress: number
}>()

const $emit = defineEmits<{
  "update:searchView": [value: SearchViewType]
}>()

const SEARCH_INITIAL_LIMIT = 100
const SEARCH_INCREASE = 100

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

onMounted(async () => {
  window.addEventListener("scroll", onScrollOrResize, { passive: true })
  window.addEventListener("resize", onScrollOrResize, { passive: true })
})

onBeforeUnmount(() => {
  abortController.abort()

  window.removeEventListener("scroll", onScrollOrResize)
  window.removeEventListener("resize", onScrollOrResize)
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

useLocationAt(
  toRef(() => props.searchResults),
  toRef(() => props.searchTotal),
  visibles,
)

function onScrollOrResize() {
  if (abortController.signal.aborted) {
    return
  }

  if (searchMoreButton.value) {
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
      searchMoreButton.value.$el.click()
    }
  }

  if (filtersMoreButton.value) {
    const viewportWidth = document.documentElement.clientWidth || document.body.clientWidth
    const scrollWidth = Math.max(
      document.body.scrollWidth,
      document.documentElement.scrollWidth,
      document.body.offsetWidth,
      document.documentElement.offsetWidth,
      document.body.clientWidth,
      document.documentElement.clientWidth,
    )
    const currentScrollXPosition = supportPageOffset ? window.pageXOffset : document.documentElement.scrollLeft

    if (currentScrollXPosition > scrollWidth - 2 * viewportWidth) {
      // We load more by clicking the button so that we have one place to disable loading more (by disabling the button).
      // This assures that UX is consistent and that user cannot load more through any interaction (click or scroll).
      filtersMoreButton.value.$el.click()
    }
  }
}

const WithPeerDBDocument = WithDocument<PeerDBDocument>
</script>

<template>
  <!--
    TODO: No idea why w-0 (and w-fit) work here, but w-full does not.
          One would assume that w-full is needed to make the container div as wide as the
          body inside which then the footer horizontally shifts.
  -->
  <div class="sticky left-0 w-0 z-20">
    <SearchResultsHeader
      v-model:search-view="searchViewValue"
      class="w-container p-1 sm:p-4"
      :search-state="searchState"
      :search-total="searchTotal"
      :search-more-than-total="searchMoreThanTotal"
    />
  </div>

  <template v-if="searchTotal !== null && searchTotal > 0">

    <div class="flex w-fit flex-row gap-x-1 sm:gap-x-4 px-1 sm:px-4">
      <!-- TODO: Make table have rounded corners. -->
      <table class="shadow border">
        <!-- Headers -->
        <thead class="bg-slate-300">
          <tr>
            <th class="p-2 text-start">#</th>
            <template v-for="filter in limitedFiltersResults" :key="'id' in filter ? filter.id : filter.type">
              <th v-if="filter.type === 'rel' || filter.type === 'amount' || filter.type === 'time' || filter.type === 'string'" class="p-2 truncate text-start max-w-[400px]">
                <DocumentRefInline :id="filter.id" class="text-lg leading-none" />
              </th>
            </template>
          </tr>
        </thead>

        <!-- Results -->
        <tbody class="divide-y">
          <tr v-for="(result, index) in limitedSearchResults" :key="result.id" :ref="track(result.id) as any" class="odd:bg-white even:bg-slate-100 hover:bg-slate-200">
            <td class="p-2 text-start">
              <RouterLink :to="{ name: 'DocumentGet', params: { id: result.id }, query: encodeQuery({ s }) }" class="link">{{ index + 1 }}</RouterLink>
            </td>
            <WithPeerDBDocument :id="result.id" name="DocumentGet">
              <template #default="{ doc }">
                <template v-for="filter in limitedFiltersResults" :key="'id' in filter ? filter.id : filter.type">
                  <td v-if="filter.type === 'rel' || filter.type === 'amount' || filter.type === 'time' || filter.type === 'string'" class="p-2 truncate max-w-[400px]">
                    <ClaimValue
                      :type="filter.type"
                      :claim="getBestClaimOfType(doc.claims, filter.type, filter.id)"
                    />
                  </td>
                </template>
              </template>
            </WithPeerDBDocument>
          </tr>
        </tbody>
      </table>

      <div v-if="filtersHasMore" class="sticky top-[37.5%] h-full z-20">
        <Button
          ref="filtersMoreButton"
          :progress="filtersProgress"
          primary
          class="h-1/4 min-h-fit !py-6 !px-2.5 [writing-mode:sideways-lr]"
          @click="filtersLoadMore"
          >More columns</Button
        >
      </div>
    </div>

    <!--
      TODO: No idea why w-0 (and w-fit) work here, but w-full does not.
            One would assume that w-full is needed to make the container div as wide as the
            body inside which then the footer horizontally shifts.
    -->
    <div class="sticky left-0 w-0 z-20">
      <div class="flex w-container p-1 sm:p-4 justify-center">
        <Button v-if="searchHasMore" ref="searchMoreButton" :progress="searchProgress" primary class="w-1/4 min-w-fit" @click="searchLoadMore"
          >Load more</Button>

        <div v-else class="my-1 sm:my-4">
          <div v-if="searchMoreThanTotal" class="text-center text-sm">All of first {{ searchResults.length }} shown of more than {{ searchTotal }} results found.</div>
          <div v-else-if="searchResults.length < searchTotal" class="text-center text-sm">
            All of first {{ searchResults.length }} shown of {{ searchTotal }} results found.
          </div>
          <div v-else-if="searchResults.length === searchTotal" class="text-center text-sm">All of {{ searchResults.length }} results shown.</div>
        </div>
      </div>
    </div>

  </template>

  <Teleport v-if="(searchTotal !== null && searchTotal > 0 && !searchHasMore) || searchTotal === 0" to="footer">
    <Footer class="border-t border-slate-50 bg-slate-200 shadow" />
  </Teleport>
</template>
