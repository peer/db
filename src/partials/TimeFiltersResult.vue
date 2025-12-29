<script setup lang="ts">
import type { API } from "nouislider"
import type { DeepReadonly } from "vue"

import type { ClientSearchSession, TimeFilterState, TimeSearchResult } from "@/types"

import noUiSlider from "nouislider"
import { computed, onBeforeUnmount, toRef, useTemplateRef, watchEffect } from "vue"

import CheckBox from "@/components/CheckBox.vue"
import DocumentRefInline from "@/partials/DocumentRefInline.vue"
import { injectProgress } from "@/progress"
import { NONE, useTimeHistogramValues } from "@/search"
import { bigIntMax, equals, formatTime, loadingShortHeights, secondsToTimestamp, timestampToSeconds, useInitialLoad } from "@/utils"

const props = defineProps<{
  searchSession: DeepReadonly<ClientSearchSession>
  searchTotal: number
  result: TimeSearchResult
  state: TimeFilterState
  updateProgress: number
}>()

const emit = defineEmits<{
  "update:state": [state: TimeFilterState]
}>()

const el = useTemplateRef<HTMLElement>("el")

const abortController = new AbortController()

onBeforeUnmount(() => {
  abortController.abort()
})

const progress = injectProgress()
const {
  results,
  min,
  max,
  error,
  url: resultsUrl,
} = useTimeHistogramValues(
  toRef(() => props.searchSession),
  toRef(() => props.result),
  el,
  progress,
)
const { laterLoad } = useInitialLoad(progress)

function onSliderChange(values: (number | string)[], handle: number, unencoded: number[], tap: boolean, positions: number[], noUiSlider: API) {
  if (abortController.signal.aborted) {
    return
  }

  const updatedState = {
    gte: values[0] as string,
    lte: values[1] as string,
  }
  if (!equals(props.state, updatedState)) {
    emit("update:state", updatedState)
  }
}

const noneState = computed({
  get(): boolean {
    return props.state === NONE
  },
  set(value: boolean) {
    if (abortController.signal.aborted) {
      return
    }

    const updatedState = value ? NONE : null
    if (!equals(props.state, updatedState)) {
      emit("update:state", updatedState)
    }
  },
})

const chartWidth = 200
const chartHeight = 30
const barWidth = computed(() => {
  // We assume here that there are at most 100 results so that we return at least 2.
  return chartWidth / results.value.length
})
const maxCount = computed(() => {
  return Math.max(...results.value.map((r) => r.count))
})

const maxSafeInteger = BigInt(Number.MAX_SAFE_INTEGER)
const scale = 1024n

let slider: API | null = null
let scaledRange = false
const sliderEl = useTemplateRef<HTMLElement>("sliderEl")

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
  const gte = props.state === null || props.state === NONE ? null : (props.state as { gte?: string; lte?: string }).gte
  const lte = props.state === null || props.state === NONE ? null : (props.state as { gte?: string; lte?: string }).lte
  const bigIntRangeMin = gte == null ? min.value : bigIntMax(timestampToSeconds(gte), min.value)
  const bigIntRangeMax = lte == null ? max.value : bigIntMax(timestampToSeconds(lte), max.value)
  let rangeMin, rangeMax
  if (bigIntRangeMax - bigIntRangeMin > maxSafeInteger) {
    const scaledMin = bigIntRangeMin / scale
    const scaledMax = bigIntRangeMax / scale
    if (scaledMax - scaledMin > maxSafeInteger) {
      throw new Error(`scaling not enough for range [${bigIntRangeMin}, ${bigIntRangeMax}]`)
    }
    rangeMin = Number(scaledMin)
    rangeMax = Number(scaledMax)
    if (bigIntRangeMax % scale !== 0n) {
      // We round up.
      rangeMax++
    }
    scaledRange = true
  } else {
    rangeMin = Number(bigIntRangeMin)
    rangeMax = Number(bigIntRangeMax)
    scaledRange = false
  }
  // rangeStart and rangeEnd are strings because noUiSlider otherwise converts the number
  // to a string using String and tries to parse it with timestampToSeconds.
  // Now it just tries to parse it with timestampToSeconds.
  const rangeStart = gte == null ? secondsToTimestamp(min.value) : gte
  const rangeEnd = lte == null ? secondsToTimestamp(max.value) : lte
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
          let v = BigInt(Math.round(value))
          if (scaledRange) {
            v *= scale
          }
          return secondsToTimestamp(v)
        },
        from: (value: string): number => {
          let s = timestampToSeconds(value)
          if (scaledRange) {
            s /= scale
            if (s > maxSafeInteger) {
              throw new Error(`scaling not enough for timestamp "${value}"`)
            }
          }
          return Number(s)
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
  if (!slider) {
    return
  }

  if (props.updateProgress > 0) {
    slider.disable()
  } else {
    slider.enable()
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
  <div class="flex flex-col rounded border bg-white p-4" :class="{ 'data-reloading': laterLoad }" :data-url="resultsUrl">
    <div class="flex items-baseline gap-x-1">
      <DocumentRefInline :id="result.id" class="mb-1.5 text-lg leading-none" />
      ({{ result.count }})
    </div>
    <ul ref="el">
      <li v-if="error">
        <i class="text-error-600">loading data failed</i>
      </li>
      <li v-else-if="min === null || max === null" class="animate-pulse">
        <div class="my-1.5 grid grid-cols-10 items-end gap-x-1" :style="`aspect-ratio: ${chartWidth - 1} / ${chartHeight}`">
          <div v-for="(h, i) in loadingShortHeights(result.id, 10)" :key="i" class="w-auto rounded-sm bg-slate-200" :class="h"></div>
        </div>
        <div class="flex flex-row justify-between gap-x-1">
          <div class="my-1.5 h-2 w-8 rounded-sm bg-slate-200"></div>
          <div class="my-1.5 h-2 w-8 rounded-sm bg-slate-200"></div>
        </div>
        <div class="my-1.5 h-2 rounded-sm bg-slate-200"></div>
      </li>
      <li v-else-if="min !== max">
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
            {{ formatTime(min) }}
          </div>
          <div>
            {{ formatTime(max) }}
          </div>
        </div>
        <div ref="sliderEl"></div>
      </li>
      <li v-else-if="results.length === 1" class="flex items-baseline gap-x-1">
        <div class="my-1 inline-block h-4 w-4 shrink-0 self-center border border-transparent"></div>
        <div class="my-1 leading-none">{{ formatTime(timestampToSeconds(results[0].min)) }}</div>
        <div class="my-1 leading-none">({{ results[0].count }})</div>
      </li>
      <li v-if="result.count < searchTotal" class="flex items-baseline gap-x-1 first:mt-0" :class="error ? 'mt-0' : min === null || max === null ? 'mt-3' : 'mt-4'">
        <CheckBox :id="'time/' + result.id + '/none'" v-model="noneState" :progress="updateProgress" class="my-1 self-center" />
        <label :for="'time/' + result.id + '/none'" class="my-1 leading-none" :class="updateProgress > 0 ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
          ><i>none</i></label
        >
        <label :for="'time/' + result.id + '/none'" class="my-1 leading-none" :class="updateProgress > 0 ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
          >({{ searchTotal - result.count }})</label
        >
      </li>
    </ul>
  </div>
</template>
