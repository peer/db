<script setup lang="ts">
import { ref, computed } from "vue"
import { debounce } from "lodash-es"

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

const pad2 = (n: string) => n.padStart(2, "0")

function progressiveValidate(raw: string): boolean {
  if (!raw) return true

  // Normalize 'T' to space
  raw = raw.replace("T", " ")

  // Checks year with negativity
  if (/^-?\d*$/.test(raw)) return true

  // Year + month (partial)
  const ym = raw.match(/^(-?\d+)-(\d{1,2})$/)
  if (ym) {
    const [, , month] = ym
    return Number(month) >= 0 && Number(month) <= 12
  }

  // Year + month + day (partial)
  const ymd = raw.match(/^(-?\d+)-(\d{1,2})-(\d{1,2})$/)
  if (ymd) {
    const [, , month, day] = ymd
    if (Number(month) < 1 || Number(month) > 12) return false
    if (Number(day) < 0 || Number(day) > 31) return false
    return true
  }

  // Full date + space, waiting for hours
  if (/^-?\d+-\d{1,2}-\d{1,2} $/.test(raw)) return true

  // Hour in progress (1–2 digits)
  const h = raw.match(/^(-?\d+)-(\d{1,2})-(\d{1,2}) (\d{1,2})$/)
  if (h) {
    const hour = Number(h[4])
    return hour >= 0 && hour <= 23
  }

  // Minute in progress
  const hm = raw.match(
      /^(-?\d+)-(\d{1,2})-(\d{1,2}) (\d{1,2}):(\d{1,2})$/,
  )
  if (hm) {
    const minute = Number(hm[5])
    return minute >= 0 && minute <= 59
  }

  // Second in progress
  const hms = raw.match(
      /^(-?\d+)-(\d{1,2})-(\d{1,2}) (\d{1,2}):(\d{1,2}):(\d{1,2})$/,
  )

  if (hms) {
    const [, , month, day, hour, minute, second] = hms.map(Number)
    if (month < 1 || month > 12) return false
    if (day < 1 || day > 31) return false
    if (hour > 23) return false
    if (minute > 59) return false
    return second >= 0 && second <= 59
  }

  // Everything else is structurally broken
  return false
}

function strictlyValid(full: string): boolean {
  const m = full.match(
      /^(-?\d+)-(\d{2})-(\d{2}) (\d{2}):(\d{2}):(\d{2})$/,
  )
  if (!m) return false

  const [, , month, day, hour, minute, second] = m.map(Number)

  return (
      month >= 1 && month <= 12 &&
      day   >= 1 && day   <= 31 &&
      hour  >= 0 && hour  <= 23 &&
      minute>= 0 && minute<= 59 &&
      second>= 0 && second<= 59
  )
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
    return `${y}-${pad2(mo)}`
  }

  // YYYY-MM-D
  const ymd = raw.match(/^(-?\d+)-(\d{2})-(\d{1,2})$/)
  if (ymd) {
    const [, y, mo, da] = ymd
    return `${y}-${mo}-${pad2(da)}`
  }

  // YYYY-MM-DD H
  const ymdh = raw.match(/^(-?\d+)-(\d{2})-(\d{2}) (\d{1,2})$/)
  if (ymdh) {
    const [, y, mo, da, h] = ymdh
    return `${y}-${mo}-${da} ${pad2(h)}`
  }

  // YYYY-MM-DD HH:M
  const ymdhm = raw.match(/^(-?\d+)-(\d{2})-(\d{2}) (\d{2}):(\d{1,2})$/)
  if (ymdhm) {
    const [, y, mo, da, h, mi] = ymdhm
    return `${y}-${mo}-${da} ${h}:${pad2(mi)}`
  }

  // YYYY-MM-DD HH:MM:S
  const ymdhms = raw.match(
      /^(-?\d+)-(\d{2})-(\d{2}) (\d{2}):(\d{2}):(\d{1,2})$/
  )
  if (ymdhms) {
    const [, y, mo, da, h, mi, s] = ymdhms
    return `${y}-${mo}-${da} ${h}:${mi}:${pad2(s)}`
  }

  // full strict timestamp
  const full = raw.match(
      /^(-?\d+)-(\d{2})-(\d{2}) (\d{2}):(\d{2}):(\d{2})$/
  )
  if (full) {
    const [, y, mo, da, h, mi, s] = full
    return `${y}-${mo}-${da} ${h}:${mi}:${s}`
  }

  return raw
}

function validateInput(raw: string): boolean {
  if (!progressiveValidate(raw)) return false

  const full = raw.match(
      /^(-?\d+)-(\d{2})-(\d{2}) (\d{2}):(\d{2}):(\d{2})$/
  )
  if (!full) return true

  return strictlyValid(raw)
}

const runValidation = debounce(() => {
  const raw = value.value

  const cleaned = cleanInput(raw)
  const formatted = formatInput(cleaned)
  const valid = validateInput(formatted)

  value.value = formatted
  isTimeInvalid.value = !valid
  errorMessage.value = valid ? "" : "Invalid timestamp structure"
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

    <p v-if="isTimeInvalid" class="text-sm text-red-500">
      {{ errorMessage }}
    </p>
  </div>
</template>