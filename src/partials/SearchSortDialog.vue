<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { AmountSearchResult, FilterResult, RefSearchResult, SearchSession, SortColumn, SortKey, TimeSearchResult } from "@/types"

import { Dialog, DialogPanel } from "@headlessui/vue"
import { BarsArrowDownIcon, BarsArrowUpIcon, ChevronDownIcon, ChevronUpIcon, PlusIcon, TrashIcon, XMarkIcon } from "@heroicons/vue/20/solid"
import { computed } from "vue"
import { useI18n } from "vue-i18n"

import Button from "@/components/Button.vue"
import CheckBox from "@/components/CheckBox.vue"
import DocumentRefInline from "@/partials/DocumentRefInline.vue"
import { clone } from "@/utils"

const props = defineProps<{
  open: boolean
  searchSession: DeepReadonly<SearchSession>
  // The session's available filter columns, used as additional sortable columns.
  filterColumns: DeepReadonly<FilterResult[]>
}>()

const $emit = defineEmits<{
  close: []
  sortUpdate: [sort: SortKey[]]
}>()

const { t } = useI18n({ useScope: "global" })

// Built-in columns and their natural default direction (relevance and time high-to-low, label a-z).
const builtinColumns: { col: SortColumn; descending: boolean }[] = [
  { col: { type: "score" }, descending: true },
  { col: { type: "time" }, descending: true },
  { col: { type: "label" }, descending: false },
]

// colKey identifies a column. Built-in "time" (no prop) and a "time" filter column (with prop) get
// distinct keys.
function colKey(c: { type: string; prop?: readonly string[]; unit?: string }): string {
  return `${c.type}|${c.prop?.join("/") ?? ""}|${c.unit ?? ""}`
}

const sort = computed<DeepReadonly<SortKey[]>>(() => props.searchSession.sort ?? [])

// Filter columns usable for sorting: top-level ref/amount/time filters (has-filters have no orderable value).
function isSortableFilter(f: DeepReadonly<FilterResult>): f is DeepReadonly<RefSearchResult | AmountSearchResult | TimeSearchResult> {
  return (f.type === "ref" || f.type === "amount" || f.type === "time") && f.props.length === 1
}

const filterSortColumns = computed<SortColumn[]>(() =>
  props.filterColumns.filter(isSortableFilter).map((f) => ({
    type: f.type,
    prop: [...f.props],
    ...(f.type === "amount" && f.unit ? { unit: f.unit } : {}),
  })),
)

// Columns not yet in the sort order, offered for adding.
const availableColumns = computed<{ col: SortColumn; descending: boolean }[]>(() => {
  const used = new Set(sort.value.map((k) => colKey(k)))
  const all = [...builtinColumns, ...filterSortColumns.value.map((col) => ({ col, descending: false }))]
  return all.filter(({ col }) => !used.has(colKey(col)))
})

// normalizeGroups enforces the invariant that grouped columns are a leading contiguous run of ref columns.
function normalizeGroups(s: SortKey[]): SortKey[] {
  let run = true
  for (const k of s) {
    if (run && k.type === "ref" && k.group) {
      continue
    }
    run = false
    k.group = false
  }
  return s
}

function emitSort(newSort: SortKey[]): void {
  $emit("sortUpdate", normalizeGroups(newSort))
}

function addColumn(col: SortColumn, descending: boolean): void {
  const newSort = clone(sort.value)
  newSort.push({ ...col, descending })
  emitSort(newSort)
}

function removeColumn(i: number): void {
  emitSort(clone(sort.value).filter((_, j) => j !== i))
}

function move(i: number, delta: number): void {
  const j = i + delta
  if (j < 0 || j >= sort.value.length) {
    return
  }
  const newSort = clone(sort.value)
  ;[newSort[i], newSort[j]] = [newSort[j], newSort[i]]
  emitSort(newSort)
}

function toggleDirection(i: number): void {
  const newSort = clone(sort.value)
  newSort[i].descending = !newSort[i].descending
  emitSort(newSort)
}

// canGroup reports whether the column at i may be grouped: it is a ref column and every column before it
// is also a ref column (so the grouped columns can stay a leading run).
function canGroup(i: number): boolean {
  if (sort.value[i].type !== "ref") {
    return false
  }
  for (let j = 0; j < i; j++) {
    if (sort.value[j].type !== "ref") {
      return false
    }
  }
  return true
}

function toggleGroup(i: number): void {
  const newSort = clone(sort.value)
  if (newSort[i].group) {
    // Turning a column off ungroups it and everything after it, keeping grouped columns leading.
    for (let j = i; j < newSort.length; j++) {
      newSort[j].group = false
    }
  } else {
    // Turning a column on groups it and every column before it (which canGroup guarantees are ref).
    for (let j = 0; j <= i; j++) {
      newSort[j].group = true
    }
  }
  emitSort(newSort)
}

function builtinLabel(type: string): string {
  switch (type) {
    case "score":
      return t("partials.SearchSortDialog.columns.score")
    case "time":
      return t("partials.SearchSortDialog.columns.time")
    default:
      return t("partials.SearchSortDialog.columns.label")
  }
}
</script>

