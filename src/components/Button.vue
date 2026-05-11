<script setup lang="ts">
import ButtonStyled from "@/components/ButtonStyled.vue"
import ProgressBar from "@/components/ProgressBar.vue"

withDefaults(
  defineProps<{
    progress?: number
    disabled?: boolean
    primary?: boolean
  }>(),
  {
    progress: 0,
    disabled: false,
    primary: false,
  },
)
</script>

<template>
  <ButtonStyled as="button" :inactive="progress > 0 || disabled" :primary="primary" :disabled="progress > 0 || disabled" class="pd-button">
    <slot />
    <ProgressBar :progress="progress" class="absolute inset-x-0 bottom-0 rounded-b" />
  </ButtonStyled>
</template>
