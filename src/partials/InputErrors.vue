<script setup lang="ts">
import type { ValidationError } from "@/types"

import { computed, ref, useId, watch } from "vue"
import { useI18n } from "vue-i18n"

import { pickErrorMessage } from "@/validation"

const errors = ref<ValidationError[]>([])

const emit = defineEmits<{ errors: [ValidationError[]] }>()
watch(errors, (v) => emit("errors", v), { flush: "sync" })

defineOptions({
  inheritAttrs: false,
})

const { t } = useI18n({ useScope: "global" })

const errorId = useId()

const message = computed<string | null>(() => pickErrorMessage(errors.value, t))
</script>

<template>
  <slot v-bind="$attrs" :aria-describedby="errors.length > 0 ? errorId : undefined" @errors="(v: ValidationError[]) => (errors = v)" />
  <p v-if="message" :id="errorId" class="mt-1 text-sm text-error-600">{{ message }}</p>
</template>
