<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { UnknownClaim } from "@/document"

import { useI18n } from "vue-i18n"

defineProps<{
  claim: UnknownClaim | DeepReadonly<UnknownClaim> | null
}>()

const { t } = useI18n({ useScope: "global" })
</script>

<template>
  <template v-if="claim"
    ><i>{{ t("common.values.unknown") }}</i></template
  >
</template>
