<script setup lang="ts">
import type { RefFilter } from "@/types"
import type { RefFilterValueToken } from "@/utils"
import type { DeepReadonly } from "vue"

import { computed } from "vue"
import { useI18n } from "vue-i18n"

import DocumentRefInline from "@/partials/DocumentRefInline.vue"
import { listFormatParts, refFilterValueTokens } from "@/utils"

// Renders a reference filter's label together with its active selection as "label: value, value (direct)".
// The label (the filter's property path) comes from the default slot so the caller controls its markup
// (links on or off). When the filter has a selection the label and the value list are woven into a single
// translation message so a translator controls the separator and its spacing. With no selection (which cannot
// really happen for valid filters) only the label is rendered, without a trailing separator. link toggles
// whether the value references link.
const props = withDefaults(
  defineProps<{
    refFilter: DeepReadonly<RefFilter>
    link?: boolean
  }>(),
  {
    link: true,
  },
)

const { t, locale } = useI18n({ useScope: "global" })

// The filter's active selection as an ordered token list: To values, then Direct values, then missing.
const tokens = computed((): RefFilterValueToken[] => refFilterValueTokens(props.refFilter))

// The token list interleaved with the locale's list separators (via Intl.ListFormat): each entry is either
// a separator to print or a token to render. The values are OR-ed by the filter, so they are listed as a
// disjunction (in English "a, b, or c").
const valueParts = computed((): Array<{ separator: string } | { token: RefFilterValueToken }> =>
  listFormatParts(locale.value, tokens.value.length, "disjunction").map((part) => (part.type === "literal" ? { separator: part.value } : { token: tokens.value[part.index] })),
)
</script>

<template>
  <i18n-t v-if="tokens.length > 0" keypath="common.labelWithValues" scope="global">
    <template #label><slot /></template>
    <template #values>
      <template v-for="(part, i) in valueParts" :key="i">
        <template v-if="'separator' in part">{{ part.separator }}</template>
        <template v-else>
          <i v-if="part.token.kind === 'missing'">{{ t("common.values.missing") }}</i>
          <i18n-t v-else-if="part.token.direct" keypath="common.valueWithDirect" scope="global">
            <template #value><DocumentRefInline :id="part.token.id" :link="link" /></template>
            <template #direct
              ><i>{{ t("common.values.direct") }}</i></template
            >
          </i18n-t>
          <DocumentRefInline v-else :id="part.token.id" :link="link" />
        </template>
      </template>
    </template>
  </i18n-t>
  <slot v-else />
</template>
