<script lang="ts">
import type { TimePrecision } from "@/document"
import type { DisplayTimePart } from "@/types"

// Precision hierarchy for determining what parts are precise.
export const PRECISION_LEVELS = ["G", "100M", "10M", "M", "100k", "10k", "k", "100y", "10y", "y", "m", "d", "h", "min", "s"] as const

// Number of trailing zeros that are imprecise for each precision level.
const TRAILING_ZEROS: Record<string, number> = {
  G: 9,
  "100M": 8,
  "10M": 7,
  M: 6,
  "100k": 5,
  "10k": 4,
  k: 3,
  "100y": 2,
  "10y": 1,
}

/**
 * Parses a timestamp string into its components.
 */
export function parseTimestamp(timestamp: string): { year: string; month: string; day: string; hour: string; minute: string; second: string } | null {
  const match = /^([+-]?\d{4,})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})Z$/.exec(timestamp)
  if (!match) {
    return null
  }
  return {
    year: match[1],
    month: match[2],
    day: match[3],
    hour: match[4],
    minute: match[5],
    second: match[6],
  }
}

/**
 * Returns the index of a precision level in the hierarchy.
 * Lower index means less precise (e.g., G=0, s=14).
 */
export function getPrecisionIndex(precision: TimePrecision): number {
  return PRECISION_LEVELS.indexOf(precision)
}

/**
 * Checks if a specific level is precise given the current precision.
 */
export function isPrecise(level: TimePrecision, precision: TimePrecision): boolean {
  const levelIndex = getPrecisionIndex(level)
  const precisionIndex = getPrecisionIndex(precision)
  return levelIndex <= precisionIndex
}

/**
 * Formats the year with grayed out imprecise trailing zeros.
 */
export function formatYearParts(yearStr: string, precision: TimePrecision): DisplayTimePart[] {
  const parts: DisplayTimePart[] = []
  const yearPrecise = isPrecise("y", precision)

  if (yearPrecise) {
    parts.push({ text: yearStr, precise: true })
    return parts
  }

  // For precisions less than "y", we need to gray out trailing zeros.
  const trailingZeros = TRAILING_ZEROS[precision] ?? 0
  const absYear = yearStr.startsWith("-") ? yearStr.slice(1) : yearStr
  const sign = yearStr.startsWith("-") ? "-" : ""

  if (trailingZeros > 0 && absYear.length > trailingZeros) {
    const preciseLen = absYear.length - trailingZeros
    parts.push({ text: sign + absYear.slice(0, preciseLen), precise: true })
    parts.push({ text: absYear.slice(preciseLen), precise: false })
  } else {
    parts.push({ text: yearStr, precise: true })
  }

  return parts
}

/**
 * Formats an absolute timestamp into display parts with precision indicators.
 */
export function formatAbsoluteParts(
  parsed: { year: string; month: string; day: string; hour: string; minute: string; second: string },
  precision: TimePrecision,
): DisplayTimePart[] {
  const parts: DisplayTimePart[] = []
  const precisionIndex = getPrecisionIndex(precision)

  // Year parts with grayed out imprecise trailing zeros.
  parts.push(...formatYearParts(parsed.year, precision))

  // For year-only precisions, stop here.
  if (precisionIndex <= getPrecisionIndex("y")) {
    return parts
  }

  // Month.
  const monthPrecise = isPrecise("m", precision)
  parts.push({ text: "-", precise: monthPrecise })
  if (monthPrecise) {
    parts.push({ text: parsed.month, precise: true })
  } else {
    parts.push({ text: "00", precise: false })
  }

  // Day.
  const dayPrecise = isPrecise("d", precision)
  parts.push({ text: "-", precise: dayPrecise })
  if (dayPrecise) {
    parts.push({ text: parsed.day, precise: true })
  } else {
    parts.push({ text: "00", precise: false })
  }

  // Time components.
  if (precisionIndex >= getPrecisionIndex("h")) {
    const hourPrecise = isPrecise("h", precision)
    parts.push({ text: " ", precise: true })
    if (hourPrecise) {
      parts.push({ text: parsed.hour, precise: true })
    } else {
      parts.push({ text: "00", precise: false })
    }

    // Minutes.
    const minPrecise = isPrecise("min", precision)
    parts.push({ text: ":", precise: minPrecise })
    if (minPrecise) {
      parts.push({ text: parsed.minute, precise: true })
    } else {
      parts.push({ text: "00", precise: false })
    }

    // Seconds.
    if (precisionIndex >= getPrecisionIndex("s")) {
      const secPrecise = isPrecise("s", precision)
      parts.push({ text: ":", precise: secPrecise })
      if (secPrecise) {
        parts.push({ text: parsed.second, precise: true })
      } else {
        parts.push({ text: "00", precise: false })
      }
    }
  }

  return parts
}

/**
 * Calculates time difference in various units from milliseconds.
 */
