<script setup lang="ts">
import type { Component, Raw } from "vue"
import type { ComponentExposed } from "vue-component-type-helpers"

import type { D } from "@/document"
import type { DocumentBeginEditResponse } from "@/types"

import { Tab, TabGroup, TabList, TabPanel, TabPanels } from "@headlessui/vue"
import { ChevronLeftIcon, ChevronRightIcon, PencilIcon } from "@heroicons/vue/20/solid"
import { computed, onBeforeUnmount, ref, toRef, useTemplateRef, watchEffect } from "vue"
import { useI18n } from "vue-i18n"
import { useRoute, useRouter } from "vue-router"

import { postJSON } from "@/api"
import Button from "@/components/Button.vue"
import ButtonLink from "@/components/ButtonLink.vue"
import InputTextLink from "@/components/InputTextLink.vue"
import WithDocument from "@/components/WithDocument.vue"
import siteContext from "@/context"
import { INSTANCE_OF } from "@/core"
import { getClaimsOfType } from "@/document"
import DocumentRefInline from "@/partials/DocumentRefInline.vue"
import Footer from "@/partials/Footer.vue"
import NavBar from "@/partials/NavBar.vue"
import NavBarSearch from "@/partials/NavBarSearch.vue"
import PropertiesRows from "@/partials/PropertiesRows.vue"
import { injectProgress } from "@/progress"
import { getDocumentComponents } from "@/registry/document"
import { useSearch, useSearchSession } from "@/search"
import { encodeQuery, getDisplayLabel, loadingLongWidth } from "@/utils"

const props = defineProps<{
  id: string
}>()

const { t, locale } = useI18n({ useScope: "global" })
const route = useRoute()
const router = useRouter()

const el = useTemplateRef<HTMLElement>("el")

const progress = injectProgress()
const editProgress = injectProgress()

const abortController = new AbortController()

onBeforeUnmount(() => {
  abortController.abort()
})

const WithDocumentD = WithDocument<D>
const withDocument = ref<ComponentExposed<typeof WithDocumentD> | null>(null)

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

const displayLabel = computed(() => getDisplayLabel(withDocument.value?.doc?.claims, locale.value))

const documentComponents = getDocumentComponents()
const documentTabs = computed(() => {
  const doc = withDocument.value?.doc
  if (!doc?.claims) return []
  const refs = getClaimsOfType(doc.claims, "ref", INSTANCE_OF)
  const tabs: { component: Raw<Component>; id: string }[] = []
  for (const ref of refs) {
    const component = documentComponents.value.get(ref.to.id)
    if (component) {
      tabs.push({ component, id: ref.to.id })
    }
  }
  return tabs
})

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
              primary
              class="px-3.5"
              :disabled="!prevNext.previous"
              :to="{ name: 'DocumentGet', params: { id: prevNext.previous }, query: encodeQuery({ s: searchSession.id }) }"
            >
              <ChevronLeftIcon class="size-5 sm:hidden" :alt="t('common.buttons.prev')" />
              <span class="hidden sm:inline">{{ t("common.buttons.prev") }}</span>
            </ButtonLink>
            <ButtonLink
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
        <Button v-if="siteContext.features.editButtons" :progress="editProgress" type="button" primary class="px-3.5" @click.prevent="onEdit">
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
          <!--
            TODO: Fix how hover interacts with focused tab.
            See: https://github.com/tailwindlabs/tailwindcss/discussions/10123
          -->
          <TabGroup>
            <TabList class="-m-4 mb-4 flex border-collapse flex-row rounded-t border-b border-gray-200 bg-slate-100">
              <Tab
                v-for="documentTab in documentTabs"
                :key="documentTab.id"
                class="border-r border-gray-200 px-4 py-3 leading-tight font-medium uppercase outline-none select-none first:rounded-tl focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 ui-selected:bg-white ui-not-selected:hover:bg-slate-50"
                ><DocumentRefInline :id="documentTab.id" :link="false"
              /></Tab>
              <Tab
                class="border-r border-gray-200 px-4 py-3 leading-tight font-medium uppercase outline-none select-none first:rounded-tl focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 ui-selected:bg-white ui-not-selected:hover:bg-slate-50"
                >{{ t("views.DocumentGet.tabs.allProperties") }}</Tab
              >
            </TabList>
            <h1 class="mb-4 text-4xl font-bold drop-shadow-xs">
              <template v-if="displayLabel">{{ displayLabel }}</template>
              <template v-else
                ><i>{{ t("common.values.noName") }}</i></template
              >
            </h1>
            <TabPanels>
              <!-- We explicitly disable tabbing. See: https://github.com/tailwindlabs/headlessui/discussions/1433 -->
              <TabPanel v-for="documentTab in documentTabs" :key="documentTab.id" tabindex="-1">
                <component :is="documentTab.component" :doc="doc" />
              </TabPanel>
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
          <div class="pd-documentget-loading flex animate-pulse flex-col gap-y-2">
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
