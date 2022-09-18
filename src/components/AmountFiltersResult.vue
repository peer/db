<script setup lang="ts">
import type { PeerDBDocument, FilterState } from "@/types"

import { ref, computed } from "vue"
import RouterLink from "@/components/RouterLink.vue"
import { useHistogramValues } from "@/search"

const props = defineProps<{
  searchTotal: number
  property: PeerDBDocument
  state: FilterState
  updateProgress: number
}>()

const emit = defineEmits<{
  (e: "update:state", state: FilterState): void
}>()

const progress = ref(0)
const { results, total, min, max, interval } = useHistogramValues(props.property, progress)

const hasLoaded = computed(() => props.property?.name?.en)

function onChange(event: Event, id: string) {
  let updatedState = [...props.state]
  if ((event.target as HTMLInputElement).checked) {
    if (!updatedState.includes(id)) {
      updatedState.push(id)
    }
  } else {
    updatedState = updatedState.filter((x) => x !== id)
  }
  if (JSON.stringify(props.state) !== JSON.stringify(updatedState)) {
    emit("update:state", updatedState)
  }
}
</script>

<template>
  <div class="rounded border bg-white p-4 shadow">
    <div v-if="hasLoaded" class="flex flex-col">
      <div class="flex items-baseline gap-x-1">
        <RouterLink :to="{ name: 'DocumentGet', params: { id: property._id } }" class="link mb-1.5 text-lg leading-none">{{ property.name.en }}</RouterLink>
        ({{ property._count }})
      </div>
    </div>
    <div v-else class="flex animate-pulse">
      <div class="flex-1 space-y-4">
        <div class="h-2 w-72 rounded bg-slate-200"></div>
      </div>
    </div>
  </div>
</template>
