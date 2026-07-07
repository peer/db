<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { TimeIntervalClaim } from "@/document"

import { computed } from "vue"
import { useI18n } from "vue-i18n"

import { IN_LOCATION } from "@/core"
import { getBestClaimOfType } from "@/document"
import TimeDisplay from "@/partials/TimeDisplay.vue"

const props = withDefaults(
  defineProps<{
    claim: TimeIntervalClaim | DeepReadonly<TimeIntervalClaim> | null
    // Passed through to TimeDisplay: format with Intl.DateTimeFormat in the current UI language.
    localized?: boolean
  }>(),
  {
    localized: false,
  },
)

const { t } = useI18n({ useScope: "global" })

// The IANA timezone both interval endpoints are in, from an IN_LOCATION sub claim. Without one
// they are in UTC.
const location = computed(() => (props.claim ? getBestClaimOfType(props.claim.sub, "id", IN_LOCATION)?.value : undefined))
</script>

<template>
  <template v-if="claim">
    <TimeDisplay v-if="claim.from" :timestamp="claim.from" :precision="claim.fromPrecision!" :localized="localized" :location="location" />
    <template v-else-if="claim.fromIsUnknown">{{ t("common.values.unknown") }}</template>
    <template v-else-if="claim.fromIsNone">{{ t("common.values.none") }}</template>
    –
    <TimeDisplay v-if="claim.to" :timestamp="claim.to" :precision="claim.toPrecision!" :localized="localized" :location="location" />
    <template v-else-if="claim.toIsUnknown">{{ t("common.values.unknown") }}</template>
    <template v-else-if="claim.toIsNone">{{ t("common.values.none") }}</template>
  </template>
</template>
