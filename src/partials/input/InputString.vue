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

// A string invalid if it is empty after trimming.
// eslint-disable-next-line @typescript-eslint/require-await
const validator: ValidatorFn<string> = async function (value) {
  // TODO: Use standard codes.
  return value.trim() === "" ? [{ code: "required" }] : []
}
</script>

<template>
  <InputText v-model="model" v-model:errors="errors" :readonly="readonly" :validator="validator" />
</template>
