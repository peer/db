<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { PeerDBDocument, Claim, ClaimTypeProp } from "@/document"

import { computed } from "vue"

import WithDocument from "@/components/WithDocument.vue"
import { claimFrom } from "@/document"
import { getName, loadingWidth } from "@/utils.ts"

const props = defineProps<{
  claim: Claim | DeepReadonly<Claim> | null
  type: ClaimTypeProp
}>()

const WithPeerDBDocument = WithDocument<PeerDBDocument>

const claimObj = computed(() => (props.claim ? claimFrom(props.claim, props.type) : null))
</script>

<template>
  <template v-if="!claimObj" />

  <!-- Id claim -->
  <template v-else-if="claimObj.type === 'id'">{{ claimObj.value }}</template>

  <!-- Ref claim -->
  <a v-else-if="claimObj.type === 'ref'" :href="claimObj.iri" class="link">{{ claimObj.iri }}</a>

  <!-- Text claim -->
  <div v-else-if="claimObj.type === 'text'" class="prose prose-slate max-w-none" v-html="claimObj.html?.en"></div>

  <!-- String claim -->
  <template v-else-if="claimObj.type === 'string'">{{ claimObj.string }}</template>

  <!-- Amount claim -->
  <template v-else-if="claimObj.type === 'amount'">
    {{ claimObj.amount }} <template v-if="claimObj.unit !== '1'">{{ claimObj.unit }}</template>
  </template>

  <!-- Amount range claim -->
  <template v-else-if="claimObj.type === 'amountRange'">
    {{ claimObj.lower }}–{{ claimObj.upper }} <template v-if="claimObj.unit !== '1'">{{ claimObj.unit }}</template>
  </template>

  <!-- Relation claim -->
  <WithPeerDBDocument v-else-if="claimObj.type === 'rel'" :id="claimObj.to.id" name="DocumentGet">
    <template #default="{ doc, url }">
      <RouterLink :to="{ name: 'DocumentGet', params: { id: claimObj.to.id } }" :data-url="url" class="link" v-html="getName(doc.claims) || '<i>no name</i>'" />
    </template>

    <template #loading="{ url }">
      <div class="inline-block h-2 animate-pulse rounded bg-slate-200" :data-url="url" :class="[loadingWidth(claimObj.to.id)]" />
    </template>
  </WithPeerDBDocument>

  <!-- File claim -->
  <template v-else-if="claimObj.type === 'file'">
    <a v-if="claimObj.preview?.[0]" :href="claimObj.url">
      <img :src="claimObj.preview[0]" alt="File preview" />
    </a>
    <a v-else :href="claimObj.url" class="link">{{ claimObj.mediaType }}</a>
  </template>

  <!-- None claim -->
  <template v-else-if="claimObj.type === 'none'">none</template>

  <!-- Unknown claim -->
  <template v-else-if="claimObj.type === 'unknown'">unknown</template>

  <!-- Time claim -->
  <template v-else-if="claimObj.type === 'time'">{{ claimObj.timestamp }}</template>

  <!-- Time range claim -->
  <template v-else-if="claimObj.type === 'timeRange'">{{ claimObj.lower }}-{{ claimObj.upper }}</template>
</template>
