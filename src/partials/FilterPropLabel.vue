<script setup lang="ts">
import DocumentRefInline from "@/partials/DocumentRefInline.vue"

// Renders a filter's property path as the label of a filter summary: a single property, or
// "parent > prop" for a sub-filter. link toggles whether the references link.
withDefaults(
  defineProps<{
    propIds: readonly string[]
    link?: boolean
  }>(),
  {
    link: true,
  },
)
</script>

<template>
  <template v-for="(prop, i) in propIds" :key="prop">
    <template v-if="i > 0">{{ " > " }}</template>
    <DocumentRefInline :id="prop" :link="link" />
  </template>
</template>
