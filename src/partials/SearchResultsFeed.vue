<script setup lang="ts">
import type { ComponentPublicInstance, DeepReadonly } from "vue"

import type { D } from "@/document"
import type { Filter, Result, SearchSession, SortKey, ViewType } from "@/types"

import { ChevronUpDownIcon, FunnelIcon, XMarkIcon } from "@heroicons/vue/20/solid"
import { ChevronDownUpIcon } from "@sidekickicons/vue/20/solid"
import { computed, onBeforeUnmount, onMounted, provide, reactive, ref, toRaw, toRef, useTemplateRef, watch } from "vue"
import { useI18n } from "vue-i18n"

import Button from "@/components/Button.vue"
import InputText from "@/components/InputText.vue"
import WithDocument from "@/components/WithDocument.vue"
import WithLock from "@/components/WithLock.vue"
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
import {
  clone,
  limitGroupedResults,
  loadingWidth,
  searchExpandKey,
  searchFilterVisibilityKey,
  searchHiddenClaimsKey,
  searchLoadAllClaimsKey,
  searchPagerKey,
  SKIP_TO_END,
  useLimitResults,
  useOnScrollOrResize,
} from "@/utils"
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
  reverseExpandUpdate: [expand: boolean]
  prefiltersClear: []
  sortUpdate: [sort: SortKey[]]
}>()

const { t } = useI18n({ useScope: "global" })

const sortDialogOpen = ref(false)

// Results are grouped when the session's sort has a leading run of group columns; the backend then returns
// nested results which we render as a group tree instead of a flat list.
const grouped = computed(() => (props.searchSession.sort ?? []).some((s) => s.group))

// groupExpand[d] reports whether the group level at depth d should render each group value as a full result
// card instead of a one-line heading. It mirrors the leading run of group columns (the same order as the
// tree's nesting), so SearchResultGroup can read its own level by depth.
const groupExpand = computed<boolean[]>(() => {
  const out: boolean[] = []
  for (const s of props.searchSession.sort ?? []) {
    if (!s.group) {
      break
    }
    out.push(s.expand ?? false)
  }
  return out
})

// setExpandLevel sets whether the grouping level at depth is expanded by writing expand on that group column
// and emitting the change, the same update the sort dialog's Expand checkbox makes. It is provided to the
// nested SearchResultGroup tree so a heading's expand control and an expanded card's collapse control can both
// trigger it in place.
function setExpandLevel(depth: number, expand: boolean): void {
  const newSort = clone(props.searchSession.sort ?? [])
  if (depth < 0 || depth >= newSort.length || !newSort[depth].group) {
    return
  }
  newSort[depth].expand = expand
  $emit("sortUpdate", newSort)
}
provide(searchExpandKey, setExpandLevel)

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

// Whether the print view's "Load all" button has been pressed. Provided to each result's FieldsView so that,
// alongside revealing every result, it also reveals every repeating claim value instead of capping them
// behind a per-field "Show all" button. It resets together with the result limit when a new result set arrives.
const loadAllClaims = ref(false)
provide(searchLoadAllClaimsKey, loadAllClaims)

// Number of FieldsView instances that currently have repeating claim values hidden behind a "Show all" button.
// anyHiddenClaims lets the print view's "Load all" button appear whenever there is anything left to reveal,
// including when every result already fits on screen but some result still caps its claims. Each FieldsView
// keeps contribution balanced, so the total never drifts.
const hiddenClaimsTotal = ref(0)
function reportHiddenClaims(delta: number): void {
  hiddenClaimsTotal.value += delta
}
provide(searchHiddenClaimsKey, reportHiddenClaims)
const anyHiddenClaims = computed(() => hiddenClaimsTotal.value > 0)

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
    loadAllClaims.value = false
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

// loadAll reveals every remaining loaded result at once (up to the server cap) and every repeating claim value
// inside each result, used by the print view's "Load all" button so a printout is not limited to the
// incrementally revealed subset of results nor to the first few values of a repeating field.
function loadAll(): void {
  if (grouped.value) {
    groupedLoadAll()
  } else {
    searchLoadAll()
  }
  loadAllClaims.value = true
  hiddenClaimsTotal.value = 0
}

const filtersEl = useTemplateRef<HTMLElement>("filtersEl")
const filtersEnabled = ref(false)

// filterQuery is the free-text the user typed into the filter-pane search box. It narrows which filters
// and filter values are shown (via the API) without changing the search itself: the facet list is limited
// to facets reachable by the text (through a value name or the facet's own property name), and each facet's
// values are limited to the matching ones (a facet reached by its name shows all of its values).
const filterQuery = ref("")

