<script setup lang="ts">
import { computed } from "vue"
import { useLink } from "vue-router"

const props = defineProps({
  to: {
    type: [String, Object],
    required: true,
  },
  replace: {
    type: Boolean,
    default: false,
  },
  afterClick: {
    type: Function,
    default: null,
  },
})

const { navigate, href } = useLink({ to: props.to, replace: props.replace })

async function onClick(event: MouseEvent) {
  await navigate(event)
  if (props.afterClick) {
    await props.afterClick()
  }
}
</script>

<template>
  <a :href="href" @click="onClick"><slot /></a>
</template>
