<script setup lang="ts">
import type { TimePrecision } from "@/types"

import { CheckIcon, ChevronUpDownIcon } from "@heroicons/vue/20/solid"
import { ref, computed, readonly as vueReadonly, watch, onBeforeMount, onMounted, useAttrs, nextTick } from "vue"
import { debounce } from "lodash-es"
import { Listbox, ListboxButton, ListboxOption, ListboxOptions } from "@headlessui/vue"

import InputText from "@/components/InputText.vue"
import { daysIn } from "@/time.ts"

// We want all fallthrough attributes to be passed to the link element.
defineOptions({
  inheritAttrs: false,
})

const props = withDefaults(
  defineProps<{
    modelValue?: string
    readonly?: boolean
    invalid?: boolean
    precision?: TimePrecision
    progress?: number
  }>(),
  {
    modelValue: "",
    readonly: false,
    invalid: false,
    precision: "y",
    progress: 0,
  },
)

const emit = defineEmits<{
  "update:modelValue": [value: string]
  "update:precision": [value: TimePrecision]
}>()

const attrs = useAttrs()

const DEBOUNCE_MS = 2000

const timePrecisionOptions = vueReadonly(["G", "100M", "10M", "M", "100k", "10k", "k", "100y", "10y", "y", "m", "d", "h", "min", "s"] as const)
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

const timePrecision = ref<TimePrecision>("y")

const isEditing = ref(false)
const isTimeInvalid = ref(false)
const errorMessage = ref("")

const isInvalid = computed(() => props.invalid || isTimeInvalid.value)

const inputId = computed(() => {
  return typeof attrs.id === "string" ? attrs.id : "identifier-property"
})

const displayValue = ref(props.modelValue ?? "")

const value = computed({
  get() {
    return displayValue.value
  },
  set(v: string) {
    displayValue.value = v
  },
})

onBeforeMount(() => {
  timePrecision.value = props.precision
  displayValue.value = props.modelValue ?? ""
})

onMounted(async () => {
  if (!displayValue.value) return

  // We want to validate and emit the canonical value on mount.
  // However, we must not overwrite the text shown in the input.
  // The props->display watcher only syncs when isEditing is false, so we temporarily
  // mark the component as "editing" to block that sync during this initial emit.
  isEditing.value = true

  // Update the precision based on the initial text
  // and emit the canonical modelValue.
  autoAdaptPrecisionFromDisplay()
  emitCanonicalFromDisplay()

  // Wait one tick so the parent can process the emitted update and push the new prop back.
  await nextTick()

  // Re-enable normal props->display syncing for future external updates.
  isEditing.value = false
})

watch(
  () => props.modelValue,
  (v) => {
    // If parent updates modelValue externally, reflect it unless user is actively editing.
    if (!isEditing.value) displayValue.value = v ?? ""
  },
)

const pad2 = (n: string) => n.padStart(2, "0")

function normalizeForParsing(raw: string): string {
  if (!raw) return ""

  let r = raw

  // Normalize any date-whitespace-time boundary to 'T' before stripping whitespace.
  // Example: "2023-1-1    2:3" => "2023-1-1T2:3"
  r = r.replace(/(-?\d+)-(\d{1,2})-(\d{1,2})\s+([0-9])/g, "$1-$2-$3T$4")

  // Normalize lowercase 't' to 'T'
  r = r.replace(/t/g, "T")

  // Remove all remaining whitespace everywhere
  r = r.replace(/\s+/g, "")

  return r
}

function cleanInputNormalized(raw: string): string {
  let r = raw

  // Remove trailing separators for validation.
  r = r.replace(/[-:T]+$/, "")

  return r
}

const matchToYear = (s: string) => s.match(/^(-?\d+)$/)
const matchToMonth = (s: string) => s.match(/^(-?\d+)-(\d{1,2})$/)
const matchToDay = (s: string) => s.match(/^(-?\d+)-(\d{1,2})-(\d{1,2})$/)
const matchToHour = (s: string) => s.match(/^(-?\d+)-(\d{1,2})-(\d{1,2})T(\d{1,2})$/)
const matchToMinute = (s: string) => s.match(/^(-?\d+)-(\d{1,2})-(\d{1,2})T(\d{1,2}):(\d{1,2})$/)
const matchToSecond = (s: string) => s.match(/^(-?\d+)-(\d{1,2})-(\d{1,2})T(\d{1,2}):(\d{1,2}):(\d{1,2})$/)

