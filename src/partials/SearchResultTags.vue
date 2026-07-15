<script setup lang="ts">
import type { D } from "@/document"

import WithDocument from "@/components/WithDocument.vue"
import DisplayLabel from "@/partials/DisplayLabel.vue"
import { loadingWidth } from "@/utils"

// A tag with an id renders as the referenced document's label (from a ref claim); a tag with a label
// renders as a literal value (from an identifier or string claim).
defineProps<{
  tags: { id?: string; label?: string }[]
}>()

const WithDocumentD = WithDocument<D>
</script>

<template>
  <ul class="flex flex-row flex-wrap content-start items-baseline gap-1 text-sm">
    <template v-for="(tag, i) of tags" :key="tag.id ?? `label-${i}`">
      <WithDocumentD v-if="tag.id" :id="tag.id" name="DocumentGet">
        <template #default="{ doc, url }">
          <li class="rounded-xs bg-slate-100 px-1.5 py-0.5 leading-none text-gray-600 shadow-xs" :data-url="url">
            <DisplayLabel :doc="doc" />
          </li>
        </template>
        <template #loading="{ url }">
          <li
            class="pd-withdocument-loading h-2 rounded-sm bg-slate-200 motion-safe:animate-pulse"
            :data-url="url"
            :class="[loadingWidth(tag.id)]"
            aria-hidden="true"
          ></li>
        </template>
      </WithDocumentD>
      <li v-else class="rounded-xs bg-slate-100 px-1.5 py-0.5 leading-none text-gray-600 shadow-xs">{{ tag.label }}</li>
    </template>
  </ul>
</template>
