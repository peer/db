<!--
A link that visually matches InputText.vue.

When disabled we render a <div> instead of an <a> so the link is not reachable
via keyboard or click.
-->

<script setup lang="ts">
import type { RouteLocationRaw } from "vue-router"

import InputStyled from "@/components/InputStyled.vue"
import RouterLink from "@/components/RouterLink.vue"

withDefaults(
  defineProps<{
    to: RouteLocationRaw
    replace?: boolean
    disabled?: boolean
    afterClick?: () => void | Promise<void>
  }>(),
  {
    replace: false,
    disabled: false,
    afterClick: undefined,
  },
)
</script>

<template>
  <InputStyled v-if="disabled" as="div" :inactive="disabled" class="pd-inputtextlink">
    <slot />
  </InputStyled>
  <InputStyled v-else :as="RouterLink" :to="to" :replace="replace" :after-click="afterClick" class="pd-inputtextlink">
    <slot />
  </InputStyled>
</template>
