<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { ClaimTypes } from "@/document"

import { asyncToReactive, getDisplayLabel, getError, isLoading } from "@/utils"
import { useI18n } from "vue-i18n"

const props = defineProps<{
  claims?: DeepReadonly<ClaimTypes> | null
}>()

const { t, locale } = useI18n({ useScope: "global" })

const displayLabel = asyncToReactive(() => getDisplayLabel(props.claims, locale.value))
</script>

<template>
  <template v-if="isLoading(displayLabel)"><!-- TODO: What to show here? --></template>
  <i v-else-if="getError(displayLabel)" class="pd-displaylabel-error text-error-600">{{ t("common.status.error") }}</i>
  <template v-else-if="displayLabel">{{ displayLabel }}</template>
  <template v-else
    ><i>{{ t("common.values.noName") }}</i></template
  >
</template>
