<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { IdentifierClaim } from "@/document"

import { computed, ref, watch } from "vue"
import { useRouter } from "vue-router"

import { getURL } from "@/api"
import { IDENTIFIER_LINK_TEMPLATE } from "@/core"
import { D, getBestClaimOfType } from "@/document"
import { useProgress } from "@/progress"

const props = defineProps<{
  claim: IdentifierClaim | DeepReadonly<IdentifierClaim> | null
}>()

const router = useRouter()

const progress = useProgress()

const linkTemplate = ref<string | null>(null)

// Load the property document to check for an identifier link template.
watch(
  () => props.claim,
  async (claim, oldClaim, onCleanup) => {
    linkTemplate.value = null

    if (!claim) return

    const abortController = new AbortController()
    onCleanup(() => abortController.abort())

    try {
      const url = router.apiResolve({
        name: "DocumentGet",
        params: {
          id: claim.prop.id,
        },
      }).href

      // TODO: Pass element as el argument.
      const { doc: rawDoc } = await getURL<object>(url, null, abortController.signal, progress)
      if (abortController.signal.aborted) {
        return
      }

      const doc = new D(rawDoc)
      linkTemplate.value = getBestClaimOfType(doc.claims, "string", IDENTIFIER_LINK_TEMPLATE)?.string ?? null
    } catch (err) {
      if (abortController.signal.aborted) {
        return
      }

      // TODO: Do something better?
      console.error("ClaimValueId.watchEffect", err)
    }
  },
  {
    immediate: true,
  },
)

// Construct the URL from the template or fall back to the identifier value itself if it looks like a URL.
const identifierUrl = computed(() => {
  if (!props.claim) return null
  if (linkTemplate.value) {
    return linkTemplate.value.replaceAll("{identifier}", props.claim.value)
  }
  if (props.claim.value.startsWith("http://") || props.claim.value.startsWith("https://")) {
    return props.claim.value
  }
  return null
})
</script>

<template>
  <template v-if="claim"
    ><a v-if="identifierUrl" :href="identifierUrl" class="link break-all">{{ claim.value }}</a
    ><template v-else>{{ claim.value }}</template></template
  >
</template>
