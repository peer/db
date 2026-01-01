<script setup lang="ts">
import type { ComponentExposed } from "vue-component-type-helpers"

import type { PeerDBDocument } from "@/document"
import type { Result } from "@/types"

import { computed, ref } from "vue"

import WithDocument from "@/components/WithDocument.vue"
import {
  CATEGORY,
  CLASSIFICATION,
  COPYRIGHT_STATUS,
  DEPARTMENT,
  DESCRIPTION,
  GENDER,
  INGREDIENTS,
  INSTANCE_OF,
  LABEL,
  MEDIAWIKI_MEDIA_TYPE,
  MEDIA_TYPE,
  MEDIUM,
  NAME,
  NATIONALITY,
  ORIGINAL_CATALOG_DESCRIPTION,
  PREVIEW_URL,
  SUBCLASS_OF,
  TITLE,
  TYPE,
} from "@/props"
import { encodeQuery, getBestClaimOfType, getClaimsListsOfType, getClaimsOfType, getName, loadingLongWidth, loadingWidth } from "@/utils"

defineProps<{
  searchSessionId: string
  result: Result
}>()

const WithPeerDBDocument = WithDocument<PeerDBDocument>
const withDocument = ref<ComponentExposed<typeof WithPeerDBDocument> | null>(null)

const docName = computed(() => getName(withDocument.value?.doc?.claims))
// TODO: Do not hard-code properties?
const description = computed(() => {
  const props = [INGREDIENTS, ORIGINAL_CATALOG_DESCRIPTION, TITLE]
  if (getBestClaimOfType(withDocument.value?.doc?.claims, "text", NAME)) {
    // If DESCRIPTION is not already used in getName, then we use it here.
    props.push(DESCRIPTION)
  }
  return getBestClaimOfType(withDocument.value?.doc?.claims, "text", props)?.html.en || ""
})
// TODO: Do not hard-code properties?
const tags = computed(() => {
  return [
    ...getClaimsOfType(withDocument.value?.doc?.claims, "rel", TYPE).map((c) => ({ id: c.to.id })),
    ...getClaimsOfType(withDocument.value?.doc?.claims, "rel", INSTANCE_OF).map((c) => ({ id: c.to.id })),
    ...getClaimsOfType(withDocument.value?.doc?.claims, "rel", SUBCLASS_OF).map((c) => ({ id: c.to.id })),
    ...getClaimsOfType(withDocument.value?.doc?.claims, "rel", LABEL).map((c) => ({ id: c.to.id })),
    ...getClaimsOfType(withDocument.value?.doc?.claims, "string", DEPARTMENT).map((c) => ({ string: c.string })),
    ...getClaimsOfType(withDocument.value?.doc?.claims, "string", CLASSIFICATION).map((c) => ({ string: c.string })),
    ...getClaimsOfType(withDocument.value?.doc?.claims, "string", MEDIUM).map((c) => ({ string: c.string })),
    ...getClaimsOfType(withDocument.value?.doc?.claims, "string", NATIONALITY).map((c) => ({ string: c.string })),
    ...getClaimsOfType(withDocument.value?.doc?.claims, "string", GENDER).map((c) => ({ string: c.string })),
    ...getClaimsOfType(withDocument.value?.doc?.claims, "string", MEDIAWIKI_MEDIA_TYPE).map((c) => ({ string: c.string })),
    ...getClaimsOfType(withDocument.value?.doc?.claims, "string", MEDIA_TYPE).map((c) => ({ string: c.string })),
    ...getClaimsOfType(withDocument.value?.doc?.claims, "rel", COPYRIGHT_STATUS).map((c) => ({ id: c.to.id })),
    ...getClaimsOfType(withDocument.value?.doc?.claims, "string", CATEGORY).map((c) => ({ string: c.string })),
  ]
})
const previewFiles = computed(() => {
  // TODO: Sort files by group by properties (e.g., "image" first) and then sort inside groups by confidence.
  return [
    ...getClaimsListsOfType(withDocument.value?.doc?.claims, "ref", PREVIEW_URL)
      .flat(1)
      .map((c) => c.iri),
    ...[...(withDocument.value?.doc?.claims?.file || [])].flatMap((c) => c.preview ?? []),
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
  <div :id="`result-${result.id}`" class="rounded-sm border border-gray-200 bg-white p-4 shadow-sm" :data-url="withDocument?.url">
    <WithPeerDBDocument :id="result.id" ref="withDocument" name="DocumentGet">
      <template #default="{ doc: resultDoc }">
        <div class="grid grid-cols-1 gap-4" :class="previewFiles.length ? `sm:grid-cols-[256px_auto] ${gridRows}` : ''">
          <h2 class="text-xl leading-none">
            <RouterLink
              :to="{ name: 'DocumentGet', params: { id: resultDoc.id }, query: encodeQuery({ s: searchSessionId }) }"
              class="link"
              v-html="docName || '<i>no name</i>'"
            ></RouterLink>
          </h2>
          <ul v-if="tags.length" class="-mt-3 flex flex-row flex-wrap content-start items-baseline gap-1 text-sm">
            <template v-for="tag of tags" :key="'id' in tag ? tag.id : tag.string">
              <li v-if="'string' in tag" class="rounded-xs bg-slate-100 px-1.5 py-0.5 leading-none text-gray-600 shadow-xs">{{ tag.string }}</li>
              <WithPeerDBDocument v-else-if="'id' in tag" :id="tag.id" name="DocumentGet">
                <template #default="{ doc, url }">
                  <li
                    class="rounded-xs bg-slate-100 px-1.5 py-0.5 leading-none text-gray-600 shadow-xs"
                    :data-url="url"
                    v-html="getName(doc.claims) || '<i>no name</i>'"
                  ></li>
                </template>
                <template #loading="{ url }">
                  <li class="h-2 animate-pulse rounded-sm bg-slate-200" :data-url="url" :class="[loadingWidth(tag.id)]"></li>
                </template>
              </WithPeerDBDocument>
            </template>
          </ul>
          <div v-if="previewFiles.length" :class="`w-full sm:order-first ${rowSpan}`">
            <RouterLink :to="{ name: 'DocumentGet', params: { id: resultDoc.id }, query: encodeQuery({ s: searchSessionId }) }"
              ><img :src="previewFiles[0]" class="mx-auto bg-white"
            /></RouterLink>
          </div>
          <!-- eslint-disable-next-line vue/no-v-html -->
          <p v-if="description" class="prose max-w-none prose-slate" v-html="description"></p>
        </div>
      </template>
      <template #loading>
        <div class="flex animate-pulse flex-col gap-y-2">
          <div class="inline-block h-2 rounded-sm bg-slate-200" :class="[loadingLongWidth(`${result.id}/1`)]"></div>
          <div class="flex gap-x-4">
            <div class="h-2 rounded-sm bg-slate-200" :class="[loadingLongWidth(`${result.id}/2`)]"></div>
            <div class="h-2 rounded-sm bg-slate-200" :class="[loadingLongWidth(`${result.id}/3`)]"></div>
          </div>
          <div class="flex gap-x-4">
            <div class="h-2 rounded-sm bg-slate-200" :class="[loadingLongWidth(`${result.id}/4`)]"></div>
            <div class="h-2 rounded-sm bg-slate-200" :class="[loadingLongWidth(`${result.id}/5`)]"></div>
          </div>
        </div>
      </template>
      <template #error>
        <i class="text-error-600">loading data failed</i>
      </template>
    </WithPeerDBDocument>
  </div>
</template>
