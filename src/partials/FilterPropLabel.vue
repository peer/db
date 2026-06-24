<script setup lang="ts">
import { useI18n } from "vue-i18n"

import DocumentRefInline from "@/partials/DocumentRefInline.vue"

// Renders a filter's property path as the label of a filter summary: a single property, or
// "parent > prop" for a sub-filter. link toggles whether the references link. appendHas appends the
// "has property" marker as a final path segment, so a has filter reads "parent > has property", or just
// "has property" when there is no parent property.
withDefaults(
  defineProps<{
    propIds: readonly string[]
    link?: boolean
    appendHas?: boolean
  }>(),
  {
    link: true,
    appendHas: false,
  },
)

const { t } = useI18n({ useScope: "global" })
</script>

<template>
  <template v-for="(prop, i) in propIds" :key="prop">
    <template v-if="i > 0">{{ " > " }}</template>
    <DocumentRefInline :id="prop" :link="link" />
  </template>
  <template v-if="appendHas">
    <template v-if="propIds.length > 0">{{ " > " }}</template>
    {{ t("partials.FilterPropLabel.hasProperty") }}
  </template>
</template>
