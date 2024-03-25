<script setup lang="ts">
import { nextTick } from "vue"
import type { RouteLocationRaw } from "vue-router"

import { useLink } from "vue-router"

const props = withDefaults(
  defineProps<{
    to: RouteLocationRaw
    replace?: boolean
    disabled?: boolean
    afterClick?: () => void
  }>(),
  {
    replace: false,
    disabled: false,
    afterClick: undefined,
  },
)

const { navigate, href } = useLink(props)

async function onClick(event: MouseEvent) {
  await navigate(event)
  if (props.afterClick) {
    await nextTick()
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
