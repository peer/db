<script setup lang="ts">
import type { ShallowUnwrapRef } from "vue"

import type { ValidatedInput, ValidationError, ValidatorFn } from "@/types"

import { computed, useTemplateRef } from "vue"

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

// An identifier is invalid if it is empty after trimming. As a side effect of
// validation the model is normalized to the trimmed value, so " abc " becomes
// "abc" on blur or before submit. The normalization is gated on !eager so the
// user is not fighting the input while typing (e.g. typing a leading space
// while the field is already in the invalid state would otherwise be erased
// immediately by the eager re-validation), and on !initial so the field is
// not mutated before the user has interacted. The required check is also
// skipped on initial so a freshly mounted empty field is not flagged.
// eslint-disable-next-line @typescript-eslint/require-await
const validator: ValidatorFn<string> = async function (value, options) {
  const trimmed = value.trim()
  if (!options.eager && !options.initial && trimmed !== model.value) {
    model.value = trimmed
  }
  if (!props.required || options.initial) {
    return []
  }
  // TODO: Use standard codes.
  return trimmed === "" ? [{ code: "required" }] : []
}

// Forward the inner InputText's ValidatedInput so the parent sees this
// wrapper as a regular validated input.
const inputTextRef = useTemplateRef<ShallowUnwrapRef<ValidatedInput>>("inputTextRef")
const validatedInput: ValidatedInput = {
  validate: async (signal) => {
    const inner = inputTextRef.value
    if (!inner) return []
    return await inner.validate(signal)
  },
  reset: () => inputTextRef.value?.reset(),
  revert: () => inputTextRef.value?.revert(),
  el: () => inputTextRef.value?.el() ?? null,
  isDirty: computed<boolean>(() => inputTextRef.value?.isDirty ?? false),
  setBaseline: () => inputTextRef.value?.setBaseline(),
}
defineExpose(validatedInput)
</script>

<template>
  <InputText ref="inputTextRef" v-model="model" v-model:errors="errors" :readonly="readonly" :validator="validator" />
</template>
