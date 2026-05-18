<script setup lang="ts">
import type { ComponentPublicInstance } from "vue"

import type { ValidationError, ValidatorFn } from "@/types"

// We use v-model-text directive to mirror what Vue does on native <textarea> elements which
// we have to do ourselves because we use <textarea> element through InputStyled component.
import { computed, onBeforeUnmount, onMounted, onUpdated, ref, useTemplateRef, vModelText, watch } from "vue"

import InputStyled from "@/components/InputStyled.vue"
import { useLock } from "@/progress"
import { useValidation } from "@/validation"

const props = withDefaults(
  defineProps<{
    readonly?: boolean
    // Presentational override.
    invalid?: boolean
    // Without a validator the textarea does not drive errors at all:
    // validate() is a no-op that returns the (empty) errors list. Pass a
    // validator to let the textarea own its own validation logic; it then
    // emits "errors" whenever its computed errors change.
    validator?: ValidatorFn<string>
  }>(),
  {
    readonly: false,
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
  <!--
    We use min-h-22 to show space for 3 lines of text at minimum, so that
    it is visually distinct from InputText and invites more content.
    whitespace-break-spaces (rather than the textarea's native pre-wrap)
    keeps trailing whitespace visible at line ends and matches the
    InputHTML editor's whitespace handling, so the two whitespace-
    preserving multi-line inputs render typed content the same way.
  -->
  <InputStyled
    ref="inputStyledRef"
    v-model-text="model"
    as="textarea"
    :inactive="inactive"
    :invalid="invalid"
    :readonly="inactive"
    :aria-invalid="invalid || undefined"
    class="pd-textarea min-h-22 resize-none whitespace-break-spaces"
    @update:model-value="model = $event"
    @blur="onBlur"
  />
</template>
