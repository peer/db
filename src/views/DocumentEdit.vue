<script setup lang="ts">
import type { DocumentBeginMetadata, DocumentEndEditResponse } from "@/types"

import { Tab, TabGroup, TabList, TabPanel, TabPanels } from "@headlessui/vue"
import { CheckIcon } from "@heroicons/vue/20/solid"
import { computed, onBeforeUnmount, readonly, ref } from "vue"
import { useRouter } from "vue-router"

import { deleteFromCache, getURL, getURLDirect, postJSON } from "@/api"
import Button from "@/components/Button.vue"
import InputText from "@/components/InputText.vue"
import { AddClaimChange, changeFrom, idAtChange, PeerDBDocument, RemoveClaimChange } from "@/document"
import Footer from "@/partials/Footer.vue"
import NavBar from "@/partials/NavBar.vue"
import NavBarSearch from "@/partials/NavBarSearch.vue"
import PropertiesRows from "@/partials/PropertiesRows.vue"
import { injectProgress } from "@/progress"
import { encodeQuery, getName } from "@/utils"

const props = defineProps<{
  id: string
  session: string
}>()

const claimTypes: ("id" | "ref" | "text" | "string" | "amount" | "amountRange" | "rel" | "file" | "none" | "unknown" | "time" | "timeRange")[] = [
  "id",
  "ref",
  "text",
  "string",
  "amount",
  "amountRange",
  "rel",
  "file",
  "none",
  "unknown",
  "time",
  "timeRange",
]
const claimType = ref<"id" | "ref" | "text" | "string" | "amount" | "amountRange" | "rel" | "file" | "none" | "unknown" | "time" | "timeRange">("id")
const claimProp = ref("")
const claimValue = ref("")

const router = useRouter()

const saveProgress = injectProgress()

const abortController = new AbortController()

onBeforeUnmount(() => {
  abortController.abort()
})

const _doc = ref<PeerDBDocument | null>(null)
const doc = import.meta.env.DEV ? readonly(_doc) : _doc

let latestChange = 0

