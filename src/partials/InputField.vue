<script setup lang="ts">
import type { ComponentPublicInstance, ShallowUnwrapRef } from "vue"

import type { ValidatedInput } from "@/types"

import { shallowRef } from "vue"

import InputBadges from "@/partials/InputBadges.vue"
import InputErrors from "@/partials/InputErrors.vue"

defineProps<{
  required?: boolean
  // Presentational override.
  invalid?: boolean
}>()

const input = shallowRef<ShallowUnwrapRef<ValidatedInput> | null>(null)

// The parameter is typed against Vue's VNodeRef signature so the function
// can be spread via v-bind="inputProps" onto any component without TS
// narrowing complaints. At runtime the consumer's input is a validated
// component instance whose defineExpose makes its ValidatedInput shape
// available with refs auto-unwrapped (ShallowUnwrapRef).
function setInputRef(i: Element | ComponentPublicInstance | null) {
  input.value = i as ShallowUnwrapRef<ValidatedInput> | null
}

// Simulates the click-to-focus behavior of a <label for=...>.
// A click on the legend focuses the wrapped input's focus target.
function onLegendClick(event: MouseEvent): void {
  const target = event.target as HTMLElement | null
  // We replicate HTML's "interactive content" exception so a click on
  // or inside a focusable descendant (e.g. the InputBadges' revert button)
  // keeps its own behavior instead of also moving focus into the input.
  if (target?.closest("a[href], button, input, select, textarea, details, [tabindex]:not([tabindex='-1'])")) return
  input.value?.el()?.focus()
}
</script>

<template>
  <fieldset v-tw-merge class="flex flex-col">
    <legend class="mb-1 flex flex-row items-center gap-1" @click="onLegendClick"
      ><slot name="label" /><InputBadges :required="required" :changed="input?.isDirty ?? false" @revert="input?.revert()"
    /></legend>
    <InputErrors v-slot="errorProps">
      <slot v-bind="errorProps" :ref="setInputRef" name="input" :required="required" :invalid="invalid" />
    </InputErrors>
  </fieldset>
</template>
