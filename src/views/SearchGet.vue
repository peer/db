<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type {
  AmountFilterState,
  ClientSearchSession,
  DocumentBeginEditResponse,
  DocumentCreateResponse,
  DownloadFile,
  FiltersState,
  FilterStateChange,
  RefFilterState,
  TimeFilterState,
  ViewType,
} from "@/types"

import { ArrowUpTrayIcon, PlusIcon } from "@heroicons/vue/20/solid"
import { onBeforeUnmount, ref, toRef, useTemplateRef, watchEffect } from "vue"
import { useI18n } from "vue-i18n"
import { useRouter } from "vue-router"

import { postJSON } from "@/api"
import Button from "@/components/Button.vue"
import siteContext from "@/context"
import { useDownload } from "@/download"
import DownloadOverlay from "@/partials/DownloadOverlay.vue"
import Footer from "@/partials/Footer.vue"
import NavBar from "@/partials/NavBar.vue"
import NavBarSearch from "@/partials/NavBarSearch.vue"
import SearchResultsFeed from "@/partials/SearchResultsFeed.vue"
import SearchResultsTable from "@/partials/SearchResultsTable.vue"
import { getParentProgress, localProgress } from "@/progress"
import { updateSearchSession, useSearch, useSearchSession } from "@/search"
import { uploadFile } from "@/upload"
import { clone, redirectServerSide } from "@/utils"

const props = defineProps<{
  id: string
}>()

const { t } = useI18n({ useScope: "global" })
const router = useRouter()

const parentProgress = getParentProgress()
const createProgress = localProgress(parentProgress)
const uploadProgress = localProgress(parentProgress)
const updateSearchSessionProgress = localProgress(parentProgress)

const abortController = new AbortController()

const upload = useTemplateRef<HTMLInputElement>("upload")

const {
  downloadMode,
  completed,
  total,
  currentFile,
  error: downloadError,
  startZipDownload,
  startBulkDownload,
  cancelDownload,
} = useDownload(abortController, updateSearchSessionProgress)

// TODO: Replace with real file list from search results.
const testFiles: DownloadFile[] = [
  { name: "License", url: "/LICENSE.txt" },
  { name: "Notice", url: "/NOTICE.txt" },
]

onBeforeUnmount(() => {
  // Aborting the controller also tears down any active download worker via useDownload's abort listener.
  abortController.abort()
})

const searchEl = useTemplateRef<HTMLElement>("searchEl")

const searchSessionVersion = ref(0)

const searchProgress = localProgress(parentProgress)
const {
  searchSession,
  error: searchSessionError,
  url: searchURL,
} = useSearchSession(
  toRef(() => ({ id: props.id, version: searchSessionVersion.value })),
  searchProgress,
)
const { results: searchResults, total: searchTotal, moreThanTotal: searchMoreThanTotal, error: searchResultsError } = useSearch(searchSession, searchEl, searchProgress)

// A non-read-only version of filters state so that we can modify it as necessary.
const filtersState = ref<FiltersState>({ ref: {}, amount: {}, time: {} })
// We keep it in sync with upstream version.
watchEffect((onCleanup) => {
  // We copy to make a read-only value mutable.
  if (searchSession.value === null || !searchSession.value.filters) {
    filtersState.value = { ref: {}, amount: {}, time: {} }
  } else {
    filtersState.value = clone(searchSession.value.filters)
  }
})

async function onSearchSessionUpdate(updatedSearchSession: DeepReadonly<ClientSearchSession>) {
  if (abortController.signal.aborted) {
    return
  }

  updateSearchSessionProgress.value += 1
  try {
    const updatedSearchSessionRef = await updateSearchSession(router, updatedSearchSession, abortController.signal, updateSearchSessionProgress)
    if (abortController.signal.aborted || !updatedSearchSessionRef) {
      return
    }
    // We know that updatedSearchSessionRef.id is the same as searchSession.id
    // because we validated that in updateSearchSession.
    searchSessionVersion.value = updatedSearchSessionRef.version
  } catch (err) {
    if (abortController.signal.aborted) {
      return
    }
    // TODO: Show notification with error.
    console.error("SearchGet.onSearchSessionUpdate", err)
  } finally {
    updateSearchSessionProgress.value -= 1
  }
}

async function onFiltersStateUpdate(updatedFilters: FiltersState) {
  // Checking abortController is done inside onSearchSessionUpdate.

  await onSearchSessionUpdate({ ...searchSession.value!, filters: updatedFilters })
}

async function onRefFiltersStateUpdate(id: string, state: RefFilterState) {
  // Checking abortController is done inside onSearchSessionUpdate.

  const updatedFilters = { ...filtersState.value }
  updatedFilters.ref = { ...updatedFilters.ref }
  updatedFilters.ref[id] = state
  await onFiltersStateUpdate(updatedFilters)
}

