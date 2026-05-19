<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { D } from "@/document"
import type { RefFilterEntry, RefSearchResult, SearchSession, ToValue } from "@/types"

import { ArrowTopRightOnSquareIcon } from "@heroicons/vue/20/solid"
import { computed, onBeforeUnmount, toRef, useId, useTemplateRef } from "vue"
import { useI18n } from "vue-i18n"

import Button from "@/components/Button.vue"
import CheckBox from "@/components/CheckBox.vue"
import WithDocument from "@/components/WithDocument.vue"
import DisplayLabel from "@/partials/DisplayLabel.vue"
import DocumentRefInline from "@/partials/DocumentRefInline.vue"
import { useLocked, useProgress } from "@/progress"
import { FILTERS_INCREASE, FILTERS_INITIAL_LIMIT, useRefFilters } from "@/search"
import { equals, loadingWidth, useInitialLoad, useLimitResults } from "@/utils"

const props = defineProps<{
  searchSession: DeepReadonly<SearchSession>
  searchTotal: number
  result: RefSearchResult
  filter?: RefFilterEntry
}>()

const locked = useLocked()

const emit = defineEmits<{
  filterUpdate: [filterId: string, filter: RefFilterEntry]
}>()

const { t } = useI18n({ useScope: "global" })

const el = useTemplateRef<HTMLElement>("el")

const labelId = useId()

const abortController = new AbortController()

onBeforeUnmount(() => {
  abortController.abort()
})

// Data loading only, no controls.
const progress = useProgress()

// The filter ID from the session's filter, if it exists.
const filterId = computed(() => props.filter?.id ?? "")

// Composite key uniquely identifying this filter panel (all props joined).
// For single-prop ref filters this is just the prop ID; for sub-ref filters
// it is "parentProp/prop" so it does not collide with the parent ref filter panel.
const propsKey = computed(() => props.result.props.join("/"))

const {
  results,
  total,
  error,
  url: resultsUrl,
} = useRefFilters(
  toRef(() => props.searchSession),
  filterId,
  computed(() => props.result.props),
  el,
  progress,
)
const { laterLoad } = useInitialLoad(progress)

const { limitedResults, hasMore, loadMore } = useLimitResults(results, FILTERS_INITIAL_LIMIT, FILTERS_INCREASE)

// Extract the selected "to" IDs from the filter value.
const selectedIds = computed((): string[] => {
  if (!props.filter?.ref?.to) {
    return []
  }
  return props.filter.ref.to.map((t: ToValue) => t.id)
})

const isMissingSelected = computed((): boolean => {
  return props.filter?.ref?.missing === true
})

const checkboxState = computed({
  get(): string[] {
    const ids = [...selectedIds.value]
    if (isMissingSelected.value) {
      ids.push("__MISSING__")
    }
    return ids
  },
  set(value: string[]) {
    if (abortController.signal.aborted) {
      return
    }

    const missingSelected = value.includes("__MISSING__")
    const toIds = value.filter((v) => v !== "__MISSING__")
    const to: ToValue[] | undefined = toIds.length > 0 ? toIds.map((id) => ({ id })) : undefined
    const missing = missingSelected ? true : undefined

    // Build the updated filter.
    const updatedFilter: RefFilterEntry = {
      id: props.filter?.id ?? "",
      base: props.filter?.base ?? [],
      prop: props.filter?.prop ?? [...props.result.props],
      ref: { to, missing },
    }

    if (!equals(props.filter, updatedFilter)) {
      emit("filterUpdate", updatedFilter.id, updatedFilter)
    }
  },
})

const WithDocumentD = WithDocument<D>
</script>

