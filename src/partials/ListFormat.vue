<!--
The items of a list rendered with the locale's separators between them (and any trailing conjunction),
through Intl.ListFormat (see listFormatParts). The items themselves are rendered by the default slot,
which is given the index of the item to render, so a list of elements (and not of plain strings) reads
the way the language writes a list instead of being joined by a hard-coded comma.

Use formatList where the items are plain strings and the result is text rather than elements.
-->

<script setup lang="ts">
import { computed } from "vue"
import { useI18n } from "vue-i18n"

import { listFormatParts } from "@/utils"

const props = withDefaults(
  defineProps<{
    count: number
    // The enumeration style: "unit" is a plain comma-style list, while "conjunction" and "disjunction"
    // add the language's "and" and "or" before the last item.
    type?: "conjunction" | "disjunction" | "unit"
  }>(),
  {
    type: "unit",
  },
)

const { locale } = useI18n({ useScope: "global" })

const parts = computed(() => listFormatParts(locale.value, props.count, props.type))
</script>

<template>
  <template v-for="(part, i) of parts" :key="i"
    ><template v-if="part.type === 'literal'">{{ part.value }}</template
    ><slot v-else :index="part.index"></slot
  ></template>
</template>
