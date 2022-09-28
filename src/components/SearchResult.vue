<script setup lang="ts">
import type { PeerDBDocument } from "@/types"

import { computed } from "vue"
import { useRoute } from "vue-router"
import RouterLink from "@/components/RouterLink.vue"
import { getBestClaimOfType, getClaimsOfType, getClaimsListsOfType } from "@/utils"
import {
  DESCRIPTION,
  ORIGINAL_CATALOG_DESCRIPTION,
  TITLE,
  LABEL,
  IS,
  INSTANCE_OF,
  SUBCLASS_OF,
  MEDIAWIKI_MEDIA_TYPE,
  MEDIA_TYPE,
  COPYRIGHT_STATUS,
  PREVIEW_URL,
} from "@/props"

const props = defineProps<{
  doc: PeerDBDocument
}>()

const route = useRoute()

const hasLoaded = computed(() => props.doc?.name?.en)
const description = computed(() => {
  return getBestClaimOfType(props.doc.active, "text", [DESCRIPTION, ORIGINAL_CATALOG_DESCRIPTION, TITLE])?.html.en || ""
})
const tags = computed(() => {
  return [
    ...getClaimsOfType(props.doc.active, "rel", IS).map((c) => c.to.name.en),
    ...getClaimsOfType(props.doc.active, "rel", INSTANCE_OF).map((c) => c.to.name.en),
    ...getClaimsOfType(props.doc.active, "rel", SUBCLASS_OF).map((c) => c.to.name.en),
    ...getClaimsOfType(props.doc.active, "rel", LABEL).map((c) => c.to.name.en),
    ...getClaimsOfType(props.doc.active, "enum", MEDIAWIKI_MEDIA_TYPE).flatMap((c) => c.enum),
    ...getClaimsOfType(props.doc.active, "string", MEDIA_TYPE).map((c) => c.string),
    ...getClaimsOfType(props.doc.active, "rel", COPYRIGHT_STATUS).map((c) => c.to.name.en),
  ]
})
const previewFiles = computed(() => {
  // TODO: Sort files by group by properties (e.g., "image" first) and then sort inside groups by confidence.
  return [
    ...getClaimsListsOfType(props.doc.active, "ref", PREVIEW_URL)
      .flat(1)
      .map((c) => c.iri),
    ...[...(props.doc.active?.file || [])].flatMap((c) => c.preview ?? []),
  ]
})
</script>

<template>
  <div class="rounded border bg-white p-4 shadow">
    <div v-if="hasLoaded" class="flex flex-row gap-x-4">
      <div v-if="previewFiles.length" class="w-[256px] flex-none">
        <RouterLink :to="{ name: 'DocumentGet', params: { id: doc._id }, query: { s: route.query.s } }"
          ><img :src="previewFiles[0]" class="mx-auto bg-white"
        /></RouterLink>
      </div>
      <div class="flex-1">
        <h2 class="text-xl">
          <RouterLink :to="{ name: 'DocumentGet', params: { id: doc._id }, query: { s: route.query.s } }" class="link">{{ doc.name?.en }}</RouterLink>
        </h2>
        <ul v-if="tags.length" class="flex flex-row flex-wrap items-start gap-1 text-sm">
          <li v-for="tag of tags" :key="tag" class="py-1px rounded bg-secondary-400 px-1.5 text-neutral-600 shadow">{{ tag }}</li>
        </ul>
        <!-- eslint-disable-next-line vue/no-v-html -->
        <p v-if="description" class="prose prose-slate mt-4 max-w-none" v-html="description"></p>
      </div>
    </div>
    <div v-else class="flex animate-pulse">
      <div class="flex-1 space-y-4">
        <div class="h-2 w-72 rounded bg-slate-200"></div>
        <div class="grid grid-cols-5 gap-4">
          <div class="col-span-1 h-2 rounded bg-slate-200"></div>
          <div class="col-span-2 h-2 rounded bg-slate-200"></div>
        </div>
      </div>
    </div>
  </div>
</template>
