<script setup lang="ts">
import type { RouteLocationRaw } from "vue-router"

import { useLink } from "vue-router"

const props = defineProps<{
  to: RouteLocationRaw
  replace?: boolean
  disabled?: boolean
  afterClick?: () => void
}>()

const { navigate, href } = useLink(props)

async function onClick(event: MouseEvent) {
  await navigate(event)
  if (props.afterClick) {
    await props.afterClick()
  }
}
</script>

<template>
  <span v-if="disabled">
    <slot />
  </span>
  <a v-else :href="href" @click="onClick"><slot /></a>
</template>
