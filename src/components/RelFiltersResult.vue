<script setup lang="ts">
import type { RelFilterState, RelSearchResult } from "@/types"

import { ref, computed } from "vue"
import { ArrowTopRightOnSquareIcon } from "@heroicons/vue/20/solid"
import RouterLink from "@/components/RouterLink.vue"
import Button from "@/components/Button.vue"
import WithDocument from "@/components/WithDocument.vue"
import { useRelFilterValues, NONE, FILTERS_INITIAL_LIMIT, FILTERS_INCREASE } from "@/search"
import { equals, getName, useLimitResults } from "@/utils"

const props = defineProps<{
  searchTotal: number
  result: RelSearchResult
  state: RelFilterState
  updateProgress: number
}>()

const emit = defineEmits<{
  (e: "update:state", state: RelFilterState): void
}>()

const el = ref(null)

const progress = ref(0)
const { results, total } = useRelFilterValues(props.result, el, progress)

const { limitedResults, hasMore, loadMore } = useLimitResults(results, FILTERS_INITIAL_LIMIT, FILTERS_INCREASE)

const limitedResultsWithNone = computed(() => {
  if (!limitedResults.value.length) {
    return limitedResults.value
  } else if (props.result._count >= props.searchTotal) {
    return limitedResults.value
  }
  const res = [...limitedResults.value, { _count: props.searchTotal - props.result._count }]
  res.sort((a, b) => b._count - a._count)
  return res
})

function onChange(event: Event, id: string | typeof NONE) {
  let updatedState = [...props.state]
  if ((event.target as HTMLInputElement).checked) {
    if (!updatedState.includes(id)) {
      updatedState.push(id)
    }
  } else {
    updatedState = updatedState.filter((x) => x !== id)
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
        <WithDocument :id="result._id" v-slot="{ doc }">
          <RouterLink
            :to="{ name: 'DocumentGet', params: { id: result._id } }"
            class="link mb-1.5 text-lg leading-none"
            v-html="getName(doc.claims) || '<i>no name</i>'"
          ></RouterLink>
        </WithDocument>
        ({{ result._count }})
      </div>
      <ul ref="el">
        <li v-for="res in limitedResultsWithNone" :key="'_id' in res ? res._id : NONE" class="flex gap-x-1">
          <template v-if="'_id' in res && (res._count != searchTotal || state.includes(res._id))">
            <input
              :id="'rel/' + result._id + '/' + res._id"
              :disabled="updateProgress > 0"
              :checked="state.includes(res._id)"
              :class="
                updateProgress > 0 ? 'cursor-not-allowed bg-gray-100 text-primary-300 focus:ring-primary-300' : 'cursor-pointer text-primary-600 focus:ring-primary-500'
              "
              type="checkbox"
              class="my-1 rounded"
              @change="onChange($event, res._id)"
            />
            <WithDocument :id="res._id" v-slot="{ doc }">
              <label
                :for="'rel/' + result._id + '/' + res._id"
                class="my-1 leading-none"
                :class="updateProgress > 0 ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
                v-html="getName(doc.claims) || '<i>no name</i>'"
              ></label>
            </WithDocument>
            <label
              :for="'rel/' + result._id + '/' + res._id"
              class="my-1 leading-none"
              :class="updateProgress > 0 ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
              >({{ res._count }})</label
            >
            <RouterLink :to="{ name: 'DocumentGet', params: { id: res._id } }" class="link"
              ><ArrowTopRightOnSquareIcon alt="Link" class="inline h-5 w-5 align-text-top"
            /></RouterLink>
          </template>
          <template v-else-if="'_id' in res && res._count == searchTotal">
            <div class="my-1 inline-block h-4 w-4 shrink-0 border border-transparent align-middle"></div>
            <WithDocument :id="res._id" v-slot="{ doc }">
              <div class="my-1 leading-none" v-html="getName(doc.claims) || '<i>no name</i>'"></div>
            </WithDocument>
            <div class="my-1 leading-none">({{ res._count }})</div>
            <RouterLink :to="{ name: 'DocumentGet', params: { id: res._id } }" class="link"
              ><ArrowTopRightOnSquareIcon alt="Link" class="inline h-5 w-5 align-text-top"
            /></RouterLink>
          </template>
          <template v-else-if="!('_id' in res)">
            <input
              :id="'rel/' + result._id + '/none'"
              :disabled="updateProgress > 0"
              :checked="stateHasNONE()"
              :class="
                updateProgress > 0 ? 'cursor-not-allowed bg-gray-100 text-primary-300 focus:ring-primary-300' : 'cursor-pointer text-primary-600 focus:ring-primary-500'
              "
              type="checkbox"
              class="my-1 rounded"
              @change="onNoneChange($event)"
            />
            <label :for="'rel/' + result._id + '/none'" class="my-1 leading-none" :class="updateProgress > 0 ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
              ><i>none</i></label
            >
            <label :for="'rel/' + result._id + '/none'" class="my-1 leading-none" :class="updateProgress > 0 ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
              >({{ res._count }})</label
            >
          </template>
          <div v-else class="flex animate-pulse">
            <div class="flex-1 space-y-4">
              <div class="my-2 h-2 w-52 rounded bg-slate-200"></div>
            </div>
          </div>
        </li>
      </ul>
      <Button v-if="total !== null && hasMore" :progress="progress" class="mt-2 w-1/2 min-w-fit self-center" @click="loadMore"
        >{{ total - limitedResults.length }} more</Button
      >
      <div v-else-if="total !== null && total > limitedResults.length" class="mt-2 text-center text-sm">{{ total - limitedResults.length }} values not shown.</div>
    </div>
  </div>
</template>
