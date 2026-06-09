<!--
A link that visually matches Button.vue.

When disabled we render a <div> instead of an <a> so the link is not reachable
via keyboard or click.
-->

<script setup lang="ts">
import type { RouteLocationRaw } from "vue-router"

import ButtonStyled from "@/components/ButtonStyled.vue"
import RouterLink from "@/components/RouterLink.vue"

withDefaults(
  defineProps<{
    to: RouteLocationRaw
    replace?: boolean
    disabled?: boolean
    primary?: boolean
    active?: boolean
  }>(),
  {
    replace: false,
    disabled: false,
    primary: false,
    active: false,
  },
)
</script>

<template>
  <ButtonStyled v-if="disabled" as="div" :inactive="disabled" :primary="primary" :active="active" class="pd-buttonlink">
    <slot />
  </ButtonStyled>
  <ButtonStyled v-else :as="RouterLink" :to="to" :replace="replace" :primary="primary" :active="active" class="pd-buttonlink">
    <slot />
  </ButtonStyled>
</template>
