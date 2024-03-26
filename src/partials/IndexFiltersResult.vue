<script setup lang="ts">
import type { IndexFilterState, IndexSearchResult } from "@/types"

import { computed, onBeforeUnmount, ref } from "vue"
import Button from "@/components/Button.vue"
import CheckBox from "@/components/CheckBox.vue"
import { useIndexFilterValues, FILTERS_INITIAL_LIMIT, FILTERS_INCREASE } from "@/search"
import { equals, useLimitResults, loadingWidth, useInitialLoad } from "@/utils"
import { injectProgress } from "@/progress"

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

const abortController = new AbortController()

onBeforeUnmount(() => {
  abortController.abort()
})

const progress = injectProgress()
const { results, total, error, url } = useIndexFilterValues(el, progress)
const { laterLoad } = useInitialLoad(progress)

const { limitedResults, hasMore, loadMore } = useLimitResults(results, FILTERS_INITIAL_LIMIT, FILTERS_INCREASE)

const checkboxState = computed({
  get(): IndexFilterState {
    return props.state
  },
  set(value: IndexFilterState) {
    if (abortController.signal.aborted) {
      return
    }

    if (!equals(props.state, value)) {
      emit("update:state", value)
    }
  },
})
</script>

<template>
  <div class="flex flex-col rounded border bg-white p-4 shadow" :class="{ 'data-reloading': laterLoad }" :data-url="url">
    <div class="flex items-baseline gap-x-1">
      <span class="mb-1.5 text-lg leading-none">document index</span>
      ({{ result.count }})
    </div>
    <ul ref="el">
      <li v-if="error">
        <i class="text-error-600">loading data failed</i>
      </li>
      <template v-else-if="total === null">
        <li v-for="i in 3" :key="i" class="flex animate-pulse items-baseline gap-x-1">
          <div class="my-1.5 h-2 w-4 rounded bg-slate-200"></div>
          <div class="my-1.5 h-2 rounded bg-slate-200" :class="[loadingWidth(`index/${i}`)]"></div>
          <div class="my-1.5 h-2 w-8 rounded bg-slate-200"></div>
        </li>
      </template>
      <template v-else>
        <li v-for="res in limitedResults" :key="res.str" class="flex items-baseline gap-x-1">
          <template v-if="res.count != props.searchTotal || state.includes(res.str)">
            <CheckBox :id="'index/' + res.str" v-model="checkboxState" :progress="updateProgress" :value="res.str" class="my-1 self-center" />
            <label :for="'index/' + res.str" class="my-1 leading-none" :class="updateProgress > 0 ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'">{{
              res.str
            }}</label>
            <label :for="'index/' + res.str" class="my-1 leading-none" :class="updateProgress > 0 ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
              >({{ res.count }})</label
            >
          </template>
          <template v-else-if="res.count == props.searchTotal">
            <div class="my-1 inline-block h-4 w-4 shrink-0 self-center border border-transparent"></div>
            <div class="my-1 leading-none">{{ res.str }}</div>
            <div class="my-1 leading-none">({{ res.count }})</div>
          </template>
        </li>
      </template>
    </ul>
    <Button v-if="total !== null && hasMore" primary class="mt-2 w-1/2 min-w-fit self-center" @click="loadMore">{{ total - limitedResults.length }} more</Button>
    <div v-else-if="total !== null && total > limitedResults.length" class="mt-2 text-center text-sm">{{ total - limitedResults.length }} values not shown.</div>
  </div>
</template>
