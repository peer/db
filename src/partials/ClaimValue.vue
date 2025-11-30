<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { PeerDBDocument, Claim } from "@/document"
import type { AnyClaimType } from "@/types"

import { computed } from "vue"

import WithDocument from "@/components/WithDocument.vue"
import { claimFrom } from "@/document"
import { getName, loadingWidth } from "@/utils.ts"

const props = defineProps<{
  claim: Claim | DeepReadonly<Claim> | null
  type: AnyClaimType
}>()

const WithPeerDBDocument = WithDocument<PeerDBDocument>

const claimObj = computed(() => (props.claim ? claimFrom(props.claim, props.type) : null))
</script>

<template>
  <span v-if="!claimObj" />

  <!-- Id claim -->
  <span v-else-if="claimObj.type === 'id'">
    <WithPeerDBDocument :id="claimObj.prop.id" name="DocumentGet">
      <template #default="{ doc, url }">
        <RouterLink :to="{ name: 'DocumentGet', params: { id: claimObj.prop.id } }" :data-url="url" class="link" v-html="getName(doc.claims) || '<i>no name</i>'" />
      </template>

      <template #loading="{ url }">
        <div class="inline-block h-2 animate-pulse rounded bg-slate-200" :data-url="url" :class="[loadingWidth(claimObj.prop.id)]" />
      </template>
    </WithPeerDBDocument>
  </span>

  <!-- Ref claim -->
  <span v-else-if="claimObj.type === 'ref'">
    <a :href="claimObj.iri" class="link">{{ claimObj.iri }}</a>
  </span>

  <!-- Text claim -->
  <span v-else-if="claimObj.type === 'text'" class="prose prose-slate max-w-none" v-html="claimObj.html?.en"></span>

  <!-- String claim -->
  <span v-else-if="claimObj.type === 'string'">
    {{ claimObj.string }}
  </span>

  <!-- Amount claim -->
  <span v-else-if="claimObj.type === 'amount'">
    {{ claimObj.amount }}<template v-if="claimObj.unit !== '1'">{{ claimObj.unit }}</template>
  </span>

  <!-- Amount range claim -->
  <span v-else-if="claimObj.type === 'amountRange'">
    {{ claimObj.lower }}–{{ claimObj.upper }}<template v-if="claimObj.unit !== '1'">{{ claimObj.unit }}</template>
  </span>

  <!-- Relation claim -->
  <span v-else-if="claimObj.type === 'rel'">
    <WithPeerDBDocument :id="claimObj.to.id" name="DocumentGet">
      <template #default="{ doc, url }">
        <RouterLink :to="{ name: 'DocumentGet', params: { id: claimObj.to.id } }" :data-url="url" class="link" v-html="getName(doc.claims) || '<i>no name</i>'" />
      </template>

      <template #loading="{ url }">
        <div class="inline-block h-2 animate-pulse rounded bg-slate-200" :data-url="url" :class="[loadingWidth(claimObj.to.id)]" />
      </template>
    </WithPeerDBDocument>
  </span>

  <!-- File claim -->
  <span v-else-if="claimObj.type === 'file'">
    <a :href="claimObj.url">
      <img v-if="claimObj.preview?.[0]" :src="claimObj.preview[0]" />
      <span v-else class="link">{{ claimObj.mediaType }}</span>
    </a>
  </span>

  <!-- None claim -->
  <span v-else-if="claimObj.type === 'none'">none</span>

  <!-- Unknown claim -->
  <span v-else-if="claimObj.type === 'unknown'">unknown</span>

  <!-- Time claim -->
  <span v-else-if="claimObj.type === 'time'">{{ claimObj.timestamp }}</span>

  <!-- Time range claim -->
  <span v-else-if="claimObj.type === 'timeRange'">{{ claimObj.lower }}-{{ claimObj.upper }}</span>
</template>
