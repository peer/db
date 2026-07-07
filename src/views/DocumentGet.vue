<script setup lang="ts">
import type { Component, Raw } from "vue"
import type { ComponentExposed } from "vue-component-type-helpers"

import type { D } from "@/document"
import type { DocumentBeginEditResponse, QueryValues } from "@/types"

import { Tab, TabGroup, TabList, TabPanel, TabPanels } from "@headlessui/vue"
import { ChevronLeftIcon, ChevronRightIcon, PencilIcon, TrashIcon } from "@heroicons/vue/20/solid"
import { computed, onBeforeUnmount, provide, ref, toRef, useTemplateRef, watch, watchEffect } from "vue"
import { useI18n } from "vue-i18n"
import { useRoute, useRouter } from "vue-router"

import { deleteFromCache, headURLDirect, postJSON } from "@/api"
import { CAN_CHANGES_DOCUMENT, CAN_DELETE_DOCUMENT, CAN_EDIT_DOCUMENT, hasPermission } from "@/auth"
import Button from "@/components/Button.vue"
import ButtonLink from "@/components/ButtonLink.vue"
import InputTextLink from "@/components/InputTextLink.vue"
import WithDocument from "@/components/WithDocument.vue"
import WithLock from "@/components/WithLock.vue"
import siteContext from "@/context"
import { CONTENT, CREATE_SHORTCUT, INSTANCE_OF, NAME, PAGE, SEARCH_SHORTCUT } from "@/core"
import { getBestClaimOfType, getClaimsOfTypeWithConfidence, selectClaimsByLanguage } from "@/document"
import { documentNavigationKey } from "@/document-navigation"
import { decodeMetadata } from "@/metadata"
import ClaimValueHtml from "@/partials/claimvalue/ClaimValueHtml.vue"
import DisplayLabel from "@/partials/DisplayLabel.vue"
import DocumentHistory from "@/partials/DocumentHistory.vue"
import DocumentRefInline from "@/partials/DocumentRefInline.vue"
import FieldsView from "@/partials/FieldsView.vue"
import Footer from "@/partials/Footer.vue"
import NavBar from "@/partials/NavBar.vue"
import NavBarSearch from "@/partials/NavBarSearch.vue"
import PropertiesRows from "@/partials/PropertiesRows.vue"
import SearchShortcutLink from "@/partials/SearchShortcutLink.vue"
import { getParentLock, localCounter, lockScope, useProgress } from "@/progress"
import { getDocumentComponents } from "@/registry/document"
import { getDocumentHeaderComponents } from "@/registry/document-header"
import { useSearch, useSearchSession } from "@/search"
import { createShortcutToQuery, shortcutToQuery } from "@/shortcut"
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

// When the URL carries a "version" query parameter, the document is fetched and shown at that
// version instead of the latest one. The History tab links to documents at specific versions.
const reqVersion = computed(() => {
  const version = Array.isArray(route.query.version) ? route.query.version[0] : route.query.version
  return version || undefined
})

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

