<script setup lang="ts">
import type { DeepReadonly } from "vue"

import { computed, inject, toRaw } from "vue"
import { useI18n } from "vue-i18n"

import type { Result } from "@/types"

import DocumentRefInline from "@/partials/DocumentRefInline.vue"
import SearchResult from "@/partials/SearchResult.vue"
import SearchResultsPager from "@/partials/SearchResultsPager.vue"
import { searchPagerKey } from "@/utils"

// A grouped result node. When node.group is set it is a group heading (node.id is the referenced value's
// document, node.count its size); otherwise it is a plain result document. The same document may appear
// under several groups (multi-placement), so children are keyed by position, not only by id. A heading
// whose node.id is "__MISSING__" is the synthetic "missing" group, labeled instead of referencing a
// document (the same sentinel the reference filter uses, see RefFilterTreeRow).
//
// depth is the nesting level, used to break the full-width progress pager out of the group indentation.
const props = withDefaults(
  defineProps<{
    node: DeepReadonly<Result>
    searchSessionId: string
    depth?: number
  }>(),
  {
    depth: 0,
  },
)

const { t } = useI18n()

// The progress pager data SearchResultsFeed computes for the whole tree (which leaf a pager precedes, the
// unique results shown, and the matching total). A leaf gets a pager when pagerBefore has an entry for it,
// whose value is the count of unique results before it.
const pager = inject(
  searchPagerKey,
  computed(() => ({ pagerBefore: new Map<object, number>(), shown: 0, total: 0 })),
)
const pagerI = computed(() => (props.node.group ? undefined : pager.value.pagerBefore.get(toRaw(props.node))))
</script>

<template>
  <div v-if="node.group" class="pd-searchresultgroup flex flex-col gap-y-1 sm:gap-y-4">
    <div class="pd-searchresultgroup-header flex items-baseline gap-x-1 border-b border-slate-200 py-1 font-semibold text-slate-700">
      <i v-if="node.id === '__MISSING__'" class="min-w-0 truncate">{{ t("common.values.missing") }}</i>
      <DocumentRefInline v-else :id="node.id" class="min-w-0 truncate" />
      <span v-if="node.count != null" class="shrink-0 font-normal text-slate-500">({{ node.count }})</span>
    </div>
    <ul class="flex flex-col gap-y-1 pl-4 sm:gap-y-4 sm:pl-6">
      <li v-for="(child, i) in node.group" :key="`${child.id}-${i}`">
        <SearchResultGroup :node="child" :search-session-id="searchSessionId" :depth="depth + 1" />
      </li>
    </ul>
  </div>
  <template v-else>
    <SearchResultsPager v-if="pagerI !== undefined" :i="pagerI" :shown="pager.shown" :total="pager.total" :depth="depth" />
    <SearchResult :search-session-id="searchSessionId" :result="node" />
  </template>
</template>
