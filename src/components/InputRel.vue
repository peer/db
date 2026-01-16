<script setup lang="ts">
import type { PeerDBDocument } from "@/document"
import type { Filters, Result } from "@/types"

import { Combobox, ComboboxButton, ComboboxInput, ComboboxOption, ComboboxOptions } from "@headlessui/vue"
import { ArrowTopRightOnSquareIcon, ChevronUpDownIcon } from "@heroicons/vue/20/solid"
import { computed, onBeforeUnmount, ref, shallowRef, toRef, useTemplateRef, watch } from "vue"
import { useRouter } from "vue-router"

import { postJSON } from "@/api"
import WithDocument from "@/components/WithDocument.vue"
import { injectMainProgress, localProgress } from "@/progress"
import { TYPE } from "@/props"
import { NONE, useSearch, useSearchSession } from "@/search"
import { encodeQuery, getName, loadingWidth } from "@/utils"

const WILDCARD_SEARCH_REGEX = /[\p{L}\p{N}]$/u

defineOptions({ inheritAttrs: false })

const props = withDefaults(
  defineProps<{
    readonly progress?: number
    type?: string | typeof NONE
  }>(),
  {
    progress: 0,
    type: "",
  },
)

const emit = defineEmits<{
  (e: "update:modelValue", id: string): void
}>()

const model = defineModel<string>({ default: "" })

const mainProgress = injectMainProgress()
const searchProgress = localProgress(mainProgress)

const router = useRouter()
const searchEl = useTemplateRef<HTMLElement>("searchEl")

const selectedDocument = shallowRef<Result | null>(null)

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

const isInProgress = computed(() => props.progress > 0 || searchProgress.value > 0)
const first100SearchResults = computed(() => searchResults.value.slice(0, 100))

let abortController = new AbortController()
const nameAbort = new AbortController()

async function search(q: string) {
  if (abortController.signal.aborted) {
    return
  }

  // Add wildcard for prefix search if query ends with a letter or number.
  if (WILDCARD_SEARCH_REGEX.test(q)) {
    q = q + "*"
  }

  // Build rel filters.
  let filters: Filters | null = null
  if (props.type) {
    if (props.type == NONE) {
      filters = { rel: { prop: TYPE, none: true } }
    } else {
      filters = { rel: { prop: TYPE, value: props.type } }
    }
  }

  searchProgress.value += 1
  try {
    // Create a new search session.
    const createResponse = await postJSON<Result>(
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

watch(
  () => model.value,
  (id) => {
    if (!id) return (selectedDocument.value = null)
    selectedDocument.value = { id }
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

watch(query, async (value) => {
  abortController.abort("new search call")
  abortController = new AbortController()
  await search(value)
})

onBeforeUnmount(() => {
  abortController.abort()
  nameAbort.abort()
})

const WithPeerDBDocument = WithDocument<PeerDBDocument>
</script>

<template>
  <div class="flex flex-col gap-1">
    <Combobox ref="searchEl" v-model="selectedDocument" :data-url="searchURL" as="div">
      <div class="relative">
        <div class="relative w-full">
          <!-- We only show input field when document is not yet selected. -->
          <ComboboxInput
            v-if="!selectedDocument?.id"
            :readonly="isInProgress"
            class="w-full rounded-sm border-none py-2 pr-10 pl-3 text-left shadow-sm ring-2 ring-neutral-300 outline-none focus:ring-2"
            :class="{
              'bg-white': !isInProgress && !(searchSessionError || searchResultsError),
              'cursor-not-allowed bg-gray-100 text-gray-800 hover:ring-neutral-300 focus:ring-primary-300': isInProgress,
              'bg-error-50': searchSessionError || searchResultsError,
              'hover:ring-neutral-400 focus:ring-primary-500': !isInProgress && !(searchSessionError || searchResultsError),
            }"
            @input="query = ($event.target as HTMLInputElement).value"
          />

          <!-- Once document is selected we resolve it with WithPeerDBDocument component
               and display its value through getName of the document. -->
          <WithPeerDBDocument v-else :id="selectedDocument.id" name="DocumentGet">
            <template #default="{ doc }">
              <ComboboxInput
                class="w-full rounded-sm border-none py-2 pr-10 pl-3 text-left shadow-sm ring-2 ring-neutral-300 outline-none focus:ring-2"
                :class="{
                  'bg-white': !isInProgress && !(searchSessionError || searchResultsError),
                  'cursor-not-allowed bg-gray-100 text-gray-800 hover:ring-neutral-300 focus:ring-primary-300': isInProgress,
                  'bg-error-50': searchSessionError || searchResultsError,
                  'hover:ring-neutral-400 focus:ring-primary-500': !isInProgress && !(searchSessionError || searchResultsError),
                }"
                :display-value="() => getName(doc?.claims) || ''"
                @input="query = ($event.target as HTMLInputElement).value"
              />
            </template>
          </WithPeerDBDocument>

          <ComboboxButton class="absolute inset-y-0 right-0 flex items-center gap-1 pr-2">
            <RouterLink
              v-if="selectedDocument?.id && searchSession?.id"
              :to="{ name: 'DocumentGet', params: { id: selectedDocument.id }, query: encodeQuery({ s: searchSession.id }) }"
              class="link"
            >
              <ArrowTopRightOnSquareIcon class="size-5 text-gray-400" aria-hidden="true" />
            </RouterLink>

            <ChevronUpDownIcon class="size-5 text-gray-400" aria-hidden="true" />
          </ComboboxButton>
        </div>

        <ComboboxOptions
          v-if="searchResults.length > 0 && !isInProgress"
          class="absolute z-10 mt-1 max-h-40 w-full overflow-auto rounded-sm bg-white shadow-sm ring-2 ring-neutral-300 outline-none"
        >
          <WithPeerDBDocument v-for="result in first100SearchResults" :id="result.id" :key="result.id" name="DocumentGet">
            <template #default="{ doc }">
              <ComboboxOption v-slot="{ active }" :value="result" as="template" :disabled="!getName(doc?.claims)">
                <li class="p-1 outline-none select-none">
                  <!--
                    We have an additional div so that the ring has the space to be shown.
                    li element has p-1 for ring space, together with py-1 and px-2 we get the effective padding
                    for option content of py-2 and px-3, same what InputText and ListboxButton have.
                  -->
                  <div class="flex flex-row items-center justify-between rounded-sm px-2 py-1" :class="active ? 'ring-2 ring-primary-500' : ''">
                    <template v-if="getName(doc?.claims)">
                      <div class="w-full cursor-pointer truncate" v-html="getName(doc?.claims)" />

                      <RouterLink
                        v-if="result?.id && searchSession?.id"
                        :to="{ name: 'DocumentGet', params: { id: result.id }, query: encodeQuery({ s: searchSession.id }) }"
                        class="link"
                      >
                        <ArrowTopRightOnSquareIcon class="size-5 text-gray-400" aria-hidden="true" />
                      </RouterLink>
                    </template>

                    <i v-else>no name</i>
                  </div>
                </li>
              </ComboboxOption>
            </template>
            <template #loading="{ url }">
              <li class="p-1 outline-none select-none">
                <i class="h-2 animate-pulse rounded bg-slate-200" :data-url="url" :class="[loadingWidth(result.id)]"></i>
              </li>
            </template>
          </WithPeerDBDocument>
        </ComboboxOptions>
      </div>
    </Combobox>

    <template v-if="searchSessionError || searchResultsError">
      <div class="my-1 text-sm"><i class="text-error-600">loading data failed</i></div>
    </template>
  </div>
</template>
