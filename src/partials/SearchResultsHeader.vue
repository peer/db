<script setup lang="ts">
import { computed, DeepReadonly, ref } from "vue"
import type { ClientSearchState } from "@/types"
import SelectButton, { SelectButtonOptionsProp } from "@/components/SelectButton.vue"
import { Bars4Icon, TableCellsIcon } from "@heroicons/vue/20/solid"

export type SearchViewType = "table" | "snippet"

const props = defineProps<{
  state: DeepReadonly<ClientSearchState | null>
  total: number | null
  results: number
  moreThanTotal: boolean
  view: SearchViewType
}>()

const $emit = defineEmits<{
  "update:view": [value: SearchViewType]
}>()

const selectButtonValue = computed({
  get() {
    return props.view
  },
  set(newValue) {
    $emit("update:view", newValue)
  },
})

const selectButtonOptions = ref<SelectButtonOptionsProp<SearchViewType>[]>([
  {
    icon: {
      component: Bars4Icon,
      alt: "Snippet",
    },
    value: "snippet",
  },
  {
    icon: {
      component: TableCellsIcon,
      alt: "Table",
    },
    value: "table",
  },
])

function countFilters(): number {
  if (!props.state) {
    return 0
  }
  if (!props.state.filters) {
    return 0
  }

  let n = 0
  for (const values of Object.values(props.state.filters.rel)) {
    n += values.length
  }
  for (const value of Object.values(props.state.filters.amount)) {
    if (value) {
      n++
    }
  }
  for (const value of Object.values(props.state.filters.time)) {
    if (value) {
      n++
    }
  }
  for (const values of Object.values(props.state.filters.str)) {
    n += values.length
  }
  if (props.state.filters.index) {
    n += props.state.filters.index.length
  }
  if (props.state.filters.size) {
    n++
  }
  return n
}
</script>

<template>
  <div class="flex gap-4 items-center">
    <div class="bg-slate-200 px-4 py-2 rounded flex flex-row justify-between w-full">
      <div v-if="state === null">Loading...</div>
      <div v-else-if="state.promptError">Error interpreting your prompt.</div>
      <div v-else-if="state.p && !state.promptDone">Interpreting your prompt...</div>
      <div v-else-if="state.q && countFilters() === 1">
        Searching query <i>{{ state.q }}</i> and 1 active filter
        <template v-if="total === null">...</template>
        <template v-else>.</template>
      </div>
      <div v-else-if="state.q">
        Searching query <i>{{ state.q }}</i> and {{ countFilters() }} active filters
        <template v-if="total === null">...</template>
        <template v-else>.</template>
      </div>
      <div v-else-if="countFilters() === 1">
        Searching without query and with 1 active filter
        <template v-if="total === null">...</template>
        <template v-else>.</template>
      </div>
      <div v-else>
        Searching without query and with {{ countFilters() }} active filters
        <template v-if="total === null">...</template>
        <template v-else>.</template>
      </div>
      <template v-if="total !== null">
        <div v-if="total === 0">No results found.</div>
        <div v-else>{{ total }} results found.</div>
      </template>
    </div>

    <SelectButton v-model="selectButtonValue" :options="selectButtonOptions" class="flex-shrink-0">
      <template #option="{ option: selectionOption }">
        <component :is="selectionOption.icon?.component" v-if="selectionOption.icon" v-bind="{ alt: selectionOption.icon.alt }" class="w-7 h-7" />
      </template>
    </SelectButton>
  </div>
</template>
