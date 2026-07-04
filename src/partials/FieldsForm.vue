<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { ClaimTypes } from "@/document"
import type { FieldData, FieldsData } from "@/fields"

import { watch } from "vue"
import { useI18n } from "vue-i18n"

import { fieldKey, getSectionName, isSimpleField } from "@/fields"
import FieldsFormField from "@/partials/FieldsFormField.vue"
import { useValidationRegistry } from "@/validation"

defineProps<{
  fieldsData: DeepReadonly<FieldsData>
  claims: DeepReadonly<ClaimTypes>
  // The session's starting claims: the doc as it was BEFORE any
  // changes in this edit session were applied. Used to detect
  // session-wide "changed" status and to compute revert diffs.
  initialClaims: DeepReadonly<ClaimTypes>
  base: DeepReadonly<string[]>
  session: string
}>()

const invalid = defineModel<boolean>("invalid", { default: false })

const { locale } = useI18n({ useScope: "global" })

// Top-level FieldsForm: its consumer (DocumentEdit) calls validateAll /
// revertAll / checkpointAll explicitly through defineExpose. There are no
// recursive FieldsForm instances anymore - sub-claim rendering lives in
// ClaimInput / ClaimCardinality, which provide their own registries.
const { validateAll, resetAll, revertAll, checkpointAll, anyError, anyDirty, firstInputEl, inputs } = useValidationRegistry()

// invalid bubbles per-input format errors up to the parent (DocumentEdit).
// Cardinality-level "field has too few values" is reported by each
// FieldsFormField's wrapped ClaimCardinality and feeds into anyError too.
watch(anyError, (v) => (invalid.value = v), { immediate: true, flush: "sync" })

function sortedByOrder<T extends { orderInList: number }>(items: readonly T[]): T[] {
  return [...items].sort((a, b) => a.orderInList - b.orderInList)
}

// A group of sibling fields uses wider spacing (gap-8) once any member is
// non-simple (repeats or has sub-fields); otherwise the fields are simple and
// sit close together (gap-4).
function groupGapClass(fields: readonly DeepReadonly<FieldData>[]): string {
  return fields.some((field) => !isSimpleField(field)) ? "gap-y-8" : "gap-y-4"
}

defineExpose({
  validateAll,
  resetAll,
  revertAll,
  checkpointAll,
  firstInputEl,
  anyError,
  anyDirty,
  inputs,
})
</script>

<template>
  <!--
    Property/value rows laid out with flex + grid. The <table>/<tbody> elements
    are display:flex/grid (not real table layout): each field group is its own
    flex <table> of FieldsFormField <tbody>s. Spacing follows field "simplicity":
    a group uses gap-8 once any field is non-simple (repeats or has sub-fields),
    else gap-4; sections are separated by gap-12.
  -->
  <div class="flex w-full flex-col gap-y-12">
    <table v-if="fieldsData.fields.length > 0" class="flex flex-col" :class="groupGapClass(fieldsData.fields)">
      <FieldsFormField
        v-for="field in sortedByOrder(fieldsData.fields)"
        :key="fieldKey(field)"
        :field="field"
        :claims="claims"
        :initial-claims="initialClaims"
        :base="base"
        :session="session"
      />
    </table>

    <div v-for="section in sortedByOrder(fieldsData.sections)" :key="'section-' + section.id" class="flex flex-col gap-y-4">
      <div class="border-b border-slate-200 px-2 pb-1 text-lg font-semibold">{{ getSectionName(section, locale) }}</div>
      <table class="flex flex-col" :class="groupGapClass(section.fields)">
        <FieldsFormField
          v-for="field in sortedByOrder(section.fields)"
          :key="fieldKey(field)"
          :field="field"
          :claims="claims"
          :initial-claims="initialClaims"
          :base="base"
          :session="session"
        />
      </table>
    </div>
  </div>
</template>
