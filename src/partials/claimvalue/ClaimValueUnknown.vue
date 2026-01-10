<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { UnknownValueClaim } from "@/document"

import { useI18n } from "vue-i18n"

defineProps<{
  claim: UnknownValueClaim | DeepReadonly<UnknownValueClaim> | null
}>()

const { t } = useI18n()
</script>

<template>
  <template v-if="claim"
    ><i>{{ t("common.values.unknown") }}</i></template
  >
</template>
