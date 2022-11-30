<script setup lang="ts">
import type { StringFilterState, StringSearchResult } from "@/types"

import { ref, computed } from "vue"
import Button from "@/components/Button.vue"
import WithDocument from "@/components/WithDocument.vue"
import { useStringFilterValues, NONE, FILTERS_INITIAL_LIMIT, FILTERS_INCREASE } from "@/search"
import { equals, getName, useLimitResults, loadingLength } from "@/utils"

const props = defineProps<{
  searchTotal: number
  result: StringSearchResult
  state: StringFilterState
  updateProgress: number
}>()

const emit = defineEmits<{
  (e: "update:state", state: StringFilterState): void
}>()

const el = ref(null)

const progress = ref(0)
const { results, total } = useStringFilterValues(props.result, el, progress)

const { limitedResults, hasMore, loadMore } = useLimitResults(results, FILTERS_INITIAL_LIMIT, FILTERS_INCREASE)

const limitedResultsWithNone = computed(() => {
  if (!limitedResults.value.length) {
    return limitedResults.value
  } else if (props.result._count >= props.searchTotal) {
    return limitedResults.value
  }
  const res = [...limitedResults.value, { count: props.searchTotal - props.result._count }]
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
    <div class="flex flex-col">
      <div class="flex items-baseline gap-x-1">
        <WithDocument :id="result._id">
          <template #default="{ doc }">
            <RouterLink
              :to="{ name: 'DocumentGet', params: { id: result._id } }"
              class="link mb-1.5 text-lg leading-none"
              v-html="getName(doc.claims) || '<i>no name</i>'"
            ></RouterLink>
          </template>
          <template #loading><div class="inline-block h-2 animate-pulse rounded bg-slate-200" :class="[loadingLength(result._id, 0)]"></div></template>
        </WithDocument>
        ({{ result._count }})
      </div>
      <ul ref="el">
        <li v-for="res in limitedResultsWithNone" :key="'str' in res ? res.str : NONE" class="flex items-baseline gap-x-1">
          <template v-if="'str' in res && (res.count != searchTotal || state.includes(res.str))">
            <input
              :id="'string/' + result._id + '/' + res.str"
              :disabled="updateProgress > 0"
              :checked="state.includes(res.str)"
              :class="
                updateProgress > 0 ? 'cursor-not-allowed bg-gray-100 text-primary-300 focus:ring-primary-300' : 'cursor-pointer text-primary-600 focus:ring-primary-500'
              "
              type="checkbox"
              class="my-1 self-center rounded"
              @change="onChange($event, res.str)"
            />
            <label
              :for="'string/' + result._id + '/' + res.str"
              class="my-1 leading-none"
              :class="updateProgress > 0 ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
              >{{ res.str }}</label
            >
            <label
              :for="'string/' + result._id + '/' + res.str"
              class="my-1 leading-none"
              :class="updateProgress > 0 ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
              >({{ res.count }})</label
            >
          </template>
          <template v-else-if="'str' in res && res.count == searchTotal">
            <div class="my-1 inline-block h-4 w-4 shrink-0 self-center border border-transparent"></div>
            <div class="my-1 leading-none">{{ res.str }}</div>
            <div class="my-1 leading-none">({{ res.count }})</div>
          </template>
          <template v-else-if="!('str' in res)">
            <input
              :id="'string/' + result._id + '/none'"
              :disabled="updateProgress > 0"
              :checked="stateHasNONE()"
              :class="
                updateProgress > 0 ? 'cursor-not-allowed bg-gray-100 text-primary-300 focus:ring-primary-300' : 'cursor-pointer text-primary-600 focus:ring-primary-500'
              "
              type="checkbox"
              class="my-1 self-center rounded"
              @change="onNoneChange($event)"
            />
            <label :for="'string/' + result._id + '/none'" class="my-1 leading-none" :class="updateProgress > 0 ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
              ><i>none</i></label
            >
            <label :for="'string/' + result._id + '/none'" class="my-1 leading-none" :class="updateProgress > 0 ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
              >({{ res.count }})</label
            >
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
