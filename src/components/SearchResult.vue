<script setup lang="ts">
import type { PeerDBDocument } from "@/types"

import { computed } from "vue"
import { useRoute } from "vue-router"
import RouterLink from "@/components/RouterLink.vue"

const props = defineProps<{
  doc: PeerDBDocument
}>()

const route = useRoute()

const hasLoaded = computed(() => "name" in props.doc)
// TODO: Do not hard-code description property ID.
const description = computed(() => {
  for (const claim of props.doc.active?.text || []) {
    if (claim.prop._id === "E7DXhBtz9UuoSG9V3uYeYF") {
      return claim.html.en
    }
  }

  return ""
})
</script>

<template>
  <div class="rounded border bg-white p-4 shadow">
    <div v-if="hasLoaded">
      <RouterLink :to="{ name: 'DocumentGet', params: { id: doc._id }, query: { s: route.query.s } }" class="link text-lg">{{ doc.name.en }}</RouterLink>
      <!-- eslint-disable-next-line vue/no-v-html -->
      <p v-if="description" v-html="description"></p>
    </div>
    <div v-else class="flex animate-pulse">
      <div class="flex-1 space-y-4">
        <div class="h-2 w-72 rounded bg-slate-200"></div>
        <div class="grid grid-cols-5 gap-4">
          <div class="col-span-1 h-2 rounded bg-slate-200"></div>
          <div class="col-span-2 h-2 rounded bg-slate-200"></div>
        </div>
      </div>
    </div>
  </div>
</template>
