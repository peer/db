<!--
We do not use :read-only or :disabled pseudo classes to style the component because
we want component to retain how it visually looks even if DOM element's read-only or
disabled attributes are set, unless they are set through component's props.
This is used during transitions/animations to disable the component by directly setting
its DOM attributes without flickering how the component looks.
-->

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
