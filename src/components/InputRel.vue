<script setup lang="ts">
import type { PeerDBDocument } from "@/document"
import type { Filters, Result } from "@/types"

import { Combobox, ComboboxButton, ComboboxInput, ComboboxOption, ComboboxOptions } from "@headlessui/vue"
import { ArrowTopRightOnSquareIcon, ChevronUpDownIcon } from "@heroicons/vue/20/solid"
import { computed, onBeforeUnmount, ref, shallowRef, watch } from "vue"
import { useRouter } from "vue-router"

import { getURL, postJSON } from "@/api"
import WithDocument from "@/components/WithDocument.vue"
import { injectProgress } from "@/progress"
import { TYPE } from "@/props"
import { NONE } from "@/search"
import { anySignal, getName, loadingWidth } from "@/utils"

// Wildcard to see if a string ends with unicode letter or number.
const WILDCARD_SEARCH_REGEX = /[\p{L}\p{N}]$/u

const props = withDefaults(
  defineProps<{
    progress?: number
    type?: string | typeof NONE
  }>(),
  {
    progress: 0,
    type: "",
  },
)

const model = defineModel<string>({ default: "" })

// We want all fallthrough attributes to be passed to the combobox input element.
defineOptions({
  inheritAttrs: false,
})

const searchProgress = injectProgress()

const router = useRouter()

const selectedDocument = shallowRef<Result | null>(null)
const query = ref("")
const isDocumentTypeValid = ref(true)
const isInProgress = computed(() => props.progress > 0 || searchProgress.value > 0)
const searchResults = ref<Result[]>([])

const mainAbortController = new AbortController()
let searchAbortController = new AbortController()

async function search(q: string) {
  const signal = anySignal(mainAbortController.signal, searchAbortController.signal)

  if (signal.aborted) {
    return
  }

  // Add wildcard for prefix search if query ends with unicode letter or number.
  if (WILDCARD_SEARCH_REGEX.test(q)) {
    q = q + "*"
  }

  // Build rel filters.
  let filters: Filters | undefined = undefined
  if (props.type) {
    if (props.type === NONE) {
      filters = { rel: { prop: TYPE, none: true } }
    } else {
      filters = { rel: { prop: TYPE, value: props.type } }
    }
  }

  searchProgress.value += 1
  try {
    // Create a new search session.
    const response = await postJSON<Result[]>(
      router.apiResolve({ name: "SearchJustResults" }).href,
      {
        query: q,
        filters: filters,
      },
      signal,
      searchProgress,
    )
    if (signal.aborted) {
      return
    }

    // We use only the first 100 results.
    searchResults.value = response.slice(0, 100)
  } catch (err) {
    if (signal.aborted) {
      return
    }
    // TODO: Show error.
    console.error("InputRel.search", err)
  } finally {
    searchProgress.value -= 1
  }
}

async function validateSelectedDocument(id: string): Promise<void> {
  const newURL = router.apiResolve({ name: "DocumentGet", params: { id } }).href
  const response = await getURL<PeerDBDocument>(newURL, null, mainAbortController.signal, searchProgress)

  const relClaims = response.doc.claims?.rel
  if (!relClaims) {
    isDocumentTypeValid.value = false
    return
  }

  isDocumentTypeValid.value = !!relClaims.find((claim) => claim.to.id == props.type)
}

watch(
  () => model.value,
  async (id) => {
    if (!id) return (selectedDocument.value = null)
    await validateSelectedDocument(id)
    if (mainAbortController.signal.aborted) {
      return
    }
    selectedDocument.value = { id }
  },
  { immediate: true },
)

watch(
  () => selectedDocument.value?.id,
  (id) => {
    if (!id) return
    model.value = id
  },
)

watch(
  query,
  async (value) => {
    searchAbortController.abort()
    searchAbortController = new AbortController()
    await search(value)
  },
  { immediate: true },
)

onBeforeUnmount(() => {
  searchAbortController.abort()
  mainAbortController.abort()
})

const WithPeerDBDocument = WithDocument<PeerDBDocument>
</script>

<template>
  <div class="flex flex-col gap-1">
    <Combobox v-model="selectedDocument" as="div">
      <div class="relative">
        <div class="relative w-full">
          <!-- We only show input field when document is not yet selected. -->
          <ComboboxInput
            v-if="!selectedDocument?.id"
            :readonly="isInProgress"
            v-bind="$attrs"
            class="w-full rounded-sm border-none py-2 pr-10 pl-3 text-left shadow-sm ring-2 ring-neutral-300 outline-none focus:ring-2"
            :class="{
              'bg-white': !isInProgress,
              'cursor-not-allowed bg-gray-100 text-gray-800 hover:ring-neutral-300 focus:ring-primary-300': isInProgress,
              'bg-error-50!': !isDocumentTypeValid,
              'hover:ring-neutral-400 focus:ring-primary-500': !isInProgress,
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
                  'bg-white': !isInProgress,
                  'cursor-not-allowed bg-gray-100 text-gray-800 hover:ring-neutral-300 focus:ring-primary-300': isInProgress,
                  'bg-error-50!': !isDocumentTypeValid,
                  'hover:ring-neutral-400 focus:ring-primary-500': !isInProgress,
                }"
                v-bind="$attrs"
                :display-value="() => getName(doc?.claims) || ''"
                @input="query = ($event.target as HTMLInputElement).value"
              />
            </template>
          </WithPeerDBDocument>

          <ComboboxButton class="absolute inset-y-0 right-0 flex items-center gap-1 pr-2">
            <RouterLink v-if="selectedDocument?.id" :to="{ name: 'DocumentGet', params: { id: selectedDocument.id } }" class="link">
              <ArrowTopRightOnSquareIcon class="size-5 text-gray-400" aria-hidden="true" />
            </RouterLink>

            <ChevronUpDownIcon class="size-5 text-gray-400" aria-hidden="true" />
          </ComboboxButton>
        </div>

        <ComboboxOptions
          v-if="searchResults.length > 0 && !isInProgress"
          class="absolute z-10 mt-1 max-h-40 w-full overflow-auto rounded-sm bg-white shadow-sm ring-2 ring-neutral-300 outline-none"
        >
          <WithPeerDBDocument v-for="result in searchResults" :id="result.id" :key="result.id" name="DocumentGet">
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

                      <RouterLink v-if="result?.id" :to="{ name: 'DocumentGet', params: { id: result.id } }" class="link">
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
  </div>
</template>
