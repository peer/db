<script setup lang="ts">
import { computed } from "vue"
import type { PeerDBDocument } from "@/types"

const props = defineProps<{
  doc: PeerDBDocument
}>()

const hasLoaded = computed(() => Object.prototype.hasOwnProperty.call(props.doc, "name"))
</script>

<template>
  <div class="rounded border bg-white p-4 shadow">
    <div v-if="hasLoaded" class="flex">
      <router-link :to="{ name: 'DocumentGet', params: { id: doc._id } }" class="link text-lg">{{ doc.name.en }}</router-link>
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
