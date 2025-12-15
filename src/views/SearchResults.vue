<script setup lang="ts">
import type {
  RelFilterState,
  AmountFilterState,
  TimeFilterState,
  StringFilterState,
  FiltersState,
  DocumentBeginEditResponse,
  DocumentCreateResponse,
  SearchViewType,
  FilterStateChange,
} from "@/types"

import { ref, toRef, onBeforeUnmount, watchEffect } from "vue"
import { useRoute, useRouter } from "vue-router"
import { ArrowUpTrayIcon, PlusIcon } from "@heroicons/vue/20/solid"

import Button from "@/components/Button.vue"
import NavBar from "@/partials/NavBar.vue"
import NavBarSearch from "@/partials/NavBarSearch.vue"
import { useSearch, useSearchState, postFilters, activeSearchState } from "@/search"
import { postJSON } from "@/api"
import { uploadFile } from "@/upload"
import { clone, encodeQuery } from "@/utils"
import { injectMainProgress, localProgress } from "@/progress"
import { AddClaimChange } from "@/document"
import SearchResultsFeed from "@/partials/SearchResultsFeed.vue"
import SearchResultsTable from "@/partials/SearchResultsTable.vue"
import Footer from "@/partials/Footer.vue"

const props = defineProps<{
  s: string
}>()

const router = useRouter()
const route = useRoute()

const mainProgress = injectMainProgress()
const createProgress = localProgress(mainProgress)
const uploadProgress = localProgress(mainProgress)

const abortController = new AbortController()

const upload = ref<HTMLInputElement>()

onBeforeUnmount(() => {
  abortController.abort()
})

const searchEl = ref(null)

const searchView = ref<SearchViewType>("feed")

const searchProgress = localProgress(mainProgress)
const {
  searchState,
  error: searchStateError,
  url: searchURL,
} = useSearchState(
  toRef(() => props.s),
  searchProgress,
)
const {
  results: searchResults,
  total: searchTotal,
  moreThanTotal: searchMoreThanTotal,
  error: searchResultsError,
} = useSearch(
  activeSearchState(
    searchState,
    toRef(() => props.s),
  ),
  searchEl,
  searchProgress,
)

const updateFiltersProgress = localProgress(mainProgress)
// A non-read-only version of filters state so that we can modify it as necessary.
const filtersState = ref<FiltersState>({ rel: {}, amount: {}, time: {}, str: {} })
// We keep it in sync with upstream version.
watchEffect((onCleanup) => {
  // We copy to make a read-only value mutable.
  if (searchState.value === null || !searchState.value.filters) {
    filtersState.value = { rel: {}, amount: {}, time: {}, str: {} }
  } else {
    filtersState.value = clone(searchState.value.filters)
  }
})

async function onRelFiltersStateUpdate(id: string, s: RelFilterState) {
  if (abortController.signal.aborted) {
    return
  }

  updateFiltersProgress.value += 1
  try {
    const updatedState = { ...filtersState.value }
    updatedState.rel = { ...updatedState.rel }
    updatedState.rel[id] = s
    await postFilters(router, route, props.s, updatedState, abortController.signal, updateFiltersProgress)
  } catch (err) {
    if (abortController.signal.aborted) {
      return
    }
    // TODO: Show notification with error.
    console.error("Search.onRelFiltersStateUpdate", err)
  } finally {
    updateFiltersProgress.value -= 1
  }
}

async function onAmountFiltersStateUpdate(id: string, unit: string, s: AmountFilterState) {
  if (abortController.signal.aborted) {
    return
  }

  updateFiltersProgress.value += 1
  try {
    const updatedState = { ...filtersState.value }
    updatedState.amount = { ...updatedState.amount }
    updatedState.amount[`${id}/${unit}`] = s
    await postFilters(router, route, props.s, updatedState, abortController.signal, updateFiltersProgress)
  } catch (err) {
    if (abortController.signal.aborted) {
      return
    }
    // TODO: Show notification with error.
    console.error("Search.onAmountFiltersStateUpdate", err)
  } finally {
    updateFiltersProgress.value -= 1
  }
}

async function onTimeFiltersStateUpdate(id: string, s: TimeFilterState) {
  if (abortController.signal.aborted) {
    return
  }

  updateFiltersProgress.value += 1
  try {
    const updatedState = { ...filtersState.value }
    updatedState.time = { ...updatedState.time }
    updatedState.time[id] = s
    await postFilters(router, route, props.s, updatedState, abortController.signal, updateFiltersProgress)
  } catch (err) {
    if (abortController.signal.aborted) {
      return
    }
    // TODO: Show notification with error.
    console.error("Search.onTimeFiltersStateUpdate", err)
  } finally {
    updateFiltersProgress.value -= 1
  }
}

async function onStringFiltersStateUpdate(id: string, s: StringFilterState) {
  if (abortController.signal.aborted) {
    return
  }

  updateFiltersProgress.value += 1
  try {
    const updatedState = { ...filtersState.value }
    updatedState.str = { ...updatedState.str }
    updatedState.str[id] = s
    await postFilters(router, route, props.s, updatedState, abortController.signal, updateFiltersProgress)
  } catch (err) {
    if (abortController.signal.aborted) {
      return
    }
    // TODO: Show notification with error.
    console.error("Search.onStringFiltersStateUpdate", err)
  } finally {
    updateFiltersProgress.value -= 1
  }
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
</script>

<template>
  <Teleport to="header">
    <NavBar>
      <NavBarSearch :s="s" />
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
  <div ref="searchEl" class="mt-12 border-t border-transparent sm:mt-[4.5rem] w-full" :data-url="searchURL">
    <div v-if="searchStateError || searchResultsError" class="my-1 sm:my-4">
      <div class="text-center text-sm"><i class="text-error-600">loading data failed</i></div>
    </div>

    <SearchResultsFeed
      v-else-if="searchView === 'feed'"
      v-model:search-view="searchView"
      :s="s"
      :search-results="searchResults"
      :search-total="searchTotal"
      :search-more-than-total="searchMoreThanTotal"
      :search-state="searchState"
      :search-progress="searchProgress"
      :filters-state="filtersState"
      :update-filters-progress="updateFiltersProgress"
      @on-filter-change="onFilterChange"
    />

    <SearchResultsTable
      v-else-if="searchView === 'table'"
      v-model:search-view="searchView"
      :s="s"
      :search-results="searchResults"
      :search-total="searchTotal"
      :search-more-than-total="searchMoreThanTotal"
      :search-state="searchState"
      :search-progress="searchProgress"
    />
  </div>

  <!--
    When there is an error, we do not show a component to display results which otherwise
    shows the footer. So we show the footer ourselves here in that case.
  -->
  <Teleport v-if="searchStateError || searchResultsError" to="footer">
    <Footer class="border-t border-slate-50 bg-slate-200 shadow" />
  </Teleport>
</template>
