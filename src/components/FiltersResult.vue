<script setup lang="ts">
import type { PeerDBDocument } from "@/types"

import { ref, computed } from "vue"
import RouterLink from "@/components/RouterLink.vue"
import { useFilterValues } from "@/search"

const props = defineProps<{
  property: PeerDBDocument
}>()

const progress = ref(0)
const { docs, total } = useFilterValues(props.property, progress)

const hasLoaded = computed(() => "name" in props.property)
</script>

<template>
  <div class="rounded border bg-white p-4 shadow">
    <div v-if="hasLoaded">
      <div class="first-letter:capitalize">
        <RouterLink :to="{ name: 'DocumentGet', params: { id: property._id } }" class="link text-lg">{{ property.name.en }}</RouterLink>
        ({{ property._count }})
      </div>
      <div>({{ total }})</div>
      <ul>
        <li v-for="doc in docs" :key="doc._id">
          <RouterLink :to="{ name: 'DocumentGet', params: { id: doc._id } }" class="link">{{ doc.name?.en || doc._id }}</RouterLink> ({{ doc._count }})
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
