<script setup lang="ts">
import type { D } from "@/document"
import type { ClaimTypes } from "@/document/claims"
import type { Result } from "@/types"
import type { DeepReadonly } from "vue"

import { Combobox, ComboboxButton, ComboboxInput, ComboboxOption, ComboboxOptions } from "@headlessui/vue"
import { ArrowTopRightOnSquareIcon, CheckIcon, ChevronUpDownIcon } from "@heroicons/vue/20/solid"
import { computed, onBeforeUnmount, ref, shallowRef, watch } from "vue"
import { useI18n } from "vue-i18n"
import { useRouter } from "vue-router"

import { postJSON } from "@/api"
import WithDocument from "@/components/WithDocument.vue"
import { selectClaimsByLanguage } from "@/document"
import { useProgress } from "@/progress"
import { anySignal, loadingWidth } from "@/utils"

// Wildcard to see if a string ends with unicode letter or number.
const WILDCARD_SEARCH_REGEX = /[\p{L}\p{N}]$/u

const props = withDefaults(
  defineProps<{
    progress?: number
    readonly?: boolean
  }>(),
  {
    progress: 0,
    readonly: false,
  },
)

const model = defineModel<string>({ default: "" })

// We want all fallthrough attributes to be passed to the combobox input element.
defineOptions({
  inheritAttrs: false,
})

const searchProgress = useProgress()

const router = useRouter()
const { locale } = useI18n({ useScope: "global" })

const selectedDocument = shallowRef<Result | null>(null)
const query = ref("")
const isInProgress = computed(() => props.progress > 0 || searchProgress.value > 0)
const searchResults = ref<Result[]>([])

const optionsVisible = ref(false)

const mainAbortController = new AbortController()
let searchAbortController = new AbortController()

// Sync display-name lookup using the NAME/TITLE string claims for the active locale.
function getName(claims: DeepReadonly<ClaimTypes> | null | undefined): string | null {
  if (!claims) return null
  const matched = selectClaimsByLanguage(claims, "string", [], locale.value, (cs) => !!(cs.length > 0 && cs[0].string))
  return matched?.[0].string ?? null
}

async function search(q: string) {
  const signal = anySignal(mainAbortController.signal, searchAbortController.signal)

  if (signal.aborted) {
    return
  }

  // Add wildcard for prefix search if query ends with unicode letter or number.
  if (WILDCARD_SEARCH_REGEX.test(q)) {
    q = q + "*"
  }

  searchProgress.value += 1
  try {
    // Create a new search session.
    const response = await postJSON<Result[]>(
      router.apiResolve({ name: "SearchJustResults" }).href,
      {
        query: q,
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
    console.error("InputRef.search", err)
  } finally {
    searchProgress.value -= 1
  }
}

watch(
  () => model.value,
  (id) => {
    selectedDocument.value = id ? { id } : null
  },
  { immediate: true },
)

watch(
  () => selectedDocument.value?.id,
  (id) => {
    model.value = id || ""
  },
)

watch(
  query,
  async (value) => {
    // Open options on search.
    if (value) optionsVisible.value = true

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

const WithPeerDBDocument = WithDocument<D>
</script>

<template>
  <div class="flex flex-col gap-1">
    <Combobox v-model="selectedDocument" as="div" @update:model-value="optionsVisible = false">
      <div class="relative">
        <div class="relative w-full">
          <!-- We only show input field when document is not yet selected. -->
          <ComboboxInput
            v-if="!selectedDocument?.id"
            :readonly="readonly"
            v-bind="$attrs"
            class="w-full rounded-sm border-none py-2 pr-10 pl-3 text-left shadow-sm ring-2 ring-neutral-300 outline-none focus:ring-2"
            :class="{
              'bg-white': !isInProgress,
              'bg-gray-100!': isInProgress || readonly,
              'cursor-not-allowed bg-gray-100 text-gray-800 hover:ring-neutral-300 focus:ring-primary-300': isInProgress || readonly,
              'text-gray-800': isInProgress || readonly,
              'hover:ring-neutral-300! focus:ring-primary-300!': isInProgress || readonly,
              'hover:ring-neutral-400 focus:ring-primary-500': !isInProgress,
            }"
            @input="query = ($event.target as HTMLInputElement).value"
            @focusin="optionsVisible = true"
            @focusout="optionsVisible = false"
          />

          <!--
            Once document is selected we resolve it with WithPeerDBDocument component
            and display its value through getName of the document.
          -->
          <WithPeerDBDocument v-else :id="selectedDocument.id" name="DocumentGet">
            <template #default="{ doc }">
              <ComboboxInput
                :readonly="readonly"
                v-bind="$attrs"
                class="w-full rounded-sm border-none py-2 pr-10 pl-3 text-left shadow-sm ring-2 ring-neutral-300 outline-none focus:ring-2"
                :class="{
                  'bg-white': !isInProgress,
                  'bg-gray-100!': isInProgress || readonly,
                  'cursor-not-allowed bg-gray-100 text-gray-800 hover:ring-neutral-300 focus:ring-primary-300': isInProgress || readonly,
                  'text-gray-800': isInProgress || readonly,
                  'hover:ring-neutral-300! focus:ring-primary-300!': isInProgress || readonly,
                  'hover:ring-neutral-400 focus:ring-primary-500': !isInProgress,
                }"
                :display-value="() => getName(doc?.claims) || ''"
                @input="query = ($event.target as HTMLInputElement).value"
              />
            </template>
          </WithPeerDBDocument>

          <ComboboxButton class="absolute inset-y-0 right-0 flex items-center gap-1 pr-2" @click.prevent="optionsVisible = !optionsVisible">
            <RouterLink v-if="selectedDocument?.id" :to="{ name: 'DocumentGet', params: { id: selectedDocument.id } }" class="link">
              <ArrowTopRightOnSquareIcon class="size-5" aria-hidden="true" />
            </RouterLink>

            <ChevronUpDownIcon
              class="size-5 text-gray-400"
              :class="{
                'cursor-not-allowed': progress > 0 || readonly,
              }"
              aria-hidden="true"
            />
          </ComboboxButton>
        </div>

        <ComboboxOptions
          v-if="optionsVisible && !isInProgress && !readonly"
          static
          class="absolute z-10 mt-1 max-h-40 w-full overflow-auto rounded-sm bg-white shadow-sm ring-2 ring-neutral-300 outline-none"
        >
          <ComboboxOption v-if="searchResults.length === 0">
            <li class="p-2"><i>No results found.</i></li>
          </ComboboxOption>

          <template v-if="searchResults.length > 0">
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
                        <div
                          class="w-full cursor-pointer truncate"
                          :class="{
                            'font-medium': result.id === selectedDocument?.id,
                          }"
                          v-html="getName(doc?.claims)"
                        />

                        <CheckIcon v-if="result.id === selectedDocument?.id" class="mr-2 size-5 text-primary-600" aria-hidden="true" />

                        <!-- We explicitly call router.push with mousedown.stop, to prevent headlesui
                            closing the options without redirect -->
                        <a v-if="result?.id" class="link hover:cursor-pointer" @mousedown.stop="() => router.push({ name: 'DocumentGet', params: { id: result.id } })">
                          <ArrowTopRightOnSquareIcon class="size-5" aria-hidden="true" />
                        </a>
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
          </template>
        </ComboboxOptions>
      </div>
    </Combobox>
  </div>
</template>
