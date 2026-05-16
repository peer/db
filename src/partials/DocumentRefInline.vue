<script setup lang="ts">
import type { ComponentExposed } from "vue-component-type-helpers"

import type { D } from "@/document"

import { computed, useTemplateRef } from "vue"

import WithDocument from "@/components/WithDocument.vue"
import DisplayLabel from "@/partials/DisplayLabel.vue"
import { loadingWidth } from "@/utils"

const props = withDefaults(
  defineProps<{
    id: string | null
    link?: boolean
    // When true, the rendered link/span gets a title attribute carrying the
    // resolved display label, so a hovering user sees the full name even
    // when the surrounding layout truncates it.
    title?: boolean
  }>(),
  {
    link: true,
    title: false,
  },
)

// We want all fallthrough attributes to be passed to the link element.
defineOptions({
  inheritAttrs: false,
})

const WithDocumentD = WithDocument<D>

// The DisplayLabel child exposes its async-resolved label; we read it back
// here only to mirror it into the title attribute when requested.
const displayLabelRef = useTemplateRef<ComponentExposed<typeof DisplayLabel>>("displayLabelRef")

const titleAttr = computed<string | undefined>(() => {
  if (!props.title) return undefined
  const label = displayLabelRef.value?.displayLabel
  return typeof label === "string" && label ? label : undefined
})
</script>

<template>
  <WithDocumentD v-if="id" :id="id" name="DocumentGet">
    <template #default="{ doc, url }">
      <RouterLink v-if="link" :to="{ name: 'DocumentGet', params: { id } }" :data-url="url" :title="titleAttr" v-bind="$attrs" class="link"
        ><DisplayLabel ref="displayLabelRef" :doc="doc"
      /></RouterLink>
      <span v-else :data-url="url" :title="titleAttr" v-bind="$attrs"><DisplayLabel ref="displayLabelRef" :doc="doc" /></span>
    </template>
    <template #loading="{ url }">
      <div
        class="pd-documentrefinline-loading inline-block h-2 rounded-sm bg-slate-200 motion-safe:animate-pulse"
        :data-url="url"
        :class="[loadingWidth(id)]"
        aria-hidden="true"
      />
    </template>
  </WithDocumentD>
</template>
