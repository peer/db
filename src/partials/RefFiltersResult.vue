<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { RefFilterEntry, RefFilterResult, RefFilterTreeNode, RefSearchResult, SearchSession, ToValue } from "@/types"

import { computed, onBeforeUnmount, ref, toRef, useId, useTemplateRef } from "vue"
import { useI18n } from "vue-i18n"

import Button from "@/components/Button.vue"
import FilterPropLabel from "@/partials/FilterPropLabel.vue"
import RefFilterTreeRow from "@/partials/RefFilterTreeRow.vue"
import { useProgress } from "@/progress"
import { FILTERS_INCREASE, FILTERS_INITIAL_LIMIT, useRefFilterMatches, useRefFilters } from "@/search"
import {
  buildRefTree,
  computeRefCheckStates,
  DIRECT_REF_FILTER_PREFIX,
  equals,
  loadingWidth,
  MISSING_VALUE_ID,
  refOverlayVisibleIds,
  SKIP_TO_END,
  toggleRefSelection,
  useInitialLoad,
  useLimitResults,
  useReportFilterVisibility,
} from "@/utils"

type FlatEntry = { node: RefFilterTreeNode; depth: number }

const props = withDefaults(
  defineProps<{
    searchSession: DeepReadonly<SearchSession>
    result: RefSearchResult
    filter?: RefFilterEntry
    // Free-text query that narrows the listed values to those whose name matches it; when it matches this
    // facet's own property name instead, all values are shown. Empty means no narrowing.
    query?: string
  }>(),
  {
    filter: undefined,
    query: "",
  },
)

const emit = defineEmits<{
  filterUpdate: [filterId: string, filter: RefFilterEntry]
}>()

const { t } = useI18n({ useScope: "global" })

const el = useTemplateRef<HTMLElement>("el")

const labelId = useId()

const abortController = new AbortController()

onBeforeUnmount(() => {
  abortController.abort()
})

// Data loading only, no controls.
const progress = useProgress()

// The filter ID from the session's filter, if it exists.
const filterId = computed(() => props.filter?.id ?? "")

// Composite key uniquely identifying this filter panel (all props joined).
// For single-prop ref filters this is just the prop ID; for sub-ref filters
// it is "parentProp/prop" so it does not collide with the parent ref filter panel.
const propsKey = computed(() => props.result.props.join("/"))

const session = toRef(() => props.searchSession)
const propsRef = computed(() => props.result.props)

// EMPTY is a stable, always-empty value query so the primary facet always fetches the unfiltered (q="")
// results and refetches only when the session/version or props change. The primary results are the single
// source of counts, the value tree and the checkbox states; the value search is layered on top as a visual
// overlay that only hides values, never recomputes them.
const EMPTY = ref("")

const primary = useRefFilters(session, filterId, propsRef, EMPTY, el, progress)
const matches = useRefFilterMatches(
  session,
  filterId,
  propsRef,
  toRef(() => props.query),
  el,
  progress,
)

// Template-facing aliases for the primary facet (top-level refs unwrap in the template).
const error = primary.error
const resultsUrl = primary.url
const loading = computed(() => primary.total.value === null)

const { laterLoad } = useInitialLoad(progress)

const searching = computed(() => props.query !== "")

// The overlay is active only once the value search has been typed and its matches have returned. While the
// match fetch is still in flight (matches.total is null) the overlay is inactive, so all primary values stay
// shown and the facet does not flicker to a hidden or empty state.
const overlayActive = computed(() => searching.value && matches.total.value !== null)

// The ids that stay visible under the active value search, or null to show everything. It is computed over
// the full primary results so a match keeps its tree path (ancestors) and its "direct" entry.
const visibleIds = computed(() => (overlayActive.value ? refOverlayVisibleIds(primary.results.value as RefFilterResult[], matches.matchedIds.value) : null))

// Extract the selected "to" IDs from the filter value.
const selectedIds = computed((): string[] => {
  if (!props.filter?.ref?.to) {
    return []
  }
  return props.filter.ref.to.map((t: ToValue) => t.id)
})

// The values selected through their "direct" option. In the checkbox state each is carried as a
// "__DIRECT__:" + value token, the same way "__MISSING__" carries the missing selection.
const selectedDirectIds = computed((): string[] => {
  if (!props.filter?.ref?.direct) {
    return []
  }
  return props.filter.ref.direct.map((t: ToValue) => t.id)
})

const isMissingSelected = computed((): boolean => {
  return props.filter?.ref?.missing === true
})

