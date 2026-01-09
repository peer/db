<script setup lang="ts">
import type { PeerDBDocument } from "@/document"

import { useI18n } from "vue-i18n";

import WithDocument from "@/components/WithDocument.vue"
import { getName, loadingWidth } from "@/utils"

defineProps<{
  id: string | null
}>()

// We want all fallthrough attributes to be passed to the link element.
defineOptions({
  inheritAttrs: false,
})

const { t } = useI18n()

const WithPeerDBDocument = WithDocument<PeerDBDocument>
</script>

<template>
  <WithPeerDBDocument v-if="id" :id="id" name="DocumentGet">
    <template #default="{ doc, url }">
      <RouterLink :to="{ name: 'DocumentGet', params: { id } }" :data-url="url" v-bind="$attrs" class="link" v-html="getName(doc.claims) || `<i>${t('common.values.noName')}</i>`" />
    </template>
    <template #loading="{ url }">
      <div class="inline-block h-2 animate-pulse rounded-sm bg-slate-200" :data-url="url" :class="[loadingWidth(id)]" />
    </template>
  </WithPeerDBDocument>
</template>
