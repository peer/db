<script setup lang="ts">
import type { PeerDBDocument } from "@/types"

import { ref, computed, watch, readonly } from "vue"
import { useRoute, useRouter } from "vue-router"
import { ChevronLeftIcon, ChevronRightIcon } from "@heroicons/vue/solid"
import Button from "@/components/Button.vue"
import NavBar from "@/components/NavBar.vue"
import Footer from "@/components/Footer.vue"
import NavBarSearch from "@/components/NavBarSearch.vue"
import PropertiesRows from "@/components/PropertiesRows.vue"
import { getDocument, useSearchState } from "@/search"

const props = defineProps({
  id: {
    type: String,
    required: true,
  },
})

const route = useRoute()
const router = useRouter()

const dataProgress = ref(0)

const _doc = ref<PeerDBDocument>({})
const doc = import.meta.env.DEV ? readonly(_doc) : _doc

watch(
  () => props.id,
  (id, oldId, onCleanup) => {
    const controller = new AbortController()
    onCleanup(() => controller.abort())

    getDocument(router, id, dataProgress, controller.signal).then((data) => {
      _doc.value = data
    })
  },
  {
    immediate: true,
  },
)

const hasLoaded = computed(() => "name" in doc.value)

const { results, query } = useSearchState(dataProgress, async (query) => {
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

async function onInputText() {
  const q = Object.assign({}, query.value, {
    at: props.id,
  })
  await router.push({
    name: "DocumentSearch",
    query: q,
  })
  document.getElementById("search-input-text")?.focus()
}

async function onPrevNext(id: string | null) {
  if (id == null) {
    return
  }

  await router.push({
    name: "DocumentGet",
    params: {
      id,
    },
    query: {
      s: query.value.s,
    },
  })
}
</script>

<template>
  <Teleport to="header">
    <NavBar :progress="dataProgress">
      <div v-if="route.query.s" class="flex flex-grow gap-x-1 sm:gap-x-4">
        <button
          class="max-w-xl flex-grow appearance-none rounded border-0 border-gray-500 bg-white px-3 py-2 text-left text-base shadow-sm outline-none ring-2 ring-neutral-300 hover:ring-neutral-400 focus:border-blue-600 focus:ring-2 focus:ring-primary-500"
          @click="onInputText"
        >
          {{ query.q }}
        </button>
        <div class="grid grid-cols-2 gap-x-1">
          <Button class="px-3.5" :disabled="!prevNext.previous" @click="onPrevNext(prevNext.previous)">
            <ChevronLeftIcon class="h-5 w-5 sm:hidden" alt="Prev" />
            <span class="hidden sm:inline">Prev</span>
          </Button>
          <Button class="px-3.5" :disabled="!prevNext.next" @click="onPrevNext(prevNext.next)">
            <ChevronRightIcon class="h-5 w-5 sm:hidden" alt="Next" />
            <span class="hidden sm:inline">Next</span>
          </Button>
        </div>
      </div>
      <NavBarSearch v-else />
    </NavBar>
  </Teleport>
  <div class="mt-12 flex w-full flex-col gap-y-1 border-t border-transparent p-1 sm:mt-[4.5rem] sm:gap-y-4 sm:p-4">
    <div class="rounded border bg-white p-4 shadow">
      <div v-if="hasLoaded">
        <h1 class="mb-4 text-4xl font-bold drop-shadow-sm">{{ doc.name.en }}</h1>
        <table class="w-full table-auto border-collapse">
          <thead>
            <tr>
              <th class="border-r border-slate-200 px-2 py-1 text-left font-bold">Property</th>
              <th class="border-l border-slate-200 px-2 py-1 text-left font-bold">Value</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="name in doc.otherNames?.en" :key="name">
              <td class="border-t border-r border-slate-200 px-2 py-1">also known as</td>
              <td class="border-t border-l border-slate-200 px-2 py-1">{{ name }}</td>
            </tr>
            <PropertiesRows :properties="doc.active" />
          </tbody>
        </table>
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
