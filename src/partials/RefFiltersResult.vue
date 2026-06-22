<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { RefFilterEntry, RefFilterResult, RefFilterTreeNode, RefSearchResult, SearchSession, ToValue } from "@/types"

import { computed, onBeforeUnmount, toRef, useId, useTemplateRef } from "vue"
import { useI18n } from "vue-i18n"

import Button from "@/components/Button.vue"
import DocumentRefInline from "@/partials/DocumentRefInline.vue"
import RefFilterTreeRow from "@/partials/RefFilterTreeRow.vue"
import { useProgress } from "@/progress"
import { FILTERS_INCREASE, FILTERS_INITIAL_LIMIT, useRefFilters } from "@/search"
import { computeRefCheckStates, equals, loadingWidth, SKIP_TO_END, toggleRefSelection, useInitialLoad, useLimitResults } from "@/utils"

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

const {
  results,
  total,
  error,
  url: resultsUrl,
} = useRefFilters(
  toRef(() => props.searchSession),
  filterId,
  computed(() => props.result.props),
  toRef(() => props.query),
  el,
  progress,
)
const { laterLoad } = useInitialLoad(progress)

// While a value query is active and no value matches, the whole facet is hidden so the filter pane shows
// only facets with matching values. During loading (total still null) the facet stays visible.
const hiddenByQuery = computed(() => props.query !== "" && total.value === 0)

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
      ids.push("__DIRECT__:" + id)
    }
    if (isMissingSelected.value) {
      ids.push("__MISSING__")
    }
    return ids
  },
  set(value: string[]) {
    if (abortController.signal.aborted) {
      return
    }

    const missingSelected = value.includes("__MISSING__")
    const directIds = value.filter((v) => v.startsWith("__DIRECT__:")).map((v) => v.slice("__DIRECT__:".length))
    const toIds = value.filter((v) => v !== "__MISSING__" && !v.startsWith("__DIRECT__:"))
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

// Build the static tree from the full result set. Iteration order is the
// count-desc order returned by the API. For each result, find which of its paths
// reaches an already-placed ancestor; attach under each such ancestor as a child,
// or push as a root. Diamond duplicates share res.id with their canonical
// placement and only carry rendered children at the canonical position.
const tree = computed((): RefFilterTreeNode[] => {
  const roots: RefFilterTreeNode[] = []
  const firstNodeById: Record<string, RefFilterTreeNode> = {}
  for (const res of results.value as RefFilterResult[]) {
    const paths = res.paths ?? []
    const attachTo: RefFilterTreeNode[] = []
    const seenAncestorIds = new Set<string>()
    for (const path of paths) {
      for (let i = path.length - 1; i >= 0; i--) {
        const ancestorId = path[i]
        if (firstNodeById[ancestorId]) {
          if (!seenAncestorIds.has(ancestorId)) {
            attachTo.push(firstNodeById[ancestorId])
            seenAncestorIds.add(ancestorId)
          }
          break
        }
      }
    }
    if (attachTo.length === 0) {
      const node: RefFilterTreeNode = { res, key: res.id, children: [] }
      roots.push(node)
      if (!firstNodeById[res.id]) {
        firstNodeById[res.id] = node
      }
    } else {
      attachTo.forEach((ancestorNode, idx) => {
        const key = idx === 0 ? res.id : res.id + "|" + ancestorNode.key
        const node: RefFilterTreeNode = { res, key, children: [] }
        ancestorNode.children.push(node)
        if (!firstNodeById[res.id]) {
          firstNodeById[res.id] = node
        }
      })
    }
  }
  return roots
})

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

const { limitedResults, hasMore, loadMore } = useLimitResults(flatTree, FILTERS_INITIAL_LIMIT, FILTERS_INCREASE)

// Distinct filter values within the paginated slice. Diamond duplicates (the same
// value rendered under multiple parents) collapse to one here, so this can trail
// the slice length. It only drives the SKIP_TO_END decision below: how many
// distinct options are still hidden behind the row limit.
const limitedUnique = computed(() => new Set(limitedResults.value.map((e) => e.node.res.id)).size)

// effectiveLimited is what we actually render. It mirrors useLimitResults'
// SKIP_TO_END short-circuit at the unique-options layer: when SKIP_TO_END or
// fewer reachable options are still hidden, expose every remaining tree row in
// one go.
const effectiveLimited = computed((): FlatEntry[] => {
  if (results.value.length - limitedUnique.value <= SKIP_TO_END) {
    return flatTree.value
  }
  return limitedResults.value as FlatEntry[]
})

// Distinct filter values actually rendered. optionsRemaining is measured against
// this rather than the pre-short-circuit slice: once effectiveLimited expands to
// the full tree every value is on screen, so the remaining count must reach zero.
// Measuring against limitedResults instead would report the diamond-duplicate gap
// between the slice length and its distinct-value count as phantom values "not shown".
const shownUnique = computed(() => new Set(effectiveLimited.value.map((e) => e.node.res.id)).size)

const optionsRemaining = computed(() => {
  if (total.value === null) {
    return 0
  }
  return Math.max(0, total.value - shownUnique.value)
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
const moreRowsAvailable = computed(() => effectiveLimited.value.length < flatTree.value.length)

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
// computeRefCheckStates. The whole panel's results feed it, so a value's state does not depend on
// whether the rows under it are currently paginated into view.
const checkStates = computed(() => computeRefCheckStates(results.value as RefFilterResult[], selectedSet.value))

// Clicking a node toggles its whole subtree. Clicking an unchecked or indeterminate value selects
// the value, its narrower values and its "direct" entry; clicking a checked value deselects that
// subtree, decomposing any broader ancestor selection into its still-selected siblings.
// The selection round-trips through checkboxState into the filter.
function onToggle(node: RefFilterTreeNode) {
  if (abortController.signal.aborted) {
    return
  }
  checkboxState.value = [...toggleRefSelection(results.value as RefFilterResult[], node.res.id, selectedSet.value)]
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
      <span v-if="result.props.length === 2" class="mb-1.5 text-lg leading-none">
        <DocumentRefInline :id="result.props[0]" />
        &gt;
        <DocumentRefInline :id="result.props[1]" />
      </span>
      <DocumentRefInline v-else :id="result.props[0]" class="mb-1.5 text-lg leading-none" />
      ({{ result.count }})
    </div>
    <ul ref="el" role="group" :aria-labelledby="labelId" class="flex flex-col">
      <li v-if="error">
        <i class="pd-reffiltersresult-error text-error-600">{{ t("common.status.loadingDataFailed") }}</i>
      </li>
      <template v-else-if="total === null">
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
    <Button v-if="total !== null && hasMore && moreRowsAvailable && optionsRemaining > 0" primary class="mt-2 w-1/2 min-w-fit self-center" @click.prevent="loadMore">{{
      t("common.buttons.loadCountMore", { count: optionsRemaining })
    }}</Button>
    <div v-else-if="total !== null && optionsRemaining > 0" class="mt-2 text-center text-sm">
      {{ t("common.status.valuesNotShown", { count: optionsRemaining }) }}
    </div>
  </div>
</template>
