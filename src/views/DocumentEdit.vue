<script setup lang="ts">
import type { DocumentBeginMetadata, DocumentEndEditResponse } from "@/types"

import { Tab, TabGroup, TabList, TabPanel, TabPanels } from "@headlessui/vue"
import { CheckIcon } from "@heroicons/vue/20/solid"
import { computed, onBeforeUnmount, readonly, ref } from "vue"
import { useI18n } from "vue-i18n"
import { useRouter } from "vue-router"

import { deleteFromCache, getURL, getURLDirect, postJSON } from "@/api"
import Button from "@/components/Button.vue"
import InputText from "@/components/InputText.vue"
import InputTime from "@/components/InputTime.vue"
import { AddClaimChange, changeFrom, D, RemoveClaimChange } from "@/document"
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

const claimTypes: ("id" | "string" | "html" | "amount" | "amountInterval" | "time" | "timeInterval" | "link" | "ref" | "has" | "none" | "unknown")[] = [
  "id",
  "string",
  "html",
  "amount",
  "amountInterval",
  "time",
  "timeInterval",
  "link",
  "ref",
  "has",
  "none",
  "unknown",
]
const claimType = ref<"id" | "string" | "html" | "amount" | "amountInterval" | "time" | "timeInterval" | "link" | "ref" | "has" | "none" | "unknown">("id")
const claimProp = ref("")
const claimValue = ref("")

const { t } = useI18n()
const router = useRouter()

const saveProgress = injectProgress()

const abortController = new AbortController()

onBeforeUnmount(() => {
  abortController.abort()
})