function precisionLabel(p: TimePrecision): string {
  return precisionLabels[p]
}

function progressiveValidate(normalized: string): string {
  if (!normalized) return ""

  // Year in progress: "202", "2023", "2023-"
  if (/^-?\d*-?$/.test(normalized)) return ""

  // Month in progress: "2023-1", "2023-12", "2023-12-"
  if (/^-?\d+-\d{0,2}-?$/.test(normalized)) {
    const m = matchToMonth(normalized.replace(/-$/, ""))
    if (!m) return ""
    const month = Number(m[2])
    if (month === 0) return ""
    return month >= 1 && month <= 12 ? "" : "Months need to be between 1-12."
  }

  // Day in progress: "2023-1-1", "2023-1-1T", "2023-1-1T1"
  if (/^-?\d+-\d{1,2}-\d{0,2}T?$/.test(normalized)) {
    const asDay = matchToDay(normalized.replace(/T$/, ""))
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
  if (/^-?\d+-\d{1,2}-\d{1,2}T\d{1,2}:?$/.test(normalized)) return ""
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
  if (/^-?\d+-\d{1,2}-\d{1,2}T\d{1,2}:\d{1,2}:?$/.test(normalized)) return ""
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

function inferYearPrecision(yearStr: string): TimePrecision {
  const year = parseInt(yearStr || "0", 10)
  const abs = Math.abs(year)

  const candidates: Array<[TimePrecision, number]> = [
    ["G", 1_000_000_000],
    ["100M", 100_000_000],
    ["10M", 10_000_000],
    ["M", 1_000_000],
    ["100k", 100_000],
    ["10k", 10_000],
    ["k", 1_000],
    ["100y", 100],
    ["10y", 10],
  ]

  for (const [p, factor] of candidates) {
    if (abs >= factor && year % factor === 0) return p
  }

  return "y"
}

function inferPrecisionFromNormalized(normalized: string): TimePrecision {
  if (matchToSecond(normalized)) return "s"
  if (matchToMinute(normalized)) return "min"
  if (matchToHour(normalized)) return "h"
  if (matchToDay(normalized)) return "d"
  if (matchToMonth(normalized)) return "m"

  const y = matchToYear(normalized)
  if (y) return inferYearPrecision(y[1])

  // While typing, we might be between tokens. Keep current precision.
  return timePrecision.value
}

function toCanonicalString(timeStruct: { y: string; m: string; d: string; h: string; min: string; s: string }, precision: TimePrecision): string {
  const y = timeStruct.y || "0"

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
  if (precision === "h") return `${y}-${m}-${d}T${h}`

  const min = pad2(timeStruct.min || "00")
  if (precision === "min") return `${y}-${m}-${d}T${h}:${min}`

  const s = pad2(timeStruct.s || "00")
  if (precision === "s") return `${y}-${m}-${d}T${h}:${min}:${s}`

  return ""
}

function applyPrecision(timeStruct: { y: string; m: string; d: string; h: string; min: string; s: string }, precision: TimePrecision): string {
  const year = parseInt(timeStruct.y || "0", 10)

  function roundDown(value: number, factor: number) {
    return Math.floor(value / factor) * factor
  }

  switch (precision) {
    case "G":
      return String(roundDown(year, 1_000_000_000))
    case "100M":
      return String(roundDown(year, 100_000_000))
    case "10M":
      return String(roundDown(year, 10_000_000))
    case "M":
      return String(roundDown(year, 1_000_000))
    case "100k":
      return String(roundDown(year, 100_000))
    case "10k":
      return String(roundDown(year, 10_000))
    case "k":
      return String(roundDown(year, 1_000))
    case "100y":
      return String(roundDown(year, 100))
    case "10y":
      return String(roundDown(year, 10))
    case "y":
      return timeStruct.y || "0"
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
    emit("update:modelValue", "")
    return
  }

  const normalized = normalizeForParsing(displayValue.value)
  const cleaned = cleanInputNormalized(normalized)

  const validationErrorMessage = progressiveValidate(cleaned)
  isTimeInvalid.value = validationErrorMessage !== ""
  errorMessage.value = validationErrorMessage

  if (validationErrorMessage) return

  const struct = getStructuredTimestamp(cleaned)
  const inferredPrecision = inferPrecisionFromNormalized(cleaned)

  const canonical = toCanonicalString(struct, inferredPrecision)
  if (canonical && canonical !== (props.modelValue ?? "")) {
    emit("update:modelValue", canonical)
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
    emit("update:precision", inferred)
  }
}

const emitCanonicalDebounce = debounce(() => {
  emitCanonicalFromDisplay()
}, DEBOUNCE_MS)

function onKeydown() {
  emitCanonicalDebounce.cancel()
}

function onInput() {
  // While user types: precision adapts, but text in the input never changes.
  autoAdaptPrecisionFromDisplay()
  emitCanonicalDebounce()
}

function onFocus() {
  isEditing.value = true
}

function onBlur() {
  isEditing.value = false
  emitCanonicalDebounce.cancel()
  emitCanonicalFromDisplay()
}

function onPrecisionSelected(p: TimePrecision) {
  // v-model will already update timePrecision, but we treat this as a manual intent.
  const normalized = normalizeForParsing(displayValue.value)
  const cleaned = cleanInputNormalized(normalized)

  const validationErrorMessage = progressiveValidate(cleaned)
  isTimeInvalid.value = validationErrorMessage !== ""
  errorMessage.value = validationErrorMessage

  emit("update:precision", p)

  if (validationErrorMessage) return

  const struct = getStructuredTimestamp(cleaned)
  const next = applyPrecision(struct, p)

  displayValue.value = next
  emit("update:modelValue", next)
}

watch(
  () => props.precision,
  (p) => {
    if (p && p !== timePrecision.value) timePrecision.value = p
  },
)
</script>

<template>
  <div class="w-full flex flex-col gap-1">
    <div class="flex gap-2 w-full">
      <div class="flex flex-col gap-1 w-full">
        <label :for="inputId" class="mt-4 mb-1">Timestamp</label>

        <InputText
          :id="inputId"
          v-model="value"
          v-bind="$attrs"
          :spellcheck="false"
          :readonly="readonly"
          :invalid="isInvalid"
          :progress="progress"
          class="w-full"
          @focus="onFocus"
          @blur="onBlur"
          @keydown="onKeydown"
          @input="onInput"
        />
      </div>

      <div v-if="!readonly" class="flex flex-col gap-1">
        <label for="precision-list" class="mt-4 mb-1">Precision</label>

        <Listbox id="precision-list" v-model="timePrecision" :disabled="progress > 0" class="w-48" @update:model-value="onPrecisionSelected">
          <div class="relative">
            <ListboxButton
              class="relative w-full rounded p-2 text-left shadow hover:cursor-pointer ring-2 ring-neutral-300 hover:ring-neutral-400"
              :class="{
                '!cursor-not-allowed bg-gray-100 text-gray-800 hover:!ring-neutral-300 focus:!border-primary-300 focus:!ring-primary-300': progress > 0,
              }"
            >
              <span class="block truncate">
                {{ precisionLabel(timePrecision) }}
              </span>

              <span class="pointer-events-none absolute inset-y-0 right-0 flex items-center pr-2">
                <ChevronUpDownIcon class="h-5 w-5 text-neutral-300" aria-hidden="true" />
              </span>
            </ListboxButton>

            <ListboxOptions class="absolute z-10 mt-2 max-h-40 w-full overflow-auto rounded bg-white shadow ring-2 ring-neutral-300 focus:outline-none">
              <ListboxOption v-for="tp in timePrecisionOptions" :key="tp" v-slot="{ active, selected }" :value="tp" as="template" class="hover:cursor-pointer">
                <li :class="[active ? 'bg-neutral-100' : '', 'relative cursor-default select-none py-2 pl-10 pr-4']">
                  <span :class="[selected ? 'font-medium' : 'font-normal', 'block truncate']">
                    {{ precisionLabel(tp) }}
                  </span>

                  <span v-if="selected" class="absolute inset-y-0 left-0 flex items-center pl-3 text-primary-500">
                    <CheckIcon class="h-5 w-5" aria-hidden="true" />
                  </span>
                </li>
              </ListboxOption>
            </ListboxOptions>
          </div>
        </Listbox>
      </div>
    </div>

    <p class="text-sm text-slate-500">Hint: (-)YYYY...-MM-DD HH:MM:SS</p>

    <div v-if="errorMessage" class="my-1">
      <div class="text-sm"
        ><i class="text-error-600">{{ errorMessage }}</i></div
      >
    </div>
  </div>
</template>
