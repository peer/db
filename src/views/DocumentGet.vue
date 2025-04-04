<script setup lang="ts">
import type { DocumentBeginEditResponse } from "@/types"
import type { PeerDBDocument } from "@/document"
import type { ComponentExposed } from "vue-component-type-helpers"

import { ref, computed, toRef, onBeforeUnmount, watchEffect } from "vue"
import { useRoute, useRouter } from "vue-router"
import { ChevronLeftIcon, ChevronRightIcon, PencilIcon } from "@heroicons/vue/20/solid"
import { TabGroup, TabList, Tab, TabPanels, TabPanel } from "@headlessui/vue"
import InputTextLink from "@/components/InputTextLink.vue"
import Button from "@/components/Button.vue"
import ButtonLink from "@/components/ButtonLink.vue"
import WithDocument from "@/components/WithDocument.vue"
import NavBar from "@/partials/NavBar.vue"
import Footer from "@/partials/Footer.vue"
import NavBarSearch from "@/partials/NavBarSearch.vue"
import PropertiesRows from "@/partials/PropertiesRows.vue"
import { useSearchState, useSearch } from "@/search"
import { postJSON } from "@/api"
import { getBestClaimOfType, getName, loadingLongWidth, encodeQuery } from "@/utils"
import { ARTICLE, FILE_URL, MEDIA_TYPE } from "@/props"
import { injectProgress } from "@/progress"

const props = defineProps<{
  id: string
}>()

const route = useRoute()
const router = useRouter()

const el = ref(null)

const progress = injectProgress()
const editProgress = injectProgress()

const abortController = new AbortController()

onBeforeUnmount(() => {
  abortController.abort()
})

const WithPeerDBDocument = WithDocument<PeerDBDocument>
const withDocument = ref<ComponentExposed<typeof WithPeerDBDocument> | null>(null)

const searchStateS = toRef(() => (Array.isArray(route.query.s) ? route.query.s[0] : route.query.s))

const { searchState, error: searchStateError } = useSearchState(searchStateS, progress)
const { results, error: searchResultsError } = useSearch(
  toRef(() => {
    if (!searchState.value) {
      return ""
    }
    if (searchState.value.s !== searchStateS.value) {
      return ""
    }
    if (searchState.value.p && !searchState.value.promptDone) {
      return ""
    }
    return searchStateS.value
  }),
  el,
  progress,
)

watchEffect(async (onCleanup) => {
  if (searchStateError.value || searchResultsError.value) {
    // Something was not OK, so we redirect to the URL without "s".
    router.replace({
      name: "DocumentGet",
      params: {
        id: props.id,
      },
      // Maybe route.query has non-empty "tab" parameter which we want to keep.
      // We do not use query argument in useSearchState callback because it
      // is about (new) search state which we just discard here.
      query: encodeQuery({ tab: route.query.tab || undefined }),
    })
  }
})

const prevNext = computed<{ previous: string | null; next: string | null }>(() => {
  const res = { previous: null, next: null } as { previous: string | null; next: string | null }
  for (let i = 0; i < results.value.length; i++) {
    if (results.value[i].id === props.id) {
      if (i > 0) {
        res.previous = results.value[i - 1].id
      }
      if (i < results.value.length - 1) {
        res.next = results.value[i + 1].id
      }
      return res
    }
  }

  if (results.value.length > 0) {
    // Results are loaded but we could not find ID. Redirect to the URL without "s".
    // Ugly, a side effect inside computed. But it works well.
    router.replace({
      name: "DocumentGet",
      params: {
        id: props.id,
      },
    })
  }
  return res
})

async function afterClick() {
  document.getElementById("search-input-text")?.focus()
}

const docName = computed(() => getName(withDocument.value?.doc?.claims))
const article = computed(() => getBestClaimOfType(withDocument.value?.doc?.claims, "text", ARTICLE))
const file = computed(() => {
  const f = {
    url: getBestClaimOfType(withDocument.value?.doc?.claims, "ref", FILE_URL)?.iri,
    mediaType: getBestClaimOfType(withDocument.value?.doc?.claims, "string", MEDIA_TYPE)?.string,
  }
  if (f.url && f.mediaType) {
    return f
  }
  return null
})

async function onEdit() {
  if (abortController.signal.aborted) {
    return
  }

  editProgress.value += 1
  try {
    const editResponse = await postJSON<DocumentBeginEditResponse>(
      router.apiResolve({
        name: "DocumentBeginEdit",
        params: {
          id: props.id,
        },
      }).href,
      {},
      abortController.signal,
      editProgress,
    )
    if (abortController.signal.aborted) {
      return
    }
    await router.push({
      name: "DocumentEdit",
      params: {
        id: props.id,
        session: editResponse.session,
      },
    })
  } catch (err) {
    if (abortController.signal.aborted) {
      return
    }
    // TODO: Show notification with error.
    console.error("DocumentGet.onEdit", err)
  } finally {
    editProgress.value -= 1
  }
}
</script>

