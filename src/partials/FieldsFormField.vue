<!--
FieldsFormField is the per-field row inside FieldsForm. It renders the
field label + "Required / Multiple / Changed + Revert" badges in the
left column and a single ClaimCardinality in the right column.

All slot management (rows, auto-grow/shrink, required check, per-row
Add / Set / Remove via saveChange) lives in ClaimCardinality and its
ClaimInput children. FieldsFormField is intentionally thin.
-->

<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { Claim, ClaimTypes } from "@/document"
import type { FieldData } from "@/fields"

import { computed, provide, useId, useTemplateRef } from "vue"

import { fieldLabelCellKey, getClaimsForField } from "@/fields"
import ClaimCardinality from "@/partials/ClaimCardinality.vue"
import DocumentRefInline from "@/partials/DocumentRefInline.vue"
import InputBadges from "@/partials/InputBadges.vue"

const props = defineProps<{
  field: DeepReadonly<FieldData>
  // Current claims for the doc this field belongs to.
  claims: DeepReadonly<ClaimTypes>
  // Pre-session claims for the doc. ClaimCardinality and its slots use
  // this as the baseline for the per-property "changed" badge and Revert.
  initialClaims: DeepReadonly<ClaimTypes>
}>()

// Extract the claims for this specific field (by property id and claim
// type). The two computeds are bound through to ClaimCardinality so the
// per-field slot state stays reactive on doc updates.
const claimsForField = computed<readonly DeepReadonly<Claim>[]>(() => {
  return getClaimsForField(props.claims, props.field)
})

const initialClaimsForField = computed<readonly DeepReadonly<Claim>[]>(() => {
  return getClaimsForField(props.initialClaims, props.field)
})

const cardinalityRef = useTemplateRef<{
  revert: () => Promise<void>
  // isDirty is exposed as a Ref by ClaimCardinality's defineExpose but the
  // parent-side proxy unwraps it, so we read it as a plain boolean.
  isDirty: boolean
  // The field's first focusable input (or null until a slot has mounted).
  inputEl: () => HTMLElement | null
}>("cardinalityRef")

// Clicking the field label focuses the field's first input, like a <label for>.
// mousedown + preventDefault (the @mousedown.prevent in the template) sends focus
// straight there instead of blurring to the body first.
function onLabelMousedown(): void {
  cardinalityRef.value?.inputEl()?.focus()
}

// labelCellRef points to the field's <th>. We provide it down the tree
// so ClaimInput's focusout handler can detect focus moving to a control
// inside it (the field-level Revert button) and skip the per-slot
// commit, which would otherwise race the Revert click and make it
// effectively no-op on the first try.
const labelCellRef = useTemplateRef<HTMLElement>("labelCellRef")
provide(fieldLabelCellKey, () => labelCellRef.value)

// Id of the property-label text, passed to ClaimCardinality so a bare value
// input is named via aria-labelledby.
const labelId = useId()

function isRequired(): boolean {
  return props.field.minCardinality > 0
}

function isMultiple(): boolean {
  return props.field.maxCardinality > 1
}

const fieldChanged = computed<boolean>(() => cardinalityRef.value?.isDirty === true)

async function revertField(): Promise<void> {
  if (!cardinalityRef.value) return
  await cardinalityRef.value.revert()
}
</script>

<template>
  <!--
    Semantic <tbody> laid out as a 2-column CSS grid (1/5 label, rest content)
    from the md breakpoint up. Below it the row stacks into one column: the
    label (with the badges inline to its right) sits above the field like a
    sub-field header does, gap-4 to the field and pl-4 so it aligns over the
    field's content (like ClaimCardinality's showHeader). With nothing left
    of the rails then, the rows' horizontal padding also drops away.
    <tr>s use display: contents (Tailwind "contents") so their <th>/<td>
    children participate directly in the grid. The gap-x-3 keeps the label
    text as far from the rails at the value cell's left edge as the value
    content is from them: the rails' pl-4 minus the w-1 rail bar itself
    leaves 12px right of the bar.
  -->
  <tbody class="grid grid-cols-1 gap-y-4 md:grid-cols-[20%_1fr] md:items-start md:gap-x-3 md:gap-y-1 md:px-2">
    <tr class="contents">
      <th ref="labelCellRef" scope="row" class="pl-4 text-left font-medium text-gray-700 md:pl-0">
        <div class="flex flex-row flex-wrap items-center gap-1 md:flex-col md:items-start">
          <span :id="labelId" class="cursor-pointer pt-0.5 leading-none" @mousedown.prevent="onLabelMousedown"
            ><DocumentRefInline :id="field.propertyId" :link="false"
          /></span>
          <div class="flex flex-row flex-wrap gap-1">
            <InputBadges :required="isRequired()" :multiple="isMultiple()" :changed="fieldChanged" @revert="revertField" />
          </div>
        </div>
      </th>
      <td>
        <ClaimCardinality ref="cardinalityRef" :model-value="claimsForField" :initial-claims="initialClaimsForField" :field="field" :label-id="labelId" />
      </td>
    </tr>
  </tbody>
</template>
