<script setup lang="ts">
import type { API } from "nouislider"
import type { DeepReadonly } from "vue"

import type { AmountFilterEntry, AmountSearchResult, SearchSession } from "@/types"

import noUiSlider from "nouislider"
import { computed, onBeforeUnmount, toRef, useId, useTemplateRef, watchEffect } from "vue"
import { useI18n } from "vue-i18n"

import Button from "@/components/Button.vue"
import CheckBox from "@/components/CheckBox.vue"
import DocumentRefInline from "@/partials/DocumentRefInline.vue"
import FilterPropLabel from "@/partials/FilterPropLabel.vue"
import { useLocked, useProgress } from "@/progress"
import { useAmountHistogramValues } from "@/search"
import { equals, loadingShortHeights, useInitialLoad } from "@/utils"

const props = defineProps<{
  searchSession: DeepReadonly<SearchSession>
  result: AmountSearchResult
  filter?: AmountFilterEntry
}>()

const locked = useLocked()

const emit = defineEmits<{
  filterUpdate: [filterId: string, filter: AmountFilterEntry]
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
} = useAmountHistogramValues(
  toRef(() => props.searchSession),
  filterId,
  computed(() => props.result.props),
  computed(() => props.result.unit),
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
    amount: { unit: props.result.unit },
  })
}

function onSliderChange(values: (number | string)[], handle: number, unencoded: number[], tap: boolean, positions: number[], noUiSlider: API) {
  if (abortController.signal.aborted) {
    return
  }

  const updatedFilter: AmountFilterEntry = {
    id: props.filter?.id ?? "",
    base: props.filter?.base ?? [],
    prop: props.filter?.prop ?? [...props.result.props],
    amount: {
      unit: props.result.unit,
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
    return props.filter?.amount?.missing === true
  },
  set(value: boolean) {
    if (abortController.signal.aborted) {
      return
    }

    const updatedFilter: AmountFilterEntry = {
      id: props.filter?.id ?? "",
      base: props.filter?.base ?? [],
      prop: props.filter?.prop ?? [...props.result.props],
      amount: value ? { unit: props.result.unit, missing: true } : { unit: props.result.unit },
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
    return props.filter?.amount?.exists === true
  },
  set(value: boolean) {
    if (abortController.signal.aborted) {
      return
    }

    const updatedFilter: AmountFilterEntry = {
      id: props.filter?.id ?? "",
      base: props.filter?.base ?? [],
      prop: props.filter?.prop ?? [...props.result.props],
      amount: value ? { unit: props.result.unit, exists: true } : { unit: props.result.unit },
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
    return props.filter?.amount?.gte != null && props.filter.amount.gte === from.value && props.filter.amount.lte === to.value
  },
  set(value: boolean) {
    if (abortController.signal.aborted) {
      return
    }

    const updatedFilter: AmountFilterEntry = {
      id: props.filter?.id ?? "",
      base: props.filter?.base ?? [],
      prop: props.filter?.prop ?? [...props.result.props],
      amount: value && from.value !== null && to.value !== null ? { unit: props.result.unit, gte: from.value, lte: to.value } : { unit: props.result.unit },
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

let slider: API | null = null
const sliderEl = useTemplateRef<HTMLElement>("sliderEl")

const valueFormat = {
  to: (value: number): string => parseFloat(value.toFixed(5)).toString(),
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
  const gte = props.filter?.amount?.gte ?? null
  const lte = props.filter?.amount?.lte ?? null
  const isMissing = props.filter?.amount?.missing === true
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
      // Tooltips are shown only while a handle is being dragged, see the noUi-tooltip rules in theme.css.
      tooltips: [valueFormat, valueFormat],
      ariaFormat: valueFormat,
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
  <div class="pd-amountfiltersresult flex flex-col" :class="{ 'data-reloading': laterLoad }" :data-url="resultsUrl">
    <div :id="labelId">
      <Button
        v-if="filter"
        type="button"
        class="float-right ml-2 px-2.5 py-1"
        :title="t('partials.AmountFiltersResult.clearFilter')"
        :aria-label="t('partials.AmountFiltersResult.clearFilter')"
        @click.prevent="clearFilter"
        >{{ t("common.buttons.clear") }}</Button
      >
      <i18n-t v-if="result.unit" keypath="common.labelWithUnit" scope="global" tag="span" class="mb-1.5 text-lg leading-none">
        <template #label><FilterPropLabel :prop-ids="result.props" /></template>
        <template #unit>
          <DocumentRefInline :id="result.unit" :link="false" />
        </template>
      </i18n-t>
      <span v-else class="mb-1.5 text-lg leading-none"><FilterPropLabel :prop-ids="result.props" /></span>
      ({{ result.count }})
    </div>
    <ul ref="el" role="group" :aria-labelledby="labelId" class="grid grid-cols-[max-content_auto] gap-x-1 gap-y-3">
      <li v-if="error" class="col-span-2">
        <i class="pd-amountfiltersresult-error text-error-600">{{ t("common.status.loadingDataFailed") }}</i>
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
        <CheckBox :id="'amount/' + result.props.join('/') + '/' + (result.unit ?? '') + '/value'" v-model="singleValueState" />
        <div class="flex items-baseline gap-x-1">
          <label
            :for="'amount/' + result.props.join('/') + '/' + (result.unit ?? '') + '/value'"
            :class="locked ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
            >{{ results[0].from }}</label
          >
          <label :for="'amount/' + result.props.join('/') + '/' + (result.unit ?? '') + '/value'" :class="locked ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
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
        <div class="flex flex-row justify-between gap-x-1">
          <div>
            {{ from }}
          </div>
          <div>
            {{ to }}
          </div>
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
        <CheckBox :id="'amount/' + result.props.join('/') + '/' + (result.unit ?? '') + '/exists'" v-model="existsState" />
        <div class="flex items-baseline gap-x-1">
          <label :for="'amount/' + result.props.join('/') + '/' + (result.unit ?? '') + '/exists'" :class="locked ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
            ><i>{{ t("common.values.exists") }}</i></label
          >
          <label :for="'amount/' + result.props.join('/') + '/' + (result.unit ?? '') + '/exists'" :class="locked ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
            >({{ result.count }})</label
          >
        </div>
      </li>
      <li v-if="(missingCount != null && missingCount > 0) || missingState" class="contents">
        <CheckBox :id="'amount/' + result.props.join('/') + '/' + (result.unit ?? '') + '/missing'" v-model="missingState" />
        <div class="flex items-baseline gap-x-1">
          <label
            :for="'amount/' + result.props.join('/') + '/' + (result.unit ?? '') + '/missing'"
            :class="locked ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
            ><i>{{ t("common.values.missing") }}</i></label
          >
          <label
            :for="'amount/' + result.props.join('/') + '/' + (result.unit ?? '') + '/missing'"
            :class="locked ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
            >({{ missingCount ?? 0 }})</label
          >
        </div>
      </li>
    </ul>
  </div>
</template>
