<script setup lang="ts">
// We use v-model-text directive to mirror what Vue does on native <textarea> elements which
// we have to do ourselves because we use <textarea> element through InputStyled component.
import { onBeforeUnmount, onMounted, onUpdated, useTemplateRef, vModelText } from "vue"

import InputStyled from "@/components/InputStyled.vue"

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

const el = useTemplateRef<InstanceType<typeof InputStyled>>("el")

function resize() {
  const ta = el.value?.$el as HTMLTextAreaElement | undefined
  if (!ta) {
    return
  }

  ta.style.height = "0"
  ta.style.height = ta.scrollHeight + "px"
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
  <InputStyled
    ref="el"
    v-model-text="model"
    as="textarea"
    :inactive="progress > 0 || readonly"
    :invalid="invalid"
    :readonly="progress > 0 || readonly"
    class="pd-textarea h-10 resize-none"
    @update:model-value="model = $event"
  />
</template>
