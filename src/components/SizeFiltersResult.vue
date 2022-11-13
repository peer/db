<script setup lang="ts">
import type { API } from "nouislider"
import type { SizeFilterState, SizeSearchResult } from "@/types"

import { ref, computed, watchEffect, onBeforeUnmount } from "vue"
import noUiSlider from "nouislider"
import { useSizeHistogramValues, NONE } from "@/search"
import { formatValue, equals } from "@/utils"

const props = defineProps<{
  searchTotal: number
  result: SizeSearchResult
  state: SizeFilterState
  updateProgress: number
}>()

const emit = defineEmits<{
  (e: "update:state", state: SizeFilterState): void
}>()

const el = ref(null)

const progress = ref(0)
const { results, min, max } = useSizeHistogramValues(el, progress)

function onSliderChange(values: (number | string)[], handle: number, unencoded: number[], tap: boolean, positions: number[], noUiSlider: API) {
  const updatedState = {
    gte: unencoded[0],
    lte: unencoded[1],
  }
  if (!equals(props.state, updatedState)) {
    emit("update:state", updatedState)
  }
}

function onNoneChange(event: Event) {
  let updatedState: typeof NONE | null
  if ((event.target as HTMLInputElement).checked) {
    updatedState = NONE
  } else {
    updatedState = null
  }
  if (!equals(props.state, updatedState)) {
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

let slider: API | null = null
const sliderEl = ref()

watchEffect((onCleanup) => {
  if (slider && slider.target != sliderEl.value) {
    slider.destroy()
    slider = null
  }
  // When sliderEl exists we know that min and max is set as well, and that min != max.
  // Still, we check it here to satisfy type checking.
  if (min.value === null || max.value === null || min.value === max.value) {
    return
  }
  const rangeMin = props.state === null || props.state === NONE ? min.value : Math.max((props.state as { gte: number; lte: number }).gte, min.value)
  const rangeMax = props.state === null || props.state === NONE ? max.value : Math.min((props.state as { gte: number; lte: number }).lte, max.value)
  const rangeStart = props.state === null || props.state === NONE ? min.value : (props.state as { gte: number; lte: number }).gte
  const rangeEnd = props.state === null || props.state === NONE ? max.value : (props.state as { gte: number; lte: number }).lte
  if (!slider && sliderEl.value) {
    slider = noUiSlider.create(sliderEl.value, {
      start: [rangeStart, rangeEnd],
      range: {
        min: [rangeMin],
        max: [rangeMax],
      },
      margin: (rangeMax - rangeMin) / results.value.length,
      connect: [false, true, false],
      // Range is divided by this number to get the keyboard step.
      keyboardDefaultStep: results.value.length,
      keyboardPageMultiplier: 10,
      animate: false,
      behaviour: "snap",
      format: {
        to: (value: number): string => {
          return formatValue(value, "B")
        },
        from: (value: string): number => {
          return parseFloat(value)
        },
      },
    })
    slider.on("change", onSliderChange)
  } else if (slider) {
    slider.updateOptions(
      {
        start: [rangeStart, rangeEnd],
        range: {
          min: [rangeMin],
          max: [rangeMax],
        },
        margin: (rangeMax - rangeMin) / results.value.length,
        // TODO: Uncomment when supported. See: https://github.com/leongersen/noUiSlider/issues/1226
        // keyboardDefaultStep: results.value.length,
      },
      true,
    )
  }
})

watchEffect((onCleanup) => {
  if (!sliderEl.value) {
    return
  }

  // TODO: Handles should not be focused when disabled.
  //       See: https://github.com/leongersen/noUiSlider/issues/1227
  if (props.updateProgress > 0) {
    sliderEl.value.setAttribute("disabled", true)
  } else {
    sliderEl.value.removeAttribute("disabled")
  }
})

onBeforeUnmount(() => {
  if (slider) {
    slider.destroy()
    slider = null
  }
})
</script>

<template>
  <div class="rounded border bg-white p-4 shadow">
    <div class="flex flex-col">
      <div class="flex items-baseline gap-x-1">
        <span class="mb-1.5 text-lg leading-none">document size</span>
        ({{ result._count }})
      </div>
      <ul ref="el">
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
              {{ formatValue(min, "B") }}
            </div>
            <div>
              {{ formatValue(max, "B") }}
            </div>
          </div>
          <div ref="sliderEl"></div>
        </li>
        <li v-else-if="results.length === 1" class="flex gap-x-1">
          <div class="my-1 inline-block h-4 w-4 shrink-0 border border-transparent align-middle"></div>
          <div class="my-1 leading-none">{{ formatValue(results[0].min, "B") }}</div>
          <div class="my-1 leading-none">({{ results[0].count }})</div>
        </li>
        <li v-if="result._count < searchTotal" class="mt-4 flex gap-x-1">
          <input
            :id="'size/none'"
            :disabled="updateProgress > 0"
            :checked="state === NONE"
            :class="
              updateProgress > 0 ? 'cursor-not-allowed bg-gray-100 text-primary-300 focus:ring-primary-300' : 'cursor-pointer text-primary-600 focus:ring-primary-500'
            "
            type="checkbox"
            class="my-1 rounded"
            @change="onNoneChange($event)"
          />
          <label :for="'size/none'" class="my-1 leading-none" :class="updateProgress > 0 ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"><i>none</i></label>
          <label :for="'size/none'" class="my-1 leading-none" :class="updateProgress > 0 ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
            >({{ searchTotal - result._count }})</label
          >
        </li>
      </ul>
    </div>
  </div>
</template>
