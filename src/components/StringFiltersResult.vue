<script setup lang="ts">
import type { PeerDBDocument, StringFilterState, StringSearchResult } from "@/types"

import { ref, computed } from "vue"
import Button from "@/components/Button.vue"
import { useStringFilterValues, NONE } from "@/search"
import { equals, getName } from "@/utils"

const props = defineProps<{
  searchTotal: number
  property: PeerDBDocument & StringSearchResult
  state: StringFilterState
  updateProgress: number
}>()

const emit = defineEmits<{
  (e: "update:state", state: StringFilterState): void
}>()

const progress = ref(0)
const { limitedResults, total, hasMore, loadMore } = useStringFilterValues(props.property, progress)

const hasLoaded = computed(() => props.property?.claims)
const propertyName = computed(() => getName(props.property?.claims))
const limitedResultsWithNone = computed(() => {
  if (!limitedResults.value.length) {
    return limitedResults.value
  } else if (props.property._count >= props.searchTotal) {
    return limitedResults.value
  }
  const res = [...limitedResults.value, { count: props.searchTotal - props.property._count }]
  res.sort((a, b) => b.count - a.count)
  return res
})

function onChange(event: Event, str: string | typeof NONE) {
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

// Workaround for a bug in Vue where type of NONE changes inside template,
// so we cannot simply use onChange($event, NONE) in the template.
// See: https://github.com/vuejs/core/issues/6817
function onNoneChange(event: Event) {
  onChange(event, NONE)
}

// Workaround for a bug in Vue where type of NONE changes inside template,
// so we cannot simply use state.includes(NONE) in the template.
// See: https://github.com/vuejs/core/issues/6817
function stateHasNONE(): boolean {
  return props.state.includes(NONE)
}
</script>

<template>
  <div class="rounded border bg-white p-4 shadow">
    <div v-if="hasLoaded" class="flex flex-col">
      <div class="flex items-baseline gap-x-1">
        <RouterLink
          :to="{ name: 'DocumentGet', params: { id: property._id } }"
          class="link mb-1.5 text-lg leading-none"
          v-html="propertyName || '<i>untitled</i>'"
        ></RouterLink>
        ({{ property._count }})
      </div>
      <ul>
        <li v-for="result in limitedResultsWithNone" :key="'str' in result ? result.str : NONE" class="flex gap-x-1">
          <template v-if="'str' in result && (result.count != searchTotal || state.includes(result.str))">
            <input
              :id="'string/' + property._id + '/' + result.str"
              :disabled="updateProgress > 0"
              :checked="state.includes(result.str)"
              :class="
                updateProgress > 0 ? 'cursor-not-allowed bg-gray-100 text-primary-300 focus:ring-primary-300' : 'cursor-pointer text-primary-600 focus:ring-primary-500'
              "
              type="checkbox"
              class="my-1 rounded"
              @change="onChange($event, result.str)"
            />
            <label
              :for="'string/' + property._id + '/' + result.str"
              class="my-1 leading-none"
              :class="updateProgress > 0 ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
              >{{ result.str }}</label
            >
            <label
              :for="'string/' + property._id + '/' + result.str"
              class="my-1 leading-none"
              :class="updateProgress > 0 ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
              >({{ result.count }})</label
            >
          </template>
          <template v-else-if="'str' in result && result.count == searchTotal">
            <div class="my-1 inline-block h-4 w-4 shrink-0 border border-transparent align-middle"></div>
            <div class="my-1 leading-none">{{ result.str }}</div>
            <div class="my-1 leading-none">({{ result.count }})</div>
          </template>
          <template v-else-if="!('str' in result)">
            <input
              :id="'string/' + property._id + '/none'"
              :disabled="updateProgress > 0"
              :checked="stateHasNONE()"
              :class="
                updateProgress > 0 ? 'cursor-not-allowed bg-gray-100 text-primary-300 focus:ring-primary-300' : 'cursor-pointer text-primary-600 focus:ring-primary-500'
              "
              type="checkbox"
              class="my-1 rounded"
              @change="onNoneChange($event)"
            />
            <label :for="'string/' + property._id + '/none'" class="my-1 leading-none" :class="updateProgress > 0 ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
              ><i>none</i></label
            >
            <label :for="'string/' + property._id + '/none'" class="my-1 leading-none" :class="updateProgress > 0 ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
              >({{ result.count }})</label
            >
          </template>
        </li>
      </ul>
      <Button v-if="total !== null && hasMore" :progress="progress" class="mt-2 w-1/2 min-w-fit self-center" @click="loadMore"
        >{{ total - limitedResults.length }} more</Button
      >
      <div v-else-if="total !== null && total > limitedResults.length" class="mt-2 text-center text-sm">{{ total - limitedResults.length }} values not shown.</div>
    </div>
    <div v-else class="flex animate-pulse">
      <div class="flex-1 space-y-4">
        <div class="h-2 w-72 rounded bg-slate-200"></div>
      </div>
    </div>
  </div>
</template>