<template>
  <div class="pd-reffiltersresult flex flex-col" :class="{ 'data-reloading': laterLoad }" :data-url="resultsUrl">
    <div :id="labelId" class="flex items-baseline gap-x-1">
      <template v-if="result.props.length === 2">
        <DocumentRefInline :id="result.props[0]" class="mb-1.5 text-lg leading-none" />
        <span class="mb-1.5 text-lg leading-none">&gt;</span>
        <DocumentRefInline :id="result.props[1]" class="mb-1.5 text-lg leading-none" />
      </template>
      <DocumentRefInline v-else :id="result.props[0]" class="mb-1.5 text-lg leading-none" />
      ({{ result.count }})
    </div>
    <ul ref="el" role="group" :aria-labelledby="labelId" class="grid grid-cols-[max-content_auto] gap-x-1">
      <li v-if="error" class="col-span-2">
        <i class="pd-reffiltersresult-error text-error-600">{{ t("common.status.loadingDataFailed") }}</i>
      </li>
      <template v-else-if="total === null">
        <li v-for="i in 3" :key="i" class="contents">
          <div class="my-1.5 h-2 w-4 rounded-sm bg-slate-200 motion-safe:animate-pulse" aria-hidden="true"></div>
          <div class="flex items-baseline gap-x-1" aria-hidden="true">
            <div class="my-1.5 h-2 rounded-sm bg-slate-200 motion-safe:animate-pulse" :class="[loadingWidth(`${propsKey}/${i}`)]"></div>
            <div class="my-1.5 h-2 w-8 rounded-sm bg-slate-200 motion-safe:animate-pulse"></div>
          </div>
        </li>
      </template>
      <template v-else>
        <li v-for="res in limitedResults" :key="res.id" class="contents">
          <template v-if="res.id === '__MISSING__'">
            <CheckBox :id="'ref/' + propsKey + '/missing'" v-model="checkboxState" value="__MISSING__" />
            <div class="flex items-baseline gap-x-1">
              <label :for="'ref/' + propsKey + '/missing'" :class="locked ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
                ><i>{{ t("common.values.missing") }}</i></label
              >
              <label :for="'ref/' + propsKey + '/missing'" :class="locked ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'">({{ res.count }})</label>
            </div>
          </template>
          <template v-else-if="res.count != searchTotal || selectedIds.includes(res.id)">
            <CheckBox :id="'ref/' + propsKey + '/' + res.id" v-model="checkboxState" :value="res.id" />
            <div class="flex items-baseline gap-x-1">
              <WithDocumentD :id="res.id" name="DocumentGet">
                <template #default="{ doc, url }">
                  <label :for="'ref/' + propsKey + '/' + res.id" :class="locked ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'" :data-url="url"
                    ><DisplayLabel :doc="doc"
                  /></label>
                </template>
                <template #loading="{ url }">
                  <div
                    class="pd-withdocument-loading h-2 rounded-sm bg-slate-200 motion-safe:animate-pulse"
                    :data-url="url"
                    :class="[loadingWidth(res.id)]"
                    aria-hidden="true"
                  ></div>
                </template>
              </WithDocumentD>
              <label :for="'ref/' + propsKey + '/' + res.id" :class="locked ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'">({{ res.count }})</label>
              <!--
                tabindex="-1" keeps the open-link icon out of the keyboard tab
                order so Tab jumps between filters without stopping
                on each row's icon. Mouse users can still click it.
              -->
              <RouterLink :to="{ name: 'DocumentGet', params: { id: res.id } }" class="link" tabindex="-1"
                ><ArrowTopRightOnSquareIcon :alt="t('common.icons.link')" class="inline size-5 align-text-top"
              /></RouterLink>
            </div>
          </template>
          <template v-else>
            <div class="h-4 w-4"></div>
            <div class="flex items-baseline gap-x-1">
              <WithDocumentD :id="res.id" name="DocumentGet">
                <template #default="{ doc, url }">
                  <div :data-url="url"><DisplayLabel :doc="doc" /></div>
                </template>
                <template #loading="{ url }">
                  <div
                    class="pd-withdocument-loading h-2 rounded-sm bg-slate-200 motion-safe:animate-pulse"
                    :data-url="url"
                    :class="[loadingWidth(res.id)]"
                    aria-hidden="true"
                  ></div>
                </template>
              </WithDocumentD>
              <div>({{ res.count }})</div>
              <!--
                tabindex="-1" keeps the open-link icon out of the keyboard tab
                order so Tab jumps between filters without stopping
                on each row's icon. Mouse users can still click it.
              -->
              <RouterLink :to="{ name: 'DocumentGet', params: { id: res.id } }" class="link" tabindex="-1"
                ><ArrowTopRightOnSquareIcon :alt="t('common.icons.link')" class="inline size-5 align-text-top"
              /></RouterLink>
            </div>
          </template>
        </li>
      </template>
    </ul>
    <Button v-if="total !== null && hasMore" primary class="mt-2 w-1/2 min-w-fit self-center" @click.prevent="loadMore">{{
      t("common.buttons.loadCountMore", { count: total - limitedResults.length })
    }}</Button>
    <div v-else-if="total !== null && total > limitedResults.length" class="mt-2 text-center text-sm">
      {{ t("common.status.valuesNotShown", { count: total - limitedResults.length }) }}
    </div>
  </div>
</template>
