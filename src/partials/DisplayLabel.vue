<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { D } from "@/document"

import { onBeforeUnmount } from "vue"
import { useI18n } from "vue-i18n"
import { useRouter } from "vue-router"

import { useProgress } from "@/progress"
import { asyncToReactive, capitalizeFirstLetter, getDisplayLabel, getError, isLoading, loadingWidth } from "@/utils"

const props = defineProps<{
  doc?: DeepReadonly<D> | null
  // When set, the first letter of the resolved label is upper-cased. It is a no-op for the loading, error, and
  // no-name states. This is done in JavaScript on the resolved string rather than with the CSS first-letter pseudo
  // element, which does not reliably apply in Firefox because Vue also injects empty text fragments.
  // See: https://github.com/orgs/vuejs/discussions/15055
  capitalize?: boolean
}>()

const router = useRouter()
const i18n = useI18n({ useScope: "global" })
const { t, locale } = i18n

function capitalizeFirst(label: unknown): unknown {
  if (!props.capitalize || typeof label !== "string") {
    return label
  }
  return capitalizeFirstLetter(label, locale.value)
}

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
  <template v-else-if="displayLabel">{{ capitalizeFirst(displayLabel) }}</template>
  <template v-else
    ><i>{{ t("common.values.noName") }}</i></template
  >
</template>
