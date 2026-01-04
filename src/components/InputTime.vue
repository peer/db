<script setup lang="ts">
import type { TimePrecision } from "@/types"

import { Listbox, ListboxButton, ListboxLabel, ListboxOption, ListboxOptions } from "@headlessui/vue"
import { CheckIcon, ChevronUpDownIcon } from "@heroicons/vue/20/solid"
import { debounce } from "lodash-es"
import { computed, nextTick, onBeforeMount, onBeforeUnmount, onMounted, ref, useId, watch } from "vue"

import InputText from "@/components/InputText.vue"
import { daysIn } from "@/time.ts"

const DEBOUNCE_MS = 2000

const props = withDefaults(
  defineProps<{
    progress?: number
    readonly?: boolean
    invalid?: boolean
    maxPrecision?: "G" | "100M" | "10M" | "M" | "100k" | "10k" | "k" | "100y" | "10y" | "y"
  }>(),
  {
    progress: 0,
    readonly: false,
    invalid: false,
    maxPrecision: "G",
  },
)

const model = defineModel<string>({ default: "" })
const precision = defineModel<TimePrecision>("precision", { default: "y" })

// We want all fallthrough attributes to be passed to the main input element.
defineOptions({
  inheritAttrs: false,
})

const abortController = new AbortController()

onBeforeUnmount(() => {
  abortController.abort()
})

const YEAR_RE = /^(-?\d+)$/
const MONTH_RE = /^(-?\d+)-(\d{1,2})$/
const DAY_RE = /^(-?\d+)-(\d{1,2})-(\d{1,2})$/
const HOUR_RE = /^(-?\d+)-(\d{1,2})-(\d{1,2})T(\d{1,2})$/
const MINUTE_RE = /^(-?\d+)-(\d{1,2})-(\d{1,2})T(\d{1,2}):(\d{1,2})$/
const SECOND_RE = /^(-?\d+)-(\d{1,2})-(\d{1,2})T(\d{1,2}):(\d{1,2}):(\d{1,2})$/

const YEAR_IN_PROGRESS_REGEX = /^-?\d*$/
const MONTH_IN_PROGRESS_REGEX = /^-?\d+-\d{0,2}$/
const DAY_IN_PROGRESS_REGEX = /^-?\d+-\d{1,2}-\d{0,2}$/
const MINUTES_IN_PROGRESS_REGEX = /^-?\d+-\d{1,2}-\d{1,2}T\d{1,2}$/
const SECONDS_IN_PROGRESS_REGEX = /^-?\d+-\d{1,2}-\d{1,2}T\d{1,2}:\d{1,2}$/

const DATE_TIME_WHITESPACE_TO_T_REGEX = /(-?\d+)\s*-\s*(\d{1,2})\s*-\s*(\d{1,2})\s+([0-9])/g
const LOWERCASE_T_REGEX = /t/g
const ALL_WHITESPACE_REGEX = /\s+/g
const TRAILING_SEPARATORS_REGEX = /[-:T]+$/
const TRAILING_DASH_REGEX = /-$/
const TRAILING_T_REGEX = /T$/

const timePrecisionOptions = ["G", "100M", "10M", "M", "100k", "10k", "k", "100y", "10y", "y", "m", "d", "h", "min", "s"] as const
const precisionLabels: Record<TimePrecision, string> = {
  G: "giga years",
  "100M": "hundred megayears",
  "10M": "ten megayears",
  M: "megayears",
  "100k": "hundred kiloyears",
  "10k": "ten kiloyears",
  k: "kiloyears",
  "100y": "hundred years",
  "10y": "ten years",
  y: "years",
  m: "months",
  d: "days",
  h: "hours",
  min: "minutes",
  s: "seconds",
}

const PRECISION_RANK = new Map<TimePrecision, number>(timePrecisionOptions.map((p, i) => [p, i]))

const timePrecision = ref<TimePrecision>("y")

const isEditing = ref(false)
const errorMessage = ref("")

const isInvalid = computed(() => props.invalid || errorMessage.value !== "")

const inputId = useId()

const timePrecisionWithMax = computed(() => {
  const reversed = timePrecisionOptions.toReversed()
  const maxPrecision = reversed.indexOf(props.maxPrecision)

  if (maxPrecision < 0) return reversed

  return reversed.slice(0, maxPrecision + 1)
})

const displayValue = ref(model.value)

onBeforeMount(() => {
  timePrecision.value = precision.value
  displayValue.value = model.value
})

