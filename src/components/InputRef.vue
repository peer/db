<script setup lang="ts">
import type { SearchResult, SearchStateCreateResponse } from "@/types"
import type { PeerDBDocument } from "@/document.ts"

import { Combobox, ComboboxInput, ComboboxOption, ComboboxOptions } from "@headlessui/vue"
import { computed, onBeforeUnmount, ref, toRef, watch } from "vue"
import { useRouter } from "vue-router"
import { debounce } from "lodash-es"

import WithDocument from "@/components/WithDocument.vue"
import { getURL, postURL } from "@/api.ts"
import { getName, loadingWidth } from "@/utils.ts"
import { activeSearchState, useSearch, useSearchState } from "@/search.ts"
import { injectMainProgress, localProgress } from "@/progress.ts"

// We want all fallthrough attributes to be passed to the link element.
defineOptions({
  inheritAttrs: false,
})

const props = withDefaults(defineProps<{ modelValue?: string; progress?: number }>(), { modelValue: "", progress: 0 })

const emit = defineEmits<{
  (e: "update:modelValue", id: string): void
}>()

const router = useRouter()

const DEBOUNCE_MS = 500

const abortController = new AbortController()

const WithPeerDBDocument = WithDocument<PeerDBDocument>

const s = ref()
const searchEl = ref(null)

const selectedDocument = ref<(SearchResult & { name: string }) | null>(null)
const nameCache = ref<Record<string, string>>({})

watch(
  () => props.modelValue,
  async (id) => {
    if (!id) return (selectedDocument.value = null)

    if (!nameCache.value[id]) {
      nameCache.value[id] = await resolveDocumentName(id)
    }

    selectedDocument.value = {
      id,
      name: nameCache.value[id],
    }
  },
)

watch(selectedDocument, (doc) => {
  if (!doc) return
  emit("update:modelValue", doc.id)
})

const isInProgress = computed(() => props.progress > 0 || searchProgress.value > 0)

const query = computed({
  get: () => props.modelValue,
  set: async (value) => {
    runSearchDebounce.cancel()
    await runSearchDebounce(value)
  },
})

const mainProgress = injectMainProgress()
const searchProgress = localProgress(mainProgress)

const {
  searchState,
  error: searchStateError,
  url: searchURL,
} = useSearchState(
  toRef(() => s.value),
  searchProgress,
)
const { results: searchResults, error: searchResultsError } = useSearch(
  activeSearchState(
    searchState,
    toRef(() => s.value),
  ),
  searchEl,
  searchProgress,
)

onBeforeUnmount(() => {
  abortController.abort()
})

const runSearchDebounce = debounce(async (q: string) => {
  await search(q)
}, DEBOUNCE_MS)

async function search(q: string) {
  const form = new FormData()
  form.set("q", q)
  form.set("filters", '{"and":[{"rel":{"prop":"CAfaL1ZZs6L4uyFdrJZ2wN","value":"8z5YTfJAd2c23dd5WFv4R5"}}]}')

  const searchState = await postURL<SearchStateCreateResponse>(
    router.apiResolve({
      name: "SearchCreate",
    }).href,
    form,
    abortController.signal,
    searchProgress,
  )

  s.value = searchState.s
}

async function resolveDocumentName(id: string): Promise<string> {
  const newURL = router.apiResolve({
    name: "DocumentGet",
    params: {
      id,
    },
  }).href

  const response = await getURL<PeerDBDocument>(newURL, null, abortController.signal, searchProgress)

  return getName(response.doc?.claims) || "no name"
}
</script>

<template>
  <div class="flex flex-col gap-1">
    <Combobox ref="searchEl" v-model="selectedDocument" :data-url="searchURL" as="div">
      <div class="relative">
        <ComboboxInput
          :readonly="isInProgress"
          class="w-full p-2 text-left rounded border-0 shadow ring-2 ring-neutral-300 focus:ring-2"
          :class="{
            'bg-white': !isInProgress && !(searchStateError || searchResultsError),
            'cursor-not-allowed bg-gray-100 text-gray-800 hover:ring-neutral-300 focus:border-primary-300 focus:ring-primary-300': isInProgress,
            'bg-error-50': searchStateError || searchResultsError,
          }"
          :display-value="(doc) => nameCache[doc?.id] ?? ''"
          @input="query = $event.target.value"
        />

        <ComboboxOptions
          v-if="searchResults.length > 0 && !isInProgress"
          class="absolute max-h-40 overflow-scroll mt-2 w-full bg-white rounded border-0 shadow ring-2 ring-neutral-300 z-10"
        >
          <ComboboxOption v-for="result in searchResults" :key="result.id" v-slot="{ active }" :value="result" as="template">
            <li :class="['cursor-pointer p-2', active ? 'bg-neutral-100' : '']">
              <WithPeerDBDocument :id="result.id" name="DocumentGet">
                <template #default="{ doc }">
                  {{ getName(doc?.claims) || "no name" }}
                </template>
                <template #loading="{ url }">
                  <i class="h-2 animate-pulse rounded bg-slate-200" :data-url="url" :class="[loadingWidth(result.id)]"></i>
                </template>
              </WithPeerDBDocument>
            </li>
          </ComboboxOption>
        </ComboboxOptions>
      </div>
    </Combobox>

    <template v-if="searchStateError || searchResultsError">
      <div class="text-sm my-1"><i class="text-error-600">loading data failed</i></div>
    </template>
  </div>
</template>