// Independent lock-scope for the Delete button, mirroring the Edit button's.
const deleteLock = lockScope(getParentLock())
const deleteBusy = localCounter(deleteLock)
function getDeleteLock() {
  return deleteLock
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

// Expose the search session and the neighboring results to registered document components, so
// downstream sites can render their own navigation between results (e.g. previous/next links
// inside the page instead of the navbar buttons).
provide(documentNavigationKey, {
  searchSessionId: computed(() => searchSession.value?.id ?? null),
  prevNext,
})

function afterClick() {
  document.getElementById("search-input-text")?.focus()
}

// Whether this document is a page (an instance of the PAGE class). Pages get a dedicated
// layout: a "Content" tab plus the "all properties" and "history" tabs. The class-based
// registry tabs and the FieldsView tab are not shown for pages.
const isPage = computed(() => {
  const doc = withDocument.value?.doc
  if (!doc?.claims) return false
  return getClaimsOfTypeWithConfidence(doc.claims, "ref", INSTANCE_OF).some((ref) => ref.to.id === PAGE)
})

// The page content claims in the current language, rendered as prose in the content tab.
const pageContent = computed(() => {
  const doc = withDocument.value?.doc
  if (!isPage.value || !doc?.claims) return []
  return selectClaimsByLanguage(doc.claims, "html", CONTENT, locale.value, (c) => c.length > 0) ?? []
})

const documentComponents = getDocumentComponents()
const documentTabs = computed(() => {
  const doc = withDocument.value?.doc
  if (isPage.value || !doc?.claims) return []
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
const hasFieldsViewPanel = computed(() => !isPage.value && documentTabs.value.length === 0 && classTabId.value !== null && mergedFieldsData.value !== null)

// Tab slugs in the template's tab order, used to reflect the active tab in the URL. Registry and
// class tabs use their class document id as the slug; the fixed tabs use stable names, so links
// can target them (e.g. ?tab=properties or ?tab=history).
const tabSlugs = computed(() => {
  const slugs: string[] = []
  if (isPage.value) {
    slugs.push("content")
  }
  for (const documentTab of documentTabs.value) {
    slugs.push(documentTab.id)
  }
  if (hasFieldsViewPanel.value && classTabId.value) {
    slugs.push(classTabId.value)
  }
  slugs.push("properties")
  // The history API requires this permission, so the tab is shown only to callers who can use it.
  if (hasPermission(CAN_CHANGES_DOCUMENT)) {
    slugs.push("history")
  }
  return slugs
})

// The active tab follows the "tab" query parameter (the first tab when absent or unknown), and
// selecting a tab updates the parameter in place, so tabs can be linked and survive reloads.
const selectedTabIndex = computed(() => {
  const tab = Array.isArray(route.query.tab) ? route.query.tab[0] : route.query.tab
  if (!tab) {
    return 0
  }
  const index = tabSlugs.value.indexOf(tab)
  return index >= 0 ? index : 0
})

// Selecting a tab pushes a history entry, so the back button returns to the previously selected tab.
function onTabChange(index: number) {
  const tab = index > 0 ? tabSlugs.value[index] : undefined
  //eslint-disable-next-line @typescript-eslint/no-floating-promises
  router.push({ query: { ...route.query, tab } })
}

type SearchShortcut = { name: string; raw: string; createRaw: string | null }
type ResolvedShortcut = { name: string; query: QueryValues; count: string | null; createQuery: QueryValues | null }

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
    // The create shortcut is only read (and later resolved) for users who can create documents, since the
    // "+" button leads to the create view which requires that permission, so it would not work for others.
    // Reading the permission here also recomputes the list when the caller's roles change.
    const canCreate = hasPermission(CAN_EDIT_DOCUMENT)
    const result: SearchShortcut[] = []
    // The same shortcut can be declared on several parent classes, so we deduplicate by the raw
    // shortcut string (the link, not the label) and keep the first occurrence.
    const seen = new Set<string>()
    for (const classDoc of classDocs.value) {
      const shortcuts = getClaimsOfTypeWithConfidence(classDoc.claims, "string", SEARCH_SHORTCUT)
      for (const shortcut of shortcuts) {
        if (!shortcut.string) {
          continue
        }
        if (seen.has(shortcut.string)) {
          continue
        }
        const name = selectClaimsByLanguage(shortcut.sub, "string", NAME, locale.value, (c) => c.length > 0)
        if (!name || name.length === 0) {
          continue
        }
        seen.add(shortcut.string)
        // The optional CREATE_SHORTCUT sub-claim (cardinality 0..1) turns the shortcut into a "create" action,
        // but only for users who can create documents.
        const createRaw = canCreate ? getBestClaimOfType(shortcut.sub, "string", CREATE_SHORTCUT)?.string || null : null
        result.push({ name: name[0].string, raw: shortcut.string, createRaw })
      }
    }
    return result
  },
  async (shortcuts: SearchShortcut[], _old, onCleanup) => {
    const result: ResolvedShortcut[] = []
    for (const shortcut of shortcuts) {
      try {
        const query = await shortcutToQuery(shortcut.raw, props.id)
        // A malformed create shortcut should not drop the whole search shortcut, so it is resolved on its own.
        let createQuery: QueryValues | null = null
        if (shortcut.createRaw) {
          try {
            createQuery = await createShortcutToQuery(shortcut.createRaw, props.id)
          } catch (err) {
            console.error("DocumentGet.createShortcut", shortcut, err)
          }
        }
        result.push({ name: shortcut.name, query, count: null, createQuery })
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

async function onDelete() {
  if (abortController.signal.aborted) {
    return
  }

  deleteBusy.value += 1
  try {
    await postJSON(
      router.apiResolve({
        name: "DocumentDelete",
        params: {
          id: props.id,
        },
      }).href,
      {},
      abortController.signal,
      deleteBusy,
    )
    if (abortController.signal.aborted) {
      return
    }
    // The document no longer exists, so drop its cached response and leave the page.
    deleteFromCache(
      router.apiResolve({
        name: "DocumentGet",
        params: {
          id: props.id,
        },
      }).href,
    )
    await router.push({
      name: "Home",
    })
  } catch (err) {
    if (abortController.signal.aborted) {
      return
    }
    // TODO: Show notification with error.
    console.error("DocumentGet.onDelete", err)
  } finally {
    deleteBusy.value -= 1
  }
}
</script>

<template>
  <Teleport to="header">
    <NavBar>
      <template #start>
        <template v-if="searchSession !== null">
          <!-- self-stretch so the query link keeps the row height even when the query is empty, instead of collapsing to its text height. -->
          <InputTextLink
            class="max-w-xl grow self-stretch"
            :to="{ name: 'SearchGet', params: { id: searchSession.id }, query: encodeQuery({ at: id }) }"
            :after-click="afterClick"
          >
            {{ searchSession.query }}
          </InputTextLink>
          <!--
            A tight prev/next pair. The floor is two button floors plus the gap-x-1 (min-w-25), so the navbar can
            compress the pair down to the buttons' own floor (an icon each) but not past it, which would let the
            buttons overflow the group and collide with the next navbar item.
          -->
          <div class="flex min-w-25 gap-x-1">
            <ButtonLink
              id="documentget-button-prev"
              primary
              :disabled="!prevNext.previous"
              :to="{ name: 'DocumentGet', params: { id: prevNext.previous }, query: encodeQuery({ s: searchSession.id }) }"
            >
              <ChevronLeftIcon class="size-5 sm:hidden" :alt="t('common.buttons.prev')" />
              <span class="hidden sm:inline">{{ t("common.buttons.prev") }}</span>
            </ButtonLink>
            <ButtonLink
              id="documentget-button-next"
              primary
              :disabled="!prevNext.next"
              :to="{ name: 'DocumentGet', params: { id: prevNext.next }, query: encodeQuery({ s: searchSession.id }) }"
            >
              <ChevronRightIcon class="size-5 sm:hidden" :alt="t('common.buttons.next')" />
              <span class="hidden sm:inline">{{ t("common.buttons.next") }}</span>
            </ButtonLink>
          </div>
        </template>
        <NavBarSearch v-else />
      </template>
      <template #end>
        <WithLock v-if="hasPermission(CAN_EDIT_DOCUMENT)" :lock="getEditLock">
          <Button :progress="editBusy" type="button" primary @click.prevent="onEdit">
            <PencilIcon class="size-5 sm:hidden" :alt="t('common.buttons.edit')" />
            <span class="hidden sm:inline">{{ t("common.buttons.edit") }}</span>
          </Button>
        </WithLock>
        <WithLock v-if="hasPermission(CAN_DELETE_DOCUMENT)" :lock="getDeleteLock">
          <Button :progress="deleteBusy" type="button" primary @click.prevent="onDelete">
            <TrashIcon class="size-5 sm:hidden" :alt="t('common.buttons.delete')" />
            <span class="hidden sm:inline">{{ t("common.buttons.delete") }}</span>
          </Button>
        </WithLock>
      </template>
    </NavBar>
  </Teleport>
  <div
    ref="el"
    class="pd-documentget mt-[var(--pd-navbar-offset)] flex w-full flex-col gap-y-1 border-t border-transparent p-1 sm:gap-y-4 sm:p-4"
    :data-url="withDocument?.url"
  >
    <!-- Registered document header components render above the card, on every tab. -->
    <component :is="component" v-for="(component, i) in getDocumentHeaderComponents().value" :id="id" :key="i" />
    <div class="pd-documentget-card rounded-sm border border-gray-200 bg-white p-4 shadow-sm">
      <WithDocumentD :id="id" ref="withDocument" name="DocumentGet" :version="reqVersion">
        <template #default="{ doc }">
          <div v-if="!classesInitialized" class="my-1 text-center sm:my-4">{{ t("common.status.loading") }}</div>
          <!--
            TODO: Fix how hover interacts with focused tab.
            See: https://github.com/tailwindlabs/tailwindcss/discussions/10123
          -->
          <TabGroup v-else manual :selected-index="selectedTabIndex" @change="onTabChange">
            <TabList class="pd-documentget-tabs -m-4 mb-4 flex border-collapse flex-row rounded-t border-b border-gray-200 bg-slate-100">
              <!-- The page content tab. The page title is shown as the h1 heading below. -->
              <Tab
                v-if="isPage"
                class="border-r border-gray-200 px-4 py-3 leading-tight font-medium uppercase outline-none select-none first:rounded-tl not-aria-selected:hover:bg-slate-50 focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 aria-selected:bg-white"
                >{{ t("views.DocumentGet.tabs.content") }}</Tab
              >
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
              <!-- The history API requires this permission, so the tab is shown only to callers who can use it. -->
              <Tab
                v-if="hasPermission(CAN_CHANGES_DOCUMENT)"
                class="border-r border-gray-200 px-4 py-3 leading-tight font-medium uppercase outline-none select-none first:rounded-tl not-aria-selected:hover:bg-slate-50 focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 aria-selected:bg-white"
                >{{ t("views.DocumentGet.tabs.history") }}</Tab
              >
            </TabList>
            <h1 v-show="displayLabelComponent?.displayLabel" class="pd-documentget-title mb-4 text-3xl font-bold drop-shadow-xs"
              ><DisplayLabel ref="displayLabelComponent" :doc="doc"
            /></h1>
            <!-- We explicitly disable tabbing. See: https://github.com/tailwindlabs/headlessui/discussions/1433 -->
            <TabPanels as="template">
              <!-- Page content tab panel: the document's content rendered as prose, in the current language. -->
              <TabPanel v-if="isPage" tabindex="-1" class="outline-none">
                <ClaimValueHtml v-for="content in pageContent" :key="content.id" :claim="content" />
              </TabPanel>
              <!-- Registry tabs. -->
              <TabPanel v-for="documentTab in documentTabs" :key="documentTab.id" tabindex="-1" class="outline-none">
                <component :is="documentTab.component" :doc="doc" />
              </TabPanel>
              <!-- Class-specific tab (if there are no registry tabs). -->
              <TabPanel v-if="hasFieldsViewPanel" tabindex="-1" class="outline-none">
                <div class="flex flex-row items-start gap-4">
                  <div class="min-w-0 grow"><FieldsView :fields-data="mergedFieldsData!" :claims="doc.claims" sections /></div>
                  <div class="pd-print-hidden flex shrink-0 flex-col gap-2">
                    <template v-for="(shortcut, i) of searchShortcuts" :key="i">
                      <SearchShortcutLink
                        v-if="showShortcut(shortcut.count)"
                        :query="shortcut.query"
                        :label="shortcutLabel(shortcut.name, shortcut.count)"
                        :create-query="shortcut.createQuery"
                      />
                    </template>
                    <SearchShortcutLink
                      v-if="showShortcut(referencedByCount)"
                      :query="encodeQuery({ reverse: id })"
                      :label="shortcutLabel(t('views.DocumentGet.referencedBy'), referencedByCount)"
                    />
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
                  <div v-if="!hasFieldsViewPanel" class="pd-print-hidden flex shrink-0 flex-col gap-2">
                    <template v-for="(shortcut, i) of searchShortcuts" :key="i">
                      <SearchShortcutLink
                        v-if="showShortcut(shortcut.count)"
                        :query="shortcut.query"
                        :label="shortcutLabel(shortcut.name, shortcut.count)"
                        :create-query="shortcut.createQuery"
                      />
                    </template>
                    <SearchShortcutLink
                      v-if="showShortcut(referencedByCount)"
                      :query="encodeQuery({ reverse: id })"
                      :label="shortcutLabel(t('views.DocumentGet.referencedBy'), referencedByCount)"
                    />
                  </div>
                </div>
              </TabPanel>
              <!-- "History" tab panel. The panel (and thus the data fetch) is mounted only when the tab is selected. -->
              <TabPanel v-if="hasPermission(CAN_CHANGES_DOCUMENT)" tabindex="-1" class="outline-none">
                <DocumentHistory :id="id" />
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
        <template #error="{ message, accessDenied }">
          <i :class="['pd-documentget-error', accessDenied ? 'text-gray-500' : 'text-error-600']">{{ message }}</i>
        </template>
      </WithDocumentD>
    </div>
  </div>
  <Teleport to="footer">
    <Footer class="border-t border-slate-50 bg-slate-200 shadow-sm" />
  </Teleport>
</template>
