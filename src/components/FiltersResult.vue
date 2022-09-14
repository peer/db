<script setup lang="ts">
import type { PeerDBDocument, FilterState } from "@/types"

import { ref, computed } from "vue"
import { ArrowTopRightOnSquareIcon } from "@heroicons/vue/20/solid"
import RouterLink from "@/components/RouterLink.vue"
import Button from "@/components/Button.vue"
import { useFilterValues } from "@/search"

const props = defineProps<{
  searchTotal: number
  property: PeerDBDocument
  state: FilterState
  updateProgress: number
}>()

const emit = defineEmits<{
  (e: "update:state", state: FilterState): void
}>()

const progress = ref(0)
const { docs, total, hasMore, loadMore } = useFilterValues(props.property, progress)

const hasLoaded = computed(() => props.property?.name?.en)
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

function onChange(event: Event, id: string) {
  let updatedState = [...props.state]
  if ((event.target as HTMLInputElement).checked) {
    if (!updatedState.includes(id)) {
      updatedState.push(id)
    }
  } else {
    updatedState = updatedState.filter((x) => x !== id)
  }
  if (JSON.stringify(props.state) !== JSON.stringify(updatedState)) {
    emit("update:state", updatedState)
  }
}
</script>

<template>
  <div class="rounded border bg-white p-4 shadow">
    <div v-if="hasLoaded" class="flex flex-col">
      <div class="flex items-baseline gap-x-1">
        <RouterLink :to="{ name: 'DocumentGet', params: { id: property._id } }" class="link mb-1.5 text-lg leading-none">{{ property.name.en }}</RouterLink>
        ({{ property._count }})
      </div>
      <ul>
        <li v-for="doc in docsWithNone" :key="doc._id" class="flex gap-x-1">
          <template v-if="doc.name?.en">
            <input
              :id="property._id + '/' + doc._id"
              :disabled="updateProgress > 0"
              :checked="state.includes(doc._id)"
              :class="
                updateProgress > 0 ? 'cursor-not-allowed bg-gray-100 text-primary-300 focus:ring-primary-300' : 'cursor-pointer text-primary-600 focus:ring-primary-500'
              "
              type="checkbox"
              class="my-1 rounded"
              @change="onChange($event, doc._id)"
            />
            <label :for="property._id + '/' + doc._id" class="my-1 leading-none" :class="updateProgress > 0 ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'">{{
              doc.name.en
            }}</label>
            <label :for="property._id + '/' + doc._id" class="my-1 leading-none" :class="updateProgress > 0 ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
              >({{ doc._count }})</label
            >
            <RouterLink :to="{ name: 'DocumentGet', params: { id: doc._id } }" class="link"
              ><ArrowTopRightOnSquareIcon alt="Link" class="inline h-5 w-5 align-text-top"
            /></RouterLink>
          </template>
          <template v-else-if="!doc._id">
            <input
              :id="property._id + '/none'"
              :disabled="updateProgress > 0"
              :checked="state.includes('none')"
              :class="
                updateProgress > 0 ? 'cursor-not-allowed bg-gray-100 text-primary-300 focus:ring-primary-300' : 'cursor-pointer text-primary-600 focus:ring-primary-500'
              "
              type="checkbox"
              class="my-1 rounded"
              @change="onChange($event, 'none')"
            />
            <label :for="property._id + '/none'" class="my-1 leading-none" :class="updateProgress > 0 ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
              ><i>none</i></label
            >
            <label :for="property._id + '/none'" class="my-1 leading-none" :class="updateProgress > 0 ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
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
      <Button v-if="hasMore" :progress="progress" class="mt-2 w-1/2 min-w-fit self-center" @click="loadMore">{{ total - docs.length }} more</Button>
      <div v-else-if="total > docs.length" class="mt-2 text-center text-sm">{{ total - docs.length }} values not shown.</div>
    </div>
    <div v-else class="flex animate-pulse">
      <div class="flex-1 space-y-4">
        <div class="h-2 w-72 rounded bg-slate-200"></div>
      </div>
    </div>
  </div>
</template>
