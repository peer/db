<script setup lang="ts">
import type {
  RelFilterState,
  AmountFilterState,
  TimeFilterState,
  StringFilterState,
  IndexFilterState,
  SizeFilterState,
  FiltersState,
  RelSearchResult,
  AmountSearchResult,
  TimeSearchResult,
  StringSearchResult,
  IndexSearchResult,
  SizeSearchResult,
  DocumentBeginEditResponse,
  DocumentCreateResponse,
} from "@/types"

import { ref, computed, toRef, watch, onMounted, onBeforeUnmount, watchEffect } from "vue"
import { useRoute, useRouter } from "vue-router"
import { ArrowUpTrayIcon, PlusIcon } from "@heroicons/vue/20/solid"
import Button from "@/components/Button.vue"
import SearchResult from "@/partials/SearchResult.vue"
import RelFiltersResult from "@/partials/RelFiltersResult.vue"
import AmountFiltersResult from "@/partials/AmountFiltersResult.vue"
import TimeFiltersResult from "@/partials/TimeFiltersResult.vue"
import StringFiltersResult from "@/partials/StringFiltersResult.vue"
import IndexFiltersResult from "@/partials/IndexFiltersResult.vue"
import SizeFiltersResult from "@/partials/SizeFiltersResult.vue"
import NavBar from "@/partials/NavBar.vue"
import NavBarSearch from "@/partials/NavBarSearch.vue"
import Footer from "@/partials/Footer.vue"
import { useSearch, useFilters, postFilters, SEARCH_INITIAL_LIMIT, SEARCH_INCREASE, FILTERS_INITIAL_LIMIT, FILTERS_INCREASE } from "@/search"
import { useVisibilityTracking } from "@/visibility"
import { postJSON } from "@/api"
import { uploadFile } from "@/upload"
import { clone, useLimitResults, encodeQuery } from "@/utils"
import { injectMainProgress, localProgress } from "@/progress"
import { AddClaimChange } from "@/document"

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

const searchProgress = localProgress(mainProgress)
const {
  results: searchResults,
  total: searchTotal,
  filters: searchFilters,
  moreThanTotal: searchMoreThanTotal,
  error: searchError,
  url: searchURL,
} = useSearch(
  toRef(() => props.s),
  searchEl,
  searchProgress,
  async (searchState) => {
    await router.replace({
      name: "SearchResults",
      params: {
        s: searchState.s,
      },
      // Maybe route.query has non-empty "at" parameter which we want to keep.
      query: encodeQuery({ q: searchState.q, at: route.query.at || undefined }),
    })
  },
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
  toRef(() => props.s),
  filtersEl,
  filtersProgress,
)

const {
  limitedResults: limitedFiltersResults,
  hasMore: filtersHasMore,
  loadMore: filtersLoadMore,
} = useLimitResults(filtersResults, FILTERS_INITIAL_LIMIT, FILTERS_INCREASE)

const idToIndex = computed(() => {
  const map = new Map<string, number>()
  for (const [i, result] of searchResults.value.entries()) {
    map.set(result.id, i)
  }
  return map
})

const { track, visibles } = useVisibilityTracking()

const initialRouteName = route.name
watch(
  () => {
    const sorted = Array.from(visibles)
    sorted.sort((a, b) => (idToIndex.value.get(a) ?? Infinity) - (idToIndex.value.get(b) ?? Infinity))
    return sorted[0]
  },
  async (topId, oldTopId, onCleanup) => {
    // Watch can continue to run for some time after the route changes.
    if (initialRouteName !== route.name) {
      return
    }
    // Initial data has not yet been loaded, so we wait.
    if (!topId && searchTotal.value === null) {
      return
    }
    await router.replace({
      name: route.name as string,
      params: route.params,
      // We do not want to set an empty "at" query parameter.
      query: encodeQuery({ ...route.query, at: topId || undefined }),
      hash: route.hash,
    })
  },
  {
    immediate: true,
  },
)

const searchMoreButton = ref()
const filtersMoreButton = ref()
const supportPageOffset = window.pageYOffset !== undefined

