<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type {
  AmountFilterState,
  ClientSearchSession,
  DocumentBeginEditResponse,
  DocumentCreateResponse,
  FiltersState,
  FilterStateChange,
  RelFilterState,
  StringFilterState,
  TimeFilterState,
  ViewType,
} from "@/types"

import { ArrowUpTrayIcon, PlusIcon } from "@heroicons/vue/20/solid"
import { onBeforeUnmount, ref, toRef, useTemplateRef, watchEffect } from "vue"
import { useRouter } from "vue-router"

import { postJSON } from "@/api"
import Button from "@/components/Button.vue"
import { AddClaimChange } from "@/document"
import Footer from "@/partials/Footer.vue"
import NavBar from "@/partials/NavBar.vue"
import NavBarSearch from "@/partials/NavBarSearch.vue"
import SearchResultsFeed from "@/partials/SearchResultsFeed.vue"
import SearchResultsTable from "@/partials/SearchResultsTable.vue"
import { injectMainProgress, localProgress } from "@/progress"
import { updateSearchSession, useSearch, useSearchSession } from "@/search"
import { uploadFile } from "@/upload"
import { clone, encodeQuery } from "@/utils"

const props = defineProps<{
  id: string
}>()

const router = useRouter()

const mainProgress = injectMainProgress()
const createProgress = localProgress(mainProgress)
const uploadProgress = localProgress(mainProgress)

const abortController = new AbortController()

const upload = useTemplateRef<HTMLInputElement>("upload")

onBeforeUnmount(() => {
  abortController.abort()
})

const searchEl = useTemplateRef<HTMLElement>("searchEl")

const searchSessionVersion = ref(0)

const searchProgress = localProgress(mainProgress)
const {
  searchSession,
  error: searchSessionError,
  url: searchURL,
} = useSearchSession(
  toRef(() => ({ id: props.id, version: searchSessionVersion.value })),
  searchProgress,
)
const { results: searchResults, total: searchTotal, moreThanTotal: searchMoreThanTotal, error: searchResultsError } = useSearch(searchSession, searchEl, searchProgress)

const updateSearchSessionProgress = localProgress(mainProgress)

