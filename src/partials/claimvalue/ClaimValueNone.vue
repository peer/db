<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { NoValueClaim } from "@/document"

import { useI18n } from "vue-i18n"

defineProps<{
  claim: NoValueClaim | DeepReadonly<NoValueClaim> | null
}>()

const { t } = useI18n()
</script>

<template>
  <template v-if="claim"
    ><i>{{ t("common.values.none") }}</i></template
  >
</template>
