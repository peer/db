<!--
We do not use :read-only or :disabled pseudo classes to style the component because
we want component to retain how it visually looks even if DOM element's read-only or
disabled attributes are set, unless they are set through component's props.
This is used during transitions/animations to disable the component by directly setting
its DOM attributes without flickering how the component looks.
-->

<script setup lang="ts">
import { onBeforeUnmount, onMounted, onUpdated, ref } from "vue"

withDefaults(
  defineProps<{
    progress?: number
    readonly?: boolean
    invalid?: boolean
  }>(),
  {
    progress: 0,
    readonly: false,
    invalid: false,
  },
)

const model = defineModel<string>({ default: "" })

const el = ref()

function resize() {
  if (!el.value) {
    return
  }

  el.value.style.height = "0"
  el.value.style.height = el.value.scrollHeight + "px"
}

onMounted(resize)
onUpdated(resize)

onMounted(() => {
  window.addEventListener("resize", resize, { passive: true })
})

onBeforeUnmount(() => {
  window.removeEventListener("resize", resize)
})
</script>

<template>
  <textarea
    ref="el"
    v-model="model"
    :readonly="progress > 0 || readonly"
    class="rounded border-0 shadow ring-2 ring-neutral-300 focus:ring-2 resize-none h-10"
    :class="{
      'cursor-not-allowed': progress > 0 || readonly,
      'bg-gray-100 text-gray-800 hover:ring-neutral-300 focus:border-primary-300 focus:ring-primary-300': progress > 0 || readonly,
      'bg-white hover:ring-neutral-400 focus:ring-primary-500': progress === 0 && !readonly,
      'bg-error-50': invalid,
    }"
  />
</template>