function onScroll() {
  if (abortController.signal.aborted) {
    return
  }
  if (!searchMoreButton.value && !filtersMoreButton.value) {
    return
  }

  const viewportHeight = document.documentElement.clientHeight || document.body.clientHeight
  const scrollHeight = Math.max(
    document.body.scrollHeight,
    document.documentElement.scrollHeight,
    document.body.offsetHeight,
    document.documentElement.offsetHeight,
    document.body.clientHeight,
    document.documentElement.clientHeight,
  )
  const currentScrollPosition = supportPageOffset ? window.pageYOffset : document.documentElement.scrollTop
  if (currentScrollPosition > scrollHeight - 2 * viewportHeight) {
    if (searchMoreButton.value) {
      searchMoreButton.value.$el.click()
    }
    if (filtersMoreButton.value) {
      filtersMoreButton.value.$el.click()
    }
  }
}

onMounted(() => {
  window.addEventListener("scroll", onScroll, { passive: true })
})

onBeforeUnmount(() => {
  window.removeEventListener("scroll", onScroll)
})

const updateFiltersProgress = localProgress(mainProgress)
// A non-read-only version of filters state so that we can modify it as necessary.
const filtersState = ref<FiltersState>({ rel: {}, amount: {}, time: {}, str: {}, index: [], size: null })
// We keep it in sync with upstream version.
watchEffect((onCleanup) => {
  // We copy to make a read-only value mutable.
  filtersState.value = clone(searchFilters.value)
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
</script>

<template>
  <Teleport to="header">
    <NavBar>
      <NavBarSearch v-model:filtersEnabled="filtersEnabled" :s="s" />
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
  <div class="mt-12 flex w-full gap-x-1 border-t border-transparent p-1 sm:mt-[4.5rem] sm:gap-x-4 sm:p-4">
    <div ref="searchEl" class="flex-auto basis-3/4 flex-col gap-y-1 sm:flex sm:gap-y-4" :class="filtersEnabled ? 'hidden' : 'flex'" :data-url="searchURL">
      <div v-if="searchError" class="my-1 sm:my-4">
        <div class="text-center text-sm"><i class="text-error-600">loading data failed</i></div>
      </div>
      <div v-else-if="searchTotal === null" class="my-1 sm:my-4">
        <div class="text-center text-sm">Searching...</div>
      </div>
      <div v-else-if="searchTotal === 0" class="my-1 sm:my-4">
        <div class="text-center text-sm">No results found.</div>
      </div>
      <template v-else-if="searchTotal > 0">
        <template v-for="(result, i) in limitedSearchResults" :key="result.id">
          <div v-if="i === 0 && searchMoreThanTotal" class="my-1 sm:my-4">
            <div class="text-center text-sm">Showing first {{ searchResults.length }} of more than {{ searchTotal }} results found.</div>
            <div class="h-2 w-full bg-slate-200"></div>
          </div>
          <div v-if="i === 0 && searchResults.length < searchTotal && !searchMoreThanTotal" class="my-1 sm:my-4">
            <div class="text-center text-sm">Showing first {{ searchResults.length }} of {{ searchTotal }} results found.</div>
            <div class="h-2 w-full bg-slate-200"></div>
          </div>
          <div v-if="i === 0 && searchResults.length == searchTotal && !searchMoreThanTotal" class="my-1 sm:my-4">
            <div class="text-center text-sm">Found {{ searchTotal }} results.</div>
            <div class="h-2 w-full bg-slate-200"></div>
          </div>
          <div v-else-if="i > 0 && i % 10 === 0" class="my-1 sm:my-4">
            <div v-if="searchResults.length < searchTotal" class="text-center text-sm">{{ i }} of {{ searchResults.length }} shown results.</div>
            <div v-else-if="searchResults.length == searchTotal" class="text-center text-sm">{{ i }} of {{ searchResults.length }} results.</div>
            <div class="relative h-2 w-full bg-slate-200">
              <div class="absolute inset-y-0 bg-secondary-400" style="left: 0" :style="{ width: (i / searchResults.length) * 100 + '%' }"></div>
            </div>
          </div>
          <SearchResult :ref="track(result.id) as any" :s="s" :result="result" />
        </template>
        <Button v-if="searchHasMore" ref="searchMoreButton" :progress="searchProgress" primary class="w-1/4 min-w-fit self-center" @click="searchLoadMore"
          >Load more</Button
        >
        <div v-else class="my-1 sm:my-4">
          <div v-if="searchMoreThanTotal" class="text-center text-sm">All of first {{ searchResults.length }} shown of more than {{ searchTotal }} results found.</div>
          <div v-else-if="searchResults.length < searchTotal && !searchMoreThanTotal" class="text-center text-sm">
            All of first {{ searchResults.length }} shown of {{ searchTotal }} results found.
          </div>
          <div v-else-if="searchResults.length === searchTotal && !searchMoreThanTotal" class="text-center text-sm">All of {{ searchResults.length }} results shown.</div>
          <div class="relative h-2 w-full bg-slate-200">
            <div class="absolute inset-y-0 bg-secondary-400" style="left: 0" :style="{ width: 100 + '%' }"></div>
          </div>
        </div>
      </template>
    </div>
    <div ref="filtersEl" class="flex-auto basis-1/4 flex-col gap-y-1 sm:flex sm:gap-y-4" :class="filtersEnabled ? 'flex' : 'hidden'" :data-url="filtersURL">
      <div v-if="searchError || filtersError" class="my-1 sm:my-4">
        <div class="text-center text-sm"><i class="text-error-600">loading data failed</i></div>
      </div>
      <div v-else-if="searchTotal === null || filtersTotal === null" class="my-1 sm:my-4">
        <div class="text-center text-sm">Determining filters...</div>
      </div>
      <div v-else-if="filtersTotal === 0" class="my-1 sm:my-4">
        <div class="text-center text-sm">No filters available.</div>
      </div>
      <template v-else-if="filtersTotal > 0">
        <div class="text-center text-sm">{{ filtersTotal }} filters available.</div>
        <template v-for="result in limitedFiltersResults" :key="result.id">
          <RelFiltersResult
            v-if="result.type === 'rel'"
            :s="s"
            :search-total="searchTotal"
            :result="result as RelSearchResult"
            :state="filtersState.rel[result.id] || (filtersState.rel[result.id] = [])"
            :update-progress="updateFiltersProgress"
            @update:state="onRelFiltersStateUpdate(result.id, $event)"
          />
          <AmountFiltersResult
            v-if="result.type === 'amount'"
            :s="s"
            :search-total="searchTotal"
            :result="result as AmountSearchResult"
            :state="filtersState.amount[`${result.id}/${result.unit}`] || (filtersState.amount[`${result.id}/${result.unit}`] = null)"
            :update-progress="updateFiltersProgress"
            @update:state="onAmountFiltersStateUpdate(result.id, result.unit, $event)"
          />
          <TimeFiltersResult
            v-if="result.type === 'time'"
            :s="s"
            :search-total="searchTotal"
            :result="result as TimeSearchResult"
            :state="filtersState.time[result.id] || (filtersState.time[result.id] = null)"
            :update-progress="updateFiltersProgress"
            @update:state="onTimeFiltersStateUpdate(result.id, $event)"
          />
          <StringFiltersResult
            v-if="result.type === 'string'"
            :s="s"
            :search-total="searchTotal"
            :result="result as StringSearchResult"
            :state="filtersState.str[result.id] || (filtersState.str[result.id] = [])"
            :update-progress="updateFiltersProgress"
            @update:state="onStringFiltersStateUpdate(result.id, $event)"
          />
          <IndexFiltersResult
            v-if="result.type === 'index'"
            :s="s"
            :search-total="searchTotal"
            :result="result as IndexSearchResult"
            :state="filtersState.index"
            :update-progress="updateFiltersProgress"
            @update:state="onIndexFiltersStateUpdate($event)"
          />
          <SizeFiltersResult
            v-if="result.type === 'size'"
            :s="s"
            :search-total="searchTotal"
            :result="result as SizeSearchResult"
            :state="filtersState.size"
            :update-progress="updateFiltersProgress"
            @update:state="onSizeFiltersStateUpdate($event)"
          />
        </template>
        <Button v-if="filtersHasMore" ref="filtersMoreButton" :progress="filtersProgress" primary class="w-1/2 min-w-fit self-center" @click="filtersLoadMore"
          >More filters</Button
        >
        <div v-else-if="filtersTotal > limitedFiltersResults.length" class="text-center text-sm">
          {{ filtersTotal - limitedFiltersResults.length }} filters not shown.
        </div>
      </template>
    </div>
  </div>
  <Teleport v-if="(searchTotal !== null && searchTotal > 0 && !searchHasMore) || searchTotal === 0" to="footer">
    <Footer class="border-t border-slate-50 bg-slate-200 shadow" />
  </Teleport>
</template>
