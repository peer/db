<script setup lang="ts">
import { ref, computed } from "vue"
import {debounce} from "lodash-es"

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
      invalid: false
    },
)

const emit = defineEmits<{
  "update:modelValue": [value: string]
}>()

const DEBOUNCE_MS = 2000

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

function progressiveValidate(raw: string): string {
  if (!raw) return ""

  // Normalize 'T' to space
  raw = raw.replace("T", " ")

  // Checks year with negativity
  if (/^-?\d*$/.test(raw)) return ""

  // Year + month (partial)
  const ym = raw.match(/^(-?\d+)-(\d{1,2})$/)
  if (ym) {
    const [, , month] = ym
    return Number(month) >= 0 && Number(month) <= 12 ? "" : "Months need to be between 1-12."
  }

  // Year + month + day (partial)
  const ymd = raw.match(/^(-?\d+)-(\d{1,2})-(\d{1,2})$/)
  if (ymd) {
    const [, , month, day] = ymd

    let errMessage = ""
    if (Number(month) < 1 || Number(month) > 12) errMessage = "Months need to be between 1-12."
    if (Number(day) < 0 || Number(day) > 31) errMessage = "Days need to be between 1-31."
    return errMessage
  }

  // Full date + space, waiting for hours
  if (/^-?\d+-\d{1,2}-\d{1,2} $/.test(raw)) return ""

  // Hour in progress (1–2 digits)
  const h = raw.match(/^(-?\d+)-(\d{1,2})-(\d{1,2}) (\d{1,2})$/)
  if (h) {
    const hour = Number(h[4])
    return hour >= 0 && hour <= 23 ? "" : "Hours needs to be between 0-23."
  }

  // Minute in progress
  const hm = raw.match(
      /^(-?\d+)-(\d{1,2})-(\d{1,2}) (\d{1,2}):(\d{1,2})$/,
  )
  if (hm) {
    const minute = Number(hm[5])
    return minute >= 0 && minute <= 59 ? "" : "Month needs to be between 0-59."
  }

  // Second in progress
  const hms = raw.match(
      /^(-?\d+)-(\d{1,2})-(\d{1,2}) (\d{1,2}):(\d{1,2}):(\d{1,2})$/,
  )

  if (hms) {
    const [, , month, day, hour, minute, second] = hms.map(Number)
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

function strictlyValid(full: string): string {
  const m = full.match(
      /^(-?\d+)-(\d{2})-(\d{2}) (\d{2}):(\d{2}):(\d{2})$/,
  )
  if (!m) return "Invalid timestamp structure."

  const [, , month, day, hour, minute, second] = m.map(Number)

  if (month < 1 || month > 12) return "Month needs to be between 1-12."
  if (day < 1 || day > 31) return "Day needs to be between 1-31."
  if (hour < 0 || hour > 23) return "Hours need to be between 0-23."
  if (minute < 0 || minute > 59) return "Minutes need to be between 0-59."
  if (second < 0 || second > 59) return "Seconds need to be between 0-59."

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
  // YYYY-M
  const ym = raw.match(/^(-?\d+)-(\d{1,2})$/)
  if (ym) {
    const [, y, mo] = ym
    return `${y}-${pad2(mo, false)}`
  }

  // YYYY-MM-D
  const ymd = raw.match(/^(-?\d+)-(\d{1,2})-(\d{1,2})$/)
  if (ymd) {
    const [, y, mo, da] = ymd
    return `${y}-${pad2(mo, false)}-${pad2(da, false)}`
  }

  // YYYY-MM-DD H
  const ymdh = raw.match(/^(-?\d+)-(\d{1,2})-(\d{1,2}) (\d{1,2})$/)
  if (ymdh) {
    const [, y, mo, da, h] = ymdh
    return `${y}-${pad2(mo, false)}-${pad2(da, false)} ${pad2(h)}`
  }

  // YYYY-MM-DD HH:M
  const ymdhm = raw.match(/^(-?\d+)-(\d{1,2})-(\d{1,2}) (\d{1,2}):(\d{1,2})$/)
  if (ymdhm) {
    const [, y, mo, da, h, mi] = ymdhm
    return `${y}-${pad2(mo, false)}-${pad2(da, false)} ${pad2(h)}:${pad2(mi)}`
  }

  // YYYY-MM-DD HH:MM:S
  const ymdhms = raw.match(
      /^(-?\d+)-(\d{1,2})-(\d{1,2}) (\d{1,2}):(\d{1,2}):(\d{1,2})$/
  )
  if (ymdhms) {
    const [, y, mo, da, h, mi, s] = ymdhms
    return `${y}-${pad2(mo, false)}-${pad2(da, false)} ${pad2(h)}:${pad2(mi)}:${pad2(s)}`
  }

  return raw
}

function validateInput(raw: string): string {
  const errorMessage = progressiveValidate(raw);
  if (errorMessage) return errorMessage
  // if (!progressiveValidate(raw)) return "Invalid timestamp structure"

  const full = raw.match(
      /^(-?\d+)-(\d{2})-(\d{2}) (\d{2}):(\d{2}):(\d{2})$/
  )
  if (!full) return ""

  return strictlyValid(raw)
}

const runValidation = debounce(() => {
  const raw = value.value

  const cleaned = cleanInput(raw)
  const formatted = formatInput(cleaned)
  const validationErrorMessage = validateInput(formatted)

  value.value = formatted
  isTimeInvalid.value = validationErrorMessage !== ""
  errorMessage.value = validationErrorMessage
}, DEBOUNCE_MS)

function onKeydown() {
  runValidation.cancel()
}

function onInput() {
  runValidation()
}
</script>

<template>
  <div class="w-full flex flex-col gap-1">
    <InputText
        :id="id"
        v-model="value"
        :readonly="readonly"
        :invalid="isInvalid"
        class="w-full"
        @keydown="onKeydown"
        @input="onInput"
    />

    <p class="text-sm text-slate-500">
      Hint: (-)YYYY...-MM-DD HH:MM:SS
    </p>

    <p v-if="errorMessage" class="text-sm text-red-500">
      {{ errorMessage }}
    </p>
  </div>
</template>