<script setup lang="ts">
import type { ComponentPublicInstance, DeepReadonly } from "vue"

import type { PeerDBDocument } from "@/document.ts"
import type { ClientSearchSession, FilterResult, Result, ViewType } from "@/types"

import { computed, onBeforeUnmount, onMounted, ref, toRef, useTemplateRef } from "vue"

import Button from "@/components/Button.vue"
import WithDocument from "@/components/WithDocument.vue"
import ClaimValue from "@/partials/ClaimValue.vue"
import DocumentRefInline from "@/partials/DocumentRefInline.vue"
import Footer from "@/partials/Footer.vue"
import SearchResultsHeader from "@/partials/SearchResultsHeader.vue"
import { encodeQuery, useLimitResults, useOnScrollOrResize, loadingWidth, getClaimsOfTypeWithConfidence, getBestClaimOfType } from "@/utils.ts"
import { FILTERS_INCREASE, FILTERS_INITIAL_LIMIT, useFilters, useLocationAt } from "@/search.ts"
import { injectProgress } from "@/progress.ts"
import { useVisibilityTracking } from "@/visibility.ts"

const props = defineProps<{
  // Search props.
  searchResults: DeepReadonly<Result[]>
  searchTotal: number | null
  searchMoreThanTotal: boolean
  searchSession: DeepReadonly<ClientSearchSession>
  searchProgress: number
}>()

