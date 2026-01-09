<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { FileClaim } from "@/document"

import { useI18n } from "vue-i18n"

defineProps<{
  claim: FileClaim | DeepReadonly<FileClaim> | null
}>()

const { t } = useI18n()
</script>

<template>
  <template v-if="claim"
    ><a v-if="claim.preview?.[0]" :href="claim.url"><img :src="claim.preview[0]" :alt="t('common.icons.filePreview')" /></a
    ><a v-else :href="claim.url" class="link">{{ claim.mediaType }}</a></template
  >
</template>
