<script setup lang="ts">
import { useI18n } from "vue-i18n"

defineProps<{
  // Number of results shown.
  first: number
  // Number of matching documents.
  total: number
  // Whether total is only a lower bound, because the server stopped counting past its track-total cap.
  moreThanTotal: boolean
}>()

const { t } = useI18n({ useScope: "global" })
</script>

<template>
  <div class="pd-pager-end pd-print-hidden my-1 sm:my-4">
    <div v-if="moreThanTotal" class="text-center text-sm">{{ t("common.status.allResultsMoreThan", { first, count: total }) }}</div>
    <div v-else-if="first < total" class="text-center text-sm">{{ t("common.status.allResultsOnly", { first, count: total }) }}</div>
    <div v-else-if="first === total" class="text-center text-sm">{{ t("common.status.allResults", { count: first }) }}</div>
    <div class="relative h-2 w-full bg-slate-200">
      <div class="absolute inset-y-0 left-0 w-full bg-secondary-400" />
    </div>
  </div>
</template>