<template>
  <Dialog as="div" class="pd-searchsortdialog relative z-50" :open="open" @close="$emit('close')">
    <div class="fixed inset-0 bg-black/30" aria-hidden="true" />
    <div class="fixed inset-0 flex items-center justify-center">
      <DialogPanel
        class="flex h-full w-full flex-col overflow-y-auto rounded-none bg-white p-1 shadow-none sm:relative sm:inset-auto sm:h-auto sm:max-h-150 sm:max-w-xl sm:rounded-sm sm:p-4 sm:shadow-sm"
      >
        <h2 class="mb-4 text-lg font-medium">{{ t("partials.SearchSortDialog.title") }}</h2>

        <h3 class="text-sm font-semibold text-slate-700">{{ t("partials.SearchSortDialog.sortOrder") }}</h3>
        <p v-if="sort.length === 0" class="mt-1 text-sm text-slate-500">{{ t("partials.SearchSortDialog.empty") }}</p>
        <ul v-else class="mt-2 flex flex-col gap-y-1">
          <li v-for="(key, i) in sort" :key="colKey(key)" class="flex items-center gap-x-2 rounded-sm border border-slate-200 bg-slate-50 p-2">
            <div class="flex flex-col">
              <button
                type="button"
                class="rounded-sm outline-none hover:bg-slate-200 focus:ring-2 focus:ring-primary-500 disabled:cursor-not-allowed disabled:text-slate-300"
                :disabled="i === 0"
                :title="t('common.buttons.moveUp')"
                @click.prevent="move(i, -1)"
              >
                <ChevronUpIcon class="size-4" :alt="t('common.buttons.moveUp')" />
              </button>
              <button
                type="button"
                class="rounded-sm outline-none hover:bg-slate-200 focus:ring-2 focus:ring-primary-500 disabled:cursor-not-allowed disabled:text-slate-300"
                :disabled="i === sort.length - 1"
                :title="t('common.buttons.moveDown')"
                @click.prevent="move(i, 1)"
              >
                <ChevronDownIcon class="size-4" :alt="t('common.buttons.moveDown')" />
              </button>
            </div>
            <span class="min-w-0 grow truncate">
              <i18n-t v-if="key.prop && key.unit" keypath="common.labelWithUnit" scope="global" tag="span">
                <template #label><DocumentRefInline :id="key.prop[0]" :link="false" /></template>
                <template #unit><DocumentRefInline :id="key.unit" :link="false" /></template>
              </i18n-t>
              <DocumentRefInline v-else-if="key.prop" :id="key.prop[0]" :link="false" />
              <i v-else>{{ builtinLabel(key.type) }}</i>
            </span>
            <button
              type="button"
              class="shrink-0 rounded-sm p-1 outline-none hover:bg-slate-200 focus:ring-2 focus:ring-primary-500"
              :title="key.descending ? t('partials.SearchSortDialog.descending') : t('partials.SearchSortDialog.ascending')"
              @click.prevent="toggleDirection(i)"
            >
              <BarsArrowDownIcon v-if="key.descending" class="size-5" :alt="t('partials.SearchSortDialog.descending')" />
              <BarsArrowUpIcon v-else class="size-5" :alt="t('partials.SearchSortDialog.ascending')" />
            </button>
            <label v-if="canGroup(i)" class="flex shrink-0 cursor-pointer items-center gap-x-1 text-sm">
              <CheckBox :model-value="key.group ?? false" @update:model-value="toggleGroup(i)" />
              {{ t("partials.SearchSortDialog.group") }}
            </label>
            <button
              type="button"
              class="shrink-0 rounded-sm p-1 text-error-600 outline-none hover:bg-error-50 focus:ring-2 focus:ring-error-500"
              :title="t('common.buttons.remove')"
              @click.prevent="removeColumn(i)"
            >
              <TrashIcon class="size-5" :alt="t('common.buttons.remove')" />
            </button>
          </li>
        </ul>

        <template v-if="availableColumns.length > 0">
          <h3 class="mt-4 text-sm font-semibold text-slate-700">{{ t("partials.SearchSortDialog.addColumn") }}</h3>
          <ul class="mt-2 flex flex-col gap-y-1">
            <li v-for="entry in availableColumns" :key="colKey(entry.col)">
              <button
                type="button"
                class="flex w-full items-center gap-x-1 rounded-sm p-2 text-left outline-none hover:bg-slate-100 focus:ring-2 focus:ring-primary-500"
                @click.prevent="addColumn(entry.col, entry.descending)"
              >
                <PlusIcon class="size-4 shrink-0 text-primary-600" :alt="t('partials.SearchSortDialog.addColumn')" />
                <span class="min-w-0 grow truncate">
                  <i18n-t v-if="entry.col.prop && entry.col.unit" keypath="common.labelWithUnit" scope="global" tag="span">
                    <template #label><DocumentRefInline :id="entry.col.prop[0]" :link="false" /></template>
                    <template #unit><DocumentRefInline :id="entry.col.unit" :link="false" /></template>
                  </i18n-t>
                  <DocumentRefInline v-else-if="entry.col.prop" :id="entry.col.prop[0]" :link="false" />
                  <i v-else>{{ builtinLabel(entry.col.type) }}</i>
                </span>
              </button>
            </li>
          </ul>
        </template>

        <Button class="absolute top-1 right-1 p-0 shadow-none inset-ring-0 sm:top-4 sm:right-4" :title="t('common.buttons.close')" @click.prevent="$emit('close')">
          <XMarkIcon class="size-5" :alt="t('common.buttons.close')" />
        </Button>
      </DialogPanel>
    </div>
  </Dialog>
</template>
