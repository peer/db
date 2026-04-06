<script setup lang="ts">
import type { DocumentBeginEditResponse, DocumentCreateResponse, Filter, SearchSessionData, ViewType } from "@/types"
import type { DeepReadonly } from "vue"

import { ArrowUpTrayIcon, PlusIcon } from "@heroicons/vue/20/solid"
import { Identifier } from "@tozd/identifier"
import { onBeforeUnmount, ref, toRef, useTemplateRef, watchEffect } from "vue"
import { useI18n } from "vue-i18n"
import { useRouter } from "vue-router"

import { postJSON } from "@/api"
import Button from "@/components/Button.vue"
import WithLock from "@/components/WithLock.vue"
import siteContext from "@/context"
import { useDownload } from "@/download"
import DownloadOverlay from "@/partials/DownloadOverlay.vue"
import Footer from "@/partials/Footer.vue"
import NavBar from "@/partials/NavBar.vue"
import NavBarSearch from "@/partials/NavBarSearch.vue"
import SearchResultsFeed from "@/partials/SearchResultsFeed.vue"
import SearchResultsTable from "@/partials/SearchResultsTable.vue"
import { getParentLock, localCounter, lockScope, useBusy } from "@/progress"
import { updateSearchSession, useSearch, useSearchSession } from "@/search"
import { uploadFile } from "@/upload"
import { clone, redirectServerSide } from "@/utils"

const props = defineProps<{
  id: string
}>()

const { t } = useI18n({ useScope: "global" })
const router = useRouter()

// Data loading and controls for data loading.
const busy = useBusy()

// Independent sub-scopes for the Create and Upload buttons.
// getParentLock here reads from the ancestor's provides (above SearchGet).
// The *Busy refs are the writable handles used in handlers and as the
// button's :progress visual: writes update a local counter (for the
// visual, isolated from any ancestor lock contributions) and propagate
// into the lockScope for descendant cascade.
const createLock = lockScope(getParentLock())
const uploadLock = lockScope(getParentLock())
const createBusy = localCounter(createLock)
const uploadBusy = localCounter(uploadLock)
function getCreateLock() {
  return createLock
}
function getUploadLock() {
  return uploadLock
}

const abortController = new AbortController()

onBeforeUnmount(() => {
  // Aborting the controller also tears down any active download worker via useDownload's abort listener.
  abortController.abort()
})

const uploadEl = useTemplateRef<HTMLInputElement>("uploadEl")
const searchEl = useTemplateRef<HTMLElement>("searchEl")

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

async function onFiltersUpdate(updatedFilters: Filter[]) {
  // Checking abortController is done inside onSearchSessionUpdate.

  await onSearchSessionUpdate({
    view: searchSession.value!.view,
    query: searchSession.value!.query,
    filters: updatedFilters.length > 0 ? updatedFilters : undefined,
  })
}

// isFilterEmpty returns true if the filter has no active selection.
function isFilterEmpty(f: Filter): boolean {
  if ("ref" in f) {
    return (!f.ref.to || f.ref.to.length === 0) && !f.ref.missing
  }
  if ("amount" in f) {
    return f.amount.gte == null && f.amount.lte == null && !f.amount.missing
  }
  if ("time" in f) {
    return f.time.gte == null && f.time.lte == null && !f.time.missing
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

async function onCreate() {
  if (abortController.signal.aborted) {
    return
  }

  createBusy.value += 1
  try {
    const createResponse = await postJSON<DocumentCreateResponse>(
      router.apiResolve({
        name: "DocumentCreate",
      }).href,
      {},
      abortController.signal,
      createBusy,
    )
    if (abortController.signal.aborted) {
      return
    }
    const editResponse = await postJSON<DocumentBeginEditResponse>(
      router.apiResolve({
        name: "DocumentBeginEdit",
        params: {
          id: createResponse.id,
        },
      }).href,
      {},
      abortController.signal,
      createBusy,
    )
    if (abortController.signal.aborted) {
      return
    }
    await router.push({
      name: "DocumentEdit",
      params: {
        id: createResponse.id,
        session: editResponse.session,
      },
    })
  } catch (err) {
    if (abortController.signal.aborted) {
      return
    }
    // TODO: Show notification with error.
    console.error("SearchResults.onCreate", err)
  } finally {
    createBusy.value -= 1
  }
}

function onUpload() {
  if (abortController.signal.aborted) {
    return
  }

  uploadEl.value?.click()
}

async function onChange() {
  if (abortController.signal.aborted) {
    return
  }

  for (const file of uploadEl.value?.files || []) {
    uploadBusy.value += 1
    try {
      const fileId = await uploadFile(router, file, abortController.signal, uploadBusy, null)
      if (abortController.signal.aborted) {
        return
      }

      // We pass busy so that redirectServerSide uses it to locks all controls.
      redirectServerSide(router.resolve({ name: "StorageGet", params: { id: fileId } }).href, false, busy)
    } catch (err) {
      if (abortController.signal.aborted) {
        return
      }
      // TODO: Show notification with error.
      console.error("SearchResults.onChange", err)
    } finally {
      uploadBusy.value -= 1
    }

    // TODO: Support uploading multiple files.
    //       Input element does not have "multiple" set, so there should be only one file.
    break
  }
}

async function onQueryChange(query: string) {
  // Checking abortController is done inside onSearchSessionUpdate.

  await onSearchSessionUpdate({
    view: searchSession.value!.view,
    query,
    filters: searchSession.value!.filters,
  })
}

async function onViewChange(view: ViewType) {
  // Checking abortController is done inside onSearchSessionUpdate.

  await onSearchSessionUpdate({
    view,
    query: searchSession.value!.query,
    filters: searchSession.value!.filters,
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
      <template #end>
        <template v-if="siteContext.features.editButtons">
          <WithLock :lock="getCreateLock">
            <Button :progress="createBusy" type="button" primary class="px-3.5" @click.prevent="onCreate">
              <PlusIcon class="size-5 sm:hidden" :alt="t('common.buttons.create')" />
              <span class="hidden sm:inline">{{ t("common.buttons.create") }}</span>
            </Button>
          </WithLock>
          <WithLock :lock="getUploadLock">
            <input ref="uploadEl" type="file" class="hidden" @change="onChange" />
            <Button :progress="uploadBusy" type="button" primary class="px-3.5" @click.prevent="onUpload">
              <ArrowUpTrayIcon class="size-5 sm:hidden" :alt="t('common.buttons.upload')" />
              <span class="hidden sm:inline">{{ t("common.buttons.upload") }}</span>
            </Button>
          </WithLock>
        </template>
      </template>
    </NavBar>
  </Teleport>
  <div ref="searchEl" class="pd-searchget mt-12 w-full border-t border-transparent sm:mt-[4.5rem]" :data-url="searchURL">
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