export function calculateTimeUnits(diffMs: number): {
  seconds: number
  minutes: number
  hours: number
  days: number
  months: number
  years: number
  kiloYears: number
  megaYears: number
  gigaYears: number
} {
  const absDiff = Math.abs(diffMs)
  const seconds = Math.floor(absDiff / 1000)
  const minutes = Math.floor(seconds / 60)
  const hours = Math.floor(minutes / 60)
  const days = Math.floor(hours / 24)
  const months = Math.floor(days / 30)
  const years = Math.floor(days / 365)
  const kiloYears = Math.floor(years / 1000)
  const megaYears = Math.floor(years / 1_000_000)
  const gigaYears = Math.floor(years / 1_000_000_000)

  return { seconds, minutes, hours, days, months, years, kiloYears, megaYears, gigaYears }
}

/**
 * Determines which time unit to use for relative display and when to update.
 */
export function getRelativeTimeInfo(diffMs: number): {
  unit: "gigaYears" | "megaYears" | "kiloYears" | "years" | "months" | "days" | "hours" | "minutes" | "seconds"
  count: number
  isPast: boolean
  nextUpdateMs: number
} {
  const isPast = diffMs >= 0
  const units = calculateTimeUnits(diffMs)

  if (units.gigaYears >= 1) {
    return {
      unit: "gigaYears",
      count: units.gigaYears,
      isPast,
      nextUpdateMs: Number.MAX_SAFE_INTEGER,
    }
  }
  if (units.megaYears >= 1) {
    return {
      unit: "megaYears",
      count: units.megaYears,
      isPast,
      nextUpdateMs: Number.MAX_SAFE_INTEGER,
    }
  }
  if (units.kiloYears >= 1) {
    return {
      unit: "kiloYears",
      count: units.kiloYears,
      isPast,
      nextUpdateMs: Number.MAX_SAFE_INTEGER,
    }
  }
  if (units.years >= 1) {
    const remainingDays = units.days - units.years * 365
    return {
      unit: "years",
      count: units.years,
      isPast,
      nextUpdateMs: (365 - remainingDays) * 24 * 60 * 60 * 1000,
    }
  }
  if (units.months >= 1) {
    const remainingDays = units.days - units.months * 30
    return {
      unit: "months",
      count: units.months,
      isPast,
      nextUpdateMs: (30 - remainingDays) * 24 * 60 * 60 * 1000,
    }
  }
  if (units.days >= 1) {
    const remainingHours = units.hours - units.days * 24
    return {
      unit: "days",
      count: units.days,
      isPast,
      nextUpdateMs: (24 - remainingHours) * 60 * 60 * 1000,
    }
  }
  if (units.hours >= 1) {
    const remainingMinutes = units.minutes - units.hours * 60
    return {
      unit: "hours",
      count: units.hours,
      isPast,
      nextUpdateMs: (60 - remainingMinutes) * 60 * 1000,
    }
  }
  if (units.minutes >= 1) {
    const remainingSeconds = units.seconds - units.minutes * 60
    return {
      unit: "minutes",
      count: units.minutes,
      isPast,
      nextUpdateMs: (60 - remainingSeconds) * 1000,
    }
  }
  return {
    unit: "seconds",
    count: units.seconds,
    isPast,
    nextUpdateMs: 1000,
  }
}
</script>

<script setup lang="ts">
import { timestampToSeconds } from "@/utils"
import { computed,onBeforeUnmount,ref,watchEffect } from "vue"
import { useI18n } from "vue-i18n"
const props = withDefaults(
  defineProps<{
    // ISO timestamp string like "2025-03-02T00:00:00Z".
    timestamp: string
    // Precision of the timestamp.
    precision: TimePrecision
    // Initial display format: "absolute" or "relative".
    format?: "absolute" | "relative"
  }>(),
  {
    format: "absolute",
  },
)

const { t } = useI18n({ useScope: "global" })

// Current display format, can be toggled by the user.
const currentFormat = ref(props.format)

// Timer for reactive relative time updates.
let updateTimer: ReturnType<typeof setTimeout> | null = null

// Current time for relative calculations, updated reactively.
const now = ref(Date.now())

// Parse the timestamp to seconds (bigint).
const timestampSeconds = computed(() => {
  try {
    return timestampToSeconds(props.timestamp)
  } catch {
    return null
  }
})

// Parse timestamp components for absolute display.
const parsed = computed(() => {
  if (timestampSeconds.value === null) {
    return null
  }
  return parseTimestamp(props.timestamp)
})

// Format absolute time with grayed out imprecise parts.
const absoluteDisplay = computed(() => {
  if (!parsed.value) {
    return { parts: [] as DisplayTimePart[] }
  }
  return { parts: formatAbsoluteParts(parsed.value, props.precision) }
})

// Calculate relative time difference.
const relativeDiff = computed(() => {
  if (timestampSeconds.value === null) {
    return null
  }

  // Convert bigint seconds to milliseconds for comparison.
  const timestampMs = Number(timestampSeconds.value) * 1000
  const diffMs = now.value - timestampMs

  return diffMs
})

