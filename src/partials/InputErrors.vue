<script setup lang="ts">
import type { ValidationError } from "@/types"

import { computed, useId } from "vue"
import { useI18n } from "vue-i18n"

// errors is a pure pass-through v-model. Callers do not have to bind it: if
// omitted, the slotted input writes to the local model and only this
// component reads it. Callers that want to inspect (or contribute to) the
// errors should bind v-model on InputErrors rather than on the slotted input.
const errors = defineModel<ValidationError[]>({ default: () => [] })

const { t } = useI18n({ useScope: "global" })

const errorId = useId()

// Ordered map of error codes to their translated messages. Insertion order is
// the priority order: when multiple errors are present, the message for the
// earliest matching code wins. Values are t() call results (not just keys)
// so static analysis can pick the translation keys up.
const codeMap = computed<Record<string, string>>(() => ({
  required: t("common.validation.required"),
  invalid: t("common.validation.invalid"),
}))

const message = computed<string | null>(() => {
  if (errors.value.length === 0) {
    return null
  }
  const map = codeMap.value
  for (const code of Object.keys(map)) {
    if (errors.value.some((e) => e.code === code)) {
      return map[code]
    }
  }
  // Fallback when none of the error codes are in the map.
  return t("common.validation.invalid")
})
</script>

<template>
  <slot :errors="errors" :aria-describedby="errors.length > 0 ? errorId : undefined" @update:errors="(v: ValidationError[]) => (errors = v)" />
  <p v-if="message" :id="errorId" class="mt-1 text-sm text-error-600">{{ message }}</p>
</template>
