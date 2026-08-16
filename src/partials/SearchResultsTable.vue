<script setup lang="ts">
import type { ComponentPublicInstance, DeepReadonly } from "vue"

import type { D } from "@/document"
import type { Filter, FilterResult, FilterUpdate, Result, SearchSession, ViewType } from "@/types"

import { LocalScope } from "@all1ndev/vue-local-scope"
import { Dialog, DialogPanel } from "@headlessui/vue"
import { ArrowTopRightOnSquareIcon, ChevronUpDownIcon, FunnelIcon, XMarkIcon } from "@heroicons/vue/20/solid"
import { ChevronDownUpIcon } from "@sidekickicons/vue/20/solid"
import { computed, onBeforeUnmount, onMounted, ref, toRef, useTemplateRef } from "vue"
import { useI18n } from "vue-i18n"

import Button from "@/components/Button.vue"
import WithDocument from "@/components/WithDocument.vue"
import siteContext from "@/context"
import { getClaimsOfTypeWithConfidence } from "@/document"
import ClaimValue from "@/partials/ClaimValue.vue"
import DisplayLabel from "@/partials/DisplayLabel.vue"
import FiltersResult from "@/partials/FiltersResult.vue"
import Footer from "@/partials/Footer.vue"
import ListFormat from "@/partials/ListFormat.vue"
import SearchResultsHeader from "@/partials/SearchResultsHeader.vue"
import { useBusy } from "@/progress"
import { getSearchHeaderComponents } from "@/registry/search-header"
import { FILTERS_INCREASE, FILTERS_INITIAL_LIMIT, filterResultKey, useFilters, useLocationAt } from "@/search"
import { useTruncationTracking } from "@/truncation"
import { encodeQuery, loadingWidth, useLimitResults, useOnScrollOrResize } from "@/utils"
import { useVisibilityTracking } from "@/visibility"

const props = defineProps<{
  // Search props.
  searchResults: DeepReadonly<Result[]>
  searchResultsUrl: string | null
  searchTotal: number | null
  searchMoreThanTotal: boolean
  searchSession: DeepReadonly<SearchSession>

  // Filter props.
  filters: Filter[]
}>()

const $emit = defineEmits<{
  filterUpdates: [updates: FilterUpdate[]]
  viewChange: [value: ViewType]
  downloadZip: []
  downloadFiles: []
}>()

const { t } = useI18n({ useScope: "global" })

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

const tableEl = useTemplateRef<HTMLElement>("tableEl")

// Data loading and controls for data loading.
const busy = useBusy()
const {
  results: filtersResults,
  total: filtersTotal,
  error: filtersError,
  url: filtersURL,
} = useFilters(
  toRef(() => props.searchSession),
  // The table view lists filter columns for the whole table and has no filter-pane search box, so the
  // value query is always empty here.
  toRef(() => ""),
  // We use the table element because data about filters is needed to display columns for the whole table.
  // Using only <tr> element inside <thead> (where data-url attribute is set for filters) would not convey that requirement.
  tableEl,
  busy,
)

const {
  limitedResults: limitedFiltersResults,
  hasMore: filtersHasMore,
  loadMore: filtersLoadMore,
} = useLimitResults(filtersResults, FILTERS_INITIAL_LIMIT, FILTERS_INCREASE)