const _doc = ref<D | null>(null)
const doc = process.env.NODE_ENV !== "production" ? readonly(_doc) : _doc

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

  _doc.value = new D(initialDoc)

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
          await change.Apply(_doc.value!)
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
          html: claimValue.value,
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
      <Button :progress="saveProgress" type="button" primary class="px-3.5" @click.prevent="onSave">
        <CheckIcon class="size-5 sm:hidden" :alt="t('common.buttons.save')" />
        <span class="hidden sm:inline">{{ t("common.buttons.save") }}</span>
      </Button>
    </NavBar>
  </Teleport>
  <div class="pd-documentedit mt-12 flex w-full flex-col gap-y-1 border-t border-transparent p-1 sm:mt-[4.5rem] sm:gap-y-4 sm:p-4">
    <div class="rounded-sm border border-gray-200 bg-white p-4 shadow-sm">
      <template v-if="doc">
        <h1 class="mb-4 text-4xl font-bold drop-shadow-xs" v-html="docName || `<i>${t('common.values.noName')}</i>`"></h1>
        <table class="w-full table-auto border-collapse">
          <thead>
            <tr>
              <th class="border-r border-slate-200 px-2 py-1 text-left font-bold">{{ t("common.labels.property") }}</th>
              <th class="border-l border-slate-200 px-2 py-1 text-left font-bold">{{ t("common.labels.value") }}</th>
              <th class="flex max-w-fit flex-row gap-1"></th>
            </tr>
          </thead>
          <tbody>
            <PropertiesRows :claims="doc.claims" editable @edit-claim="onEditClaim" @remove-claim="onRemoveClaim" />
          </tbody>
        </table>
        <h2 class="mt-4 text-xl font-bold drop-shadow-xs">{{ t("views.DocumentEdit.addClaim") }}</h2>
        <TabGroup @change="onChangeTab">
          <TabList class="mt-4 flex border-collapse flex-row border border-gray-200 bg-slate-100">
            <Tab
              class="border-r border-gray-200 px-4 py-3 leading-tight font-medium uppercase outline-none select-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 ui-selected:bg-white ui-not-selected:hover:bg-slate-50"
              >{{ t("views.DocumentEdit.claimTypes.identifier") }}</Tab
            >
            <Tab
              class="border-r border-gray-200 px-4 py-3 leading-tight font-medium uppercase outline-none select-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 ui-selected:bg-white ui-not-selected:hover:bg-slate-50"
              >{{ t("views.DocumentEdit.claimTypes.string") }}</Tab
            >
            <Tab
              class="border-r border-gray-200 px-4 py-3 leading-tight font-medium uppercase outline-none select-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 ui-selected:bg-white ui-not-selected:hover:bg-slate-50"
              >{{ t("views.DocumentEdit.claimTypes.html") }}</Tab
            >
            <Tab
              class="border-r border-gray-200 px-4 py-3 leading-tight font-medium uppercase outline-none select-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 ui-selected:bg-white ui-not-selected:hover:bg-slate-50"
              >{{ t("views.DocumentEdit.claimTypes.amount") }}</Tab
            >
            <Tab
              class="border-r border-gray-200 px-4 py-3 leading-tight font-medium uppercase outline-none select-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 ui-selected:bg-white ui-not-selected:hover:bg-slate-50"
              >{{ t("views.DocumentEdit.claimTypes.amountInterval") }}</Tab
            >
            <Tab
              class="border-r border-gray-200 px-4 py-3 leading-tight font-medium uppercase outline-none select-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 ui-selected:bg-white ui-not-selected:hover:bg-slate-50"
              >{{ t("views.DocumentEdit.claimTypes.time") }}</Tab
            >
            <Tab
              class="border-r border-gray-200 px-4 py-3 leading-tight font-medium uppercase outline-none select-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 ui-selected:bg-white ui-not-selected:hover:bg-slate-50"
              >{{ t("views.DocumentEdit.claimTypes.timeInterval") }}</Tab
            >
            <Tab
              class="border-r border-gray-200 px-4 py-3 leading-tight font-medium uppercase outline-none select-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 ui-selected:bg-white ui-not-selected:hover:bg-slate-50"
              >{{ t("views.DocumentEdit.claimTypes.link") }}</Tab
            >
            <Tab
              class="border-r border-gray-200 px-4 py-3 leading-tight font-medium uppercase outline-none select-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 ui-selected:bg-white ui-not-selected:hover:bg-slate-50"
              >{{ t("views.DocumentEdit.claimTypes.reference") }}</Tab
            >
            <Tab
              class="border-r border-gray-200 px-4 py-3 leading-tight font-medium uppercase outline-none select-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 ui-selected:bg-white ui-not-selected:hover:bg-slate-50"
              >{{ t("views.DocumentEdit.claimTypes.has") }}</Tab
            >
            <Tab
              class="border-r border-gray-200 px-4 py-3 leading-tight font-medium uppercase outline-none select-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 ui-selected:bg-white ui-not-selected:hover:bg-slate-50"
              >{{ t("views.DocumentEdit.claimTypes.none") }}</Tab
            >
            <Tab
              class="border-r border-gray-200 px-4 py-3 leading-tight font-medium uppercase outline-none select-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 ui-selected:bg-white ui-not-selected:hover:bg-slate-50"
              >{{ t("views.DocumentEdit.claimTypes.unknown") }}</Tab
            >
          </TabList>
          <TabPanels>
            <!-- We explicitly disable tabbing. See: https://github.com/tailwindlabs/headlessui/discussions/1433 -->
            <TabPanel tabindex="-1" class="flex flex-col">
              <label for="identifier-property" class="mt-4 mb-1">{{ t("common.labels.property") }}</label>
              <InputText id="identifier-property" v-model="claimProp" class="min-w-0 flex-auto grow" />
              <label for="identifier-value" class="mt-4 mb-1">{{ t("common.labels.value") }}</label>
              <InputText id="identifier-value" v-model="claimValue" class="min-w-0 flex-auto grow" />
            </TabPanel>
            <TabPanel tabindex="-1" class="flex flex-col">
              <label for="string-property" class="mt-4 mb-1">{{ t("common.labels.property") }}</label>
              <InputText id="string-property" v-model="claimProp" class="min-w-0 flex-auto grow" />
              <label for="string-value" class="mt-4 mb-1">{{ t("views.DocumentEdit.labels.string") }}</label>
              <InputText id="string-value" v-model="claimValue" class="min-w-0 flex-auto grow" />
            </TabPanel>
            <TabPanel tabindex="-1" class="flex flex-col">
              <label for="html-property" class="mt-4 mb-1">{{ t("common.labels.property") }}</label>
              <InputText id="html-property" v-model="claimProp" class="min-w-0 flex-auto grow" />
              <label for="html-value" class="mt-4 mb-1">{{ t("views.DocumentEdit.labels.html") }}</label>
              <InputText id="html-value" v-model="claimValue" class="min-w-0 flex-auto grow" />
            </TabPanel>
            <TabPanel tabindex="-1" class="flex flex-col">
              <label for="amount-property" class="mt-4 mb-1">{{ t("common.labels.property") }}</label>
              <InputText id="amount-property" v-model="claimProp" class="min-w-0 flex-auto grow" />
              <label for="amount-value" class="mt-4 mb-1">{{ t("views.DocumentEdit.labels.amount") }}</label>
              <InputText id="amount-value" v-model="claimValue" class="min-w-0 flex-auto grow" />
            </TabPanel>
            <TabPanel tabindex="-1" class="flex flex-col">
              <label for="amountInterval-property" class="mt-4 mb-1">{{ t("common.labels.property") }}</label>
              <InputText id="amountInterval-property" class="min-w-0 flex-auto grow" />
              <label for="amountInterval-from" class="mt-4 mb-1">{{ t("views.DocumentEdit.labels.from") }}</label>
              <InputText id="amountInterval-from" class="min-w-0 flex-auto grow" />
              <label for="amountInterval-to" class="mt-4 mb-1">{{ t("views.DocumentEdit.labels.to") }}</label>
              <InputText id="amountInterval-to" class="min-w-0 flex-auto grow" />
            </TabPanel>
            <TabPanel tabindex="-1" class="flex flex-col">
              <label for="time-property" class="mt-4 mb-1">{{ t("common.labels.property") }}</label>
              <InputText id="time-property" v-model="claimProp" class="min-w-0 flex-auto grow" />
              <InputTime v-model="claimValue" class="mt-4 min-w-0 flex-auto grow" />
            </TabPanel>
            <TabPanel tabindex="-1" class="flex flex-col">
              <label for="timeInterval-property" class="mt-4 mb-1">{{ t("common.labels.property") }}</label>
              <InputText id="timeInterval-property" class="min-w-0 flex-auto grow" />
              <InputTime class="mt-4 min-w-0 flex-auto grow">
                <template #timestamp-label>{{ t("views.DocumentEdit.labels.from") }}</template>
              </InputTime>
              <InputTime class="mt-4 min-w-0 flex-auto grow">
                <template #timestamp-label>{{ t("views.DocumentEdit.labels.to") }}</template>
              </InputTime>
            </TabPanel>
            <TabPanel tabindex="-1" class="flex flex-col">
              <label for="link-property" class="mt-4 mb-1">{{ t("common.labels.property") }}</label>
              <InputText id="link-property" v-model="claimProp" class="min-w-0 flex-auto grow" />
              <label for="link-value" class="mt-4 mb-1">{{ t("views.DocumentEdit.labels.iri") }}</label>
              <InputText id="link-value" v-model="claimValue" class="min-w-0 flex-auto grow" />
            </TabPanel>
            <TabPanel tabindex="-1" class="flex flex-col">
              <label for="reference-property" class="mt-4 mb-1">{{ t("common.labels.property") }}</label>
              <InputText id="reference-property" v-model="claimProp" class="min-w-0 flex-auto grow" />
              <label for="reference-value" class="mt-4 mb-1">{{ t("views.DocumentEdit.labels.to") }}</label>
              <InputText id="reference-value" v-model="claimValue" class="min-w-0 flex-auto grow" />
            </TabPanel>
            <TabPanel tabindex="-1" class="flex flex-col">
              <label for="has-property" class="mt-4 mb-1">{{ t("common.labels.property") }}</label>
              <InputText id="has-property" v-model="claimProp" class="min-w-0 flex-auto grow" />
            </TabPanel>
            <TabPanel tabindex="-1" class="flex flex-col">
              <label for="none-property" class="mt-4 mb-1">{{ t("common.labels.property") }}</label>
              <InputText id="none-property" v-model="claimProp" class="min-w-0 flex-auto grow" />
            </TabPanel>
            <TabPanel tabindex="-1" class="flex flex-col">
              <label for="unknown-property" class="mt-4 mb-1">{{ t("common.labels.property") }}</label>
              <InputText id="unknown-property" v-model="claimProp" class="min-w-0 flex-auto grow" />
            </TabPanel>
          </TabPanels>
        </TabGroup>
        <Button type="button" class="mt-6" @click.prevent="onAddClaim">{{ t("common.buttons.add") }}</Button>
      </template>
    </div>
  </div>
  <Teleport to="footer">
    <Footer class="border-t border-slate-50 bg-slate-200 shadow-sm" />
  </Teleport>
</template>
