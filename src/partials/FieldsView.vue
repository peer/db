<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { Claim } from "@/document"
import type { FieldData, FieldsData, SectionData } from "@/fields"

import { computed, inject, onBeforeUnmount, ref, watch } from "vue"
import { useI18n } from "vue-i18n"

import Button from "@/components/Button.vue"
import { IN_LANGUAGE } from "@/core"
import { ClaimTypes, claimTypeName, getClaimsOfTypeWithConfidence, selectClaimsByLanguage } from "@/document"
import { fieldKey, getClaimsForField, getSectionName, valueTypeToClaimType } from "@/fields"
import ClaimValue from "@/partials/ClaimValue.vue"
import DocumentRefInline from "@/partials/DocumentRefInline.vue"
import { searchHiddenClaimsKey, searchLoadAllClaimsKey, SKIP_TO_END } from "@/utils"

const props = withDefaults(
  defineProps<{
    fieldsData: DeepReadonly<FieldsData>
    claims: DeepReadonly<ClaimTypes>
    sections?: boolean
    // When limited is true, fields whose property repeats show only the first few claim values, with a
    // "Show all" button to reveal the rest. Used in compact contexts like search result cards.
    limited?: boolean
    // True when this instance renders the sub-fields of a claim (the recursive calls set it). It
    // changes where this instance places its OWN sub-field tables: a top-level instance puts them
    // in the value column (sub-fields sit under the field's value, slightly indented), while a
    // nested instance spans them across both columns (deeper sub-fields sit under the sub-field's
    // label, slightly indented, with their values landing slightly right of its value).
    nested?: boolean
  }>(),
  {
    sections: false,
    limited: false,
    nested: false,
  },
)

const { t, locale } = useI18n({ useScope: "global" })

// Number of repeating claim values shown for a field before the "Show all" button when limited is true.
const LIMITED_CLAIMS = 3

// Set by the search print view's "Load all" button (see searchLoadAllClaimsKey). When true, every repeating
// claim value is shown regardless of the limit, so a printout is not cut off at LIMITED_CLAIMS.
const loadAllClaims = inject(searchLoadAllClaimsKey, null)

// Field keys (see fieldKey) for fields whose repeating claim values have been expanded via "Show all".
const expandedFields = ref(new Set<string>())

function expandField(field: FieldData): void {
  expandedFields.value = new Set(expandedFields.value).add(fieldKey(field))
}

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

function sortedByOrder<T extends { orderInList: number }>(items: readonly T[]): T[] {
  return [...items].sort((a, b) => a.orderInList - b.orderInList)
}

// Check if any claims for a field have IN_LANGUAGE sub-claims in the actual data.
function hasLanguageClaims(field: FieldData): boolean {
  const claims = getClaimsForField(normalizedClaims.value, field)
  return claims.some((claim) => claim.sub && getClaimsOfTypeWithConfidence(claim.sub, "ref", IN_LANGUAGE).length > 0)
}

// Get the claims to display for a field. If claims have IN_LANGUAGE sub-claims, only claims
// matching the current locale (with fallbacks) are returned. Each claim is rendered by its own
// type (see claimTypeName in the template), so a field whose claims span more than one type (a
// value field whose default lets it also hold none/unknown claims) renders each correctly.
function claimsForField(field: FieldData): DeepReadonly<Claim>[] {
  if (hasLanguageClaims(field)) {
    const claimType = valueTypeToClaimType(field.valueType)
    const claims = selectClaimsByLanguage(normalizedClaims.value, claimType, field.propertyId, locale.value, (c) => c.length > 0)
    return claims ?? []
  }
  return getClaimsForField(normalizedClaims.value, field)
}

// Check if a field has any claim values.
function hasValues(field: FieldData): boolean {
  return claimsForField(field).length > 0
}

