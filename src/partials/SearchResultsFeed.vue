<script setup lang="ts">
import type { ComponentPublicInstance, DeepReadonly } from "vue"

import type { D } from "@/document"
import type { Filter, Result, SearchSession, SortKey, ViewType } from "@/types"

import { FunnelIcon, XMarkIcon } from "@heroicons/vue/20/solid"
import { computed, onBeforeUnmount, onMounted, provide, ref, toRaw, toRef, useTemplateRef, watch } from "vue"
import { useI18n } from "vue-i18n"

import Button from "@/components/Button.vue"
import WithDocument from "@/components/WithDocument.vue"
import DisplayLabel from "@/partials/DisplayLabel.vue"
import FiltersResult from "@/partials/FiltersResult.vue"
import Footer from "@/partials/Footer.vue"
import PrefilterLabel from "@/partials/PrefilterLabel.vue"
import SearchPrintFilters from "@/partials/SearchPrintFilters.vue"
import SearchResult from "@/partials/SearchResult.vue"
import SearchResultGroup from "@/partials/SearchResultGroup.vue"
import SearchResultsEndBar from "@/partials/SearchResultsEndBar.vue"
import SearchResultsHeader from "@/partials/SearchResultsHeader.vue"
import SearchResultsPager from "@/partials/SearchResultsPager.vue"
import SearchSortDialog from "@/partials/SearchSortDialog.vue"
import TimeDisplay from "@/partials/TimeDisplay.vue"
import { useBusy } from "@/progress"
import { FILTERS_INCREASE, FILTERS_INITIAL_LIMIT, filterResultKey, useFilters, useLocationAt } from "@/search"
import { limitGroupedResults, loadingWidth, searchPagerKey, SKIP_TO_END, useLimitResults, useOnScrollOrResize } from "@/utils"
import { searchTrackKey, useVisibilityTracking } from "@/visibility"

const props = defineProps<{
  // Search props.
  searchResults: DeepReadonly<Result[]>
  searchTotal: number | null
  searchMoreThanTotal: boolean
  searchSession: DeepReadonly<SearchSession>
  isDownloading: boolean

  // Filter props.
  filters: Filter[]
}>()

const $emit = defineEmits<{
  filterUpdate: [filterId: string, filter: Filter]
  viewChange: [value: ViewType]
  downloadZip: []
  downloadFiles: []
  reverseClear: []
  prefiltersClear: []
  sortUpdate: [sort: SortKey[]]
}>()

const { t } = useI18n({ useScope: "global" })

const sortDialogOpen = ref(false)

// Results are grouped when the session's sort has a leading run of group columns; the backend then returns
// nested results which we render as a group tree instead of a flat list.
const grouped = computed(() => (props.searchSession.sort ?? []).some((s) => s.group))

// In the grouped view the actual results are leaf nodes nested under group headings, and a document placed
// under several groups appears several times. groupedTotals walks the whole tree once to count each result
// only on its first appearance (so the shown total matches the server's distinct-document total) and to mark
// every later occurrence as a duplicate (rendered as a back-reference instead of in full). It is independent
// of the load-more limit; the per-pager positions are computed separately over the rendered subset below.
const groupedTotals = computed(() => {
  const seen = new Set<string>()
  const duplicates = new Set<object>()
  const walk = (nodes: DeepReadonly<Result[]>): void => {
    for (const node of nodes) {
      if (node.group) {
        walk(node.group)
      } else if (!seen.has(node.id)) {
        seen.add(node.id)
      } else {
        duplicates.add(toRaw(node))
      }
    }
  }
  walk(props.searchResults)
  return { shown: seen.size, duplicates }
})

// Print view: an in-app preview of how the results print. It shares its layout with actual printing
// (@media print): the filters move above the results as a list, a live timestamp shows top-right, and
// interactive chrome is hidden. A body class drives the preview; @media print drives real printing.
const printMode = ref(false)

watch(printMode, (on) => {
  document.body.classList.toggle("pd-printing", on)
})

