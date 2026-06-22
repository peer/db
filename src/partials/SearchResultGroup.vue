<script setup lang="ts">
import type { DeepReadonly } from "vue"

import { ChevronUpDownIcon } from "@heroicons/vue/20/solid"
import { ChevronDownUpIcon } from "@sidekickicons/vue/20/solid"
import { computed, inject, toRaw } from "vue"
import { useI18n } from "vue-i18n"

import type { Result } from "@/types"

import DocumentRefInline from "@/partials/DocumentRefInline.vue"
import SearchResult from "@/partials/SearchResult.vue"
import SearchResultsPager from "@/partials/SearchResultsPager.vue"
import { searchExpandKey, searchPagerKey } from "@/utils"
import { searchTrackKey } from "@/visibility"

// A grouped result node. When node.group is set it is a group heading (node.id is the referenced value's
// document, node.count its size); otherwise it is a plain result document. The same document may appear
// under several groups (multi-placement), so children are keyed by position, not only by id. A heading
// whose node.id is "__MISSING__" is the synthetic "missing" group, labeled instead of referencing a
// document (the same sentinel the reference filter uses, see RefFilterTreeRow).
//
// depth is the nesting level, used to break the full-width progress pager out of the group indentation.
//
// expandLevels[d] reports whether the group at depth d is rendered as a full result card for its grouping
// value's document instead of a one-line heading. It is indexed by depth so the recursive children read
// their own level; it is the same array for the whole tree.
const props = withDefaults(
  defineProps<{
    node: DeepReadonly<Result>
    searchSessionId: string
    depth?: number
    expandLevels?: readonly boolean[]
  }>(),
  {
    depth: 0,
    expandLevels: () => [],
  },
)

const { t } = useI18n()

// levelExpanded reports whether this group level (across all of its values) is expanded into full result
// cards; expanded reports whether this particular heading renders as a card. The synthetic "missing" group
// has no document to show a card for, so it always stays a one-line heading even when the level is expanded.
const levelExpanded = computed(() => props.expandLevels[props.depth] ?? false)
const expanded = computed(() => props.node.id !== "__MISSING__" && levelExpanded.value)

// setExpand switches this group level between its expanded (full result cards) and collapsed (one-line
// headings) forms in the search state, the in-place equivalent of the sort dialog's Expand checkbox. The
// heading offers it to expand, the expanded card offers it to collapse.
const setExpand = inject(searchExpandKey, () => undefined)

// The progress pager data SearchResultsFeed computes for the whole tree (which leaf a pager precedes, the
// unique results shown, and the matching total).
const pager = inject(
  searchPagerKey,
  computed(() => ({ pagerBefore: new Map<object, number>(), shown: 0, total: 0, duplicates: new Set<object>() })),
)

// isDuplicate reports whether a leaf result's document already appeared earlier in the grouped tree, in which
// case its card is rendered as a back-reference to the first occurrence.
function isDuplicate(node: DeepReadonly<Result>): boolean {
  return pager.value.duplicates.has(toRaw(node))
}

// track registers each leaf result with the feed's visibility observer (a no-op when not provided) so the
// "at" scroll position follows grouped results the same as flat ones.
const track = inject(searchTrackKey, () => () => undefined)

// childPagerIndex returns the unique-result count to show on the pager that precedes a child, or undefined
// when that child gets no pager. The child may be a leaf or a group: a pager landing at a group's start is
// keyed to the group node so it renders above the heading. The pager is its own list item before the child,
// a standalone flex item with the same spacing above and below as the flat view's pager.
function childPagerIndex(child: DeepReadonly<Result>): number | undefined {
  return pager.value.pagerBefore.get(toRaw(child))
}
</script>

<template>
  <div v-if="node.group" class="pd-searchresultgroup flex flex-col gap-y-1 sm:gap-y-4">
    <!--
      An expanded group value shows the full result card for its document. It is not a search result and is
      not registered with the visibility tracker, so the "at" scroll position keeps following the leaves.
    -->
    <SearchResult v-if="expanded" :search-session-id="searchSessionId" :result="node">
      <template #labelAside>
        <span class="flex shrink-0 items-baseline gap-x-1 text-base font-normal text-slate-500">
          <span v-if="node.count != null">({{ node.count }})</span>
          <button
            type="button"
            class="self-center rounded-sm p-0.5 text-slate-400 outline-none hover:bg-slate-200 hover:text-slate-600 focus:ring-2 focus:ring-primary-500"
            :title="t('partials.SearchResultGroup.collapse')"
            @click.prevent="setExpand(depth, false)"
          >
            <ChevronDownUpIcon class="size-5" :alt="t('partials.SearchResultGroup.collapse')" />
          </button>
        </span>
      </template>
    </SearchResult>
    <div v-else class="pd-searchresultgroup-header flex items-baseline gap-x-1 border-b border-slate-200 py-1 font-semibold text-slate-700">
      <i v-if="node.id === '__MISSING__'" class="min-w-0 truncate">{{ t("common.values.missing") }}</i>
      <DocumentRefInline v-else :id="node.id" class="min-w-0 truncate" />
      <span v-if="node.count != null" class="shrink-0 font-normal text-slate-500">({{ node.count }})</span>
      <button
        v-if="!levelExpanded"
        type="button"
        class="shrink-0 self-center rounded-sm p-0.5 font-normal text-slate-400 outline-none hover:bg-slate-200 hover:text-slate-600 focus:ring-2 focus:ring-primary-500"
        :title="t('partials.SearchResultGroup.expand')"
        @click.prevent="setExpand(depth, true)"
      >
        <ChevronUpDownIcon class="size-5" :alt="t('partials.SearchResultGroup.expand')" />
      </button>
    </div>
    <ul class="flex flex-col gap-y-1 pl-4 sm:gap-y-4 sm:pl-6">
      <template v-for="(child, i) in node.group" :key="`${child.id}-${i}`">
        <li v-if="childPagerIndex(child) !== undefined" class="pd-print-hidden">
          <SearchResultsPager :i="childPagerIndex(child)!" :shown="pager.shown" :total="pager.total" :depth="depth + 1" />
        </li>
        <li>
          <SearchResultGroup :node="child" :search-session-id="searchSessionId" :depth="depth + 1" :expand-levels="expandLevels" />
        </li>
      </template>
    </ul>
  </div>
  <SearchResult v-else :ref="track(node.id)" :search-session-id="searchSessionId" :result="node" :duplicate="isDuplicate(node)" />
</template>