// Data loading and controls for data loading. The filter-pane search box opts out of this lock (see
// filterBoxLock) so it stays editable while results refresh.
const busy = useBusy()
const {
  results: filtersResults,
  total: filtersTotal,
  error: filtersError,
  url: filtersURL,
} = useFilters(
  toRef(() => props.searchSession),
  filterQuery,
  filtersEl,
  busy,
)

// Lock provided around just the filter-pane search box. It is a constant zero, so the box never enters the
// readonly/disabled state busy puts the rest of the pane in: narrowing by typing must not disable the box.
// Each keystroke aborts the previous request and issues a new one (see useFilters). The box has no
// validation of its own, so nothing else would lock it.
const filterBoxLock = ref(0)
function getFilterBoxLock() {
  return filterBoxLock
}

const {
  limitedResults: limitedFiltersResults,
  hasMore: filtersHasMore,
  loadMore: filtersLoadMore,
} = useLimitResults(filtersResults, FILTERS_INITIAL_LIMIT, FILTERS_INCREASE)

// Number of active (enabled) filters in the result list. The backend emits a facet carrying the filter id for
// every active filter, even when the current combination of filters matches no document (the available-filters
// total is then zero and facet discovery returns nothing else). We keep showing these so an enabled filter can
// always be cleared, instead of collapsing the pane to the "no filters" message.
const activeFiltersCount = computed(() => filtersResults.value.filter((result) => Boolean(result.filterId)).length)
const hasActiveFilters = computed(() => activeFiltersCount.value > 0)

// Visibility of each rendered filter facet, keyed by the stable id it reports through searchFilterVisibilityKey. A
// reference or has facet hides itself while no value matches the filter-pane search (see hiddenByQuery); amount and
// time facets have no such state and stay visible. Tracking the real per-facet state (rather than inferring it from
// the result list, which cannot see each facet's own value query) keeps the no-match message in sync with the screen.
const filterVisibility = reactive(new Map<string, boolean>())
function reportFilterVisibility(id: string, visible: boolean | null): void {
  if (visible === null) {
    filterVisibility.delete(id)
  } else {
    filterVisibility.set(id, visible)
  }
}
provide(searchFilterVisibilityKey, reportFilterVisibility)

