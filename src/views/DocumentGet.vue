<script setup lang="ts">
import type { PeerDBDocument } from "@/types"

import { ref, computed, watch, readonly } from "vue"
import { useRoute, useRouter } from "vue-router"
import InputText from "@/components/InputText.vue"
import NavBar from "@/components/NavBar.vue"
import Footer from "@/components/Footer.vue"
import PropertiesRows from "@/components/PropertiesRows.vue"
import { getDocument } from "@/search"

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

const hasLoaded = computed(() => Object.prototype.hasOwnProperty.call(doc.value, "name"))
</script>

<template>
  <Teleport to="header">
    <NavBar :progress="dataProgress">
      <form ref="form" class="flex flex-grow gap-x-1 sm:gap-x-4" @submit.prevent>
        <InputText name="q" class="max-w-xl flex-grow" :value="route.query.q" />
      </form>
    </NavBar>
  </Teleport>
  <div class="mt-12 flex w-full flex-col gap-y-1 border-t border-transparent p-1 sm:mt-[4.5rem] sm:gap-y-4 sm:p-4">
    <div class="rounded border bg-white p-4 shadow">
      <div v-if="hasLoaded">
        <h1 class="mb-4 text-4xl font-medium drop-shadow-sm">{{ doc.name.en }}</h1>
        <table class="w-full table-auto border-collapse">
          <thead>
            <tr>
              <th class="border-r border-slate-200 px-2 py-1 text-left font-medium">Property</th>
              <th class="border-l border-slate-200 px-2 py-1 text-left font-medium">Value</th>
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
