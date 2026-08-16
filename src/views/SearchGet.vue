<script setup lang="ts">
import type { PrefilterPayload } from "@/search"
import type { Filter, FilterUpdate, SearchSessionData, SortKey, ViewType } from "@/types"
import type { DeepReadonly } from "vue"

import { Identifier } from "@tozd/identifier"
import { computed, onBeforeUnmount, provide, ref, toRef, useTemplateRef, watch, watchEffect } from "vue"
import { useI18n } from "vue-i18n"
import { useRouter } from "vue-router"

import { useDownload } from "@/download"
import { useNavbarSearchQuery } from "@/navbar"
import WithLock from "@/components/WithLock.vue"
import DownloadOverlay from "@/partials/DownloadOverlay.vue"
import Footer from "@/partials/Footer.vue"
import NavBar from "@/partials/NavBar.vue"
import NavBarSearch from "@/partials/NavBarSearch.vue"
import SearchResultsFeed from "@/partials/SearchResultsFeed.vue"
import SearchResultsTable from "@/partials/SearchResultsTable.vue"
import { pairCounters, useLock, useProgress } from "@/progress"
import { searchShortcutControllerKey, updateSearchSession, useSearch, useSearchSession } from "@/search"
import { clone } from "@/utils"

const props = defineProps<{
  id: string
}>()

const { t, locale } = useI18n({ useScope: "global" })
const router = useRouter()

// Data loading and controls for data loading. The two channels are made here rather than through useBusy so
// that the download can raise the lock alone: while files are being gathered the view's controls have to
// stay out of the way, but the bar across the top of the page is about the page fetching what it shows, and
// the download has an overlay of its own which reports how far it has got.
const progress = useProgress()
const lock = useLock()
const busy = pairCounters(progress, lock)

const abortController = new AbortController()
onBeforeUnmount(() => {
  // Aborting the controller also tears down any active download worker via useDownload's abort listener.
  abortController.abort()
})

const searchEl = useTemplateRef<HTMLElement>("searchEl")

// The current navbar search input value, so applying a prefilter can commit the possibly uncommitted
// query, just as clicking the search button does.
const navbarSearchQuery = useNavbarSearchQuery()

const searchSessionVersion = ref(0)

const {
  searchSession,
  error: searchSessionError,
  url: searchURL,
} = useSearchSession(
  toRef(() => ({ id: props.id, version: searchSessionVersion.value })),
  busy,
)
const { results: searchResults, total: searchTotal, moreThanTotal: searchMoreThanTotal, error: searchResultsError } = useSearch(searchSession, searchEl, busy)

const {
  downloadingPhase,
  completed,
  total,
  currentFile,
  error: downloadError,
  startZipDownload,
  startBulkDownload,
  cancelDownload,
} = useDownload(abortController, router, searchResults)

// A download in preparation makes the view busy: its controls lock, so a filter, a sort or an edit cannot
// land in the middle of gathering the files it was asked for. It is raised on the lock alone (see above),
// and the overlay opts out of it below, because the way out of a download is its own cancel button.
watch(downloadingPhase, (phase, previous) => {
  if (phase !== null && previous === null) {
    lock.value += 1
  } else if (phase === null && previous !== null) {
    lock.value -= 1
  }
})

// The lock the download overlay is rendered under: a constant zero, so the overlay stays usable while the
// download it reports on has the rest of the view locked.
const overlayLock = ref(0)
function getOverlayLock() {
  return overlayLock
}

// A non-read-only version of filters so that we can modify it as necessary.
const filters = ref<Filter[]>([])
// We keep it in sync with upstream version.
watchEffect(() => {
  // We copy to make a read-only value mutable.
  if (searchSession.value === null || !searchSession.value.filters) {
    filters.value = []
  } else {
    filters.value = clone(searchSession.value.filters)
  }
})

async function onSearchSessionUpdate(searchData: DeepReadonly<SearchSessionData>) {
  if (abortController.signal.aborted) {
    return
  }

  busy.value += 1
  try {
    const response = await updateSearchSession(router, props.id, searchData, abortController.signal, busy)
    if (abortController.signal.aborted || !response) {
      return
    }
    searchSessionVersion.value = response.version
  } catch (err) {
    if (abortController.signal.aborted) {
      return
    }
    // TODO: Show notification with error.
    console.error("SearchGet.onSearchSessionUpdate", err)
  } finally {
    busy.value -= 1
  }
}

