<script setup lang="ts">
import { computed } from "vue"
import { useRouter } from "vue-router"

import { classifyLink, LINK_CLASS_INTERNAL, LINK_CLASS_INTERNAL_NOVIEW } from "@/internal-links"
import { parseUrl } from "@/utils"

const props = defineProps<{
  iri: string
}>()

const router = useRouter()

const linkClasses = computed(() => classifyLink(props.iri, router))

const internalPath = computed<string | null>(() => {
  if (!linkClasses.value.includes(LINK_CLASS_INTERNAL)) return null
  try {
    const url = parseUrl(props.iri)
    return url.pathname + url.search + url.hash
  } catch {
    return null
  }
})

const internalNoView = computed(() => linkClasses.value.includes(LINK_CLASS_INTERNAL_NOVIEW))
</script>

<template>
  <!-- We use RouterLink for internal links with view. -->
  <RouterLink v-if="internalPath && !internalNoView" :to="internalPath" class="link break-all" :class="linkClasses"
    ><slot>{{ internalPath }}</slot></RouterLink
  >
  <!-- We use a for internal links without view and external links. -->
  <a v-else :href="internalPath || iri" class="link break-all" :class="linkClasses"
    ><slot>{{ internalPath || iri }}</slot></a
  >
</template>
