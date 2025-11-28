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

import { computed, toRef, ref, onMounted, onBeforeUnmount } from "vue"

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

onMounted(async () => {
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
})

const abortController = new AbortController()

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

    <div ref="scrollContainer" class="w-full h-full rounded overflow-y-hidden overflow-x-scroll">
      <table class="table-fixed text-sm min-w-max shadow border">
        <!-- Headers -->
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
    </div>
  </div>

  <Teleport to="footer">
    <Footer class="border-t border-slate-50 bg-slate-200 shadow" />
  </Teleport>
</template>
