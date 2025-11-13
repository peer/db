<script setup lang="ts">
import SearchResultsHeader, { SearchViewType } from "@/partials/SearchResultsHeader.vue"
import { computed, DeepReadonly } from "vue"
import type { ClientSearchState, SearchResult as SearchResultType } from "@/types"

export type SearchResultsFeedProps = {
  searchView: SearchViewType
  searchUrl: Readonly<string | null>
  searchResultsError: string | null
  searchStateError: string | null
  searchMoreThanTotal: boolean
  searchState: DeepReadonly<ClientSearchState | null>
  searchTotal: number | null
  searchResults: DeepReadonly<SearchResultType[]>
}

const props = defineProps<SearchResultsFeedProps>()

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
  <div ref="searchEl" class="w-full flex-auto sm:flex flex-col gap-y-1 sm:gap-y-4" :data-url="searchUrl">
    <div v-if="searchStateError || searchResultsError" class="my-1 sm:my-4">
      <div class="text-center text-sm">
        <i class="text-error-600">loading data failed</i>
      </div>
    </div>

    <SearchResultsHeader
      v-else
      v-model:view="searchViewValue"
      :state="searchState"
      :total="searchTotal"
      :results="searchResults.length"
      :more-than-total="searchMoreThanTotal"
    />
  </div>
</template>

<style scoped></style>