const $emit = defineEmits<{
  viewChange: [value: ViewType]
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

const content = useTemplateRef<HTMLElement>("content")

const filtersProgress = injectProgress()
const {
  results: filtersResults,
  total: filtersTotal,
  error: filtersError,
  url: filtersURL,
} = useFilters(
  toRef(() => props.searchSession),
  // We use the content element because data about filters is needed to display columns for the whole table.
  // Using only <tr> element inside <thead> (where data-url attribute is set for filters) would not convey that requirement.
  content,
  filtersProgress,
)

const {
  limitedResults: limitedFiltersResults,
  hasMore: filtersHasMore,
  loadMore: filtersLoadMore,
} = useLimitResults(filtersResults, FILTERS_INITIAL_LIMIT, FILTERS_INCREASE)

function supportedFilter(filter: FilterResult) {
  return filter.type === "rel" || filter.type === "amount" || filter.type === "time" || filter.type === "string"
}

const rowColspan = computed(() => {
  if (filtersTotal.value === null) {
    return 1
  }
  let count = 0
  for (const filter of limitedFiltersResults.value) {
    if (supportedFilter(filter)) {
      count++
    }
  }
  return count
})

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

useOnScrollOrResize(content, onScrollOrResize)

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

const headerAttrs = ref<{ style: { top: string } }>({ style: { top: "-1px" } })

// TODO: Find a better way to get the header to stick to the bottom of the navbar.
function onScroll() {
  const el = document.getElementById("navbar")
  if (!el) {
    return
  }

  const { bottom } = el.getBoundingClientRect()
  // We use -1 because we have a 1px border on the table which we want to offset.Otherwise there
  // is a 1px gap between the top edge of the window and where the header gets stuck
  const top = Math.max(-1, bottom - 1)
  headerAttrs.value.style.top = `${top}px`
}

onMounted(() => {
  window.addEventListener("scroll", onScroll, { passive: true })
})

onBeforeUnmount(() => {
  window.removeEventListener("scroll", onScroll)
})

const WithPeerDBDocument = WithDocument<PeerDBDocument>
</script>

<template>
  <!--
    TODO: No idea why w-0 (and w-fit) work here, but w-full does not.
          One would assume that w-full is needed to make the container div as wide as the
          body inside which then the footer horizontally shifts.
  -->
  <div class="sticky left-0 z-20 w-0">
    <SearchResultsHeader
      class="w-container p-1 sm:p-4"
      :search-session="searchSession"
      :search-total="searchTotal"
      :search-more-than-total="searchMoreThanTotal"
      @view-change="(v) => $emit('viewChange', v)"
    />
  </div>

  <div v-if="filtersError" class="mb-1 px-1 text-center sm:mb-4 sm:px-4">
    <i class="text-error-600">loading data failed</i>
  </div>

  <template v-else-if="searchTotal !== null && searchTotal > 0">
    <div ref="content" class="flex flex-row gap-x-1 px-1 sm:gap-x-4 sm:px-4">
      <!-- TODO: Make table have rounded corners. -->
      <table class="border shadow">
        <!-- Headers -->
        <!--
          We use -top-px because we have a 1px border on the table which we want to offset. Otherwise there
          is a 1px gap between the top edge of the window and where the header gets stuck
        -->
        <thead class="sticky -top-px z-10 bg-slate-300" v-bind="headerAttrs">
          <tr :data-url="filtersURL">
            <th class="p-2 text-start">#</th>
            <th v-if="filtersTotal === null" class="p-2 text-start">
              <div class="inline-block h-2 animate-pulse rounded bg-slate-200" :class="[loadingWidth(`${searchSession.id}/0`)]" />
            </th>
            <template v-for="filter in limitedFiltersResults" v-else :key="filter.id">
              <th v-if="supportedFilter(filter)" class="max-w-[400px] truncate p-2 text-start">
                <DocumentRefInline :id="filter.id" class="text-lg leading-none" />
              </th>
            </template>
          </tr>
        </thead>

        <!-- Results -->
        <tbody class="divide-y">
          <template v-for="(result, index) in limitedSearchResults" :key="result.id">
            <WithPeerDBDocument :id="result.id" name="DocumentGet">
              <template #default="{ doc, url }">
                <tr :ref="track(result.id)" class="odd:bg-white even:bg-slate-100 hover:bg-slate-200" :data-url="url">
                  <td class="p-2 text-start">
                    <RouterLink :to="{ name: 'DocumentGet', params: { id: result.id }, query: encodeQuery({ s: searchSession.id }) }" class="link">{{
                      index + 1
                    }}</RouterLink>
                  </td>
                  <td v-if="filtersTotal === null" class="p-2 text-start">
                    <div class="inline-block h-2 animate-pulse rounded bg-slate-200" :class="[loadingWidth(`${searchSession.id}/${index + 1}`)]" />
                  </td>
                  <template v-for="filter in limitedFiltersResults" v-else :key="filter.id">
                    <td v-if="supportedFilter(filter)" class="max-w-[400px] text-start truncate p-2">
                      <template v-for="(claim, cIndex) in getClaimsOfTypeWithConfidence(doc.claims, filter.type, filter.id)" :key="claim.id">
                        <template v-if="cIndex !== 0">, </template>
                        <ClaimValue :type="filter.type" :claim="claim" />
                      </template>
                    </td>
                  </template>
                </tr>
              </template>
              <template #loading="{ url }">
                <!--
                  We do not track(result.id) <tr> here because in that case Vue would first track loading <tr>, then it would remove it and untrack it,
                  and then it would track the final <tr>. That makes "at" URL query parameter to first show the first ID (because loading <tr>s are visible),
                  then it loops through all IDs as their loading <tr>s are being removed and "new" top (loading) <tr>s are found, and then finally again "at"
                  URL query parameter is set to the first ID for final <tr>s, the same one which was the first ID for loading <tr>s. To prevent this "flicker"
                  of "at" URL query parameter we do not track loading and error <tr>s.
                -->
                <tr class="odd:bg-white even:bg-slate-100 hover:bg-slate-200" :data-url="url">
                  <td class="p-2 text-start">
                    <RouterLink :to="{ name: 'DocumentGet', params: { id: result.id }, query: encodeQuery({ s: searchSession.id }) }" class="link">{{
                      index + 1
                    }}</RouterLink>
                  </td>
                  <td :colspan="rowColspan" class="p-2 text-start">
                    <div class="inline-block h-2 animate-pulse rounded bg-slate-200" :class="[loadingWidth(result.id)]" />
                  </td>
                </tr>
              </template>
              <!-- We do not track(result.id) <tr> here. See explanation above. -->
              <template #error="{ url }">
                <tr class="odd:bg-white even:bg-slate-100 hover:bg-slate-200" :data-url="url">
                  <td class="p-2 text-start">
                    <RouterLink :to="{ name: 'DocumentGet', params: { id: result.id }, query: encodeQuery({ s: searchSession.id }) }" class="link">{{
                      index + 1
                    }}</RouterLink>
                  </td>
                  <td :colspan="rowColspan" class="p-2 text-start">
                    <i class="text-error-600">loading data failed</i>
                  </td>
                </tr>
              </template>
            </WithPeerDBDocument>
          </template>
        </tbody>
      </table>

      <div v-if="filtersHasMore" class="sticky top-[37.5%] z-20 h-full">
        <Button ref="filtersMoreButton" :progress="filtersProgress" primary class="h-1/4 min-h-fit !px-2.5 !py-6 [writing-mode:sideways-lr]" @click="filtersLoadMore"
          >More columns</Button
        >
      </div>
    </div>

    <!--
      TODO: No idea why w-0 (and w-fit) work here, but w-full does not.
            One would assume that w-full is needed to make the container div as wide as the
            body inside which then the footer horizontally shifts.
    -->
    <div class="sticky left-0 z-20 w-0">
      <div class="w-container flex justify-center p-1 sm:p-4">
        <Button v-if="searchHasMore" ref="searchMoreButton" :progress="searchProgress" primary class="w-1/4 min-w-fit" @click="searchLoadMore">Load more</Button>

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
