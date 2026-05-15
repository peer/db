<script setup lang="ts">
import type { ComponentPublicInstance } from "vue"

import type { ValidationError, ValidatorFn } from "@/types"

// We use v-model-text directive to mirror what Vue does on native <input> elements which
// we have to do ourselves because we use <input> element through InputStyled component.
import { computed, ref, useTemplateRef, vModelText, watch } from "vue"

import InputStyled from "@/components/InputStyled.vue"
import { useLock } from "@/progress"
import { useValidation } from "@/validation"

const props = withDefaults(
  defineProps<{
    readonly?: boolean
    type?: string
    // Presentational override.
    invalid?: boolean
    // Without a validator the input does not drive errors at all:
    // validate() is a no-op that returns the (empty) errors list. Pass a
    // validator to let the input own its own validation logic. The input
    // then emits "errors" whenever its computed errors change.
    validator?: ValidatorFn<string>
  }>(),
  {
    readonly: false,
    type: "text",
    invalid: false,
    validator: undefined,
  },
)

const model = defineModel<string>({ default: "" })
const errors = ref<ValidationError[]>([])

const emit = defineEmits<{ errors: [ValidationError[]] }>()
watch(errors, (v) => emit("errors", v), { flush: "sync" })

const invalid = computed(() => props.invalid || errors.value.length > 0)

// Data modification and controls.
const lock = useLock()
const inactive = computed(() => lock.value > 0 || props.readonly)

const inputStyledRef = useTemplateRef<ComponentPublicInstance>("inputStyledRef")

const { runValidation, validatedInput } = useValidation(
  model,
  errors,
  lock,
  () => props.validator,
  () => inputStyledRef.value?.$el ?? null,
  () => {
    model.value = ""
    errors.value = []
  },
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
    :inactive="inactive"
    :invalid="invalid"
    :type="type"
    :readonly="inactive"
    :aria-invalid="invalid || undefined"
    class="pd-inputtext"
    @update:model-value="model = $event"
    @blur="onBlur"
  />
</template>
