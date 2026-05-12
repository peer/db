<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { D } from "@/document"
import type { ClientSearchSession, RefFilterState, RefSearchResult } from "@/types"

import { ArrowTopRightOnSquareIcon } from "@heroicons/vue/20/solid"
import { computed, onBeforeUnmount, toRef, useTemplateRef } from "vue"
import { useI18n } from "vue-i18n"

import Button from "@/components/Button.vue"
import CheckBox from "@/components/CheckBox.vue"
import WithDocument from "@/components/WithDocument.vue"
import DisplayLabel from "@/partials/DisplayLabel.vue"
import DocumentRefInline from "@/partials/DocumentRefInline.vue"
import { useProgress } from "@/progress"
import { FILTERS_INCREASE, FILTERS_INITIAL_LIMIT, NONE, useRefFilterValues } from "@/search"
import { equals, loadingWidth, useInitialLoad, useLimitResults } from "@/utils"

const props = defineProps<{
  searchSession: DeepReadonly<ClientSearchSession>
  searchTotal: number
  result: RefSearchResult
  state: RefFilterState
  updateProgress: number
}>()

const emit = defineEmits<{
  "update:state": [state: RefFilterState]
}>()

const { t } = useI18n({ useScope: "global" })

const el = useTemplateRef<HTMLElement>("el")

const abortController = new AbortController()

onBeforeUnmount(() => {
  abortController.abort()
})

const progress = useProgress()
const {
  results,
  total,
  error,
  url: resultsUrl,
} = useRefFilterValues(
  toRef(() => props.searchSession),
  toRef(() => props.result),
  el,
  progress,
)
const { laterLoad } = useInitialLoad(progress)

const { limitedResults, hasMore, loadMore } = useLimitResults(results, FILTERS_INITIAL_LIMIT, FILTERS_INCREASE)

const limitedResultsWithNone = computed(() => {
  // We cannot add "none" result without knowing other results because the "none" result might not be
  // shown initially at all if other results have higher counts. If were to add "none" result always,
  // it could happen that it flashes initially and then is hidden once other results load.
  if (!limitedResults.value.length) {
    return limitedResults.value
  } else if (props.result.count >= props.searchTotal) {
    return limitedResults.value
  }
  const res = [...limitedResults.value, { count: props.searchTotal - props.result.count }]
  res.sort((a, b) => b.count - a.count)
  return res
})

const checkboxState = computed({
  get(): RefFilterState {
    return props.state
  },
  set(value: RefFilterState) {
    if (abortController.signal.aborted) {
      return
    }

    // TODO: Remove workaround for Vue not supporting Symbols for checkbox values.
    //       See: https://github.com/vuejs/core/issues/10597
    value = value.map((v) => (v === "__NONE__" ? NONE : v))

    if (!equals(props.state, value)) {
      emit("update:state", value)
    }
  },
})

const WithDocumentD = WithDocument<D>
</script>

<template>
  <div class="pd-reffiltersresult flex flex-col" :class="{ 'data-reloading': laterLoad }" :data-url="resultsUrl">
    <div class="flex items-baseline gap-x-1">
      <DocumentRefInline :id="result.id" class="mb-1.5 text-lg leading-none" />
      ({{ result.count }})
    </div>
    <ul ref="el" class="grid grid-cols-[max-content_auto] gap-x-1">
      <li v-if="error" class="col-span-2">
        <i class="pd-reffiltersresult-error text-error-600">{{ t("common.status.loadingDataFailed") }}</i>
      </li>
      <template v-else-if="total === null">
        <li v-for="i in 3" :key="i" class="contents">
          <div class="my-1.5 h-2 w-4 rounded-sm bg-slate-200 motion-safe:animate-pulse" aria-hidden="true"></div>
          <div class="flex items-baseline gap-x-1" aria-hidden="true">
            <div class="my-1.5 h-2 rounded-sm bg-slate-200 motion-safe:animate-pulse" :class="[loadingWidth(`${result.id}/${i}`)]"></div>
            <div class="my-1.5 h-2 w-8 rounded-sm bg-slate-200 motion-safe:animate-pulse"></div>
          </div>
        </li>
      </template>
      <template v-else>
        <li v-for="res in limitedResultsWithNone" :key="'id' in res ? res.id : NONE" class="contents">
          <template v-if="'id' in res && (res.count != searchTotal || state.includes(res.id))">
            <CheckBox :id="'ref/' + result.id + '/' + res.id" v-model="checkboxState" :progress="updateProgress" :value="res.id" />
            <div class="flex items-baseline gap-x-1">
              <WithDocumentD :id="res.id" name="DocumentGet">
                <template #default="{ doc, url }">
                  <label :for="'ref/' + result.id + '/' + res.id" :class="updateProgress > 0 ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'" :data-url="url"
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
              <label :for="'ref/' + result.id + '/' + res.id" :class="updateProgress > 0 ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
                >({{ res.count }})</label
              >
              <RouterLink :to="{ name: 'DocumentGet', params: { id: res.id } }" class="link"
                ><ArrowTopRightOnSquareIcon :alt="t('common.icons.link')" class="inline size-5"
              /></RouterLink>
            </div>
          </template>
          <template v-else-if="'id' in res && res.count == searchTotal">
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
              <RouterLink :to="{ name: 'DocumentGet', params: { id: res.id } }" class="link"
                ><ArrowTopRightOnSquareIcon :alt="t('common.icons.link')" class="inline size-5"
              /></RouterLink>
            </div>
          </template>
          <template v-else-if="!('id' in res)">
            <CheckBox :id="'ref/' + result.id + '/none'" v-model="checkboxState" :progress="updateProgress" value="__NONE__" />
            <div class="flex items-baseline gap-x-1">
              <label :for="'ref/' + result.id + '/none'" :class="updateProgress > 0 ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
                ><i>{{ t("common.values.none") }}</i></label
              >
              <label :for="'ref/' + result.id + '/none'" :class="updateProgress > 0 ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'">({{ res.count }})</label>
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
