<script setup lang="ts">
import type { Component, Raw } from "vue"
import type { ComponentExposed } from "vue-component-type-helpers"

import type { D } from "@/document"
import type { DocumentBeginEditResponse, QueryValues } from "@/types"

import { Tab, TabGroup, TabList, TabPanel, TabPanels } from "@headlessui/vue"
import { ChevronLeftIcon, ChevronRightIcon, PencilIcon } from "@heroicons/vue/20/solid"
import { computed, onBeforeUnmount, ref, toRef, useTemplateRef, watch, watchEffect } from "vue"
import { useI18n } from "vue-i18n"
import { useRoute, useRouter } from "vue-router"

import { headURLDirect, postJSON } from "@/api"
import { CAN_EDIT_DOCUMENT, hasPermission } from "@/auth"
import Button from "@/components/Button.vue"
import ButtonLink from "@/components/ButtonLink.vue"
import InputTextLink from "@/components/InputTextLink.vue"
import WithDocument from "@/components/WithDocument.vue"
import WithLock from "@/components/WithLock.vue"
import siteContext from "@/context"
import { INSTANCE_OF, NAME, SEARCH_SHORTCUT } from "@/core"
import { getClaimsOfTypeWithConfidence, selectClaimsByLanguage } from "@/document"
import { decodeMetadata } from "@/metadata"
import DisplayLabel from "@/partials/DisplayLabel.vue"
import DocumentRefInline from "@/partials/DocumentRefInline.vue"
import FieldsView from "@/partials/FieldsView.vue"
import Footer from "@/partials/Footer.vue"
import NavBar from "@/partials/NavBar.vue"
import NavBarSearch from "@/partials/NavBarSearch.vue"
import PropertiesRows from "@/partials/PropertiesRows.vue"
import { getParentLock, localCounter, lockScope, useProgress } from "@/progress"
import { getDocumentComponents } from "@/registry/document"
import { useSearch, useSearchSession } from "@/search"
import { shortcutToQuery } from "@/shortcut"
import { useDocumentFields } from "@/useDocumentFields"
import { useParentClasses } from "@/useParentClasses"
import { anySignal, encodeQuery, loadingLongWidth } from "@/utils"

const props = defineProps<{
  id: string
}>()

const { t, locale } = useI18n({ useScope: "global" })
const route = useRoute()
const router = useRouter()

const el = useTemplateRef<HTMLElement>("el")

// Data loading only, no controls.
const progress = useProgress()

// Independent lock-scope for the Edit button.
// getParentLock here reads from the ancestor's provides (above DocumentGet).
// editBusy is the writable handle used in the handler and as the button's
// :progress visual. Local count is isolated from any ancestor lock
// contributions; writes still propagate into editLock for descendant cascade.
const editLock = lockScope(getParentLock())
const editBusy = localCounter(editLock)
function getEditLock() {
  return editLock
}

const abortController = new AbortController()

onBeforeUnmount(() => {
  abortController.abort()
})