// applyPrefilters replaces the session's prefilters with the given shortcut payloads (generating
// Base/ID for each, as onFilterUpdate does for filters), or clears them when null/empty. It is exposed
// to navbar search shortcut buttons via the controller so they can toggle prefilters in place.
async function applyPrefilters(payloads: PrefilterPayload[] | null) {
  // Checking abortController is done inside onSearchSessionUpdate.
  if (!searchSession.value) {
    return
  }
  let prefilters: Filter[] | undefined
  if (payloads && payloads.length > 0) {
    prefilters = []
    // A payload's values become a ref prefilter and its missing selection the path's specials
    // prefilter (a filter carries exactly one selection kind), each with its own generated Base/ID.
    for (const payload of payloads) {
      if (payload.to.length > 0 || payload.direct.length > 0) {
        const filterBase = [...searchSession.value.base, "FILTER", Identifier.new().toString()]
        // Deriving the identifier is asynchronous, so the view can be left in the middle of the loop.
        const id = (await Identifier.from(...filterBase)).toString()
        if (abortController.signal.aborted) {
          return
        }
        prefilters.push({
          id,
          base: filterBase,
          prop: payload.prop,
          ref: {
            to: payload.to.length > 0 ? payload.to : undefined,
            direct: payload.direct.length > 0 ? payload.direct : undefined,
          },
        })
      }
      if (payload.missing) {
        const filterBase = [...searchSession.value.base, "FILTER", Identifier.new().toString()]
        const id = (await Identifier.from(...filterBase)).toString()
        if (abortController.signal.aborted) {
          return
        }
        prefilters.push({
          id,
          base: filterBase,
          prop: payload.prop,
          specials: { missing: true },
        })
      }
    }
  }
  await onSearchSessionUpdate({
    view: searchSession.value.view,
    // Commit the current navbar query input together with the prefilter change, so clicking a search
    // shortcut behaves like clicking the search button (using the possibly edited, uncommitted query)
    // and in addition sets the prefilter.
    query: navbarSearchQuery.value,
    filters: searchSession.value.filters,
    reverse: searchSession.value.reverse,
    reverseExpand: searchSession.value.reverseExpand,
    ids: searchSession.value.ids,
    prefilters,
    language: searchSession.value.language,
    sort: searchSession.value.sort,
  })
}

// Expose the current prefilters and an apply function so navbar search shortcut buttons (rendered in
// the teleported NavBar, which is a logical descendant of this view) can toggle them.
provide(searchShortcutControllerKey, {
  prefilters: computed(() => searchSession.value?.prefilters),
  applyPrefilters,
})

// Changing the UI language while viewing a session is treated like any other change to the session
// data: we set the new language and refetch results. It is on purpose not updated on search session
// load time so that users with different languages do not update language when loading but just on
// explicit language changes.
watch(locale, async () => {
  // Checking abortController is done inside onSearchSessionUpdate.
  if (!searchSession.value) {
    return
  }

  await onSearchSessionUpdate({
    view: searchSession.value.view,
    query: searchSession.value.query,
    filters: searchSession.value.filters,
    reverse: searchSession.value.reverse,
    reverseExpand: searchSession.value.reverseExpand,
    ids: searchSession.value.ids,
    prefilters: searchSession.value.prefilters,
    language: locale.value,
    sort: searchSession.value.sort,
  })
})

async function onFiltersUpdate(updatedFilters: Filter[]) {
  // Checking abortController is done inside onSearchSessionUpdate.

  await onSearchSessionUpdate({
    view: searchSession.value!.view,
    query: searchSession.value!.query,
    filters: updatedFilters.length > 0 ? updatedFilters : undefined,
    reverse: searchSession.value!.reverse,
    reverseExpand: searchSession.value!.reverseExpand,
    ids: searchSession.value!.ids,
    prefilters: searchSession.value!.prefilters,
    language: searchSession.value!.language,
    sort: searchSession.value!.sort,
  })
}

