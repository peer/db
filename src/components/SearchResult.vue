<script setup lang="ts">
import type { PeerDBDocument } from "@/types"

import { computed } from "vue"
import { useRoute } from "vue-router"
import RouterLink from "@/components/RouterLink.vue"
import { getStandardPropertyID, getBestClaimOfType, getClaimsOfType, getWikidataDocumentID } from "@/utils"

const DESCRIPTION = getStandardPropertyID("DESCRIPTION")
const ORIGINAL_CATALOG_DESCRIPTION = getWikidataDocumentID("P10358")
const TITLE = getWikidataDocumentID("P1476")
const LABEL = getStandardPropertyID("LABEL")
const IS = getStandardPropertyID("IS")
const INSTANCE_OF = getWikidataDocumentID("P31")
const SUBCLASS_OF = getWikidataDocumentID("P279")
const MEDIAWIKI_MEDIA_TYPE = getStandardPropertyID("MEDIAWIKI_MEDIA_TYPE")
const MEDIA_TYPE = getStandardPropertyID("MEDIA_TYPE")
const COPYRIGHT_STATUS = getWikidataDocumentID("P6216")

const props = defineProps<{
  doc: PeerDBDocument
}>()

const route = useRoute()

const hasLoaded = computed(() => props.doc?.name?.en)
const description = computed(() => {
  return getBestClaimOfType(props.doc, "text", [DESCRIPTION, ORIGINAL_CATALOG_DESCRIPTION, TITLE])?.html.en || ""
})
const tags = computed(() => {
  return [
    ...getClaimsOfType(props.doc, "rel", IS).map((c) => c.to.name.en),
    ...getClaimsOfType(props.doc, "rel", INSTANCE_OF).map((c) => c.to.name.en),
    ...getClaimsOfType(props.doc, "rel", SUBCLASS_OF).map((c) => c.to.name.en),
    ...getClaimsOfType(props.doc, "rel", LABEL).map((c) => c.to.name.en),
    ...getClaimsOfType(props.doc, "enum", MEDIAWIKI_MEDIA_TYPE)
      .map((c) => c.enum)
      .flat(1),
    ...getClaimsOfType(props.doc, "string", MEDIA_TYPE).map((c) => c.string),
    ...getClaimsOfType(props.doc, "rel", COPYRIGHT_STATUS).map((c) => c.to.name.en),
  ]
})
</script>

<template>
  <div class="rounded border bg-white p-4 shadow">
    <div v-if="hasLoaded">
      <h2 class="text-xl">
        <RouterLink :to="{ name: 'DocumentGet', params: { id: doc._id }, query: { s: route.query.s } }" class="link">{{ doc.name?.en }}</RouterLink>
      </h2>
      <ul v-if="tags.length" class="flex flex-row flex-wrap items-start gap-1 text-sm">
        <li v-for="tag of tags" :key="tag" class="py-1px rounded bg-secondary-400 px-1.5 text-neutral-600 shadow">{{ tag }}</li>
      </ul>
      <!-- eslint-disable-next-line vue/no-v-html -->
      <p v-if="description" class="prose prose-slate mt-4 max-w-none" v-html="description"></p>
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