const WithDocumentD = WithDocument<D>
const withDocument = useTemplateRef<ComponentExposed<typeof WithDocumentD>>("withDocument")
const displayLabelComponent = useTemplateRef<ComponentExposed<typeof DisplayLabel>>("displayLabelComponent")

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
watchEffect(async () => {
  if (searchSessionError.value || searchResultsError.value) {
    // Something was not OK, so we redirect to the URL without "s".
    await router.replace({
      name: "DocumentGet",
      params: {
        id: props.id,
      },
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

// Whether the class-based FieldsView tab panel is rendered.
// Side links (search shortcuts + "referenced by") render inside this panel
// when available, otherwise inside the "all properties" panel.
const hasFieldsViewPanel = computed(() => documentTabs.value.length === 0 && classTabId.value !== null && mergedFieldsData.value !== null)

type SearchShortcut = { name: string; raw: string }
type ResolvedShortcut = { name: string; query: QueryValues; count: string | null }

async function fetchShortcutCount(query: QueryValues, signal: AbortSignal): Promise<string | null> {
  const url = router.apiResolve({ name: "SearchJustResults", query }).href
  // TODO: Use headURL when it will be available.
  const headers = await headURLDirect(url, signal, null)
  if (signal.aborted) {
    return null
  }
  const metadata = decodeMetadata(headers, siteContext.metadataHeaderPrefix ?? "")
  if ("total" in metadata) {
    return String(metadata["total"])
  }
  return null
}

const searchShortcuts = ref<ResolvedShortcut[]>([])
watch(
  () => {
    const result: SearchShortcut[] = []
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
        result.push({ name: name[0].string, raw: shortcut.string })
      }
    }
    return result
  },
  async (shortcuts: SearchShortcut[], _old, onCleanup) => {
    const result: ResolvedShortcut[] = []
    for (const shortcut of shortcuts) {
      try {
        result.push({ name: shortcut.name, query: await shortcutToQuery(shortcut.raw, props.id), count: null })
      } catch (err) {
        console.error("DocumentGet.searchShortcuts", shortcut, err)
      }
    }
    searchShortcuts.value = result

    // Abort in-flight count fetches when this watcher re-fires or stops, and
    // also when the component unmounts.
    const controller = new AbortController()
    onCleanup(() => controller.abort())
    const signal = anySignal(abortController.signal, controller.signal)

    // Fetch counts in parallel.
    // TODO: Instead of fetching it here, fetch it as a component inside the link.
    //       This allows us to show a placeholder while data is being fetched.
    //       Also we can set data-url attribute on the count.
    //       Another idea: all our links to internal pages could have a "preview" view
    //       defined and we could automatically do HEAD on them and display that preview.
    //       For search session links that would be the count of results.
    // Iterate the reactive array (searchShortcuts.value), not the raw result, so
    // assigning sc.count below goes through the reactive proxy and triggers a
    // re-render. Mutating the raw result objects updates the data but not the view,
    // so counts would surface only when some unrelated change forced a redraw.
    for (const sc of searchShortcuts.value) {
      void (async () => {
        try {
          const count = await fetchShortcutCount(sc.query, signal)
          if (signal.aborted) {
            return
          }
          sc.count = count
        } catch (err) {
          if (signal.aborted) {
            return
          }
          console.error("DocumentGet.shortcutCount", sc, err)
        }
      })()
    }
  },
  {
    immediate: true,
  },
)

// TODO: Instead of fetching it here, fetch it as a component inside the link.
const referencedByCount = ref<string | null>(null)
watch(
  () => props.id,
  async (id, _old, onCleanup) => {
    referencedByCount.value = null
    const controller = new AbortController()
    onCleanup(() => controller.abort())
    const signal = anySignal(abortController.signal, controller.signal)
    try {
      const count = await fetchShortcutCount(encodeQuery({ reverse: id }), signal)
      if (signal.aborted) {
        return
      }
      referencedByCount.value = count
    } catch (err) {
      if (signal.aborted) {
        return
      }
      console.error("DocumentGet.referencedByCount", err)
    }
  },
  {
    immediate: true,
  },
)

function shortcutLabel(name: string, count: number | string | null): string {
  if (count === null) {
    return name
  }
  return t("views.DocumentGet.shortcutWithCount", { name, count })
}

// Whether to render a count-bearing side link. A link known to have zero results is
// hidden because following it leads to an empty search, but users who can edit
// documents keep seeing it (so that we can show them a button to create new document
// next to it). A null count (still loading, or a fetch that returned no total)
// is always shown.
function showShortcut(count: string | null): boolean {
  return count !== "0" || hasPermission(CAN_EDIT_DOCUMENT)
}

async function onEdit() {
  if (abortController.signal.aborted) {
    return
  }

  editBusy.value += 1
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
      editBusy,
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
    editBusy.value -= 1
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
        <WithLock v-if="hasPermission(CAN_EDIT_DOCUMENT)" :lock="getEditLock">
          <Button :progress="editBusy" type="button" primary class="px-3.5" @click.prevent="onEdit">
            <PencilIcon class="size-5 sm:hidden" :alt="t('common.buttons.edit')" />
            <span class="hidden sm:inline">{{ t("common.buttons.edit") }}</span>
          </Button>
        </WithLock>
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
          <TabGroup v-else manual>
            <TabList class="-m-4 mb-4 flex border-collapse flex-row rounded-t border-b border-gray-200 bg-slate-100">
              <Tab
                v-for="documentTab in documentTabs"
                :key="documentTab.id"
                class="border-r border-gray-200 px-4 py-3 leading-tight font-medium uppercase outline-none select-none first:rounded-tl not-aria-selected:hover:bg-slate-50 focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 aria-selected:bg-white"
                ><DocumentRefInline :id="documentTab.id" :link="false"
              /></Tab>
              <Tab
                v-if="hasFieldsViewPanel"
                class="border-r border-gray-200 px-4 py-3 leading-tight font-medium uppercase outline-none select-none first:rounded-tl not-aria-selected:hover:bg-slate-50 focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 aria-selected:bg-white"
                ><DocumentRefInline :id="classTabId!" :link="false"
              /></Tab>
              <Tab
                class="border-r border-gray-200 px-4 py-3 leading-tight font-medium uppercase outline-none select-none first:rounded-tl not-aria-selected:hover:bg-slate-50 focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 aria-selected:bg-white"
                >{{ t("views.DocumentGet.tabs.allProperties") }}</Tab
              >
            </TabList>
            <h1 v-show="displayLabelComponent?.displayLabel" class="mb-4 text-4xl font-bold drop-shadow-xs"><DisplayLabel ref="displayLabelComponent" :doc="doc" /></h1>
            <!-- We explicitly disable tabbing. See: https://github.com/tailwindlabs/headlessui/discussions/1433 -->
            <TabPanels as="template">
              <!-- Registry tabs. -->
              <TabPanel v-for="documentTab in documentTabs" :key="documentTab.id" tabindex="-1" class="outline-none">
                <component :is="documentTab.component" :doc="doc" />
              </TabPanel>
              <!-- Class-specific tab (if there are no registry tabs). -->
              <TabPanel v-if="hasFieldsViewPanel" tabindex="-1" class="outline-none">
                <div class="flex flex-row items-start gap-4">
                  <div class="min-w-0 grow"><FieldsView :fields-data="mergedFieldsData!" :claims="doc.claims" sections /></div>
                  <div class="flex shrink-0 flex-col gap-2">
                    <template v-for="(shortcut, i) of searchShortcuts" :key="i">
                      <ButtonLink v-if="showShortcut(shortcut.count)" :to="{ name: 'SearchShortcut', query: shortcut.query }">{{
                        shortcutLabel(shortcut.name, shortcut.count)
                      }}</ButtonLink>
                    </template>
                    <ButtonLink v-if="showShortcut(referencedByCount)" :to="{ name: 'SearchShortcut', query: encodeQuery({ reverse: id }) }">{{
                      shortcutLabel(t("views.DocumentGet.referencedBy"), referencedByCount)
                    }}</ButtonLink>
                  </div>
                </div>
              </TabPanel>
              <!-- "All properties" tab panel. -->
              <TabPanel tabindex="-1" class="outline-none">
                <div class="flex flex-row items-start gap-4">
                  <div class="min-w-0 grow">
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
                  </div>
                  <div v-if="!hasFieldsViewPanel" class="flex shrink-0 flex-col gap-2">
                    <template v-for="(shortcut, i) of searchShortcuts" :key="i">
                      <ButtonLink v-if="showShortcut(shortcut.count)" :to="{ name: 'SearchShortcut', query: shortcut.query }">{{
                        shortcutLabel(shortcut.name, shortcut.count)
                      }}</ButtonLink>
                    </template>
                    <ButtonLink v-if="showShortcut(referencedByCount)" :to="{ name: 'SearchShortcut', query: encodeQuery({ reverse: id }) }">{{
                      shortcutLabel(t("views.DocumentGet.referencedBy"), referencedByCount)
                    }}</ButtonLink>
                  </div>
                </div>
              </TabPanel>
            </TabPanels>
          </TabGroup>
        </template>
        <template #loading>
          <div class="pd-documentget-loading flex flex-col gap-y-2 motion-safe:animate-pulse" aria-hidden="true">
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