onMounted(async () => {
  if (!displayValue.value) return

  // We want to validate and emit the canonical value on mount.
  // However, we must not overwrite the text shown in the input.
  // The model->display watcher only syncs when isEditing is false, so we temporarily
  // mark the component as "editing" to block that sync during this initial emit.
  isEditing.value = true

  // Update the precision based on the initial text
  // and emit the canonical model value.
  autoAdaptPrecisionFromDisplay()
  emitCanonicalFromDisplay()

  // Wait one tick so the parent can process the emitted update and push the new model back.
  await nextTick()

  // Re-enable normal model->display syncing for future external updates.
  isEditing.value = false
})

watch(
  () => model.value,
  (v) => {
    // If parent updates model value externally, reflect it unless user is actively editing.
    if (!isEditing.value) displayValue.value = v ?? ""
  },
)

const pad2 = (n: string) => n.padStart(2, "0")

function normalizeForParsing(raw: string): string {
  if (!raw) return ""

  let r = raw

  // Normalize date + time boundary to 'T'
  // Example: "2023-1-1    2:3" → "2023-1-1T2:3"
  r = r.replace(DATE_TIME_WHITESPACE_TO_T_REGEX, "$1-$2-$3T$4")

  // Normalize lowercase 't' to uppercase 'T'
  r = r.replace(LOWERCASE_T_REGEX, "T")

  // Remove all remaining whitespace
  r = r.replace(ALL_WHITESPACE_REGEX, "")

  return r
}

function cleanInputNormalized(raw: string): string {
  let r = raw

  // Remove trailing separators for validation
  r = r.replace(TRAILING_SEPARATORS_REGEX, "")

  return r
}

const matchToYear = (s: string) => s.match(YEAR_RE)
const matchToMonth = (s: string) => s.match(MONTH_RE)
const matchToDay = (s: string) => s.match(DAY_RE)
const matchToHour = (s: string) => s.match(HOUR_RE)
const matchToMinute = (s: string) => s.match(MINUTE_RE)
const matchToSecond = (s: string) => s.match(SECOND_RE)

function precisionLabel(p: TimePrecision): string {
  return precisionLabels[p]
}

function progressiveValidate(normalized: string): string {
  if (!normalized) return ""

  // Year in progress: "202", "2023", "2023-"
  if (YEAR_IN_PROGRESS_REGEX.test(normalized)) return ""

  // Month in progress: "2023-1", "2023-12", "2023-12-"
  if (MONTH_IN_PROGRESS_REGEX.test(normalized)) {
    const m = matchToMonth(normalized.replace(TRAILING_DASH_REGEX, ""))
    if (!m) return ""
    const month = Number(m[2])
    if (month === 0) return ""
    return month >= 1 && month <= 12 ? "" : "Months need to be between 1-12."
  }

  // Day in progress: "2023-1-1", "2023-1-1T", "2023-1-1T1"
  if (DAY_IN_PROGRESS_REGEX.test(normalized)) {
    const asDay = matchToDay(normalized.replace(TRAILING_T_REGEX, ""))
    if (!asDay) return ""
    const year = Number(asDay[1])
    const month = Number(asDay[2])
    const day = Number(asDay[3])

    if (month < 1 || month > 12) return "Months need to be between 1-12."

    if (day === 0) return ""
    const maxDay = daysIn(month, year)
    if (day < 1 || day > maxDay) return `Day must be between 1-${maxDay}.`

    return ""
  }

  const toHour = matchToHour(normalized)
  if (toHour) {
    const year = Number(toHour[1])
    const month = Number(toHour[2])
    const day = Number(toHour[3])
    const hour = Number(toHour[4])

    if (month < 1 || month > 12) return "Months need to be between 1-12."
    const maxDay = daysIn(month, year)
    if (day < 1 || day > maxDay) return `Day must be between 1-${maxDay}.`
    if (hour < 0 || hour > 23) return "Hours needs to be between 0-23."

    return ""
  }

  // Minutes in progress: allow trailing ":" while typing
  if (MINUTES_IN_PROGRESS_REGEX.test(normalized)) return ""
  const toMinute = matchToMinute(normalized)
  if (toMinute) {
    const year = Number(toMinute[1])
    const month = Number(toMinute[2])
    const day = Number(toMinute[3])
    const hour = Number(toMinute[4])
    const minute = Number(toMinute[5])

    if (month < 1 || month > 12) return "Months need to be between 1-12."
    const maxDay = daysIn(month, year)
    if (day < 1 || day > maxDay) return `Day must be between 1-${maxDay}.`
    if (hour < 0 || hour > 23) return "Hours needs to be between 0-23."
    if (minute < 0 || minute > 59) return "Minutes need to be between 0-59."

    return ""
  }

  // Seconds in progress: allow trailing ":" while typing
  if (SECONDS_IN_PROGRESS_REGEX.test(normalized)) return ""
  const toSecond = matchToSecond(normalized)
  if (toSecond) {
    const year = Number(toSecond[1])
    const month = Number(toSecond[2])
    const day = Number(toSecond[3])
    const hour = Number(toSecond[4])
    const minute = Number(toSecond[5])
    const second = Number(toSecond[6])

    if (month < 1 || month > 12) return "Months need to be between 1-12."
    const maxDay = daysIn(month, year)
    if (day < 1 || day > maxDay) return `Day must be between 1-${maxDay}.`
    if (hour < 0 || hour > 23) return "Hours need to be between 0-23."
    if (minute < 0 || minute > 59) return "Minutes need to be between 0-59."
    if (second < 0 || second > 59) return "Seconds need to be between 0-59."

    return ""
  }

  return "Invalid timestamp structure."
}

