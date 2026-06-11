<script setup lang="ts">
import type { TimePrecision } from "@/document"

import { computed, onBeforeUnmount, ref, watchEffect } from "vue"
import { useI18n } from "vue-i18n"

import { timeFloat64 } from "@/document/time"
import { formatAbsoluteParts, getRelativeTimeInfo, parseTimestamp } from "@/partials/TimeDisplay.utils"

const props = withDefaults(
  defineProps<{
    // Time string in the claim Time format (see src/document/types.d.ts).
    timestamp: string
    // Precision of the timestamp.
    precision: TimePrecision
    // Initial display format: "absolute" or "relative".
    format?: "absolute" | "relative"
    // Whether clicking toggles between the formats. Disable when the display is wrapped
    // in another clickable element (e.g., a label) which should receive the click.
    toggle?: boolean
  }>(),
  {
    format: "absolute",
    toggle: true,
  },
)

const { t } = useI18n({ useScope: "global" })

// Current display format, can be toggled by the user.
const currentFormat = ref(props.format)

// Timer for reactive relative time updates.
let updateTimer: ReturnType<typeof setTimeout> | null = null

// Current time for relative calculations, updated reactively.
const now = ref(Date.now())

// Parse the timestamp to float64 seconds since the Unix epoch.
const timestampSeconds = computed(() => {
  try {
    return timeFloat64(props.timestamp, 0)
  } catch {
    return null
  }
})

// Parse timestamp components for absolute display.
const parsed = computed(() => parseTimestamp(props.timestamp))

// Format absolute time with grayed out imprecise parts.
const absoluteDisplay = computed(() => {
  if (!parsed.value) {
    return { parts: [] }
  }
  return { parts: formatAbsoluteParts(parsed.value, props.precision) }
})

// Calculate relative time difference.
const relativeDiff = computed(() => {
  if (timestampSeconds.value === null) {
    return null
  }
  return now.value - timestampSeconds.value * 1000
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
        text = t("partials.TimeDisplay.relative.gigaYearsAgo", { count: info.count })
        break
      case "megaYears":
        text = t("partials.TimeDisplay.relative.megaYearsAgo", { count: info.count })
        break
      case "kiloYears":
        text = t("partials.TimeDisplay.relative.kiloYearsAgo", { count: info.count })
        break
      case "years":
        text = t("partials.TimeDisplay.relative.yearsAgo", { count: info.count })
        break
      case "months":
        text = t("partials.TimeDisplay.relative.monthsAgo", { count: info.count })
        break
      case "days":
        text = t("partials.TimeDisplay.relative.daysAgo", { count: info.count })
        break
      case "hours":
        text = t("partials.TimeDisplay.relative.hoursAgo", { count: info.count })
        break
      case "minutes":
        text = t("partials.TimeDisplay.relative.minutesAgo", { count: info.count })
        break
      case "seconds":
        text = t("partials.TimeDisplay.relative.secondsAgo", { count: info.count })
        break
    }
  } else {
    switch (info.unit) {
      case "gigaYears":
        text = t("partials.TimeDisplay.relative.inGigaYears", { count: info.count })
        break
      case "megaYears":
        text = t("partials.TimeDisplay.relative.inMegaYears", { count: info.count })
        break
      case "kiloYears":
        text = t("partials.TimeDisplay.relative.inKiloYears", { count: info.count })
        break
      case "years":
        text = t("partials.TimeDisplay.relative.inYears", { count: info.count })
        break
      case "months":
        text = t("partials.TimeDisplay.relative.inMonths", { count: info.count })
        break
      case "days":
        text = t("partials.TimeDisplay.relative.inDays", { count: info.count })
        break
      case "hours":
        text = t("partials.TimeDisplay.relative.inHours", { count: info.count })
        break
      case "minutes":
        text = t("partials.TimeDisplay.relative.inMinutes", { count: info.count })
        break
      case "seconds":
        text = t("partials.TimeDisplay.relative.inSeconds", { count: info.count })
        break
    }
  }

  return { text, nextUpdateMs: info.nextUpdateMs }
})

// Toggle between formats.
function toggleFormat() {
  if (!props.toggle) {
    return
  }
  currentFormat.value = currentFormat.value === "absolute" ? "relative" : "absolute"
}

// In absolute mode the tooltip shows the relative phrasing computed from now,
// but now is only ticked by the relative-mode timer / visibility handler. Refresh
// it on hover so the tooltip shown to the user is current rather than stale.
function refreshNowForTooltip() {
  if (currentFormat.value === "absolute") {
    now.value = Date.now()
  }
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

// When the tab returns to the foreground refresh now immediately: browsers
// throttle setTimeout on hidden tabs, so the next scheduled tick could be
// arbitrarily late, leaving the relative phrasing stale.
function onVisibilityChange() {
  if (document.visibilityState === "visible") {
    now.value = Date.now()
  }
}
document.addEventListener("visibilitychange", onVisibilityChange)

// Clean up timer and listener on unmount.
onBeforeUnmount(() => {
  document.removeEventListener("visibilitychange", onVisibilityChange)
  if (updateTimer !== null) {
    clearTimeout(updateTimer)
    updateTimer = null
  }
})

// Compute the tooltip based on current format. In absolute mode the tooltip
// is the relative phrasing; in relative mode it's a compact rendering of the
// timestamp at the current precision.
const tooltip = computed(() => {
  if (currentFormat.value === "absolute") {
    return relativeDisplay.value?.text ?? ""
  }
  if (!parsed.value) {
    return props.timestamp
  }
  return absoluteDisplay.value.parts.map((p) => p.text).join("")
})
</script>

<template>
  <span :class="{ 'cursor-pointer': toggle }" :title="tooltip" @click="toggleFormat" @mouseenter="refreshNowForTooltip">
    <template v-if="currentFormat === 'absolute'">
      <span v-for="(part, index) in absoluteDisplay.parts" :key="index" :class="{ 'text-neutral-400': !part.precise }">{{ part.text }}</span>
    </template>
    <template v-else>
      {{ relativeDisplay.text }}
    </template>
  </span>
</template>
