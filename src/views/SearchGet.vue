<script setup lang="ts">
import type { PrefilterPayload } from "@/search"
import type { Filter, SearchSessionData, SortKey, ViewType } from "@/types"
import type { DeepReadonly } from "vue"

import { Identifier } from "@tozd/identifier"
import { computed, onBeforeUnmount, provide, ref, toRef, useTemplateRef, watch, watchEffect } from "vue"
import { useI18n } from "vue-i18n"
import { useRouter } from "vue-router"

import { useDownload } from "@/download"
import { useNavbarSearchQuery } from "@/navbar"
import DownloadOverlay from "@/partials/DownloadOverlay.vue"
import Footer from "@/partials/Footer.vue"
import NavBar from "@/partials/NavBar.vue"
import NavBarSearch from "@/partials/NavBarSearch.vue"
import SearchResultsFeed from "@/partials/SearchResultsFeed.vue"
import SearchResultsTable from "@/partials/SearchResultsTable.vue"
import { useBusy } from "@/progress"
import { searchShortcutControllerKey, updateSearchSession, useSearch, useSearchSession } from "@/search"
import { clone } from "@/utils"

const props = defineProps<{
  id: string
}>()

const { t, locale } = useI18n({ useScope: "global" })
const router = useRouter()

// Data loading and controls for data loading.
const busy = useBusy()

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
    for (const payload of payloads) {
      const filterBase = [...searchSession.value.base, "FILTER", Identifier.new().toString()]
      const id = (await Identifier.from(...filterBase)).toString()
      prefilters.push({
        id,
        base: filterBase,
        prop: payload.prop,
        ref: {
          to: payload.to.length > 0 ? payload.to : undefined,
          direct: payload.direct.length > 0 ? payload.direct : undefined,
          missing: payload.missing ? true : undefined,
        },
      })
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
    return (!f.ref.to || f.ref.to.length === 0) && (!f.ref.direct || f.ref.direct.length === 0) && !f.ref.missing
  }
  if ("amount" in f) {
    return f.amount.gte == null && f.amount.lte == null && !f.amount.missing && !f.amount.exists
  }
  if ("time" in f) {
    return f.time.gte == null && f.time.lte == null && !f.time.missing && !f.time.exists
  }
  if ("has" in f) {
    return !f.has.props || f.has.props.length === 0
  }
  return true
}

async function onFilterUpdate(filterId: string, updatedFilter: Filter) {
  // Checking abortController is done inside onSearchSessionUpdate.

  if (isFilterEmpty(updatedFilter)) {
    // Filter has no active selection: remove it from the session.
    const updatedFilters = filters.value.filter((f) => f.id !== filterId)
    await onFiltersUpdate(updatedFilters)
  } else if (filterId && filters.value.some((f) => f.id === filterId)) {
    // Existing filter: replace it.
    const updatedFilters = filters.value.map((f) => (f.id === filterId ? updatedFilter : f))
    await onFiltersUpdate(updatedFilters)
  } else {
    // New filter: generate Base/ID and add it.
    const filterBase = [...searchSession.value!.base, "FILTER", Identifier.new().toString()]
    const id = (await Identifier.from(...filterBase)).toString()
    const newFilter = { ...updatedFilter, base: filterBase, id }
    await onFiltersUpdate([...filters.value, newFilter])
  }
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
  <div ref="searchEl" class="pd-searchget mt-[var(--pd-navbar-offset)] w-full border-t border-transparent" :data-url="searchURL">
    <div v-if="searchSessionError || searchResultsError" class="my-1 text-center sm:my-4"
      ><i class="pd-searchget-error text-error-600">{{ t("common.status.loadingDataFailed") }}</i></div
    >

    <div v-else-if="searchSession === null" class="my-1 text-center sm:my-4">{{ t("common.status.loading") }}</div>

    <SearchResultsFeed
      v-else-if="searchSession.view === 'feed'"
      :search-results="searchResults"
      :search-total="searchTotal"
      :search-more-than-total="searchMoreThanTotal"
      :search-session="searchSession"
      :filters="filters"
      :is-downloading="downloadingPhase !== null"
      @filter-update="onFilterUpdate"
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
      :is-downloading="downloadingPhase !== null"
      @filter-update="onFilterUpdate"
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

  <DownloadOverlay
    :open="(downloadingPhase !== null && downloadingPhase !== 'picking') || downloadError !== null"
    :downloading-phase="downloadingPhase"
    :completed="completed"
    :total="total"
    :current-file="currentFile"
    :error="downloadError"
    @cancel="cancelDownload"
  />
</template>
