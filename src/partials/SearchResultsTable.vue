<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type {
  AmountSearchResult,
  ClientSearchState,
  RelSearchResult,
  SearchResult as SearchResultType,
  SearchViewType,
  StringSearchResult,
  TimeSearchResult,
} from "@/types"
import type { PeerDBDocument } from "@/document.ts"

import { computed, toRef, ref, onMounted } from "vue"

import WithDocument from "@/components/WithDocument.vue"
import Footer from "@/partials/Footer.vue"
import SearchResultsHeader from "@/partials/SearchResultsHeader.vue"
import { getName, loadingWidth, useLimitResults } from "@/utils.ts"
import { activeSearchState, FILTERS_INITIAL_LIMIT, FILTERS_TABLE_INCREASE, SEARCH_INITIAL_LIMIT, SEARCH_TABLE_INCREASE, useFilters } from "@/search.ts"
import { injectProgress } from "@/progress.ts"

const props = defineProps<{
  s: string
  searchView: SearchViewType
  searchMoreThanTotal: boolean
  searchState: DeepReadonly<ClientSearchState | null>
  searchTotal: number | null
  searchResults: DeepReadonly<SearchResultType[]>
}>()

const $emit = defineEmits<{
  "update:searchView": [value: SearchViewType]
}>()

const WithPeerDBDocument = WithDocument<PeerDBDocument>

const {
  limitedResults: limitedSearchResults,
  hasMore: searchHasMore,
  loadMore: searchLoadMore,
} = useLimitResults(
  toRef(() => props.searchResults),
  SEARCH_INITIAL_LIMIT,
  SEARCH_TABLE_INCREASE,
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
} = useLimitResults(filtersResults, FILTERS_INITIAL_LIMIT, FILTERS_TABLE_INCREASE)

const verticalSentinel = ref<HTMLElement | null>(null)
const horizontalSentinel = ref<HTMLElement | null>(null)
const scrollContainer = ref<HTMLElement | null>(null)

function isHorizontalVisible() {
  const root = scrollContainer.value
  const sentinel = horizontalSentinel.value
  if (!root || !sentinel) return false

  const sentinelRect = sentinel.getBoundingClientRect()
  const rootRect = root.getBoundingClientRect()
  return sentinelRect.left < rootRect.right
}

function ensureOverflow() {
  if (!filtersHasMore || !isHorizontalVisible()) return
  filtersLoadMore()
  requestAnimationFrame(ensureOverflow)
}

function observeHorizontal() {
  const root = scrollContainer.value
  if (!root || !horizontalSentinel.value) return

  const observer = new IntersectionObserver(
    (entries) => {
      if (entries[0].isIntersecting && filtersHasMore) filtersLoadMore()
    },
    {
      root,
      rootMargin: "0px 120px 0px 0px",
      threshold: 0,
    },
  )
  observer.observe(horizontalSentinel.value)
}

function observeVertical() {
  const root = scrollContainer.value
  if (!root || !verticalSentinel.value) return

  const observer = new IntersectionObserver(
    (entries) => {
      if (entries[0].isIntersecting) {
        if (searchHasMore) searchLoadMore()
        if (filtersHasMore) filtersLoadMore()
      }
    },
    {
      root,
      rootMargin: "0px 0px 300px 0px",
      threshold: 0,
    },
  )
  observer.observe(verticalSentinel.value)
}

onMounted(() => {
  ensureOverflow()
  observeHorizontal()
  observeVertical()
})

const searchViewValue = computed({
  get() {
    return props.searchView
  },
  set(value) {
    $emit("update:searchView", value)
  },
})

function getDocumentRelPropertyId(filterResult: RelSearchResult, searchDocument: DeepReadonly<PeerDBDocument>): string {
  const claims = searchDocument.claims?.[filterResult.type]
  if (!claims) return ""

  const match = claims.find((claim) => claim.prop.id === filterResult.id)
  return match?.to.id ?? ""
}