// A non-read-only version of filters state so that we can modify it as necessary.
const filtersState = ref<FiltersState>({ rel: {}, amount: {}, time: {}, str: {} })
// We keep it in sync with upstream version.
watchEffect((onCleanup) => {
  // We copy to make a read-only value mutable.
  if (searchSession.value === null || !searchSession.value.filters) {
    filtersState.value = { rel: {}, amount: {}, time: {}, str: {} }
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

async function onRelFiltersStateUpdate(id: string, state: RelFilterState) {
  // Checking abortController is done inside onSearchSessionUpdate.

  const updatedFilters = { ...filtersState.value }
  updatedFilters.rel = { ...updatedFilters.rel }
  updatedFilters.rel[id] = state
  await onFiltersStateUpdate(updatedFilters)
}

async function onAmountFiltersStateUpdate(id: string, unit: string, state: AmountFilterState) {
  // Checking abortController is done inside onSearchSessionUpdate.

  const updatedFilters = { ...filtersState.value }
  updatedFilters.amount = { ...updatedFilters.amount }
  updatedFilters.amount[`${id}/${unit}`] = state
  await onFiltersStateUpdate(updatedFilters)
}

async function onTimeFiltersStateUpdate(id: string, state: TimeFilterState) {
  // Checking abortController is done inside onSearchSessionUpdate.

  const updatedFilters = { ...filtersState.value }
  updatedFilters.time = { ...updatedFilters.time }
  updatedFilters.time[id] = state
  await onFiltersStateUpdate(updatedFilters)
}

async function onStringFiltersStateUpdate(id: string, state: StringFilterState) {
  // Checking abortController is done inside onSearchSessionUpdate.

  const updatedFilters = { ...filtersState.value }
  updatedFilters.str = { ...updatedFilters.str }
  updatedFilters.str[id] = state
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

async function onUpload() {
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

      const createResponse = await postJSON<DocumentCreateResponse>(
        router.apiResolve({
          name: "DocumentCreate",
        }).href,
        {},
        abortController.signal,
        uploadProgress,
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
        uploadProgress,
      )
      if (abortController.signal.aborted) {
        return
      }

      await postJSON(
        router.apiResolve({
          name: "DocumentSaveChange",
          params: {
            session: editResponse.session,
          },
          query: encodeQuery({ change: String(1) }),
        }).href,
        new AddClaimChange({
          patch: {
            type: "rel",
            prop: "CAfaL1ZZs6L4uyFdrJZ2wN", // TYPE.
            to: "7m6uUqF9ZnimT4sw3W8zdg", // FILE.
          },
        }),
        abortController.signal,
        null,
      )
      if (abortController.signal.aborted) {
        return
      }

      await postJSON(
        router.apiResolve({
          name: "DocumentSaveChange",
          params: {
            session: editResponse.session,
          },
          query: encodeQuery({ change: String(2) }),
        }).href,
        new AddClaimChange({
          patch: {
            type: "string",
            prop: "GUjybqSkBqwfUTZNTw4vWE", // MEDIA_TYPE.
            string: file.type || "application/octet-stream",
          },
        }),
        abortController.signal,
        null,
      )
      if (abortController.signal.aborted) {
        return
      }

      await postJSON(
        router.apiResolve({
          name: "DocumentSaveChange",
          params: {
            session: editResponse.session,
          },
          query: encodeQuery({ change: String(3) }),
        }).href,
        new AddClaimChange({
          patch: {
            type: "ref",
            prop: "9tssq1syFPE7S7vYEDTPiF", // FILE_URL.
            iri: router.resolve({
              name: "StorageGet",
              params: {
                id: fileId,
              },
            }).href,
          },
        }),
        abortController.signal,
        null,
      )
      if (abortController.signal.aborted) {
        return
      }

      if (file.name) {
        await postJSON(
          router.apiResolve({
            name: "DocumentSaveChange",
            params: {
              session: editResponse.session,
            },
            query: encodeQuery({ change: String(4) }),
          }).href,
          new AddClaimChange({
            patch: {
              type: "text",
              prop: "CjZig63YSyvb2KdyCL3XTg", // NAME.
              html: {
                en: file.name,
              },
            },
          }),
          abortController.signal,
          null,
        )
        if (abortController.signal.aborted) {
          return
        }
      }

      await postJSON(
        router.apiResolve({
          name: "DocumentEndEdit",
          params: {
            session: editResponse.session,
          },
        }).href,
        {},
        abortController.signal,
        null,
      )
      if (abortController.signal.aborted) {
        return
      }

      await router.push({
        name: "DocumentGet",
        params: {
          id: createResponse.id,
        },
      })
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
    case "rel": {
      return onRelFiltersStateUpdate(change.id, change.value)
    }

    case "amount": {
      return onAmountFiltersStateUpdate(change.id, change.unit, change.value)
    }

    case "time": {
      return onTimeFiltersStateUpdate(change.id, change.value)
    }

    case "string": {
      return onStringFiltersStateUpdate(change.id, change.value)
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
</script>

<template>
  <Teleport to="header">
    <NavBar>
      <NavBarSearch :search-session="searchSession" :update-search-session-progress="updateSearchSessionProgress" @query-change="onQueryChange" />
      <Button :progress="createProgress" type="button" primary class="!px-3.5" @click.prevent="onCreate">
        <PlusIcon class="h-5 w-5 sm:hidden" alt="Create" />
        <span class="hidden sm:inline">Create</span>
      </Button>
      <input ref="upload" type="file" class="hidden" @change="onChange" />
      <Button :progress="uploadProgress" type="button" primary class="!px-3.5" @click.prevent="onUpload">
        <ArrowUpTrayIcon class="h-5 w-5 sm:hidden" alt="Upload" />
        <span class="hidden sm:inline">Upload</span>
      </Button>
    </NavBar>
  </Teleport>
  <div ref="searchEl" class="mt-12 w-full border-t border-transparent sm:mt-[4.5rem]" :data-url="searchURL">
    <div v-if="searchSessionError || searchResultsError" class="my-1 text-center sm:my-4"><i class="text-error-600">loading data failed</i></div>

    <div v-else-if="searchSession === null" class="my-1 text-center sm:my-4">Loading...</div>

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
    />

    <SearchResultsTable
      v-else-if="searchSession.view === 'table'"
      :search-results="searchResults"
      :search-total="searchTotal"
      :search-more-than-total="searchMoreThanTotal"
      :search-session="searchSession"
      :search-progress="searchProgress"
      @view-change="onViewChange"
    />
  </div>

  <!--
    When there is an error, we do not show a component to display results which otherwise
    shows the footer. So we show the footer ourselves here in that case.
  -->
  <Teleport v-if="searchSessionError || searchResultsError" to="footer">
    <Footer class="border-t border-slate-50 bg-slate-200 shadow" />
  </Teleport>
</template>
