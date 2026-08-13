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
    const subClaims = (claim.sub?.AllClaims() ?? []) as DeepReadonly<Claim>[]
    return {
      claim,
      propId: claim.prop.id,
      firstOfProperty,
      typeName,
      hasSub: subClaims.length > 0,
      // The sub-claim of a value-less claim when that is all the claim carries: one value-less claim with nothing under it,
      // a marker like "selection". Such a claim has nothing to lay out, so the marker's label is rendered where a value
      // would be instead of through a table of its own, whose label column is a share (20%) of the cell holding it and
      // wraps a marker of a few words once the nesting is deep enough.
      loneMarker: typeName === "has" && subClaims.length === 1 && claimTypeName(subClaims[0]) === "has" && subClaims[0].Size() === 0 ? subClaims[0] : null,
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
  <table v-if="hasContent" class="pd-propertiesview flex w-full flex-col">
    <tbody class="pd-propertiesview-body grid grid-cols-1 sm:grid-cols-[20%_1fr] sm:gap-x-3">
      <template v-for="row in rows" :key="row.claim.id">
        <!--
          Every row carries the property it is for in its class name. A property's label is rendered once,
          on the first of its rows, so a row other than that one cannot be told which property it belongs
          to from what it renders, while the rows of one property are what code driving the view addresses.
        -->
        <tr class="pd-propertiesview-row contents" :class="`pd-propertiesview-row-${row.propId}`">
          <td v-if="editable || row.firstOfProperty" class="pd-propertiesview-label px-2 py-1 align-top">
            <div class="flex flex-row flex-wrap items-center gap-x-2 gap-y-1 sm:flex-col sm:items-start">
              <span class="pd-propertiesview-label-text font-medium text-gray-700" :class="{ 'leading-none sm:pt-0.5': editable }"
                ><DocumentRefInline :id="row.propId" :link="false"
              /></span>
              <div v-if="editable" class="pd-propertiesview-actions flex flex-row items-center gap-0.5">
                <Button
                  type="button"
                  :primary="editingClaimId === row.claim.id"
                  :disabled="editingClaimId === row.claim.id"
                  class="pd-propertiesview-button-edit px-0.5 py-0.5"
                  @click.prevent="onEdit(row.claim.id)"
                  ><PencilIcon class="size-3" :alt="t('common.buttons.edit')"
                /></Button>
                <Button
                  type="button"
                  :primary="subClaimParentId === row.claim.id"
                  :disabled="subClaimParentId === row.claim.id"
                  class="pd-propertiesview-button-subclaim px-0.5 py-0.5"
                  @click.prevent="onSubClaim(row.claim.id)"
                  ><PlusIcon class="size-3" :alt="t('common.buttons.subClaim')"
                /></Button>
                <Button type="button" class="pd-propertiesview-button-remove px-0.5 py-0.5" @click.prevent="onRemove(row.claim.id)"
                  ><MinusIcon class="size-3" :alt="t('common.buttons.remove')"
                /></Button>
              </div>
            </div>
          </td>
          <td v-else class="hidden sm:block"></td>
          <!--
            A value-less claim which carries nothing but a marker (see rows) puts the marker's label where a value would
            be, at every depth. When editable the marker's own buttons follow it under the value, the way the buttons of
            the claim this cell belongs to sit under its label, so the marker stays editable without a table of its own.
          -->
          <td v-if="row.loneMarker" class="pd-propertiesview-value px-2 pt-0 pb-1 align-top text-gray-700 sm:pt-1">
            <div class="flex flex-row flex-wrap items-center gap-x-2 gap-y-1 sm:flex-col sm:items-start">
              <span :class="{ 'leading-none sm:pt-0.5': editable }"><DocumentRefInline :id="row.loneMarker.prop.id" :link="false" /></span>
              <div v-if="editable" class="flex flex-row items-center gap-0.5">
                <Button
                  type="button"
                  :primary="editingClaimId === row.loneMarker.id"
                  :disabled="editingClaimId === row.loneMarker.id"
                  class="pd-propertiesview-button-edit px-0.5 py-0.5"
                  @click.prevent="onEdit(row.loneMarker!.id)"
                  ><PencilIcon class="size-3" :alt="t('common.buttons.edit')"
                /></Button>
                <Button
                  type="button"
                  :primary="subClaimParentId === row.loneMarker.id"
                  :disabled="subClaimParentId === row.loneMarker.id"
                  class="pd-propertiesview-button-subclaim px-0.5 py-0.5"
                  @click.prevent="onSubClaim(row.loneMarker!.id)"
                  ><PlusIcon class="size-3" :alt="t('common.buttons.subClaim')"
                /></Button>
                <Button type="button" class="pd-propertiesview-button-remove px-0.5 py-0.5" @click.prevent="onRemove(row.loneMarker!.id)"
                  ><MinusIcon class="size-3" :alt="t('common.buttons.remove')"
                /></Button>
              </div>
            </div>
          </td>
          <!--
            A value-less claim (HAS, see rows) renders no value. At the top level its sub-claims sit in the value cell, so
            the first sub-claim aligns to the right of the label (where a value would be) from sm up, and indents one step
            (pl-2) below sm so it sits under the label. In a nested instance the value column is left empty (an empty cell
            keeps the grid aligned from sm up, hidden below sm so it adds no line) and the sub-claims stair-step in the
            sub-row below, so a deep HAS chain does not march across the value columns.
          -->
          <td v-else-if="row.valueless && !nested && row.hasSub" class="pd-propertiesview-value py-0 pr-0 pl-2 align-top sm:pl-0">
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
          <td v-else-if="row.valueless" class="hidden sm:block"></td>
          <td v-else class="pd-propertiesview-value px-2 pt-0 pb-1 align-top text-gray-700 sm:pt-1">
            <ClaimValue :claim="row.claim" :type="row.typeName" />
          </td>
        </tr>
        <!--
          Sub-claims render indented below the claim. A top-level value-less (HAS) claim's sub-claims already sit in its
          value cell above, as does the marker of a claim which carries nothing else, so only value claims and nested HAS
          claims render here. In a top-level instance the sub-table sits in the value column (under the value); in a
          nested instance it spans both columns (sm:col-span-2) and indents by this cell's px-2 under the label, so
          deeper sub-claims stair-step down per level.
        -->
        <tr
          v-if="row.hasSub && !row.loneMarker && !(row.valueless && !nested)"
          class="pd-propertiesview-row-sub contents"
          :class="`pd-propertiesview-row-sub-${row.propId}`"
        >
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
