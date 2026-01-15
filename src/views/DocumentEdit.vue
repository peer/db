<script setup lang="ts">
import type { TimePrecision } from "@/document"
import type { FieldsFormSaveChange, FlushFn } from "@/fields"
import type { DocumentBeginMetadata, DocumentEditStatus, DocumentEndEditResponse } from "@/types"

import { Tab, TabGroup, TabList, TabPanel, TabPanels } from "@headlessui/vue"
import { CheckIcon } from "@heroicons/vue/20/solid"
import { onBeforeUnmount, provide, readonly, ref, toRef, useTemplateRef, watch } from "vue"
import { useI18n } from "vue-i18n"
import { useRouter } from "vue-router"

import { deleteFromCache, getURL, getURLDirect, postJSON } from "@/api"
import Button from "@/components/Button.vue"
import InputRel from "@/components/InputRel.vue"
import InputText from "@/components/InputText.vue"
import siteContext from "@/context"
import { D, HighConfidence } from "@/document"
import { changeFrom, RemoveClaimChange } from "@/document/patch"
import { getNextChangeNumberKey, registerForFlushKey, saveChangeKey, unregisterForFlushKey } from "@/fields"
import DocumentRefInline from "@/partials/DocumentRefInline.vue"
import FieldsForm from "@/partials/FieldsForm.vue"
import Footer from "@/partials/Footer.vue"
import InputTime from "@/partials/input/InputTime.vue"
import NavBar from "@/partials/NavBar.vue"
import NavBarSearch from "@/partials/NavBarSearch.vue"
import PropertiesRows from "@/partials/PropertiesRows.vue"
import { getParentProgress, localProgress } from "@/progress"
import { useDocumentFields } from "@/useDocumentFields"
import { useParentClasses } from "@/useParentClasses"
import { encodeQuery, makeAddClaimChange } from "@/utils"

const props = defineProps<{
  id: string
  session: string
}>()

