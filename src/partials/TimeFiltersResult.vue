<script setup lang="ts">
import type { API } from "nouislider"
import type { DeepReadonly } from "vue"

import type { FilterUpdate, SearchSession, SpecialsFilter, SpecialsFilterEntry, TimeFilterEntry, TimeSearchResult } from "@/types"

import noUiSlider from "nouislider"
import { computed, onBeforeUnmount, toRef, useId, useTemplateRef, watchEffect } from "vue"
import { useI18n } from "vue-i18n"

import Button from "@/components/Button.vue"
import CheckBox from "@/components/CheckBox.vue"
import FilterPropLabel from "@/partials/FilterPropLabel.vue"
import TimeDisplay from "@/partials/TimeDisplay.vue"
import TimeRange from "@/partials/TimeRange.vue"
import { useLocked, useProgress } from "@/progress"
import { useTimeHistogramValues } from "@/search"
import { equals, loadingShortHeights, timePrecisionForValue, timeRangeDisplay, timeStringFromFloat64, useInitialLoad, useReportFilterVisibility } from "@/utils"

const props = defineProps<{
  searchSession: DeepReadonly<SearchSession>
  result: TimeSearchResult
  filter?: TimeFilterEntry
  specials?: SpecialsFilterEntry
}>()

const locked = useLocked()

const emit = defineEmits<{
  filterUpdates: [updates: FilterUpdate[]]
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
  none: noneCount,
  unknown: unknownCount,
  hasProperty: hasPropertyCount,
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

// This facet has no hidden-by-query state, so it stays visible whenever rendered. Report that to the filter pane
// so its no-match message appears only when no facet is visible.
useReportFilterVisibility(() => true)

function clearFilter() {
  if (abortController.signal.aborted || (!props.filter && !props.specials)) {
    return
  }
  const updates: FilterUpdate[] = []
  if (props.filter) {
    updates.push({
      filterId: props.filter.id,
      filter: { id: props.filter.id, base: props.filter.base, prop: props.filter.prop, time: {} },
    })
  }
  if (props.specials) {
    updates.push({
      filterId: props.specials.id,
      filter: { id: props.specials.id, base: props.specials.base, prop: props.specials.prop, specials: {} },
    })
  }
  emit("filterUpdates", updates)
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
    emit("filterUpdates", [{ filterId: updatedFilter.id, filter: updatedFilter }])
  }
}

// setSpecial toggles one special selection in the path's specials filter, emitting the updated entry.
// Clearing a special which is not set (and with no specials entry present) is a no-op.
function setSpecial(key: keyof SpecialsFilter, value: boolean) {
  if (abortController.signal.aborted || (!props.specials && !value)) {
    return
  }

  const specials: SpecialsFilter = { ...props.specials?.specials }
  if (value) {
    specials[key] = true
  } else {
    delete specials[key]
  }
  const updatedSpecials: SpecialsFilterEntry = {
    id: props.specials?.id ?? "",
    base: props.specials?.base ?? [],
    prop: props.specials?.prop ?? [...props.result.props],
    specials,
  }
  if (!equals(props.specials, updatedSpecials)) {
    emit("filterUpdates", [{ filterId: updatedSpecials.id, filter: updatedSpecials }])
  }
}

const missingState = computed({
  get(): boolean {
    return props.specials?.specials.missing === true
  },
  set(value: boolean) {
    setSpecial("missing", value)
  },
})

const noneState = computed({
  get(): boolean {
    return props.specials?.specials.none === true
  },
  set(value: boolean) {
    setSpecial("none", value)
  },
})

const unknownState = computed({
  get(): boolean {
    return props.specials?.specials.unknown === true
  },
  set(value: boolean) {
    setSpecial("unknown", value)
  },
})

const hasPropertyState = computed({
  get(): boolean {
    return props.specials?.specials.hasProperty === true
  },
  set(value: boolean) {
    setSpecial("hasProperty", value)
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
      emit("filterUpdates", [{ filterId: updatedFilter.id, filter: updatedFilter }])
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
      emit("filterUpdates", [{ filterId: updatedFilter.id, filter: updatedFilter }])
    }
  },
})

