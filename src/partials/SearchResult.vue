<script setup lang="ts">
import type { ComponentExposed } from "vue-component-type-helpers"
import type { DeepReadonly } from "vue"

import type { D } from "@/document"
import type { Result } from "@/types"

import { ArrowRightIcon } from "@heroicons/vue/20/solid"
import { computed, onBeforeUnmount, toRef, useTemplateRef } from "vue"
import { useI18n } from "vue-i18n"
import { useRoute } from "vue-router"

import ButtonLink from "@/components/ButtonLink.vue"
import WithDocument from "@/components/WithDocument.vue"
import { DESCRIPTION, INSTANCE_OF, PAGE, SUBCLASS_OF } from "@/core"
import { getClaimsOfTypeWithConfidence, selectClaimsByLanguage } from "@/document"
import { useInternalLinksClick, useTransformedHtml } from "@/internal-links"
import DisplayLabel from "@/partials/DisplayLabel.vue"
import FieldsView from "@/partials/FieldsView.vue"
import SearchResultTags from "@/partials/SearchResultTags.vue"
import { useProgress } from "@/progress"
import { getSearchResultComponents } from "@/registry/search-result"
import { getDescriptionProperties, getPreviewProperties, getTagsProperties } from "@/registry/search-result-properties"
import { useDocumentFields } from "@/useDocumentFields"
import { useParentClasses } from "@/useParentClasses"
import { encodeQuery, loadingLongWidth } from "@/utils"

const props = withDefaults(
  defineProps<{
    // The search session this result belongs to. The document links carry it as the "s" query parameter.
    searchSessionId?: string
    result: DeepReadonly<Result>
    // duplicate is true when this result's document already appeared earlier in the grouped results; the card
    // then shows only its heading and a link back to the first occurrence instead of its contents.
    duplicate?: boolean
  }>(),
  {
    searchSessionId: undefined,
    duplicate: false,
  },
)

const { t, locale } = useI18n({ useScope: "global" })

// duplicateOfLink points at the first occurrence of this result through the "at" query parameter, the link a
// duplicate card offers back to it.
const route = useRoute()
const duplicateOfLink = computed(() => ({
  name: route.name as string,
  params: route.params,
  query: encodeQuery({ ...route.query, at: props.result.id }),
}))

const el = useTemplateRef<HTMLElement>("el")

const abortController = new AbortController()

onBeforeUnmount(() => {
  abortController.abort()
})

// Data loading only, no controls.
const progress = useProgress()

const WithDocumentD = WithDocument<D>
const withDocument = useTemplateRef<ComponentExposed<typeof WithDocumentD>>("withDocument")
const displayLabelComponent = useTemplateRef<ComponentExposed<typeof DisplayLabel>>("displayLabelComponent")

const searchResultComponents = getSearchResultComponents()
const customResultComponent = computed(() => {
  const doc = withDocument.value?.doc
  if (!doc?.claims) return null
  const refs = getClaimsOfTypeWithConfidence(doc.claims, "ref", INSTANCE_OF)
  for (const ref of refs) {
    const comp = searchResultComponents.value.get(ref.to.id)
    if (comp) {
      return comp
    }
  }
  return null
})

// Whether this result is a page (an instance of the PAGE class). Pages are kept out of the
// FieldsView layout so they render via the generic card (title, the instance-of "page" badge,
// and the description) instead of dumping the Content field into the result.
const isPage = computed(() => {
  const doc = withDocument.value?.doc
  if (!doc?.claims) return false
  return getClaimsOfTypeWithConfidence(doc.claims, "ref", INSTANCE_OF).some((ref) => ref.to.id === PAGE)
})

// Resolve field definitions for this document.
const docRef = toRef(() => withDocument.value?.doc ?? null)
const { classDocs, instanceOfClassIds } = useParentClasses(docRef, el, progress)
const { fieldsData } = useDocumentFields(classDocs, instanceOfClassIds)

const onDescriptionClick = useInternalLinksClick()

// The description, tags, and preview images are taken from configurable property registries so that
// consumers can adapt what a search result card shows (see registry/search-result-properties). Each
// registry is empty by default; while empty we fall back to the built-in defaults below, and once a
// consumer registers any property the registry replaces those defaults entirely.
const descriptionProperties = getDescriptionProperties()
const tagsProperties = getTagsProperties()
const previewProperties = getPreviewProperties()

const effectiveDescriptionProperties = computed(() => (descriptionProperties.value.length ? descriptionProperties.value : [DESCRIPTION]))
const effectiveTagsProperties = computed(() => (tagsProperties.value.length ? tagsProperties.value : [INSTANCE_OF, SUBCLASS_OF]))

// Only HTML claims of the description properties are used; the first match (resolved by language) wins.
const description = computed(() => {
  const claims = selectClaimsByLanguage(withDocument.value?.doc?.claims, "html", effectiveDescriptionProperties.value, locale.value, (c) => c.length > 0 && !!c[0].html)
  return claims && claims.length > 0 ? claims[0].html : ""
})
const transformedDescription = useTransformedHtml(description)

// A tag with an id renders as the referenced document's label (from ref claims); a tag with a label
// renders as a literal value (from identifier and string claims).
const tags = computed<{ id?: string; label?: string }[]>(() => {
  const claims = withDocument.value?.doc?.claims
  return [
    ...getClaimsOfTypeWithConfidence(claims, "ref", effectiveTagsProperties.value).map((c) => ({ id: c.to.id })),
    ...getClaimsOfTypeWithConfidence(claims, "id", effectiveTagsProperties.value).map((c) => ({ label: c.value })),
    ...getClaimsOfTypeWithConfidence(claims, "string", effectiveTagsProperties.value).map((c) => ({ label: c.string })),
  ]
})