function getStructuredTimestamp(normalized: string): { y: string; m: string; d: string; h: string; min: string; s: string } {
  const timeStruct = { y: "", m: "", d: "", h: "", min: "", s: "" }
  if (!normalized) return timeStruct

  const toYear = matchToYear(normalized)
  if (toYear) {
    timeStruct.y = toYear[1]
    return timeStruct
  }

  const toMonth = matchToMonth(normalized)
  if (toMonth) {
    timeStruct.y = toMonth[1]
    timeStruct.m = toMonth[2]
    return timeStruct
  }

  const toDay = matchToDay(normalized)
  if (toDay) {
    timeStruct.y = toDay[1]
    timeStruct.m = toDay[2]
    timeStruct.d = toDay[3]
    return timeStruct
  }

  const toHour = matchToHour(normalized)
  if (toHour) {
    timeStruct.y = toHour[1]
    timeStruct.m = toHour[2]
    timeStruct.d = toHour[3]
    timeStruct.h = toHour[4]
    return timeStruct
  }

  const toMinute = matchToMinute(normalized)
  if (toMinute) {
    timeStruct.y = toMinute[1]
    timeStruct.m = toMinute[2]
    timeStruct.d = toMinute[3]
    timeStruct.h = toMinute[4]
    timeStruct.min = toMinute[5]
    return timeStruct
  }

  const toSecond = matchToSecond(normalized)
  if (toSecond) {
    timeStruct.y = toSecond[1]
    timeStruct.m = toSecond[2]
    timeStruct.d = toSecond[3]
    timeStruct.h = toSecond[4]
    timeStruct.min = toSecond[5]
    timeStruct.s = toSecond[6]
    return timeStruct
  }

  return timeStruct
}

function inferYearPrecision(yearStr: string, max: TimePrecision): TimePrecision {
  if (!yearStr) return clampToMax("y", max)

  const year = BigInt(yearStr)
  const abs = year < 0n ? -year : year

  const candidates: Array<[TimePrecision, bigint]> = [
    ["G", 1_000_000_000n],
    ["100M", 100_000_000n],
    ["10M", 10_000_000n],
    ["M", 1_000_000n],
    ["100k", 100_000n],
    ["10k", 10_000n],
    ["k", 1_000n],
    ["100y", 100n],
    ["10y", 10n],
  ]

  for (const [p, factor] of candidates) {
    if (abs >= factor && year % factor === 0n && abs > 9999n) {
      return clampToMax(p, max)
    }
  }

  return clampToMax("y", max)
}

function inferPrecisionFromNormalized(normalized: string): TimePrecision {
  let inferred: TimePrecision

  if (matchToSecond(normalized)) {
    inferred = "s"
  } else if (matchToMinute(normalized)) {
    inferred = "min"
  } else if (matchToHour(normalized)) {
    inferred = "h"
  } else if (matchToDay(normalized)) {
    inferred = "d"
  } else if (matchToMonth(normalized)) {
    inferred = "m"
  } else {
    const y = matchToYear(normalized)
    inferred = y ? inferYearPrecision(y[1], props.maxPrecision) : timePrecision.value
  }

  return clampToMax(inferred, props.maxPrecision as TimePrecision)
}

