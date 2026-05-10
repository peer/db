<script setup lang="ts">
import type { Component, Raw } from "vue"
import type { ComponentExposed } from "vue-component-type-helpers"

import type { D } from "@/document"
import type { DocumentBeginEditResponse, QueryValues } from "@/types"

import { Tab, TabGroup, TabList, TabPanel, TabPanels } from "@headlessui/vue"
import { ChevronLeftIcon, ChevronRightIcon, PencilIcon } from "@heroicons/vue/20/solid"
import { Identifier } from "@tozd/identifier"
import { computed, onBeforeUnmount, ref, toRef, useTemplateRef, watch, watchEffect } from "vue"
import { useI18n } from "vue-i18n"
import { useRoute, useRouter } from "vue-router"

import { postJSON } from "@/api"
import Button from "@/components/Button.vue"
import ButtonLink from "@/components/ButtonLink.vue"
import InputTextLink from "@/components/InputTextLink.vue"
import WithDocument from "@/components/WithDocument.vue"
import { INSTANCE_OF, NAME, SEARCH_SHORTCUT } from "@/core"
import { getClaimsOfTypeWithConfidence, selectClaimsByLanguage } from "@/document"
import DisplayLabel from "@/partials/DisplayLabel.vue"
import DocumentRefInline from "@/partials/DocumentRefInline.vue"
import FieldsView from "@/partials/FieldsView.vue"
import Footer from "@/partials/Footer.vue"
import NavBar from "@/partials/NavBar.vue"
import NavBarSearch from "@/partials/NavBarSearch.vue"
import PropertiesRows from "@/partials/PropertiesRows.vue"
import { getParentProgress, localProgress } from "@/progress"
import { getDocumentComponents } from "@/registry/document"
import { useSearch, useSearchSession } from "@/search"
import { useDocumentFields } from "@/useDocumentFields"
import { useParentClasses } from "@/useParentClasses"
import { encodeQuery, loadingLongWidth } from "@/utils"

const props = defineProps<{
  id: string
}>()

const { t, locale } = useI18n({ useScope: "global" })
const route = useRoute()
const router = useRouter()

const el = useTemplateRef<HTMLElement>("el")

const parentProgress = getParentProgress()
const progress = localProgress(parentProgress)
const editProgress = localProgress(parentProgress)

const abortController = new AbortController()

onBeforeUnmount(() => {
  abortController.abort()
})

const WithDocumentD = WithDocument<D>
const withDocument = useTemplateRef<ComponentExposed<typeof WithDocumentD>>("withDocument")
const displayLabelComponent = useTemplateRef<ComponentExposed<typeof DisplayLabel>>("displayLabelComponent")

const selectedTab = ref(0)

async function changeTab(index: number) {
  const offset = (classTabId.value && mergedFieldsData.value ? 1 : 0) + documentTabs.value.length
  const searchShortcut = searchShortcuts.value[index - offset]
  if (searchShortcut) {
    await router.push({
      name: "SearchShortcut",
      query: searchShortcut.query,
    })
    return
  }
  selectedTab.value = index
}

// Resolve field definitions for this document's class(es).
const docRef = toRef(() => withDocument.value?.doc ?? null)
const { classDocs, instanceOfClassIds, initialized: classesInitialized } = useParentClasses(docRef, el, progress)
const { fieldsData: mergedFieldsData, classTabId } = useDocumentFields(classDocs, instanceOfClassIds)

const { searchSession, error: searchSessionError } = useSearchSession(
  toRef(() => {
    const searchSessionId = Array.isArray(route.query.s) ? route.query.s[0] : route.query.s
    if (!searchSessionId) {
      return null
    }
    return {
      id: searchSessionId,
      // We always set it to 0 as it is not really used.
      // TODO: Use and track real versions on the client.
      version: 0,
    }
  }),
  progress,
)
const { results, error: searchResultsError } = useSearch(searchSession, el, progress)

// See: https://github.com/vuejs/core/issues/14249
//eslint-disable-next-line @typescript-eslint/no-misused-promises
watchEffect(async (onCleanup) => {
  if (searchSessionError.value || searchResultsError.value) {
    // Something was not OK, so we redirect to the URL without "s".
    await router.replace({
      name: "DocumentGet",
      params: {
        id: props.id,
      },
      // Maybe route.query has non-empty "tab" parameter which we want to keep.
      query: encodeQuery({ tab: route.query.tab || undefined }),
    })
  }
})

const prevNext = computed<{ previous: string | null; next: string | null }>(() => {
  const res: { previous: string | null; next: string | null } = { previous: null, next: null }
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
    //eslint-disable-next-line @typescript-eslint/no-floating-promises
    router.replace({
      name: "DocumentGet",
      params: {
        id: props.id,
      },
    })
  }
  return res
})