// Only link claims of the preview properties are used, and their IRIs are shown as preview images.
// There is no default preview property, so an empty registry means no preview.
const previewFiles = computed<string[]>(() => {
  return getClaimsOfTypeWithConfidence(withDocument.value?.doc?.claims, "link", previewProperties.value).map((c) => c.iri)
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
  <div
    :id="`result-${result.id}`"
    ref="el"
    class="pd-searchresult flex flex-col gap-y-2 rounded-sm border border-gray-200 bg-white p-4 shadow-sm"
    :data-url="withDocument?.url"
  >
    <WithDocumentD :id="result.id" ref="withDocument" name="DocumentGet">
      <template #default="{ doc: resultDoc }">
        <div v-if="duplicate">
          <ButtonLink
            :to="{ name: 'DocumentGet', params: { id: resultDoc.id }, query: encodeQuery({ s: searchSessionId }) }"
            class="pd-print-hidden float-end mb-1 ml-4 px-4"
          >
            <ArrowRightIcon class="size-5 sm:hidden" :alt="t('partials.SearchResult.details')" />
            <span class="hidden sm:inline">{{ t("partials.SearchResult.details") }}</span>
          </ButtonLink>
          <h2 v-show="displayLabelComponent?.displayLabel" class="mb-2 flex items-baseline gap-x-1 text-xl leading-none">
            <RouterLink :to="{ name: 'DocumentGet', params: { id: resultDoc.id }, query: encodeQuery({ s: searchSessionId }) }" class="link min-w-0"
              ><DisplayLabel ref="displayLabelComponent" :doc="resultDoc"
            /></RouterLink>
            <slot name="labelAside" />
          </h2>
          <SearchResultTags v-if="tags.length" :tags="tags" class="mb-2" />
          <i18n-t keypath="partials.SearchResult.resultShownAlready" scope="global" tag="p" class="text-slate-500 italic">
            <template #above>
              <RouterLink :to="duplicateOfLink" class="link">{{ t("partials.SearchResult.above") }}</RouterLink>
            </template>
          </i18n-t>
        </div>
        <component :is="customResultComponent" v-else-if="customResultComponent" :doc="resultDoc" :search-session-id="searchSessionId" />
        <!-- Pages skip the FieldsView layout (which would dump the Content field into the card) and fall through to the generic card below, which renders title, the instance-of "page" badge, and the description. -->
        <div v-else-if="!isPage && fieldsData && resultDoc.claims">
          <ButtonLink
            :to="{ name: 'DocumentGet', params: { id: resultDoc.id }, query: encodeQuery({ s: searchSessionId }) }"
            class="pd-print-hidden float-end mb-1 ml-4 px-4"
          >
            <ArrowRightIcon class="size-5 sm:hidden" :alt="t('partials.SearchResult.details')" />
            <span class="hidden sm:inline">{{ t("partials.SearchResult.details") }}</span>
          </ButtonLink>
          <h2 v-show="displayLabelComponent?.displayLabel" class="mb-2 flex items-baseline gap-x-1 text-xl leading-none">
            <RouterLink :to="{ name: 'DocumentGet', params: { id: resultDoc.id }, query: encodeQuery({ s: searchSessionId }) }" class="link min-w-0"
              ><DisplayLabel ref="displayLabelComponent" :doc="resultDoc"
            /></RouterLink>
            <slot name="labelAside" />
          </h2>
          <SearchResultTags v-if="tags.length" :tags="tags" class="mb-2" />
          <FieldsView :fields-data="fieldsData" :claims="resultDoc.claims" limited />
        </div>
        <div v-else class="grid grid-cols-1 gap-4" :class="previewFiles.length ? `sm:grid-cols-[256px_auto] ${gridRows}` : ''">
          <div>
            <ButtonLink :to="{ name: 'DocumentGet', params: { id: resultDoc.id }, query: encodeQuery({ s: searchSessionId }) }" class="pd-print-hidden float-end px-4">{{
              t("partials.SearchResult.details")
            }}</ButtonLink>
            <h2 v-show="displayLabelComponent?.displayLabel" class="mb-2 flex items-baseline gap-x-1 text-xl leading-none">
              <RouterLink :to="{ name: 'DocumentGet', params: { id: resultDoc.id }, query: encodeQuery({ s: searchSessionId }) }" class="link min-w-0"
                ><DisplayLabel ref="displayLabelComponent" :doc="resultDoc"
              /></RouterLink>
              <slot name="labelAside" />
            </h2>
            <SearchResultTags v-if="tags.length" :tags="tags" />
          </div>
          <div v-if="previewFiles.length" :class="`w-full sm:order-first ${rowSpan}`">
            <RouterLink :to="{ name: 'DocumentGet', params: { id: resultDoc.id }, query: encodeQuery({ s: searchSessionId }) }"
              ><img :src="previewFiles[0]" class="mx-auto bg-white"
            /></RouterLink>
          </div>
          <!-- eslint-disable-next-line vue/no-v-html -->
          <p v-if="description" class="prose max-w-none prose-gray" @click="onDescriptionClick" v-html="transformedDescription"></p>
        </div>
      </template>
      <template #loading>
        <div class="pd-withdocument-loading flex flex-col gap-y-2 motion-safe:animate-pulse" aria-hidden="true">
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
      <template #error="{ message, accessDenied }">
        <i :class="['pd-withdocument-error', accessDenied ? 'text-gray-500' : 'text-error-600']">{{ message }}</i>
      </template>
    </WithDocumentD>
  </div>
</template>
