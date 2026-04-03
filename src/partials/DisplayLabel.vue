<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { ClaimTypes } from "@/document"

import { onBeforeUnmount } from "vue"
import { useI18n } from "vue-i18n"
import { useRouter } from "vue-router"

import { injectProgress } from "@/progress"
import { asyncToReactive, getDisplayLabel, getError, isLoading } from "@/utils"

const props = defineProps<{
  claims?: DeepReadonly<ClaimTypes> | null
}>()

const router = useRouter()
const i18n = useI18n({ useScope: "global" })
const { t } = i18n

const progress = injectProgress()

let abortController = new AbortController()

onBeforeUnmount(() => abortController.abort())

// TODO: Pass "el" in.
const displayLabel = asyncToReactive(() => getDisplayLabel(props.claims, router, i18n, null, abortController.signal, progress))
</script>

<template>
  <template v-if="isLoading(displayLabel)"><!-- TODO: What to show here? --></template>
  <i v-else-if="getError(displayLabel)" class="pd-displaylabel-error text-error-600">{{ t("common.status.error") }}</i>
  <template v-else-if="displayLabel">{{ displayLabel }}</template>
  <template v-else
    ><i>{{ t("common.values.noName") }}</i></template
  >
</template>