// Format relative time string.
const relativeDisplay = computed(() => {
  if (relativeDiff.value === null) {
    return { text: "", nextUpdateMs: 0 }
  }

  const info = getRelativeTimeInfo(relativeDiff.value)
  let text: string

  if (info.isPast) {
    switch (info.unit) {
      case "gigaYears":
        text = t("components.TimeDisplay.relative.gigaYearsAgo", { count: info.count })
        break
      case "megaYears":
        text = t("components.TimeDisplay.relative.megaYearsAgo", { count: info.count })
        break
      case "kiloYears":
        text = t("components.TimeDisplay.relative.kiloYearsAgo", { count: info.count })
        break
      case "years":
        text = t("components.TimeDisplay.relative.yearsAgo", { count: info.count })
        break
      case "months":
        text = t("components.TimeDisplay.relative.monthsAgo", { count: info.count })
        break
      case "days":
        text = t("components.TimeDisplay.relative.daysAgo", { count: info.count })
        break
      case "hours":
        text = t("components.TimeDisplay.relative.hoursAgo", { count: info.count })
        break
      case "minutes":
        text = t("components.TimeDisplay.relative.minutesAgo", { count: info.count })
        break
      case "seconds":
        text = t("components.TimeDisplay.relative.secondsAgo", { count: info.count })
        break
    }
  } else {
    switch (info.unit) {
      case "gigaYears":
        text = t("components.TimeDisplay.relative.inGigaYears", { count: info.count })
        break
      case "megaYears":
        text = t("components.TimeDisplay.relative.inMegaYears", { count: info.count })
        break
      case "kiloYears":
        text = t("components.TimeDisplay.relative.inKiloYears", { count: info.count })
        break
      case "years":
        text = t("components.TimeDisplay.relative.inYears", { count: info.count })
        break
      case "months":
        text = t("components.TimeDisplay.relative.inMonths", { count: info.count })
        break
      case "days":
        text = t("components.TimeDisplay.relative.inDays", { count: info.count })
        break
      case "hours":
        text = t("components.TimeDisplay.relative.inHours", { count: info.count })
        break
      case "minutes":
        text = t("components.TimeDisplay.relative.inMinutes", { count: info.count })
        break
      case "seconds":
        text = t("components.TimeDisplay.relative.inSeconds", { count: info.count })
        break
    }
  }

  return { text, nextUpdateMs: info.nextUpdateMs }
})

// Toggle between formats.
function toggleFormat() {
  currentFormat.value = currentFormat.value === "absolute" ? "relative" : "absolute"
}

// Schedule the next update for relative time.
function scheduleUpdate() {
  if (updateTimer !== null) {
    clearTimeout(updateTimer)
    updateTimer = null
  }

  if (currentFormat.value !== "relative") {
    return
  }

  const nextMs = relativeDisplay.value?.nextUpdateMs
  if (nextMs && nextMs > 0 && nextMs < Number.MAX_SAFE_INTEGER) {
    // Cap the timeout to a reasonable value (1 hour max to handle drift).
    const timeout = Math.min(nextMs, 60 * 60 * 1000)
    updateTimer = setTimeout(() => {
      now.value = Date.now()
    }, timeout)
  }
}

// Watch for format changes and schedule updates.
watchEffect(() => {
  scheduleUpdate()
})

// Clean up timer on unmount.
onBeforeUnmount(() => {
  if (updateTimer !== null) {
    clearTimeout(updateTimer)
    updateTimer = null
  }
})

// Compute the tooltip based on current format.
const tooltip = computed(() => {
  if (currentFormat.value === "absolute") {
    return relativeDisplay.value?.text ?? ""
  } else {
    // For absolute tooltip, format it nicely.
    if (!parsed.value) {
      return props.timestamp
    }
    const p = parsed.value
    const precisionIndex = getPrecisionIndex(props.precision)
    let result = p.year
    if (precisionIndex >= getPrecisionIndex("m")) {
      result += `-${p.month}`
    }
    if (precisionIndex >= getPrecisionIndex("d")) {
      result += `-${p.day}`
    }
    if (precisionIndex >= getPrecisionIndex("h")) {
      result += ` ${p.hour}`
    }
    if (precisionIndex >= getPrecisionIndex("min")) {
      result += `:${p.minute}`
    }
    if (precisionIndex >= getPrecisionIndex("s")) {
      result += `:${p.second}`
    }
    return result
  }
})
</script>

<template>
  <span class="cursor-pointer" :title="tooltip" @click="toggleFormat">
    <template v-if="currentFormat === 'absolute'">
      <span v-for="(part, index) in absoluteDisplay.parts" :key="index" :class="{ 'text-neutral-400': !part.precise }">{{ part.text }}</span>
    </template>
    <template v-else>
      {{ relativeDisplay.text }}
    </template>
  </span>
</template>
