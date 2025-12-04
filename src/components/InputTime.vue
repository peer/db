<script setup lang="ts">
import type { TimePrecision } from "@/types"

import { ref, computed, readonly as vueReadonly, watch } from "vue"
import { debounce } from "lodash-es"
import { Listbox, ListboxButton, ListboxOption, ListboxOptions } from "@headlessui/vue"

import InputText from "@/components/InputText.vue"

const props = withDefaults(
  defineProps<{
    modelValue?: string
    readonly?: boolean
    id?: string
    invalid?: boolean
  }>(),
  {
    modelValue: "",
    readonly: false,
    id: "timestamp-input",
    invalid: false,
  },
)

const emit = defineEmits<{
  "update:modelValue": [value: string]
  "update:precision": [value: TimePrecision]
}>()

const DEBOUNCE_MS = 2000

const timePrecisionOptions = vueReadonly(["G", "100M", "10M", "M", "100k", "10k", "k", "100y", "10y", "y", "m", "d", "h", "min", "s"] as const)
const timePrecision = ref<TimePrecision>("d")

const isTimeInvalid = ref(false)
const errorMessage = ref("")

const isInvalid = computed(() => props.invalid || isTimeInvalid.value)

const value = computed({
  get() {
    return props.modelValue
  },
  set(v: string) {
    emit("update:modelValue", v)
  },
})

const pad2 = (n: string, padToZero = true) => {
  if (!padToZero && (n === "0" || n === "00")) return "01"
  return n.padStart(2, "0")
}

const matchToYear = (s: string) => s.match(/^(-?\d+)$/)
const matchToMonth = (s: string) => s.match(/^(-?\d+)-(\d{1,2})$/)
const matchToDay = (s: string) => s.match(/^(-?\d+)-(\d{1,2})-(\d{1,2})$/)
const matchToHour = (s: string) => s.match(/^(-?\d+)-(\d{1,2})-(\d{1,2}) (\d{1,2})$/)
const matchToMinute = (s: string) => s.match(/^(-?\d+)-(\d{1,2})-(\d{1,2}) (\d{1,2}):(\d{1,2})$/)
const matchToSecond = (s: string) => s.match(/^(-?\d+)-(\d{1,2})-(\d{1,2}) (\d{1,2}):(\d{1,2}):(\d{1,2})$/)
const matchFull = (s: string) => s.match(/^(-?\d+)-(\d{2})-(\d{2}) (\d{2}):(\d{2}):(\d{2})$/)

function progressiveValidate(raw: string): string {
  if (!raw) return ""

  // Normalize 'T' to space
  const normalized = raw.replace("T", " ")

  // Checks year with negativity
  if (/^-?\d*$/.test(normalized)) return ""

  const toMonth = matchToMonth(normalized)
  if (toMonth) {
    const [, , month] = toMonth
    return Number(month) >= 0 && Number(month) <= 12 ? "" : "Months need to be between 1-12."
  }

  const toDay = matchToDay(normalized)
  if (toDay) {
    const [, , month, day] = toDay

    let errMessage = ""
    if (Number(month) < 1 || Number(month) > 12) errMessage = "Months need to be between 1-12."
    if (Number(day) < 0 || Number(day) > 31) errMessage = "Days need to be between 1-31."
    return errMessage
  }

  // Full date + space
  if (/^-?\d+-\d{1,2}-\d{1,2} $/.test(normalized)) return ""

  const toHours = matchToHour(normalized)
  if (toHours) {
    const hour = Number(toHours[4])
    return hour >= 0 && hour <= 23 ? "" : "Hours needs to be between 0-23."
  }

  const toMinutes = matchToMinute(normalized)
  if (toMinutes) {
    const minute = Number(toMinutes[5])
    return minute >= 0 && minute <= 59 ? "" : "Minutes needs to be between 0-59."
  }

  const toSeconds = matchToSecond(normalized)
  if (toSeconds) {
    const [, , month, day, hour, minute, second] = toSeconds.map(Number)
    if (month < 1 || month > 12) return "Month needs to be between 1-12."
    if (day < 1 || day > 31) return "Day needs to be between 1-31."
    if (hour < 0 || hour > 23) return "Hours need to be between 0-23."
    if (minute < 0 || minute > 59) return "Minutes need to be between 0-59."
    if (second < 0 || second > 59) return "Seconds need to be between 0-59."

    return ""
  }

  // Everything else is structurally broken
  return "Invalid timestamp structure."
}

function getStructuredTimestamp(formatted: string): { y: string; m: string; d: string; h: string; min: string; s: string } {
  const timeStruct = { y: "", m: "", d: "", h: "", min: "", s: "" }

  if (!formatted) return timeStruct

  const toYear = matchToYear(formatted)
  if (toYear) {
    const [, y] = toYear

    timeStruct.y = y

    return timeStruct
  }

  const toMonth = matchToMonth(formatted)
  if (toMonth) {
    const [, y, m] = toMonth

    timeStruct.y = y
    timeStruct.m = m

    return timeStruct
  }

  const toDay = matchToDay(formatted)
  if (toDay) {
    const [, y, m, d] = toDay

    timeStruct.y = y
    timeStruct.m = m
    timeStruct.d = d

    return timeStruct
  }

  const toHour = matchToHour(formatted)
  if (toHour) {
    const [, y, m, d, h] = toHour

    timeStruct.y = y
    timeStruct.m = m
    timeStruct.d = d
    timeStruct.h = h

    return timeStruct
  }

  const toMinute = matchToMinute(formatted)
  if (toMinute) {
    const [, y, m, d, h, min] = toMinute

    timeStruct.y = y
    timeStruct.m = m
    timeStruct.d = d
    timeStruct.h = h
    timeStruct.min = min

    return timeStruct
  }

  const toSecond = matchToSecond(formatted)
  if (toSecond) {
    const [, y, m, d, h, min, s] = toSecond

    timeStruct.y = y
    timeStruct.m = m
    timeStruct.d = d
    timeStruct.h = h
    timeStruct.min = min
    timeStruct.s = s

    return timeStruct
  }

  return timeStruct
}

