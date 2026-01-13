<script setup lang="ts">
import type { ComponentPublicInstance, DeepReadonly } from "vue"

import type { PeerDBDocument } from "@/document"
import type { ClientSearchSession, FilterResult, FiltersState, FilterStateChange, Result, ViewType } from "@/types"

import { LocalScope } from "@all1ndev/vue-local-scope"
import { Dialog, DialogPanel } from "@headlessui/vue"
import { ArrowTopRightOnSquareIcon, ChevronUpDownIcon, FunnelIcon, XMarkIcon } from "@heroicons/vue/20/solid"
import { ChevronDownUpIcon } from "@sidekickicons/vue/20/solid"
import { computed, onBeforeUnmount, onMounted, ref, toRef, useTemplateRef } from "vue"
import { useI18n } from "vue-i18n"

import Button from "@/components/Button.vue"
import WithDocument from "@/components/WithDocument.vue"
import ClaimValue from "@/partials/ClaimValue.vue"
import FiltersResult from "@/partials/FiltersResult.vue"
import Footer from "@/partials/Footer.vue"
import SearchResultsHeader from "@/partials/SearchResultsHeader.vue"
import { injectProgress } from "@/progress"
import { FILTERS_INCREASE, FILTERS_INITIAL_LIMIT, useFilters, useLocationAt } from "@/search"
import { useTruncationTracking } from "@/truncation"
import { encodeQuery, getClaimsOfTypeWithConfidence, getName, loadingWidth, useLimitResults, useOnScrollOrResize } from "@/utils"
import { useVisibilityTracking } from "@/visibility"

const props = defineProps<{
  // Search props.
  searchResults: DeepReadonly<Result[]>
  searchTotal: number | null
  searchMoreThanTotal: boolean
  searchSession: DeepReadonly<ClientSearchSession>
  searchProgress: number
  updateSearchSessionProgress: number

  // Filter props.
  filtersState: FiltersState
}>()

const $emit = defineEmits<{
  filterChange: [change: FilterStateChange]
  viewChange: [value: ViewType]
}>()

const { t } = useI18n()

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
      ;(searchMoreButton.value.$el as HTMLButtonElement).click()
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
      ;(filtersMoreButton.value.$el as HTMLButtonElement).click()
    }
  }
}

const headerAttrs = ref<{ style: { top: string } }>({ style: { top: "-1px" } })