const checkboxState = computed({
  get(): string[] {
    const ids = [...selectedIds.value]
    for (const id of selectedDirectIds.value) {
      ids.push(DIRECT_REF_FILTER_PREFIX + id)
    }
    if (isMissingSelected.value) {
      ids.push(MISSING_VALUE_ID)
    }
    return ids
  },
  set(value: string[]) {
    if (abortController.signal.aborted) {
      return
    }

    const missingSelected = value.includes(MISSING_VALUE_ID)
    const directIds = value.filter((v) => v.startsWith(DIRECT_REF_FILTER_PREFIX)).map((v) => v.slice(DIRECT_REF_FILTER_PREFIX.length))
    const toIds = value.filter((v) => v !== MISSING_VALUE_ID && !v.startsWith(DIRECT_REF_FILTER_PREFIX))
    const to: ToValue[] | undefined = toIds.length > 0 ? toIds.map((id) => ({ id })) : undefined
    const direct: ToValue[] | undefined = directIds.length > 0 ? directIds.map((id) => ({ id })) : undefined
    const missing = missingSelected ? true : undefined

    // Build the updated filter.
    const updatedFilter: RefFilterEntry = {
      id: props.filter?.id ?? "",
      base: props.filter?.base ?? [],
      prop: props.filter?.prop ?? [...props.result.props],
      ref: { to, direct, missing },
    }

    if (!equals(props.filter, updatedFilter)) {
      emit("filterUpdate", updatedFilter.id, updatedFilter)
    }
  },
})

const selectedSet = computed(() => new Set<string>(checkboxState.value))

// Build the static tree from the full primary result set. Iteration order is the count-desc order returned by
// the API, which buildRefTree preserves while placing each value under its deepest already-placed ancestor
// (duplicated under each parent for diamond hierarchies). The value search never narrows this, so the tree and
// the check states it feeds always cover the whole facet.
const tree = computed((): RefFilterTreeNode[] => buildRefTree(primary.results.value as RefFilterResult[]))

// Bottom-up "any of this subtree (including self) is selected" map. A node
// counts as "selected" for sort purposes when its own id is in the selection
// or any of its descendants is, which covers both fully-checked and
// indeterminate visuals.
function buildHasSelected(nodes: RefFilterTreeNode[], selected: ReadonlySet<string>, out: Map<RefFilterTreeNode, boolean>): boolean {
  let any = false
  for (const node of nodes) {
    const childHas = buildHasSelected(node.children, selected, out)
    const has = childHas || selected.has(node.res.id)
    out.set(node, has)
    if (has) {
      any = true
    }
  }
  return any
}

const hasSelectedInSubtree = computed(() => {
  const out = new Map<RefFilterTreeNode, boolean>()
  buildHasSelected(tree.value, selectedSet.value, out)
  return out
})

// Pre-order DFS that sorts each level by (any subtree selection first, then
// the original count-desc order). Fully checked and indeterminate nodes both
// bubble to the top of their sibling group.
function flattenSorted(nodes: RefFilterTreeNode[], depth: number, hasSelected: Map<RefFilterTreeNode, boolean>, out: FlatEntry[]): void {
  const ordered = [...nodes]
  ordered.sort((a, b) => {
    const aSel = hasSelected.get(a) ? 0 : 1
    const bSel = hasSelected.get(b) ? 0 : 1
    return aSel - bSel
  })
  for (const node of ordered) {
    out.push({ node, depth })
    if (node.children.length > 0) {
      flattenSorted(node.children, depth + 1, hasSelected, out)
    }
  }
}

const flatTree = computed((): FlatEntry[] => {
  const out: FlatEntry[] = []
  flattenSorted(tree.value, 0, hasSelectedInSubtree.value, out)
  return out
})

// The flat tree narrowed to the value search overlay: every entry stays when the overlay is inactive,
// otherwise only entries whose value id is visible. Ancestors of a match are in the visible set, so no visible
// node ever loses its parent and the tree stays valid.
const visibleFlatTree = computed((): FlatEntry[] => {
  const ids = visibleIds.value
  if (ids === null) {
    return flatTree.value
  }
  return flatTree.value.filter((e) => ids.has(e.node.res.id))
})

// While the value search is active and nothing in this facet is visible, the whole facet is hidden so the
// filter pane shows only facets with matching values. Until the overlay is active the facet stays visible.
const hiddenByQuery = computed(() => overlayActive.value && visibleFlatTree.value.length === 0)

// Report visibility to the filter pane so it can show the no-match message only when no facet is visible.
useReportFilterVisibility(() => !hiddenByQuery.value)

const { limitedResults, hasMore, loadMore } = useLimitResults(visibleFlatTree, FILTERS_INITIAL_LIMIT, FILTERS_INCREASE)

// Distinct filter values within the paginated slice. Diamond duplicates (the same
// value rendered under multiple parents) collapse to one here, so this can trail
// the slice length. It only drives the SKIP_TO_END decision below: how many
// distinct options are still hidden behind the row limit.
const limitedUnique = computed(() => new Set(limitedResults.value.map((e) => e.node.res.id)).size)

// effectiveLimited is what we actually render. It mirrors useLimitResults'
// SKIP_TO_END short-circuit at the unique-options layer: when SKIP_TO_END or
// fewer reachable options are still hidden, expose every remaining visible tree
// row in one go.
const effectiveLimited = computed((): FlatEntry[] => {
  if (visibleFlatTree.value.length - limitedUnique.value <= SKIP_TO_END) {
    return visibleFlatTree.value
  }
  return limitedResults.value as FlatEntry[]
})

