<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { ClientSearchState, SearchResult as SearchResultType, SearchViewType } from "@/types"

import { computed, toRef } from "vue"

import SearchResultsHeader from "@/partials/SearchResultsHeader.vue"
import { useLimitResults } from "@/utils.ts"
import WithDocument from "@/components/WithDocument.vue"
import { PeerDBDocument } from "@/document.ts"
import { SEARCH_INCREASE, SEARCH_INITIAL_LIMIT } from "@/search.ts"
import Footer from "@/partials/Footer.vue"

const props = defineProps<{
  searchView: SearchViewType
  searchMoreThanTotal: boolean
  searchState: DeepReadonly<ClientSearchState | null>
  searchTotal: number | null
  searchResults: DeepReadonly<SearchResultType[]>
}>()

const $emit = defineEmits<{
  "update:searchView": [value: SearchViewType]
}>()

const WithPeerDBDocument = WithDocument<PeerDBDocument>

const {
  limitedResults: limitedSearchResults,
  hasMore: searchHasMore,
  loadMore: searchLoadMore,
} = useLimitResults(
  toRef(() => props.searchResults),
  SEARCH_INITIAL_LIMIT,
  SEARCH_INCREASE,
)

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
  <div class="w-full h-full flex-auto sm:flex flex-col gap-y-1 sm:gap-y-4">
    <SearchResultsHeader v-model:search-view="searchViewValue" :search-state="searchState" :search-total="searchTotal" :search-more-than-total="searchMoreThanTotal" />

    <!-- TODO: Calculate height with flex-col and h-full (change structure to the body, header, main , footer) -->
    <div class="shadow bg-white border rounded" style="height: calc(100vh - 215px)">
      <div class="overflow-x-auto overflow-y-auto h-full w-full">
        <table class="table-fixed text-sm min-w-max">
          <thead class="bg-slate-300 sticky top-0 z-10">
            <tr>
              <th class="p-2 min-w-[200px] text-left">Heading 1</th>
              <th class="p-2 min-w-[200px] text-left">Heading 2</th>
              <th class="p-2 min-w-[200px] text-left">Heading 3</th>
              <th class="p-2 min-w-[200px] text-left">Heading 4</th>
              <th class="p-2 min-w-[200px] text-left">Heading 5</th>
              <th class="p-2 min-w-[200px] text-left">Heading 6</th>
            </tr>
          </thead>

          <tbody class="divide-y">
            <tr v-for="result in limitedSearchResults" :key="result.id" class="odd:bg-white even:bg-slate-100 hover:bg-slate-200 cursor-pointer">
              <td class="p-2">
                <WithPeerDBDocument :id="result.id" name="DocumentGet">
                  <template #default="{ doc: resultDoc }"> {{ resultDoc }} </template>
                </WithPeerDBDocument>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>

  <Teleport to="footer">
    <Footer class="border-t border-slate-50 bg-slate-200 shadow" />
  </Teleport>
</template>
