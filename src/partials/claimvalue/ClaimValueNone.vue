<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { NoneClaim } from "@/document"

import { useI18n } from "vue-i18n"

defineProps<{
  claim: NoneClaim | DeepReadonly<NoneClaim> | null
}>()

const { t } = useI18n()
</script>

<template>
  <template v-if="claim"
    ><i>{{ t("common.values.none") }}</i></template
  >
</template>
