<script setup lang="ts">
import type { DeepReadonly } from "vue"

import { getName, loadingWidth } from "@/utils"
import type { Claim } from "@/document"

defineProps<{
  claim: Claim | DeepReadonly<Claim> | null
}>()
</script>

<template>
  <WithPeerDBDocument v-if="claim" :id="claim.prop.id" name="DocumentGet">
    <template #default="{ doc, url }">
      <RouterLink :to="{ name: 'DocumentGet', params: { id: claim.prop.id } }" :data-url="url" class="link" v-html="getName(doc.claims) || '<i>no name</i>'"></RouterLink>
    </template>
    <template #loading="{ url }">
      <div class="inline-block h-2 animate-pulse rounded bg-slate-200" :data-url="url" :class="[loadingWidth(claim.prop.id)]"></div>
    </template>
  </WithPeerDBDocument>
</template>
