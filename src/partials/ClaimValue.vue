<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { PeerDBDocument, Claim, ClaimTypeProp } from "@/document"

import WithDocument from "@/components/WithDocument.vue"
import { getName, loadingWidth } from "@/utils.ts"

defineProps<{
  claim: Claim | DeepReadonly<Claim> | null
  type: ClaimTypeProp
}>()

const WithPeerDBDocument = WithDocument<PeerDBDocument>
</script>

<template>
  <template v-if="!claim" />

  <!-- Id claim -->
  <template v-else-if="type === 'id'">{{ claim.value }}</template>

  <!-- Ref claim -->
  <a v-else-if="type === 'ref'" :href="claim.iri" class="link">{{ claim.iri }}</a>

  <!-- Text claim -->
  <div v-else-if="type === 'text'" class="prose prose-slate max-w-none" v-html="claim.html?.en"></div>

  <!-- String claim -->
  <template v-else-if="type === 'string'">{{ claim.string }}</template>

  <!-- Amount claim -->
  <template v-else-if="type === 'amount'">
    {{ claim.amount }} <template v-if="claim.unit !== '1'">{{ claim.unit }}</template>
  </template>

  <!-- Amount range claim -->
  <template v-else-if="type === 'amountRange'">
    {{ claim.lower }}–{{ claim.upper }} <template v-if="claim.unit !== '1'">{{ claim.unit }}</template>
  </template>

  <!-- Relation claim -->
  <WithPeerDBDocument v-else-if="type === 'rel'" :id="claim.to.id" name="DocumentGet">
    <template #default="{ doc, url }">
      <RouterLink :to="{ name: 'DocumentGet', params: { id: claim.to.id } }" :data-url="url" class="link" v-html="getName(doc.claims) || '<i>no name</i>'" />
    </template>

    <template #loading="{ url }">
      <div class="inline-block h-2 animate-pulse rounded bg-slate-200" :data-url="url" :class="[loadingWidth(claim.to.id)]" />
    </template>
  </WithPeerDBDocument>

  <!-- File claim -->
  <template v-else-if="type === 'file'">
    <a v-if="claim.preview?.[0]" :href="claim.url">
      <img :src="claim.preview[0]" alt="File preview" />
    </a>
    <a v-else :href="claim.url" class="link">{{ claim.mediaType }}</a>
  </template>

  <!-- None claim -->
  <template v-else-if="type === 'none'">none</template>

  <!-- Unknown claim -->
  <template v-else-if="type === 'unknown'">unknown</template>

  <!-- Time claim -->
  <template v-else-if="type === 'time'">{{ claim.timestamp }}</template>

  <!-- Time range claim -->
  <template v-else-if="type === 'timeRange'">{{ claim.lower }}-{{ claim.upper }}</template>
</template>
