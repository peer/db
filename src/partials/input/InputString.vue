<script setup lang="ts">
import type { ShallowUnwrapRef } from "vue"

import type { ValidatedInput, ValidationError, ValidatorFn } from "@/types"

import { computed, useTemplateRef } from "vue"

import InputText from "@/components/InputText.vue"

const props = withDefaults(
  defineProps<{
    readonly?: boolean
    required?: boolean
    // Presentational override.
    invalid?: boolean
  }>(),
  {
    readonly: false,
    required: false,
    invalid: false,
  },
)

const model = defineModel<string>({ default: "" })

const emit = defineEmits<{ errors: [ValidationError[]] }>()

// A string invalid if it is empty after trimming. The required check is
// skipped on initial so a freshly mounted empty field is not flagged before
// the user has interacted.
// eslint-disable-next-line @typescript-eslint/require-await
const validator: ValidatorFn<string> = async function (value, options) {
  if (!props.required || options.initial) {
    return []
  }
  // TODO: Use standard codes.
  return value.trim() === "" ? [{ code: "required" }] : []
}

// Forward the inner InputText's ValidatedInput so the parent sees this
// wrapper as a regular validated input.
const inputTextRef = useTemplateRef<ShallowUnwrapRef<ValidatedInput>>("inputTextRef")
const validatedInput: ValidatedInput = {
  validate: async (signal) => {
    await inputTextRef.value?.validate(signal)
  },
  reset: () => inputTextRef.value?.reset(),
  revert: () => inputTextRef.value?.revert(),
  el: () => inputTextRef.value?.el() ?? null,
  isDirty: computed<boolean>(() => inputTextRef.value?.isDirty ?? false),
  isEmpty: computed<boolean>(() => inputTextRef.value?.isEmpty ?? true),
  errors: computed<ValidationError[]>(() => inputTextRef.value?.errors ?? []),
  checkpoint: () => inputTextRef.value?.checkpoint(),
}
defineExpose(validatedInput)
</script>

<template>
  <InputText ref="inputTextRef" v-model="model" :readonly="readonly" :invalid="invalid" :validator="validator" @errors="(v: ValidationError[]) => emit('errors', v)" />
</template>
