<script setup lang="ts">
import type { ComponentPublicInstance, ShallowUnwrapRef } from "vue"

import type { ValidatedInput } from "@/types"

import { shallowRef, useId } from "vue"

import InputBadges from "@/partials/InputBadges.vue"
import InputErrors from "@/partials/InputErrors.vue"

defineProps<{
  required?: boolean
  // Presentational override.
  invalid?: boolean
}>()

// Fallthrough attrs land on the label.
defineOptions({
  inheritAttrs: false,
})

const inputId = useId()
const input = shallowRef<ShallowUnwrapRef<ValidatedInput> | null>(null)

// The parameter is typed against Vue's VNodeRef signature so the function
// can be spread via v-bind="inputProps" onto any component without TS
// narrowing complaints. At runtime the consumer's input is a validated
// component instance whose defineExpose makes its ValidatedInput shape
// available with refs auto-unwrapped (ShallowUnwrapRef).
function setInputRef(i: Element | ComponentPublicInstance | null) {
  input.value = i as ShallowUnwrapRef<ValidatedInput> | null
}
</script>

<template>
  <label v-tw-merge :for="inputId" v-bind="$attrs" class="mb-1 flex flex-row items-center gap-1"
    ><slot name="label" /><InputBadges :required="required" :changed="input?.isDirty ?? false" @revert="input?.revert()"
  /></label>
  <InputErrors v-slot="errorProps">
    <slot v-bind="errorProps" :id="inputId" :ref="setInputRef" name="input" :required="required" :invalid="invalid" />
  </InputErrors>
</template>