function afterClick() {
  document.getElementById("search-input-text")?.focus()
}

const documentComponents = getDocumentComponents()
const documentTabs = computed(() => {
  const doc = withDocument.value?.doc
  if (!doc?.claims) return []
  const refs = getClaimsOfTypeWithConfidence(doc.claims, "ref", INSTANCE_OF)
  const tabs: { component: Raw<Component>; id: string }[] = []
  for (const ref of refs) {
    const component = documentComponents.value.get(ref.to.id)
    if (component) {
      tabs.push({ component, id: ref.to.id })
    }
  }
  return tabs
})

const searchShortcuts = ref<{ name: string; query: QueryValues }[]>([])
watch(
  () => {
    const result: { name: string; filter: Record<string, string> }[] = []
    for (const classDoc of classDocs.value) {
      const shortcuts = getClaimsOfTypeWithConfidence(classDoc.claims, "string", SEARCH_SHORTCUT)
      for (const shortcut of shortcuts) {
        if (!shortcut.string) {
          continue
        }
        const name = selectClaimsByLanguage(shortcut.sub, "string", NAME, locale.value, (c) => c.length > 0)
        if (!name || name.length === 0) {
          continue
        }
        const parts = shortcut.string.split(";")
        const filter: Record<string, string> = {}
        for (const part of parts) {
          const f = part.split(":")
          if (f.length != 2) {
            console.error("invalid search shortcut", classDoc.id, shortcut.string)
            continue
          }
          filter[f[0]] = f[1]
        }
        if (Object.keys(filter).length === 0) {
          continue
        }
        result.push({ name: name[0].string, filter })
      }
    }
    return result
  },
  async (shortcuts: { name: string; filter: Record<string, string> }[]) => {
    try {
      const result = []
      for (const shortcut of shortcuts) {
        const filter: Record<string, string> = {}
        for (const [key, value] of Object.entries(shortcut.filter)) {
          const k = await Identifier.from(...key.split(","))
          const v = await Identifier.from(...value.split(","))
          filter[k.toString()] = v.toString()
        }
        // We could make computing the query be moved to changeTab which is already async,
        // but we prefer that any exceptions happen here so that we then set documentSearchShortcuts
        // to [] here and not even show tabs with problematic shortcuts.
        result.push({ name: shortcut.name, query: encodeQuery(filter) })
      }
      searchShortcuts.value = result
    } catch (err) {
      console.error("documentSearchShortcuts.watch", err)
      searchShortcuts.value = []
    }
  },
  {
    immediate: true,
  },
)

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
      <template #start>
        <div v-if="searchSession !== null" class="flex grow gap-x-1 sm:gap-x-4">
          <InputTextLink class="max-w-xl grow" :to="{ name: 'SearchGet', params: { id: searchSession.id }, query: encodeQuery({ at: id }) }" :after-click="afterClick">
            {{ searchSession.query }}
          </InputTextLink>
          <div class="grid grid-cols-2 gap-x-1">
            <ButtonLink
              id="documentget-button-prev"
              primary
              class="px-3.5"
              :disabled="!prevNext.previous"
              :to="{ name: 'DocumentGet', params: { id: prevNext.previous }, query: encodeQuery({ s: searchSession.id }) }"
            >
              <ChevronLeftIcon class="size-5 sm:hidden" :alt="t('common.buttons.prev')" />
              <span class="hidden sm:inline">{{ t("common.buttons.prev") }}</span>
            </ButtonLink>
            <ButtonLink
              id="documentget-button-next"
              primary
              class="px-3.5"
              :disabled="!prevNext.next"
              :to="{ name: 'DocumentGet', params: { id: prevNext.next }, query: encodeQuery({ s: searchSession.id }) }"
            >
              <ChevronRightIcon class="size-5 sm:hidden" :alt="t('common.buttons.next')" />
              <span class="hidden sm:inline">{{ t("common.buttons.next") }}</span>
            </ButtonLink>
          </div>
        </div>
        <NavBarSearch v-else />
      </template>
      <template #end>
        <Button :progress="editProgress" type="button" primary class="px-3.5" @click.prevent="onEdit">
          <PencilIcon class="size-5 sm:hidden" :alt="t('common.buttons.edit')" />
          <span class="hidden sm:inline">{{ t("common.buttons.edit") }}</span>
        </Button>
      </template>
    </NavBar>
  </Teleport>
  <div ref="el" class="pd-documentget mt-12 flex w-full flex-col gap-y-1 border-t border-transparent p-1 sm:mt-[4.5rem] sm:gap-y-4 sm:p-4" :data-url="withDocument?.url">
    <div class="rounded-sm border border-gray-200 bg-white p-4 shadow-sm">
      <WithDocumentD :id="id" ref="withDocument" name="DocumentGet">
        <template #default="{ doc }">
          <div v-if="!classesInitialized" class="my-1 text-center sm:my-4">{{ t("common.status.loading") }}</div>
          <!--
            TODO: Fix how hover interacts with focused tab.
            See: https://github.com/tailwindlabs/tailwindcss/discussions/10123
          -->
          <TabGroup v-else :selected-index="selectedTab" manual @change="changeTab">
            <TabList class="-m-4 mb-4 flex border-collapse flex-row rounded-t border-b border-gray-200 bg-slate-100">
              <Tab
                v-for="documentTab in documentTabs"
                :key="documentTab.id"
                class="border-r border-gray-200 px-4 py-3 leading-tight font-medium uppercase outline-none select-none first:rounded-tl focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 ui-selected:bg-white ui-not-selected:hover:bg-slate-50"
                ><DocumentRefInline :id="documentTab.id" :link="false"
              /></Tab>
              <Tab
                v-if="documentTabs.length === 0 && classTabId && mergedFieldsData"
                class="border-r border-gray-200 px-4 py-3 leading-tight font-medium uppercase outline-none select-none first:rounded-tl focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 ui-selected:bg-white ui-not-selected:hover:bg-slate-50"
                ><DocumentRefInline :id="classTabId" :link="false"
              /></Tab>
              <Tab
                v-for="(searchShortcut, i) of searchShortcuts"
                :key="i"
                class="border-r border-gray-200 px-4 py-3 leading-tight font-medium uppercase outline-none select-none first:rounded-tl focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 ui-selected:bg-white ui-not-selected:hover:bg-slate-50"
                >{{ searchShortcut.name }}</Tab
              >
              <Tab
                class="border-r border-gray-200 px-4 py-3 leading-tight font-medium uppercase outline-none select-none first:rounded-tl focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 ui-selected:bg-white ui-not-selected:hover:bg-slate-50"
                >{{ t("views.DocumentGet.tabs.allProperties") }}</Tab
              >
            </TabList>
            <h1 v-show="displayLabelComponent?.displayLabel" class="mb-4 text-4xl font-bold drop-shadow-xs"><DisplayLabel ref="displayLabelComponent" :doc="doc" /></h1>
            <!-- We explicitly disable tabbing. See: https://github.com/tailwindlabs/headlessui/discussions/1433 -->
            <TabPanels>
              <TabPanel v-for="documentTab in documentTabs" :key="documentTab.id" tabindex="-1">
                <component :is="documentTab.component" :doc="doc" />
              </TabPanel>
              <TabPanel v-if="documentTabs.length === 0 && classTabId && mergedFieldsData" tabindex="-1">
                <FieldsView :fields-data="mergedFieldsData" :claims="doc.claims" sections />
              </TabPanel>
              <TabPanel v-for="(_, i) of searchShortcuts" :key="i" tabindex="-1"><!-- Empty because this panel should never be rendered. --></TabPanel>
              <TabPanel tabindex="-1">
                <table class="w-full table-auto border-collapse">
                  <thead>
                    <tr>
                      <th class="border-r border-slate-200 px-2 py-1 text-left font-bold">{{ t("common.labels.property") }}</th>
                      <th class="border-l border-slate-200 px-2 py-1 text-left font-bold">{{ t("common.labels.value") }}</th>
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
          <div class="pd-documentget-loading flex flex-col gap-y-2 motion-safe:animate-pulse">
            <div class="inline-block h-2 rounded-sm bg-slate-200" :class="[loadingLongWidth(`${id}/1`)]"></div>
            <div class="flex gap-x-4">
              <div class="h-2 rounded-sm bg-slate-200" :class="[loadingLongWidth(`${id}/2`)]"></div>
              <div class="h-2 rounded-sm bg-slate-200" :class="[loadingLongWidth(`${id}/3`)]"></div>
            </div>
            <div class="flex gap-x-4">
              <div class="h-2 rounded-sm bg-slate-200" :class="[loadingLongWidth(`${id}/4`)]"></div>
              <div class="h-2 rounded-sm bg-slate-200" :class="[loadingLongWidth(`${id}/5`)]"></div>
            </div>
          </div>
        </template>
        <template #error>
          <i class="pd-documentget-error text-error-600">{{ t("common.status.loadingDataFailed") }}</i>
        </template>
      </WithDocumentD>
    </div>
  </div>
  <Teleport to="footer">
    <Footer class="border-t border-slate-50 bg-slate-200 shadow-sm" />
  </Teleport>
</template>
