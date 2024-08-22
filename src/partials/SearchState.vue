<script setup lang="ts">
import type {DeepReadonly} from "vue"
import type { ClientSearchState } from "@/types"

const props = defineProps<{
  state: DeepReadonly<ClientSearchState>
}>()

function countFilters(): number {
  if (!props.state.filters) {
    return 0
  }

  let n = 0;
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
  <div v-if="state.promptError">
    Error interpreting your prompt.
  </div>
  <div v-else-if="state.p && !state.promptCall">
    Interpreting your prompt...
  </div>
  <div v-else-if="countFilters() === 1">
    Search query <i>{{ state.q }}</i> and 1 active filter.
  </div>
  <div v-else>
    Search query <i>{{ state.q }}</i> and {{ countFilters() }} active filters.
  </div>
</template>
