<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { LinkClaim } from "@/document"

import { computed } from "vue"
import { useI18n } from "vue-i18n"

import Link from "@/components/Link.vue"
import { NAME } from "@/core"
import { selectClaimsByLanguage } from "@/document/claims"

const props = defineProps<{
  claim: LinkClaim | DeepReadonly<LinkClaim> | null
}>()

const { locale } = useI18n({ useScope: "global" })

// If the link claim carries a NAME sub-claim in the current locale (or a
// fallback language), use that as the visible link text instead of the IRI.
const name = computed<string | null>(() => {
  if (!props.claim?.sub) return null
  const claims = selectClaimsByLanguage(props.claim.sub, "string", NAME, locale.value, (cs) => cs.length > 0 && !!cs[0].string)
  return claims?.[0]?.string ?? null
})
</script>

<template>
  <Link v-if="claim" :iri="claim.iri"><template v-if="name">{{ name }}</template></Link>
</template>