function getDocumentAmountPropertyValue(filterResult: AmountSearchResult, searchDocument: DeepReadonly<PeerDBDocument>): string {
  const claims = searchDocument.claims?.[filterResult.type]
  if (!claims) return ""

  const match = claims.find((claim) => claim.prop.id === filterResult.id)
  return match?.amount.toString() ?? ""
}

function getDocumentStringPropertyValue(filterResult: StringSearchResult, searchDocument: DeepReadonly<PeerDBDocument>): string {
  const claims = searchDocument.claims?.[filterResult.type]
  if (!claims) return ""

  const match = claims.find((claim) => claim.prop.id === filterResult.id)
  return match?.string ?? ""
}

function getDocumentTimePropertyValue(filterResult: TimeSearchResult, searchDocument: DeepReadonly<PeerDBDocument>): string {
  const claims = searchDocument.claims?.[filterResult.type]
  if (!claims) return ""

  const match = claims.find((claim) => claim.prop.id === filterResult.id)

  return match?.timestamp ?? ""
}
</script>

<template>
  <div class="w-full h-full flex flex-col gap-y-1 sm:gap-y-4">
    <SearchResultsHeader v-model:search-view="searchViewValue" :search-state="searchState" :search-total="searchTotal" :search-more-than-total="searchMoreThanTotal" />

    <!-- TODO: Calculate height with flex-col and h-full (change structure to the body, header, main , footer) -->
    <div class="shadow bg-white border rounded" style="height: calc(100vh - 215px)">
      <div ref="scrollContainer" class="overflow-x-auto overflow-y-auto h-full w-full">
        <table class="table-fixed text-sm min-w-max">
          <!-- Header filters -->
          <thead class="bg-slate-300 sticky top-0 z-10">
            <tr>
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

              <div ref="horizontalSentinel" />
            </tr>
          </thead>

          <!-- Results -->
          <tbody v-if="searchTotal !== null && searchTotal > 0" class="divide-y">
            <tr v-for="result in limitedSearchResults" :key="result.id" class="odd:bg-white even:bg-slate-100 hover:bg-slate-200 cursor-pointer">
              <WithPeerDBDocument :id="result.id" name="DocumentGet">
                <template #default="{ doc: searchDoc }">
                  <td v-for="(filter, index) in limitedFiltersResults" :key="index" class="p-2">
                    <!-- Document rel property -->
                    <WithPeerDBDocument
                      v-if="filter.type === 'rel' && getDocumentRelPropertyId(filter, searchDoc)"
                      :id="getDocumentRelPropertyId(filter, searchDoc)"
                      name="DocumentGet"
                    >
                      <template #default="{ doc: resultDoc }">
                        {{ getName(resultDoc.claims) }}
                      </template>
                      <template #loading="{ url }">
                        <div
                          class="inline-block h-2 animate-pulse rounded bg-slate-200"
                          :data-url="url"
                          :class="[loadingWidth(getDocumentRelPropertyId(filter, searchDoc))]"
                        ></div>
                      </template>
                    </WithPeerDBDocument>

                    <!-- Document amount property -->
                    <template v-else-if="filter.type === 'amount'">
                      {{ getDocumentAmountPropertyValue(filter, searchDoc) }}
                    </template>

                    <!-- Document time property -->
                    <template v-else-if="filter.type === 'time'">
                      {{ getDocumentTimePropertyValue(filter, searchDoc) }}
                    </template>

                    <!-- Document string property -->
                    <template v-else-if="filter.type === 'string'">
                      {{ getDocumentStringPropertyValue(filter, searchDoc) }}
                    </template>
                  </td>
                </template>
              </WithPeerDBDocument>
            </tr>
          </tbody>
        </table>

        <div ref="verticalSentinel" />
      </div>
    </div>
  </div>

  <Teleport to="footer">
    <Footer class="border-t border-slate-50 bg-slate-200 shadow" />
  </Teleport>
</template>
