<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { D } from "@/document"

import { onBeforeUnmount } from "vue"
import { useI18n } from "vue-i18n"
import { useRouter } from "vue-router"

import { useProgress } from "@/progress"
import { asyncToReactive, getDisplayLabel, getError, isLoading, loadingWidth } from "@/utils"

const props = defineProps<{
  doc?: DeepReadonly<D> | null
}>()

const router = useRouter()
const i18n = useI18n({ useScope: "global" })
const { t } = i18n

// Data loading only, no controls.
const progress = useProgress()

let abortController = new AbortController()

onBeforeUnmount(() => abortController.abort())

// TODO: Pass "el" in.
const displayLabel = asyncToReactive(() => getDisplayLabel(props.doc?.claims, router, i18n, null, abortController.signal, progress))

defineExpose({
  displayLabel,
})
</script>

<template>
  <template v-if="isLoading(displayLabel)"
    ><div
      v-if="doc"
      class="pd-displaylabel-loading inline-block h-2 rounded-sm bg-slate-200 motion-safe:animate-pulse"
      :class="[loadingWidth(doc.id)]"
      aria-hidden="true"
  /></template>
  <i v-else-if="getError(displayLabel)" class="pd-displaylabel-error text-error-600">{{ t("common.status.error") }}</i>
  <template v-else-if="displayLabel">{{ displayLabel }}</template>
  <template v-else
    ><i>{{ t("common.values.noName") }}</i></template
  >
</template>