// nowTimestamp is a local-time string in the claim Time format, ticked every second so the print
// timestamp (and an actual print) always shows the current time.
function nowTimestamp(): string {
  const d = new Date()
  const pad = (n: number, width = 2): string => String(n).padStart(width, "0")
  // The claim Time format separates the date and time with a space (see timeRegex in document/time.ts).
  return `${pad(d.getFullYear(), 4)}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}
const now = ref(nowTimestamp())
let clockTimer: ReturnType<typeof setInterval> | null = null

// The active filters listed in the print layout: prefilters first, then regular filters.
const printFilters = computed(() => [...(props.searchSession.prefilters ?? []), ...(props.searchSession.filters ?? [])])
onMounted(() => {
  clockTimer = setInterval(() => {
    now.value = nowTimestamp()
  }, 1000)
})

const SEARCH_INITIAL_LIMIT = 10
const SEARCH_INCREASE = 10

const {
  limitedResults: limitedSearchResults,
  hasMore: searchHasMore,
  loadMore: searchLoadMore,
  loadAll: searchLoadAll,
} = useLimitResults(
  toRef(() => props.searchResults),
  SEARCH_INITIAL_LIMIT,
  SEARCH_INCREASE,
)

// The grouped view reveals results incrementally like the flat list above, but the tree is limited by unique
// result (matching the pagers and all-shown bar) rather than by array slice. The backend returns the whole
// tree at once, so this is purely client-side. groupLimit resets whenever a new result set arrives.
const groupLimit = ref(SEARCH_INITIAL_LIMIT)
watch(
  () => props.searchResults,
  () => {
    groupLimit.value = SEARCH_INITIAL_LIMIT
  },
)
const limitedGroupedResults = computed(() => {
  const total = groupedTotals.value.shown
  let limit = Math.min(groupLimit.value, total)
  // If the last increase would reveal SKIP_TO_END or fewer remaining results, just show all of them.
  if (limit + SKIP_TO_END >= total) {
    limit = total
  }
  return limitGroupedResults(props.searchResults, limit)
})
const groupedHasMore = computed(() => limitedGroupedResults.value.shown < groupedTotals.value.shown)
function groupedLoadMore(): void {
  groupLimit.value += SEARCH_INCREASE
}
function groupedLoadAll(): void {
  groupLimit.value = groupedTotals.value.shown
}

// groupedPager records, over the results actually rendered (the limited tree, so node identities match what
// is on screen), where each progress pager goes: pagerBefore maps the node a pager precedes to the count of
// unique results before it. When a pager lands at the start of a new group it is keyed to that group node, so
// it renders above the group's heading rather than below it; otherwise it is keyed to the leaf. shown, total,
// and duplicates come from the whole-tree groupedTotals so the bars size against the full result set. These
// are provided to the SearchResultGroup tree so a nested pager can render without drilling state through it.
const groupedPager = computed(() => {
  const seen = new Set<string>()
  const pagerBefore = new Map<object, number>()
  // Groups entered since the last leaf, outermost first; the next leaf seen is their shared first leaf.
  let pending: DeepReadonly<Result>[] = []
  const walk = (nodes: DeepReadonly<Result[]>): void => {
    for (const node of nodes) {
      if (node.group) {
        pending.push(node)
        walk(node.group)
      } else {
        if (!seen.has(node.id)) {
          const uniqueBefore = seen.size
          seen.add(node.id)
          if (uniqueBefore > 0 && uniqueBefore % 10 === 0) {
            pagerBefore.set(toRaw(pending.length > 0 ? pending[0] : node), uniqueBefore)
          }
        }
        pending = []
      }
    }
  }
  walk(limitedGroupedResults.value.results)
  return { pagerBefore, shown: groupedTotals.value.shown, total: props.searchTotal ?? 0, duplicates: groupedTotals.value.duplicates }
})
provide(searchPagerKey, groupedPager)

// topPagerIndex returns the unique-result count for a pager that precedes a top-level group (one that begins
// at a 10-result boundary), or undefined when none does. Nested pagers are placed by SearchResultGroup.
function topPagerIndex(node: DeepReadonly<Result>): number | undefined {
  return groupedPager.value.pagerBefore.get(toRaw(node))
}

// hasMore reports whether either view still has results to reveal. It gates the footer, which should appear
// only once the user has reached the end of the results, the same in the grouped view as in the flat one.
const hasMore = computed(() => (grouped.value ? groupedHasMore.value : searchHasMore.value))

// loadAll reveals every remaining loaded result at once (up to the server cap), used by the print view's
// "Load all" button so a printout is not limited to the incrementally revealed subset.
function loadAll(): void {
  if (grouped.value) {
    groupedLoadAll()
  } else {
    searchLoadAll()
  }
}

const filtersEl = useTemplateRef<HTMLElement>("filtersEl")
const filtersEnabled = ref(false)

// Data loading and controls for data loading.
const busy = useBusy()
const {
  results: filtersResults,
  total: filtersTotal,
  error: filtersError,
  url: filtersURL,
} = useFilters(
  toRef(() => props.searchSession),
  filtersEl,
  busy,
)

const {
  limitedResults: limitedFiltersResults,
  hasMore: filtersHasMore,
  loadMore: filtersLoadMore,
} = useLimitResults(filtersResults, FILTERS_INITIAL_LIMIT, FILTERS_INCREASE)

const { track, visibles } = useVisibilityTracking()
// The grouped result tree renders leaf results deep in SearchResultGroup, so the tracker is provided for
// those leaves to register the same way the flat results do here.
provide(searchTrackKey, track)

const abortController = new AbortController()

onBeforeUnmount(() => {
  abortController.abort()
  if (clockTimer !== null) {
    clearInterval(clockTimer)
  }
  // Make sure the print layout never lingers after leaving the feed.
  document.body.classList.remove("pd-printing")
})

const searchMoreButton = useTemplateRef<ComponentPublicInstance>("searchMoreButton")
const filtersMoreButton = useTemplateRef<ComponentPublicInstance>("filtersMoreButton")
const supportPageOffset = window.pageYOffset !== undefined

// orderedResults is the results in display order, used to pick the topmost visible result for the "at" query
// parameter: the flat list as-is, or the grouped tree's leaf results flattened into traversal order (each
// result once). Without this, grouped leaf ids would not map to a position and "at" could not follow scroll.
const orderedResults = computed<DeepReadonly<Result[]>>(() => {
  if (!grouped.value) {
    return props.searchResults
  }
  const out: DeepReadonly<Result>[] = []
  const seen = new Set<string>()
  const walk = (nodes: DeepReadonly<Result[]>): void => {
    for (const node of nodes) {
      if (node.group) {
        walk(node.group)
      } else if (!seen.has(node.id)) {
        seen.add(node.id)
        out.push(node)
      }
    }
  }
  walk(props.searchResults)
  return out
})

useLocationAt(
  orderedResults,
  toRef(() => props.searchTotal),
  visibles,
)

const content = useTemplateRef<HTMLElement>("content")

useOnScrollOrResize(content, onScrollOrResize)

function onScrollOrResize() {
  if (abortController.signal.aborted) {
    return
  }

  if (searchMoreButton.value || filtersMoreButton.value) {
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
      if (searchMoreButton.value) {
        ;(searchMoreButton.value.$el as HTMLButtonElement).click()
      }
      if (filtersMoreButton.value) {
        ;(filtersMoreButton.value.$el as HTMLButtonElement).click()
      }
    }
  }
}

function onFilters() {
  if (abortController.signal.aborted) {
    return
  }

  filtersEnabled.value = !filtersEnabled.value
}

function onSkipTo(targetId: string) {
  document.getElementById(targetId)?.focus()
}

const WithDocumentD = WithDocument<D>
</script>

<template>
  <Teleport to="#navbarsearch-teleport-end">
    <Button primary class="px-3.5 sm:hidden" type="button" @click.prevent="onFilters">
      <FunnelIcon class="size-5" :alt="t('common.buttons.filters')" />
    </Button>
  </Teleport>

  <div ref="content" class="pd-searchresultsfeed relative flex w-full gap-x-1 p-1 sm:gap-x-4 sm:p-4">
    <a
      href="#search-filters"
      class="sr-only focus:not-sr-only focus:absolute focus:top-1 focus:left-1 focus:z-50 focus:rounded-sm focus:bg-primary-600 focus:px-4 focus:py-2 focus:font-medium focus:text-white focus:shadow-lg focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 focus:outline-none sm:focus:top-4 sm:focus:left-4"
      @click.prevent="onSkipTo('search-filters')"
      >{{ t("partials.SearchResultsFeed.skipToFilters") }}</a
    >
    <a
      href="#search-results"
      class="sr-only focus:not-sr-only focus:absolute focus:top-1 focus:left-1 focus:z-50 focus:rounded-sm focus:bg-primary-600 focus:px-4 focus:py-2 focus:font-medium focus:text-white focus:shadow-lg focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 focus:outline-none sm:focus:top-4 sm:focus:left-4"
      @click.prevent="onSkipTo('search-results')"
      >{{ t("partials.SearchResultsFeed.skipToResults") }}</a
    >
    <!-- Search results column -->
    <div
      id="search-results"
      tabindex="-1"
      class="flex-auto basis-3/4 flex-col gap-y-1 rounded-sm [--pd-indent:calc(var(--spacing)*4)] focus-visible:ring-2 focus-visible:ring-primary-500 focus-visible:ring-offset-1 focus-visible:outline-none sm:flex sm:gap-y-4 sm:[--pd-indent:calc(var(--spacing)*6)]"
      :class="filtersEnabled ? 'hidden' : 'flex'"
    >
      <!-- Print row: the close and load-all buttons (preview only, left) and a live timestamp (right). -->
      <div class="pd-print-only-flex mb-2 items-center gap-x-2">
        <button
          type="button"
          class="pd-preview-only items-center gap-x-1 rounded-sm bg-slate-700 px-3 py-2 text-sm text-white shadow-lg outline-none hover:bg-slate-800 focus:ring-2 focus:ring-primary-500 focus:ring-offset-1"
          @click.prevent="printMode = false"
        >
          <XMarkIcon class="size-5" :alt="t('partials.SearchResultsFeed.closePrint')" />
          {{ t("partials.SearchResultsFeed.closePrint") }}
        </button>
        <button
          v-if="hasMore"
          type="button"
          class="pd-preview-only items-center gap-x-1 rounded-sm bg-primary-600 px-3 py-2 text-sm text-white shadow-lg outline-none hover:bg-primary-700 focus:ring-2 focus:ring-primary-500 focus:ring-offset-1"
          @click.prevent="loadAll"
        >
          {{ t("partials.SearchResultsFeed.loadAll") }}
        </button>
        <div class="ml-auto text-sm text-slate-600"><TimeDisplay :timestamp="now" precision="s" :toggle="false" /></div>
      </div>

      <SearchResultsHeader
        :search-session="searchSession"
        :search-total="searchTotal"
        :search-more-than-total="searchMoreThanTotal"
        :is-downloading="isDownloading"
        sortable
        printable
        @view-change="(v) => $emit('viewChange', v)"
        @download-zip="$emit('downloadZip')"
        @download-files="$emit('downloadFiles')"
        @sort-open="sortDialogOpen = true"
        @print-open="printMode = true"
      />

      <!-- Print-only: the active filters (prefilters first) listed under the status line, which acts as their heading. -->
      <SearchPrintFilters :filters="printFilters" class="pd-print-only" />

      <!-- Print-only: the reverse scope (documents referencing a target), shown above the filters list. -->
      <div v-if="searchSession.reverse" class="pd-print-only">
        <i18n-t keypath="partials.SearchResultsFeed.resultsReferencing" scope="global">
          <template #label>
            <WithDocumentD :id="searchSession.reverse" name="DocumentGet">
              <template #default="{ doc }">
                <DisplayLabel :doc="doc" />
              </template>
              <template #loading>
                <span
                  class="pd-withdocument-loading inline-block h-2 rounded-sm bg-slate-200 motion-safe:animate-pulse"
                  :class="[loadingWidth(searchSession.reverse!)]"
                />
              </template>
            </WithDocumentD>
          </template>
        </i18n-t>
      </div>

      <template v-if="searchTotal !== null && searchTotal > 0">
        <template v-if="grouped">
          <template v-for="(node, gi) in limitedGroupedResults.results" :key="`${node.id}-${gi}`">
            <SearchResultsPager v-if="topPagerIndex(node) !== undefined" :i="topPagerIndex(node)!" :shown="groupedPager.shown" :total="groupedPager.total" :depth="0" />
            <SearchResultGroup :node="node" :search-session-id="searchSession.id" :depth="0" />
          </template>

          <Button
            v-if="groupedHasMore"
            id="searchresultsfeed-button-loadmore"
            ref="searchMoreButton"
            primary
            class="pd-print-hidden w-1/4 min-w-fit self-center"
            @click.prevent="groupedLoadMore"
            >{{ t("common.buttons.loadMore") }}</Button
          >

          <!-- Print: instead of a load-more button, note how many results are not shown. -->
          <div v-if="searchTotal - limitedGroupedResults.shown > 0" class="pd-print-only my-1 text-center text-sm sm:my-4">
            {{ t("partials.SearchResultsFeed.resultsNotShown", { count: searchTotal - limitedGroupedResults.shown, total: searchTotal }) }}
          </div>

          <!--
            The end bar shows once nothing more can be loaded, including when the result cap (MaxResultsCount, assumed
            below TrackTotalHits) hides some matches.
          -->
          <SearchResultsEndBar v-if="!groupedHasMore" :first="groupedPager.shown" :total="searchTotal" :more-than-total="searchMoreThanTotal" />
        </template>
        <template v-else>
          <template v-for="(result, i) in limitedSearchResults" :key="result.id">
            <SearchResultsPager v-if="i > 0 && i % 10 === 0" :i="i" :shown="searchResults.length" :total="searchTotal" />
            <SearchResult :ref="track(result.id)" :search-session-id="searchSession.id" :result="result" />
          </template>

          <Button
            v-if="searchHasMore"
            id="searchresultsfeed-button-loadmore"
            ref="searchMoreButton"
            primary
            class="pd-print-hidden w-1/4 min-w-fit self-center"
            @click.prevent="searchLoadMore"
            >{{ t("common.buttons.loadMore") }}</Button
          >

          <!-- Print: instead of pager bars or a load-more button, note how many results are not shown. -->
          <div v-if="searchTotal - limitedSearchResults.length > 0" class="pd-print-only my-1 text-center text-sm sm:my-4">
            {{ t("partials.SearchResultsFeed.resultsNotShown", { count: searchTotal - limitedSearchResults.length, total: searchTotal }) }}
          </div>

          <!--
            The end bar shows once nothing more can be loaded, including when the result cap (MaxResultsCount, assumed
            below TrackTotalHits) hides some matches.
          -->
          <SearchResultsEndBar v-if="!searchHasMore" :first="searchResults.length" :total="searchTotal" :more-than-total="searchMoreThanTotal" />
        </template>
      </template>
    </div>

    <!-- Filters column -->
    <div
      id="search-filters"
      ref="filtersEl"
      tabindex="-1"
      class="pd-print-hidden flex-auto basis-1/4 flex-col gap-y-1 rounded-sm focus-visible:ring-2 focus-visible:ring-primary-500 focus-visible:ring-offset-1 focus-visible:outline-none sm:flex sm:gap-y-4"
      :class="filtersEnabled ? 'flex' : 'hidden'"
      :data-url="filtersURL"
    >
      <div v-if="filtersError" class="pd-searchresultsfeed-filters-error-wrapper my-1 sm:my-4">
        <div class="text-center text-sm"
          ><i class="pd-searchresultsfeed-filters-error text-error-600">{{ t("common.status.loadingDataFailed") }}</i></div
        >
      </div>

      <div v-else-if="searchTotal === null || filtersTotal === null" class="my-1 sm:my-4">
        <div class="text-center text-sm">{{ t("partials.SearchResultsFeed.determiningFilters") }}</div>
      </div>

      <template v-else>
        <div v-if="searchSession.reverse" class="text-center text-sm">
          <Button
            type="button"
            class="float-right ml-2 px-2.5 py-1"
            :title="t('partials.SearchResultsFeed.clearReferencing')"
            :aria-label="t('partials.SearchResultsFeed.clearReferencing')"
            @click.prevent="$emit('reverseClear')"
            >{{ t("common.buttons.clear") }}</Button
          >
          <i18n-t keypath="partials.SearchResultsFeed.resultsReferencing" scope="global">
            <template #label>
              <RouterLink :to="{ name: 'DocumentGet', params: { id: searchSession.reverse } }" class="link">
                <WithDocumentD :id="searchSession.reverse" name="DocumentGet">
                  <template #default="{ doc }">
                    <DisplayLabel :doc="doc" />
                  </template>
                  <template #loading>
                    <span
                      class="pd-withdocument-loading inline-block h-2 rounded-sm bg-slate-200 motion-safe:animate-pulse"
                      :class="[loadingWidth(searchSession.reverse!)]"
                    />
                  </template>
                </WithDocumentD>
              </RouterLink>
            </template>
          </i18n-t>
        </div>

        <div v-if="searchSession.prefilters && searchSession.prefilters.length > 0" class="text-center text-sm">
          <Button
            type="button"
            class="float-right ml-2 px-2.5 py-1"
            :title="t('partials.SearchResultsFeed.clearLimited')"
            :aria-label="t('partials.SearchResultsFeed.clearLimited')"
            @click.prevent="$emit('prefiltersClear')"
            >{{ t("common.buttons.clear") }}</Button
          >
          <i18n-t v-if="searchSession.prefilters.length === 1" keypath="partials.SearchResultsFeed.resultsLimitedTo" scope="global">
            <template #filter>
              <PrefilterLabel :filter="searchSession.prefilters[0]" />
            </template>
          </i18n-t>
          <template v-else>
            {{ t("partials.SearchResultsFeed.resultsLimitedToMany") }}
            <template v-for="prefilter in searchSession.prefilters" :key="prefilter.id">
              <br />
              <PrefilterLabel :filter="prefilter" />
            </template>
          </template>
        </div>

        <div v-if="filtersTotal === 0" class="my-1 sm:my-4">
          <div class="text-center text-sm">{{ t("partials.SearchResultsFeed.noFilters") }}</div>
        </div>

        <template v-else-if="filtersTotal > 0 || searchSession.reverse">
          <div class="text-center text-sm">{{ t("partials.SearchResultsFeed.filtersAvailable", { count: filtersTotal }) }}</div>

          <template v-for="filter in limitedFiltersResults" :key="filter.filterId ?? filterResultKey(filter)">
            <FiltersResult
              :result="filter"
              :search-session="searchSession"
              :filters="filters"
              class="rounded-sm border border-gray-200 bg-white p-4 shadow-sm"
              @filter-update="(filterId, filter) => $emit('filterUpdate', filterId, filter)"
            />
          </template>

          <Button v-if="filtersHasMore" ref="filtersMoreButton" primary class="w-1/2 min-w-fit self-center" @click.prevent="filtersLoadMore">{{
            t("partials.SearchResultsFeed.moreFilters")
          }}</Button>

          <div v-else-if="filtersTotal > limitedFiltersResults.length" class="text-center text-sm">{{
            t("partials.SearchResultsFeed.filtersNotShown", { count: filtersTotal - limitedFiltersResults.length })
          }}</div>
        </template>
      </template>
    </div>
  </div>

  <SearchSortDialog
    :open="sortDialogOpen"
    :search-session="searchSession"
    :filter-columns="filtersResults"
    @close="sortDialogOpen = false"
    @sort-update="(sort) => $emit('sortUpdate', sort)"
  />

  <Teleport v-if="(searchTotal !== null && searchTotal > 0 && !hasMore) || searchTotal === 0" to="footer">
    <Footer class="border-t border-slate-50 bg-slate-200 shadow-sm" />
  </Teleport>
</template>
