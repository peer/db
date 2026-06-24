<script setup lang="ts">
import type { API } from "nouislider"
import type { DeepReadonly } from "vue"

import type { SearchSession, TimeFilterEntry, TimeSearchResult } from "@/types"

import noUiSlider from "nouislider"
import { computed, onBeforeUnmount, toRef, useId, useTemplateRef, watchEffect } from "vue"
import { useI18n } from "vue-i18n"

import Button from "@/components/Button.vue"
import CheckBox from "@/components/CheckBox.vue"
import FilterPropLabel from "@/partials/FilterPropLabel.vue"
import TimeDisplay from "@/partials/TimeDisplay.vue"
import { useLocked, useProgress } from "@/progress"
import { useTimeHistogramValues } from "@/search"
import { equals, loadingShortHeights, timePrecisionForRange, timePrecisionForValue, timeStringFromFloat64, useInitialLoad } from "@/utils"

const props = defineProps<{
  searchSession: DeepReadonly<SearchSession>
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
  total,
  missing: missingCount,
  from,
  to,
  error,
  url: resultsUrl,
} = useTimeHistogramValues(
  toRef(() => props.searchSession),
  filterId,
  computed(() => props.result.props),
  el,
  progress,
)
const { laterLoad } = useInitialLoad(progress)

function clearFilter() {
  if (abortController.signal.aborted || !props.filter) {
    return
  }
  emit("filterUpdate", props.filter.id, {
    id: props.filter.id,
    base: props.filter.base,
    prop: props.filter.prop,
    time: {},
  })
}

function onSliderChange(values: (number | string)[], handle: number, unencoded: number[], tap: boolean, positions: number[], noUiSlider: API) {
  if (abortController.signal.aborted) {
    return
  }

  const updatedFilter: TimeFilterEntry = {
    id: props.filter?.id ?? "",
    base: props.filter?.base ?? [],
    prop: props.filter?.prop ?? [...props.result.props],
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
      prop: props.filter?.prop ?? [...props.result.props],
      time: value ? { missing: true } : {},
    }
    if (!equals(props.filter, updatedFilter)) {
      emit("filterUpdate", updatedFilter.id, updatedFilter)
    }
  },
})

// Selects documents which have the property, with any value. This is the only selection
// which matches documents whose claims have no known endpoint values at all, so the exists
// row is offered when the histogram has nothing to span (and kept visible whenever the
// filter is active so it can be unchecked).
const existsState = computed({
  get(): boolean {
    return props.filter?.time?.exists === true
  },
  set(value: boolean) {
    if (abortController.signal.aborted) {
      return
    }

    const updatedFilter: TimeFilterEntry = {
      id: props.filter?.id ?? "",
      base: props.filter?.base ?? [],
      prop: props.filter?.prop ?? [...props.result.props],
      time: value ? { exists: true } : {},
    }
    if (!equals(props.filter, updatedFilter)) {
      emit("filterUpdate", updatedFilter.id, updatedFilter)
    }
  },
})

