<script setup lang="ts" generic="T extends ClaimTypeProp">
import type { DeepReadonly } from "vue"

import type { ClaimTypeProp, ClaimForType } from "@/document"

import Id from "@/partials/claimvalue/Id.vue"
import Ref from "@/partials/claimvalue/Ref.vue"
import Text from "@/partials/claimvalue/Text.vue"
import String from "@/partials/claimvalue/String.vue"
import Amount from "@/partials/claimvalue/Amount.vue"
import AmountRange from "@/partials/claimvalue/AmountRange.vue"
import Rel from "@/partials/claimvalue/Rel.vue"
import File from "@/partials/claimvalue/File.vue"
import None from "@/partials/claimvalue/None.vue"
import Unknown from "@/partials/claimvalue/Unknown.vue"
import Time from "@/partials/claimvalue/Time.vue"
import TimeRange from "@/partials/claimvalue/TimeRange.vue"

defineProps<{
  claim: ClaimForType<T> | DeepReadonly<ClaimForType<T>> | null
  type: T
}>()
</script>

<template>
  <Id v-if="type === 'id'" :claim="claim as ClaimForType<'id'>" />
  <Ref v-else-if="type === 'ref'" :claim="claim as ClaimForType<'ref'>" />
  <Text v-else-if="type === 'text'" :claim="claim as ClaimForType<'text'>" />
  <String v-else-if="type === 'string'" :claim="claim as ClaimForType<'string'>" />
  <Amount v-else-if="type === 'amount'" :claim="claim as ClaimForType<'amount'>" />
  <AmountRange v-else-if="type === 'amountRange'" :claim="claim as ClaimForType<'amountRange'>" />
  <Rel v-else-if="type === 'rel'" :claim="claim as ClaimForType<'rel'>" />
  <File v-else-if="type === 'file'" :claim="claim as ClaimForType<'file'>" />
  <None v-else-if="type === 'none'" :claim="claim as ClaimForType<'none'>" />
  <Unknown v-else-if="type === 'unknown'" :claim="claim as ClaimForType<'unknown'>" />
  <Time v-else-if="type === 'time'" :claim="claim as ClaimForType<'time'>" />
  <TimeRange v-else-if="type === 'timeRange'" :claim="claim as ClaimForType<'timeRange'>" />
</template>
