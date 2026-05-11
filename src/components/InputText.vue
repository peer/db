<script setup lang="ts">
// We use v-model-text directive to mirror what Vue does on native <input> elements which
// we have to do ourselves because we use <input> element through InputStyled component.
import { vModelText } from "vue"

import InputStyled from "@/components/InputStyled.vue"

withDefaults(
  defineProps<{
    progress?: number
    readonly?: boolean
    type?: string
    invalid?: boolean
  }>(),
  {
    progress: 0,
    readonly: false,
    type: "text",
    invalid: false,
  },
)

const model = defineModel<string>({ default: "" })
</script>

<template>
  <InputStyled
    v-model-text="model"
    as="input"
    :inactive="progress > 0 || readonly"
    :invalid="invalid"
    :type="type"
    :readonly="progress > 0 || readonly"
    class="pd-inputtext"
    @update:model-value="model = $event"
  />
</template>
