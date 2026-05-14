<script setup lang="ts">
import ButtonStyled from "@/components/ButtonStyled.vue"
import ProgressBar from "@/components/ProgressBar.vue"
import { useLocked } from "@/progress"

const props = withDefaults(
  defineProps<{
    // progress drives the visual ProgressBar inside the button only. It does not
    // participate in disabling the button. Lock state comes from the surrounding
    // useLock boundary via useLocked. Pass progress when you want to display
    // in-button progress (e.g. submit button or file upload) and rely on
    // an enclosing useBusy/useLock to lock the button during the same work.
    progress?: number
    total?: number | null
    disabled?: boolean
    primary?: boolean
    active?: boolean
    invalid?: boolean
  }>(),
  {
    progress: 0,
    total: null,
    disabled: false,
    primary: false,
    active: false,
    invalid: false,
  },
)

const locked = useLocked()
const inactive = () => locked.value || props.disabled
</script>

<template>
  <ButtonStyled as="button" :inactive="inactive()" :primary="primary" :active="active" :invalid="invalid" :disabled="inactive()" class="pd-button">
    <slot />
    <ProgressBar :progress="progress" :total="total" class="absolute inset-x-0 bottom-0 rounded-b" />
  </ButtonStyled>
</template>