// The claim values to render for a field. When limited and the field is not expanded, repeating values
// are capped at LIMITED_CLAIMS so the remaining ones stay hidden behind the "Show all" button. If capping
// would leave only SKIP_TO_END or fewer values hidden, all values are shown instead of the button. The print
// view's "Load all" button (loadAllClaims) lifts the cap entirely so a printout shows every value.
function displayedClaimsForField(field: FieldData): DeepReadonly<Claim>[] {
  const claims = claimsForField(field)
  if (props.limited && !loadAllClaims?.value && !expandedFields.value.has(fieldKey(field)) && claims.length > LIMITED_CLAIMS + SKIP_TO_END) {
    return claims.slice(0, LIMITED_CLAIMS)
  }
  return claims
}

// Count of claim values currently hidden for a field (zero unless limited and the field is collapsed).
function hiddenClaimsCount(field: FieldData): number {
  return claimsForField(field).length - displayedClaimsForField(field).length
}

// Report to the search print view whether any field this instance renders still has repeating claim values hidden,
// so its "Load all" button can appear even when every result already fits. Sub-fields render their own FieldsView
// and report separately, so only this instance's own fields count here.
const reportHiddenClaims = inject(searchHiddenClaimsKey, null)

const hasHiddenClaims = computed(() => {
  const fields = [...props.fieldsData.fields]
  if (props.sections) {
    for (const section of props.fieldsData.sections) {
      fields.push(...section.fields)
    }
  }
  return fields.some((field) => hiddenClaimsCount(field) > 0)
})

// Emit each transition as a +1/-1 delta, guarded by reported so this instance is only ever counted once, and on
// unmount release a contribution that is still standing, so the print view's total stays balanced. Syncing the
// watcher keeps reported in lockstep with hasHiddenClaims so the count is accurate the moment a card loads.
let reported = 0
watch(
  hasHiddenClaims,
  (now) => {
    if (now && reported === 0) {
      reportHiddenClaims?.(1)
      reported = 1
    } else if (!now && reported === 1) {
      reportHiddenClaims?.(-1)
      reported = 0
    }
  },
  { immediate: true, flush: "sync" },
)

onBeforeUnmount(() => {
  if (reported > 0) {
    reportHiddenClaims?.(-reported)
  }
})

// Get sub-claims for a specific claim (for recursive sub-fields).
function getSubClaims(claimId: string): ClaimTypes {
  const claim = normalizedClaims.value.GetByID(claimId)
  return new ClaimTypes(claim?.sub ?? {})
}

// Check if any top-level field has values.
const hasAnyFieldValues = computed(() => props.fieldsData.fields.some(hasValues))

// Check if any section field has values.
const hasAnySectionValues = computed(() => props.fieldsData.sections.some((s) => s.fields.some(hasValues)))

// Check if there's anything to display.
const hasContent = computed(() => hasAnyFieldValues.value || (props.sections && hasAnySectionValues.value))
</script>

