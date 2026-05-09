<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { LinkClaim } from "@/document"

import { computed } from "vue"
import { useRouter } from "vue-router"

import { classifyLink, LINK_CLASS_INTERNAL, LINK_CLASS_INTERNAL_NOVIEW } from "@/internal-links"

const props = defineProps<{
  claim: LinkClaim | DeepReadonly<LinkClaim> | null
}>()

const router = useRouter()

const linkClasses = computed(() => classifyLink(props.claim?.iri ?? "", router))

const internalPath = computed<string | null>(() => {
  if (!props.claim) return null
  if (!linkClasses.value.includes(LINK_CLASS_INTERNAL)) return null
  if (linkClasses.value.includes(LINK_CLASS_INTERNAL_NOVIEW)) return null
  try {
    const url = new URL(props.claim.iri, window.location.href)
    return url.pathname + url.search + url.hash
  } catch {
    return null
  }
})
</script>

<template>
  <template v-if="claim">
    <RouterLink v-if="internalPath" :to="internalPath" class="link break-all" :class="linkClasses">{{ claim.iri }}</RouterLink>
    <a v-else :href="claim.iri" class="link break-all" :class="linkClasses">{{ claim.iri }}</a>
  </template>
</template>
