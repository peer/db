<script setup lang="ts">
import type { ComponentPublicInstance } from "vue"

import type { ValidationError, ValidatorFn } from "@/types"

// We use v-model-text directive to mirror what Vue does on native <textarea> elements which
// we have to do ourselves because we use <textarea> element through InputStyled component.
import { computed, onBeforeUnmount, onMounted, onUpdated, useTemplateRef, vModelText } from "vue"

import InputStyled from "@/components/InputStyled.vue"
import { useValidation } from "@/validation"

const props = withDefaults(
  defineProps<{
    readonly?: boolean
    // Without a validator the textarea does not drive errors at all: errors is
    // fully owned by the parent via v-model:errors (or :errors), and
    // validate() is a no-op that returns the current errors. Pass a validator
    // to let the textarea own its own validation logic.
    validator?: ValidatorFn<string>
  }>(),
  {
    readonly: false,
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

function resize() {
  const ta = inputStyledRef.value?.$el as HTMLTextAreaElement | undefined
  if (!ta) {
    return
  }

  ta.style.height = "0"
  ta.style.height = ta.scrollHeight + "px"
}

onMounted(resize)
onUpdated(resize)

onMounted(() => {
  window.addEventListener("resize", resize, { passive: true })
})

onBeforeUnmount(() => {
  window.removeEventListener("resize", resize)
})
</script>

<template>
  <!-- We use min-h-22 to show space for 3 lines of text at minimum, so that it is visually distinct from InputText and invites more content. -->
  <InputStyled
    ref="inputStyledRef"
    v-model-text="model"
    as="textarea"
    :inactive="progress > 0 || readonly"
    :invalid="invalid"
    :readonly="progress > 0 || readonly"
    :aria-invalid="invalid || undefined"
    class="pd-textarea min-h-22 resize-none"
    @update:model-value="model = $event"
    @blur="onBlur"
  />
</template>
