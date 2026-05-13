<script setup lang="ts">
import type { ValidationError, ValidatorFn } from "@/types"

import InputText from "@/components/InputText.vue"

const props = withDefaults(
  defineProps<{
    readonly?: boolean
    required?: boolean
  }>(),
  {
    readonly: false,
    required: false,
  },
)

const model = defineModel<string>({ default: "" })
const errors = defineModel<ValidationError[]>("errors", { default: () => [] })

// A string invalid if it is empty after trimming.
// eslint-disable-next-line @typescript-eslint/require-await
const validator: ValidatorFn<string> = async function (value) {
  if (!props.required) {
    return []
  }
  // TODO: Use standard codes.
  return value.trim() === "" ? [{ code: "required" }] : []
}
</script>

<template>
  <InputText v-model="model" v-model:errors="errors" :readonly="readonly" :validator="validator" />
</template>
