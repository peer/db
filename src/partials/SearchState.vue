<script setup lang="ts">
import type { ClientSearchState } from "@/types"

const props = defineProps<{
  state: ClientSearchState
}>()

function countFilters(): number {
  if (!props.state.filters) {
    return 0
  }

  let n = 0;
  for (const t of ["rel", "amount", "time", "str"]) {
    for (const values of Object.values(props.state.filters[t])) {
      n += values.length
    }
  }
  if (props.state.index) {
    n += Object.values(props.state.index).length
  }
  if (props.state.size) {
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
    Search query <i>{{ state.q }}</i> and 1 filter.
  </div>
  <div v-else>
    Search query <i>{{ state.q }}</i> and {{ countFilters() }} filters.
  </div>
</template>
