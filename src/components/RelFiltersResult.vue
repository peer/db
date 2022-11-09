<script setup lang="ts">
import type { PeerDBDocument, RelFilterState, RelSearchResult } from "@/types"

import { ref, computed } from "vue"
import { ArrowTopRightOnSquareIcon } from "@heroicons/vue/20/solid"
import RouterLink from "@/components/RouterLink.vue"
import Button from "@/components/Button.vue"
import { useRelFilterValues, NONE } from "@/search"
import { equals, getName } from "@/utils"

const props = defineProps<{
  searchTotal: number
  property: PeerDBDocument & RelSearchResult
  state: RelFilterState
  updateProgress: number
}>()

const emit = defineEmits<{
  (e: "update:state", state: RelFilterState): void
}>()

const progress = ref(0)
const { docs, total, hasMore, loadMore } = useRelFilterValues(props.property, progress)

const hasLoaded = computed(() => props.property?.claims)
const propertyName = computed(() => getName(props.property?.claims))
const docsWithNone = computed(() => {
  if (!docs.value.length) {
    return docs.value
  } else if (props.property._count >= props.searchTotal) {
    return docs.value
  }
  const res = [...docs.value, { _count: props.searchTotal - props.property._count }]
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
        <li v-for="doc in docsWithNone" :key="'_id' in doc ? doc._id : NONE" class="flex gap-x-1">
          <template v-if="'_id' in doc && doc.claims && (doc._count != searchTotal || state.includes(doc._id))">
            <input
              :id="'rel/' + property._id + '/' + doc._id"
              :disabled="updateProgress > 0"
              :checked="state.includes(doc._id)"
              :class="
                updateProgress > 0 ? 'cursor-not-allowed bg-gray-100 text-primary-300 focus:ring-primary-300' : 'cursor-pointer text-primary-600 focus:ring-primary-500'
              "
              type="checkbox"
              class="my-1 rounded"
              @change="onChange($event, doc._id)"
            />
            <label
              :for="'rel/' + property._id + '/' + doc._id"
              class="my-1 leading-none"
              :class="updateProgress > 0 ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
              v-html="getName(doc.claims) || '<i>untitled</i>'"
            ></label>
            <label
              :for="'rel/' + property._id + '/' + doc._id"
              class="my-1 leading-none"
              :class="updateProgress > 0 ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
              >({{ doc._count }})</label
            >
            <RouterLink :to="{ name: 'DocumentGet', params: { id: doc._id } }" class="link"
              ><ArrowTopRightOnSquareIcon alt="Link" class="inline h-5 w-5 align-text-top"
            /></RouterLink>
          </template>
          <template v-else-if="'_id' in doc && doc.claims && doc._count == searchTotal">
            <div class="my-1 inline-block h-4 w-4 shrink-0 border border-transparent align-middle"></div>
            <div class="my-1 leading-none" v-html="getName(doc.claims) || '<i>untitled</i>'"></div>
            <div class="my-1 leading-none">({{ doc._count }})</div>
            <RouterLink :to="{ name: 'DocumentGet', params: { id: doc._id } }" class="link"
              ><ArrowTopRightOnSquareIcon alt="Link" class="inline h-5 w-5 align-text-top"
            /></RouterLink>
          </template>
          <template v-else-if="!('_id' in doc)">
            <input
              :id="'rel/' + property._id + '/none'"
              :disabled="updateProgress > 0"
              :checked="stateHasNONE()"
              :class="
                updateProgress > 0 ? 'cursor-not-allowed bg-gray-100 text-primary-300 focus:ring-primary-300' : 'cursor-pointer text-primary-600 focus:ring-primary-500'
              "
              type="checkbox"
              class="my-1 rounded"
              @change="onNoneChange($event)"
            />
            <label :for="'rel/' + property._id + '/none'" class="my-1 leading-none" :class="updateProgress > 0 ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
              ><i>none</i></label
            >
            <label :for="'rel/' + property._id + '/none'" class="my-1 leading-none" :class="updateProgress > 0 ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
              >({{ doc._count }})</label
            >
          </template>
          <div v-else class="flex animate-pulse">
            <div class="flex-1 space-y-4">
              <div class="my-2 h-2 w-52 rounded bg-slate-200"></div>
            </div>
          </div>
        </li>
      </ul>
      <Button v-if="total !== null && hasMore" :progress="progress" class="mt-2 w-1/2 min-w-fit self-center" @click="loadMore">{{ total - docs.length }} more</Button>
      <div v-else-if="total !== null && total > docs.length" class="mt-2 text-center text-sm">{{ total - docs.length }} values not shown.</div>
    </div>
    <div v-else class="flex animate-pulse">
      <div class="flex-1 space-y-4">
        <div class="h-2 w-72 rounded bg-slate-200"></div>
      </div>
    </div>
  </div>
</template>
