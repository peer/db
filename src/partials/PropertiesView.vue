<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { Claim } from "@/document"

import { PencilIcon, PlusIcon, MinusIcon } from "@heroicons/vue/20/solid"
import { computed } from "vue"
import { useI18n } from "vue-i18n"

import Button from "@/components/Button.vue"
import { ClaimTypes, claimTypeName } from "@/document"
import ClaimValue from "@/partials/ClaimValue.vue"
import DocumentRefInline from "@/partials/DocumentRefInline.vue"

const props = withDefaults(
  defineProps<{
    claims: DeepReadonly<ClaimTypes>
    // True when this instance renders the sub-claims of a claim (the recursive calls set it). Like FieldsView,
    // a nested instance spans its rows across both columns instead of leaving an empty label cell, so deeper
    // sub-claims indent under the sub-claim's label rather than in the value column.
    nested?: boolean
    // When editable, each claim gets Edit / Sub-value / Remove icon buttons to the right of its label, and the
    // property label is repeated per value instead of shown once.
    editable?: boolean
    // ID of the claim currently being edited, so its Edit button renders primary to highlight which row
    // populated the form.
    editingClaimId?: string | null
    // ID of the claim under which a sub-claim is currently being added, so its Sub-value button renders primary.
    subClaimParentId?: string | null
  }>(),
  {
    nested: false,
    editable: false,
    editingClaimId: null,
    subClaimParentId: null,
  },
)

const emit = defineEmits<{
  editClaim: [value: string]
  removeClaim: [value: string]
  subClaim: [value: string]
}>()

const { t } = useI18n({ useScope: "global" })

// Ensure claims is a proper ClaimTypes instance (props may receive raw JSON from WithDocument).
const normalizedClaims = computed(() => {
  if (!props.claims) {
    return new ClaimTypes({})
  }
  if (props.claims instanceof ClaimTypes) {
    return props.claims
  }
  return new ClaimTypes(props.claims as unknown as Record<string, object[]>)
})

// One entry per top-level claim (AllClaims returns only top-level claims, in claim-type bucket order, not
// sub-claims). firstOfProperty marks the first claim of each property: read-only shows the property label
// only there (values stack under one label, like FieldsView); editable repeats it so every value is labelled.
const rows = computed(() => {
  const seen = new Set<string>()
  return (normalizedClaims.value.AllClaims() as DeepReadonly<Claim>[]).map((claim) => {
    const firstOfProperty = !seen.has(claim.prop.id)
    seen.add(claim.prop.id)
    const typeName = claimTypeName(claim)
    return {
      claim,
      propId: claim.prop.id,
      firstOfProperty,
      typeName,
      hasSub: !!claim.sub && claim.sub.AllClaims().length > 0,
      // A value-less claim renders nothing for its own value (ClaimValueHas is an empty span), so its sub-claims move up into the value cell of the label row
      // instead of sitting a line below an empty value. HAS is the only such type: none and unknown still render a label ("none"/"unknown"), so they are not value-less.
      valueless: typeName === "has",
    }
  })
})

function onEdit(id: string) {
  emit("editClaim", id)
}

function onSubClaim(id: string) {
  emit("subClaim", id)
}

function onRemove(id: string) {
  emit("removeClaim", id)
}

const hasContent = computed(() => rows.value.length > 0)
</script>

<template>
  <!--
    All of the document's properties and their values, matching FieldsView's layout: the table is laid out as a
    CSS grid (the tbody is the grid and the tr elements are display: contents), so it reflows on narrow viewports.
    Below sm it is a single column (the property label on its own line, then the value below it); from sm up it is
    a two-column grid. Read-only shows a property's label once with its values stacked; the empty label cells for
    the extra values and the sub-claim indent cell are dropped below sm so a stacked value does not sit under a
    blank line. Sub-claims render recursively, like FieldsView's sub-fields. When editable, each claim also gets
    Edit / Sub-value / Remove icon buttons following FieldsForm's badge placement (in a row under the property
    label from sm up, to the right of it below sm), and the label is repeated per value.
  -->
  <table v-if="hasContent" class="flex w-full flex-col">
    <tbody class="grid grid-cols-1 sm:grid-cols-[20%_1fr] sm:gap-x-3">
      <template v-for="row in rows" :key="row.claim.id">
        <tr class="contents">
          <td v-if="editable || row.firstOfProperty" class="px-2 py-1 align-top">
            <div class="flex flex-row flex-wrap items-center gap-x-2 gap-y-1 sm:flex-col sm:items-start">
              <span class="font-medium text-gray-700" :class="{ 'leading-none sm:pt-0.5': editable }"><DocumentRefInline :id="row.propId" :link="false" /></span>
              <div v-if="editable" class="flex flex-row items-center gap-0.5">
                <Button
                  type="button"
                  :primary="editingClaimId === row.claim.id"
                  :disabled="editingClaimId === row.claim.id"
                  class="px-0.5 py-0.5"
                  @click.prevent="onEdit(row.claim.id)"
                  ><PencilIcon class="size-3" :alt="t('common.buttons.edit')"
                /></Button>
                <Button
                  type="button"
                  :primary="subClaimParentId === row.claim.id"
                  :disabled="subClaimParentId === row.claim.id"
                  class="px-0.5 py-0.5"
                  @click.prevent="onSubClaim(row.claim.id)"
                  ><PlusIcon class="size-3" :alt="t('common.buttons.subClaim')"
                /></Button>
                <Button
                  type="button"
                  class="px-0.5 py-0.5"
                  @click.prevent="onRemove(row.claim.id)"
                  ><MinusIcon class="size-3" :alt="t('common.buttons.remove')"
                /></Button>
              </div>
            </div>
          </td>
          <td v-else class="hidden sm:block"></td>
          <!--
            A value-less claim (see rows) renders no value of its own, so when it has sub-claims they sit directly
            in the value cell of the label row (aligned with the label) instead of leaving an empty value line
            above them. The p-0 cell lets the nested table's label cells (px-2, like this table's value cells)
            align with sibling values.
          -->
          <td v-if="row.valueless && row.hasSub" class="p-0 align-top">
            <PropertiesView
              :claims="row.claim.sub!"
              nested
              :editable="editable"
              :editing-claim-id="editingClaimId"
              :sub-claim-parent-id="subClaimParentId"
              @edit-claim="onEdit"
              @remove-claim="onRemove"
              @sub-claim="onSubClaim"
            />
          </td>
          <td v-else class="px-2 pt-0 pb-1 align-top text-gray-700 sm:pt-1">
            <ClaimValue :claim="row.claim" :type="row.typeName" />
          </td>
        </tr>
        <!--
          Sub-claims of a claim that renders a value; a value-less claim's sub-claims already sit in its value
          cell above. In a top-level instance the nested table sits in the value column (under the value, indented
          by this cell's px-2). In a nested instance it spans both columns (sm:col-span-2), so deeper sub-claims
          indent under the sub-claim's label rather than its value.
        -->
        <tr v-if="row.hasSub && !row.valueless" class="contents">
          <td v-if="!nested" class="hidden sm:block"></td>
          <td class="px-2 py-0 align-top" :class="{ 'sm:col-span-2': nested }">
            <PropertiesView
              :claims="row.claim.sub!"
              nested
              :editable="editable"
              :editing-claim-id="editingClaimId"
              :sub-claim-parent-id="subClaimParentId"
              @edit-claim="onEdit"
              @remove-claim="onRemove"
              @sub-claim="onSubClaim"
            />
          </td>
        </tr>
      </template>
    </tbody>
  </table>
</template>
