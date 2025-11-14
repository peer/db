<script setup lang="ts">
import type {
  RelFilterState,
  AmountFilterState,
  TimeFilterState,
  StringFilterState,
  IndexFilterState,
  SizeFilterState,
  FiltersState,
  DocumentBeginEditResponse,
  DocumentCreateResponse,
  SearchResultFilterType,
  SearchViewType,
  FilterStateChange,
  AmountFilterStateChange,
  RelFilterStateChange,
  TimeFilterStateChange,
  StringFilterStateChange,
  IndexFilterStateChange,
  SizeFilterStateChange,
} from "@/types"

import { ref, toRef, onBeforeUnmount, watchEffect } from "vue"
import { useRoute, useRouter } from "vue-router"
import { ArrowUpTrayIcon, PlusIcon } from "@heroicons/vue/20/solid"
import Button from "@/components/Button.vue"
import NavBar from "@/partials/NavBar.vue"
import NavBarSearch from "@/partials/NavBarSearch.vue"
import Footer from "@/partials/Footer.vue"
import { useSearch, useSearchState, useFilters, postFilters, SEARCH_INITIAL_LIMIT, SEARCH_INCREASE, FILTERS_INITIAL_LIMIT, FILTERS_INCREASE } from "@/search"
import { postJSON } from "@/api"
import { uploadFile } from "@/upload"
import { clone, useLimitResults, encodeQuery } from "@/utils"
import { injectMainProgress, localProgress } from "@/progress"
import { AddClaimChange } from "@/document"
import SearchResultsFeed from "@/partials/SearchResultsFeed.vue"
import SearchResultsTable from "@/partials/SearchResultsTable.vue"

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
  toRef(() => {
    if (!searchState.value) {
      return ""
    }
    if (searchState.value.s !== props.s) {
      return ""
    }
    if (searchState.value.p && !searchState.value.promptDone) {
      return ""
    }
    return props.s
  }),
  searchEl,
  searchProgress,
)

const { limitedResults: limitedSearchResults, hasMore: searchHasMore, loadMore: searchLoadMore } = useLimitResults(searchResults, SEARCH_INITIAL_LIMIT, SEARCH_INCREASE)

const filtersEl = ref(null)

const filtersProgress = localProgress(mainProgress)
const {
  results: filtersResults,
  total: filtersTotal,
  error: filtersError,
  url: filtersURL,
} = useFilters(
  toRef(() => {
    if (!searchState.value) {
      return ""
    }
    if (searchState.value.s !== props.s) {
      return ""
    }
    if (searchState.value.p && !searchState.value.promptDone) {
      return ""
    }
    return props.s
  }),
  filtersEl,
  filtersProgress,
)

const {
  limitedResults: limitedFiltersResults,
  hasMore: filtersHasMore,
  loadMore: filtersLoadMore,
} = useLimitResults(filtersResults, FILTERS_INITIAL_LIMIT, FILTERS_INCREASE)

const updateFiltersProgress = localProgress(mainProgress)
// A non-read-only version of filters state so that we can modify it as necessary.
const filtersState = ref<FiltersState>({ rel: {}, amount: {}, time: {}, str: {}, index: [], size: null })
// We keep it in sync with upstream version.
watchEffect((onCleanup) => {
  // We copy to make a read-only value mutable.
  if (searchState.value === null || !searchState.value.filters) {
    filtersState.value = { rel: {}, amount: {}, time: {}, str: {}, index: [], size: null }
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

async function onIndexFiltersStateUpdate(s: IndexFilterState) {
  if (abortController.signal.aborted) {
    return
  }

  updateFiltersProgress.value += 1
  try {
    const updatedState = { ...filtersState.value }
    updatedState.index = s
    await postFilters(router, route, props.s, updatedState, abortController.signal, updateFiltersProgress)
  } catch (err) {
    if (abortController.signal.aborted) {
      return
    }
    // TODO: Show notification with error.
    console.error("Search.onIndexFiltersStateUpdate", err)
  } finally {
    updateFiltersProgress.value -= 1
  }
}

async function onSizeFiltersStateUpdate(s: SizeFilterState) {
  if (abortController.signal.aborted) {
    return
  }

  updateFiltersProgress.value += 1
  try {
    const updatedState = { ...filtersState.value }
    updatedState.size = s
    await postFilters(router, route, props.s, updatedState, abortController.signal, updateFiltersProgress)
  } catch (err) {
    if (abortController.signal.aborted) {
      return
    }
    // TODO: Show notification with error.
    console.error("Search.onSizeFiltersStateUpdate", err)
  } finally {
    updateFiltersProgress.value -= 1
  }
}

const filtersEnabled = ref(false)

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

function onFilterChange(type: SearchResultFilterType, payload: FilterStateChange) {
  switch (type) {
    case "rel": {
      const relChange = payload as RelFilterStateChange
      return onRelFiltersStateUpdate(relChange.id, relChange.value)
    }

    case "amount": {
      const amountChange = payload as AmountFilterStateChange
      return onAmountFiltersStateUpdate(amountChange.id, amountChange.unit, amountChange.value)
    }

    case "time": {
      const timeChange = payload as TimeFilterStateChange
      return onTimeFiltersStateUpdate(timeChange.id, timeChange.value)
    }

    case "string": {
      const stringChange = payload as StringFilterStateChange
      return onStringFiltersStateUpdate(stringChange.id, stringChange.value)
    }

    case "index": {
      const indexChange = payload as IndexFilterStateChange
      return onIndexFiltersStateUpdate(indexChange.value)
    }

    case "size": {
      const sizeChange = payload as SizeFilterStateChange
      return onSizeFiltersStateUpdate(sizeChange.value)
    }
  }
}
</script>

<template>
  <Teleport to="header">
    <NavBar>
      <NavBarSearch v-model:filters-enabled="filtersEnabled" :s="s" />
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
  <div class="mt-12 w-full p-1 sm:mt-[4.5rem] sm:p-4 border-t border-transparent" :data-url="searchURL">
    <div v-if="searchStateError || searchResultsError" class="my-1 sm:my-4">
      <div class="text-center text-sm">
        <i class="text-error-600">loading data failed</i>
      </div>
    </div>

    <template v-else>
      <SearchResultsFeed
        v-if="searchView === 'feed'"
        v-model:search-view="searchView"
        :limited-search-results="limitedSearchResults"
        :search-results="searchResults"
        :search-total="searchTotal"
        :s="s"
        :search-has-more="searchHasMore"
        :search-progress="searchProgress"
        :search-more-than-total="searchMoreThanTotal"
        :search-state="searchState"
        :filters-enabled="filtersEnabled"
        :filters-error="filtersError"
        :filters-url="filtersURL"
        :filters-total="filtersTotal"
        :limited-filters-results="limitedFiltersResults"
        :filters-state="filtersState"
        :filters-has-more="filtersHasMore"
        :filters-progress="filtersProgress"
        :filters-el="filtersEl"
        :update-filters-progress="updateFiltersProgress"
        @on-filter-change="onFilterChange"
        @on-more-results="searchLoadMore"
        @on-more-filters="filtersLoadMore"
      />

      <SearchResultsTable
        v-else-if="searchView === 'table'"
        v-model:search-view="searchView"
        :search-more-than-total="searchMoreThanTotal"
        :search-state="searchState"
        :search-total="searchTotal"
        :search-results="searchResults"
      />
    </template>
  </div>
  <Teleport v-if="(searchTotal !== null && searchTotal > 0 && !searchHasMore) || searchTotal === 0" to="footer">
    <Footer class="border-t border-slate-50 bg-slate-200 shadow" />
  </Teleport>
</template>
