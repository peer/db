<script setup lang="ts" generic="T extends ClaimTypeName">
import type { DeepReadonly } from "vue"

import type { ClaimForType, ClaimTypeName } from "@/document"

import ClaimValueAmount from "@/partials/claimvalue/ClaimValueAmount.vue"
import ClaimValueAmountRange from "@/partials/claimvalue/ClaimValueAmountRange.vue"
import ClaimValueFile from "@/partials/claimvalue/ClaimValueFile.vue"
import ClaimValueId from "@/partials/claimvalue/ClaimValueId.vue"
import ClaimValueNone from "@/partials/claimvalue/ClaimValueNone.vue"
import ClaimValueRef from "@/partials/claimvalue/ClaimValueRef.vue"
import ClaimValueRel from "@/partials/claimvalue/ClaimValueRel.vue"
import ClaimValueString from "@/partials/claimvalue/ClaimValueString.vue"
import ClaimValueText from "@/partials/claimvalue/ClaimValueText.vue"
import ClaimValueTime from "@/partials/claimvalue/ClaimValueTime.vue"
import ClaimValueTimeRange from "@/partials/claimvalue/ClaimValueTimeRange.vue"
import ClaimValueUnknown from "@/partials/claimvalue/ClaimValueUnknown.vue"

defineProps<{
  claim: ClaimForType<T> | DeepReadonly<ClaimForType<T>> | null
  type: T
}>()
</script>

<template>
  <ClaimValueId v-if="type === 'id'" :claim="claim as ClaimForType<'id'>" />
  <ClaimValueRef v-else-if="type === 'ref'" :claim="claim as ClaimForType<'ref'>" />
  <ClaimValueText v-else-if="type === 'text'" :claim="claim as ClaimForType<'text'>" />
  <ClaimValueString v-else-if="type === 'string'" :claim="claim as ClaimForType<'string'>" />
  <ClaimValueAmount v-else-if="type === 'amount'" :claim="claim as ClaimForType<'amount'>" />
  <ClaimValueAmountRange v-else-if="type === 'amountRange'" :claim="claim as ClaimForType<'amountRange'>" />
  <ClaimValueRel v-else-if="type === 'rel'" :claim="claim as ClaimForType<'rel'>" />
  <ClaimValueFile v-else-if="type === 'file'" :claim="claim as ClaimForType<'file'>" />
  <ClaimValueNone v-else-if="type === 'none'" :claim="claim as ClaimForType<'none'>" />
  <ClaimValueUnknown v-else-if="type === 'unknown'" :claim="claim as ClaimForType<'unknown'>" />
  <ClaimValueTime v-else-if="type === 'time'" :claim="claim as ClaimForType<'time'>" />
  <ClaimValueTimeRange v-else-if="type === 'timeRange'" :claim="claim as ClaimForType<'timeRange'>" />
</template>
