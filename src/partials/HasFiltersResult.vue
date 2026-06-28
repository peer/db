<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { D } from "@/document"
import type { HasFilterEntry, HasSearchResult, HasValue, SearchSession } from "@/types"

import { ArrowTopRightOnSquareIcon } from "@heroicons/vue/20/solid"
import { computed, onBeforeUnmount, ref, toRef, useId, useTemplateRef } from "vue"
import { useI18n } from "vue-i18n"

import Button from "@/components/Button.vue"
import CheckBox from "@/components/CheckBox.vue"
import WithDocument from "@/components/WithDocument.vue"
import DisplayLabel from "@/partials/DisplayLabel.vue"
import FilterPropLabel from "@/partials/FilterPropLabel.vue"
import { useLocked, useProgress } from "@/progress"
import { FILTERS_INCREASE, FILTERS_INITIAL_LIMIT, useHasFilterMatches, useHasFilters } from "@/search"
import { equals, loadingWidth, useInitialLoad, useLimitResults, useReportFilterVisibility } from "@/utils"

const props = withDefaults(
  defineProps<{
    searchSession: DeepReadonly<SearchSession>
    result: HasSearchResult
    filter?: HasFilterEntry
    // Free-text query that narrows the listed properties to those whose name matches it. Empty means no narrowing.
    query?: string
  }>(),
  {
    filter: undefined,
    query: "",
  },
)

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

const session = toRef(() => props.searchSession)
const propsRef = computed(() => props.result.props ?? [])

// EMPTY is a stable, always-empty value query so the primary facet always fetches the unfiltered (q="")
// results and refetches only when the session/version or props change. The primary results are the single
// source of counts; the value search is layered on top as a visual overlay that only hides properties.
const EMPTY = ref("")

const primary = useHasFilters(session, filterId, propsRef, EMPTY, el, progress)
const matches = useHasFilterMatches(
  session,
  filterId,
  propsRef,
  toRef(() => props.query),
  el,
  progress,
)

// Template-facing aliases for the primary facet (top-level refs unwrap in the template).
const error = primary.error
const resultsUrl = primary.url
const loading = computed(() => primary.total.value === null)

const { laterLoad } = useInitialLoad(progress)

const searching = computed(() => props.query !== "")

// The overlay is active only once the value search has been typed and its matches have returned. While the
// match fetch is still in flight (matches.total is null) the overlay is inactive, so all primary properties
// stay shown and the facet does not flicker to a hidden or empty state.
const overlayActive = computed(() => searching.value && matches.total.value !== null)

// The primary properties narrowed to the value search overlay: every property stays when the overlay is
// inactive, otherwise only those whose id matched. Has filters are flat, so there are no ancestors or "direct"
// entries to keep, the visible set is exactly the matched ids.
const visibleResults = computed(() => {
  if (!overlayActive.value) {
    return primary.results.value
  }
  const ids = matches.matchedIds.value
  return primary.results.value.filter((res) => ids.has(res.id))
})

// While the value search is active and no property in this facet is visible, the whole facet is hidden so the
// filter pane shows only facets with matching values. Until the overlay is active the facet stays visible.
const hiddenByQuery = computed(() => overlayActive.value && visibleResults.value.length === 0)

// Report visibility to the filter pane so it can show the no-match message only when no facet is visible.
useReportFilterVisibility(() => !hiddenByQuery.value)

// Extract the selected prop IDs from the filter value.
const selectedIds = computed((): string[] => {
  if (!props.filter?.has?.props) {
    return []
  }
  return props.filter.has.props.map((p: HasValue) => p.id)
})

// Reorder so selected options come first (each group keeps the count-desc order from the API).
const sortedResults = computed(() => {
  const selected = new Set<string>(selectedIds.value)
  return [...visibleResults.value.filter((res) => selected.has(res.id)), ...visibleResults.value.filter((res) => !selected.has(res.id))]
})

const { limitedResults, hasMore, loadMore } = useLimitResults(sortedResults, FILTERS_INITIAL_LIMIT, FILTERS_INCREASE)

// The number of properties the user can still reach: the whole facet count from the primary results when not
// searching, or the count of the currently visible overlay when a value search is active.
const displayTotal = computed((): number => (overlayActive.value ? visibleResults.value.length : (primary.total.value ?? 0)))

function clearFilter() {
  if (abortController.signal.aborted || !props.filter) {
    return
  }
  emit("filterUpdate", props.filter.id, {
    id: props.filter.id,
    base: props.filter.base,
    prop: props.filter.prop,
    has: { props: undefined },
  })
}

const checkboxState = computed({
  get(): string[] {
    return [...selectedIds.value]
  },
  set(value: string[]) {
    if (abortController.signal.aborted) {
      return
    }

    const hasProps: HasValue[] | undefined = value.length > 0 ? value.map((id) => ({ id })) : undefined

    // Build the updated filter. prop carries the parentProp for sub-has filters,
    // and is empty for top-level has filters.
    const updatedFilter: HasFilterEntry = {
      id: props.filter?.id ?? "",
      base: props.filter?.base ?? [],
      prop: props.filter?.prop ?? (props.result.props ? [...props.result.props] : []),
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
  <div v-if="!hiddenByQuery" class="pd-hasfiltersresult flex flex-col" :class="{ 'data-reloading': laterLoad }" :data-url="resultsUrl">
    <div :id="labelId">
      <Button
        v-if="filter"
        type="button"
        class="float-right ml-2 px-2.5 py-1"
        :title="t('partials.HasFiltersResult.clearFilter')"
        :aria-label="t('partials.HasFiltersResult.clearFilter')"
        @click.prevent="clearFilter"
        >{{ t("common.buttons.clear") }}</Button
      >
      <span class="mb-1.5 text-lg leading-none"><FilterPropLabel :prop-ids="result.props ?? []" append-has /></span>
      ({{ result.count }})
    </div>
    <ul ref="el" role="group" :aria-labelledby="labelId" class="grid grid-cols-[max-content_auto] gap-x-1">
      <li v-if="error" class="col-span-2">
        <i class="pd-hasfiltersresult-error text-error-600">{{ t("common.status.loadingDataFailed") }}</i>
      </li>
      <template v-else-if="loading">
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
          <CheckBox :id="'has/' + (result.props?.join('/') ?? '') + '/' + res.id" v-model="checkboxState" :value="res.id" />
          <div class="flex items-baseline gap-x-1">
            <WithDocumentD :id="res.id" name="DocumentGet">
              <template #default="{ doc, url }">
                <label
                  :for="'has/' + (result.props?.join('/') ?? '') + '/' + res.id"
                  :class="locked ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
                  :data-url="url"
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
            <label :for="'has/' + (result.props?.join('/') ?? '') + '/' + res.id" :class="locked ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
              >({{ res.count }})</label
            >
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
    <Button v-if="!loading && hasMore" primary class="mt-2 w-1/2 min-w-fit self-center" @click.prevent="loadMore">{{
      t("common.buttons.loadCountMore", { count: displayTotal - limitedResults.length })
    }}</Button>
    <div v-else-if="!loading && displayTotal > limitedResults.length" class="mt-2 text-center text-sm">
      {{ t("common.status.valuesNotShown", { count: displayTotal - limitedResults.length }) }}
    </div>
  </div>
</template>
