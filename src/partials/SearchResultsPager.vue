<script setup lang="ts">
import { computed } from "vue"
import { useI18n } from "vue-i18n"

const props = withDefaults(
  defineProps<{
    // Number of results that precede this pager (10, 20, ...).
    i: number
    // Total number of results currently loaded; the bar fills to i/shown.
    shown: number
    // Number of matching documents, used to tell whether all results are shown.
    total: number
    // Result nesting depth in the grouped view. The pager cancels that many levels of left indentation
    // (each var(--pd-indent) wide) so it still spans the whole results column. 0 (no breakout) in the flat view.
    depth?: number
  }>(),
  {
    depth: 0,
  },
)

const { t } = useI18n({ useScope: "global" })

// Full-bleed across the results column: cancel the cumulative left indentation of the enclosing groups so a
// pager that falls inside a nested group still spans the same width as in the flat view.
const breakoutStyle = computed(() => (props.depth > 0 ? { marginLeft: `calc(var(--pd-indent) * -${props.depth})` } : undefined))
</script>

<template>
  <div class="pd-pager pd-print-hidden my-1 sm:my-4" :style="breakoutStyle">
    <div v-if="shown < total" class="pd-count text-center text-sm">{{ t("partials.SearchResultsFeed.shownResultsOnly", { i, count: shown }) }}</div>
    <div v-else-if="shown === total" class="pd-count text-center text-sm">{{ t("partials.SearchResultsFeed.shownResults", { i, count: shown }) }}</div>
    <!-- We do not use ProgressBar here because we plan to make this an interactive bar on which you can click to move to that location. -->
    <div class="pd-track relative h-2 w-full bg-slate-200">
      <div class="pd-thumb absolute inset-y-0 left-0 bg-secondary-400" :style="{ width: (i / shown) * 100 + '%' }" />
    </div>
  </div>
</template>
