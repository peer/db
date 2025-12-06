<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { ClientSearchState, SearchResult as SearchResultType, SearchViewType } from "@/types"
import type { PeerDBDocument } from "@/document.ts"

import { computed, toRef, ref, onMounted, onBeforeUnmount, nextTick, watch } from "vue"

import Footer from "@/partials/Footer.vue"
import SearchResultsHeader from "@/partials/SearchResultsHeader.vue"
import ClaimValue from "@/partials/ClaimValue.vue"
import WithDocument from "@/components/WithDocument.vue"
import Button from "@/components/Button.vue"
import { encodeQuery, getBestClaimOfType, getName, loadingWidth, useLimitResults } from "@/utils.ts"
import { activeSearchState, FILTERS_INCREASE, FILTERS_INITIAL_LIMIT, useFilters } from "@/search.ts"
import { injectProgress } from "@/progress.ts"
import { useVisibilityTracking } from "@/visibility.ts"
import { useRoute, useRouter } from "vue-router"

const props = defineProps<{
  s: string
  searchView: SearchViewType
  searchMoreThanTotal: boolean
  searchState: DeepReadonly<ClientSearchState | null>
  searchTotal: number | null
  searchResults: DeepReadonly<SearchResultType[]>
  searchProgress: number
}>()

const $emit = defineEmits<{
  "update:searchView": [value: SearchViewType]
}>()

const router = useRouter()
const route = useRoute()

const SEARCH_INITIAL_LIMIT = 50
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
const { results: filtersResults } = useFilters(
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
  await nextTick(() => {
    initResizeObserver()
  })

  window.addEventListener("scroll", onVerticalScroll, { passive: true })
  if (scrollContainer.value) {
    scrollContainer.value.addEventListener("scroll", onHorizontalScroll, { passive: true })
  }
})

onBeforeUnmount(() => {
  abortController.abort()

  window.removeEventListener("scroll", onVerticalScroll)
  if (scrollContainer.value) {
    scrollContainer.value.removeEventListener("scroll", onHorizontalScroll)
  }

  destroyResizeObserver()
})

const TABLE_RENDER_THRESHOLD = 50 // 50px

const WithPeerDBDocument = WithDocument<PeerDBDocument>
const initialRouteName = route.name

let resizeObserver: ResizeObserver | null = null
const abortController = new AbortController()

const tableWrapper = ref<HTMLDivElement | null>(null)
const scrollContainer = ref<HTMLDivElement | null>(null)
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

