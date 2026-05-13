<script setup lang="ts">
import type { ValidationError, ValidatorFn } from "@/types"

import InputText from "@/components/InputText.vue"

withDefaults(
  defineProps<{
    readonly?: boolean
  }>(),
  {
    readonly: false,
  },
)

const model = defineModel<string>({ default: "" })
const errors = defineModel<ValidationError[]>("errors", { default: () => [] })

// An identifier is invalid if it is empty after trimming. As a side effect of
// validation the model is normalized to the trimmed value, so " abc " becomes
// "abc" on blur or before submit. The normalization is gated on !eager so the
// user is not fighting the input while typing (e.g. typing a leading space
// while the field is already in the invalid state would otherwise be erased
// immediately by the eager re-validation).
// eslint-disable-next-line @typescript-eslint/require-await
const validator: ValidatorFn<string> = async function (value, options) {
  const trimmed = value.trim()
  if (!options.eager && trimmed !== model.value) {
    model.value = trimmed
  }
  // TODO: Use standard codes.
  return trimmed === "" ? [{ code: "required" }] : []
}
</script>

<template>
  <InputText v-model="model" v-model:errors="errors" :readonly="readonly" :validator="validator" />
</template>
