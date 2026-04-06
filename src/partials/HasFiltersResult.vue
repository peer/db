<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { D } from "@/document"
import type { HasFilterEntry, HasSearchResult, HasValue, SearchSession } from "@/types"

import { ArrowTopRightOnSquareIcon } from "@heroicons/vue/20/solid"
import { computed, onBeforeUnmount, toRef, useTemplateRef } from "vue"
import { useI18n } from "vue-i18n"

import Button from "@/components/Button.vue"
import CheckBox from "@/components/CheckBox.vue"
import WithDocument from "@/components/WithDocument.vue"
import DisplayLabel from "@/partials/DisplayLabel.vue"
import { injectProgress } from "@/progress"
import { FILTERS_INCREASE, FILTERS_INITIAL_LIMIT, useHasFilters } from "@/search"
import { equals, loadingWidth, useInitialLoad, useLimitResults } from "@/utils"

const props = defineProps<{
  searchSession: DeepReadonly<SearchSession>
  searchTotal: number
  result: HasSearchResult
  filter?: HasFilterEntry
  updateProgress: number
}>()

const emit = defineEmits<{
  filterUpdate: [filterId: string, filter: HasFilterEntry]
}>()

const { t } = useI18n({ useScope: "global" })

const el = useTemplateRef<HTMLElement>("el")

const abortController = new AbortController()

onBeforeUnmount(() => {
  abortController.abort()
})

const progress = injectProgress()

// The filter ID from the session's filter, if it exists.
const filterId = computed(() => props.filter?.id ?? "")

const {
  results,
  total,
  error,
  url: resultsUrl,
} = useHasFilters(
  toRef(() => props.searchSession),
  filterId,
  el,
  progress,
)
const { laterLoad } = useInitialLoad(progress)

const { limitedResults, hasMore, loadMore } = useLimitResults(results, FILTERS_INITIAL_LIMIT, FILTERS_INCREASE)

// Extract the selected prop IDs from the filter value.
const selectedIds = computed((): string[] => {
  if (!props.filter?.has?.props) {
    return []
  }
  return props.filter.has.props.map((p: HasValue) => p.id)
})

const checkboxState = computed({
  get(): string[] {
    return [...selectedIds.value]
  },
  set(value: string[]) {
    if (abortController.signal.aborted) {
      return
    }

    const hasProps: HasValue[] | undefined = value.length > 0 ? value.map((id) => ({ id })) : undefined

    // Build the updated filter.
    const updatedFilter: HasFilterEntry = {
      id: props.filter?.id ?? "",
      base: props.filter?.base ?? [],
      prop: [],
      has: { props: hasProps },
    }

    if (!equals(props.filter, updatedFilter)) {
      emit("filterUpdate", updatedFilter.id, updatedFilter)
    }
  },
})

const WithDocumentD = WithDocument<D>
</script>

<template>
  <div class="pd-hasfiltersresult flex flex-col" :class="{ 'data-reloading': laterLoad }" :data-url="resultsUrl">
    <div class="flex items-baseline gap-x-1">
      <span class="mb-1.5 text-lg leading-none">{{ t("partials.HasFiltersResult.title") }}</span>
      ({{ result.count }})
    </div>
    <ul ref="el">
      <li v-if="error">
        <i class="pd-hasfiltersresult-error text-error-600">{{ t("common.status.loadingDataFailed") }}</i>
      </li>
      <template v-else-if="total === null">
        <li v-for="i in 3" :key="i" class="flex animate-pulse items-baseline gap-x-1">
          <div class="my-1.5 h-2 w-4 rounded-sm bg-slate-200"></div>
          <div class="my-1.5 h-2 rounded-sm bg-slate-200" :class="[loadingWidth(`has/${i}`)]"></div>
          <div class="my-1.5 h-2 w-8 rounded-sm bg-slate-200"></div>
        </li>
      </template>
      <template v-else>
        <li v-for="res in limitedResults" :key="res.id" class="flex items-baseline gap-x-1">
          <CheckBox :id="'has/' + res.id" v-model="checkboxState" :progress="updateProgress" :value="res.id" class="my-1 self-center" />
          <WithDocumentD :id="res.id" name="DocumentGet">
            <template #default="{ doc, url }">
              <label
                :for="'has/' + res.id"
                class="my-1 leading-none"
                :class="updateProgress > 0 ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
                :data-url="url"
                ><DisplayLabel :doc="doc"
              /></label>
            </template>
            <template #loading="{ url }">
              <div class="pd-withdocument-loading inline-block h-2 animate-pulse rounded-sm bg-slate-200" :data-url="url" :class="[loadingWidth(res.id)]"></div>
            </template>
          </WithDocumentD>
          <label :for="'has/' + res.id" class="my-1 leading-none" :class="updateProgress > 0 ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
            >({{ res.count }})</label
          >
          <RouterLink :to="{ name: 'DocumentGet', params: { id: res.id } }" class="link"
            ><ArrowTopRightOnSquareIcon :alt="t('common.icons.link')" class="inline size-5 align-text-top"
          /></RouterLink>
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