const addClaimTypes: ("id" | "string" | "html" | "amount" | "amountInterval" | "time" | "timeInterval" | "link" | "ref" | "has" | "none" | "unknown")[] = [
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
const claimAmountPrecision = ref("")
const claimTimePrecision = ref<TimePrecision>("y")
const claimFrom = ref("")
const claimFromAmountPrecision = ref("")
const claimFromTimePrecision = ref<TimePrecision>("y")
const claimTo = ref("")
const claimToAmountPrecision = ref("")
const claimToTimePrecision = ref<TimePrecision>("y")

const { t } = useI18n({ useScope: "global" })
const router = useRouter()

const parentProgress = getParentProgress()
const progress = localProgress(parentProgress)
const editProgress = localProgress(parentProgress)

const el = useTemplateRef<HTMLElement>("el")

let abortController = new AbortController()

function cleanup() {
  abortController.abort()
}

onBeforeUnmount(() => {
  cleanup()
})

const _doc = ref<D | null>(null)
const doc = process.env.NODE_ENV !== "production" ? readonly(_doc) : _doc

// Tracks the change number which was committed in the backend.
let committedChange = 0
// Tracks the next change number to submit (may be ahead of committedChange when changes are in-flight).
let nextChangeToSubmit = 1

const fieldsFormInvalid = ref(false)

// Flush registry: all FieldsForm instances register here so we can flush them before save.
const flushRegistry = new Set<FlushFn>()

// Provide shared services for recursive FieldsForm instances.
provide(getNextChangeNumberKey, () => nextChangeToSubmit++)
provide(saveChangeKey, async (change: object, changeNumber: number) => {
  await postJSON(
    router.apiResolve({
      name: "DocumentSaveChange",
      params: { session: props.session },
      query: encodeQuery({ change: String(changeNumber) }),
    }).href,
    change,
    abortController.signal,
    null,
  )
})
provide(registerForFlushKey, (instance: FlushFn) => {
  flushRegistry.add(instance)
})
provide(unregisterForFlushKey, (instance: FlushFn) => {
  flushRegistry.delete(instance)
})

// Poll interval in milliseconds.
const pollInterval = 1000

// Resolve field definitions for the document's class(es).
const docRef = toRef(() => doc.value ?? null)
const { classDocs, instanceOfClassIds, initialized: classesInitialized } = useParentClasses(docRef, el, progress)
const { fieldsData: mergedFieldsData, classTabId } = useDocumentFields(classDocs, instanceOfClassIds)

let running = false
async function loadChanges() {
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
    for (; changesList.length > 0 && committedChange < changesList[0]; committedChange++) {
      const { doc: changeDoc } = await getURL<object>(
        router.apiResolve({
          name: "DocumentGetChange",
          params: {
            session: props.session,
            change: committedChange + 1,
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
}

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
  const timer = setInterval(() => {
    loadChanges().catch((error) => {
      // TODO: Show error state to the user.
      console.error("loadAndSubscribe interval", error)
    })
  }, pollInterval)
  abortController.signal.addEventListener("abort", () => {
    clearInterval(timer)
  })
  // Load initial changes.
  await loadChanges()
  // Initialize next change counter after loading existing changes.
  nextChangeToSubmit = committedChange + 1
}
// Re-initialize when route params change.
watch(
  () => ({ id: props.id, session: props.session }),
  () => {
    // Abort previous session's work.
    cleanup()
    abortController = new AbortController()

    // Reset state.
    _doc.value = null
    committedChange = 0
    nextChangeToSubmit = 1
    fieldsFormInvalid.value = false

    loadAndSubscribe().catch((error) => {
      // TODO: Show error state to the user.
      console.error("loadAndSubscribe", error)
    })
  },
  // Initialize the first time.
  {
    immediate: true,
  },
)

async function onSave() {
  if (abortController.signal.aborted) {
    return
  }

  // Flush any pending edits from all FieldsForm instances before saving.
  // Flush returns only valid changes; invalid fields remain and set fieldsFormInvalid.
  const allPendingChanges: FieldsFormSaveChange[] = []
  for (const flush of flushRegistry) {
    const changes = await flush()
    allPendingChanges.push(...changes)
  }

  // Post all flushed changes first (they are valid and have consumed change numbers).
  for (const { change, changeNumber } of allPendingChanges) {
    await postJSON(
      router.apiResolve({
        name: "DocumentSaveChange",
        params: { session: props.session },
        query: encodeQuery({ change: String(changeNumber) }),
      }).href,
      change,
      abortController.signal,
      null,
    )
    if (abortController.signal.aborted) {
      return
    }
  }

  // Check if any FieldsForm is invalid after flush. Abort save but keep the valid changes posted above.
  if (fieldsFormInvalid.value) {
    return
  }

  // Stop polling for changes before ending the session by aborting and creating a fresh controller.
  // The fresh controller is needed for the save request itself.
  abortController.abort()
  abortController = new AbortController()

  editProgress.value += 1
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
      editProgress,
    )
    if (abortController.signal.aborted) {
      return
    }

    // Poll until the session is fully completed (document committed).
    const editStatusURL = router.apiResolve({
      name: "DocumentEdit",
      params: {
        id: props.id,
        session: props.session,
      },
    }).href
    while (true) {
      await new Promise((resolve) => setTimeout(resolve, pollInterval))
      if (abortController.signal.aborted) {
        return
      }
      const { doc: status } = await getURLDirect<DocumentEditStatus>(editStatusURL, abortController.signal, null)
      if (abortController.signal.aborted) {
        return
      }
      if (status.changeset || status.discarded) {
        break
      }
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
    editProgress.value -= 1
  }
}

function makePatch(): object {
  const shared = { type: claimType.value, confidence: HighConfidence, prop: claimProp.value }
  switch (claimType.value) {
    case "id":
      return { ...shared, value: claimValue.value }
    case "string":
      return { ...shared, string: claimValue.value }
    case "html":
      return { ...shared, html: claimValue.value }
    case "amount":
      return { ...shared, amount: claimValue.value, precision: parseFloat(claimAmountPrecision.value) }
    case "amountInterval":
      return {
        ...shared,
        ...(claimFrom.value ? { from: claimFrom.value, fromPrecision: parseFloat(claimFromAmountPrecision.value) } : {}),
        ...(claimTo.value ? { to: claimTo.value, toPrecision: parseFloat(claimToAmountPrecision.value) } : {}),
      }
    case "time":
      return { ...shared, time: claimValue.value, precision: claimTimePrecision.value }
    case "timeInterval":
      return {
        ...shared,
        ...(claimFrom.value ? { from: claimFrom.value, fromPrecision: claimFromTimePrecision.value } : {}),
        ...(claimTo.value ? { to: claimTo.value, toPrecision: claimToTimePrecision.value } : {}),
      }
    case "link":
      return { ...shared, iri: claimValue.value }
    case "ref":
      return { ...shared, to: claimValue.value }
    case "has":
    case "none":
    case "unknown":
      return shared
    default:
      // eslint-disable-next-line @typescript-eslint/restrict-template-expressions
      throw new Error(`unsupported claim type: ${claimType.value}`)
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
        query: encodeQuery({ change: String(committedChange + 1) }),
      }).href,
      await makeAddClaimChange(doc.value!.base, props.session, committedChange + 1, makePatch()),
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
        query: encodeQuery({ change: String(committedChange + 1) }),
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

function onChangeAddClaimTab(index: number) {
  if (abortController.signal.aborted) {
    return
  }

  claimType.value = addClaimTypes[index]
}

function canSave(): boolean {
  return !fieldsFormInvalid.value
}
</script>

<template>
  <Teleport to="header">
    <NavBar>
      <template #start>
        <NavBarSearch />
      </template>
      <template #end>
        <Button
          v-if="doc && (siteContext.features.editButtons || (classTabId && mergedFieldsData))"
          :progress="editProgress"
          type="button"
          primary
          class="px-3.5"
          :disabled="!canSave()"
          @click.prevent="onSave"
        >
          <CheckIcon class="size-5 sm:hidden" :alt="t('common.buttons.save')" />
          <span class="hidden sm:inline">{{ t("common.buttons.save") }}</span>
        </Button>
      </template>
    </NavBar>
  </Teleport>
  <div ref="el" class="pd-documentedit mt-12 flex w-full flex-col gap-y-1 border-t border-transparent p-1 sm:mt-[4.5rem] sm:gap-y-4 sm:p-4">
    <div class="rounded-sm border border-gray-200 bg-white p-4 shadow-sm">
      <template v-if="doc && (siteContext.features.editButtons || (classTabId && mergedFieldsData))">
        <!--
          TODO: Fix how hover interacts with focused tab.
          See: https://github.com/tailwindlabs/tailwindcss/discussions/10123
        -->
        <TabGroup>
          <TabList class="-m-4 mb-4 flex border-collapse flex-row rounded-t border-b border-gray-200 bg-slate-100">
            <Tab
              v-if="classTabId && mergedFieldsData"
              :key="classTabId"
              class="border-r border-gray-200 px-4 py-3 leading-tight font-medium uppercase outline-none select-none first:rounded-tl focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 ui-selected:bg-white ui-not-selected:hover:bg-slate-50"
              ><DocumentRefInline :id="classTabId" :link="false"
            /></Tab>
            <Tab
              v-if="siteContext.features.editButtons"
              class="border-r border-gray-200 px-4 py-3 leading-tight font-medium uppercase outline-none select-none first:rounded-tl focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 ui-selected:bg-white ui-not-selected:hover:bg-slate-50"
              >{{ t("views.DocumentEdit.tabs.allProperties") }}</Tab
            >
          </TabList>
          <!-- We explicitly disable tabbing. See: https://github.com/tailwindlabs/headlessui/discussions/1433 -->
          <TabPanels>
            <!-- Class-specific tab. -->
            <TabPanel v-if="classTabId && mergedFieldsData" :key="classTabId" tabindex="-1">
              <FieldsForm
                v-model:invalid="fieldsFormInvalid"
                :fields-data="mergedFieldsData"
                :claims="doc.claims"
                :base="doc.base"
                :session="session"
                :progress="editProgress"
              />
            </TabPanel>
            <!-- "All properties" tab panel. -->
            <TabPanel v-if="siteContext.features.editButtons" tabindex="-1">
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
              <TabGroup @change="onChangeAddClaimTab">
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
                    <label for="amount-precision" class="mt-4 mb-1">{{ t("common.labels.precision") }}</label>
                    <InputText id="amount-precision" v-model="claimAmountPrecision" class="min-w-0 flex-auto grow" />
                  </TabPanel>
                  <TabPanel tabindex="-1" class="flex flex-col">
                    <label for="amountInterval-property" class="mt-4 mb-1">{{ t("common.labels.property") }}</label>
                    <InputText id="amountInterval-property" v-model="claimProp" class="min-w-0 flex-auto grow" />
                    <label for="amountInterval-from" class="mt-4 mb-1">{{ t("views.DocumentEdit.labels.from") }}</label>
                    <InputText id="amountInterval-from" v-model="claimFrom" class="min-w-0 flex-auto grow" />
                    <label for="amountInterval-fromPrecision" class="mt-4 mb-1">{{ t("common.labels.precision") }}</label>
                    <InputText id="amountInterval-fromPrecision" v-model="claimFromAmountPrecision" class="min-w-0 flex-auto grow" />
                    <label for="amountInterval-to" class="mt-4 mb-1">{{ t("views.DocumentEdit.labels.to") }}</label>
                    <InputText id="amountInterval-to" v-model="claimTo" class="min-w-0 flex-auto grow" />
                    <label for="amountInterval-toPrecision" class="mt-4 mb-1">{{ t("common.labels.precision") }}</label>
                    <InputText id="amountInterval-toPrecision" v-model="claimToAmountPrecision" class="min-w-0 flex-auto grow" />
                  </TabPanel>
                  <TabPanel tabindex="-1" class="flex flex-col">
                    <label for="time-property" class="mt-4 mb-1">{{ t("common.labels.property") }}</label>
                    <InputText id="time-property" v-model="claimProp" class="min-w-0 flex-auto grow" />
                    <InputTime v-model="claimValue" v-model:precision="claimTimePrecision" class="mt-4 min-w-0 flex-auto grow" />
                  </TabPanel>
                  <TabPanel tabindex="-1" class="flex flex-col">
                    <label for="timeInterval-property" class="mt-4 mb-1">{{ t("common.labels.property") }}</label>
                    <InputText id="timeInterval-property" v-model="claimProp" class="min-w-0 flex-auto grow" />
                    <InputTime v-model="claimFrom" v-model:precision="claimFromTimePrecision" class="mt-4 min-w-0 flex-auto grow">
                      <template #time-label>{{ t("views.DocumentEdit.labels.from") }}</template>
                    </InputTime>
                    <InputTime v-model="claimTo" v-model:precision="claimToTimePrecision" class="mt-4 min-w-0 flex-auto grow">
                      <template #time-label>{{ t("views.DocumentEdit.labels.to") }}</template>
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
                    <InputRel id="reference-value" v-model="claimValue" class="min-w-0 flex-auto grow" />
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
            </TabPanel>
          </TabPanels>
        </TabGroup>
        <div class="mt-4 flex flex-row justify-end">
          <Button id="documentedit-button-save" type="submit" primary :disabled="!canSave()" :progress="editProgress" @click.prevent="onSave">{{
            t("common.buttons.save")
          }}</Button>
        </div>
      </template>
      <div v-else-if="!classesInitialized" class="my-1 text-center sm:my-4">{{ t("common.status.loading") }}</div>
      <div v-else-if="doc" class="my-1 text-center sm:my-4">{{ t("common.status.editingNotAllowed") }}</div>
      <div v-else class="my-1 text-center sm:my-4">{{ t("common.status.loading") }}</div>
    </div>
  </div>
  <Teleport to="footer">
    <Footer class="border-t border-slate-50 bg-slate-200 shadow-sm" />
  </Teleport>
</template>
