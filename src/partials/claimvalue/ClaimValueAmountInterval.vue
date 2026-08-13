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
    <template v-if="claim.from"
      ><span class="pd-claimvalueamountinterval-from">{{ claim.from }}</span></template
    >
    <template v-else-if="claim.fromIsUnknown"
      ><span class="pd-claimvalueamountinterval-from">{{ t("common.values.unknown") }}</span></template
    >
    <template v-else-if="claim.fromIsNone"
      ><span class="pd-claimvalueamountinterval-from">{{ t("common.values.none") }}</span></template
    >
    –
    <template v-if="claim.to"
      ><span class="pd-claimvalueamountinterval-to">{{ claim.to }}</span></template
    >
    <template v-else-if="claim.toIsUnknown"
      ><span class="pd-claimvalueamountinterval-to">{{ t("common.values.unknown") }}</span></template
    >
    <template v-else-if="claim.toIsNone"
      ><span class="pd-claimvalueamountinterval-to">{{ t("common.values.none") }}</span></template
    >
  </template>
</template>
