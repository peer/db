<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { Claims } from "@/document"

import { computed } from "vue"

import { asyncToReactive, getDisplayLabel } from "@/utils"
import { useI18n } from "vue-i18n"

const props = defineProps<{
  claims?: DeepReadonly<Claims> | null
}>()

const { t, locale } = useI18n({ useScope: "global" })

const displayLabel = asyncToReactive(() => getDisplayLabel(props.claims, locale.value))

const isLoading = computed(() => {
  if (!displayLabel.value) {
    return false
  }
  if (typeof displayLabel.value !== "object") {
    return false
  }
  return "loading" in displayLabel.value && displayLabel.value.loading
})

const isError = computed(() => {
  if (!displayLabel.value) {
    return false
  }
  if (typeof displayLabel.value !== "object") {
    return false
  }
  if ("error" in displayLabel.value) {
    // This is a side-effect inside computed, but it is for debugging so it is fine.
    console.error("DisplayLabel.isError", displayLabel.value.error)
    return true
  }
  return false
})
</script>

<template>
  <template v-if="isLoading"><!-- TODO: What to show here? --></template>
  <i v-else-if="isError" class="pd-displaylabel-error text-error-600">{{ t("common.status.error") }}</i>
  <template v-else-if="displayLabel">{{ displayLabel }}</template>
  <template v-else
    ><i>{{ t("common.values.noName") }}</i></template
  >
</template>