function clampToMax(p: TimePrecision, max: TimePrecision): TimePrecision {
  const pr = PRECISION_RANK.get(p)
  const mr = PRECISION_RANK.get(max)

  if (pr == null) throw new Error(`unknown precision: ${p}`)
  if (mr == null) throw new Error(`unknown maxPrecision: ${max}`)

  return pr < mr ? max : p
}

function toCanonicalString(timeStruct: { y: string; m: string; d: string; h: string; min: string; s: string }, precision: TimePrecision): string {
  const y = timeStruct.y || "0000"

  if (
    precision === "G" ||
    precision === "100M" ||
    precision === "10M" ||
    precision === "M" ||
    precision === "100k" ||
    precision === "10k" ||
    precision === "k" ||
    precision === "100y" ||
    precision === "10y" ||
    precision === "y"
  ) {
    return y
  }

  const m = pad2(timeStruct.m || "01")
  if (precision === "m") return `${y}-${m}`

  const d = pad2(timeStruct.d || "01")
  if (precision === "d") return `${y}-${m}-${d}`

  const h = pad2(timeStruct.h || "00")
  if (precision === "h") return `${y}-${m}-${d} ${h}`

  const min = pad2(timeStruct.min || "00")
  if (precision === "min") return `${y}-${m}-${d} ${h}:${min}`

  const s = pad2(timeStruct.s || "00")
  if (precision === "s") return `${y}-${m}-${d} ${h}:${min}:${s}`

  return ""
}

function formatYear(year: number): string {
  const sign = year < 0 ? "-" : ""
  const abs = Math.abs(year)
  return sign + String(abs).padStart(4, "0")
}

function roundDown(value: number, factor: number) {
  return Math.floor(value / factor) * factor
}

function applyPrecision(timeStruct: { y: string; m: string; d: string; h: string; min: string; s: string }, precision: TimePrecision): string {
  const year = parseInt(timeStruct.y || "0000", 10)

  switch (precision) {
    case "G":
      return formatYear(roundDown(year, 1_000_000_000))
    case "100M":
      return formatYear(roundDown(year, 100_000_000))
    case "10M":
      return formatYear(roundDown(year, 10_000_000))
    case "M":
      return formatYear(roundDown(year, 1_000_000))
    case "100k":
      return formatYear(roundDown(year, 100_000))
    case "10k":
      return formatYear(roundDown(year, 10_000))
    case "k":
      return formatYear(roundDown(year, 1_000))
    case "100y":
      return formatYear(roundDown(year, 100))
    case "10y":
      return formatYear(roundDown(year, 10))
    case "y":
      return timeStruct.y || "0000"
    case "m":
    case "d":
    case "h":
    case "min":
    case "s":
      return toCanonicalString(timeStruct, precision)
    default:
      return ""
  }
}

function emitCanonicalFromDisplay(): void {
  if (!displayValue.value) {
    model.value = ""
    return
  }

  const normalized = normalizeForParsing(displayValue.value)
  const cleaned = cleanInputNormalized(normalized)

  const validationErrorMessage = progressiveValidate(cleaned)
  errorMessage.value = validationErrorMessage

  if (validationErrorMessage) return

  const struct = getStructuredTimestamp(cleaned)
  const inferredPrecision = inferPrecisionFromNormalized(cleaned)

  const canonical = toCanonicalString(struct, inferredPrecision)
  if (canonical && canonical !== model.value) {
    model.value = canonical
  }
}

function autoAdaptPrecisionFromDisplay(): void {
  const normalized = normalizeForParsing(displayValue.value)
  const cleaned = cleanInputNormalized(normalized)

  const validationErrorMessage = progressiveValidate(cleaned)
  // Only adapt when the structure isn't clearly broken.
  if (validationErrorMessage && validationErrorMessage !== "") return

  const inferred = inferPrecisionFromNormalized(cleaned)

  if (inferred !== timePrecision.value) {
    timePrecision.value = inferred
    precision.value = inferred
  }
}

const emitCanonicalDebounce = debounce(() => {
  if (abortController.signal.aborted) {
    return
  }

  emitCanonicalFromDisplay()
}, DEBOUNCE_MS)

function onKeydown() {
  if (abortController.signal.aborted) {
    return
  }

  errorMessage.value = ""
  emitCanonicalDebounce.cancel()
}

function onInput() {
  if (abortController.signal.aborted) {
    return
  }

  // While user types: precision adapts, but text in the input never changes.
  autoAdaptPrecisionFromDisplay()
  emitCanonicalDebounce()
}

