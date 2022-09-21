<script setup lang="ts">
import type { PeerDBDocument, FilterState } from "@/types"

import { ref, computed } from "vue"
import RouterLink from "@/components/RouterLink.vue"
import { useHistogramValues } from "@/search"
import { formatValue } from "@/utils"

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
const { results, min, max } = useHistogramValues(props.property, progress)

const hasLoaded = computed(() => props.property?.name?.en)

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

const chartWidth = 200
const chartHeight = 30
const barWidth = computed(() => {
  // We assume here that there are at most 100 results so that returns at least 2.
  return chartWidth / results.value.length
})
const maxCount = computed(() => {
  return Math.max(...results.value.map((r) => r.count))
})
</script>

<template>
  <div class="rounded border bg-white p-4 shadow">
    <div v-if="hasLoaded" class="flex flex-col">
      <div class="flex items-baseline gap-x-1">
        <RouterLink :to="{ name: 'DocumentGet', params: { id: property._id } }" class="link mb-1.5 text-lg leading-none">{{ property.name.en }}</RouterLink>
        ({{ property._count }})
      </div>
      <ul>
        <li v-if="min !== null && max !== null && min !== max">
          <!-- We subtract 1 from chartWidth because we subtract 1 from bar width, so there would be a gap after the last one. -->
          <svg :viewBox="`0 0 ${chartWidth - 1} ${chartHeight}`">
            <!-- We subtract 1 from bar width to have a gap between bars. -->
            <rect
              v-for="(res, i) in results"
              :key="i"
              :height="Math.ceil((chartHeight * res.count) / maxCount)"
              :width="barWidth - 1"
              :y="chartHeight - Math.ceil((chartHeight * res.count) / maxCount)"
              :x="i * barWidth"
            ></rect>
          </svg>
          <div class="flex flex-row justify-between gap-x-1">
            <div>
              {{ formatValue(min, property._unit) }}
            </div>
            <div>
              {{ formatValue(max, property._unit) }}
            </div>
          </div>
        </li>
        <li v-if="property._count < searchTotal" class="flex gap-x-1">
          <input
            :id="property._id + '/' + property._unit + '/none'"
            :disabled="updateProgress > 0"
            :checked="state.includes('none')"
            :class="
              updateProgress > 0 ? 'cursor-not-allowed bg-gray-100 text-primary-300 focus:ring-primary-300' : 'cursor-pointer text-primary-600 focus:ring-primary-500'
            "
            type="checkbox"
            class="my-1 rounded"
            @change="onChange($event, 'none')"
          />
          <label
            :for="property._id + '/' + property._unit + '/none'"
            class="my-1 leading-none"
            :class="updateProgress > 0 ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
            ><i>none</i></label
          >
          <label
            :for="property._id + '/' + property._unit + '/none'"
            class="my-1 leading-none"
            :class="updateProgress > 0 ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
            >({{ searchTotal - property._count }})</label
          >
        </li>
      </ul>
    </div>
    <div v-else class="flex animate-pulse">
      <div class="flex-1 space-y-4">
        <div class="h-2 w-72 rounded bg-slate-200"></div>
      </div>
    </div>
  </div>
</template>
