<script setup lang="ts">
import type { PeerDBDocument } from "@/types"

import { computed } from "vue"
import { useRoute } from "vue-router"
import RouterLink from "@/components/RouterLink.vue"
import { getBestClaimOfType, getClaimsOfType, getClaimsListsOfType, getName } from "@/utils"
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
  DEPARTMENT,
  CLASSIFICATION,
  MEDIUM,
  NATIONALITY,
  GENDER,
} from "@/props"

const props = defineProps<{
  doc: PeerDBDocument
}>()

const route = useRoute()

const hasLoaded = computed(() => props.doc?.claims)
const docName = computed(() => getName(props.doc?.claims))
// TODO: Do not hard-code properties?
const description = computed(() => {
  return getBestClaimOfType(props.doc.claims, "text", [DESCRIPTION, ORIGINAL_CATALOG_DESCRIPTION, TITLE])?.html.en || ""
})
// TODO: Do not hard-code properties?
const tags = computed(() => {
  return [
    ...getClaimsOfType(props.doc.claims, "rel", IS).map((c) => c.to._id), // TODO: Render name.
    ...getClaimsOfType(props.doc.claims, "rel", INSTANCE_OF).map((c) => c.to._id), // TODO: Render name.
    ...getClaimsOfType(props.doc.claims, "rel", SUBCLASS_OF).map((c) => c.to._id), // TODO: Render name.
    ...getClaimsOfType(props.doc.claims, "rel", LABEL).map((c) => c.to._id), // TODO: Render name.
    ...getClaimsOfType(props.doc.claims, "string", DEPARTMENT).map((c) => c.string),
    ...getClaimsOfType(props.doc.claims, "string", CLASSIFICATION).map((c) => c.string),
    ...getClaimsOfType(props.doc.claims, "string", MEDIUM).map((c) => c.string),
    ...getClaimsOfType(props.doc.claims, "string", NATIONALITY).map((c) => c.string),
    ...getClaimsOfType(props.doc.claims, "string", GENDER).map((c) => c.string),
    ...getClaimsOfType(props.doc.claims, "string", MEDIAWIKI_MEDIA_TYPE).map((c) => c.string),
    ...getClaimsOfType(props.doc.claims, "string", MEDIA_TYPE).map((c) => c.string),
    ...getClaimsOfType(props.doc.claims, "rel", COPYRIGHT_STATUS).map((c) => c.to._id), // TODO: Render name.
  ]
})
const previewFiles = computed(() => {
  // TODO: Sort files by group by properties (e.g., "image" first) and then sort inside groups by confidence.
  return [
    ...getClaimsListsOfType(props.doc.claims, "ref", PREVIEW_URL)
      .flat(1)
      .map((c) => c.iri),
    ...[...(props.doc.claims?.file || [])].flatMap((c) => c.preview ?? []),
  ]
})
const rowsCount = computed(() => {
  let r = 1
  if (tags.value.length) {
    r++
  }
  if (description.value) {
    r++
  }
  return r
})
// We have to use complete class names for Tailwind to detect used classes and generating the
// corresponding CSS and do not do string interpolation or concatenation of partial class names.
// See: https://tailwindcss.com/docs/content-configuration#dynamic-class-names
const gridRows = computed(() => {
  switch (rowsCount.value) {
    case 1:
      return "sm:grid-rows-[1fr]"
    case 2:
      return "sm:grid-rows-[auto_1fr]"
    case 3:
      return "sm:grid-rows-[auto_auto_1fr]"
    default:
      throw new Error(`unexpected count of rows: ${rowsCount.value}`)
  }
})
const rowSpan = computed(() => {
  switch (rowsCount.value) {
    case 1:
      return "sm:row-span-1"
    case 2:
      return "sm:row-span-2"
    case 3:
      return "sm:row-span-3"
    default:
      throw new Error(`unexpected count of rows: ${rowsCount.value}`)
  }
})
</script>

<template>
  <div class="rounded border bg-white p-4 shadow">
    <div v-if="hasLoaded" class="grid grid-cols-1 gap-4" :class="previewFiles.length ? `sm:grid-cols-[256px_auto] ${gridRows}` : ''">
      <h2 class="text-xl leading-none">
        <RouterLink :to="{ name: 'DocumentGet', params: { id: doc._id }, query: { s: route.query.s } }" class="link" v-html="docName || '<i>no name</i>'"></RouterLink>
      </h2>
      <ul v-if="tags.length" class="-mt-3 flex flex-row flex-wrap content-start items-start gap-1 text-sm">
        <li v-for="tag of tags" :key="tag" class="rounded-sm bg-secondary-400 py-0.5 px-1.5 leading-none text-neutral-600 shadow-sm">{{ tag }}</li>
      </ul>
      <div v-if="previewFiles.length" :class="`w-full sm:order-first ${rowSpan}`">
        <RouterLink :to="{ name: 'DocumentGet', params: { id: doc._id }, query: { s: route.query.s } }"
          ><img :src="previewFiles[0]" class="mx-auto bg-white"
        /></RouterLink>
      </div>
      <!-- eslint-disable-next-line vue/no-v-html -->
      <p v-if="description" class="prose prose-slate max-w-none" v-html="description"></p>
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
