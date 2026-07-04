<script setup lang="ts">
import type { ShallowUnwrapRef } from "vue"

import type { ValidatedInput, ValidationError, ValidatorFn } from "@/types"

import { ArrowTopRightOnSquareIcon } from "@heroicons/vue/20/solid"
import { computed, useTemplateRef } from "vue"
import { useI18n } from "vue-i18n"
import { useRouter } from "vue-router"

import InputText from "@/components/InputText.vue"
import { classifyLink, LINK_CLASS_INTERNAL, LINK_CLASS_INTERNAL_NOVIEW } from "@/internal-links"
import { normalizeUrl, parseUrl } from "@/utils"

const props = withDefaults(
  defineProps<{
    readonly?: boolean
    required?: boolean
    // Presentational override.
    invalid?: boolean
    allowContact?: boolean
  }>(),
  {
    readonly: false,
    required: false,
    invalid: false,
    allowContact: true,
  },
)

const model = defineModel<string>({ default: "" })

const emit = defineEmits<{ errors: [ValidationError[]] }>()

const { t } = useI18n({ useScope: "global" })

const router = useRouter()

// An example URL, shown under the input (when there is no error) so the
// expected format is clear.
const hints = computed<string[]>(() => [t("partials.input.InputLink.hint")])

const canOpen = computed(() => {
  const trimmed = model.value.trim()
  if (!trimmed) return false
  try {
    parseUrl(trimmed, { allowContact: props.allowContact })
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
  try {
    return normalizeUrl(model.value.trim(), { allowContact: props.allowContact })
  } catch {
    return null
  }
})
const useRouterLink = computed(() => internalPath.value !== null && !linkClasses.value.includes(LINK_CLASS_INTERNAL_NOVIEW))

// A link is invalid if it does not parse as a URL via parseUrl (which
// accepts both absolute URLs and site-relative paths beginning with "/").
// As a side effect of validation the model is normalized: same-origin URLs
// collapse to "/path?query#hash" so they match the leading-slash convention
// used by InputFile and by Link.vue's display, and external URLs are
// re-stringified (so "https://Example.com" becomes "https://example.com/",
// surrounding whitespace is stripped, etc.). The normalization is gated on
// !eager so the user is not fighting the input while typing, and on !initial
// so the field is not mutated before the user has interacted. The required
// check is also skipped on initial and while eager, so it flags only on the lazy
// blur pass; URL-parse failure is not skipped, so a pre-populated or freshly
// typed invalid link surfaces immediately.
// eslint-disable-next-line @typescript-eslint/require-await
const validator: ValidatorFn<string> = async function (value, options) {
  const trimmed = value.trim()
  if (trimmed === "") {
    if (!options.eager && !options.initial && trimmed !== model.value) {
      model.value = trimmed
    }
    if (!props.required || options.initial || options.eager) {
      return []
    }
    // TODO: Use standard codes.
    return [{ code: "required" }]
  }
  let normalized: string
  try {
    normalized = normalizeUrl(trimmed, { allowContact: props.allowContact })
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
    await inputTextRef.value?.validate(signal)
  },
  reset: () => inputTextRef.value?.reset(),
  revert: () => inputTextRef.value?.revert(),
  inputEl: () => inputTextRef.value?.inputEl() ?? null,
  mainEl: () => inputTextRef.value?.mainEl() ?? null,
  isDirty: computed<boolean>(() => inputTextRef.value?.isDirty ?? false),
  isEmpty: computed<boolean>(() => inputTextRef.value?.isEmpty ?? true),
  errors: computed<ValidationError[]>(() => inputTextRef.value?.errors ?? []),
  checkpoint: () => inputTextRef.value?.checkpoint(),
  hints,
}
defineExpose(validatedInput)
</script>

<template>
  <div class="relative">
    <!--
      pr-9 reserves space on the right for the absolutely-positioned open-link
      icon overlay so the input text does not slide underneath it.
    -->
    <InputText
      ref="inputTextRef"
      v-model="model"
      :readonly="readonly"
      :invalid="invalid"
      :validator="validator"
      class="w-full"
      :class="canOpen ? 'pr-9' : ''"
      @errors="(v: ValidationError[]) => emit('errors', v)"
    />
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