// Selects the single known value using the bounds provided by the backend. The from bound
// can be below the value itself when needed to match claims ending exclusively at it.
const singleValueState = computed({
  get(): boolean {
    return props.filter?.time?.gte != null && props.filter.time.gte === from.value && props.filter.time.lte === to.value
  },
  set(value: boolean) {
    if (abortController.signal.aborted) {
      return
    }

    const updatedFilter: TimeFilterEntry = {
      id: props.filter?.id ?? "",
      base: props.filter?.base ?? [],
      prop: props.filter?.prop ?? [...props.result.props],
      time: value && from.value !== null && to.value !== null ? { gte: from.value, lte: to.value } : {},
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

// When the histogram collapses to a single bucket, the bucket's from is the
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

const tooltipFormat = {
  to: (value: number): string => {
    const precision = rangeDisplay.value?.precision ?? timePrecisionForValue(value)
    return timeStringFromFloat64(value, precision)
  },
}

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
  // The backend sends from/to as the slider track bounds (the full data range, or the current
  // selection widened by a margin and clamped to the data) and gte/lte as the handle positions,
  // so the handles can be dragged outward into the widened track to expand the selection.
  const rangeMin = from.value
  const rangeMax = to.value
  const rangeStart = gte == null ? from.value : Math.max(from.value, Math.min(gte, to.value))
  const rangeEnd = lte == null ? to.value : Math.max(from.value, Math.min(lte, to.value))
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
      // Tooltips are shown only while a handle is being dragged, see the noUi-tooltip rules in theme.css.
      tooltips: [tooltipFormat, tooltipFormat],
      ariaFormat: tooltipFormat,
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
    <div :id="labelId">
      <Button
        v-if="filter"
        type="button"
        class="float-right ml-2 px-2.5 py-1"
        :title="t('partials.TimeFiltersResult.clearFilter')"
        :aria-label="t('partials.TimeFiltersResult.clearFilter')"
        @click.prevent="clearFilter"
        >{{ t("common.buttons.clear") }}</Button
      >
      <span class="mb-1.5 text-lg leading-none"><FilterPropLabel :prop-ids="result.props" /></span>
      ({{ result.count }})
    </div>
    <ul ref="el" role="group" :aria-labelledby="labelId" class="grid grid-cols-[max-content_auto] gap-x-1 gap-y-3">
      <li v-if="error" class="col-span-2">
        <i class="pd-timefiltersresult-error text-error-600">{{ t("common.status.loadingDataFailed") }}</i>
      </li>
      <li v-else-if="total === null" class="col-span-2 motion-safe:animate-pulse" aria-hidden="true">
        <div class="my-1.5 grid grid-cols-10 items-end gap-x-1" :style="`aspect-ratio: ${chartWidth - 1} / ${chartHeight}`">
          <div v-for="(h, i) in loadingShortHeights(result.props.join('/'), 10)" :key="i" class="w-auto rounded-sm bg-slate-200" :class="h"></div>
        </div>
        <div class="flex flex-row justify-between gap-x-1">
          <div class="my-1.5 h-2 w-8 rounded-sm bg-slate-200"></div>
          <div class="my-1.5 h-2 w-8 rounded-sm bg-slate-200"></div>
        </div>
        <div class="my-1.5 h-2 rounded-sm bg-slate-200"></div>
      </li>
      <li v-else-if="results.length === 1" class="contents">
        <CheckBox :id="'time/' + result.props.join('/') + '/value'" v-model="singleValueState" />
        <div class="flex items-baseline gap-x-1">
          <label v-if="singleValueDisplay" :for="'time/' + result.props.join('/') + '/value'" :class="locked ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'">
            <TimeDisplay :timestamp="singleValueDisplay.timestamp" :precision="singleValueDisplay.precision" :toggle="false" />
          </label>
          <label :for="'time/' + result.props.join('/') + '/value'" :class="locked ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
            >({{ results[0].count }})</label
          >
        </div>
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
      <!--
        The exists row is the only selection which matches documents whose claims have no
        known endpoint values, so it is offered when a loaded response has no histogram to
        show (total is 0) while documents with the property exist, and it stays visible
        whenever the exists filter is active so it can be unchecked.
      -->
      <li v-if="(total === 0 && result.count > 0) || existsState" class="contents">
        <CheckBox :id="'time/' + result.props.join('/') + '/exists'" v-model="existsState" />
        <div class="flex items-baseline gap-x-1">
          <label :for="'time/' + result.props.join('/') + '/exists'" :class="locked ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
            ><i>{{ t("common.values.exists") }}</i></label
          >
          <label :for="'time/' + result.props.join('/') + '/exists'" :class="locked ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'">({{ result.count }})</label>
        </div>
      </li>
      <li v-if="(missingCount != null && missingCount > 0) || missingState" class="contents">
        <CheckBox :id="'time/' + result.props.join('/') + '/missing'" v-model="missingState" />
        <div class="flex items-baseline gap-x-1">
          <label :for="'time/' + result.props.join('/') + '/missing'" :class="locked ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
            ><i>{{ t("common.values.missing") }}</i></label
          >
          <label :for="'time/' + result.props.join('/') + '/missing'" :class="locked ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
            >({{ missingCount ?? 0 }})</label
          >
        </div>
      </li>
    </ul>
  </div>
</template>
