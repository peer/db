<script setup lang="ts">
import type { PeerDBDocument } from "@/types"

import { ref, computed } from "vue"
import RouterLink from "@/components/RouterLink.vue"
import { useFilterValues } from "@/search"

const props = defineProps<{
  searchTotal: number
  property: PeerDBDocument
}>()

const progress = ref(0)
const { docs, total } = useFilterValues(props.property, progress)

const hasLoaded = computed(() => props.property?.name?.en)
const docsWithNone = computed(() => {
  if (!docs.value.length) {
    return docs.value
  } else if (props.property._count >= props.searchTotal) {
    return docs.value
  }
  const res = [...docs.value, { _count: props.searchTotal - props.property._count }]
  res.sort((a, b) => b._count - a._count)
  return res
})
</script>

<template>
  <div class="rounded border bg-white p-4 shadow">
    <div v-if="hasLoaded">
      <div>
        <RouterLink :to="{ name: 'DocumentGet', params: { id: property._id } }" class="link text-lg">{{ property.name.en }}</RouterLink>
        ({{ property._count }})
      </div>
      <div>({{ total }})</div>
      <ul>
        <li v-for="doc in docsWithNone" :key="doc._id">
          <template v-if="doc.name?.en">
            <RouterLink :to="{ name: 'DocumentGet', params: { id: doc._id } }" class="link">{{ doc.name.en }}</RouterLink> ({{ doc._count }})
          </template>
          <template v-else-if="!doc._id"><i>none</i> ({{ doc._count }})</template>
          <div v-else class="flex animate-pulse">
            <div class="flex-1 space-y-4">
              <div class="my-2 h-2 w-52 rounded bg-slate-200"></div>
            </div>
          </div>
        </li>
      </ul>
    </div>
    <div v-else class="flex animate-pulse">
      <div class="flex-1 space-y-4">
        <div class="h-2 w-72 rounded bg-slate-200"></div>
      </div>
    </div>
  </div>
</template>