function supportedFilter(filter: FilterResult) {
  return filter.type === "ref" || filter.type === "amount" || filter.type === "time"
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

useLocationAt(
  toRef(() => props.searchResults),
  toRef(() => props.searchTotal),
  visibles,
)

// A site which asks for its table to be filled by hand is not observed at all, so nothing but a press of a load
// more button loads anything. The feature is read here rather than checked inside the handler because the site
// context is fetched at boot and never changes, so there is no state in which this component is mounted with the
// observer registered under the wrong answer.
if (!siteContext.features.disableLoadingOnScroll) {
  useOnScrollOrResize([tableEl], onScrollOrResize)
}

// fill reveals another batch while the table ends less than one viewport past the fold in the direction that batch
// extends it, which both fills the first screen after a search and keeps loading while scrolling. More results add
// rows (so the table ends lower) and more filters add columns (so it ends further right), and each direction is
// therefore measured on its own end of the table.
function fill(moreButton: ComponentPublicInstance | null, end: number, viewport: number) {
  if (moreButton === null || end >= 2 * viewport) {
    return
  }

  // We load more by clicking the button so that we have one place to disable loading more (by disabling the button).
  // This assures that UX is consistent and that user cannot load more through any interaction (click or scroll).
  ;(moreButton.$el as HTMLButtonElement).click()
}

function onScrollOrResize() {
  if (abortController.signal.aborted) {
    return
  }

  // The table is measured instead of the page because the page is also as tall as everything around the table, and
  // as wide as its widest element, while the two buttons extend the table itself. It is rendered only once there
  // are results, and then it always has a box.
  if (tableEl.value === null) {
    return
  }

  const { bottom, right } = tableEl.value.getBoundingClientRect()
  fill(searchMoreButton.value, bottom, document.documentElement.clientHeight || document.body.clientHeight)
  fill(filtersMoreButton.value, right, document.documentElement.clientWidth || document.body.clientWidth)
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
  // We use -1 because we have a 1px border on the table which we want to offset. Otherwise there
  // is a 1px gap between the top edge of the window and where the header gets stuck.
  const top = Math.max(-1, bottom - 1)
  headerAttrs.value.style.top = `${top}px`
}

onMounted(() => {
  window.addEventListener("scroll", onScroll, { passive: true })
})

onBeforeUnmount(() => {
  window.removeEventListener("scroll", onScroll)
})

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
  return !!filter.filterId
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

const WithDocumentD = WithDocument<D>
</script>

<template>
  <!--
    TODO: No idea why w-0 (and w-fit) work here, but w-full does not.
          One would assume that w-full is needed to make the container div as wide as the
          body inside which then the footer horizontally shifts.
  -->
  <component :is="component" v-for="(component, i) in getSearchHeaderComponents().value" :key="i" :search-session="searchSession" />

  <div class="pd-searchresultstable pd-searchresultstable-header sticky left-0 z-20 w-0">
    <SearchResultsHeader
      class="w-container p-1 sm:p-4"
      :search-session="searchSession"
      :search-total="searchTotal"
      :search-more-than-total="searchMoreThanTotal"
      @view-change="(v) => $emit('viewChange', v)"
      @download-zip="$emit('downloadZip')"
      @download-files="$emit('downloadFiles')"
    />
  </div>

  <div v-if="filtersError" class="pd-searchresultstable-filters-error-wrapper mb-1 px-1 text-center sm:mb-4 sm:px-4">
    <i class="pd-searchresultstable-filters-error text-error-600">{{ t("common.status.loadingDataFailed") }}</i>
  </div>

  <template v-else-if="searchTotal !== null && searchTotal > 0">
    <div class="pd-searchresultstable flex flex-row gap-x-1 px-1 sm:gap-x-4 sm:px-4">
      <!-- TODO: Make table have rounded corners. -->
      <table ref="tableEl" class="pd-searchresultstable-table border border-gray-200 shadow-sm">
        <!-- Headers -->
        <!--
          We use -top-px because we have a 1px border on the table which we want to offset. Otherwise there
          is a 1px gap between the top edge of the window and where the header gets stuck.
        -->
        <thead class="pd-searchresultstable-head sticky -top-px z-10 bg-slate-300" v-bind="headerAttrs">
          <tr class="pd-searchresultstable-row-header" :data-url="filtersURL">
            <th class="pd-searchresultstable-column-index p-2 text-start">#</th>
            <th v-if="filtersTotal === null" class="p-2 text-start">
              <div
                class="pd-searchresultstable-loading inline-block h-2 rounded-sm bg-slate-200 motion-safe:animate-pulse"
                :class="[loadingWidth(`${searchSession.id}/0`)]"
                aria-hidden="true"
              />
            </th>
            <template v-for="filter in limitedFiltersResults" v-else :key="filter.filterId ?? filterResultKey(filter)">
              <th
                v-if="supportedFilter(filter)"
                class="pd-searchresultstable-column-filter text-start"
                :class="`pd-searchresultstable-column-filter-${filter.props?.[0] ?? ''}`"
              >
                <!-- <div class="flex flex-row items-center justify-between"> -->
                <WithDocumentD :id="filter.props?.[0] ?? ''" name="DocumentGet">
                  <template #default="{ doc, url }">
                    <Button
                      :data-url="url"
                      class="pd-searchresultstable-button-filter flex w-full max-w-100 flex-row items-center justify-between gap-x-1 p-2 leading-none shadow-none inset-ring-0"
                      @click.prevent="onOpenFilterModal(filter)"
                    >
                      <span class="truncate"><DisplayLabel :doc="doc" /></span>
                      <FunnelIcon class="size-5" :class="isFilterActive(filter) ? '' : 'text-primary-300'" />
                    </Button>
                  </template>
                  <template #loading="{ url }">
                    <div
                      class="pd-withdocument-loading inline-block h-2 rounded-sm bg-slate-200 motion-safe:animate-pulse"
                      :data-url="url"
                      :class="[loadingWidth(filter.props?.[0] ?? '')]"
                      aria-hidden="true"
                    />
                  </template>
                </WithDocumentD>
              </th>
            </template>
          </tr>
        </thead>

        <!-- Results -->
        <tbody :data-url="searchResultsUrl" class="pd-searchresultstable-list-results divide-y divide-gray-200">
          <template v-for="(result, index) in limitedSearchResults" :key="result.id">
            <WithDocumentD :id="result.id" name="DocumentGet">
              <template #default="{ doc, url }">
                <tr
                  :id="`result-${result.id}`"
                  :ref="track(result.id)"
                  class="pd-searchresultstable-row-result odd:bg-white even:bg-slate-100 hover:bg-slate-200"
                  :data-url="url"
                >
                  <td class="pd-searchresultstable-cell-index flex items-center justify-between gap-1 p-2">
                    <RouterLink
                      :to="{ name: 'DocumentGet', params: { id: result.id }, query: encodeQuery({ s: searchSession.id }) }"
                      class="pd-searchresultstable-link-document link"
                      >{{ index + 1 }}</RouterLink
                    >

                    <Button
                      v-if="canRowExpand(result.id) || isRowExpanded(result.id)"
                      :title="getButtonTitle(result.id)"
                      class="pd-searchresultstable-button-expandrow p-0 shadow-none inset-ring-0"
                      @click.prevent="onToggleRow(result.id)"
                    >
                      <ChevronDownUpIcon v-if="isRowExpanded(result.id)" class="size-5" aria-expanded="true" :aria-controls="`result-${result.id}`" />
                      <ChevronUpDownIcon v-else class="size-5" aria-expanded="false" :aria-controls="`result-${result.id}`" />
                    </Button>
                  </td>
                  <td v-if="filtersTotal === null" class="p-2">
                    <div
                      class="pd-searchresultstable-loading inline-block h-2 rounded-sm bg-slate-200 motion-safe:animate-pulse"
                      :class="[loadingWidth(`${searchSession.id}/${index + 1}`)]"
                      aria-hidden="true"
                    />
                  </td>
                  <template v-for="filter in limitedFiltersResults" v-else :key="filter.filterId ?? filterResultKey(filter)">
                    <td v-if="supportedFilter(filter)" class="pd-searchresultstable-cell-value pd-searchresultstable-value align-top">
                      <LocalScope
                        v-slot="{ rowExpanded, cellTruncated, cellExpanded }"
                        :row-expanded="isRowExpanded(result.id)"
                        :cell-truncated="isCellTruncated(result.id, `${filter.filterId ?? `${filter.type}/${filter.props?.join('/') ?? ''}`}`)"
                        :cell-expanded="isCellExpanded(result.id, `${filter.filterId ?? `${filter.type}/${filter.props?.join('/') ?? ''}`}`)"
                      >
                        <!--
                          We have div wrapper so that we can control the height of the row. td elements cannot have height set.
                          We set min-height to line height + padding.
                        -->
                        <div
                          :ref="trackTruncation(result.id, `${filter.filterId ?? `${filter.type}/${filter.props?.join('/') ?? ''}`}`)"
                          class="min-h-[calc(1lh+var(--spacing)*2)] max-w-100 overscroll-contain p-2"
                          :class="[rowExpanded ? 'max-h-75 overflow-auto' : 'max-h-[calc(1lh+var(--spacing)*2)] truncate overflow-clip']"
                        >
                          <div
                            v-if="(cellTruncated && rowExpanded) || cellExpanded || cellTruncated"
                            class="pd-searchresultstable-actions-cell float-right mt-[calc((1lh-var(--spacing)*5)/2)] flex gap-1"
                          >
                            <RouterLink
                              v-if="cellTruncated && rowExpanded"
                              :to="{ name: 'DocumentGet', params: { id: result.id }, query: encodeQuery({ s: searchSession.id }) }"
                              class="pd-searchresultstable-link-open link"
                            >
                              <ArrowTopRightOnSquareIcon class="size-5" />
                            </RouterLink>

                            <Button
                              v-if="cellExpanded || cellTruncated"
                              :title="getButtonTitle(result.id)"
                              class="pd-searchresultstable-button-expandrow p-0 shadow-none inset-ring-0"
                              @click.prevent="onToggleRow(result.id)"
                            >
                              <ChevronDownUpIcon v-if="rowExpanded" class="size-5" aria-expanded="true" :aria-controls="`result-${result.id}`" />
                              <ChevronUpDownIcon v-else class="size-5" aria-expanded="false" :aria-controls="`result-${result.id}`" />
                            </Button>
                          </div>

                          <LocalScope v-slot="{ claims }" :claims="getClaimsOfTypeWithConfidence(doc.claims, filter.type, filter.props?.[0] ?? '')">
                            <ListFormat v-slot="{ index: claimIndex }" :count="claims.length"><ClaimValue :type="filter.type" :claim="claims[claimIndex]" /></ListFormat>
                          </LocalScope>
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
                <tr class="pd-searchresultstable-row-loading pd-withdocument-loading-wrapper odd:bg-white even:bg-slate-100 hover:bg-slate-200" :data-url="url">
                  <td class="p-2">
                    <RouterLink
                      :to="{ name: 'DocumentGet', params: { id: result.id }, query: encodeQuery({ s: searchSession.id }) }"
                      class="pd-searchresultstable-link-document link"
                      >{{ index + 1 }}</RouterLink
                    >
                  </td>
                  <td :colspan="rowColspan" class="p-2">
                    <div
                      class="pd-withdocument-loading inline-block h-2 rounded-sm bg-slate-200 motion-safe:animate-pulse"
                      :class="[loadingWidth(result.id)]"
                      aria-hidden="true"
                    />
                  </td>
                </tr>
              </template>
              <!-- We do not track(result.id) <tr> here. See explanation above. -->
              <template #error="{ message, accessDenied, url }">
                <tr class="pd-searchresultstable-row-error pd-withdocument-error-wrapper odd:bg-white even:bg-slate-100 hover:bg-slate-200" :data-url="url">
                  <td class="p-2">
                    <RouterLink
                      :to="{ name: 'DocumentGet', params: { id: result.id }, query: encodeQuery({ s: searchSession.id }) }"
                      class="pd-searchresultstable-link-document link"
                      >{{ index + 1 }}</RouterLink
                    >
                  </td>
                  <td :colspan="rowColspan" class="p-2">
                    <i :class="['pd-withdocument-error', accessDenied ? 'text-gray-500' : 'text-error-600']">{{ message }}</i>
                  </td>
                </tr>
              </template>
            </WithDocumentD>
          </template>
        </tbody>
      </table>

      <div v-if="filtersHasMore" class="pd-searchresultstable-actions-columns sticky top-[37.5%] z-20 h-full">
        <Button
          ref="filtersMoreButton"
          primary
          class="pd-searchresultstable-button-morecolumns h-1/4 min-h-fit [writing-mode:sideways-lr]"
          @click.prevent="filtersLoadMore"
          >{{ t("partials.SearchResultsTable.moreColumns") }}</Button
        >
      </div>
    </div>

    <!--
      TODO: No idea why w-0 (and w-fit) work here, but w-full does not.
            One would assume that w-full is needed to make the container div as wide as the
            body inside which then the footer horizontally shifts.
    -->
    <div class="pd-searchresultstable-footer sticky left-0 z-20 w-0">
      <div class="w-container flex justify-center p-1 sm:p-4">
        <Button v-if="searchHasMore" id="searchresultstable-button-loadmore" ref="searchMoreButton" primary class="w-1/4 min-w-fit" @click.prevent="searchLoadMore">{{
          t("common.buttons.loadMore")
        }}</Button>

        <div v-else class="my-1 sm:my-4">
          <!-- Here we assume that MaxResultsCount is always set to a smaller value than what TrackTotalHits is set to. -->
          <div v-if="searchMoreThanTotal" class="pd-searchresultstable-count-results text-center text-sm">{{
            t("common.status.allResultsMoreThan", { first: searchResults.length, count: searchTotal })
          }}</div>
          <div v-else-if="searchResults.length < searchTotal" class="pd-searchresultstable-count-results text-center text-sm">{{
            t("common.status.allResultsOnly", { first: searchResults.length, count: searchTotal })
          }}</div>
          <div v-else-if="searchResults.length === searchTotal" class="pd-searchresultstable-count-results text-center text-sm">{{
            t("common.status.allResults", { count: searchResults.length })
          }}</div>
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
  <Dialog as="div" class="pd-searchresultstable-dialog relative z-50" :open="activeFilter !== null && searchTotal !== null" @close="onCloseFilterModal">
    <!-- Backdrop. -->
    <div class="pd-searchresultstable-backdrop fixed inset-0 bg-black/30" aria-hidden="true" />

    <!-- Full-screen container to center the panel. -->
    <div class="fixed inset-0 flex items-center justify-center">
      <DialogPanel
        class="pd-searchresultstable-panel-filter flex h-full w-full flex-col overflow-y-auto rounded-none bg-white p-1 shadow-none sm:relative sm:inset-auto sm:h-auto sm:max-h-150 sm:max-w-xl sm:rounded-sm sm:p-4 sm:shadow-sm"
      >
        <FiltersResult :result="activeFilter!" :search-session="searchSession" :filters="filters" @filter-updates="(updates) => $emit('filterUpdates', updates)" />

        <Button
          class="pd-searchresultstable-button-closefilter absolute top-1 right-1 p-0 shadow-none inset-ring-0 sm:top-4 sm:right-4"
          title="Close"
          @click="onCloseFilterModal"
        >
          <XMarkIcon class="size-5" />
        </Button>
      </DialogPanel>
    </div>
  </Dialog>
</template>