// rangeState reports whether a range (or single value) is selected, and clearing it removes the range. It backs
// the fallback row shown when the property has no documents to histogram: the active filter excludes its own
// range, so an empty histogram means the selected range matches nothing, and the row keeps the selection visible
// at count 0 (like the augmented reference values) so the user sees it and can uncheck it, with no slider to draw.
const rangeState = computed({
  get(): boolean {
    return props.filter?.time?.gte != null
  },
  set(value: boolean) {
    if (abortController.signal.aborted || value) {
      return
    }

    const updatedFilter: TimeFilterEntry = {
      id: props.filter?.id ?? "",
      base: props.filter?.base ?? [],
      prop: props.filter?.prop ?? [...props.result.props],
      time: {},
    }
    if (!equals(props.filter, updatedFilter)) {
      emit("filterUpdates", [{ filterId: updatedFilter.id, filter: updatedFilter }])
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
  return timeRangeDisplay(from.value, to.value)
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
// Whether the slider currently draws the connected segment between the handles. It follows whether a range
// is actively selected; we track it so the connect option is only updated (which rebuilds the connect
// element) when it actually changes.
let sliderConnected = false
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
    sliderConnected = false
  }
  // When sliderEl exists we know that from and to is set as well, and that from != to.
  // Still, we check it here to satisfy type checking.
  if (from.value === null || to.value === null || from.value === to.value) {
    return
  }
  const gte = props.filter?.time?.gte ?? null
  const lte = props.filter?.time?.lte ?? null
  // The connected segment between the handles is drawn only while a range is actively selected (a lower bound
  // is set). Until then the slider is a plain track.
  const connected = gte != null
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
      connect: [false, connected, false],
      // Range is divided by this number to get the keyboard step.
      keyboardDefaultStep: results.value.length,
      keyboardPageMultiplier: 10,
      animate: false,
      behaviour: "snap",
      // Tooltips are shown only while a handle is being dragged, see the noUi-tooltip rules in theme.css.
      tooltips: [tooltipFormat, tooltipFormat],
      ariaFormat: tooltipFormat,
    })
    sliderConnected = connected
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
        // Updating connect rebuilds the connect element, so only pass it when it actually changed.
        ...(connected !== sliderConnected ? { connect: [false, connected, false] } : {}),
        // TODO: Uncomment when supported. See: https://github.com/leongersen/noUiSlider/issues/1226
        // keyboardDefaultStep: results.value.length,
      },
      true,
    )
    sliderConnected = connected
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
        v-if="filter || specials"
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
    <ul ref="el" role="group" :aria-labelledby="labelId" class="grid grid-cols-[max-content_auto] gap-x-1">
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
          <!-- v-if here is just to satisfy typing, results.length === 1 already checked that. -->
          <label v-if="singleValueDisplay" :for="'time/' + result.props.join('/') + '/value'" :class="locked ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'">
            <TimeDisplay :timestamp="singleValueDisplay.timestamp" :precision="singleValueDisplay.precision" />
          </label>
          <label :for="'time/' + result.props.join('/') + '/value'" :class="locked ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
            >({{ results[0].count }})</label
          >
        </div>
      </li>
      <li v-else-if="from !== to" class="col-span-2 mb-3">
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
        When the property has no documents to histogram, a selected range or single value cannot be drawn on the
        slider, so it is shown here at count 0 (like the augmented reference values): it stays visible so the user
        sees the selection and can uncheck it to clear the range.
      -->
      <li v-if="total === 0 && rangeState" class="contents">
        <CheckBox :id="'time/' + result.props.join('/') + '/range'" v-model="rangeState" />
        <div class="flex items-baseline gap-x-1">
          <label :for="'time/' + result.props.join('/') + '/range'" :class="locked ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'">
            <!-- v-if here is just to satisfy typing, rangeState already checked that. -->
            <TimeRange v-if="filter?.time?.gte != null" :from="filter.time.gte" :to="filter.time.lte" />
          </label>
          <label :for="'time/' + result.props.join('/') + '/range'" :class="locked ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'">(0)</label>
        </div>
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
      <li v-if="(hasPropertyCount != null && hasPropertyCount > 0) || hasPropertyState" class="contents">
        <CheckBox :id="'time/' + result.props.join('/') + '/hasProperty'" v-model="hasPropertyState" />
        <div class="flex items-baseline gap-x-1">
          <label :for="'time/' + result.props.join('/') + '/hasProperty'" :class="locked ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
            ><i>{{ t("common.values.hasProperty") }}</i></label
          >
          <label :for="'time/' + result.props.join('/') + '/hasProperty'" :class="locked ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
            >({{ hasPropertyCount ?? 0 }})</label
          >
        </div>
      </li>
      <li v-if="(unknownCount != null && unknownCount > 0) || unknownState" class="contents">
        <CheckBox :id="'time/' + result.props.join('/') + '/unknown'" v-model="unknownState" />
        <div class="flex items-baseline gap-x-1">
          <label :for="'time/' + result.props.join('/') + '/unknown'" :class="locked ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
            ><i>{{ t("common.values.unknown") }}</i></label
          >
          <label :for="'time/' + result.props.join('/') + '/unknown'" :class="locked ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
            >({{ unknownCount ?? 0 }})</label
          >
        </div>
      </li>
      <li v-if="(noneCount != null && noneCount > 0) || noneState" class="contents">
        <CheckBox :id="'time/' + result.props.join('/') + '/none'" v-model="noneState" />
        <div class="flex items-baseline gap-x-1">
          <label :for="'time/' + result.props.join('/') + '/none'" :class="locked ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
            ><i>{{ t("common.values.none") }}</i></label
          >
          <label :for="'time/' + result.props.join('/') + '/none'" :class="locked ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'">({{ noneCount ?? 0 }})</label>
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
