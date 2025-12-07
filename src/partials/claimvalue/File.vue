<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { FileClaim } from "@/document"

defineProps<{
  claim: FileClaim | DeepReadonly<FileClaim> | null
}>()
</script>

<template>
  <template v-if="claim"
    ><a v-if="claim.preview?.[0]" :href="claim.url"><img :src="claim.preview[0]" alt="File preview" /></a
    ><a v-else :href="claim.url" class="link">{{ claim.mediaType }}</a></template
  >
</template>
