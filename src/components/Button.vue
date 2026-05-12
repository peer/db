<script setup lang="ts">
import ButtonStyled from "@/components/ButtonStyled.vue"
import ProgressBar from "@/components/ProgressBar.vue"

withDefaults(
  defineProps<{
    progress?: number
    total?: number | null
    disabled?: boolean
    primary?: boolean
    active?: boolean
  }>(),
  {
    progress: 0,
    total: null,
    disabled: false,
    primary: false,
    active: false,
  },
)
</script>

<template>
  <ButtonStyled as="button" :inactive="progress > 0 || disabled" :primary="primary" :active="active" :disabled="progress > 0 || disabled" class="pd-button">
    <slot />
    <ProgressBar :progress="progress" :total="total" class="absolute inset-x-0 bottom-0 rounded-b" />
  </ButtonStyled>
</template>
