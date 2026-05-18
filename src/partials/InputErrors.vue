<script setup lang="ts">
import type { ValidationError } from "@/types"

import { computed, ref, useId, watch } from "vue"
import { useI18n } from "vue-i18n"

const errors = ref<ValidationError[]>([])

const emit = defineEmits<{ errors: [ValidationError[]] }>()
watch(errors, (v) => emit("errors", v), { flush: "sync" })

defineOptions({
  inheritAttrs: false,
})

const { t } = useI18n({ useScope: "global" })

const errorId = useId()

// Ordered map of error codes to their translated messages. Insertion order is
// the priority order: when multiple errors are present, the message for the
// earliest matching code wins. Values are t() call results (not just keys)
// so static analysis can pick the translation keys up.
const codeMap = computed<Record<string, string>>(() => ({
  required: t("common.validation.required"),
  invalid: t("common.validation.invalid"),
  requiredPrecision: t("common.validation.requiredPrecision"),
  invalidPrecision: t("common.validation.invalidPrecision"),
}))

const message = computed<string | null>(() => {
  if (errors.value.length === 0) {
    return null
  }
  const map = codeMap.value
  for (const code of Object.keys(map)) {
    for (const e of errors.value) {
      if (e.code === code) {
        return e.userMessage || map[code]
      }
    }
  }
  // Fallback when none of the error codes are in the map.
  return t("common.validation.invalid")
})
</script>

<template>
  <slot v-bind="$attrs" :aria-describedby="errors.length > 0 ? errorId : undefined" @errors="(v: ValidationError[]) => (errors = v)" />
  <p v-if="message" :id="errorId" class="mt-1 text-sm text-error-600">{{ message }}</p>
</template>