<template>
  <table v-if="hasContent" class="w-full table-auto border-collapse">
    <tbody>
      <!-- Top-level fields first (sorted by orderInList). -->
      <template v-for="field in sortedByOrder(fieldsData.fields)" :key="fieldKey(field)">
        <template v-if="hasValues(field)">
          <template v-for="(claim, cIndex) in displayedClaimsForField(field)" :key="claim.GetID()">
            <!--
              A HAS claim renders no value of its own, so its sub-fields table sits
              directly in the value cell of the label row: the first sub-field row
              aligns with the field's label instead of leaving an empty value line
              above it. The cell has no padding of its own so the nested table's
              label cells (px-2, like this table's value cells) align exactly with
              the values of sibling fields: with no value above them these are
              nested fields, not sub-fields of a value, so they get no indent.
            -->
            <tr v-if="claimTypeName(claim) === 'has' && field.subFields.length > 0 && claim.sub">
              <td v-if="cIndex === 0" class="w-1/5 px-2 py-1 align-top font-medium text-gray-700">
                <DocumentRefInline :id="field.propertyId" :link="false" />
              </td>
              <td v-else></td>
              <td class="p-0 align-top">
                <FieldsView :fields-data="{ sections: [], fields: field.subFields }" :claims="getSubClaims(claim.GetID())" :limited="limited" nested />
              </td>
            </tr>
            <template v-else>
              <tr>
                <td v-if="cIndex === 0" class="w-1/5 px-2 py-1 align-top font-medium text-gray-700">
                  <DocumentRefInline :id="field.propertyId" :link="false" />
                </td>
                <td v-else></td>
                <td class="px-2 py-1 align-top text-gray-700">
                  <ClaimValue :claim="claim" :type="claimTypeName(claim)" />
                </td>
              </tr>
              <!--
                Sub-fields for this claim value (recursive). In a top-level instance the
                sub-table sits in the value column (under the field's value, indented
                slightly right by this cell's px-2). In a nested instance it spans both
                columns instead, so deeper sub-fields indent under the sub-field's LABEL
                rather than its value, their own values landing slightly right of it.
              -->
              <tr v-if="field.subFields.length > 0 && claim.sub">
                <td v-if="!nested"></td>
                <td :colspan="nested ? 2 : 1" class="px-2 py-0 align-top">
                  <FieldsView :fields-data="{ sections: [], fields: field.subFields }" :claims="getSubClaims(claim.GetID())" :limited="limited" nested />
                </td>
              </tr>
            </template>
          </template>
          <tr v-if="hiddenClaimsCount(field) > 0">
            <td></td>
            <td class="px-2 py-1">
              <Button type="button" class="px-2.5 py-1" @click.prevent="expandField(field)">{{ t("common.buttons.showAll") }}</Button>
            </td>
          </tr>
        </template>
      </template>

      <!-- Sections (sorted by orderInList), only if sections prop is true. -->
      <template v-if="sections">
        <template v-for="section in sortedByOrder(fieldsData.sections)" :key="'section-' + section.id">
          <template v-if="section.fields.some(hasValues)">
            <tr>
              <th colspan="2" class="border-b border-slate-200 px-2 pt-4 pb-1 text-left text-lg font-semibold">{{ getSectionName(section as SectionData, locale) }}</th>
            </tr>
            <template v-for="field in sortedByOrder(section.fields)" :key="fieldKey(field)">
              <template v-if="hasValues(field)">
                <template v-for="(claim, cIndex) in displayedClaimsForField(field)" :key="claim.GetID()">
                  <!-- A HAS claim's sub-fields table sits directly in the label row's value cell, un-indented, see above. -->
                  <tr v-if="claimTypeName(claim) === 'has' && field.subFields.length > 0 && claim.sub">
                    <td v-if="cIndex === 0" class="w-1/5 px-2 py-1 align-top font-medium text-gray-700">
                      <DocumentRefInline :id="field.propertyId" :link="false" />
                    </td>
                    <td v-else></td>
                    <td class="p-0 align-top">
                      <FieldsView :fields-data="{ sections: [], fields: field.subFields }" :claims="getSubClaims(claim.GetID())" :limited="limited" nested />
                    </td>
                  </tr>
                  <template v-else>
                    <tr>
                      <td v-if="cIndex === 0" class="w-1/5 px-2 py-1 align-top font-medium text-gray-700">
                        <DocumentRefInline :id="field.propertyId" :link="false" />
                      </td>
                      <td v-else></td>
                      <td class="px-2 py-1 align-top text-gray-700">
                        <ClaimValue :claim="claim" :type="claimTypeName(claim)" />
                      </td>
                    </tr>
                    <!-- Sub-fields for this claim value (recursive), see above. -->
                    <tr v-if="field.subFields.length > 0 && claim.sub">
                      <td v-if="!nested"></td>
                      <td :colspan="nested ? 2 : 1" class="px-2 py-0 align-top">
                        <FieldsView :fields-data="{ sections: [], fields: field.subFields }" :claims="getSubClaims(claim.GetID())" :limited="limited" nested />
                      </td>
                    </tr>
                  </template>
                </template>
                <tr v-if="hiddenClaimsCount(field) > 0">
                  <td></td>
                  <td class="px-2 py-1">
                    <Button type="button" class="px-2.5 py-1" @click.prevent="expandField(field)">{{ t("common.buttons.showAll") }}</Button>
                  </td>
                </tr>
              </template>
            </template>
          </template>
        </template>
      </template>
    </tbody>
  </table>
</template>
