<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { RelationClaim, PeerDBDocument } from "@/document"

import WithDocument from "@/components/WithDocument.vue"
import { getName, loadingWidth } from "@/utils.ts"

defineProps<{
  claim: RelationClaim | DeepReadonly<RelationClaim> | null
}>()

const WithPeerDBDocument = WithDocument<PeerDBDocument>
</script>

<template>
  <WithPeerDBDocument v-if="claim" :id="claim.to.id" name="DocumentGet">
    <template #default="{ doc, url }">
      <RouterLink :to="{ name: 'DocumentGet', params: { id: claim.to.id } }" :data-url="url" class="link" v-html="getName(doc.claims) || '<i>no name</i>'" />
    </template>

    <template #loading="{ url }">
      <div class="inline-block h-2 animate-pulse rounded bg-slate-200" :data-url="url" :class="[loadingWidth(claim.to.id)]" />
    </template>
  </WithPeerDBDocument>
</template>
