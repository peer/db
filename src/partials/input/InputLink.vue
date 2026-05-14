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

// A link is invalid if it does not parse as an absolute URL via the URL
// constructor. As a side effect of validation the model is normalized to the
// re-stringified URL (so "https://Example.com" becomes "https://example.com/",
// surrounding whitespace is stripped, etc.). The normalization is gated on
// !eager so the user is not fighting the input while typing.
// eslint-disable-next-line @typescript-eslint/require-await
const validator: ValidatorFn<string> = async function (value, options) {
  const trimmed = value.trim()
  if (trimmed === "") {
    if (!options.eager && trimmed !== model.value) {
      model.value = trimmed
    }
    if (!props.required) {
      return []
    }
    // TODO: Use standard codes.
    return [{ code: "required" }]
  }
  let normalized: string
  try {
    normalized = new URL(trimmed).toString()
  } catch (err) {
    // TODO: Use standard codes.
    return [
      {
        code: "invalid",
        ...(err instanceof Error ? { debugError: err } : {}),
        // eslint-disable-next-line @typescript-eslint/restrict-template-expressions
        debugMessage: `${err}`,
      },
    ]
  }
  if (!options.eager && normalized !== model.value) {
    model.value = normalized
  }
  return []
}
</script>

<template>
  <InputText v-model="model" v-model:errors="errors" :readonly="readonly" :validator="validator" />
</template>