<template>
  <Teleport to="header">
    <NavBar>
      <div v-if="searchState !== null" class="flex flex-grow gap-x-1 sm:gap-x-4">
        <InputTextLink
          class="max-w-xl flex-grow"
          :to="{ name: 'SearchResults', params: { s: searchState.s }, query: encodeQuery({ q: searchState.q, at: id }) }"
          :after-click="afterClick"
        >
          {{ searchState.q }}
        </InputTextLink>
        <div class="grid grid-cols-2 gap-x-1">
          <ButtonLink
            primary
            class="!px-3.5"
            :disabled="!prevNext.previous"
            :to="{ name: 'DocumentGet', params: { id: prevNext.previous }, query: encodeQuery({ s: searchState.s }) }"
          >
            <ChevronLeftIcon class="h-5 w-5 sm:hidden" alt="Prev" />
            <span class="hidden sm:inline">Prev</span>
          </ButtonLink>
          <ButtonLink
            primary
            class="!px-3.5"
            :disabled="!prevNext.next"
            :to="{ name: 'DocumentGet', params: { id: prevNext.next }, query: encodeQuery({ s: searchState.s }) }"
          >
            <ChevronRightIcon class="h-5 w-5 sm:hidden" alt="Next" />
            <span class="hidden sm:inline">Next</span>
          </ButtonLink>
        </div>
      </div>
      <NavBarSearch v-else />
      <Button :progress="editProgress" type="button" primary class="!px-3.5" @click.prevent="onEdit">
        <PencilIcon class="h-5 w-5 sm:hidden" alt="Edit" />
        <span class="hidden sm:inline">Edit</span>
      </Button>
    </NavBar>
  </Teleport>
  <div ref="el" class="mt-12 flex w-full flex-col gap-y-1 border-t border-transparent p-1 sm:mt-[4.5rem] sm:gap-y-4 sm:p-4" :data-url="withDocument?.url">
    <div class="rounded border bg-white p-4 shadow">
      <WithPeerDBDocument :id="id" ref="withDocument" name="DocumentGet">
        <template #default="{ doc }">
          <!--
            TODO: Fix how hover interacts with focused tab.
            See: https://github.com/tailwindlabs/tailwindcss/discussions/10123
          -->
          <TabGroup>
            <TabList class="-m-4 mb-4 flex border-collapse flex-row rounded-t border-b bg-slate-100">
              <Tab
                v-if="article"
                class="select-none border-r px-4 py-3 font-medium uppercase leading-tight outline-none first:rounded-tl focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 ui-selected:bg-white ui-not-selected:hover:bg-slate-50"
                >Article</Tab
              >
              <Tab
                v-if="file"
                class="select-none border-r px-4 py-3 font-medium uppercase leading-tight outline-none first:rounded-tl-md focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 ui-selected:bg-white ui-not-selected:hover:bg-slate-50"
                >File</Tab
              >
              <Tab
                class="select-none border-r px-4 py-3 font-medium uppercase leading-tight outline-none first:rounded-tl focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 ui-selected:bg-white ui-not-selected:hover:bg-slate-50"
                >All properties</Tab
              >
            </TabList>
            <h1 class="mb-4 text-4xl font-bold drop-shadow-sm" v-html="docName || '<i>no name</i>'"></h1>
            <TabPanels>
              <!-- We explicitly disable tabbing. See: https://github.com/tailwindlabs/headlessui/discussions/1433 -->
              <TabPanel v-if="article" tabindex="-1">
                <!-- eslint-disable-next-line vue/no-v-html -->
                <div class="prose prose-slate max-w-none" v-html="article.html.en"></div>
              </TabPanel>
              <TabPanel v-if="file" tabindex="-1">
                <template v-if="file.mediaType?.startsWith('image/')">
                  <a :href="file.url"><img :src="file.url" /></a>
                </template>
              </TabPanel>
              <TabPanel tabindex="-1">
                <table class="w-full table-auto border-collapse">
                  <thead>
                    <tr>
                      <th class="border-r border-slate-200 px-2 py-1 text-left font-bold">Property</th>
                      <th class="border-l border-slate-200 px-2 py-1 text-left font-bold">Value</th>
                    </tr>
                  </thead>
                  <tbody>
                    <PropertiesRows :claims="doc.claims" />
                  </tbody>
                </table>
              </TabPanel>
            </TabPanels>
          </TabGroup>
        </template>
        <template #loading>
          <div class="flex animate-pulse flex-col gap-y-2">
            <div class="inline-block h-2 rounded bg-slate-200" :class="[loadingLongWidth(`${id}/1`)]"></div>
            <div class="flex gap-x-4">
              <div class="h-2 rounded bg-slate-200" :class="[loadingLongWidth(`${id}/2`)]"></div>
              <div class="h-2 rounded bg-slate-200" :class="[loadingLongWidth(`${id}/3`)]"></div>
            </div>
            <div class="flex gap-x-4">
              <div class="h-2 rounded bg-slate-200" :class="[loadingLongWidth(`${id}/4`)]"></div>
              <div class="h-2 rounded bg-slate-200" :class="[loadingLongWidth(`${id}/5`)]"></div>
            </div>
          </div>
        </template>
        <template #error>
          <i class="text-error-600">loading data failed</i>
        </template>
      </WithPeerDBDocument>
    </div>
  </div>
  <Teleport to="footer">
    <Footer class="border-t border-slate-50 bg-slate-200 shadow" />
  </Teleport>
</template>
