<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { TimeIntervalClaim } from "@/document"

import { useI18n } from "vue-i18n"

import TimeDisplay from "@/partials/TimeDisplay.vue"

defineProps<{
  claim: TimeIntervalClaim | DeepReadonly<TimeIntervalClaim> | null
}>()

const { t } = useI18n({ useScope: "global" })
</script>

<template>
  <template v-if="claim">
    <TimeDisplay v-if="claim.from" :timestamp="claim.from" :precision="claim.fromPrecision!" />
    <template v-else-if="claim.fromIsUnknown">{{ t("common.values.unknown") }}</template>
    <template v-else-if="claim.fromIsNone">{{ t("common.values.none") }}</template>
    –
    <TimeDisplay v-if="claim.to" :timestamp="claim.to" :precision="claim.toPrecision!" />
    <template v-else-if="claim.toIsUnknown">{{ t("common.values.unknown") }}</template>
    <template v-else-if="claim.toIsNone">{{ t("common.values.none") }}</template>
  </template>
</template>
