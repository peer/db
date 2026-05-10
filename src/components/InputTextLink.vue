<!--
A link that visually matches InputText.vue. Built on InputStyled.vue.
-->

<script setup lang="ts">
import type { RouteLocationRaw } from "vue-router"

import { RouterLink } from "vue-router"

import InputStyled from "@/components/InputStyled.vue"

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
  <InputStyled :as="RouterLink" :to="to" :replace="replace" :inactive="disabled" :after-click="afterClick" class="pd-inputtextlink">
    <slot />
  </InputStyled>
</template>
