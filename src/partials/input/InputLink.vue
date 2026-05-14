<script setup lang="ts">
import type { ShallowUnwrapRef } from "vue"

import type { ValidatedInput, ValidationError, ValidatorFn } from "@/types"

import { ArrowTopRightOnSquareIcon } from "@heroicons/vue/20/solid"
import { computed, useTemplateRef } from "vue"
import { useRouter } from "vue-router"

import InputText from "@/components/InputText.vue"
import { classifyLink, LINK_CLASS_INTERNAL, LINK_CLASS_INTERNAL_NOVIEW } from "@/internal-links"

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

const router = useRouter()

const canOpen = computed(() => {
  const trimmed = model.value.trim()
  if (!trimmed) return false
  try {
    new URL(trimmed)
  } catch {
    return false
  }
  return true
})
const linkClasses = computed(() => {
  const trimmed = model.value.trim()
  if (!trimmed) return []
  return classifyLink(trimmed, router)
})
const internalPath = computed<string | null>(() => {
  if (!linkClasses.value.includes(LINK_CLASS_INTERNAL)) return null
  const trimmed = model.value.trim()
  try {
    const url = new URL(trimmed)
    return url.pathname + url.search + url.hash
  } catch {
    return null
  }
})
const useRouterLink = computed(() => internalPath.value !== null && !linkClasses.value.includes(LINK_CLASS_INTERNAL_NOVIEW))

// A link is invalid if it does not parse as an absolute URL via the URL
// constructor. As a side effect of validation the model is normalized to the
// re-stringified URL (so "https://Example.com" becomes "https://example.com/",
// surrounding whitespace is stripped, etc.). The normalization is gated on
// !eager so the user is not fighting the input while typing, and on !initial
// so the field is not mutated before the user has interacted. The required
// check is also skipped on initial, but URL-parse failure is still reported
// so a pre-populated invalid link surfaces immediately.
// eslint-disable-next-line @typescript-eslint/require-await
const validator: ValidatorFn<string> = async function (value, options) {
  const trimmed = value.trim()
  if (trimmed === "") {
    if (!options.eager && !options.initial && trimmed !== model.value) {
      model.value = trimmed
    }
    if (!props.required || options.initial) {
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
  if (!options.eager && !options.initial && normalized !== model.value) {
    model.value = normalized
  }
  return []
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
  el: () => inputTextRef.value?.el() ?? null,
  isDirty: computed<boolean>(() => inputTextRef.value?.isDirty ?? false),
  setBaseline: () => inputTextRef.value?.setBaseline(),
}
defineExpose(validatedInput)
</script>

<template>
  <div class="relative">
    <!--
      pr-9 reserves space on the right for the absolutely-positioned open-link
      icon overlay so the input text does not slide underneath it.
    -->
    <InputText ref="inputTextRef" v-model="model" v-model:errors="errors" :readonly="readonly" :validator="validator" class="w-full" :class="canOpen ? 'pr-9' : ''" />
    <div v-if="canOpen" class="absolute inset-y-0 right-0 flex items-center pr-2">
      <RouterLink v-if="useRouterLink && internalPath" :to="internalPath" class="link">
        <ArrowTopRightOnSquareIcon class="size-5" aria-hidden="true" />
      </RouterLink>
      <a v-else :href="model.trim()" class="link" rel="noreferrer">
        <ArrowTopRightOnSquareIcon class="size-5" aria-hidden="true" />
      </a>
    </div>
  </div>
</template>
