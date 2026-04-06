<script setup lang="ts">
import type { API } from "nouislider"
import type { DeepReadonly } from "vue"

import type { SearchSession, TimeFilterEntry, TimeSearchResult } from "@/types"

import noUiSlider from "nouislider"
import { computed, onBeforeUnmount, toRef, useId, useTemplateRef, watchEffect } from "vue"
import { useI18n } from "vue-i18n"

import CheckBox from "@/components/CheckBox.vue"
import DocumentRefInline from "@/partials/DocumentRefInline.vue"
import TimeDisplay from "@/partials/TimeDisplay.vue"
import { useLocked, useProgress } from "@/progress"
import { useTimeHistogramValues } from "@/search"
import { equals, loadingShortHeights, timePrecisionForRange, timePrecisionForValue, timeStringFromFloat64, useInitialLoad } from "@/utils"

const props = defineProps<{
  searchSession: DeepReadonly<SearchSession>
  searchTotal: number
  result: TimeSearchResult
  filter?: TimeFilterEntry
}>()

const locked = useLocked()

const emit = defineEmits<{
  filterUpdate: [filterId: string, filter: TimeFilterEntry]
}>()

const { t } = useI18n({ useScope: "global" })

const el = useTemplateRef<HTMLElement>("el")

const labelId = useId()

const abortController = new AbortController()

onBeforeUnmount(() => {
  abortController.abort()
})

// Data loading only, no controls.
const progress = useProgress()

// The filter ID from the session's filter, if it exists.
const filterId = computed(() => props.filter?.id ?? "")

const {
  results,
  missing: missingCount,
  from,
  to,
  error,
  url: resultsUrl,
} = useTimeHistogramValues(
  toRef(() => props.searchSession),
  filterId,
  computed(() => props.result.propId),
  el,
  progress,
)
const { laterLoad } = useInitialLoad(progress)

function onSliderChange(values: (number | string)[], handle: number, unencoded: number[], tap: boolean, positions: number[], noUiSlider: API) {
  if (abortController.signal.aborted) {
    return
  }

  const updatedFilter: TimeFilterEntry = {
    id: props.filter?.id ?? "",
    base: props.filter?.base ?? [],
    prop: props.filter?.prop ?? [props.result.propId],
    time: {
      gte: unencoded[0],
      lte: unencoded[1],
    },
  }
  if (!equals(props.filter, updatedFilter)) {
    emit("filterUpdate", updatedFilter.id, updatedFilter)
  }
}

