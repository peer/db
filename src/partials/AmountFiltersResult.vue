<script setup lang="ts">
import type { API } from "nouislider"
import type { AmountFilterState, AmountSearchResult } from "@/types"
import type { PeerDBDocument } from "@/document"

import { ref, computed, toRef, watchEffect, onBeforeUnmount } from "vue"
import noUiSlider from "nouislider"
import WithDocument from "@/components/WithDocument.vue"
import CheckBox from "@/components/CheckBox.vue"
import { useAmountHistogramValues, NONE } from "@/search"
import { formatValue, equals, getName, loadingWidth, useInitialLoad, loadingShortHeights } from "@/utils"
import { injectProgress } from "@/progress"

const props = defineProps<{
  s: string
  searchTotal: number
  result: AmountSearchResult
  state: AmountFilterState
  updateProgress: number
}>()

const emit = defineEmits<{
  (e: "update:state", state: AmountFilterState): void
}>()

const el = ref(null)

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
} = useAmountHistogramValues(
  toRef(() => props.s),
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
    gte: unencoded[0],
    lte: unencoded[1],
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
  const gte = props.state === null || props.state === NONE ? null : (props.state as { gte?: number; lte?: number }).gte
  const lte = props.state === null || props.state === NONE ? null : (props.state as { gte?: number; lte?: number }).lte
  const rangeMin = gte == null ? min.value : Math.max(gte, min.value)
  const rangeMax = lte == null ? max.value : Math.min(lte, max.value)
  const rangeStart = gte == null ? min.value : gte
  const rangeEnd = lte == null ? max.value : lte
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
          return formatValue(value, props.result.unit)
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

const WithPeerDBDocument = WithDocument<PeerDBDocument>
</script>

<template>
  <div class="flex flex-col rounded border bg-white p-4 shadow" :class="{ 'data-reloading': laterLoad }" :data-url="resultsUrl">
    <div class="flex items-baseline gap-x-1">
      <WithPeerDBDocument :id="result.id" name="DocumentGet">
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
      </WithPeerDBDocument>
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
            {{ formatValue(min, result.unit) }}
          </div>
          <div>
            {{ formatValue(max, result.unit) }}
          </div>
        </div>
        <div ref="sliderEl"></div>
      </li>
      <li v-else-if="results.length === 1" class="flex items-baseline gap-x-1">
        <div class="my-1 inline-block h-4 w-4 shrink-0 self-center border border-transparent"></div>
        <div class="my-1 leading-none">{{ formatValue(results[0].min, result.unit) }}</div>
        <div class="my-1 leading-none">({{ results[0].count }})</div>
      </li>
      <li v-if="result.count < searchTotal" class="flex items-baseline gap-x-1 first:mt-0" :class="error ? 'mt-0' : min === null || max === null ? 'mt-3' : 'mt-4'">
        <CheckBox :id="'amount/' + result.id + '/' + result.unit + '/none'" v-model="noneState" :progress="updateProgress" class="my-1 self-center" />
        <label
          :for="'amount/' + result.id + '/' + result.unit + '/none'"
          class="my-1 leading-none"
          :class="updateProgress > 0 ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
          ><i>none</i></label
        >
        <label
          :for="'amount/' + result.id + '/' + result.unit + '/none'"
          class="my-1 leading-none"
          :class="updateProgress > 0 ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
          >({{ searchTotal - result.count }})</label
        >
      </li>
    </ul>
  </div>
</template>
