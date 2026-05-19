<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { ClaimTypes } from "@/document"
import type { FieldsData } from "@/fields"

import { useTemplateRef, watch } from "vue"

import { fieldKey } from "@/fields"
import FieldsFormField from "@/partials/FieldsFormField.vue"
import { useValidationRegistry } from "@/validation"

const props = withDefaults(
  defineProps<{
    fieldsData: DeepReadonly<FieldsData>
    claims: DeepReadonly<ClaimTypes>
    // The session's starting claims: the doc as it was BEFORE any
    // changes in this edit session were applied. Used to detect
    // session-wide "changed" status and to compute revert diffs.
    initialClaims: DeepReadonly<ClaimTypes>
    base: DeepReadonly<string[]>
    session: string
    parentClaimId?: string
  }>(),
  {
    parentClaimId: undefined,
  },
)

const invalid = defineModel<boolean>("invalid", { default: false })

// rootRef is the DOM identity for nested FieldsForm self-registration. The
// top-level FieldsForm has no parentClaimId and therefore no elGetter: its
// consumer (DocumentEdit) calls validateAll explicitly through defineExpose.
// Nested FieldsForms (rendered inside a parent FieldsFormField for a HAS
// claim's sub-fields) self-register so the outermost validateAll cascades
// through every level without a per-component template ref chain.
const rootRef = useTemplateRef<HTMLElement>("rootRef")
const elGetter = props.parentClaimId !== undefined ? () => rootRef.value : undefined

const { validateAll, resetAll, revertAll, checkpointAll, anyError, anyDirty, firstEl, inputs } = useValidationRegistry(undefined, elGetter)

// invalid bubbles per-input format errors up to the parent (DocumentEdit).
// Cardinality-level "field has too few values" is reported by each
// FieldsFormField's wrapped InputCardinality, whose own missing check
// surfaces as a row error and feeds into anyError too.
watch(anyError, (v) => (invalid.value = v), { immediate: true, flush: "sync" })

function sortedByOrder<T extends { orderInList: number }>(items: readonly T[]): T[] {
  return [...items].sort((a, b) => a.orderInList - b.orderInList)
}

defineExpose({
  validateAll,
  resetAll,
  revertAll,
  checkpointAll,
  firstEl,
  anyError,
  anyDirty,
  inputs,
})
</script>

<template>
  <!--
    Semantic table for property/value pairs, but laid out with flex + grid:
      - <table> is display: flex column with gap-y-4, so each FieldsFormField
        (a <tbody>) is one flex item separated from its neighbours.
      - Each FieldsFormField's <tbody> is its own grid (2 cols: label / content),
        with tight gap-y-1 inside the field.
      - <tr>s use display: contents (Tailwind "contents") so their <th>/<td>
        children participate in the tbody's grid directly.
    Section header sits between tbodies as its own flex item.
  -->
  <table ref="rootRef" class="flex w-full flex-col gap-y-4">
    <FieldsFormField
      v-for="field in sortedByOrder(fieldsData.fields)"
      :key="fieldKey(field)"
      :field="field"
      :claims="claims"
      :initial-claims="initialClaims"
      :base="base"
      :session="session"
      :parent-claim-id="parentClaimId"
    />

    <template v-for="section in sortedByOrder(fieldsData.sections)" :key="'section-' + section.id">
      <thead class="block">
        <tr class="block">
          <th colspan="2" class="block border-b border-slate-200 px-2 pb-1 text-left text-lg font-semibold">{{ section.id }}</th>
        </tr>
      </thead>
      <FieldsFormField
        v-for="field in sortedByOrder(section.fields)"
        :key="fieldKey(field)"
        :field="field"
        :claims="claims"
        :initial-claims="initialClaims"
        :base="base"
        :session="session"
        :parent-claim-id="parentClaimId"
      />
    </template>
  </table>
</template>
