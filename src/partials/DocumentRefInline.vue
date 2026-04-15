<script setup lang="ts">
import type { D } from "@/document"

import WithDocument from "@/components/WithDocument.vue"
import DisplayLabel from "@/partials/DisplayLabel.vue"
import { loadingWidth } from "@/utils"

withDefaults(
  defineProps<{
    id: string | null
    link?: boolean
  }>(),
  {
    link: true,
  },
)

// We want all fallthrough attributes to be passed to the link element.
defineOptions({
  inheritAttrs: false,
})

const WithDocumentD = WithDocument<D>
</script>

<template>
  <WithDocumentD v-if="id" :id="id" name="DocumentGet">
    <template #default="{ doc, url }">
      <RouterLink v-if="link" :to="{ name: 'DocumentGet', params: { id } }" :data-url="url" v-bind="$attrs" class="link"><DisplayLabel :doc="doc" /></RouterLink>
      <span v-else :data-url="url" v-bind="$attrs"><DisplayLabel :doc="doc" /></span>
    </template>
    <template #loading="{ url }">
      <div class="pd-documentrefinline-loading inline-block h-2 motion-safe:animate-pulse rounded-sm bg-slate-200" :data-url="url" :class="[loadingWidth(id)]" />
    </template>
  </WithDocumentD>
</template>
