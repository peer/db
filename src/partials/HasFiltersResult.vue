<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { D } from "@/document"
import type { HasFilterEntry, HasSearchResult, HasValue, SearchSession } from "@/types"

import { ArrowTopRightOnSquareIcon } from "@heroicons/vue/20/solid"
import { computed, onBeforeUnmount, toRef, useId, useTemplateRef } from "vue"
import { useI18n } from "vue-i18n"

import Button from "@/components/Button.vue"
import CheckBox from "@/components/CheckBox.vue"
import WithDocument from "@/components/WithDocument.vue"
import DisplayLabel from "@/partials/DisplayLabel.vue"
import { useLocked, useProgress } from "@/progress"
import { FILTERS_INCREASE, FILTERS_INITIAL_LIMIT, useHasFilters } from "@/search"
import { equals, loadingWidth, useInitialLoad, useLimitResults } from "@/utils"

const props = defineProps<{
  searchSession: DeepReadonly<SearchSession>
  searchTotal: number
  result: HasSearchResult
  filter?: HasFilterEntry
}>()

const locked = useLocked()

const emit = defineEmits<{
  filterUpdate: [filterId: string, filter: HasFilterEntry]
}>()

const { t } = useI18n({ useScope: "global" })

const el = useTemplateRef<HTMLElement>("el")

const labelId = useId()

const abortController = new AbortController()

onBeforeUnmount(() => {
  abortController.abort()
})

const progress = useProgress()

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
    <div :id="labelId" class="flex items-baseline gap-x-1">
      <span class="mb-1.5 text-lg leading-none">{{ t("partials.HasFiltersResult.title") }}</span>
      ({{ result.count }})
    </div>
    <ul ref="el" role="group" :aria-labelledby="labelId" class="grid grid-cols-[max-content_auto] gap-x-1">
      <li v-if="error" class="col-span-2">
        <i class="pd-hasfiltersresult-error text-error-600">{{ t("common.status.loadingDataFailed") }}</i>
      </li>
      <template v-else-if="total === null">
        <li v-for="i in 3" :key="i" class="contents">
          <div class="my-1.5 h-2 w-4 rounded-sm bg-slate-200 motion-safe:animate-pulse" aria-hidden="true"></div>
          <div class="flex items-baseline gap-x-1" aria-hidden="true">
            <div class="my-1.5 h-2 rounded-sm bg-slate-200 motion-safe:animate-pulse" :class="[loadingWidth(`has/${i}`)]"></div>
            <div class="my-1.5 h-2 w-8 rounded-sm bg-slate-200 motion-safe:animate-pulse"></div>
          </div>
        </li>
      </template>
      <template v-else>
        <li v-for="res in limitedResults" :key="res.id" class="contents">
          <CheckBox :id="'has/' + res.id" v-model="checkboxState" :value="res.id" />
          <div class="flex items-baseline gap-x-1">
            <WithDocumentD :id="res.id" name="DocumentGet">
              <template #default="{ doc, url }">
                <label :for="'has/' + res.id" :class="locked ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'" :data-url="url"><DisplayLabel :doc="doc" /></label>
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
            <label :for="'has/' + res.id" :class="locked ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'">({{ res.count }})</label>
            <!--
              tabindex="-1" keeps the open-link icon out of the keyboard tab
              order so Tab jumps between filters without stopping
              on each row's icon. Mouse users can still click it.
            -->
            <RouterLink :to="{ name: 'DocumentGet', params: { id: res.id } }" class="link" tabindex="-1"
              ><ArrowTopRightOnSquareIcon :alt="t('common.icons.link')" class="inline size-5 align-text-top"
            /></RouterLink>
          </div>
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