const missingState = computed({
  get(): boolean {
    return props.filter?.time?.missing === true
  },
  set(value: boolean) {
    if (abortController.signal.aborted) {
      return
    }

    const updatedFilter: TimeFilterEntry = {
      id: props.filter?.id ?? "",
      base: props.filter?.base ?? [],
      prop: props.filter?.prop ?? [props.result.propId],
      time: value ? { missing: true } : {},
    }
    if (!equals(props.filter, updatedFilter)) {
      emit("filterUpdate", updatedFilter.id, updatedFilter)
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

// Pick a single display precision for both edges so they line up visually,
// and render each edge as a Time-claim string at that precision.
const rangeDisplay = computed(() => {
  if (from.value === null || to.value === null) {
    return null
  }
  let f = Math.floor(from.value)
  let t = Math.ceil(to.value)
  if (f === t) {
    f -= 0.5
    t += 0.5
  }
  const precision = timePrecisionForRange(f, t)
  return {
    precision,
    from: timeStringFromFloat64(f, precision),
    to: timeStringFromFloat64(t, precision),
  }
})

// When the histogram collapses to a single bucket, the bucket's `from` is the
// claim value itself. Infer its precision from divisibility / calendar fields.
const singleValueDisplay = computed(() => {
  if (results.value.length !== 1) {
    return null
  }
  const v = results.value[0].from
  const precision = timePrecisionForValue(v)
  return {
    precision,
    timestamp: timeStringFromFloat64(v, precision),
  }
})

let slider: API | null = null
const sliderEl = useTemplateRef<HTMLElement>("sliderEl")

watchEffect(() => {
  if (slider && slider.target != sliderEl.value) {
    slider.destroy()
    slider = null
  }
  // When sliderEl exists we know that from and to is set as well, and that from != to.
  // Still, we check it here to satisfy type checking.
  if (from.value === null || to.value === null || from.value === to.value) {
    return
  }
  const gte = props.filter?.time?.gte ?? null
  const lte = props.filter?.time?.lte ?? null
  const isMissing = props.filter?.time?.missing === true
  const rangeMin = gte == null ? from.value : Math.max(gte, from.value)
  const rangeMax = lte == null ? to.value : Math.min(lte, to.value)
  const rangeStart = gte == null ? from.value : gte
  const rangeEnd = lte == null ? to.value : lte
  if (!slider && sliderEl.value) {
    slider = noUiSlider.create(sliderEl.value, {
      start: [rangeStart, rangeEnd],
      range: {
        min: [rangeMin],
        max: [rangeMax],
      },
      margin: (rangeMax - rangeMin) / results.value.length,
      connect: [false, !isMissing, false],
      // Range is divided by this number to get the keyboard step.
      keyboardDefaultStep: results.value.length,
      keyboardPageMultiplier: 10,
      animate: false,
      behaviour: "snap",
      ariaFormat: {
        to: (value: number): string => {
          return new Date(value * 1000).toISOString()
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
    // Update connect to reflect whether missing is active (no range highlight) or not.
    slider.updateOptions({ connect: [false, !isMissing, false] }, false)
  }
})

watchEffect(() => {
  if (!slider) {
    return
  }

  if (locked.value) {
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
  <div class="pd-timefiltersresult flex flex-col" :class="{ 'data-reloading': laterLoad }" :data-url="resultsUrl">
    <div :id="labelId" class="flex items-baseline gap-x-1">
      <DocumentRefInline :id="result.propId" class="mb-1.5 text-lg leading-none" />
      ({{ result.count }})
    </div>
    <ul ref="el" role="group" :aria-labelledby="labelId" class="grid grid-cols-[max-content_auto] gap-x-1 gap-y-3">
      <li v-if="error" class="col-span-2">
        <i class="pd-timefiltersresult-error text-error-600">{{ t("common.status.loadingDataFailed") }}</i>
      </li>
      <li v-else-if="from === null || to === null" class="col-span-2 motion-safe:animate-pulse" aria-hidden="true">
        <div class="my-1.5 grid grid-cols-10 items-end gap-x-1" :style="`aspect-ratio: ${chartWidth - 1} / ${chartHeight}`">
          <div v-for="(h, i) in loadingShortHeights(result.propId, 10)" :key="i" class="w-auto rounded-sm bg-slate-200" :class="h"></div>
        </div>
        <div class="flex flex-row justify-between gap-x-1">
          <div class="my-1.5 h-2 w-8 rounded-sm bg-slate-200"></div>
          <div class="my-1.5 h-2 w-8 rounded-sm bg-slate-200"></div>
        </div>
        <div class="my-1.5 h-2 rounded-sm bg-slate-200"></div>
      </li>
      <li v-else-if="from !== to" class="col-span-2">
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
        <div v-if="rangeDisplay" class="flex flex-row justify-between gap-x-1">
          <TimeDisplay :timestamp="rangeDisplay.from" :precision="rangeDisplay.precision" />
          <TimeDisplay :timestamp="rangeDisplay.to" :precision="rangeDisplay.precision" />
        </div>
        <div ref="sliderEl"></div>
      </li>
      <li v-else-if="results.length === 1" class="contents">
        <div class="h-4 w-4 shrink-0 border border-transparent"></div>
        <div class="flex items-baseline gap-x-1">
          <TimeDisplay v-if="singleValueDisplay" :timestamp="singleValueDisplay.timestamp" :precision="singleValueDisplay.precision" />
          <div>({{ results[0].count }})</div>
        </div>
      </li>
      <li v-if="(missingCount != null && missingCount > 0) || missingState" class="contents">
        <CheckBox :id="'time/' + result.propId + '/missing'" v-model="missingState" />
        <div class="flex items-baseline gap-x-1">
          <label :for="'time/' + result.propId + '/missing'" :class="locked ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
            ><i>{{ t("common.values.missing") }}</i></label
          >
          <label :for="'time/' + result.propId + '/missing'" :class="locked ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'">({{ missingCount ?? 0 }})</label>
        </div>
      </li>
    </ul>
  </div>
</template>