// TODO: Find a better way to get the header to stick to the bottom of the navbar.
function onScroll() {
  if (abortController.signal.aborted) {
    return
  }

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

const { track: trackTruncation, truncated } = useTruncationTracking()

const expandedRows = ref(new Map<string, Set<string>>())

function isCellTruncated(resultId: string, propertyId: string): boolean {
  return truncated.value.get(resultId)?.has(propertyId) ?? false
}

function isRowExpanded(resultId: string): boolean {
  return expandedRows.value.has(resultId)
}

function isCellExpanded(resultId: string, propertyId: string): boolean {
  return expandedRows.value.get(resultId)?.has(propertyId) ?? false
}

function canRowExpand(resultId: string) {
  return truncated.value.has(resultId)
}

function onToggleRow(resultId: string) {
  if (abortController.signal.aborted) {
    return
  }

  if (expandedRows.value.has(resultId)) {
    expandedRows.value.delete(resultId)
  } else {
    expandedRows.value.set(resultId, new Set<string>(truncated.value.get(resultId)))
  }
}

function getButtonTitle(resultId: string): string {
  return isRowExpanded(resultId) ? t("partials.SearchResultsTable.collapseRow") : t("partials.SearchResultsTable.expandRow")
}

const isFilterActive = (filter: FilterResult) => {
  const filterType = filter.type === "string" ? "str" : filter.type
  return !!props.filtersState?.[filterType]?.[filter.id]
}

const activeFilter = ref<FilterResult | null>(null)

function onOpenFilterModal(filter: FilterResult) {
  if (abortController.signal.aborted) {
    return
  }

  activeFilter.value = filter
}

function onCloseFilterModal() {
  if (abortController.signal.aborted) {
    return
  }

  activeFilter.value = null
}
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
    <i class="text-error-600">{{ t("common.status.loadingDataFailed") }}</i>
  </div>

  <template v-else-if="searchTotal !== null && searchTotal > 0">
    <div ref="content" class="flex flex-row gap-x-1 px-1 sm:gap-x-4 sm:px-4">
      <!-- TODO: Make table have rounded corners. -->
      <table class="border border-gray-200 shadow-sm">
        <!-- Headers -->
        <!--
          We use -top-px because we have a 1px border on the table which we want to offset. Otherwise there
          is a 1px gap between the top edge of the window and where the header gets stuck
        -->
        <thead class="sticky -top-px z-10 bg-slate-300" v-bind="headerAttrs">
          <tr :data-url="filtersURL">
            <th class="p-2 text-start">#</th>
            <th v-if="filtersTotal === null" class="p-2 text-start">
              <div class="inline-block h-2 animate-pulse rounded-sm bg-slate-200" :class="[loadingWidth(`${searchSession.id}/0`)]" />
            </th>
            <template v-for="filter in limitedFiltersResults" v-else :key="`${filter.type}/${filter.id}`">
              <th v-if="supportedFilter(filter)" class="text-start">
                <!-- <div class="flex flex-row items-center justify-between"> -->
                <WithPeerDBDocument :id="filter.id" name="DocumentGet">
                  <template #default="{ doc, url }">
                    <Button
                      :data-url="url"
                      class="flex w-full max-w-[400px] flex-row items-center justify-between gap-x-1 border-none! p-2! leading-none! shadow-none!"
                      @click.prevent="onOpenFilterModal(filter)"
                    >
                      <!-- We need a span to be able to use v-html. -->
                      <span class="truncate" v-html="getName(doc.claims) || `<i>${t('common.values.noName')}</i>`" />
                      <FunnelIcon class="size-5" :class="isFilterActive(filter) ? '' : 'text-primary-300'" />
                    </Button>
                  </template>
                  <template #loading="{ url }">
                    <div class="inline-block h-2 animate-pulse rounded-sm bg-slate-200" :data-url="url" :class="[loadingWidth(filter.id)]" />
                  </template>
                </WithPeerDBDocument>
              </th>
            </template>
          </tr>
        </thead>

        <!-- Results -->
        <tbody class="divide-y divide-gray-200">
          <template v-for="(result, index) in limitedSearchResults" :key="result.id">
            <WithPeerDBDocument :id="result.id" name="DocumentGet">
              <template #default="{ doc, url }">
                <tr :id="`result-${result.id}`" :ref="track(result.id)" class="odd:bg-white even:bg-slate-100 hover:bg-slate-200" :data-url="url">
                  <td class="flex items-center justify-between gap-1 p-2">
                    <RouterLink :to="{ name: 'DocumentGet', params: { id: result.id }, query: encodeQuery({ s: searchSession.id }) }" class="link">{{
                      index + 1
                    }}</RouterLink>

                    <Button
                      v-if="canRowExpand(result.id) || isRowExpanded(result.id)"
                      :title="getButtonTitle(result.id)"
                      class="border-none! p-0! shadow-none!"
                      @click.prevent="onToggleRow(result.id)"
                    >
                      <ChevronDownUpIcon v-if="isRowExpanded(result.id)" class="size-5" aria-expanded="true" :aria-controls="`result-${result.id}`" />
                      <ChevronUpDownIcon v-else class="size-5" aria-expanded="false" :aria-controls="`result-${result.id}`" />
                    </Button>
                  </td>
                  <td v-if="filtersTotal === null" class="p-2">
                    <div class="inline-block h-2 animate-pulse rounded-sm bg-slate-200" :class="[loadingWidth(`${searchSession.id}/${index + 1}`)]" />
                  </td>
                  <template v-for="filter in limitedFiltersResults" v-else :key="`${filter.type}/${filter.id}`">
                    <td v-if="supportedFilter(filter)" class="align-top">
                      <LocalScope
                        v-slot="{ rowExpanded, cellTruncated, cellExpanded }"
                        :row-expanded="isRowExpanded(result.id)"
                        :cell-truncated="isCellTruncated(result.id, `${filter.type}/${filter.id}`)"
                        :cell-expanded="isCellExpanded(result.id, `${filter.type}/${filter.id}`)"
                      >
                        <!--
                          We have div wrapper so that we can control the height of the row. td elements cannot have height set.
                          We set min-height to line height + padding.
                        -->
                        <div
                          :ref="trackTruncation(result.id, `${filter.type}/${filter.id}`)"
                          class="min-h-[calc(1lh+var(--spacing)*2)] max-w-[400px] overscroll-contain p-2"
                          :class="[rowExpanded ? 'max-h-[300px] overflow-auto' : 'max-h-[calc(1lh+var(--spacing)*2)] truncate overflow-clip']"
                        >
                          <div v-if="(cellTruncated && rowExpanded) || cellExpanded || cellTruncated" class="float-right mt-[calc((1lh-var(--spacing)*5)/2)] flex gap-1">
                            <RouterLink
                              v-if="cellTruncated && rowExpanded"
                              :to="{ name: 'DocumentGet', params: { id: result.id }, query: encodeQuery({ s: searchSession.id }) }"
                              class="link"
                            >
                              <ArrowTopRightOnSquareIcon class="size-5" />
                            </RouterLink>

                            <Button
                              v-if="cellExpanded || cellTruncated"
                              :title="getButtonTitle(result.id)"
                              class="border-none! p-0! shadow-none!"
                              @click.prevent="onToggleRow(result.id)"
                            >
                              <ChevronDownUpIcon v-if="rowExpanded" class="size-5" aria-expanded="true" :aria-controls="`result-${result.id}`" />
                              <ChevronUpDownIcon v-else class="size-5" aria-expanded="false" :aria-controls="`result-${result.id}`" />
                            </Button>
                          </div>

                          <template v-for="(claim, cIndex) in getClaimsOfTypeWithConfidence(doc.claims, filter.type, filter.id)" :key="claim.id">
                            <template v-if="cIndex !== 0">, </template>
                            <ClaimValue :type="filter.type" :claim="claim" />
                          </template>
                        </div>
                      </LocalScope>
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
                  <td class="p-2">
                    <RouterLink :to="{ name: 'DocumentGet', params: { id: result.id }, query: encodeQuery({ s: searchSession.id }) }" class="link">{{
                      index + 1
                    }}</RouterLink>
                  </td>
                  <td :colspan="rowColspan" class="p-2">
                    <div class="inline-block h-2 animate-pulse rounded-sm bg-slate-200" :class="[loadingWidth(result.id)]" />
                  </td>
                </tr>
              </template>
              <!-- We do not track(result.id) <tr> here. See explanation above. -->
              <template #error="{ url }">
                <tr class="odd:bg-white even:bg-slate-100 hover:bg-slate-200" :data-url="url">
                  <td class="p-2">
                    <RouterLink :to="{ name: 'DocumentGet', params: { id: result.id }, query: encodeQuery({ s: searchSession.id }) }" class="link">{{
                      index + 1
                    }}</RouterLink>
                  </td>
                  <td :colspan="rowColspan" class="p-2">
                    <i class="text-error-600">{{ t("common.status.loadingDataFailed") }}</i>
                  </td>
                </tr>
              </template>
            </WithPeerDBDocument>
          </template>
        </tbody>
      </table>

      <div v-if="filtersHasMore" class="sticky top-[37.5%] z-20 h-full">
        <Button ref="filtersMoreButton" :progress="filtersProgress" primary class="h-1/4 min-h-fit [writing-mode:sideways-lr]" @click.prevent="filtersLoadMore">{{
          t("partials.SearchResultsTable.moreColumns")
        }}</Button>
      </div>
    </div>

    <!--
      TODO: No idea why w-0 (and w-fit) work here, but w-full does not.
            One would assume that w-full is needed to make the container div as wide as the
            body inside which then the footer horizontally shifts.
    -->
    <div class="sticky left-0 z-20 w-0">
      <div class="w-container flex justify-center p-1 sm:p-4">
        <Button v-if="searchHasMore" ref="searchMoreButton" :progress="searchProgress" primary class="w-1/4 min-w-fit" @click.prevent="searchLoadMore">{{
          t("common.buttons.loadMore")
        }}</Button>

        <div v-else class="my-1 sm:my-4">
          <!-- Here we assume that MaxResultsCount is always set to a smaller value than what TrackTotalHits is set to. -->
          <div v-if="searchMoreThanTotal" class="text-center text-sm">{{
            t("common.status.allResultsMoreThan", { first: searchResults.length, count: searchTotal })
          }}</div>
          <div v-else-if="searchResults.length < searchTotal" class="text-center text-sm">{{
            t("common.status.allResultsOnly", { first: searchResults.length, count: searchTotal })
          }}</div>
          <div v-else-if="searchResults.length === searchTotal" class="text-center text-sm">{{ t("common.status.allResults", { count: searchResults.length }) }}</div>
        </div>
      </div>
    </div>
  </template>

  <Teleport v-if="(searchTotal !== null && searchTotal > 0 && !searchHasMore) || searchTotal === 0" to="footer">
    <Footer class="border-t border-slate-50 bg-slate-200 shadow-sm" />
  </Teleport>

  <!--
    We make the dialog z-50 (and to be able to do so, we have to make it relative) to make it higher than the navbar and other floating elements.
  -->
  <Dialog as="div" class="relative z-50" :open="activeFilter !== null && searchTotal !== null" @close="onCloseFilterModal">
    <!-- Backdrop. -->
    <div class="fixed inset-0 bg-black/30" aria-hidden="true" />

    <!-- Full-screen container to center the panel. -->
    <div class="fixed inset-0 flex items-center justify-center">
      <DialogPanel
        class="flex h-full w-full flex-col overflow-y-auto rounded-none bg-white p-1 shadow-none sm:relative sm:inset-auto sm:h-auto sm:max-h-[600px] sm:max-w-xl sm:rounded-sm sm:p-4 sm:shadow-sm"
      >
        <FiltersResult
          :result="activeFilter!"
          :search-session="searchSession"
          :search-total="searchTotal!"
          :update-search-session-progress="updateSearchSessionProgress"
          :filters-state="filtersState"
          @filter-change="(c) => $emit('filterChange', c)"
        />

        <Button class="absolute! top-1 right-1 border-none! p-0! shadow-none! sm:top-4 sm:right-4" title="Close" @click="onCloseFilterModal">
          <XMarkIcon class="size-5" />
        </Button>
      </DialogPanel>
    </div>
  </Dialog>
</template>
