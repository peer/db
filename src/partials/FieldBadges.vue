<script setup lang="ts">
import { useI18n } from "vue-i18n"

defineProps<{
  required?: boolean
  changed?: boolean
}>()

const { t } = useI18n({ useScope: "global" })
</script>

<template>
  <span v-if="required" class="rounded-xs bg-slate-100 px-1.5 py-0.5 text-xs leading-none text-gray-600 shadow-xs">{{ t("common.labels.required") }}</span>
  <span v-if="changed" class="rounded-xs bg-primary-300 px-1.5 py-0.5 text-xs leading-none text-gray-100 shadow-xs">{{ t("common.labels.changed") }}</span>
</template>
