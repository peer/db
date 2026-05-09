<script setup lang="ts">
import { computed } from "vue"
import { useRouter } from "vue-router"

import { classifyLink, LINK_CLASS_INTERNAL, LINK_CLASS_INTERNAL_NOVIEW } from "@/internal-links"

const props = defineProps<{
  iri: string
}>()

const router = useRouter()

const linkClasses = computed(() => classifyLink(props.iri, router))

const internalPath = computed<string | null>(() => {
  if (!linkClasses.value.includes(LINK_CLASS_INTERNAL)) return null
  if (linkClasses.value.includes(LINK_CLASS_INTERNAL_NOVIEW)) return null
  try {
    const url = new URL(props.iri, window.location.href)
    return url.pathname + url.search + url.hash
  } catch {
    return null
  }
})
</script>

<template>
  <RouterLink v-if="internalPath" :to="internalPath" class="link break-all" :class="linkClasses"
    ><slot>{{ iri }}</slot></RouterLink
  >
  <a v-else :href="iri" class="link break-all" :class="linkClasses"
    ><slot>{{ iri }}</slot></a
  >
</template>