function strictlyValid(full: string): string {
  const match = matchFull(full)
  if (!match) return "Invalid timestamp structure."

  const [, , m, d, h, min, s] = match.map(Number)

  if (m < 1 || m > 12) return "Month needs to be between 1-12."
  if (d < 1 || d > 31) return "Day needs to be between 1-31."
  if (h < 0 || h > 23) return "Hours need to be between 0-23."
  if (min < 0 || min > 59) return "Minutes need to be between 0-59."
  if (s < 0 || s > 59) return "Seconds need to be between 0-59."

  return ""
}

function cleanInput(raw: string): string {
  let r = raw

  // remove trailing T
  r = r.replace(/^(-?\d+)-(\d{1,2})-(\d{1,2})T$/, "$1-$2-$3")

  // replace T with space only when followed by a time digit
  r = r.replace("T", " ")

  // remove trailing separators
  r = r.replace(/[-: ]+$/, "")

  return r
}

function formatInput(raw: string): string {
  const toMonth = matchToMonth(raw)
  if (toMonth) {
    const [, y, m] = toMonth
    return `${y}-${pad2(m, false)}`
  }

  const toDay = matchToDay(raw)
  if (toDay) {
    const [, y, m, d] = toDay
    return `${y}-${pad2(m, false)}-${pad2(d, false)}`
  }

  const toHour = matchToHour(raw)
  if (toHour) {
    const [, y, m, d, h] = toHour
    return `${y}-${pad2(m, false)}-${pad2(d, false)} ${pad2(h)}`
  }

  const toMinute = matchToMinute(raw)
  if (toMinute) {
    const [, y, m, d, h, min] = toMinute
    return `${y}-${pad2(m, false)}-${pad2(d, false)} ${pad2(h)}:${pad2(min)}`
  }

  const toSecond = matchToSecond(raw)
  if (toSecond) {
    const [, y, m, d, h, min, s] = toSecond
    return `${y}-${pad2(m, false)}-${pad2(d, false)} ${pad2(h)}:${pad2(min)}:${pad2(s)}`
  }

  return raw
}

function validateInput(raw: string): string {
  const errorMessage = progressiveValidate(raw)
  if (errorMessage) return errorMessage

  const full = matchFull(raw)
  if (!full) return ""

  return strictlyValid(raw)
}

function applyPrecision(timeStruct: { y: string; m: string; d: string; h: string; min: string; s: string }, precision: TimePrecision): string {
  switch (precision) {
    case "y":
      return timeStruct.y
    case "m":
      return `${timeStruct.y}-${timeStruct.m || "01"}`
    case "d":
      return `${timeStruct.y}-${timeStruct.m || "01"}-${timeStruct.d || "01"}`
    case "h":
      return `${timeStruct.y}-${timeStruct.m || "01"}-${timeStruct.d || "01"} ${timeStruct.h || "00"}`
    case "min":
      return `${timeStruct.y}-${timeStruct.m || "01"}-${timeStruct.d || "01"} ${timeStruct.h || "00"}:${timeStruct.min || "00"}`
    case "s":
      return `${timeStruct.y}-${timeStruct.m || "01"}-${timeStruct.d || "01"} ${timeStruct.h || "00"}:${timeStruct.min || "00"}:${timeStruct.s || "00"}`
    default:
      return ""
  }
}

function runValidation(): void {
  const raw = value.value

  const cleaned = cleanInput(raw)
  const formatted = formatInput(cleaned)
  const validationErrorMessage = validateInput(formatted)

  value.value = validationErrorMessage ? formatted : applyPrecision(getStructuredTimestamp(formatted), timePrecision.value)
  isTimeInvalid.value = validationErrorMessage !== ""
  errorMessage.value = validationErrorMessage
}

const runValidationDebounce = debounce(() => {
  runValidation()
}, DEBOUNCE_MS)

function onKeydown() {
  runValidationDebounce.cancel()
}

function onInput() {
  runValidationDebounce()
}

watch(timePrecision, () => {
  runValidation()
})
</script>

<template>
  <div class="w-full flex gap-2 items-center">
    <div class="w-full flex flex-col gap-1">
      <div class="flex gap-2">
        <InputText :id="id" v-model="value" :readonly="readonly" :invalid="isInvalid" class="w-full" @keydown="onKeydown" @input="onInput" />
        <Listbox v-model="timePrecision" class="w-20">
          <div class="relative">
            <ListboxButton class="w-full cursor-pointer p-2 bg-white text-left rounded border-0 shadow ring-2 ring-neutral-300 focus:ring-2">
              {{ timePrecision }}
            </ListboxButton>

            <ListboxOptions class="absolute max-h-40 overflow-scroll mt-2 w-full bg-white rounded border-0 shadow ring-2 ring-neutral-300 z-10">
              <ListboxOption v-for="p in timePrecisionOptions" :key="p" :value="p" class="cursor-pointer p-2 hover:bg-neutral-100">
                {{ p }}
              </ListboxOption>
            </ListboxOptions>
          </div>
        </Listbox>
      </div>

      <p class="text-sm text-slate-500">Hint: (-)YYYY...-MM-DD HH:MM:SS</p>

      <p v-if="errorMessage" class="text-sm text-red-500">
        {{ errorMessage }}
      </p>
    </div>
  </div>
</template>