// isFilterEmpty returns true if the filter has no active selection.
function isFilterEmpty(f: Filter): boolean {
  if ("ref" in f) {
    return (!f.ref.to || f.ref.to.length === 0) && (!f.ref.direct || f.ref.direct.length === 0)
  }
  if ("amount" in f) {
    return f.amount.gte == null && f.amount.lte == null && !f.amount.exists
  }
  if ("time" in f) {
    return f.time.gte == null && f.time.lte == null && !f.time.exists
  }
  if ("has" in f) {
    return !f.has.props || f.has.props.length === 0
  }
  if ("specials" in f) {
    return !f.specials.missing && !f.specials.none && !f.specials.unknown && !f.specials.hasProperty
  }
  return true
}

async function onFilterUpdates(updates: FilterUpdate[]) {
  // Checking abortController is done inside onSearchSessionUpdate.

  // A facet interaction can change several filters at once (its typed filter and the path's specials
  // filter), so the whole batch is applied to the filter list first and the session is updated once.
  let updatedFilters = [...filters.value]
  for (const { filterId, filter } of updates) {
    if (isFilterEmpty(filter)) {
      // Filter has no active selection: remove it from the session.
      updatedFilters = updatedFilters.filter((f) => f.id !== filterId)
    } else if (filterId && updatedFilters.some((f) => f.id === filterId)) {
      // Existing filter: replace it.
      updatedFilters = updatedFilters.map((f) => (f.id === filterId ? filter : f))
    } else {
      // New filter: generate Base/ID and add it.
      const filterBase = [...searchSession.value!.base, "FILTER", Identifier.new().toString()]
      // Deriving the identifier is asynchronous, so the view can be left in the middle of the batch.
      const id = (await Identifier.from(...filterBase)).toString()
      if (abortController.signal.aborted) {
        return
      }
      updatedFilters.push({ ...filter, base: filterBase, id })
    }
  }
  await onFiltersUpdate(updatedFilters)
}

async function onQueryChange(query: string) {
  // Checking abortController is done inside onSearchSessionUpdate.

  await onSearchSessionUpdate({
    view: searchSession.value!.view,
    query,
    filters: searchSession.value!.filters,
    reverse: searchSession.value!.reverse,
    reverseExpand: searchSession.value!.reverseExpand,
    ids: searchSession.value!.ids,
    prefilters: searchSession.value!.prefilters,
    language: searchSession.value!.language,
    sort: searchSession.value!.sort,
  })
}

async function onViewChange(view: ViewType) {
  // Checking abortController is done inside onSearchSessionUpdate.

  await onSearchSessionUpdate({
    view,
    query: searchSession.value!.query,
    filters: searchSession.value!.filters,
    reverse: searchSession.value!.reverse,
    reverseExpand: searchSession.value!.reverseExpand,
    ids: searchSession.value!.ids,
    prefilters: searchSession.value!.prefilters,
    language: searchSession.value!.language,
    sort: searchSession.value!.sort,
  })
}

async function onReverseClear() {
  // Checking abortController is done inside onSearchSessionUpdate.

  await onSearchSessionUpdate({
    view: searchSession.value!.view,
    query: searchSession.value!.query,
    filters: searchSession.value!.filters,
    reverse: undefined,
    reverseExpand: undefined,
    ids: searchSession.value!.ids,
    prefilters: searchSession.value!.prefilters,
    language: searchSession.value!.language,
    sort: searchSession.value!.sort,
  })
}

async function onIdsClear() {
  // Checking abortController is done inside onSearchSessionUpdate.

  await onSearchSessionUpdate({
    view: searchSession.value!.view,
    query: searchSession.value!.query,
    filters: searchSession.value!.filters,
    reverse: searchSession.value!.reverse,
    reverseExpand: searchSession.value!.reverseExpand,
    ids: undefined,
    prefilters: searchSession.value!.prefilters,
    language: searchSession.value!.language,
    sort: searchSession.value!.sort,
  })
}

