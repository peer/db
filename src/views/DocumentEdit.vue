<script setup lang="ts">
import type { PeerDBDocument, DocumentEndEditResponse, DocumentBeginMetadata } from "@/types"

import { ref, computed, watch, readonly } from "vue"
import { useRouter, useRoute } from "vue-router"
import { CheckIcon } from "@heroicons/vue/20/solid"
import Button from "@/components/Button.vue"
import NavBar from "@/partials/NavBar.vue"
import Footer from "@/partials/Footer.vue"
import NavBarSearch from "@/partials/NavBarSearch.vue"
import PropertiesRows from "@/partials/PropertiesRows.vue"
import { getName, anySignal, encodeQuery } from "@/utils"
import { injectProgress } from "@/progress"
import { getURL, postJSON } from "@/api"

const props = defineProps<{
  id: string
  session: string
}>()

const route = useRoute()
const router = useRouter()

const progress = injectProgress()
const saveProgress = injectProgress()

const abortController = new AbortController()

const _doc = ref<PeerDBDocument | null>(null)
const _error = ref<string | null>(null)
const doc = import.meta.env.DEV ? readonly(_doc) : _doc
const error = import.meta.env.DEV ? readonly(_error) : _error

const initialRouteName = route.name
watch(
  props,
  async (newProps, oldProps, onCleanup) => {
    // Watch can continue to run for some time after the route changes.
    if (initialRouteName !== route.name) {
      return
    }

    // We want to eagerly remove any error.
    _error.value = null

    const controller = new AbortController()
    onCleanup(() => controller.abort())
    const signal = anySignal(abortController.signal, controller.signal)
    let data
    try {
      const { doc: beginMetadata } = await getURL<DocumentBeginMetadata>(
        router.apiResolve({
          name: "DocumentEdit",
          params: {
            id: newProps.id,
            session: newProps.session,
          },
        }).href,
        null,
        signal,
        progress,
      )
      if (signal.aborted) {
        return
      }

      data = await getURL<PeerDBDocument>(
        router.apiResolve({
          name: "DocumentGet",
          params: {
            id: newProps.id,
          },
          query: encodeQuery({ version: beginMetadata.version }),
        }).href,
        null,
        signal,
        progress,
      )
      if (signal.aborted) {
        return
      }
    } catch (err) {
      if (signal.aborted) {
        return
      }
      console.error("DocumentEdit", err)
      _doc.value = null
      _error.value = `${err}`
      return
    }
    if (signal.aborted) {
      return
    }
    _doc.value = data.doc
  },
  {
    immediate: true,
  },
)

const docName = computed(() => getName(doc.value?.claims))

async function onSave() {
  if (abortController.signal.aborted) {
    return
  }

  saveProgress.value += 1
  try {
    await postJSON<DocumentEndEditResponse>(
      router.apiResolve({
        name: "DocumentEndEdit",
        params: {
          session: props.session,
        },
      }).href,
      {},
      abortController.signal,
      saveProgress,
    )
    if (abortController.signal.aborted) {
      return
    }
    await router.push({
      name: "DocumentGet",
      params: {
        id: props.id,
      },
    })
  } catch (err) {
    if (abortController.signal.aborted) {
      return
    }
    // TODO: Show notification with error.
    console.error("DocumentEdit.onSave", err)
  } finally {
    saveProgress.value -= 1
  }
}
</script>

<template>
  <Teleport to="header">
    <NavBar>
      <NavBarSearch />
      <Button :progress="saveProgress" type="button" primary class="!px-3.5" @click.prevent="onSave">
        <CheckIcon class="h-5 w-5 sm:hidden" alt="Save" />
        <span class="hidden sm:inline">Save</span>
      </Button>
    </NavBar>
  </Teleport>
  <div class="mt-12 flex w-full flex-col gap-y-1 border-t border-transparent p-1 sm:mt-[4.5rem] sm:gap-y-4 sm:p-4">
    <div class="rounded border bg-white p-4 shadow">
      <template v-if="doc">
        <h1 class="mb-4 text-4xl font-bold drop-shadow-sm" v-html="docName || '<i>no name</i>'"></h1>
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
      </template>
      <template v-else-if="error">
        <i class="text-error-600">loading data failed</i>
      </template>
    </div>
  </div>
  <Teleport to="footer">
    <Footer class="border-t border-slate-50 bg-slate-200 shadow" />
  </Teleport>
</template>
