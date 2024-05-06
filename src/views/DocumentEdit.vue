<script setup lang="ts">
import type { DocumentEndEditResponse, DocumentBeginMetadata } from "@/types"

import { ref, computed, readonly } from "vue"
import { useRouter } from "vue-router"
import { CheckIcon } from "@heroicons/vue/20/solid"
import Button from "@/components/Button.vue"
import NavBar from "@/partials/NavBar.vue"
import Footer from "@/partials/Footer.vue"
import NavBarSearch from "@/partials/NavBarSearch.vue"
import PropertiesRows from "@/partials/PropertiesRows.vue"
import { changeFrom, PeerDBDocument, RemoveClaimChange, idAtChange } from "@/document"
import { getName, encodeQuery } from "@/utils"
import { injectProgress } from "@/progress"
import { getURL, postJSON, getURLDirect, deleteFromCache } from "@/api"

const props = defineProps<{
  id: string
  session: string
}>()

const router = useRouter()

const saveProgress = injectProgress()

const abortController = new AbortController()

const _doc = ref<PeerDBDocument | null>(null)
const doc = import.meta.env.DEV ? readonly(_doc) : _doc

let latestChange = 0

;(async () => {
  const { doc: beginMetadata } = await getURL<DocumentBeginMetadata>(
    router.apiResolve({
      name: "DocumentEdit",
      params: {
        id: props.id,
        session: props.session,
      },
    }).href,
    null,
    abortController.signal,
    null,
  )
  if (abortController.signal.aborted) {
    return
  }

  const { doc: initialDoc } = await getURL<object>(
    router.apiResolve({
      name: "DocumentGet",
      params: {
        id: props.id,
      },
      query: encodeQuery({ version: beginMetadata.version }),
    }).href,
    null,
    abortController.signal,
    null,
  )
  if (abortController.signal.aborted) {
    return
  }

  _doc.value = new PeerDBDocument(initialDoc)

  let running = false
  const timer = setInterval(async () => {
    if (running) {
      return
    }
    running = true
    try {
      const { doc: changesList } = await getURLDirect<number[]>(
        router.apiResolve({
          name: "DocumentListChanges",
          params: {
            session: props.session,
          },
        }).href,
        abortController.signal,
        null,
      )
      if (abortController.signal.aborted) {
        return
      }
      for (; changesList.length > 0 && latestChange < changesList[0]; latestChange++) {
        const { doc: changeDoc } = await getURL<object>(
          router.apiResolve({
            name: "DocumentGetChange",
            params: {
              session: props.session,
              change: latestChange + 1,
            },
          }).href,
          null,
          abortController.signal,
          null,
        )
        if (abortController.signal.aborted) {
          return
        }
        const change = changeFrom(changeDoc)
        change.Apply(_doc.value!, idAtChange(props.session, latestChange + 1))
      }
    } finally {
      running = false
    }
  }, 1000)
  abortController.signal.addEventListener("abort", () => clearTimeout(timer))
})()

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
    deleteFromCache(
      router.apiResolve({
        name: "DocumentGet",
        params: {
          id: props.id,
        },
      }).href,
    )
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

async function onAddClaim() {
  if (abortController.signal.aborted) {
    return
  }
}

async function onEditClaim(id: string) {
  if (abortController.signal.aborted) {
    return
  }

  console.log("edit", id)
}

async function onRemoveClaim(id: string) {
  if (abortController.signal.aborted) {
    return
  }

  try {
    await postJSON<DocumentEndEditResponse>(
      router.apiResolve({
        name: "DocumentSaveChange",
        params: {
          session: props.session,
        },
        query: encodeQuery({ change: String(latestChange + 1) }),
      }).href,
      new RemoveClaimChange({
        id,
      }),
      abortController.signal,
      null,
    )
    if (abortController.signal.aborted) {
      return
    }
  } catch (err) {
    if (abortController.signal.aborted) {
      return
    }
    // TODO: Show notification with error.
    console.error("DocumentEdit.onRemoveClaim", err)
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
              <th class="flex flex-row gap-1 max-w-fit"></th>
            </tr>
          </thead>
          <tbody>
            <PropertiesRows :claims="doc.claims" editable @edit-claim="onEditClaim" @remove-claim="onRemoveClaim" />
          </tbody>
        </table>
        <Button type="button" class="mt-4" @click.prevent="onAddClaim">Add claim</Button>
      </template>
    </div>
  </div>
  <Teleport to="footer">
    <Footer class="border-t border-slate-50 bg-slate-200 shadow" />
  </Teleport>
</template>
