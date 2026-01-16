<script setup lang="ts">
import type { PeerDBDocument } from "@/document"
import type { Filters, Result } from "@/types"

import { Combobox, ComboboxButton, ComboboxInput, ComboboxOption, ComboboxOptions } from "@headlessui/vue"
import { ChevronUpDownIcon } from "@heroicons/vue/20/solid"
import { debounce } from "lodash-es"
import { computed, onBeforeUnmount, ref, shallowRef, toRef, useTemplateRef, watch } from "vue"
import { useRouter } from "vue-router"

import { getURL, postJSON } from "@/api"
import WithDocument from "@/components/WithDocument.vue"
import { injectMainProgress, localProgress } from "@/progress"
import { TYPE } from "@/props"
import { NONE, useSearch, useSearchSession } from "@/search"
import { getName, loadingWidth } from "@/utils"

defineOptions({ inheritAttrs: false })

const props = withDefaults(
  defineProps<{
    readonly progress?: number
    type?: string
  }>(),
  {
    progress: 0,
    type: "",
  },
)

const model = defineModel<string>({ default: "" })

const emit = defineEmits<{
  (e: "update:modelValue", id: string): void
}>()

const router = useRouter()
const DEBOUNCE_MS = 500

const abortController = new AbortController()
const nameAbort = new AbortController()

const WithPeerDBDocument = WithDocument<PeerDBDocument>

const searchEl = useTemplateRef<HTMLElement>("searchEl")

type ResultWithName = Result & { name: string }

const selectedDocument = shallowRef<ResultWithName | null>(null)
const nameCache = ref<Record<string, string>>({})

watch(
  () => model.value,
  async (id) => {
    if (!id) return (selectedDocument.value = null)
    if (!nameCache.value[id]) {
      nameCache.value[id] = await resolveDocumentName(id)
    }
    selectedDocument.value = { id, name: nameCache.value[id] }
  },
  { immediate: true },
)

watch(
  () => selectedDocument.value?.id,
  (id) => {
    if (!id) return
    emit("update:modelValue", id)
  },
)

const mainProgress = injectMainProgress()
const searchProgress = localProgress(mainProgress)

const isInProgress = computed(() => props.progress > 0 || searchProgress.value > 0)

const query = ref("")
const searchSessionId = ref<string | null>(null)
const searchSessionVersion = ref(0)

const {
  searchSession,
  error: searchSessionError,
  url: searchURL,
} = useSearchSession(
  toRef(() => (searchSessionId.value ? { id: searchSessionId.value, version: searchSessionVersion.value } : null)),
  searchProgress,
)

const { results: searchResults, error: searchResultsError } = useSearch(searchSession, searchEl, searchProgress)

watch(query, async (value) => {
  runSearchDebounce.cancel()
  await runSearchDebounce(value)
})

onBeforeUnmount(() => {
  abortController.abort()
  nameAbort.abort()
})

const runSearchDebounce = debounce(async (q: string) => {
  await search(q)
}, DEBOUNCE_MS)

async function search(q: string) {
  if (abortController.signal.aborted) {
    return
  }

  // Build rel filters.
  let filters: Filters | null = null
  if (props.type) {
    if (props.type === NONE.toString()) {
      filters = { rel: { prop: TYPE.toString(), none: true } }
    } else {
      filters = { rel: { prop: TYPE.toString(), value: props.type } }
    }
  }

  searchProgress.value += 1
  try {
    // Create a new search session.
    const createResponse = await postJSON<{ id: string }>(
      router.apiResolve({ name: "SearchCreate" }).href,
      {
        query: q,
        filters: filters ?? undefined,
      },
      abortController.signal,
      searchProgress,
    )

    if (abortController.signal.aborted) {
      return
    }

    searchSessionId.value = createResponse.id
    searchSessionVersion.value = 0
  } catch (err) {
    if (abortController.signal.aborted) {
      return
    }
    console.error("InputRel.search", err)
  } finally {
    searchProgress.value -= 1
  }
}

async function resolveDocumentName(id: string): Promise<string> {
  const newURL = router.apiResolve({ name: "DocumentGet", params: { id } }).href
  const response = await getURL<PeerDBDocument>(newURL, null, nameAbort.signal, searchProgress)
  return getName(response.doc?.claims) || "no name"
}
</script>

<template>
  <div class="flex flex-col gap-1">
    <Combobox ref="searchEl" v-model="selectedDocument" :data-url="searchURL" as="div">
      <div class="relative">
        <div class="relative w-full">
          <ComboboxInput
            :readonly="isInProgress"
            class="w-full rounded-sm border-none py-2 pr-10 pl-3 text-left shadow-sm ring-2 ring-neutral-300 outline-none focus:ring-2"
            :class="{
              'bg-white': !isInProgress && !(searchSessionError || searchResultsError),
              'cursor-not-allowed bg-gray-100 text-gray-800 hover:ring-neutral-300 focus:ring-primary-300': isInProgress,
              'bg-error-50': searchSessionError || searchResultsError,
              'hover:ring-neutral-400 focus:ring-primary-500': !isInProgress && !(searchSessionError || searchResultsError),
            }"
            :display-value="
              (item: unknown) => {
                // We have to type it, because parameter expects unknown.
                const doc = item as ResultWithName | null | undefined
                return doc?.name ?? (doc?.id ? nameCache[doc.id] : '') ?? ''
              }
            "
            @input="query = ($event.target as HTMLInputElement).value"
          />

          <ComboboxButton class="absolute inset-y-0 right-0 flex items-center pr-2">
            <ChevronUpDownIcon class="size-5 text-gray-400" aria-hidden="true" />
          </ComboboxButton>
        </div>

        <ComboboxOptions
          v-if="searchResults.length > 0 && !isInProgress"
          class="absolute z-10 mt-1 max-h-40 w-full overflow-auto rounded-sm bg-white shadow-sm ring-2 ring-neutral-300 outline-none"
        >
          <ComboboxOption v-for="result in searchResults" :key="result.id" v-slot="{ active }" :value="result" as="template">
            <li class="cursor-pointer p-1 outline-none select-none">
              <div class="flex flex-row justify-between gap-x-1 rounded-sm px-2 py-1" :class="active ? 'ring-2 ring-primary-500' : ''">
                <WithPeerDBDocument :id="result.id" name="DocumentGet">
                  <template #default="{ doc }">
                    <div class="truncate">{{ getName(doc?.claims) || "no name" }}</div>
                  </template>
                  <template #loading="{ url }">
                    <i class="h-2 animate-pulse rounded bg-slate-200" :data-url="url" :class="[loadingWidth(result.id)]"></i>
                  </template>
                </WithPeerDBDocument>
              </div>
            </li>
          </ComboboxOption>
        </ComboboxOptions>
      </div>
    </Combobox>

    <template v-if="searchSessionError || searchResultsError">
      <div class="my-1 text-sm"><i class="text-error-600">loading data failed</i></div>
    </template>
  </div>
</template>