async function onPrefiltersClear() {
  // Checking abortController is done inside onSearchSessionUpdate.

  await onSearchSessionUpdate({
    view: searchSession.value!.view,
    query: searchSession.value!.query,
    filters: searchSession.value!.filters,
    reverse: searchSession.value!.reverse,
    reverseExpand: searchSession.value!.reverseExpand,
    ids: searchSession.value!.ids,
    prefilters: undefined,
    language: searchSession.value!.language,
  })
}

async function onSortUpdate(sort: SortKey[]) {
  // Checking abortController is done inside onSearchSessionUpdate.

  await onSearchSessionUpdate({
    view: searchSession.value!.view,
    query: searchSession.value!.query,
    filters: searchSession.value!.filters,
    reverse: searchSession.value!.reverse,
    reverseExpand: searchSession.value!.reverseExpand,
    ids: searchSession.value!.ids,
    prefilters: searchSession.value!.prefilters,
    language: searchSession.value!.language,
    sort: sort.length > 0 ? sort : undefined,
  })
}

async function onReverseExpandUpdate(reverseExpand: boolean) {
  // Checking abortController is done inside onSearchSessionUpdate.

  await onSearchSessionUpdate({
    view: searchSession.value!.view,
    query: searchSession.value!.query,
    filters: searchSession.value!.filters,
    reverse: searchSession.value!.reverse,
    reverseExpand,
    ids: searchSession.value!.ids,
    prefilters: searchSession.value!.prefilters,
    language: searchSession.value!.language,
    sort: searchSession.value!.sort,
  })
}

async function onDownloadZip() {
  if (abortController.signal.aborted) {
    return
  }

  await startZipDownload()
}

async function onDownloadFiles() {
  if (abortController.signal.aborted) {
    return
  }

  await startBulkDownload()
}
</script>

<template>
  <Teleport to="header">
    <NavBar>
      <template #start>
        <NavBarSearch :search-session="searchSession" @query-change="onQueryChange" />
      </template>
    </NavBar>
  </Teleport>
  <div ref="searchEl" class="pd-searchget mt-[var(--pd-navbar-offset)] w-full" :data-url="searchURL">
    <div v-if="searchSessionError || searchResultsError" class="py-1 text-center sm:py-4"
      ><i class="pd-searchget-error text-error-600">{{ t("common.status.loadingDataFailed") }}</i></div
    >

    <div v-else-if="searchSession === null" class="pd-searchget-loading py-1 text-center sm:py-4">{{ t("common.status.loading") }}</div>

    <SearchResultsFeed
      v-else-if="searchSession.view === 'feed'"
      :search-results="searchResults"
      :search-total="searchTotal"
      :search-more-than-total="searchMoreThanTotal"
      :search-session="searchSession"
      :filters="filters"
      @filter-updates="onFilterUpdates"
      @view-change="onViewChange"
      @download-zip="onDownloadZip"
      @download-files="onDownloadFiles"
      @reverse-clear="onReverseClear"
      @reverse-expand-update="onReverseExpandUpdate"
      @ids-clear="onIdsClear"
      @prefilters-clear="onPrefiltersClear"
      @sort-update="onSortUpdate"
    />

    <SearchResultsTable
      v-else-if="searchSession.view === 'table'"
      :search-results="searchResults"
      :search-total="searchTotal"
      :search-more-than-total="searchMoreThanTotal"
      :search-session="searchSession"
      :filters="filters"
      @filter-updates="onFilterUpdates"
      @view-change="onViewChange"
      @download-zip="onDownloadZip"
      @download-files="onDownloadFiles"
    />
  </div>

  <!--
    When there is an error, we do not show a component to display results which otherwise
    shows the footer. So we show the footer ourselves here in that case.
  -->
  <Teleport v-if="searchSessionError || searchResultsError" to="footer">
    <Footer class="border-t border-slate-50 bg-slate-200 shadow-sm" />
  </Teleport>

  <WithLock :lock="getOverlayLock">
    <DownloadOverlay
      :open="(downloadingPhase !== null && downloadingPhase !== 'picking') || downloadError !== null"
      :downloading-phase="downloadingPhase"
      :completed="completed"
      :total="total"
      :current-file="currentFile"
      :error="downloadError"
      @cancel="cancelDownload"
    />
  </WithLock>
</template>
