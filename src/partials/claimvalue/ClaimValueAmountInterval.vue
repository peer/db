<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { AmountIntervalClaim } from "@/document"

import { useI18n } from "vue-i18n"

defineProps<{
  claim: AmountIntervalClaim | DeepReadonly<AmountIntervalClaim> | null
}>()

const { t } = useI18n({ useScope: "global" })
</script>

<template>
  <template v-if="claim">
    <template v-if="claim.from">{{ claim.from }}</template>
    <template v-else-if="claim.fromIsUnknown">{{ t("common.values.unknown") }}</template>
    <template v-else-if="claim.fromIsNone">{{ t("common.values.none") }}</template>
    –
    <template v-if="claim.to">{{ claim.to }}</template>
    <template v-else-if="claim.toIsUnknown">{{ t("common.values.unknown") }}</template>
    <template v-else-if="claim.toIsNone">{{ t("common.values.none") }}</template>
  </template>
</template>
