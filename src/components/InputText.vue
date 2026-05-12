<script setup lang="ts">
import type { ComponentPublicInstance } from "vue"

import type { ValidationError, ValidatorFn } from "@/types"

// We use v-model-text directive to mirror what Vue does on native <input> elements which
// we have to do ourselves because we use <input> element through InputStyled component.
import { computed, useTemplateRef, vModelText } from "vue"

import InputStyled from "@/components/InputStyled.vue"
import { useValidation } from "@/validation"

const props = withDefaults(
  defineProps<{
    readonly?: boolean
    type?: string
    // Without a validator the input does not drive errors at all: errors is
    // fully owned by the parent via v-model:errors (or :errors), and
    // validate() is a no-op that returns the current errors. Pass a validator
    // to let the input own its own validation logic.
    validator?: ValidatorFn<string>
  }>(),
  {
    readonly: false,
    type: "text",
    validator: undefined,
  },
)

const model = defineModel<string>({ default: "" })
const errors = defineModel<ValidationError[]>("errors", { default: () => [] })
const progress = defineModel<number>("progress", { default: 0 })
const invalid = computed(() => errors.value.length > 0)

const inputStyledRef = useTemplateRef<ComponentPublicInstance>("inputStyledRef")

const { runValidation, validatedInput } = useValidation(
  model,
  errors,
  progress,
  () => props.validator,
  () => inputStyledRef.value?.$el ?? null,
)

defineExpose(validatedInput)

async function onBlur() {
  await runValidation()
}
</script>

<template>
  <InputStyled
    ref="inputStyledRef"
    v-model-text="model"
    as="input"
    :inactive="progress > 0 || readonly"
    :invalid="invalid"
    :type="type"
    :readonly="progress > 0 || readonly"
    :aria-invalid="invalid || undefined"
    class="pd-inputtext"
    @update:model-value="model = $event"
    @blur="onBlur"
  />
</template>
