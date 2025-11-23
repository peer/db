<script setup lang="ts">
import type { ClientSearchState, SearchResult as SearchResultType, SearchViewType } from "@/types"

import SearchResultsHeader from "@/partials/SearchResultsHeader.vue"

import { computed, DeepReadonly } from "vue"

const props = defineProps<{
  searchView: SearchViewType
  searchResults: DeepReadonly<SearchResultType[]>
  searchTotal: number | null
  searchMoreThanTotal: boolean
  searchState: DeepReadonly<ClientSearchState | null>
}>()

const $emit = defineEmits<{
  "update:searchView": [value: SearchViewType]
}>()

const searchViewValue = computed({
  get() {
    return props.searchView
  },
  set(value) {
    $emit("update:searchView", value)
  },
})
</script>

<template>
  <div class="w-full flex-auto sm:flex flex-col gap-y-1 sm:gap-y-4">
    <SearchResultsHeader
      v-model:search-view="searchViewValue"
      :state="searchState"
      :total="searchTotal"
      :results="searchResults.length"
      :more-than-total="searchMoreThanTotal"
    />
  </div>
</template>
