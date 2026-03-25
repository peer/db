<script setup lang="ts" generic="T extends ClaimTypeName">
import type { DeepReadonly } from "vue"

import type { ClaimForType, ClaimTypeName } from "@/document"

import ClaimValueAmount from "@/partials/claimvalue/ClaimValueAmount.vue"
import ClaimValueAmountInterval from "@/partials/claimvalue/ClaimValueAmountInterval.vue"
import ClaimValueHas from "@/partials/claimvalue/ClaimValueHas.vue"
import ClaimValueHtml from "@/partials/claimvalue/ClaimValueHtml.vue"
import ClaimValueId from "@/partials/claimvalue/ClaimValueId.vue"
import ClaimValueLink from "@/partials/claimvalue/ClaimValueLink.vue"
import ClaimValueNone from "@/partials/claimvalue/ClaimValueNone.vue"
import ClaimValueRef from "@/partials/claimvalue/ClaimValueRef.vue"
import ClaimValueString from "@/partials/claimvalue/ClaimValueString.vue"
import ClaimValueTime from "@/partials/claimvalue/ClaimValueTime.vue"
import ClaimValueTimeInterval from "@/partials/claimvalue/ClaimValueTimeInterval.vue"
import ClaimValueUnknown from "@/partials/claimvalue/ClaimValueUnknown.vue"

defineProps<{
  claim: ClaimForType<T> | DeepReadonly<ClaimForType<T>> | null
  type: T
}>()
</script>

<template>
  <ClaimValueId v-if="type === 'id'" :claim="claim as ClaimForType<'id'>" />
  <ClaimValueString v-else-if="type === 'string'" :claim="claim as ClaimForType<'string'>" />
  <ClaimValueHtml v-else-if="type === 'html'" :claim="claim as ClaimForType<'html'>" />
  <ClaimValueAmount v-else-if="type === 'amount'" :claim="claim as ClaimForType<'amount'>" />
  <ClaimValueAmountInterval v-else-if="type === 'amountInterval'" :claim="claim as ClaimForType<'amountInterval'>" />
  <ClaimValueTime v-else-if="type === 'time'" :claim="claim as ClaimForType<'time'>" />
  <ClaimValueTimeInterval v-else-if="type === 'timeInterval'" :claim="claim as ClaimForType<'timeInterval'>" />
  <ClaimValueLink v-else-if="type === 'link'" :claim="claim as ClaimForType<'link'>" />
  <ClaimValueRef v-else-if="type === 'ref'" :claim="claim as ClaimForType<'ref'>" />
  <ClaimValueHas v-else-if="type === 'has'" :claim="claim as ClaimForType<'has'>" />
  <ClaimValueNone v-else-if="type === 'none'" :claim="claim as ClaimForType<'none'>" />
  <ClaimValueUnknown v-else-if="type === 'unknown'" :claim="claim as ClaimForType<'unknown'>" />
</template>
