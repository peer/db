<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { TimeClaim } from "@/document"

import { computed } from "vue"

import { IN_LOCATION } from "@/core"
import { getBestClaimOfType } from "@/document"
import TimeDisplay from "@/partials/TimeDisplay.vue"

const props = withDefaults(
  defineProps<{
    claim: TimeClaim | DeepReadonly<TimeClaim> | null
    // Passed through to TimeDisplay. When unset, TimeDisplay falls back to the site's localizedTimeDisplay feature.
    localized?: boolean
  }>(),
  {
    localized: undefined,
  },
)

// The IANA timezone the timestamp is in, from an IN_LOCATION sub claim. Without one it is in UTC.
const location = computed(() => (props.claim ? getBestClaimOfType(props.claim.sub, "id", IN_LOCATION)?.value : undefined))
</script>

<template>
  <TimeDisplay v-if="claim" :timestamp="claim.time" :precision="claim.precision" :localized="localized" :location="location" />
</template>