watch(
  () => {
    const sorted = Array.from(visibles)
    sorted.sort((a, b) => (idToIndex.value.get(a) ?? Infinity) - (idToIndex.value.get(b) ?? Infinity))
    return sorted[0]
  },
  async (topId) => {
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

function onVerticalScroll() {
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
    searchLoadMore()
  }
}

function onHorizontalScroll() {
  if (abortController.signal.aborted) {
    return
  }

  const el = scrollContainer.value
  if (!el) return

  const viewportWidth = el.clientWidth
  const scrollWidth = el.scrollWidth
  const currentScrollLeft = el.scrollLeft

  if (currentScrollLeft > scrollWidth - 2 * viewportWidth) {
    filtersLoadMore()
  }
}

function initResizeObserver(): void {
  if (!tableWrapper.value) return

  resizeObserver = new ResizeObserver((entries) => {
    for (const entry of entries) {
      const height = entry.contentRect.height
      const width = entry.contentRect.width

      handleTableHeightResize(height)
      handleTableWidthResize(width)
    }
  })

  resizeObserver.observe(tableWrapper.value)
}

function destroyResizeObserver(): void {
  if (resizeObserver && tableWrapper.value) {
    resizeObserver.unobserve(tableWrapper.value)
    resizeObserver.disconnect()
  }
}

function handleTableHeightResize(height: number): void {
  if (height < TABLE_RENDER_THRESHOLD) return

  const viewportHeight = window.innerHeight

  // If the table is shorter than the viewport, try to load more rows
  if (height < viewportHeight && searchHasMore.value && !abortController.signal.aborted) {
    searchLoadMore()
  }
}

function handleTableWidthResize(width: number): void {
  if (width < TABLE_RENDER_THRESHOLD) return

  const viewportWidth = window.innerWidth

  // If the table is narrower than the viewport, try to load more columns
  if (width < viewportWidth && searchHasMore.value && !abortController.signal.aborted) {
    filtersLoadMore()
  }
}

function goTo(id: string): void {
  router.push({
    name: "DocumentGet",
    params: { id },
    query: encodeQuery({ s: props.s }),
  })
}
</script>

<template>
  <div class="w-full h-full">
    <div ref="scrollContainer" class="w-full h-full flex flex-col gap-y-1 sm:gap-y-4 p-1 sm:p-4 rounded overflow-x-auto overflow-y-visible">
      <SearchResultsHeader
        v-model:search-view="searchViewValue"
        class="sticky left-0 w-full"
        :search-state="searchState"
        :search-total="searchTotal"
        :search-more-than-total="searchMoreThanTotal"
      />

      <div ref="tableWrapper" class="flex gap-x-1 sm:gap-x-4 w-fit">
        <div class="rounded shadow border w-fit">
          <table class="table-fixed text-sm min-w-max">
            <!-- Headers -->
            <thead class="bg-slate-300 sticky top-0 z-10">
              <tr>
                <th class="p-2 min-w-[50px] text-start">#</th>
                <th v-for="(result, index) in limitedFiltersResults" :key="index" class="p-2 min-w-[200px] text-left">
                  <div class="flex items-center gap-x-1">
                    <template v-if="result.type === 'rel' || result.type === 'amount' || result.type === 'time' || result.type === 'string'">
                      <WithPeerDBDocument :id="result.id" name="DocumentGet">
                        <template #default="{ doc, url }">
                          <RouterLink
                            :to="{ name: 'DocumentGet', params: { id: result.id } }"
                            :data-url="url"
                            class="link text-lg leading-none"
                            v-html="getName(doc.claims) || '<i>no name</i>'"
                          ></RouterLink>
                        </template>
                        <template #loading="{ url }">
                          <div class="inline-block h-2 animate-pulse rounded bg-slate-200" :data-url="url" :class="[loadingWidth(result.id)]"></div>
                        </template>
                      </WithPeerDBDocument>
                      ({{ result.count }})
                    </template>

                    <template v-else-if="result.type === 'index'">
                      <div class="flex items-baseline gap-x-1">
                        <span class="mb-1.5 text-lg leading-none">document index</span>
                        ({{ result.count }})
                      </div>
                    </template>

                    <template v-else-if="result.type === 'size'">
                      <div class="flex items-baseline gap-x-1">
                        <span class="mb-1.5 text-lg leading-none">document size</span>
                        ({{ result.count }})
                      </div>
                    </template>
                  </div>
                </th>
              </tr>
            </thead>

            <!-- Results -->
            <tbody v-if="searchTotal !== null && searchTotal > 0" class="divide-y">
              <tr
                v-for="(result, i) in limitedSearchResults"
                :key="result.id"
                :ref="track(result.id) as any"
                class="odd:bg-white even:bg-slate-100 hover:bg-slate-200 cursor-pointer"
                @click.prevent="goTo(result.id)"
              >
                <td class="p-2 min-w-[50px] text-start">
                  <RouterLink :to="{ name: 'DocumentGet', params: { id: result.id }, query: encodeQuery({ s }) }" class="link">{{ i + 1 }}</RouterLink>
                </td>
                <WithPeerDBDocument :id="result.id" name="DocumentGet">
                  <template #default="{ doc: searchDoc }">
                    <td v-for="(filter, index) in limitedFiltersResults" :key="index" class="p-2 min-w-[200px]">
                      <ClaimValue
                        v-if="filter.type === 'rel' || filter.type === 'amount' || filter.type === 'time' || filter.type === 'string'"
                        :type="filter.type"
                        :claim="getBestClaimOfType(searchDoc.claims, filter.type, filter.id)"
                      />
                    </td>
                  </template>
                </WithPeerDBDocument>
              </tr>
            </tbody>
          </table>
        </div>

        <Button v-if="filtersHasMore" ref="filtersMoreButton" :progress="filtersProgress" primary class="absolute top-0 w-fit h-fit min-w-fit" @click="filtersLoadMore"
          >More filters</Button
        >
      </div>

      <div class="sticky left-0 w-full text-center">
        <Button v-if="searchHasMore" ref="searchMoreButton" :progress="searchProgress" primary class="w-1/4 min-w-fit self-center" @click="searchLoadMore"
          >Load more</Button
        >
      </div>
    </div>
  </div>

  <Teleport to="footer">
    <Footer class="border-t border-slate-50 bg-slate-200 shadow" />
  </Teleport>
</template>
