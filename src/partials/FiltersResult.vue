<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { AmountFilterEntry, Filter, FilterResult, FilterUpdate, HasFilterEntry, RefFilterEntry, SearchSession, SpecialsFilterEntry, TimeFilterEntry } from "@/types"

import { computed, onBeforeUnmount } from "vue"

import AmountFiltersResult from "@/partials/AmountFiltersResult.vue"
import HasFiltersResult from "@/partials/HasFiltersResult.vue"
import RefFiltersResult from "@/partials/RefFiltersResult.vue"
import TimeFiltersResult from "@/partials/TimeFiltersResult.vue"

const props = withDefaults(
  defineProps<{
    result: FilterResult
    searchSession: DeepReadonly<SearchSession>
    filters: Filter[]
    // Free-text query narrowing reference and has facet values to those whose value or property name
    // matches; empty means no narrowing. It is forwarded only to reference and has facets, the ones with
    // searchable values; amount and time facets carry no such values, so they render in full (the facet list
    // itself is already narrowed to matching facets by the backend).
    query?: string
  }>(),
  {
    query: "",
  },
)

const $emit = defineEmits<{
  filterUpdates: [updates: FilterUpdate[]]
}>()

// We have to explicitly pass attributes because we use multiple root nodes.
defineOptions({
  inheritAttrs: false,
})

const abortController = new AbortController()

onBeforeUnmount(() => {
  abortController.abort()
})

// A CSS class naming which facet this is. The facet's type followed by the identifiers of its property path.
// The type is part of it because a reference facet and a has facet can share the same property path. A has
// facet on the document itself has no property path and is named by its type alone.
const filterClass = computed(() => ["pd-filtersresult", props.result.type, ...(props.result.props ?? [])].join("-"))

// samePath reports whether a filter's property path equals a facet's property path.
function samePath(a: readonly string[] | undefined, b: readonly string[] | undefined): boolean {
  const aa = a ?? []
  const bb = b ?? []
  return aa.length === bb.length && aa.every((v, i) => v === bb[i])
}

// The active filters of a facet's path are matched by the path (and unit for amount facets), not by the
// facet's filterId: a path can have both a typed filter and a specials filter active, and a facet needs
// both to render its selections.
function findRefFilter(result: FilterResult): RefFilterEntry | undefined {
  return props.filters.find((f): f is RefFilterEntry => "ref" in f && samePath(f.prop, result.props))
}

function findAmountFilter(result: FilterResult): AmountFilterEntry | undefined {
  return props.filters.find(
    (f): f is AmountFilterEntry => "amount" in f && samePath(f.prop, result.props) && f.amount.unit === (result.type === "amount" ? result.unit : undefined),
  )
}

function findTimeFilter(result: FilterResult): TimeFilterEntry | undefined {
  return props.filters.find((f): f is TimeFilterEntry => "time" in f && samePath(f.prop, result.props))
}

function findHasFilter(result: FilterResult): HasFilterEntry | undefined {
  return props.filters.find((f): f is HasFilterEntry => "has" in f && samePath(f.prop, result.props))
}

function findSpecialsFilter(result: FilterResult): SpecialsFilterEntry | undefined {
  return props.filters.find((f): f is SpecialsFilterEntry => "specials" in f && samePath(f.prop, result.props))
}

function onFilterUpdates(updates: FilterUpdate[]) {
  if (abortController.signal.aborted) {
    return
  }

  $emit("filterUpdates", updates)
}
</script>

<template>
  <RefFiltersResult
    v-if="result.type === 'ref'"
    class="pd-filtersresult"
    :class="filterClass"
    :search-session="searchSession"
    :result="result"
    :filter="findRefFilter(result)"
    :specials="findSpecialsFilter(result)"
    :query="query"
    v-bind="$attrs"
    @filter-updates="onFilterUpdates"
  />

  <AmountFiltersResult
    v-if="result.type === 'amount'"
    class="pd-filtersresult"
    :class="filterClass"
    :search-session="searchSession"
    :result="result"
    :filter="findAmountFilter(result)"
    :specials="findSpecialsFilter(result)"
    v-bind="$attrs"
    @filter-updates="onFilterUpdates"
  />

  <TimeFiltersResult
    v-if="result.type === 'time'"
    class="pd-filtersresult"
    :class="filterClass"
    :search-session="searchSession"
    :result="result"
    :filter="findTimeFilter(result)"
    :specials="findSpecialsFilter(result)"
    v-bind="$attrs"
    @filter-updates="onFilterUpdates"
  />

  <HasFiltersResult
    v-if="result.type === 'has'"
    class="pd-filtersresult"
    :class="filterClass"
    :search-session="searchSession"
    :result="result"
    :filter="findHasFilter(result)"
    :query="query"
    v-bind="$attrs"
    @filter-updates="onFilterUpdates"
  />
</template>
