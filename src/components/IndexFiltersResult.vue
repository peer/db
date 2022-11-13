<script setup lang="ts">
import type { IndexFilterState, IndexSearchResult } from "@/types"

import { ref } from "vue"
import Button from "@/components/Button.vue"
import { useIndexFilterValues, FILTERS_INITIAL_LIMIT, FILTERS_INCREASE } from "@/search"
import { equals, useLimitResults } from "@/utils"

const props = defineProps<{
  searchTotal: number
  result: IndexSearchResult
  state: IndexFilterState
  updateProgress: number
}>()

const emit = defineEmits<{
  (e: "update:state", state: IndexFilterState): void
}>()

const el = ref(null)

const progress = ref(0)
const { results, total } = useIndexFilterValues(el, progress)

const { limitedResults, hasMore, loadMore } = useLimitResults(results, FILTERS_INITIAL_LIMIT, FILTERS_INCREASE)

function onChange(event: Event, str: string) {
  let updatedState = [...props.state]
  if ((event.target as HTMLInputElement).checked) {
    if (!updatedState.includes(str)) {
      updatedState.push(str)
    }
  } else {
    updatedState = updatedState.filter((x) => x !== str)
  }
  if (!equals(props.state, updatedState)) {
    emit("update:state", updatedState)
  }
}
</script>

<template>
  <div class="rounded border bg-white p-4 shadow">
    <div class="flex flex-col">
      <div class="flex items-baseline gap-x-1">
        <span class="mb-1.5 text-lg leading-none">document index</span>
        ({{ result._count }})
      </div>
      <ul ref="el">
        <li v-for="res in limitedResults" :key="res.str" class="flex gap-x-1">
          <template v-if="res.count != props.searchTotal || state.includes(res.str)">
            <input
              :id="'index/' + res.str"
              :disabled="updateProgress > 0"
              :checked="state.includes(res.str)"
              :class="
                updateProgress > 0 ? 'cursor-not-allowed bg-gray-100 text-primary-300 focus:ring-primary-300' : 'cursor-pointer text-primary-600 focus:ring-primary-500'
              "
              type="checkbox"
              class="my-1 rounded"
              @change="onChange($event, res.str)"
            />
            <label :for="'index/' + res.str" class="my-1 leading-none" :class="updateProgress > 0 ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'">{{
              res.str
            }}</label>
            <label :for="'index/' + res.str" class="my-1 leading-none" :class="updateProgress > 0 ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
              >({{ res.count }})</label
            >
          </template>
          <template v-else-if="res.count == props.searchTotal">
            <div class="my-1 inline-block h-4 w-4 shrink-0 border border-transparent align-middle"></div>
            <div class="my-1 leading-none">{{ res.str }}</div>
            <div class="my-1 leading-none">({{ res.count }})</div>
          </template>
        </li>
      </ul>
      <Button v-if="total !== null && hasMore" :progress="progress" class="mt-2 w-1/2 min-w-fit self-center" @click="loadMore"
        >{{ total - limitedResults.length }} more</Button
      >
      <div v-else-if="total !== null && total > limitedResults.length" class="mt-2 text-center text-sm">{{ total - limitedResults.length }} values not shown.</div>
    </div>
  </div>
</template>