// Whether any rendered filter facet is currently visible. While a search is in progress and this is false the
// list is empty on screen, so we show the no-match message in its place.
const anyFilterVisible = computed(() => {
  for (const visible of filterVisibility.values()) {
    if (visible) {
      return true
    }
  }
  return false
})

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
      <!-- Print row: the close and show-all buttons (preview only, left) and a live timestamp (right). -->
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
          v-if="hasMore || anyHiddenClaims"
          type="button"
          class="pd-preview-only items-center gap-x-1 rounded-sm bg-primary-600 px-3 py-2 text-sm text-white shadow-lg outline-none hover:bg-primary-700 focus:ring-2 focus:ring-primary-500 focus:ring-offset-1"
          @click.prevent="loadAll"
        >
          {{ t("common.buttons.showAll") }}
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

      <!--
        Print-only: the reverse scope (documents referencing a target), shown above the filters list. When
        reverseExpand is set it shows the target's full result card instead of the one-line heading. The
        expand/collapse controls are preview-only (interactive), so a real print just shows the chosen form.
      -->
      <div v-if="searchSession.reverse" class="pd-print-only">
        <!-- Collapsed: the referenced target inline, with a control to expand it into its full card. -->
        <div v-if="!searchSession.reverseExpand" class="mx-1 flex items-baseline gap-x-1">
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
          <button
            type="button"
            class="pd-preview-only shrink-0 self-center rounded-sm p-0.5 text-slate-400 outline-none hover:bg-slate-200 hover:text-slate-600 focus:ring-2 focus:ring-primary-500"
            :title="t('partials.SearchResultsFeed.expandReferencing')"
            @click.prevent="$emit('reverseExpandUpdate', true)"
          >
            <ChevronUpDownIcon class="size-5" :alt="t('partials.SearchResultsFeed.expandReferencing')" />
          </button>
        </div>
        <!-- Expanded: a heading with a collapse control, followed by the referenced target's full card. -->
        <template v-else>
          <div class="mx-1 flex items-baseline gap-x-1">
            {{ t("partials.SearchResultsFeed.resultsReferencingExpanded") }}
            <button
              type="button"
              class="pd-preview-only shrink-0 self-center rounded-sm p-0.5 text-slate-400 outline-none hover:bg-slate-200 hover:text-slate-600 focus:ring-2 focus:ring-primary-500"
              :title="t('partials.SearchResultsFeed.collapseReferencing')"
              @click.prevent="$emit('reverseExpandUpdate', false)"
            >
              <ChevronDownUpIcon class="size-5" :alt="t('partials.SearchResultsFeed.collapseReferencing')" />
            </button>
          </div>
          <SearchResult :search-session-id="searchSession.id" :result="{ id: searchSession.reverse }" class="mt-1 sm:mt-4" />
          <!-- Separator below the expanded reference card, matching the border under a group heading. -->
          <hr class="mt-1 border-slate-200 sm:mt-4" />
        </template>
      </div>

      <template v-if="searchTotal !== null && searchTotal > 0">
        <template v-if="grouped">
          <template v-for="(node, gi) in limitedGroupedResults.results" :key="`${node.id}-${gi}`">
            <SearchResultsPager v-if="topPagerIndex(node) !== undefined" :i="topPagerIndex(node)!" :shown="groupedPager.shown" :total="groupedPager.total" :depth="0" />
            <SearchResultGroup :node="node" :search-session-id="searchSession.id" :depth="0" :expand-levels="groupExpand" />
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
        <div v-if="searchSession.reverse" class="text-sm">
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

        <div v-if="searchSession.prefilters && searchSession.prefilters.length > 0" class="text-sm">
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

        <!--
          No filters at all and not searching: there is nothing to search, so the box is not shown.
          Active filters keep the pane open even when nothing is available, so they can still be cleared.
        -->
        <div v-if="filtersTotal === 0 && !filterQuery && !hasActiveFilters" class="my-1 sm:my-4">
          <div class="text-center text-sm">{{ t("partials.SearchResultsFeed.noFilters") }}</div>
        </div>

        <template v-else>
          <!--
            This branch is the complement of the no-filters case above, so there are always facets to narrow:
            available filters, active filters (their values can still be searched, e.g. to find one to deselect),
            or a search already in progress. The search box is therefore always shown here; it narrows which
            filters and filter values are shown, never the search itself.
          -->
          <div>
            <!--
              The count is the number of filters for the current search and stays constant as the box is typed
              in; the box only narrows which filters and filter values are shown (by a value name or the facet's
              own name), never the search itself. When only active filters are shown (none available to add), the
              label counts those instead.
            -->
            <div v-if="filtersTotal > 0" class="mb-1 text-sm">{{ t("partials.SearchResultsFeed.filtersAvailable", { count: filtersTotal }) }}</div>
            <div v-else-if="hasActiveFilters" class="mb-1 text-sm">{{ t("partials.SearchResultsFeed.filtersActive", { count: activeFiltersCount }) }}</div>

            <WithLock :lock="getFilterBoxLock">
              <InputText v-model="filterQuery" class="pd-print-hidden w-full" :aria-label="t('partials.SearchResultsFeed.filtersSearchLabel')" />
            </WithLock>
          </div>

          <template v-for="filter in limitedFiltersResults" :key="filter.filterId ?? filterResultKey(filter)">
            <FiltersResult
              :result="filter"
              :search-session="searchSession"
              :filters="filters"
              :query="filterQuery"
              class="rounded-sm border border-gray-200 bg-white p-4 shadow-sm"
              @filter-update="(filterId, filter) => $emit('filterUpdate', filterId, filter)"
            />
          </template>

          <Button v-if="filtersHasMore" ref="filtersMoreButton" primary class="w-1/2 min-w-fit self-center" @click.prevent="filtersLoadMore">{{
            t("partials.SearchResultsFeed.moreFilters")
          }}</Button>

          <!-- Counts of shown vs returned use the (possibly narrowed) returned facets, not the constant total. -->
          <div v-else-if="filtersResults.length > limitedFiltersResults.length" class="text-center text-sm">{{
            t("partials.SearchResultsFeed.filtersNotShown", { count: filtersResults.length - limitedFiltersResults.length })
          }}</div>

          <!--
            When a search hides every facet (each reference or has facet hides itself while no value matches it, and
            no always-visible amount or time facet remains), the list is empty on screen, so the no-match message
            takes its place; the box stays above so the query can be changed or cleared.
          -->
          <div v-if="!anyFilterVisible && filterQuery" class="text-center text-sm">{{ t("partials.SearchResultsFeed.filtersNoMatch") }}</div>
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
