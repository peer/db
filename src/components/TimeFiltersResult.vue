<script setup lang="ts">
import type { API } from "nouislider"
import type { TimeFilterState, TimeSearchResult } from "@/types"

import { ref, computed, watchEffect, onBeforeUnmount } from "vue"
import noUiSlider from "nouislider"
import WithDocument from "@/components/WithDocument.vue"
import { useTimeHistogramValues, NONE } from "@/search"
import { timestampToSeconds, secondsToTimestamp, formatTime, bigIntMax, equals, getName, loadingWidth, useInitialLoad, loadingShortHeights } from "@/utils"
import { injectProgress } from "@/progress"

const props = defineProps<{
  searchTotal: number
  result: TimeSearchResult
  state: TimeFilterState
  updateProgress: number
}>()

const emit = defineEmits<{
  (e: "update:state", state: TimeFilterState): void
}>()

const el = ref(null)

const abortController = new AbortController()

onBeforeUnmount(() => {
  abortController.abort()
})

const progress = injectProgress()
const { results, min, max, error, url: resultsUrl } = useTimeHistogramValues(props.result, el, progress)
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

function onNoneChange(event: Event) {
  if (abortController.signal.aborted) {
    return
  }

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
  const bigIntRangeMin =
    props.state === null || props.state === NONE ? min.value : bigIntMax(timestampToSeconds((props.state as { gte: string; lte: string }).gte), min.value)
  const bigIntRangeMax =
    props.state === null || props.state === NONE ? max.value : bigIntMax(timestampToSeconds((props.state as { gte: string; lte: string }).lte), max.value)
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
  const rangeStart = props.state === null || props.state === NONE ? secondsToTimestamp(min.value) : (props.state as { gte: string; lte: string }).gte
  const rangeEnd = props.state === null || props.state === NONE ? secondsToTimestamp(max.value) : (props.state as { gte: string; lte: string }).lte
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
  <div class="flex flex-col rounded border bg-white p-4 shadow" :class="{ 'data-reloading': laterLoad }" :data-url="resultsUrl">
    <div class="flex items-baseline gap-x-1">
      <WithDocument :id="result.id">
        <template #default="{ doc, url }">
          <RouterLink
            :to="{ name: 'DocumentGet', params: { id: result.id } }"
            :data-url="url"
            class="link mb-1.5 text-lg leading-none"
            v-html="getName(doc.claims) || '<i>no name</i>'"
          ></RouterLink>
        </template>
        <template #loading="{ url }">
          <div class="inline-block h-2 animate-pulse rounded bg-slate-200" :data-url="url" :class="[loadingWidth(result.id)]"></div>
        </template>
      </WithDocument>
      ({{ result.count }})
    </div>
    <ul ref="el">
      <li v-if="error">
        <i class="text-error-600">loading data failed</i>
      </li>
      <li v-else-if="min === null || max === null" class="animate-pulse">
        <div class="my-1.5 grid grid-cols-10 items-end gap-x-1" :style="`aspect-ratio: ${chartWidth - 1} / ${chartHeight}`">
          <div v-for="(h, i) in loadingShortHeights(result.id, 10)" :key="i" class="w-auto rounded bg-slate-200" :class="h"></div>
        </div>
        <div class="flex flex-row justify-between gap-x-1">
          <div class="my-1.5 h-2 w-8 rounded bg-slate-200"></div>
          <div class="my-1.5 h-2 w-8 rounded bg-slate-200"></div>
        </div>
        <div class="my-1.5 h-2 rounded bg-slate-200"></div>
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
        <input
          :id="'time/' + result.id + '/none'"
          :disabled="updateProgress > 0"
          :checked="state === NONE"
          :class="
            updateProgress > 0 ? 'cursor-not-allowed bg-gray-100 text-primary-300 focus:ring-primary-300' : 'cursor-pointer text-primary-600 focus:ring-primary-500'
          "
          type="checkbox"
          class="my-1 self-center rounded"
          @change="onNoneChange($event)"
        />
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