async function loadAndSubscribe() {
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

  // TODO: Use websocket to watch for new changes.
  let running = false
  const timer = setInterval(() => {
    ;(async () => {
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
    })().catch((error) => {
      // TODO: Show error state to the user.
      console.error("loadAndSubscribe interval", error)
    })
  }, 1000)
  abortController.signal.addEventListener("abort", () => clearTimeout(timer))
}
loadAndSubscribe().catch((error) => {
  // TODO: Show error state to the user.
  console.error("loadAndSubscribe", error)
})

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

  try {
    await postJSON(
      router.apiResolve({
        name: "DocumentSaveChange",
        params: {
          session: props.session,
        },
        query: encodeQuery({ change: String(latestChange + 1) }),
      }).href,
      new AddClaimChange({
        patch: {
          // TODO: Make more specific for each patch.
          type: claimType.value,
          prop: claimProp.value,
          value: claimValue.value,
          iri: claimValue.value,
          html: {
            en: claimValue.value,
          },
          string: claimValue.value,
          amount: claimValue.value,
          to: claimValue.value,
          timestamp: claimValue.value,
        },
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
    console.error("DocumentEdit.onAddClaim", err)
  }
}

function onEditClaim(id: string) {
  if (abortController.signal.aborted) {
    return
  }

  // TODO: Implement.
  console.log("edit", id)
}

async function onRemoveClaim(id: string) {
  if (abortController.signal.aborted) {
    return
  }

  try {
    await postJSON(
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

function onChangeTab(index: number) {
  if (abortController.signal.aborted) {
    return
  }

  claimType.value = claimTypes[index]
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
    <div class="rounded border bg-white p-4 shadow-sm">
      <template v-if="doc">
        <h1 class="mb-4 text-4xl font-bold drop-shadow-xs" v-html="docName || '<i>no name</i>'"></h1>
        <table class="w-full table-auto border-collapse">
          <thead>
            <tr>
              <th class="border-r border-slate-200 px-2 py-1 text-left font-bold">Property</th>
              <th class="border-l border-slate-200 px-2 py-1 text-left font-bold">Value</th>
              <th class="flex max-w-fit flex-row gap-1"></th>
            </tr>
          </thead>
          <tbody>
            <PropertiesRows :claims="doc.claims" editable @edit-claim="onEditClaim" @remove-claim="onRemoveClaim" />
          </tbody>
        </table>
        <h2 class="mt-4 text-xl font-bold drop-shadow-xs">Add claim</h2>
        <TabGroup @change="onChangeTab">
          <TabList class="mt-4 flex border-collapse flex-row border bg-slate-100">
            <Tab
              class="border-r px-4 py-3 leading-tight font-medium uppercase outline-none select-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 ui-selected:bg-white ui-not-selected:hover:bg-slate-50"
              >Identifier</Tab
            >
            <Tab
              class="border-r px-4 py-3 leading-tight font-medium uppercase outline-none select-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 ui-selected:bg-white ui-not-selected:hover:bg-slate-50"
              >Reference</Tab
            >
            <Tab
              class="border-r px-4 py-3 leading-tight font-medium uppercase outline-none select-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 ui-selected:bg-white ui-not-selected:hover:bg-slate-50"
              >Text</Tab
            >
            <Tab
              class="border-r px-4 py-3 leading-tight font-medium uppercase outline-none select-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 ui-selected:bg-white ui-not-selected:hover:bg-slate-50"
              >String</Tab
            >
            <Tab
              class="border-r px-4 py-3 leading-tight font-medium uppercase outline-none select-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 ui-selected:bg-white ui-not-selected:hover:bg-slate-50"
              >Amount</Tab
            >
            <Tab
              class="border-r px-4 py-3 leading-tight font-medium uppercase outline-none select-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 ui-selected:bg-white ui-not-selected:hover:bg-slate-50"
              >Amount range</Tab
            >
            <Tab
              class="border-r px-4 py-3 leading-tight font-medium uppercase outline-none select-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 ui-selected:bg-white ui-not-selected:hover:bg-slate-50"
              >Relation</Tab
            >
            <Tab
              class="border-r px-4 py-3 leading-tight font-medium uppercase outline-none select-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 ui-selected:bg-white ui-not-selected:hover:bg-slate-50"
              >File</Tab
            >
            <Tab
              class="border-r px-4 py-3 leading-tight font-medium uppercase outline-none select-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 ui-selected:bg-white ui-not-selected:hover:bg-slate-50"
              >No value</Tab
            >
            <Tab
              class="border-r px-4 py-3 leading-tight font-medium uppercase outline-none select-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 ui-selected:bg-white ui-not-selected:hover:bg-slate-50"
              >Unknown value</Tab
            >
            <Tab
              class="border-r px-4 py-3 leading-tight font-medium uppercase outline-none select-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 ui-selected:bg-white ui-not-selected:hover:bg-slate-50"
              >Time</Tab
            >
            <Tab
              class="border-r px-4 py-3 leading-tight font-medium uppercase outline-none select-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 ui-selected:bg-white ui-not-selected:hover:bg-slate-50"
              >Time range</Tab
            >
          </TabList>
          <TabPanels>
            <!-- We explicitly disable tabbing. See: https://github.com/tailwindlabs/headlessui/discussions/1433 -->
            <TabPanel tabindex="-1" class="flex flex-col">
              <label for="identifier-property" class="mt-4 mb-1">Property</label>
              <InputText id="identifier-property" v-model="claimProp" class="min-w-0 flex-auto flex-grow" />
              <label for="identifier-property" class="mt-4 mb-1">Value</label>
              <InputText id="identifier-property" v-model="claimValue" class="min-w-0 flex-auto flex-grow" />
            </TabPanel>
            <TabPanel tabindex="-1" class="flex flex-col">
              <label for="identifier-property" class="mt-4 mb-1">Property</label>
              <InputText id="identifier-property" v-model="claimProp" class="min-w-0 flex-auto flex-grow" />
              <label for="identifier-property" class="mt-4 mb-1">IRI</label>
              <InputText id="identifier-property" v-model="claimValue" class="min-w-0 flex-auto flex-grow" />
            </TabPanel>
            <TabPanel tabindex="-1" class="flex flex-col">
              <label for="identifier-property" class="mt-4 mb-1">Property</label>
              <InputText id="identifier-property" v-model="claimProp" class="min-w-0 flex-auto flex-grow" />
              <label for="identifier-property" class="mt-4 mb-1">Text</label>
              <InputText id="identifier-property" v-model="claimValue" class="min-w-0 flex-auto flex-grow" />
            </TabPanel>
            <TabPanel tabindex="-1" class="flex flex-col">
              <label for="identifier-property" class="mt-4 mb-1">Property</label>
              <InputText id="identifier-property" v-model="claimProp" class="min-w-0 flex-auto flex-grow" />
              <label for="identifier-property" class="mt-4 mb-1">String</label>
              <InputText id="identifier-property" v-model="claimValue" class="min-w-0 flex-auto flex-grow" />
            </TabPanel>
            <TabPanel tabindex="-1" class="flex flex-col">
              <label for="identifier-property" class="mt-4 mb-1">Property</label>
              <InputText id="identifier-property" v-model="claimProp" class="min-w-0 flex-auto flex-grow" />
              <label for="identifier-property" class="mt-4 mb-1">Amount</label>
              <InputText id="identifier-property" v-model="claimValue" class="min-w-0 flex-auto flex-grow" />
            </TabPanel>
            <TabPanel tabindex="-1" class="flex flex-col">
              <label for="identifier-property" class="mt-4 mb-1">Property</label>
              <InputText id="identifier-property" class="min-w-0 flex-auto flex-grow" />
              <label for="identifier-property" class="mt-4 mb-1">Lower</label>
              <InputText id="identifier-property" class="min-w-0 flex-auto flex-grow" />
              <label for="identifier-property" class="mt-4 mb-1">Upper</label>
              <InputText id="identifier-property" class="min-w-0 flex-auto flex-grow" />
            </TabPanel>
            <TabPanel tabindex="-1" class="flex flex-col">
              <label for="identifier-property" class="mt-4 mb-1">Property</label>
              <InputText id="identifier-property" v-model="claimProp" class="min-w-0 flex-auto flex-grow" />
              <label for="identifier-property" class="mt-4 mb-1">To</label>
              <InputText id="identifier-property" v-model="claimValue" class="min-w-0 flex-auto flex-grow" />
            </TabPanel>
            <TabPanel tabindex="-1" class="flex flex-col">
              <label for="identifier-property" class="mt-4 mb-1">Property</label>
              <InputText id="identifier-property" class="min-w-0 flex-auto flex-grow" />
              <label for="identifier-property" class="mt-4 mb-1">Media type</label>
              <InputText id="identifier-property" class="min-w-0 flex-auto flex-grow" />
              <label for="identifier-property" class="mt-4 mb-1">URL</label>
              <InputText id="identifier-property" class="min-w-0 flex-auto flex-grow" />
              <label for="identifier-property" class="mt-4 mb-1">Preview URL</label>
              <InputText id="identifier-property" class="min-w-0 flex-auto flex-grow" />
            </TabPanel>
            <TabPanel tabindex="-1" class="flex flex-col">
              <label for="identifier-property" class="mt-4 mb-1">Property</label>
              <InputText id="identifier-property" v-model="claimProp" class="min-w-0 flex-auto flex-grow" />
            </TabPanel>
            <TabPanel tabindex="-1" class="flex flex-col">
              <label for="identifier-property" class="mt-4 mb-1">Property</label>
              <InputText id="identifier-property" v-model="claimProp" class="min-w-0 flex-auto flex-grow" />
            </TabPanel>
            <TabPanel tabindex="-1" class="flex flex-col">
              <label for="identifier-property" class="mt-4 mb-1">Property</label>
              <InputText id="identifier-property" v-model="claimProp" class="min-w-0 flex-auto flex-grow" />
              <label for="identifier-property" class="mt-4 mb-1">Timestamp</label>
              <InputText id="identifier-property" v-model="claimValue" class="min-w-0 flex-auto flex-grow" />
            </TabPanel>
            <TabPanel tabindex="-1" class="flex flex-col">
              <label for="identifier-property" class="mt-4 mb-1">Property</label>
              <InputText id="identifier-property" class="min-w-0 flex-auto flex-grow" />
              <label for="identifier-property" class="mt-4 mb-1">Lower</label>
              <InputText id="identifier-property" class="min-w-0 flex-auto flex-grow" />
              <label for="identifier-property" class="mt-4 mb-1">Upper</label>
              <InputText id="identifier-property" class="min-w-0 flex-auto flex-grow" />
            </TabPanel>
          </TabPanels>
        </TabGroup>
        <Button type="button" class="mt-6" @click.prevent="onAddClaim">Add</Button>
      </template>
    </div>
  </div>
  <Teleport to="footer">
    <Footer class="border-t border-slate-50 bg-slate-200 shadow-sm" />
  </Teleport>
</template>