async function onAmountFiltersStateUpdate(id: string, unit: string | undefined, state: AmountFilterState) {
  // Checking abortController is done inside onSearchSessionUpdate.

  const updatedFilters = { ...filtersState.value }
  updatedFilters.amount = { ...updatedFilters.amount }
  const key = unit ? `${id}/${unit}` : id
  updatedFilters.amount[key] = state
  await onFiltersStateUpdate(updatedFilters)
}

async function onTimeFiltersStateUpdate(id: string, state: TimeFilterState) {
  // Checking abortController is done inside onSearchSessionUpdate.

  const updatedFilters = { ...filtersState.value }
  updatedFilters.time = { ...updatedFilters.time }
  updatedFilters.time[id] = state
  await onFiltersStateUpdate(updatedFilters)
}

async function onCreate() {
  if (abortController.signal.aborted) {
    return
  }

  createProgress.value += 1
  try {
    const createResponse = await postJSON<DocumentCreateResponse>(
      router.apiResolve({
        name: "DocumentCreate",
      }).href,
      {},
      abortController.signal,
      createProgress,
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
      createProgress,
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
    createProgress.value -= 1
  }
}

function onUpload() {
  if (abortController.signal.aborted) {
    return
  }

  upload.value?.click()
}

async function onChange() {
  if (abortController.signal.aborted) {
    return
  }

  for (const file of upload.value?.files || []) {
    uploadProgress.value += 1
    try {
      const fileId = await uploadFile(router, file, abortController.signal, uploadProgress)
      if (abortController.signal.aborted) {
        return
      }

      redirectServerSide(router.resolve({ name: "StorageGet", params: { id: fileId } }).href, false, parentProgress)
    } catch (err) {
      if (abortController.signal.aborted) {
        return
      }
      // TODO: Show notification with error.
      console.error("SearchResults.onChange", err)
    } finally {
      uploadProgress.value -= 1
    }

    // TODO: Support uploading multiple files.
    //       Input element does not have "multiple" set, so there should be only one file.
    break
  }
}

function onFilterChange(change: FilterStateChange) {
  // Checking abortController is done inside onSearchSessionUpdate.

  switch (change.type) {
    case "ref": {
      return onRefFiltersStateUpdate(change.id, change.value)
    }

    case "amount": {
      return onAmountFiltersStateUpdate(change.id, change.unit, change.value)
    }

    case "time": {
      return onTimeFiltersStateUpdate(change.id, change.value)
    }
  }
}

async function onQueryChange(query: string) {
  // Checking abortController is done inside onSearchSessionUpdate.

  await onSearchSessionUpdate({ ...searchSession.value!, query })
}

async function onViewChange(view: ViewType) {
  // Checking abortController is done inside onSearchSessionUpdate.

  await onSearchSessionUpdate({ ...searchSession.value!, view })
}

async function onDownloadZip() {
  if (abortController.signal.aborted) {
    return
  }

  await startZipDownload(testFiles)
}

async function onDownloadFiles() {
  if (abortController.signal.aborted) {
    return
  }

  await startBulkDownload(testFiles)
}
</script>

<template>
  <Teleport to="header">
    <NavBar>
      <template #start>
        <NavBarSearch :search-session="searchSession" :update-search-session-progress="updateSearchSessionProgress" @query-change="onQueryChange" />
      </template>
      <template #end>
        <template v-if="siteContext.features.editButtons">
          <Button :progress="createProgress" type="button" primary class="px-3.5" @click.prevent="onCreate">
            <PlusIcon class="size-5 sm:hidden" :alt="t('common.buttons.create')" />
            <span class="hidden sm:inline">{{ t("common.buttons.create") }}</span>
          </Button>
          <input ref="upload" type="file" class="hidden" @change="onChange" />
          <Button :progress="uploadProgress" type="button" primary class="px-3.5" @click.prevent="onUpload">
            <ArrowUpTrayIcon class="size-5 sm:hidden" :alt="t('common.buttons.upload')" />
            <span class="hidden sm:inline">{{ t("common.buttons.upload") }}</span>
          </Button>
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
      :search-progress="searchProgress"
      :filters-state="filtersState"
      :update-search-session-progress="updateSearchSessionProgress"
      @filter-change="onFilterChange"
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
      :search-progress="searchProgress"
      :filters-state="filtersState"
      :update-search-session-progress="updateSearchSessionProgress"
      @filter-change="onFilterChange"
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
    :open="total > 0 || downloadError !== null"
    :mode="downloadMode"
    :completed="completed"
    :total="total"
    :current-file="currentFile"
    :error="downloadError"
    @cancel="cancelDownload"
  />
</template>
