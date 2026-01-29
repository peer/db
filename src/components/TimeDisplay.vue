<script setup lang="ts">
import type { TimePrecision } from "@/types"

import { computed, onBeforeUnmount, ref, watchEffect } from "vue"
import { useI18n } from "vue-i18n"

import { timestampToSeconds } from "@/utils"

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

const { t, n } = useI18n({ useScope: "global" })

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
  const match = /^([+-]?\d{4,})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})Z$/.exec(props.timestamp)
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
})

// Precision hierarchy for determining what parts are precise.
const precisionLevels = ["G", "100M", "10M", "M", "100k", "10k", "k", "100y", "10y", "y", "m", "d", "h", "min", "s"] as const
const precisionIndex = computed(() => precisionLevels.indexOf(props.precision))

// Check if a specific level is precise (i.e., included in the precision).
function isPrecise(level: TimePrecision): boolean {
  const levelIndex = precisionLevels.indexOf(level)
  return levelIndex <= precisionIndex.value
}

// Format absolute time with grayed out imprecise parts.
const absoluteDisplay = computed(() => {
  if (!parsed.value) {
    return { parts: [], tooltip: "" }
  }

  const p = parsed.value
  const parts: Array<{ text: string; precise: boolean }> = []

  // Year is always shown.
  // For year-level precisions (G to y), we show only the year.
  // For less precise years, we gray out trailing zeros.
  const yearPrecise = isPrecise("y")

  // Determine how many trailing zeros in the year are imprecise.
  const yearStr = p.year
  let preciseYearPart = yearStr
  let impreciseYearPart = ""

  if (!yearPrecise) {
    // For precisions less than "y", we need to gray out trailing zeros.
    const factors: Record<string, number> = {
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
    const trailingZeros = factors[props.precision] ?? 0
    const absYear = yearStr.startsWith("-") ? yearStr.slice(1) : yearStr
    const sign = yearStr.startsWith("-") ? "-" : ""

    if (trailingZeros > 0 && absYear.length > trailingZeros) {
      const preciseLen = absYear.length - trailingZeros
      preciseYearPart = sign + absYear.slice(0, preciseLen)
      impreciseYearPart = absYear.slice(preciseLen)
    } else {
      preciseYearPart = yearStr
    }
  }

  parts.push({ text: preciseYearPart, precise: true })
  if (impreciseYearPart) {
    parts.push({ text: impreciseYearPart, precise: false })
  }

  // For year-only precisions, stop here.
  if (precisionIndex.value <= precisionLevels.indexOf("y")) {
    return { parts, tooltip: formatTooltip() }
  }

  // Month.
  const monthPrecise = isPrecise("m")
  parts.push({ text: "-", precise: monthPrecise })
  if (monthPrecise) {
    parts.push({ text: p.month, precise: true })
  } else {
    parts.push({ text: "00", precise: false })
  }

  // Day.
  const dayPrecise = isPrecise("d")
  parts.push({ text: "-", precise: dayPrecise })
  if (dayPrecise) {
    parts.push({ text: p.day, precise: true })
  } else {
    parts.push({ text: "00", precise: false })
  }

  // Time components (only if precision includes time).
  if (precisionIndex.value >= precisionLevels.indexOf("h")) {
    const hourPrecise = isPrecise("h")
    parts.push({ text: " ", precise: true })
    if (hourPrecise) {
      parts.push({ text: p.hour, precise: true })
    } else {
      parts.push({ text: "00", precise: false })
    }

    // Minutes.
    const minPrecise = isPrecise("min")
    parts.push({ text: ":", precise: minPrecise })
    if (minPrecise) {
      parts.push({ text: p.minute, precise: true })
    } else {
      parts.push({ text: "00", precise: false })
    }

    // Seconds (only if precision is "s").
    if (precisionIndex.value >= precisionLevels.indexOf("s")) {
      const secPrecise = isPrecise("s")
      parts.push({ text: ":", precise: secPrecise })
      if (secPrecise) {
        parts.push({ text: p.second, precise: true })
      } else {
        parts.push({ text: "00", precise: false })
      }
    }
  }

  return { parts, tooltip: formatTooltip() }
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
    return { text: "", tooltip: "" }
  }

  const diffMs = relativeDiff.value
  const absDiff = Math.abs(diffMs)
  const isPast = diffMs >= 0

  // Calculate time units.
  const seconds = Math.floor(absDiff / 1000)
  const minutes = Math.floor(seconds / 60)
  const hours = Math.floor(minutes / 60)
  const days = Math.floor(hours / 24)
  const months = Math.floor(days / 30)
  const years = Math.floor(days / 365)

  // For very large time spans (beyond year precision).
  const kiloYears = Math.floor(years / 1000)
  const megaYears = Math.floor(years / 1_000_000)
  const gigaYears = Math.floor(years / 1_000_000_000)

  let text: string
  let nextUpdateMs: number

  if (gigaYears >= 1) {
    text = isPast ? t("components.TimeDisplay.relative.gigaYearsAgo", { count: gigaYears }) : t("components.TimeDisplay.relative.inGigaYears", { count: gigaYears })
    nextUpdateMs = 365 * 24 * 60 * 60 * 1000 * 1_000_000_000 // Essentially never update.
  } else if (megaYears >= 1) {
    text = isPast ? t("components.TimeDisplay.relative.megaYearsAgo", { count: megaYears }) : t("components.TimeDisplay.relative.inMegaYears", { count: megaYears })
    nextUpdateMs = 365 * 24 * 60 * 60 * 1000 * 1_000_000
  } else if (kiloYears >= 1) {
    text = isPast ? t("components.TimeDisplay.relative.kiloYearsAgo", { count: kiloYears }) : t("components.TimeDisplay.relative.inKiloYears", { count: kiloYears })
    nextUpdateMs = 365 * 24 * 60 * 60 * 1000 * 1000
  } else if (years >= 1) {
    text = isPast ? t("components.TimeDisplay.relative.yearsAgo", { count: years }) : t("components.TimeDisplay.relative.inYears", { count: years })
    // Update when we cross to the next year.
    const remainingDays = days - years * 365
    nextUpdateMs = (365 - remainingDays) * 24 * 60 * 60 * 1000
  } else if (months >= 1) {
    text = isPast ? t("components.TimeDisplay.relative.monthsAgo", { count: months }) : t("components.TimeDisplay.relative.inMonths", { count: months })
    // Update when we cross to the next month.
    const remainingDays = days - months * 30
    nextUpdateMs = (30 - remainingDays) * 24 * 60 * 60 * 1000
  } else if (days >= 1) {
    text = isPast ? t("components.TimeDisplay.relative.daysAgo", { count: days }) : t("components.TimeDisplay.relative.inDays", { count: days })
    // Update when we cross to the next day.
    const remainingHours = hours - days * 24
    nextUpdateMs = (24 - remainingHours) * 60 * 60 * 1000
  } else if (hours >= 1) {
    text = isPast ? t("components.TimeDisplay.relative.hoursAgo", { count: hours }) : t("components.TimeDisplay.relative.inHours", { count: hours })
    // Update when we cross to the next hour.
    const remainingMinutes = minutes - hours * 60
    nextUpdateMs = (60 - remainingMinutes) * 60 * 1000
  } else if (minutes >= 1) {
    text = isPast ? t("components.TimeDisplay.relative.minutesAgo", { count: minutes }) : t("components.TimeDisplay.relative.inMinutes", { count: minutes })
    // Update when we cross to the next minute.
    const remainingSeconds = seconds - minutes * 60
    nextUpdateMs = (60 - remainingSeconds) * 1000
  } else {
    text = isPast ? t("components.TimeDisplay.relative.secondsAgo", { count: seconds }) : t("components.TimeDisplay.relative.inSeconds", { count: seconds })
    // Update every second.
    nextUpdateMs = 1000
  }

  return { text, nextUpdateMs, tooltip: formatTooltip() }
})

// Format tooltip showing the other format.
function formatTooltip(): string {
  if (currentFormat.value === "absolute") {
    // Show relative time in tooltip.
    return relativeDisplay.value?.text ?? ""
  } else {
    // Show absolute time in tooltip.
    return props.timestamp
  }
}

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
    let result = p.year
    if (precisionIndex.value >= precisionLevels.indexOf("m")) {
      result += `-${p.month}`
    }
    if (precisionIndex.value >= precisionLevels.indexOf("d")) {
      result += `-${p.day}`
    }
    if (precisionIndex.value >= precisionLevels.indexOf("h")) {
      result += ` ${p.hour}`
    }
    if (precisionIndex.value >= precisionLevels.indexOf("min")) {
      result += `:${p.minute}`
    }
    if (precisionIndex.value >= precisionLevels.indexOf("s")) {
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