// Distinct filter values actually rendered. optionsRemaining is measured against
// this rather than the pre-short-circuit slice: once effectiveLimited expands to
// the full tree every value is on screen, so the remaining count must reach zero.
// Measuring against limitedResults instead would report the diamond-duplicate gap
// between the slice length and its distinct-value count as phantom values "not shown".
const shownUnique = computed(() => new Set(effectiveLimited.value.map((e) => e.node.res.id)).size)

// The number of distinct options the user can still reach: the whole facet count from the primary results when
// not searching, or the distinct count of the currently visible overlay when a value search is active.
const displayTotal = computed(() => (visibleIds.value === null ? primary.total.value : new Set(visibleFlatTree.value.map((e) => e.node.res.id)).size))

const optionsRemaining = computed(() => {
  if (displayTotal.value === null) {
    return 0
  }
  return Math.max(0, displayTotal.value - shownUnique.value)
})

// The render-time tree: a stack walk over effectiveLimited that rebuilds parent
// links only for visible nodes. Hidden subtrees are simply absent.
const partialTree = computed((): RefFilterTreeNode[] => {
  const roots: RefFilterTreeNode[] = []
  const stack: RefFilterTreeNode[] = []
  for (const { node, depth } of effectiveLimited.value) {
    const cloned: RefFilterTreeNode = { res: node.res, key: node.key, children: [] }
    if (depth === 0) {
      roots.push(cloned)
    } else {
      const parent = stack[depth - 1]
      parent.children.push(cloned)
    }
    stack[depth] = cloned
    stack.length = depth + 1
  }
  return roots
})

// Whether anything is still hidden behind the row limit.
const moreRowsAvailable = computed(() => effectiveLimited.value.length < visibleFlatTree.value.length)

function clearFilter() {
  if (abortController.signal.aborted || !props.filter) {
    return
  }
  emit("filterUpdate", props.filter.id, {
    id: props.filter.id,
    base: props.filter.base,
    prop: props.filter.prop,
    ref: { to: undefined, direct: undefined, missing: undefined },
  })
}

// Per-value tri-state for rendering: a value is checked when its own value, or an ancestor, is
// selected, or when all of its children are; indeterminate when only part of its subtree is. See
// computeRefCheckStates. The whole facet's primary results feed it, so a value's state does not depend on
// whether the rows under it are currently paginated into view nor on the active value search.
const checkStates = computed(() => computeRefCheckStates(primary.results.value as RefFilterResult[], selectedSet.value))

// Clicking a node toggles its whole subtree. Clicking an unchecked or indeterminate value selects
// the value, its narrower values and its "direct" entry; clicking a checked value deselects that
// subtree, decomposing any broader ancestor selection into its still-selected siblings.
// The selection round-trips through checkboxState into the filter.
function onToggle(node: RefFilterTreeNode) {
  if (abortController.signal.aborted) {
    return
  }
  checkboxState.value = [...toggleRefSelection(primary.results.value as RefFilterResult[], node.res.id, selectedSet.value)]
}
</script>

<template>
  <div v-if="!hiddenByQuery" class="pd-reffiltersresult flex flex-col" :class="{ 'data-reloading': laterLoad }" :data-url="resultsUrl">
    <div :id="labelId">
      <Button
        v-if="filter"
        type="button"
        class="float-right ml-2 px-2.5 py-1"
        :title="t('partials.RefFiltersResult.clearFilter')"
        :aria-label="t('partials.RefFiltersResult.clearFilter')"
        @click.prevent="clearFilter"
        >{{ t("common.buttons.clear") }}</Button
      >
      <span class="mb-1.5 text-lg leading-none"><FilterPropLabel :prop-ids="result.props" /></span>
      ({{ result.count }})
    </div>
    <ul ref="el" role="group" :aria-labelledby="labelId" class="flex flex-col">
      <li v-if="error">
        <i class="pd-reffiltersresult-error text-error-600">{{ t("common.status.loadingDataFailed") }}</i>
      </li>
      <template v-else-if="loading">
        <li v-for="i in 3" :key="i" class="flex items-baseline gap-x-1" aria-hidden="true">
          <div class="my-1.5 h-2 w-4 rounded-sm bg-slate-200 motion-safe:animate-pulse"></div>
          <div class="my-1.5 h-2 rounded-sm bg-slate-200 motion-safe:animate-pulse" :class="[loadingWidth(`${propsKey}/${i}`)]"></div>
          <div class="my-1.5 h-2 w-8 rounded-sm bg-slate-200 motion-safe:animate-pulse"></div>
        </li>
      </template>
      <template v-else>
        <RefFilterTreeRow v-for="node in partialTree" :key="node.key" :node="node" :props-key="propsKey" :check-states="checkStates" :on-toggle="onToggle" />
      </template>
    </ul>
    <Button v-if="!loading && hasMore && moreRowsAvailable && optionsRemaining > 0" primary class="mt-2 w-1/2 min-w-fit self-center" @click.prevent="loadMore">{{
      t("common.buttons.loadCountMore", { count: optionsRemaining })
    }}</Button>
    <div v-else-if="!loading && optionsRemaining > 0" class="mt-2 text-center text-sm">
      {{ t("common.status.valuesNotShown", { count: optionsRemaining }) }}
    </div>
  </div>
</template>
