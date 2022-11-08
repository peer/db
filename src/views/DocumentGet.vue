<script setup lang="ts">
import type { PeerDBDocument } from "@/types"

import { ref, computed, watch, readonly } from "vue"
import { useRoute } from "vue-router"
import { ChevronLeftIcon, ChevronRightIcon } from "@heroicons/vue/20/solid"
import { TabGroup, TabList, Tab, TabPanels, TabPanel } from "@headlessui/vue"
import RouterLink from "@/components/RouterLink.vue"
import InputText from "@/components/InputText.vue"
import ButtonLink from "@/components/ButtonLink.vue"
import NavBar from "@/components/NavBar.vue"
import Footer from "@/components/Footer.vue"
import NavBarSearch from "@/components/NavBarSearch.vue"
import PropertiesRows from "@/components/PropertiesRows.vue"
import { getDocument, useSearchState } from "@/search"
import { globalProgress } from "@/api"
import { getBestClaimOfType, useRouter } from "@/utils"
import { ARTICLE, FILE_URL, MEDIA_TYPE } from "@/props"

const props = defineProps({
  id: {
    type: String,
    required: true,
  },
})

const route = useRoute()
const router = useRouter()

const _doc = ref<PeerDBDocument | Record<string, never>>({})
const doc = import.meta.env.DEV ? readonly(_doc) : _doc

watch(
  () => props.id,
  (id, oldId, onCleanup) => {
    const controller = new AbortController()
    onCleanup(() => controller.abort())

    getDocument(router, { _id: id }, 0, controller.signal).then((data) => {
      _doc.value = data
    })
  },
  {
    immediate: true,
  },
)

const { results, query } = useSearchState(async (query) => {
  // Something was not OK, so we redirect to the URL without "s".
  // TODO: This has still created a new search state on the server, we should not do that.
  await router.replace({
    name: "DocumentGet",
    params: {
      id: props.id,
    },
  })
})

const prevNext = computed<{ previous: string | null; next: string | null }>(() => {
  const res = { previous: null, next: null } as { previous: string | null; next: string | null }
  for (let i = 0; i < results.value.length; i++) {
    if (results.value[i]._id === props.id) {
      if (i > 0) {
        res.previous = results.value[i - 1]._id
      }
      if (i < results.value.length - 1) {
        res.next = results.value[i + 1]._id
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

const hasLoaded = computed(() => doc.value?.name?.en)
const article = computed(() => getBestClaimOfType(doc.value?.claims, "text", ARTICLE))
const file = computed(() => {
  const f = {
    url: getBestClaimOfType(doc.value?.claims, "ref", FILE_URL)?.iri,
    mediaType: getBestClaimOfType(doc.value?.claims, "string", MEDIA_TYPE)?.string,
  }
  if (f.url && f.mediaType) {
    return f
  }
  return null
})
</script>

<template>
  <Teleport to="header">
    <NavBar :progress="globalProgress">
      <div v-if="route.query.s" class="flex flex-grow gap-x-1 sm:gap-x-4">
        <InputText v-if="!query.s" :progress="1" class="max-w-xl flex-grow" :value="query.q" />
        <RouterLink
          v-else
          class="max-w-xl flex-grow appearance-none rounded border-0 border-gray-500 bg-white px-3 py-2 text-left text-base shadow outline-none ring-2 ring-neutral-300 hover:ring-neutral-400 focus:border-blue-600 focus:ring-2 focus:ring-primary-500"
          :to="{ name: 'DocumentSearch', query: { ...query, at: id } }"
          :after-click="afterClick"
        >
          {{ query.q }}
        </RouterLink>
        <div class="grid grid-cols-2 gap-x-1">
          <ButtonLink class="px-3.5" :disabled="!prevNext.previous" :to="{ name: 'DocumentGet', params: { id: prevNext.previous }, query: { s: query.s } }">
            <ChevronLeftIcon class="h-5 w-5 sm:hidden" alt="Prev" />
            <span class="hidden sm:inline">Prev</span>
          </ButtonLink>
          <ButtonLink class="px-3.5" :disabled="!prevNext.next" :to="{ name: 'DocumentGet', params: { id: prevNext.next }, query: { s: query.s } }">
            <ChevronRightIcon class="h-5 w-5 sm:hidden" alt="Next" />
            <span class="hidden sm:inline">Next</span>
          </ButtonLink>
        </div>
      </div>
      <NavBarSearch v-else />
    </NavBar>
  </Teleport>
  <div class="mt-12 flex w-full flex-col gap-y-1 border-t border-transparent p-1 sm:mt-[4.5rem] sm:gap-y-4 sm:p-4">
    <div class="rounded border bg-white p-4 shadow">
      <div v-if="hasLoaded">
        <!--
          TODO: Consider using manual mode and draw ring only around the focused Tab component.
          See: https://github.com/tailwindlabs/headlessui/issues/1881
        -->
        <TabGroup>
          <TabList
            class="-m-4 mb-4 flex border-collapse flex-row rounded-t border-b bg-slate-100 focus-within:ring-2 focus-within:ring-primary-500 focus-within:ring-offset-1"
          >
            <Tab
              v-if="article"
              class="select-none border-r px-4 py-3 font-medium uppercase leading-tight outline-none first:rounded-tl ui-selected:bg-white ui-not-selected:hover:bg-slate-50"
              >Article</Tab
            >
            <Tab
              v-if="file"
              class="select-none border-r px-4 py-3 font-medium uppercase leading-tight outline-none first:rounded-tl-md ui-selected:bg-white ui-not-selected:hover:bg-slate-50"
              >File</Tab
            >
            <Tab
              class="select-none border-r px-4 py-3 font-medium uppercase leading-tight outline-none first:rounded-tl ui-selected:bg-white ui-not-selected:hover:bg-slate-50"
              >All properties</Tab
            >
          </TabList>
          <h1 class="mb-4 text-4xl font-bold drop-shadow-sm">{{ doc.name?.en }}</h1>
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
                  <PropertiesRows :properties="doc.claims" />
                </tbody>
              </table>
            </TabPanel>
          </TabPanels>
        </TabGroup>
      </div>
      <div v-else class="flex animate-pulse">
        <div class="flex-1 space-y-4">
          <div class="h-2 w-72 rounded bg-slate-200"></div>
          <div class="grid grid-cols-5 gap-4">
            <div class="col-span-1 h-2 rounded bg-slate-200"></div>
            <div class="col-span-2 h-2 rounded bg-slate-200"></div>
          </div>
        </div>
      </div>
    </div>
  </div>
  <Teleport to="footer">
    <Footer class="border-t border-slate-50 bg-slate-200 shadow" />
  </Teleport>
</template>
