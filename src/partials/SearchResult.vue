<!--
A search result: the card, the document behind it, and what stands in for it while it is being fetched.
The document itself is rendered by SearchResultDocument, which is what a page with the document already
at hand renders on its own.
-->

<script setup lang="ts">
import type { ComponentExposed } from "vue-component-type-helpers"
import type { DeepReadonly } from "vue"

import type { D } from "@/document"
import type { Result } from "@/types"

import { useTemplateRef } from "vue"

import WithDocument from "@/components/WithDocument.vue"
import SearchResultDocument from "@/partials/SearchResultDocument.vue"
import { loadingLongWidth } from "@/utils"

withDefaults(
  defineProps<{
    // The search session this result belongs to. The document links carry it as the "s" query parameter.
    searchSessionId?: string
    result: DeepReadonly<Result>
    // duplicate is true when this result's document already appeared earlier in the grouped results; the card
    // then shows only its heading and a link back to the first occurrence instead of its contents.
    duplicate?: boolean
    // flat drops the card the result renders on (its border, background, shadow, and padding), for a page
    // which shows a single result on a card of its own. The pd-searchresult-flat class marks such a
    // result, so a site's theme can leave out its own card styling there as well.
    flat?: boolean
  }>(),
  {
    searchSessionId: undefined,
    duplicate: false,
    flat: false,
  },
)

const WithDocumentD = WithDocument<D>
const withDocument = useTemplateRef<ComponentExposed<typeof WithDocumentD>>("withDocument")
</script>

<template>
  <div
    :id="duplicate ? undefined : `result-${result.id}`"
    class="pd-searchresult flex flex-col gap-y-2"
    :class="flat ? 'pd-searchresult-flat' : 'rounded-sm border border-gray-200 bg-white p-4 shadow-sm'"
    :data-url="withDocument?.url"
  >
    <WithDocumentD :id="result.id" ref="withDocument" name="DocumentGet">
      <template #default="{ doc }">
        <SearchResultDocument :doc="doc" :search-session-id="searchSessionId" :duplicate="duplicate">
          <template #labelAside><slot name="labelAside" /></template>
        </SearchResultDocument>
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