function onFocus() {
  if (abortController.signal.aborted) {
    return
  }

  isEditing.value = true
}

function onBlur() {
  if (abortController.signal.aborted) {
    return
  }

  isEditing.value = false
  emitCanonicalDebounce.cancel()
  emitCanonicalFromDisplay()
}

function onPrecisionSelected(p: TimePrecision) {
  if (abortController.signal.aborted) {
    return
  }

  // v-model will already update timePrecision, but we treat this as a manual intent.
  const normalized = normalizeForParsing(displayValue.value)
  const cleaned = cleanInputNormalized(normalized)

  const validationErrorMessage = progressiveValidate(cleaned)
  errorMessage.value = validationErrorMessage

  precision.value = p

  if (validationErrorMessage) return

  const struct = getStructuredTimestamp(cleaned)
  const next = applyPrecision(struct, p)

  displayValue.value = next
  model.value = next
}

watch(
  () => precision.value,
  (p) => {
    if (p && p !== timePrecision.value) timePrecision.value = p
  },
)
</script>

<template>
  <div class="flex flex-row gap-x-1 sm:gap-x-4" v-bind="$attrs">
    <div class="flex grow flex-col">
      <label :for="inputId" class="mb-1"><slot name="timestamp-label">Timestamp</slot></label>

      <InputText
        :id="inputId"
        v-model="displayValue"
        spellcheck="false"
        autocorrect="off"
        autocapitalize="none"
        :readonly="readonly"
        :invalid="isInvalid"
        :progress="progress"
        @focus="onFocus"
        @blur="onBlur"
        @keydown="onKeydown"
        @input="onInput"
      />
    </div>

    <Listbox v-model="timePrecision" :disabled="progress > 0 || readonly" as="div" class="flex w-48 flex-col" @update:model-value="onPrecisionSelected">
      <ListboxLabel class="mb-1"><slot name="precision-label">Precision</slot></ListboxLabel>

      <div class="relative">
        <!--
          Expected padding is py-2 and px-3, same what InputText has, but we add additional
          padding on the right to make space for the icon.
        -->
        <ListboxButton
          class="relative w-full rounded-sm border-none py-2 pr-10 pl-3 text-left shadow-sm ring-2 ring-neutral-300 outline-none focus:ring-2"
          :class="{
            'cursor-not-allowed': progress > 0 || readonly,
            'bg-gray-100': progress > 0 || readonly,
            'bg-white': progress === 0 && !readonly,
            'text-gray-800': progress > 0 || readonly,
            'hover:ring-neutral-300 focus:ring-primary-300': progress > 0 || readonly,
            'hover:ring-neutral-400 focus:ring-primary-500': progress === 0 && !readonly,
          }"
        >
          <div class="truncate" :title="precisionLabel(timePrecision)">{{ precisionLabel(timePrecision) }}</div>

          <div class="pointer-events-none absolute inset-y-0 right-0 flex items-center pr-2">
            <ChevronUpDownIcon class="h-5 w-5 text-gray-400" aria-hidden="true" />
          </div>
        </ListboxButton>

        <ListboxOptions class="absolute z-10 mt-1 max-h-40 w-full overflow-auto rounded-sm bg-white shadow-sm ring-2 ring-neutral-300 outline-none">
          <ListboxOption v-for="tp in timePrecisionWithMax" :key="tp" v-slot="{ active, selected }" :value="tp" as="template">
            <li class="cursor-pointer p-1 outline-none select-none">
              <!--
                We have an additional div so that the ring has the space to be shown.
                li element has p-1 for ring space, together with py-1 and px-2 we get the effective padding
                for option content of py-2 and px-3, same what InputText and ListboxButton have.
              -->
              <div class="flex flex-row justify-between gap-x-1 rounded-sm px-2 py-1" :class="active ? 'ring-2 ring-primary-500' : ''">
                <div class="truncate" :class="selected ? 'font-medium' : ''" :title="precisionLabel(tp)">{{ precisionLabel(tp) }}</div>

                <CheckIcon v-if="selected" class="h-5 w-5 text-primary-600" aria-hidden="true" />
              </div>
            </li>
          </ListboxOption>
        </ListboxOptions>
      </div>
    </Listbox>
  </div>

  <div v-if="errorMessage" class="mt-1 text-sm text-error-600">{{ errorMessage }}</div>
  <div v-else class="mt-1 text-sm text-neutral-500 italic">Format: YYYY-MM-DD HH:MM:SS</div>
</template>
